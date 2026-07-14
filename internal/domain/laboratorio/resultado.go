package laboratorio

import (
	"context"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// EstadoResultado é o estado do ciclo de vida de um resultado de análise.
//
//	PENDENTE → COLHIDA → PROCESSADA → VALIDADA → CONCLUIDA
//	    └──────────┴─────► RECUSADA
//
// VALIDADA e CONCLUIDA existem já no enum e nas CHECK da base de dados; a transição
// Validar (com a invariante de segregação submissor ≠ validador) é do Sprint 13.
type EstadoResultado string

const (
	ResPendente   EstadoResultado = "PENDENTE"
	ResColhida    EstadoResultado = "COLHIDA"
	ResProcessada EstadoResultado = "PROCESSADA"
	ResValidada   EstadoResultado = "VALIDADA"
	ResConcluida  EstadoResultado = "CONCLUIDA"
	ResRecusada   EstadoResultado = "RECUSADA"
)

// Resultado é um agregado raiz do BC Laboratório: o resultado de uma análise de uma
// requisição. É criado em PENDENTE, um por item da requisição.
type Resultado struct {
	id                     string
	requisicaoID           string
	codigoAnalise          string
	valor                  string
	unidade                string
	observacoes            string
	motivoRecusa           string
	estado                 EstadoResultado
	estadoAnterior         EstadoResultado
	tecnicoColheitaID      string
	tecnicoSubmissorID     string
	patologistaValidadorID string
	colhidaEm              *time.Time
	submetidaEm            *time.Time
	validadaEm             *time.Time
	valorCritico           bool
	criadoEm               time.Time
}

// NovoResultado cria um resultado em PENDENTE para um item da requisição.
func NovoResultado(requisicaoID, codigoAnalise, unidade string) (*Resultado, error) {
	requisicaoID = strings.TrimSpace(requisicaoID)
	if requisicaoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "requisição do resultado em falta")
	}
	codigoAnalise = strings.ToUpper(strings.TrimSpace(codigoAnalise))
	if codigoAnalise == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "código de análise do resultado em falta")
	}
	unidade = strings.TrimSpace(unidade)
	if unidade == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "unidade do resultado em falta")
	}
	return &Resultado{
		requisicaoID: requisicaoID, codigoAnalise: codigoAnalise, unidade: unidade,
		estado: ResPendente, estadoAnterior: ResPendente,
	}, nil
}

// ColherAmostra transita PENDENTE → COLHIDA. O técnico é o sujeito autenticado.
func (r *Resultado) ColherAmostra(tecnicoID string, em time.Time) error {
	if r.estado != ResPendente {
		return erros.Novo(erros.CategoriaConflito, "só é possível colher a amostra de um resultado pendente")
	}
	tecnicoID = strings.TrimSpace(tecnicoID)
	if tecnicoID == "" {
		return erros.Novo(erros.CategoriaValidacao, "técnico da colheita em falta")
	}
	if em.IsZero() {
		return erros.Novo(erros.CategoriaValidacao, "data da colheita em falta")
	}
	r.estadoAnterior = r.estado
	r.estado = ResColhida
	r.tecnicoColheitaID = tecnicoID
	r.colhidaEm = &em
	return nil
}

// RecusarAmostra transita PENDENTE ou COLHIDA → RECUSADA. O motivo é obrigatório:
// uma amostra inviável sem motivo registado não é auditável nem repetível.
func (r *Resultado) RecusarAmostra(motivo string, em time.Time) error {
	if r.estado != ResPendente && r.estado != ResColhida {
		return erros.Novo(erros.CategoriaConflito,
			"só é possível recusar uma amostra pendente ou colhida")
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return erros.Novo(erros.CategoriaValidacao, "motivo da recusa da amostra em falta")
	}
	if em.IsZero() {
		return erros.Novo(erros.CategoriaValidacao, "data da recusa em falta")
	}
	r.estadoAnterior = r.estado
	r.estado = ResRecusada
	r.motivoRecusa = motivo
	return nil
}

// SubmeterPreliminar transita COLHIDA → PROCESSADA. O submissor é o sujeito
// autenticado — nunca um campo do pedido: é contra ele que a validação do Sprint 13
// compara o patologista para impor a segregação de funções.
func (r *Resultado) SubmeterPreliminar(tecnicoID, valor, observacoes string, em time.Time) error {
	if r.estado != ResColhida {
		return erros.Novo(erros.CategoriaConflito,
			"só é possível submeter o resultado de uma amostra colhida")
	}
	tecnicoID = strings.TrimSpace(tecnicoID)
	if tecnicoID == "" {
		return erros.Novo(erros.CategoriaValidacao, "técnico submissor em falta")
	}
	valor = strings.TrimSpace(valor)
	if valor == "" {
		return erros.Novo(erros.CategoriaValidacao, "valor do resultado em falta")
	}
	if em.IsZero() {
		return erros.Novo(erros.CategoriaValidacao, "data da submissão em falta")
	}
	r.estadoAnterior = r.estado
	r.estado = ResProcessada
	r.tecnicoSubmissorID = tecnicoID
	r.valor = valor
	r.observacoes = strings.TrimSpace(observacoes)
	r.submetidaEm = &em
	return nil
}

// ID devolve o identificador atribuído pela base de dados.
func (r *Resultado) ID() string { return r.id }

