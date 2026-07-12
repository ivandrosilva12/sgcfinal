package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

const (
	limiteDefault = 20
	limiteMaximo  = 100
)

// CasoPesquisarDoentes pesquisa doentes por nome (fuzzy), BI, número de processo
// ou telefone, com paginação.
type CasoPesquisarDoentes struct {
	repo dominio.RepositorioDoentes
}

// NovoCasoPesquisarDoentes constrói o caso de uso.
func NovoCasoPesquisarDoentes(repo dominio.RepositorioDoentes) *CasoPesquisarDoentes {
	return &CasoPesquisarDoentes{repo: repo}
}

// Executar normaliza os limites e delega a pesquisa ao repositório.
func (c *CasoPesquisarDoentes) Executar(ctx context.Context, filtro FiltroDoentes) (PaginaDoentes, error) {
	if filtro.Limite <= 0 {
		filtro.Limite = limiteDefault
	}
	if filtro.Limite > limiteMaximo {
		filtro.Limite = limiteMaximo
	}
	if filtro.Deslocamento < 0 {
		filtro.Deslocamento = 0
	}
	return c.repo.Pesquisar(ctx, filtro)
}
