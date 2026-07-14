// Package laboratorio (adaptadores) contém adaptadores de saída do BC Laboratório.
// Camada 3 — Adaptadores.
package laboratorio

import (
	"context"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// LeitorClinico implementa a porta anti-corrupção applaboratorio.LeitorClinico,
// lendo o BC Clínico através dos seus repositórios e traduzindo o que interessa ao
// Laboratório (duas perguntas booleanas) — sem deixar passar tipos do Clínico.
type LeitorClinico struct {
	doentes   clinico.RepositorioDoentes
	episodios clinico.RepositorioEpisodios
}

// NovoLeitorClinico constrói o adaptador sobre os repositórios clínicos.
func NovoLeitorClinico(doentes clinico.RepositorioDoentes, episodios clinico.RepositorioEpisodios) *LeitorClinico {
	return &LeitorClinico{doentes: doentes, episodios: episodios}
}

// DoenteActivo indica se o doente existe e está activo. Um doente inexistente
// devolve false sem erro — para o Laboratório, "não existe" e "não pode" são a
// mesma resposta.
func (l *LeitorClinico) DoenteActivo(ctx context.Context, doenteID string) (bool, error) {
	d, err := l.doentes.ObterPorID(ctx, doenteID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return false, nil
		}
		return false, err
	}
	return d.Estado() == clinico.EstadoActivo, nil
}

// EpisodioAbertoDoDoente indica se o episódio existe, pertence ao doente e está
// ABERTO. Requisitar análises para um episódio fechado deixaria resultados órfãos na
// fila, sem consulta onde os devolver.
func (l *LeitorClinico) EpisodioAbertoDoDoente(ctx context.Context, episodioID, doenteID string) (bool, error) {
	ep, err := l.episodios.ObterPorID(ctx, episodioID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return false, nil
		}
		return false, err
	}
	return ep.DoenteID() == doenteID && ep.Estado() == clinico.EstadoEpisodioAberto, nil
}

// Garantia de conformidade com a porta.
var _ applaboratorio.LeitorClinico = (*LeitorClinico)(nil)
