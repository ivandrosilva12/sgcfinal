package http_test

import (
	"context"
	nethttp "net/http"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes dos serviços de farmácia ---

type fakeRegistarMed struct {
	out appfarmacia.DetalheMedicamento
	err error
}

func (f fakeRegistarMed) Executar(_ context.Context, _ string, _ appfarmacia.DadosNovoMedicamento) (appfarmacia.DetalheMedicamento, error) {
	return f.out, f.err
}

type fakeActualizarMed struct {
	out appfarmacia.DetalheMedicamento
	err error
}

func (f fakeActualizarMed) Executar(_ context.Context, _, _ string, _ appfarmacia.DadosActualizarMedicamento) (appfarmacia.DetalheMedicamento, error) {
	return f.out, f.err
}

type fakeEstadoMed struct {
	out appfarmacia.DetalheMedicamento
	err error
}

func (f fakeEstadoMed) Activar(_ context.Context, _, _ string) (appfarmacia.DetalheMedicamento, error) {
	return f.out, f.err
}
func (f fakeEstadoMed) Desactivar(_ context.Context, _, _ string) (appfarmacia.DetalheMedicamento, error) {
	return f.out, f.err
}

type fakeObterMed struct {
	out appfarmacia.DetalheMedicamento
	err error
}

func (f fakeObterMed) Executar(_ context.Context, _ string) (appfarmacia.DetalheMedicamento, error) {
	return f.out, f.err
}

type fakePesquisarMed struct {
	out appfarmacia.PaginaMedicamentos
	err error
}

func (f fakePesquisarMed) Executar(_ context.Context, _ appfarmacia.FiltroMedicamentos) (appfarmacia.PaginaMedicamentos, error) {
	return f.out, f.err
}

type fakeEmitirReceita struct {
	out appfarmacia.DetalheReceita
	err error
}

func (f fakeEmitirReceita) Executar(_ context.Context, _ string, _ appfarmacia.DadosNovaReceita) (appfarmacia.DetalheReceita, error) {
	return f.out, f.err
}

type fakeAnularReceita struct {
	out appfarmacia.DetalheReceita
	err error
}

func (f fakeAnularReceita) Executar(_ context.Context, _, _, _ string) (appfarmacia.DetalheReceita, error) {
	return f.out, f.err
}

type fakeObterReceita struct {
	out appfarmacia.DetalheReceita
	err error
}

func (f fakeObterReceita) Executar(_ context.Context, _, _ string) (appfarmacia.DetalheReceita, error) {
	return f.out, f.err
}

type fakeListarReceitas struct {
	out appfarmacia.PaginaReceitas
	err error
}

func (f fakeListarReceitas) Executar(_ context.Context, _ appfarmacia.FiltroReceitas) (appfarmacia.PaginaReceitas, error) {
	return f.out, f.err
}

func routerFarmacia(sessao dominio.Sessao, emitir fakeEmitirReceita) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoFarmaciaHandler(
		fakeRegistarMed{out: appfarmacia.DetalheMedicamento{ID: "med-1", CodigoInterno: "MED-00001"}},
		fakeActualizarMed{out: appfarmacia.DetalheMedicamento{ID: "med-1"}},
		fakeEstadoMed{out: appfarmacia.DetalheMedicamento{ID: "med-1", Activo: false}},
		fakeObterMed{out: appfarmacia.DetalheMedicamento{ID: "med-1"}},
		fakePesquisarMed{out: appfarmacia.PaginaMedicamentos{Total: 0}},
		emitir,
		fakeAnularReceita{out: appfarmacia.DetalheReceita{ID: "rec-1", Estado: "ANULADA"}},
		fakeObterReceita{out: appfarmacia.DetalheReceita{ID: "rec-1"}},
		fakeListarReceitas{out: appfarmacia.PaginaReceitas{Total: 0}},
	)
	adhttp.RegistarFarmacia(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

const corpoMed = `{"nome_comercial":"Amoxil","nome_generico":"Amoxicilina","forma_farmaceutica":"COMPRIMIDO","dosagem":"500 mg","via_administracao":"ORAL","requer_receita":true,"stock_minimo":10}`
const corpoReceita = `{"episodio_id":"ep-1","doente_id":"d-1","itens":[{"medicamento_id":"med-1","posologia":"1 comp 8/8h","quantidade_prescrita":20}]}`

func TestFarmacia_RegistarMedicamento_FarmaceuticoPermitido(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeEmitirReceita{})
	w := pedidoCorpo(r, "POST", "/api/v1/farmacia/medicamentos", corpoMed)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_RegistarMedicamento_MedicoProibido(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/medicamentos", corpoMed); w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestFarmacia_RegistarMedicamento_CorpoInvalido(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/medicamentos", `{invalido`); w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_PesquisarMedicamentos_LeituraAmpla(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeEmitirReceita{})
	if w := pedido(r, "GET", "/api/v1/farmacia/medicamentos?termo=amox", "Bearer x"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestFarmacia_ObterMedicamento_LeituraAmpla(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}}, fakeEmitirReceita{})
	if w := pedido(r, "GET", "/api/v1/farmacia/medicamentos/med-1", "Bearer x"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_ActualizarMedicamento_FarmaceuticoSenior(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceuticoSenior}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "PATCH", "/api/v1/farmacia/medicamentos/med-1", corpoMed); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_ActualizarMedicamento_CorpoInvalido(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceuticoSenior}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "PATCH", "/api/v1/farmacia/medicamentos/med-1", `{invalido`); w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_ActivarMedicamento_Farmaceutico(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/medicamentos/med-1/activar", ``); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_Desactivar_Farmaceutico(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceuticoSenior}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/medicamentos/med-1/desactivar", ``); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_EmitirReceita_MedicoPermitido(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeEmitirReceita{out: appfarmacia.DetalheReceita{ID: "rec-1", Estado: "EMITIDA"}})
	w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas", corpoReceita)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_EmitirReceita_FarmaceuticoProibido(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas", corpoReceita); w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestFarmacia_EmitirReceita_CorpoInvalido(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas", `{invalido`); w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_EmitirReceita_Alergia_422(t *testing.T) {
	emitir := fakeEmitirReceita{err: erros.Novo(erros.CategoriaRegraNegocio, "colide com alergia grave")}
	r := routerFarmacia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, emitir)
	w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas", corpoReceita)
	if w.Code != nethttp.StatusUnprocessableEntity {
		t.Fatalf("esperava 422, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_AnularReceita_SoMedico(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas/rec-1/anular", `{"motivo":"erro"}`); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
	r2 := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r2, "POST", "/api/v1/farmacia/receitas/rec-1/anular", `{"motivo":"erro"}`); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Farmacêutico não devia anular: obtive %d", w.Code)
	}
}

