package farmacia

import (
	"context"
	"time"
)

// FiltroFornecedores parametriza a listagem de fornecedores.
type FiltroFornecedores struct {
	Termo         string
	ApenasActivos bool
	Limite        int
	Deslocamento  int
}

// ResumoFornecedor é o read-model de um fornecedor numa listagem.
type ResumoFornecedor struct {
	ID     string  `json:"id"`
	Nome   string  `json:"nome"`
	NIF    *string `json:"nif,omitempty"`
	Activo bool    `json:"activo"`
}

// PaginaFornecedores é uma página de fornecedores.
type PaginaFornecedores struct {
	Itens        []ResumoFornecedor `json:"itens"`
	Total        int                `json:"total"`
	Limite       int                `json:"limite"`
	Deslocamento int                `json:"deslocamento"`
}

// RepositorioFornecedores é a porta de saída dos fornecedores.
type RepositorioFornecedores interface {
	Guardar(ctx context.Context, f *Fornecedor) (string, error)
	ObterPorID(ctx context.Context, id string) (*Fornecedor, error)
	Listar(ctx context.Context, filtro FiltroFornecedores) (PaginaFornecedores, error)
}

// ResumoLote é o read-model de um lote numa listagem.
type ResumoLote struct {
	ID               string    `json:"id"`
	NumeroLote       string    `json:"numero_lote"`
	Validade         time.Time `json:"validade"`
	QuantidadeActual int       `json:"quantidade_actual"`
	FornecedorID     *string   `json:"fornecedor_id,omitempty"`
}

// RepositorioLotes é a porta de saída dos lotes de stock.
type RepositorioLotes interface {
	// RegistarEntrada persiste o lote e o movimento ENTRADA, atomicamente.
	RegistarEntrada(ctx context.Context, l *Lote, realizadoPor string) (id string, err error)
	ObterPorID(ctx context.Context, id string) (*Lote, error)
	ListarPorMedicamento(ctx context.Context, medicamentoID string, apenasDisponiveis bool) ([]ResumoLote, error)
	StockDisponivel(ctx context.Context, medicamentoID string) (int, error)
}
