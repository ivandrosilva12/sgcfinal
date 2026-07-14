//go:build integration

// Teste de integração da cirurgia ambulatória contra a BD real. Segue o padrão
// de episodios_test.go: SKIP (nunca FAIL) quando DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestCirurgia_CicloEProibicoes(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	repoEp := pgrepo.NovoRepositorioEpisodios(pool)
	repoCons := pgrepo.NovoRepositorioConsentimentos(pool)
	repoProc := pgrepo.NovoRepositorioProcedimentos(pool)
	repoCat := pgrepo.NovoRepositorioCatalogoProcedimentos(pool)

	// O catálogo tem PRC001, vindo do seed da migração 0005.
	cat, err := repoCat.ObterPorCodigo(ctx, "PRC001")
	if err != nil || cat.Codigo != "PRC001" {
		t.Fatalf("catálogo PRC001 devia existir do seed: %v", err)
	}

	// Doente + episódio de cirurgia ambulatória via agregados de domínio.
	nasc := time.Date(1985, 5, 5, 0, 0, 0, 0, time.UTC)
	bi := "00345678LA044"
	num, err := repoDoentes.ProximoNumeroProcesso(ctx, 2026)
	if err != nil {
		t.Fatalf("próximo número: %v", err)
	}
	ident, _ := dominio.NovaIdentificacao("Teste Cirurgia", nasc, dominio.SexoFeminino, &bi, nil, nil)
	ct, _ := dominio.NovosContactos("+244923111111", nil, nil)
	doente, _ := dominio.NovoDoente(num, ident, ct, "AO")
	doenteID, err := repoDoentes.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar doente: %v", err)
	}

	const espID = "00000000-0000-4000-8000-0000000000e1"
	const medID = "00000000-0000-4000-8000-0000000000e2"
	ep, _ := dominio.NovoEpisodio(doenteID, dominio.EpisodioCirurgiaAmbulatoria, espID, medID, time.Now())
	episodioID, err := repoEp.Guardar(ctx, ep)
	if err != nil {
		t.Fatalf("guardar episódio: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.procedimentos_cirurgicos WHERE episodio_id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.consentimentos WHERE doente_id=$1`, doenteID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.episodios_clinicos WHERE id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})

	cons, err := dominio.NovoConsentimento(doenteID, dominio.FinalidadeCirurgia, true, "s3://c.pdf",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("consentimento inválido: %v", err)
	}
	consID, err := repoCons.Guardar(ctx, cons)
	if err != nil {
		t.Fatalf("guardar consentimento: %v", err)
	}
	consPersistido, err := repoCons.ObterPorID(ctx, consID)
	if err != nil {
		t.Fatalf("obter consentimento: %v", err)
	}

	proc, err := dominio.NovoProcedimento(dominio.DadosNovoProcedimento{
		EpisodioID: episodioID, Codigo: "PRC001", Descricao: "Sutura de ferida superficial",
		CirurgiaoID: "00000000-0000-4000-8000-0000000000c1", Anestesia: dominio.AnestesiaLocal,
		AnestesistaID: "00000000-0000-4000-8000-0000000000a1",
	}, consPersistido)
	if err != nil {
		t.Fatalf("novo procedimento: %v", err)
	}
	procID, err := repoProc.Guardar(ctx, proc)
	if err != nil {
		t.Fatalf("guardar procedimento: %v", err)
	}

	// Ciclo: AGENDADO → EM_CURSO → CONCLUIDO, persistido e coerente com as
	// CHECKs estado↔timestamps de 0006 (é o UPDATE de transição do
	// procedimentos_repo que aqui se exercita a sério).
	p, err := repoProc.ObterPorID(ctx, procID)
	if err != nil {
		t.Fatalf("obter procedimento: %v", err)
	}
	if err := p.Iniciar(time.Now()); err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if _, err := repoProc.Guardar(ctx, p); err != nil {
		t.Fatalf("persistir início: %v", err)
	}
	p2, err := repoProc.ObterPorID(ctx, procID)
	if err != nil {
		t.Fatalf("obter procedimento após início: %v", err)
	}
	if err := p2.Concluir(time.Now().Add(time.Hour), "sem complicações", ""); err != nil {
		t.Fatalf("concluir: %v", err)
	}
	if _, err := repoProc.Guardar(ctx, p2); err != nil {
		t.Fatalf("persistir conclusão: %v", err)
	}
	final, err := repoProc.ObterPorID(ctx, procID)
	if err != nil {
		t.Fatalf("obter procedimento final: %v", err)
	}
	if final.Estado() != dominio.ProcConcluido {
		t.Fatalf("esperado CONCLUIDO, veio %s", final.Estado())
	}

	// Proibição: um consentimento não-cirúrgico não deixa construir o
	// procedimento (invariante-estrela, CategoriaRegraNegocio).
	consTrat, err := dominio.NovoConsentimento(doenteID, dominio.FinalidadeTratamento, true, "",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("consentimento de tratamento inválido: %v", err)
	}
	if _, err := dominio.NovoProcedimento(dominio.DadosNovoProcedimento{
		EpisodioID: episodioID, Codigo: "PRC001", Descricao: "X",
		CirurgiaoID: "00000000-0000-4000-8000-0000000000c1", Anestesia: dominio.AnestesiaNenhuma,
	}, consTrat); err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("consentimento não-cirúrgico devia bloquear com RegraNegocio, veio %v", err)
	}
}
