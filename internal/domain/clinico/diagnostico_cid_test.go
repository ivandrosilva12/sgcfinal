package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovoDiagnosticoCID(t *testing.T) {
	d, err := clinico.NovoDiagnosticoCID("J11", true)
	if err != nil || d.CID != "J11" || !d.Principal {
		t.Fatalf("diagnóstico inesperado: %+v, %v", d, err)
	}
}

func TestNovoDiagnosticoCID_Vazio(t *testing.T) {
	if _, err := clinico.NovoDiagnosticoCID("  ", false); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}
