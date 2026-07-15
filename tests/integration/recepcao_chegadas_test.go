//go:build integration

// Teste de integração do repositório pgx RepositorioChegadas (BC Recepção,
// Task 6). Segue o padrão de migracoes_test.go: ligar(t) só liga o pool, as
// migrações aplicam-se explicitamente a seguir, e SKIPa (nunca FAIL) quando
// DATABASE_URL não está definido.
//
// Reutiliza instMarcacao(t, s) — já definido em recepcao_marcacoes_test.go,
// mesmo package — para converter literais RFC3339 em time.Time.
package integration_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestRecepcaoChegadasRepo_WalkInTransitarFila(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repo := pgrepo.NovoRepositorioChegadas(pool)
	esp := "66666666-6666-6666-6666-666666666666"

	c, err := dominio.NovaChegadaWalkIn(
		"77777777-7777-7777-7777-777777777777", esp, instMarcacao(t, "2026-08-10T09:00:00Z"))
	if err != nil {
		t.Fatalf("construir chegada walk-in: %v", err)
	}
	id, err := repo.Guardar(ctx, c)
	if err != nil {
		t.Fatalf("guardar walk-in: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.chegadas WHERE id=$1`, id)
	})

	fila, err := repo.ListarFila(ctx, esp)
	if err != nil || len(fila) != 1 || fila[0].ID != id {
		t.Fatalf("fila: %v (n=%d)", err, len(fila))
	}

	obtida, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if err := obtida.Chamar(instMarcacao(t, "2026-08-10T09:10:00Z")); err != nil {
		t.Fatalf("chamar (domínio): %v", err)
	}
	if err := repo.Transitar(ctx, obtida); err != nil {
		t.Fatalf("transitar: %v", err)
	}
	// CHAMADO já não aparece na fila
	if fila2, err := repo.ListarFila(ctx, esp); err != nil || len(fila2) != 0 {
		t.Fatalf("CHAMADO não devia estar na fila, veio n=%d (%v)", len(fila2), err)
	}
}

func TestRecepcaoChegadasRepo_CheckinTransaccionalEUnique(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	marc := pgrepo.NovoRepositorioMarcacoes(pool)
	cheg := pgrepo.NovoRepositorioChegadas(pool)
	doente := "88888888-8888-8888-8888-888888888888"
	medico := "99999999-9999-9999-9999-999999999999"
	especialidade := "aaaaaaaa-1111-1111-1111-111111111111"

	// marcação MARCADA
	m, err := dominio.NovaMarcacao(
		doente, medico, especialidade,
		instMarcacao(t, "2026-08-11T09:00:00Z"), instMarcacao(t, "2026-08-11T09:30:00Z"))
	if err != nil {
		t.Fatalf("construir marcação: %v", err)
	}
	mid, err := marc.Guardar(ctx, m)
	if err != nil {
		t.Fatalf("guardar marcação: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.chegadas WHERE marcacao_id=$1`, mid)
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.marcacoes WHERE id=$1`, mid)
	})

	// check-in: transita a marcação + cria a chegada, atomicamente
	original, err := marc.ObterPorID(ctx, mid)
	if err != nil {
		t.Fatalf("obter marcação original: %v", err)
	}
	if err := original.RegistarComparencia(instMarcacao(t, "2026-08-11T08:50:00Z")); err != nil {
		t.Fatalf("registar comparência (domínio): %v", err)
	}
	ch, err := dominio.NovaChegadaAgendada(original.DoenteID(), original.ID(), original.MedicoID(),
		original.EspecialidadeID(), instMarcacao(t, "2026-08-11T08:50:00Z"))
	if err != nil {
		t.Fatalf("construir chegada agendada: %v", err)
	}
	if _, err := cheg.RegistarChegadaAgendada(ctx, ch, original); err != nil {
		t.Fatalf("check-in transaccional: %v", err)
	}
	// a marcação ficou COMPARECEU
	recarregada, err := marc.ObterPorID(ctx, mid)
	if err != nil {
		t.Fatalf("obter marcação recarregada: %v", err)
	}
	if recarregada.Estado() != dominio.MarcCompareceu {
		t.Fatalf("marcação devia estar COMPARECEU, veio %s", recarregada.Estado())
	}

	// segundo check-in da MESMA marcação: a guarda CAS (marcação já não MARCADA) nega
	original2, err := marc.ObterPorID(ctx, mid) // está COMPARECEU
	if err != nil {
		t.Fatalf("obter marcação (segunda leitura): %v", err)
	}
	// forçar uma tentativa como se ainda estivesse MARCADA não é possível pelo domínio;
	// aqui exercitamos a guarda do repositório directamente construindo o cenário:
	if _, err := cheg.RegistarChegadaAgendada(ctx, ch, recarregadaComoMarcada(t, original2)); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("check-in duplo devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
}

// recarregadaComoMarcada reconstrói a marcação como se ainda estivesse MARCADA (o
// EstadoAnterior fica MARCADA), para exercitar a guarda CAS do repositório contra a
// linha que já está COMPARECEU na BD.
func recarregadaComoMarcada(t *testing.T, m *dominio.Marcacao) *dominio.Marcacao {
	t.Helper()
	s := m.Snapshot()
	s.Estado = dominio.MarcCompareceu      // o que se quer escrever
	s.EstadoAnterior = dominio.MarcMarcada // a guarda que já não bate (a BD tem COMPARECEU)
	return dominio.ReconstruirMarcacao(s)
}
