package http_test

import (
	"context"
	nethttp "net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

type fakeRegistarForn struct {
	out appfarmacia.DetalheFornecedor
	err error
}

func (f fakeRegistarForn) Executar(_ context.Context, _ string, _ appfarmacia.DadosNovoFornecedor) (appfarmacia.DetalheFornecedor, error) {
	return f.out, f.err
}

type fakeListarForn struct {
	out appfarmacia.PaginaFornecedores
	err error
}

func (f fakeListarForn) Executar(_ context.Context, _ appfarmacia.FiltroFornecedores) (appfarmacia.PaginaFornecedores, error) {
	return f.out, f.err
}

type fakeEntradaStock struct {
	out appfarmacia.DetalheLote
	err error
}

func (f fakeEntradaStock) Executar(_ context.Context, _ string, _ appfarmacia.DadosEntradaStock) (appfarmacia.DetalheLote, error) {
	return f.out, f.err
}

type fakeConsultarStock struct {
	out appfarmacia.StockDTO
	err error
}

func (f fakeConsultarStock) Executar(_ context.Context, _ string) (appfarmacia.StockDTO, error) {
	return f.out, f.err
}

type fakeListarLotes struct {
	out []appfarmacia.ResumoLote
	err error
}

func (f fakeListarLotes) Executar(_ context.Context, _ string, _ bool) ([]appfarmacia.ResumoLote, error) {
	return f.out, f.err
}

type fakeDispensar struct {
	out appfarmacia.DetalheReceita
	err error
}

func (f fakeDispensar) Executar(_ context.Context, _, _ string, _ appfarmacia.DadosDispensa) (appfarmacia.DetalheReceita, error) {
	return f.out, f.err
}

func routerFarmaciaStock(sessao dominio.Sessao, dispensar fakeDispensar) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoFarmaciaStockHandler(
		fakeRegistarForn{out: appfarmacia.DetalheFornecedor{ID: "forn-1", Nome: "Farmédica"}},
		fakeListarForn{out: appfarmacia.PaginaFornecedores{Total: 0}},
		fakeEntradaStock{out: appfarmacia.DetalheLote{ID: "lote-1", QuantidadeActual: 100}},
		fakeConsultarStock{out: appfarmacia.StockDTO{MedicamentoID: "med-1", Disponivel: 150}},
		fakeListarLotes{out: []appfarmacia.ResumoLote{}},
		dispensar,
	)
	adhttp.RegistarFarmaciaStock(r, h, adhttp.Auth(fakeAuth{sessao: sessao}), adhttp.MFAObrigatoria())
	return r
}

const corpoEntrada = `{"medicamento_id":"med-1","numero_lote":"L001","validade":"2027-01-01","quantidade":100,"preco_unit_custo":"12.5"}`
const corpoDispensa = `{"itens":[{"medicamento_id":"med-1","quantidade":5}]}`

func TestFarmaciaStock_RegistarFornecedor_Farmaceutico(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeDispensar{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/fornecedores", `{"nome":"Farmédica"}`); w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmaciaStock_RegistarFornecedor_MedicoProibido(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeDispensar{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/fornecedores", `{"nome":"X"}`); w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestFarmaciaStock_EntradaStock_Farmaceutico(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceuticoSenior}}, fakeDispensar{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/lotes", corpoEntrada); w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmaciaStock_EntradaStock_ValidadeInvalida_400(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeDispensar{})
	corpo := `{"medicamento_id":"med-1","numero_lote":"L001","validade":"01-01-2027","quantidade":100,"preco_unit_custo":"12.5"}`
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/lotes", corpo); w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestFarmaciaStock_ConsultarStock_LeituraAmpla(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeDispensar{})
	if w := pedido(r, "GET", "/api/v1/farmacia/medicamentos/med-1/stock", "Bearer x"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestFarmaciaStock_Dispensar_Farmaceutico(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeDispensar{out: appfarmacia.DetalheReceita{ID: "rec-1", Estado: "PARCIAL"}})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas/rec-1/dispensar", corpoDispensa); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmaciaStock_Dispensar_MedicoProibido(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeDispensar{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas/rec-1/dispensar", corpoDispensa); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Médico não devia dispensar: obtive %d", w.Code)
	}
}

func TestFarmaciaStock_Dispensar_Alergia_422(t *testing.T) {
	disp := fakeDispensar{err: erros.Novo(erros.CategoriaRegraNegocio, "colide com alergia")}
	r := routerFarmaciaStock(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, disp)
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas/rec-1/dispensar", corpoDispensa); w.Code != nethttp.StatusUnprocessableEntity {
		t.Fatalf("esperava 422, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmaciaStock_ListarLotes_LeituraAmpla(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}, AutenticacaoForte: true}, fakeDispensar{})
	if w := pedido(r, "GET", "/api/v1/farmacia/medicamentos/med-1/lotes?apenas_disponiveis=true", "Bearer x"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

// ADR-042: antes desta fatia, o grupo de Farmácia-Stock não recebia a
// MFAObrigatoria, pelo que um papel sensível consultava o stock sem segundo
// factor. Usa-se o Director porque é um papel sensível que o `leitura` do handler
// admite — com um papel fora do RBAC o par de testes provaria o RBAC, não o MFA. A
// rota GET /api/v1/farmacia/medicamentos/:id/stock é leitura pura (consultarStock).
func TestFarmaciaStock_PapelSensivelSemSegundoFactor_403(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{
		Sujeito: "dir-1",
		Papeis:  []dominio.Papel{dominio.PapelDirector},
		// sem AutenticacaoForte: é este o ponto do teste
	}, fakeDispensar{})
	w := pedido(r, "GET", "/api/v1/farmacia/medicamentos/med-1/stock", "Bearer x")
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

func TestFarmaciaStock_PapelSensivelComSegundoFactor_Prossegue(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{
		Sujeito:           "dir-1",
		Papeis:            []dominio.Papel{dominio.PapelDirector},
		AutenticacaoForte: true,
	}, fakeDispensar{})
	w := pedido(r, "GET", "/api/v1/farmacia/medicamentos/med-1/stock", "Bearer x")
	if w.Code == nethttp.StatusForbidden {
		t.Errorf("com segundo factor não devia dar 403; corpo = %s", w.Body.String())
	}
}
