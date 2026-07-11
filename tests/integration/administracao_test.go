//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/url"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/keycloak"
	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

// tokenDe obtém um access token por password grant para o utilizador indicado.
func tokenDe(t *testing.T, issuer, utilizador, senha string) string {
	t.Helper()
	form := url.Values{
		"client_id":  {"sgc-api"},
		"grant_type": {"password"},
		"username":   {utilizador},
		"password":   {senha},
	}
	// #nosec G107 -- issuer vem da configuração de teste.
	resp, err := nethttp.PostForm(issuer+"/protocol/openid-connect/token", form)
	if err != nil {
		t.Skipf("Keycloak inacessível: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("token de %s devolveu %d", utilizador, resp.StatusCode)
	}
	var corpo struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&corpo); err != nil {
		t.Fatalf("descodificar token: %v", err)
	}
	return corpo.AccessToken
}

// TestMFA_AdminSemOTP_NaoTemAutenticacaoForte confirma que o token do Admin de
// teste (sem OTP) não comprova segundo factor, e que a regra de domínio o rejeita.
func TestMFA_AdminSemOTP_NaoTemAutenticacaoForte(t *testing.T) {
	issuer := issuerTeste()
	token := tokenDe(t, issuer, "admin.teste", "teste")

	verificador, err := keycloak.Novo(context.Background(), issuer, "sgc-api", []string{"mfa", "gold", "2"})
	if err != nil {
		t.Fatalf("inicializar Keycloak: %v", err)
	}
	sessao, err := verificador.Verificar(context.Background(), token)
	if err != nil {
		t.Fatalf("verificar token: %v", err)
	}
	if !sessao.TemPapel(dominio.PapelAdmin) {
		t.Fatalf("esperava papel Admin, obtive %v", sessao.Papeis)
	}
	if sessao.AutenticacaoForte {
		t.Fatal("Admin de teste não tem OTP; AutenticacaoForte devia ser false")
	}
	if err := dominio.VerificarAutenticacaoForte(sessao); err == nil {
		t.Fatal("esperava rejeição MFA para Admin sem segundo factor")
	}
}

// TestAdmin_AtribuirPapelViaKeycloak exercita o AdminCliente contra o Keycloak
// real: atribui um papel ao medico.teste e confirma na leitura.
func TestAdmin_AtribuirPapelViaKeycloak(t *testing.T) {
	issuer := issuerTeste()
	admin, err := keycloak.NovoAdmin(issuer, "sgc-admin", "segredo-admin")
	if err != nil {
		t.Fatalf("NovoAdmin: %v", err)
	}
	ctx := context.Background()

	lista, err := admin.ListarUtilizadores(ctx, appident.FiltroUtilizadores{Termo: "medico.teste"})
	if err != nil {
		t.Skipf("Admin API indisponível: %v", err)
	}
	if len(lista) == 0 {
		t.Fatal("esperava encontrar medico.teste")
	}
	id := lista[0].ID

	if err := admin.AtribuirPapel(ctx, id, dominio.PapelAdministrativo); err != nil {
		t.Fatalf("atribuir papel: %v", err)
	}
	det, err := admin.ObterUtilizador(ctx, id)
	if err != nil {
		t.Fatalf("obter utilizador: %v", err)
	}
	tem := false
	for _, p := range det.Papeis {
		if p == "Administrativo" {
			tem = true
		}
	}
	if !tem {
		t.Fatalf("papel Administrativo não atribuído: %v", det.Papeis)
	}

	// Limpeza: revogar para deixar o realm no estado inicial.
	if err := admin.RevogarPapel(ctx, id, dominio.PapelAdministrativo); err != nil {
		t.Fatalf("revogar papel: %v", err)
	}
}
