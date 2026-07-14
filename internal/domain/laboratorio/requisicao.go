package laboratorio

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Prioridade é a urgência de uma requisição de análises.
type Prioridade string

const (
	PrioridadeRotina  Prioridade = "ROTINA"
	PrioridadeUrgente Prioridade = "URGENTE"
)

var prioridadesValidas = map[Prioridade]bool{PrioridadeRotina: true, PrioridadeUrgente: true}

// ParsePrioridade valida e normaliza uma prioridade (aceita minúsculas).
func ParsePrioridade(codigo string) (Prioridade, error) {
	p := Prioridade(strings.ToUpper(strings.TrimSpace(codigo)))
	if !prioridadesValidas[p] {
		return "", erros.Novo(erros.CategoriaValidacao,
			"prioridade da requisição inválida (esperado ROTINA ou URGENTE)")
	}
	return p, nil
}

// EstadoRequisicao é o estado do ciclo de vida da requisição.
type EstadoRequisicao string

const (
	RequisicaoEmitida   EstadoRequisicao = "EMITIDA"
	RequisicaoCancelada EstadoRequisicao = "CANCELADA"
)

// ItemRequisicao é uma análise pedida numa requisição.
type ItemRequisicao struct {
	CodigoAnalise string `json:"codigo_analise"`
	Observacoes   string `json:"observacoes,omitempty"`
}

// RequisicaoLab é um agregado raiz do BC Laboratório: o pedido de análises de um
// médico para um episódio. episodioID/doenteID são referências a outro bounded
// context — validadas pela ACL na aplicação, sem FK.
type RequisicaoLab struct {
	id                   string
	episodioID           string
	doenteID             string
	medicoRequisitanteID string
	prioridade           Prioridade
	itens                []ItemRequisicao
	estado               EstadoRequisicao
	criadoEm             time.Time
}

// DadosNovaRequisicao agrupa os parâmetros de construção.
type DadosNovaRequisicao struct {
	EpisodioID           string
	DoenteID             string
	MedicoRequisitanteID string
	Prioridade           Prioridade
	Itens                []ItemRequisicao
}

// NovaRequisicao valida as invariantes e devolve a requisição EMITIDA. Os códigos
// de análise são normalizados para maiúsculas; repetições são rejeitadas (pedir a
// mesma análise duas vezes na mesma requisição criaria duas linhas na fila do
// técnico para a mesma colheita).
func NovaRequisicao(d DadosNovaRequisicao) (*RequisicaoLab, error) {
	d.EpisodioID = strings.TrimSpace(d.EpisodioID)
	if d.EpisodioID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "episódio da requisição em falta")
	}
	d.DoenteID = strings.TrimSpace(d.DoenteID)
	if d.DoenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente da requisição em falta")
	}
	d.MedicoRequisitanteID = strings.TrimSpace(d.MedicoRequisitanteID)
	if d.MedicoRequisitanteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "médico requisitante em falta")
	}
	prioridade, err := ParsePrioridade(string(d.Prioridade))
	if err != nil {
		return nil, err
	}
	if len(d.Itens) == 0 {
		return nil, erros.Novo(erros.CategoriaValidacao, "a requisição tem de pedir pelo menos uma análise")
	}
	vistos := map[string]bool{}
	itens := make([]ItemRequisicao, 0, len(d.Itens))
	for _, i := range d.Itens {
		codigo := strings.ToUpper(strings.TrimSpace(i.CodigoAnalise))
		if codigo == "" {
			return nil, erros.Novo(erros.CategoriaValidacao, "código de análise da requisição em falta")
		}
		if vistos[codigo] {
			return nil, erros.Novo(erros.CategoriaValidacao, "análise repetida na requisição: "+codigo)
		}
		vistos[codigo] = true
		itens = append(itens, ItemRequisicao{CodigoAnalise: codigo, Observacoes: strings.TrimSpace(i.Observacoes)})
	}
	return &RequisicaoLab{
		episodioID: d.EpisodioID, doenteID: d.DoenteID,
		medicoRequisitanteID: d.MedicoRequisitanteID, prioridade: prioridade,
		itens: itens, estado: RequisicaoEmitida,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados (vazio se não persistida).
func (r *RequisicaoLab) ID() string { return r.id }

// EpisodioID devolve o episódio a que a requisição pertence.
func (r *RequisicaoLab) EpisodioID() string { return r.episodioID }

// DoenteID devolve o doente da requisição.
func (r *RequisicaoLab) DoenteID() string { return r.doenteID }

// Itens devolve os itens pedidos.
func (r *RequisicaoLab) Itens() []ItemRequisicao { return r.itens }

// Estado devolve o estado actual.
func (r *RequisicaoLab) Estado() EstadoRequisicao { return r.estado }

// SnapshotRequisicao carrega o estado completo para persistência ou rehidratação.
type SnapshotRequisicao struct {
	ID                   string
	EpisodioID           string
	DoenteID             string
	MedicoRequisitanteID string
	Prioridade           Prioridade
	Itens                []ItemRequisicao
	Estado               EstadoRequisicao
	CriadoEm             time.Time
}

// Snapshot devolve o estado completo do agregado.
func (r *RequisicaoLab) Snapshot() SnapshotRequisicao {
	return SnapshotRequisicao{
		ID: r.id, EpisodioID: r.episodioID, DoenteID: r.doenteID,
		MedicoRequisitanteID: r.medicoRequisitanteID, Prioridade: r.prioridade,
		Itens: r.itens, Estado: r.estado, CriadoEm: r.criadoEm,
	}
}

// ReconstruirRequisicao reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirRequisicao(s SnapshotRequisicao) *RequisicaoLab {
	return &RequisicaoLab{
		id: s.ID, episodioID: s.EpisodioID, doenteID: s.DoenteID,
		medicoRequisitanteID: s.MedicoRequisitanteID, prioridade: s.Prioridade,
		itens: s.Itens, estado: s.Estado, criadoEm: s.CriadoEm,
	}
}

// ResumoRequisicao é a projecção de leitura de uma requisição.
type ResumoRequisicao struct {
	ID          string    `json:"id"`
	EpisodioID  string    `json:"episodio_id"`
	DoenteID    string    `json:"doente_id"`
	Prioridade  string    `json:"prioridade"`
	Estado      string    `json:"estado"`
	NumAnalises int       `json:"num_analises"`
	CriadoEm    time.Time `json:"criado_em"`
}

// Nota: a porta RepositorioRequisicoes vive em resultado.go (Task 4) — a sua
// operação de emissão recebe os *Resultado criados com a requisição, e mantê-la
// junto do Resultado evita que este ficheiro dependa de um tipo que ainda não existe.
