//go:build integration

// Teste de integração dos consentimentos LPDP contra a BD real. Segue o padrão
// de doentes_test.go: SKIP (nunca FAIL) quando DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestRepositorioConsentimentos_CicloCompleto(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	repo := pgrepo.NovoRepositorioConsentimentos(pool)

	// Doente mínimo via agregado de domínio (mesmo padrão de doentes_test.go).
	nasc := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
	bi := "00234567LA043"
	num, err := repoDoentes.ProximoNumeroProcesso(ctx, 2026)
	if err != nil {
		t.Fatalf("próximo número: %v", err)
	}
	ident, _ := dominio.NovaIdentificacao("Teste Consentimento", nasc, dominio.SexoMasculino, &bi, nil, nil)
	ct, _ := dominio.NovosContactos("+244923000000", nil, nil)
	doente, _ := dominio.NovoDoente(num, ident, ct, "AO")
	doenteID, err := repoDoentes.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar doente: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.consentimentos WHERE doente_id=$1`, doenteID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})

	c, err := dominio.NovoConsentimento(doenteID, dominio.FinalidadeCirurgia, true, "s3://consent.pdf",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("consentimento inválido: %v", err)
	}
	id, err := repo.Guardar(ctx, c)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}

	obtido, err := repo.ObterPorID(ctx, id)
	if err != nil || !obtido.EstaVigente() || !obtido.TemAnexo() {
		t.Fatalf("esperado vigente com anexo, veio %+v err=%v", obtido, err)
	}

	if err := obtido.Revogar(time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("revogar domínio: %v", err)
	}
	if _, err := repo.Guardar(ctx, obtido); err != nil {
		t.Fatalf("guardar revogação: %v", err)
	}
	depois, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("obter após revogação: %v", err)
	}
	if depois.EstaVigente() {
		t.Fatalf("consentimento revogado não devia estar vigente")
	}

	lista, err := repo.ListarPorDoente(ctx, doenteID, dominio.FiltroConsentimentos{})
	if err != nil || len(lista) != 1 {
		t.Fatalf("esperava 1 consentimento listado, veio %d err=%v", len(lista), err)
	}
}
