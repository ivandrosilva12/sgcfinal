package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Serviços de administração (casos de uso de application/identidade).
type (
	// ServicoListar lista utilizadores.
	ServicoListar interface {
		Executar(ctx context.Context, f appident.FiltroUtilizadores) ([]appident.ResumoUtilizador, error)
	}
	// ServicoObterUtilizador devolve o detalhe de um utilizador.
	ServicoObterUtilizador interface {
		Executar(ctx context.Context, id string) (appident.DetalheUtilizador, error)
	}
	// ServicoAtribuirPapel atribui um papel.
	ServicoAtribuirPapel interface {
		Executar(ctx context.Context, actor, id string, papel dominio.Papel) error
	}
	// ServicoRevogarPapel revoga um papel.
	ServicoRevogarPapel interface {
		Executar(ctx context.Context, actor, id string, papel dominio.Papel) error
	}
	// ServicoDefinirActivo activa/desactiva um utilizador.
	ServicoDefinirActivo interface {
		Executar(ctx context.Context, actor, id string, activo bool) error
	}
	// ServicoCriarUtilizador cria um utilizador.
	ServicoCriarUtilizador interface {
		Executar(ctx context.Context, actor string, entrada appident.CriacaoUtilizador) (appident.UtilizadorCriado, error)
	}
)

// AdministracaoHandler expõe os endpoints de gestão de utilizadores/papéis.
type AdministracaoHandler struct {
	listar   ServicoListar
	obter    ServicoObterUtilizador
	atribuir ServicoAtribuirPapel
	revogar  ServicoRevogarPapel
	activar  ServicoDefinirActivo
	criar    ServicoCriarUtilizador
}

// NovoAdministracaoHandler constrói o handler com os casos de uso.
func NovoAdministracaoHandler(
	listar ServicoListar,
	obter ServicoObterUtilizador,
	atribuir ServicoAtribuirPapel,
	revogar ServicoRevogarPapel,
	activar ServicoDefinirActivo,
	criar ServicoCriarUtilizador,
) *AdministracaoHandler {
	return &AdministracaoHandler{listar: listar, obter: obter, atribuir: atribuir, revogar: revogar, activar: activar, criar: criar}
}

// RegistarAdministracao regista as rotas sob /api/v1/identidade/utilizadores. Os
// middlewares `protecao` (rate limit + Auth + MFAObrigatoria) aplicam-se ao grupo;
// o RBAC por papel é aplicado por rota (escrita: Admin; leitura: Admin/Auditor/DPO).
func RegistarAdministracao(r gin.IRouter, h *AdministracaoHandler, protecao ...gin.HandlerFunc) {
	g := r.Group("/api/v1/identidade/utilizadores")
	g.Use(protecao...)

	leitura := RBAC(dominio.PapelAdmin, dominio.PapelAuditor, dominio.PapelDPO)
	escrita := RBAC(dominio.PapelAdmin)

	g.GET("", leitura, h.listarUtilizadores)
	g.POST("", escrita, h.criarUtilizador)
	g.GET("/:id", leitura, h.obterUtilizador)
	g.POST("/:id/papeis", escrita, h.atribuirPapel)
	g.DELETE("/:id/papeis/:papel", escrita, h.revogarPapel)
	g.PATCH("/:id", escrita, h.definirActivo)
}

func (h *AdministracaoHandler) listarUtilizadores(c *gin.Context) {
	out, err := h.listar.Executar(c.Request.Context(), appident.FiltroUtilizadores{Termo: c.Query("q")})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *AdministracaoHandler) obterUtilizador(c *gin.Context) {
	out, err := h.obter.Executar(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoPapel struct {
	Papel string `json:"papel"`
}

func (h *AdministracaoHandler) atribuirPapel(c *gin.Context) {
	var corpo corpoPapel
	if err := c.ShouldBindJSON(&corpo); err != nil || corpo.Papel == "" {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPapelInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	if err := h.atribuir.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), dominio.Papel(corpo.Papel)); err != nil {
		responderErro(c, err)
		return
	}
	c.Status(nethttp.StatusNoContent)
}

func (h *AdministracaoHandler) revogarPapel(c *gin.Context) {
	actor, _ := SessaoDe(c)
	if err := h.revogar.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), dominio.Papel(c.Param("papel"))); err != nil {
		responderErro(c, err)
		return
	}
	c.Status(nethttp.StatusNoContent)
}

type corpoActivo struct {
	Activo *bool `json:"activo"`
}

func (h *AdministracaoHandler) definirActivo(c *gin.Context) {
	var corpo corpoActivo
	if err := c.ShouldBindJSON(&corpo); err != nil || corpo.Activo == nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	if err := h.activar.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), *corpo.Activo); err != nil {
		responderErro(c, err)
		return
	}
	c.Status(nethttp.StatusNoContent)
}

type corpoCriacao struct {
	Username string   `json:"username"`
	Nome     string   `json:"nome"`
	Email    string   `json:"email"`
	Papeis   []string `json:"papeis"`
}

func (h *AdministracaoHandler) criarUtilizador(c *gin.Context) {
	var corpo corpoCriacao
	if err := c.ShouldBindJSON(&corpo); err != nil || corpo.Username == "" {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgCriacaoInvalida)))
		return
	}
	papeis := make([]dominio.Papel, 0, len(corpo.Papeis))
	for _, p := range corpo.Papeis {
		papeis = append(papeis, dominio.Papel(p))
	}
	actor, _ := SessaoDe(c)
	out, err := h.criar.Executar(c.Request.Context(), actor.Sujeito, appident.CriacaoUtilizador{
		Username: corpo.Username, Nome: corpo.Nome, Email: corpo.Email, Papeis: papeis,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}
