//go:build integration

// Testes de integração da fundação: runner de migrations forward-only,
// imutabilidade do audit log e seed dos papéis. Exigem um PostgreSQL acessível
// via DATABASE_URL. Corridos com: go test -tags=integration ./tests/integration/...
package integration_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func ligar(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL não definido; a saltar testes de integração")
	}
	ctx := context.Background()
	pool, err := db.LigarPool(ctx, dsn)
	if err != nil {
		t.Fatalf("ligar ao PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, ctx
}

func contarMigracoes(t *testing.T, pool *pgxpool.Pool, ctx context.Context) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("contar schema_migrations: %v", err)
	}
	return n
}

func TestMigracoes_AplicamESaoIdempotentes(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("primeira aplicação falhou: %v", err)
	}
	primeira := contarMigracoes(t, pool, ctx)
	if primeira == 0 {
		t.Fatal("esperava schema_migrations populado")
	}

	// Idempotência: reexecutar não deve aplicar nem registar nada de novo.
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("segunda aplicação falhou: %v", err)
	}
	if segunda := contarMigracoes(t, pool, ctx); segunda != primeira {
		t.Fatalf("não idempotente: %d → %d", primeira, segunda)
	}
}

func TestAuditoria_Imutavel(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO auditoria.auditoria_eventos (actor, accao) VALUES ($1, $2)`,
		"tester", "integracao.teste"); err != nil {
		t.Fatalf("INSERT devia ser permitido: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE auditoria.auditoria_eventos SET accao = 'x'`); err == nil {
		t.Fatal("UPDATE em auditoria_eventos devia ser rejeitado pelo trigger")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM auditoria.auditoria_eventos`); err == nil {
		t.Fatal("DELETE em auditoria_eventos devia ser rejeitado pelo trigger")
	}
}

func TestSeed_DozePapeis(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	seed, err := os.ReadFile("../../seeds/papeis.sql")
	if err != nil {
		t.Fatalf("ler seed: %v", err)
	}
	if _, err := pool.Exec(ctx, string(seed)); err != nil {
		t.Fatalf("aplicar seed: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identidade.papeis`).Scan(&n); err != nil {
		t.Fatalf("contar papéis: %v", err)
	}
	if n != 12 {
		t.Fatalf("esperava 12 papéis (DDM-001 + Tesoureiro/ERRATA-002), obtive %d", n)
	}
}

func TestOutbox_TemColunasDeReentrega(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("aplicar migrations: %v", err)
	}
	var n int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='shared' AND table_name='outbox'
		AND column_name IN ('tentativas','ultimo_erro')`).Scan(&n)
	if err != nil {
		t.Fatalf("consultar colunas: %v", err)
	}
	if n != 2 {
		t.Fatalf("esperava 2 colunas novas no outbox, obtive %d", n)
	}
}
