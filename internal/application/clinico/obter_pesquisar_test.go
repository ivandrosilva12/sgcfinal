package clinico_test

import (
	"context"
	"testing"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func registarNoRepo(t *testing.T, repo *fakeRepo) string {
	t.Helper()
	caso := appclinico.NovoCasoRegistarDoente(repo, &fakeAuditor{})
	out, err := caso.Executar(context.Background(), "sys", dadosBase())
	if err != nil {
		t.Fatalf("preparar doente: %v", err)
	}
	return out.ID
}

func TestObterDoente_Audita(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoObterDoente(repo, aud)

	out, err := caso.Executar(context.Background(), "actor-1", id)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if out.ID != id {
		t.Fatalf("id inesperado: %q", out.ID)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.consultado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestObterDoente_NaoEncontrado(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoObterDoente(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "actor-1", "inexistente")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestPesquisarDoentes_AplicaLimiteDefault(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoPesquisarDoentes(repo)
	if _, err := caso.Executar(context.Background(), appclinico.FiltroDoentes{Termo: "ana"}); err != nil {
		t.Fatalf("pesquisar: %v", err)
	}
	if repo.ultimoFilt.Limite != 20 {
		t.Fatalf("limite default=%d, esperava 20", repo.ultimoFilt.Limite)
	}
}

func TestPesquisarDoentes_LimiteMaximo(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoPesquisarDoentes(repo)
	if _, err := caso.Executar(context.Background(), appclinico.FiltroDoentes{Limite: 5000}); err != nil {
		t.Fatalf("pesquisar: %v", err)
	}
	if repo.ultimoFilt.Limite != 100 {
		t.Fatalf("limite máximo=%d, esperava 100", repo.ultimoFilt.Limite)
	}
}
