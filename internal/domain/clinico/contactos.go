package clinico

import (
	"net/mail"
	"strings"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/identity"
)

// Morada é o Value Object de morada do doente (campos morada_* do DDM-001).
type Morada struct {
	Provincia  string
	Municipio  string
	Comuna     string
	Bairro     string
	Rua        string
	Casa       *string
	Referencia *string
}

// Contactos é o Value Object de contactos do doente. Telefone é obrigatório
// (telemóvel angolano); email e morada são opcionais.
type Contactos struct {
	Telefone string
	Email    *string
	Morada   *Morada
}

// NovosContactos valida e normaliza os contactos. Telefone obrigatório e
// normalizado para "+244 9XX XXX XXX"; email validado se presente.
func NovosContactos(telefone string, email *string, morada *Morada) (Contactos, error) {
	tel, err := identity.NovoTelefone(strings.TrimSpace(telefone))
	if err != nil {
		return Contactos{}, erros.Novo(erros.CategoriaValidacao, "telefone inválido")
	}

	emailNorm := normalizarOpcional(email)
	if emailNorm != nil {
		if _, err := mail.ParseAddress(*emailNorm); err != nil {
			return Contactos{}, erros.Novo(erros.CategoriaValidacao, "email inválido")
		}
	}

	return Contactos{
		Telefone: tel.String(),
		Email:    emailNorm,
		Morada:   morada,
	}, nil
}
