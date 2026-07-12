package clinico_test

import (
	"context"
	"testing"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRegistarAlergia(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoRegistarAlergia(repo, aud)

	out, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAlergia{
		Substancia: "Penicilina", Severidade: "GRAVE",
	})
	if err != nil {
		t.Fatalf("registar alergia: %v", err)
	}
	if len(out.Alergias) != 1 || out.Alergias[0].Substancia != "Penicilina" {
		t.Fatalf("alergia não registada: %+v", out.Alergias)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.alergia.registada" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestRegistarAlergia_SeveridadeInvalida(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoRegistarAlergia(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAlergia{
		Substancia: "X", Severidade: "EXTREMA",
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestRegistarAntecedente(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoRegistarAntecedente(repo, aud)

	out, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAntecedente{
		Tipo: "PESSOAL", Descricao: "Hipertensão", CID: "I10", Activo: true,
	})
	if err != nil {
		t.Fatalf("registar antecedente: %v", err)
	}
	if len(out.Antecedentes) != 1 || out.Antecedentes[0].Descricao != "Hipertensão" {
		t.Fatalf("antecedente não registado: %+v", out.Antecedentes)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.antecedente.registado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}
