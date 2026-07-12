package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso de stock/dispensa.
type (
	ServicoRegistarFornecedor interface {
		Executar(ctx context.Context, actor string, dados appfarmacia.DadosNovoFornecedor) (appfarmacia.DetalheFornecedor, error)
	}
	ServicoListarFornecedores interface {
		Executar(ctx context.Context, filtro appfarmacia.FiltroFornecedores) (appfarmacia.PaginaFornecedores, error)
	}
	ServicoRegistarEntradaStock interface {
		Executar(ctx context.Context, actor string, dados appfarmacia.DadosEntradaStock) (appfarmacia.DetalheLote, error)
	}
	ServicoConsultarStock interface {
		Executar(ctx context.Context, medicamentoID string) (appfarmacia.StockDTO, error)
	}
	ServicoListarLotes interface {
		Executar(ctx context.Context, medicamentoID string, apenasDisponiveis bool) ([]appfarmacia.ResumoLote, error)
	}
	ServicoDispensarReceita interface {
		Executar(ctx context.Context, actor, receitaID string, dados appfarmacia.DadosDispensa) (appfarmacia.DetalheReceita, error)
	}
)

// FarmaciaStockHandler expõe os endpoints de stock e dispensa.
type FarmaciaStockHandler struct {
	registarForn ServicoRegistarFornecedor
	listarForn   ServicoListarFornecedores
	entrada      ServicoRegistarEntradaStock
	stock        ServicoConsultarStock
	lotes        ServicoListarLotes
	dispensar    ServicoDispensarReceita
}

func NovoFarmaciaStockHandler(
	registarForn ServicoRegistarFornecedor,
	listarForn ServicoListarFornecedores,
	entrada ServicoRegistarEntradaStock,
	stock ServicoConsultarStock,
	lotes ServicoListarLotes,
	dispensar ServicoDispensarReceita,
) *FarmaciaStockHandler {
	return &FarmaciaStockHandler{
		registarForn: registarForn, listarForn: listarForn, entrada: entrada,
		stock: stock, lotes: lotes, dispensar: dispensar,
	}
}

// RegistarFarmaciaStock regista as rotas de stock/dispensa no grupo /api/v1/farmacia.
func RegistarFarmaciaStock(r gin.IRouter, h *FarmaciaStockHandler, protecao ...gin.HandlerFunc) {
	g := r.Group("/api/v1/farmacia")
	g.Use(protecao...)

	leitura := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelFarmaceutico,
		dominio.PapelFarmaceuticoSenior, dominio.PapelDirector, dominio.PapelDPO, dominio.PapelAuditor)
	stockEscrita := RBAC(dominio.PapelFarmaceutico, dominio.PapelFarmaceuticoSenior)

	g.POST("/fornecedores", stockEscrita, h.registarFornecedor)
	g.GET("/fornecedores", leitura, h.listarFornecedores)
	g.POST("/lotes", stockEscrita, h.registarEntrada)
	g.GET("/medicamentos/:id/stock", leitura, h.consultarStock)
	g.GET("/medicamentos/:id/lotes", leitura, h.listarLotes)
	g.POST("/receitas/:id/dispensar", stockEscrita, h.dispensarReceita)
}

const formatoDataStock = "2006-01-02"

type corpoFornecedor struct {
	Nome     string  `json:"nome"`
	NIF      *string `json:"nif"`
	Contacto *string `json:"contacto"`
}

func (h *FarmaciaStockHandler) registarFornecedor(c *gin.Context) {
	var corpo corpoFornecedor
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registarForn.Executar(c.Request.Context(), actor.Sujeito, appfarmacia.DadosNovoFornecedor{
		Nome: corpo.Nome, NIF: corpo.NIF, Contacto: corpo.Contacto,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *FarmaciaStockHandler) listarFornecedores(c *gin.Context) {
	filtro := appfarmacia.FiltroFornecedores{
		Termo:         c.Query("termo"),
		ApenasActivos: c.Query("apenas_activos") == "true",
		Limite:        inteiroQuery(c, "limite"),
		Deslocamento:  inteiroQuery(c, "deslocamento"),
	}
	out, err := h.listarForn.Executar(c.Request.Context(), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoEntradaStock struct {
	MedicamentoID      string  `json:"medicamento_id"`
	NumeroLote         string  `json:"numero_lote"`
	Validade           string  `json:"validade"`
	Quantidade         int     `json:"quantidade"`
	PrecoUnitarioCusto string  `json:"preco_unit_custo"`
	FornecedorID       *string `json:"fornecedor_id"`
	Notas              string  `json:"notas"`
}

func (h *FarmaciaStockHandler) registarEntrada(c *gin.Context) {
	var corpo corpoEntradaStock
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	validade, err := time.Parse(formatoDataStock, corpo.Validade)
	if err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "validade inválida (formato esperado AAAA-MM-DD)"))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.entrada.Executar(c.Request.Context(), actor.Sujeito, appfarmacia.DadosEntradaStock{
		MedicamentoID: corpo.MedicamentoID, NumeroLote: corpo.NumeroLote, Validade: validade,
		Quantidade: corpo.Quantidade, PrecoUnitarioCusto: corpo.PrecoUnitarioCusto,
		FornecedorID: corpo.FornecedorID, Notas: corpo.Notas,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *FarmaciaStockHandler) consultarStock(c *gin.Context) {
	out, err := h.stock.Executar(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaStockHandler) listarLotes(c *gin.Context) {
	out, err := h.lotes.Executar(c.Request.Context(), c.Param("id"), c.Query("apenas_disponiveis") == "true")
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoItemDispensa struct {
	MedicamentoID string `json:"medicamento_id"`
	Quantidade    int    `json:"quantidade"`
}

type corpoDispensa struct {
	Itens                []corpoItemDispensa `json:"itens"`
	IgnorarAlertaAlergia bool                `json:"ignorar_alerta_alergia"`
	JustificacaoAlerta   string              `json:"justificacao_alerta"`
}

func (h *FarmaciaStockHandler) dispensarReceita(c *gin.Context) {
	var corpo corpoDispensa
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	itens := make([]appfarmacia.ItemDispensaDTO, 0, len(corpo.Itens))
	for _, it := range corpo.Itens {
		itens = append(itens, appfarmacia.ItemDispensaDTO{MedicamentoID: it.MedicamentoID, Quantidade: it.Quantidade})
	}
	actor, _ := SessaoDe(c)
	out, err := h.dispensar.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), appfarmacia.DadosDispensa{
		Itens: itens, IgnorarAlertaAlergia: corpo.IgnorarAlertaAlergia, JustificacaoAlerta: corpo.JustificacaoAlerta,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
