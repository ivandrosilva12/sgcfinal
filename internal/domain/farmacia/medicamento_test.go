package farmacia_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func medicamentoValido(t *testing.T) *farmacia.Medicamento {
	t.Helper()
	m, err := farmacia.NovoMedicamento("MED-00001", "Amoxil 500mg", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "GSK", true, false, nil, 10)
	if err != nil {
		t.Fatalf("NovoMedicamento: %v", err)
	}
	return m
}

func TestNovoMedicamento_Valido(t *testing.T) {
	m := medicamentoValido(t)
	if m.CodigoInterno() != "MED-00001" || !m.Activo() {
		t.Fatalf("medicamento inesperado: %+v", m.Snapshot())
	}
}

func TestNovoMedicamento_CamposObrigatorios(t *testing.T) {
	if _, err := farmacia.NovoMedicamento("MED-1", "", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, 10); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para nome comercial vazio")
	}
	if _, err := farmacia.NovoMedicamento("MED-1", "Amoxil", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, -1); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para stock mínimo negativo")
	}
}

func TestCorrespondeSubstancia(t *testing.T) {
	m := medicamentoValido(t)
	if !m.CorrespondeSubstancia("amoxicilina") {
		t.Fatal("esperava correspondência com o nome genérico (case-insensitive)")
	}
	if !m.CorrespondeSubstancia("AMOXIL") {
		t.Fatal("esperava correspondência com o nome comercial")
	}
	if m.CorrespondeSubstancia("Penicilina") {
		t.Fatal("não devia corresponder a substância ausente")
	}
}

func TestMedicamento_ActivarDesactivar(t *testing.T) {
	m := medicamentoValido(t)
	m.Desactivar()
	if m.Activo() {
		t.Fatal("esperava inactivo após Desactivar")
	}
	m.Activar()
	if !m.Activo() {
		t.Fatal("esperava activo após Activar")
	}
}

func TestReconstruirMedicamento_PreservaEstado(t *testing.T) {
	orig := medicamentoValido(t)
	orig.Desactivar()
	snap := orig.Snapshot()
	snap.ID = "id-1"
	rec := farmacia.ReconstruirMedicamento(snap)
	if rec.ID() != "id-1" || rec.Activo() {
		t.Fatalf("rehidratação perdeu estado: id=%q activo=%v", rec.ID(), rec.Activo())
	}
}
