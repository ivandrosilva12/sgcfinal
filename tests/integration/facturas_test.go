//go:build integration

// Teste de integração do BC Financeiro (ADR-039) contra a BD real. SKIP (nunca
// FAIL) quando DATABASE_URL não está definido. O repositório pgx de facturas fica
// fora do gate de cobertura unitário — é este ficheiro que o cobre, provando o
// upsert transaccional, a reescrita de linhas e o total do read model.
package integration_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
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

// gerarUUIDv4 produz um UUID v4 aleatório a partir de crypto/rand (stdlib), sem
// recorrer a github.com/google/uuid: o componente tests não pode depender de
// bibliotecas externas arbitrárias (go-arch-lint, regra "tests"), e o
// google/uuid continua legítimo nos adaptadores (internal/adapters/http), que
// não têm essa restrição. Serve para produzir um episodio_id válido em Go
// antes de tocar na BD; gen_random_uuid() do próprio Postgres (já usado
// noutros pontos deste ficheiro) continua a ser a via preferida quando já há
// ligação disponível. Nunca entra em pânico — um erro de crypto/rand é
// praticamente impossível, mas fica reportado por t.Fatalf, não silenciado.
func gerarUUIDv4(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("gerar uuid v4: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // versão 4
	b[8] = (b[8] & 0x3f) | 0x80 // variante RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// limparFactura remove a factura e as suas linhas (ON DELETE CASCADE trata as linhas).
// O erro da limpeza é registado (t.Logf), nunca silenciado: foi exactamente o
// _, _ = original que mascarou a regressão do ON DELETE CASCADE na Task 4,
// deixando lixo acumular-se em silêncio numa BD partilhada enquanto os testes
// reportavam PASS. Não é t.Fatal — uma limpeza falhada não pode reprovar um
// teste que já passou, só deixar o sinal visível no log.
func limparFactura(t *testing.T, pool *pgxpool.Pool, ctx context.Context, id string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id=$1`, id); err != nil {
			t.Logf("limparFactura: falhou a apagar a factura %s: %v", id, err)
		}
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
ON CONFLICT (numero) WHERE numero IS NOT NULL DO NOTHING
RETURNING id::text`, numero).Scan(&id)
	if err != nil {
		// Uma corrida anterior deste teste já deixou esta factura na BD — o
		// trigger torna-a permanentemente irremovível, de propósito. Reutiliza-a:
		// o alvo do teste é o trigger, não a inserção. O erro do INSERT é
		// registado (não descartado): foi a sua opacidade que, no defeito real do
		// ON CONFLICT sobre índice parcial, escondeu o SQLSTATE 42P10 por detrás
		// de um "no rows in result set" no fallback.
		t.Logf("inserir factura emitida (a reutilizar via fallback): %v", err)
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
// (restrict_violation). A fixture semeia a linha ENQUANTO a factura ainda está
// em RASCUNHO e só depois promove a EMITIDA por UPDATE — desde a correcção ao
// achado F1 da revisão final (trg_itens_factura_imutaveis passou a disparar
// também em INSERT), inserir a linha directamente numa factura já EMITIDA é,
// de propósito, o próprio comportamento que este teste (e
// TestInserirItemEmFacturaEmitida_RejeitadoNaBD) prova estar bloqueado.
func TestItemFacturaEmitida_ImutavelNaBD(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)

	const numero = "FAC 2026/09999998"
	var facturaID string
	if err := pool.QueryRow(ctx,
		`SELECT id::text FROM financeiro.facturas WHERE numero=$1`, numero).Scan(&facturaID); err != nil {
		// Ainda não existe: cria como RASCUNHO, semeia a linha (permitido em
		// RASCUNHO) e só então promove a EMITIDA — a emissão parte sempre de
		// um RASCUNHO e o UPDATE passa (OLD.estado='RASCUNHO').
		if err := pool.QueryRow(ctx, `
INSERT INTO financeiro.facturas (estado, cliente_nome, episodio_id)
VALUES ('RASCUNHO','Cliente',gen_random_uuid())
RETURNING id::text`).Scan(&facturaID); err != nil {
			t.Fatalf("inserir factura rascunho: %v", err)
		}
		if _, err := pool.Exec(ctx, `
INSERT INTO financeiro.itens_factura
    (factura_id, descricao, tipo, quantidade, preco_unitario_centimos, regime_iva, ordem)
VALUES ($1,'Linha emitida','CONSULTA',1,5000,'ISENTO',0)`, facturaID); err != nil {
			t.Fatalf("inserir linha em rascunho: %v", err)
		}
		if _, err := pool.Exec(ctx, `
UPDATE financeiro.facturas
   SET estado='EMITIDA', numero=$2, serie='2026', sequencial=9999998,
       data_emissao=now(), hash='abc', hash_anterior=''
 WHERE id=$1`, facturaID, numero); err != nil {
			t.Fatalf("promover factura a emitida: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id=$1 AND estado='RASCUNHO'`, facturaID)
	})

	var itemID string
	if err := pool.QueryRow(ctx,
		`SELECT id::text FROM financeiro.itens_factura WHERE factura_id=$1 LIMIT 1`, facturaID).Scan(&itemID); err != nil {
		t.Fatalf("obter linha da factura emitida: %v", err)
	}

	_, err := pool.Exec(ctx, `UPDATE financeiro.itens_factura SET quantidade=99 WHERE id=$1`, itemID)
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

// TestInserirItemEmFacturaEmitida_RejeitadoNaBD prova o achado F1 da revisão
// final ao ADR-040: trg_itens_factura_imutaveis passou a disparar também em
// INSERT (antes só UPDATE OR DELETE), pelo que já não é possível ACRESCENTAR
// uma linha a uma factura EMITIDA por SQL directo. Reutiliza a factura
// permanente FAC 2026/09999999 (a mesma de TestFacturaEmitida_ImutavelNaBD):
// o alvo é o trigger, não uma factura nova.
//
// Robustez (achado das revisões ao ADR-040): numa BD onde a 0003 ainda não
// tenha chegado (por exemplo, só a 0002 aplicada), o trigger ainda não cobre
// o INSERT — a inserção abaixo passa em vez de falhar. Sem cuidado adicional
// isto envenenaria a BD para sempre, porque o DELETE de linhas de uma factura
// EMITIDA já está bloqueado desde a 0002: a "Linha intrusa" ficaria presa
// mal entrasse, e um t.Fatal (linha "tinha de falhar" abaixo) saltaria
// qualquer limpeza registada depois dele. Já aconteceu nesta sprint e teve de
// ser corrigido à mão com DISABLE TRIGGER/DELETE/ENABLE TRIGGER — a limpeza
// abaixo automatiza exactamente esse procedimento, registada em t.Cleanup
// ANTES da tentativa de INSERT (para correr mesmo que o teste dê t.Fatal a
// seguir) e só actua se sobrar mesmo uma linha residual: no caminho normal
// (trigger da 0003 presente), o INSERT já falha e não há nada a limpar.
func TestInserirItemEmFacturaEmitida_RejeitadoNaBD(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)

	const numero = "FAC 2026/09999999"
	var facturaID string
	err := pool.QueryRow(ctx, `
INSERT INTO financeiro.facturas
    (estado, cliente_nome, episodio_id, numero, serie, sequencial,
     data_emissao, hash, hash_anterior)
VALUES ('EMITIDA','Cliente',gen_random_uuid(),$1,'2026',9999999,
        now(),'abc','')
ON CONFLICT (numero) WHERE numero IS NOT NULL DO NOTHING
RETURNING id::text`, numero).Scan(&facturaID)
	if err != nil {
		// Corrida anterior (ou TestFacturaEmitida_ImutavelNaBD) já deixou esta
		// factura na BD — permanentemente irremovível, de propósito. Reutiliza-a.
		// O erro do INSERT é registado (não descartado): a sua opacidade foi o
		// que escondeu o SQLSTATE 42P10 real do defeito do ON CONFLICT sobre
		// índice parcial por detrás de um "no rows in result set" no fallback.
		t.Logf("inserir factura emitida (a reutilizar via fallback): %v", err)
		if err := pool.QueryRow(ctx,
			`SELECT id::text FROM financeiro.facturas WHERE numero=$1`, numero).Scan(&facturaID); err != nil {
			t.Fatalf("inserir/obter factura emitida: %v", err)
		}
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx,
			`DELETE FROM financeiro.facturas WHERE id=$1 AND estado='RASCUNHO'`, facturaID); err != nil {
			t.Logf("limpar factura (RASCUNHO): %v", err)
		}
	})

	// Regista a limpeza da linha intrusa ANTES de tentar o INSERT (ver
	// comentário da função): t.Cleanup corre sempre, mesmo com t.Fatal a
	// meio da função, mas só se JÁ estiver registado nesse momento.
	t.Cleanup(func() {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM financeiro.itens_factura WHERE factura_id=$1 AND descricao='Linha intrusa'`,
			facturaID).Scan(&n); err != nil {
			t.Logf("limpeza da linha intrusa: verificar resíduo: %v", err)
			return
		}
		if n == 0 {
			return // caminho normal: o trigger já bloqueou o INSERT, nada a limpar
		}
		// A linha entrou porque o trigger da BD-alvo ainda não cobre o INSERT.
		// O DELETE já está bloqueado pelo mesmo trigger desde a 0002 — a única
		// forma de a remover é desactivá-lo pontualmente para esta limpeza.
		if _, err := pool.Exec(ctx,
			`ALTER TABLE financeiro.itens_factura DISABLE TRIGGER trg_itens_factura_imutaveis`); err != nil {
			t.Logf("limpeza da linha intrusa: desactivar trigger: %v", err)
			return
		}
		defer func() {
			if _, err := pool.Exec(ctx,
				`ALTER TABLE financeiro.itens_factura ENABLE TRIGGER trg_itens_factura_imutaveis`); err != nil {
				t.Logf("limpeza da linha intrusa: reactivar trigger: %v", err)
			}
		}()
		if _, err := pool.Exec(ctx,
			`DELETE FROM financeiro.itens_factura WHERE factura_id=$1 AND descricao='Linha intrusa'`,
			facturaID); err != nil {
			t.Logf("limpeza da linha intrusa: apagar resíduo: %v", err)
		}
	})

	_, err = pool.Exec(ctx, `
INSERT INTO financeiro.itens_factura
    (factura_id, descricao, tipo, quantidade, preco_unitario_centimos, regime_iva, ordem)
VALUES ($1,'Linha intrusa','CONSULTA',1,999900,'ISENTO',99)`, facturaID)
	if err == nil {
		t.Fatal("INSERT de uma linha numa factura emitida tinha de falhar")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23001" {
		t.Errorf("esperava SQLSTATE 23001 (restrict_violation), deu: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM financeiro.itens_factura WHERE factura_id=$1 AND descricao='Linha intrusa'`,
		facturaID).Scan(&n); err != nil {
		t.Fatalf("contar linhas intrusas: %v", err)
	}
	if n != 0 {
		t.Errorf("a linha intrusa não devia ter entrado na tabela, encontrou %d", n)
	}
}

// TestFacturaRascunho_InserirLinhaContinuaAPassar confirma, a par da correcção
// acima, que fechar o buraco do INSERT numa factura EMITIDA não partiu o
// caminho normal de edição: um INSERT directo de uma linha numa factura em
// RASCUNHO continua a passar. Sem este teste, a correcção ao F1 poderia ter
// bloqueado por engano também o INSERT legítimo (por exemplo trocando
// COALESCE(NEW.factura_id, OLD.factura_id) por OLD.factura_id, que em INSERT é
// sempre NULL e faria estado_pai ficar sempre NULL — mascarando o problema
// inverso: nenhum INSERT seria bloqueado, nem sequer o de uma EMITIDA).
func TestFacturaRascunho_InserirLinhaContinuaAPassar(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	cli, _ := fin.NovoClienteSnapshot("Cliente Rascunho Insert", "", "")
	f, _ := fin.NovaFactura(cli, "99999999-9999-9999-9999-999999999999")
	_ = f.AdicionarItem("Original", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(1000), fin.RegimeIsento)
	id, err := repo.Guardar(ctx, f)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	limparFactura(t, pool, ctx, id)

	var novoID string
	err = pool.QueryRow(ctx, `
INSERT INTO financeiro.itens_factura
    (factura_id, descricao, tipo, quantidade, preco_unitario_centimos, regime_iva, ordem)
VALUES ($1,'Linha adicionada em rascunho','CONSULTA',1,200000,'ISENTO',1)
RETURNING id::text`, id).Scan(&novoID)
	if err != nil {
		t.Fatalf("INSERT de linha numa factura RASCUNHO devia passar, deu: %v", err)
	}
	if novoID == "" {
		t.Fatal("id da nova linha em falta")
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM financeiro.itens_factura WHERE factura_id=$1`, id).Scan(&n); err != nil {
		t.Fatalf("contar linhas: %v", err)
	}
	if n != 2 {
		t.Errorf("esperava 2 linhas (original + inserida) numa factura RASCUNHO, tem %d", n)
	}
}

// TestGuardarFactura_BloqueioOptimista prova que o Guardar fecha o lost-update
// do rascunho (dívida técnica assumida do ADR-039, fechada no ADR-040): dois
// leitores carregam a mesma versão, a primeira escrita vence e avança a versão,
// a segunda (sobre a versão velha) recebe CategoriaConflito — e, decisivo, a
// linha da vencedora sobrevive na releitura (sem isto, um teste que só
// confirmasse o erro da segunda escrita não provaria que a primeira não foi
// silenciosamente apagada).
func TestGuardarFactura_BloqueioOptimista(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	cliente, err := fin.NovoClienteSnapshot("Cliente", "", "")
	if err != nil {
		t.Fatalf("NovoClienteSnapshot: %v", err)
	}
	f, err := fin.NovaFactura(cliente, gerarUUIDv4(t))
	if err != nil {
		t.Fatalf("NovaFactura: %v", err)
	}
	id, err := repo.Guardar(ctx, f)
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	limparFactura(t, pool, ctx, id)

	// Dois leitores carregam a MESMA versão.
	a, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("ObterPorID a: %v", err)
	}
	b, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("ObterPorID b: %v", err)
	}
	if err := a.AdicionarItem("Consulta A", fin.LinhaConsulta, "", 1,
		moeda.DeCentimos(1000), fin.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem a: %v", err)
	}
	if err := b.AdicionarItem("Consulta B", fin.LinhaConsulta, "", 1,
		moeda.DeCentimos(2000), fin.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem b: %v", err)
	}

	if _, err := repo.Guardar(ctx, a); err != nil {
		t.Fatalf("a primeira escrita devia passar: %v", err)
	}
	_, err = repo.Guardar(ctx, b)
	if err == nil {
		t.Fatal("a segunda escrita sobre versão velha tinha de dar conflito (lost update)")
	}
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Errorf("esperava Conflito, deu %v", err)
	}

	// A linha de 'a' sobreviveu — nada se perdeu.
	final, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("ObterPorID final: %v", err)
	}
	if len(final.Itens()) != 1 || final.Itens()[0].Descricao != "Consulta A" {
		t.Errorf("esperava só a linha de A, tem %d linhas", len(final.Itens()))
	}
	// Prova o read-back de versao em ObterPorID (achado Important da revisão à
	// Task 5, mutação M2): sem ele, esta asserção sozinha já falha, porque o
	// snapshot lido traria sempre versao=0 em vez do valor real gravado pela
	// escrita vencedora (0→1).
	if final.Versao() != 1 {
		t.Errorf("esperava versao=1 após a escrita vencedora, tem %d", final.Versao())
	}
}

// TestGuardarFactura_DuasEdicoesSequenciais prova, contra a mutação M2 do
// achado Important da revisão à Task 5 (retirar 'versao' do SELECT/Scan de
// ObterPorID), que o read-back de versao em ObterPorID é real e não um
// acidente de coincidência com o teste de bloqueio optimista. Sem ele, todas
// as leituras trazem versao=0 e qualquer factura já em versao>=1 fica
// permanentemente ingravável: a segunda de duas edições sequenciais — sem
// qualquer concorrência real — falharia com um 409 espúrio
// ("a factura foi alterada entretanto"), porque o snapshot lido nunca reflecte
// a versão que a própria escrita anterior avançou.
func TestGuardarFactura_DuasEdicoesSequenciais(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	cliente, err := fin.NovoClienteSnapshot("Cliente Sequencial", "", "")
	if err != nil {
		t.Fatalf("NovoClienteSnapshot: %v", err)
	}
	f, err := fin.NovaFactura(cliente, gerarUUIDv4(t))
	if err != nil {
		t.Fatalf("NovaFactura: %v", err)
	}
	id, err := repo.Guardar(ctx, f)
	if err != nil {
		t.Fatalf("Guardar inicial: %v", err)
	}
	limparFactura(t, pool, ctx, id)

	// Primeira edição: carregar, alterar, guardar.
	primeira, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("ObterPorID (1ª edição): %v", err)
	}
	if err := primeira.AdicionarItem("Consulta 1", fin.LinhaConsulta, "", 1,
		moeda.DeCentimos(1000), fin.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem (1ª edição): %v", err)
	}
	if _, err := repo.Guardar(ctx, primeira); err != nil {
		t.Fatalf("Guardar (1ª edição): %v", err)
	}

	// Segunda edição: recarregar (a BD já vai em versao=1), alterar de novo,
	// guardar. Sem o read-back de versao, ObterPorID devolveria sempre
	// versao=0 e esta segunda escrita — apesar de não haver qualquer outra
	// escrita concorrente — falharia com um 409 espúrio.
	segunda, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("ObterPorID (2ª edição): %v", err)
	}
	if err := segunda.AdicionarItem("Consulta 2", fin.LinhaConsulta, "", 1,
		moeda.DeCentimos(2000), fin.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem (2ª edição): %v", err)
	}
	if _, err := repo.Guardar(ctx, segunda); err != nil {
		t.Fatalf("a segunda escrita, sem qualquer concorrência, tinha de passar: %v", err)
	}

	final, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("ObterPorID final: %v", err)
	}
	if final.Versao() != 2 {
		t.Errorf("esperava versao=2 após duas edições sequenciais, tem %d", final.Versao())
	}
}

// TestGuardarFactura_GuardaDeEstadoRejeitaFacturaEmitida prova, contra a
// mutação M3 do achado Important da revisão à Task 5 (retirar
// "AND estado='RASCUNHO'" do WHERE do UPDATE em Guardar), que a guarda de
// estado está coberta e distingue o 409 correcto do 500 espúrio. A guarda
// está presente e correcta no código actual: sem ela, o UPDATE alcançaria uma
// factura EMITIDA e chegaria ao trigger de imutabilidade da Task 4
// (SQLSTATE 23001), cujo erro cru não é um erros.ErroDominio — propagar-se-ia
// como CategoriaInterno (→ HTTP 500) em vez de CategoriaConflito (→ HTTP 409).
// Confirmar só err != nil não distingue os dois casos; esta é precisamente a
// distinção em causa. Cria a sua própria factura EMITIDA (não depende das
// residuais doutros testes) com o mesmo padrão de
// TestFacturaEmitida_ImutavelNaBD: ON CONFLICT DO NOTHING + fallback SELECT,
// para reutilizar entre corridas em vez de acumular linhas permanentes.
func TestGuardarFactura_GuardaDeEstadoRejeitaFacturaEmitida(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	const numero = "FAC 2026/09999997"
	var id string
	var versao int
	err := pool.QueryRow(ctx, `
INSERT INTO financeiro.facturas
    (estado, cliente_nome, episodio_id, numero, serie, sequencial,
     data_emissao, hash, hash_anterior)
VALUES ('EMITIDA','Cliente Emitido',gen_random_uuid(),$1,'2026',9999997,
        now(),'abc','')
ON CONFLICT (numero) WHERE numero IS NOT NULL DO NOTHING
RETURNING id::text, versao`, numero).Scan(&id, &versao)
	if err != nil {
		// Uma corrida anterior já deixou esta factura na BD — o trigger torna-a
		// permanentemente irremovível, de propósito. Reutiliza-a. O erro do
		// INSERT é registado (não descartado): a sua opacidade foi o que
		// escondeu o SQLSTATE 42P10 real do defeito do ON CONFLICT sobre índice
		// parcial por detrás de um "no rows in result set" no fallback.
		t.Logf("inserir factura emitida (a reutilizar via fallback): %v", err)
		if err := pool.QueryRow(ctx,
			`SELECT id::text, versao FROM financeiro.facturas WHERE numero=$1`, numero).Scan(&id, &versao); err != nil {
			t.Fatalf("inserir/obter factura emitida: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id=$1 AND estado='RASCUNHO'`, id)
	})

	cliente, err := fin.NovoClienteSnapshot("Outro Cliente", "", "")
	if err != nil {
		t.Fatalf("NovoClienteSnapshot: %v", err)
	}
	emitida := fin.ReconstruirFactura(fin.SnapshotFactura{
		ID: id, Estado: fin.FactEmitida, Cliente: cliente,
		EpisodioID: gerarUUIDv4(t), Versao: versao,
	})

	_, err = repo.Guardar(ctx, emitida)
	if err == nil {
		t.Fatal("guardar sobre uma factura EMITIDA tinha de falhar")
	}
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Errorf("esperava CategoriaConflito (409), deu %v (categoria %v)", err, erros.CategoriaDe(err))
	}
}

