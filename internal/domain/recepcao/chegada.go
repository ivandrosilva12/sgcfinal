// internal/domain/recepcao/chegada.go
package recepcao

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// EstadoChegada é o estado do ciclo de vida de uma chegada (o doente na fila).
//
//	AGUARDA ─┬─ Chamar ──────────► CHAMADO   (entrega à triagem/consulta)
//	         └─ RegistarDesistencia► DESISTIU
type EstadoChegada string

const (
	ChegAguarda  EstadoChegada = "AGUARDA"
	ChegChamado  EstadoChegada = "CHAMADO"
	ChegDesistiu EstadoChegada = "DESISTIU"
)

// Chegada é um agregado raiz do BC Recepção: o doente presente na clínica hoje, à
// espera de ser atendido. Nasce de um check-in de marcação (com marcacaoID e médico) ou
// de um walk-in (sem marcação nem médico). Refere doente/marcação/médico/especialidade
// por id. O id é gerado pela base de dados.
type Chegada struct {
	id              string
	doenteID        string
	marcacaoID      string
	especialidadeID string
	medicoID        string
	horaChegada     time.Time
	estado          EstadoChegada
	estadoAnterior  EstadoChegada
	criadoEm        time.Time
	actualizadoEm   time.Time
}

// NovaChegadaAgendada constrói a chegada de um doente com marcação (check-in). Doente,
// marcação, médico, especialidade e hora são todos obrigatórios (o médico e a
// especialidade vêm da marcação). Estado inicial AGUARDA.
func NovaChegadaAgendada(doenteID, marcacaoID, medicoID, especialidadeID string, hora time.Time) (*Chegada, error) {
	doenteID = strings.TrimSpace(doenteID)
	if doenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente da chegada em falta")
	}
	marcacaoID = strings.TrimSpace(marcacaoID)
	if marcacaoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "marcação da chegada em falta")
	}
	medicoID = strings.TrimSpace(medicoID)
	if medicoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "médico da chegada em falta")
	}
	especialidadeID = strings.TrimSpace(especialidadeID)
	if especialidadeID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "especialidade da chegada em falta")
	}
	if hora.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "hora da chegada em falta")
	}
	return &Chegada{
		doenteID: doenteID, marcacaoID: marcacaoID, medicoID: medicoID,
		especialidadeID: especialidadeID, horaChegada: hora, estado: ChegAguarda,
	}, nil
}

// NovaChegadaWalkIn constrói a chegada de um doente sem marcação (walk-in). Só o doente,
// a especialidade e a hora são obrigatórios; o médico fica por atribuir. Estado inicial
// AGUARDA.
func NovaChegadaWalkIn(doenteID, especialidadeID string, hora time.Time) (*Chegada, error) {
	doenteID = strings.TrimSpace(doenteID)
	if doenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente da chegada em falta")
	}
	especialidadeID = strings.TrimSpace(especialidadeID)
	if especialidadeID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "especialidade da chegada em falta")
	}
	if hora.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "hora da chegada em falta")
	}
	return &Chegada{
		doenteID: doenteID, especialidadeID: especialidadeID, horaChegada: hora,
		estado: ChegAguarda,
	}, nil
}

// Chamar transita AGUARDA → CHAMADO (o doente é chamado para ser atendido).
func (c *Chegada) Chamar(em time.Time) error {
	if c.estado != ChegAguarda {
		return erros.Novo(erros.CategoriaConflito, "só é possível chamar uma chegada em espera")
	}
	c.estado = ChegChamado
	c.actualizadoEm = em
	return nil
}

// RegistarDesistencia transita AGUARDA → DESISTIU (o doente foi embora antes de ser
// chamado).
func (c *Chegada) RegistarDesistencia(em time.Time) error {
	if c.estado != ChegAguarda {
		return erros.Novo(erros.CategoriaConflito, "só é possível registar a desistência de uma chegada em espera")
	}
	c.estado = ChegDesistiu
	c.actualizadoEm = em
	return nil
}

// ID devolve o identificador atribuído pela base de dados.
func (c *Chegada) ID() string { return c.id }

// DoenteID devolve o doente da chegada.
func (c *Chegada) DoenteID() string { return c.doenteID }

// MarcacaoID devolve a marcação de origem (vazio no walk-in).
func (c *Chegada) MarcacaoID() string { return c.marcacaoID }

// MedicoID devolve o médico (vazio no walk-in).
func (c *Chegada) MedicoID() string { return c.medicoID }

// EspecialidadeID devolve a especialidade da chegada.
func (c *Chegada) EspecialidadeID() string { return c.especialidadeID }

// HoraChegada devolve o instante da chegada.
func (c *Chegada) HoraChegada() time.Time { return c.horaChegada }

// Estado devolve o estado actual.
func (c *Chegada) Estado() EstadoChegada { return c.estado }

// SnapshotChegada carrega o estado completo para persistência ou rehidratação.
//
// EstadoAnterior é o estado lido da base de dados (vazio num agregado novo); o
// repositório usa-o como guarda compare-and-set. É derivado — quem reconstrói não o
// preenche.
type SnapshotChegada struct {
	ID              string
	DoenteID        string
	MarcacaoID      string
	EspecialidadeID string
	MedicoID        string
	HoraChegada     time.Time
	Estado          EstadoChegada
	EstadoAnterior  EstadoChegada
	CriadoEm        time.Time
	ActualizadoEm   time.Time
}

// Snapshot devolve o estado completo do agregado.
func (c *Chegada) Snapshot() SnapshotChegada {
	return SnapshotChegada{
		ID: c.id, DoenteID: c.doenteID, MarcacaoID: c.marcacaoID,
		EspecialidadeID: c.especialidadeID, MedicoID: c.medicoID, HoraChegada: c.horaChegada,
		Estado: c.estado, EstadoAnterior: c.estadoAnterior,
		CriadoEm: c.criadoEm, ActualizadoEm: c.actualizadoEm,
	}
}

// ReconstruirChegada reconstrói o agregado a partir de um snapshot persistido.
// EstadoAnterior é fixado no estado lido.
func ReconstruirChegada(s SnapshotChegada) *Chegada {
	return &Chegada{
		id: s.ID, doenteID: s.DoenteID, marcacaoID: s.MarcacaoID,
		especialidadeID: s.EspecialidadeID, medicoID: s.MedicoID, horaChegada: s.HoraChegada,
		estado: s.Estado, estadoAnterior: s.Estado,
		criadoEm: s.CriadoEm, actualizadoEm: s.ActualizadoEm,
	}
}
