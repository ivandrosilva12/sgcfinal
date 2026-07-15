// internal/adapters/pgrepo/chegadas_repo.go
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// codigoUnicoPG é o SQLSTATE de violação de restrição UNIQUE.
const codigoUnicoPG = "23505"

// RepositorioChegadas implementa dominio.RepositorioChegadas com pgx.
type RepositorioChegadas struct {
	pool *pgxpool.Pool
}

// NovoRepositorioChegadas constrói o repositório sobre o pool pgx.
func NovoRepositorioChegadas(pool *pgxpool.Pool) *RepositorioChegadas {
	return &RepositorioChegadas{pool: pool}
}

const colunasChegada = `id::text, doente_id::text, COALESCE(marcacao_id::text,''),
       especialidade_id::text, COALESCE(medico_id::text,''), hora_chegada, estado,
       criado_em, actualizado_em`

// Guardar insere uma chegada (walk-in) e devolve o id gerado.
func (r *RepositorioChegadas) Guardar(ctx context.Context, c *dominio.Chegada) (string, error) {
	return r.inserir(ctx, r.pool, c)
}

func (r *RepositorioChegadas) inserir(ctx context.Context, q querier, c *dominio.Chegada) (string, error) {
	s := c.Snapshot()
	const sql = `
INSERT INTO recepcao.chegadas
    (doente_id, marcacao_id, especialidade_id, medico_id, hora_chegada, estado)
VALUES ($1::uuid, NULLIF($2,'')::uuid, $3::uuid, NULLIF($4,'')::uuid, $5, $6)
RETURNING id::text`
	var id string
	err := q.QueryRow(ctx, sql, s.DoenteID, s.MarcacaoID, s.EspecialidadeID, s.MedicoID,
		s.HoraChegada, string(s.Estado)).Scan(&id)
	if err != nil {
		if ehUnica(err) {
			return "", erros.Novo(erros.CategoriaConflito, "já existe uma chegada para esta marcação")
		}
		return "", fmt.Errorf("guardar chegada: %w", err)
	}
	return id, nil
}

// RegistarChegadaAgendada grava, numa única transacção, a marcação a passar a
// COMPARECEU (guarda compare-and-set sobre MARCADA) e a nova chegada.
func (r *RepositorioChegadas) RegistarChegadaAgendada(ctx context.Context, chegada *dominio.Chegada, marcacao *dominio.Marcacao) (string, error) {
	sm := marcacao.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção de check-in: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	const upd = `UPDATE recepcao.marcacoes SET estado=$2, actualizado_em=$3 WHERE id=$1 AND estado=$4`
	ct, err := tx.Exec(ctx, upd, sm.ID, string(sm.Estado), sm.ActualizadoEm, string(sm.EstadoAnterior))
	if err != nil {
		return "", fmt.Errorf("marcar comparência: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", r.erroTransicaoMarcacao(ctx, sm.ID)
	}
	id, err := r.inserir(ctx, tx, chegada)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar check-in: %w", err)
	}
	return id, nil
}

// ObterPorID reconstrói a chegada. NaoEncontrado se não existir.
func (r *RepositorioChegadas) ObterPorID(ctx context.Context, id string) (*dominio.Chegada, error) {
	q := `SELECT ` + colunasChegada + ` FROM recepcao.chegadas WHERE id=$1`
	var s dominio.SnapshotChegada
	var estado string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.DoenteID, &s.MarcacaoID, &s.EspecialidadeID,
		&s.MedicoID, &s.HoraChegada, &estado, &s.CriadoEm, &s.ActualizadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "chegada não encontrada")
		}
		return nil, fmt.Errorf("obter chegada: %w", err)
	}
	s.Estado = dominio.EstadoChegada(estado)
	return dominio.ReconstruirChegada(s), nil
}

// Transitar aplica a transição de estado da chegada com guarda compare-and-set.
func (r *RepositorioChegadas) Transitar(ctx context.Context, c *dominio.Chegada) error {
	s := c.Snapshot()
	if s.ID == "" {
		return erros.Novo(erros.CategoriaNaoEncontrado, "chegada não encontrada")
	}
	const q = `UPDATE recepcao.chegadas SET estado=$2, actualizado_em=$3 WHERE id=$1 AND estado=$4`
	ct, err := r.pool.Exec(ctx, q, s.ID, string(s.Estado), s.ActualizadoEm, string(s.EstadoAnterior))
	if err != nil {
		return fmt.Errorf("actualizar chegada: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return r.erroTransicaoChegada(ctx, s.ID)
	}
	return nil
}

// ListarFila devolve as chegadas em AGUARDA, ordenadas por hora de chegada (FIFO).
// Especialidade vazia = todas.
func (r *RepositorioChegadas) ListarFila(ctx context.Context, especialidadeID string) ([]dominio.ResumoChegada, error) {
	const q = `
SELECT id::text, doente_id::text, COALESCE(marcacao_id::text,''), COALESCE(medico_id::text,''),
       especialidade_id::text, estado, hora_chegada
FROM recepcao.chegadas
WHERE estado='AGUARDA' AND ($1='' OR especialidade_id=NULLIF($1,'')::uuid)
ORDER BY hora_chegada, id`
	linhas, err := r.pool.Query(ctx, q, especialidadeID)
	if err != nil {
		return nil, fmt.Errorf("listar fila: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoChegada{}
	for linhas.Next() {
		var rc dominio.ResumoChegada
		if err := linhas.Scan(&rc.ID, &rc.DoenteID, &rc.MarcacaoID, &rc.MedicoID,
			&rc.EspecialidadeID, &rc.Estado, &rc.HoraChegada); err != nil {
			return nil, fmt.Errorf("ler chegada: %w", err)
		}
		out = append(out, rc)
	}
	return out, linhas.Err()
}

func (r *RepositorioChegadas) erroTransicaoChegada(ctx context.Context, id string) error {
	return r.erroTransicao(ctx, "recepcao.chegadas", "chegada", id)
}

func (r *RepositorioChegadas) erroTransicaoMarcacao(ctx context.Context, id string) error {
	return r.erroTransicao(ctx, "recepcao.marcacoes", "marcação", id)
}

// erroTransicao distingue 404 (linha inexistente) de 409 (estado mudou).
func (r *RepositorioChegadas) erroTransicao(ctx context.Context, tabela, substantivo, id string) error {
	var existe bool
	q := `SELECT EXISTS (SELECT 1 FROM ` + tabela + ` WHERE id=$1)`
	if err := r.pool.QueryRow(ctx, q, id).Scan(&existe); err != nil {
		return fmt.Errorf("verificar %s: %w", substantivo, err)
	}
	if !existe {
		return erros.Novo(erros.CategoriaNaoEncontrado, substantivo+" não encontrada")
	}
	return erros.Novo(erros.CategoriaConflito,
		"o estado da "+substantivo+" mudou entretanto; recarregue e repita a operação")
}

// ehUnica indica se o erro é uma violação de restrição UNIQUE.
func ehUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoUnicoPG
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioChegadas = (*RepositorioChegadas)(nil)
