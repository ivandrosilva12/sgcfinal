package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso do BC Farmácia.
type (
	ServicoRegistarMedicamento interface {
		Executar(ctx context.Context, actor string, dados appfarmacia.DadosNovoMedicamento) (appfarmacia.DetalheMedicamento, error)
	}
	ServicoActualizarMedicamento interface {
		Executar(ctx context.Context, actor, id string, dados appfarmacia.DadosActualizarMedicamento) (appfarmacia.DetalheMedicamento, error)
	}
	ServicoDefinirEstadoMedicamento interface {
		Activar(ctx context.Context, actor, id string) (appfarmacia.DetalheMedicamento, error)
		Desactivar(ctx context.Context, actor, id string) (appfarmacia.DetalheMedicamento, error)
	}
	ServicoObterMedicamento interface {
		Executar(ctx context.Context, id string) (appfarmacia.DetalheMedicamento, error)
	}
	ServicoPesquisarMedicamentos interface {
		Executar(ctx context.Context, filtro appfarmacia.FiltroMedicamentos) (appfarmacia.PaginaMedicamentos, error)
	}
	ServicoEmitirReceita interface {
		Executar(ctx context.Context, actor string, dados appfarmacia.DadosNovaReceita) (appfarmacia.DetalheReceita, error)
	}
	ServicoAnularReceita interface {
		Executar(ctx context.Context, actor, id, motivo string) (appfarmacia.DetalheReceita, error)
	}
	ServicoObterReceita interface {
		Executar(ctx context.Context, actor, id string) (appfarmacia.DetalheReceita, error)
	}
	ServicoListarReceitas interface {
		Executar(ctx context.Context, filtro appfarmacia.FiltroReceitas) (appfarmacia.PaginaReceitas, error)
	}
)

// FarmaciaHandler expõe os endpoints HTTP do BC Farmácia.
type FarmaciaHandler struct {
	registarMed   ServicoRegistarMedicamento
	actualizarMed ServicoActualizarMedicamento
	estadoMed     ServicoDefinirEstadoMedicamento
	obterMed      ServicoObterMedicamento
	pesquisarMed  ServicoPesquisarMedicamentos
	emitir        ServicoEmitirReceita
	anular        ServicoAnularReceita
	obterReceita  ServicoObterReceita
	listarReceita ServicoListarReceitas
}

// NovoFarmaciaHandler constrói o handler com os casos de uso.
func NovoFarmaciaHandler(
	registarMed ServicoRegistarMedicamento,
	actualizarMed ServicoActualizarMedicamento,
	estadoMed ServicoDefinirEstadoMedicamento,
	obterMed ServicoObterMedicamento,
	pesquisarMed ServicoPesquisarMedicamentos,
	emitir ServicoEmitirReceita,
	anular ServicoAnularReceita,
	obterReceita ServicoObterReceita,
	listarReceita ServicoListarReceitas,
) *FarmaciaHandler {
	return &FarmaciaHandler{
		registarMed: registarMed, actualizarMed: actualizarMed, estadoMed: estadoMed,
		obterMed: obterMed, pesquisarMed: pesquisarMed, emitir: emitir, anular: anular,
		obterReceita: obterReceita, listarReceita: listarReceita,
	}
}

// RegistarFarmacia regista as rotas sob /api/v1/farmacia.
func RegistarFarmacia(r gin.IRouter, h *FarmaciaHandler, protecao ...gin.HandlerFunc) {
	g := r.Group("/api/v1/farmacia")
	g.Use(protecao...)

	leitura := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelFarmaceutico,
		dominio.PapelFarmaceuticoSenior, dominio.PapelDirector, dominio.PapelDPO, dominio.PapelAuditor)
	catalogo := RBAC(dominio.PapelFarmaceutico, dominio.PapelFarmaceuticoSenior)
	soMedico := RBAC(dominio.PapelMedico)

	g.POST("/medicamentos", catalogo, h.registarMedicamento)
	g.GET("/medicamentos", leitura, h.pesquisarMedicamentos)
	g.GET("/medicamentos/:id", leitura, h.obterMedicamento)
	g.PATCH("/medicamentos/:id", catalogo, h.actualizarMedicamento)
	g.POST("/medicamentos/:id/activar", catalogo, h.activarMedicamento)
	g.POST("/medicamentos/:id/desactivar", catalogo, h.desactivarMedicamento)

	g.POST("/receitas", soMedico, h.emitirReceita)
	g.GET("/receitas", leitura, h.listarReceitas)
	g.GET("/receitas/:id", leitura, h.obterReceitaHandler)
	g.POST("/receitas/:id/anular", soMedico, h.anularReceita)
}

