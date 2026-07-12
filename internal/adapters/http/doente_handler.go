package http

import (
	"context"
	nethttp "net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso do BC Clínico (application/clinico).
type (
	// ServicoRegistarDoente regista um novo doente.
	ServicoRegistarDoente interface {
		Executar(ctx context.Context, actor string, dados appclinico.DadosNovoDoente) (appclinico.DetalheDoente, error)
	}
	// ServicoObterDoente devolve o detalhe de um doente.
	ServicoObterDoente interface {
		Executar(ctx context.Context, actor, id string) (appclinico.DetalheDoente, error)
	}
	// ServicoPesquisarDoentes pesquisa doentes.
	ServicoPesquisarDoentes interface {
		Executar(ctx context.Context, filtro appclinico.FiltroDoentes) (appclinico.PaginaDoentes, error)
	}
	// ServicoActualizarDoente actualiza identificação/contactos/grupo.
	ServicoActualizarDoente interface {
		Executar(ctx context.Context, actor, id string, dados appclinico.DadosActualizarDoente) (appclinico.DetalheDoente, error)
	}
	// ServicoGerirEstadoDoente aplica transições de estado.
	ServicoGerirEstadoDoente interface {
		Desactivar(ctx context.Context, actor, id, motivo string) (appclinico.DetalheDoente, error)
		DeclararFalecido(ctx context.Context, actor, id string, data time.Time, causaCID string) (appclinico.DetalheDoente, error)
	}
	// ServicoRegistarAlergia regista uma alergia.
	ServicoRegistarAlergia interface {
		Executar(ctx context.Context, actor, doenteID string, dados appclinico.DadosAlergia) (appclinico.DetalheDoente, error)
	}
	// ServicoRegistarAntecedente regista um antecedente clínico.
	ServicoRegistarAntecedente interface {
		Executar(ctx context.Context, actor, doenteID string, dados appclinico.DadosAntecedente) (appclinico.DetalheDoente, error)
	}
)

// DoentesHandler expõe os endpoints HTTP do agregado Doente.
type DoentesHandler struct {
	registar    ServicoRegistarDoente
	obter       ServicoObterDoente
	pesquisar   ServicoPesquisarDoentes
	actualizar  ServicoActualizarDoente
	estado      ServicoGerirEstadoDoente
	alergia     ServicoRegistarAlergia
	antecedente ServicoRegistarAntecedente
}

// NovoDoentesHandler constrói o handler com os casos de uso.
func NovoDoentesHandler(
	registar ServicoRegistarDoente,
	obter ServicoObterDoente,
	pesquisar ServicoPesquisarDoentes,
	actualizar ServicoActualizarDoente,
	estado ServicoGerirEstadoDoente,
	alergia ServicoRegistarAlergia,
	antecedente ServicoRegistarAntecedente,
) *DoentesHandler {
	return &DoentesHandler{
		registar: registar, obter: obter, pesquisar: pesquisar, actualizar: actualizar,
		estado: estado, alergia: alergia, antecedente: antecedente,
	}
}

// RegistarDoentes regista as rotas sob /api/v1/doentes, aplicando `protecao` ao
// grupo (rate limit + Auth) e o RBAC por rota.
func RegistarDoentes(r gin.IRouter, h *DoentesHandler, protecao ...gin.HandlerFunc) {
	g := r.Group("/api/v1/doentes")
	g.Use(protecao...)

	leitura := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelAdministrativo,
		dominio.PapelFarmaceutico, dominio.PapelTecnicoLab, dominio.PapelDirector,
		dominio.PapelDPO, dominio.PapelAuditor)
	demografia := RBAC(dominio.PapelAdministrativo, dominio.PapelMedico, dominio.PapelEnfermeiro)
	clinicos := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro)

	g.POST("", demografia, h.registarDoente)
	g.GET("", leitura, h.pesquisarDoentes)
	g.GET("/:id", leitura, h.obterDoente)
	g.PATCH("/:id", demografia, h.actualizarDoente)
	g.POST("/:id/estado", demografia, h.gerirEstado)
	g.POST("/:id/alergias", clinicos, h.registarAlergia)
	g.POST("/:id/antecedentes", clinicos, h.registarAntecedente)
}

