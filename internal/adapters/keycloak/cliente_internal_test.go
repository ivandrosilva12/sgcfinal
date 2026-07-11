package keycloak

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestClaimAud_Unmarshal(t *testing.T) {
	var s claimAud
	if err := json.Unmarshal([]byte(`"sgc-api"`), &s); err != nil || len(s) != 1 || s[0] != "sgc-api" {
		t.Fatalf("aud como string: %v (%v)", s, err)
	}
	var arr claimAud
	if err := json.Unmarshal([]byte(`["account","sgc-api"]`), &arr); err != nil || len(arr) != 2 {
		t.Fatalf("aud como lista: %v (%v)", arr, err)
	}
	var mau claimAud
	if err := json.Unmarshal([]byte(`123`), &mau); err == nil {
		t.Fatal("esperava erro para aud numérica")
	}
}

func TestAudienceValida(t *testing.T) {
	semAud := &Cliente{audiencia: ""}
	if !semAud.audienceValida(claims{}) {
		t.Error("sem audiência configurada deve aceitar")
	}
	cli := &Cliente{audiencia: "sgc-api"}
	if !cli.audienceValida(claims{Azp: "sgc-api"}) {
		t.Error("azp correspondente deve aceitar")
	}
	if !cli.audienceValida(claims{Aud: claimAud{"account", "sgc-api"}}) {
		t.Error("aud contendo a audiência deve aceitar")
	}
	if cli.audienceValida(claims{Azp: "outro", Aud: claimAud{"account"}}) {
		t.Error("sem correspondência deve recusar")
	}
}

func TestVerificar_TokenVazio(t *testing.T) {
	cli := &Cliente{audiencia: "sgc-api"}
	_, err := cli.Verificar(context.Background(), "")
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaNaoAutorizado {
		t.Fatalf("token vazio deve dar 401, obtive %v", err)
	}
}

func TestVerificarSaude(t *testing.T) {
	okSrv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		w.WriteHeader(nethttp.StatusOK)
	}))
	defer okSrv.Close()
	if err := (&Cliente{discovery: okSrv.URL, http: okSrv.Client()}).VerificarSaude(context.Background()); err != nil {
		t.Fatalf("esperava saúde OK, obtive %v", err)
	}

	badSrv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		w.WriteHeader(nethttp.StatusInternalServerError)
	}))
	defer badSrv.Close()
	if err := (&Cliente{discovery: badSrv.URL, http: badSrv.Client()}).VerificarSaude(context.Background()); err == nil {
		t.Fatal("esperava erro para 500")
	}

	if err := (&Cliente{discovery: "http://127.0.0.1:0/x", http: &nethttp.Client{}}).VerificarSaude(context.Background()); err == nil {
		t.Fatal("esperava erro para endpoint inacessível")
	}
}

func TestNovo_DiscoveryInvalido(t *testing.T) {
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		nethttp.NotFound(w, r)
	}))
	defer srv.Close()
	if _, err := Novo(context.Background(), srv.URL, "sgc-api"); err == nil {
		t.Fatal("esperava erro de discovery inválido")
	}
}

func TestNovo_DiscoveryValido_EVerificarTokenInvalido(t *testing.T) {
	var base string
	mux := nethttp.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 base,
			"authorization_endpoint": base + "/auth",
			"token_endpoint":         base + "/token",
			"jwks_uri":               base + "/certs",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL

	cli, err := Novo(context.Background(), base, "sgc-api")
	if err != nil {
		t.Fatalf("esperava discovery válido, obtive %v", err)
	}
	// Token malformado: exercita a via de falha do verifier (→ 401).
	if _, err := cli.Verificar(context.Background(), "token.invalido.xyz"); err == nil {
		t.Fatal("token inválido deve dar 401")
	}
}
