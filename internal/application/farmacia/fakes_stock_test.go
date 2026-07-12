package farmacia_test

import (
	"context"
	"strconv"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeRepoFornecedores em memória.
type fakeRepoFornecedores struct {
	porID  map[string]*farmacia.Fornecedor
	seq    int
	pagina farmacia.PaginaFornecedores
}

func novoFakeRepoFornecedores() *fakeRepoFornecedores {
	return &fakeRepoFornecedores{porID: map[string]*farmacia.Fornecedor{}}
}
func (f *fakeRepoFornecedores) Guardar(_ context.Context, forn *farmacia.Fornecedor) (string, error) {
	snap := forn.Snapshot()
	id := snap.ID
	if id == "" {
		f.seq++
		id = "forn-" + strconv.Itoa(f.seq)
		snap.ID = id
	}
	f.porID[id] = farmacia.ReconstruirFornecedor(snap)
	return id, nil
}
func (f *fakeRepoFornecedores) ObterPorID(_ context.Context, id string) (*farmacia.Fornecedor, error) {
	forn, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "fornecedor não encontrado")
	}
	return forn, nil
}
func (f *fakeRepoFornecedores) Listar(_ context.Context, _ farmacia.FiltroFornecedores) (farmacia.PaginaFornecedores, error) {
	return f.pagina, nil
}

// fakeRepoLotes em memória.
type fakeRepoLotes struct {
	porID   map[string]*farmacia.Lote
	seq     int
	stock   int
	lotes   []farmacia.ResumoLote
	entrErr error
}

func novoFakeRepoLotes() *fakeRepoLotes {
	return &fakeRepoLotes{porID: map[string]*farmacia.Lote{}}
}
func (f *fakeRepoLotes) RegistarEntrada(_ context.Context, l *farmacia.Lote, _ string) (string, error) {
	if f.entrErr != nil {
		return "", f.entrErr
	}
	snap := l.Snapshot()
	f.seq++
	id := "lote-" + strconv.Itoa(f.seq)
	snap.ID = id
	f.porID[id] = farmacia.ReconstruirLote(snap)
	return id, nil
}
func (f *fakeRepoLotes) ObterPorID(_ context.Context, id string) (*farmacia.Lote, error) {
	l, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "lote não encontrado")
	}
	return l, nil
}
func (f *fakeRepoLotes) ListarPorMedicamento(_ context.Context, _ string, _ bool) ([]farmacia.ResumoLote, error) {
	return f.lotes, nil
}
func (f *fakeRepoLotes) StockDisponivel(_ context.Context, _ string) (int, error) {
	return f.stock, nil
}
