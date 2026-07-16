package sms_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/sms"
)

func TestNotificadorNulo_DevolveNil(t *testing.T) {
	n := sms.NovoNotificadorNulo(nil)
	if err := n.NotificarValorCritico(context.Background(), "+244923000000", "HB", "2.5"); err != nil {
		t.Fatalf("notificador nulo devia devolver nil, obtive %v", err)
	}
}

func TestNotificadorNulo_RegistaEmDebugQuandoHaLogger(t *testing.T) {
	log := slog.Default()
	n := sms.NovoNotificadorNulo(log)
	if err := n.NotificarValorCritico(context.Background(), "+244923000000", "HB", "2.5"); err != nil {
		t.Fatalf("notificador nulo devia devolver nil, obtive %v", err)
	}
}
