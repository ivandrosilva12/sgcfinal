package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

// CasoListarConsentimentos lista os consentimentos de um doente (não audita — leitura).
type CasoListarConsentimentos struct {
	consentimentos dominio.RepositorioConsentimentos
}

// NovoCasoListarConsentimentos constrói o caso de uso.
func NovoCasoListarConsentimentos(c dominio.RepositorioConsentimentos) *CasoListarConsentimentos {
	return &CasoListarConsentimentos{consentimentos: c}
}

// Executar devolve os consentimentos do doente segundo o filtro.
func (uc *CasoListarConsentimentos) Executar(ctx context.Context, doenteID string, filtro FiltroConsentimentos) ([]ResumoConsentimento, error) {
	return uc.consentimentos.ListarPorDoente(ctx, doenteID, filtro)
}
