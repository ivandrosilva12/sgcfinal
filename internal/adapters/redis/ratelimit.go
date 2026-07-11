package redis

import (
	"context"
	"fmt"
	"time"
)

// LimitadorTaxa implementa rate limiting por janela fixa sobre Redis (INCR +
// EXPIRE). É partilhável entre chaves (por IP, por utilizador, por endpoint).
type LimitadorTaxa struct {
	cli *Cliente
}

// Limitador constrói um LimitadorTaxa a partir do cliente Redis.
func (c *Cliente) Limitador() *LimitadorTaxa {
	return &LimitadorTaxa{cli: c}
}

// Permitir regista mais um acesso à chave dentro da janela e indica se está
// dentro do limite. Devolve também o número de pedidos restantes e, quando
// excedido, o tempo até a janela reiniciar (para o cabeçalho Retry-After).
func (l *LimitadorTaxa) Permitir(ctx context.Context, chave string, limite int, janela time.Duration) (bool, int, time.Duration, error) {
	n, err := l.cli.rdb.Incr(ctx, chave).Result()
	if err != nil {
		return false, 0, 0, fmt.Errorf("rate limit incr: %w", err)
	}
	if n == 1 {
		// Primeiro acesso na janela: definir a expiração.
		if err := l.cli.rdb.Expire(ctx, chave, janela).Err(); err != nil {
			return false, 0, 0, fmt.Errorf("rate limit expire: %w", err)
		}
	}

	if int(n) > limite {
		ttl, err := l.cli.rdb.TTL(ctx, chave).Result()
		if err != nil || ttl < 0 {
			ttl = janela
		}
		return false, 0, ttl, nil
	}
	return true, limite - int(n), 0, nil
}
