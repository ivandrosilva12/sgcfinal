package farmacia

import (
	"context"
	"time"
)

// FiltroReceitas parametriza a listagem de receitas de um doente.
type FiltroReceitas struct {
	DoenteID     string
	EpisodioID   string // filtro opcional
	Estado       string // filtro opcional
	Limite       int
	Deslocamento int
}

// ResumoReceita é o read-model de uma receita numa listagem.
type ResumoReceita struct {
	ID         string    `json:"id"`
	EpisodioID string    `json:"episodio_id"`
	MedicoID   string    `json:"medico_id"`
	EmitidaEm  time.Time `json:"emitida_em"`
	Estado     string    `json:"estado"`
	ExpiraEm   time.Time `json:"expira_em"`
	NumItens   int       `json:"num_itens"`
}

// PaginaReceitas é uma página de receitas.
type PaginaReceitas struct {
	Itens        []ResumoReceita `json:"itens"`
	Total        int             `json:"total"`
	Limite       int             `json:"limite"`
	Deslocamento int             `json:"deslocamento"`
}

// RepositorioReceitas é a porta de saída das receitas. Implementada em pgrepo.
type RepositorioReceitas interface {
	Guardar(ctx context.Context, r *Receita) (string, error)
	ObterPorID(ctx context.Context, id string) (*Receita, error)
	ListarPorDoente(ctx context.Context, f FiltroReceitas) (PaginaReceitas, error)
}
