package http_test

import (
	nethttp "net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
)

func TestValidarUUIDs_ParamValido_Passa(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.ValidarUUIDs("papel"))
	r.GET("/doentes/:id", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := fazerPedido(r, "GET", "/doentes/1a2b3c4d-1111-2222-3333-444455556666")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200 com uuid válido, obtive %d (corpo: %s)", w.Code, w.Body.String())
	}
}

func TestValidarUUIDs_ParamMalformado_400(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.ValidarUUIDs("papel"))
	handlerChamado := false
	r.GET("/doentes/:id", func(c *gin.Context) {
		handlerChamado = true
		c.Status(nethttp.StatusOK)
	})

	w := fazerPedido(r, "GET", "/doentes/id-1")
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (corpo: %s)", w.Code, w.Body.String())
	}
	if handlerChamado {
		t.Fatal("o handler não deveria ter corrido com um id malformado")
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("esperava content-type application/problem+json, obtive %q", ct)
	}
	corpo := w.Body.String()
	if !strings.Contains(corpo, `"status":400`) || !strings.Contains(corpo, "inválido") {
		t.Fatalf("corpo problem+json inesperado: %s", corpo)
	}
}

func TestValidarUUIDs_ParamIsento_Passa(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.ValidarUUIDs("papel"))
	r.DELETE("/utilizadores/:id/papeis/:papel", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := fazerPedido(r, "DELETE", "/utilizadores/1a2b3c4d-1111-2222-3333-444455556666/papeis/enfermeiro")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200 com :papel isento não-uuid, obtive %d (corpo: %s)", w.Code, w.Body.String())
	}
}

func TestValidarUUIDs_ParamNaoIsentoNaMesmaRota_400(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.ValidarUUIDs("papel"))
	r.DELETE("/utilizadores/:id/papeis/:papel", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := fazerPedido(r, "DELETE", "/utilizadores/id-invalido/papeis/enfermeiro")
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400 com :id inválido mesmo com :papel isento na mesma rota, obtive %d (corpo: %s)", w.Code, w.Body.String())
	}
}

func TestValidarUUIDs_SemParams_Passa(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.ValidarUUIDs("papel"))
	r.GET("/doentes", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := fazerPedido(r, "GET", "/doentes")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200 numa rota sem params, obtive %d (corpo: %s)", w.Code, w.Body.String())
	}
}

func TestValidarUUIDs_SegundoParamMalformado_400(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.ValidarUUIDs("papel"))
	handlerChamado := false
	r.GET("/doentes/:id/episodios/:episodioId", func(c *gin.Context) {
		handlerChamado = true
		c.Status(nethttp.StatusOK)
	})

	w := fazerPedido(r, "GET", "/doentes/1a2b3c4d-1111-2222-3333-444455556666/episodios/nao-uuid")
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400 com o segundo param malformado, obtive %d (corpo: %s)", w.Code, w.Body.String())
	}
	if handlerChamado {
		t.Fatal("o handler não deveria ter corrido com o segundo id malformado")
	}
}
