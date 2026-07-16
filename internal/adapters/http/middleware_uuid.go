package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// ValidarUUIDs valida que todos os parâmetros de caminho do pedido são UUIDs,
// excepto os nomes isentos (parâmetros de negócio, p.ex. :papel). Um id
// malformado responde 400 (RFC 7807) em vez de chegar à base de dados e
// rebentar em 500 (SQLSTATE 22P02).
func ValidarUUIDs(isentos ...string) gin.HandlerFunc {
	conjuntoIsentos := make(map[string]struct{}, len(isentos))
	for _, nome := range isentos {
		conjuntoIsentos[nome] = struct{}{}
	}

	return func(c *gin.Context) {
		for _, p := range c.Params {
			if _, isento := conjuntoIsentos[p.Key]; isento {
				continue
			}
			if _, err := uuid.Parse(p.Value); err != nil {
				responderErro(c, erros.Novo(erros.CategoriaValidacao, "identificador inválido no caminho do pedido"))
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
