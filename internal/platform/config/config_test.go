package config_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/platform/config"
)

func TestCarregar_FaltamObrigatorias(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "")
	if _, err := config.Carregar(); err == nil {
		t.Fatal("esperava erro por faltarem DATABASE_URL e REDIS_URL")
	}
}

func TestCarregar_Valido(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/sgc")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("KEYCLOAK_ISSUER", "http://localhost:8081/realms/sgc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "sgc-admin")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "segredo-admin")
	t.Setenv("APP_ENV", "dev")

	cfg, err := config.Carregar()
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if cfg.PortaHTTP != "8080" {
		t.Fatalf("porta por omissão errada: %q", cfg.PortaHTTP)
	}
	if cfg.EmProducao() {
		t.Fatal("dev não deve ser produção")
	}
	if len(cfg.OrigensCORS) != 1 || cfg.OrigensCORS[0] != "*" {
		t.Fatalf("CORS por omissão em dev errado: %v", cfg.OrigensCORS)
	}
	if cfg.LimiteTaxaIP != 100 {
		t.Fatalf("limite de taxa por omissão errado: %d", cfg.LimiteTaxaIP)
	}
	if cfg.KeycloakAdminClientID != "sgc-admin" {
		t.Fatalf("admin client id errado: %q", cfg.KeycloakAdminClientID)
	}
	if len(cfg.KeycloakACRFortes) == 0 {
		t.Fatal("esperava lista de ACR fortes por omissão")
	}
}

func TestCarregar_FaltaKeycloakIssuer(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/sgc")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("KEYCLOAK_ISSUER", "")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "sgc-admin")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "segredo-admin")
	t.Setenv("APP_ENV", "dev")
	if _, err := config.Carregar(); err == nil {
		t.Fatal("esperava erro por faltar KEYCLOAK_ISSUER")
	}
}

func TestCarregar_AmbienteInvalido(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/sgc")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("KEYCLOAK_ISSUER", "http://localhost:8081/realms/sgc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "sgc-admin")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "segredo-admin")
	t.Setenv("APP_ENV", "producao-errada")
	if _, err := config.Carregar(); err == nil {
		t.Fatal("esperava erro por APP_ENV inválido")
	}
}

func TestCarregar_OutboxDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("REDIS_URL", "redis://x")
	t.Setenv("KEYCLOAK_ISSUER", "http://kc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "id")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "s")
	cfg, err := config.Carregar()
	if err != nil {
		t.Fatalf("carregar: %v", err)
	}
	if cfg.OutboxIntervalo != 2*time.Second {
		t.Fatalf("intervalo por omissão errado: %v", cfg.OutboxIntervalo)
	}
	if cfg.OutboxLote != 100 {
		t.Fatalf("lote por omissão errado: %d", cfg.OutboxLote)
	}
}

func TestCarregar_FaltaAdminClient(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/sgc")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("KEYCLOAK_ISSUER", "http://localhost:8081/realms/sgc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "")
	t.Setenv("APP_ENV", "dev")
	if _, err := config.Carregar(); err == nil {
		t.Fatal("esperava erro por faltar KEYCLOAK_ADMIN_CLIENT_ID/SECRET")
	}
}
