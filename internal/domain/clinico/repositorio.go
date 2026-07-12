package clinico

import (
	"context"
	"time"
)

// FiltroDoentes parametriza a pesquisa de doentes.
type FiltroDoentes struct {
	Termo        string // nome (fuzzy), BI, num de processo ou telefone (exacto)
	Estado       string // filtro opcional por estado
	Limite       int    // máximo de resultados
	Deslocamento int    // paginação (offset)
}

// ResumoDoente é o read-model de um doente numa listagem/pesquisa.
type ResumoDoente struct {
	ID             string    `json:"id"`
	NumProcesso    string    `json:"num_processo"`
	NomeCompleto   string    `json:"nome_completo"`
	DataNascimento time.Time `json:"data_nascimento"`
	Sexo           string    `json:"sexo"`
	Telefone       string    `json:"telefone"`
	Estado         string    `json:"estado"`
}

// PaginaDoentes é uma página de resultados de pesquisa.
type PaginaDoentes struct {
	Itens        []ResumoDoente `json:"itens"`
	Total        int            `json:"total"`
	Limite       int            `json:"limite"`
	Deslocamento int            `json:"deslocamento"`
}

// RepositorioDoentes é a porta de saída para persistência do agregado Doente. A
// implementação vive em adapters/pgrepo.
type RepositorioDoentes interface {
	// Guardar persiste o doente (INSERT se id vazio, senão UPDATE) e devolve o id.
	// Conflitos de unicidade (num de processo ou BI) devolvem CategoriaConflito.
	Guardar(ctx context.Context, d *Doente) (string, error)
	// ObterPorID devolve o doente e as suas entidades-filho. NaoEncontrado se não existir.
	ObterPorID(ctx context.Context, id string) (*Doente, error)
	// ObterPorNumProcesso devolve o doente pelo número de processo. NaoEncontrado se não existir.
	ObterPorNumProcesso(ctx context.Context, num string) (*Doente, error)
	// Pesquisar devolve uma página de doentes segundo o filtro.
	Pesquisar(ctx context.Context, f FiltroDoentes) (PaginaDoentes, error)
	// ProximoNumeroProcesso reserva e devolve o próximo número automático do ano
	// indicado, no formato "P-{ano}-{sequencial:06d}".
	ProximoNumeroProcesso(ctx context.Context, ano int) (string, error)
}
