package clinico_test

import (
	"context"
	"strconv"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeRepoEpisodios é um repositório de episódios em memória para os testes.
type fakeRepoEpisodios struct {
	porID      map[string]*clinico.EpisodioClinico
	seq        int
	guardarErr error
	pagina     clinico.PaginaEpisodios
	ultimoFilt clinico.FiltroEpisodios
}

func novoFakeRepoEpisodios() *fakeRepoEpisodios {
	return &fakeRepoEpisodios{porID: map[string]*clinico.EpisodioClinico{}}
}

func (f *fakeRepoEpisodios) Guardar(_ context.Context, e *clinico.EpisodioClinico) (string, error) {
	if f.guardarErr != nil {
		return "", f.guardarErr
	}
	snap := e.Snapshot()
	id := snap.ID
	if id == "" {
		f.seq++
		id = "ep-" + strconv.Itoa(f.seq)
		snap.ID = id
	}
	f.porID[id] = clinico.ReconstruirEpisodio(snap)
	return id, nil
}

func (f *fakeRepoEpisodios) ObterPorID(_ context.Context, id string) (*clinico.EpisodioClinico, error) {
	e, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "episódio não encontrado")
	}
	return e, nil
}

func (f *fakeRepoEpisodios) ListarPorDoente(_ context.Context, filt clinico.FiltroEpisodios) (clinico.PaginaEpisodios, error) {
	f.ultimoFilt = filt
	return f.pagina, nil
}
