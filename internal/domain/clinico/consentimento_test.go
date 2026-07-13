package clinico_test

import (
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovoConsentimento_Cirurgia_ExigeAnexoEConcedido(t *testing.T) {
	quando := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	// Sem anexo → RegraNegocio.
	if _, err := dominio.NovoConsentimento("doente-1", dominio.FinalidadeCirurgia, true, "", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("cirurgia sem anexo devia falhar com RegraNegocio, veio %v", err)
	}
	// Não concedido → RegraNegocio.
	if _, err := dominio.NovoConsentimento("doente-1", dominio.FinalidadeCirurgia, false, "s3://doc.pdf", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("cirurgia não concedida devia falhar com RegraNegocio, veio %v", err)
	}
	// Válido.
	c, err := dominio.NovoConsentimento("doente-1", dominio.FinalidadeCirurgia, true, "s3://doc.pdf", quando)
	if err != nil {
		t.Fatalf("consentimento de cirurgia válido não devia falhar: %v", err)
	}
	if !c.TemAnexo() || !c.EstaVigente() {
		t.Fatalf("esperado com anexo e vigente")
	}
}

func TestConsentimento_Revogar(t *testing.T) {
	quando := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	c, _ := dominio.NovoConsentimento("doente-1", dominio.FinalidadeTratamento, true, "", quando)
	if err := c.Revogar(quando); err != nil {
		t.Fatalf("revogar devia funcionar: %v", err)
	}
	if c.EstaVigente() {
		t.Fatalf("consentimento revogado não devia estar vigente")
	}
	if err := c.Revogar(quando); err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("revogar de novo devia falhar com Conflito, veio %v", err)
	}
}

func TestParseFinalidade_Invalida(t *testing.T) {
	if _, err := dominio.ParseFinalidade("QUALQUER"); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("finalidade inválida devia falhar com Validacao, veio %v", err)
	}
}
