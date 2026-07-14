package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// duploEmitir devolve a requisição que lhe pedirem, guardando o actor recebido.
type duploEmitir struct {
	actorRecebido string
	erro          error
}

func (d *duploEmitir) Executar(_ context.Context, actor string, _ applaboratorio.DadosEmitirRequisicao) (applaboratorio.DetalheRequisicao, error) {
	d.actorRecebido = actor
	if d.erro != nil {
		return applaboratorio.DetalheRequisicao{}, d.erro
	}
	return applaboratorio.DetalheRequisicao{ID: "req-1", MedicoRequisitanteID: actor}, nil
}

type duploSubmeter struct {
	actorRecebido string
	valorRecebido string
}

func (d *duploSubmeter) Executar(_ context.Context, actor, _ string, dados applaboratorio.DadosSubmeterPreliminar) (applaboratorio.DetalheResultado, error) {
	d.actorRecebido = actor
	d.valorRecebido = dados.Valor
	return applaboratorio.DetalheResultado{ID: "res-1", Estado: string(dominio.ResProcessada), TecnicoSubmissorID: actor}, nil
}

// Os restantes casos de uso não são exercitados por estes testes: duplos mínimos.
type duploColher struct{}

func (duploColher) Executar(_ context.Context, actor, id string) (applaboratorio.DetalheResultado, error) {
	return applaboratorio.DetalheResultado{ID: id, Estado: string(dominio.ResColhida)}, nil
}

type duploRecusar struct{}

func (duploRecusar) Executar(_ context.Context, _, id, motivo string) (applaboratorio.DetalheResultado, error) {
	if motivo == "" {
		return applaboratorio.DetalheResultado{}, erros.Novo(erros.CategoriaValidacao, "motivo em falta")
	}
	return applaboratorio.DetalheResultado{ID: id, Estado: string(dominio.ResRecusada)}, nil
}

type duploRegistarAnalise struct{}

func (duploRegistarAnalise) Executar(_ context.Context, _ string, d applaboratorio.DadosNovaAnalise) (applaboratorio.DetalheAnalise, error) {
	return applaboratorio.DetalheAnalise{Codigo: d.Codigo}, nil
}

type duploListarAnalises struct{}

func (duploListarAnalises) Executar(_ context.Context) ([]applaboratorio.ResumoAnalise, error) {
	return []applaboratorio.ResumoAnalise{{Codigo: "HB"}}, nil
}

type duploObterRequisicao struct{}

func (duploObterRequisicao) Executar(_ context.Context, id string) (applaboratorio.DetalheRequisicao, error) {
	return applaboratorio.DetalheRequisicao{ID: id}, nil
}

type duploListarRequisicoes struct{}

func (duploListarRequisicoes) Executar(_ context.Context, _ string) ([]applaboratorio.ResumoRequisicao, error) {
	return []applaboratorio.ResumoRequisicao{}, nil
}

type duploListarFila struct{}

func (duploListarFila) Executar(_ context.Context, _ []dominio.EstadoResultado) ([]applaboratorio.ResumoResultado, error) {
	return []applaboratorio.ResumoResultado{{ID: "res-1", Estado: string(dominio.ResPendente)}}, nil
}

type duploListarResultadosEpisodio struct{}

func (duploListarResultadosEpisodio) Executar(_ context.Context, _ string) ([]applaboratorio.ResumoResultado, error) {
	return []applaboratorio.ResumoResultado{}, nil
}

