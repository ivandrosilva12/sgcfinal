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
	// obterErr/obterErrNaChamada: ver comentário equivalente em fakeRepo
	// (fakes_test.go) — permite simular a leitura inicial ou a releitura final a
	// falhar isoladamente.
	obterErr          error
	obterErrNaChamada int
	obterChamadas     int
	// listarErr, se definido, faz ListarPorDoente falhar (usado por ObterEHR e
	// ListarEpisodios).
	listarErr  error
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
	f.obterChamadas++
	if f.obterErr != nil && (f.obterErrNaChamada == 0 || f.obterChamadas == f.obterErrNaChamada) {
		return nil, f.obterErr
	}
	e, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "episódio não encontrado")
	}
	return e, nil
}

func (f *fakeRepoEpisodios) ListarPorDoente(_ context.Context, filt clinico.FiltroEpisodios) (clinico.PaginaEpisodios, error) {
	f.ultimoFilt = filt
	if f.listarErr != nil {
		return clinico.PaginaEpisodios{}, f.listarErr
	}
	return f.pagina, nil
}
