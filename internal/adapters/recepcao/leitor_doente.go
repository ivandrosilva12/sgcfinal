// Package recepcao (adaptadores) contém adaptadores de saída do BC Recepção.
// Camada 3 — Adaptadores.
package recepcao

import (
	"context"

	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// LeitorDoente implementa a porta anti-corrupção apprecepcao.LeitorDoente, lendo o BC
// Clínico através do seu repositório e traduzindo o que interessa à Recepção (uma
// pergunta booleana) — sem deixar passar tipos do Clínico.
type LeitorDoente struct {
	doentes clinico.RepositorioDoentes
}

// NovoLeitorDoente constrói o adaptador sobre o repositório de doentes.
func NovoLeitorDoente(doentes clinico.RepositorioDoentes) *LeitorDoente {
	return &LeitorDoente{doentes: doentes}
}

// DoenteActivo indica se o doente existe e está activo. Um doente inexistente devolve
// false sem erro — para a Recepção, "não existe" e "não pode ser marcado" são a mesma
// resposta.
func (l *LeitorDoente) DoenteActivo(ctx context.Context, doenteID string) (bool, error) {
	d, err := l.doentes.ObterPorID(ctx, doenteID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return false, nil
		}
		return false, err
	}
	return d.Estado() == clinico.EstadoActivo, nil
}

// Garantia de conformidade com a porta.
var _ apprecepcao.LeitorDoente = (*LeitorDoente)(nil)
