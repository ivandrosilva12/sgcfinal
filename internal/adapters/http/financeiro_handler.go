// Package http (adaptadores) — este ficheiro expõe o BC Financeiro. Camada 3.
package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	appfinanceiro "github.com/ivandrosilva12/sgcfinal/internal/application/financeiro"
	dominiofin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso do BC Financeiro.
type (
	// ServicoCriarFactura cria uma factura em rascunho.
	ServicoCriarFactura interface {
		Executar(ctx context.Context, actor string, d appfinanceiro.DadosNovaFactura) (appfinanceiro.DetalheFactura, error)
	}
	// ServicoAdicionarItem acrescenta uma linha.
	ServicoAdicionarItem interface {
		Executar(ctx context.Context, actor string, d appfinanceiro.DadosNovoItem) (appfinanceiro.DetalheFactura, error)
	}
	// ServicoRemoverItem retira uma linha.
	ServicoRemoverItem interface {
		Executar(ctx context.Context, actor, facturaID, itemID string) (appfinanceiro.DetalheFactura, error)
	}
	// ServicoObterFactura devolve o detalhe de uma factura.
	ServicoObterFactura interface {
		Executar(ctx context.Context, id string) (appfinanceiro.DetalheFactura, error)
	}
	// ServicoListarFacturas lista as facturas de um episódio.
	ServicoListarFacturas interface {
		Executar(ctx context.Context, episodioID string) ([]appfinanceiro.ResumoFactura, error)
	}
	// ServicoEmitirFactura emite uma factura em rascunho.
	ServicoEmitirFactura interface {
		Executar(ctx context.Context, actor, facturaID string) (appfinanceiro.DetalheFactura, error)
	}
	// ServicoVerificarCadeia verifica a integridade da cadeia de uma série.
	ServicoVerificarCadeia interface {
		Executar(ctx context.Context, serie string) (appfinanceiro.ResultadoVerificacao, error)
	}
)

// FinanceiroHandler expõe os endpoints HTTP do BC Financeiro.
type FinanceiroHandler struct {
	criar     ServicoCriarFactura
	adicionar ServicoAdicionarItem
	remover   ServicoRemoverItem
	obter     ServicoObterFactura
	listar    ServicoListarFacturas
	emitir    ServicoEmitirFactura
	verificar ServicoVerificarCadeia
}

// NovoFinanceiroHandler constrói o handler.
func NovoFinanceiroHandler(criar ServicoCriarFactura, adicionar ServicoAdicionarItem,
	remover ServicoRemoverItem, obter ServicoObterFactura, listar ServicoListarFacturas,
	emitir ServicoEmitirFactura, verificar ServicoVerificarCadeia) *FinanceiroHandler {
	return &FinanceiroHandler{criar: criar, adicionar: adicionar, remover: remover, obter: obter,
		listar: listar, emitir: emitir, verificar: verificar}
}

// RegistarFinanceiro regista as rotas, aplicando `protecao` (rate limit + Auth +
// MFAObrigatoria) e o RBAC por rota. A escrita (facturação) é do Tesoureiro; a
// leitura abre também ao Director e Auditor. Desde o ADR-040 os três papéis são
// sensíveis (ERRATA-002, revisão de 2026-07-18), pelo que a MFAObrigatoria no
// grupo é o que torna efectiva a exigência de segundo factor no Financeiro.
func RegistarFinanceiro(r gin.IRouter, h *FinanceiroHandler, protecao ...gin.HandlerFunc) {
	escrita := RBAC(dominio.PapelTesoureiro)
	leitura := RBAC(dominio.PapelTesoureiro, dominio.PapelDirector, dominio.PapelAuditor)

	g := r.Group("/api/v1/financeiro/facturas")
	g.Use(protecao...)
	g.POST("", escrita, h.criarFacturaHTTP)
	g.GET("", leitura, h.listarFacturasHTTP)
	// A rota literal /cadeia/verificacao regista-se antes de /:fid por clareza
	// documental: a árvore radix do Gin já dá prioridade a segmentos estáticos
	// sobre parâmetros, independentemente da ordem de registo, pelo que esta
	// ordem não é uma protecção — é só a leitura mais natural do ficheiro.
	g.GET("/cadeia/verificacao", leitura, h.verificarCadeiaHTTP)
	g.GET("/:fid", leitura, h.obterFacturaHTTP)
	g.POST("/:fid/itens", escrita, h.adicionarItemHTTP)
	g.DELETE("/:fid/itens/:itemID", escrita, h.removerItemHTTP)
	g.POST("/:fid/emitir", escrita, h.emitirFacturaHTTP)
}

type corpoNovaFactura struct {
	EpisodioID    string `json:"episodio_id"`
	ClienteNome   string `json:"cliente_nome"`
	ClienteNIF    string `json:"cliente_nif"`
	ClienteMorada string `json:"cliente_morada"`
}

type corpoNovoItem struct {
	Descricao             string `json:"descricao"`
	Tipo                  string `json:"tipo"`
	OperacaoID            string `json:"operacao_id"`
	Quantidade            int    `json:"quantidade"`
	PrecoUnitarioCentimos int64  `json:"preco_unitario_centimos"`
	RegimeIVA             string `json:"regime_iva"`
}

// episodioIDValido valida que o episodio_id (vindo do corpo ou da query, fora do
// alcance do middleware ValidarUUIDs que só cobre path params) é um uuid canónico.
func episodioIDValido(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil && len(s) == 36
}

func (h *FinanceiroHandler) criarFacturaHTTP(c *gin.Context) {
	var corpo corpoNovaFactura
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	if !episodioIDValido(corpo.EpisodioID) {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "episódio da factura inválido"))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.criar.Executar(c.Request.Context(), actor.Sujeito, appfinanceiro.DadosNovaFactura{
		EpisodioID: corpo.EpisodioID, ClienteNome: corpo.ClienteNome,
		ClienteNIF: corpo.ClienteNIF, ClienteMorada: corpo.ClienteMorada,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *FinanceiroHandler) listarFacturasHTTP(c *gin.Context) {
	episodioID := c.Query("episodio_id")
	if !episodioIDValido(episodioID) {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "episódio inválido ou em falta"))
		return
	}
	out, err := h.listar.Executar(c.Request.Context(), episodioID)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

func (h *FinanceiroHandler) obterFacturaHTTP(c *gin.Context) {
	out, err := h.obter.Executar(c.Request.Context(), c.Param("fid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FinanceiroHandler) adicionarItemHTTP(c *gin.Context) {
	var corpo corpoNovoItem
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.adicionar.Executar(c.Request.Context(), actor.Sujeito, appfinanceiro.DadosNovoItem{
		FacturaID: c.Param("fid"), Descricao: corpo.Descricao, Tipo: corpo.Tipo,
		OperacaoID: corpo.OperacaoID, Quantidade: corpo.Quantidade,
		PrecoUnitarioCentimos: corpo.PrecoUnitarioCentimos, RegimeIVA: corpo.RegimeIVA,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *FinanceiroHandler) removerItemHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.remover.Executar(c.Request.Context(), actor.Sujeito, c.Param("fid"), c.Param("itemID"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FinanceiroHandler) emitirFacturaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.emitir.Executar(c.Request.Context(), actor.Sujeito, c.Param("fid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FinanceiroHandler) verificarCadeiaHTTP(c *gin.Context) {
	serie := c.Query("serie")
	if serie == "" {
		serie = dominiofin.SerieDe(time.Now())
	}
	out, err := h.verificar.Executar(c.Request.Context(), serie)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
