// Package moeda modela o Kwanza angolano (AOA) como Value Object do Shared
// Kernel. Os montantes são guardados em cêntimos (int64) para evitar erros de
// vírgula flutuante em cálculos financeiros.
//
// Camada 1 (Domínio): apenas biblioteca-padrão, sem infra.
package moeda

import (
	"errors"
	"strconv"
	"strings"
)

// ErrMontanteNegativo é devolvido quando se tenta criar um montante negativo
// num contexto que não o admite.
var ErrMontanteNegativo = errors.New("montante negativo não permitido")

// AOA representa um montante em Kwanzas, guardado em cêntimos. Value Object
// imutável.
type AOA struct {
	centimos int64
}

// DeCentimos constrói um AOA a partir de um valor em cêntimos.
func DeCentimos(centimos int64) AOA {
	return AOA{centimos: centimos}
}

// DeKwanzas constrói um AOA a partir de um valor em kwanzas (unidade inteira).
func DeKwanzas(kwanzas int64) AOA {
	return AOA{centimos: kwanzas * 100}
}

// Centimos devolve o montante total em cêntimos.
func (a AOA) Centimos() int64 {
	return a.centimos
}

// Somar devolve um novo AOA com a soma dos dois montantes.
func (a AOA) Somar(outro AOA) AOA {
	return AOA{centimos: a.centimos + outro.centimos}
}

// Subtrair devolve um novo AOA com a diferença. Pode resultar negativo.
func (a AOA) Subtrair(outro AOA) AOA {
	return AOA{centimos: a.centimos - outro.centimos}
}

// Negativo indica se o montante é inferior a zero.
func (a AOA) Negativo() bool {
	return a.centimos < 0
}

// String devolve a representação de apresentação angolana: separador de
// milhares "." e decimal ",", sufixo " Kz". Exemplo: 123450 → "1.234,50 Kz".
func (a AOA) String() string {
	negativo := a.centimos < 0
	abs := a.centimos
	if negativo {
		abs = -abs
	}

	inteiro := abs / 100
	fraccao := abs % 100

	inteiroStr := agruparMilhares(strconv.FormatInt(inteiro, 10))
	fraccaoStr := strconv.FormatInt(fraccao, 10)
	if len(fraccaoStr) == 1 {
		fraccaoStr = "0" + fraccaoStr
	}

	sinal := ""
	if negativo {
		sinal = "-"
	}
	return sinal + inteiroStr + "," + fraccaoStr + " Kz"
}

// agruparMilhares insere "." a cada três dígitos a contar da direita.
func agruparMilhares(digitos string) string {
	n := len(digitos)
	if n <= 3 {
		return digitos
	}
	var b strings.Builder
	// Primeiro grupo (1 a 3 dígitos).
	primeiro := n % 3
	if primeiro == 0 {
		primeiro = 3
	}
	b.WriteString(digitos[:primeiro])
	for i := primeiro; i < n; i += 3 {
		b.WriteString(".")
		b.WriteString(digitos[i : i+3])
	}
	return b.String()
}
