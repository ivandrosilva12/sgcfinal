package farmacia

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Fornecedor é o agregado de um fornecedor de medicamentos.
type Fornecedor struct {
	id       string
	nome     string
	nif      *string
	contacto *string
	activo   bool
	criadoEm time.Time
}

// NovoFornecedor valida e constrói um fornecedor activo. Nome obrigatório.
func NovoFornecedor(nome string, nif, contacto *string) (*Fornecedor, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "nome do fornecedor em falta")
	}
	return &Fornecedor{nome: nome, nif: normalizarOpcional(nif), contacto: normalizarOpcional(contacto), activo: true}, nil
}

func (f *Fornecedor) ID() string   { return f.id }
func (f *Fornecedor) Activo() bool { return f.activo }
func (f *Fornecedor) Activar()     { f.activo = true }
func (f *Fornecedor) Desactivar()  { f.activo = false }

// SnapshotFornecedor carrega o estado completo para persistência/rehidratação.
type SnapshotFornecedor struct {
	ID       string
	Nome     string
	NIF      *string
	Contacto *string
	Activo   bool
	CriadoEm time.Time
}

func (f *Fornecedor) Snapshot() SnapshotFornecedor {
	return SnapshotFornecedor{ID: f.id, Nome: f.nome, NIF: f.nif, Contacto: f.contacto, Activo: f.activo, CriadoEm: f.criadoEm}
}

func ReconstruirFornecedor(s SnapshotFornecedor) *Fornecedor {
	return &Fornecedor{id: s.ID, nome: s.Nome, nif: s.NIF, contacto: s.Contacto, activo: s.Activo, criadoEm: s.CriadoEm}
}
