// Package recepcao contém os casos de uso do BC Recepção (Camada 2 — Aplicação).
package recepcao

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// Auditor persiste registos de auditoria de forma append-only.
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}

// LeitorDoente é a porta anti-corrupção para leitura do BC Clínico. A Recepção nunca
// importa tipos do domínio Clínico: só faz esta pergunta booleana.
type LeitorDoente interface {
	// DoenteActivo indica se o doente existe e está activo.
	DoenteActivo(ctx context.Context, doenteID string) (bool, error)
}

// Reexports dos read-models do domínio.
type (
	ResumoMarcacao    = dominio.ResumoMarcacao
	ResumoChegada     = dominio.ResumoChegada
	ResumoFilaClinica = dominio.ResumoFilaClinica
)

// DadosDefinirJanela é a entrada da definição de uma janela. O MedicoID vem do
// caminho (:mid); a especialidade e o intervalo vêm do corpo.
type DadosDefinirJanela struct {
	MedicoID        string
	EspecialidadeID string    `json:"especialidade_id"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
}

// DetalheJanela é o detalhe de uma janela numa resposta.
type DetalheJanela struct {
	ID              string    `json:"id"`
	MedicoID        string    `json:"medico_id"`
	EspecialidadeID string    `json:"especialidade_id"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
}

// DadosMarcar é a entrada de uma marcação. Todos os ids vêm do corpo; o actor
// (quem marca) vem da sessão, não daqui.
type DadosMarcar struct {
	DoenteID        string    `json:"doente_id"`
	MedicoID        string    `json:"medico_id"`
	EspecialidadeID string    `json:"especialidade_id"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
}

// DadosRemarcar é a entrada de uma remarcação (novo intervalo).
type DadosRemarcar struct {
	Inicio time.Time `json:"inicio"`
	Fim    time.Time `json:"fim"`
}

// DetalheMarcacao é o detalhe de uma marcação numa resposta.
type DetalheMarcacao struct {
	ID              string    `json:"id"`
	DoenteID        string    `json:"doente_id"`
	MedicoID        string    `json:"medico_id"`
	EspecialidadeID string    `json:"especialidade_id"`
	Estado          string    `json:"estado"`
	Motivo          string    `json:"motivo,omitempty"`
	RemarcaDe       string    `json:"remarca_de,omitempty"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
}

// Agenda é a leitura combinada da agenda de um médico num intervalo.
type Agenda struct {
	Janelas   []DetalheJanela  `json:"janelas"`
	Marcacoes []ResumoMarcacao `json:"marcacoes"`
}

// paraDetalheJanela projecta o agregado para o read-model de resposta.
func paraDetalheJanela(j *dominio.JanelaDisponibilidade) DetalheJanela {
	s := j.Snapshot()
	return DetalheJanela{
		ID: s.ID, MedicoID: s.MedicoID, EspecialidadeID: s.EspecialidadeID,
		Inicio: s.Inicio, Fim: s.Fim,
	}
}

// paraDetalheMarcacao projecta o agregado para o read-model de resposta.
func paraDetalheMarcacao(m *dominio.Marcacao) DetalheMarcacao {
	s := m.Snapshot()
	return DetalheMarcacao{
		ID: s.ID, DoenteID: s.DoenteID, MedicoID: s.MedicoID, EspecialidadeID: s.EspecialidadeID,
		Estado: string(s.Estado), Motivo: s.Motivo, RemarcaDe: s.RemarcaDe,
		Inicio: s.Inicio, Fim: s.Fim,
	}
}

// DadosWalkIn é a entrada de um walk-in (doente sem marcação). O actor vem da sessão.
type DadosWalkIn struct {
	DoenteID        string `json:"doente_id"`
	EspecialidadeID string `json:"especialidade_id"`
}

// DetalheChegada é o detalhe de uma chegada numa resposta.
type DetalheChegada struct {
	ID              string    `json:"id"`
	DoenteID        string    `json:"doente_id"`
	MarcacaoID      string    `json:"marcacao_id,omitempty"`
	MedicoID        string    `json:"medico_id,omitempty"`
	EspecialidadeID string    `json:"especialidade_id"`
	Estado          string    `json:"estado"`
	HoraChegada     time.Time `json:"hora_chegada"`
}

// paraDetalheChegada projecta o agregado para o read-model de resposta.
func paraDetalheChegada(c *dominio.Chegada) DetalheChegada {
	s := c.Snapshot()
	return DetalheChegada{
		ID: s.ID, DoenteID: s.DoenteID, MarcacaoID: s.MarcacaoID, MedicoID: s.MedicoID,
		EspecialidadeID: s.EspecialidadeID, Estado: string(s.Estado), HoraChegada: s.HoraChegada,
	}
}

// DadosTriagem é a entrada do registo de uma triagem. Os sinais vitais são opcionais.
// O MedicoID só é usado no walk-in (atribuição). O enfermeiro triador vem da sessão.
type DadosTriagem struct {
	Prioridade             string   `json:"prioridade"`
	TensaoSistolica        *int     `json:"tensao_sistolica"`
	TensaoDiastolica       *int     `json:"tensao_diastolica"`
	FrequenciaCardiaca     *int     `json:"frequencia_cardiaca"`
	Temperatura            *float64 `json:"temperatura"`
	FrequenciaRespiratoria *int     `json:"frequencia_respiratoria"`
	SaturacaoO2            *int     `json:"saturacao_o2"`
	Dor                    *int     `json:"dor"`
	Glicemia               *int     `json:"glicemia"`
	Peso                   *float64 `json:"peso"`
	Observacoes            string   `json:"observacoes"`
	MedicoID               string   `json:"medico_id"`
}

// DetalheTriagem é o detalhe de uma triagem numa resposta.
type DetalheTriagem struct {
	ID           string               `json:"id"`
	ChegadaID    string               `json:"chegada_id"`
	EnfermeiroID string               `json:"enfermeiro_id"`
	Prioridade   string               `json:"prioridade"`
	SinaisVitais dominio.SinaisVitais `json:"sinais_vitais"`
	Observacoes  string               `json:"observacoes,omitempty"`
	TriadaEm     time.Time            `json:"triada_em"`
}

// paraDetalheTriagem projecta o agregado para o read-model de resposta.
func paraDetalheTriagem(t *dominio.Triagem) DetalheTriagem {
	s := t.Snapshot()
	return DetalheTriagem{
		ID: s.ID, ChegadaID: s.ChegadaID, EnfermeiroID: s.EnfermeiroID,
		Prioridade: string(s.Prioridade), SinaisVitais: s.SinaisVitais,
		Observacoes: s.Observacoes, TriadaEm: s.TriadaEm,
	}
}
