// Package config carrega e valida a configuração da aplicação a partir do
// ambiente. A validação é explícita e falha no arranque (devolve erro, nunca
// panic) se faltar configuração obrigatória. Camada 4 — Plataforma.
package config

import (
	"fmt"
	"os"
	"strconv"
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
	KeycloakIssuer  string        // issuer OIDC (obrigatório desde Sprint 2)
	KeycloakAudNome string        // audience/client esperado (Sprint 2)
	OrigensCORS     []string      // origens permitidas em CORS (por ambiente)
	LimiteTaxaIP    int           // limite de pedidos por IP na janela de taxa
	JanelaTaxa      time.Duration // janela do rate limiting
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
	ambiente := valorOu("APP_ENV", "dev")
	cfg := Config{
		Ambiente:        ambiente,
		PortaHTTP:       valorOu("HTTP_PORT", "8080"),
		NivelLog:        valorOu("LOG_LEVEL", "info"),
		URLBaseDados:    os.Getenv("DATABASE_URL"),
		URLRedis:        os.Getenv("REDIS_URL"),
		TimeoutParagem:  15 * time.Second,
		KeycloakIssuer:  os.Getenv("KEYCLOAK_ISSUER"),
		KeycloakAudNome: os.Getenv("KEYCLOAK_AUDIENCE"),
		OrigensCORS:     parseCORS(os.Getenv("CORS_ORIGINS"), ambiente),
		LimiteTaxaIP:    inteiroOu("RATE_LIMIT_IP", 100),
		JanelaTaxa:      time.Minute,
	}

	erro := &erroConfig{}
	if cfg.URLBaseDados == "" {
		erro.faltam = append(erro.faltam, "DATABASE_URL")
	}
	if cfg.URLRedis == "" {
		erro.faltam = append(erro.faltam, "REDIS_URL")
	}
	if cfg.KeycloakIssuer == "" {
		erro.faltam = append(erro.faltam, "KEYCLOAK_ISSUER")
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

// parseCORS interpreta CORS_ORIGINS (lista separada por vírgulas). Se vazio, em
// dev permite todas as origens ("*"); noutros ambientes não permite nenhuma
// (lista vazia) — CORS tem de ser configurado explicitamente em staging/prod.
func parseCORS(raw, ambiente string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if ambiente == "dev" {
			return []string{"*"}
		}
		return nil
	}
	partes := strings.Split(raw, ",")
	origens := make([]string, 0, len(partes))
	for _, p := range partes {
		if o := strings.TrimSpace(p); o != "" {
			origens = append(origens, o)
		}
	}
	return origens
}

// inteiroOu lê um inteiro do ambiente; devolve omissao se ausente ou inválido.
func inteiroOu(chave string, omissao int) int {
	if v := os.Getenv(chave); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return omissao
}
