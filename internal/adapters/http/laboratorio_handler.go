// Package http (adaptadores) contém os handlers Gin do SGC Angola. Este ficheiro
// expõe o BC Laboratório. Camada 3 — Adaptadores.
package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	dominiolab "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso do BC Laboratório.
type (
	// ServicoRegistarAnalise regista uma análise no catálogo.
	ServicoRegistarAnalise interface {
		Executar(ctx context.Context, actor string, dados applaboratorio.DadosNovaAnalise) (applaboratorio.DetalheAnalise, error)
	}
	// ServicoListarAnalises lista o catálogo.
	ServicoListarAnalises interface {
		Executar(ctx context.Context) ([]applaboratorio.ResumoAnalise, error)
	}
	// ServicoEmitirRequisicao emite uma requisição de análises.
	ServicoEmitirRequisicao interface {
		Executar(ctx context.Context, actor string, dados applaboratorio.DadosEmitirRequisicao) (applaboratorio.DetalheRequisicao, error)
	}
	// ServicoObterRequisicao devolve o detalhe de uma requisição.
	ServicoObterRequisicao interface {
		Executar(ctx context.Context, id string) (applaboratorio.DetalheRequisicao, error)
	}
	// ServicoListarRequisicoes lista as requisições de um episódio.
	ServicoListarRequisicoes interface {
		Executar(ctx context.Context, episodioID string) ([]applaboratorio.ResumoRequisicao, error)
	}
	// ServicoColherAmostra regista a colheita.
	ServicoColherAmostra interface {
		Executar(ctx context.Context, actor, resultadoID string) (applaboratorio.DetalheResultado, error)
	}
	// ServicoRecusarAmostra recusa a amostra.
	ServicoRecusarAmostra interface {
		Executar(ctx context.Context, actor, resultadoID, motivo string) (applaboratorio.DetalheResultado, error)
	}
	// ServicoSubmeterPreliminar submete o resultado preliminar.
	ServicoSubmeterPreliminar interface {
		Executar(ctx context.Context, actor, resultadoID string, dados applaboratorio.DadosSubmeterPreliminar) (applaboratorio.DetalheResultado, error)
	}
	// ServicoListarFila lista a fila de trabalho do laboratório.
	ServicoListarFila interface {
		Executar(ctx context.Context, estados []dominiolab.EstadoResultado) ([]applaboratorio.ResumoResultado, error)
	}
	// ServicoListarResultadosDoEpisodio é a leitura clínica dos resultados.
	ServicoListarResultadosDoEpisodio interface {
		Executar(ctx context.Context, episodioID string) ([]applaboratorio.ResumoResultado, error)
	}
)

// LaboratorioHandler expõe os endpoints HTTP do BC Laboratório.
type LaboratorioHandler struct {
	registarAnalise    ServicoRegistarAnalise
	listarAnalises     ServicoListarAnalises
	emitir             ServicoEmitirRequisicao
	obterRequisicao    ServicoObterRequisicao
	listarRequisicoes  ServicoListarRequisicoes
	colher             ServicoColherAmostra
	recusar            ServicoRecusarAmostra
	submeter           ServicoSubmeterPreliminar
	listarFila         ServicoListarFila
	resultadosEpisodio ServicoListarResultadosDoEpisodio
}

// NovoLaboratorioHandler constrói o handler.
func NovoLaboratorioHandler(
	registarAnalise ServicoRegistarAnalise, listarAnalises ServicoListarAnalises,
	emitir ServicoEmitirRequisicao, obterRequisicao ServicoObterRequisicao,
	listarRequisicoes ServicoListarRequisicoes, colher ServicoColherAmostra,
	recusar ServicoRecusarAmostra, submeter ServicoSubmeterPreliminar,
	listarFila ServicoListarFila, resultadosEpisodio ServicoListarResultadosDoEpisodio,
) *LaboratorioHandler {
	return &LaboratorioHandler{
		registarAnalise: registarAnalise, listarAnalises: listarAnalises,
		emitir: emitir, obterRequisicao: obterRequisicao, listarRequisicoes: listarRequisicoes,
		colher: colher, recusar: recusar, submeter: submeter,
		listarFila: listarFila, resultadosEpisodio: resultadosEpisodio,
	}
}

