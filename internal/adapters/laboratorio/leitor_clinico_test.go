package laboratorio_test

import (
	"context"
	"testing"
	"time"

	adlaboratorio "github.com/ivandrosilva12/sgcfinal/internal/adapters/laboratorio"
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

func doenteActivo(id string) *clinico.Doente {
	return clinico.ReconstruirDoente(clinico.SnapshotDoente{ID: id, Estado: clinico.EstadoActivo})
}

func doenteInactivo(id string) *clinico.Doente {
	return clinico.ReconstruirDoente(clinico.SnapshotDoente{ID: id, Estado: clinico.EstadoInactivo})
}

func TestDoenteActivo_Activo(t *testing.T) {
	leitor := adlaboratorio.NovoLeitorClinico(fakeDoentes{d: doenteActivo("d-1")}, fakeEpisodios{})
	activo, err := leitor.DoenteActivo(context.Background(), "d-1")
	if err != nil || !activo {
		t.Fatalf("esperava activo=true sem erro; activo=%v err=%v", activo, err)
	}
}

func TestDoenteActivo_Inactivo(t *testing.T) {
	leitor := adlaboratorio.NovoLeitorClinico(fakeDoentes{d: doenteInactivo("d-1")}, fakeEpisodios{})
	activo, err := leitor.DoenteActivo(context.Background(), "d-1")
	if err != nil || activo {
		t.Fatalf("esperava activo=false sem erro para doente inactivo; activo=%v err=%v", activo, err)
	}
}

// TestDoenteActivo_Inexistente prova a tradução central da ACL: um doente
// inexistente devolve activo=false sem erro — "não existe" e "não pode" são a
// mesma resposta para o Laboratório.
func TestDoenteActivo_Inexistente(t *testing.T) {
	leitor := adlaboratorio.NovoLeitorClinico(fakeDoentes{}, fakeEpisodios{})
	activo, err := leitor.DoenteActivo(context.Background(), "inexistente")
	if err != nil || activo {
		t.Fatalf("doente inexistente devia dar activo=false sem erro; activo=%v err=%v", activo, err)
	}
}

func TestEpisodioAbertoDoDoente_AbertoEPertence(t *testing.T) {
	ep := clinico.ReconstruirEpisodio(clinico.SnapshotEpisodio{
		ID: "e-1", DoenteID: "d-1", Estado: clinico.EstadoEpisodioAberto, Inicio: time.Now(),
	})
	leitor := adlaboratorio.NovoLeitorClinico(fakeDoentes{}, fakeEpisodios{e: ep})
	ok, err := leitor.EpisodioAbertoDoDoente(context.Background(), "e-1", "d-1")
	if err != nil || !ok {
		t.Fatalf("esperava aberto=true; ok=%v err=%v", ok, err)
	}
}

func TestEpisodioAbertoDoDoente_OutroDoente(t *testing.T) {
	ep := clinico.ReconstruirEpisodio(clinico.SnapshotEpisodio{
		ID: "e-1", DoenteID: "d-1", Estado: clinico.EstadoEpisodioAberto, Inicio: time.Now(),
	})
	leitor := adlaboratorio.NovoLeitorClinico(fakeDoentes{}, fakeEpisodios{e: ep})
	ok, err := leitor.EpisodioAbertoDoDoente(context.Background(), "e-1", "outro-doente")
	if err != nil || ok {
		t.Fatalf("episódio de outro doente não devia validar; ok=%v err=%v", ok, err)
	}
}

// TestEpisodioAbertoDoDoente_Fechado prova a segunda invariante da ACL: um
// episódio fechado não passa, mesmo pertencendo ao doente — requisitar para um
// episódio fechado deixaria resultados órfãos na fila.
func TestEpisodioAbertoDoDoente_Fechado(t *testing.T) {
	ep := clinico.ReconstruirEpisodio(clinico.SnapshotEpisodio{
		ID: "e-1", DoenteID: "d-1", Estado: clinico.EstadoEpisodioFechado, Inicio: time.Now(),
	})
	leitor := adlaboratorio.NovoLeitorClinico(fakeDoentes{}, fakeEpisodios{e: ep})
	ok, err := leitor.EpisodioAbertoDoDoente(context.Background(), "e-1", "d-1")
	if err != nil || ok {
		t.Fatalf("episódio fechado não devia validar; ok=%v err=%v", ok, err)
	}
}

func TestEpisodioAbertoDoDoente_Inexistente(t *testing.T) {
	leitor := adlaboratorio.NovoLeitorClinico(fakeDoentes{}, fakeEpisodios{})
	ok, err := leitor.EpisodioAbertoDoDoente(context.Background(), "inexistente", "d-1")
	if err != nil || ok {
		t.Fatalf("episódio inexistente devia dar ok=false sem erro; ok=%v err=%v", ok, err)
	}
}
