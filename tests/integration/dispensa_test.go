//go:build integration

// Teste de integração do BC Farmácia (motor de dispensa transaccional FEFO)
// contra a BD real. Segue o padrão de stock_test.go: SKIP (nunca FAIL) quando
// DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestMotorDispensa_FEFO(t *testing.T) {
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
	m, _ := dominio.NovoMedicamento(cod, "Amoxil Disp", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, 10)
	medID, _ := repoMed.Guardar(ctx, m)

	// Lote A expira antes (deve ser consumido primeiro), Lote B depois.
	loteA, _ := dominio.NovoLote(medID, "A", time.Now().AddDate(0, 1, 0), 15, "1", nil, "")
	loteAID, _ := repoLotes.RegistarEntrada(ctx, loteA, "00000000-0000-4000-8000-0000000000b1")
	loteB, _ := dominio.NovoLote(medID, "B", time.Now().AddDate(0, 6, 0), 30, "1", nil, "")
	loteBID, _ := repoLotes.RegistarEntrada(ctx, loteB, "00000000-0000-4000-8000-0000000000b1")

	// Receita com 1 item prescrito 40, do medicamento.
	item, _ := dominio.NovoItemReceita(medID, "1 comp", nil, 40, "")
	const doenteID = "00000000-0000-4000-8000-0000000000b2"
	const episodioID = "00000000-0000-4000-8000-0000000000b3"
	rec, _ := dominio.NovaReceita(episodioID, doenteID, "00000000-0000-4000-8000-0000000000b4", []dominio.ItemReceita{item}, "", time.Now(), time.Now().AddDate(0, 0, 30))
	recID, _ := repoReceitas.Guardar(ctx, rec)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.receitas WHERE id=$1`, recID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.movimentos_stock WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.lotes WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.medicamentos WHERE id=$1`, medID)
	})

	// Dispensa parcial de 20: FEFO → 15 de A + 5 de B; receita fica PARCIAL.
	lido, _ := repoReceitas.ObterPorID(ctx, recID)
	_ = lido.RegistarDispensa(medID, 20)
	snap := lido.Snapshot()
	snap.ID = recID
	if _, err := motor.Dispensar(ctx, snap, []appfarmacia.ItemDispensa{{MedicamentoID: medID, Quantidade: 20}}, "00000000-0000-4000-8000-0000000000b5"); err != nil {
		t.Fatalf("dispensar: %v", err)
	}

	// Confirmar FEFO: A esgotado (0), B com 25.
	la, _ := repoLotes.ObterPorID(ctx, loteAID)
	lb, _ := repoLotes.ObterPorID(ctx, loteBID)
	if la.QuantidadeActual() != 0 || lb.QuantidadeActual() != 25 {
		t.Fatalf("FEFO errado: A=%d (esperava 0), B=%d (esperava 25)", la.QuantidadeActual(), lb.QuantidadeActual())
	}
	final, _ := repoReceitas.ObterPorID(ctx, recID)
	if final.Estado() != dominio.ReceitaParcial || final.Snapshot().Itens[0].QuantidadeDispensada != 20 {
		t.Fatalf("receita não ficou PARCIAL/20: estado=%v qtd=%d", final.Estado(), final.Snapshot().Itens[0].QuantidadeDispensada)
	}

	// Segunda dispensa de 20 → total 40 → DISPENSADA.
	lido2, _ := repoReceitas.ObterPorID(ctx, recID)
	_ = lido2.RegistarDispensa(medID, 20)
	snap2 := lido2.Snapshot()
	if _, err := motor.Dispensar(ctx, snap2, []appfarmacia.ItemDispensa{{MedicamentoID: medID, Quantidade: 20}}, "00000000-0000-4000-8000-0000000000b5"); err != nil {
		t.Fatalf("segunda dispensa: %v", err)
	}
	final2, _ := repoReceitas.ObterPorID(ctx, recID)
	if final2.Estado() != dominio.ReceitaDispensada {
		t.Fatalf("esperava DISPENSADA, obtive %v", final2.Estado())
	}
}

// TestMotorDispensa_StockInsuficiente_RollbackTotal prova que, quando o stock
// disponível não chega para a quantidade pedida, a transacção é totalmente
// revertida: nenhum lote é decrementado, nenhum movimento é registado e a
// receita mantém o estado/quantidades anteriores à tentativa.
func TestMotorDispensa_StockInsuficiente_RollbackTotal(t *testing.T) {
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
	m, _ := dominio.NovoMedicamento(cod, "Amoxil Insuf", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, 10)
	medID, _ := repoMed.Guardar(ctx, m)

	// Só existem 10 unidades disponíveis num único lote.
	lote, _ := dominio.NovoLote(medID, "U", time.Now().AddDate(0, 1, 0), 10, "1", nil, "")
	loteID, _ := repoLotes.RegistarEntrada(ctx, lote, "00000000-0000-4000-8000-0000000000c1")

	item, _ := dominio.NovoItemReceita(medID, "1 comp", nil, 40, "")
	const doenteID = "00000000-0000-4000-8000-0000000000c2"
	const episodioID = "00000000-0000-4000-8000-0000000000c3"
	rec, _ := dominio.NovaReceita(episodioID, doenteID, "00000000-0000-4000-8000-0000000000c4", []dominio.ItemReceita{item}, "", time.Now(), time.Now().AddDate(0, 0, 30))
	recID, _ := repoReceitas.Guardar(ctx, rec)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.receitas WHERE id=$1`, recID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.movimentos_stock WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.lotes WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.medicamentos WHERE id=$1`, medID)
	})

	// Tentativa de dispensar 20, mas só há 10 em stock: deve falhar com
	// RegraNegocio e não alterar nada.
	lido, _ := repoReceitas.ObterPorID(ctx, recID)
	_ = lido.RegistarDispensa(medID, 20)
	snap := lido.Snapshot()
	snap.ID = recID
	_, err := motor.Dispensar(ctx, snap, []appfarmacia.ItemDispensa{{MedicamentoID: medID, Quantidade: 20}}, "00000000-0000-4000-8000-0000000000c5")
	if err == nil {
		t.Fatal("esperava erro de stock insuficiente")
	}
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio, obtive %v (%v)", erros.CategoriaDe(err), err)
	}

	// Nada deve ter mudado: lote intacto, receita ainda EMITIDA/0 e nenhum
	// movimento SAIDA_DISPENSA registado (só o ENTRADA da semeadura inicial
	// do lote deve existir).
	l, _ := repoLotes.ObterPorID(ctx, loteID)
	if l.QuantidadeActual() != 10 {
		t.Fatalf("lote foi alterado apesar do rollback: quantidade_actual=%d, esperava 10", l.QuantidadeActual())
	}
	var nMov int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM farmacia.movimentos_stock WHERE medicamento_id=$1 AND tipo=$2`,
		medID, string(dominio.MovimentoSaidaDispensa)).Scan(&nMov); err != nil {
		t.Fatalf("contar movimentos de saída: %v", err)
	}
	if nMov != 0 {
		t.Fatalf("movimento de saída registado apesar do rollback: %d", nMov)
	}
	final, _ := repoReceitas.ObterPorID(ctx, recID)
	if final.Estado() != dominio.ReceitaEmitida || final.Snapshot().Itens[0].QuantidadeDispensada != 0 {
		t.Fatalf("receita foi alterada apesar do rollback: estado=%v qtd=%d", final.Estado(), final.Snapshot().Itens[0].QuantidadeDispensada)
	}
}
