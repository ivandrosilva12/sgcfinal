package http_test

import (
	nethttp "net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

func TestMFAObrigatoria_PapelSensivelSemMFA_403(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	sessao := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}, AutenticacaoForte: false}
	r.Use(adhttp.Auth(fakeAuth{sessao: sessao}))
	r.Use(adhttp.MFAObrigatoria())
	r.GET("/x", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := pedido(r, "GET", "/x", "Bearer xyz")
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "mfa-obrigatorio") {
		t.Fatalf("esperava type mfa-obrigatorio: %s", w.Body.String())
	}
}

func TestMFAObrigatoria_PapelSensivelComMFA_Prossegue(t *testing.T) {
	r := novoRouter()
	sessao := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}, AutenticacaoForte: true}
	r.Use(adhttp.Auth(fakeAuth{sessao: sessao}))
	r.Use(adhttp.MFAObrigatoria())
	r.GET("/x", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	if w := pedido(r, "GET", "/x", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestMFAObrigatoria_PapelComum_Prossegue(t *testing.T) {
	r := novoRouter()
	sessao := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}
	r.Use(adhttp.Auth(fakeAuth{sessao: sessao}))
	r.Use(adhttp.MFAObrigatoria())
	r.GET("/x", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	if w := pedido(r, "GET", "/x", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}
