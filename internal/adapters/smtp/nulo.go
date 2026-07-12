package smtp

import (
	"context"
	"log/slog"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
)

// NotificadorNulo é usado quando o SMTP não está configurado: não envia nada,
// apenas regista em nível debug. Garante que operações não falham por ausência
// de infra de email.
type NotificadorNulo struct{ log *slog.Logger }

// NovoNotificadorNulo constrói o notificador no-op (log opcional).
func NovoNotificadorNulo(log *slog.Logger) NotificadorNulo {
	return NotificadorNulo{log: log}
}

// NotificarCriacao não envia; regista em debug (sem a senha).
func (n NotificadorNulo) NotificarCriacao(_ context.Context, email, _, _ string) error {
	if n.log != nil {
		n.log.Debug("notificação de criação suprimida (SMTP não configurado)", "email", email)
	}
	return nil
}

// NotificarResetPassword não envia; regista em debug (sem a senha).
func (n NotificadorNulo) NotificarResetPassword(_ context.Context, email, _, _ string) error {
	if n.log != nil {
		n.log.Debug("notificação de reset suprimida (SMTP não configurado)", "email", email)
	}
	return nil
}

// Garantia de conformidade com a porta.
var _ appident.Notificador = NotificadorNulo{}
