// Package identity contém validadores de identificação nacional angolana
// (Bilhete de Identidade e telefone), pertencentes ao Shared Kernel do domínio.
//
// Regra de dependência: este pacote é Camada 1 (Domínio). Não importa infra
// (pgx, gin, net/http) — apenas a biblioteca-padrão.
package identity

import (
	"errors"
	"regexp"
	"strings"
)

// ErrBIInvalido é devolvido quando um Bilhete de Identidade não respeita o
// formato angolano.
var ErrBIInvalido = errors.New("bilhete de identidade inválido")

// formatoBI valida o formato do BI angolano: 8 dígitos, 2 letras, 3 dígitos.
// Exemplo: "00123456LA042".
var formatoBI = regexp.MustCompile(`^[0-9]{8}[A-Z]{2}[0-9]{3}$`)

// BI representa um Bilhete de Identidade angolano validado. É um Value Object
// imutável — a sua existência garante que o valor é bem-formado.
type BI struct {
	valor string
}

// NovoBI valida e constrói um BI. A entrada é normalizada (espaços removidos e
// letras em maiúsculas) antes da validação. Devolve ErrBIInvalido se o formato
// não corresponder a 8 dígitos + 2 letras + 3 dígitos.
func NovoBI(entrada string) (BI, error) {
	normalizado := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(entrada), " ", ""))
	if !formatoBI.MatchString(normalizado) {
		return BI{}, ErrBIInvalido
	}
	return BI{valor: normalizado}, nil
}

// String devolve a representação canónica do BI.
func (b BI) String() string {
	return b.valor
}

// Valido indica se um BI foi construído com sucesso (valor não vazio).
func (b BI) Valido() bool {
	return b.valor != ""
}
