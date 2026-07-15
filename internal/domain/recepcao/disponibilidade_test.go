// internal/domain/recepcao/disponibilidade_test.go
package recepcao_test

import (
	"testing"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func janela(esp, de, ate string) recepcao.JanelaDisponibilidade {
	j, _ := recepcao.NovaJanela("med-1", esp, inst(de), inst(ate))
	return *j
}

func marcada(de, ate string) recepcao.Marcacao {
	m, _ := recepcao.NovaMarcacao("doe-x", "med-1", "esp-1", inst(de), inst(ate))
	return *m
}

func TestVerificar_CabeNaJanela_SemMarcacoes(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-1", "08:00", "13:00")}
	err := recepcao.VerificarDisponibilidade(janelas, nil, "esp-1", inst("09:00"), inst("09:30"), inst("07:00"))
	if err != nil {
		t.Fatalf("devia aceitar: %v", err)
	}
}

func TestVerificar_ForaDeQualquerJanela_RegraNegocio(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-1", "08:00", "10:00")}
	err := recepcao.VerificarDisponibilidade(janelas, nil, "esp-1", inst("11:00"), inst("11:30"), inst("07:00"))
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio (fora de janela), veio %v", erros.CategoriaDe(err))
	}
}

func TestVerificar_JanelaDeOutraEspecialidade_NaoConta(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-2", "08:00", "13:00")}
	err := recepcao.VerificarDisponibilidade(janelas, nil, "esp-1", inst("09:00"), inst("09:30"), inst("07:00"))
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("janela de outra especialidade não devia servir; veio %v", erros.CategoriaDe(err))
	}
}

func TestVerificar_NoPassado_RegraNegocio(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-1", "08:00", "13:00")}
	// agora depois do início proposto
	err := recepcao.VerificarDisponibilidade(janelas, nil, "esp-1", inst("09:00"), inst("09:30"), inst("09:15"))
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio (passado), veio %v", erros.CategoriaDe(err))
	}
}

func TestVerificar_Sobreposicao_Conflito(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-1", "08:00", "13:00")}
	activas := []recepcao.Marcacao{marcada("09:00", "09:30")}
	// proposta 09:15-09:45 sobrepõe
	err := recepcao.VerificarDisponibilidade(janelas, activas, "esp-1", inst("09:15"), inst("09:45"), inst("07:00"))
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito (sobreposição), veio %v", erros.CategoriaDe(err))
	}
}

func TestVerificar_EncostoExacto_NaoESobreposicao(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-1", "08:00", "13:00")}
	activas := []recepcao.Marcacao{marcada("09:00", "09:30")}
	// proposta 09:30-10:00 encosta exactamente ao fim da anterior — permitido
	err := recepcao.VerificarDisponibilidade(janelas, activas, "esp-1", inst("09:30"), inst("10:00"), inst("07:00"))
	if err != nil {
		t.Fatalf("encosto exacto não devia ser conflito: %v", err)
	}
}
