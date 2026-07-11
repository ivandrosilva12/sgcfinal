// Package platform é o composition root da aplicação (Camada 4). Fia a
// configuração, o logging, a base de dados, a observabilidade e o servidor
// HTTP. É a única camada autorizada a importar todas as outras.
package platform

import (
	"context"
	"fmt"
	"log/slog"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	adredis "github.com/ivandrosilva12/sgcfinal/internal/adapters/redis"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/config"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/observ"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/server"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// ExecutarServidor carrega a configuração, estabelece as dependências e arranca
// o servidor HTTP, bloqueando até ctx ser cancelado.
func ExecutarServidor(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}

	pool, err := db.LigarPool(ctx, cfg.URLBaseDados)
	if err != nil {
		return err
	}
	defer pool.Close()

	redisCli, err := adredis.Ligar(cfg.URLRedis)
	if err != nil {
		return err
	}
	defer redisCli.Fechar() //nolint:errcheck // best-effort no encerramento

	metricas := observ.Novo()
	verificacoes := []adhttp.Verificacao{
		{Nome: "postgres", Verificar: pool.Ping},
		{Nome: "redis", Verificar: redisCli.Ping},
	}

	logger.Info("dependências estabelecidas", "ambiente", cfg.Ambiente)
	srv := server.Novo(cfg, logger, metricas, verificacoes)
	return srv.Iniciar(ctx)
}

// ExecutarMigracoes carrega a configuração e aplica as migrations forward-only
// embebidas, saindo no fim. Usado por `make migrate` (subcomando "migrate").
func ExecutarMigracoes(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}

	pool, err := db.LigarPool(ctx, cfg.URLBaseDados)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		return fmt.Errorf("aplicar migrations: %w", err)
	}
	return nil
}
