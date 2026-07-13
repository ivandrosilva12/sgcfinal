package http_test

import (
	"context"
	nethttp "net/http"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes dos serviços de consentimento ---

type fakeRegistarConsent struct {
	out appclinico.DetalheConsentimento
	err error
}

func (f fakeRegistarConsent) Executar(context.Context, string, appclinico.DadosNovoConsentimento) (appclinico.DetalheConsentimento, error) {
	return f.out, f.err
}

type fakeRevogarConsent struct {
	out appclinico.DetalheConsentimento
	err error
}

func (f fakeRevogarConsent) Executar(context.Context, string, string) (appclinico.DetalheConsentimento, error) {
	return f.out, f.err
}

type fakeListarConsent struct {
	out []appclinico.ResumoConsentimento
	err error
}

func (f fakeListarConsent) Executar(context.Context, string, appclinico.FiltroConsentimentos) ([]appclinico.ResumoConsentimento, error) {
	return f.out, f.err
}

type fakeObterConsent struct {
	out appclinico.DetalheConsentimento
	err error
}

func (f fakeObterConsent) Executar(context.Context, string) (appclinico.DetalheConsentimento, error) {
	return f.out, f.err
}

func routerConsent(sessao dominio.Sessao) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoConsentimentosHandler(
		fakeRegistarConsent{out: appclinico.DetalheConsentimento{ID: "cons-1", Vigente: true}},
		fakeRevogarConsent{out: appclinico.DetalheConsentimento{ID: "cons-1"}},
		fakeListarConsent{},
		fakeObterConsent{out: appclinico.DetalheConsentimento{ID: "cons-1"}},
	)
	adhttp.RegistarConsentimentos(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestConsentimentos_Registar_Administrativo_201(t *testing.T) {
	r := routerConsent(dominio.Sessao{Sujeito: "a1", Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/consentimentos", `{"finalidade":"TRATAMENTO","concedido":true}`)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestConsentimentos_Registar_Medico_201(t *testing.T) {
	r := routerConsent(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/consentimentos", `{"finalidade":"CIRURGIA","concedido":true,"documento_url":"https://x/y"}`)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestConsentimentos_Registar_Enfermeiro_Proibido(t *testing.T) {
	r := routerConsent(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/consentimentos", `{"finalidade":"TRATAMENTO","concedido":true}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestConsentimentos_Registar_CorpoInvalido_400(t *testing.T) {
	r := routerConsent(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/consentimentos", `{`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestConsentimentos_Listar_LeituraClinica_200(t *testing.T) {
	r := routerConsent(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}})
	w := pedido(r, "GET", "/api/v1/doentes/d1/consentimentos", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestConsentimentos_Listar_ComFiltros_200(t *testing.T) {
	r := routerConsent(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelDirector}})
	w := pedido(r, "GET", "/api/v1/doentes/d1/consentimentos?finalidade=TRATAMENTO&vigentes=true", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestConsentimentos_Listar_Proibido(t *testing.T) {
	r := routerConsent(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}})
	w := pedido(r, "GET", "/api/v1/doentes/d1/consentimentos", "Bearer xyz")
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestConsentimentos_Obter_LeituraClinica_200(t *testing.T) {
	r := routerConsent(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedido(r, "GET", "/api/v1/consentimentos/cons-1", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestConsentimentos_Obter_Proibido(t *testing.T) {
	r := routerConsent(dominio.Sessao{})
	w := pedido(r, "GET", "/api/v1/consentimentos/cons-1", "Bearer xyz")
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestConsentimentos_Revogar_Medico_200(t *testing.T) {
	r := routerConsent(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/consentimentos/cons-1/revogar", ``)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestConsentimentos_Revogar_Proibido(t *testing.T) {
	r := routerConsent(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedidoCorpo(r, "POST", "/api/v1/consentimentos/cons-1/revogar", ``)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestConsentimentos_Registar_ErroConflito_409(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoConsentimentosHandler(
		fakeRegistarConsent{err: erros.Novo(erros.CategoriaConflito, "consentimento já registado")},
		fakeRevogarConsent{}, fakeListarConsent{}, fakeObterConsent{},
	)
	adhttp.RegistarConsentimentos(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}))
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/consentimentos", `{"finalidade":"TRATAMENTO","concedido":true}`)
	if w.Code != nethttp.StatusConflict {
		t.Fatalf("esperava 409, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestConsentimentos_Revogar_ErroRegraNegocio_422(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoConsentimentosHandler(
		fakeRegistarConsent{}, fakeRevogarConsent{err: erros.Novo(erros.CategoriaRegraNegocio, "consentimento já revogado")},
		fakeListarConsent{}, fakeObterConsent{},
	)
	adhttp.RegistarConsentimentos(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}}))
	w := pedidoCorpo(r, "POST", "/api/v1/consentimentos/cons-1/revogar", ``)
	if w.Code != nethttp.StatusUnprocessableEntity {
		t.Fatalf("esperava 422, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestConsentimentos_Obter_ErroNaoEncontrado_404(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoConsentimentosHandler(
		fakeRegistarConsent{}, fakeRevogarConsent{},
		fakeListarConsent{}, fakeObterConsent{err: erros.Novo(erros.CategoriaNaoEncontrado, "consentimento não encontrado")},
	)
	adhttp.RegistarConsentimentos(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}))
	w := pedido(r, "GET", "/api/v1/consentimentos/inexistente", "Bearer xyz")
	if w.Code != nethttp.StatusNotFound {
		t.Fatalf("esperava 404, obtive %d", w.Code)
	}
}
