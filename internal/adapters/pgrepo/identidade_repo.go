// Package pgrepo contém as implementações de repositório sobre PostgreSQL via
// pgx v5 (SQL puro, sem ORM). Camada 3 — Adaptadores.
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioUtilizadores implementa dominio.RepositorioUtilizadores com pgx.
type RepositorioUtilizadores struct {
	pool *pgxpool.Pool
}

// NovoRepositorioUtilizadores constrói o repositório sobre o pool pgx.
func NovoRepositorioUtilizadores(pool *pgxpool.Pool) *RepositorioUtilizadores {
	return &RepositorioUtilizadores{pool: pool}
}

// ObterPorID devolve o utilizador e os seus papéis. Devolve ErroDominio de
// categoria NaoEncontrado se não existir.
func (r *RepositorioUtilizadores) ObterPorID(ctx context.Context, keycloakID string) (*dominio.Utilizador, error) {
	const q = `
SELECT keycloak_id::text, nome, email, COALESCE(telefone, ''), COALESCE(bi, ''), activo
FROM identidade.utilizadores
WHERE keycloak_id = $1`

	var u dominio.Utilizador
	err := r.pool.QueryRow(ctx, q, keycloakID).
		Scan(&u.KeycloakID, &u.Nome, &u.Email, &u.Telefone, &u.BI, &u.Activo)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "utilizador não encontrado")
	}
	if err != nil {
		return nil, fmt.Errorf("obter utilizador: %w", err)
	}

	papeis, err := r.papeisDe(ctx, keycloakID)
	if err != nil {
		return nil, err
	}
	u.Papeis = papeis
	return &u, nil
}

// papeisDe lê os papéis atribuídos a um utilizador, por ordem estável.
func (r *RepositorioUtilizadores) papeisDe(ctx context.Context, keycloakID string) ([]dominio.Papel, error) {
	const q = `
SELECT papel_codigo
FROM identidade.utilizadores_papeis
WHERE utilizador_id = $1
ORDER BY papel_codigo`

	rows, err := r.pool.Query(ctx, q, keycloakID)
	if err != nil {
		return nil, fmt.Errorf("obter papéis: %w", err)
	}
	defer rows.Close()

	var papeis []dominio.Papel
	for rows.Next() {
		var codigo string
		if err := rows.Scan(&codigo); err != nil {
			return nil, fmt.Errorf("ler papel: %w", err)
		}
		papeis = append(papeis, dominio.Papel(codigo))
	}
	return papeis, rows.Err()
}

// GuardarComPapeis faz o upsert do utilizador e sincroniza os seus papéis numa
// única transacção. No conflito por keycloak_id, actualiza nome/email/activo
// (o Keycloak é a fonte de verdade) mas preserva telefone/bi definidos
// localmente. Suporta o provisionamento JIT.
func (r *RepositorioUtilizadores) GuardarComPapeis(ctx context.Context, u *dominio.Utilizador) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const upsert = `
INSERT INTO identidade.utilizadores (keycloak_id, nome, email, telefone, bi, activo)
VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6)
ON CONFLICT (keycloak_id) DO UPDATE
SET nome = EXCLUDED.nome,
    email = EXCLUDED.email,
    activo = EXCLUDED.activo,
    actualizado_em = now()`

	if _, err := tx.Exec(ctx, upsert,
		u.KeycloakID, u.Nome, u.Email, u.Telefone, u.BI, u.Activo); err != nil {
		return fmt.Errorf("upsert utilizador: %w", err)
	}

	// Sincronizar papéis: substituir o conjunto pelo actual (idempotente).
	if _, err := tx.Exec(ctx,
		`DELETE FROM identidade.utilizadores_papeis WHERE utilizador_id = $1`, u.KeycloakID); err != nil {
		return fmt.Errorf("limpar papéis: %w", err)
	}
	for _, p := range u.Papeis {
		if _, err := tx.Exec(ctx,
			`INSERT INTO identidade.utilizadores_papeis (utilizador_id, papel_codigo) VALUES ($1, $2)`,
			u.KeycloakID, string(p)); err != nil {
			return fmt.Errorf("atribuir papel %q: %w", p, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar transacção: %w", err)
	}
	return nil
}
