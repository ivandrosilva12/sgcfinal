// Package redis fornece o cliente Redis usado para cache, sessões e (a partir
// de Sprint 2) rate limiting. Em M1/Sprint 1 expõe apenas ligação e verificação
// de saúde (Ping) para o endpoint /readyz. Camada 3 — Adaptadores.
package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

// Cliente encapsula a ligação ao Redis.
type Cliente struct {
	rdb *goredis.Client
}

// Ligar cria um cliente Redis a partir de uma URL (redis://...). Não contacta o
// servidor; use Ping para verificar disponibilidade.
func Ligar(url string) (*Cliente, error) {
	opt, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("URL de Redis inválida: %w", err)
	}
	return &Cliente{rdb: goredis.NewClient(opt)}, nil
}

// Ping verifica a disponibilidade do Redis. Usado pelo /readyz.
func (c *Cliente) Ping(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis indisponível: %w", err)
	}
	return nil
}

// Fechar liberta a ligação.
func (c *Cliente) Fechar() error {
	return c.rdb.Close()
}
