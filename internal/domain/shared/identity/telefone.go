package identity

import (
	"errors"
	"regexp"
	"strings"
)

// ErrTelefoneInvalido é devolvido quando um número de telefone não respeita o
// formato de telemóvel angolano.
var ErrTelefoneInvalido = errors.New("telefone inválido")

// apenasDigitos remove tudo o que não seja dígito, para normalizar entradas
// como "+244 923 456 789" ou "(+244) 923-456-789".
var apenasDigitos = regexp.MustCompile(`[^0-9]`)

// Telefone representa um número de telemóvel angolano validado. Value Object
// imutável. O formato aceite é `+244 9XX XXX XXX` (9 dígitos nacionais a
// começar por 9), com ou sem indicativo internacional 244.
type Telefone struct {
	// nacional guarda os 9 dígitos nacionais (ex.: "923456789").
	nacional string
}

// NovoTelefone valida e constrói um Telefone. Aceita entradas com o indicativo
// +244 (ou 244) e separadores diversos; exige 9 dígitos nacionais a iniciar
// por 9. Devolve ErrTelefoneInvalido caso contrário.
func NovoTelefone(entrada string) (Telefone, error) {
	digitos := apenasDigitos.ReplaceAllString(entrada, "")
	digitos = strings.TrimPrefix(digitos, "244")
	if len(digitos) != 9 || digitos[0] != '9' {
		return Telefone{}, ErrTelefoneInvalido
	}
	return Telefone{nacional: digitos}, nil
}

// String devolve o telefone no formato de apresentação `+244 9XX XXX XXX`.
func (t Telefone) String() string {
	if t.nacional == "" {
		return ""
	}
	n := t.nacional
	return "+244 " + n[0:3] + " " + n[3:6] + " " + n[6:9]
}

// E164 devolve o telefone no formato E.164 (`+2449XXXXXXXX`), adequado a
// integrações (Keycloak, SMS).
func (t Telefone) E164() string {
	if t.nacional == "" {
		return ""
	}
	return "+244" + t.nacional
}

// Valido indica se o Telefone foi construído com sucesso.
func (t Telefone) Valido() bool {
	return t.nacional != ""
}
