package clinico

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Doente é o agregado raiz do BC Clínico. Encapsula a identificação, os contactos,
// o estado do ciclo de vida e as entidades-filho (alergias e antecedentes). Os
// campos são privados: a construção (NovoDoente) e as transições garantem os
// invariantes. O id é gerado pela base de dados (vazio até persistir).
type Doente struct {
	id                string
	numProcesso       string
	identificacao     Identificacao
	contactos         Contactos
	nacionalidade     string
	grupoSanguineo    *GrupoSanguineo
	estado            EstadoDoente
	alergias          []Alergia
	antecedentes      []AntecedenteClinico
	criadoEm          time.Time
	actualizadoEm     time.Time
	desactivadoEm     *time.Time
	desactivadoMotivo string
	falecidoEm        *time.Time
	causaMorteCID     string
}

// NovoDoente valida e constrói um novo Doente no estado ACTIVO. O número de
// processo já vem resolvido (automático ou manual) da camada de aplicação.
// nacionalidade vazia assume "AO".
func NovoDoente(numProcesso string, ident Identificacao, contactos Contactos, nacionalidade string) (*Doente, error) {
	numProcesso = strings.TrimSpace(numProcesso)
	if numProcesso == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "número de processo em falta")
	}
	// Revalida os VOs recebidos (defesa em profundidade: garante que não foram
	// construídos por atribuição directa a partir de dados não validados).
	if _, err := NovaIdentificacao(ident.NomeCompleto, ident.DataNascimento, ident.Sexo, ident.BI, ident.NIF, ident.Passaporte); err != nil {
		return nil, err
	}
	contactosValidados, err := NovosContactos(contactos.Telefone, contactos.Email, contactos.Morada)
	if err != nil {
		return nil, err
	}
	nac := strings.TrimSpace(nacionalidade)
	if nac == "" {
		nac = "AO"
	}
	return &Doente{
		numProcesso:   numProcesso,
		identificacao: ident,
		contactos:     contactosValidados,
		nacionalidade: nac,
		estado:        EstadoActivo,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados (vazio se ainda não
// persistido).
func (d *Doente) ID() string { return d.id }

// NumProcesso devolve o número de processo do doente.
func (d *Doente) NumProcesso() string { return d.numProcesso }

// Estado devolve o estado actual do ciclo de vida.
func (d *Doente) Estado() EstadoDoente { return d.estado }

// AdicionarAlergia acrescenta uma alergia ao doente. Proibido se o doente estiver
// apagado.
func (d *Doente) AdicionarAlergia(a Alergia) error {
	if d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível alterar um doente apagado")
	}
	if _, err := NovaAlergia(a.Substancia, a.Severidade, a.ReaccaoTipica, a.ConfirmadaEm, a.Notas); err != nil {
		return err
	}
	d.alergias = append(d.alergias, a)
	return nil
}

// AdicionarAntecedente acrescenta um antecedente clínico ao doente. Proibido se o
// doente estiver apagado.
func (d *Doente) AdicionarAntecedente(a AntecedenteClinico) error {
	if d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível alterar um doente apagado")
	}
	if _, err := NovoAntecedente(a.Tipo, a.Descricao, a.CID, a.DataInicio, a.Activo, a.Notas); err != nil {
		return err
	}
	d.antecedentes = append(d.antecedentes, a)
	return nil
}

// Desactivar coloca o doente em INACTIVO com um motivo. Proíbe se já estiver
// falecido ou apagado.
func (d *Doente) Desactivar(motivo string, em time.Time) error {
	if d.estado == EstadoFalecido || d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível desactivar um doente falecido ou apagado")
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return erros.Novo(erros.CategoriaValidacao, "motivo de desactivação em falta")
	}
	d.estado = EstadoInactivo
	d.desactivadoEm = &em
	d.desactivadoMotivo = motivo
	return nil
}

// Reactivar repõe um doente inactivo em ACTIVO, limpando os campos de
// desactivação. Só válido a partir de INACTIVO.
func (d *Doente) Reactivar() error {
	if d.estado != EstadoInactivo {
		return erros.Novo(erros.CategoriaConflito, "apenas um doente inactivo pode ser reactivado")
	}
	d.estado = EstadoActivo
	d.desactivadoEm = nil
	d.desactivadoMotivo = ""
	return nil
}

