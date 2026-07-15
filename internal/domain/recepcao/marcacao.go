// internal/domain/recepcao/marcacao.go
package recepcao

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// EstadoMarcacao é o estado do ciclo de vida de uma marcação.
//
//	MARCADA ─┬─ Cancelar ──────────► CANCELADA
//	         ├─ Remarcar ──────────► REMARCADA  (+ nova Marcacao MARCADA)
//	         ├─ RegistarFalta ─────► FALTOU
//	         └─ RegistarComparencia► COMPARECEU (check-in do doente)
type EstadoMarcacao string

const (
	MarcMarcada    EstadoMarcacao = "MARCADA"
	MarcCancelada  EstadoMarcacao = "CANCELADA"
	MarcRemarcada  EstadoMarcacao = "REMARCADA"
	MarcFaltou     EstadoMarcacao = "FALTOU"
	MarcCompareceu EstadoMarcacao = "COMPARECEU"
)

// Marcacao é o agregado raiz do BC Recepção: uma consulta agendada para um doente,
// com um médico e uma especialidade, num intervalo. Refere doente/médico/especialidade
// por id (agregados de outros contextos). O id é gerado pela base de dados.
type Marcacao struct {
	id              string
	doenteID        string
	medicoID        string
	especialidadeID string
	inicio          time.Time
	fim             time.Time
	estado          EstadoMarcacao
	estadoAnterior  EstadoMarcacao
	motivo          string
	remarcaDe       string
	criadoEm        time.Time
	actualizadoEm   time.Time
}

// NovaMarcacao valida e constrói uma marcação no estado MARCADA. Doente, médico,
// especialidade e um intervalo com fim posterior ao início são obrigatórios. A
// verificação de disponibilidade (janela livre, sem sobreposição, não no passado) é
// feita pelo caso de uso com VerificarDisponibilidade — não aqui, porque cruza outros
// agregados.
func NovaMarcacao(doenteID, medicoID, especialidadeID string, inicio, fim time.Time) (*Marcacao, error) {
	doenteID = strings.TrimSpace(doenteID)
	if doenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente da marcação em falta")
	}
	medicoID = strings.TrimSpace(medicoID)
	if medicoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "médico da marcação em falta")
	}
	especialidadeID = strings.TrimSpace(especialidadeID)
	if especialidadeID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "especialidade da marcação em falta")
	}
	if inicio.IsZero() || fim.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "intervalo da marcação em falta")
	}
	if !fim.After(inicio) {
		return nil, erros.Novo(erros.CategoriaValidacao, "o fim da marcação tem de ser posterior ao início")
	}
	return &Marcacao{
		doenteID: doenteID, medicoID: medicoID, especialidadeID: especialidadeID,
		inicio: inicio, fim: fim, estado: MarcMarcada,
	}, nil
}

// Cancelar transita MARCADA → CANCELADA. O motivo é obrigatório: um cancelamento sem
// razão registada não é auditável.
func (m *Marcacao) Cancelar(motivo string, em time.Time) error {
	if m.estado != MarcMarcada {
		return erros.Novo(erros.CategoriaConflito, "só é possível cancelar uma marcação em estado MARCADA")
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return erros.Novo(erros.CategoriaValidacao, "motivo do cancelamento em falta")
	}
	m.estado = MarcCancelada
	m.motivo = motivo
	m.actualizadoEm = em
	return nil
}

// Remarcar transita a marcação receptora MARCADA → REMARCADA e devolve uma NOVA
// marcação MARCADA para o novo intervalo, apontando para a original (RemarcaDe). O
// histórico da original é preservado. A disponibilidade do novo intervalo é verificada
// pelo caso de uso. A original tem de já estar persistida (ter id) para que a nova a
// possa referenciar.
func (m *Marcacao) Remarcar(novoInicio, novoFim, em time.Time) (*Marcacao, error) {
	if m.estado != MarcMarcada {
		return nil, erros.Novo(erros.CategoriaConflito, "só é possível remarcar uma marcação em estado MARCADA")
	}
	if m.id == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "a marcação original tem de estar persistida para ser remarcada")
	}
	nova, err := NovaMarcacao(m.doenteID, m.medicoID, m.especialidadeID, novoInicio, novoFim)
	if err != nil {
		return nil, err
	}
	nova.remarcaDe = m.id
	m.estado = MarcRemarcada
	m.actualizadoEm = em
	return nova, nil
}

