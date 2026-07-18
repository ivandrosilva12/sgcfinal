package financeiro

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// NumeroFactura é o número legal AGT de uma factura: "FAC 2026/00012345"
// (DDM-001 v2.0 §5.2.1). Prefixo fixo, série, barra, sequencial a 8 dígitos.
type NumeroFactura string

const prefixoNumero = "FAC"

// NovoNumeroFactura compõe o número legal a partir da série e do sequencial.
func NovoNumeroFactura(serie string, sequencial int) (NumeroFactura, error) {
	serie = strings.TrimSpace(serie)
	if serie == "" {
		return "", erros.Novo(erros.CategoriaValidacao, "série da factura em falta")
	}
	if sequencial <= 0 {
		return "", erros.Novo(erros.CategoriaValidacao, "sequencial da factura tem de ser positivo")
	}
	return NumeroFactura(fmt.Sprintf("%s %s/%08d", prefixoNumero, serie, sequencial)), nil
}

// String devolve a representação legal do número.
func (n NumeroFactura) String() string { return string(n) }

// ParseNumeroFactura decompõe um número legal na série e no sequencial.
func ParseNumeroFactura(s string) (string, int, error) {
	invalido := erros.Novo(erros.CategoriaValidacao, "número de factura inválido")
	partes := strings.SplitN(strings.TrimSpace(s), " ", 2)
	if len(partes) != 2 || partes[0] != prefixoNumero {
		return "", 0, invalido
	}
	corpo := strings.SplitN(partes[1], "/", 2)
	if len(corpo) != 2 || corpo[0] == "" {
		return "", 0, invalido
	}
	seq, err := strconv.Atoi(corpo[1])
	if err != nil || seq <= 0 {
		return "", 0, invalido
	}
	return corpo[0], seq, nil
}

// SerieDe devolve a série a que um instante pertence. A série é o ano civil em
// UTC — a mesma normalização usada no hash, para que a factura emitida à
// meia-noite não caia em séries diferentes conforme o fuso do servidor.
func SerieDe(momento time.Time) string {
	return strconv.Itoa(momento.UTC().Year())
}
