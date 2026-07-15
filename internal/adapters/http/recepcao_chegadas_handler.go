// Package http (adaptadores) — este ficheiro expõe o Check-in do BC Recepção (chegadas
// e fila). Handler separado do de marcações para manter os construtores enxutos.
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

// Interfaces dos casos de uso do Check-in.
type (
	// ServicoRegistarChegada faz o check-in de uma marcação.
	ServicoRegistarChegada interface {
		Executar(ctx context.Context, actor, marcacaoID string) (apprecepcao.DetalheChegada, error)
	}
	// ServicoRegistarWalkIn regista a chegada de um doente sem marcação.
	ServicoRegistarWalkIn interface {
		Executar(ctx context.Context, actor string, dados apprecepcao.DadosWalkIn) (apprecepcao.DetalheChegada, error)
	}
	// ServicoChamar chama uma chegada da fila.
	ServicoChamar interface {
		Executar(ctx context.Context, actor, chegadaID string) (apprecepcao.DetalheChegada, error)
	}
	// ServicoRegistarDesistencia regista a desistência de uma chegada.
	ServicoRegistarDesistencia interface {
		Executar(ctx context.Context, actor, chegadaID string) (apprecepcao.DetalheChegada, error)
	}
	// ServicoListarFilaChegadas devolve a fila de espera por especialidade. Nome com o
	// sufixo "Chegadas" porque já existe um ServicoListarFila em laboratorio_handler.go
	// (fila de resultados pendentes, assinatura diferente).
	ServicoListarFilaChegadas interface {
		Executar(ctx context.Context, especialidadeID string) ([]apprecepcao.ResumoChegada, error)
	}
)

// RecepcaoChegadasHandler expõe os endpoints HTTP do Check-in.
type RecepcaoChegadasHandler struct {
	registarChegada ServicoRegistarChegada
	registarWalkIn  ServicoRegistarWalkIn
	chamar          ServicoChamar
	desistencia     ServicoRegistarDesistencia
	listarFila      ServicoListarFilaChegadas
}

// NovoRecepcaoChegadasHandler constrói o handler.
func NovoRecepcaoChegadasHandler(
	registarChegada ServicoRegistarChegada, registarWalkIn ServicoRegistarWalkIn,
	chamar ServicoChamar, desistencia ServicoRegistarDesistencia, listarFila ServicoListarFilaChegadas,
) *RecepcaoChegadasHandler {
	return &RecepcaoChegadasHandler{
		registarChegada: registarChegada, registarWalkIn: registarWalkIn,
		chamar: chamar, desistencia: desistencia, listarFila: listarFila,
	}
}

// RegistarRecepcaoChegadas regista as rotas do Check-in. O check-in, o walk-in e a
// desistência são função do Administrativo (balcão); chamar o próximo é de quem vai
// atender (Enfermeiro/Médico) e também do Administrativo; a fila é visível ao pessoal
// de balcão e clínico.
func RegistarRecepcaoChegadas(r gin.IRouter, h *RecepcaoChegadasHandler, protecao ...gin.HandlerFunc) {
	soAdministrativo := RBAC(dominio.PapelAdministrativo, dominio.PapelDirector, dominio.PapelAdmin)
	chamada := RBAC(dominio.PapelEnfermeiro, dominio.PapelMedico, dominio.PapelAdministrativo)
	filaLeitura := RBAC(dominio.PapelAdministrativo, dominio.PapelEnfermeiro, dominio.PapelMedico)

	gmar := r.Group("/api/v1/marcacoes")
	gmar.Use(protecao...)
	gmar.POST("/:mid/chegada", soAdministrativo, h.registarChegadaHTTP)

	gc := r.Group("/api/v1/chegadas")
	gc.Use(protecao...)
	gc.POST("", soAdministrativo, h.registarWalkInHTTP)
	gc.POST("/:cid/chamada", chamada, h.chamarHTTP)
	gc.POST("/:cid/desistencia", soAdministrativo, h.desistenciaHTTP)

	gf := r.Group("/api/v1/recepcao")
	gf.Use(protecao...)
	gf.GET("/fila", filaLeitura, h.listarFilaHTTP)
}

func (h *RecepcaoChegadasHandler) registarChegadaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.registarChegada.Executar(c.Request.Context(), actor.Sujeito, c.Param("mid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoChegadasHandler) registarWalkInHTTP(c *gin.Context) {
	var dados apprecepcao.DadosWalkIn
	if err := c.ShouldBindJSON(&dados); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registarWalkIn.Executar(c.Request.Context(), actor.Sujeito, dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoChegadasHandler) chamarHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.chamar.Executar(c.Request.Context(), actor.Sujeito, c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoChegadasHandler) desistenciaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.desistencia.Executar(c.Request.Context(), actor.Sujeito, c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoChegadasHandler) listarFilaHTTP(c *gin.Context) {
	out, err := h.listarFila.Executar(c.Request.Context(), c.Query("especialidade"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}
