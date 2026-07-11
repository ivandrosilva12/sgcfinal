// Package config carrega e valida a configuração da aplicação a partir do
// ambiente. A validação é explícita e falha no arranque (devolve erro, nunca
// panic) se faltar configuração obrigatória. Camada 4 — Plataforma.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config agrega toda a configuração da API.
type Config struct {
	Ambiente        string        // "dev" | "staging" | "prod"
	PortaHTTP       string        // porta do servidor HTTP (ex.: "8080")
	NivelLog        string        // "debug" | "info" | "warn" | "error"
	URLBaseDados    string        // DSN PostgreSQL (pgx)
	URLRedis        string        // URL Redis (redis://...)
	TimeoutParagem  time.Duration // tempo máximo de shutdown gracioso
	KeycloakIssuer  string        // issuer OIDC (usado a partir de Sprint 2)
	KeycloakAudNome string        // audience/client esperado (Sprint 2)
}

// erroConfig acumula erros de validação para reportar todos de uma vez.
type erroConfig struct {
	faltam []string
}

func (e *erroConfig) Error() string {
	return "configuração inválida: variáveis em falta ou inválidas: " + strings.Join(e.faltam, ", ")
}

// Carregar lê a configuração do ambiente e valida-a. Devolve erro se alguma
// variável obrigatória estiver ausente.
func Carregar() (Config, error) {
	cfg := Config{
		Ambiente:        valorOu("APP_ENV", "dev"),
		PortaHTTP:       valorOu("HTTP_PORT", "8080"),
		NivelLog:        valorOu("LOG_LEVEL", "info"),
		URLBaseDados:    os.Getenv("DATABASE_URL"),
		URLRedis:        os.Getenv("REDIS_URL"),
		TimeoutParagem:  15 * time.Second,
		KeycloakIssuer:  os.Getenv("KEYCLOAK_ISSUER"),
		KeycloakAudNome: os.Getenv("KEYCLOAK_AUDIENCE"),
	}

	erro := &erroConfig{}
	if cfg.URLBaseDados == "" {
		erro.faltam = append(erro.faltam, "DATABASE_URL")
	}
	if cfg.URLRedis == "" {
		erro.faltam = append(erro.faltam, "REDIS_URL")
	}
	if !ambienteValido(cfg.Ambiente) {
		erro.faltam = append(erro.faltam, fmt.Sprintf("APP_ENV (valor inválido: %q)", cfg.Ambiente))
	}

	if len(erro.faltam) > 0 {
		return Config{}, erro
	}
	return cfg, nil
}

// EmProducao indica se o ambiente é de produção.
func (c Config) EmProducao() bool {
	return c.Ambiente == "prod"
}

func valorOu(chave, omissao string) string {
	if v := os.Getenv(chave); v != "" {
		return v
	}
	return omissao
}

func ambienteValido(a string) bool {
	switch a {
	case "dev", "staging", "prod":
		return true
	default:
		return false
	}
}
