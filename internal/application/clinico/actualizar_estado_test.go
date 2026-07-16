package clinico_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestActualizarDoente_Contactos(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoActualizarDoente(repo, aud)

	novoTel := "+244912000000"
	out, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{
		Contactos: &appclinico.DadosContactos{Telefone: novoTel},
	})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.Telefone != "+244 912 000 000" {
		t.Fatalf("telefone não actualizado: %q", out.Telefone)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.actualizado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestActualizarDoente_GrupoSanguineo(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoActualizarDoente(repo, &fakeAuditor{})
	g := "O+"
	out, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{GrupoSanguineo: &g})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.GrupoSanguineo == nil || *out.GrupoSanguineo != "O+" {
		t.Fatalf("grupo sanguíneo não definido: %v", out.GrupoSanguineo)
	}
}

func TestActualizarDoente_NaoEncontrado(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoActualizarDoente(repo, &fakeAuditor{})
	novoTel := "+244912000000"
	_, err := caso.Executar(context.Background(), "actor-1", "inexistente", appclinico.DadosActualizarDoente{
		Contactos: &appclinico.DadosContactos{Telefone: novoTel},
	})
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestActualizarDoente_IdentificacaoInvalida(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoActualizarDoente(repo, &fakeAuditor{})
	ident := &appclinico.DadosIdentificacao{
		NomeCompleto: "Ana Domingos", DataNascimento: time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC),
		Sexo: "INVALIDO", BI: ptrS("00123456LA042"),
	}
	_, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{Identificacao: ident})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("sexo inválido devia falhar com Validacao, obtive %v", err)
	}
}

func TestActualizarDoente_ContactosInvalidos(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoActualizarDoente(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{
		Contactos: &appclinico.DadosContactos{Telefone: "123"},
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("telefone inválido devia falhar com Validacao, obtive %v", err)
	}
}

func TestActualizarDoente_GrupoSanguineoInvalido(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoActualizarDoente(repo, &fakeAuditor{})
	g := "ZZ"
	_, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{GrupoSanguineo: &g})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("grupo sanguíneo inválido devia falhar com Validacao, obtive %v", err)
	}
}

func TestActualizarDoente_GuardarFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	repo.guardarErr = errSimulado
	caso := appclinico.NovoCasoActualizarDoente(repo, &fakeAuditor{})
	g := "O+"
	_, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{GrupoSanguineo: &g})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestActualizarDoente_AuditorFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{err: errSimulado}
	caso := appclinico.NovoCasoActualizarDoente(repo, aud)
	g := "O+"
	_, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{GrupoSanguineo: &g})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestActualizarDoente_ReleituraFinalFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	repo.obterChamadas = 0 // reinicia a contagem: registarNoRepo já fez uma leitura.
	repo.obterErr = errSimulado
	repo.obterErrNaChamada = 2 // 1ª chamada (leitura inicial) passa; a releitura final falha.
	caso := appclinico.NovoCasoActualizarDoente(repo, &fakeAuditor{})
	g := "O+"
	_, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{GrupoSanguineo: &g})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}

func TestGerirEstado_Desactivar(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, aud)

	out, err := caso.Desactivar(context.Background(), "actor-1", id, "dados duplicados")
	if err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	if out.Estado != "INACTIVO" {
		t.Fatalf("estado=%q, esperava INACTIVO", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.desactivado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestGerirEstado_DesactivarSemMotivo(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, &fakeAuditor{})
	_, err := caso.Desactivar(context.Background(), "actor-1", id, "  ")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestGerirEstado_DeclararFalecido(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, aud)

	data := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	out, err := caso.DeclararFalecido(context.Background(), "actor-1", id, data, "I21")
	if err != nil {
		t.Fatalf("declarar falecido: %v", err)
	}
	if out.Estado != "FALECIDO" {
		t.Fatalf("estado=%q, esperava FALECIDO", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.falecido" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestGerirEstado_DoenteNaoEncontrado(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, &fakeAuditor{})
	_, err := caso.Desactivar(context.Background(), "actor-1", "inexistente", "motivo")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestGerirEstado_DeclararFalecidoDataFutura(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, &fakeAuditor{})
	_, err := caso.DeclararFalecido(context.Background(), "actor-1", id, time.Now().Add(24*time.Hour), "I21")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("data de óbito futura devia falhar com Validacao, obtive %v", err)
	}
}

func TestGerirEstado_GuardarFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	repo.guardarErr = errSimulado
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, &fakeAuditor{})
	_, err := caso.Desactivar(context.Background(), "actor-1", id, "motivo")
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestGerirEstado_AuditorFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{err: errSimulado}
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, aud)
	_, err := caso.Desactivar(context.Background(), "actor-1", id, "motivo")
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestGerirEstado_ReleituraFinalFalha(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	repo.obterChamadas = 0 // reinicia a contagem: registarNoRepo já fez uma leitura.
	repo.obterErr = errSimulado
	repo.obterErrNaChamada = 2 // 1ª chamada (leitura inicial) passa; a releitura final falha.
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, &fakeAuditor{})
	_, err := caso.Desactivar(context.Background(), "actor-1", id, "motivo")
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}
