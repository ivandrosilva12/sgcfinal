package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovoAntecedente(t *testing.T) {
	a, err := clinico.NovoAntecedente(clinico.AntecedentePessoal, "Hipertensão", "I10", nil, true, "")
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if a.Tipo != clinico.AntecedentePessoal || !a.Activo {
		t.Fatalf("antecedente inesperado: %+v", a)
	}
}

func TestNovoAntecedente_DescricaoObrigatoria(t *testing.T) {
	_, err := clinico.NovoAntecedente(clinico.AntecedenteFamiliar, "", "", nil, true, "")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestParseTipoAntecedente(t *testing.T) {
	if _, err := clinico.ParseTipoAntecedente("cirurgico"); err != nil {
		t.Fatalf("cirurgico devia ser válido: %v", err)
	}
	if _, err := clinico.ParseTipoAntecedente("GENETICO"); err == nil {
		t.Fatal("esperava erro para tipo inválido")
	}
}
