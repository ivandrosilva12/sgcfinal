package identity_test

import (
	"errors"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/identity"
)

func TestNovoBI_Valido(t *testing.T) {
	casos := []struct {
		nome     string
		entrada  string
		esperado string
	}{
		{"formato canónico", "00123456LA042", "00123456LA042"},
		{"minúsculas normalizadas", "00123456la042", "00123456LA042"},
		{"com espaços", " 00123456 LA 042 ", "00123456LA042"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			bi, err := identity.NovoBI(c.entrada)
			if err != nil {
				t.Fatalf("esperava sucesso, obtive erro: %v", err)
			}
			if bi.String() != c.esperado {
				t.Fatalf("esperava %q, obtive %q", c.esperado, bi.String())
			}
			if !bi.Valido() {
				t.Fatal("esperava BI válido")
			}
		})
	}
}

func TestNovoBI_Invalido(t *testing.T) {
	casos := []string{
		"",
		"12345678AB12",   // 2 dígitos finais em vez de 3
		"1234567AB123",   // 7 dígitos iniciais
		"00123456L0042",  // apenas 1 letra
		"00123456LAB042", // 3 letras
		"ABCDEFGHLA042",  // letras onde deviam ser dígitos
	}
	for _, entrada := range casos {
		t.Run(entrada, func(t *testing.T) {
			if _, err := identity.NovoBI(entrada); !errors.Is(err, identity.ErrBIInvalido) {
				t.Fatalf("esperava ErrBIInvalido para %q, obtive %v", entrada, err)
			}
		})
	}
}

func TestBI_ZeroValueInvalido(t *testing.T) {
	var bi identity.BI
	if bi.Valido() {
		t.Fatal("BI de valor-zero não deve ser válido")
	}
}
