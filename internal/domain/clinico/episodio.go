package clinico

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// EpisodioClinico é um agregado raiz do BC Clínico: um episódio de cuidados
// (consulta, urgência ou internamento) de um doente. Referencia o doente por id
// (agregado independente). O id é gerado pela base de dados.
type EpisodioClinico struct {
	id              string
	doenteID        string
	tipo            TipoEpisodio
	especialidadeID string
	medicoID        string
	inicio          time.Time
	fim             *time.Time
	nota            NotaClinica
	diagnosticosCID []DiagnosticoCID
	estado          EstadoEpisodio
	criadoEm        time.Time
	actualizadoEm   time.Time
	fechadoEm       *time.Time
	fechadoPor      string
}

// NovoEpisodio valida e constrói um episódio no estado ABERTO. O doente,
// a especialidade, o médico e o início são obrigatórios.
func NovoEpisodio(doenteID string, tipo TipoEpisodio, especialidadeID, medicoID string, inicio time.Time) (*EpisodioClinico, error) {
	doenteID = strings.TrimSpace(doenteID)
	if doenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente do episódio em falta")
	}
	if _, err := ParseTipoEpisodio(string(tipo)); err != nil {
		return nil, err
	}
	especialidadeID = strings.TrimSpace(especialidadeID)
	if especialidadeID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "especialidade do episódio em falta")
	}
	medicoID = strings.TrimSpace(medicoID)
	if medicoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "médico do episódio em falta")
	}
	if inicio.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "início do episódio em falta")
	}
	return &EpisodioClinico{
		doenteID:        doenteID,
		tipo:            tipo,
		especialidadeID: especialidadeID,
		medicoID:        medicoID,
		inicio:          inicio,
		estado:          EstadoEpisodioAberto,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados (vazio se não persistido).
func (e *EpisodioClinico) ID() string { return e.id }

// DoenteID devolve o id do doente a que o episódio pertence.
func (e *EpisodioClinico) DoenteID() string { return e.doenteID }

// Estado devolve o estado actual do episódio.
func (e *EpisodioClinico) Estado() EstadoEpisodio { return e.estado }

// ActualizarNota substitui a nota clínica. Só permitido em ABERTO.
func (e *EpisodioClinico) ActualizarNota(n NotaClinica) error {
	if e.estado != EstadoEpisodioAberto {
		return erros.Novo(erros.CategoriaConflito, "só é possível alterar um episódio aberto")
	}
	e.nota = n
	return nil
}

// DefinirDiagnosticosCID substitui a lista de diagnósticos CID. Só permitido em
// ABERTO; valida cada CID e admite no máximo um principal.
func (e *EpisodioClinico) DefinirDiagnosticosCID(cids []DiagnosticoCID) error {
	if e.estado != EstadoEpisodioAberto {
		return erros.Novo(erros.CategoriaConflito, "só é possível alterar um episódio aberto")
	}
	principais := 0
	for _, c := range cids {
		if _, err := NovoDiagnosticoCID(c.CID, c.Principal); err != nil {
			return err
		}
		if c.Principal {
			principais++
		}
	}
	if principais > 1 {
		return erros.Novo(erros.CategoriaValidacao, "só pode existir um diagnóstico principal")
	}
	e.diagnosticosCID = cids
	return nil
}

// Fechar encerra o episódio. Só de ABERTO; exige nota clínica completa e pelo
// menos um diagnóstico CID.
func (e *EpisodioClinico) Fechar(fechadoPor string, em time.Time) error {
	if e.estado != EstadoEpisodioAberto {
		return erros.Novo(erros.CategoriaConflito, "só é possível fechar um episódio aberto")
	}
	if !e.nota.Completa() {
		return erros.Novo(erros.CategoriaValidacao, "a nota clínica (queixa, exame, diagnóstico e plano) é obrigatória para fechar o episódio")
	}
	if len(e.diagnosticosCID) == 0 {
		return erros.Novo(erros.CategoriaValidacao, "é obrigatório pelo menos um diagnóstico CID para fechar o episódio")
	}
	fechadoPor = strings.TrimSpace(fechadoPor)
	if fechadoPor == "" {
		return erros.Novo(erros.CategoriaValidacao, "autor do fecho em falta")
	}
	e.estado = EstadoEpisodioFechado
	e.fim = &em
	e.fechadoEm = &em
	e.fechadoPor = fechadoPor
	return nil
}

// Cancelar cancela o episódio. Só de ABERTO. O motivo é auditado na aplicação.
func (e *EpisodioClinico) Cancelar(em time.Time) error {
	if e.estado != EstadoEpisodioAberto {
		return erros.Novo(erros.CategoriaConflito, "só é possível cancelar um episódio aberto")
	}
	e.estado = EstadoEpisodioCancelado
	e.fim = &em
	return nil
}

// SnapshotEpisodio carrega o estado completo de um episódio para persistência ou
// rehidratação (dados de fonte confiável — não revalida invariantes).
type SnapshotEpisodio struct {
	ID              string
	DoenteID        string
	Tipo            TipoEpisodio
	EspecialidadeID string
	MedicoID        string
	Inicio          time.Time
	Fim             *time.Time
	Nota            NotaClinica
	DiagnosticosCID []DiagnosticoCID
	Estado          EstadoEpisodio
	CriadoEm        time.Time
	ActualizadoEm   time.Time
	FechadoEm       *time.Time
	FechadoPor      string
}

// Snapshot devolve o estado completo do agregado.
func (e *EpisodioClinico) Snapshot() SnapshotEpisodio {
	return SnapshotEpisodio{
		ID:              e.id,
		DoenteID:        e.doenteID,
		Tipo:            e.tipo,
		EspecialidadeID: e.especialidadeID,
		MedicoID:        e.medicoID,
		Inicio:          e.inicio,
		Fim:             e.fim,
		Nota:            e.nota,
		DiagnosticosCID: e.diagnosticosCID,
		Estado:          e.estado,
		CriadoEm:        e.criadoEm,
		ActualizadoEm:   e.actualizadoEm,
		FechadoEm:       e.fechadoEm,
		FechadoPor:      e.fechadoPor,
	}
}

// ReconstruirEpisodio reconstrói um agregado a partir de um snapshot persistido.
func ReconstruirEpisodio(s SnapshotEpisodio) *EpisodioClinico {
	return &EpisodioClinico{
		id:              s.ID,
		doenteID:        s.DoenteID,
		tipo:            s.Tipo,
		especialidadeID: s.EspecialidadeID,
		medicoID:        s.MedicoID,
		inicio:          s.Inicio,
		fim:             s.Fim,
		nota:            s.Nota,
		diagnosticosCID: s.DiagnosticosCID,
		estado:          s.Estado,
		criadoEm:        s.CriadoEm,
		actualizadoEm:   s.ActualizadoEm,
		fechadoEm:       s.FechadoEm,
		fechadoPor:      s.FechadoPor,
	}
}