const formatoData = "2006-01-02"

type corpoMorada struct {
	Provincia  string  `json:"provincia"`
	Municipio  string  `json:"municipio"`
	Comuna     string  `json:"comuna"`
	Bairro     string  `json:"bairro"`
	Rua        string  `json:"rua"`
	Casa       *string `json:"casa"`
	Referencia *string `json:"referencia"`
}

func (m *corpoMorada) paraDTO() *appclinico.DadosMorada {
	if m == nil {
		return nil
	}
	return &appclinico.DadosMorada{
		Provincia: m.Provincia, Municipio: m.Municipio, Comuna: m.Comuna,
		Bairro: m.Bairro, Rua: m.Rua, Casa: m.Casa, Referencia: m.Referencia,
	}
}

type corpoRegistarDoente struct {
	NumProcesso    string       `json:"num_processo"`
	NomeCompleto   string       `json:"nome_completo"`
	DataNascimento string       `json:"data_nascimento"`
	Sexo           string       `json:"sexo"`
	BI             *string      `json:"bi"`
	NIF            *string      `json:"nif"`
	Passaporte     *string      `json:"passaporte"`
	Nacionalidade  string       `json:"nacionalidade"`
	Telefone       string       `json:"telefone"`
	Email          *string      `json:"email"`
	Morada         *corpoMorada `json:"morada"`
	GrupoSanguineo *string      `json:"grupo_sanguineo"`
}

