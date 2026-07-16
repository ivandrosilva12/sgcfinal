package clinico_test

import (
	"context"
	"errors"
	"testing"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestObterEpisodio_Audita(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	aud := &fakeAuditor{}
	out, err := appclinico.NovoCasoObterEpisodio(repoEp, aud).Executar(context.Background(), "medico-1", id)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if out.ID != id {
		t.Fatalf("id inesperado: %q", out.ID)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.episodio.consultado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestObterEpisodio_NaoEncontrado(t *testing.T) {
	_, err := appclinico.NovoCasoObterEpisodio(novoFakeRepoEpisodios(), &fakeAuditor{}).Executar(context.Background(), "m", "inexistente")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestListarEpisodios_AplicaDoenteELimite(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	caso := appclinico.NovoCasoListarEpisodios(repoEp)
	if _, err := caso.Executar(context.Background(), "doente-9", appclinico.FiltroEpisodios{}); err != nil {
		t.Fatalf("listar: %v", err)
	}
	if repoEp.ultimoFilt.DoenteID != "doente-9" || repoEp.ultimoFilt.Limite != 20 {
		t.Fatalf("filtro inesperado: %+v", repoEp.ultimoFilt)
	}
	if _, err := caso.Executar(context.Background(), "doente-9", appclinico.FiltroEpisodios{Limite: 5000}); err != nil {
		t.Fatalf("listar: %v", err)
	}
	if repoEp.ultimoFilt.Limite != 100 {
		t.Fatalf("limite máximo=%d, esperava 100", repoEp.ultimoFilt.Limite)
	}
}

func TestObterEHR_CombinaDoenteEEpisodios(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	repoEp.pagina = clinico.PaginaEpisodios{Total: 1, Itens: []clinico.ResumoEpisodio{{ID: "ep-1", Estado: "ABERTO"}}}
	aud := &fakeAuditor{}

	ehr, err := appclinico.NovoCasoObterEHR(repoDoentes, repoEp, aud).Executar(context.Background(), "medico-1", doenteID, appclinico.FiltroEpisodios{})
	if err != nil {
		t.Fatalf("ehr: %v", err)
	}
	if ehr.Doente.ID != doenteID || ehr.Episodios.Total != 1 {
		t.Fatalf("EHR inesperado: %+v", ehr)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.ehr.consultado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestObterEHR_DoenteNaoEncontrado(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	_, err := appclinico.NovoCasoObterEHR(novoFakeRepo(), repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", "inexistente", appclinico.FiltroEpisodios{})
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestObterEHR_ListarEpisodiosFalha(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	repoEp.listarErr = errSimulado
	_, err := appclinico.NovoCasoObterEHR(repoDoentes, repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", doenteID, appclinico.FiltroEpisodios{})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de ListarPorDoente, obtive %v", err)
	}
}

func TestObterEHR_AuditorFalha(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	aud := &fakeAuditor{err: errSimulado}
	_, err := appclinico.NovoCasoObterEHR(repoDoentes, repoEp, aud).Executar(context.Background(), "medico-1", doenteID, appclinico.FiltroEpisodios{})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

// TestObterEHR_NormalizaLimiteEDeslocamento prova que o EHR aplica as mesmas
// regras de paginação defensiva que ListarEpisodios: limite <=0 cai no default,
// limite acima do máximo é cortado, e deslocamento negativo é normalizado a 0
// (nunca propagado tal e qual ao repositório).
func TestObterEHR_NormalizaLimiteEDeslocamento(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	caso := appclinico.NovoCasoObterEHR(repoDoentes, repoEp, &fakeAuditor{})

	if _, err := caso.Executar(context.Background(), "medico-1", doenteID, appclinico.FiltroEpisodios{Deslocamento: -5}); err != nil {
		t.Fatalf("ehr: %v", err)
	}
	if repoEp.ultimoFilt.Limite != 20 || repoEp.ultimoFilt.Deslocamento != 0 {
		t.Fatalf("filtro inesperado: %+v", repoEp.ultimoFilt)
	}

	if _, err := caso.Executar(context.Background(), "medico-1", doenteID, appclinico.FiltroEpisodios{Limite: 5000}); err != nil {
		t.Fatalf("ehr: %v", err)
	}
	if repoEp.ultimoFilt.Limite != 100 {
		t.Fatalf("limite máximo=%d, esperava 100", repoEp.ultimoFilt.Limite)
	}
}
