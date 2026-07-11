package pgrepo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// RepositorioAuditoria persiste registos de auditoria de forma append-only em
// auditoria.auditoria_eventos (a imutabilidade é garantida por trigger PG).
// Implementa a porta application/identidade.Auditor.
type RepositorioAuditoria struct {
	pool *pgxpool.Pool
}

// NovoRepositorioAuditoria constrói o repositório sobre o pool pgx.
func NovoRepositorioAuditoria(pool *pgxpool.Pool) *RepositorioAuditoria {
	return &RepositorioAuditoria{pool: pool}
}

// Registar insere um evento de auditoria. Só faz INSERT — nunca UPDATE/DELETE.
func (r *RepositorioAuditoria) Registar(ctx context.Context, reg auditoria.Registo) error {
	const q = `
INSERT INTO auditoria.auditoria_eventos (actor, accao, entidade, entidade_id, detalhe, ocorrido_em)
VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), COALESCE(NULLIF($5, '')::jsonb, '{}'::jsonb), $6)`

	if _, err := r.pool.Exec(ctx, q,
		reg.Actor, reg.Accao, reg.Entidade, reg.EntidadeID, reg.Detalhe, reg.OcorridoEm); err != nil {
		return fmt.Errorf("registar auditoria: %w", err)
	}
	return nil
}
