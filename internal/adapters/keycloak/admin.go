package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// AdminCliente fala com a Admin REST API do Keycloak usando o service account de
// um client confidencial (client_credentials). Camada 3 — Adaptadores.
// Implementa application/identidade.AdminIdentidade.
type AdminCliente struct {
	base    string // ex.: http://localhost:8081
	realm   string // ex.: sgc
	id      string // client id confidencial (sgc-admin)
	segredo string
	http    *nethttp.Client
	agora   func() time.Time

	mu     sync.Mutex
	token  string
	expira time.Time
}

// NovoAdmin constrói o cliente derivando base e realm do issuer OIDC.
func NovoAdmin(issuer, clientID, clientSecret string) (*AdminCliente, error) {
	base, realm, ok := dividirIssuer(issuer)
	if !ok {
		return nil, fmt.Errorf("issuer inválido (esperado .../realms/<realm>): %q", issuer)
	}
	return &AdminCliente{
		base:    base,
		realm:   realm,
		id:      clientID,
		segredo: clientSecret,
		http:    &nethttp.Client{Timeout: 10 * time.Second},
		agora:   time.Now,
	}, nil
}

// dividirIssuer separa "http://host/realms/sgc" em base e realm.
func dividirIssuer(issuer string) (base, realm string, ok bool) {
	const marca = "/realms/"
	i := strings.LastIndex(issuer, marca)
	if i < 0 {
		return "", "", false
	}
	base = issuer[:i]
	realm = issuer[i+len(marca):]
	if base == "" || realm == "" || strings.Contains(realm, "/") {
		return "", "", false
	}
	return base, realm, true
}

// tokenServico obtém (e cacheia) um access token de serviço via client_credentials.
func (a *AdminCliente) tokenServico(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.token != "" && a.agora().Before(a.expira) {
		return a.token, nil
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {a.id},
		"client_secret": {a.segredo},
	}
	endpoint := a.base + "/realms/" + a.realm + "/protocol/openid-connect/token"
	// #nosec G107 -- endpoint deriva de KEYCLOAK_ISSUER (config de confiança).
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("token de serviço: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != nethttp.StatusOK {
		return "", fmt.Errorf("token de serviço devolveu %d", resp.StatusCode)
	}
	var corpo struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&corpo); err != nil {
		return "", err
	}
	a.token = corpo.AccessToken
	// Renovar 30s antes de expirar para evitar corridas com a expiração.
	a.expira = a.agora().Add(time.Duration(maxInt(corpo.ExpiresIn-30, 5)) * time.Second)
	return a.token, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// pedir executa um pedido autenticado à Admin API e descodifica a resposta em
// `saida` (se não-nil). Trata 404 como NaoEncontrado e outros ≥400 como interno.
func (a *AdminCliente) pedir(ctx context.Context, metodo, caminho string, corpo, saida any) error {
	tok, err := a.tokenServico(ctx)
	if err != nil {
		return err
	}
	var leitor *bytes.Reader
	if corpo != nil {
		b, err := json.Marshal(corpo)
		if err != nil {
			return err
		}
		leitor = bytes.NewReader(b)
	} else {
		leitor = bytes.NewReader(nil)
	}
	// #nosec G107 -- URL deriva de KEYCLOAK_ISSUER + id do recurso (config de confiança).
	req, err := nethttp.NewRequestWithContext(ctx, metodo, a.base+"/admin/realms/"+a.realm+caminho, leitor)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("keycloak admin: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == nethttp.StatusNotFound {
		return erros.Novo(erros.CategoriaNaoEncontrado, i18n.T(i18n.MsgUtilizadorNaoEncontrado))
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("keycloak admin devolveu %d em %s %s", resp.StatusCode, metodo, caminho)
	}
	if saida != nil {
		return json.NewDecoder(resp.Body).Decode(saida)
	}
	return nil
}

// --- Representações do Keycloak ---

type kcUser struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Enabled   bool   `json:"enabled"`
}

type kcRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func nomeCompleto(u kcUser) string {
	n := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if n == "" {
		return u.Username
	}
	return n
}

// ListarUtilizadores devolve utilizadores do realm (com os seus realm roles).
func (a *AdminCliente) ListarUtilizadores(ctx context.Context, f appident.FiltroUtilizadores) ([]appident.ResumoUtilizador, error) {
	q := url.Values{}
	if f.Termo != "" {
		q.Set("search", f.Termo)
	}
	limite := f.Limite
	if limite <= 0 {
		limite = 100
	}
	q.Set("max", strconv.Itoa(limite))
	q.Set("first", strconv.Itoa(f.Deslocamento))

	var users []kcUser
	if err := a.pedir(ctx, nethttp.MethodGet, "/users?"+q.Encode(), nil, &users); err != nil {
		return nil, err
	}
	out := make([]appident.ResumoUtilizador, 0, len(users))
	for _, u := range users {
		papeis, err := a.papeisDe(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, appident.ResumoUtilizador{
			ID: u.ID, Nome: nomeCompleto(u), Email: u.Email, Activo: u.Enabled, Papeis: papeis,
		})
	}
	return out, nil
}

// ObterUtilizador devolve o detalhe de um utilizador e os seus papéis.
func (a *AdminCliente) ObterUtilizador(ctx context.Context, id string) (appident.DetalheUtilizador, error) {
	var u kcUser
	if err := a.pedir(ctx, nethttp.MethodGet, "/users/"+url.PathEscape(id), nil, &u); err != nil {
		return appident.DetalheUtilizador{}, err
	}
	papeis, err := a.papeisDe(ctx, id)
	if err != nil {
		return appident.DetalheUtilizador{}, err
	}
	return appident.DetalheUtilizador{
		ID: u.ID, Nome: nomeCompleto(u), Email: u.Email, Activo: u.Enabled, Papeis: papeis,
	}, nil
}

// papeisDe lê os realm roles atribuídos, filtrando pelos papéis canónicos do SGC.
func (a *AdminCliente) papeisDe(ctx context.Context, id string) ([]string, error) {
	var roles []kcRole
	if err := a.pedir(ctx, nethttp.MethodGet, "/users/"+url.PathEscape(id)+"/role-mappings/realm", nil, &roles); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		if dominio.PapelValido(r.Name) {
			out = append(out, r.Name)
		}
	}
	return out, nil
}

// papelRepresentacao obtém a representação (id+name) de um realm role pelo nome.
func (a *AdminCliente) papelRepresentacao(ctx context.Context, papel dominio.Papel) (kcRole, error) {
	var r kcRole
	err := a.pedir(ctx, nethttp.MethodGet, "/roles/"+url.PathEscape(string(papel)), nil, &r)
	return r, err
}

// AtribuirPapel adiciona um realm role ao utilizador.
func (a *AdminCliente) AtribuirPapel(ctx context.Context, id string, papel dominio.Papel) error {
	r, err := a.papelRepresentacao(ctx, papel)
	if err != nil {
		return err
	}
	return a.pedir(ctx, nethttp.MethodPost, "/users/"+url.PathEscape(id)+"/role-mappings/realm", []kcRole{r}, nil)
}

// RevogarPapel remove um realm role do utilizador.
func (a *AdminCliente) RevogarPapel(ctx context.Context, id string, papel dominio.Papel) error {
	r, err := a.papelRepresentacao(ctx, papel)
	if err != nil {
		return err
	}
	return a.pedir(ctx, nethttp.MethodDelete, "/users/"+url.PathEscape(id)+"/role-mappings/realm", []kcRole{r}, nil)
}

// DefinirActivo activa/desactiva a conta (flag enabled do Keycloak).
func (a *AdminCliente) DefinirActivo(ctx context.Context, id string, activo bool) error {
	return a.pedir(ctx, nethttp.MethodPut, "/users/"+url.PathEscape(id), map[string]any{"enabled": activo}, nil)
}

// Garantia de conformidade com a porta.
var _ appident.AdminIdentidade = (*AdminCliente)(nil)
