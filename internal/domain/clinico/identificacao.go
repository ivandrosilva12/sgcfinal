// Package clinico é o domínio do Bounded Context Clínico do SGC Angola. Contém o
// agregado Doente e os seus Value Objects e entidades-filho. Camada 1 (Domínio):
// importa apenas a biblioteca-padrão e o Shared Kernel — sem infra.
package clinico

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/identity"
)

// Sexo é o sexo biológico registado do doente (DDM-001: CHAR(1) M|F|O).
type Sexo string

const (
	SexoMasculino Sexo = "M"
	SexoFeminino  Sexo = "F"
	SexoOutro     Sexo = "O"
)

// ParseSexo valida e normaliza um código de sexo (aceita minúsculas).
func ParseSexo(codigo string) (Sexo, error) {
	switch Sexo(strings.ToUpper(strings.TrimSpace(codigo))) {
	case SexoMasculino:
		return SexoMasculino, nil
	case SexoFeminino:
		return SexoFeminino, nil
	case SexoOutro:
		return SexoOutro, nil
	default:
		return "", erros.Novo(erros.CategoriaValidacao, "sexo inválido (esperado M, F ou O)")
	}
}

// Identificacao é o Value Object de identificação civil do doente. Invariante do
// DDM-001: pelo menos um de BI ou Passaporte tem de estar presente.
type Identificacao struct {
	NomeCompleto   string
	DataNascimento time.Time
	Sexo           Sexo
	BI             *string
	NIF            *string
	Passaporte     *string
}

// NovaIdentificacao valida e normaliza a identificação. Nome obrigatório; data de
// nascimento não pode ser futura; sexo válido; BI ou Passaporte obrigatório; BI e
// NIF, quando presentes, são validados/normalizados pelo Shared Kernel.
func NovaIdentificacao(nome string, dataNasc time.Time, sexo Sexo, bi, nif, passaporte *string) (Identificacao, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return Identificacao{}, erros.Novo(erros.CategoriaValidacao, "nome completo em falta")
	}
	if dataNasc.After(time.Now()) {
		return Identificacao{}, erros.Novo(erros.CategoriaValidacao, "data de nascimento não pode ser futura")
	}
	if _, err := ParseSexo(string(sexo)); err != nil {
		return Identificacao{}, err
	}

	biNorm := normalizarOpcional(bi)
	passNorm := normalizarOpcional(passaporte)
	if biNorm == nil && passNorm == nil {
		return Identificacao{}, erros.Novo(erros.CategoriaValidacao, "é obrigatório indicar o Bilhete de Identidade ou o Passaporte")
	}

	if biNorm != nil {
		b, err := identity.NovoBI(*biNorm)
		if err != nil {
			return Identificacao{}, erros.Novo(erros.CategoriaValidacao, "bilhete de identidade inválido")
		}
		v := b.String()
		biNorm = &v
	}

	nifNorm := normalizarOpcional(nif)
	if nifNorm != nil {
		n, err := identity.NovoNIF(*nifNorm)
		if err != nil {
			return Identificacao{}, erros.Novo(erros.CategoriaValidacao, "nif inválido")
		}
		v := n.String()
		nifNorm = &v
	}

	return Identificacao{
		NomeCompleto:   nome,
		DataNascimento: dataNasc,
		Sexo:           sexo,
		BI:             biNorm,
		NIF:            nifNorm,
		Passaporte:     passNorm,
	}, nil
}

// normalizarOpcional apara espaços e devolve nil se o resultado for vazio.
func normalizarOpcional(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}
