package pgrepo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

// inserirEventos persiste os eventos de domínio pendentes de um agregado na
// tabela shared.outbox, dentro da transacção fornecida (mesma tx da mudança de
// estado — garantia atómica do padrão Outbox). Sem eventos, é um no-op.
func inserirEventos(ctx context.Context, tx pgx.Tx, eventos []evento.EventoDominio) error {
	for _, e := range eventos {
		agregado, payload, err := outbox.Codificar(e)
		if err != nil {
			return err
		}
		const q = `INSERT INTO shared.outbox (agregado, tipo_evento, payload)
			VALUES ($1, $2, $3)`
		if _, err := tx.Exec(ctx, q, agregado, e.NomeEvento(), payload); err != nil {
			return fmt.Errorf("inserir evento %s no outbox: %w", e.NomeEvento(), err)
		}
	}
	return nil
}
