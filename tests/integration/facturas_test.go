//go:build integration

// Teste de integração do BC Financeiro (ADR-039) contra a BD real. SKIP (nunca
// FAIL) quando DATABASE_URL não está definido. O repositório pgx de facturas fica
// fora do gate de cobertura unitário — é este ficheiro que o cobre, provando o
// upsert transaccional, a reescrita de linhas e o total do read model.
package integration_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
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
// (o id existente é preservado no re-INSERT, gen_random_uuid() só para linhas novas) e que a ordem de
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

// TestFacturaEmitida_ImutavelNaBD prova o trg_facturas_imutaveis (ADR-040): uma
// factura EMITIDA não pode ser alterada nem apagada por SQL directo — a
// imutabilidade é defesa em profundidade, para além da guarda no domínio. A
// factura é inserida com um sequencial fora do alcance da aplicação (9999999)
// porque o alvo é o trigger, não o repositório; o Cleanup não a consegue apagar
// e essa impossibilidade é precisamente a propriedade em teste.
func TestFacturaEmitida_ImutavelNaBD(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)

	const numero = "FAC 2026/09999999"
	var id string
	err := pool.QueryRow(ctx, `
INSERT INTO financeiro.facturas
    (estado, cliente_nome, episodio_id, numero, serie, sequencial,
     data_emissao, hash, hash_anterior)
VALUES ('EMITIDA','Cliente',gen_random_uuid(),$1,'2026',9999999,
        now(),'abc','')
ON CONFLICT (numero) DO NOTHING
RETURNING id::text`, numero).Scan(&id)
	if err != nil {
		// Uma corrida anterior deste teste já deixou esta factura na BD — o
		// trigger torna-a permanentemente irremovível, de propósito. Reutiliza-a:
		// o alvo do teste é o trigger, não a inserção.
		if err := pool.QueryRow(ctx,
			`SELECT id::text FROM financeiro.facturas WHERE numero=$1`, numero).Scan(&id); err != nil {
			t.Fatalf("inserir/obter factura emitida: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id=$1 AND estado='RASCUNHO'`, id)
	})

	_, err = pool.Exec(ctx, `UPDATE financeiro.facturas SET cliente_nome='Outro' WHERE id=$1`, id)
	if err == nil {
		t.Fatal("UPDATE numa factura emitida tinha de falhar")
	}
	// SQLSTATE 23001 é restrict_violation (classe 23, Integrity Constraint
	// Violation) — o código que o PostgreSQL atribui de facto a
	// USING ERRCODE = 'restrict_violation'. Não é 2F004 (classe 2F, SQL Routine
	// Exception): confirmado empiricamente contra o Postgres real nesta migração.
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23001" {
		t.Errorf("esperava SQLSTATE 23001 (restrict_violation), deu: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id=$1`, id); err == nil {
		t.Error("DELETE numa factura emitida tinha de falhar (retenção 10 anos)")
	}
}

// TestFacturaRascunho_ApagarComLinhasNaoDisparaOTriggerDeImutabilidade prova que
// apagar uma factura RASCUNHO com linhas não é bloqueado pelo trigger dos itens.
// trg_itens_factura_imutaveis dispara via ON DELETE CASCADE quando a factura-mãe
// é apagada; nesse instante a linha da factura-mãe já não existe (foi apagada
// dentro da mesma instrução), pelo que a função tem de tratar "factura-mãe não
// encontrada" como não-bloqueio — e não como equivalente a "não está em
// RASCUNHO". Regressão real encontrada durante o Task 4: a primeira versão da
// função bloqueava qualquer DELETE em cascata de uma factura RASCUNHO com linhas.
func TestFacturaRascunho_ApagarComLinhasNaoDisparaOTriggerDeImutabilidade(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	cli, _ := fin.NovoClienteSnapshot("Cliente Cascata", "", "")
	f, _ := fin.NovaFactura(cli, "66666666-6666-6666-6666-666666666666")
	_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(5000), fin.RegimeIsento)
	id, err := repo.Guardar(ctx, f)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id=$1`, id); err != nil {
		t.Fatalf("apagar factura RASCUNHO com linhas devia passar, deu: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM financeiro.itens_factura WHERE factura_id=$1`, id).Scan(&n); err != nil {
		t.Fatalf("contar linhas remanescentes: %v", err)
	}
	if n != 0 {
		t.Errorf("esperava 0 linhas após CASCADE, tem %d", n)
	}
}

