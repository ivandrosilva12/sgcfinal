package http_test

import (
	"context"
	nethttp "net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes dos serviços de episódio ---

type fakeIniciarEpisodio struct {
	out appclinico.DetalheEpisodio
	err error
}

func (f fakeIniciarEpisodio) Executar(_ context.Context, _ string, _ appclinico.DadosNovoEpisodio) (appclinico.DetalheEpisodio, error) {
	return f.out, f.err
}

type fakeObterEpisodio struct {
	out    appclinico.DetalheEpisodio
	err    error
	papeis []string
}

func (f *fakeObterEpisodio) Executar(_ context.Context, _ string, papeis []string, _ string) (appclinico.DetalheEpisodio, error) {
	f.papeis = papeis
	return f.out, f.err
}

type fakeListarEpisodios struct {
	out    appclinico.PaginaEpisodios
	err    error
	papeis []string
}

func (f *fakeListarEpisodios) Executar(_ context.Context, _ string, papeis []string, _ appclinico.FiltroEpisodios) (appclinico.PaginaEpisodios, error) {
	f.papeis = papeis
	return f.out, f.err
}

type fakeActualizarEpisodio struct {
	out appclinico.DetalheEpisodio
	err error
}

func (f fakeActualizarEpisodio) Executar(_ context.Context, _, _ string, _ appclinico.DadosActualizarEpisodio) (appclinico.DetalheEpisodio, error) {
	return f.out, f.err
}

type fakeFecharEpisodio struct {
	out appclinico.DetalheEpisodio
	err error
}

func (f fakeFecharEpisodio) Executar(_ context.Context, _, _ string) (appclinico.DetalheEpisodio, error) {
	return f.out, f.err
}

type fakeCancelarEpisodio struct {
	out appclinico.DetalheEpisodio
	err error
}

func (f fakeCancelarEpisodio) Executar(_ context.Context, _, _, _ string) (appclinico.DetalheEpisodio, error) {
	return f.out, f.err
}

type fakeObterEHR struct {
	out    appclinico.EHR
	err    error
	papeis []string
}

func (f *fakeObterEHR) Executar(_ context.Context, _ string, papeis []string, _ string, _ appclinico.FiltroEpisodios) (appclinico.EHR, error) {
	f.papeis = papeis
	return f.out, f.err
}

func routerEpisodios(sessao dominio.Sessao) *gin.Engine {
	return routerEpisodiosComFakes(sessao, &fakeObterEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1"}})
}

// routerEpisodiosComFakes constrói o router com o fake de ObterEpisodio
// injectado (para os testes que precisam de inspeccionar os papéis
// recebidos); os restantes casos de uso usam os fakes por omissão.
func routerEpisodiosComFakes(sessao dominio.Sessao, obter *fakeObterEpisodio) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoEpisodiosHandler(
		fakeIniciarEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1", Estado: "ABERTO"}},
		obter,
		&fakeListarEpisodios{out: appclinico.PaginaEpisodios{Total: 0}},
		fakeActualizarEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1"}},
		fakeFecharEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1", Estado: "FECHADO"}},
		fakeCancelarEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1", Estado: "CANCELADO"}},
		&fakeObterEHR{out: appclinico.EHR{}},
	)
	adhttp.RegistarEpisodios(r, h, adhttp.Auth(fakeAuth{sessao: sessao}), adhttp.MFAObrigatoria())
	return r
}

// routerEpisodiosComFakesEHR constrói o router com o fake de ObterEHR
// injectado (para os testes que precisam de inspeccionar os papéis
// recebidos); os restantes casos de uso usam os fakes por omissão.
func routerEpisodiosComFakesEHR(sessao dominio.Sessao, ehr *fakeObterEHR) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoEpisodiosHandler(
		fakeIniciarEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1", Estado: "ABERTO"}},
		&fakeObterEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1"}},
		&fakeListarEpisodios{out: appclinico.PaginaEpisodios{Total: 0}},
		fakeActualizarEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1"}},
		fakeFecharEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1", Estado: "FECHADO"}},
		fakeCancelarEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1", Estado: "CANCELADO"}},
		ehr,
	)
	adhttp.RegistarEpisodios(r, h, adhttp.Auth(fakeAuth{sessao: sessao}), adhttp.MFAObrigatoria())
	return r
}

