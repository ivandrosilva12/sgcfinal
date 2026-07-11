package http_test

import (
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
)

func TestRequestID_Gera(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	r.GET("/", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := fazerPedido(r, "GET", "/")
	if id := w.Header().Get("X-Request-ID"); id == "" {
		t.Fatal("esperava X-Request-ID gerado na resposta")
	}
}

func TestRequestID_Propaga(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	r.GET("/", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "existente-123")
	r.ServeHTTP(w, req)
	if got := w.Header().Get("X-Request-ID"); got != "existente-123" {
		t.Fatalf("esperava propagação do id existente, obtive %q", got)
	}
}

func TestLogging_PassaAdiante(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := novoRouter()
	r.Use(adhttp.RequestID(), adhttp.Logging(logger))
	r.GET("/rota", func(c *gin.Context) { c.Status(nethttp.StatusTeapot) })

	w := fazerPedido(r, "GET", "/rota")
	if w.Code != nethttp.StatusTeapot {
		t.Fatalf("esperava 418 (pass-through), obtive %d", w.Code)
	}
}
