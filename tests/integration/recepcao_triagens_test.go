//go:build integration

// Teste de integração do repositório pgx RepositorioTriagens (BC Recepção,
// Task 8). Segue o padrão de migracoes_test.go: ligar(t) só liga o pool, as
// migrações aplicam-se explicitamente a seguir, e SKIPa (nunca FAIL) quando
// DATABASE_URL não está definido.
//
// A prova da guarda CAS segue o mesmo padrão de
// TestRecepcaoChegadasRepo_CheckinTransaccionalEUnique (recepcao_chegadas_test.go):
// duas leituras da MESMA chegada CHAMADO, ambas ANTES da primeira triagem — cada
// uma fica com EstadoAnterior=CHAMADO, o estado real na BD nesse momento
// (ReconstruirChegada fixa sempre EstadoAnterior=Estado do snapshot lido). A
// primeira triagem sucede a partir da 1ª leitura; a segunda, a partir da 2ª
// leitura (que ainda "pensa" CHAMADO), é rejeitada pela guarda CAS do UPDATE
// `WHERE estado='CHAMADO'` (0 linhas — a BD já está TRIADO), ANTES do INSERT.
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

func TestRecepcaoTriagensRepo_RegistarObterFila(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	chegRepo := pgrepo.NovoRepositorioChegadas(pool)
	triRepo := pgrepo.NovoRepositorioTriagens(pool)
	medico := "77777777-7777-7777-7777-777777777777"
	esp := "88888888-8888-8888-8888-888888888888"
	enfermeiro := "aaaaaaaa-0000-0000-0000-000000000001"

	// walk-in chamado (sem médico)
	c, err := dominio.NovaChegadaWalkIn("99999999-9999-9999-9999-999999999999", esp, instMarcacao(t, "2026-08-20T09:00:00Z"))
	if err != nil {
		t.Fatalf("construir chegada walk-in: %v", err)
	}
	if err := c.Chamar(instMarcacao(t, "2026-08-20T09:05:00Z")); err != nil {
		t.Fatalf("chamar (domínio): %v", err)
	}
	cid, err := chegRepo.Guardar(ctx, c)
	if err != nil {
		t.Fatalf("guardar chegada: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.triagens WHERE chegada_id=$1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.chegadas WHERE id=$1`, cid)
	})

	// duas leituras da MESMA chegada CHAMADO, ambas ANTES de qualquer triagem — ver nota
	// de topo do ficheiro sobre a prova da guarda CAS.
	obtidaA, err := chegRepo.ObterPorID(ctx, cid)
	if err != nil {
		t.Fatalf("obter chegada (1ª leitura): %v", err)
	}
	obtidaB, err := chegRepo.ObterPorID(ctx, cid)
	if err != nil {
		t.Fatalf("obter chegada (2ª leitura): %v", err)
	}

	// registar triagem: transita CHAMADO→TRIADO + atribui médico + insere triagem
	if err := obtidaA.RegistarTriada(medico, instMarcacao(t, "2026-08-20T09:10:00Z")); err != nil {
		t.Fatalf("registar triada (domínio): %v", err)
	}
	sinais, err := dominio.NovosSinaisVitais(dominio.SinaisVitais{Temperatura: fptrI(37.5), SaturacaoO2: iptrI(98)})
	if err != nil {
		t.Fatalf("construir sinais vitais: %v", err)
	}
	tr, err := dominio.NovaTriagem(cid, enfermeiro, dominio.ManAmarelo, sinais, "cefaleia", instMarcacao(t, "2026-08-20T09:10:00Z"))
	if err != nil {
		t.Fatalf("construir triagem: %v", err)
	}
	tid, err := triRepo.RegistarTriagem(ctx, tr, obtidaA)
	if err != nil {
		t.Fatalf("registar triagem (repo): %v", err)
	}

	// a chegada ficou TRIADO com o médico
	rec, err := chegRepo.ObterPorID(ctx, cid)
	if err != nil {
		t.Fatalf("obter chegada recarregada: %v", err)
	}
	if rec.Estado() != dominio.ChegTriado || rec.MedicoID() != medico {
		t.Fatalf("chegada mal transitada: estado=%s medico=%s", rec.Estado(), rec.MedicoID())
	}

	// obter por chegada devolve a triagem com os sinais vitais
	got, err := triRepo.ObterPorChegada(ctx, cid)
	if err != nil || got.ID() != tid {
		t.Fatalf("obter por chegada: %v (%v)", err, got)
	}
	if got.SinaisVitais().Temperatura == nil || *got.SinaisVitais().Temperatura != 37.5 {
		t.Fatalf("temperatura mal persistida: %+v", got.SinaisVitais())
	}
	if got.SinaisVitais().SaturacaoO2 == nil || *got.SinaisVitais().SaturacaoO2 != 98 {
		t.Fatalf("saturação O2 mal persistida: %+v", got.SinaisVitais())
	}

	// fila clínica do médico devolve esta chegada
	fila, err := triRepo.ListarFilaClinica(ctx, medico)
	if err != nil || len(fila) != 1 || fila[0].ChegadaID != cid {
		t.Fatalf("fila clínica: %v (n=%d)", err, len(fila))
	}

	// segunda triagem da mesma chegada, a partir da 2ª leitura (que ainda "pensa"
	// CHAMADO) → Conflito pela guarda CAS, ANTES do INSERT (a UNIQUE de
	// recepcao.triagens nem chega a entrar em jogo).
	if err := obtidaB.RegistarTriada(medico, instMarcacao(t, "2026-08-20T09:20:00Z")); err != nil {
		t.Fatalf("registar triada (domínio, 2ª): %v", err)
	}
	tr2, err := dominio.NovaTriagem(cid, enfermeiro, dominio.ManVerde, dominio.SinaisVitais{}, "", instMarcacao(t, "2026-08-20T09:20:00Z"))
	if err != nil {
		t.Fatalf("construir 2ª triagem: %v", err)
	}
	if _, err := triRepo.RegistarTriagem(ctx, tr2, obtidaB); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("triagem duplicada devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestRecepcaoTriagensRepo_FilaOrdenadaPorPrioridade(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	chegRepo := pgrepo.NovoRepositorioChegadas(pool)
	triRepo := pgrepo.NovoRepositorioTriagens(pool)
	medico := "66666666-6666-6666-6666-666666666666"
	esp := "88888888-8888-8888-8888-888888888888"
	enfermeiro := "aaaaaaaa-0000-0000-0000-000000000001"

	triar := func(doente, hora string, p dominio.PrioridadeManchester) string {
		t.Helper()
		c, err := dominio.NovaChegadaWalkIn(doente, esp, instMarcacao(t, hora))
		if err != nil {
			t.Fatalf("construir chegada walk-in: %v", err)
		}
		if err := c.Chamar(instMarcacao(t, hora)); err != nil {
			t.Fatalf("chamar (domínio): %v", err)
		}
		cid, err := chegRepo.Guardar(ctx, c)
		if err != nil {
			t.Fatalf("guardar chegada: %v", err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM recepcao.triagens WHERE chegada_id=$1`, cid)
			_, _ = pool.Exec(ctx, `DELETE FROM recepcao.chegadas WHERE id=$1`, cid)
		})
		obt, err := chegRepo.ObterPorID(ctx, cid)
		if err != nil {
			t.Fatalf("obter chegada: %v", err)
		}
		if err := obt.RegistarTriada(medico, instMarcacao(t, hora)); err != nil {
			t.Fatalf("registar triada (domínio): %v", err)
		}
		tr, err := dominio.NovaTriagem(cid, enfermeiro, p, dominio.SinaisVitais{}, "", instMarcacao(t, hora))
		if err != nil {
			t.Fatalf("construir triagem: %v", err)
		}
		if _, err := triRepo.RegistarTriagem(ctx, tr, obt); err != nil {
			t.Fatalf("registar triagem: %v", err)
		}
		return cid
	}
	// o VERDE chega primeiro; o VERMELHO chega depois mas é mais urgente
	_ = triar("11111111-2222-2222-2222-222222222222", "2026-08-21T08:00:00Z", dominio.ManVerde)
	vermelho := triar("11111111-3333-3333-3333-333333333333", "2026-08-21T09:00:00Z", dominio.ManVermelho)

	fila, err := triRepo.ListarFilaClinica(ctx, medico)
	if err != nil || len(fila) < 2 {
		t.Fatalf("fila clínica: %v (n=%d)", err, len(fila))
	}
	if fila[0].ChegadaID != vermelho {
		t.Fatalf("o VERMELHO devia vir primeiro, veio %+v", fila[0])
	}
}

func iptrI(v int) *int         { return &v }
func fptrI(v float64) *float64 { return &v }
