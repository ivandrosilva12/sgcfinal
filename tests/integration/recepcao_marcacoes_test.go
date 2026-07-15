//go:build integration

// Teste de integração do repositório pgx RepositorioMarcacoes (BC Recepção,
// Task 10). Segue o padrão de migracoes_test.go: ligar(t) só liga o pool,
// as migrações aplicam-se explicitamente a seguir, e SKIPa (nunca FAIL)
// quando DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// Ids fixos e distintos dos usados em recepcao_janelas_test.go — o BC Recepção
// não valida doente/médico/especialidade (referências textuais a outros
// bounded contexts, sem FK, ver migração 0001).
const (
	doenteMarcacaoID        = "33333333-3333-3333-3333-333333333333"
	medicoMarcacaoID        = "44444444-4444-4444-4444-444444444444"
	especialidadeMarcacaoID = "55555555-5555-5555-5555-555555555555"
)

func instMarcacao(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("data inválida: %v", err)
	}
	return v
}

func TestRecepcaoMarcacoesRepo_GuardarTransitarRemarcar(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	repo := pgrepo.NovoRepositorioMarcacoes(pool)

	m, err := dominio.NovaMarcacao(doenteMarcacaoID, medicoMarcacaoID, especialidadeMarcacaoID,
		instMarcacao(t, "2026-08-02T09:00:00Z"), instMarcacao(t, "2026-08-02T09:30:00Z"))
	if err != nil {
		t.Fatalf("construir marcação: %v", err)
	}
	id, err := repo.Guardar(ctx, m)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.marcacoes WHERE remarca_de=$1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.marcacoes WHERE id=$1`, id)
	})

	// remarcar: original → REMARCADA + nova MARCADA
	original, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("obter original: %v", err)
	}
	nova, err := original.Remarcar(instMarcacao(t, "2026-08-02T10:00:00Z"), instMarcacao(t, "2026-08-02T10:30:00Z"),
		instMarcacao(t, "2026-08-01T00:00:00Z"))
	if err != nil {
		t.Fatalf("remarcar (domínio): %v", err)
	}
	novoID, err := repo.Remarcar(ctx, original, nova)
	if err != nil {
		t.Fatalf("remarcar (repo): %v", err)
	}

	recarregada, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("obter original recarregada: %v", err)
	}
	if recarregada.Estado() != dominio.MarcRemarcada {
		t.Fatalf("original devia estar REMARCADA, veio %s", recarregada.Estado())
	}

	nv, err := repo.ObterPorID(ctx, novoID)
	if err != nil {
		t.Fatalf("obter nova: %v", err)
	}
	if nv.Estado() != dominio.MarcMarcada || nv.RemarcaDe() != id {
		t.Fatalf("nova mal gravada: estado=%s remarca_de=%s", nv.Estado(), nv.RemarcaDe())
	}
}

func TestRecepcaoMarcacoesRepo_ExcludeNegaSobreposicao(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	repo := pgrepo.NovoRepositorioMarcacoes(pool)

	m1, err := dominio.NovaMarcacao(doenteMarcacaoID, medicoMarcacaoID, especialidadeMarcacaoID,
		instMarcacao(t, "2026-08-03T09:00:00Z"), instMarcacao(t, "2026-08-03T09:30:00Z"))
	if err != nil {
		t.Fatalf("construir m1: %v", err)
	}
	id1, err := repo.Guardar(ctx, m1)
	if err != nil {
		t.Fatalf("guardar m1: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.marcacoes WHERE id=$1`, id1)
	})

	// sobreposta, mesmo médico, ambas MARCADA → a EXCLUDE tem de negar
	m2, err := dominio.NovaMarcacao(doenteMarcacaoID, medicoMarcacaoID, especialidadeMarcacaoID,
		instMarcacao(t, "2026-08-03T09:15:00Z"), instMarcacao(t, "2026-08-03T09:45:00Z"))
	if err != nil {
		t.Fatalf("construir m2: %v", err)
	}
	if _, err := repo.Guardar(ctx, m2); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("EXCLUDE devia negar com CategoriaConflito, veio %v (%v)", erros.CategoriaDe(err), err)
	}
}
