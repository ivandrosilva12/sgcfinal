package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso de episódio (application/clinico).
type (
	// ServicoIniciarEpisodio inicia um episódio.
	ServicoIniciarEpisodio interface {
		Executar(ctx context.Context, actor string, dados appclinico.DadosNovoEpisodio) (appclinico.DetalheEpisodio, error)
	}
	// ServicoObterEpisodio devolve o detalhe de um episódio.
	ServicoObterEpisodio interface {
		Executar(ctx context.Context, actor string, papeis []string, id string) (appclinico.DetalheEpisodio, error)
	}
	// ServicoListarEpisodios lista os episódios de um doente.
	ServicoListarEpisodios interface {
		Executar(ctx context.Context, doenteID string, papeis []string, filtro appclinico.FiltroEpisodios) (appclinico.PaginaEpisodios, error)
	}
	// ServicoActualizarEpisodio actualiza a nota/diagnósticos.
	ServicoActualizarEpisodio interface {
		Executar(ctx context.Context, actor, id string, dados appclinico.DadosActualizarEpisodio) (appclinico.DetalheEpisodio, error)
	}
	// ServicoFecharEpisodio fecha um episódio.
	ServicoFecharEpisodio interface {
		Executar(ctx context.Context, actor, id string) (appclinico.DetalheEpisodio, error)
	}
	// ServicoCancelarEpisodio cancela um episódio.
	ServicoCancelarEpisodio interface {
		Executar(ctx context.Context, actor, id, motivo string) (appclinico.DetalheEpisodio, error)
	}
	// ServicoObterEHR devolve a projecção EHR de um doente.
	ServicoObterEHR interface {
		Executar(ctx context.Context, actor string, papeis []string, doenteID string, filtro appclinico.FiltroEpisodios) (appclinico.EHR, error)
	}
)

// EpisodiosHandler expõe os endpoints HTTP do episódio clínico e do EHR.
type EpisodiosHandler struct {
	iniciar    ServicoIniciarEpisodio
	obter      ServicoObterEpisodio
	listar     ServicoListarEpisodios
	actualizar ServicoActualizarEpisodio
	fechar     ServicoFecharEpisodio
	cancelar   ServicoCancelarEpisodio
	ehr        ServicoObterEHR
}

// NovoEpisodiosHandler constrói o handler com os casos de uso.
func NovoEpisodiosHandler(
	iniciar ServicoIniciarEpisodio,
	obter ServicoObterEpisodio,
	listar ServicoListarEpisodios,
	actualizar ServicoActualizarEpisodio,
	fechar ServicoFecharEpisodio,
	cancelar ServicoCancelarEpisodio,
	ehr ServicoObterEHR,
) *EpisodiosHandler {
	return &EpisodiosHandler{
		iniciar: iniciar, obter: obter, listar: listar, actualizar: actualizar,
		fechar: fechar, cancelar: cancelar, ehr: ehr,
	}
}

// RegistarEpisodios regista as rotas de episódio e EHR, aplicando `protecao`
// (rate limit + Auth) e o RBAC por rota.
func RegistarEpisodios(r gin.IRouter, h *EpisodiosHandler, protecao ...gin.HandlerFunc) {
	leituraClinica := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelFarmaceutico,
		dominio.PapelTecnicoLab, dominio.PapelDirector, dominio.PapelDPO, dominio.PapelAuditor)
	clinicos := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro)
	soMedico := RBAC(dominio.PapelMedico)

	gd := r.Group("/api/v1/doentes")
	gd.Use(protecao...)
	gd.POST("/:id/episodios", clinicos, h.iniciarEpisodio)
	gd.GET("/:id/episodios", leituraClinica, h.listarEpisodios)
	gd.GET("/:id/ehr", leituraClinica, h.obterEHR)

	ge := r.Group("/api/v1/episodios")
	ge.Use(protecao...)
	ge.GET("/:eid", leituraClinica, h.obterEpisodio)
	ge.PATCH("/:eid", clinicos, h.actualizarEpisodio)
	ge.POST("/:eid/fechar", soMedico, h.fecharEpisodio)
	ge.POST("/:eid/cancelar", soMedico, h.cancelarEpisodio)
}

// papeisDe converte os papéis da sessão para os literais esperados pela
// aplicação (a Camada 2 não importa o domínio Identidade — ADR-037).
func papeisDe(s dominio.Sessao) []string {
	out := make([]string, 0, len(s.Papeis))
	for _, p := range s.Papeis {
		out = append(out, string(p))
	}
	return out
}

