//go:build integration

// Teste de integração do BC Clínico (agregado EpisodioClinico) contra a BD real.
// Segue o padrão de doentes_test.go: SKIP (nunca FAIL) quando DATABASE_URL não
// está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestRepositorioEpisodios_CicloCompleto(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	repoEp := pgrepo.NovoRepositorioEpisodios(pool)

	// Cria um doente (FK).
	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	bi := "00123456LA042"
	num, _ := repoDoentes.ProximoNumeroProcesso(ctx, 2026)
	ident, _ := dominio.NovaIdentificacao("Ana Episódio", nasc, dominio.SexoFeminino, &bi, nil, nil)
	ct, _ := dominio.NovosContactos("+244923456789", nil, nil)
	doente, _ := dominio.NovoDoente(num, ident, ct, "AO")
	doenteID, err := repoDoentes.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar doente: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.episodios_clinicos WHERE doente_id=$1`, doenteID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})

	// Iniciar episódio (medico/especialidade são uuid — usa valores uuid válidos).
	const espID = "00000000-0000-4000-8000-000000000001"
	const medID = "00000000-0000-4000-8000-000000000002"
	ep, _ := dominio.NovoEpisodio(doenteID, dominio.EpisodioConsulta, espID, medID, time.Now())
	epID, err := repoEp.Guardar(ctx, ep)
	if err != nil {
		t.Fatalf("guardar episódio: %v", err)
	}

	// Preencher nota + CID e fechar.
	lido, err := repoEp.ObterPorID(ctx, epID)
	if err != nil {
		t.Fatalf("obter episódio: %v", err)
	}
	_ = lido.ActualizarNota(dominio.NovaNotaClinica("Febre", "", "Temp 39", "Gripe", "Repouso"))
	cid, _ := dominio.NovoDiagnosticoCID("J11", true)
	_ = lido.DefinirDiagnosticosCID([]dominio.DiagnosticoCID{cid})
	if err := lido.Fechar(medID, time.Now()); err != nil {
		t.Fatalf("fechar (domínio): %v", err)
	}
	if _, err := repoEp.Guardar(ctx, snapshotComID(lido, epID)); err != nil {
		t.Fatalf("guardar fecho: %v", err)
	}

	// Reler e confirmar fecho + CID persistido.
	final, err := repoEp.ObterPorID(ctx, epID)
	if err != nil || final.Estado() != dominio.EstadoEpisodioFechado {
		t.Fatalf("episódio não fechou: %v (estado=%v)", err, final.Estado())
	}
	if len(final.Snapshot().DiagnosticosCID) != 1 {
		t.Fatalf("diagnóstico não persistido: %d", len(final.Snapshot().DiagnosticosCID))
	}

	// Listar por doente.
	pag, err := repoEp.ListarPorDoente(ctx, dominio.FiltroEpisodios{DoenteID: doenteID, Limite: 10})
	if err != nil || pag.Total < 1 {
		t.Fatalf("listar falhou: %v (total=%d)", err, pag.Total)
	}
}

// snapshotComID reconstrói o episódio com o id atribuído pela BD.
func snapshotComID(e *dominio.EpisodioClinico, id string) *dominio.EpisodioClinico {
	s := e.Snapshot()
	s.ID = id
	return dominio.ReconstruirEpisodio(s)
}
