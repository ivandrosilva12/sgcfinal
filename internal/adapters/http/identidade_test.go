package http_test

import (
	"context"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes ---

type fakeAuth struct {
	sessao dominio.Sessao
	err    error
}

func (f fakeAuth) Executar(context.Context, string) (dominio.Sessao, error) {
	return f.sessao, f.err
}

type fakePerfil struct {
	perfil appident.Perfil
	err    error
}

func (f fakePerfil) Executar(context.Context, dominio.Sessao) (appident.Perfil, error) {
	return f.perfil, f.err
}

type fakeLimitador struct {
	ok    bool
	retry time.Duration
	err   error
}

func (f fakeLimitador) Permitir(context.Context, string, int, time.Duration) (bool, int, time.Duration, error) {
	return f.ok, 0, f.retry, f.err
}

func pedido(r nethttp.Handler, metodo, caminho, autorizacao string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(metodo, caminho, nil)
	if autorizacao != "" {
		req.Header.Set("Authorization", autorizacao)
	}
	r.ServeHTTP(w, req)
	return w
}

// --- Auth ---

func TestAuth_TokenInvalido_401(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	r.Use(adhttp.Auth(fakeAuth{err: erros.Novo(erros.CategoriaNaoAutorizado, "token inválido")}))
	r.GET("/protegido", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := pedido(r, "GET", "/protegido", "")
	if w.Code != nethttp.StatusUnauthorized {
		t.Fatalf("esperava 401, obtive %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/problem+json") {
		t.Fatalf("esperava problem+json, obtive %q", ct)
	}
	if !strings.Contains(w.Body.String(), `"status":401`) {
		t.Fatalf("corpo RFC 7807 inesperado: %s", w.Body.String())
	}
	// instance preenchido com o request-id.
	if !strings.Contains(w.Body.String(), `"instance":`) {
		t.Fatalf("esperava instance (request-id) no corpo: %s", w.Body.String())
	}
}

func TestAuth_TokenValido_Prossegue(t *testing.T) {
	r := novoRouter()
	sessao := dominio.Sessao{Sujeito: "uuid-1", Papeis: []dominio.Papel{dominio.PapelMedico}}
	r.Use(adhttp.Auth(fakeAuth{sessao: sessao}))
	r.GET("/protegido", func(c *gin.Context) {
		s, ok := adhttp.SessaoDe(c)
		if !ok || s.Sujeito != "uuid-1" {
			c.Status(nethttp.StatusInternalServerError)
			return
		}
		c.Status(nethttp.StatusOK)
	})

	if w := pedido(r, "GET", "/protegido", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

// --- RBAC ---

func TestRBAC_SemPapel_403(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}))
	r.GET("/admin", adhttp.RBAC(dominio.PapelAdmin), func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := pedido(r, "GET", "/admin", "Bearer xyz")
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestRBAC_ComPapel_200(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}))
	r.GET("/clinico", adhttp.RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro), func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	if w := pedido(r, "GET", "/clinico", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestRBAC_SemSessao_401(t *testing.T) {
	r := novoRouter()
	r.GET("/x", adhttp.RBAC(dominio.PapelMedico), func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	if w := pedido(r, "GET", "/x", ""); w.Code != nethttp.StatusUnauthorized {
		t.Fatalf("esperava 401 (sem sessão), obtive %d", w.Code)
	}
}

// --- Rate limit ---

func TestLimiteTaxa_Excedido_429(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	r.Use(adhttp.LimiteTaxa(fakeLimitador{ok: false, retry: 30 * time.Second}, 10, time.Minute))
	r.GET("/x", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := pedido(r, "GET", "/x", "")
	if w.Code != nethttp.StatusTooManyRequests {
		t.Fatalf("esperava 429, obtive %d", w.Code)
	}
	if ra := w.Header().Get("Retry-After"); ra != "30" {
		t.Fatalf("esperava Retry-After=30, obtive %q", ra)
	}
}

func TestLimiteTaxa_Permitido_Prossegue(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.LimiteTaxa(fakeLimitador{ok: true}, 10, time.Minute))
	r.GET("/x", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := pedido(r, "GET", "/x", "")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
	if w.Header().Get("X-RateLimit-Remaining") == "" {
		t.Fatal("esperava cabeçalho X-RateLimit-Remaining")
	}
}

func TestLimiteTaxa_FailOpen(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.LimiteTaxa(fakeLimitador{err: context.DeadlineExceeded}, 10, time.Minute))
	r.GET("/x", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	if w := pedido(r, "GET", "/x", ""); w.Code != nethttp.StatusOK {
		t.Fatalf("falha do backend deve deixar passar (fail-open); obtive %d", w.Code)
	}
}

// --- Cabeçalhos de segurança ---

func TestSegurancaHTTP_Cabecalhos(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.SegurancaHTTP([]string{"*"}, false))
	r.GET("/x", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := pedido(r, "GET", "/x", "")
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("esperava X-Content-Type-Options: nosniff")
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("esperava X-Frame-Options: DENY")
	}
}

// --- Handler de perfil (registo completo) ---

func TestRegistarIdentidade_Perfil_200(t *testing.T) {
	r := novoRouter()
	sessao := dominio.Sessao{Sujeito: "uuid-1", Papeis: []dominio.Papel{dominio.PapelMedico}}
	perfil := appident.Perfil{KeycloakID: "uuid-1", Nome: "Ana Silva", Email: "ana@sgc.ao", Activo: true, Papeis: []string{"Medico"}}
	h := adhttp.NovoIdentidadeHandler(fakePerfil{perfil: perfil})

	adhttp.RegistarIdentidade(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))

	w := pedido(r, "GET", "/api/v1/identidade/perfil", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (corpo: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"keycloak_id":"uuid-1"`) {
		t.Fatalf("corpo do perfil inesperado: %s", w.Body.String())
	}
}

func TestRegistarIdentidade_Perfil_SemToken_401(t *testing.T) {
	r := novoRouter()
	h := adhttp.NovoIdentidadeHandler(fakePerfil{})
	adhttp.RegistarIdentidade(r, h, adhttp.Auth(fakeAuth{err: erros.Novo(erros.CategoriaNaoAutorizado, "sem token")}))

	if w := pedido(r, "GET", "/api/v1/identidade/perfil", ""); w.Code != nethttp.StatusUnauthorized {
		t.Fatalf("esperava 401, obtive %d", w.Code)
	}
}
