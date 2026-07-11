package config_test

import (
	"testing"

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
}

func TestCarregar_AmbienteInvalido(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/sgc")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("APP_ENV", "producao-errada")
	if _, err := config.Carregar(); err == nil {
		t.Fatal("esperava erro por APP_ENV inválido")
	}
}
