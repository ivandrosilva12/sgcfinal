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

// responderProblema escreve uma resposta RFC 7807 e aborta a cadeia. O instance
// é preenchido com o request-id para correlação.
func responderProblema(c *gin.Context, status int, titulo, detalhe string) {
	instancia := ""
	if v, ok := c.Get("request_id"); ok {
		if s, ok := v.(string); ok {
			instancia = s
		}
	}
	corpo, _ := json.Marshal(Problema{
		Type:     "about:blank",
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
	responderProblema(c, estadoDe(cat), tituloDe(cat), detalhe)
}

func estadoDe(cat erros.Categoria) int {
	switch cat {
	case erros.CategoriaValidacao:
		return nethttp.StatusBadRequest
	case erros.CategoriaNaoAutorizado:
		return nethttp.StatusUnauthorized
	case erros.CategoriaProibido:
		return nethttp.StatusForbidden
	case erros.CategoriaNaoEncontrado:
		return nethttp.StatusNotFound
	case erros.CategoriaConflito:
		return nethttp.StatusConflict
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
	case erros.CategoriaNaoEncontrado:
		return i18n.T(i18n.MsgRecursoNaoEncontrado)
	case erros.CategoriaConflito:
		return i18n.T(i18n.MsgConflito)
	default:
		return i18n.T(i18n.MsgErroInterno)
	}
}
