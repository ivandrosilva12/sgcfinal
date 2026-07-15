// internal/domain/recepcao/triagem.go
package recepcao

import (
	"context"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Triagem é um agregado raiz do BC Recepção: o registo clínico da triagem de uma
// chegada — a prioridade de Manchester e os sinais vitais. É imutável após criação (não
// tem máquina de estados). Refere a chegada e o enfermeiro por id.
type Triagem struct {
	id           string
	chegadaID    string
	prioridade   PrioridadeManchester
	sinaisVitais SinaisVitais
	observacoes  string
	enfermeiroID string
	triadaEm     time.Time
	criadoEm     time.Time
}

// NovaTriagem valida e constrói uma triagem. Chegada, enfermeiro, uma prioridade válida
// e o instante são obrigatórios; os sinais vitais assumem-se já validados (VO
// SinaisVitais). O enfermeiro é o sujeito autenticado (na aplicação), nunca do corpo.
func NovaTriagem(chegadaID, enfermeiroID string, prioridade PrioridadeManchester, sinais SinaisVitais, observacoes string, em time.Time) (*Triagem, error) {
	chegadaID = strings.TrimSpace(chegadaID)
	if chegadaID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "chegada da triagem em falta")
	}
	enfermeiroID = strings.TrimSpace(enfermeiroID)
	if enfermeiroID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "enfermeiro da triagem em falta")
	}
	p, err := ParsePrioridade(string(prioridade))
	if err != nil {
		return nil, err
	}
	if em.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "instante da triagem em falta")
	}
	return &Triagem{
		chegadaID: chegadaID, enfermeiroID: enfermeiroID, prioridade: p,
		sinaisVitais: sinais, observacoes: strings.TrimSpace(observacoes), triadaEm: em,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados.
func (t *Triagem) ID() string { return t.id }

// ChegadaID devolve a chegada triada.
func (t *Triagem) ChegadaID() string { return t.chegadaID }

// Prioridade devolve a cor de Manchester.
func (t *Triagem) Prioridade() PrioridadeManchester { return t.prioridade }

// SinaisVitais devolve os sinais vitais registados.
func (t *Triagem) SinaisVitais() SinaisVitais { return t.sinaisVitais }

// Observacoes devolve as observações livres (vazio se não houver).
func (t *Triagem) Observacoes() string { return t.observacoes }

// EnfermeiroID devolve o enfermeiro triador.
func (t *Triagem) EnfermeiroID() string { return t.enfermeiroID }

// TriadaEm devolve o instante da triagem.
func (t *Triagem) TriadaEm() time.Time { return t.triadaEm }

// SnapshotTriagem carrega o estado completo para persistência ou rehidratação.
type SnapshotTriagem struct {
	ID           string
	ChegadaID    string
	Prioridade   PrioridadeManchester
	SinaisVitais SinaisVitais
	Observacoes  string
	EnfermeiroID string
	TriadaEm     time.Time
	CriadoEm     time.Time
}

// Snapshot devolve o estado completo do agregado.
func (t *Triagem) Snapshot() SnapshotTriagem {
	return SnapshotTriagem{
		ID: t.id, ChegadaID: t.chegadaID, Prioridade: t.prioridade,
		SinaisVitais: t.sinaisVitais, Observacoes: t.observacoes,
		EnfermeiroID: t.enfermeiroID, TriadaEm: t.triadaEm, CriadoEm: t.criadoEm,
	}
}

// ReconstruirTriagem reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirTriagem(s SnapshotTriagem) *Triagem {
	return &Triagem{
		id: s.ID, chegadaID: s.ChegadaID, prioridade: s.Prioridade,
		sinaisVitais: s.SinaisVitais, observacoes: s.Observacoes,
		enfermeiroID: s.EnfermeiroID, triadaEm: s.TriadaEm, criadoEm: s.CriadoEm,
	}
}

// ResumoFilaClinica é a projecção de leitura de uma linha da fila clínica (chegada
// triada à espera do médico).
type ResumoFilaClinica struct {
	ChegadaID       string    `json:"chegada_id"`
	TriagemID       string    `json:"triagem_id"`
	DoenteID        string    `json:"doente_id"`
	MedicoID        string    `json:"medico_id"`
	EspecialidadeID string    `json:"especialidade_id"`
	Prioridade      string    `json:"prioridade"`
	HoraChegada     time.Time `json:"hora_chegada"`
	TriadaEm        time.Time `json:"triada_em"`
}

// RepositorioTriagens é a porta de saída de persistência de triagens.
//
// RegistarTriagem grava, numa única transacção, a chegada a passar a TRIADO (guarda
// compare-and-set sobre CHAMADO, com o médico atribuído) e a nova triagem — um registo
// que transitasse a chegada sem criar a triagem (ou vice-versa) deixaria a recepção
// incoerente. ListarFilaClinica devolve as chegadas TRIADO ordenadas por severidade de
// Manchester (mais urgente primeiro) e depois por hora de chegada; médico vazio = todos.
type RepositorioTriagens interface {
	RegistarTriagem(ctx context.Context, triagem *Triagem, chegada *Chegada) (string, error)
	ObterPorChegada(ctx context.Context, chegadaID string) (*Triagem, error)
	ListarFilaClinica(ctx context.Context, medicoID string) ([]ResumoFilaClinica, error)
}
