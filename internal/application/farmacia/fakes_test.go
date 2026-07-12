package farmacia_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeRepoMed é um repositório de medicamentos em memória.
type fakeRepoMed struct {
	porID      map[string]*farmacia.Medicamento
	seq        int
	guardarErr error
	pagina     farmacia.PaginaMedicamentos
	ultimoFilt farmacia.FiltroMedicamentos
}

func novoFakeRepoMed() *fakeRepoMed { return &fakeRepoMed{porID: map[string]*farmacia.Medicamento{}} }

func (f *fakeRepoMed) Guardar(_ context.Context, m *farmacia.Medicamento) (string, error) {
	if f.guardarErr != nil {
		return "", f.guardarErr
	}
	snap := m.Snapshot()
	id := snap.ID
	if id == "" {
		f.seq++
		id = "med-" + strconv.Itoa(f.seq)
		snap.ID = id
	}
	f.porID[id] = farmacia.ReconstruirMedicamento(snap)
	return id, nil
}
func (f *fakeRepoMed) ObterPorID(_ context.Context, id string) (*farmacia.Medicamento, error) {
	m, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "medicamento não encontrado")
	}
	return m, nil
}
func (f *fakeRepoMed) Pesquisar(_ context.Context, filt farmacia.FiltroMedicamentos) (farmacia.PaginaMedicamentos, error) {
	f.ultimoFilt = filt
	return f.pagina, nil
}
func (f *fakeRepoMed) ProximoCodigo(_ context.Context) (string, error) {
	f.seq++
	return "MED-" + leftPad(f.seq), nil
}

func leftPad(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 5 {
		s = "0" + s
	}
	return s
}

// fakeAuditor recolhe os registos de auditoria.
type fakeAuditor struct{ registos []auditoria.Registo }

func (a *fakeAuditor) Registar(_ context.Context, r auditoria.Registo) error {
	a.registos = append(a.registos, r)
	return nil
}

// medicamentoParaRepo constrói um *farmacia.Medicamento válido para testes
// (reutilizado pela Task 6).
func medicamentoParaRepo(t *testing.T) *farmacia.Medicamento {
	t.Helper()
	m, err := farmacia.NovoMedicamento("MED-00001", "Amoxil", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
