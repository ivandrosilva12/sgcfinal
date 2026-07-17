package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
)

func relayVazio() *outbox.Relay {
	return outbox.NovoRelay(nil, 100, outbox.ObservadorNulo{},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestDespachar_ChamaHandlersDoTipo(t *testing.T) {
	r := relayVazio()
	var visto string
	r.Registar("clinico.episodio.fechado", func(_ context.Context, ev outbox.EventoEntregue) error {
		visto = string(ev.Payload)
		return nil
	})
	err := r.Despachar(context.Background(), outbox.EventoEntregue{
		TipoEvento: "clinico.episodio.fechado", Payload: []byte(`{"x":1}`)})
	if err != nil {
		t.Fatalf("despachar: %v", err)
	}
	if visto != `{"x":1}` {
		t.Fatalf("handler não recebeu o payload, obtive %q", visto)
	}
}

func TestDespachar_SemHandler_NaoErra(t *testing.T) {
	r := relayVazio()
	if err := r.Despachar(context.Background(), outbox.EventoEntregue{TipoEvento: "n.a"}); err != nil {
		t.Fatalf("sem handler devia ser no-op sem erro, obtive %v", err)
	}
}

func TestDespachar_HandlerFalha_Propaga(t *testing.T) {
	r := relayVazio()
	r.Registar("t", func(context.Context, outbox.EventoEntregue) error { return errors.New("falhou") })
	if err := r.Despachar(context.Background(), outbox.EventoEntregue{TipoEvento: "t"}); err == nil {
		t.Fatalf("erro do handler devia propagar")
	}
}
