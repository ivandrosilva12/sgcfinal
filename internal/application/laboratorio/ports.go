// Package laboratorio contém os casos de uso do BC Laboratório (Camada 2 — Aplicação).
package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// Auditor persiste registos de auditoria de forma append-only.
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}

// LeitorClinico é a porta anti-corrupção para leitura de dados do BC Clínico. O
// Laboratório nunca importa tipos do domínio Clínico: só faz estas duas perguntas.
type LeitorClinico interface {
	// DoenteActivo indica se o doente existe e está activo.
	DoenteActivo(ctx context.Context, doenteID string) (bool, error)
	// EpisodioAbertoDoDoente indica se o episódio existe, pertence ao doente e
	// está ABERTO.
	EpisodioAbertoDoDoente(ctx context.Context, episodioID, doenteID string) (bool, error)
}

// Reexports dos read-models do domínio.
type (
	ResumoAnalise    = dominio.ResumoAnalise
	ResumoRequisicao = dominio.ResumoRequisicao
	ResumoResultado  = dominio.ResumoResultado
)

// DadosNovaAnalise é a entrada do registo de uma análise no catálogo.
type DadosNovaAnalise struct {
	Codigo          string                        `json:"codigo"`
	Nome            string                        `json:"nome"`
	Unidade         string                        `json:"unidade"`
	Intervalos      []dominio.IntervaloReferencia `json:"intervalos_referencia"`
	ValoresCriticos []dominio.ValorCritico        `json:"valores_criticos"`
}

// DetalheAnalise é o detalhe de uma análise numa resposta.
type DetalheAnalise struct {
	Codigo          string                        `json:"codigo"`
	Nome            string                        `json:"nome"`
	Unidade         string                        `json:"unidade"`
	Intervalos      []dominio.IntervaloReferencia `json:"intervalos_referencia"`
	ValoresCriticos []dominio.ValorCritico        `json:"valores_criticos"`
	Activo          bool                          `json:"activo"`
}

// ItemPedido é uma análise pedida numa requisição.
type ItemPedido struct {
	CodigoAnalise string `json:"codigo_analise"`
	Observacoes   string `json:"observacoes"`
}

// DadosEmitirRequisicao é a entrada da emissão de uma requisição. O EpisodioID vem
// do caminho; o MedicoRequisitanteID é o sujeito autenticado, não um campo do corpo.
type DadosEmitirRequisicao struct {
	EpisodioID string
	DoenteID   string       `json:"doente_id"`
	Prioridade string       `json:"prioridade"`
	Itens      []ItemPedido `json:"itens"`
}

// DetalheRequisicao é o detalhe de uma requisição numa resposta.
type DetalheRequisicao struct {
	ID                   string                   `json:"id"`
	EpisodioID           string                   `json:"episodio_id"`
	DoenteID             string                   `json:"doente_id"`
	MedicoRequisitanteID string                   `json:"medico_requisitante_id"`
	Prioridade           string                   `json:"prioridade"`
	Estado               string                   `json:"estado"`
	Itens                []dominio.ItemRequisicao `json:"itens"`
	CriadoEm             time.Time                `json:"criado_em"`
}

// DadosSubmeterPreliminar é a entrada da submissão do resultado preliminar.
type DadosSubmeterPreliminar struct {
	Valor       string `json:"valor"`
	Observacoes string `json:"observacoes"`
}

// DetalheResultado é o detalhe de um resultado numa resposta.
type DetalheResultado struct {
	ID                 string     `json:"id"`
	RequisicaoID       string     `json:"requisicao_id"`
	CodigoAnalise      string     `json:"codigo_analise"`
	Valor              string     `json:"valor,omitempty"`
	Unidade            string     `json:"unidade"`
	Observacoes        string     `json:"observacoes,omitempty"`
	MotivoRecusa       string     `json:"motivo_recusa,omitempty"`
	Estado             string     `json:"estado"`
	TecnicoSubmissorID string     `json:"tecnico_submissor_id,omitempty"`
	ColhidaEm          *time.Time `json:"colhida_em,omitempty"`
	SubmetidaEm        *time.Time `json:"submetida_em,omitempty"`
	ValorCritico       bool       `json:"valor_critico"`
}

// EstadosVisiveisAoMedico são os únicos estados que a leitura clínica devolve: o
// resultado preliminar (PROCESSADA) não é visível ao médico — só a validação do
// patologista o torna visível (critério de saída do marco).
var EstadosVisiveisAoMedico = []dominio.EstadoResultado{dominio.ResValidada, dominio.ResConcluida}
