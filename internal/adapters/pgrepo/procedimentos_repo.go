package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioProcedimentos implementa dominio.RepositorioProcedimentos com pgx.
type RepositorioProcedimentos struct {
	pool *pgxpool.Pool
}

// NovoRepositorioProcedimentos constrói o repositório sobre o pool pgx.
func NovoRepositorioProcedimentos(pool *pgxpool.Pool) *RepositorioProcedimentos {
	return &RepositorioProcedimentos{pool: pool}
}

// Guardar insere (id vazio) ou actualiza a transição de estado (id presente).
func (r *RepositorioProcedimentos) Guardar(ctx context.Context, p *dominio.ProcedimentoCirurgico) (string, error) {
	s := p.Snapshot()
	if s.ID == "" {
		const q = `
INSERT INTO clinico.procedimentos_cirurgicos (
    episodio_id, codigo_procedimento, descricao, sala, cirurgiao_id, auxiliar_id,
    anestesia, anestesista_id, consentimento_id, observacoes, estado
) VALUES (
    $1,$2,$3,NULLIF($4,''),$5,NULLIF($6,'')::uuid,
    $7,NULLIF($8,'')::uuid,$9,NULLIF($10,''),$11
) RETURNING id::text`
		var id string
		err := r.pool.QueryRow(ctx, q,
			s.EpisodioID, s.Codigo, s.Descricao, s.Sala, s.CirurgiaoID, s.AuxiliarID,
			string(s.Anestesia), s.AnestesistaID, s.ConsentimentoID, s.Observacoes, string(s.Estado),
		).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("inserir procedimento: %w", err)
		}
		return id, nil
	}
	// Transição de estado: escreve apenas estado/inicio/fim/complicacoes/observacoes —
	// nunca as colunas de identidade do procedimento (episodio_id, codigo, consentimento_id,
	// cirurgiao_id, etc.), que não fazem parte de nenhuma transição.
	const q = `
UPDATE clinico.procedimentos_cirurgicos SET
    estado=$2, inicio=$3, fim=$4, complicacoes=NULLIF($5,''), observacoes=NULLIF($6,'')
WHERE id=$1`
	ct, err := r.pool.Exec(ctx, q, s.ID, string(s.Estado), s.Inicio, s.Fim, s.Complicacoes, s.Observacoes)
	if err != nil {
		return "", fmt.Errorf("actualizar procedimento: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", erros.Novo(erros.CategoriaNaoEncontrado, "procedimento não encontrado")
	}
	return s.ID, nil
}

// ObterPorID reconstrói o procedimento. NaoEncontrado se não existir.
func (r *RepositorioProcedimentos) ObterPorID(ctx context.Context, id string) (*dominio.ProcedimentoCirurgico, error) {
	const q = `
SELECT id::text, episodio_id::text, codigo_procedimento, descricao, COALESCE(sala,''),
       cirurgiao_id::text, COALESCE(auxiliar_id::text,''), anestesia, COALESCE(anestesista_id::text,''),
       inicio, fim, consentimento_id::text, COALESCE(complicacoes,''), COALESCE(observacoes,''),
       estado, criado_em
FROM clinico.procedimentos_cirurgicos WHERE id=$1`
	var s dominio.SnapshotProcedimento
	var anestesia, estado string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.EpisodioID, &s.Codigo, &s.Descricao, &s.Sala,
		&s.CirurgiaoID, &s.AuxiliarID, &anestesia, &s.AnestesistaID, &s.Inicio, &s.Fim,
		&s.ConsentimentoID, &s.Complicacoes, &s.Observacoes, &estado, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "procedimento não encontrado")
		}
		return nil, fmt.Errorf("obter procedimento: %w", err)
	}
	s.Anestesia = dominio.Anestesia(anestesia)
	s.Estado = dominio.EstadoProcedimento(estado)
	return dominio.ReconstruirProcedimento(s), nil
}

// ListarPorEpisodio devolve os procedimentos do episódio (mais recentes primeiro).
func (r *RepositorioProcedimentos) ListarPorEpisodio(ctx context.Context, episodioID string) ([]dominio.ResumoProcedimento, error) {
	const q = `
SELECT id::text, episodio_id::text, codigo_procedimento, descricao, estado, anestesia,
       inicio, fim, criado_em
FROM clinico.procedimentos_cirurgicos WHERE episodio_id=$1 ORDER BY criado_em DESC`
	linhas, err := r.pool.Query(ctx, q, episodioID)
	if err != nil {
		return nil, fmt.Errorf("listar procedimentos: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoProcedimento{}
	for linhas.Next() {
		var rp dominio.ResumoProcedimento
		if err := linhas.Scan(&rp.ID, &rp.EpisodioID, &rp.Codigo, &rp.Descricao, &rp.Estado, &rp.Anestesia,
			&rp.Inicio, &rp.Fim, &rp.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler procedimento: %w", err)
		}
		out = append(out, rp)
	}
	return out, linhas.Err()
}
