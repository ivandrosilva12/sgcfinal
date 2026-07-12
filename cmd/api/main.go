// Comando api — entrypoint do SGC Angola.
//
// Uso:
//
//	api              arranca o servidor HTTP (com shutdown gracioso)
//	api migrate      aplica as migrations forward-only e sai
//	api healthcheck  faz GET /healthz local e sai 0/1 (para o container)
//
// @title                     SGC Angola — API
// @version                   0.1.0-M1
// @description               API do Sistema de Gestão de Clínicas privadas em Angola (Marco M1 — Fundações).
// @BasePath                  /
// @schemes                   http
// @contact.name              Equipa SGC Angola
package main

import (
	"context"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/platform"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/log"
)

func main() {
	subcomando := ""
	if len(os.Args) > 1 {
		subcomando = os.Args[1]
	}

	if subcomando == "healthcheck" {
		os.Exit(healthcheck())
	}

	logger := log.Novo(os.Getenv("LOG_LEVEL"))
	// Encaminha os slog.* de pacote (avisos best-effort) para o logger JSON.
	slog.SetDefault(logger)

	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	var err error
	switch subcomando {
	case "migrate":
		err = platform.ExecutarMigracoes(ctx, logger)
	default:
		err = platform.ExecutarServidor(ctx, logger)
	}

	if err != nil {
		logger.Error("falha fatal", "erro", err)
		os.Exit(1)
	}
}

// healthcheck contacta o /healthz local; devolve 0 se 200, 1 caso contrário.
// Permite ao container distroless (sem shell/curl) verificar a própria saúde.
func healthcheck() int {
	// Validar a porta como inteiro no intervalo permitido. O host é sempre
	// loopback, pelo que não existe superfície de SSRF (URL totalmente
	// controlada pela própria configuração).
	porta, err := strconv.Atoi(os.Getenv("HTTP_PORT"))
	if err != nil || porta <= 0 || porta > 65535 {
		porta = 8080
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", porta)

	cliente := &nethttp.Client{Timeout: 3 * time.Second}
	resp, err := cliente.Get(url)
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != nethttp.StatusOK {
		return 1
	}
	return 0
}
