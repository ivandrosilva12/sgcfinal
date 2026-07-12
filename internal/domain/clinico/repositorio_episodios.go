package clinico

import (
	"context"
	"time"
)

// FiltroEpisodios parametriza a listagem de episódios de um doente.
type FiltroEpisodios struct {
	DoenteID     string
	Estado       string // filtro opcional por estado
	Limite       int
	Deslocamento int
}

// ResumoEpisodio é o read-model de um episódio numa listagem.
type ResumoEpisodio struct {
	ID              string     `json:"id"`
	Tipo            string     `json:"tipo"`
	EspecialidadeID string     `json:"especialidade_id"`
	MedicoID        string     `json:"medico_id"`
	Inicio          time.Time  `json:"inicio"`
	Fim             *time.Time `json:"fim,omitempty"`
	Estado          string     `json:"estado"`
}

// PaginaEpisodios é uma página de episódios.
type PaginaEpisodios struct {
	Itens        []ResumoEpisodio `json:"itens"`
	Total        int              `json:"total"`
	Limite       int              `json:"limite"`
	Deslocamento int              `json:"deslocamento"`
}

// RepositorioEpisodios é a porta de saída para persistência do agregado
// EpisodioClinico. A implementação vive em adapters/pgrepo.
type RepositorioEpisodios interface {
	Guardar(ctx context.Context, e *EpisodioClinico) (string, error)
	ObterPorID(ctx context.Context, id string) (*EpisodioClinico, error)
	ListarPorDoente(ctx context.Context, f FiltroEpisodios) (PaginaEpisodios, error)
}
