//go:build integration

// Teste de integração do BC Financeiro (ADR-039) contra a BD real. SKIP (nunca
// FAIL) quando DATABASE_URL não está definido. O repositório pgx de facturas fica
// fora do gate de cobertura unitário — é este ficheiro que o cobre, provando o
// upsert transaccional, a reescrita de linhas e o total do read model.
package integration_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// migrarFinanceiro aplica as migrações forward-only (idempotente); ligar(t) só
// liga o pool. Modelada em migrarLaboratorio (laboratorio_test.go).
func migrarFinanceiro(t *testing.T, pool *pgxpool.Pool, ctx context.Context) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
}

// limparFactura remove a factura e as suas linhas (ON DELETE CASCADE trata as linhas).
func limparFactura(t *testing.T, pool *pgxpool.Pool, ctx context.Context, id string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id=$1`, id)
	})
}

func TestRepositorioFacturas_GuardarEObter(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	cli, _ := fin.NovoClienteSnapshot("Clínica Sol", "", "")
	f, _ := fin.NovaFactura(cli, "11111111-1111-1111-1111-111111111111")
	_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(5000), fin.RegimeIsento)
	_ = f.AdicionarItem("Medicamento", fin.LinhaDispensa, "22222222-2222-2222-2222-222222222222", 2, moeda.DeKwanzas(1000), fin.RegimeStandard)

	id, err := repo.Guardar(ctx, f)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if id == "" {
		t.Fatal("id gerado em falta")
	}
	limparFactura(t, pool, ctx, id)

	lida, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if lida.Estado() != fin.FactRascunho || len(lida.Itens()) != 2 {
		t.Errorf("factura mal lida: estado=%s itens=%d", lida.Estado(), len(lida.Itens()))
	}
	if lida.Totais().Total.Centimos() != 728000 {
		t.Errorf("total = %d; esperava 728000", lida.Totais().Total.Centimos())
	}

	// Listar por episódio devolve o total do domínio.
	resumos, err := repo.ListarPorEpisodio(ctx, "11111111-1111-1111-1111-111111111111")
	if err != nil || len(resumos) != 1 {
		t.Fatalf("listar: err=%v n=%d", err, len(resumos))
	}
	if resumos[0].TotalCentimos != 728000 || resumos[0].NumItens != 2 {
		t.Errorf("resumo errado: %+v", resumos[0])
	}
}

func TestRepositorioFacturas_ReescreveItens(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	cli, _ := fin.NovoClienteSnapshot("Sol", "", "")
	f, _ := fin.NovaFactura(cli, "33333333-3333-3333-3333-333333333333")
	_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(5000), fin.RegimeIsento)
	id, _ := repo.Guardar(ctx, f)
	limparFactura(t, pool, ctx, id)

	lida, _ := repo.ObterPorID(ctx, id)
	item0 := lida.Itens()[0].ID
	if err := lida.RemoverItem(item0); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Guardar(ctx, lida); err != nil {
		t.Fatalf("reguardar: %v", err)
	}
	rel, _ := repo.ObterPorID(ctx, id)
	if len(rel.Itens()) != 0 {
		t.Errorf("esperava 0 itens após remoção; tem %d", len(rel.Itens()))
	}
}

// TestRepositorioFacturas_PreservaIdsEOrdemNaReescrita prova, com 3 linhas
// reais, que o re-INSERT do Guardar preserva o id das linhas existentes
// (via COALESCE(NULLIF($1,'')::uuid, gen_random_uuid())) e que a ordem de
// leitura (ORDER BY ordem) segue a ordem de inserção — não a ordem de
// remoção nem uma ordem alfabética/aleatória. Uma regressão que baralhasse a
// ordem ou regenerasse ids a cada gravação passaria despercebida no teste
// TestRepositorioFacturas_ReescreveItens (que só usa 1 item).
func TestRepositorioFacturas_PreservaIdsEOrdemNaReescrita(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	cli, _ := fin.NovoClienteSnapshot("Sol", "", "")
	f, _ := fin.NovaFactura(cli, "44444444-4444-4444-4444-444444444444")
	_ = f.AdicionarItem("Linha A", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(1000), fin.RegimeIsento)
	_ = f.AdicionarItem("Linha B", fin.LinhaDispensa, "55555555-5555-5555-5555-555555555555", 1, moeda.DeKwanzas(2000), fin.RegimeStandard)
	_ = f.AdicionarItem("Linha C", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(3000), fin.RegimeIsento)
	id, err := repo.Guardar(ctx, f)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	limparFactura(t, pool, ctx, id)

	lida, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	itens := lida.Itens()
	if len(itens) != 3 {
		t.Fatalf("esperava 3 itens; tem %d", len(itens))
	}
	// A ordem de leitura deve seguir ORDER BY ordem = ordem de inserção.
	if itens[0].Descricao != "Linha A" || itens[1].Descricao != "Linha B" || itens[2].Descricao != "Linha C" {
		t.Fatalf("ordem inicial errada: %v %v %v", itens[0].Descricao, itens[1].Descricao, itens[2].Descricao)
	}
	idA, idB, idC := itens[0].ID, itens[1].ID, itens[2].ID
	if idA == "" || idB == "" || idC == "" {
		t.Fatalf("ids em falta após a primeira gravação: A=%q B=%q C=%q", idA, idB, idC)
	}

	// Remove a linha do meio (B) e re-guarda — A e C têm ids não-vazios, o
	// re-INSERT tem de os PRESERVAR (não regenerar) e manter a ordem A→C.
	if err := lida.RemoverItem(idB); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Guardar(ctx, lida); err != nil {
		t.Fatalf("reguardar: %v", err)
	}

	rel, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("reobter: %v", err)
	}
	relItens := rel.Itens()
	if len(relItens) != 2 {
		t.Fatalf("esperava 2 itens após remoção; tem %d", len(relItens))
	}
	if relItens[0].ID != idA || relItens[1].ID != idC {
		t.Errorf("ids não preservados/ordem trocada: [%s,%s] esperava [%s,%s]",
			relItens[0].ID, relItens[1].ID, idA, idC)
	}
	if relItens[0].Descricao != "Linha A" || relItens[1].Descricao != "Linha C" {
		t.Errorf("ordem errada após reescrita: %v, %v", relItens[0].Descricao, relItens[1].Descricao)
	}
}
