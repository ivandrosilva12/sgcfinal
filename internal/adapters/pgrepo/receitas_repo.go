package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioReceitas implementa dominio.RepositorioReceitas com pgx.
type RepositorioReceitas struct {
	pool *pgxpool.Pool
}

// NovoRepositorioReceitas constrói o repositório sobre o pool pgx.
func NovoRepositorioReceitas(pool *pgxpool.Pool) *RepositorioReceitas {
	return &RepositorioReceitas{pool: pool}
}

// Guardar persiste a receita e os seus itens numa transacção.
func (r *RepositorioReceitas) Guardar(ctx context.Context, rec *dominio.Receita) (string, error) {
	s := rec.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := s.ID
	if id == "" {
		const q = `
INSERT INTO farmacia.receitas (episodio_id, doente_id, medico_id, emitida_em, estado, notas, expira_em)
VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7) RETURNING id::text`
		if err := tx.QueryRow(ctx, q, s.EpisodioID, s.DoenteID, s.MedicoID, s.EmitidaEm, string(s.Estado), s.Notas, s.ExpiraEm).Scan(&id); err != nil {
			return "", fmt.Errorf("inserir receita: %w", err)
		}
	} else {
		const q = `
UPDATE farmacia.receitas SET episodio_id=$2, doente_id=$3, medico_id=$4, emitida_em=$5,
    estado=$6, notas=NULLIF($7,''), expira_em=$8 WHERE id=$1`
		ct, err := tx.Exec(ctx, q, id, s.EpisodioID, s.DoenteID, s.MedicoID, s.EmitidaEm, string(s.Estado), s.Notas, s.ExpiraEm)
		if err != nil {
			return "", fmt.Errorf("actualizar receita: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return "", erros.Novo(erros.CategoriaNaoEncontrado, "receita não encontrada")
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM farmacia.itens_receita WHERE receita_id=$1`, id); err != nil {
		return "", fmt.Errorf("limpar itens: %w", err)
	}
	for _, it := range s.Itens {
		if _, err := tx.Exec(ctx,
			`INSERT INTO farmacia.itens_receita (receita_id, medicamento_id, posologia, duracao_dias, quantidade_prescrita, quantidade_dispensada, notas)
			 VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''))`,
			id, it.MedicamentoID, it.Posologia, it.DuracaoDias, it.QuantidadePrescrita, it.QuantidadeDispensada, it.Notas); err != nil {
			return "", fmt.Errorf("inserir item: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar transacção: %w", err)
	}
	return id, nil
}

// ObterPorID devolve a receita com os itens. NaoEncontrado se não existir.
func (r *RepositorioReceitas) ObterPorID(ctx context.Context, id string) (*dominio.Receita, error) {
	const q = `
SELECT id::text, episodio_id::text, doente_id::text, medico_id::text, emitida_em, estado,
       COALESCE(notas,''), expira_em
FROM farmacia.receitas WHERE id=$1`
	var s dominio.SnapshotReceita
	var estado string
	if err := r.pool.QueryRow(ctx, q, id).Scan(
		&s.ID, &s.EpisodioID, &s.DoenteID, &s.MedicoID, &s.EmitidaEm, &estado, &s.Notas, &s.ExpiraEm,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "receita não encontrada")
		}
		return nil, fmt.Errorf("obter receita: %w", err)
	}
	s.Estado = dominio.EstadoReceita(estado)
	itens, err := r.carregarItens(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	s.Itens = itens
	return dominio.ReconstruirReceita(s), nil
}

func (r *RepositorioReceitas) carregarItens(ctx context.Context, id string) ([]dominio.ItemReceita, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT medicamento_id::text, posologia, duracao_dias, quantidade_prescrita, quantidade_dispensada, COALESCE(notas,'')
		 FROM farmacia.itens_receita WHERE receita_id=$1 ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("carregar itens: %w", err)
	}
	defer linhas.Close()
	var out []dominio.ItemReceita
	for linhas.Next() {
		var it dominio.ItemReceita
		if err := linhas.Scan(&it.MedicamentoID, &it.Posologia, &it.DuracaoDias, &it.QuantidadePrescrita, &it.QuantidadeDispensada, &it.Notas); err != nil {
			return nil, fmt.Errorf("ler item: %w", err)
		}
		out = append(out, it)
	}
	return out, linhas.Err()
}

// ListarPorDoente devolve uma página das receitas do doente, mais recentes primeiro.
func (r *RepositorioReceitas) ListarPorDoente(ctx context.Context, f dominio.FiltroReceitas) (dominio.PaginaReceitas, error) {
	base := `FROM farmacia.receitas r WHERE r.doente_id::text=$1 AND ($2='' OR r.episodio_id::text=$2) AND ($3='' OR r.estado=$3)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+base, f.DoenteID, f.EpisodioID, f.Estado).Scan(&total); err != nil {
		return dominio.PaginaReceitas{}, fmt.Errorf("contar receitas: %w", err)
	}
	q := `SELECT r.id::text, r.episodio_id::text, r.medico_id::text, r.emitida_em, r.estado, r.expira_em,
	         (SELECT count(*) FROM farmacia.itens_receita i WHERE i.receita_id=r.id) ` +
		base + ` ORDER BY r.emitida_em DESC LIMIT $4 OFFSET $5`
	linhas, err := r.pool.Query(ctx, q, f.DoenteID, f.EpisodioID, f.Estado, f.Limite, f.Deslocamento)
	if err != nil {
		return dominio.PaginaReceitas{}, fmt.Errorf("listar receitas: %w", err)
	}
	defer linhas.Close()
	pagina := dominio.PaginaReceitas{Total: total, Limite: f.Limite, Deslocamento: f.Deslocamento, Itens: []dominio.ResumoReceita{}}
	for linhas.Next() {
		var it dominio.ResumoReceita
		if err := linhas.Scan(&it.ID, &it.EpisodioID, &it.MedicoID, &it.EmitidaEm, &it.Estado, &it.ExpiraEm, &it.NumItens); err != nil {
			return dominio.PaginaReceitas{}, fmt.Errorf("ler resumo de receita: %w", err)
		}
		pagina.Itens = append(pagina.Itens, it)
	}
	return pagina, linhas.Err()
}