func TestEpisodios_Iniciar_MedicoPermitido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/episodios", `{"tipo":"CONSULTA","especialidade_id":"e1","medico_id":"m1"}`)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Iniciar_AdministrativoProibido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/episodios", `{"tipo":"CONSULTA","especialidade_id":"e1","medico_id":"m1"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestEpisodios_Iniciar_InicioInvalido_400(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/episodios", `{"tipo":"CONSULTA","especialidade_id":"e1","medico_id":"m1","inicio":"ontem"}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestEpisodios_Iniciar_ComInicioValido_201(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/episodios", `{"tipo":"CONSULTA","especialidade_id":"e1","medico_id":"m1","inicio":"2026-07-11T10:00:00Z"}`)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Iniciar_CorpoInvalido_400(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/episodios", `{`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestEpisodios_Fechar_SoMedico(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	if w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep-1/fechar", ``); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
	r2 := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	if w := pedidoCorpo(r2, "POST", "/api/v1/episodios/ep-1/fechar", ``); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Enfermeiro não devia fechar: obtive %d", w.Code)
	}
}

func TestEpisodios_Actualizar_Clinicos(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "e1", Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedidoCorpo(r, "PATCH", "/api/v1/episodios/ep-1", `{"nota":{"queixa_principal":"Febre"}}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Actualizar_ComDiagnosticosCID_200(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	corpo := `{"nota":{"queixa_principal":"Febre","diagnostico":"Malária"},"diagnosticos_cid":[{"cid":"B54","principal":true}]}`
	w := pedidoCorpo(r, "PATCH", "/api/v1/episodios/ep-1", corpo)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Actualizar_CorpoInvalido_400(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "PATCH", "/api/v1/episodios/ep-1", `{`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestEpisodios_Actualizar_Proibido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedidoCorpo(r, "PATCH", "/api/v1/episodios/ep-1", `{}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestEpisodios_Cancelar_SoMedico(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep-1/cancelar", `{"motivo":"duplicado"}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Cancelar_SemCorpo_200(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep-1/cancelar", ``)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Cancelar_Proibido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep-1/cancelar", `{"motivo":"duplicado"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestEpisodios_Listar_LeituraClinica(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}})
	if w := pedido(r, "GET", "/api/v1/doentes/d1/episodios", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestEpisodios_Listar_ComPaginacao_200(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelTecnicoLab}})
	w := pedido(r, "GET", "/api/v1/doentes/d1/episodios?estado=ABERTO&limite=10&deslocamento=5", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Listar_Proibido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedido(r, "GET", "/api/v1/doentes/d1/episodios", "Bearer xyz")
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestEpisodios_EHR_AdministrativoProibido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	if w := pedido(r, "GET", "/api/v1/doentes/d1/ehr", "Bearer xyz"); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Administrativo não devia ler EHR: obtive %d", w.Code)
	}
}

func TestEpisodios_EHR_MedicoPermitido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	if w := pedido(r, "GET", "/api/v1/doentes/d1/ehr", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_EHR_DirectorPermitido_200(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "d1", Papeis: []dominio.Papel{dominio.PapelDirector}, AutenticacaoForte: true})
	w := pedido(r, "GET", "/api/v1/doentes/d1/ehr?estado=ABERTO&limite=5", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Obter_LeituraClinica_200(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "au1", Papeis: []dominio.Papel{dominio.PapelAuditor}, AutenticacaoForte: true})
	w := pedido(r, "GET", "/api/v1/episodios/ep-1", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Obter_Proibido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedido(r, "GET", "/api/v1/episodios/ep-1", "Bearer xyz")
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestEpisodios_Obter_PassaPapeisDaSessao(t *testing.T) {
	f := &fakeObterEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1"}}
	r := routerEpisodiosComFakes(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, f)
	w := pedido(r, "GET", "/api/v1/episodios/ep-1", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, veio %d", w.Code)
	}
	if len(f.papeis) != 1 || f.papeis[0] != "Medico" {
		t.Fatalf("papéis da sessão mal passados: %v", f.papeis)
	}
}

func TestEpisodios_EHR_PassaPapeisDaSessao(t *testing.T) {
	f := &fakeObterEHR{out: appclinico.EHR{}}
	r := routerEpisodiosComFakesEHR(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, f)
	w := pedido(r, "GET", "/api/v1/doentes/d1/ehr", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, veio %d", w.Code)
	}
	if len(f.papeis) != 1 || f.papeis[0] != "Farmaceutico" {
		t.Fatalf("papéis da sessão mal passados: %v", f.papeis)
	}
}

func TestEpisodios_Obter_ErroMapeado_404(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoEpisodiosHandler(
		fakeIniciarEpisodio{}, &fakeObterEpisodio{err: erros.Novo(erros.CategoriaNaoEncontrado, "episódio não encontrado")},
		&fakeListarEpisodios{}, fakeActualizarEpisodio{}, fakeFecharEpisodio{}, fakeCancelarEpisodio{}, &fakeObterEHR{},
	)
	adhttp.RegistarEpisodios(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}), adhttp.MFAObrigatoria())
	if w := pedido(r, "GET", "/api/v1/episodios/inexistente", "Bearer xyz"); w.Code != nethttp.StatusNotFound {
		t.Fatalf("esperava 404, obtive %d", w.Code)
	}
}

