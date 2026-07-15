// internal/domain/recepcao/disponibilidade.go
package recepcao

import (
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// VerificarDisponibilidade é a invariante de negócio central da marcação. É uma função
// pura (sem I/O): o caso de uso alimenta-a com as janelas e as marcações activas lidas
// dos repositórios. Assume-se que `janelas` e `activas` são do mesmo médico da proposta.
//
// Verifica, por esta ordem:
//  1. a proposta não está no passado (início >= agora);
//  2. a proposta cabe inteira dentro de uma janela da mesma especialidade;
//  3. a proposta não sobrepõe nenhuma marcação activa (MARCADA) do médico.
//
// Encosto exacto (fim de uma == início da outra) NÃO é sobreposição.
func VerificarDisponibilidade(janelas []JanelaDisponibilidade, activas []Marcacao, especialidadeID string, inicio, fim, agora time.Time) error {
	if inicio.Before(agora) {
		return erros.Novo(erros.CategoriaRegraNegocio, "não é possível marcar no passado")
	}
	if !cabeNumaJanela(janelas, especialidadeID, inicio, fim) {
		return erros.Novo(erros.CategoriaRegraNegocio,
			"não há disponibilidade do médico para essa especialidade e horário")
	}
	for i := range activas {
		if activas[i].estado == MarcMarcada && seSobrepoe(inicio, fim, activas[i].inicio, activas[i].fim) {
			return erros.Novo(erros.CategoriaConflito,
				"o horário sobrepõe outra marcação do médico")
		}
	}
	return nil
}

// cabeNumaJanela indica se [inicio,fim] está inteiramente contido numa janela da
// especialidade dada.
func cabeNumaJanela(janelas []JanelaDisponibilidade, especialidadeID string, inicio, fim time.Time) bool {
	for i := range janelas {
		j := janelas[i]
		if j.especialidadeID != especialidadeID {
			continue
		}
		if !inicio.Before(j.inicio) && !fim.After(j.fim) {
			return true
		}
	}
	return false
}

// seSobrepoe indica se dois intervalos [aDe,aAte] e [bDe,bAte] se sobrepõem. Encosto
// exacto (aAte == bDe ou bAte == aDe) não conta como sobreposição.
func seSobrepoe(aDe, aAte, bDe, bAte time.Time) bool {
	return aDe.Before(bAte) && bDe.Before(aAte)
}
