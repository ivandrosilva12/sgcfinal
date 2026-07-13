package clinico

import "context"

// CatalogoProcedimento é a projecção de leitura de uma entrada do catálogo de
// procedimentos cirúrgicos (dados de referência).
type CatalogoProcedimento struct {
	Codigo             string `json:"codigo"`
	Descricao          string `json:"descricao"`
	Especialidade      string `json:"especialidade,omitempty"`
	DuracaoEstimadaMin int    `json:"duracao_estimada_min,omitempty"`
	RequerAnestesista  bool   `json:"requer_anestesista"`
	Activo             bool   `json:"activo"`
}

// RepositorioCatalogoProcedimentos é a porta de leitura do catálogo.
type RepositorioCatalogoProcedimentos interface {
	ObterPorCodigo(ctx context.Context, codigo string) (*CatalogoProcedimento, error)
}
