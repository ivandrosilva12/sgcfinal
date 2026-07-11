// Package keycloak integra com o Keycloak 25 via OIDC: discovery, cache de JWKS
// e validação de JWT RS256 (assinatura, issuer, expiração e audience). Camada 3
// — Adaptadores. Implementa a porta application/identidade.VerificadorToken.
package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Cliente valida tokens OIDC emitidos por um realm Keycloak.
type Cliente struct {
	verifier  *oidc.IDTokenVerifier
	audiencia string
	discovery string
	http      *nethttp.Client
}

// audiencia desserializa o claim "aud", que pode ser string ou lista de strings.
type claimAud []string

// UnmarshalJSON aceita tanto "aud": "x" como "aud": ["x","y"].
func (a *claimAud) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*a = claimAud{s}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	*a = arr
	return nil
}

// claims são os campos do token que o SGC consome.
type claims struct {
	Sub         string   `json:"sub"`
	Nome        string   `json:"name"`
	Email       string   `json:"email"`
	Azp         string   `json:"azp"`
	Aud         claimAud `json:"aud"`
	RealmAccess struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}

// Novo cria o cliente fazendo OIDC discovery no issuer. audiencia é o
// client/audience esperado (ex.: "sgc-api"). Devolve erro se o discovery falhar
// (Keycloak inacessível ou issuer errado) — chamado no arranque (composition root).
func Novo(ctx context.Context, issuer, audiencia string) (*Cliente, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discovery OIDC em %q: %w", issuer, err)
	}
	// A verificação de audience é feita manualmente (aud OU azp), porque o
	// client público do Keycloak coloca o client em azp e aud="account".
	verifier := provider.Verifier(&oidc.Config{SkipClientIDCheck: true})
	return &Cliente{
		verifier:  verifier,
		audiencia: audiencia,
		discovery: issuer + "/.well-known/openid-configuration",
		http:      &nethttp.Client{Timeout: 5 * time.Second},
	}, nil
}

// Verificar valida o token (assinatura RS256, issuer, expiração) e a audience,
// devolvendo a Sessao derivada dos claims. Qualquer falha devolve um ErroDominio
// de categoria NaoAutorizado (→ 401), sem revelar detalhes ao cliente.
func (c *Cliente) Verificar(ctx context.Context, tokenBruto string) (dominio.Sessao, error) {
	if tokenBruto == "" {
		return dominio.Sessao{}, erros.Novo(erros.CategoriaNaoAutorizado, "token em falta")
	}

	tok, err := c.verifier.Verify(ctx, tokenBruto)
	if err != nil {
		return dominio.Sessao{}, erros.Novo(erros.CategoriaNaoAutorizado, "token inválido")
	}

	var cl claims
	if err := tok.Claims(&cl); err != nil {
		return dominio.Sessao{}, erros.Novo(erros.CategoriaNaoAutorizado, "token inválido")
	}

	if !c.audienceValida(cl) {
		return dominio.Sessao{}, erros.Novo(erros.CategoriaNaoAutorizado, "audience inválida")
	}

	return dominio.Sessao{
		Sujeito: cl.Sub,
		Nome:    cl.Nome,
		Email:   cl.Email,
		Papeis:  dominio.PapeisDe(cl.RealmAccess.Roles),
	}, nil
}

// audienceValida aceita o token se a audience esperada constar de "aud" ou de
// "azp" (authorized party). Se nenhuma audience for configurada, não valida.
func (c *Cliente) audienceValida(cl claims) bool {
	if c.audiencia == "" {
		return true
	}
	if cl.Azp == c.audiencia {
		return true
	}
	for _, a := range cl.Aud {
		if a == c.audiencia {
			return true
		}
	}
	return false
}

// VerificarSaude confirma que o endpoint de discovery do Keycloak responde.
// Usado pelo /readyz (verificação de disponibilidade do JWKS/OIDC).
func (c *Cliente) VerificarSaude(ctx context.Context) error {
	// #nosec G107 -- o URL de discovery deriva da configuração de confiança
	// (KEYCLOAK_ISSUER), não de entrada externa; não há superfície de SSRF.
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, c.discovery, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("keycloak indisponível: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != nethttp.StatusOK {
		return fmt.Errorf("keycloak discovery devolveu %d", resp.StatusCode)
	}
	return nil
}
