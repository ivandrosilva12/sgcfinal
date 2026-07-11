// Package log configura o logging estruturado da aplicação com slog em formato
// JSON (destinado a journald → agregação). Camada 4 — Plataforma.
package log

import (
	"log/slog"
	"os"
	"strings"
)

// Novo cria um *slog.Logger em JSON com o nível indicado ("debug", "info",
// "warn", "error"). Níveis desconhecidos assumem "info".
func Novo(nivel string) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: nivelSlog(nivel),
	})
	return slog.New(h)
}

func nivelSlog(nivel string) slog.Level {
	switch strings.ToLower(nivel) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
