package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso de consentimento.
type (
	// ServicoRegistarConsentimento regista um consentimento.
	ServicoRegistarConsentimento interface {
		Executar(ctx context.Context, actor string, dados appclinico.DadosNovoConsentimento) (appclinico.DetalheConsentimento, error)
	}
	// ServicoRevogarConsentimento revoga um consentimento.
	ServicoRevogarConsentimento interface {
		Executar(ctx context.Context, actor, id string) (appclinico.DetalheConsentimento, error)
	}
	// ServicoListarConsentimentos lista os consentimentos de um doente.
	ServicoListarConsentimentos interface {
		Executar(ctx context.Context, doenteID string, filtro appclinico.FiltroConsentimentos) ([]appclinico.ResumoConsentimento, error)
	}
	// ServicoObterConsentimento devolve o detalhe de um consentimento.
	ServicoObterConsentimento interface {
		Executar(ctx context.Context, id string) (appclinico.DetalheConsentimento, error)
	}
)

// ConsentimentosHandler expõe os endpoints HTTP de consentimentos LPDP.
type ConsentimentosHandler struct {
	registar ServicoRegistarConsentimento
	revogar  ServicoRevogarConsentimento
	listar   ServicoListarConsentimentos
	obter    ServicoObterConsentimento
}

// NovoConsentimentosHandler constrói o handler.
func NovoConsentimentosHandler(
	registar ServicoRegistarConsentimento, revogar ServicoRevogarConsentimento,
	listar ServicoListarConsentimentos, obter ServicoObterConsentimento,
) *ConsentimentosHandler {
	return &ConsentimentosHandler{registar: registar, revogar: revogar, listar: listar, obter: obter}
}

// RegistarConsentimentos regista as rotas, aplicando `protecao` e o RBAC por rota.
func RegistarConsentimentos(r gin.IRouter, h *ConsentimentosHandler, protecao ...gin.HandlerFunc) {
	leituraClinica := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelAdministrativo,
		dominio.PapelDirector, dominio.PapelDPO, dominio.PapelAuditor)
	escritaConsent := RBAC(dominio.PapelMedico, dominio.PapelAdministrativo)

	gd := r.Group("/api/v1/doentes")
	gd.Use(protecao...)
	gd.POST("/:id/consentimentos", escritaConsent, h.registarConsentimento)
	gd.GET("/:id/consentimentos", leituraClinica, h.listarConsentimentos)

	gc := r.Group("/api/v1/consentimentos")
	gc.Use(protecao...)
	gc.GET("/:cid", leituraClinica, h.obterConsentimento)
	gc.POST("/:cid/revogar", escritaConsent, h.revogarConsentimento)
}

type corpoConsentimento struct {
	Finalidade   string `json:"finalidade"`
	Concedido    bool   `json:"concedido"`
	DocumentoURL string `json:"documento_url"`
}

func (h *ConsentimentosHandler) registarConsentimento(c *gin.Context) {
	var corpo corpoConsentimento
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registar.Executar(c.Request.Context(), actor.Sujeito, appclinico.DadosNovoConsentimento{
		DoenteID: c.Param("id"), Finalidade: corpo.Finalidade,
		Concedido: corpo.Concedido, DocumentoURL: corpo.DocumentoURL,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *ConsentimentosHandler) listarConsentimentos(c *gin.Context) {
	out, err := h.listar.Executar(c.Request.Context(), c.Param("id"), appclinico.FiltroConsentimentos{
		Finalidade:     c.Query("finalidade"),
		ApenasVigentes: c.Query("vigentes") == "true",
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

func (h *ConsentimentosHandler) obterConsentimento(c *gin.Context) {
	out, err := h.obter.Executar(c.Request.Context(), c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *ConsentimentosHandler) revogarConsentimento(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.revogar.Executar(c.Request.Context(), actor.Sujeito, c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
