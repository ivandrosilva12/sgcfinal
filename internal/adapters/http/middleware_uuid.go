package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// ValidarUUIDs valida que todos os parâmetros de caminho do pedido são UUIDs
// na forma canónica (36 caracteres, com hífens), excepto os nomes isentos
// (parâmetros de negócio, p.ex. :papel). Um id malformado responde 400
// (RFC 7807) em vez de chegar à base de dados e rebentar em 500 (SQLSTATE
// 22P02).
//
// Exige-se exactamente 36 caracteres porque uuid.Parse aceita variantes que
// o Postgres rejeita — 32 caracteres hex sem hífens, "{uuid}" entre
// chavetas (38) e "urn:uuid:uuid" (45); a própria documentação do pacote
// avisa que uuid.Parse não deve ser usado para validar strings. Sem esta
// restrição, um segmento de caminho como "urn:uuid:1a2b..." passaria o
// guard e reproduziria o 500 que ele existe para eliminar.
func ValidarUUIDs(isentos ...string) gin.HandlerFunc {
	const tamanhoCanonico = 36

	conjuntoIsentos := make(map[string]struct{}, len(isentos))
	for _, nome := range isentos {
		conjuntoIsentos[nome] = struct{}{}
	}

	return func(c *gin.Context) {
		for _, p := range c.Params {
			if _, isento := conjuntoIsentos[p.Key]; isento {
				continue
			}
			if _, err := uuid.Parse(p.Value); err != nil || len(p.Value) != tamanhoCanonico {
				responderErro(c, erros.Novo(erros.CategoriaValidacao, "identificador inválido no caminho do pedido"))
				return
			}
		}
		c.Next()
	}
}
