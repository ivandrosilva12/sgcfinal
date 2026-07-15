// internal/domain/recepcao/prioridade.go

package recepcao

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// PrioridadeManchester é a classificação de prioridade da triagem pelo Sistema de
// Triagem de Manchester (5 cores, cada uma com um tempo-alvo máximo de espera).
type PrioridadeManchester string

const (
	ManVermelho PrioridadeManchester = "VERMELHO" // Emergente     — 0 min
	ManLaranja  PrioridadeManchester = "LARANJA"  // Muito urgente — 10 min
	ManAmarelo  PrioridadeManchester = "AMARELO"  // Urgente       — 60 min
	ManVerde    PrioridadeManchester = "VERDE"    // Pouco urgente — 120 min
	ManAzul     PrioridadeManchester = "AZUL"     // Não urgente   — 240 min
)

// atributosManchester guarda a severidade (1 = mais urgente) e o tempo-alvo de cada cor.
var atributosManchester = map[PrioridadeManchester]struct {
	severidade int
	tempoAlvo  time.Duration
}{
	ManVermelho: {1, 0},
	ManLaranja:  {2, 10 * time.Minute},
	ManAmarelo:  {3, 60 * time.Minute},
	ManVerde:    {4, 120 * time.Minute},
	ManAzul:     {5, 240 * time.Minute},
}

// ParsePrioridade valida e normaliza uma cor de Manchester (aceita minúsculas e espaços).
func ParsePrioridade(codigo string) (PrioridadeManchester, error) {
	p := PrioridadeManchester(strings.ToUpper(strings.TrimSpace(codigo)))
	if _, ok := atributosManchester[p]; !ok {
		return "", erros.Novo(erros.CategoriaValidacao,
			"prioridade de triagem inválida (esperado VERMELHO, LARANJA, AMARELO, VERDE ou AZUL)")
	}
	return p, nil
}

// Severidade devolve a ordem de urgência: 1 (VERMELHO, mais urgente) a 5 (AZUL). Usada
// para ordenar a fila clínica. Uma cor desconhecida devolve 99 (fica no fim).
func (p PrioridadeManchester) Severidade() int {
	if a, ok := atributosManchester[p]; ok {
		return a.severidade
	}
	return 99
}

// TempoAlvo devolve o tempo máximo de espera recomendado para a cor.
func (p PrioridadeManchester) TempoAlvo() time.Duration {
	return atributosManchester[p].tempoAlvo
}
