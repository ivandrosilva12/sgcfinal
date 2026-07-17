//go:build integration

package integration_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
)

func TestRelay_ProcessarLote_MarcaPublicado(t *testing.T) {
	pool, ctx := ligar(t)
	// limpa e semeia duas linhas pendentes
	if _, err := pool.Exec(ctx, `DELETE FROM shared.outbox`); err != nil {
		t.Fatalf("limpar outbox: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO shared.outbox (agregado, tipo_evento, payload)
			VALUES ('teste','t.evento','{}'::jsonb)`); err != nil {
			t.Fatalf("semear: %v", err)
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := outbox.NovoRelay(pool, 100, outbox.ObservadorNulo{}, log)
	var vistos int
	r.Registar("t.evento", func(context.Context, outbox.EventoEntregue) error { vistos++; return nil })

	n, err := r.ProcessarLote(ctx)
	if err != nil {
		t.Fatalf("processar: %v", err)
	}
	if n != 2 || vistos != 2 {
		t.Fatalf("esperava 2 processados e 2 vistos, obtive n=%d vistos=%d", n, vistos)
	}
	// segunda passagem: nada pendente
	n2, _ := r.ProcessarLote(ctx)
	if n2 != 0 {
		t.Fatalf("segunda passagem devia processar 0, obtive %d", n2)
	}
}
