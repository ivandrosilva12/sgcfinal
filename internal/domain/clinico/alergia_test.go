package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovaAlergia(t *testing.T) {
	a, err := clinico.NovaAlergia("Penicilina", clinico.SeveridadeGrave, "Urticária", nil, "")
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if a.Substancia != "Penicilina" || a.Severidade != clinico.SeveridadeGrave {
		t.Fatalf("alergia inesperada: %+v", a)
	}
}

func TestNovaAlergia_SubstanciaObrigatoria(t *testing.T) {
	_, err := clinico.NovaAlergia("  ", clinico.SeveridadeLeve, "", nil, "")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestNovaAlergia_SeveridadeInvalida(t *testing.T) {
	_, err := clinico.NovaAlergia("Penicilina", clinico.Severidade("EXTREMA"), "", nil, "")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}
