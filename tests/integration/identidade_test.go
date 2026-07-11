//go:build integration

// Teste de integração da fatia vertical do BC Identidade (Sprint 2). Exige o
// compose a correr (PostgreSQL + Keycloak com o realm sgc). Corre com:
//
//	DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
//	KEYCLOAK_ISSUER=http://localhost:8081/realms/sgc \
//	go test -tags=integration ./tests/integration/...
package integration_test

import (
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/keycloak"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func issuerTeste() string {
	if v := os.Getenv("KEYCLOAK_ISSUER"); v != "" {
		return v
	}
	return "http://localhost:8081/realms/sgc"
}

// obterTokenPassword usa o direct access grant do client público sgc-api para
// obter um access token do utilizador de teste.
func obterTokenPassword(t *testing.T, issuer string) string {
	t.Helper()
	form := url.Values{
		"client_id":  {"sgc-api"},
		"grant_type": {"password"},
		"username":   {"medico.teste"},
		"password":   {"teste"},
	}
	// #nosec G107 -- issuer vem da configuração de teste, não de entrada externa.
	resp, err := nethttp.PostForm(issuer+"/protocol/openid-connect/token", form)
	if err != nil {
		t.Skipf("Keycloak inacessível em %s: %v", issuer, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("token endpoint devolveu %d", resp.StatusCode)
	}
	var corpo struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&corpo); err != nil {
		t.Fatalf("descodificar token: %v", err)
	}
	if corpo.AccessToken == "" {
		t.Fatal("access_token vazio")
	}
	return corpo.AccessToken
}

func TestIdentidade_FluxoAutenticadoEndToEnd(t *testing.T) {
	pool, ctx := ligar(t) // definido em migracoes_test.go; salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	issuer := issuerTeste()
	token := obterTokenPassword(t, issuer)

	verificador, err := keycloak.Novo(ctx, issuer, "sgc-api", []string{"mfa", "gold", "2"})
	if err != nil {
		t.Fatalf("inicializar Keycloak: %v", err)
	}

	// 1) Verificação real do token (assinatura RS256 + JWKS + issuer + audience).
	sessao, err := verificador.Verificar(ctx, token)
	if err != nil {
		t.Fatalf("verificar token real: %v", err)
	}
	if sessao.Sujeito == "" {
		t.Fatal("esperava sujeito (keycloak_id) na sessão")
	}
	temMedico := false
	for _, p := range sessao.Papeis {
		if p == "Medico" {
			temMedico = true
		}
	}
	if !temMedico {
		t.Fatalf("esperava papel Medico nos claims, obtive %v", sessao.Papeis)
	}

	// 2) Caso de uso: JIT provisioning + auditoria + perfil.
	repoU := pgrepo.NovoRepositorioUtilizadores(pool)
	repoA := pgrepo.NovoRepositorioAuditoria(pool)
	caso := appident.NovoCasoObterPerfil(repoU, repoA)

	perfil, err := caso.Executar(ctx, sessao)
	if err != nil {
		t.Fatalf("obter perfil: %v", err)
	}
	if perfil.KeycloakID != sessao.Sujeito {
		t.Fatalf("perfil.KeycloakID = %q; esperava %q", perfil.KeycloakID, sessao.Sujeito)
	}
	if !strings.Contains(strings.Join(perfil.Papeis, ","), "Medico") {
		t.Fatalf("perfil sem papel Medico: %v", perfil.Papeis)
	}

	// 3) JIT: utilizador foi persistido.
	var nome string
	if err := pool.QueryRow(ctx,
		`SELECT nome FROM identidade.utilizadores WHERE keycloak_id = $1`, sessao.Sujeito).Scan(&nome); err != nil {
		t.Fatalf("utilizador não foi provisionado (JIT): %v", err)
	}

	// 4) Auditoria de acesso foi registada (append-only).
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM auditoria.auditoria_eventos WHERE actor = $1 AND accao = 'identidade.perfil.consultado'`,
		sessao.Sujeito).Scan(&n); err != nil {
		t.Fatalf("contar auditoria: %v", err)
	}
	if n < 1 {
		t.Fatal("esperava pelo menos um evento de auditoria 'identidade.perfil.consultado'")
	}
}
