package http

import (
	"context"
	"errors"
	"io"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso de procedimento cirúrgico.
type (
	// ServicoAgendarProcedimento agenda um procedimento.
	ServicoAgendarProcedimento interface {
		Executar(ctx context.Context, actor string, dados appclinico.DadosAgendarProcedimento) (appclinico.DetalheProcedimento, error)
	}
	// ServicoIniciarProcedimento inicia um procedimento.
	ServicoIniciarProcedimento interface {
		Executar(ctx context.Context, actor, id string) (appclinico.DetalheProcedimento, error)
	}
	// ServicoConcluirProcedimento conclui um procedimento.
	ServicoConcluirProcedimento interface {
		Executar(ctx context.Context, actor, id string, dados appclinico.DadosConcluirProcedimento) (appclinico.DetalheProcedimento, error)
	}
	// ServicoCancelarProcedimento cancela um procedimento.
	ServicoCancelarProcedimento interface {
		Executar(ctx context.Context, actor, id, motivo string) (appclinico.DetalheProcedimento, error)
	}
	// ServicoObterProcedimento devolve o detalhe de um procedimento.
	ServicoObterProcedimento interface {
		Executar(ctx context.Context, id string) (appclinico.DetalheProcedimento, error)
	}
	// ServicoListarProcedimentos lista os procedimentos de um episódio.
	ServicoListarProcedimentos interface {
		Executar(ctx context.Context, episodioID string) ([]appclinico.ResumoProcedimento, error)
	}
)

// CirurgiaHandler expõe os endpoints HTTP de procedimentos cirúrgicos.
type CirurgiaHandler struct {
	agendar  ServicoAgendarProcedimento
	iniciar  ServicoIniciarProcedimento
	concluir ServicoConcluirProcedimento
	cancelar ServicoCancelarProcedimento
	obter    ServicoObterProcedimento
	listar   ServicoListarProcedimentos
}

// NovoCirurgiaHandler constrói o handler.
func NovoCirurgiaHandler(
	agendar ServicoAgendarProcedimento, iniciar ServicoIniciarProcedimento,
	concluir ServicoConcluirProcedimento, cancelar ServicoCancelarProcedimento,
	obter ServicoObterProcedimento, listar ServicoListarProcedimentos,
) *CirurgiaHandler {
	return &CirurgiaHandler{agendar: agendar, iniciar: iniciar, concluir: concluir, cancelar: cancelar, obter: obter, listar: listar}
}

// RegistarCirurgia regista as rotas, aplicando `protecao` e o RBAC por rota.
func RegistarCirurgia(r gin.IRouter, h *CirurgiaHandler, protecao ...gin.HandlerFunc) {
	leituraClinica := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelAdministrativo,
		dominio.PapelDirector, dominio.PapelDPO, dominio.PapelAuditor)
	soMedico := RBAC(dominio.PapelMedico)

	ge := r.Group("/api/v1/episodios")
	ge.Use(protecao...)
	ge.POST("/:eid/procedimentos", soMedico, h.agendarProcedimento)
	ge.GET("/:eid/procedimentos", leituraClinica, h.listarProcedimentos)

	gp := r.Group("/api/v1/procedimentos")
	gp.Use(protecao...)
	gp.GET("/:pid", leituraClinica, h.obterProcedimento)
	gp.POST("/:pid/iniciar", soMedico, h.iniciarProcedimento)
	gp.POST("/:pid/concluir", soMedico, h.concluirProcedimento)
	gp.POST("/:pid/cancelar", soMedico, h.cancelarProcedimento)
}

type corpoAgendar struct {
	Codigo          string `json:"codigo_procedimento"`
	Descricao       string `json:"descricao"`
	Sala            string `json:"sala"`
	CirurgiaoID     string `json:"cirurgiao_id"`
	AuxiliarID      string `json:"auxiliar_id"`
	Anestesia       string `json:"anestesia"`
	AnestesistaID   string `json:"anestesista_id"`
	ConsentimentoID string `json:"consentimento_id"`
	Observacoes     string `json:"observacoes"`
}

type corpoConcluir struct {
	Complicacoes string `json:"complicacoes"`
	Observacoes  string `json:"observacoes"`
}

type corpoCancelar struct {
	Motivo string `json:"motivo"`
}

func (h *CirurgiaHandler) agendarProcedimento(c *gin.Context) {
	var corpo corpoAgendar
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.agendar.Executar(c.Request.Context(), actor.Sujeito, appclinico.DadosAgendarProcedimento{
		EpisodioID: c.Param("eid"), Codigo: corpo.Codigo, Descricao: corpo.Descricao,
		Sala: corpo.Sala, CirurgiaoID: corpo.CirurgiaoID, AuxiliarID: corpo.AuxiliarID,
		Anestesia: corpo.Anestesia, AnestesistaID: corpo.AnestesistaID,
		ConsentimentoID: corpo.ConsentimentoID, Observacoes: corpo.Observacoes,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *CirurgiaHandler) listarProcedimentos(c *gin.Context) {
	out, err := h.listar.Executar(c.Request.Context(), c.Param("eid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

func (h *CirurgiaHandler) obterProcedimento(c *gin.Context) {
	out, err := h.obter.Executar(c.Request.Context(), c.Param("pid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *CirurgiaHandler) iniciarProcedimento(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.iniciar.Executar(c.Request.Context(), actor.Sujeito, c.Param("pid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *CirurgiaHandler) concluirProcedimento(c *gin.Context) {
	var corpo corpoConcluir
	// O corpo é opcional — um pedido sem corpo (io.EOF no bind) é aceite e conclui
	// sem complicações nem observações. Mas um corpo presente e malformado tem de
	// falhar: aceitá-lo em silêncio devolveria 200 ao cliente, confirmando o registo
	// de complicações que na verdade se perderam.
	if err := c.ShouldBindJSON(&corpo); err != nil && !errors.Is(err, io.EOF) {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.concluir.Executar(c.Request.Context(), actor.Sujeito, c.Param("pid"), appclinico.DadosConcluirProcedimento{
		Complicacoes: corpo.Complicacoes, Observacoes: corpo.Observacoes,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *CirurgiaHandler) cancelarProcedimento(c *gin.Context) {
	var corpo corpoCancelar
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.cancelar.Executar(c.Request.Context(), actor.Sujeito, c.Param("pid"), corpo.Motivo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
