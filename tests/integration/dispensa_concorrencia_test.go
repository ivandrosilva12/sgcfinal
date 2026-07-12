//go:build integration

// Teste de integração que prova, contra a BD real, a correcção do bug CRITICAL
// de concorrência: duas dispensas concorrentes da MESMA receita (ambas a ler o
// mesmo snapshot com quantidade_dispensada=0) já não conseguem sobre-dispensar.
// Antes da correcção, o MotorDispensa bloqueava os LOTES (FOR UPDATE) mas nunca
// a receita, e persistia quantidade_dispensada como overwrite absoluto a partir
// do snapshot lido antes do lock — permitindo que as duas confirmassem e
// dispensassem 200 contra uma prescrição de 100. Segue o padrão de
// dispensa_test.go: SKIP (nunca FAIL) quando DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// TestMotorDispensa_ConcorrenciaMesmaReceita_SoUmaConfirma dispara duas
// dispensas de 100 unidades em goroutines separadas, contra uma receita
// prescrita 100, com stock generoso (200) para que uma eventual falta de
// stock nunca possa ser confundida com a guarda de não-exceder. Uma barreira
// (canal `pronto`) garante que as duas só chamam Dispensar depois de ambas
// terem arrancado — nenhuma tem vantagem de arranque.
func TestMotorDispensa_ConcorrenciaMesmaReceita_SoUmaConfirma(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repoMed := pgrepo.NovoRepositorioMedicamentos(pool)
	repoLotes := pgrepo.NovoRepositorioLotes(pool)
	repoReceitas := pgrepo.NovoRepositorioReceitas(pool)
	motor := pgrepo.NovoMotorDispensa(pool)

	cod, _ := repoMed.ProximoCodigo(ctx)
	m, _ := dominio.NovoMedicamento(cod, "Amoxil Corrida", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, 10)
	medID, err := repoMed.Guardar(ctx, m)
	if err != nil {
		t.Fatalf("guardar medicamento: %v", err)
	}

	// Stock generoso (200) para 2x a quantidade prescrita (100): SE a corrida
	// não fosse serializada, ambas as tentativas de 100 encontrariam stock de
	// sobra — a prova de correcção está em quantas dispensas CONFIRMAM
	// (deve ser 1), não em ficarem bloqueadas por falta de stock.
	lote, _ := dominio.NovoLote(medID, "R", time.Now().AddDate(0, 6, 0), 200, "1", nil, "")
	loteID, err := repoLotes.RegistarEntrada(ctx, lote, "00000000-0000-4000-8000-0000000000d1")
	if err != nil {
		t.Fatalf("registar entrada de lote: %v", err)
	}

	item, _ := dominio.NovoItemReceita(medID, "1 comp", nil, 100, "")
	const doenteID = "00000000-0000-4000-8000-0000000000d2"
	const episodioID = "00000000-0000-4000-8000-0000000000d3"
	rec, _ := dominio.NovaReceita(episodioID, doenteID, "00000000-0000-4000-8000-0000000000d4", []dominio.ItemReceita{item}, "", time.Now(), time.Now().AddDate(0, 0, 30))
	recID, err := repoReceitas.Guardar(ctx, rec)
	if err != nil {
		t.Fatalf("guardar receita: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.receitas WHERE id=$1`, recID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.movimentos_stock WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.lotes WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.medicamentos WHERE id=$1`, medID)
	})

	// Snapshot ÚNICO, partilhado pelas duas goroutines: simula duas requisições
	// HTTP concorrentes que ambas leram a MESMA receita (quantidade_dispensada=0)
	// antes de qualquer uma ter confirmado — exactamente o cenário do bug.
	lida, err := repoReceitas.ObterPorID(ctx, recID)
	if err != nil {
		t.Fatalf("obter receita: %v", err)
	}
	snapshotPartilhado := lida.Snapshot()

	type resultado struct {
		quem        string
		err         error
		inicio, fim time.Time
	}
	resultados := make(chan resultado, 2)

	pronto := make(chan struct{}) // barreira: ambas só avançam depois de AMBAS estarem prontas
	var arrancarWG sync.WaitGroup
	arrancarWG.Add(2)
	var execWG sync.WaitGroup
	execWG.Add(2)

	disparar := func(quem string) {
		defer execWG.Done()
		arrancarWG.Done()
		<-pronto // barreira de arranque simultâneo
		inicio := time.Now()
		t.Logf("[%s] a chamar Dispensar às %s (ainda ninguém confirmou)", quem, inicio.Format(time.RFC3339Nano))
		_, callErr := motor.Dispensar(ctx, snapshotPartilhado,
			[]appfarmacia.ItemDispensa{{MedicamentoID: medID, Quantidade: 100}},
			"00000000-0000-4000-8000-0000000000d5")
		fim := time.Now()
		t.Logf("[%s] Dispensar terminou às %s (duração=%s) err=%v", quem, fim.Format(time.RFC3339Nano), fim.Sub(inicio), callErr)
		resultados <- resultado{quem: quem, err: callErr, inicio: inicio, fim: fim}
	}

	go disparar("A")
	go disparar("B")
	go func() { arrancarWG.Wait(); close(pronto) }()
	execWG.Wait()
	close(resultados)

	var todos []resultado
	for r := range resultados {
		todos = append(todos, r)
	}
	if len(todos) != 2 {
		t.Fatalf("esperava 2 resultados, obtive %d", len(todos))
	}

	// Prova de que a corrida foi REAL (não apenas duas chamadas sequenciais
	// disfarçadas): os intervalos [inicio,fim] das duas goroutines sobrepõem-se,
	// ou seja, ambas estavam "em voo" ao mesmo tempo — a segunda arrancou
	// (chamou Dispensar) antes de a primeira ter terminado (commit/rollback).
	sobrepoem := todos[0].inicio.Before(todos[1].fim) && todos[1].inicio.Before(todos[0].fim)
	t.Logf("A=[%s , %s]  B=[%s , %s]  sobrepõem-se=%v",
		todos[0].inicio.Format(time.RFC3339Nano), todos[0].fim.Format(time.RFC3339Nano),
		todos[1].inicio.Format(time.RFC3339Nano), todos[1].fim.Format(time.RFC3339Nano), sobrepoem)
	if !sobrepoem {
		t.Fatal("os intervalos de execução não se sobrepõem — o teste não prova concorrência real; corra novamente ou reveja a barreira")
	}

	var sucesso, falha int
	var erroFalha error
	for _, r := range todos {
		if r.err == nil {
			sucesso++
		} else {
			falha++
			erroFalha = r.err
		}
	}
	if sucesso != 1 || falha != 1 {
		t.Fatalf("esperava exactamente 1 confirmação e 1 falha sob concorrência, obtive sucesso=%d falha=%d (A.err=%v B.err=%v)",
			sucesso, falha, todos[0].err, todos[1].err)
	}
	if erros.CategoriaDe(erroFalha) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava que a dispensa perdedora falhasse com CategoriaRegraNegocio (não exceder o prescrito), obtive %v (%v)",
			erros.CategoriaDe(erroFalha), erroFalha)
	}

	// Stock: só 100 (não 200) foram consumidos do lote de 200 — prova directa
	// de que não houve sobre-dispensa de stock físico.
	l, err := repoLotes.ObterPorID(ctx, loteID)
	if err != nil {
		t.Fatalf("obter lote: %v", err)
	}
	if l.QuantidadeActual() != 100 {
		t.Fatalf("SOBRE-DISPENSA DE STOCK: quantidade_actual do lote = %d, esperava 100 (200 iniciais - 100 consumidos)", l.QuantidadeActual())
	}

	// Exactamente um movimento SAIDA_DISPENSA (não dois).
	var nMov int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM farmacia.movimentos_stock WHERE medicamento_id=$1 AND tipo=$2`,
		medID, string(dominio.MovimentoSaidaDispensa)).Scan(&nMov); err != nil {
		t.Fatalf("contar movimentos de saída: %v", err)
	}
	if nMov != 1 {
		t.Fatalf("SOBRE-DISPENSA: %d movimentos SAIDA_DISPENSA registados, esperava exactamente 1", nMov)
	}

	// Receita: exactamente 100 dispensados (não 200), estado DISPENSADA.
	final, err := repoReceitas.ObterPorID(ctx, recID)
	if err != nil {
		t.Fatalf("obter receita final: %v", err)
	}
	if got := final.Snapshot().Itens[0].QuantidadeDispensada; got != 100 {
		t.Fatalf("SOBRE-DISPENSA NA RECEITA: quantidade_dispensada=%d, esperava exactamente 100 (não 200)", got)
	}
	if final.Estado() != dominio.ReceitaDispensada {
		t.Fatalf("estado da receita=%v, esperava DISPENSADA", final.Estado())
	}
}
