package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AplicarMigracoes executa, de forma forward-only e idempotente, todas as
// migrations SQL presentes em fsys, organizadas por bounded context
// (subdirectório). Cada ficheiro é aplicado uma única vez, dentro de uma
// transacção própria, e registado em public.schema_migrations. A ordem é
// determinística: bounded contexts por ordem alfabética, ficheiros por ordem
// numérica do nome.
func AplicarMigracoes(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, logger *slog.Logger) error {
	if err := garantirTabelaControlo(ctx, pool); err != nil {
		return err
	}

	bcs, err := boundedContexts(fsys)
	if err != nil {
		return err
	}

	total := 0
	for _, bc := range bcs {
		ficheiros, err := ficheirosSQL(fsys, bc)
		if err != nil {
			return err
		}
		for _, fich := range ficheiros {
			versao := strings.TrimSuffix(fich, ".sql")
			aplicada, err := jaAplicada(ctx, pool, bc, versao)
			if err != nil {
				return err
			}
			if aplicada {
				continue
			}
			conteudo, err := fs.ReadFile(fsys, path.Join(bc, fich))
			if err != nil {
				return fmt.Errorf("ler migration %s/%s: %w", bc, fich, err)
			}
			if err := aplicarUma(ctx, pool, bc, versao, string(conteudo)); err != nil {
				return err
			}
			total++
			if logger != nil {
				logger.Info("migration aplicada", "bounded_context", bc, "versao", versao)
			}
		}
	}

	if logger != nil {
		logger.Info("migrations concluídas", "aplicadas_agora", total)
	}
	return nil
}

func garantirTabelaControlo(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    bounded_context text        NOT NULL,
    versao          text        NOT NULL,
    aplicada_em     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (bounded_context, versao)
);`
	if _, err := pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("criar schema_migrations: %w", err)
	}
	return nil
}

func boundedContexts(fsys fs.FS) ([]string, error) {
	entradas, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("listar bounded contexts: %w", err)
	}
	var bcs []string
	for _, e := range entradas {
		if e.IsDir() {
			bcs = append(bcs, e.Name())
		}
	}
	sort.Strings(bcs)
	return bcs, nil
}

func ficheirosSQL(fsys fs.FS, bc string) ([]string, error) {
	entradas, err := fs.ReadDir(fsys, bc)
	if err != nil {
		return nil, fmt.Errorf("listar migrations de %s: %w", bc, err)
	}
	var ficheiros []string
	for _, e := range entradas {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			ficheiros = append(ficheiros, e.Name())
		}
	}
	sort.Strings(ficheiros)
	return ficheiros, nil
}

func jaAplicada(ctx context.Context, pool *pgxpool.Pool, bc, versao string) (bool, error) {
	var existe bool
	const q = `SELECT EXISTS(
        SELECT 1 FROM public.schema_migrations WHERE bounded_context = $1 AND versao = $2
    )`
	if err := pool.QueryRow(ctx, q, bc, versao).Scan(&existe); err != nil {
		return false, fmt.Errorf("verificar migration %s/%s: %w", bc, versao, err)
	}
	return existe, nil
}

func aplicarUma(ctx context.Context, pool *pgxpool.Pool, bc, versao, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciar transacção para %s/%s: %w", bc, versao, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("aplicar migration %s/%s: %w", bc, versao, err)
	}
	const ins = `INSERT INTO public.schema_migrations (bounded_context, versao) VALUES ($1, $2)`
	if _, err := tx.Exec(ctx, ins, bc, versao); err != nil {
		return fmt.Errorf("registar migration %s/%s: %w", bc, versao, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar migration %s/%s: %w", bc, versao, err)
	}
	return nil
}
