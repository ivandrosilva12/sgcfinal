package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovosContactos_NormalizaTelefone(t *testing.T) {
	ct, err := clinico.NovosContactos("+244923456789", nil, nil)
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if ct.Telefone != "+244 923 456 789" {
		t.Fatalf("telefone não normalizado: %q", ct.Telefone)
	}
}

func TestNovosContactos_TelefoneObrigatorio(t *testing.T) {
	_, err := clinico.NovosContactos("", nil, nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestNovosContactos_EmailInvalido(t *testing.T) {
	mau := "nao-e-email"
	_, err := clinico.NovosContactos("+244923456789", &mau, nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestNovosContactos_ComMorada(t *testing.T) {
	m := &clinico.Morada{Provincia: "Luanda", Municipio: "Belas", Comuna: "Benfica", Bairro: "Morro Bento", Rua: "Rua 1"}
	ct, err := clinico.NovosContactos("+244923456789", nil, m)
	if err != nil || ct.Morada == nil || ct.Morada.Provincia != "Luanda" {
		t.Fatalf("morada não preservada: %+v, %v", ct.Morada, err)
	}
}
