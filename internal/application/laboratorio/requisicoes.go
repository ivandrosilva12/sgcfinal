package laboratorio

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
)

// CasoObterRequisicao devolve o detalhe de uma requisição.
type CasoObterRequisicao struct {
	requisicoes dominio.RepositorioRequisicoes
}

// NovoCasoObterRequisicao constrói o caso de uso.
func NovoCasoObterRequisicao(r dominio.RepositorioRequisicoes) *CasoObterRequisicao {
	return &CasoObterRequisicao{requisicoes: r}
}

// Executar devolve a requisição ou NaoEncontrado.
func (uc *CasoObterRequisicao) Executar(ctx context.Context, id string) (DetalheRequisicao, error) {
	r, err := uc.requisicoes.ObterPorID(ctx, id)
	if err != nil {
		return DetalheRequisicao{}, err
	}
	return paraDetalheRequisicao(r), nil
}

// CasoListarRequisicoesDoEpisodio lista as requisições de um episódio.
type CasoListarRequisicoesDoEpisodio struct {
	requisicoes dominio.RepositorioRequisicoes
}

// NovoCasoListarRequisicoesDoEpisodio constrói o caso de uso.
func NovoCasoListarRequisicoesDoEpisodio(r dominio.RepositorioRequisicoes) *CasoListarRequisicoesDoEpisodio {
	return &CasoListarRequisicoesDoEpisodio{requisicoes: r}
}

// Executar devolve as requisições do episódio.
func (uc *CasoListarRequisicoesDoEpisodio) Executar(ctx context.Context, episodioID string) ([]ResumoRequisicao, error) {
	return uc.requisicoes.ListarPorEpisodio(ctx, episodioID)
}
