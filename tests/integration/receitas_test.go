//go:build integration

// Teste de integração do BC Farmácia (agregado Receita) contra a BD real.
// Segue o padrão de doentes_test.go/episodios_test.go: SKIP (nunca FAIL)
// quando DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestRepositorioReceitas_CicloCompleto(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repoMed := pgrepo.NovoRepositorioMedicamentos(pool)
	repoRec := pgrepo.NovoRepositorioReceitas(pool)

	cod, _ := repoMed.ProximoCodigo(ctx)
	m, _ := dominio.NovoMedicamento(cod, "Amoxil Rec", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, 10)
	medID, err := repoMed.Guardar(ctx, m)
	if err != nil {
		t.Fatalf("guardar medicamento: %v", err)
	}

	emitida := time.Now()
	item, _ := dominio.NovoItemReceita(medID, "1 comp 8/8h", nil, 20, "")
	const doenteID = "00000000-0000-4000-8000-0000000000f1"
	const episodioID = "00000000-0000-4000-8000-0000000000f2"
	const medicoID = "00000000-0000-4000-8000-0000000000f3"
	rec, _ := dominio.NovaReceita(episodioID, doenteID, medicoID, []dominio.ItemReceita{item}, "notas", emitida, emitida.AddDate(0, 0, 30))
	recID, err := repoRec.Guardar(ctx, rec)
	if err != nil {
		t.Fatalf("guardar receita: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.receitas WHERE id=$1`, recID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.medicamentos WHERE id=$1`, medID)
	})

	lido, err := repoRec.ObterPorID(ctx, recID)
	if err != nil || len(lido.Snapshot().Itens) != 1 {
		t.Fatalf("obter receita falhou: %v", err)
	}

	pag, err := repoRec.ListarPorDoente(ctx, dominio.FiltroReceitas{DoenteID: doenteID, Limite: 10})
	if err != nil || pag.Total < 1 || pag.Itens[0].NumItens != 1 {
		t.Fatalf("listar falhou: %v (%+v)", err, pag)
	}
}
