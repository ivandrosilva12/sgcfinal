//go:build integration

// Teste de integração do BC Farmácia (catálogo de medicamentos) contra a BD
// real. Segue o padrão de doentes_test.go: SKIP (nunca FAIL) quando
// DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestRepositorioMedicamentos_CicloCompleto(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repo := pgrepo.NovoRepositorioMedicamentos(pool)

	cod, err := repo.ProximoCodigo(ctx)
	if err != nil || cod[:4] != "MED-" {
		t.Fatalf("próximo código: %v (%q)", err, cod)
	}
	m, _ := dominio.NovoMedicamento(cod, "Amoxil Integração 500", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "GSK", true, false, nil, 10)
	id, err := repo.Guardar(ctx, m)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM farmacia.medicamentos WHERE id=$1`, id) })

	lido, err := repo.ObterPorID(ctx, id)
	if err != nil || lido.CodigoInterno() != cod {
		t.Fatalf("obter falhou: %v", err)
	}

	pag, err := repo.Pesquisar(ctx, dominio.FiltroMedicamentos{Termo: "Integração", ApenasActivos: true, Limite: 10})
	if err != nil || pag.Total < 1 {
		t.Fatalf("pesquisar falhou: %v (total=%d)", err, pag.Total)
	}

	// Código duplicado → conflito.
	dup, _ := dominio.NovoMedicamento(cod, "Outro", "Outro", "COMPRIMIDO", "1 g", "ORAL", "", true, false, nil, 5)
	if _, err := repo.Guardar(ctx, dup); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito de código, obtive %v", err)
	}
}