type corpoMedicamento struct {
	NomeComercial     string  `json:"nome_comercial"`
	NomeGenerico      string  `json:"nome_generico"`
	FormaFarmaceutica string  `json:"forma_farmaceutica"`
	Dosagem           string  `json:"dosagem"`
	ViaAdministracao  string  `json:"via_administracao"`
	Fabricante        string  `json:"fabricante"`
	RequerReceita     bool    `json:"requer_receita"`
	Psicotropico      bool    `json:"psicotropico"`
	ClasseATC         *string `json:"classe_atc"`
	StockMinimo       int     `json:"stock_minimo"`
}

func (c corpoMedicamento) paraDados() appfarmacia.DadosNovoMedicamento {
	return appfarmacia.DadosNovoMedicamento{
		NomeComercial: c.NomeComercial, NomeGenerico: c.NomeGenerico, FormaFarmaceutica: c.FormaFarmaceutica,
		Dosagem: c.Dosagem, ViaAdministracao: c.ViaAdministracao, Fabricante: c.Fabricante,
		RequerReceita: c.RequerReceita, Psicotropico: c.Psicotropico, ClasseATC: c.ClasseATC, StockMinimo: c.StockMinimo,
	}
}

func (h *FarmaciaHandler) registarMedicamento(c *gin.Context) {
	var corpo corpoMedicamento
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registarMed.Executar(c.Request.Context(), actor.Sujeito, corpo.paraDados())
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *FarmaciaHandler) actualizarMedicamento(c *gin.Context) {
	var corpo corpoMedicamento
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.actualizarMed.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), corpo.paraDados())
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) activarMedicamento(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.estadoMed.Activar(c.Request.Context(), actor.Sujeito, c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) desactivarMedicamento(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.estadoMed.Desactivar(c.Request.Context(), actor.Sujeito, c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) obterMedicamento(c *gin.Context) {
	out, err := h.obterMed.Executar(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) pesquisarMedicamentos(c *gin.Context) {
	filtro := appfarmacia.FiltroMedicamentos{
		Termo:         c.Query("termo"),
		ApenasActivos: c.Query("apenas_activos") == "true",
		Limite:        inteiroQuery(c, "limite"),
		Deslocamento:  inteiroQuery(c, "deslocamento"),
	}
	out, err := h.pesquisarMed.Executar(c.Request.Context(), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoItemReceita struct {
	MedicamentoID       string `json:"medicamento_id"`
	Posologia           string `json:"posologia"`
	DuracaoDias         *int   `json:"duracao_dias"`
	QuantidadePrescrita int    `json:"quantidade_prescrita"`
	Notas               string `json:"notas"`
}

type corpoEmitirReceita struct {
	EpisodioID           string             `json:"episodio_id"`
	DoenteID             string             `json:"doente_id"`
	Itens                []corpoItemReceita `json:"itens"`
	Notas                string             `json:"notas"`
	IgnorarAlertaAlergia bool               `json:"ignorar_alerta_alergia"`
	JustificacaoAlerta   string             `json:"justificacao_alerta"`
}

func (h *FarmaciaHandler) emitirReceita(c *gin.Context) {
	var corpo corpoEmitirReceita
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	itens := make([]appfarmacia.DadosItemReceita, 0, len(corpo.Itens))
	for _, it := range corpo.Itens {
		itens = append(itens, appfarmacia.DadosItemReceita{
			MedicamentoID: it.MedicamentoID, Posologia: it.Posologia, DuracaoDias: it.DuracaoDias,
			QuantidadePrescrita: it.QuantidadePrescrita, Notas: it.Notas,
		})
	}
	actor, _ := SessaoDe(c)
	out, err := h.emitir.Executar(c.Request.Context(), actor.Sujeito, appfarmacia.DadosNovaReceita{
		EpisodioID: corpo.EpisodioID, DoenteID: corpo.DoenteID, Itens: itens, Notas: corpo.Notas,
		IgnorarAlertaAlergia: corpo.IgnorarAlertaAlergia, JustificacaoAlerta: corpo.JustificacaoAlerta,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

type corpoAnularReceita struct {
	Motivo string `json:"motivo"`
}

func (h *FarmaciaHandler) anularReceita(c *gin.Context) {
	var corpo corpoAnularReceita
	_ = c.ShouldBindJSON(&corpo) // motivo é opcional
	actor, _ := SessaoDe(c)
	out, err := h.anular.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), corpo.Motivo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) obterReceitaHandler(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.obterReceita.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) listarReceitas(c *gin.Context) {
	filtro := appfarmacia.FiltroReceitas{
		DoenteID:     c.Query("doente_id"),
		EpisodioID:   c.Query("episodio_id"),
		Estado:       c.Query("estado"),
		Limite:       inteiroQuery(c, "limite"),
		Deslocamento: inteiroQuery(c, "deslocamento"),
	}
	out, err := h.listarReceita.Executar(c.Request.Context(), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
