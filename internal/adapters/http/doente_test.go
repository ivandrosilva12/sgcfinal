package http_test

import (
	"context"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes dos serviços de doentes ---

type fakeRegistarDoente struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeRegistarDoente) Executar(_ context.Context, _ string, _ appclinico.DadosNovoDoente) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

type fakeObterDoente struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeObterDoente) Executar(_ context.Context, _, _ string) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

type fakePesquisarDoentes struct {
	out appclinico.PaginaDoentes
	err error
}

func (f fakePesquisarDoentes) Executar(_ context.Context, _ appclinico.FiltroDoentes) (appclinico.PaginaDoentes, error) {
	return f.out, f.err
}

type fakeActualizarDoente struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeActualizarDoente) Executar(_ context.Context, _, _ string, _ appclinico.DadosActualizarDoente) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

type fakeGerirEstado struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeGerirEstado) Desactivar(_ context.Context, _, _, _ string) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}
func (f fakeGerirEstado) DeclararFalecido(_ context.Context, _, _ string, _ time.Time, _ string) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

type fakeRegistarAlergia struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeRegistarAlergia) Executar(_ context.Context, _, _ string, _ appclinico.DadosAlergia) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

type fakeRegistarAntecedente struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeRegistarAntecedente) Executar(_ context.Context, _, _ string, _ appclinico.DadosAntecedente) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

func routerDoentes(sessao dominio.Sessao) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoDoentesHandler(
		fakeRegistarDoente{out: appclinico.DetalheDoente{ID: "id-1", NumProcesso: "P-2026-000001"}},
		fakeObterDoente{out: appclinico.DetalheDoente{ID: "id-1"}},
		fakePesquisarDoentes{out: appclinico.PaginaDoentes{Total: 0, Itens: nil}},
		fakeActualizarDoente{out: appclinico.DetalheDoente{ID: "id-1"}},
		fakeGerirEstado{out: appclinico.DetalheDoente{ID: "id-1", Estado: "INACTIVO"}},
		fakeRegistarAlergia{out: appclinico.DetalheDoente{ID: "id-1"}},
		fakeRegistarAntecedente{out: appclinico.DetalheDoente{ID: "id-1"}},
	)
	adhttp.RegistarDoentes(r, h, adhttp.Auth(fakeAuth{sessao: sessao}), adhttp.MFAObrigatoria())
	return r
}

