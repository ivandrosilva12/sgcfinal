package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

// CasoObterProcedimento devolve o detalhe de um procedimento (não audita).
type CasoObterProcedimento struct {
	procedimentos dominio.RepositorioProcedimentos
}

// NovoCasoObterProcedimento constrói o caso de uso.
func NovoCasoObterProcedimento(p dominio.RepositorioProcedimentos) *CasoObterProcedimento {
	return &CasoObterProcedimento{procedimentos: p}
}

// Executar carrega e projecta o procedimento.
func (uc *CasoObterProcedimento) Executar(ctx context.Context, id string) (DetalheProcedimento, error) {
	p, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	return paraDetalheProcedimento(p), nil
}
