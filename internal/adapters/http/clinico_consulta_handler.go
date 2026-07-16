// internal/adapters/http/clinico_consulta_handler.go
//
// Package http (adaptadores) — este ficheiro expõe o início da consulta
// (integração Recepção→Clínico, ADR-036). Handler separado para manter os
// construtores enxutos: a rota vive no grupo das chegadas, mas o caso de uso é
// do BC Clínico.
package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

// ServicoIniciarConsulta consome uma chegada TRIADO e abre o episódio CONSULTA.
type ServicoIniciarConsulta interface {
	Executar(ctx context.Context, actor, chegadaID string) (appclinico.DetalheEpisodio, error)
}

// ClinicoConsultaHandler expõe o endpoint HTTP do início da consulta.
type ClinicoConsultaHandler struct {
	iniciar ServicoIniciarConsulta
}

// NovoClinicoConsultaHandler constrói o handler.
func NovoClinicoConsultaHandler(iniciar ServicoIniciarConsulta) *ClinicoConsultaHandler {
	return &ClinicoConsultaHandler{iniciar: iniciar}
}

// RegistarClinicoConsulta regista a rota do início da consulta. Só Médico:
// iniciar a consulta é acto do médico atribuído (a guarda de dono corre no
// domínio e no CAS); o Enfermeiro pode iniciar episódios genéricos, mas não
// consumir a fila clínica.
func RegistarClinicoConsulta(r gin.IRouter, h *ClinicoConsultaHandler, protecao ...gin.HandlerFunc) {
	soMedico := RBAC(dominio.PapelMedico)

	gc := r.Group("/api/v1/chegadas")
	gc.Use(protecao...)
	gc.POST("/:cid/iniciar-consulta", soMedico, h.iniciarConsultaHTTP)
}

// iniciarConsultaHTTP não tem corpo: tudo vem da chegada e da sessão.
func (h *ClinicoConsultaHandler) iniciarConsultaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.iniciar.Executar(c.Request.Context(), actor.Sujeito, c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}
