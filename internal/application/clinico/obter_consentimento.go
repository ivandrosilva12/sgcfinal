package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

// CasoObterConsentimento devolve o detalhe de um consentimento (não audita — leitura).
type CasoObterConsentimento struct {
	consentimentos dominio.RepositorioConsentimentos
}

// NovoCasoObterConsentimento constrói o caso de uso.
func NovoCasoObterConsentimento(c dominio.RepositorioConsentimentos) *CasoObterConsentimento {
	return &CasoObterConsentimento{consentimentos: c}
}

// Executar carrega e projecta o consentimento.
func (uc *CasoObterConsentimento) Executar(ctx context.Context, id string) (DetalheConsentimento, error) {
	c, err := uc.consentimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	return paraDetalheConsentimento(c), nil
}
