package clinico_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func dadosBase() appclinico.DadosNovoDoente {
	return appclinico.DadosNovoDoente{
		Identificacao: appclinico.DadosIdentificacao{
			NomeCompleto:   "Ana Domingos",
			DataNascimento: time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC),
			Sexo:           "F",
			BI:             ptrS("00123456LA042"),
		},
		Contactos: appclinico.DadosContactos{Telefone: "+244923456789"},
	}
}

func ptrS(s string) *string { return &s }

func TestRegistarDoente_NumeroAutomatico(t *testing.T) {
	repo := novoFakeRepo()
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoRegistarDoente(repo, aud)

	out, err := caso.Executar(context.Background(), "actor-1", dadosBase())
	if err != nil {
		t.Fatalf("registar: %v", err)
	}
	if out.NumProcesso == "" || out.ID == "" {
		t.Fatalf("saída incompleta: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.registado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
	if aud.registos[0].Actor != "actor-1" || aud.registos[0].EntidadeID != out.ID {
		t.Fatalf("auditoria com dados errados: %+v", aud.registos[0])
	}
}

func TestRegistarDoente_NumeroManual(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoRegistarDoente(repo, &fakeAuditor{})
	dados := dadosBase()
	dados.NumProcesso = "PROC-LEGADO-42"

	out, err := caso.Executar(context.Background(), "actor-1", dados)
	if err != nil {
		t.Fatalf("registar: %v", err)
	}
	if out.NumProcesso != "PROC-LEGADO-42" {
		t.Fatalf("num de processo manual não respeitado: %q", out.NumProcesso)
	}
}

func TestRegistarDoente_IdentificacaoInvalida(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoRegistarDoente(repo, &fakeAuditor{})
	dados := dadosBase()
	dados.Identificacao.BI = nil
	dados.Identificacao.Passaporte = nil // sem BI nem passaporte

	_, err := caso.Executar(context.Background(), "actor-1", dados)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestRegistarDoente_ContactosInvalidos(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoRegistarDoente(repo, &fakeAuditor{})
	dados := dadosBase()
	dados.Contactos.Telefone = "123"

	_, err := caso.Executar(context.Background(), "actor-1", dados)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("telefone inválido devia falhar com Validacao, obtive %v", err)
	}
}

func TestRegistarDoente_GrupoSanguineoInvalido(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoRegistarDoente(repo, &fakeAuditor{})
	dados := dadosBase()
	g := "ZZ"
	dados.GrupoSanguineo = &g

	_, err := caso.Executar(context.Background(), "actor-1", dados)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("grupo sanguíneo inválido devia falhar com Validacao, obtive %v", err)
	}
}

func TestRegistarDoente_ProximoNumeroProcessoFalha(t *testing.T) {
	repo := novoFakeRepo()
	repo.proxErr = errSimulado
	caso := appclinico.NovoCasoRegistarDoente(repo, &fakeAuditor{})

	_, err := caso.Executar(context.Background(), "actor-1", dadosBase()) // NumProcesso vazio → gera automático
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de ProximoNumeroProcesso, obtive %v", err)
	}
}

func TestRegistarDoente_GuardarFalha(t *testing.T) {
	repo := novoFakeRepo()
	repo.guardarErr = errSimulado
	caso := appclinico.NovoCasoRegistarDoente(repo, &fakeAuditor{})

	_, err := caso.Executar(context.Background(), "actor-1", dadosBase())
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestRegistarDoente_AuditorFalha(t *testing.T) {
	repo := novoFakeRepo()
	aud := &fakeAuditor{err: errSimulado}
	caso := appclinico.NovoCasoRegistarDoente(repo, aud)

	_, err := caso.Executar(context.Background(), "actor-1", dadosBase())
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestRegistarDoente_ReleituraFinalFalha(t *testing.T) {
	repo := novoFakeRepo()
	repo.obterErr = errSimulado
	repo.obterErrNaChamada = 1 // o registo ainda não fez nenhuma leitura antes: a única ObterPorID é a releitura final.
	caso := appclinico.NovoCasoRegistarDoente(repo, &fakeAuditor{})

	_, err := caso.Executar(context.Background(), "actor-1", dadosBase())
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}
