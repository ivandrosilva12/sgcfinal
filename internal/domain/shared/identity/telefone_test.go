package identity_test

import (
	"errors"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/identity"
)

func TestNovoTelefone_Valido(t *testing.T) {
	casos := []struct {
		nome            string
		entrada         string
		esperadoDisplay string
		esperadoE164    string
	}{
		{"nacional simples", "923456789", "+244 923 456 789", "+244923456789"},
		{"com indicativo", "+244923456789", "+244 923 456 789", "+244923456789"},
		{"com espaços e sinais", "+244 923-456 789", "+244 923 456 789", "+244923456789"},
		{"244 sem +", "244923456789", "+244 923 456 789", "+244923456789"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			tel, err := identity.NovoTelefone(c.entrada)
			if err != nil {
				t.Fatalf("esperava sucesso, obtive erro: %v", err)
			}
			if got := tel.String(); got != c.esperadoDisplay {
				t.Fatalf("display: esperava %q, obtive %q", c.esperadoDisplay, got)
			}
			if got := tel.E164(); got != c.esperadoE164 {
				t.Fatalf("E164: esperava %q, obtive %q", c.esperadoE164, got)
			}
		})
	}
}

func TestTelefone_ZeroValue(t *testing.T) {
	var tel identity.Telefone
	if tel.Valido() {
		t.Fatal("telefone de valor-zero não deve ser válido")
	}
	if tel.String() != "" {
		t.Fatalf("display de valor-zero deve ser vazio, obtive %q", tel.String())
	}
	if tel.E164() != "" {
		t.Fatalf("E164 de valor-zero deve ser vazio, obtive %q", tel.E164())
	}
}

func TestTelefone_ValidoTrue(t *testing.T) {
	tel, _ := identity.NovoTelefone("923456789")
	if !tel.Valido() {
		t.Fatal("telefone construído deve ser válido")
	}
}

func TestNovoTelefone_Invalido(t *testing.T) {
	casos := []string{
		"",
		"823456789",     // não começa por 9
		"92345678",      // 8 dígitos
		"9234567890",    // 10 dígitos
		"+351923456789", // indicativo errado (fica com dígitos a mais)
	}
	for _, entrada := range casos {
		t.Run(entrada, func(t *testing.T) {
			if _, err := identity.NovoTelefone(entrada); !errors.Is(err, identity.ErrTelefoneInvalido) {
				t.Fatalf("esperava ErrTelefoneInvalido para %q, obtive %v", entrada, err)
			}
		})
	}
}