func (h *DoentesHandler) registarDoente(c *gin.Context) {
	var corpo corpoRegistarDoente
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	nasc, err := time.Parse(formatoData, corpo.DataNascimento)
	if err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "data de nascimento inválida (formato esperado AAAA-MM-DD)"))
		return
	}
	actor, _ := SessaoDe(c)
	dados := appclinico.DadosNovoDoente{
		NumProcesso: corpo.NumProcesso,
		Identificacao: appclinico.DadosIdentificacao{
			NomeCompleto: corpo.NomeCompleto, DataNascimento: nasc, Sexo: corpo.Sexo,
			BI: corpo.BI, NIF: corpo.NIF, Passaporte: corpo.Passaporte,
		},
		Contactos:      appclinico.DadosContactos{Telefone: corpo.Telefone, Email: corpo.Email, Morada: corpo.Morada.paraDTO()},
		Nacionalidade:  corpo.Nacionalidade,
		GrupoSanguineo: corpo.GrupoSanguineo,
	}
	out, err := h.registar.Executar(c.Request.Context(), actor.Sujeito, dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *DoentesHandler) pesquisarDoentes(c *gin.Context) {
	filtro := appclinico.FiltroDoentes{
		Termo:        c.Query("termo"),
		Estado:       c.Query("estado"),
		Limite:       inteiroQuery(c, "limite"),
		Deslocamento: inteiroQuery(c, "deslocamento"),
	}
	out, err := h.pesquisar.Executar(c.Request.Context(), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *DoentesHandler) obterDoente(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.obter.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoActualizarDoente struct {
	Identificacao *struct {
		NomeCompleto   string  `json:"nome_completo"`
		DataNascimento string  `json:"data_nascimento"`
		Sexo           string  `json:"sexo"`
		BI             *string `json:"bi"`
		NIF            *string `json:"nif"`
		Passaporte     *string `json:"passaporte"`
	} `json:"identificacao"`
	Contactos *struct {
		Telefone string       `json:"telefone"`
		Email    *string      `json:"email"`
		Morada   *corpoMorada `json:"morada"`
	} `json:"contactos"`
	GrupoSanguineo *string `json:"grupo_sanguineo"`
}

func (h *DoentesHandler) actualizarDoente(c *gin.Context) {
	var corpo corpoActualizarDoente
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	var dados appclinico.DadosActualizarDoente
	if corpo.Identificacao != nil {
		nasc, err := time.Parse(formatoData, corpo.Identificacao.DataNascimento)
		if err != nil {
			responderErro(c, erros.Novo(erros.CategoriaValidacao, "data de nascimento inválida (formato esperado AAAA-MM-DD)"))
			return
		}
		dados.Identificacao = &appclinico.DadosIdentificacao{
			NomeCompleto: corpo.Identificacao.NomeCompleto, DataNascimento: nasc, Sexo: corpo.Identificacao.Sexo,
			BI: corpo.Identificacao.BI, NIF: corpo.Identificacao.NIF, Passaporte: corpo.Identificacao.Passaporte,
		}
	}
	if corpo.Contactos != nil {
		dados.Contactos = &appclinico.DadosContactos{
			Telefone: corpo.Contactos.Telefone, Email: corpo.Contactos.Email, Morada: corpo.Contactos.Morada.paraDTO(),
		}
	}
	dados.GrupoSanguineo = corpo.GrupoSanguineo

	actor, _ := SessaoDe(c)
	out, err := h.actualizar.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoEstado struct {
	Accao     string `json:"accao"`      // "desactivar" | "falecido"
	Motivo    string `json:"motivo"`     // desactivar
	DataObito string `json:"data_obito"` // falecido (AAAA-MM-DD)
	CausaCID  string `json:"causa_cid"`  // falecido
}

func (h *DoentesHandler) gerirEstado(c *gin.Context) {
	var corpo corpoEstado
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	id := c.Param("id")
	var (
		out appclinico.DetalheDoente
		err error
	)
	switch corpo.Accao {
	case "desactivar":
		out, err = h.estado.Desactivar(c.Request.Context(), actor.Sujeito, id, corpo.Motivo)
	case "falecido":
		data, perr := time.Parse(formatoData, corpo.DataObito)
		if perr != nil {
			responderErro(c, erros.Novo(erros.CategoriaValidacao, "data de óbito inválida (formato esperado AAAA-MM-DD)"))
			return
		}
		out, err = h.estado.DeclararFalecido(c.Request.Context(), actor.Sujeito, id, data, corpo.CausaCID)
	default:
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "acção de estado inválida (esperado 'desactivar' ou 'falecido')"))
		return
	}
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoAlergia struct {
	Substancia    string  `json:"substancia"`
	Severidade    string  `json:"severidade"`
	ReaccaoTipica string  `json:"reaccao_tipica"`
	ConfirmadaEm  *string `json:"confirmada_em"`
	Notas         string  `json:"notas"`
}

func (h *DoentesHandler) registarAlergia(c *gin.Context) {
	var corpo corpoAlergia
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	confirmada, err := dataOpcional(corpo.ConfirmadaEm)
	if err != nil {
		responderErro(c, err)
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.alergia.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), appclinico.DadosAlergia{
		Substancia: corpo.Substancia, Severidade: corpo.Severidade, ReaccaoTipica: corpo.ReaccaoTipica,
		ConfirmadaEm: confirmada, Notas: corpo.Notas,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoAntecedente struct {
	Tipo       string  `json:"tipo"`
	Descricao  string  `json:"descricao"`
	CID        string  `json:"cid"`
	DataInicio *string `json:"data_inicio"`
	Activo     bool    `json:"activo"`
	Notas      string  `json:"notas"`
}

func (h *DoentesHandler) registarAntecedente(c *gin.Context) {
	var corpo corpoAntecedente
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	inicio, err := dataOpcional(corpo.DataInicio)
	if err != nil {
		responderErro(c, err)
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.antecedente.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), appclinico.DadosAntecedente{
		Tipo: corpo.Tipo, Descricao: corpo.Descricao, CID: corpo.CID,
		DataInicio: inicio, Activo: corpo.Activo, Notas: corpo.Notas,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

// dataOpcional converte uma data "AAAA-MM-DD" opcional (ponteiro) num *time.Time.
func dataOpcional(v *string) (*time.Time, error) {
	if v == nil || *v == "" {
		return nil, nil
	}
	t, err := time.Parse(formatoData, *v)
	if err != nil {
		return nil, erros.Novo(erros.CategoriaValidacao, "data inválida (formato esperado AAAA-MM-DD)")
	}
	return &t, nil
}

// inteiroQuery lê um parâmetro de query como inteiro; 0 se ausente ou inválido.
func inteiroQuery(c *gin.Context, chave string) int {
	v := c.Query(chave)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}
