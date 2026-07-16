package clinico_test

import (
	"context"
	"errors"
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

func TestRegistarConsentimento_DoenteNaoEncontrado(t *testing.T) {
	repoC := novoFakeConsentimentos()
	repoD := novoFakeRepo()
	uc := app.NovoCasoRegistarConsentimento(repoC, repoD, &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosNovoConsentimento{
		DoenteID: "inexistente", Finalidade: "TRATAMENTO", Concedido: true,
	})
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestRegistarConsentimento_FinalidadeInvalida(t *testing.T) {
	repoC := novoFakeConsentimentos()
	repoD := novoFakeRepo()
	repoD.porID["doente-1"] = novoDoenteValido(t)
	uc := app.NovoCasoRegistarConsentimento(repoC, repoD, &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosNovoConsentimento{
		DoenteID: "doente-1", Finalidade: "DESCONHECIDA", Concedido: true,
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("finalidade inválida devia falhar com Validacao, obtive %v", err)
	}
}

func TestRegistarConsentimento_GuardarFalha(t *testing.T) {
	repoC := novoFakeConsentimentos()
	repoD := novoFakeRepo()
	repoD.porID["doente-1"] = novoDoenteValido(t)
	repoC.guardarErr = errSimulado
	uc := app.NovoCasoRegistarConsentimento(repoC, repoD, &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosNovoConsentimento{
		DoenteID: "doente-1", Finalidade: "TRATAMENTO", Concedido: true,
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestRegistarConsentimento_AuditorFalha(t *testing.T) {
	repoC := novoFakeConsentimentos()
	repoD := novoFakeRepo()
	repoD.porID["doente-1"] = novoDoenteValido(t)
	aud := &fakeAuditor{err: errSimulado}
	uc := app.NovoCasoRegistarConsentimento(repoC, repoD, aud)

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosNovoConsentimento{
		DoenteID: "doente-1", Finalidade: "TRATAMENTO", Concedido: true,
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestRegistarConsentimento_ReleituraFinalFalha(t *testing.T) {
	repoC := novoFakeConsentimentos()
	repoD := novoFakeRepo()
	repoD.porID["doente-1"] = novoDoenteValido(t)
	repoC.obterErr = errSimulado
	repoC.obterErrNaChamada = 1 // o registo não lê consentimentos antes de guardar: a única ObterPorID é a releitura final.
	uc := app.NovoCasoRegistarConsentimento(repoC, repoD, &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosNovoConsentimento{
		DoenteID: "doente-1", Finalidade: "TRATAMENTO", Concedido: true,
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
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

func TestRevogarConsentimento_NaoEncontrado(t *testing.T) {
	uc := app.NovoCasoRevogarConsentimento(novoFakeConsentimentos(), &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "actor-1", "inexistente")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestRevogarConsentimento_JaRevogado(t *testing.T) {
	repoC := novoFakeConsentimentos()
	c, _ := clinico.NovoConsentimento("doente-1", clinico.FinalidadeTratamento, true, "", nowUTC())
	id, _ := repoC.Guardar(context.Background(), c)
	aud := &fakeAuditor{}
	uc := app.NovoCasoRevogarConsentimento(repoC, aud)
	if _, err := uc.Executar(context.Background(), "actor-1", id); err != nil {
		t.Fatalf("revogar devia funcionar: %v", err)
	}
	_, err := uc.Executar(context.Background(), "actor-1", id)
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("revogar um consentimento já revogado devia falhar com Conflito, obtive %v", err)
	}
}

func TestRevogarConsentimento_GuardarFalha(t *testing.T) {
	repoC := novoFakeConsentimentos()
	c, _ := clinico.NovoConsentimento("doente-1", clinico.FinalidadeTratamento, true, "", nowUTC())
	id, _ := repoC.Guardar(context.Background(), c)
	repoC.guardarErr = errSimulado
	uc := app.NovoCasoRevogarConsentimento(repoC, &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "actor-1", id)
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestRevogarConsentimento_AuditorFalha(t *testing.T) {
	repoC := novoFakeConsentimentos()
	c, _ := clinico.NovoConsentimento("doente-1", clinico.FinalidadeTratamento, true, "", nowUTC())
	id, _ := repoC.Guardar(context.Background(), c)
	aud := &fakeAuditor{err: errSimulado}
	uc := app.NovoCasoRevogarConsentimento(repoC, aud)
	_, err := uc.Executar(context.Background(), "actor-1", id)
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestRevogarConsentimento_ReleituraFinalFalha(t *testing.T) {
	repoC := novoFakeConsentimentos()
	c, _ := clinico.NovoConsentimento("doente-1", clinico.FinalidadeTratamento, true, "", nowUTC())
	id, _ := repoC.Guardar(context.Background(), c)
	repoC.obterErr = errSimulado
	repoC.obterErrNaChamada = 2
	uc := app.NovoCasoRevogarConsentimento(repoC, &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "actor-1", id)
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}

// TestListarConsentimentos_DevolveOsResumosDoRepositorio prova o pass-through da
// listagem: o caso de uso devolve, sem os alterar, os resumos que o repositório
// produz. (Não afirma nada sobre auditoria — `CasoListarConsentimentos` nem sequer
// recebe um Auditor, logo auditar seria estruturalmente impossível.)
func TestListarConsentimentos_DevolveOsResumosDoRepositorio(t *testing.T) {
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
