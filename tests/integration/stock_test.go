//go:build integration

// Teste de integração do BC Farmácia (stock: fornecedores e lotes) contra a
// BD real. Segue o padrão de medicamentos_test.go/receitas_test.go: SKIP
// (nunca FAIL) quando DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestRepositorioStock_EntradaEConsulta(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repoMed := pgrepo.NovoRepositorioMedicamentos(pool)
	repoLotes := pgrepo.NovoRepositorioLotes(pool)

	cod, _ := repoMed.ProximoCodigo(ctx)
	m, _ := dominio.NovoMedicamento(cod, "Amoxil Stock", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, 10)
	medID, err := repoMed.Guardar(ctx, m)
	if err != nil {
		t.Fatalf("guardar medicamento: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.movimentos_stock WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.lotes WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.medicamentos WHERE id=$1`, medID)
	})

	l1, _ := dominio.NovoLote(medID, "L001", time.Now().AddDate(0, 1, 0), 100, "12.5000", nil, "")
	if _, err := repoLotes.RegistarEntrada(ctx, l1, "00000000-0000-4000-8000-0000000000a1"); err != nil {
		t.Fatalf("entrada: %v", err)
	}
	l2, _ := dominio.NovoLote(medID, "L002", time.Now().AddDate(0, 3, 0), 50, "12.5000", nil, "")
	if _, err := repoLotes.RegistarEntrada(ctx, l2, "00000000-0000-4000-8000-0000000000a1"); err != nil {
		t.Fatalf("entrada 2: %v", err)
	}

	total, err := repoLotes.StockDisponivel(ctx, medID)
	if err != nil || total != 150 {
		t.Fatalf("stock=%d, esperava 150 (%v)", total, err)
	}
	lotes, err := repoLotes.ListarPorMedicamento(ctx, medID, true)
	if err != nil || len(lotes) != 2 || lotes[0].NumeroLote != "L001" {
		t.Fatalf("lotes inesperados: %+v (%v)", lotes, err)
	}

	// Lote com o mesmo (medicamento, número, fornecedor) → conflito de unicidade.
	dup, _ := dominio.NovoLote(medID, "L001", time.Now().AddDate(0, 2, 0), 10, "1", nil, "")
	if _, err := repoLotes.RegistarEntrada(ctx, dup, "00000000-0000-4000-8000-0000000000a1"); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito de lote, obtive %v", err)
	}
}

func TestRepositorioFornecedores_CicloCompleto(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repo := pgrepo.NovoRepositorioFornecedores(pool)

	nif := "5417123456"
	contacto := "923 000 111"
	f, err := dominio.NovoFornecedor("Farmadis Integração", &nif, &contacto)
	if err != nil {
		t.Fatalf("construir fornecedor: %v", err)
	}
	id, err := repo.Guardar(ctx, f)
	if err != nil {
		t.Fatalf("guardar fornecedor: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM farmacia.fornecedores WHERE id=$1`, id) })

	lido, err := repo.ObterPorID(ctx, id)
	if err != nil || lido.ID() != id || !lido.Activo() {
		t.Fatalf("obter fornecedor falhou: %v", err)
	}

	// ObterPorID deve devolver sempre um agregado fresco: alterar a cópia
	// lida não pode afectar leituras subsequentes vindas da BD.
	lido.Desactivar()
	relido, err := repo.ObterPorID(ctx, id)
	if err != nil || !relido.Activo() {
		t.Fatalf("agregado partilhado entre chamadas: esperava activo=true, obtive %v (%v)", relido.Activo(), err)
	}

	pag, err := repo.Listar(ctx, dominio.FiltroFornecedores{Termo: "Integração", ApenasActivos: true, Limite: 10})
	if err != nil || pag.Total < 1 {
		t.Fatalf("listar falhou: %v (total=%d)", err, pag.Total)
	}
}
