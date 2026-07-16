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
	corrigeResultadoID     string
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
		estado: ResPendente,
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
	// em só sobrevive no evento AmostraRecusada — não há coluna "recusada_em" no
	// agregado, por isso não é guardado em campo nenhum aqui.
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
	r.estado = ResProcessada
	r.tecnicoSubmissorID = tecnicoID
	r.valor = valor
	r.observacoes = strings.TrimSpace(observacoes)
	r.submetidaEm = &em
	return nil
}

// Validar transita PROCESSADA → VALIDADA. O validador é o sujeito autenticado. A
// invariante de segregação de funções é o coração do Sprint 13: quem submeteu o
// preliminar NUNCA o valida. `critico` é avaliado pela aplicação contra o catálogo
// (o agregado não conhece a Analise) e gravado aqui.
func (r *Resultado) Validar(patologistaID string, critico bool, em time.Time) error {
	if r.estado != ResProcessada {
		return erros.Novo(erros.CategoriaConflito, "só é possível validar um resultado processado")
	}
	patologistaID = strings.TrimSpace(patologistaID)
	if patologistaID == "" {
		return erros.Novo(erros.CategoriaValidacao, "patologista validador em falta")
	}
	if patologistaID == r.tecnicoSubmissorID {
		return erros.Novo(erros.CategoriaRegraNegocio,
			"segregação de funções: quem submeteu o resultado não o pode validar")
	}
	if em.IsZero() {
		return erros.Novo(erros.CategoriaValidacao, "data da validação em falta")
	}
	r.estado = ResValidada
	r.patologistaValidadorID = patologistaID
	r.validadaEm = &em
	r.valorCritico = critico
	return nil
}

// CorrigeResultadoID devolve o id do resultado que este corrige (vazio se não é
// uma correcção).
func (r *Resultado) CorrigeResultadoID() string { return r.corrigeResultadoID }

// Corrigir arquiva o resultado validado (→ CONCLUIDA) e devolve um NOVO Resultado
// VALIDADA que o substitui, apontando-lhe via corrigeResultadoID. Preserva o técnico
// submissor original (proveniência) — pelo que a segregação continua a valer: o
// corrector nunca é o técnico que submeteu o preliminar original. O novo agregado
// nasce por inserir (sem estado anterior).
func (r *Resultado) Corrigir(patologistaID, novoValor, observacoes string, critico bool, em time.Time) (*Resultado, error) {
	if r.estado != ResValidada {
		return nil, erros.Novo(erros.CategoriaConflito, "só é possível corrigir um resultado validado")
	}
	patologistaID = strings.TrimSpace(patologistaID)
	if patologistaID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "patologista corrector em falta")
	}
	if patologistaID == r.tecnicoSubmissorID {
		return nil, erros.Novo(erros.CategoriaRegraNegocio,
			"segregação de funções: quem submeteu o resultado não o pode corrigir")
	}
	novoValor = strings.TrimSpace(novoValor)
	if novoValor == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "valor corrigido em falta")
	}
	if em.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "data da correcção em falta")
	}
	novo := &Resultado{
		requisicaoID:           r.requisicaoID,
		codigoAnalise:          r.codigoAnalise,
		valor:                  novoValor,
		unidade:                r.unidade,
		observacoes:            strings.TrimSpace(observacoes),
		estado:                 ResValidada,
		tecnicoColheitaID:      r.tecnicoColheitaID,
		tecnicoSubmissorID:     r.tecnicoSubmissorID,
		patologistaValidadorID: patologistaID,
		colhidaEm:              r.colhidaEm,
		submetidaEm:            r.submetidaEm,
		validadaEm:             &em,
		valorCritico:           critico,
		corrigeResultadoID:     r.id,
	}
	r.estado = ResConcluida
	return novo, nil
}

// ID devolve o identificador atribuído pela base de dados.
func (r *Resultado) ID() string { return r.id }

// RequisicaoID devolve a requisição a que o resultado pertence.
func (r *Resultado) RequisicaoID() string { return r.requisicaoID }

// Estado devolve o estado actual.
func (r *Resultado) Estado() EstadoResultado { return r.estado }

// TecnicoSubmissorID devolve quem submeteu o preliminar (vazio antes da submissão).
func (r *Resultado) TecnicoSubmissorID() string { return r.tecnicoSubmissorID }

// Valor devolve o valor submetido (vazio antes da submissão).
func (r *Resultado) Valor() string { return r.valor }

// SnapshotResultado carrega o estado completo para persistência ou rehidratação.
//
// EstadoAnterior é o estado lido da base de dados (vazio num agregado novo). O
// repositório usa-o como guarda compare-and-set no UPDATE de transição. É
// derivado — quem reconstrói não o preenche.
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
	CorrigeResultadoID     string
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
		ValorCritico: r.valorCritico, CorrigeResultadoID: r.corrigeResultadoID, CriadoEm: r.criadoEm,
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
		valorCritico: s.ValorCritico, corrigeResultadoID: s.CorrigeResultadoID, criadoEm: s.CriadoEm,
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
// snapshot). A semântica de uma lista de estados vazia é assimétrica entre as duas
// listagens, por desenho: em ListarFila, vazio/nil significa "todos os estados" — é o
// que a fila de trabalho de quem executa precisa. Em ListarPorEpisodio, vazio/nil
// significa "nenhum estado" (zero linhas) — é fail-closed de propósito, porque é o
// único filtro que impede um resultado PROCESSADA (preliminar, ainda não validado) de
// vazar para a vista clínica do médico.
type RepositorioResultados interface {
	ObterPorID(ctx context.Context, id string) (*Resultado, error)
	Transitar(ctx context.Context, r *Resultado) error
	// Corrigir persiste uma correcção numa única transacção: INSERT do novo Resultado
	// (VALIDADA, corrige_resultado_id→original) e UPDATE compare-and-set do original
	// (VALIDADA→CONCLUIDA). Qualquer falha faz rollback de ambos. Devolve o id do novo.
	Corrigir(ctx context.Context, novo *Resultado, original *Resultado) (string, error)
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
