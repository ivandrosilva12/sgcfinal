package identity

import "testing"

func TestNovoNIF(t *testing.T) {
	casos := []struct {
		nome    string
		entrada string
		querErr bool
		querStr string
	}{
		{"colectiva 10 dígitos", "5417000001", false, "5417000001"},
		{"singular 9 dígitos + letra", "004567890A", false, "004567890A"},
		{"minúsculas normalizadas", "004567890a", false, "004567890A"},
		{"com espaços", " 5417 000 001 ", false, "5417000001"},
		{"curto demais", "12345", true, ""},
		{"longo demais", "12345678901", true, ""},
		{"letra no meio", "5417A00001", true, ""},
		{"vazio", "", true, ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			nif, err := NovoNIF(c.entrada)
			if c.querErr {
				if err == nil {
					t.Fatalf("esperava erro para %q", c.entrada)
				}
				return
			}
			if err != nil {
				t.Fatalf("inesperado: %v", err)
			}
			if nif.String() != c.querStr {
				t.Fatalf("String()=%q, esperava %q", nif.String(), c.querStr)
			}
			if !nif.Valido() {
				t.Fatal("esperava Valido()=true")
			}
		})
	}
}
