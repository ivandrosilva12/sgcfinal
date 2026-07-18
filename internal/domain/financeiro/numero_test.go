package financeiro_test

import (
	"testing"
	"time"

	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
)

func TestNumeroFactura_FormatoLegal(t *testing.T) {
	n, err := fin.NovoNumeroFactura("2026", 12345)
	if err != nil {
		t.Fatalf("NovoNumeroFactura: %v", err)
	}
	if got, quer := n.String(), "FAC 2026/00012345"; got != quer {
		t.Errorf("número = %q, queria %q", got, quer)
	}
}

func TestNumeroFactura_IdaEVolta(t *testing.T) {
	n, _ := fin.NovoNumeroFactura("2026", 7)
	serie, seq, err := fin.ParseNumeroFactura(n.String())
	if err != nil {
		t.Fatalf("ParseNumeroFactura: %v", err)
	}
	if serie != "2026" || seq != 7 {
		t.Errorf("parse = (%q,%d), queria (\"2026\",7)", serie, seq)
	}
}

func TestNumeroFactura_Invalidos(t *testing.T) {
	if _, err := fin.NovoNumeroFactura("", 1); err == nil {
		t.Error("série vazia devia falhar")
	}
	if _, err := fin.NovoNumeroFactura("2026", 0); err == nil {
		t.Error("sequencial zero devia falhar")
	}
	for _, s := range []string{"", "FAC2026/00000001", "REC 2026/00000001", "FAC 2026/abc"} {
		if _, _, err := fin.ParseNumeroFactura(s); err == nil {
			t.Errorf("ParseNumeroFactura(%q) devia falhar", s)
		}
	}
}

func TestSerieDe_EhOAno(t *testing.T) {
	m := time.Date(2026, 7, 18, 23, 30, 0, 0, time.FixedZone("WAT", 1*60*60))
	if got := fin.SerieDe(m); got != "2026" {
		t.Errorf("SerieDe = %q, queria \"2026\" (ano em UTC)", got)
	}
}

// O formato legal AGT fixa o sequencial em 8 dígitos. Acima de 99999999 o
// "%08d" alargaria o campo em silêncio, produzindo um número fora do formato
// sem qualquer erro — a série tem de esgotar com um erro explícito.
func TestNovoNumeroFactura_RejeitaSequencialAcimaDoLimite(t *testing.T) {
	if _, err := fin.NovoNumeroFactura("2026", 99999999); err != nil {
		t.Errorf("99999999 é o último sequencial válido, não devia falhar: %v", err)
	}
	for _, seq := range []int{100000000, 123456789} {
		n, err := fin.NovoNumeroFactura("2026", seq)
		if err == nil {
			t.Errorf("NovoNumeroFactura(2026, %d) devia falhar, devolveu %q", seq, n)
		}
	}
}
