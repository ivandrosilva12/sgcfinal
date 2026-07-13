package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

// CasoListarProcedimentos lista os procedimentos de um episódio (não audita).
type CasoListarProcedimentos struct {
	procedimentos dominio.RepositorioProcedimentos
}

// NovoCasoListarProcedimentos constrói o caso de uso.
func NovoCasoListarProcedimentos(p dominio.RepositorioProcedimentos) *CasoListarProcedimentos {
	return &CasoListarProcedimentos{procedimentos: p}
}

// Executar devolve os procedimentos do episódio.
func (uc *CasoListarProcedimentos) Executar(ctx context.Context, episodioID string) ([]ResumoProcedimento, error) {
	return uc.procedimentos.ListarPorEpisodio(ctx, episodioID)
}
