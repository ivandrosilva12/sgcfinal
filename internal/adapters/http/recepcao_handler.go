// Package http (adaptadores) contém os handlers Gin do SGC Angola. Este ficheiro
// expõe o BC Recepção. Camada 3 — Adaptadores.
package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso do BC Recepção.
type (
	// ServicoDefinirJanela define uma janela de disponibilidade de um médico.
	ServicoDefinirJanela interface {
		Executar(ctx context.Context, actor string, dados apprecepcao.DadosDefinirJanela) (apprecepcao.DetalheJanela, error)
	}
	// ServicoRemoverJanela remove uma janela de disponibilidade.
	ServicoRemoverJanela interface {
		Executar(ctx context.Context, actor, janelaID string) error
	}
	// ServicoMarcar cria uma marcação.
	ServicoMarcar interface {
		Executar(ctx context.Context, actor string, dados apprecepcao.DadosMarcar) (apprecepcao.DetalheMarcacao, error)
	}
	// ServicoRemarcar altera o intervalo de uma marcação existente.
	ServicoRemarcar interface {
		Executar(ctx context.Context, actor, marcacaoID string, dados apprecepcao.DadosRemarcar) (apprecepcao.DetalheMarcacao, error)
	}
	// ServicoCancelar cancela uma marcação.
	ServicoCancelar interface {
		Executar(ctx context.Context, actor, marcacaoID, motivo string) (apprecepcao.DetalheMarcacao, error)
	}
	// ServicoRegistarFalta regista a falta do doente a uma marcação.
	ServicoRegistarFalta interface {
		Executar(ctx context.Context, actor, marcacaoID string) (apprecepcao.DetalheMarcacao, error)
	}
	// ServicoListarAgenda devolve a agenda combinada (janelas + marcações) de um médico.
	ServicoListarAgenda interface {
		Executar(ctx context.Context, medicoID string, de, ate time.Time) (apprecepcao.Agenda, error)
	}
	// ServicoListarMarcacoesDoente lista as marcações de um doente.
	ServicoListarMarcacoesDoente interface {
		Executar(ctx context.Context, doenteID string) ([]apprecepcao.ResumoMarcacao, error)
	}
)

// RecepcaoHandler expõe os endpoints HTTP do BC Recepção.
type RecepcaoHandler struct {
	definirJanela   ServicoDefinirJanela
	removerJanela   ServicoRemoverJanela
	marcar          ServicoMarcar
	remarcar        ServicoRemarcar
	cancelar        ServicoCancelar
	registarFalta   ServicoRegistarFalta
	listarAgenda    ServicoListarAgenda
	marcacoesDoente ServicoListarMarcacoesDoente
}

// NovoRecepcaoHandler constrói o handler.
func NovoRecepcaoHandler(
	definirJanela ServicoDefinirJanela, removerJanela ServicoRemoverJanela,
	marcar ServicoMarcar, remarcar ServicoRemarcar, cancelar ServicoCancelar,
	registarFalta ServicoRegistarFalta, listarAgenda ServicoListarAgenda,
	marcacoesDoente ServicoListarMarcacoesDoente,
) *RecepcaoHandler {
	return &RecepcaoHandler{
		definirJanela: definirJanela, removerJanela: removerJanela,
		marcar: marcar, remarcar: remarcar, cancelar: cancelar,
		registarFalta: registarFalta, listarAgenda: listarAgenda,
		marcacoesDoente: marcacoesDoente,
	}
}

