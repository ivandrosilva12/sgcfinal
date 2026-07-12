// Package farmacia contém os casos de uso do BC Farmácia (Camada 2 — Aplicação).
package farmacia

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// Auditor persiste registos de auditoria de forma append-only.
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}

// AlergiaClinica é a projecção de uma alergia do doente vista pela Farmácia.
type AlergiaClinica struct {
	Substancia string
	Severidade string
}

// LeitorClinico é a porta anti-corrupção para leitura de dados do BC Clínico.
type LeitorClinico interface {
	// ObterContextoDoente devolve se o doente existe e está activo, e as suas
	// alergias GRAVE/ANAFILÁCTICA.
	ObterContextoDoente(ctx context.Context, doenteID string) (activo bool, alergiasGraves []AlergiaClinica, err error)
	// EpisodioDoDoente indica se o episódio existe e pertence ao doente.
	EpisodioDoDoente(ctx context.Context, episodioID, doenteID string) (bool, error)
}

// Reexports dos read-models do domínio.
type (
	FiltroMedicamentos = dominio.FiltroMedicamentos
	PaginaMedicamentos = dominio.PaginaMedicamentos
	ResumoMedicamento  = dominio.ResumoMedicamento
	FiltroReceitas     = dominio.FiltroReceitas
	PaginaReceitas     = dominio.PaginaReceitas
	ResumoReceita      = dominio.ResumoReceita
)

// DadosNovoMedicamento é a entrada do registo de medicamento.
type DadosNovoMedicamento struct {
	NomeComercial     string  `json:"nome_comercial"`
	NomeGenerico      string  `json:"nome_generico"`
	FormaFarmaceutica string  `json:"forma_farmaceutica"`
	Dosagem           string  `json:"dosagem"`
	ViaAdministracao  string  `json:"via_administracao"`
	Fabricante        string  `json:"fabricante"`
	RequerReceita     bool    `json:"requer_receita"`
	Psicotropico      bool    `json:"psicotropico"`
	ClasseATC         *string `json:"classe_atc"`
	StockMinimo       int     `json:"stock_minimo"`
}

// DadosActualizarMedicamento tem a mesma forma (substituição integral dos campos mutáveis).
type DadosActualizarMedicamento = DadosNovoMedicamento

// DetalheMedicamento é o detalhe de um medicamento numa resposta.
type DetalheMedicamento struct {
	ID                string    `json:"id"`
	CodigoInterno     string    `json:"codigo_interno"`
	NomeComercial     string    `json:"nome_comercial"`
	NomeGenerico      string    `json:"nome_generico"`
	FormaFarmaceutica string    `json:"forma_farmaceutica"`
	Dosagem           string    `json:"dosagem"`
	ViaAdministracao  string    `json:"via_administracao"`
	Fabricante        string    `json:"fabricante,omitempty"`
	RequerReceita     bool      `json:"requer_receita"`
	Psicotropico      bool      `json:"psicotropico"`
	ClasseATC         *string   `json:"classe_atc,omitempty"`
	StockMinimo       int       `json:"stock_minimo"`
	Activo            bool      `json:"activo"`
	CriadoEm          time.Time `json:"criado_em"`
	ActualizadoEm     time.Time `json:"actualizado_em"`
}

// DadosItemReceita é um item num pedido de emissão.
type DadosItemReceita struct {
	MedicamentoID       string `json:"medicamento_id"`
	Posologia           string `json:"posologia"`
	DuracaoDias         *int   `json:"duracao_dias"`
	QuantidadePrescrita int    `json:"quantidade_prescrita"`
	Notas               string `json:"notas"`
}

// DadosNovaReceita é a entrada da emissão. MedicoID = actor autenticado.
type DadosNovaReceita struct {
	EpisodioID           string
	DoenteID             string
	Itens                []DadosItemReceita
	Notas                string
	IgnorarAlertaAlergia bool
	JustificacaoAlerta   string
}

// DadosAnularReceita é a entrada da anulação.
type DadosAnularReceita struct {
	Motivo string
}

// ItemReceitaDTO é um item numa resposta.
type ItemReceitaDTO struct {
	MedicamentoID        string `json:"medicamento_id"`
	Posologia            string `json:"posologia"`
	DuracaoDias          *int   `json:"duracao_dias,omitempty"`
	QuantidadePrescrita  int    `json:"quantidade_prescrita"`
	QuantidadeDispensada int    `json:"quantidade_dispensada"`
	Notas                string `json:"notas,omitempty"`
}

// DetalheReceita é o detalhe de uma receita numa resposta.
type DetalheReceita struct {
	ID         string           `json:"id"`
	EpisodioID string           `json:"episodio_id"`
	DoenteID   string           `json:"doente_id"`
	MedicoID   string           `json:"medico_id"`
	EmitidaEm  time.Time        `json:"emitida_em"`
	Estado     string           `json:"estado"`
	Notas      string           `json:"notas,omitempty"`
	ExpiraEm   time.Time        `json:"expira_em"`
	Itens      []ItemReceitaDTO `json:"itens"`
}
