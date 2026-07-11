//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/keycloak"
	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

// jsonDecode descodifica o corpo JSON de uma resposta HTTP para v.
func jsonDecode(resp *nethttp.Response, v any) error {
	return json.NewDecoder(resp.Body).Decode(v)
}

// apagarUtilizador remove um utilizador via Admin API (limpeza do teste).
func apagarUtilizador(t *testing.T, issuer, id string) {
	t.Helper()
	base, realm, _ := strings.Cut(issuer, "/realms/")
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"sgc-admin"},
		"client_secret": {"segredo-admin"},
	}
	// #nosec G107 -- issuer da config de teste.
	resp, err := nethttp.PostForm(issuer+"/protocol/openid-connect/token", form)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	var corpo struct {
		AccessToken string `json:"access_token"`
	}
	_ = jsonDecode(resp, &corpo)
	req, _ := nethttp.NewRequest(nethttp.MethodDelete, base+"/admin/realms/"+realm+"/users/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+corpo.AccessToken)
	if r, err := nethttp.DefaultClient.Do(req); err == nil {
		_ = r.Body.Close()
	}
}

func TestCriarUtilizador_ViaKeycloak(t *testing.T) {
	issuer := issuerTeste()
	admin, err := keycloak.NovoAdmin(issuer, "sgc-admin", "segredo-admin")
	if err != nil {
		t.Fatalf("NovoAdmin: %v", err)
	}
	ctx := context.Background()

	username := "novo.teste.sprint4"
	id, err := admin.CriarUtilizador(ctx, appident.DadosNovoUtilizador{
		Username: username, Nome: "Novo Teste", Email: "novo.teste.sprint4@sgc.ao",
		SenhaTemporaria: "Temp-1234", Papeis: []dominio.Papel{dominio.PapelMedico}, ConfigurarOTP: false,
	})
	if err != nil {
		t.Skipf("Admin API indisponível ou utilizador já existe: %v", err)
	}
	defer apagarUtilizador(t, issuer, id)

	det, err := admin.ObterUtilizador(ctx, id)
	if err != nil {
		t.Fatalf("obter utilizador criado: %v", err)
	}
	if det.Email != "novo.teste.sprint4@sgc.ao" {
		t.Fatalf("email inesperado: %q", det.Email)
	}
	temMedico := false
	for _, p := range det.Papeis {
		if p == "Medico" {
			temMedico = true
		}
	}
	if !temMedico {
		t.Fatalf("papel Medico não atribuído na criação: %v", det.Papeis)
	}
}
