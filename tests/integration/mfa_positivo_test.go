//go:build integration

package integration_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
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

// TestSpike_ClaimsMFA imprime acr/amr do token OTP — usado no spike para decidir
// o mecanismo. NÃO é um gate; remove-se/ajusta-se para asserção na Task 2.
func TestSpike_ClaimsMFA(t *testing.T) {
	issuer := issuerTeste()
	token := tokenDirectorComOTP(t, issuer)
	c := claimsDe(t, token)
	t.Logf("acr=%v amr=%v", c["acr"], c["amr"])
}
