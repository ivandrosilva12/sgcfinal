// internal/domain/recepcao/janela.go
package recepcao

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// JanelaDisponibilidade é a agenda declarada de um médico: um intervalo datado onde
// é possível marcar consultas dessa especialidade. É um agregado sem máquina de
// estados — existe ou é removido. O id é gerado pela base de dados.
type JanelaDisponibilidade struct {
	id              string
	medicoID        string
	especialidadeID string
	inicio          time.Time
	fim             time.Time
	criadoEm        time.Time
}

// NovaJanela valida e constrói uma janela de disponibilidade. Médico, especialidade e
// um intervalo com fim estritamente posterior ao início são obrigatórios.
func NovaJanela(medicoID, especialidadeID string, inicio, fim time.Time) (*JanelaDisponibilidade, error) {
	medicoID = strings.TrimSpace(medicoID)
	if medicoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "médico da janela em falta")
	}
	especialidadeID = strings.TrimSpace(especialidadeID)
	if especialidadeID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "especialidade da janela em falta")
	}
	if inicio.IsZero() || fim.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "intervalo da janela em falta")
	}
	if !fim.After(inicio) {
		return nil, erros.Novo(erros.CategoriaValidacao, "o fim da janela tem de ser posterior ao início")
	}
	return &JanelaDisponibilidade{
		medicoID: medicoID, especialidadeID: especialidadeID, inicio: inicio, fim: fim,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados (vazio se não persistido).
func (j *JanelaDisponibilidade) ID() string { return j.id }

// MedicoID devolve o médico a que a janela pertence.
func (j *JanelaDisponibilidade) MedicoID() string { return j.medicoID }

// EspecialidadeID devolve a especialidade da janela.
func (j *JanelaDisponibilidade) EspecialidadeID() string { return j.especialidadeID }

// Inicio devolve o início da janela.
func (j *JanelaDisponibilidade) Inicio() time.Time { return j.inicio }

// Fim devolve o fim da janela.
func (j *JanelaDisponibilidade) Fim() time.Time { return j.fim }

// SnapshotJanela carrega o estado completo para persistência ou rehidratação.
type SnapshotJanela struct {
	ID              string
	MedicoID        string
	EspecialidadeID string
	Inicio          time.Time
	Fim             time.Time
	CriadoEm        time.Time
}

// Snapshot devolve o estado completo do agregado.
func (j *JanelaDisponibilidade) Snapshot() SnapshotJanela {
	return SnapshotJanela{
		ID: j.id, MedicoID: j.medicoID, EspecialidadeID: j.especialidadeID,
		Inicio: j.inicio, Fim: j.fim, CriadoEm: j.criadoEm,
	}
}

// ReconstruirJanela reconstrói um agregado a partir de um snapshot persistido (dados
// de fonte confiável — não revalida invariantes).
func ReconstruirJanela(s SnapshotJanela) *JanelaDisponibilidade {
	return &JanelaDisponibilidade{
		id: s.ID, medicoID: s.MedicoID, especialidadeID: s.EspecialidadeID,
		inicio: s.Inicio, fim: s.Fim, criadoEm: s.CriadoEm,
	}
}
