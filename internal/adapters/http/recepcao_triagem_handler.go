// internal/adapters/http/recepcao_triagem_handler.go
//
// Package http (adaptadores) — este ficheiro expõe a Triagem do BC Recepção. Handler
// separado dos de marcação/check-in para manter os construtores enxutos.
package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso da Triagem.
type (
	// ServicoRegistarTriagem regista a triagem de uma chegada.
	ServicoRegistarTriagem interface {
		Executar(ctx context.Context, actor, chegadaID string, dados apprecepcao.DadosTriagem) (apprecepcao.DetalheTriagem, error)
	}
	// ServicoObterTriagem devolve a triagem de uma chegada.
	ServicoObterTriagem interface {
		Executar(ctx context.Context, chegadaID string) (apprecepcao.DetalheTriagem, error)
	}
	// ServicoListarFilaClinica devolve a fila clínica por médico.
	ServicoListarFilaClinica interface {
		Executar(ctx context.Context, medicoID string) ([]apprecepcao.ResumoFilaClinica, error)
	}
)

// RecepcaoTriagemHandler expõe os endpoints HTTP da Triagem.
type RecepcaoTriagemHandler struct {
	registar    ServicoRegistarTriagem
	obter       ServicoObterTriagem
	filaClinica ServicoListarFilaClinica
}

// NovoRecepcaoTriagemHandler constrói o handler.
func NovoRecepcaoTriagemHandler(
	registar ServicoRegistarTriagem, obter ServicoObterTriagem, filaClinica ServicoListarFilaClinica,
) *RecepcaoTriagemHandler {
	return &RecepcaoTriagemHandler{registar: registar, obter: obter, filaClinica: filaClinica}
}

// RegistarRecepcaoTriagem regista as rotas da Triagem. Registar a triagem é de quem a faz
// (Enfermeiro/Médico); a leitura da triagem e da fila clínica é clínica (Médico/Enfermeiro/
// Director) — sem Administrativo/Admin, porque os sinais vitais e a prioridade derivada são
// dado clínico (minimização LPDP).
func RegistarRecepcaoTriagem(r gin.IRouter, h *RecepcaoTriagemHandler, protecao ...gin.HandlerFunc) {
	triagemEscrita := RBAC(dominio.PapelEnfermeiro, dominio.PapelMedico)
	leituraClinica := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelDirector)

	gc := r.Group("/api/v1/chegadas")
	gc.Use(protecao...)
	gc.POST("/:cid/triagem", triagemEscrita, h.registarTriagemHTTP)
	gc.GET("/:cid/triagem", leituraClinica, h.obterTriagemHTTP)

	gf := r.Group("/api/v1/recepcao")
	gf.Use(protecao...)
	gf.GET("/fila-clinica", leituraClinica, h.filaClinicaHTTP)
}

func (h *RecepcaoTriagemHandler) registarTriagemHTTP(c *gin.Context) {
	var dados apprecepcao.DadosTriagem
	if err := c.ShouldBindJSON(&dados); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registar.Executar(c.Request.Context(), actor.Sujeito, c.Param("cid"), dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoTriagemHandler) obterTriagemHTTP(c *gin.Context) {
	out, err := h.obter.Executar(c.Request.Context(), c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoTriagemHandler) filaClinicaHTTP(c *gin.Context) {
	out, err := h.filaClinica.Executar(c.Request.Context(), c.Query("medico"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}
