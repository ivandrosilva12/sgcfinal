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

// --- Fakes dos serviços de procedimento cirúrgico ---

type fakeAgendar struct {
	out appclinico.DetalheProcedimento
	err error
}

func (f fakeAgendar) Executar(context.Context, string, appclinico.DadosAgendarProcedimento) (appclinico.DetalheProcedimento, error) {
	return f.out, f.err
}

type fakeIniciarProc struct {
	out appclinico.DetalheProcedimento
	err error
}

func (f fakeIniciarProc) Executar(context.Context, string, string) (appclinico.DetalheProcedimento, error) {
	return f.out, f.err
}

type fakeConcluirProc struct {
	out appclinico.DetalheProcedimento
	err error
}

func (f fakeConcluirProc) Executar(context.Context, string, string, appclinico.DadosConcluirProcedimento) (appclinico.DetalheProcedimento, error) {
	return f.out, f.err
}

type fakeCancelarProc struct {
	out appclinico.DetalheProcedimento
	err error
}

func (f fakeCancelarProc) Executar(context.Context, string, string, string) (appclinico.DetalheProcedimento, error) {
	return f.out, f.err
}

type fakeObterProc struct {
	out appclinico.DetalheProcedimento
	err error
}

func (f fakeObterProc) Executar(context.Context, string) (appclinico.DetalheProcedimento, error) {
	return f.out, f.err
}

type fakeListarProc struct {
	out []appclinico.ResumoProcedimento
	err error
}

func (f fakeListarProc) Executar(context.Context, string) ([]appclinico.ResumoProcedimento, error) {
	return f.out, f.err
}

func routerCirurgia(sessao dominio.Sessao) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoCirurgiaHandler(
		fakeAgendar{out: appclinico.DetalheProcedimento{ID: "proc-1", Estado: "AGENDADO"}},
		fakeIniciarProc{out: appclinico.DetalheProcedimento{ID: "proc-1", Estado: "EM_CURSO"}},
		fakeConcluirProc{out: appclinico.DetalheProcedimento{ID: "proc-1", Estado: "CONCLUIDO"}},
		fakeCancelarProc{out: appclinico.DetalheProcedimento{ID: "proc-1", Estado: "CANCELADO"}},
		fakeObterProc{out: appclinico.DetalheProcedimento{ID: "proc-1"}},
		fakeListarProc{},
	)
	adhttp.RegistarCirurgia(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestCirurgia_Agendar_Medico_201(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep1/procedimentos",
		`{"codigo_procedimento":"PRC001","descricao":"Sutura","cirurgiao_id":"c1","anestesia":"NENHUMA","consentimento_id":"cons-1"}`)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestCirurgia_Agendar_Enfermeiro_Proibido(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep1/procedimentos", `{"codigo_procedimento":"PRC001"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestCirurgia_Agendar_Administrativo_Proibido(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep1/procedimentos", `{"codigo_procedimento":"PRC001"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestCirurgia_Agendar_CorpoInvalido_400(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep1/procedimentos", `{`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestCirurgia_Listar_LeituraClinica_200(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}})
	w := pedido(r, "GET", "/api/v1/episodios/ep1/procedimentos", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestCirurgia_Listar_Proibido(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}})
	w := pedido(r, "GET", "/api/v1/episodios/ep1/procedimentos", "Bearer xyz")
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestCirurgia_Obter_LeituraClinica_200(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedido(r, "GET", "/api/v1/procedimentos/proc-1", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestCirurgia_Obter_Proibido(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{})
	w := pedido(r, "GET", "/api/v1/procedimentos/proc-1", "Bearer xyz")
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestCirurgia_Iniciar_Medico_200(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/procedimentos/proc-1/iniciar", ``)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestCirurgia_Iniciar_Proibido(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedidoCorpo(r, "POST", "/api/v1/procedimentos/proc-1/iniciar", ``)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestCirurgia_Concluir_SemCorpo_200(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/procedimentos/proc-1/concluir", ``)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestCirurgia_Concluir_ComCorpo_200(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/procedimentos/proc-1/concluir", `{"complicacoes":"nenhuma","observacoes":"sem intercorrências"}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestCirurgia_Concluir_Proibido(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedidoCorpo(r, "POST", "/api/v1/procedimentos/proc-1/concluir", ``)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestCirurgia_Cancelar_Medico_200(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/procedimentos/proc-1/cancelar", `{"motivo":"doente desistiu"}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestCirurgia_Cancelar_Proibido(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedidoCorpo(r, "POST", "/api/v1/procedimentos/proc-1/cancelar", `{"motivo":"doente desistiu"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestCirurgia_Agendar_ErroConflito_409(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoCirurgiaHandler(
		fakeAgendar{err: erros.Novo(erros.CategoriaConflito, "sala já ocupada")},
		fakeIniciarProc{}, fakeConcluirProc{}, fakeCancelarProc{}, fakeObterProc{}, fakeListarProc{},
	)
	adhttp.RegistarCirurgia(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}}))
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep1/procedimentos", `{"codigo_procedimento":"PRC001"}`)
	if w.Code != nethttp.StatusConflict {
		t.Fatalf("esperava 409, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestCirurgia_Iniciar_ErroRegraNegocio_422(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoCirurgiaHandler(
		fakeAgendar{}, fakeIniciarProc{err: erros.Novo(erros.CategoriaRegraNegocio, "consentimento em falta")},
		fakeConcluirProc{}, fakeCancelarProc{}, fakeObterProc{}, fakeListarProc{},
	)
	adhttp.RegistarCirurgia(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}}))
	w := pedidoCorpo(r, "POST", "/api/v1/procedimentos/proc-1/iniciar", ``)
	if w.Code != nethttp.StatusUnprocessableEntity {
		t.Fatalf("esperava 422, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestCirurgia_Obter_ErroNaoEncontrado_404(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoCirurgiaHandler(
		fakeAgendar{}, fakeIniciarProc{}, fakeConcluirProc{}, fakeCancelarProc{},
		fakeObterProc{err: erros.Novo(erros.CategoriaNaoEncontrado, "procedimento não encontrado")}, fakeListarProc{},
	)
	adhttp.RegistarCirurgia(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}))
	w := pedido(r, "GET", "/api/v1/procedimentos/inexistente", "Bearer xyz")
	if w.Code != nethttp.StatusNotFound {
		t.Fatalf("esperava 404, obtive %d", w.Code)
	}
}
