package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

// duploMarcar guarda o actor recebido e devolve uma marcação fixa.
type duploMarcar struct{ actorRecebido string }

func (d *duploMarcar) Executar(_ context.Context, actor string, _ apprecepcao.DadosMarcar) (apprecepcao.DetalheMarcacao, error) {
	d.actorRecebido = actor
	return apprecepcao.DetalheMarcacao{ID: "marc-1", Estado: "MARCADA"}, nil
}

type duploDefinirJanela struct{}

func (duploDefinirJanela) Executar(_ context.Context, _ string, _ apprecepcao.DadosDefinirJanela) (apprecepcao.DetalheJanela, error) {
	return apprecepcao.DetalheJanela{ID: "jan-1"}, nil
}

type duploRemoverJanela struct{}

func (duploRemoverJanela) Executar(_ context.Context, _, _ string) error { return nil }

type duploRemarcar struct{}

func (duploRemarcar) Executar(_ context.Context, _, _ string, _ apprecepcao.DadosRemarcar) (apprecepcao.DetalheMarcacao, error) {
	return apprecepcao.DetalheMarcacao{ID: "marc-2", Estado: "MARCADA", RemarcaDe: "marc-1"}, nil
}

type duploCancelar struct{}

func (duploCancelar) Executar(_ context.Context, _, _, _ string) (apprecepcao.DetalheMarcacao, error) {
	return apprecepcao.DetalheMarcacao{ID: "marc-1", Estado: "CANCELADA"}, nil
}

type duploRegistarFalta struct{}

func (duploRegistarFalta) Executar(_ context.Context, _, _ string) (apprecepcao.DetalheMarcacao, error) {
	return apprecepcao.DetalheMarcacao{ID: "marc-1", Estado: "FALTOU"}, nil
}

type duploListarAgenda struct{}

func (duploListarAgenda) Executar(_ context.Context, _ string, _, _ time.Time) (apprecepcao.Agenda, error) {
	return apprecepcao.Agenda{}, nil
}

type duploListarMarcacoesDoente struct{}

func (duploListarMarcacoesDoente) Executar(_ context.Context, _ string) ([]apprecepcao.ResumoMarcacao, error) {
	return []apprecepcao.ResumoMarcacao{}, nil
}

func routerRecepcao(t *testing.T, marcar *duploMarcar, sessao identidade.Sessao) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := adhttp.NovoRecepcaoHandler(
		duploDefinirJanela{}, duploRemoverJanela{},
		marcar, duploRemarcar{}, duploCancelar{}, duploRegistarFalta{},
		duploListarAgenda{}, duploListarMarcacoesDoente{},
	)
	adhttp.RegistarRecepcao(r, h, adhttp.Auth(fakeAuth{sessao: sessao}), adhttp.MFAObrigatoria())
	return r
}

// sessaoRecepcaoDe constrói uma sessão de teste com um papel.
func sessaoRecepcaoDe(sujeito string, papel identidade.Papel) identidade.Sessao {
	// Segundo factor sempre presente: o router de teste espelha a cadeia da
	// produção (Auth + MFAObrigatoria), pelo que as sessões de papel sensível
	// (Director, Admin) têm de comprovar autenticação forte. Para os papéis
	// não-sensíveis o campo é inócuo — o middleware é um no-op.
	return identidade.Sessao{Sujeito: sujeito, Papeis: []identidade.Papel{papel}, AutenticacaoForte: true}
}

