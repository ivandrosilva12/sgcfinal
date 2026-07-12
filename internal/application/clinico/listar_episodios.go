package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

// CasoListarEpisodios lista os episódios de um doente, com paginação.
type CasoListarEpisodios struct {
	episodios dominio.RepositorioEpisodios
}

// NovoCasoListarEpisodios constrói o caso de uso.
func NovoCasoListarEpisodios(ep dominio.RepositorioEpisodios) *CasoListarEpisodios {
	return &CasoListarEpisodios{episodios: ep}
}

// Executar normaliza os limites e delega a listagem ao repositório.
func (c *CasoListarEpisodios) Executar(ctx context.Context, doenteID string, filtro FiltroEpisodios) (PaginaEpisodios, error) {
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
	return c.episodios.ListarPorDoente(ctx, filtro)
}
