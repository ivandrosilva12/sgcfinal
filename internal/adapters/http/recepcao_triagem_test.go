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
	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

type duploRegistarTriagem struct{ actorRecebido string }

func (d *duploRegistarTriagem) Executar(_ context.Context, actor, _ string, _ apprecepcao.DadosTriagem) (apprecepcao.DetalheTriagem, error) {
	d.actorRecebido = actor
	return apprecepcao.DetalheTriagem{ID: "tri-1", ChegadaID: "cheg-1", Prioridade: "AMARELO"}, nil
}

type duploObterTriagem struct{}

func (duploObterTriagem) Executar(_ context.Context, _ string) (apprecepcao.DetalheTriagem, error) {
	return apprecepcao.DetalheTriagem{ID: "tri-1", ChegadaID: "cheg-1", Prioridade: "VERDE"}, nil
}

type duploListarFilaClinica struct{}

func (duploListarFilaClinica) Executar(_ context.Context, _ string) ([]apprecepcao.ResumoFilaClinica, error) {
	return []apprecepcao.ResumoFilaClinica{}, nil
}

func routerTriagem(t *testing.T, registar *duploRegistarTriagem, sessao identidade.Sessao) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := adhttp.NovoRecepcaoTriagemHandler(registar, duploObterTriagem{}, duploListarFilaClinica{})
	adhttp.RegistarRecepcaoTriagem(r, h, adhttp.Auth(fakeAuth{sessao: sessao}), adhttp.MFAObrigatoria())
	return r
}

func TestRegistarTriagem_UsaOSujeitoAutenticado(t *testing.T) {
	reg := &duploRegistarTriagem{}
	r := routerTriagem(t, reg, sessaoRecepcaoDe("enf-9", identidade.PapelEnfermeiro))
	corpo, _ := json.Marshal(map[string]any{"prioridade": "AMARELO", "medico_id": "med-1"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/triagem", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	if reg.actorRecebido != "enf-9" {
		t.Fatalf("o enfermeiro devia vir da sessão, veio %q", reg.actorRecebido)
	}
}

func TestRegistarTriagem_Administrativo_Proibido(t *testing.T) {
	r := routerTriagem(t, &duploRegistarTriagem{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	corpo, _ := json.Marshal(map[string]any{"prioridade": "AMARELO"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/triagem", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("o Administrativo não tria: esperava 403, veio %d", w.Code)
	}
}

func TestRegistarTriagem_CorpoMalformado_400(t *testing.T) {
	r := routerTriagem(t, &duploRegistarTriagem{}, sessaoRecepcaoDe("enf-1", identidade.PapelEnfermeiro))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/triagem", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

func TestObterTriagem_Administrativo_Proibido(t *testing.T) {
	r := routerTriagem(t, &duploRegistarTriagem{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/chegadas/cheg-1/triagem", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("leitura clínica proibida ao Administrativo: esperava 403, veio %d", w.Code)
	}
}

func TestFilaClinica_MedicoPodeLer(t *testing.T) {
	r := routerTriagem(t, &duploRegistarTriagem{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/fila-clinica?medico=med-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("o médico pode ler a fila clínica: esperava 200, veio %d", w.Code)
	}
}

// ADR-042: antes desta fatia, os grupos da Triagem não recebiam a MFAObrigatoria,
// pelo que um papel sensível alcançava a leitura clínica da triagem sem segundo
// factor. Usa-se o Director porque é o papel sensível que o `leituraClinica` do
// handler admite — com um papel fora do RBAC o par de testes provaria o RBAC, não
// o MFA. A sessão é construída à mão (e não pela `sessaoRecepcaoDe`) porque esta
// fixa sempre o segundo factor — é justamente a sua ausência que se prova. A rota
// GET /api/v1/chegadas/:cid/triagem é leitura pura (obterTriagem).
func TestTriagem_PapelSensivelSemSegundoFactor_403(t *testing.T) {
	r := routerTriagem(t, &duploRegistarTriagem{}, identidade.Sessao{
		Sujeito: "dir-1",
		Papeis:  []identidade.Papel{identidade.PapelDirector},
		// sem AutenticacaoForte: é este o ponto do teste
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/chegadas/cheg-1/triagem", nil)
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

func TestTriagem_PapelSensivelComSegundoFactor_Prossegue(t *testing.T) {
	r := routerTriagem(t, &duploRegistarTriagem{}, sessaoRecepcaoDe("dir-1", identidade.PapelDirector))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/chegadas/cheg-1/triagem", nil)
	r.ServeHTTP(w, req)
	if w.Code == 403 {
		t.Errorf("com segundo factor não devia dar 403; corpo = %s", w.Body.String())
	}
}
