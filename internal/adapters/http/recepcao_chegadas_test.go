package http_test

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

type duploRegistarChegada struct{ actorRecebido string }

func (d *duploRegistarChegada) Executar(_ context.Context, actor, _ string) (apprecepcao.DetalheChegada, error) {
	d.actorRecebido = actor
	return apprecepcao.DetalheChegada{ID: "cheg-1", Estado: "AGUARDA", MarcacaoID: "marc-1"}, nil
}

type duploRegistarWalkIn struct{ actorRecebido string }

func (d *duploRegistarWalkIn) Executar(_ context.Context, actor string, _ apprecepcao.DadosWalkIn) (apprecepcao.DetalheChegada, error) {
	d.actorRecebido = actor
	return apprecepcao.DetalheChegada{ID: "cheg-2", Estado: "AGUARDA"}, nil
}

type duploChamar struct{}

func (duploChamar) Executar(_ context.Context, _, _ string) (apprecepcao.DetalheChegada, error) {
	return apprecepcao.DetalheChegada{ID: "cheg-1", Estado: "CHAMADO"}, nil
}

type duploDesistencia struct{}

func (duploDesistencia) Executar(_ context.Context, _, _ string) (apprecepcao.DetalheChegada, error) {
	return apprecepcao.DetalheChegada{ID: "cheg-1", Estado: "DESISTIU"}, nil
}

// duploListarFilaChegadas: nomeado com o sufixo "Chegadas" porque já existe um
// duploListarFila em laboratorio_test.go (fila de resultados, assinatura diferente).
type duploListarFilaChegadas struct{}

func (duploListarFilaChegadas) Executar(_ context.Context, _ string) ([]apprecepcao.ResumoChegada, error) {
	return []apprecepcao.ResumoChegada{}, nil
}

func routerChegadas(t *testing.T, chegar *duploRegistarChegada, walkin *duploRegistarWalkIn, sessao identidade.Sessao) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := adhttp.NovoRecepcaoChegadasHandler(chegar, walkin, duploChamar{}, duploDesistencia{}, duploListarFilaChegadas{})
	adhttp.RegistarRecepcaoChegadas(r, h, adhttp.Auth(fakeAuth{sessao: sessao}), adhttp.MFAObrigatoria())
	return r
}

func TestRegistarChegada_UsaOSujeitoAutenticado(t *testing.T) {
	chegar := &duploRegistarChegada{}
	r := routerChegadas(t, chegar, &duploRegistarWalkIn{}, sessaoRecepcaoDe("adm-9", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/chegada", nil)
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	if chegar.actorRecebido != "adm-9" {
		t.Fatalf("o actor devia vir da sessão, veio %q", chegar.actorRecebido)
	}
}

func TestRegistarChegada_Medico_Proibido(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/chegada", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("um médico não faz check-in: esperava 403, veio %d", w.Code)
	}
}

func TestWalkIn_CorpoMalformado_400(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

func TestChamar_Enfermeiro_Permitido(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("enf-1", identidade.PapelEnfermeiro))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/chamada", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("o enfermeiro pode chamar: esperava 200, veio %d", w.Code)
	}
}

func TestFila_MedicoPodeLer(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/fila?especialidade=esp-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("o médico pode ver a fila: esperava 200, veio %d", w.Code)
	}
}

// --- Testes adicionais (cobertura das restantes rotas e casos de erro) ---

func TestWalkIn_Administrativo_Criado(t *testing.T) {
	walkin := &duploRegistarWalkIn{}
	r := routerChegadas(t, &duploRegistarChegada{}, walkin, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	corpo := []byte(`{"doente_id":"doe-1","especialidade_id":"esp-1"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	if walkin.actorRecebido != "adm-1" {
		t.Fatalf("o actor devia vir da sessão, veio %q", walkin.actorRecebido)
	}
}

func TestWalkIn_Medico_Proibido(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	corpo := []byte(`{"doente_id":"doe-1","especialidade_id":"esp-1"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("um médico não regista walk-in: esperava 403, veio %d", w.Code)
	}
}

func TestChamar_Administrativo_Permitido(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/chamada", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("o administrativo pode chamar: esperava 200, veio %d", w.Code)
	}
}

func TestChamar_ForaDoRBAC_Proibido(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, identidade.Sessao{Sujeito: "x-1", Papeis: []identidade.Papel{}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/chamada", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403, veio %d", w.Code)
	}
}

func TestDesistencia_Administrativo_OK(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/desistencia", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestDesistencia_Medico_Proibido(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/desistencia", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("um médico não regista desistência: esperava 403, veio %d", w.Code)
	}
}

func TestFila_Administrativo_OK(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/fila?especialidade=esp-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFila_ForaDoRBAC_Proibida(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, identidade.Sessao{Sujeito: "x-1", Papeis: []identidade.Papel{}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/fila?especialidade=esp-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403, veio %d", w.Code)
	}
}

// ADR-042: antes desta fatia, os grupos de Chegadas não recebiam a MFAObrigatoria,
// pelo que um papel sensível alcançava as rotas sem segundo factor. Ao contrário
// das outras famílias, a fila de chegadas (filaLeitura) não admite nenhum papel
// sensível: o único papel sensível exposto é o Director, no `soAdministrativo`, que
// governa POST /api/v1/marcacoes/:mid/chegada (subgrupo gmar). É essa a rota onde o
// Director passa o RBAC — logo é aí que se prova que a MFA guarda o subgrupo. Um
// papel fora do RBAC provaria o RBAC, não o MFA.
func TestChegadas_PapelSensivelSemSegundoFactor_403(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, identidade.Sessao{
		Sujeito: "dir-1",
		Papeis:  []identidade.Papel{identidade.PapelDirector},
		// sem AutenticacaoForte: é este o ponto do teste
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/chegada", nil)
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

func TestChegadas_PapelSensivelComSegundoFactor_Prossegue(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("dir-1", identidade.PapelDirector))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/chegada", nil)
	r.ServeHTTP(w, req)
	if w.Code == 403 {
		t.Errorf("com segundo factor não devia dar 403; corpo = %s", w.Body.String())
	}
}
