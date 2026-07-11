package http_test

import (
	"context"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
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

// --- Fakes dos serviços de administração ---

type fakeListar struct {
	out []appident.ResumoUtilizador
	err error
}

func (f fakeListar) Executar(context.Context, appident.FiltroUtilizadores) ([]appident.ResumoUtilizador, error) {
	return f.out, f.err
}

type fakeObter struct {
	out appident.DetalheUtilizador
	err error
}

func (f fakeObter) Executar(context.Context, string) (appident.DetalheUtilizador, error) {
	return f.out, f.err
}

type fakePapel struct {
	ultimoActor string
	ultimoAlvo  string
	ultimoPapel dominio.Papel
	err         error
}

func (f *fakePapel) Executar(_ context.Context, actor, id string, p dominio.Papel) error {
	f.ultimoActor, f.ultimoAlvo, f.ultimoPapel = actor, id, p
	return f.err
}

type fakeActivo struct {
	ultimoActivo bool
	err          error
}

func (f *fakeActivo) Executar(_ context.Context, _, _ string, activo bool) error {
	f.ultimoActivo = activo
	return f.err
}

type fakeCriar struct {
	out appident.UtilizadorCriado
	err error
}

func (f fakeCriar) Executar(context.Context, string, appident.CriacaoUtilizador) (appident.UtilizadorCriado, error) {
	return f.out, f.err
}

func routerAdmin(sessao dominio.Sessao, atribuir *fakePapel) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoAdministracaoHandler(
		fakeListar{out: []appident.ResumoUtilizador{{ID: "u1", Nome: "Ana"}}},
		fakeObter{out: appident.DetalheUtilizador{ID: "u1", Nome: "Ana"}},
		atribuir,
		&fakePapel{},
		&fakeActivo{},
		fakeCriar{out: appident.UtilizadorCriado{ID: "novo-id", SenhaTemporaria: "senha-temp"}},
	)
	adhttp.RegistarAdministracao(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func pedidoCorpo(r nethttp.Handler, metodo, caminho, corpo string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(metodo, caminho, strings.NewReader(corpo))
	req.Header.Set("Authorization", "Bearer xyz")
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestAdmin_Listar_AdminPermitido(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedido(r, "GET", "/api/v1/identidade/utilizadores", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"id":"u1"`) {
		t.Fatalf("corpo inesperado: %s", w.Body.String())
	}
}

func TestAdmin_Listar_AuditorPermitido(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}}, &fakePapel{})
	if w := pedido(r, "GET", "/api/v1/identidade/utilizadores", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("Auditor deve poder listar; obtive %d", w.Code)
	}
}

func TestAdmin_Listar_MedicoProibido(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, &fakePapel{})
	if w := pedido(r, "GET", "/api/v1/identidade/utilizadores", "Bearer xyz"); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Medico não deve listar; obtive %d", w.Code)
	}
}

func TestAdmin_AtribuirPapel_AdminOk(t *testing.T) {
	atribuir := &fakePapel{}
	r := routerAdmin(dominio.Sessao{Sujeito: "actor-1", Papeis: []dominio.Papel{dominio.PapelAdmin}}, atribuir)
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores/u1/papeis", `{"papel":"Medico"}`)
	if w.Code != nethttp.StatusNoContent {
		t.Fatalf("esperava 204, obtive %d (%s)", w.Code, w.Body.String())
	}
	if atribuir.ultimoActor != "actor-1" || atribuir.ultimoAlvo != "u1" || atribuir.ultimoPapel != dominio.PapelMedico {
		t.Fatalf("delegação errada: %+v", atribuir)
	}
}

func TestAdmin_AtribuirPapel_AuditorProibido(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}}, &fakePapel{})
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores/u1/papeis", `{"papel":"Medico"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("Auditor não deve escrever; obtive %d", w.Code)
	}
}

func TestAdmin_AtribuirPapel_CorpoInvalido_400(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores/u1/papeis", `{}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400 para papel em falta; obtive %d", w.Code)
	}
}

func TestAdmin_DesactivarUtilizador_204(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedidoCorpo(r, "PATCH", "/api/v1/identidade/utilizadores/u1", `{"activo":false}`)
	if w.Code != nethttp.StatusNoContent {
		t.Fatalf("esperava 204, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestAdmin_CriarUtilizador_AdminOk_201(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Sujeito: "actor-1", Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores",
		`{"username":"ana.silva","nome":"Ana Silva","email":"ana@sgc.ao","papeis":["Medico"]}`)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"senha_temporaria":"senha-temp"`) {
		t.Fatalf("corpo inesperado: %s", w.Body.String())
	}
}

func TestAdmin_CriarUtilizador_MedicoProibido_403(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, &fakePapel{})
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores",
		`{"username":"x","nome":"X","email":"x@sgc.ao"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("Medico não deve criar; obtive %d", w.Code)
	}
}

func TestAdmin_CriarUtilizador_CorpoInvalido_400(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores", `{"nome":"sem username"}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400 (username em falta); obtive %d", w.Code)
	}
}
