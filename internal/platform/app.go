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
	adsmtp "github.com/ivandrosilva12/sgcfinal/internal/adapters/smtp"
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
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

	verificador, err := keycloak.Novo(ctx, cfg.KeycloakIssuer, cfg.KeycloakAudNome, cfg.KeycloakACRFortes)
	if err != nil {
		return fmt.Errorf("inicializar Keycloak: %w", err)
	}

	adminKC, err := keycloak.NovoAdmin(cfg.KeycloakIssuer, cfg.KeycloakAdminClientID, cfg.KeycloakAdminClientSecret)
	if err != nil {
		return fmt.Errorf("inicializar Keycloak admin: %w", err)
	}

	var notificador appident.Notificador
	if cfg.SMTPHost == "" {
		notificador = adsmtp.NovoNotificadorNulo(logger)
		logger.Info("notificações por email desactivadas (SMTP_HOST vazio)")
	} else {
		notificador = adsmtp.NovoNotificadorSMTP(cfg.SMTPHost, cfg.SMTPPorta, cfg.SMTPRemetente)
		logger.Info("notificações por email activadas", "smtp", cfg.SMTPHost+":"+cfg.SMTPPorta)
	}

	// BC Identidade: repositórios, casos de uso e handler.
	repoUtilizadores := pgrepo.NovoRepositorioUtilizadores(pool)
	repoAuditoria := pgrepo.NovoRepositorioAuditoria(pool)
	casoAutenticar := appident.NovoCasoAutenticar(verificador)
	casoPerfil := appident.NovoCasoObterPerfil(repoUtilizadores, repoAuditoria)
	casoAtualizarPerfil := appident.NovoCasoAtualizarPerfil(repoUtilizadores, repoAuditoria)
	handlerIdentidade := adhttp.NovoIdentidadeHandler(casoPerfil, casoAtualizarPerfil)

	casoListar := appident.NovoCasoListarUtilizadores(adminKC)
	casoObter := appident.NovoCasoObterUtilizador(adminKC)
	casoAtribuir := appident.NovoCasoAtribuirPapel(adminKC, repoAuditoria)
	casoRevogar := appident.NovoCasoRevogarPapel(adminKC, repoAuditoria)
	casoActivo := appident.NovoCasoDefinirActivo(adminKC, repoAuditoria)
	casoCriar := appident.NovoCasoCriarUtilizador(adminKC, repoAuditoria, notificador)
	casoResetPass := appident.NovoCasoResetPassword(adminKC, repoAuditoria, notificador)
	casoResetOTP := appident.NovoCasoResetOTP(adminKC, repoAuditoria)
	casoListarSessoes := appident.NovoCasoListarSessoes(adminKC)
	casoRevogarSessao := appident.NovoCasoRevogarSessao(adminKC, repoAuditoria)
	casoEditarPerfilAdmin := appident.NovoCasoEditarPerfilAdmin(adminKC, repoUtilizadores, repoAuditoria)
	handlerAdmin := adhttp.NovoAdministracaoHandler(
		casoListar, casoObter, casoAtribuir, casoRevogar, casoActivo, casoCriar,
		casoResetPass, casoResetOTP, casoListarSessoes, casoRevogarSessao, casoEditarPerfilAdmin,
	)

	// BC Clínico: repositório, casos de uso e handler do agregado Doente.
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	handlerDoentes := adhttp.NovoDoentesHandler(
		appclinico.NovoCasoRegistarDoente(repoDoentes, repoAuditoria),
		appclinico.NovoCasoObterDoente(repoDoentes, repoAuditoria),
		appclinico.NovoCasoPesquisarDoentes(repoDoentes),
		appclinico.NovoCasoActualizarDoente(repoDoentes, repoAuditoria),
		appclinico.NovoCasoGerirEstadoDoente(repoDoentes, repoAuditoria),
		appclinico.NovoCasoRegistarAlergia(repoDoentes, repoAuditoria),
		appclinico.NovoCasoRegistarAntecedente(repoDoentes, repoAuditoria),
	)

	// Middlewares transversais e do grupo protegido.
	segurancaMW := adhttp.SegurancaHTTP(cfg.OrigensCORS, cfg.EmProducao())
	limiteMW := adhttp.LimiteTaxa(redisCli.Limitador(), cfg.LimiteTaxaIP, cfg.JanelaTaxa)
	authMW := adhttp.Auth(casoAutenticar)
	mfaMW := adhttp.MFAObrigatoria()

	metricas := observ.Novo()
	verificacoes := []adhttp.Verificacao{
		{Nome: "postgres", Verificar: pool.Ping},
		{Nome: "redis", Verificar: redisCli.Ping},
		{Nome: "keycloak", Verificar: verificador.VerificarSaude},
	}

	registarRotas := func(r gin.IRouter) {
		adhttp.RegistarIdentidade(r, handlerIdentidade, limiteMW, authMW, mfaMW)
		adhttp.RegistarAdministracao(r, handlerAdmin, limiteMW, authMW, mfaMW)
		adhttp.RegistarDoentes(r, handlerDoentes, limiteMW, authMW)
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
