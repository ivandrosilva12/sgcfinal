package clinico

import (
	"context"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// ProcedimentoCirurgico é um agregado raiz do BC Clínico: um procedimento
// cirúrgico ambulatório de um episódio. O id é gerado pela base de dados.
type ProcedimentoCirurgico struct {
	id              string
	episodioID      string
	codigo          string
	descricao       string
	sala            string
	cirurgiaoID     string
	auxiliarID      string
	anestesia       Anestesia
	anestesistaID   string
	inicio          *time.Time
	fim             *time.Time
	consentimentoID string
	complicacoes    string
	observacoes     string
	estado          EstadoProcedimento
	criadoEm        time.Time
}

// DadosNovoProcedimento agrupa os parâmetros de construção.
type DadosNovoProcedimento struct {
	EpisodioID    string
	Codigo        string
	Descricao     string
	Sala          string
	CirurgiaoID   string
	AuxiliarID    string
	Anestesia     Anestesia
	AnestesistaID string
	Observacoes   string
}

// NovoProcedimento valida as invariantes e devolve o agregado em AGENDADO.
// Recebe o Consentimento (não só o id) para impor a invariante-estrela: só há
// procedimento com consentimento de finalidade CIRURGIA, anexado e vigente.
func NovoProcedimento(d DadosNovoProcedimento, consentimento *Consentimento) (*ProcedimentoCirurgico, error) {
	d.EpisodioID = strings.TrimSpace(d.EpisodioID)
	if d.EpisodioID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "episódio do procedimento em falta")
	}
	d.Codigo = strings.TrimSpace(d.Codigo)
	if d.Codigo == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "código do procedimento em falta")
	}
	d.Descricao = strings.TrimSpace(d.Descricao)
	if d.Descricao == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "descrição do procedimento em falta")
	}
	d.CirurgiaoID = strings.TrimSpace(d.CirurgiaoID)
	if d.CirurgiaoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "cirurgião do procedimento em falta")
	}
	if _, err := ParseAnestesia(string(d.Anestesia)); err != nil {
		return nil, err
	}
	d.AnestesistaID = strings.TrimSpace(d.AnestesistaID)
	if d.Anestesia.RequerAnestesista() && d.AnestesistaID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "anestesista obrigatório quando há anestesia")
	}
	if consentimento == nil {
		return nil, erros.Novo(erros.CategoriaRegraNegocio, "consentimento cirúrgico em falta")
	}
	if consentimento.Finalidade() != FinalidadeCirurgia || !consentimento.TemAnexo() || !consentimento.EstaVigente() {
		return nil, erros.Novo(erros.CategoriaRegraNegocio,
			"consentimento cirúrgico inválido (exige finalidade CIRURGIA, anexo e estar vigente)")
	}
	return &ProcedimentoCirurgico{
		episodioID: d.EpisodioID, codigo: d.Codigo, descricao: d.Descricao,
		sala: strings.TrimSpace(d.Sala), cirurgiaoID: d.CirurgiaoID,
		auxiliarID: strings.TrimSpace(d.AuxiliarID), anestesia: d.Anestesia,
		anestesistaID: d.AnestesistaID, consentimentoID: consentimento.ID(),
		observacoes: strings.TrimSpace(d.Observacoes), estado: ProcAgendado,
	}, nil
}

// Iniciar transita AGENDADO → EM_CURSO.
func (p *ProcedimentoCirurgico) Iniciar(em time.Time) error {
	if p.estado != ProcAgendado {
		return erros.Novo(erros.CategoriaConflito, "só é possível iniciar um procedimento agendado")
	}
	if em.IsZero() {
		return erros.Novo(erros.CategoriaValidacao, "início do procedimento em falta")
	}
	p.estado = ProcEmCurso
	p.inicio = &em
	return nil
}

// Concluir transita EM_CURSO → CONCLUIDO. O fim não pode ser anterior ao início.
func (p *ProcedimentoCirurgico) Concluir(em time.Time, complicacoes, observacoes string) error {
	if p.estado != ProcEmCurso {
		return erros.Novo(erros.CategoriaConflito, "só é possível concluir um procedimento em curso")
	}
	if p.inicio == nil {
		return erros.Novo(erros.CategoriaConflito, "procedimento em curso sem início registado (estado incoerente)")
	}
	if em.Before(*p.inicio) {
		return erros.Novo(erros.CategoriaValidacao, "o fim do procedimento não pode ser anterior ao início")
	}
	p.estado = ProcConcluido
	p.fim = &em
	p.complicacoes = strings.TrimSpace(complicacoes)
	if obs := strings.TrimSpace(observacoes); obs != "" {
		p.observacoes = obs
	}
	return nil
}

