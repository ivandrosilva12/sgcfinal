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
