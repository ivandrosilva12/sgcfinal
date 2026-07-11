package redis_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/redis"
)

func TestLigar_URLInvalida(t *testing.T) {
	if _, err := redis.Ligar("isto-nao-e-uma-url"); err == nil {
		t.Fatal("esperava erro para URL de Redis inválida")
	}
}

func TestLigar_URLValida(t *testing.T) {
	// ParseURL válido não estabelece ligação; apenas cria o cliente.
	c, err := redis.Ligar("redis://localhost:6379/0")
	if err != nil {
		t.Fatalf("esperava sucesso a criar cliente, obtive %v", err)
	}
	if c == nil {
		t.Fatal("cliente não deve ser nil")
	}
	if err := c.Fechar(); err != nil {
		t.Fatalf("Fechar não deve falhar: %v", err)
	}
}
