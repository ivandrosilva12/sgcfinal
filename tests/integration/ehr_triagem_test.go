//go:build integration

// Teste de integração do LeitorTriagem (triagem no EHR, ADR-037) contra a BD
// real: prova a junção recepcao.chegadas ⋈ recepcao.triagens por episodio_id.
// SKIPa (nunca FAIL) quando DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	domclinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestLeitorTriagem_TriagemDoEpisodio(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	doenteID, chegadaID := criaChegadaTriadaComDoente(t, pool, ctx, "00987659LA026", "+244923111777")
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)

	// episódio nascido da fila (ConsumirEIniciar liga chegada→episódio)
	ep, err := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	if err != nil {
		t.Fatalf("construir episódio: %v", err)
	}
	epID, err := integ.ConsumirEIniciar(ctx, chegadaID, medInicioConsulta, ep)
	if err != nil {
		t.Fatalf("consumir e iniciar: %v", err)
	}

	tr, ok, err := integ.TriagemDoEpisodio(ctx, epID)
	if err != nil || !ok {
		t.Fatalf("triagem do episódio: %v (ok=%v)", err, ok)
	}
	if tr.Prioridade != "VERDE" || tr.EnfermeiroID != enfInicioConsulta {
		t.Fatalf("triagem inesperada: %+v", tr)
	}
	if tr.SinaisVitais.Temperatura != nil {
		t.Fatalf("sinais vitais deviam estar vazios (não medidos): %+v", tr.SinaisVitais)
	}
	if tr.TriadaEm.IsZero() {
		t.Fatal("triadaEm em falta")
	}

	// episódio criado pelo endpoint antigo (sem chegada) → ok=false, sem erro
	epAntigo, err := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	if err != nil {
		t.Fatalf("construir episódio antigo: %v", err)
	}
	epAntigoID, err := pgrepo.NovoRepositorioEpisodios(pool).Guardar(ctx, epAntigo)
	if err != nil {
		t.Fatalf("guardar episódio antigo: %v", err)
	}
	if _, ok, err := integ.TriagemDoEpisodio(ctx, epAntigoID); err != nil || ok {
		t.Fatalf("episódio sem fila devia dar ok=false sem erro, veio ok=%v err=%v", ok, err)
	}

	// lote com mistura: só o episódio da fila aparece no mapa
	prioridades, err := integ.PrioridadesDosEpisodios(ctx, []string{epID, epAntigoID})
	if err != nil {
		t.Fatalf("prioridades em lote: %v", err)
	}
	if len(prioridades) != 1 || prioridades[epID] != "VERDE" {
		t.Fatalf("lote inesperado: %+v", prioridades)
	}

	// lote vazio → mapa vazio, sem erro (sem tocar na BD)
	vazio, err := integ.PrioridadesDosEpisodios(ctx, nil)
	if err != nil || len(vazio) != 0 {
		t.Fatalf("lote vazio devia dar mapa vazio: %v (%v)", vazio, err)
	}
}
