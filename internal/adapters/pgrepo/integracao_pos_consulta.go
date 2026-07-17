// internal/adapters/pgrepo/integracao_pos_consulta.go
//
// Consumidor do evento clinico.episodio.fechado (ADR-038): transita a chegada da
// Recepção que originou o episódio para ATENDIDO. Adaptador de integração — o
// único ponto que conhece a ponte episodio_id entre o Clínico e a Recepção. A
// entrega é at-least-once, por isso a operação é idempotente (CAS por estado).
package pgrepo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
)

// IntegracaoPosConsulta implementa o desfecho pós-consulta da chegada.
type IntegracaoPosConsulta struct {
	pool *pgxpool.Pool
}

// NovaIntegracaoPosConsulta constrói o adaptador sobre o pool pgx.
func NovaIntegracaoPosConsulta(pool *pgxpool.Pool) *IntegracaoPosConsulta {
	return &IntegracaoPosConsulta{pool: pool}
}

// MarcarChegadaAtendida transita a chegada EM_CONSULTA→ATENDIDO pela ponte
// episodio_id. Idempotente: 0 linhas afectadas (já ATENDIDO por reentrega, ou
// episódio sem chegada associada) é sucesso/no-op. A guarda de estado no WHERE é
// o CAS que fecha corridas — espelha a máquina de estados do domínio (Atender).
func (a *IntegracaoPosConsulta) MarcarChegadaAtendida(ctx context.Context, episodioID string) error {
	const q = `UPDATE recepcao.chegadas
		SET estado = 'ATENDIDO', actualizado_em = now()
		WHERE episodio_id = $1 AND estado = 'EM_CONSULTA'`
	if _, err := a.pool.Exec(ctx, q, episodioID); err != nil {
		return fmt.Errorf("marcar chegada atendida (episódio %s): %w", episodioID, err)
	}
	return nil
}

// HandlerEpisodioFechado é o Handler de relay para clinico.episodio.fechado.
func (a *IntegracaoPosConsulta) HandlerEpisodioFechado(ctx context.Context, ev outbox.EventoEntregue) error {
	var p struct {
		EpisodioID string `json:"EpisodioID"`
	}
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return fmt.Errorf("payload de episodio.fechado inválido: %w", err)
	}
	if p.EpisodioID == "" {
		return fmt.Errorf("payload de episodio.fechado sem EpisodioID")
	}
	return a.MarcarChegadaAtendida(ctx, p.EpisodioID)
}
