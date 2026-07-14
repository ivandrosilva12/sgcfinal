package pgrepo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioConsentimentos implementa dominio.RepositorioConsentimentos com pgx.
type RepositorioConsentimentos struct {
	pool *pgxpool.Pool
}

// NovoRepositorioConsentimentos constrói o repositório sobre o pool pgx.
func NovoRepositorioConsentimentos(pool *pgxpool.Pool) *RepositorioConsentimentos {
	return &RepositorioConsentimentos{pool: pool}
}

// Guardar insere (id vazio) ou actualiza a revogação (id presente).
func (r *RepositorioConsentimentos) Guardar(ctx context.Context, c *dominio.Consentimento) (string, error) {
	s := c.Snapshot()
	if s.ID == "" {
		const q = `
INSERT INTO clinico.consentimentos (doente_id, finalidade, concedido, documento_url, concedido_em, revogado_em)
VALUES ($1,$2,$3,NULLIF($4,''),$5,$6) RETURNING id::text`
		var id string
		err := r.pool.QueryRow(ctx, q, s.DoenteID, string(s.Finalidade), s.Concedido, s.DocumentoURL, s.ConcedidoEm, s.RevogadoEm).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("inserir consentimento: %w", err)
		}
		return id, nil
	}
	// Guarda compare-and-set: a revogação só se aplica a um consentimento ainda não
	// revogado. Duas revogações concorrentes lêem ambas um consentimento vigente e
	// passam ambas a guarda do domínio — sem esta condição, ambas escreviam e o
	// trilho de auditoria (imutável) ficava com dois `clinico.consentimento.revogado`
	// para o mesmo consentimento.
	const q = `UPDATE clinico.consentimentos SET revogado_em=$2 WHERE id=$1 AND revogado_em IS NULL`
	ct, err := r.pool.Exec(ctx, q, s.ID, s.RevogadoEm)
	if err != nil {
		return "", fmt.Errorf("actualizar consentimento: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", r.erroRevogacaoFalhada(ctx, s.ID)
	}
	return s.ID, nil
}

// erroRevogacaoFalhada distingue "o consentimento não existe" (NaoEncontrado/404)
// de "já estava revogado" (Conflito/409, corrida perdida).
func (r *RepositorioConsentimentos) erroRevogacaoFalhada(ctx context.Context, id string) error {
	const q = `SELECT EXISTS (SELECT 1 FROM clinico.consentimentos WHERE id=$1)`
	var existe bool
	if err := r.pool.QueryRow(ctx, q, id).Scan(&existe); err != nil {
		return fmt.Errorf("verificar consentimento: %w", err)
	}
	if !existe {
		return erros.Novo(erros.CategoriaNaoEncontrado, "consentimento não encontrado")
	}
	return erros.Novo(erros.CategoriaConflito, "o consentimento já foi revogado entretanto")
}

// ObterPorID reconstrói o consentimento. NaoEncontrado se não existir.
func (r *RepositorioConsentimentos) ObterPorID(ctx context.Context, id string) (*dominio.Consentimento, error) {
	const q = `
SELECT id::text, doente_id::text, finalidade, concedido, COALESCE(documento_url,''),
       concedido_em, revogado_em, criado_em
FROM clinico.consentimentos WHERE id=$1`
	var s dominio.SnapshotConsentimento
	var fin string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.DoenteID, &fin, &s.Concedido, &s.DocumentoURL,
		&s.ConcedidoEm, &s.RevogadoEm, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "consentimento não encontrado")
		}
		return nil, fmt.Errorf("obter consentimento: %w", err)
	}
	s.Finalidade = dominio.Finalidade(fin)
	return dominio.ReconstruirConsentimento(s), nil
}

// ListarPorDoente devolve os consentimentos do doente segundo o filtro.
func (r *RepositorioConsentimentos) ListarPorDoente(ctx context.Context, doenteID string, filtro dominio.FiltroConsentimentos) ([]dominio.ResumoConsentimento, error) {
	q := `
SELECT id::text, doente_id::text, finalidade, concedido, COALESCE(documento_url,''),
       concedido_em, revogado_em, (concedido AND revogado_em IS NULL) AS vigente
FROM clinico.consentimentos WHERE doente_id=$1`
	args := []any{doenteID}
	if f := strings.TrimSpace(filtro.Finalidade); f != "" {
		args = append(args, strings.ToUpper(f))
		q += fmt.Sprintf(" AND finalidade=$%d", len(args))
	}
	if filtro.ApenasVigentes {
		q += " AND concedido AND revogado_em IS NULL"
	}
	q += " ORDER BY concedido_em DESC"
	linhas, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listar consentimentos: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoConsentimento{}
	for linhas.Next() {
		var rc dominio.ResumoConsentimento
		if err := linhas.Scan(&rc.ID, &rc.DoenteID, &rc.Finalidade, &rc.Concedido, &rc.DocumentoURL,
			&rc.ConcedidoEm, &rc.RevogadoEm, &rc.Vigente); err != nil {
			return nil, fmt.Errorf("ler consentimento: %w", err)
		}
		out = append(out, rc)
	}
	return out, linhas.Err()
}
