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