// Cancelar transita EM_CURSO → CANCELADO (cancelamento intra-operatório, DDM
// estrito). O motivo é guardado nas observações e auditado na aplicação.
func (p *ProcedimentoCirurgico) Cancelar(em time.Time, motivo string) error {
	if p.estado != ProcEmCurso {
		return erros.Novo(erros.CategoriaConflito, "só é possível cancelar um procedimento em curso")
	}
	if p.inicio == nil {
		return erros.Novo(erros.CategoriaConflito, "procedimento em curso sem início registado (estado incoerente)")
	}
	if em.Before(*p.inicio) {
		return erros.Novo(erros.CategoriaValidacao, "o fim do procedimento não pode ser anterior ao início")
	}
	p.estado = ProcCancelado
	p.fim = &em
	if m := strings.TrimSpace(motivo); m != "" {
		p.observacoes = m
	}
	return nil
}

// ID devolve o identificador atribuído pela base de dados.
func (p *ProcedimentoCirurgico) ID() string { return p.id }

// EpisodioID devolve o id do episódio a que o procedimento pertence.
func (p *ProcedimentoCirurgico) EpisodioID() string { return p.episodioID }

// Estado devolve o estado actual.
func (p *ProcedimentoCirurgico) Estado() EstadoProcedimento { return p.estado }

// ConsentimentoID devolve o id do consentimento associado.
func (p *ProcedimentoCirurgico) ConsentimentoID() string { return p.consentimentoID }

// SnapshotProcedimento carrega o estado completo para persistência ou rehidratação.
type SnapshotProcedimento struct {
	ID              string
	EpisodioID      string
	Codigo          string
	Descricao       string
	Sala            string
	CirurgiaoID     string
	AuxiliarID      string
	Anestesia       Anestesia
	AnestesistaID   string
	Inicio          *time.Time
	Fim             *time.Time
	ConsentimentoID string
	Complicacoes    string
	Observacoes     string
	Estado          EstadoProcedimento
	CriadoEm        time.Time
}

// Snapshot devolve o estado completo do agregado.
func (p *ProcedimentoCirurgico) Snapshot() SnapshotProcedimento {
	return SnapshotProcedimento{
		ID: p.id, EpisodioID: p.episodioID, Codigo: p.codigo, Descricao: p.descricao,
		Sala: p.sala, CirurgiaoID: p.cirurgiaoID, AuxiliarID: p.auxiliarID,
		Anestesia: p.anestesia, AnestesistaID: p.anestesistaID, Inicio: p.inicio, Fim: p.fim,
		ConsentimentoID: p.consentimentoID, Complicacoes: p.complicacoes,
		Observacoes: p.observacoes, Estado: p.estado, CriadoEm: p.criadoEm,
	}
}

// ReconstruirProcedimento reconstrói um agregado a partir de um snapshot persistido.
func ReconstruirProcedimento(s SnapshotProcedimento) *ProcedimentoCirurgico {
	return &ProcedimentoCirurgico{
		id: s.ID, episodioID: s.EpisodioID, codigo: s.Codigo, descricao: s.Descricao,
		sala: s.Sala, cirurgiaoID: s.CirurgiaoID, auxiliarID: s.AuxiliarID,
		anestesia: s.Anestesia, anestesistaID: s.AnestesistaID, inicio: s.Inicio, fim: s.Fim,
		consentimentoID: s.ConsentimentoID, complicacoes: s.Complicacoes,
		observacoes: s.Observacoes, estado: s.Estado, criadoEm: s.CriadoEm,
	}
}

// ResumoProcedimento é a projecção de leitura de um procedimento.
type ResumoProcedimento struct {
	ID         string     `json:"id"`
	EpisodioID string     `json:"episodio_id"`
	Codigo     string     `json:"codigo_procedimento"`
	Descricao  string     `json:"descricao"`
	Estado     string     `json:"estado"`
	Anestesia  string     `json:"anestesia"`
	Inicio     *time.Time `json:"inicio,omitempty"`
	Fim        *time.Time `json:"fim,omitempty"`
	CriadoEm   time.Time  `json:"criado_em"`
}

// RepositorioProcedimentos é a porta de saída de persistência de procedimentos.
type RepositorioProcedimentos interface {
	Guardar(ctx context.Context, p *ProcedimentoCirurgico) (string, error)
	ObterPorID(ctx context.Context, id string) (*ProcedimentoCirurgico, error)
	ListarPorEpisodio(ctx context.Context, episodioID string) ([]ResumoProcedimento, error)
}
