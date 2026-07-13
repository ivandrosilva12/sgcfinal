// Package clinico contém os casos de uso do BC Clínico (Camada 2 — Aplicação).
// Orquestra o agregado Doente sobre portas de saída (repositório, auditoria),
// sem qualquer dependência de infra.
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// Auditor persiste registos de auditoria de forma append-only. Implementado por
// pgrepo.RepositorioAuditoria (partilhado com o BC Identidade).
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}

// Reexports dos read-models do domínio, para os handlers não importarem o domínio.
type (
	FiltroDoentes = dominio.FiltroDoentes
	PaginaDoentes = dominio.PaginaDoentes
	ResumoDoente  = dominio.ResumoDoente
)

// DadosMorada é a morada num pedido.
type DadosMorada struct {
	Provincia  string  `json:"provincia"`
	Municipio  string  `json:"municipio"`
	Comuna     string  `json:"comuna"`
	Bairro     string  `json:"bairro"`
	Rua        string  `json:"rua"`
	Casa       *string `json:"casa,omitempty"`
	Referencia *string `json:"referencia,omitempty"`
}

// DadosIdentificacao é a identificação num pedido.
type DadosIdentificacao struct {
	NomeCompleto   string    `json:"nome_completo"`
	DataNascimento time.Time `json:"-"`
	Sexo           string    `json:"sexo"`
	BI             *string   `json:"bi,omitempty"`
	NIF            *string   `json:"nif,omitempty"`
	Passaporte     *string   `json:"passaporte,omitempty"`
}

// DadosContactos são os contactos num pedido.
type DadosContactos struct {
	Telefone string       `json:"telefone"`
	Email    *string      `json:"email,omitempty"`
	Morada   *DadosMorada `json:"morada,omitempty"`
}

// DadosNovoDoente é a entrada do caso de uso de registo.
type DadosNovoDoente struct {
	NumProcesso    string
	Identificacao  DadosIdentificacao
	Contactos      DadosContactos
	Nacionalidade  string
	GrupoSanguineo *string
}

// DadosActualizarDoente é a entrada da actualização (campos a nil são ignorados).
type DadosActualizarDoente struct {
	Identificacao  *DadosIdentificacao
	Contactos      *DadosContactos
	GrupoSanguineo *string // "" limpa; presente redefine
}

// DadosAlergia é a entrada do registo de alergia.
type DadosAlergia struct {
	Substancia    string     `json:"substancia"`
	Severidade    string     `json:"severidade"`
	ReaccaoTipica string     `json:"reaccao_tipica,omitempty"`
	ConfirmadaEm  *time.Time `json:"-"`
	Notas         string     `json:"notas,omitempty"`
}

// DadosAntecedente é a entrada do registo de antecedente clínico.
type DadosAntecedente struct {
	Tipo       string     `json:"tipo"`
	Descricao  string     `json:"descricao"`
	CID        string     `json:"cid,omitempty"`
	DataInicio *time.Time `json:"-"`
	Activo     bool       `json:"activo"`
	Notas      string     `json:"notas,omitempty"`
}

// MoradaDTO é a morada numa resposta.
type MoradaDTO struct {
	Provincia  string  `json:"provincia"`
	Municipio  string  `json:"municipio"`
	Comuna     string  `json:"comuna"`
	Bairro     string  `json:"bairro"`
	Rua        string  `json:"rua"`
	Casa       *string `json:"casa,omitempty"`
	Referencia *string `json:"referencia,omitempty"`
}

// AlergiaDTO é uma alergia numa resposta.
type AlergiaDTO struct {
	Substancia    string     `json:"substancia"`
	Severidade    string     `json:"severidade"`
	ReaccaoTipica string     `json:"reaccao_tipica,omitempty"`
	ConfirmadaEm  *time.Time `json:"confirmada_em,omitempty"`
	Notas         string     `json:"notas,omitempty"`
}

// AntecedenteDTO é um antecedente numa resposta.
type AntecedenteDTO struct {
	Tipo       string     `json:"tipo"`
	Descricao  string     `json:"descricao"`
	CID        string     `json:"cid,omitempty"`
	DataInicio *time.Time `json:"data_inicio,omitempty"`
	Activo     bool       `json:"activo"`
	Notas      string     `json:"notas,omitempty"`
}

// DetalheDoente é o detalhe completo de um doente numa resposta.
type DetalheDoente struct {
	ID             string           `json:"id"`
	NumProcesso    string           `json:"num_processo"`
	NomeCompleto   string           `json:"nome_completo"`
	DataNascimento time.Time        `json:"data_nascimento"`
	Sexo           string           `json:"sexo"`
	BI             *string          `json:"bi,omitempty"`
	NIF            *string          `json:"nif,omitempty"`
	Passaporte     *string          `json:"passaporte,omitempty"`
	Nacionalidade  string           `json:"nacionalidade"`
	Telefone       string           `json:"telefone"`
	Email          *string          `json:"email,omitempty"`
	Morada         *MoradaDTO       `json:"morada,omitempty"`
	GrupoSanguineo *string          `json:"grupo_sanguineo,omitempty"`
	Estado         string           `json:"estado"`
	Alergias       []AlergiaDTO     `json:"alergias"`
	Antecedentes   []AntecedenteDTO `json:"antecedentes"`
	CriadoEm       time.Time        `json:"criado_em"`
	ActualizadoEm  time.Time        `json:"actualizado_em"`
}

// --- Episódio Clínico ---

// Reexports dos read-models de episódio.
type (
	FiltroEpisodios = dominio.FiltroEpisodios
	PaginaEpisodios = dominio.PaginaEpisodios
	ResumoEpisodio  = dominio.ResumoEpisodio
)

