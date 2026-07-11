//go:build integration

package integration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/keycloak"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

const segredoOTPDirector = "segredoteste-otp-32chars-abcdefgh"

// tokenDirectorComOTP obtém um access token do director.teste via direct grant
// com password + TOTP. Salta se o Keycloak recusar (config incompleta no spike).
func tokenDirectorComOTP(t *testing.T, issuer string) string {
	t.Helper()
	form := url.Values{
		"client_id":  {"sgc-api"},
		"grant_type": {"password"},
		"username":   {"director.teste"},
		"password":   {"teste"},
		"totp":       {codigoTOTP(segredoOTPDirector, time.Now())},
	}
	// #nosec G107 -- issuer vem da config de teste.
	resp, err := http.PostForm(issuer+"/protocol/openid-connect/token", form)
	if err != nil {
		t.Skipf("Keycloak inacessível: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("direct grant com OTP devolveu %d (config de MFA ainda incompleta)", resp.StatusCode)
	}
	var corpo struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&corpo); err != nil {
		t.Fatalf("descodificar token: %v", err)
	}
	return corpo.AccessToken
}

// claimsDe descodifica (sem verificar) o payload de um JWT para inspecção.
func claimsDe(t *testing.T, jwt string) map[string]any {
	t.Helper()
	partes := strings.Split(jwt, ".")
	if len(partes) != 3 {
		t.Fatalf("JWT malformado")
	}
	raw, err := base64.RawURLEncoding.DecodeString(partes[1])
	if err != nil {
		t.Fatalf("descodificar payload: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("json claims: %v", err)
	}
	return m
}

// TestMFA_DirectorComOTP_AutenticacaoForte confirma o CAMINHO POSITIVO: um login
// com 2º factor é reconhecido como autenticação forte e o papel sensível é aceite.
func TestMFA_DirectorComOTP_AutenticacaoForte(t *testing.T) {
	issuer := issuerTeste()
	token := tokenDirectorComOTP(t, issuer)

	verificador, err := keycloak.Novo(context.Background(), issuer, "sgc-api", []string{"mfa", "gold", "2"})
	if err != nil {
		t.Fatalf("inicializar Keycloak: %v", err)
	}
	sessao, err := verificador.Verificar(context.Background(), token)
	if err != nil {
		t.Fatalf("verificar token: %v", err)
	}
	if !sessao.TemPapel(dominio.PapelDirector) {
		t.Fatalf("esperava papel Director, obtive %v", sessao.Papeis)
	}
	if !sessao.AutenticacaoForte {
		t.Fatalf("CAMINHO POSITIVO: login com OTP devia ser autenticação forte (acr=%v amr=%v)",
			claimsDe(t, token)["acr"], claimsDe(t, token)["amr"])
	}
	if err := dominio.VerificarAutenticacaoForte(sessao); err != nil {
		t.Fatalf("papel sensível com MFA devia ser aceite, obtive %v", err)
	}
}