func TestDoentes_Registar_AdministrativoPermitido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Sujeito: "a1", Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	corpo := `{"nome_completo":"Ana","data_nascimento":"1990-05-20","sexo":"F","bi":"00123456LA042","telefone":"+244923456789"}`
	w := pedidoCorpo(r, "POST", "/api/v1/doentes", corpo)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Registar_FarmaceuticoProibido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}})
	corpo := `{"nome_completo":"Ana","data_nascimento":"1990-05-20","sexo":"F","bi":"00123456LA042","telefone":"+244923456789"}`
	w := pedidoCorpo(r, "POST", "/api/v1/doentes", corpo)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestDoentes_Registar_DataInvalida_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	corpo := `{"nome_completo":"Ana","data_nascimento":"20-05-1990","sexo":"F","bi":"00123456LA042","telefone":"+244923456789"}`
	w := pedidoCorpo(r, "POST", "/api/v1/doentes", corpo)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Registar_CorpoInvalido_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes", `{`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Pesquisar_LeituraAmpla(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}})
	w := pedido(r, "GET", "/api/v1/doentes?termo=ana", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestDoentes_Pesquisar_ComPaginacao(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelDirector}, AutenticacaoForte: true})
	w := pedido(r, "GET", "/api/v1/doentes?termo=ana&estado=ACTIVO&limite=10&deslocamento=5", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Pesquisar_Proibido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{}})
	w := pedido(r, "GET", "/api/v1/doentes", "Bearer xyz")
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestDoentes_Obter_LeituraAmpla(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}, AutenticacaoForte: true})
	w := pedido(r, "GET", "/api/v1/doentes/id-1", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestDoentes_Actualizar_200(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Sujeito: "a1", Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	corpo := `{"identificacao":{"nome_completo":"Ana Maria","data_nascimento":"1990-05-20","sexo":"F"},"contactos":{"telefone":"+244923456780"}}`
	w := pedidoCorpo(r, "PATCH", "/api/v1/doentes/id-1", corpo)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Actualizar_DataInvalida_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	corpo := `{"identificacao":{"nome_completo":"Ana","data_nascimento":"nao-e-data","sexo":"F"}}`
	w := pedidoCorpo(r, "PATCH", "/api/v1/doentes/id-1", corpo)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Actualizar_Proibido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}, AutenticacaoForte: true})
	w := pedidoCorpo(r, "PATCH", "/api/v1/doentes/id-1", `{}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
	// ADR-042: o Auditor é um papel sensível, pelo que este 403 tem de vir do RBAC
	// e não da MFAObrigatoria. Sem esta asserção, perder o `AutenticacaoForte` da
	// sessão deixava o teste verde a provar a coisa errada.
	if corpo := w.Body.String(); strings.Contains(corpo, "mfa-obrigatorio") {
		t.Errorf("o 403 devia vir do RBAC, não do MFA; corpo = %s", corpo)
	}
}

func TestDoentes_Alergia_MedicoPermitido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/alergias", `{"substancia":"Penicilina","severidade":"GRAVE"}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Alergia_ComDataConfirmacao_200(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	corpo := `{"substancia":"Penicilina","severidade":"GRAVE","confirmada_em":"2020-01-15"}`
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/alergias", corpo)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Alergia_DataConfirmacaoInvalida_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	corpo := `{"substancia":"Penicilina","severidade":"GRAVE","confirmada_em":"nao-e-data"}`
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/alergias", corpo)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Alergia_AdministrativoProibido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/alergias", `{"substancia":"Penicilina","severidade":"GRAVE"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestDoentes_Antecedente_MedicoPermitido_200(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	corpo := `{"tipo":"DOENCA_CRONICA","descricao":"Diabetes tipo 2","data_inicio":"2015-03-10","activo":true}`
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/antecedentes", corpo)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Antecedente_DataInicioInvalida_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	corpo := `{"tipo":"DOENCA_CRONICA","descricao":"Diabetes","data_inicio":"nao-e-data"}`
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/antecedentes", corpo)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Antecedente_AdministrativoProibido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/antecedentes", `{"tipo":"X","descricao":"y"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestDoentes_Estado_Desactivar(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Sujeito: "a1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/estado", `{"accao":"desactivar","motivo":"engano"}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "INACTIVO") {
		t.Fatalf("esperava estado INACTIVO no corpo: %s", w.Body.String())
	}
}

func TestDoentes_Estado_Falecido_200(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Sujeito: "a1", Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/estado", `{"accao":"falecido","data_obito":"2026-01-10","causa_cid":"I21"}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Estado_FalecidoDataInvalida_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/estado", `{"accao":"falecido","data_obito":"nao-e-data"}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Estado_AccaoInvalida_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/estado", `{"accao":"teletransportar"}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestDoentes_Estado_CorpoInvalido_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/estado", `{`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestDoentes_Estado_Proibido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}, AutenticacaoForte: true})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/estado", `{"accao":"desactivar","motivo":"engano"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
	// ADR-042: o Auditor é um papel sensível, pelo que este 403 tem de vir do RBAC
	// e não da MFAObrigatoria. Sem esta asserção, perder o `AutenticacaoForte` da
	// sessão deixava o teste verde a provar a coisa errada.
	if corpo := w.Body.String(); strings.Contains(corpo, "mfa-obrigatorio") {
		t.Errorf("o 403 devia vir do RBAC, não do MFA; corpo = %s", corpo)
	}
}

func TestDoentes_Alergia_CorpoInvalido_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/alergias", `{`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestDoentes_Antecedente_CorpoInvalido_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/antecedentes", `{`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestDoentes_Antecedente_ErroAplicacaoMapeado(t *testing.T) {
	// Um erro de validação da aplicação deve mapear para 400 (RFC 7807).
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoDoentesHandler(
		fakeRegistarDoente{}, fakeObterDoente{}, fakePesquisarDoentes{}, fakeActualizarDoente{},
		fakeGerirEstado{}, fakeRegistarAlergia{},
		fakeRegistarAntecedente{err: erros.Novo(erros.CategoriaValidacao, "tipo inválido")},
	)
	adhttp.RegistarDoentes(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}), adhttp.MFAObrigatoria())
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/antecedentes", `{"tipo":"X","descricao":"y"}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestDoentes_Pesquisar_ErroAplicacaoMapeado_500(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoDoentesHandler(
		fakeRegistarDoente{}, fakeObterDoente{},
		fakePesquisarDoentes{err: erros.Novo(erros.CategoriaInterno, "falha na base de dados")},
		fakeActualizarDoente{}, fakeGerirEstado{}, fakeRegistarAlergia{}, fakeRegistarAntecedente{},
	)
	adhttp.RegistarDoentes(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}), adhttp.MFAObrigatoria())
	w := pedido(r, "GET", "/api/v1/doentes", "Bearer xyz")
	if w.Code != nethttp.StatusInternalServerError {
		t.Fatalf("esperava 500, obtive %d", w.Code)
	}
}

// ADR-042: antes desta fatia, o grupo de Doentes não recebia a MFAObrigatoria,
// pelo que um papel sensível alcançava dados clínicos sem segundo factor. Usa-se o
// Director (e não o Admin) porque é um papel sensível que o RBAC de leitura de
// doentes admite — com um papel fora do RBAC o par de testes provaria o RBAC, não
// o MFA.
func TestDoentes_PapelSensivelSemSegundoFactor_403(t *testing.T) {
	r := routerDoentes(dominio.Sessao{
		Sujeito: "dir-1",
		Papeis:  []dominio.Papel{dominio.PapelDirector},
		// sem AutenticacaoForte: é este o ponto do teste
	})
	w := pedido(r, "GET", "/api/v1/doentes/id-1", "Bearer xyz")
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

func TestDoentes_PapelSensivelComSegundoFactor_Prossegue(t *testing.T) {
	r := routerDoentes(dominio.Sessao{
		Sujeito:           "dir-1",
		Papeis:            []dominio.Papel{dominio.PapelDirector},
		AutenticacaoForte: true,
	})
	w := pedido(r, "GET", "/api/v1/doentes/id-1", "Bearer xyz")
	if w.Code == nethttp.StatusForbidden {
		t.Errorf("com segundo factor não devia dar 403; corpo = %s", w.Body.String())
	}
}

func TestDoentes_Obter_ErroNaoEncontrado_404(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoDoentesHandler(
		fakeRegistarDoente{}, fakeObterDoente{err: erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")},
		fakePesquisarDoentes{}, fakeActualizarDoente{}, fakeGerirEstado{}, fakeRegistarAlergia{}, fakeRegistarAntecedente{},
	)
	adhttp.RegistarDoentes(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}), adhttp.MFAObrigatoria())
	w := pedido(r, "GET", "/api/v1/doentes/id-inexistente", "Bearer xyz")
	if w.Code != nethttp.StatusNotFound {
		t.Fatalf("esperava 404, obtive %d", w.Code)
	}
}
