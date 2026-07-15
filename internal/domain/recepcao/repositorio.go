// internal/domain/recepcao/repositorio.go
package recepcao

import (
	"context"
	"time"
)

// ResumoMarcacao é a projecção de leitura de uma marcação.
type ResumoMarcacao struct {
	ID              string    `json:"id"`
	DoenteID        string    `json:"doente_id"`
	MedicoID        string    `json:"medico_id"`
	EspecialidadeID string    `json:"especialidade_id"`
	Estado          string    `json:"estado"`
	Motivo          string    `json:"motivo,omitempty"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
	CriadoEm        time.Time `json:"criado_em"`
}

// ResumoChegada é a projecção de leitura de uma chegada (linha da fila).
type ResumoChegada struct {
	ID              string    `json:"id"`
	DoenteID        string    `json:"doente_id"`
	MarcacaoID      string    `json:"marcacao_id,omitempty"`
	MedicoID        string    `json:"medico_id,omitempty"`
	EspecialidadeID string    `json:"especialidade_id"`
	Estado          string    `json:"estado"`
	HoraChegada     time.Time `json:"hora_chegada"`
}

// RepositorioJanelas é a porta de saída de persistência de janelas de disponibilidade.
// ListarPorMedicoIntervalo devolve as janelas do médico que SE SOBREPÕEM ao intervalo
// [de,ate] (não apenas as inteiramente contidas): é sobre essas que
// VerificarDisponibilidade decide o encaixe.
type RepositorioJanelas interface {
	Guardar(ctx context.Context, j *JanelaDisponibilidade) (string, error)
	ObterPorID(ctx context.Context, id string) (*JanelaDisponibilidade, error)
	ListarPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]JanelaDisponibilidade, error)
	Remover(ctx context.Context, id string) error
}

// RepositorioMarcacoes é a porta de saída de persistência de marcações.
//
// Transitar aplica a transição de estado com guarda compare-and-set (usa
// EstadoAnterior do snapshot). Remarcar grava, numa única transacção, a original a
// passar a REMARCADA e a nova MARCADA — uma marcação remarcada sem a nova deixaria o
// doente sem consulta.
//
// ListarActivasPorMedicoIntervalo devolve os agregados das marcações MARCADA do médico
// que se sobrepõem ao intervalo (para VerificarDisponibilidade). ListarPorMedicoIntervalo
// e ListarPorDoente devolvem read-models de TODOS os estados (para a agenda/consulta).
type RepositorioMarcacoes interface {
	Guardar(ctx context.Context, m *Marcacao) (string, error)
	ObterPorID(ctx context.Context, id string) (*Marcacao, error)
	Transitar(ctx context.Context, m *Marcacao) error
	Remarcar(ctx context.Context, original, nova *Marcacao) (string, error)
	ListarActivasPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]Marcacao, error)
	ListarPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]ResumoMarcacao, error)
	ListarPorDoente(ctx context.Context, doenteID string) ([]ResumoMarcacao, error)
}

// RepositorioChegadas é a porta de saída de persistência de chegadas.
//
// RegistarChegadaAgendada grava, numa única transacção, a marcação a passar a
// COMPARECEU (guarda compare-and-set sobre MARCADA) e a nova chegada — um check-in que
// transitasse a marcação sem criar a chegada (ou vice-versa) deixaria a recepção
// incoerente. Transitar aplica a transição de estado da chegada (CAS). ListarFila
// devolve as chegadas em AGUARDA (fila), ordenadas por hora de chegada; especialidade
// vazia = todas.
type RepositorioChegadas interface {
	Guardar(ctx context.Context, c *Chegada) (string, error)
	RegistarChegadaAgendada(ctx context.Context, chegada *Chegada, marcacao *Marcacao) (string, error)
	ObterPorID(ctx context.Context, id string) (*Chegada, error)
	Transitar(ctx context.Context, c *Chegada) error
	ListarFila(ctx context.Context, especialidadeID string) ([]ResumoChegada, error)
}
