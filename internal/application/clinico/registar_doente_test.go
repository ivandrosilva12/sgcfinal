package clinico_test

import (
	"context"
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