// RegistarLaboratorio regista as rotas, aplicando `protecao` e o RBAC por rota.
//
// A separação das rotas é o que dá corpo à regra de visibilidade do marco: a fila do
// laboratório (todos os estados, preliminares incluídos) é só do Técnico/Patologista
// e da direcção clínica — nunca do Médico, porque essa rota não filtra estados por
// desenho. Os resultados do episódio (leitura clínica, só os validados — o filtro
// vive na aplicação, ver CasoListarResultadosDoEpisodio) é que ficam abertos ao
// pessoal clínico.
func RegistarLaboratorio(r gin.IRouter, h *LaboratorioHandler, protecao ...gin.HandlerFunc) {
	leituraClinica := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelDirector,
		dominio.PapelTecnicoLab, dominio.PapelPatologista, dominio.PapelAdmin)
	catalogoEscrita := RBAC(dominio.PapelAdmin, dominio.PapelDirector)
	soMedico := RBAC(dominio.PapelMedico)
	soTecnico := RBAC(dominio.PapelTecnicoLab)
	// A fila é de quem executa o trabalho laboratorial (e de quem o dirige) — nunca
	// do Médico: devolve todos os estados, incluindo o preliminar (PROCESSADA), que a
	// regra do marco proíbe expressamente de chegar ao médico.
	filaLab := RBAC(dominio.PapelTecnicoLab, dominio.PapelPatologista, dominio.PapelDirector)

	ga := r.Group("/api/v1/analises")
	ga.Use(protecao...)
	ga.POST("", catalogoEscrita, h.registarAnaliseHTTP)
	ga.GET("", leituraClinica, h.listarAnalisesHTTP)

	ge := r.Group("/api/v1/episodios")
	ge.Use(protecao...)
	ge.POST("/:eid/requisicoes", soMedico, h.emitirRequisicaoHTTP)
	ge.GET("/:eid/requisicoes", leituraClinica, h.listarRequisicoesHTTP)
	ge.GET("/:eid/resultados", leituraClinica, h.listarResultadosDoEpisodioHTTP)

	gr := r.Group("/api/v1/requisicoes")
	gr.Use(protecao...)
	gr.GET("/:rid", leituraClinica, h.obterRequisicaoHTTP)

	gl := r.Group("/api/v1/laboratorio")
	gl.Use(protecao...)
	gl.GET("/fila", filaLab, h.listarFilaHTTP)

	gres := r.Group("/api/v1/resultados")
	gres.Use(protecao...)
	gres.POST("/:rid/colheita", soTecnico, h.colherAmostraHTTP)
	gres.POST("/:rid/recusa", soTecnico, h.recusarAmostraHTTP)
	gres.POST("/:rid/preliminar", soTecnico, h.submeterPreliminarHTTP)
}

type corpoEmitirRequisicao struct {
	DoenteID   string                      `json:"doente_id"`
	Prioridade string                      `json:"prioridade"`
	Itens      []applaboratorio.ItemPedido `json:"itens"`
}

type corpoRecusa struct {
	Motivo string `json:"motivo"`
}

type corpoPreliminar struct {
	Valor       string `json:"valor"`
	Observacoes string `json:"observacoes"`
}

func (h *LaboratorioHandler) registarAnaliseHTTP(c *gin.Context) {
	var corpo applaboratorio.DadosNovaAnalise
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registarAnalise.Executar(c.Request.Context(), actor.Sujeito, corpo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *LaboratorioHandler) listarAnalisesHTTP(c *gin.Context) {
	out, err := h.listarAnalises.Executar(c.Request.Context())
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

func (h *LaboratorioHandler) emitirRequisicaoHTTP(c *gin.Context) {
	var corpo corpoEmitirRequisicao
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.emitir.Executar(c.Request.Context(), actor.Sujeito, applaboratorio.DadosEmitirRequisicao{
		EpisodioID: c.Param("eid"), DoenteID: corpo.DoenteID,
		Prioridade: corpo.Prioridade, Itens: corpo.Itens,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *LaboratorioHandler) obterRequisicaoHTTP(c *gin.Context) {
	out, err := h.obterRequisicao.Executar(c.Request.Context(), c.Param("rid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *LaboratorioHandler) listarRequisicoesHTTP(c *gin.Context) {
	out, err := h.listarRequisicoes.Executar(c.Request.Context(), c.Param("eid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

// listarFilaHTTP aceita ?estado=PENDENTE&estado=COLHIDA; sem filtro devolve todos.
func (h *LaboratorioHandler) listarFilaHTTP(c *gin.Context) {
	var estados []dominiolab.EstadoResultado
	for _, e := range c.QueryArray("estado") {
		estados = append(estados, dominiolab.EstadoResultado(e))
	}
	out, err := h.listarFila.Executar(c.Request.Context(), estados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

// listarResultadosDoEpisodioHTTP é a leitura clínica: o caso de uso filtra os estados
// visíveis (o preliminar não aparece aqui).
func (h *LaboratorioHandler) listarResultadosDoEpisodioHTTP(c *gin.Context) {
	out, err := h.resultadosEpisodio.Executar(c.Request.Context(), c.Param("eid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

func (h *LaboratorioHandler) colherAmostraHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.colher.Executar(c.Request.Context(), actor.Sujeito, c.Param("rid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *LaboratorioHandler) recusarAmostraHTTP(c *gin.Context) {
	var corpo corpoRecusa
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.recusar.Executar(c.Request.Context(), actor.Sujeito, c.Param("rid"), corpo.Motivo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *LaboratorioHandler) submeterPreliminarHTTP(c *gin.Context) {
	var corpo corpoPreliminar
	// O corpo é obrigatório: sem valor não há resultado. Um corpo malformado tem de
	// falhar com 400 — aceitá-lo em silêncio devolveria 200 a confirmar um resultado
	// que na verdade não foi gravado (lição do Sprint 11: ver cancelarProcedimento).
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.submeter.Executar(c.Request.Context(), actor.Sujeito, c.Param("rid"),
		applaboratorio.DadosSubmeterPreliminar{Valor: corpo.Valor, Observacoes: corpo.Observacoes})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
