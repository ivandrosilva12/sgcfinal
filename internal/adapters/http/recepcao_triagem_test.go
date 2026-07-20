package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
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
