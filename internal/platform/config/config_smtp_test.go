package config

import "testing"

func prepararEnvObrigatorio(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("REDIS_URL", "redis://x")
	t.Setenv("KEYCLOAK_ISSUER", "http://kc/realms/sgc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "sgc-admin")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "segredo")
}

func TestCarregar_SMTPDefaults(t *testing.T) {
	prepararEnvObrigatorio(t)
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("SMTP_FROM", "")

	cfg, err := Carregar()
	if err != nil {
		t.Fatalf("Carregar: %v", err)
	}
	if cfg.SMTPHost != "" {
		t.Fatalf("SMTPHost default devia ser vazio, obtive %q", cfg.SMTPHost)
	}
	if cfg.SMTPPorta != "1025" {
		t.Fatalf("SMTPPorta default = %q; quer 1025", cfg.SMTPPorta)
	}
	if cfg.SMTPRemetente != "nao-responder@sgc.ao" {
		t.Fatalf("SMTPRemetente default = %q; quer nao-responder@sgc.ao", cfg.SMTPRemetente)
	}
}

func TestCarregar_SMTPConfigurado(t *testing.T) {
	prepararEnvObrigatorio(t)
	t.Setenv("SMTP_HOST", "mailhog")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_FROM", "sgc@clinica.ao")

	cfg, err := Carregar()
	if err != nil {
		t.Fatalf("Carregar: %v", err)
	}
	if cfg.SMTPHost != "mailhog" || cfg.SMTPPorta != "2525" || cfg.SMTPRemetente != "sgc@clinica.ao" {
		t.Fatalf("SMTP não lido correctamente: %+v", cfg)
	}
}
