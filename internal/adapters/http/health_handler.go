// Package http contém os adaptadores de entrada HTTP (handlers e middleware)
// sobre o Gin. Camada 3 — Adaptadores.
//
// Em M1/Sprint 1 expõe apenas os endpoints de saúde. Os handlers de negócio
// (ex.: perfil de Identidade) e o middleware de auth/RBAC entram em Sprint 2.
package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Verificacao é uma dependência a inspeccionar no readiness probe.
type Verificacao struct {
	Nome      string
	Verificar func(context.Context) error
}

// RegistarHealth regista os endpoints de saúde no router:
//   - GET /healthz  liveness  — o processo está vivo (sempre 200).
//   - GET /readyz   readiness — todas as dependências estão prontas (200) ou
//     alguma está em baixo (503).
func RegistarHealth(r gin.IRouter, verificacoes []Verificacao) {
	r.GET("/healthz", liveness)
	r.GET("/readyz", readiness(verificacoes))
}

// liveness godoc
// @Summary  Liveness probe
// @Description Indica que o processo está vivo. Não verifica dependências.
// @Tags     saude
// @Produce  json
// @Success  200 {object} map[string]string
// @Router   /healthz [get]
func liveness(c *gin.Context) {
	c.JSON(nethttp.StatusOK, gin.H{"estado": "vivo"})
}

// readiness godoc
// @Summary  Readiness probe
// @Description Verifica PostgreSQL e Redis. Devolve 503 se alguma dependência falhar.
// @Tags     saude
// @Produce  json
// @Success  200 {object} map[string]any
// @Failure  503 {object} map[string]any
// @Router   /readyz [get]
func readiness(verificacoes []Verificacao) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		dependencias := make(map[string]string, len(verificacoes))
		pronto := true
		for _, v := range verificacoes {
			if err := v.Verificar(ctx); err != nil {
				pronto = false
				dependencias[v.Nome] = "indisponível"
			} else {
				dependencias[v.Nome] = "ok"
			}
		}

		codigo := nethttp.StatusOK
		mensagem := i18n.T(i18n.MsgServicoOperacional)
		if !pronto {
			codigo = nethttp.StatusServiceUnavailable
			mensagem = i18n.T(i18n.MsgServicoIndisponivel)
		}

		c.JSON(codigo, gin.H{
			"pronto":       pronto,
			"mensagem":     mensagem,
			"dependencias": dependencias,
		})
	}
}