// TestEmitirFacturas_NumeracaoSemBuracosSobConcorrencia é a prova central da
// ADR-040: a lei angolana (AGT/SAF-T-AO) exige numeração sequencial sem buracos
// por série. N emissões verdadeiramente simultâneas na mesma série têm de
// produzir sequenciais distintos e contíguos, e uma cadeia de hashes bem
// encadeada. Um teste sequencial não provaria serialização nenhuma — daí a
// barreira de arranque, que solta as goroutines todas no mesmo instante.
//
// O teste não apaga a linha da série nem as facturas emitidas: o trigger de
// imutabilidade impede-o, por desenho. Em vez disso mede-se em relação ao estado
// inicial da série, o que o torna correcto tanto numa BD limpa (CI, base 0) como
// em execuções repetidas na BD de desenvolvimento.
func TestEmitirFacturas_NumeracaoSemBuracosSobConcorrencia(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	// Série própria desta corrida. A série é derivada do ANO do momento por
	// fin.SerieDe, logo escolhe-se um ano ao acaso numa banda reservada.
	//
	// Porquê ao acaso, e não um ano fixo: as facturas EMITIDA são imortais (o
	// trigger de imutabilidade impede apagá-las), pelo que uma série fixa acumula
	// os elos de todas as corridas anteriores. Basta a canonicalização do hash
	// mudar uma vez para que os elos antigos deixem de fechar, e o teste passa a
	// acusar uma quebra que não é da cadeia mas da história. Uma série por corrida
	// torna o teste auto-contido e imune a futuras mudanças de formato.
	//
	// A banda evita o 2999, que já tem elos em formato anterior ao ADR-041.
	ano := 2100 + mathrand.Intn(800)
	serie := strconv.Itoa(ano)
	momento := time.Date(ano, 1, 15, 9, 0, 0, 0, time.UTC)

	var base int
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(max(ultimo_sequencial),0) FROM financeiro.series WHERE serie=$1`,
		serie).Scan(&base); err != nil {
		t.Fatalf("ler o estado inicial da série: %v", err)
	}

	const n = 12
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		cliente, err := fin.NovoClienteSnapshot("Cliente Concorrente", "", "")
		if err != nil {
			t.Fatalf("NovoClienteSnapshot: %v", err)
		}
		f, err := fin.NovaFactura(cliente, gerarUUIDv4(t))
		if err != nil {
			t.Fatalf("NovaFactura: %v", err)
		}
		if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1,
			moeda.DeCentimos(int64(1000+i)), fin.RegimeIsento); err != nil {
			t.Fatalf("AdicionarItem: %v", err)
		}
		id, err := repo.Guardar(ctx, f)
		if err != nil {
			t.Fatalf("Guardar: %v", err)
		}
		// Enquanto está em rascunho ainda é removível; depois de emitida não.
		limparFactura(t, pool, ctx, id)
		ids = append(ids, id)
	}

	// Emitir todas em simultâneo, soltas por uma barreira comum.
	arranque := make(chan struct{})
	var wg sync.WaitGroup
	erradas := make([]error, n)
	emitidas := make([]*fin.Factura, n)
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			<-arranque
			emitidas[i], erradas[i] = repo.Emitir(ctx, id, momento)
		}(i, id)
	}
	close(arranque)
	wg.Wait()
	for i, err := range erradas {
		if err != nil {
			t.Fatalf("emissão %d falhou: %v", i, err)
		}
	}

	// Cada emissão devolve o agregado já emitido, com número e elo próprios.
	seqDevolvidos := map[int]bool{}
	for i, f := range emitidas {
		if f.Estado() != fin.FactEmitida {
			t.Errorf("emissão %d: estado %q, queria EMITIDA", i, f.Estado())
		}
		if f.Hash() == "" {
			t.Errorf("emissão %d: ficou sem hash", i)
		}
		if querido := fmt.Sprintf("FAC %s/%08d", serie, f.Sequencial()); f.Numero().String() != querido {
			t.Errorf("emissão %d: número %q, queria %q", i, f.Numero(), querido)
		}
		if seqDevolvidos[f.Sequencial()] {
			t.Errorf("sequencial %d devolvido a duas emissões", f.Sequencial())
		}
		seqDevolvidos[f.Sequencial()] = true
	}

	snaps, err := repo.ListarSnapshotsPorSerie(ctx, serie)
	if err != nil {
		t.Fatalf("ListarSnapshotsPorSerie: %v", err)
	}
	if len(snaps) != base+n {
		t.Fatalf("esperava %d facturas na série, tem %d", base+n, len(snaps))
	}
	vistos := map[int]bool{}
	for _, s := range snaps {
		if vistos[s.Sequencial] {
			t.Errorf("sequencial %d repetido", s.Sequencial)
		}
		vistos[s.Sequencial] = true
	}
	for i := 1; i <= base+n; i++ {
		if !vistos[i] {
			t.Errorf("buraco na série: falta o sequencial %d", i)
		}
	}
	// Cada elo tem de apontar ao hash da factura anterior, e o conteúdo tem de
	// bater certo com o hash gravado.
	if err := fin.VerificarCadeia(snaps); err != nil {
		t.Errorf("cadeia devia estar íntegra após emissões concorrentes: %v", err)
	}
}
