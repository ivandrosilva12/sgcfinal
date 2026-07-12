// Package farmacia (adaptadores) contém adaptadores de saída do BC Farmácia.
// Camada 3 — Adaptadores.
package farmacia

import (
	"context"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// LeitorClinico implementa a porta anti-corrupção appfarmacia.LeitorClinico,
// lendo o BC Clínico através dos seus repositórios.
type LeitorClinico struct {
	doentes   clinico.RepositorioDoentes
	episodios clinico.RepositorioEpisodios
}

// NovoLeitorClinico constrói o adaptador sobre os repositórios clínicos.
func NovoLeitorClinico(doentes clinico.RepositorioDoentes, episodios clinico.RepositorioEpisodios) *LeitorClinico {
	return &LeitorClinico{doentes: doentes, episodios: episodios}
}

// ObterContextoDoente devolve se o doente existe e está activo, e as suas alergias
// GRAVE/ANAFILÁCTICA. Um doente inexistente devolve activo=false sem erro.
func (l *LeitorClinico) ObterContextoDoente(ctx context.Context, doenteID string) (bool, []appfarmacia.AlergiaClinica, error) {
	d, err := l.doentes.ObterPorID(ctx, doenteID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return false, nil, nil
		}
		return false, nil, err
	}
	s := d.Snapshot()
	var graves []appfarmacia.AlergiaClinica
	for _, a := range s.Alergias {
		if a.Severidade == clinico.SeveridadeGrave || a.Severidade == clinico.SeveridadeAnafilactica {
			graves = append(graves, appfarmacia.AlergiaClinica{Substancia: a.Substancia, Severidade: string(a.Severidade)})
		}
	}
	return d.Estado() == clinico.EstadoActivo, graves, nil
}

// EpisodioDoDoente indica se o episódio existe e pertence ao doente.
func (l *LeitorClinico) EpisodioDoDoente(ctx context.Context, episodioID, doenteID string) (bool, error) {
	ep, err := l.episodios.ObterPorID(ctx, episodioID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return false, nil
		}
		return false, err
	}
	return ep.DoenteID() == doenteID, nil
}

// Garantia de conformidade com a porta.
var _ appfarmacia.LeitorClinico = (*LeitorClinico)(nil)
