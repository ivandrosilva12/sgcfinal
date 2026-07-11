// Package platform é o composition root da aplicação (Camada 4). Fia a
// configuração, o logging, a base de dados, a observabilidade, os adaptadores e
// o servidor HTTP. É a única camada autorizada a importar todas as outras.
package platform

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/keycloak"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	adredis "github.com/ivandrosilva12/sgcfinal/internal/adapters/redis"
	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/config"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/observ"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/server"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// ExecutarServidor carrega a configuração, estabelece as dependências (BD, Redis,
// Keycloak), monta o BC Identidade e arranca o servidor HTTP, bloqueando até ctx
// ser cancelado.
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

	verificador, err := keycloak.Novo(ctx, cfg.KeycloakIssuer, cfg.KeycloakAudNome)
	if err != nil {
		return fmt.Errorf("inicializar Keycloak: %w", err)
	}

	// BC Identidade: repositórios, casos de uso e handler.
	repoUtilizadores := pgrepo.NovoRepositorioUtilizadores(pool)
	repoAuditoria := pgrepo.NovoRepositorioAuditoria(pool)
	casoAutenticar := appident.NovoCasoAutenticar(verificador)
	casoPerfil := appident.NovoCasoObterPerfil(repoUtilizadores, repoAuditoria)
	handlerIdentidade := adhttp.NovoIdentidadeHandler(casoPerfil)

	// Middlewares transversais e do grupo protegido.
	segurancaMW := adhttp.SegurancaHTTP(cfg.OrigensCORS, cfg.EmProducao())
	limiteMW := adhttp.LimiteTaxa(redisCli.Limitador(), cfg.LimiteTaxaIP, cfg.JanelaTaxa)
	authMW := adhttp.Auth(casoAutenticar)

	metricas := observ.Novo()
	verificacoes := []adhttp.Verificacao{
		{Nome: "postgres", Verificar: pool.Ping},
		{Nome: "redis", Verificar: redisCli.Ping},
		{Nome: "keycloak", Verificar: verificador.VerificarSaude},
	}

	registarRotas := func(r gin.IRouter) {
		adhttp.RegistarIdentidade(r, handlerIdentidade, limiteMW, authMW)
	}

	logger.Info("dependências estabelecidas", "ambiente", cfg.Ambiente)
	srv := server.NovoComRotas(cfg, logger, metricas, verificacoes,
		[]gin.HandlerFunc{segurancaMW}, registarRotas)
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
