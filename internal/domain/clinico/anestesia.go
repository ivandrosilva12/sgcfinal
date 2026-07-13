package clinico

import (
	"strings"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Anestesia é o tipo de anestesia de um procedimento cirúrgico (DDM-001 v2.1).
type Anestesia string

const (
	AnestesiaNenhuma        Anestesia = "NENHUMA"
	AnestesiaLocal          Anestesia = "LOCAL"
	AnestesiaSedacaoLigeira Anestesia = "SEDACAO_LIGEIRA"
	AnestesiaLocoRegional   Anestesia = "LOCO_REGIONAL"
)

var anestesiasValidas = map[Anestesia]bool{
	AnestesiaNenhuma: true, AnestesiaLocal: true,
	AnestesiaSedacaoLigeira: true, AnestesiaLocoRegional: true,
}

// ParseAnestesia valida e normaliza um tipo de anestesia (aceita minúsculas).
func ParseAnestesia(codigo string) (Anestesia, error) {
	a := Anestesia(strings.ToUpper(strings.TrimSpace(codigo)))
	if !anestesiasValidas[a] {
		return "", erros.Novo(erros.CategoriaValidacao,
			"tipo de anestesia inválido (esperado NENHUMA, LOCAL, SEDACAO_LIGEIRA ou LOCO_REGIONAL)")
	}
	return a, nil
}

// RequerAnestesista indica se este tipo de anestesia obriga a um anestesista.
func (a Anestesia) RequerAnestesista() bool { return a != AnestesiaNenhuma }