// TestFacturaRascunho_ActualizarLinhaAlteraOValor prova a correcção ao achado
// Important da revisão à Task 4: impedir_mutacao_item_factura() terminava com
// RETURN OLD, o que num trigger BEFORE UPDATE FOR EACH ROW faz o PostgreSQL
// gravar os valores ANTIGOS — o UPDATE reporta sucesso (UPDATE 1) mas não
// altera nada, perda silenciosa de dados num contexto fiscal. A correcção
// devolve NEW em UPDATE (só OLD em DELETE, onde NEW não existe). O teste lê o
// valor de volta depois do UPDATE: sem essa leitura, "UPDATE 1" sozinho não
// distingue o comportamento correcto do defeito.
func TestFacturaRascunho_ActualizarLinhaAlteraOValor(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	cli, _ := fin.NovoClienteSnapshot("Cliente Rascunho", "", "")
	f, _ := fin.NovaFactura(cli, "77777777-7777-7777-7777-777777777777")
	_ = f.AdicionarItem("Original", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(1000), fin.RegimeIsento)
	id, err := repo.Guardar(ctx, f)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	limparFactura(t, pool, ctx, id)

	lida, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	itemID := lida.Itens()[0].ID

	ct, err := pool.Exec(ctx,
		`UPDATE financeiro.itens_factura SET descricao='NOVO', quantidade=7 WHERE id=$1`, itemID)
	if err != nil {
		t.Fatalf("actualizar linha de rascunho devia passar, deu: %v", err)
	}
	if ct.RowsAffected() != 1 {
		t.Fatalf("esperava 1 linha afectada, teve %d", ct.RowsAffected())
	}

	var descricao string
	var quantidade int
	if err := pool.QueryRow(ctx,
		`SELECT descricao, quantidade FROM financeiro.itens_factura WHERE id=$1`, itemID).
		Scan(&descricao, &quantidade); err != nil {
		t.Fatalf("reler linha: %v", err)
	}
	if descricao != "NOVO" || quantidade != 7 {
		t.Fatalf("UPDATE reportou sucesso mas não alterou a linha (defeito RETURN OLD reintroduzido): descricao=%q quantidade=%d", descricao, quantidade)
	}
}

// TestItemFacturaEmitida_ImutavelNaBD reconfirma, depois da correcção ao
// RETURN OLD acima, que a imutabilidade das linhas de uma factura EMITIDA se
// mantém estanque: UPDATE e DELETE continuam bloqueados com SQLSTATE 23001
// (restrict_violation). A linha é inserida directamente por SQL (contornando
// o domínio, tal como TestFacturaEmitida_ImutavelNaBD) porque o alvo é o
// trigger, não o repositório.
func TestItemFacturaEmitida_ImutavelNaBD(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)

	const numero = "FAC 2026/09999998"
	var facturaID string
	err := pool.QueryRow(ctx, `
INSERT INTO financeiro.facturas
    (estado, cliente_nome, episodio_id, numero, serie, sequencial,
     data_emissao, hash, hash_anterior)
VALUES ('EMITIDA','Cliente',gen_random_uuid(),$1,'2026',9999998,
        now(),'abc','')
ON CONFLICT (numero) WHERE numero IS NOT NULL DO NOTHING
RETURNING id::text`, numero).Scan(&facturaID)
	if err != nil {
		// Corrida anterior já deixou esta factura (e a sua linha) na BD —
		// permanentemente irremovíveis, de propósito. Reutiliza-as.
		if err := pool.QueryRow(ctx,
			`SELECT id::text FROM financeiro.facturas WHERE numero=$1`, numero).Scan(&facturaID); err != nil {
			t.Fatalf("inserir/obter factura emitida: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id=$1 AND estado='RASCUNHO'`, facturaID)
	})

	var itemID string
	if err := pool.QueryRow(ctx,
		`SELECT id::text FROM financeiro.itens_factura WHERE factura_id=$1 LIMIT 1`, facturaID).Scan(&itemID); err != nil {
		if err := pool.QueryRow(ctx, `
INSERT INTO financeiro.itens_factura
    (factura_id, descricao, tipo, quantidade, preco_unitario_centimos, regime_iva, ordem)
VALUES ($1,'Linha emitida','CONSULTA',1,5000,'ISENTO',0)
RETURNING id::text`, facturaID).Scan(&itemID); err != nil {
			t.Fatalf("inserir linha de factura emitida: %v", err)
		}
	}

	_, err = pool.Exec(ctx, `UPDATE financeiro.itens_factura SET quantidade=99 WHERE id=$1`, itemID)
	if err == nil {
		t.Fatal("UPDATE numa linha de factura emitida tinha de falhar")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23001" {
		t.Errorf("esperava SQLSTATE 23001 (restrict_violation), deu: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM financeiro.itens_factura WHERE id=$1`, itemID); err == nil {
		t.Error("DELETE numa linha de factura emitida tinha de falhar")
	}
}
