// internal/application/recepcao/triagens.go
package recepcao

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
)

// CasoObterTriagem lê a triagem de uma chegada.
type CasoObterTriagem struct {
	triagens dominio.RepositorioTriagens
}

// NovoCasoObterTriagem constrói o caso de uso.
func NovoCasoObterTriagem(t dominio.RepositorioTriagens) *CasoObterTriagem {
	return &CasoObterTriagem{triagens: t}
}

// Executar devolve o detalhe da triagem de uma chegada.
func (uc *CasoObterTriagem) Executar(ctx context.Context, chegadaID string) (DetalheTriagem, error) {
	t, err := uc.triagens.ObterPorChegada(ctx, chegadaID)
	if err != nil {
		return DetalheTriagem{}, err
	}
	return paraDetalheTriagem(t), nil
}

// CasoListarFilaClinica lê a fila clínica (chegadas TRIADO por prioridade).
type CasoListarFilaClinica struct {
	triagens dominio.RepositorioTriagens
}

// NovoCasoListarFilaClinica constrói o caso de uso.
func NovoCasoListarFilaClinica(t dominio.RepositorioTriagens) *CasoListarFilaClinica {
	return &CasoListarFilaClinica{triagens: t}
}

// Executar devolve a fila clínica; médico vazio = todos.
func (uc *CasoListarFilaClinica) Executar(ctx context.Context, medicoID string) ([]ResumoFilaClinica, error) {
	return uc.triagens.ListarFilaClinica(ctx, medicoID)
}
