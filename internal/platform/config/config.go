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
	Ambiente                  string        // "dev" | "staging" | "prod"
	PortaHTTP                 string        // porta do servidor HTTP (ex.: "8080")
	NivelLog                  string        // "debug" | "info" | "warn" | "error"
	URLBaseDados              string        // DSN PostgreSQL (pgx)
	URLMigracaoBaseDados      string        // DSN do migrador (ADR-043); opcional — o servidor corre sem ela
	URLRedis                  string        // URL Redis (redis://...)
	TimeoutParagem            time.Duration // tempo máximo de shutdown gracioso
	KeycloakIssuer            string        // issuer OIDC (obrigatório desde Sprint 2)
	KeycloakAudNome           string        // audience/client esperado (Sprint 2)
	KeycloakAdminClientID     string        // client confidencial para a Admin API (sgc-admin)
	KeycloakAdminClientSecret string        // segredo do client admin
	KeycloakACRFortes         []string      // valores de acr considerados MFA
	OrigensCORS               []string      // origens permitidas em CORS (por ambiente)
	LimiteTaxaIP              int           // limite de pedidos por IP na janela de taxa
	JanelaTaxa                time.Duration // janela do rate limiting
	SMTPHost                  string        // host SMTP para notificações (vazio → notificador no-op)
	SMTPPorta                 string        // porta SMTP (default 1025 — MailHog)
	SMTPRemetente             string        // remetente dos emails (From)
	SMSEndpoint               string        // endpoint HTTP do gateway SMS (vazio → notificador no-op)
	SMSRemetente              string        // remetente (sender id) das mensagens SMS
	OutboxIntervalo           time.Duration // intervalo entre passagens do relay do outbox (ADR-038)
	OutboxLote                int           // máximo de eventos por passagem do relay
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
		Ambiente:                  ambiente,
		PortaHTTP:                 valorOu("HTTP_PORT", "8080"),
		NivelLog:                  valorOu("LOG_LEVEL", "info"),
		URLBaseDados:              os.Getenv("DATABASE_URL"),
		URLMigracaoBaseDados:      os.Getenv("DATABASE_MIGRATION_URL"),
		URLRedis:                  os.Getenv("REDIS_URL"),
		TimeoutParagem:            15 * time.Second,
		KeycloakIssuer:            os.Getenv("KEYCLOAK_ISSUER"),
		KeycloakAudNome:           os.Getenv("KEYCLOAK_AUDIENCE"),
		KeycloakAdminClientID:     os.Getenv("KEYCLOAK_ADMIN_CLIENT_ID"),
		KeycloakAdminClientSecret: os.Getenv("KEYCLOAK_ADMIN_CLIENT_SECRET"),
		KeycloakACRFortes:         parseLista(valorOu("KEYCLOAK_ACR_FORTE", "mfa,gold,2")),
		OrigensCORS:               parseCORS(os.Getenv("CORS_ORIGINS"), ambiente),
		LimiteTaxaIP:              inteiroOu("RATE_LIMIT_IP", 100),
		JanelaTaxa:                time.Minute,
		SMTPHost:                  os.Getenv("SMTP_HOST"),
		SMTPPorta:                 valorOu("SMTP_PORT", "1025"),
		SMTPRemetente:             valorOu("SMTP_FROM", "nao-responder@sgc.ao"),
		SMSEndpoint:               os.Getenv("SMS_ENDPOINT"),
		SMSRemetente:              valorOu("SMS_FROM", "SGC"),
		OutboxIntervalo:           time.Duration(inteiroOu("OUTBOX_INTERVALO_MS", 2000)) * time.Millisecond,
		OutboxLote:                inteiroOu("OUTBOX_LOTE", 100),
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
	if cfg.KeycloakAdminClientID == "" {
		erro.faltam = append(erro.faltam, "KEYCLOAK_ADMIN_CLIENT_ID")
	}
	if cfg.KeycloakAdminClientSecret == "" {
		erro.faltam = append(erro.faltam, "KEYCLOAK_ADMIN_CLIENT_SECRET")
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

// parseLista interpreta uma lista separada por vírgulas, ignorando vazios.
func parseLista(raw string) []string {
	partes := strings.Split(raw, ",")
	out := make([]string, 0, len(partes))
	for _, p := range partes {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
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
