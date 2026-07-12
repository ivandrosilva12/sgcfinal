package clinico_test

import (
	"context"
	"strconv"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeRepo é um repositório de doentes em memória para os testes de aplicação.
type fakeRepo struct {
	porID      map[string]*clinico.Doente
	seq        int
	proxErr    error
	guardarErr error
	pagina     clinico.PaginaDoentes
	ultimoFilt clinico.FiltroDoentes
}

func novoFakeRepo() *fakeRepo { return &fakeRepo{porID: map[string]*clinico.Doente{}} }

func (f *fakeRepo) Guardar(_ context.Context, d *clinico.Doente) (string, error) {
	if f.guardarErr != nil {
		return "", f.guardarErr
	}
	snap := d.Snapshot()
	id := snap.ID
	if id == "" {
		f.seq++
		id = "id-" + strconv.Itoa(f.seq)
		snap.ID = id
	}
	f.porID[id] = clinico.ReconstruirDoente(snap)
	return id, nil
}

func (f *fakeRepo) ObterPorID(_ context.Context, id string) (*clinico.Doente, error) {
	d, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
	}
	return d, nil
}

func (f *fakeRepo) ObterPorNumProcesso(_ context.Context, num string) (*clinico.Doente, error) {
	for _, d := range f.porID {
		if d.NumProcesso() == num {
			return d, nil
		}
	}
	return nil, erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
}

func (f *fakeRepo) Pesquisar(_ context.Context, filt clinico.FiltroDoentes) (clinico.PaginaDoentes, error) {
	f.ultimoFilt = filt
	return f.pagina, nil
}

func (f *fakeRepo) ProximoNumeroProcesso(_ context.Context, ano int) (string, error) {
	if f.proxErr != nil {
		return "", f.proxErr
	}
	f.seq++
	return "P-" + strconv.Itoa(ano) + "-" + leftPad(f.seq), nil
}

func leftPad(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 6 {
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