type corpoIniciarEpisodio struct {
	Tipo            string  `json:"tipo"`
	EspecialidadeID string  `json:"especialidade_id"`
	MedicoID        string  `json:"medico_id"`
	Inicio          *string `json:"inicio"` // RFC 3339 opcional
}

func (h *EpisodiosHandler) iniciarEpisodio(c *gin.Context) {
	var corpo corpoIniciarEpisodio
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	var inicio *time.Time
	if corpo.Inicio != nil && *corpo.Inicio != "" {
		t, err := time.Parse(time.RFC3339, *corpo.Inicio)
		if err != nil {
			responderErro(c, erros.Novo(erros.CategoriaValidacao, "início inválido (formato esperado RFC 3339)"))
			return
		}
		inicio = &t
	}
	actor, _ := SessaoDe(c)
	out, err := h.iniciar.Executar(c.Request.Context(), actor.Sujeito, appclinico.DadosNovoEpisodio{
		DoenteID: c.Param("id"), Tipo: corpo.Tipo, EspecialidadeID: corpo.EspecialidadeID,
		MedicoID: corpo.MedicoID, Inicio: inicio,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *EpisodiosHandler) listarEpisodios(c *gin.Context) {
	filtro := appclinico.FiltroEpisodios{
		Estado:       c.Query("estado"),
		Limite:       inteiroQuery(c, "limite"),
		Deslocamento: inteiroQuery(c, "deslocamento"),
	}
	actor, _ := SessaoDe(c)
	out, err := h.listar.Executar(c.Request.Context(), c.Param("id"), papeisDe(actor), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *EpisodiosHandler) obterEHR(c *gin.Context) {
	actor, _ := SessaoDe(c)
	filtro := appclinico.FiltroEpisodios{
		Estado:       c.Query("estado"),
		Limite:       inteiroQuery(c, "limite"),
		Deslocamento: inteiroQuery(c, "deslocamento"),
	}
	out, err := h.ehr.Executar(c.Request.Context(), actor.Sujeito, papeisDe(actor), c.Param("id"), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *EpisodiosHandler) obterEpisodio(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.obter.Executar(c.Request.Context(), actor.Sujeito, papeisDe(actor), c.Param("eid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoActualizarEpisodio struct {
	Nota *struct {
		QueixaPrincipal string `json:"queixa_principal"`
		HistoriaDoenca  string `json:"historia_doenca"`
		ExameObjectivo  string `json:"exame_objectivo"`
		Diagnostico     string `json:"diagnostico"`
		Plano           string `json:"plano"`
	} `json:"nota"`
	DiagnosticosCID *[]struct {
		CID       string `json:"cid"`
		Principal bool   `json:"principal"`
	} `json:"diagnosticos_cid"`
}

func (h *EpisodiosHandler) actualizarEpisodio(c *gin.Context) {
	var corpo corpoActualizarEpisodio
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	var dados appclinico.DadosActualizarEpisodio
	if corpo.Nota != nil {
		dados.Nota = &appclinico.DadosNotaClinica{
			QueixaPrincipal: corpo.Nota.QueixaPrincipal, HistoriaDoenca: corpo.Nota.HistoriaDoenca,
			ExameObjectivo: corpo.Nota.ExameObjectivo, Diagnostico: corpo.Nota.Diagnostico, Plano: corpo.Nota.Plano,
		}
	}
	if corpo.DiagnosticosCID != nil {
		lista := make([]appclinico.DadosDiagnosticoCID, 0, len(*corpo.DiagnosticosCID))
		for _, d := range *corpo.DiagnosticosCID {
			lista = append(lista, appclinico.DadosDiagnosticoCID{CID: d.CID, Principal: d.Principal})
		}
		dados.DiagnosticosCID = &lista
	}
	actor, _ := SessaoDe(c)
	out, err := h.actualizar.Executar(c.Request.Context(), actor.Sujeito, c.Param("eid"), dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *EpisodiosHandler) fecharEpisodio(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.fechar.Executar(c.Request.Context(), actor.Sujeito, c.Param("eid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoCancelarEpisodio struct {
	Motivo string `json:"motivo"`
}

func (h *EpisodiosHandler) cancelarEpisodio(c *gin.Context) {
	var corpo corpoCancelarEpisodio
	// O corpo é opcional; um motivo em falta é aceitável.
	_ = c.ShouldBindJSON(&corpo)
	actor, _ := SessaoDe(c)
	out, err := h.cancelar.Executar(c.Request.Context(), actor.Sujeito, c.Param("eid"), corpo.Motivo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
