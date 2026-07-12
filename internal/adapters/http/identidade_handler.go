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

// ServicoPerfil é o caso de uso de obtenção de perfil (application/identidade).
type ServicoPerfil interface {
	Executar(ctx context.Context, s dominio.Sessao) (appident.Perfil, error)
}

// ServicoAtualizarPerfil actualiza o perfil (telefone/BI) do próprio utilizador.
type ServicoAtualizarPerfil interface {
	Executar(ctx context.Context, s dominio.Sessao, telefone, bi *string) (appident.Perfil, error)
}

// IdentidadeHandler expõe os endpoints HTTP do BC Identidade.
type IdentidadeHandler struct {
	perfil    ServicoPerfil
	atualizar ServicoAtualizarPerfil
}

// NovoIdentidadeHandler constrói o handler com o caso de uso de perfil.
func NovoIdentidadeHandler(p ServicoPerfil, atualizar ServicoAtualizarPerfil) *IdentidadeHandler {
	return &IdentidadeHandler{perfil: p, atualizar: atualizar}
}

// RegistarIdentidade regista as rotas do BC Identidade sob /api/v1/identidade,
// aplicando ao grupo os middlewares indicados (ex.: rate limit + autenticação).
func RegistarIdentidade(r gin.IRouter, h *IdentidadeHandler, middlewares ...gin.HandlerFunc) {
	grupo := r.Group("/api/v1/identidade")
	grupo.Use(middlewares...)
	grupo.GET("/perfil", h.obterPerfil)
	grupo.PATCH("/perfil", h.atualizarPerfil)
}

// obterPerfil godoc
// @Summary      Perfil do utilizador autenticado
// @Description  Devolve o perfil (com papéis) do utilizador autenticado; faz o provisionamento JIT na primeira consulta.
// @Tags         identidade
// @Produce      json
// @Success      200 {object} identidade.Perfil
// @Failure      401 {object} http.Problema
// @Failure      500 {object} http.Problema
// @Security     BearerAuth
// @Router       /api/v1/identidade/perfil [get]
func (h *IdentidadeHandler) obterPerfil(c *gin.Context) {
	sessao, ok := SessaoDe(c)
	if !ok {
		responderErro(c, erros.Novo(erros.CategoriaNaoAutorizado, i18n.T(i18n.MsgNaoAutenticado)))
		return
	}
	perfil, err := h.perfil.Executar(c.Request.Context(), sessao)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, perfil)
}

type corpoPerfil struct {
	Telefone *string `json:"telefone"`
	Bi       *string `json:"bi"`
}

func (h *IdentidadeHandler) atualizarPerfil(c *gin.Context) {
	sessao, ok := SessaoDe(c)
	if !ok {
		responderErro(c, erros.Novo(erros.CategoriaNaoAutorizado, i18n.T(i18n.MsgNaoAutenticado)))
		return
	}
	var corpo corpoPerfil
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	perfil, err := h.atualizar.Executar(c.Request.Context(), sessao, corpo.Telefone, corpo.Bi)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, perfil)
}
