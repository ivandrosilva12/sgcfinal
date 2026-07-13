package clinico_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRegistarConsentimento_CirurgiaSemAnexo(t *testing.T) {
	repoC := novoFakeConsentimentos()
	repoD := novoFakeRepo()
	repoD.porID["doente-1"] = novoDoenteValido(t) // helper existente nos testes de doente
	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarConsentimento(repoC, repoD, aud)

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosNovoConsentimento{
		DoenteID: "doente-1", Finalidade: "CIRURGIA", Concedido: true, DocumentoURL: "",
	})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("cirurgia sem anexo devia falhar com RegraNegocio, veio %v", err)
	}
}

func TestRegistarConsentimento_Sucesso(t *testing.T) {
	repoC := novoFakeConsentimentos()
	repoD := novoFakeRepo()
	repoD.porID["doente-1"] = novoDoenteValido(t)
	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarConsentimento(repoC, repoD, aud)

	out, err := uc.Executar(context.Background(), "actor-1", app.DadosNovoConsentimento{
		DoenteID: "doente-1", Finalidade: "TRATAMENTO", Concedido: true,
	})
	if err != nil {
		t.Fatalf("registo devia funcionar: %v", err)
	}
	if out.ID == "" || !out.Vigente {
		t.Fatalf("esperado consentimento vigente com id, veio %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.consentimento.registado" {
		t.Fatalf("esperada auditoria clinico.consentimento.registado, veio %v", aud.registos)
	}
}

func TestRevogarConsentimento(t *testing.T) {
	repoC := novoFakeConsentimentos()
	c, _ := clinico.NovoConsentimento("doente-1", clinico.FinalidadeTratamento, true, "", nowUTC())
	id, _ := repoC.Guardar(context.Background(), c)
	aud := &fakeAuditor{}
	uc := app.NovoCasoRevogarConsentimento(repoC, aud)

	out, err := uc.Executar(context.Background(), "actor-1", id)
	if err != nil {
		t.Fatalf("revogar devia funcionar: %v", err)
	}
	if out.Vigente {
		t.Fatalf("consentimento revogado não devia estar vigente")
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.consentimento.revogado" {
		t.Fatalf("esperada auditoria clinico.consentimento.revogado, veio %v", aud.registos)
	}
}

func TestListarConsentimentos_NaoAudita(t *testing.T) {
	repoC := novoFakeConsentimentos()
	repoC.lista = []clinico.ResumoConsentimento{{ID: "cons-1", DoenteID: "doente-1"}}
	uc := app.NovoCasoListarConsentimentos(repoC)

	out, err := uc.Executar(context.Background(), "doente-1", app.FiltroConsentimentos{})
	if err != nil {
		t.Fatalf("listar devia funcionar: %v", err)
	}
	if len(out) != 1 || out[0].ID != "cons-1" {
		t.Fatalf("esperada lista com o resumo preparado, veio %+v", out)
	}
}

func TestObterConsentimento_Sucesso(t *testing.T) {
	repoC := novoFakeConsentimentos()
	c, _ := clinico.NovoConsentimento("doente-1", clinico.FinalidadeTratamento, true, "", nowUTC())
	id, _ := repoC.Guardar(context.Background(), c)
	uc := app.NovoCasoObterConsentimento(repoC)

	out, err := uc.Executar(context.Background(), id)
	if err != nil {
		t.Fatalf("obter devia funcionar: %v", err)
	}
	if out.ID != id || !out.Vigente {
		t.Fatalf("esperado detalhe vigente com o id pedido, veio %+v", out)
	}
}

func TestObterConsentimento_NaoEncontrado(t *testing.T) {
	uc := app.NovoCasoObterConsentimento(novoFakeConsentimentos())

	_, err := uc.Executar(context.Background(), "inexistente")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}
