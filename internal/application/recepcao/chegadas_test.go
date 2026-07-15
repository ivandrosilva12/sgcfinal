// internal/application/recepcao/chegadas_test.go
package recepcao_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRegistarWalkIn_CriaEAudita(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	aud := &fakeAuditor{}
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	uc := app.NovoCasoRegistarWalkIn(chegadas, leitor, aud)

	out, err := uc.Executar(context.Background(), "adm-1", app.DadosWalkIn{DoenteID: "doe-1", EspecialidadeID: "esp-1"})
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.ID == "" || out.Estado != string(dominio.ChegAguarda) || out.MarcacaoID != "" {
		t.Fatalf("detalhe mal preenchido: %+v", out)
	}
	if !aud.tem("recepcao.chegada.walkin") {
		t.Fatal("esperava auditoria recepcao.chegada.walkin")
	}
}

func TestRegistarWalkIn_DoenteInactivo_RegraNegocio(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	leitor := fakeLeitorDoente{activos: map[string]bool{}}
	uc := app.NovoCasoRegistarWalkIn(chegadas, leitor, &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "adm-1", app.DadosWalkIn{DoenteID: "doe-1", EspecialidadeID: "esp-1"})
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio, veio %v", erros.CategoriaDe(err))
	}
}

func TestChamar_TransitaEAudita(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	id, _ := chegadas.Guardar(context.Background(), chegadaWalkIn(t, "doe-1", "esp-1", "09:00"))
	aud := &fakeAuditor{}
	uc := app.NovoCasoChamar(chegadas, aud)
	uc.DefinirRelogio(agoraFixo("09:10"))

	out, err := uc.Executar(context.Background(), "enf-1", id)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.Estado != string(dominio.ChegChamado) {
		t.Fatalf("esperava CHAMADO, veio %s", out.Estado)
	}
	if !aud.tem("recepcao.chegada.chamada") {
		t.Fatal("esperava auditoria recepcao.chegada.chamada")
	}
}

func TestRegistarDesistencia_TransitaEAudita(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	id, _ := chegadas.Guardar(context.Background(), chegadaWalkIn(t, "doe-1", "esp-1", "09:00"))
	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarDesistencia(chegadas, aud)
	uc.DefinirRelogio(agoraFixo("09:10"))

	out, err := uc.Executar(context.Background(), "adm-1", id)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.Estado != string(dominio.ChegDesistiu) {
		t.Fatalf("esperava DESISTIU, veio %s", out.Estado)
	}
	if !aud.tem("recepcao.chegada.desistiu") {
		t.Fatal("esperava auditoria recepcao.chegada.desistiu")
	}
}

func TestRegistarChegada_TransitaMarcacaoECriaChegada(t *testing.T) {
	marc := novoFakeMarcacoes()
	// marcação MARCADA persistida
	mid, _ := marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-1", "med-1", "esp-1", "09:00", "09:30"))
	chegadas := novoFakeChegadas(marc)
	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarChegada(chegadas, marc, aud)
	uc.DefinirRelogio(agoraFixo("08:50"))

	out, err := uc.Executar(context.Background(), "adm-1", mid)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.ID == "" || out.MarcacaoID != mid || out.MedicoID != "med-1" || out.Estado != string(dominio.ChegAguarda) {
		t.Fatalf("chegada mal preenchida: %+v", out)
	}
	// a marcação passou a COMPARECEU
	m, _ := marc.ObterPorID(context.Background(), mid)
	if m.Estado() != dominio.MarcCompareceu {
		t.Fatalf("a marcação devia estar COMPARECEU, veio %s", m.Estado())
	}
	if !aud.tem("recepcao.chegada.registada") {
		t.Fatal("esperava auditoria recepcao.chegada.registada")
	}
}

func TestRegistarChegada_MarcacaoInexistente_NaoEncontrado(t *testing.T) {
	marc := novoFakeMarcacoes()
	uc := app.NovoCasoRegistarChegada(novoFakeChegadas(marc), marc, &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "adm-1", "marc-inexistente")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava CategoriaNaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

func TestRegistarChegada_Duplicado_Conflito(t *testing.T) {
	marc := novoFakeMarcacoes()
	mid, _ := marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-1", "med-1", "esp-1", "09:00", "09:30"))
	chegadas := novoFakeChegadas(marc)
	uc := app.NovoCasoRegistarChegada(chegadas, marc, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("08:50"))
	if _, err := uc.Executar(context.Background(), "adm-1", mid); err != nil {
		t.Fatalf("primeiro check-in não devia falhar: %v", err)
	}
	// segundo check-in da mesma marcação: já não está MARCADA → Conflito
	_, err := uc.Executar(context.Background(), "adm-1", mid)
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("check-in duplo devia dar CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestListarFila_SoAguardaFiltradoPorEspecialidade(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	_, _ = chegadas.Guardar(context.Background(), chegadaWalkIn(t, "doe-1", "esp-1", "09:00"))
	_, _ = chegadas.Guardar(context.Background(), chegadaWalkIn(t, "doe-2", "esp-2", "09:05"))
	chamada := chegadaWalkIn(t, "doe-3", "esp-1", "09:10")
	_ = chamada.Chamar(inst("09:12"))
	_, _ = chegadas.Guardar(context.Background(), chamada) // CHAMADO não entra na fila

	uc := app.NovoCasoListarFila(chegadas)
	out, err := uc.Executar(context.Background(), "esp-1")
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if len(out) != 1 || out[0].DoenteID != "doe-1" {
		t.Fatalf("esperava só o doe-1 em AGUARDA de esp-1, veio %+v", out)
	}
}
