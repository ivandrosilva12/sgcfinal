package identidade_test

import (
	"testing"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestAtualizarContacto_Valido(t *testing.T) {
	u := &dominio.Utilizador{KeycloakID: "id-1", Nome: "Ana", Email: "ana@sgc.ao", Activo: true}
	if err := u.AtualizarContacto("+244 923 456 789", "00123456LA042"); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if u.Telefone != "+244 923 456 789" {
		t.Fatalf("telefone normalizado errado: %q", u.Telefone)
	}
	if u.BI != "00123456LA042" {
		t.Fatalf("BI normalizado errado: %q", u.BI)
	}
}

func TestAtualizarContacto_LimpaComVazio(t *testing.T) {
	u := &dominio.Utilizador{KeycloakID: "id-1", Telefone: "+244 923 456 789", BI: "00123456LA042"}
	if err := u.AtualizarContacto("", ""); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if u.Telefone != "" || u.BI != "" {
		t.Fatalf("esperava campos limpos, obtive tel=%q bi=%q", u.Telefone, u.BI)
	}
}

func TestAtualizarContacto_TelefoneInvalido(t *testing.T) {
	u := &dominio.Utilizador{KeycloakID: "id-1"}
	err := u.AtualizarContacto("123", "")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestAtualizarContacto_BIInvalido(t *testing.T) {
	u := &dominio.Utilizador{KeycloakID: "id-1"}
	err := u.AtualizarContacto("", "invalido")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}
