// Package db gere o acesso a PostgreSQL: o pool de ligações pgx (pgxpool) e o
// runner de migrations forward-only. Camada 4 — Plataforma.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LigarPool cria um pgxpool a partir de um DSN e valida a ligação com Ping.
func LigarPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("DSN de PostgreSQL inválido: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("criar pool pgx: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("PostgreSQL indisponível: %w", err)
	}
	return pool, nil
}
