package clinico_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes das portas de integração (ADR-036) ---

type fakeLeitorRecepcao struct {
	out appclinico.ChegadaTriada
	err error
}

func (f fakeLeitorRecepcao) ChegadaTriada(_ context.Context, _ string) (appclinico.ChegadaTriada, error) {
	return f.out, f.err
}

// fakeConsumidorChegadas delega no fakeRepoEpisodios para o episódio ficar
// disponível na releitura final do caso de uso.
type fakeConsumidorChegadas struct {
	repo      *fakeRepoEpisodios
	err       error
	chegadaID string
	medicoID  string
	chamadas  int
}

func (f *fakeConsumidorChegadas) ConsumirEIniciar(ctx context.Context, chegadaID, medicoID string, e *clinico.EpisodioClinico) (string, error) {
	f.chamadas++
	if f.err != nil {
		return "", f.err
	}
	f.chegadaID, f.medicoID = chegadaID, medicoID
	return f.repo.Guardar(ctx, e)
}

func casoIniciarConsultaTeste(t *testing.T) (*appclinico.CasoIniciarConsulta, string, *fakeConsumidorChegadas, *fakeAuditor) {
	t.Helper()
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes) // doente ACTIVO
	repoEp := novoFakeRepoEpisodios()
	consumidor := &fakeConsumidorChegadas{repo: repoEp}
	leitor := fakeLeitorRecepcao{out: appclinico.ChegadaTriada{
		DoenteID: doenteID, MedicoID: "medico-1", EspecialidadeID: "esp-1",
	}}
	aud := &fakeAuditor{}
	return appclinico.NovoCasoIniciarConsulta(leitor, consumidor, repoDoentes, repoEp, aud), doenteID, consumidor, aud
}

func TestIniciarConsulta_FluxoFeliz(t *testing.T) {
	caso, doenteID, consumidor, aud := casoIniciarConsultaTeste(t)

	out, err := caso.Executar(context.Background(), "medico-1", "cheg-1")
	if err != nil {
		t.Fatalf("iniciar consulta: %v", err)
	}
	if out.ID == "" || out.Estado != "ABERTO" || out.Tipo != "CONSULTA" {
		t.Fatalf("episódio inesperado: %+v", out)
	}
	if out.DoenteID != doenteID || out.MedicoID != "medico-1" || out.EspecialidadeID != "esp-1" {
		t.Fatalf("dados da chegada mal propagados: %+v", out)
	}
	if consumidor.chegadaID != "cheg-1" || consumidor.medicoID != "medico-1" {
		t.Fatalf("consumidor mal invocado: %+v", consumidor)
	}
	if len(aud.registos) != 2 ||
		aud.registos[0].Accao != "clinico.episodio.aberto" ||
		aud.registos[1].Accao != "recepcao.chegada.consulta_iniciada" {
		t.Fatalf("auditoria inesperada: %+v", aud.registos)
	}
	if aud.registos[1].EntidadeID != "cheg-1" || aud.registos[1].Entidade != "chegada" {
		t.Fatalf("auditoria da chegada mal preenchida: %+v", aud.registos[1])
	}
}

func TestIniciarConsulta_ChegadaNaoEncontrada_Propaga(t *testing.T) {
	_, _, consumidor, _ := casoIniciarConsultaTeste(t)
	// substitui o leitor por um que devolve 404 — reconstruir o caso
	repoDoentes := novoFakeRepo()
	repoEp := novoFakeRepoEpisodios()
	caso := appclinico.NovoCasoIniciarConsulta(
		fakeLeitorRecepcao{err: erros.Novo(erros.CategoriaNaoEncontrado, "chegada triada não encontrada")},
		consumidor, repoDoentes, repoEp, &fakeAuditor{})

	if _, err := caso.Executar(context.Background(), "medico-1", "cheg-x"); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava CategoriaNaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

func TestIniciarConsulta_ActorNaoEOMedico_Proibido(t *testing.T) {
	caso, _, consumidor, aud := casoIniciarConsultaTeste(t)

	_, err := caso.Executar(context.Background(), "medico-2", "cheg-1")
	if erros.CategoriaDe(err) != erros.CategoriaProibido {
		t.Fatalf("esperava CategoriaProibido, veio %v", erros.CategoriaDe(err))
	}
	if consumidor.chamadas != 0 {
		t.Fatal("o consumidor não devia ser invocado quando o actor não é o médico")
	}
	if len(aud.registos) != 0 {
		t.Fatalf("não devia haver auditoria: %+v", aud.registos)
	}
}

func TestIniciarConsulta_DoenteInactivo_Conflito(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	d, err := repoDoentes.ObterPorID(context.Background(), doenteID)
	if err != nil {
		t.Fatalf("obter doente: %v", err)
	}
	if err := d.Desactivar("mudou de clínica", time.Now()); err != nil {
		t.Fatalf("desactivar doente: %v", err)
	}
	repoEp := novoFakeRepoEpisodios()
	consumidor := &fakeConsumidorChegadas{repo: repoEp}
	caso := appclinico.NovoCasoIniciarConsulta(
		fakeLeitorRecepcao{out: appclinico.ChegadaTriada{DoenteID: doenteID, MedicoID: "medico-1", EspecialidadeID: "esp-1"}},
		consumidor, repoDoentes, repoEp, &fakeAuditor{})

	if _, err := caso.Executar(context.Background(), "medico-1", "cheg-1"); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
	if consumidor.chamadas != 0 {
		t.Fatal("o consumidor não devia ser invocado com o doente inactivo")
	}
}

func TestIniciarConsulta_FalhaDoConsumidor_PropagaSemAuditar(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	falha := erros.Novo(erros.CategoriaConflito, "o estado da chegada mudou entretanto; recarregue e repita a operação")
	consumidor := &fakeConsumidorChegadas{repo: repoEp, err: falha}
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoIniciarConsulta(
		fakeLeitorRecepcao{out: appclinico.ChegadaTriada{DoenteID: doenteID, MedicoID: "medico-1", EspecialidadeID: "esp-1"}},
		consumidor, repoDoentes, repoEp, aud)

	_, err := caso.Executar(context.Background(), "medico-1", "cheg-1")
	if !errors.Is(err, falha) {
		t.Fatalf("esperava a falha do consumidor, veio %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("não devia haver auditoria após falha: %+v", aud.registos)
	}
}
