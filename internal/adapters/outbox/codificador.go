// Package outbox implementa o padrão Outbox: a codificação de eventos de domínio
// para persistência e o relay de publicação assíncrona inter-bounded-context.
// Camada 3 — Adaptadores.
package outbox

import (
	"encoding/json"
	"fmt"

	domclinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

// Codificar traduz um evento de domínio numa linha de Outbox: o nome do agregado
// de origem e o payload JSON. O tipo do evento persistido é e.NomeEvento(). Só os
// tipos explicitamente mapeados são aceites — um evento novo tem de ser registado
// aqui (falha explícita em vez de publicação de um payload não contratado).
func Codificar(e evento.EventoDominio) (agregado string, payload []byte, err error) {
	switch e.(type) {
	case domclinico.EpisodioFechado:
		agregado = "episodio"
	default:
		return "", nil, fmt.Errorf("outbox: evento não mapeado para publicação: %s (%T)", e.NomeEvento(), e)
	}
	payload, err = json.Marshal(e)
	if err != nil {
		return "", nil, fmt.Errorf("outbox: serializar evento %s: %w", e.NomeEvento(), err)
	}
	return agregado, payload, nil
}
