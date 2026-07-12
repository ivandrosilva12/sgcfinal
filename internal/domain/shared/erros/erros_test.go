package erros_test

import (
	"errors"
	"fmt"
	"testing"

	dominioerros "github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovo_ErrorEMensagem(t *testing.T) {
	e := dominioerros.Novo(dominioerros.CategoriaValidacao, "BI inválido")
	if e.Error() != "BI inválido" {
		t.Fatalf("mensagem inesperada: %q", e.Error())
	}
	if e.Categoria != dominioerros.CategoriaValidacao {
		t.Fatalf("categoria inesperada: %v", e.Categoria)
	}
}

func TestUnwrap(t *testing.T) {
	causa := errors.New("causa raiz")
	e := &dominioerros.ErroDominio{
		Categoria: dominioerros.CategoriaInterno,
		Mensagem:  "falhou",
		Causa:     causa,
	}
	if !errors.Is(e, causa) {
		t.Fatal("errors.Is deve encontrar a causa via Unwrap")
	}
}

func TestCategoriaRegraNegocio_RoundTrip(t *testing.T) {
	err := dominioerros.Novo(dominioerros.CategoriaRegraNegocio, "regra violada")
	if dominioerros.CategoriaDe(err) != dominioerros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio, obtive %v", dominioerros.CategoriaDe(err))
	}
}

func TestCategoriaDe(t *testing.T) {
	casos := []struct {
		nome     string
		err      error
		esperado dominioerros.Categoria
	}{
		{"erro de domínio", dominioerros.Novo(dominioerros.CategoriaProibido, "x"), dominioerros.CategoriaProibido},
		{"erro embrulhado", fmt.Errorf("contexto: %w", dominioerros.Novo(dominioerros.CategoriaNaoEncontrado, "y")), dominioerros.CategoriaNaoEncontrado},
		{"erro genérico", errors.New("qualquer"), dominioerros.CategoriaInterno},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := dominioerros.CategoriaDe(c.err); got != c.esperado {
				t.Fatalf("esperava %v, obtive %v", c.esperado, got)
			}
		})
	}
}
