package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRevogarConsentimento revoga um consentimento e audita.
type CasoRevogarConsentimento struct {
	consentimentos dominio.RepositorioConsentimentos
	auditor        Auditor
	agora          func() time.Time
}

// NovoCasoRevogarConsentimento constrói o caso de uso.
func NovoCasoRevogarConsentimento(c dominio.RepositorioConsentimentos, a Auditor) *CasoRevogarConsentimento {
	return &CasoRevogarConsentimento{consentimentos: c, auditor: a, agora: time.Now}
}

// Executar carrega, revoga, persiste e audita.
func (uc *CasoRevogarConsentimento) Executar(ctx context.Context, actor, id string) (DetalheConsentimento, error) {
	c, err := uc.consentimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	if err := c.Revogar(uc.agora()); err != nil {
		return DetalheConsentimento{}, err
	}
	if _, err := uc.consentimentos.Guardar(ctx, c); err != nil {
		return DetalheConsentimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.consentimento.revogado",
		Entidade: "consentimento", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheConsentimento{}, err
	}
	final, err := uc.consentimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	return paraDetalheConsentimento(final), nil
}