// DadosNovoEpisodio é a entrada do caso de uso de iniciar episódio. DoenteID vem
// do caminho do pedido; Inicio é opcional (default: momento da criação).
type DadosNovoEpisodio struct {
	DoenteID        string
	Tipo            string
	EspecialidadeID string
	MedicoID        string
	Inicio          *time.Time
}

// DadosNotaClinica é a nota clínica num pedido de actualização.
type DadosNotaClinica struct {
	QueixaPrincipal string `json:"queixa_principal"`
	HistoriaDoenca  string `json:"historia_doenca"`
	ExameObjectivo  string `json:"exame_objectivo"`
	Diagnostico     string `json:"diagnostico"`
	Plano           string `json:"plano"`
}

// DadosDiagnosticoCID é um diagnóstico CID num pedido.
type DadosDiagnosticoCID struct {
	CID       string `json:"cid"`
	Principal bool   `json:"principal"`
}

// DadosActualizarEpisodio é a entrada da actualização (campos a nil ignorados).
type DadosActualizarEpisodio struct {
	Nota            *DadosNotaClinica
	DiagnosticosCID *[]DadosDiagnosticoCID
}

// NotaClinicaDTO é a nota clínica numa resposta.
type NotaClinicaDTO struct {
	QueixaPrincipal string `json:"queixa_principal,omitempty"`
	HistoriaDoenca  string `json:"historia_doenca,omitempty"`
	ExameObjectivo  string `json:"exame_objectivo,omitempty"`
	Diagnostico     string `json:"diagnostico,omitempty"`
	Plano           string `json:"plano,omitempty"`
}

// DiagnosticoCIDDTO é um diagnóstico CID numa resposta.
type DiagnosticoCIDDTO struct {
	CID       string `json:"cid"`
	Principal bool   `json:"principal"`
}

// DetalheEpisodio é o detalhe completo de um episódio numa resposta.
type DetalheEpisodio struct {
	ID              string              `json:"id"`
	DoenteID        string              `json:"doente_id"`
	Tipo            string              `json:"tipo"`
	EspecialidadeID string              `json:"especialidade_id"`
	MedicoID        string              `json:"medico_id"`
	Inicio          time.Time           `json:"inicio"`
	Fim             *time.Time          `json:"fim,omitempty"`
	Nota            NotaClinicaDTO      `json:"nota"`
	DiagnosticosCID []DiagnosticoCIDDTO `json:"diagnosticos_cid"`
	Estado          string              `json:"estado"`
	CriadoEm        time.Time           `json:"criado_em"`
	ActualizadoEm   time.Time           `json:"actualizado_em"`
	FechadoEm       *time.Time          `json:"fechado_em,omitempty"`
	FechadoPor      string              `json:"fechado_por,omitempty"`
}

// EHR é a projecção de leitura do registo clínico: doente (com alergias e
// antecedentes) + episódios paginados.
type EHR struct {
	Doente    DetalheDoente   `json:"doente"`
	Episodios PaginaEpisodios `json:"episodios"`
}

// --- Consentimento (LPDP) ---

// Reexports dos read-models de consentimento.
type (
	FiltroConsentimentos = dominio.FiltroConsentimentos
	ResumoConsentimento  = dominio.ResumoConsentimento
)

// DadosNovoConsentimento é a entrada do registo de consentimento. DoenteID vem do
// caminho; ConcedidoEm é opcional (default: momento do registo).
type DadosNovoConsentimento struct {
	DoenteID     string
	Finalidade   string
	Concedido    bool
	DocumentoURL string
	ConcedidoEm  *time.Time
}

// DetalheConsentimento é o detalhe de um consentimento numa resposta.
type DetalheConsentimento struct {
	ID           string     `json:"id"`
	DoenteID     string     `json:"doente_id"`
	Finalidade   string     `json:"finalidade"`
	Concedido    bool       `json:"concedido"`
	DocumentoURL string     `json:"documento_url,omitempty"`
	ConcedidoEm  time.Time  `json:"concedido_em"`
	RevogadoEm   *time.Time `json:"revogado_em,omitempty"`
	Vigente      bool       `json:"vigente"`
}

// --- Procedimento Cirúrgico ---

// Reexport do read-model de procedimento.
type ResumoProcedimento = dominio.ResumoProcedimento

// DadosAgendarProcedimento é a entrada do agendamento. EpisodioID vem do caminho.
type DadosAgendarProcedimento struct {
	EpisodioID      string
	Codigo          string
	Descricao       string
	Sala            string
	CirurgiaoID     string
	AuxiliarID      string
	Anestesia       string
	AnestesistaID   string
	ConsentimentoID string
	Observacoes     string
}

// DadosConcluirProcedimento é a entrada da conclusão.
type DadosConcluirProcedimento struct {
	Complicacoes string
	Observacoes  string
}

// DetalheProcedimento é o detalhe de um procedimento numa resposta.
type DetalheProcedimento struct {
	ID              string     `json:"id"`
	EpisodioID      string     `json:"episodio_id"`
	Codigo          string     `json:"codigo_procedimento"`
	Descricao       string     `json:"descricao"`
	Sala            string     `json:"sala,omitempty"`
	CirurgiaoID     string     `json:"cirurgiao_id"`
	AuxiliarID      string     `json:"auxiliar_id,omitempty"`
	Anestesia       string     `json:"anestesia"`
	AnestesistaID   string     `json:"anestesista_id,omitempty"`
	ConsentimentoID string     `json:"consentimento_id"`
	Inicio          *time.Time `json:"inicio,omitempty"`
	Fim             *time.Time `json:"fim,omitempty"`
	Complicacoes    string     `json:"complicacoes,omitempty"`
	Observacoes     string     `json:"observacoes,omitempty"`
	Estado          string     `json:"estado"`
	CriadoEm        time.Time  `json:"criado_em"`
}