func TestMarcar_UsaOSujeitoAutenticado(t *testing.T) {
	marcar := &duploMarcar{}
	r := routerRecepcao(t, marcar, sessaoRecepcaoDe("adm-9", identidade.PapelAdministrativo))
	corpo, _ := json.Marshal(map[string]any{
		"doente_id": "doe-1", "medico_id": "med-1", "especialidade_id": "esp-1",
		"inicio": "2026-08-01T09:00:00Z", "fim": "2026-08-01T09:30:00Z",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	if marcar.actorRecebido != "adm-9" {
		t.Fatalf("o actor devia vir da sessão (adm-9), veio %q", marcar.actorRecebido)
	}
}

func TestMarcar_Medico_Proibido(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	corpo, _ := json.Marshal(map[string]any{"doente_id": "doe-1", "medico_id": "med-1", "especialidade_id": "esp-1", "inicio": "2026-08-01T09:00:00Z", "fim": "2026-08-01T09:30:00Z"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("um médico não marca: esperava 403, veio %d", w.Code)
	}
}

func TestMarcar_CorpoMalformado_400(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

func TestListarAgenda_MedicoPodeLer(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/agenda?medico=med-1&de=2026-08-01T00:00:00Z&ate=2026-08-01T23:00:00Z", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("o médico devia poder ler a agenda: esperava 200, veio %d", w.Code)
	}
}

// --- Testes adicionais (cobertura das restantes rotas e casos de erro) ---

func TestDefinirJanela_Administrativo_Criado(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	corpo, _ := json.Marshal(map[string]any{
		"especialidade_id": "esp-1", "inicio": "2026-08-01T09:00:00Z", "fim": "2026-08-01T12:00:00Z",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/medicos/med-1/janelas", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestDefinirJanela_CorpoMalformado_400(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/medicos/med-1/janelas", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

func TestDefinirJanela_Medico_Proibido(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	corpo, _ := json.Marshal(map[string]any{
		"especialidade_id": "esp-1", "inicio": "2026-08-01T09:00:00Z", "fim": "2026-08-01T12:00:00Z",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/medicos/med-1/janelas", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("um médico não define janelas: esperava 403, veio %d", w.Code)
	}
}

func TestRemoverJanela_Administrativo_SemConteudo(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/janelas/jan-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("esperava 204, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestRemoverJanela_Medico_Proibido(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/janelas/jan-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403, veio %d", w.Code)
	}
}

func TestRemarcar_Administrativo_Criado(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	corpo, _ := json.Marshal(map[string]any{"inicio": "2026-08-02T09:00:00Z", "fim": "2026-08-02T09:30:00Z"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/remarcacao", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestRemarcar_CorpoMalformado_400(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/remarcacao", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

func TestCancelar_Administrativo_OK(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	corpo, _ := json.Marshal(map[string]any{"motivo": "desistência do doente"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/cancelamento", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestCancelar_CorpoMalformado_400(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/cancelamento", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

func TestCancelar_Medico_Proibido(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	corpo, _ := json.Marshal(map[string]any{"motivo": "desistência do doente"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/cancelamento", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403, veio %d", w.Code)
	}
}

func TestRegistarFalta_Administrativo_OK(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/falta", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestRegistarFalta_Medico_Proibido(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/falta", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403, veio %d", w.Code)
	}
}

func TestListarAgenda_ParametroDeInvalido_400(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/agenda?medico=med-1&de=nao-e-data&ate=2026-08-01T23:00:00Z", nil)
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("esperava 400, veio %d", w.Code)
	}
}

func TestListarAgenda_ParametroAteInvalido_400(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/agenda?medico=med-1&de=2026-08-01T00:00:00Z&ate=nao-e-data", nil)
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("esperava 400, veio %d", w.Code)
	}
}

func TestListarAgenda_Administrativo_Proibida_Recepcionista(t *testing.T) {
	// Papel sem qualquer permissão de leitura de agenda: qualquer papel fora do RBAC.
	r := routerRecepcao(t, &duploMarcar{}, identidade.Sessao{Sujeito: "x-1", Papeis: []identidade.Papel{}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/agenda?medico=med-1&de=2026-08-01T00:00:00Z&ate=2026-08-01T23:00:00Z", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403, veio %d", w.Code)
	}
}

func TestListarMarcacoesDoente_Medico_OK(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/doentes/doe-1/marcacoes", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200, veio %d (%s)", w.Code, w.Body.String())
	}
}

// ADR-042: antes desta fatia, os grupos da Recepção não recebiam a
// MFAObrigatoria, pelo que um papel sensível alcançava a agenda sem segundo
// factor. A sessão é construída à mão (e não pela `sessaoRecepcaoDe`) porque esta
// fixa sempre o segundo factor — é justamente a sua ausência que se prova.
func TestRecepcao_PapelSensivelSemSegundoFactor_403(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, identidade.Sessao{
		Sujeito: "dir-1",
		Papeis:  []identidade.Papel{identidade.PapelDirector},
		// sem AutenticacaoForte: é este o ponto do teste
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/agenda?medico=med-1&de=2026-08-01T00:00:00Z&ate=2026-08-01T23:00:00Z", nil)
	r.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Fatalf("código = %d, queria 403", w.Code)
	}
	// Asserir o tipo do problema, e não só o 403: sem isto, o teste não distingue
	// o 403 do MFA do 403 do RBAC, e passaria a verde pela razão errada se o RBAC
	// mudasse.
	if corpo := w.Body.String(); !strings.Contains(corpo, "mfa-obrigatorio") {
		t.Errorf("corpo = %s, queria type mfa-obrigatorio", corpo)
	}
}

func TestRecepcao_PapelSensivelComSegundoFactor_Prossegue(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("dir-1", identidade.PapelDirector))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/agenda?medico=med-1&de=2026-08-01T00:00:00Z&ate=2026-08-01T23:00:00Z", nil)
	r.ServeHTTP(w, req)

	if w.Code == 403 {
		t.Errorf("com segundo factor não devia dar 403; corpo = %s", w.Body.String())
	}
}
