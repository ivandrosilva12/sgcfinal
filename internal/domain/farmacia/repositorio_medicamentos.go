package farmacia

import "context"

// FiltroMedicamentos parametriza a pesquisa no catálogo.
type FiltroMedicamentos struct {
	Termo         string
	ApenasActivos bool
	Limite        int
	Deslocamento  int
}

// ResumoMedicamento é o read-model de um medicamento numa listagem.
type ResumoMedicamento struct {
	ID                string `json:"id"`
	CodigoInterno     string `json:"codigo_interno"`
	NomeComercial     string `json:"nome_comercial"`
	NomeGenerico      string `json:"nome_generico"`
	FormaFarmaceutica string `json:"forma_farmaceutica"`
	Dosagem           string `json:"dosagem"`
	Activo            bool   `json:"activo"`
}

// PaginaMedicamentos é uma página de resultados do catálogo.
type PaginaMedicamentos struct {
	Itens        []ResumoMedicamento `json:"itens"`
	Total        int                 `json:"total"`
	Limite       int                 `json:"limite"`
	Deslocamento int                 `json:"deslocamento"`
}

// RepositorioMedicamentos é a porta de saída do catálogo. Implementada em pgrepo.
type RepositorioMedicamentos interface {
	Guardar(ctx context.Context, m *Medicamento) (string, error)
	ObterPorID(ctx context.Context, id string) (*Medicamento, error)
	Pesquisar(ctx context.Context, f FiltroMedicamentos) (PaginaMedicamentos, error)
	ProximoCodigo(ctx context.Context) (string, error) // "MED-00001"
}
