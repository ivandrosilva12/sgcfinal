package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appfinanceiro "github.com/ivandrosilva12/sgcfinal/internal/application/financeiro"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// duploCriarFactura devolve uma factura RASCUNHO canned, guardando o actor recebido.
type duploCriarFactura struct {
	actorRecebido string
}

func (d *duploCriarFactura) Executar(_ context.Context, actor string, dados appfinanceiro.DadosNovaFactura) (appfinanceiro.DetalheFactura, error) {
	d.actorRecebido = actor
	return appfinanceiro.DetalheFactura{
		ID: "fac-1", Estado: "RASCUNHO",
		ClienteNome: dados.ClienteNome, EpisodioID: dados.EpisodioID,
	}, nil
}

// duploAdicionarItemFin devolve o detalhe da factura já com o item somado (total
// canned: 100000 cêntimos × 2 unidades, IVA STANDARD 14% → 228000).
type duploAdicionarItemFin struct {
	actorRecebido string
}

func (d *duploAdicionarItemFin) Executar(_ context.Context, actor string, dados appfinanceiro.DadosNovoItem) (appfinanceiro.DetalheFactura, error) {
	d.actorRecebido = actor
	return appfinanceiro.DetalheFactura{
		ID: dados.FacturaID, Estado: "RASCUNHO",
		Itens: []appfinanceiro.LinhaDetalhe{{
			ID: "item-1", Descricao: dados.Descricao, Tipo: dados.Tipo, OperacaoID: dados.OperacaoID,
			Quantidade: dados.Quantidade, PrecoUnitarioCentimos: dados.PrecoUnitarioCentimos,
			RegimeIVA: dados.RegimeIVA, SubtotalCentimos: 200000, ValorIVACentimos: 28000, TotalCentimos: 228000,
		}},
		SubtotalCentimos: 200000, TotalIVACentimos: 28000, TotalCentimos: 228000, Total: "228.000,00 Kz",
	}, nil
}

// duploRemoverItemFin não é exercitado por estes testes: duplo mínimo.
type duploRemoverItemFin struct{}

func (duploRemoverItemFin) Executar(_ context.Context, _, facturaID, _ string) (appfinanceiro.DetalheFactura, error) {
	return appfinanceiro.DetalheFactura{ID: facturaID, Estado: "RASCUNHO"}, nil
}

type duploObterFactura struct{}

func (duploObterFactura) Executar(_ context.Context, id string) (appfinanceiro.DetalheFactura, error) {
	return appfinanceiro.DetalheFactura{ID: id, Estado: "RASCUNHO"}, nil
}

// duploListarFacturas não é exercitado por estes testes: duplo mínimo.
type duploListarFacturas struct{}

func (duploListarFacturas) Executar(_ context.Context, episodioID string) ([]appfinanceiro.ResumoFactura, error) {
	return []appfinanceiro.ResumoFactura{{ID: "fac-1", EpisodioID: episodioID}}, nil
}

// duploEmitirFactura devolve uma factura emitida, guardando o actor recebido;
// err força um erro de domínio.
type duploEmitirFactura struct {
	err           error
	actorRecebido string
}

func (d *duploEmitirFactura) Executar(_ context.Context, actor, facturaID string) (appfinanceiro.DetalheFactura, error) {
	d.actorRecebido = actor
	if d.err != nil {
		return appfinanceiro.DetalheFactura{}, d.err
	}
	return appfinanceiro.DetalheFactura{
		ID: facturaID, Estado: "EMITIDA",
		Numero: "FAC 2026/00000001", Serie: "2026", Sequencial: 1,
		Hash: "0000000000000000000000000000000000000000000000000000000000000000",
	}, nil
}

// duploVerificarCadeia devolve, por omissão, uma cadeia íntegra; com
// `quebrada`, devolve Integra:false e um Detalhe de diagnóstico — ambos os
// veredictos vêm no corpo com 200, nunca como erro HTTP.
type duploVerificarCadeia struct{ quebrada bool }

func (d duploVerificarCadeia) Executar(_ context.Context, serie string) (appfinanceiro.ResultadoVerificacao, error) {
	if d.quebrada {
		return appfinanceiro.ResultadoVerificacao{
			Serie: serie, TotalFacturas: 3, Integra: false,
			Detalhe: "quebra de cadeia detectada entre duas facturas consecutivas da série",
		}, nil
	}
	return appfinanceiro.ResultadoVerificacao{Serie: serie, TotalFacturas: 3, Integra: true}, nil
}

