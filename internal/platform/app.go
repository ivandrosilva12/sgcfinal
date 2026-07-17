// Package platform é o composition root da aplicação (Camada 4). Fia a
// configuração, o logging, a base de dados, a observabilidade, os adaptadores e
// o servidor HTTP. É a única camada autorizada a importar todas as outras.
package platform

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"

	adfarmacia "github.com/ivandrosilva12/sgcfinal/internal/adapters/farmacia"
	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/keycloak"
	adlaboratorio "github.com/ivandrosilva12/sgcfinal/internal/adapters/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	adrecepcao "github.com/ivandrosilva12/sgcfinal/internal/adapters/recepcao"
	adredis "github.com/ivandrosilva12/sgcfinal/internal/adapters/redis"
	adsms "github.com/ivandrosilva12/sgcfinal/internal/adapters/sms"
	adsmtp "github.com/ivandrosilva12/sgcfinal/internal/adapters/smtp"
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
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

	// BC Clínico: episódios e EHR.
	repoEpisodios := pgrepo.NovoRepositorioEpisodios(pool)
	// ACL: a leitura de episódios/EHR usa o LeitorTriagem para incluir a
	// triagem (Recepção) na projecção clínica (ADR-037).
	integracaoConsulta := pgrepo.NovaIntegracaoInicioConsulta(pool)
	handlerEpisodios := adhttp.NovoEpisodiosHandler(
		appclinico.NovoCasoIniciarEpisodio(repoEpisodios, repoDoentes, repoAuditoria),
		appclinico.NovoCasoObterEpisodio(repoEpisodios, integracaoConsulta, repoAuditoria),
		appclinico.NovoCasoListarEpisodios(repoEpisodios, integracaoConsulta),
		appclinico.NovoCasoActualizarEpisodio(repoEpisodios, repoAuditoria),
		appclinico.NovoCasoFecharEpisodio(repoEpisodios, repoAuditoria),
		appclinico.NovoCasoCancelarEpisodio(repoEpisodios, repoAuditoria),
		appclinico.NovoCasoObterEHR(repoDoentes, repoEpisodios, integracaoConsulta, repoAuditoria),
	)

	// BC Clínico: cirurgia ambulatória + consentimentos (LPDP).
	repoConsentimentos := pgrepo.NovoRepositorioConsentimentos(pool)
	repoProcedimentos := pgrepo.NovoRepositorioProcedimentos(pool)
	repoCatalogo := pgrepo.NovoRepositorioCatalogoProcedimentos(pool)
	handlerConsentimentos := adhttp.NovoConsentimentosHandler(
		appclinico.NovoCasoRegistarConsentimento(repoConsentimentos, repoDoentes, repoAuditoria),
		appclinico.NovoCasoRevogarConsentimento(repoConsentimentos, repoAuditoria),
		appclinico.NovoCasoListarConsentimentos(repoConsentimentos),
		appclinico.NovoCasoObterConsentimento(repoConsentimentos),
	)
	handlerCirurgia := adhttp.NovoCirurgiaHandler(
		appclinico.NovoCasoAgendarProcedimento(repoProcedimentos, repoEpisodios, repoConsentimentos, repoCatalogo, repoAuditoria),
		appclinico.NovoCasoIniciarProcedimento(repoProcedimentos, repoEpisodios, repoConsentimentos, repoAuditoria),
		appclinico.NovoCasoConcluirProcedimento(repoProcedimentos, repoAuditoria),
		appclinico.NovoCasoCancelarProcedimento(repoProcedimentos, repoAuditoria),
		appclinico.NovoCasoObterProcedimento(repoProcedimentos),
		appclinico.NovoCasoListarProcedimentos(repoProcedimentos),
	)

	// BC Farmácia: catálogo de medicamentos e receitas.
	repoMedicamentos := pgrepo.NovoRepositorioMedicamentos(pool)
	repoReceitas := pgrepo.NovoRepositorioReceitas(pool)
	leitorClinico := adfarmacia.NovoLeitorClinico(repoDoentes, repoEpisodios)
	handlerFarmacia := adhttp.NovoFarmaciaHandler(
		appfarmacia.NovoCasoRegistarMedicamento(repoMedicamentos, repoAuditoria),
		appfarmacia.NovoCasoActualizarMedicamento(repoMedicamentos, repoAuditoria),
		appfarmacia.NovoCasoDefinirEstadoMedicamento(repoMedicamentos, repoAuditoria),
		appfarmacia.NovoCasoObterMedicamento(repoMedicamentos),
		appfarmacia.NovoCasoPesquisarMedicamentos(repoMedicamentos),
		appfarmacia.NovoCasoEmitirReceita(repoReceitas, repoMedicamentos, leitorClinico, repoAuditoria),
		appfarmacia.NovoCasoAnularReceita(repoReceitas, repoAuditoria),
		appfarmacia.NovoCasoObterReceita(repoReceitas, repoAuditoria),
		appfarmacia.NovoCasoListarReceitas(repoReceitas),
	)

	// BC Farmácia: stock e dispensa.
	repoFornecedores := pgrepo.NovoRepositorioFornecedores(pool)
	repoLotes := pgrepo.NovoRepositorioLotes(pool)
	motorDispensa := pgrepo.NovoMotorDispensa(pool)
	handlerFarmaciaStock := adhttp.NovoFarmaciaStockHandler(
		appfarmacia.NovoCasoRegistarFornecedor(repoFornecedores, repoAuditoria),
		appfarmacia.NovoCasoListarFornecedores(repoFornecedores),
		appfarmacia.NovoCasoRegistarEntradaStock(repoLotes, repoMedicamentos, repoFornecedores, repoAuditoria),
		appfarmacia.NovoCasoConsultarStock(repoLotes),
		appfarmacia.NovoCasoListarLotes(repoLotes),
		appfarmacia.NovoCasoDispensarReceita(repoReceitas, repoMedicamentos, leitorClinico, motorDispensa, repoAuditoria),
	)

	// BC Laboratório (M3): catálogo, requisição, amostra e resultado preliminar.
	repoAnalises := pgrepo.NovoRepositorioAnalises(pool)
	repoRequisicoes := pgrepo.NovoRepositorioRequisicoes(pool)
	repoResultados := pgrepo.NovoRepositorioResultados(pool)
	// ACL: o Laboratório lê o Clínico apenas através deste adaptador.
	leitorClinicoLab := adlaboratorio.NovoLeitorClinico(repoDoentes, repoEpisodios)
	// Alertas de valor crítico por SMS: gateway real se configurado, senão no-op.
	var notificadorCritico applaboratorio.NotificadorCritico
	if cfg.SMSEndpoint == "" {
		notificadorCritico = adsms.NovoNotificadorNulo(logger)
		logger.Info("alertas por SMS desactivados (SMS_ENDPOINT vazio)")
	} else {
		notificadorCritico = adsms.NovoNotificadorSMS(cfg.SMSEndpoint, cfg.SMSRemetente)
		logger.Info("alertas por SMS activados", "endpoint", cfg.SMSEndpoint)
	}
	// ACL de contacto: o telefone do médico vive no BC Identidade.
	resolvedorContacto := adlaboratorio.NovoResolvedorContacto(repoUtilizadores)
	handlerLaboratorio := adhttp.NovoLaboratorioHandler(
		applaboratorio.NovoCasoRegistarAnalise(repoAnalises, repoAuditoria),
		applaboratorio.NovoCasoListarAnalises(repoAnalises),
		applaboratorio.NovoCasoEmitirRequisicao(repoRequisicoes, repoAnalises, leitorClinicoLab, repoAuditoria),
		applaboratorio.NovoCasoObterRequisicao(repoRequisicoes),
		applaboratorio.NovoCasoListarRequisicoesDoEpisodio(repoRequisicoes),
		applaboratorio.NovoCasoColherAmostra(repoResultados, repoAuditoria),
		applaboratorio.NovoCasoRecusarAmostra(repoResultados, repoAuditoria),
		applaboratorio.NovoCasoSubmeterPreliminar(repoResultados, repoAuditoria),
		applaboratorio.NovoCasoListarFila(repoResultados),
		applaboratorio.NovoCasoListarResultadosDoEpisodio(repoResultados),
		applaboratorio.NovoCasoValidarResultado(repoResultados, repoRequisicoes, repoAnalises, resolvedorContacto, notificadorCritico, repoAuditoria),
		applaboratorio.NovoCasoCorrigirResultado(repoResultados, repoRequisicoes, repoAnalises, resolvedorContacto, notificadorCritico, repoAuditoria),
	)

	// BC Recepção (marco Percurso Ambulatório): marcação e agenda por disponibilidade.
	repoJanelas := pgrepo.NovoRepositorioJanelas(pool)
	repoMarcacoes := pgrepo.NovoRepositorioMarcacoes(pool)
	// ACL: a Recepção lê o Clínico apenas através deste adaptador.
	leitorDoenteRec := adrecepcao.NovoLeitorDoente(repoDoentes)
	handlerRecepcao := adhttp.NovoRecepcaoHandler(
		apprecepcao.NovoCasoDefinirJanela(repoJanelas, repoAuditoria),
		apprecepcao.NovoCasoRemoverJanela(repoJanelas, repoMarcacoes, repoAuditoria),
		apprecepcao.NovoCasoMarcar(repoMarcacoes, repoJanelas, leitorDoenteRec, repoAuditoria),
		apprecepcao.NovoCasoRemarcar(repoMarcacoes, repoJanelas, repoAuditoria),
		apprecepcao.NovoCasoCancelar(repoMarcacoes, repoAuditoria),
		apprecepcao.NovoCasoRegistarFalta(repoMarcacoes, repoAuditoria),
		apprecepcao.NovoCasoListarAgenda(repoJanelas, repoMarcacoes),
		apprecepcao.NovoCasoListarMarcacoesDoente(repoMarcacoes),
	)

	// BC Recepção — Check-in (chegada, fila de espera).
	repoChegadas := pgrepo.NovoRepositorioChegadas(pool)
	handlerRecepcaoChegadas := adhttp.NovoRecepcaoChegadasHandler(
		apprecepcao.NovoCasoRegistarChegada(repoChegadas, repoMarcacoes, repoAuditoria),
		apprecepcao.NovoCasoRegistarWalkIn(repoChegadas, leitorDoenteRec, repoAuditoria),
		apprecepcao.NovoCasoChamar(repoChegadas, repoAuditoria),
		apprecepcao.NovoCasoRegistarDesistencia(repoChegadas, repoAuditoria),
		apprecepcao.NovoCasoListarFila(repoChegadas),
	)

	// BC Recepção — Triagem (prioridade Manchester, sinais vitais, fila clínica).
	repoTriagens := pgrepo.NovoRepositorioTriagens(pool)
	handlerRecepcaoTriagem := adhttp.NovoRecepcaoTriagemHandler(
		apprecepcao.NovoCasoRegistarTriagem(repoTriagens, repoChegadas, repoAuditoria),
		apprecepcao.NovoCasoObterTriagem(repoTriagens),
		apprecepcao.NovoCasoListarFilaClinica(repoTriagens),
	)

	// Integração Recepção→Clínico — início da consulta (ADR-036). O adaptador de
	// integração implementa as duas portas (leitor + consumidor transaccional);
	// `integracaoConsulta` já foi construído acima (junto de repoEpisodios).
	handlerClinicoConsulta := adhttp.NovoClinicoConsultaHandler(
		appclinico.NovoCasoIniciarConsulta(integracaoConsulta, integracaoConsulta,
			repoDoentes, repoEpisodios, repoAuditoria),
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
		adhttp.RegistarEpisodios(r, handlerEpisodios, limiteMW, authMW)
		adhttp.RegistarConsentimentos(r, handlerConsentimentos, limiteMW, authMW)
		adhttp.RegistarCirurgia(r, handlerCirurgia, limiteMW, authMW)
		adhttp.RegistarFarmacia(r, handlerFarmacia, limiteMW, authMW)
		adhttp.RegistarFarmaciaStock(r, handlerFarmaciaStock, limiteMW, authMW)
		adhttp.RegistarLaboratorio(r, handlerLaboratorio, limiteMW, authMW)
		adhttp.RegistarRecepcao(r, handlerRecepcao, limiteMW, authMW)
		adhttp.RegistarRecepcaoChegadas(r, handlerRecepcaoChegadas, limiteMW, authMW)
		adhttp.RegistarRecepcaoTriagem(r, handlerRecepcaoTriagem, limiteMW, authMW)
		adhttp.RegistarClinicoConsulta(r, handlerClinicoConsulta, limiteMW, authMW)
	}

	logger.Info("dependências estabelecidas", "ambiente", cfg.Ambiente)
	// ValidarUUIDs corre a nível de engine (antes das rotas de negócio): o match
	// da rota já populou c.Params, pelo que um id malformado devolve 400 sem
	// chegar aos handlers nem à base de dados. Isenção: :papel (enum de negócio).
	srv := server.NovoComRotas(cfg, logger, metricas, verificacoes,
		[]gin.HandlerFunc{segurancaMW, adhttp.ValidarUUIDs("papel")}, registarRotas)

	// Relay do Outbox (ADR-038): publica eventos de domínio inter-BC. Handlers
	// registados por tipo; o loop pára com o ctx do shutdown gracioso.
	relay := outbox.NovoRelay(pool, cfg.OutboxLote, metricas, logger)
	posConsulta := pgrepo.NovaIntegracaoPosConsulta(pool)
	relay.Registar("clinico.episodio.fechado", posConsulta.HandlerEpisodioFechado)
	go relay.Correr(ctx, cfg.OutboxIntervalo)

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
