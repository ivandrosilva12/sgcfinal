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

	// duas leituras da MESMA marcação MARCADA, ambas ANTES de qualquer check-in — cada
	// uma fica com EstadoAnterior=MARCADA, o estado real na BD nesse momento
	// (ReconstruirMarcacao fixa sempre EstadoAnterior=Estado do snapshot lido).
	original1, err := marc.ObterPorID(ctx, mid)
	if err != nil {
		t.Fatalf("obter marcação (1ª leitura): %v", err)
	}
	original2, err := marc.ObterPorID(ctx, mid)
	if err != nil {
		t.Fatalf("obter marcação (2ª leitura): %v", err)
	}

	// primeiro check-in: transita a marcação + cria a chegada, atomicamente — tem de suceder
	if err := original1.RegistarComparencia(instMarcacao(t, "2026-08-11T08:50:00Z")); err != nil {
		t.Fatalf("registar comparência (domínio, 1ª): %v", err)
	}
	ch1, err := dominio.NovaChegadaAgendada(original1.DoenteID(), original1.ID(), original1.MedicoID(),
		original1.EspecialidadeID(), instMarcacao(t, "2026-08-11T08:50:00Z"))
	if err != nil {
		t.Fatalf("construir 1ª chegada agendada: %v", err)
	}
	if _, err := cheg.RegistarChegadaAgendada(ctx, ch1, original1); err != nil {
		t.Fatalf("check-in transaccional (1º): %v", err)
	}
	// a marcação ficou COMPARECEU
	recarregada, err := marc.ObterPorID(ctx, mid)
	if err != nil {
		t.Fatalf("obter marcação recarregada: %v", err)
	}
	if recarregada.Estado() != dominio.MarcCompareceu {
		t.Fatalf("marcação devia estar COMPARECEU, veio %s", recarregada.Estado())
	}

	// segundo check-in, a partir de original2: foi lida ANTES do primeiro check-in, por
	// isso ainda "pensa" que a marcação está MARCADA (EstadoAnterior=MARCADA). No
	// domínio, RegistarComparencia sucede (Estado()==MARCADA em memória). Mas no
	// repositório, o UPDATE `WHERE estado='MARCADA'` já não encontra nenhuma linha — a
	// BD tem COMPARECEU desde o primeiro check-in — pelo que RowsAffected()==0 e
	// RegistarChegadaAgendada devolve Conflito ANTES de sequer chegar ao INSERT da
	// chegada (a restrição UNIQUE nem chega a entrar em jogo). É esta guarda CAS,
	// genuína, que o teste afirma a seguir.
	if err := original2.RegistarComparencia(instMarcacao(t, "2026-08-11T08:55:00Z")); err != nil {
		t.Fatalf("registar comparência (domínio, 2ª): %v", err)
	}
	ch2, err := dominio.NovaChegadaAgendada(original2.DoenteID(), original2.ID(), original2.MedicoID(),
		original2.EspecialidadeID(), instMarcacao(t, "2026-08-11T08:55:00Z"))
	if err != nil {
		t.Fatalf("construir 2ª chegada agendada: %v", err)
	}
	if _, err := cheg.RegistarChegadaAgendada(ctx, ch2, original2); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("check-in duplo devia dar Conflito (guarda CAS da marcação), veio %v", erros.CategoriaDe(err))
	}
}
