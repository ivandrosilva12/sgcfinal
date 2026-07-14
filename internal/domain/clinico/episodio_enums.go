package clinico

import (
	"strings"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// TipoEpisodio classifica um episódio clínico (DDM-001).
type TipoEpisodio string

const (
	EpisodioConsulta            TipoEpisodio = "CONSULTA"
	EpisodioUrgencia            TipoEpisodio = "URGENCIA"
	EpisodioInternamento        TipoEpisodio = "INTERNAMENTO"
	EpisodioCirurgiaAmbulatoria TipoEpisodio = "CIRURGIA_AMBULATORIA"
)

var tiposEpisodioValidos = map[TipoEpisodio]bool{
	EpisodioConsulta: true, EpisodioUrgencia: true, EpisodioInternamento: true,
	EpisodioCirurgiaAmbulatoria: true,
}

// ParseTipoEpisodio valida e normaliza um tipo de episódio (aceita minúsculas).
func ParseTipoEpisodio(codigo string) (TipoEpisodio, error) {
	t := TipoEpisodio(strings.ToUpper(strings.TrimSpace(codigo)))
	if !tiposEpisodioValidos[t] {
		return "", erros.Novo(erros.CategoriaValidacao, "tipo de episódio inválido (esperado CONSULTA, URGENCIA, INTERNAMENTO ou CIRURGIA_AMBULATORIA)")
	}
	return t, nil
}

// EstadoEpisodio é o estado do ciclo de vida de um episódio (DDM-001).
type EstadoEpisodio string

const (
	EstadoEpisodioAberto    EstadoEpisodio = "ABERTO"
	EstadoEpisodioFechado   EstadoEpisodio = "FECHADO"
	EstadoEpisodioCancelado EstadoEpisodio = "CANCELADO"
)
