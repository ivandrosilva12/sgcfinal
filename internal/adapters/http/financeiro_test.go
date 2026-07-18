package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appfinanceiro "github.com/ivandrosilva12/sgcfinal/internal/application/financeiro"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
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

// routerFin monta o router com os duplos e uma sessão fixa. Usa o `fakeAuth` já
// existente no pacote de testes (ver `identidade_test.go`) e a `sessaoLabDe`
// genérica (ver `laboratorio_test.go`) — não redefine nenhum dos dois.
func routerFin(t *testing.T, criar *duploCriarFactura, adicionar *duploAdicionarItemFin, sessao identidade.Sessao) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := adhttp.NovoFinanceiroHandler(criar, adicionar, duploRemoverItemFin{}, duploObterFactura{}, duploListarFacturas{})
	adhttp.RegistarFinanceiro(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestFinanceiro_CriarFactura_Tesoureiro_201_UsaOSujeitoAutenticado(t *testing.T) {
	criar := &duploCriarFactura{}
	r := routerFin(t, criar, &duploAdicionarItemFin{}, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

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
	r := routerFin(t, &duploCriarFactura{}, adicionar, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

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
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, sessaoLabDe("dir-1", identidade.PapelDirector))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/financeiro/facturas/fac-1", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("esperava 200 para o Director, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFinanceiro_CriarFactura_Medico_Proibido(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, sessaoLabDe("med-1", identidade.PapelMedico))

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
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

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
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/financeiro/facturas", nil)
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("esperava 400 sem episodio_id, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFinanceiro_ListarFacturas_EpisodioValido_200(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/financeiro/facturas?episodio_id=11111111-1111-1111-1111-111111111111", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("esperava 200 com episodio_id válido, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFinanceiro_CriarFactura_EpisodioInvalido_400(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, sessaoLabDe("tes-1", identidade.PapelTesoureiro))

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
