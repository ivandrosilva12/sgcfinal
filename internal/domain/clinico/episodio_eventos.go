package clinico

import (
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

// EpisodioAberto é emitido quando um episódio é iniciado.
type EpisodioAberto struct {
	EpisodioID string
	DoenteID   string
	Em         time.Time
}

func (e EpisodioAberto) NomeEvento() string    { return "clinico.episodio.aberto" }
func (e EpisodioAberto) OcorridoEm() time.Time { return e.Em }

// EpisodioFechado é emitido quando um episódio é fechado.
type EpisodioFechado struct {
	EpisodioID string
	DoenteID   string
	Em         time.Time
}

func (e EpisodioFechado) NomeEvento() string    { return "clinico.episodio.fechado" }
func (e EpisodioFechado) OcorridoEm() time.Time { return e.Em }

// EpisodioCancelado é emitido quando um episódio é cancelado.
type EpisodioCancelado struct {
	EpisodioID string
	DoenteID   string
	Em         time.Time
}

func (e EpisodioCancelado) NomeEvento() string    { return "clinico.episodio.cancelado" }
func (e EpisodioCancelado) OcorridoEm() time.Time { return e.Em }

// Garantias de conformidade com a interface de evento de domínio.
var (
	_ evento.EventoDominio = EpisodioAberto{}
	_ evento.EventoDominio = EpisodioFechado{}
	_ evento.EventoDominio = EpisodioCancelado{}
)
