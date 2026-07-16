package clinico_test

import (
	"context"
	"errors"
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

func TestRegistarAlergia_DoenteNaoEncontrado(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoRegistarAlergia(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", "inexistente", appclinico.DadosAlergia{
		Substancia: "Penicilina", Severidade: "GRAVE",
	})
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestRegistarAlergia_SubstanciaVazia(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoRegistarAlergia(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAlergia{
		Substancia: "   ", Severidade: "GRAVE",
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("substância vazia devia falhar com Validacao, obtive %v", err)
	}
}

func TestRegistarAlergia_GuardarFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	repo.guardarErr = errSimulado
	caso := appclinico.NovoCasoRegistarAlergia(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAlergia{
		Substancia: "Penicilina", Severidade: "GRAVE",
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestRegistarAlergia_AuditorFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{err: errSimulado}
	caso := appclinico.NovoCasoRegistarAlergia(repo, aud)
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAlergia{
		Substancia: "Penicilina", Severidade: "GRAVE",
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestRegistarAlergia_ReleituraFinalFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	repo.obterChamadas = 0 // reinicia a contagem: registarNoRepo já fez uma leitura.
	repo.obterErr = errSimulado
	repo.obterErrNaChamada = 2
	caso := appclinico.NovoCasoRegistarAlergia(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAlergia{
		Substancia: "Penicilina", Severidade: "GRAVE",
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
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

func TestRegistarAntecedente_DoenteNaoEncontrado(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoRegistarAntecedente(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", "inexistente", appclinico.DadosAntecedente{
		Tipo: "PESSOAL", Descricao: "Hipertensão",
	})
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestRegistarAntecedente_TipoInvalido(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoRegistarAntecedente(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAntecedente{
		Tipo: "DESCONHECIDO", Descricao: "Hipertensão",
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("tipo inválido devia falhar com Validacao, obtive %v", err)
	}
}

func TestRegistarAntecedente_DescricaoVazia(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoRegistarAntecedente(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAntecedente{
		Tipo: "PESSOAL", Descricao: "   ",
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("descrição vazia devia falhar com Validacao, obtive %v", err)
	}
}

func TestRegistarAntecedente_GuardarFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	repo.guardarErr = errSimulado
	caso := appclinico.NovoCasoRegistarAntecedente(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAntecedente{
		Tipo: "PESSOAL", Descricao: "Hipertensão",
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestRegistarAntecedente_AuditorFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{err: errSimulado}
	caso := appclinico.NovoCasoRegistarAntecedente(repo, aud)
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAntecedente{
		Tipo: "PESSOAL", Descricao: "Hipertensão",
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestRegistarAntecedente_ReleituraFinalFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	repo.obterChamadas = 0 // reinicia a contagem: registarNoRepo já fez uma leitura.
	repo.obterErr = errSimulado
	repo.obterErrNaChamada = 2
	caso := appclinico.NovoCasoRegistarAntecedente(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAntecedente{
		Tipo: "PESSOAL", Descricao: "Hipertensão",
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}
