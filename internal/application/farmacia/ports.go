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

// --- Stock & Dispensa ---

// ItemDispensa é uma linha de dispensa (medicamento + quantidade) passada ao motor.
type ItemDispensa struct {
	MedicamentoID string
	Quantidade    int
}

// MotorDispensa é a porta transaccional da dispensa: aloca stock por FEFO, regista
// os movimentos e persiste a receita, atomicamente.
type MotorDispensa interface {
	Dispensar(ctx context.Context, receita dominio.SnapshotReceita, itens []ItemDispensa, realizadoPor string) ([]dominio.AlocacaoFEFO, error)
}

// Reexports dos read-models de stock.
type (
	FiltroFornecedores = dominio.FiltroFornecedores
	PaginaFornecedores = dominio.PaginaFornecedores
	ResumoFornecedor   = dominio.ResumoFornecedor
	ResumoLote         = dominio.ResumoLote
)

// DadosNovoFornecedor é a entrada do registo de fornecedor.
type DadosNovoFornecedor struct {
	Nome     string  `json:"nome"`
	NIF      *string `json:"nif"`
	Contacto *string `json:"contacto"`
}

// DetalheFornecedor é o detalhe de um fornecedor numa resposta.
type DetalheFornecedor struct {
	ID       string    `json:"id"`
	Nome     string    `json:"nome"`
	NIF      *string   `json:"nif,omitempty"`
	Contacto *string   `json:"contacto,omitempty"`
	Activo   bool      `json:"activo"`
	CriadoEm time.Time `json:"criado_em"`
}

// DadosEntradaStock é a entrada de um lote de stock (UC-FAR-01).
type DadosEntradaStock struct {
	MedicamentoID      string
	NumeroLote         string
	Validade           time.Time
	Quantidade         int
	PrecoUnitarioCusto string
	FornecedorID       *string
	Notas              string
}

// DetalheLote é o detalhe de um lote numa resposta.
type DetalheLote struct {
	ID                 string    `json:"id"`
	MedicamentoID      string    `json:"medicamento_id"`
	NumeroLote         string    `json:"numero_lote"`
	Validade           time.Time `json:"validade"`
	QuantidadeInicial  int       `json:"quantidade_inicial"`
	QuantidadeActual   int       `json:"quantidade_actual"`
	PrecoUnitarioCusto string    `json:"preco_unit_custo"`
	FornecedorID       *string   `json:"fornecedor_id,omitempty"`
	EntradaEm          time.Time `json:"entrada_em"`
	Notas              string    `json:"notas,omitempty"`
}

// StockDTO é o stock disponível total de um medicamento.
type StockDTO struct {
	MedicamentoID string `json:"medicamento_id"`
	Disponivel    int    `json:"disponivel"`
}

// ItemDispensaDTO é um item num pedido de dispensa.
type ItemDispensaDTO struct {
	MedicamentoID string `json:"medicamento_id"`
	Quantidade    int    `json:"quantidade"`
}

// DadosDispensa é a entrada da dispensa de uma receita.
type DadosDispensa struct {
	Itens                []ItemDispensaDTO
	IgnorarAlertaAlergia bool
	JustificacaoAlerta   string
}