// routerLab monta o router com os duplos e uma sessão fixa. Usa o `fakeAuth` que já
// existe no pacote de testes (ver `identidade_test.go`, reutilizado por
// `cirurgia_test.go`) — não criar outro.
func routerLab(t *testing.T, emitir *duploEmitir, submeter *duploSubmeter, sessao identidade.Sessao) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := adhttp.NovoLaboratorioHandler(
		duploRegistarAnalise{}, duploListarAnalises{},
		emitir, duploObterRequisicao{}, duploListarRequisicoes{},
		duploColher{}, duploRecusar{}, submeter,
		duploListarFila{}, duploListarResultadosEpisodio{},
	)
	adhttp.RegistarLaboratorio(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

// sessaoLabDe constrói uma sessão de teste com um papel.
func sessaoLabDe(sujeito string, papel identidade.Papel) identidade.Sessao {
	return identidade.Sessao{Sujeito: sujeito, Papeis: []identidade.Papel{papel}}
}

func TestEmitirRequisicao_UsaOSujeitoAutenticado(t *testing.T) {
	emitir := &duploEmitir{}
	r := routerLab(t, emitir, &duploSubmeter{}, sessaoLabDe("med-99", identidade.PapelMedico))

	corpo, _ := json.Marshal(map[string]any{
		"doente_id":  "doente-1",
		"prioridade": "ROTINA",
		"itens":      []map[string]string{{"codigo_analise": "HB"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/episodios/ep-1/requisicoes", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	// O requisitante nunca vem do corpo: vem da sessão.
	if emitir.actorRecebido != "med-99" {
		t.Fatalf("esperava o actor da sessão (med-99), veio %q", emitir.actorRecebido)
	}
}

func TestEmitirRequisicao_CorpoMalformado_400(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/episodios/ep-1/requisicoes", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

func TestEmitirRequisicao_Enfermeiro_Proibido(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("enf-1", identidade.PapelEnfermeiro))
	corpo, _ := json.Marshal(map[string]any{"doente_id": "doente-1", "prioridade": "ROTINA"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/episodios/ep-1/requisicoes", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403 para o Enfermeiro, veio %d", w.Code)
	}
}

func TestSubmeterPreliminar_SoTecnicoLab(t *testing.T) {
	// Um médico não submete resultados: 403.
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("med-1", identidade.PapelMedico))
	corpo, _ := json.Marshal(map[string]string{"valor": "12.5"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/preliminar", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403 para o Medico, veio %d", w.Code)
	}

	// O técnico submete, e o submissor é o sujeito da sessão.
	submeter := &duploSubmeter{}
	rt := routerLab(t, &duploEmitir{}, submeter, sessaoLabDe("tec-7", identidade.PapelTecnicoLab))
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/v1/resultados/res-1/preliminar", bytes.NewReader(corpo))
	req2.Header.Set("Content-Type", "application/json")
	rt.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("esperava 200 para o TecnicoLab, veio %d (%s)", w2.Code, w2.Body.String())
	}
	if submeter.actorRecebido != "tec-7" || submeter.valorRecebido != "12.5" {
		t.Fatalf("submissão não usou a sessão/corpo esperados: actor=%q valor=%q",
			submeter.actorRecebido, submeter.valorRecebido)
	}
}

func TestSubmeterPreliminar_CorpoMalformado(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("tec-1", identidade.PapelTecnicoLab))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/preliminar", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

// TestFila_Medico_Proibido é a prova central deste sprint: a fila do laboratório
// devolve todos os estados (preliminares incluídos), pelo que o Médico não pode
// entrar por esta rota — só o pessoal do laboratório e a direcção clínica.
func TestFila_Medico_Proibido(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/laboratorio/fila", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403 para o Medico na fila, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFila_TecnicoLab_200(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("tec-1", identidade.PapelTecnicoLab))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/laboratorio/fila", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200 para o TecnicoLab na fila, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFila_Patologista_200(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("pat-1", identidade.PapelPatologista))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/laboratorio/fila", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200 para o Patologista na fila, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFila_Director_200(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("dir-1", identidade.PapelDirector))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/laboratorio/fila", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200 para o Director na fila, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFila_Enfermeiro_Proibido(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("enf-1", identidade.PapelEnfermeiro))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/laboratorio/fila", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403 para o Enfermeiro na fila, veio %d", w.Code)
	}
}

func TestResultadosEpisodio_Medico_200(t *testing.T) {
	// A leitura clínica é aberta ao Médico — o filtro de visibilidade (preliminar
	// invisível) vive na aplicação, não nesta rota.
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/episodios/ep-1/resultados", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200 para o Medico nos resultados do episódio, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestResultadosEpisodio_Farmaceutico_Proibido(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("farm-1", identidade.PapelFarmaceutico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/episodios/ep-1/resultados", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403 para o Farmaceutico nos resultados do episódio, veio %d", w.Code)
	}
}

func TestColherAmostra_TecnicoLab_200(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("tec-1", identidade.PapelTecnicoLab))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/colheita", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200 para o TecnicoLab na colheita, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestColherAmostra_Medico_Proibido(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/colheita", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403 para o Medico na colheita, veio %d", w.Code)
	}
}

func TestRecusarAmostra_TecnicoLab_200(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("tec-1", identidade.PapelTecnicoLab))
	corpo, _ := json.Marshal(map[string]string{"motivo": "amostra hemolisada"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/recusa", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200 para o TecnicoLab na recusa, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestRecusarAmostra_MotivoEmFalta_400(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("tec-1", identidade.PapelTecnicoLab))
	corpo, _ := json.Marshal(map[string]string{"motivo": ""})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/recusa", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("esperava 400 por motivo em falta, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestRecusarAmostra_CorpoMalformado_400(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("tec-1", identidade.PapelTecnicoLab))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/recusa", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

func TestRegistarAnalise_Admin_201(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("adm-1", identidade.PapelAdmin))
	corpo, _ := json.Marshal(map[string]string{"codigo": "HB", "nome": "Hemoglobina", "unidade": "g/dL"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/analises", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("esperava 201 para o Admin a registar análise, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestRegistarAnalise_TecnicoLab_Proibido(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("tec-1", identidade.PapelTecnicoLab))
	corpo, _ := json.Marshal(map[string]string{"codigo": "HB"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/analises", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403 para o TecnicoLab a registar análise, veio %d", w.Code)
	}
}

func TestListarAnalises_LeituraClinica_200(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("enf-1", identidade.PapelEnfermeiro))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/analises", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200 a listar análises, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestObterRequisicao_LeituraClinica_200(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/requisicoes/req-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200 a obter requisição, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestListarRequisicoes_LeituraClinica_200(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoLabDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/episodios/ep-1/requisicoes", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200 a listar requisições do episódio, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestEmitirRequisicao_ErroDoCasoDeUso_Propagado(t *testing.T) {
	emitir := &duploEmitir{erro: erros.Novo(erros.CategoriaConflito, "só é possível requisitar num episódio aberto")}
	r := routerLab(t, emitir, &duploSubmeter{}, sessaoLabDe("med-1", identidade.PapelMedico))
	corpo, _ := json.Marshal(map[string]any{"doente_id": "doente-1", "prioridade": "ROTINA"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/episodios/ep-1/requisicoes", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Fatalf("esperava 409 propagado do caso de uso, veio %d (%s)", w.Code, w.Body.String())
	}
}
