package identidade

import (
	"net/mail"
	"strings"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/identity"
)

// Utilizador é o agregado raiz do BC Identidade — o perfil local de um
// utilizador cuja autenticação é gerida pelo Keycloak (keycloak_id é a chave).
// Domínio rico: a construção valida os invariantes; telefone e BI, quando
// presentes, são normalizados pelos Value Objects do Shared Kernel.
type Utilizador struct {
	KeycloakID string // claim "sub" do Keycloak (uuid em texto)
	Nome       string
	Email      string
	Telefone   string // formato de apresentação "+244 9XX XXX XXX" ou vazio
	BI         string // formato canónico ou vazio
	Activo     bool
	Papeis     []Papel
}

// NovoUtilizador valida e constrói um Utilizador a partir dos dados do token
// (JIT provisioning). keycloak_id, nome e email são obrigatórios. Telefone e BI
// são opcionais; quando fornecidos, são validados/normalizados. Devolve um
// ErroDominio de categoria Validação em caso de dados inválidos.
func NovoUtilizador(keycloakID, nome, email, telefone, bi string, papeis []Papel) (*Utilizador, error) {
	keycloakID = strings.TrimSpace(keycloakID)
	if keycloakID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "keycloak_id em falta")
	}
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "nome em falta")
	}
	email = strings.TrimSpace(email)
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, erros.Novo(erros.CategoriaValidacao, "email inválido")
	}

	u := &Utilizador{
		KeycloakID: keycloakID,
		Nome:       nome,
		Email:      email,
		Activo:     true,
		Papeis:     papeis,
	}

	if s := strings.TrimSpace(telefone); s != "" {
		t, err := identity.NovoTelefone(s)
		if err != nil {
			return nil, erros.Novo(erros.CategoriaValidacao, "telefone inválido")
		}
		u.Telefone = t.String()
	}
	if s := strings.TrimSpace(bi); s != "" {
		b, err := identity.NovoBI(s)
		if err != nil {
			return nil, erros.Novo(erros.CategoriaValidacao, "bilhete de identidade inválido")
		}
		u.BI = b.String()
	}

	return u, nil
}

// TemPapel indica se o utilizador possui o papel indicado.
func (u *Utilizador) TemPapel(p Papel) bool {
	for _, atribuido := range u.Papeis {
		if atribuido == p {
			return true
		}
	}
	return false
}

// TemAlgumPapel indica se o utilizador possui pelo menos um dos papéis indicados.
func (u *Utilizador) TemAlgumPapel(permitidos ...Papel) bool {
	for _, p := range permitidos {
		if u.TemPapel(p) {
			return true
		}
	}
	return false
}
