//go:build integration

// Testes de integração da fundação: runner de migrations forward-only,
// imutabilidade do audit log e seed dos papéis. Exigem um PostgreSQL acessível
// via DATABASE_MIGRATION_URL (migrador) e DATABASE_URL (runtime) — ver ADR-043.
// Corridos com: go test -tags=integration ./tests/integration/...
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

// ligar liga com a credencial de MIGRAÇÃO (sgc): tem DDL, é o que os testes de
// integração precisam para aplicar migrations e montar cenários.
func ligar(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	return ligarCom(t, "DATABASE_MIGRATION_URL")
}

// ligarApp liga com a credencial de RUNTIME (sgc_app): sem DDL, sem posse das
// tabelas. É com esta que se provam as garantias da ADR-043.
func ligarApp(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	return ligarCom(t, "DATABASE_URL")
}

func ligarCom(t *testing.T, chave string) (*pgxpool.Pool, context.Context) {
	t.Helper()
	runtime := os.Getenv("DATABASE_URL")
	migracao := os.Getenv("DATABASE_MIGRATION_URL")

	// Nada configurado: a suite salta, como sempre saltou.
	if runtime == "" && migracao == "" {
		t.Skip("DATABASE_URL/DATABASE_MIGRATION_URL não definidos; a saltar testes de integração")
	}
	// Configuração pela metade é erro, não motivo para saltar. Uma suite que se
	// cala quando devia correr é exactamente o modo de falha que a ADR-042
	// apanhou: provas a passar a verde pela razão errada.
	if runtime == "" || migracao == "" {
		t.Fatal("configuração pela metade: DATABASE_URL e DATABASE_MIGRATION_URL têm de estar " +
			"ambas definidas (ADR-043)")
	}

	ctx := context.Background()
	pool, err := db.LigarPool(ctx, os.Getenv(chave))
	if err != nil {
		t.Fatalf("ligar ao PostgreSQL com %s: %v", chave, err)
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

	// O Tesoureiro passou a sensível com a emissão (ADR-040, revisão da
	// ERRATA-002). O seed e a migração forward-only 0006 têm de concordar com
	// o enum do domínio (identidade.EhSensivel).
	var sensivel bool
	if err := pool.QueryRow(ctx,
		`SELECT sensivel FROM identidade.papeis WHERE codigo = 'Tesoureiro'`).Scan(&sensivel); err != nil {
		t.Fatalf("ler sensibilidade do Tesoureiro: %v", err)
	}
	if !sensivel {
		t.Fatal("Tesoureiro devia estar marcado como sensível (ADR-040)")
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
