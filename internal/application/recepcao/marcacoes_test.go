// internal/application/recepcao/marcacoes_test.go
package recepcao_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func cenarioComJanela(t *testing.T) (*fakeJanelas, *fakeMarcacoes) {
	t.Helper()
	janelas := novoFakeJanelas()
	marc := novoFakeMarcacoes()
	_, _ = janelas.Guardar(context.Background(), janelaAgregada(t, "med-1", "esp-1", "08:00", "13:00"))
	return janelas, marc
}

func TestMarcar_DentroDaJanela_CriaEAudita(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	aud := &fakeAuditor{}
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	uc := app.NovoCasoMarcar(marc, janelas, leitor, aud)
	uc.DefinirRelogio(agoraFixo("07:00"))

	out, err := uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.ID == "" || out.Estado != string(dominio.MarcMarcada) {
		t.Fatalf("detalhe mal preenchido: %+v", out)
	}
	if !aud.tem("recepcao.marcacao.criada") {
		t.Fatal("esperava auditoria recepcao.marcacao.criada")
	}
}

func TestMarcar_DoenteInactivo_RegraNegocio(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{}} // doe-1 não activo
	uc := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))

	_, err := uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio (doente inactivo), veio %v", erros.CategoriaDe(err))
	}
}

func TestMarcar_ForaDaJanela_RegraNegocio(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	uc := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))

	_, err := uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("14:00"), Fim: inst("14:30"), // fora da janela 08-13
	})
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio (fora de janela), veio %v", erros.CategoriaDe(err))
	}
}

func TestMarcar_Sobreposicao_Conflito(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true, "doe-2": true}}
	uc := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))
	_, _ = uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})
	_, err := uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-2", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:15"), Fim: inst("09:45"),
	})
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestRemarcar_SupersedeEAudita(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	marcar := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	marcar.DefinirRelogio(agoraFixo("07:00"))
	criada, _ := marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})

	aud := &fakeAuditor{}
	uc := app.NovoCasoRemarcar(marc, janelas, aud)
	uc.DefinirRelogio(agoraFixo("07:00"))
	nova, err := uc.Executar(context.Background(), "adm-1", criada.ID, app.DadosRemarcar{
		Inicio: inst("10:00"), Fim: inst("10:30"),
	})
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if nova.RemarcaDe != criada.ID {
		t.Fatalf("a nova devia apontar para %s, veio %q", criada.ID, nova.RemarcaDe)
	}
	original, _ := marc.ObterPorID(context.Background(), criada.ID)
	if original.Estado() != dominio.MarcRemarcada {
		t.Fatalf("a original devia estar REMARCADA, veio %s", original.Estado())
	}
	if !aud.tem("recepcao.marcacao.remarcada") {
		t.Fatal("esperava auditoria recepcao.marcacao.remarcada")
	}
}

func TestCancelar_ComMotivo_Audita(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	marcar := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	marcar.DefinirRelogio(agoraFixo("07:00"))
	criada, _ := marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})

	aud := &fakeAuditor{}
	uc := app.NovoCasoCancelar(marc, aud)
	uc.DefinirRelogio(agoraFixo("07:00"))
	out, err := uc.Executar(context.Background(), "adm-1", criada.ID, "doente desistiu")
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.Estado != string(dominio.MarcCancelada) {
		t.Fatalf("esperava CANCELADA, veio %s", out.Estado)
	}
	if !aud.tem("recepcao.marcacao.cancelada") {
		t.Fatal("esperava auditoria recepcao.marcacao.cancelada")
	}
}

func TestRegistarFalta_AposHora_Audita(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	marcar := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	marcar.DefinirRelogio(agoraFixo("07:00"))
	criada, _ := marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})

	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarFalta(marc, aud)
	uc.DefinirRelogio(agoraFixo("10:00")) // depois do fim
	out, err := uc.Executar(context.Background(), "adm-1", criada.ID)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.Estado != string(dominio.MarcFaltou) {
		t.Fatalf("esperava FALTOU, veio %s", out.Estado)
	}
	if !aud.tem("recepcao.marcacao.faltou") {
		t.Fatal("esperava auditoria recepcao.marcacao.faltou")
	}
}

// --- Testes adicionais de cobertura: caminhos de erro dos quatro casos. ---

func TestMarcar_ErroNaACL_Propaga(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	falha := erros.Novo(erros.CategoriaInterno, "falha a consultar o doente")
	leitor := fakeLeitorDoente{erro: falha}
	uc := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))

	_, err := uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})
	if erros.CategoriaDe(err) != erros.CategoriaInterno {
		t.Fatalf("esperava o erro da ACL a propagar-se, veio %v", err)
	}
}

