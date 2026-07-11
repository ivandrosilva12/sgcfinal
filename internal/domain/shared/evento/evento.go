// Package evento define o contrato de eventos de domínio do Shared Kernel.
// Os eventos são publicados via Outbox (assíncrono) para comunicação
// inter-bounded-context. Camada 1 (Domínio) — sem infra.
package evento

import "time"

// EventoDominio é a interface comum a todos os eventos de domínio.
type EventoDominio interface {
	// NomeEvento devolve o identificador estável do tipo de evento
	// (ex.: "identidade.utilizador.autenticado").
	NomeEvento() string
	// OcorridoEm devolve o instante em que o evento ocorreu.
	OcorridoEm() time.Time
}
