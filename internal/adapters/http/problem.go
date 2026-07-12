package http

import (
	"encoding/json"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Problema é o corpo de erro conforme RFC 7807 (application/problem+json).
type Problema struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// responderProblema mantém a assinatura usada pelo rate limiter (type genérico).
func responderProblema(c *gin.Context, status int, titulo, detalhe string) {
	responderProblemaTipo(c, status, "about:blank", titulo, detalhe)
}

// responderProblemaTipo escreve uma resposta RFC 7807 com um type específico.
func responderProblemaTipo(c *gin.Context, status int, tipo, titulo, detalhe string) {
	instancia := ""
	if v, ok := c.Get("request_id"); ok {
		if s, ok := v.(string); ok {
			instancia = s
		}
	}
	corpo, _ := json.Marshal(Problema{
		Type:     tipo,
		Title:    titulo,
		Status:   status,
		Detail:   detalhe,
		Instance: instancia,
	})
	c.Data(status, "application/problem+json; charset=utf-8", corpo)
	c.Abort()
}

// responderErro mapeia um erro de domínio para a resposta RFC 7807 adequada.
// Erros internos não vazam detalhes ao cliente.
func responderErro(c *gin.Context, err error) {
	cat := erros.CategoriaDe(err)
	detalhe := err.Error()
	if cat == erros.CategoriaInterno {
		detalhe = i18n.T(i18n.MsgErroInterno)
	}
	responderProblemaTipo(c, estadoDe(cat), tipoDe(cat), tituloDe(cat), detalhe)
}

func estadoDe(cat erros.Categoria) int {
	switch cat {
	case erros.CategoriaValidacao:
		return nethttp.StatusBadRequest
	case erros.CategoriaNaoAutorizado:
		return nethttp.StatusUnauthorized
	case erros.CategoriaProibido:
		return nethttp.StatusForbidden
	case erros.CategoriaMFAObrigatorio:
		return nethttp.StatusForbidden
	case erros.CategoriaNaoEncontrado:
		return nethttp.StatusNotFound
	case erros.CategoriaConflito:
		return nethttp.StatusConflict
	case erros.CategoriaRegraNegocio:
		return nethttp.StatusUnprocessableEntity
	default:
		return nethttp.StatusInternalServerError
	}
}

func tituloDe(cat erros.Categoria) string {
	switch cat {
	case erros.CategoriaValidacao:
		return i18n.T(i18n.MsgPedidoInvalido)
	case erros.CategoriaNaoAutorizado:
		return i18n.T(i18n.MsgNaoAutenticado)
	case erros.CategoriaProibido:
		return i18n.T(i18n.MsgSemPermissao)
	case erros.CategoriaMFAObrigatorio:
		return i18n.T(i18n.MsgMFAObrigatoria)
	case erros.CategoriaNaoEncontrado:
		return i18n.T(i18n.MsgRecursoNaoEncontrado)
	case erros.CategoriaConflito:
		return i18n.T(i18n.MsgConflito)
	case erros.CategoriaRegraNegocio:
		return i18n.T(i18n.MsgRegraNegocio)
	default:
		return i18n.T(i18n.MsgErroInterno)
	}
}

// tipoDe devolve o URI de tipo RFC 7807 para a categoria. Distingue o caso MFA;
// as restantes categorias usam "about:blank" (o título já as identifica).
func tipoDe(cat erros.Categoria) string {
	if cat == erros.CategoriaMFAObrigatorio {
		return "/erros/mfa-obrigatorio"
	}
	return "about:blank"
}
