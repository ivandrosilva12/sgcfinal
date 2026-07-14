package clinico

import (
	"context"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Finalidade classifica a finalidade LPDP de um consentimento (DDM-001 v2.0;
// CIRURGIA acrescentada pela adenda v2.1).
type Finalidade string

const (
	FinalidadeTratamento         Finalidade = "TRATAMENTO"
	FinalidadeComunicacao        Finalidade = "COMUNICACAO"
	FinalidadePartilhaSeguradora Finalidade = "PARTILHA_SEGURADORA"
	FinalidadeInvestigacao       Finalidade = "INVESTIGACAO"
	FinalidadeCirurgia           Finalidade = "CIRURGIA"
)

var finalidadesValidas = map[Finalidade]bool{
	FinalidadeTratamento: true, FinalidadeComunicacao: true,
	FinalidadePartilhaSeguradora: true, FinalidadeInvestigacao: true,
	FinalidadeCirurgia: true,
}

// ParseFinalidade valida e normaliza uma finalidade (aceita minúsculas).
func ParseFinalidade(codigo string) (Finalidade, error) {
	f := Finalidade(strings.ToUpper(strings.TrimSpace(codigo)))
	if !finalidadesValidas[f] {
		return "", erros.Novo(erros.CategoriaValidacao,
			"finalidade de consentimento inválida (esperado TRATAMENTO, COMUNICACAO, PARTILHA_SEGURADORA, INVESTIGACAO ou CIRURGIA)")
	}
	return f, nil
}

// Consentimento é um agregado raiz do BC Clínico: o consentimento LPDP de um
// doente para uma finalidade. O id é gerado pela base de dados.
type Consentimento struct {
	id           string
	doenteID     string
	finalidade   Finalidade
	concedido    bool
	documentoURL string
	concedidoEm  time.Time
	revogadoEm   *time.Time
	criadoEm     time.Time
}

// NovoConsentimento valida e constrói um consentimento. Para a finalidade
// CIRURGIA impõe a invariante-estrela: tem de estar concedido e com anexo.
func NovoConsentimento(doenteID string, f Finalidade, concedido bool, documentoURL string, concedidoEm time.Time) (*Consentimento, error) {
	doenteID = strings.TrimSpace(doenteID)
	if doenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente do consentimento em falta")
	}
	if _, err := ParseFinalidade(string(f)); err != nil {
		return nil, err
	}
	if concedidoEm.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "data de concessão do consentimento em falta")
	}
	documentoURL = strings.TrimSpace(documentoURL)
	if f == FinalidadeCirurgia {
		if !concedido {
			return nil, erros.Novo(erros.CategoriaRegraNegocio, "o consentimento de cirurgia tem de estar concedido")
		}
		if documentoURL == "" {
			return nil, erros.Novo(erros.CategoriaRegraNegocio, "o consentimento de cirurgia exige documento anexado")
		}
	}
	return &Consentimento{
		doenteID: doenteID, finalidade: f, concedido: concedido,
		documentoURL: documentoURL, concedidoEm: concedidoEm,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados (vazio se não persistido).
func (c *Consentimento) ID() string { return c.id }

// DoenteID devolve o id do doente a que o consentimento pertence.
func (c *Consentimento) DoenteID() string { return c.doenteID }

// Finalidade devolve a finalidade LPDP.
func (c *Consentimento) Finalidade() Finalidade { return c.finalidade }

// TemAnexo indica se há documento anexado.
func (c *Consentimento) TemAnexo() bool { return c.documentoURL != "" }

// EstaVigente indica se o consentimento está concedido e não revogado.
func (c *Consentimento) EstaVigente() bool { return c.concedido && c.revogadoEm == nil }

// Revogar revoga o consentimento. Só de um consentimento concedido e não revogado.
func (c *Consentimento) Revogar(em time.Time) error {
	if !c.concedido {
		return erros.Novo(erros.CategoriaConflito, "não é possível revogar um consentimento que não foi concedido")
	}
	if c.revogadoEm != nil {
		return erros.Novo(erros.CategoriaConflito, "o consentimento já foi revogado")
	}
	c.revogadoEm = &em
	return nil
}

// SnapshotConsentimento carrega o estado completo para persistência ou rehidratação.
type SnapshotConsentimento struct {
	ID           string
	DoenteID     string
	Finalidade   Finalidade
	Concedido    bool
	DocumentoURL string
	ConcedidoEm  time.Time
	RevogadoEm   *time.Time
	CriadoEm     time.Time
}

// Snapshot devolve o estado completo do agregado.
func (c *Consentimento) Snapshot() SnapshotConsentimento {
	return SnapshotConsentimento{
		ID: c.id, DoenteID: c.doenteID, Finalidade: c.finalidade,
		Concedido: c.concedido, DocumentoURL: c.documentoURL,
		ConcedidoEm: c.concedidoEm, RevogadoEm: c.revogadoEm, CriadoEm: c.criadoEm,
	}
}

// ReconstruirConsentimento reconstrói um agregado a partir de um snapshot persistido.
func ReconstruirConsentimento(s SnapshotConsentimento) *Consentimento {
	return &Consentimento{
		id: s.ID, doenteID: s.DoenteID, finalidade: s.Finalidade,
		concedido: s.Concedido, documentoURL: s.DocumentoURL,
		concedidoEm: s.ConcedidoEm, revogadoEm: s.RevogadoEm, criadoEm: s.CriadoEm,
	}
}

// FiltroConsentimentos filtra a listagem de consentimentos de um doente.
type FiltroConsentimentos struct {
	Finalidade     string
	ApenasVigentes bool
}

// ResumoConsentimento é a projecção de leitura de um consentimento.
type ResumoConsentimento struct {
	ID           string     `json:"id"`
	DoenteID     string     `json:"doente_id"`
	Finalidade   string     `json:"finalidade"`
	Concedido    bool       `json:"concedido"`
	DocumentoURL string     `json:"documento_url,omitempty"`
	ConcedidoEm  time.Time  `json:"concedido_em"`
	RevogadoEm   *time.Time `json:"revogado_em,omitempty"`
	Vigente      bool       `json:"vigente"`
}

// RepositorioConsentimentos é a porta de saída de persistência de consentimentos.
type RepositorioConsentimentos interface {
	Guardar(ctx context.Context, c *Consentimento) (string, error)
	ObterPorID(ctx context.Context, id string) (*Consentimento, error)
	ListarPorDoente(ctx context.Context, doenteID string, filtro FiltroConsentimentos) ([]ResumoConsentimento, error)
}