func TestMarcar_IntervaloInvalido_Validacao(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	uc := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))

	_, err := uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:30"), Fim: inst("09:00"), // fim antes do início
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestRemarcar_MarcacaoInexistente_NaoEncontrado(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	uc := app.NovoCasoRemarcar(marc, janelas, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))

	_, err := uc.Executar(context.Background(), "adm-1", "marc-inexistente", app.DadosRemarcar{
		Inicio: inst("10:00"), Fim: inst("10:30"),
	})
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava CategoriaNaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

func TestRemarcar_OriginalJaRemarcada_Conflito(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	marcar := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	marcar.DefinirRelogio(agoraFixo("07:00"))
	criada, _ := marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})

	uc := app.NovoCasoRemarcar(marc, janelas, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))
	if _, err := uc.Executar(context.Background(), "adm-1", criada.ID, app.DadosRemarcar{
		Inicio: inst("10:00"), Fim: inst("10:30"),
	}); err != nil {
		t.Fatalf("primeira remarcação não devia falhar: %v", err)
	}
	// A original já está REMARCADA — remarcar de novo tem de falhar.
	_, err := uc.Executar(context.Background(), "adm-1", criada.ID, app.DadosRemarcar{
		Inicio: inst("11:00"), Fim: inst("11:30"),
	})
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestRemarcar_ComOutraMarcacaoActivaNoNovoIntervalo_Conflito(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true, "doe-2": true}}
	marcar := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	marcar.DefinirRelogio(agoraFixo("07:00"))
	criada, _ := marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})
	_, _ = marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-2", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("10:00"), Fim: inst("10:30"),
	})

	uc := app.NovoCasoRemarcar(marc, janelas, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))
	// Remarcar a marcação de doe-1 para cima da de doe-2: a original é excluída da
	// verificação (semAMarcacao), mas a de doe-2 continua a contar.
	_, err := uc.Executar(context.Background(), "adm-1", criada.ID, app.DadosRemarcar{
		Inicio: inst("10:00"), Fim: inst("10:30"),
	})
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito (colisão com a marcação de doe-2), veio %v", erros.CategoriaDe(err))
	}
}

func TestCancelar_MarcacaoInexistente_NaoEncontrado(t *testing.T) {
	_, marc := cenarioComJanela(t)
	uc := app.NovoCasoCancelar(marc, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))

	_, err := uc.Executar(context.Background(), "adm-1", "marc-inexistente", "motivo qualquer")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava CategoriaNaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

func TestCancelar_SemMotivo_Validacao(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	marcar := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	marcar.DefinirRelogio(agoraFixo("07:00"))
	criada, _ := marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})

	uc := app.NovoCasoCancelar(marc, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))
	_, err := uc.Executar(context.Background(), "adm-1", criada.ID, "   ")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao (motivo em falta), veio %v", erros.CategoriaDe(err))
	}
}

func TestCancelar_JaCancelada_Conflito(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	marcar := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	marcar.DefinirRelogio(agoraFixo("07:00"))
	criada, _ := marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})

	uc := app.NovoCasoCancelar(marc, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))
	if _, err := uc.Executar(context.Background(), "adm-1", criada.ID, "primeira razão"); err != nil {
		t.Fatalf("primeiro cancelamento não devia falhar: %v", err)
	}
	_, err := uc.Executar(context.Background(), "adm-1", criada.ID, "segunda razão")
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito (já cancelada), veio %v", erros.CategoriaDe(err))
	}
}

func TestRegistarFalta_MarcacaoInexistente_NaoEncontrado(t *testing.T) {
	_, marc := cenarioComJanela(t)
	uc := app.NovoCasoRegistarFalta(marc, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("10:00"))

	_, err := uc.Executar(context.Background(), "adm-1", "marc-inexistente")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava CategoriaNaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

func TestRegistarFalta_AntesDaHora_RegraNegocio(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	marcar := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	marcar.DefinirRelogio(agoraFixo("07:00"))
	criada, _ := marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})

	uc := app.NovoCasoRegistarFalta(marc, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("09:15")) // ainda antes do fim (09:30)
	_, err := uc.Executar(context.Background(), "adm-1", criada.ID)
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio (antes da hora), veio %v", erros.CategoriaDe(err))
	}
}
