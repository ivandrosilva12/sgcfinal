package identidade

import (
	"context"
	"log/slog"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoResetPassword repõe a password de um utilizador com uma nova senha
// temporária gerada, delegando no Keycloak e auditando.
type CasoResetPassword struct {
	admin   AdminIdentidade
	auditor Auditor
	notif   Notificador
	agora   func() time.Time
}

// NovoCasoResetPassword constrói o caso de uso.
func NovoCasoResetPassword(a AdminIdentidade, aud Auditor, notif Notificador) *CasoResetPassword {
	return &CasoResetPassword{admin: a, auditor: aud, notif: notif, agora: time.Now}
}

// Executar gera uma nova senha temporária, define-a no Keycloak, audita e
// devolve-a (uma única vez).
func (c *CasoResetPassword) Executar(ctx context.Context, actor, id string) (CredencialReposta, error) {
	senha, err := gerarSenhaTemporaria()
	if err != nil {
		return CredencialReposta{}, err
	}
	if err := c.admin.DefinirPasswordTemporaria(ctx, id, senha); err != nil {
		return CredencialReposta{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.password.reposta",
		Entidade:   "utilizador",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	}); err != nil {
		return CredencialReposta{}, err
	}
	// Notificação best-effort: obtém o email do alvo e envia; nada disto falha o reset.
	if det, err := c.admin.ObterUtilizador(ctx, id); err != nil {
		slog.Warn("não obteve o email do utilizador para notificar o reset", "utilizador", id, "erro", err)
	} else if err := c.notif.NotificarResetPassword(ctx, det.Email, det.Nome, senha); err != nil {
		slog.Warn("falha ao notificar reset por email", "utilizador", id, "erro", err)
	}

	return CredencialReposta{SenhaTemporaria: senha}, nil
}

// CasoResetOTP remove o 2º factor de um utilizador (re-inscrição forçada) e audita.
type CasoResetOTP struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoResetOTP constrói o caso de uso.
func NovoCasoResetOTP(a AdminIdentidade, aud Auditor) *CasoResetOTP {
	return &CasoResetOTP{admin: a, auditor: aud, agora: time.Now}
}

// Executar delega a reposição de OTP no Keycloak e regista a auditoria.
func (c *CasoResetOTP) Executar(ctx context.Context, actor, id string) error {
	if err := c.admin.ResetOTP(ctx, id); err != nil {
		return err
	}
	return c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.otp.reposto",
		Entidade:   "utilizador",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	})
}