// DeclararFalecido coloca o doente em FALECIDO com a data de óbito e a causa
// (código CID opcional). Proíbe se apagado; a data não pode ser futura.
func (d *Doente) DeclararFalecido(data time.Time, causaCID string) error {
	if d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível declarar falecido um doente apagado")
	}
	if data.After(time.Now()) {
		return erros.Novo(erros.CategoriaValidacao, "data de óbito não pode ser futura")
	}
	d.estado = EstadoFalecido
	d.falecidoEm = &data
	d.causaMorteCID = strings.TrimSpace(causaCID)
	return nil
}

// AtualizarIdentificacao substitui a identificação (já validada como VO). Proíbe
// se apagado.
func (d *Doente) AtualizarIdentificacao(ident Identificacao) error {
	if d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível alterar um doente apagado")
	}
	if _, err := NovaIdentificacao(ident.NomeCompleto, ident.DataNascimento, ident.Sexo, ident.BI, ident.NIF, ident.Passaporte); err != nil {
		return err
	}
	d.identificacao = ident
	return nil
}

// AtualizarContactos substitui os contactos (já validados como VO). Proíbe se
// apagado.
func (d *Doente) AtualizarContactos(ct Contactos) error {
	if d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível alterar um doente apagado")
	}
	ctValidados, err := NovosContactos(ct.Telefone, ct.Email, ct.Morada)
	if err != nil {
		return err
	}
	d.contactos = ctValidados
	return nil
}

// DefinirGrupoSanguineo define (ou limpa, com nil) o grupo sanguíneo.
func (d *Doente) DefinirGrupoSanguineo(g *GrupoSanguineo) {
	d.grupoSanguineo = g
}

// SnapshotDoente carrega o estado completo de um Doente para persistência ou
// rehidratação. Não valida invariantes — os dados vêm de fonte confiável
// (agregado em memória ou base de dados).
type SnapshotDoente struct {
	ID                string
	NumProcesso       string
	Identificacao     Identificacao
	Contactos         Contactos
	Nacionalidade     string
	GrupoSanguineo    *GrupoSanguineo
	Estado            EstadoDoente
	Alergias          []Alergia
	Antecedentes      []AntecedenteClinico
	CriadoEm          time.Time
	ActualizadoEm     time.Time
	DesactivadoEm     *time.Time
	DesactivadoMotivo string
	FalecidoEm        *time.Time
	CausaMorteCID     string
}

// Snapshot devolve o estado completo do agregado (para mapear DTOs ou persistir).
func (d *Doente) Snapshot() SnapshotDoente {
	return SnapshotDoente{
		ID:                d.id,
		NumProcesso:       d.numProcesso,
		Identificacao:     d.identificacao,
		Contactos:         d.contactos,
		Nacionalidade:     d.nacionalidade,
		GrupoSanguineo:    d.grupoSanguineo,
		Estado:            d.estado,
		Alergias:          d.alergias,
		Antecedentes:      d.antecedentes,
		CriadoEm:          d.criadoEm,
		ActualizadoEm:     d.actualizadoEm,
		DesactivadoEm:     d.desactivadoEm,
		DesactivadoMotivo: d.desactivadoMotivo,
		FalecidoEm:        d.falecidoEm,
		CausaMorteCID:     d.causaMorteCID,
	}
}

// ReconstruirDoente reconstrói um agregado a partir de um snapshot persistido
// (usado pelo repositório na leitura). Não revalida invariantes.
func ReconstruirDoente(s SnapshotDoente) *Doente {
	return &Doente{
		id:                s.ID,
		numProcesso:       s.NumProcesso,
		identificacao:     s.Identificacao,
		contactos:         s.Contactos,
		nacionalidade:     s.Nacionalidade,
		grupoSanguineo:    s.GrupoSanguineo,
		estado:            s.Estado,
		alergias:          s.Alergias,
		antecedentes:      s.Antecedentes,
		criadoEm:          s.CriadoEm,
		actualizadoEm:     s.ActualizadoEm,
		desactivadoEm:     s.DesactivadoEm,
		desactivadoMotivo: s.DesactivadoMotivo,
		falecidoEm:        s.FalecidoEm,
		causaMorteCID:     s.CausaMorteCID,
	}
}