func TestFarmacia_AnularReceita_CorpoVazio(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas/rec-1/anular", ``); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200 (motivo opcional), obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_ObterReceita_LeituraAmpla(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelDirector}}, fakeEmitirReceita{})
	if w := pedido(r, "GET", "/api/v1/farmacia/receitas/rec-1", "Bearer x"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_ListarReceitas_LeituraAmpla(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeEmitirReceita{})
	if w := pedido(r, "GET", "/api/v1/farmacia/receitas?doente_id=d-1", "Bearer x"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

// erroServico é o erro de aplicação usado nos testes de falha do serviço (→ 500).
var erroServico = erros.Novo(erros.CategoriaInterno, "falha inesperada")

// routerFarmaciaFalhas constrói o router com todos os serviços a falhar, para
// exercitar os ramos de erro de cada handler (responderErro → 500).
func routerFarmaciaFalhas(sessao dominio.Sessao) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoFarmaciaHandler(
		fakeRegistarMed{err: erroServico},
		fakeActualizarMed{err: erroServico},
		fakeEstadoMed{err: erroServico},
		fakeObterMed{err: erroServico},
		fakePesquisarMed{err: erroServico},
		fakeEmitirReceita{err: erroServico},
		fakeAnularReceita{err: erroServico},
		fakeObterReceita{err: erroServico},
		fakeListarReceitas{err: erroServico},
	)
	adhttp.RegistarFarmacia(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestFarmacia_PesquisarMedicamentos_ErroServico(t *testing.T) {
	r := routerFarmaciaFalhas(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedido(r, "GET", "/api/v1/farmacia/medicamentos", "Bearer x")
	if w.Code != nethttp.StatusInternalServerError {
		t.Fatalf("esperava 500, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_RegistarMedicamento_ErroServico(t *testing.T) {
	r := routerFarmaciaFalhas(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}})
	w := pedidoCorpo(r, "POST", "/api/v1/farmacia/medicamentos", corpoMed)
	if w.Code != nethttp.StatusInternalServerError {
		t.Fatalf("esperava 500, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_ActualizarMedicamento_ErroServico(t *testing.T) {
	r := routerFarmaciaFalhas(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceuticoSenior}})
	w := pedidoCorpo(r, "PATCH", "/api/v1/farmacia/medicamentos/med-1", corpoMed)
	if w.Code != nethttp.StatusInternalServerError {
		t.Fatalf("esperava 500, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_ActivarMedicamento_ErroServico(t *testing.T) {
	r := routerFarmaciaFalhas(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}})
	w := pedidoCorpo(r, "POST", "/api/v1/farmacia/medicamentos/med-1/activar", ``)
	if w.Code != nethttp.StatusInternalServerError {
		t.Fatalf("esperava 500, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_DesactivarMedicamento_ErroServico(t *testing.T) {
	r := routerFarmaciaFalhas(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceuticoSenior}})
	w := pedidoCorpo(r, "POST", "/api/v1/farmacia/medicamentos/med-1/desactivar", ``)
	if w.Code != nethttp.StatusInternalServerError {
		t.Fatalf("esperava 500, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_ObterMedicamento_ErroServico(t *testing.T) {
	r := routerFarmaciaFalhas(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedido(r, "GET", "/api/v1/farmacia/medicamentos/med-1", "Bearer x")
	if w.Code != nethttp.StatusInternalServerError {
		t.Fatalf("esperava 500, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_AnularReceita_ErroServico(t *testing.T) {
	r := routerFarmaciaFalhas(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas/rec-1/anular", `{"motivo":"erro"}`)
	if w.Code != nethttp.StatusInternalServerError {
		t.Fatalf("esperava 500, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_ObterReceita_ErroServico(t *testing.T) {
	r := routerFarmaciaFalhas(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelDirector}})
	w := pedido(r, "GET", "/api/v1/farmacia/receitas/rec-1", "Bearer x")
	if w.Code != nethttp.StatusInternalServerError {
		t.Fatalf("esperava 500, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_ListarReceitas_ErroServico(t *testing.T) {
	r := routerFarmaciaFalhas(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}})
	w := pedido(r, "GET", "/api/v1/farmacia/receitas?doente_id=d-1", "Bearer x")
	if w.Code != nethttp.StatusInternalServerError {
		t.Fatalf("esperava 500, obtive %d (%s)", w.Code, w.Body.String())
	}
}
