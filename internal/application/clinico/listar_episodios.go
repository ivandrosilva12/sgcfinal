package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

// CasoListarEpisodios lista os episódios de um doente, com paginação.
type CasoListarEpisodios struct {
	episodios dominio.RepositorioEpisodios
	triagem   LeitorTriagem
}

// NovoCasoListarEpisodios constrói o caso de uso.
func NovoCasoListarEpisodios(ep dominio.RepositorioEpisodios, triagem LeitorTriagem) *CasoListarEpisodios {
	return &CasoListarEpisodios{episodios: ep, triagem: triagem}
}

// Executar normaliza os limites, delega a listagem ao repositório e, se o
// actor tiver papel autorizado, preenche a prioridade de triagem em lote
// (minimização LPDP, ADR-034/ADR-037).
func (c *CasoListarEpisodios) Executar(ctx context.Context, doenteID string, papeis []string, filtro FiltroEpisodios) (PaginaEpisodios, error) {
	filtro.DoenteID = doenteID
	if filtro.Limite <= 0 {
		filtro.Limite = limiteDefault
	}
	if filtro.Limite > limiteMaximo {
		filtro.Limite = limiteMaximo
	}
	if filtro.Deslocamento < 0 {
		filtro.Deslocamento = 0
	}
	pagina, err := c.episodios.ListarPorDoente(ctx, filtro)
	if err != nil {
		return PaginaEpisodios{}, err
	}
	if err := preencherPrioridadesTriagem(ctx, c.triagem, papeis, pagina.Itens); err != nil {
		return PaginaEpisodios{}, err
	}
	return pagina, nil
}
