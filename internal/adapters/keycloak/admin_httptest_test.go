package keycloak

import (
	"context"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// registoFalso guarda, de forma concorrente-segura, os pedidos recebidos pelo
// servidor Keycloak falso (método+caminho+query) e os corpos JSON enviados,
// para que os testes possam verificar QUE pedidos reais foram feitos.
type registoFalso struct {
	mu      sync.Mutex
	pedidos []string
	corpos  map[string][]byte
}

func (r *registoFalso) registar(metodo, alvo string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pedidos = append(r.pedidos, metodo+" "+alvo)
}

func (r *registoFalso) guardarCorpo(chave string, corpo []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.corpos == nil {
		r.corpos = map[string][]byte{}
	}
	r.corpos[chave] = corpo
}

// contem indica se algum pedido registado contém a sub-cadeia dada.
func (r *registoFalso) contem(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.pedidos {
		if strings.Contains(p, sub) {
			return true
		}
	}
	return false
}

func (r *registoFalso) corpoDe(chave string) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.corpos[chave]
}

// servidorFalsoKeycloak arranca um httptest.Server que imita o subconjunto da
// Admin REST API do Keycloak usado por AdminCliente: emissão de token de
// serviço, listagem/consulta de utilizadores, papéis (realm roles) e
// activação/desactivação de contas. Devolve também o registo de pedidos para
// que os testes possam afirmar comportamento real (não apenas "não rebentou").
func servidorFalsoKeycloak(t *testing.T) (*httptest.Server, *registoFalso) {
	t.Helper()
	reg := &registoFalso{}

	utilizadores := map[string]kcUser{
		"u1": {ID: "u1", Username: "asilva", FirstName: "Ana", LastName: "Silva", Email: "ana.silva@exemplo.ao", Enabled: true},
	}
	papeisPorUtilizador := map[string][]kcRole{
		// u1 tem um papel canónico (Medico) e um role interno do Keycloak
		// (offline_access) que deve ser filtrado por papeisDe/PapelValido.
		"u1": {{ID: "role-medico", Name: "Medico"}, {ID: "role-offline", Name: "offline_access"}},
	}

	mux := nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		reg.registar(r.Method, r.URL.Path+"?"+r.URL.RawQuery)

		switch {
		// --- token de serviço (client_credentials) ---
		case r.Method == nethttp.MethodPost && r.URL.Path == "/realms/sgc/protocol/openid-connect/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "tok-teste",
				"expires_in":   300,
			})

		// --- listar utilizadores ---
		case r.Method == nethttp.MethodGet && r.URL.Path == "/admin/realms/sgc/users":
			lista := make([]kcUser, 0, len(utilizadores))
			for _, u := range utilizadores {
				lista = append(lista, u)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(lista)

		// --- papéis do utilizador (GET) ---
		case r.Method == nethttp.MethodGet && strings.HasSuffix(r.URL.Path, "/role-mappings/realm"):
			id := extrairIDUtilizador(r.URL.Path, "/role-mappings/realm")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(papeisPorUtilizador[id])

		// --- atribuir papel (POST) ---
		case r.Method == nethttp.MethodPost && strings.HasSuffix(r.URL.Path, "/role-mappings/realm"):
			id := extrairIDUtilizador(r.URL.Path, "/role-mappings/realm")
			corpo := lerCorpo(t, r)
			reg.guardarCorpo("POST role-mappings "+id, corpo)
			w.WriteHeader(nethttp.StatusNoContent)

		// --- revogar papel (DELETE) ---
		case r.Method == nethttp.MethodDelete && strings.HasSuffix(r.URL.Path, "/role-mappings/realm"):
			id := extrairIDUtilizador(r.URL.Path, "/role-mappings/realm")
			corpo := lerCorpo(t, r)
			reg.guardarCorpo("DELETE role-mappings "+id, corpo)
			w.WriteHeader(nethttp.StatusNoContent)

		// --- representação de um realm role pelo nome ---
		case r.Method == nethttp.MethodGet && strings.HasPrefix(r.URL.Path, "/admin/realms/sgc/roles/"):
			nome := strings.TrimPrefix(r.URL.Path, "/admin/realms/sgc/roles/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(kcRole{ID: "id-" + nome, Name: nome})

		// --- activar/desactivar (PUT) ---
		case r.Method == nethttp.MethodPut && strings.HasPrefix(r.URL.Path, "/admin/realms/sgc/users/"):
			id := strings.TrimPrefix(r.URL.Path, "/admin/realms/sgc/users/")
			corpo := lerCorpo(t, r)
			reg.guardarCorpo("PUT users "+id, corpo)
			w.WriteHeader(nethttp.StatusNoContent)

		// --- obter um utilizador ---
		case r.Method == nethttp.MethodGet && strings.HasPrefix(r.URL.Path, "/admin/realms/sgc/users/"):
			id := strings.TrimPrefix(r.URL.Path, "/admin/realms/sgc/users/")
			u, ok := utilizadores[id]
			if !ok {
				w.WriteHeader(nethttp.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(u)

		default:
			w.WriteHeader(nethttp.StatusNotFound)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, reg
}

// extrairIDUtilizador retira o {id} de um caminho "/admin/realms/sgc/users/{id}<sufixo>".
func extrairIDUtilizador(caminho, sufixo string) string {
	const prefixo = "/admin/realms/sgc/users/"
	meio := strings.TrimPrefix(caminho, prefixo)
	return strings.TrimSuffix(meio, sufixo)
}

func lerCorpo(t *testing.T, r *nethttp.Request) []byte {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("erro inesperado a ler corpo do pedido: %v", err)
	}
	return b
}

// novoAdminDeTeste constrói um AdminCliente apontado ao servidor falso,
// derivando base/realm do issuer tal como NovoAdmin faz em produção.
func novoAdminDeTeste(t *testing.T, srv *httptest.Server) *AdminCliente {
	t.Helper()
	admin, err := NovoAdmin(srv.URL+"/realms/sgc", "sgc-admin", "segredo-teste")
	if err != nil {
		t.Fatalf("NovoAdmin: erro inesperado: %v", err)
	}
	return admin
}

func TestAdminCliente_ListarUtilizadores_FiltraPapeisCanonicos(t *testing.T) {
	srv, reg := servidorFalsoKeycloak(t)
	admin := novoAdminDeTeste(t, srv)

	out, err := admin.ListarUtilizadores(context.Background(), appident.FiltroUtilizadores{Termo: "Silva"})
	if err != nil {
		t.Fatalf("ListarUtilizadores: erro inesperado: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("esperava 1 utilizador, obtive %d", len(out))
	}
	u := out[0]
	if u.Nome != "Ana Silva" {
		t.Errorf("Nome = %q; quer %q", u.Nome, "Ana Silva")
	}
	if u.Email != "ana.silva@exemplo.ao" {
		t.Errorf("Email = %q; quer %q", u.Email, "ana.silva@exemplo.ao")
	}
	if !u.Activo {
		t.Error("Activo = false; quer true (enabled=true no Keycloak)")
	}
	// offline_access não é um papel canónico do SGC e deve ser filtrado;
	// Medico é canónico e deve sobreviver.
	achouMedico := false
	for _, p := range u.Papeis {
		if p == "offline_access" {
			t.Errorf("papel interno %q não devia sobreviver ao filtro PapelValido", p)
		}
		if p == "Medico" {
			achouMedico = true
		}
	}
	if !achouMedico {
		t.Errorf("esperava o papel canónico Medico em %v", u.Papeis)
	}
	// O termo de pesquisa deve ter sido propagado ao pedido GET /users.
	if !reg.contem("search=Silva") {
		t.Errorf("esperava que o pedido de listagem incluísse search=Silva; pedidos: %v", reg.pedidos)
	}
}

func TestAdminCliente_ObterUtilizador_ComPapeis(t *testing.T) {
	srv, _ := servidorFalsoKeycloak(t)
	admin := novoAdminDeTeste(t, srv)

	det, err := admin.ObterUtilizador(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ObterUtilizador: erro inesperado: %v", err)
	}
	if det.ID != "u1" || det.Nome != "Ana Silva" || det.Email != "ana.silva@exemplo.ao" {
		t.Fatalf("detalhe inesperado: %+v", det)
	}
	if len(det.Papeis) != 1 || det.Papeis[0] != "Medico" {
		t.Fatalf("Papeis = %v; quer apenas [Medico]", det.Papeis)
	}
}

func TestAdminCliente_AtribuirPapel_ConsultaERoleEAtribui(t *testing.T) {
	srv, reg := servidorFalsoKeycloak(t)
	admin := novoAdminDeTeste(t, srv)

	if err := admin.AtribuirPapel(context.Background(), "u1", dominio.PapelAdministrativo); err != nil {
		t.Fatalf("AtribuirPapel: erro inesperado: %v", err)
	}
	if !reg.contem("GET /admin/realms/sgc/roles/Administrativo") {
		t.Errorf("esperava consulta GET ao role Administrativo; pedidos: %v", reg.pedidos)
	}
	if !reg.contem("POST /admin/realms/sgc/users/u1/role-mappings/realm") {
		t.Errorf("esperava POST de atribuição de papel; pedidos: %v", reg.pedidos)
	}
	corpo := reg.corpoDe("POST role-mappings u1")
	var papeisEnviados []kcRole
	if err := json.Unmarshal(corpo, &papeisEnviados); err != nil {
		t.Fatalf("corpo do POST não é JSON válido: %v (%s)", err, corpo)
	}
	if len(papeisEnviados) != 1 || papeisEnviados[0].Name != "Administrativo" {
		t.Fatalf("corpo do POST = %v; quer role Administrativo", papeisEnviados)
	}
}

func TestAdminCliente_RevogarPapel_EnviaDelete(t *testing.T) {
	srv, reg := servidorFalsoKeycloak(t)
	admin := novoAdminDeTeste(t, srv)

	if err := admin.RevogarPapel(context.Background(), "u1", dominio.PapelMedico); err != nil {
		t.Fatalf("RevogarPapel: erro inesperado: %v", err)
	}
	if !reg.contem("DELETE /admin/realms/sgc/users/u1/role-mappings/realm") {
		t.Errorf("esperava DELETE de revogação de papel; pedidos: %v", reg.pedidos)
	}
	corpo := reg.corpoDe("DELETE role-mappings u1")
	var papeisEnviados []kcRole
	if err := json.Unmarshal(corpo, &papeisEnviados); err != nil {
		t.Fatalf("corpo do DELETE não é JSON válido: %v (%s)", err, corpo)
	}
	if len(papeisEnviados) != 1 || papeisEnviados[0].Name != "Medico" {
		t.Fatalf("corpo do DELETE = %v; quer role Medico", papeisEnviados)
	}
}

func TestAdminCliente_DefinirActivo_EnviaEnabledFalse(t *testing.T) {
	srv, reg := servidorFalsoKeycloak(t)
	admin := novoAdminDeTeste(t, srv)

	if err := admin.DefinirActivo(context.Background(), "u1", false); err != nil {
		t.Fatalf("DefinirActivo: erro inesperado: %v", err)
	}
	if !reg.contem("PUT /admin/realms/sgc/users/u1") {
		t.Errorf("esperava PUT ao utilizador u1; pedidos: %v", reg.pedidos)
	}
	corpo := reg.corpoDe("PUT users u1")
	var recebido struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(corpo, &recebido); err != nil {
		t.Fatalf("corpo do PUT não é JSON válido: %v (%s)", err, corpo)
	}
	if recebido.Enabled {
		t.Errorf("corpo do PUT tem enabled=true; quer enabled=false")
	}
}

func TestAdminCliente_ObterUtilizador_NaoEncontrado(t *testing.T) {
	srv, _ := servidorFalsoKeycloak(t)
	admin := novoAdminDeTeste(t, srv)

	_, err := admin.ObterUtilizador(context.Background(), "desconhecido")
	if err == nil {
		t.Fatal("esperava erro para utilizador desconhecido")
	}
	if cat := erros.CategoriaDe(err); cat != erros.CategoriaNaoEncontrado {
		t.Fatalf("categoria do erro = %v; quer CategoriaNaoEncontrado", cat)
	}
}
