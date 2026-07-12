package farmacia_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovoFornecedor(t *testing.T) {
	f, err := farmacia.NovoFornecedor("Farmédica Lda", nil, nil)
	if err != nil || !f.Activo() {
		t.Fatalf("fornecedor inesperado: %v", err)
	}
}

func TestNovoFornecedor_NomeObrigatorio(t *testing.T) {
	if _, err := farmacia.NovoFornecedor("  ", nil, nil); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestFornecedor_ActivarDesactivar(t *testing.T) {
	f, _ := farmacia.NovoFornecedor("X", nil, nil)
	f.Desactivar()
	if f.Activo() {
		t.Fatal("esperava inactivo")
	}
	f.Activar()
	if !f.Activo() {
		t.Fatal("esperava activo")
	}
}

func TestReconstruirFornecedor(t *testing.T) {
	orig, _ := farmacia.NovoFornecedor("Y", nil, nil)
	orig.Desactivar()
	snap := orig.Snapshot()
	snap.ID = "f-1"
	rec := farmacia.ReconstruirFornecedor(snap)
	if rec.ID() != "f-1" || rec.Activo() {
		t.Fatalf("rehidratação perdeu estado: %+v", rec.Snapshot())
	}
}
