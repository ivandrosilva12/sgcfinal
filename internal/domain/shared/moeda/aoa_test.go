package moeda_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
)

func TestAOA_String(t *testing.T) {
	casos := []struct {
		nome     string
		centimos int64
		esperado string
	}{
		{"zero", 0, "0,00 Kz"},
		{"cêntimos apenas", 5, "0,05 Kz"},
		{"unidade", 100, "1,00 Kz"},
		{"milhar com decimais", 123450, "1.234,50 Kz"},
		{"milhão", 100000000, "1.000.000,00 Kz"},
		{"negativo", -123450, "-1.234,50 Kz"},
		{"centena", 99900, "999,00 Kz"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := moeda.DeCentimos(c.centimos).String(); got != c.esperado {
				t.Fatalf("esperava %q, obtive %q", c.esperado, got)
			}
		})
	}
}

func TestAOA_Aritmetica(t *testing.T) {
	a := moeda.DeKwanzas(1000)
	b := moeda.DeCentimos(50)

	if got := a.Somar(b).Centimos(); got != 100050 {
		t.Fatalf("somar: esperava 100050, obtive %d", got)
	}
	if got := b.Subtrair(a); !got.Negativo() {
		t.Fatalf("subtrair: esperava resultado negativo, obtive %s", got)
	}
	if a.Negativo() {
		t.Fatal("1000 Kz não deve ser negativo")
	}
}

func TestAOA_DeKwanzas(t *testing.T) {
	if got := moeda.DeKwanzas(1234).String(); got != "1.234,00 Kz" {
		t.Fatalf("esperava \"1.234,00 Kz\", obtive %q", got)
	}
}