func TestEpisodios_Listar_ErroAplicacaoMapeado_500(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoEpisodiosHandler(
		fakeIniciarEpisodio{}, &fakeObterEpisodio{},
		&fakeListarEpisodios{err: erros.Novo(erros.CategoriaInterno, "falha na base de dados")},
		fakeActualizarEpisodio{}, fakeFecharEpisodio{}, fakeCancelarEpisodio{}, &fakeObterEHR{},
	)
	adhttp.RegistarEpisodios(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}), adhttp.MFAObrigatoria())
	w := pedido(r, "GET", "/api/v1/doentes/d1/episodios", "Bearer xyz")
	if w.Code != nethttp.StatusInternalServerError {
		t.Fatalf("esperava 500, obtive %d", w.Code)
	}
}

func TestEpisodios_Fechar_ErroAplicacaoMapeado(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoEpisodiosHandler(
		fakeIniciarEpisodio{}, &fakeObterEpisodio{}, &fakeListarEpisodios{}, fakeActualizarEpisodio{},
		fakeFecharEpisodio{err: erros.Novo(erros.CategoriaValidacao, "episódio já fechado")},
		fakeCancelarEpisodio{}, &fakeObterEHR{},
	)
	adhttp.RegistarEpisodios(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}), adhttp.MFAObrigatoria())
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep-1/fechar", ``)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Cancelar_ErroAplicacaoMapeado(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoEpisodiosHandler(
		fakeIniciarEpisodio{}, &fakeObterEpisodio{}, &fakeListarEpisodios{}, fakeActualizarEpisodio{},
		fakeFecharEpisodio{},
		fakeCancelarEpisodio{err: erros.Novo(erros.CategoriaValidacao, "motivo obrigatório")},
		&fakeObterEHR{},
	)
	adhttp.RegistarEpisodios(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}), adhttp.MFAObrigatoria())
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep-1/cancelar", `{}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

// ADR-042: antes desta fatia, os grupos de Episódios não recebiam a
// MFAObrigatoria, pelo que um papel sensível alcançava a leitura clínica sem
// segundo factor. Usa-se o DPO (e não o Admin, que nem está no RBAC de leitura
// clínica de episódios) porque é um papel sensível que o `leituraClinica` do
// handler admite — com um papel fora do RBAC o par de testes provaria o RBAC, não
// o MFA. A rota GET /api/v1/episodios/:eid é leitura pura (obterEpisodio).
func TestEpisodios_PapelSensivelSemSegundoFactor_403(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{
		Sujeito: "dpo-1",
		Papeis:  []dominio.Papel{dominio.PapelDPO},
		// sem AutenticacaoForte: é este o ponto do teste
	})
	w := pedido(r, "GET", "/api/v1/episodios/ep-1", "Bearer xyz")
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("código = %d, queria 403", w.Code)
	}
	// Asserir o tipo do problema, e não só o 403: sem isto, o teste não distingue
	// o 403 do MFA do 403 do RBAC, e passaria a verde pela razão errada se o RBAC
	// mudasse.
	if corpo := w.Body.String(); !strings.Contains(corpo, "mfa-obrigatorio") {
		t.Errorf("corpo = %s, queria type mfa-obrigatorio", corpo)
	}
}

func TestEpisodios_PapelSensivelComSegundoFactor_Prossegue(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{
		Sujeito:           "dpo-1",
		Papeis:            []dominio.Papel{dominio.PapelDPO},
		AutenticacaoForte: true,
	})
	w := pedido(r, "GET", "/api/v1/episodios/ep-1", "Bearer xyz")
	if w.Code == nethttp.StatusForbidden {
		t.Errorf("com segundo factor não devia dar 403; corpo = %s", w.Body.String())
	}
}

func TestEpisodios_EHR_ErroAplicacaoMapeado_404(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoEpisodiosHandler(
		fakeIniciarEpisodio{}, &fakeObterEpisodio{}, &fakeListarEpisodios{}, fakeActualizarEpisodio{},
		fakeFecharEpisodio{}, fakeCancelarEpisodio{},
		&fakeObterEHR{err: erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")},
	)
	adhttp.RegistarEpisodios(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}), adhttp.MFAObrigatoria())
	w := pedido(r, "GET", "/api/v1/doentes/inexistente/ehr", "Bearer xyz")
	if w.Code != nethttp.StatusNotFound {
		t.Fatalf("esperava 404, obtive %d", w.Code)
	}
}
