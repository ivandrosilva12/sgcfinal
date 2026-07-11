package http_test

import (
	"context"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
)

func novoRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func fazerPedido(r nethttp.Handler, metodo, caminho string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(metodo, caminho, nil)
	r.ServeHTTP(w, req)
	return w
}

func TestHealthz(t *testing.T) {
	r := novoRouter()
	adhttp.RegistarHealth(r, nil)

	w := fazerPedido(r, "GET", "/healthz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "vivo") {
		t.Fatalf("corpo inesperado: %s", w.Body.String())
	}
}

func TestReadyz_TodasProntas(t *testing.T) {
	r := novoRouter()
	verificacoes := []adhttp.Verificacao{
		{Nome: "postgres", Verificar: func(context.Context) error { return nil }},
		{Nome: "redis", Verificar: func(context.Context) error { return nil }},
	}
	adhttp.RegistarHealth(r, verificacoes)

	w := fazerPedido(r, "GET", "/readyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (corpo: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"pronto":true`) {
		t.Fatalf("esperava pronto=true, corpo: %s", w.Body.String())
	}
}

func TestReadyz_DependenciaEmBaixo(t *testing.T) {
	r := novoRouter()
	verificacoes := []adhttp.Verificacao{
		{Nome: "postgres", Verificar: func(context.Context) error { return nil }},
		{Nome: "redis", Verificar: func(context.Context) error { return errors.New("ligação recusada") }},
	}
	adhttp.RegistarHealth(r, verificacoes)

	w := fazerPedido(r, "GET", "/readyz")
	if w.Code != nethttp.StatusServiceUnavailable {
		t.Fatalf("esperava 503, obtive %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"pronto":false`) {
		t.Fatalf("esperava pronto=false, corpo: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "indisponível") {
		t.Fatalf("esperava marcação de dependência indisponível, corpo: %s", w.Body.String())
	}
}
