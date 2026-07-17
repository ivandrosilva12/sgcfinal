package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EventoEntregue é a forma persistida de um evento que o relay entrega aos
// handlers. O consumidor desserializa Payload no que precisar — fica desacoplado
// do struct do produtor.
type EventoEntregue struct {
	ID         int64
	Agregado   string
	TipoEvento string
	Payload    []byte
}

// Handler processa um evento entregue. Deve ser idempotente: a entrega é
// at-least-once (uma linha pode ser reprocessada após uma falha tardia).
type Handler func(ctx context.Context, ev EventoEntregue) error

// Observador recebe sinais de instrumentação do relay (métricas). Definido aqui
// (Camada 3) porque a regra de dependência proíbe o adaptador de importar o
// Prometheus da Plataforma; esta é implementada na Camada 4.
type Observador interface {
	Pendentes(n int)
	Publicado()
	FalhaHandler(tipoEvento string)
}

// ObservadorNulo é a implementação no-op (testes e omissão).
type ObservadorNulo struct{}

func (ObservadorNulo) Pendentes(int)       {}
func (ObservadorNulo) Publicado()          {}
func (ObservadorNulo) FalhaHandler(string) {}

// Relay faz poll da tabela shared.outbox e despacha os eventos pendentes aos
// handlers registados por tipo. Camada 3 — Adaptadores.
type Relay struct {
	pool     *pgxpool.Pool
	lote     int
	obs      Observador
	log      *slog.Logger
	handlers map[string][]Handler
}

// NovoRelay constrói o relay. lote é o máximo de eventos por passagem.
func NovoRelay(pool *pgxpool.Pool, lote int, obs Observador, log *slog.Logger) *Relay {
	if lote <= 0 {
		lote = 100
	}
	return &Relay{pool: pool, lote: lote, obs: obs, log: log, handlers: map[string][]Handler{}}
}

// Registar liga um handler a um tipo de evento (vários handlers por tipo).
func (r *Relay) Registar(tipoEvento string, h Handler) {
	r.handlers[tipoEvento] = append(r.handlers[tipoEvento], h)
}

// Despachar chama todos os handlers do tipo do evento. Sem handler é no-op. O
// primeiro erro interrompe e propaga (a linha não é marcada publicada).
func (r *Relay) Despachar(ctx context.Context, ev EventoEntregue) error {
	for _, h := range r.handlers[ev.TipoEvento] {
		if err := h(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

// ProcessarLote faz uma passagem: selecciona até `lote` eventos pendentes com
// FOR UPDATE SKIP LOCKED (uma linha-veneno não bloqueia as sãs), despacha cada um
// e marca publicado em sucesso; em falha incrementa tentativas e grava ultimo_erro.
// Devolve o número de linhas processadas com sucesso.
func (r *Relay) ProcessarLote(ctx context.Context) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	const sel = `SELECT id, agregado, tipo_evento, payload
		FROM shared.outbox WHERE publicado_em IS NULL
		ORDER BY id FOR UPDATE SKIP LOCKED LIMIT $1`
	rows, err := tx.Query(ctx, sel, r.lote)
	if err != nil {
		return 0, err
	}
	var lote []EventoEntregue
	for rows.Next() {
		var ev EventoEntregue
		if err := rows.Scan(&ev.ID, &ev.Agregado, &ev.TipoEvento, &ev.Payload); err != nil {
			rows.Close()
			return 0, err
		}
		lote = append(lote, ev)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	publicados := 0
	for _, ev := range lote {
		if err := r.Despachar(ctx, ev); err != nil {
			r.obs.FalhaHandler(ev.TipoEvento)
			r.log.Warn("falha ao entregar evento do outbox", "id", ev.ID,
				"tipo_evento", ev.TipoEvento, "erro", err)
			if _, e := tx.Exec(ctx, `UPDATE shared.outbox
				SET tentativas = tentativas + 1, ultimo_erro = $2 WHERE id = $1`,
				ev.ID, err.Error()); e != nil {
				return publicados, e
			}
			continue
		}
		if _, e := tx.Exec(ctx, `UPDATE shared.outbox SET publicado_em = now() WHERE id = $1`, ev.ID); e != nil {
			return publicados, e
		}
		r.obs.Publicado()
		publicados++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return publicados, nil
}

// Correr executa ProcessarLote em ciclo, no intervalo dado, até ctx ser
// cancelado (drena a passagem em curso antes de sair — shutdown gracioso).
func (r *Relay) Correr(ctx context.Context, intervalo time.Duration) {
	t := time.NewTicker(intervalo)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := r.ProcessarLote(ctx); err != nil && ctx.Err() == nil {
				r.log.Error("erro no ciclo do relay de outbox", "erro", err)
			}
		}
	}
}
