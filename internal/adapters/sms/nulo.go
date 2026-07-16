package sms

import (
	"context"
	"log/slog"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
)

// NotificadorNulo é usado quando o SMS não está configurado: não envia nada, apenas
// regista em debug. Garante que a validação nunca falha por ausência de gateway SMS.
type NotificadorNulo struct{ log *slog.Logger }

// NovoNotificadorNulo constrói o notificador no-op (log opcional).
func NovoNotificadorNulo(log *slog.Logger) NotificadorNulo {
	return NotificadorNulo{log: log}
}

// NotificarValorCritico não envia; regista em debug.
func (n NotificadorNulo) NotificarValorCritico(_ context.Context, telefone, codigoAnalise, valor string) error {
	if n.log != nil {
		n.log.Debug("alerta de valor crítico suprimido (SMS não configurado)",
			"telefone", telefone, "analise", codigoAnalise, "valor", valor)
	}
	return nil
}

var _ applaboratorio.NotificadorCritico = NotificadorNulo{}
