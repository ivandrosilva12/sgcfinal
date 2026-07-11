package server_test

import (
	"context"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/config"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/observ"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/server"
)

func novoServidorTeste() *server.Servidor {
	cfg := config.Config{Ambiente: "dev", PortaHTTP: "0"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	verificacoes := []adhttp.Verificacao{
		{Nome: "fake", Verificar: func(context.Context) error { return nil }},
	}
	return server.Novo(cfg, logger, observ.Novo(), verificacoes)
}

func TestServidor_HealthzEMetrics(t *testing.T) {
	h := novoServidorTeste().Handler()

	// /healthz responde 200.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(nethttp.MethodGet, "/healthz", nil))
	if w.Code != nethttp.StatusOK {
		t.Fatalf("/healthz: esperava 200, obtive %d", w.Code)
	}

	// /metrics expõe os coletores HTTP da API.
	wm := httptest.NewRecorder()
	h.ServeHTTP(wm, httptest.NewRequest(nethttp.MethodGet, "/metrics", nil))
	if wm.Code != nethttp.StatusOK {
		t.Fatalf("/metrics: esperava 200, obtive %d", wm.Code)
	}
	if !strings.Contains(wm.Body.String(), "sgc_http_pedidos_total") {
		t.Fatal("/metrics não expõe a métrica sgc_http_pedidos_total")
	}
}

func TestServidor_ReadyzComDependenciaEmBaixo(t *testing.T) {
	cfg := config.Config{Ambiente: "dev", PortaHTTP: "0"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	verificacoes := []adhttp.Verificacao{
		{Nome: "fake", Verificar: func(context.Context) error { return context.DeadlineExceeded }},
	}
	h := server.Novo(cfg, logger, observ.Novo(), verificacoes).Handler()

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(nethttp.MethodGet, "/readyz", nil))
	if w.Code != nethttp.StatusServiceUnavailable {
		t.Fatalf("/readyz com dependência em baixo: esperava 503, obtive %d", w.Code)
	}
}
