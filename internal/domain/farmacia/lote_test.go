package farmacia_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func loteValido(t *testing.T) *farmacia.Lote {
	t.Helper()
	l, err := farmacia.NovoLote("med-1", "L001", time.Now().AddDate(1, 0, 0), 100, "12.3456", nil, "")
	if err != nil {
		t.Fatalf("NovoLote: %v", err)
	}
	return l
}

func TestNovoLote_Valido(t *testing.T) {
	l := loteValido(t)
	if l.QuantidadeActual() != 100 || l.MedicamentoID() != "med-1" {
		t.Fatalf("lote inesperado: %+v", l.Snapshot())
	}
	if !l.Disponivel(time.Now()) {
		t.Fatal("esperava disponível")
	}
}

func TestNovoLote_ValidadePassada(t *testing.T) {
	if _, err := farmacia.NovoLote("med-1", "L001", time.Now().AddDate(0, 0, -1), 10, "1", nil, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para validade passada")
	}
}

func TestNovoLote_QuantidadeEPreco(t *testing.T) {
	fut := time.Now().AddDate(1, 0, 0)
	if _, err := farmacia.NovoLote("med-1", "L001", fut, 0, "1", nil, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para quantidade zero")
	}
	if _, err := farmacia.NovoLote("med-1", "L001", fut, 5, "1.23456", nil, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para preço com >4 casas")
	}
	if _, err := farmacia.NovoLote("med-1", "L001", fut, 5, "abc", nil, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para preço não-numérico")
	}
}

func TestReconstruirLote(t *testing.T) {
	orig := loteValido(t)
	snap := orig.Snapshot()
	snap.ID = "lote-1"
	snap.QuantidadeActual = 40
	rec := farmacia.ReconstruirLote(snap)
	if rec.ID() != "lote-1" || rec.QuantidadeActual() != 40 {
		t.Fatalf("rehidratação perdeu estado: %+v", rec.Snapshot())
	}
}
