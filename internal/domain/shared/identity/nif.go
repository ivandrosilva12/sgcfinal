package identity

import (
	"errors"
	"regexp"
	"strings"
)

// ErrNIFInvalido é devolvido quando um Número de Identificação Fiscal não
// respeita o formato angolano.
var ErrNIFInvalido = errors.New("nif inválido")

// formatoNIF valida o NIF angolano de 10 caracteres: ou 10 dígitos (pessoa
// colectiva), ou 9 dígitos seguidos de 1 letra (pessoa singular).
// Exemplos: "5417000001", "004567890A".
var formatoNIF = regexp.MustCompile(`^([0-9]{10}|[0-9]{9}[A-Z])$`)

// NIF representa um Número de Identificação Fiscal angolano validado. Value
// Object imutável — a sua existência garante que o valor é bem-formado.
type NIF struct {
	valor string
}

// NovoNIF valida e constrói um NIF. A entrada é normalizada (espaços removidos e
// letras em maiúsculas) antes da validação. Devolve ErrNIFInvalido se o formato
// não corresponder a 10 dígitos ou 9 dígitos + 1 letra.
func NovoNIF(entrada string) (NIF, error) {
	normalizado := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(entrada), " ", ""))
	if !formatoNIF.MatchString(normalizado) {
		return NIF{}, ErrNIFInvalido
	}
	return NIF{valor: normalizado}, nil
}

// String devolve a representação canónica do NIF.
func (n NIF) String() string {
	return n.valor
}

// Valido indica se um NIF foi construído com sucesso (valor não vazio).
func (n NIF) Valido() bool {
	return n.valor != ""
}
