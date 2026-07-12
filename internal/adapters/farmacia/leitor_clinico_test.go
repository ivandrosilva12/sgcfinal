package farmacia_test

import (
	"context"
	"testing"
	"time"

	adfarmacia "github.com/ivandrosilva12/sgcfinal/internal/adapters/farmacia"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakes dos repositórios clínicos (só os métodos usados pelo LeitorClinico).
type fakeDoentes struct{ d *clinico.Doente }

func (f fakeDoentes) Guardar(context.Context, *clinico.Doente) (string, error) { return "", nil }
func (f fakeDoentes) ObterPorID(_ context.Context, _ string) (*clinico.Doente, error) {
	if f.d == nil {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
	}
	return f.d, nil
}
func (f fakeDoentes) ObterPorNumProcesso(context.Context, string) (*clinico.Doente, error) {
	return nil, erros.Novo(erros.CategoriaNaoEncontrado, "n/d")
}
func (f fakeDoentes) Pesquisar(context.Context, clinico.FiltroDoentes) (clinico.PaginaDoentes, error) {
	return clinico.PaginaDoentes{}, nil
}
func (f fakeDoentes) ProximoNumeroProcesso(context.Context, int) (string, error) { return "", nil }

type fakeEpisodios struct{ e *clinico.EpisodioClinico }

func (f fakeEpisodios) Guardar(context.Context, *clinico.EpisodioClinico) (string, error) {
	return "", nil
}
func (f fakeEpisodios) ObterPorID(_ context.Context, _ string) (*clinico.EpisodioClinico, error) {
	if f.e == nil {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "episódio não encontrado")
	}
	return f.e, nil
}
func (f fakeEpisodios) ListarPorDoente(context.Context, clinico.FiltroEpisodios) (clinico.PaginaEpisodios, error) {
	return clinico.PaginaEpisodios{}, nil
}

func doenteComAlergiaGrave(t *testing.T) *clinico.Doente {
	t.Helper()
	snap := clinico.SnapshotDoente{
		ID:     "d-1",
		Estado: clinico.EstadoActivo,
		Alergias: []clinico.Alergia{
			{Substancia: "Penicilina", Severidade: clinico.SeveridadeGrave},
			{Substancia: "Pó", Severidade: clinico.SeveridadeLeve},
		},
	}
	return clinico.ReconstruirDoente(snap)
}

func TestObterContextoDoente_FiltraAlergiasGraves(t *testing.T) {
	leitor := adfarmacia.NovoLeitorClinico(fakeDoentes{d: doenteComAlergiaGrave(t)}, fakeEpisodios{})
	activo, alergias, err := leitor.ObterContextoDoente(context.Background(), "d-1")
	if err != nil || !activo {
		t.Fatalf("esperava activo: %v", err)
	}
	if len(alergias) != 1 || alergias[0].Substancia != "Penicilina" {
		t.Fatalf("esperava só a alergia grave: %+v", alergias)
	}
}

func TestObterContextoDoente_Inexistente(t *testing.T) {
	leitor := adfarmacia.NovoLeitorClinico(fakeDoentes{}, fakeEpisodios{})
	activo, _, err := leitor.ObterContextoDoente(context.Background(), "x")
	if err != nil || activo {
		t.Fatalf("doente inexistente devia dar activo=false sem erro; got activo=%v err=%v", activo, err)
	}
}

func TestEpisodioDoDoente(t *testing.T) {
	// Reconstrói um episódio do doente d-1.
	ep := clinico.ReconstruirEpisodio(clinico.SnapshotEpisodio{ID: "e-1", DoenteID: "d-1", Estado: clinico.EstadoEpisodioAberto, Inicio: time.Now()})
	leitor := adfarmacia.NovoLeitorClinico(fakeDoentes{}, fakeEpisodios{e: ep})
	ok, err := leitor.EpisodioDoDoente(context.Background(), "e-1", "d-1")
	if err != nil || !ok {
		t.Fatalf("esperava pertença: ok=%v err=%v", ok, err)
	}
	nok, _ := leitor.EpisodioDoDoente(context.Background(), "e-1", "outro")
	if nok {
		t.Fatal("não devia pertencer a outro doente")
	}
}