// RegistarRecepcao regista as rotas, aplicando `protecao` e o RBAC por rota. A gestão
// de agenda e as marcações são função do Administrativo (secretaria/recepção), com
// supervisão do Director/Admin. A leitura da agenda é aberta também ao Médico, que
// precisa de a consultar; a escrita nunca.
func RegistarRecepcao(r gin.IRouter, h *RecepcaoHandler, protecao ...gin.HandlerFunc) {
	soAdministrativo := RBAC(dominio.PapelAdministrativo, dominio.PapelDirector, dominio.PapelAdmin)
	leituraAgenda := RBAC(dominio.PapelAdministrativo, dominio.PapelDirector, dominio.PapelAdmin, dominio.PapelMedico)

	gm := r.Group("/api/v1/medicos")
	gm.Use(protecao...)
	gm.POST("/:mid/janelas", soAdministrativo, h.definirJanelaHTTP)

	gj := r.Group("/api/v1/janelas")
	gj.Use(protecao...)
	gj.DELETE("/:jid", soAdministrativo, h.removerJanelaHTTP)

	gmar := r.Group("/api/v1/marcacoes")
	gmar.Use(protecao...)
	gmar.POST("", soAdministrativo, h.marcarHTTP)
	gmar.POST("/:mid/remarcacao", soAdministrativo, h.remarcarHTTP)
	gmar.POST("/:mid/cancelamento", soAdministrativo, h.cancelarHTTP)
	gmar.POST("/:mid/falta", soAdministrativo, h.registarFaltaHTTP)

	gr := r.Group("/api/v1/recepcao")
	gr.Use(protecao...)
	gr.GET("/agenda", leituraAgenda, h.listarAgendaHTTP)

	gd := r.Group("/api/v1/doentes")
	gd.Use(protecao...)
	gd.GET("/:did/marcacoes", leituraAgenda, h.listarMarcacoesDoenteHTTP)
}

type corpoDefinirJanela struct {
	EspecialidadeID string    `json:"especialidade_id"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
}

type corpoRemarcar struct {
	Inicio time.Time `json:"inicio"`
	Fim    time.Time `json:"fim"`
}

// corpoCancelarRecepcao — nome com sufixo "Recepcao" porque `corpoCancelar` já
// existe em cirurgia_handler.go (pacote http partilhado entre BCs).
type corpoCancelarRecepcao struct {
	Motivo string `json:"motivo"`
}

func (h *RecepcaoHandler) definirJanelaHTTP(c *gin.Context) {
	var corpo corpoDefinirJanela
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.definirJanela.Executar(c.Request.Context(), actor.Sujeito, apprecepcao.DadosDefinirJanela{
		MedicoID: c.Param("mid"), EspecialidadeID: corpo.EspecialidadeID,
		Inicio: corpo.Inicio, Fim: corpo.Fim,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoHandler) removerJanelaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	if err := h.removerJanela.Executar(c.Request.Context(), actor.Sujeito, c.Param("jid")); err != nil {
		responderErro(c, err)
		return
	}
	c.Status(nethttp.StatusNoContent)
}

func (h *RecepcaoHandler) marcarHTTP(c *gin.Context) {
	var dados apprecepcao.DadosMarcar
	if err := c.ShouldBindJSON(&dados); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.marcar.Executar(c.Request.Context(), actor.Sujeito, dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoHandler) remarcarHTTP(c *gin.Context) {
	var corpo corpoRemarcar
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.remarcar.Executar(c.Request.Context(), actor.Sujeito, c.Param("mid"),
		apprecepcao.DadosRemarcar{Inicio: corpo.Inicio, Fim: corpo.Fim})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoHandler) cancelarHTTP(c *gin.Context) {
	var corpo corpoCancelarRecepcao
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.cancelar.Executar(c.Request.Context(), actor.Sujeito, c.Param("mid"), corpo.Motivo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoHandler) registarFaltaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.registarFalta.Executar(c.Request.Context(), actor.Sujeito, c.Param("mid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoHandler) listarAgendaHTTP(c *gin.Context) {
	de, err := time.Parse(time.RFC3339, c.Query("de"))
	if err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "parâmetro 'de' inválido (esperado RFC3339)"))
		return
	}
	ate, err := time.Parse(time.RFC3339, c.Query("ate"))
	if err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "parâmetro 'ate' inválido (esperado RFC3339)"))
		return
	}
	out, err := h.listarAgenda.Executar(c.Request.Context(), c.Query("medico"), de, ate)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoHandler) listarMarcacoesDoenteHTTP(c *gin.Context) {
	out, err := h.marcacoesDoente.Executar(c.Request.Context(), c.Param("did"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}
