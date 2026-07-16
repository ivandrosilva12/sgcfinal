package clinico_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func timeFixa() time.Time { return time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC) }

func dadosEpisodioBase(doenteID string) appclinico.DadosNovoEpisodio {
	return appclinico.DadosNovoEpisodio{
		DoenteID: doenteID, Tipo: "CONSULTA", EspecialidadeID: "esp-1", MedicoID: "medico-1",
	}
}

func TestIniciarEpisodio_DoenteActivo(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes) // cria um doente ACTIVO
	repoEp := novoFakeRepoEpisodios()
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoIniciarEpisodio(repoEp, repoDoentes, aud)

	out, err := caso.Executar(context.Background(), "medico-1", dadosEpisodioBase(doenteID))
	if err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if out.ID == "" || out.Estado != "ABERTO" {
		t.Fatalf("saída inesperada: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.episodio.aberto" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestIniciarEpisodio_DoenteNaoEncontrado(t *testing.T) {
	caso := appclinico.NovoCasoIniciarEpisodio(novoFakeRepoEpisodios(), novoFakeRepo(), &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", dadosEpisodioBase("inexistente"))
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestIniciarEpisodio_DoenteNaoActivo(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	// Desactiva o doente directamente no fake.
	d, _ := repoDoentes.ObterPorID(context.Background(), doenteID)
	_ = d.Desactivar("teste", timeFixa())
	_, _ = repoDoentes.Guardar(context.Background(), d)

	caso := appclinico.NovoCasoIniciarEpisodio(novoFakeRepoEpisodios(), repoDoentes, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", dadosEpisodioBase(doenteID))
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito (doente não activo), obtive %v", err)
	}
}

func TestIniciarEpisodio_TipoInvalido(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	caso := appclinico.NovoCasoIniciarEpisodio(novoFakeRepoEpisodios(), repoDoentes, &fakeAuditor{})
	dados := dadosEpisodioBase(doenteID)
	dados.Tipo = "DESCONHECIDO"
	_, err := caso.Executar(context.Background(), "medico-1", dados)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("tipo inválido devia falhar com Validacao, obtive %v", err)
	}
}

func TestIniciarEpisodio_GuardarFalha(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	repoEp.guardarErr = errSimulado
	caso := appclinico.NovoCasoIniciarEpisodio(repoEp, repoDoentes, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", dadosEpisodioBase(doenteID))
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestIniciarEpisodio_AuditorFalha(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	aud := &fakeAuditor{err: errSimulado}
	caso := appclinico.NovoCasoIniciarEpisodio(novoFakeRepoEpisodios(), repoDoentes, aud)
	_, err := caso.Executar(context.Background(), "medico-1", dadosEpisodioBase(doenteID))
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestIniciarEpisodio_ReleituraFinalFalha(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	repoEp.obterErr = errSimulado
	repoEp.obterErrNaChamada = 1 // fresco: a única ObterPorID de episódios é a releitura final.
	caso := appclinico.NovoCasoIniciarEpisodio(repoEp, repoDoentes, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", dadosEpisodioBase(doenteID))
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}