// RequisicaoID devolve a requisição a que o resultado pertence.
func (r *Resultado) RequisicaoID() string { return r.requisicaoID }

// Estado devolve o estado actual.
func (r *Resultado) Estado() EstadoResultado { return r.estado }

// TecnicoSubmissorID devolve quem submeteu o preliminar (vazio antes da submissão).
func (r *Resultado) TecnicoSubmissorID() string { return r.tecnicoSubmissorID }

// SnapshotResultado carrega o estado completo para persistência ou rehidratação.
//
// EstadoAnterior é o estado com que o agregado foi lido da base de dados: é o que o
// repositório usa na guarda compare-and-set do UPDATE de transição. Num agregado
// recém-lido (ou recém-criado) é igual a Estado.
type SnapshotResultado struct {
	ID                     string
	RequisicaoID           string
	CodigoAnalise          string
	Valor                  string
	Unidade                string
	Observacoes            string
	MotivoRecusa           string
	Estado                 EstadoResultado
	EstadoAnterior         EstadoResultado
	TecnicoColheitaID      string
	TecnicoSubmissorID     string
	PatologistaValidadorID string
	ColhidaEm              *time.Time
	SubmetidaEm            *time.Time
	ValidadaEm             *time.Time
	ValorCritico           bool
	CriadoEm               time.Time
}

// Snapshot devolve o estado completo do agregado.
func (r *Resultado) Snapshot() SnapshotResultado {
	return SnapshotResultado{
		ID: r.id, RequisicaoID: r.requisicaoID, CodigoAnalise: r.codigoAnalise,
		Valor: r.valor, Unidade: r.unidade, Observacoes: r.observacoes,
		MotivoRecusa: r.motivoRecusa, Estado: r.estado, EstadoAnterior: r.estadoAnterior,
		TecnicoColheitaID: r.tecnicoColheitaID, TecnicoSubmissorID: r.tecnicoSubmissorID,
		PatologistaValidadorID: r.patologistaValidadorID,
		ColhidaEm:              r.colhidaEm, SubmetidaEm: r.submetidaEm, ValidadaEm: r.validadaEm,
		ValorCritico: r.valorCritico, CriadoEm: r.criadoEm,
	}
}

// ReconstruirResultado reconstrói o agregado a partir de um snapshot persistido.
// EstadoAnterior é fixado no estado lido — qualquer transição posterior deixa-o a
// apontar para o estado que está na base de dados.
func ReconstruirResultado(s SnapshotResultado) *Resultado {
	return &Resultado{
		id: s.ID, requisicaoID: s.RequisicaoID, codigoAnalise: s.CodigoAnalise,
		valor: s.Valor, unidade: s.Unidade, observacoes: s.Observacoes,
		motivoRecusa: s.MotivoRecusa, estado: s.Estado, estadoAnterior: s.Estado,
		tecnicoColheitaID: s.TecnicoColheitaID, tecnicoSubmissorID: s.TecnicoSubmissorID,
		patologistaValidadorID: s.PatologistaValidadorID,
		colhidaEm:              s.ColhidaEm, submetidaEm: s.SubmetidaEm, validadaEm: s.ValidadaEm,
		valorCritico: s.ValorCritico, criadoEm: s.CriadoEm,
	}
}

// ResumoResultado é a projecção de leitura de um resultado.
type ResumoResultado struct {
	ID            string     `json:"id"`
	RequisicaoID  string     `json:"requisicao_id"`
	EpisodioID    string     `json:"episodio_id,omitempty"`
	CodigoAnalise string     `json:"codigo_analise"`
	Valor         string     `json:"valor,omitempty"`
	Unidade       string     `json:"unidade"`
	Estado        string     `json:"estado"`
	ValorCritico  bool       `json:"valor_critico"`
	ColhidaEm     *time.Time `json:"colhida_em,omitempty"`
	SubmetidaEm   *time.Time `json:"submetida_em,omitempty"`
	CriadoEm      time.Time  `json:"criado_em"`
}

// RepositorioResultados é a porta de saída de persistência de resultados.
//
// Transitar aplica a transição com guarda compare-and-set (usa EstadoAnterior do
// snapshot); uma lista de estados vazia em ListarFila/ListarPorEpisodio significa
// "todos os estados".
type RepositorioResultados interface {
	ObterPorID(ctx context.Context, id string) (*Resultado, error)
	Transitar(ctx context.Context, r *Resultado) error
	ListarFila(ctx context.Context, estados []EstadoResultado) ([]ResumoResultado, error)
	ListarPorEpisodio(ctx context.Context, episodioID string, estados []EstadoResultado) ([]ResumoResultado, error)
}

// RepositorioRequisicoes é a porta de saída de persistência de requisições.
//
// Emitir grava a requisição, os seus itens e os resultados PENDENTE numa única
// transacção: uma requisição sem resultados nunca chegaria à fila do laboratório e
// ficaria invisível para toda a gente.
type RepositorioRequisicoes interface {
	Emitir(ctx context.Context, r *RequisicaoLab, resultados []*Resultado) (string, error)
	ObterPorID(ctx context.Context, id string) (*RequisicaoLab, error)
	ListarPorEpisodio(ctx context.Context, episodioID string) ([]ResumoRequisicao, error)
}