// routerFin monta o router com os duplos e uma sessão fixa. Usa o `fakeAuth` já
// existente no pacote de testes (ver `identidade_test.go`) e a `sessaoLabDe`
// genérica (ver `laboratorio_test.go`) — não redefine nenhum dos dois. O
// `verificar` é variádico e opcional (por omissão, cadeia íntegra) para não
// obrigar os chamadores já existentes a mudar.
func routerFin(t *testing.T, criar *duploCriarFactura, adicionar *duploAdicionarItemFin,
	emitir *duploEmitirFactura, sessao identidade.Sessao, verificar ...*duploVerificarCadeia) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if emitir == nil {
		emitir = &duploEmitirFactura{}
	}
	v := &duploVerificarCadeia{}
	if len(verificar) > 0 && verificar[0] != nil {
		v = verificar[0]
	}
	h := adhttp.NovoFinanceiroHandler(criar, adicionar, duploRemoverItemFin{},
		duploObterFactura{}, duploListarFacturas{}, emitir, v)
	adhttp.RegistarFinanceiro(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestFinanceiro_CriarFactura_Tesoureiro_201_UsaOSujeitoAutenticado(t *testing.T) {
	criar := &duploCriarFactura{}
	r := routerFin(t, criar, &duploAdicionarItemFin{}, nil, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	corpo, _ := json.Marshal(map[string]string{
		"episodio_id": "11111111-1111-1111-1111-111111111111", "cliente_nome": "Maria João", "cliente_nif": "", "cliente_morada": "Luanda",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	if criar.actorRecebido != "tes-1" {
		t.Fatalf("esperava o actor da sessão (tes-1), veio %q", criar.actorRecebido)
	}
}

func TestFinanceiro_AdicionarItem_Tesoureiro_201_TotalCorrecto(t *testing.T) {
	adicionar := &duploAdicionarItemFin{}
	r := routerFin(t, &duploCriarFactura{}, adicionar, nil, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	corpo, _ := json.Marshal(map[string]any{
		"descricao": "Dispensa de Paracetamol", "tipo": "DISPENSA", "operacao_id": "disp-1",
		"quantidade": 2, "preco_unitario_centimos": 100000, "regime_iva": "STANDARD",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas/fac-1/itens", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	var resp appfinanceiro.DetalheFactura
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta não é JSON válido: %v", err)
	}
	if resp.TotalCentimos != 228000 {
		t.Fatalf("esperava total 228000, veio %d", resp.TotalCentimos)
	}
}

func TestFinanceiro_ObterFactura_Director_200(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil, sessaoLabDe("dir-1", identidade.PapelDirector))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/financeiro/facturas/fac-1", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("esperava 200 para o Director, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFinanceiro_CriarFactura_Medico_Proibido(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil, sessaoLabDe("med-1", identidade.PapelMedico))

	corpo, _ := json.Marshal(map[string]string{"episodio_id": "ep-1", "cliente_nome": "Maria João"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Fatalf("esperava 403 para o Medico, veio %d", w.Code)
	}
}

func TestFinanceiro_CriarFactura_CorpoMalformado_400(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

// episodio_id não é path param: o middleware ValidarUUIDs (que só cobre
// c.Params) não o alcança. Sem a validação em episodioIDValido, um episodio_id
// ausente ou malformado batia no cast ::uuid do Postgres e voltava 500 em vez
// de 400.

func TestFinanceiro_ListarFacturas_SemEpisodioID_400(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/financeiro/facturas", nil)
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("esperava 400 sem episodio_id, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFinanceiro_ListarFacturas_EpisodioValido_200(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/financeiro/facturas?episodio_id=11111111-1111-1111-1111-111111111111", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("esperava 200 com episodio_id válido, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFinanceiro_CriarFactura_EpisodioInvalido_400(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	corpo, _ := json.Marshal(map[string]string{
		"episodio_id": "nao-uuid", "cliente_nome": "Maria João", "cliente_nif": "", "cliente_morada": "Luanda",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("esperava 400 com episodio_id inválido, veio %d (%s)", w.Code, w.Body.String())
	}
}

const facturaIDTeste = "22222222-2222-2222-2222-222222222222"

func TestFinanceiro_Emitir_Tesoureiro_200(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil,
		sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas/"+facturaIDTeste+"/emitir", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("esperava 200, veio %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "FAC 2026/00000001") {
		t.Errorf("resposta devia trazer o número legal: %s", w.Body.String())
	}
}

func TestFinanceiro_Emitir_Medico_Proibido(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil,
		sessaoLabDe("med-1", identidade.PapelMedico))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas/"+facturaIDTeste+"/emitir", nil)
	r.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Fatalf("esperava 403, veio %d", w.Code)
	}
}

func TestFinanceiro_Emitir_SemLinhas_422(t *testing.T) {
	emitir := &duploEmitirFactura{
		err: erros.Novo(erros.CategoriaRegraNegocio, "não é possível emitir uma factura sem linhas"),
	}
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, emitir,
		sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas/"+facturaIDTeste+"/emitir", nil)
	r.ServeHTTP(w, req)

	if w.Code != 422 {
		t.Fatalf("esperava 422, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFinanceiro_Emitir_JaEmitida_409(t *testing.T) {
	emitir := &duploEmitirFactura{
		err: erros.Novo(erros.CategoriaConflito, "só é possível emitir uma factura em rascunho"),
	}
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, emitir,
		sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas/"+facturaIDTeste+"/emitir", nil)
	r.ServeHTTP(w, req)

	if w.Code != 409 {
		t.Fatalf("esperava 409, veio %d", w.Code)
	}
}

// A emissão é o acto fiscalmente relevante e auditado: o actor gravado no
// audit log tem de vir sempre da sessão autenticada, nunca de um valor que o
// chamador tente injectar no corpo do pedido.
func TestFinanceiro_Emitir_ActorVemDaSessaoNaoDoCorpo(t *testing.T) {
	emitir := &duploEmitirFactura{}
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, emitir,
		sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	corpo, _ := json.Marshal(map[string]string{"actor": "outro-utilizador"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas/"+facturaIDTeste+"/emitir", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("esperava 200, veio %d (%s)", w.Code, w.Body.String())
	}
	if emitir.actorRecebido != "tes-1" {
		t.Fatalf("esperava o actor da sessão (tes-1), veio %q", emitir.actorRecebido)
	}
}

func TestFinanceiro_VerificarCadeia_Auditor_200(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil,
		sessaoLabDe("aud-1", identidade.PapelAuditor))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/financeiro/facturas/cadeia/verificacao?serie=2026", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("esperava 200, veio %d (%s)", w.Code, w.Body.String())
	}
}

// O comportamento mais importante desta rota: uma cadeia quebrada devolve
// 200 com o veredicto no corpo, e não um erro HTTP — um auditor tem de
// conseguir distinguir "cadeia partida" de "serviço em baixo".
func TestFinanceiro_VerificarCadeia_Quebrada_200ComVeredicto(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil,
		sessaoLabDe("aud-1", identidade.PapelAuditor), &duploVerificarCadeia{quebrada: true})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/financeiro/facturas/cadeia/verificacao?serie=2026", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("uma cadeia quebrada devia responder 200 com o veredicto, veio %d (%s)", w.Code, w.Body.String())
	}
	var resp appfinanceiro.ResultadoVerificacao
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta não é JSON válido: %v", err)
	}
	if resp.Integra {
		t.Fatalf("esperava integra=false para uma cadeia quebrada, veio true")
	}
	if resp.Detalhe == "" {
		t.Fatalf("esperava o detalhe do diagnóstico da quebra no corpo, veio vazio")
	}
}

// A rota literal /cadeia/verificacao tem de continuar alcançável mesmo sem
// query params. Nota: isto não depende da ordem de registo em
// RegistarFinanceiro — a árvore radix do Gin já prioriza segmentos estáticos
// sobre parâmetros (confirmado invertendo a ordem: o teste continua a
// passar) — este teste cobre só o comportamento observável da rota.
func TestFinanceiro_CadeiaVerificacao_RotaLiteralAlcancavel_200(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil,
		sessaoLabDe("aud-1", identidade.PapelAuditor))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/financeiro/facturas/cadeia/verificacao", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("a rota da cadeia foi capturada por /:fid: veio %d", w.Code)
	}
}
