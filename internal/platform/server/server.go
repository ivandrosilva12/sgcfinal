// Package server constrói e opera o servidor HTTP (Gin) com shutdown gracioso.
// Camada 4 — Plataforma.
package server

import (
	"context"
	"errors"
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/config"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/observ"
)

// Servidor encapsula o http.Server e a sua configuração.
type Servidor struct {
	http    *nethttp.Server
	logger  *slog.Logger
	timeout time.Duration
}

// Novo monta o router Gin base (middleware, /metrics e endpoints de saúde) sem
// rotas de negócio. Mantido para testes e usos simples.
func Novo(cfg config.Config, logger *slog.Logger, metricas *observ.Metricas, verificacoes []adhttp.Verificacao) *Servidor {
	return NovoComRotas(cfg, logger, metricas, verificacoes, nil, nil)
}

// NovoComRotas monta o router Gin com middleware base, middlewares globais
// adicionais (ex.: cabeçalhos de segurança) e um registador de rotas de negócio
// (ex.: BC Identidade). /metrics e os endpoints de saúde permanecem públicos e
// fora do rate limiting (para não afectar o scrape do Prometheus).
func NovoComRotas(
	cfg config.Config,
	logger *slog.Logger,
	metricas *observ.Metricas,
	verificacoes []adhttp.Verificacao,
	globais []gin.HandlerFunc,
	registar func(gin.IRouter),
) *Servidor {
	if cfg.EmProducao() {
		gin.SetMode(gin.ReleaseMode)
	}

	eng := gin.New()
	eng.Use(gin.Recovery())
	eng.Use(adhttp.RequestID())
	eng.Use(adhttp.Logging(logger))
	eng.Use(metricas.Middleware())
	for _, m := range globais {
		eng.Use(m)
	}

	eng.GET("/metrics", gin.WrapH(metricas.Handler()))
	adhttp.RegistarHealth(eng, verificacoes)

	if registar != nil {
		registar(eng)
	}

	srv := &nethttp.Server{
		Addr:              ":" + cfg.PortaHTTP,
		Handler:           eng,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &Servidor{http: srv, logger: logger, timeout: cfg.TimeoutParagem}
}

// Handler devolve o http.Handler subjacente (router Gin já configurado).
// Útil para testes que exercem o servidor sem abrir uma porta.
func (s *Servidor) Handler() nethttp.Handler {
	return s.http.Handler
}

// Iniciar arranca o servidor e bloqueia até ctx ser cancelado (SIGINT/SIGTERM),
// altura em que executa um shutdown gracioso dentro do timeout configurado.
func (s *Servidor) Iniciar(ctx context.Context) error {
	erros := make(chan error, 1)
	go func() {
		s.logger.Info("servidor HTTP a arrancar", "endereco", s.http.Addr)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			erros <- err
		}
	}()

	select {
	case err := <-erros:
		return err
	case <-ctx.Done():
		s.logger.Info("sinal de paragem recebido; a encerrar graciosamente")
		ctxParagem, cancel := context.WithTimeout(context.Background(), s.timeout)
		defer cancel()
		if err := s.http.Shutdown(ctxParagem); err != nil {
			return err
		}
		s.logger.Info("servidor HTTP encerrado")
		return nil
	}
}
