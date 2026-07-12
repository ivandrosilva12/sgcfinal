package identidade

import (
	"context"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoListarSessoes lista as sessões activas de um utilizador (leitura).
type CasoListarSessoes struct{ admin AdminIdentidade }

// NovoCasoListarSessoes constrói o caso de uso.
func NovoCasoListarSessoes(a AdminIdentidade) *CasoListarSessoes {
	return &CasoListarSessoes{admin: a}
}

// Executar devolve as sessões activas do utilizador indicado.
func (c *CasoListarSessoes) Executar(ctx context.Context, userID string) ([]SessaoActiva, error) {
	return c.admin.ListarSessoes(ctx, userID)
}

// CasoRevogarSessao revoga uma sessão específica e audita a acção.
type CasoRevogarSessao struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoRevogarSessao constrói o caso de uso.
func NovoCasoRevogarSessao(a AdminIdentidade, aud Auditor) *CasoRevogarSessao {
	return &CasoRevogarSessao{admin: a, auditor: aud, agora: time.Now}
}

// Executar revoga a sessão no Keycloak e regista a auditoria.
func (c *CasoRevogarSessao) Executar(ctx context.Context, actor, sessionID string) error {
	if err := c.admin.RevogarSessao(ctx, sessionID); err != nil {
		return err
	}
	return c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.sessao.revogada",
		Entidade:   "sessao",
		EntidadeID: sessionID,
		OcorridoEm: c.agora(),
	})
}