// RegistarFalta transita MARCADA → FALTOU. Só é possível depois da hora marcada
// (em >= fim): registar falta antes de a consulta acabar não faz sentido.
func (m *Marcacao) RegistarFalta(em time.Time) error {
	if m.estado != MarcMarcada {
		return erros.Novo(erros.CategoriaConflito, "só é possível registar falta de uma marcação em estado MARCADA")
	}
	if em.Before(m.fim) {
		return erros.Novo(erros.CategoriaRegraNegocio, "só é possível registar falta depois da hora marcada")
	}
	m.estado = MarcFaltou
	m.actualizadoEm = em
	return nil
}

// RegistarComparencia transita MARCADA → COMPARECEU (o doente chegou e fez check-in).
// Desfecho simétrico ao FALTOU: depois de comparecer, a marcação já não pode ser
// cancelada, remarcada nem dar falta (essas transições continuam a exigir MARCADA).
func (m *Marcacao) RegistarComparencia(em time.Time) error {
	if m.estado != MarcMarcada {
		return erros.Novo(erros.CategoriaConflito, "só é possível registar a comparência de uma marcação em estado MARCADA")
	}
	m.estado = MarcCompareceu
	m.actualizadoEm = em
	return nil
}

// ID devolve o identificador atribuído pela base de dados.
func (m *Marcacao) ID() string { return m.id }

// DoenteID devolve o doente da marcação.
func (m *Marcacao) DoenteID() string { return m.doenteID }

// MedicoID devolve o médico da marcação.
func (m *Marcacao) MedicoID() string { return m.medicoID }

// EspecialidadeID devolve a especialidade da marcação.
func (m *Marcacao) EspecialidadeID() string { return m.especialidadeID }

// Inicio devolve o início da marcação.
func (m *Marcacao) Inicio() time.Time { return m.inicio }

// Fim devolve o fim da marcação.
func (m *Marcacao) Fim() time.Time { return m.fim }

// Estado devolve o estado actual.
func (m *Marcacao) Estado() EstadoMarcacao { return m.estado }

// RemarcaDe devolve o id da marcação original que esta remarca (vazio se não for uma
// remarcação).
func (m *Marcacao) RemarcaDe() string { return m.remarcaDe }

// SnapshotMarcacao carrega o estado completo para persistência ou rehidratação.
//
// EstadoAnterior é o estado lido da base de dados (vazio num agregado novo). O
// repositório usa-o como guarda compare-and-set no UPDATE de transição. É derivado —
// quem reconstrói não o preenche.
type SnapshotMarcacao struct {
	ID              string
	DoenteID        string
	MedicoID        string
	EspecialidadeID string
	Inicio          time.Time
	Fim             time.Time
	Estado          EstadoMarcacao
	EstadoAnterior  EstadoMarcacao
	Motivo          string
	RemarcaDe       string
	CriadoEm        time.Time
	ActualizadoEm   time.Time
}

// Snapshot devolve o estado completo do agregado.
func (m *Marcacao) Snapshot() SnapshotMarcacao {
	return SnapshotMarcacao{
		ID: m.id, DoenteID: m.doenteID, MedicoID: m.medicoID, EspecialidadeID: m.especialidadeID,
		Inicio: m.inicio, Fim: m.fim, Estado: m.estado, EstadoAnterior: m.estadoAnterior,
		Motivo: m.motivo, RemarcaDe: m.remarcaDe, CriadoEm: m.criadoEm, ActualizadoEm: m.actualizadoEm,
	}
}

// ReconstruirMarcacao reconstrói o agregado a partir de um snapshot persistido.
// EstadoAnterior é fixado no estado lido — qualquer transição posterior deixa-o a
// apontar para o estado que está na base de dados.
func ReconstruirMarcacao(s SnapshotMarcacao) *Marcacao {
	return &Marcacao{
		id: s.ID, doenteID: s.DoenteID, medicoID: s.MedicoID, especialidadeID: s.EspecialidadeID,
		inicio: s.Inicio, fim: s.Fim, estado: s.Estado, estadoAnterior: s.Estado,
		motivo: s.Motivo, remarcaDe: s.RemarcaDe, criadoEm: s.CriadoEm, actualizadoEm: s.ActualizadoEm,
	}
}
