package http

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// cabecalhoRequestID é o cabeçalho usado para correlação de pedidos.
const cabecalhoRequestID = "X-Request-ID"

// RequestID injecta (ou propaga) um identificador de correlação por pedido,
// disponível no contexto e devolvido no cabeçalho de resposta.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(cabecalhoRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Writer.Header().Set(cabecalhoRequestID, id)
		c.Next()
	}
}

// Logging regista cada pedido em formato estruturado (slog), incluindo o
// request-id, método, rota, código e latência.
func Logging(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		inicio := time.Now()
		c.Next()

		rota := c.FullPath()
		if rota == "" {
			rota = c.Request.URL.Path
		}
		reqID, _ := c.Get("request_id")
		logger.Info("pedido http",
			"request_id", reqID,
			"metodo", c.Request.Method,
			"rota", rota,
			"codigo", c.Writer.Status(),
			"latencia_ms", time.Since(inicio).Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}
