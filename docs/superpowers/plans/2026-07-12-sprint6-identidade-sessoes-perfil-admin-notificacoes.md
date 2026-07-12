# Sprint 6 — Sessões activas, edição admin de perfil e notificações — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fechar os loose-ends do BC Identidade — gestão de sessões activas (listar/revogar granular), edição administrativa do perfil (telefone/BI) de outros utilizadores, e notificações por email/SMTP best-effort com fallback no-op.

**Architecture:** Go/Gin, DDD + Clean Architecture (Domínio → Aplicação → Adaptadores → Plataforma, dependência para dentro). Keycloak é a fonte de verdade de utilizadores/papéis/sessões via Admin REST API; a BD local guarda o perfil operacional (telefone/BI). O email é um efeito lateral best-effort (falha nunca falha a operação). Reutiliza o padrão do Sprint 5 (`AtualizarContacto`, casos de uso de reset, VOs Angola).

**Tech Stack:** Go 1.22+, Gin, `net/smtp` (stdlib — sem dependências novas), Keycloak 25 Admin API, PostgreSQL (pgx), MailHog (dev). Testes: `net/http/httptest`, fakes.

## Global Constraints

- **Todo o output em PT-PT angolano** — código, comentários, mensagens de erro, JSON, commits. Nunca inglês nem PT-BR.
- **Domínio (`internal/domain/**`) não importa infra** (pgx/gin/net/http/oidc/smtp). i18n/erros/identity/evento/auditoria são Shared Kernel e são importáveis.
- **Sem `panic()`** fora de inicialização.
- **Erros como RFC 7807** (problem+json), via `erros.Novo(categoria, msg)` + `responderErro`.
- **Sem dependências novas**: adaptador de email usa `net/smtp` da stdlib. `net/smtp` é stdlib (não é vendor) e o pacote fica em `internal/adapters/**` — **o `.go-arch-lint.yml` não precisa de alterações**.
- **Email best-effort**: falha de envio (ou de obtenção do email do alvo) regista `slog.Warn` e **não** falha a operação; a senha temporária **mantém-se na resposta HTTP**. **Nunca registar a senha nos logs** — só id/email + erro.
- **Keycloak é a fonte de verdade**; a BD local só guarda telefone/BI.
- **Conventional Commits em PT-PT**, cada commit termina com:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- **Cobertura (gate agregado por camada)**: domínio ≥85%, aplicação ≥75%, adaptadores ≥60% (`bash scripts/cobertura.sh`).
- **Branch**: `m1-sprint6-identidade` (já criado; a spec já está commitada nele).
- **Nota de build oscilante**: adicionar métodos a `AdminIdentidade` e trocar assinaturas de construtores parte e reconstrói implementadores/chamadores ao longo das tasks. Cada task indica o **âmbito de verificação** (que pacote testar). O módulo inteiro (`go build ./...`) só volta a compilar na **Task 8** (composition root). Isto é esperado e igual aos Sprints 3–5.

---

### Task 1: Domínio — evento `SessaoRevogada`

**Files:**
- Modify: `internal/domain/identidade/eventos.go`
- Test: `internal/domain/identidade/eventos_sessao_test.go` (criar)

**Interfaces:**
- Consumes: `evento.EventoDominio` (interface do Shared Kernel: `NomeEvento() string`, `OcorridoEm() time.Time`).
- Produces: `identidade.SessaoRevogada{Actor, SessionID string, Em time.Time}` com `NomeEvento() == "identidade.sessao.revogada"`.

Nota: já existe `SessoesRevogadas{Actor, Alvo, Em}` (plural, para o logout total ao desactivar). Este é o singular, granular (revogar UMA sessão por sessionId). Nomes distintos — não há colisão.

- [ ] **Step 1: Escrever o teste que falha**

Criar `internal/domain/identidade/eventos_sessao_test.go`:

```go
package identidade

import (
	"testing"
	"time"
)

func TestSessaoRevogada_Evento(t *testing.T) {
	em := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	e := SessaoRevogada{Actor: "admin-1", SessionID: "sess-9", Em: em}

	if e.NomeEvento() != "identidade.sessao.revogada" {
		t.Fatalf("NomeEvento = %q; quer identidade.sessao.revogada", e.NomeEvento())
	}
	if !e.OcorridoEm().Equal(em) {
		t.Fatalf("OcorridoEm = %v; quer %v", e.OcorridoEm(), em)
	}
}
```

- [ ] **Step 2: Correr o teste e confirmar que falha**

Run: `go test ./internal/domain/identidade/ -run TestSessaoRevogada_Evento`
Expected: FAIL (compilação: `SessaoRevogada` indefinido).

- [ ] **Step 3: Implementar o evento**

Em `internal/domain/identidade/eventos.go`, adicionar (antes do bloco `var (...)` de conformidade, junto dos outros eventos):

```go
// SessaoRevogada é emitido quando um administrador revoga uma sessão específica.
type SessaoRevogada struct {
	Actor     string
	SessionID string
	Em        time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e SessaoRevogada) NomeEvento() string { return "identidade.sessao.revogada" }

// OcorridoEm implementa evento.EventoDominio.
func (e SessaoRevogada) OcorridoEm() time.Time { return e.Em }
```

E acrescentar a linha de conformidade dentro do bloco `var (...)` existente (a seguir a `_ evento.EventoDominio = PerfilActualizado{}`):

```go
	_ evento.EventoDominio = SessaoRevogada{}
```

- [ ] **Step 4: Correr o teste e confirmar que passa**

Run: `go test ./internal/domain/identidade/ -run TestSessaoRevogada_Evento`
Expected: PASS

- [ ] **Step 5: Confirmar o domínio inteiro verde**

Run: `go test ./internal/domain/...`
Expected: PASS (ok todos os pacotes de domínio)

- [ ] **Step 6: Commit**

```bash
git add internal/domain/identidade/eventos.go internal/domain/identidade/eventos_sessao_test.go
git commit -m "$(cat <<'EOF'
feat(identidade): evento de domínio SessaoRevogada (revogação granular)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Porta `AdminIdentidade` + adaptador Keycloak (sessões)

**Files:**
- Modify: `internal/application/identidade/ports.go`
- Modify: `internal/adapters/keycloak/admin.go`
- Modify: `internal/application/identidade/gerir_utilizadores_test.go` (fake `fakeAdmin` — acrescentar os 2 métodos)
- Modify: `internal/application/identidade/criar_utilizador_test.go` (fake `fakeCriador` — acrescentar os 2 métodos)
- Test: `internal/adapters/keycloak/admin_httptest_test.go` (acrescentar 2 testes)

**Interfaces:**
- Consumes: helper privado `(*AdminCliente).pedir(ctx, metodo, caminho, corpo, saida)` (prefixa `/admin/realms/{realm}`; 404 → `NaoEncontrado`).
- Produces:
  - DTO `appident.SessaoActiva{ID, IP string; Inicio, UltimoAcesso time.Time; Clientes []string}` (JSON: `id`, `ip`, `inicio`, `ultimo_acesso`, `clientes`).
  - `AdminIdentidade.ListarSessoes(ctx, userID string) ([]SessaoActiva, error)`
  - `AdminIdentidade.RevogarSessao(ctx, sessionID string) error`

- [ ] **Step 1: Escrever os testes de adaptador que falham**

Em `internal/adapters/keycloak/admin_httptest_test.go`, acrescentar ao fim do ficheiro:

```go
func TestListarSessoes_MapeiaJSON(t *testing.T) {
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
		case r.Method == nethttp.MethodGet && strings.HasSuffix(r.URL.Path, "/users/u1/sessions"):
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": "sess-1", "ipAddress": "10.0.0.5",
					"start": int64(1720000000000), "lastAccess": int64(1720000600000),
					"clients": map[string]string{"sgc-api": "sgc-api"},
				},
			})
		default:
			w.WriteHeader(nethttp.StatusNotFound)
		}
	}))
	defer srv.Close()
	admin, _ := NovoAdmin(srv.URL+"/realms/sgc", "sgc-admin", "segredo")

	out, err := admin.ListarSessoes(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListarSessoes: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("esperava 1 sessão, obtive %d", len(out))
	}
	s := out[0]
	if s.ID != "sess-1" || s.IP != "10.0.0.5" {
		t.Fatalf("sessão mapeada errada: %+v", s)
	}
	if s.Inicio.UnixMilli() != 1720000000000 || s.UltimoAcesso.UnixMilli() != 1720000600000 {
		t.Fatalf("tempos mapeados errados: %+v", s)
	}
	if len(s.Clientes) != 1 || s.Clientes[0] != "sgc-api" {
		t.Fatalf("clientes = %v; quer [sgc-api]", s.Clientes)
	}
}

func TestRevogarSessao_DELETE(t *testing.T) {
	var revogou bool
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
		case r.Method == nethttp.MethodDelete && r.URL.Path == "/admin/realms/sgc/sessions/sess-1":
			revogou = true
			w.WriteHeader(nethttp.StatusNoContent)
		default:
			w.WriteHeader(nethttp.StatusNotFound)
		}
	}))
	defer srv.Close()
	admin, _ := NovoAdmin(srv.URL+"/realms/sgc", "sgc-admin", "segredo")

	if err := admin.RevogarSessao(context.Background(), "sess-1"); err != nil {
		t.Fatalf("RevogarSessao: %v", err)
	}
	if !revogou {
		t.Fatal("esperava DELETE /sessions/sess-1")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha (compilação)**

Run: `go test ./internal/adapters/keycloak/ -run 'TestListarSessoes_MapeiaJSON|TestRevogarSessao_DELETE'`
Expected: FAIL (métodos `ListarSessoes`/`RevogarSessao` e tipo `SessaoActiva` indefinidos).

- [ ] **Step 3: Adicionar o DTO e os métodos à porta**

Em `internal/application/identidade/ports.go`, adicionar o `import "time"` (o ficheiro só importa `context` e os pacotes de domínio — acrescentar `"time"` ao bloco de imports) e o DTO logo antes da interface `AdminIdentidade`:

```go
// SessaoActiva é o read-model de uma sessão Keycloak activa de um utilizador.
type SessaoActiva struct {
	ID           string    `json:"id"`
	IP           string    `json:"ip"`
	Inicio       time.Time `json:"inicio"`
	UltimoAcesso time.Time `json:"ultimo_acesso"`
	Clientes     []string  `json:"clientes"`
}
```

E acrescentar os 2 métodos à interface `AdminIdentidade` (a seguir a `ApagarUtilizador`):

```go
	ListarSessoes(ctx context.Context, userID string) ([]SessaoActiva, error)
	RevogarSessao(ctx context.Context, sessionID string) error
```

- [ ] **Step 4: Implementar no adaptador Keycloak**

Em `internal/adapters/keycloak/admin.go`, acrescentar `"sort"` e `"time"` ao bloco de imports (já importa `strconv`, `strings`, `sync`, `time`; confirmar que `time` está presente — está — e acrescentar `"sort"`). Depois adicionar, a seguir a `ApagarUtilizador`:

```go
// kcSession é a representação de uma sessão de utilizador na Admin API.
type kcSession struct {
	ID         string            `json:"id"`
	IPAddress  string            `json:"ipAddress"`
	Start      int64             `json:"start"`
	LastAccess int64             `json:"lastAccess"`
	Clients    map[string]string `json:"clients"`
}

// ListarSessoes devolve as sessões activas do utilizador.
func (a *AdminCliente) ListarSessoes(ctx context.Context, userID string) ([]appident.SessaoActiva, error) {
	var sessoes []kcSession
	if err := a.pedir(ctx, nethttp.MethodGet, "/users/"+url.PathEscape(userID)+"/sessions", nil, &sessoes); err != nil {
		return nil, err
	}
	out := make([]appident.SessaoActiva, 0, len(sessoes))
	for _, s := range sessoes {
		clientes := make([]string, 0, len(s.Clients))
		for _, nome := range s.Clients {
			clientes = append(clientes, nome)
		}
		sort.Strings(clientes) // ordem determinística
		out = append(out, appident.SessaoActiva{
			ID:           s.ID,
			IP:           s.IPAddress,
			Inicio:       time.UnixMilli(s.Start).UTC(),
			UltimoAcesso: time.UnixMilli(s.LastAccess).UTC(),
			Clientes:     clientes,
		})
	}
	return out, nil
}

// RevogarSessao termina uma sessão específica pelo seu sessionId.
func (a *AdminCliente) RevogarSessao(ctx context.Context, sessionID string) error {
	return a.pedir(ctx, nethttp.MethodDelete, "/sessions/"+url.PathEscape(sessionID), nil, nil)
}
```

- [ ] **Step 5: Actualizar os fakes do pacote de teste da aplicação**

Em `internal/application/identidade/gerir_utilizadores_test.go`, acrescentar campos de captura ao `fakeAdmin` (dentro do struct, junto de `apagados`):

```go
	sessoesPorUtilizador map[string][]appident.SessaoActiva
	sessoesRevogadas1    []string
```

E os 2 métodos (a seguir a `ApagarUtilizador`):

```go
func (f *fakeAdmin) ListarSessoes(_ context.Context, userID string) ([]appident.SessaoActiva, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sessoesPorUtilizador[userID], nil
}
func (f *fakeAdmin) RevogarSessao(_ context.Context, sessionID string) error {
	if f.err != nil {
		return f.err
	}
	f.sessoesRevogadas1 = append(f.sessoesRevogadas1, sessionID)
	return nil
}
```

Em `internal/application/identidade/criar_utilizador_test.go`, acrescentar ao `fakeCriador` (a seguir a `ApagarUtilizador`):

```go
func (f *fakeCriador) ListarSessoes(context.Context, string) ([]appident.SessaoActiva, error) {
	return nil, nil
}
func (f *fakeCriador) RevogarSessao(context.Context, string) error { return nil }
```

- [ ] **Step 6: Correr os testes do adaptador e do pacote de aplicação**

Run: `go test ./internal/adapters/keycloak/ ./internal/application/identidade/`
Expected: PASS (ambos os pacotes compilam e passam; a conformidade `var _ AdminIdentidade = (*AdminCliente)(nil)` continua válida).

- [ ] **Step 7: Commit**

```bash
git add internal/application/identidade/ports.go internal/adapters/keycloak/admin.go \
  internal/adapters/keycloak/admin_httptest_test.go \
  internal/application/identidade/gerir_utilizadores_test.go \
  internal/application/identidade/criar_utilizador_test.go
git commit -m "$(cat <<'EOF'
feat(identidade): porta e adaptador Keycloak para listar/revogar sessões

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Aplicação — casos de uso de sessões

**Files:**
- Create: `internal/application/identidade/sessoes.go`
- Test: `internal/application/identidade/sessoes_test.go`

**Interfaces:**
- Consumes: `AdminIdentidade.ListarSessoes`, `AdminIdentidade.RevogarSessao`, `Auditor.Registar`, `SessaoActiva`.
- Produces:
  - `CasoListarSessoes` com `NovoCasoListarSessoes(a AdminIdentidade) *CasoListarSessoes` e `Executar(ctx, userID string) ([]SessaoActiva, error)`.
  - `CasoRevogarSessao` com `NovoCasoRevogarSessao(a AdminIdentidade, aud Auditor) *CasoRevogarSessao` e `Executar(ctx, actor, sessionID string) error`.

- [ ] **Step 1: Escrever os testes que falham**

Criar `internal/application/identidade/sessoes_test.go`:

```go
package identidade_test

import (
	"context"
	"errors"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
)

func TestListarSessoes_Delega(t *testing.T) {
	admin := &fakeAdmin{sessoesPorUtilizador: map[string][]appident.SessaoActiva{
		"u1": {{ID: "sess-1", IP: "10.0.0.5"}},
	}}
	caso := appident.NovoCasoListarSessoes(admin)

	out, err := caso.Executar(context.Background(), "u1")
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(out) != 1 || out[0].ID != "sess-1" {
		t.Fatalf("sessões inesperadas: %v", out)
	}
}

func TestRevogarSessao_Audita(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoRevogarSessao(admin, aud)

	if err := caso.Executar(context.Background(), "actor-1", "sess-1"); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(admin.sessoesRevogadas1) != 1 || admin.sessoesRevogadas1[0] != "sess-1" {
		t.Fatalf("revogação não delegada: %v", admin.sessoesRevogadas1)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.sessao.revogada" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
	if aud.registos[0].Actor != "actor-1" || aud.registos[0].EntidadeID != "sess-1" {
		t.Fatalf("auditoria com dados errados: %+v", aud.registos[0])
	}
}

func TestRevogarSessao_PropagaErro(t *testing.T) {
	admin := &fakeAdmin{err: errors.New("kc down")}
	caso := appident.NovoCasoRevogarSessao(admin, &fakeAuditor{})
	if err := caso.Executar(context.Background(), "actor-1", "sess-1"); err == nil {
		t.Fatal("esperava erro propagado")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/identidade/ -run 'Sessoes|Sessao'`
Expected: FAIL (construtores indefinidos).

- [ ] **Step 3: Implementar os casos de uso**

Criar `internal/application/identidade/sessoes.go`:

```go
package identidade

import (
	"context"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoListarSessoes lista as sessões activas de um utilizador (leitura).
type CasoListarSessoes struct{ admin AdminIdentidade }

// NovoCasoListarSessoes constrói o caso de uso.
func NovoCasoListarSessoes(a AdminIdentidade) *CasoListarSessoes {
	return &CasoListarSessoes{admin: a}
}

// Executar devolve as sessões activas do utilizador indicado.
func (c *CasoListarSessoes) Executar(ctx context.Context, userID string) ([]SessaoActiva, error) {
	return c.admin.ListarSessoes(ctx, userID)
}

// CasoRevogarSessao revoga uma sessão específica e audita a acção.
type CasoRevogarSessao struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoRevogarSessao constrói o caso de uso.
func NovoCasoRevogarSessao(a AdminIdentidade, aud Auditor) *CasoRevogarSessao {
	return &CasoRevogarSessao{admin: a, auditor: aud, agora: time.Now}
}

// Executar revoga a sessão no Keycloak e regista a auditoria.
func (c *CasoRevogarSessao) Executar(ctx context.Context, actor, sessionID string) error {
	if err := c.admin.RevogarSessao(ctx, sessionID); err != nil {
		return err
	}
	return c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.sessao.revogada",
		Entidade:   "sessao",
		EntidadeID: sessionID,
		OcorridoEm: c.agora(),
	})
}
```

- [ ] **Step 4: Correr e confirmar que passa**

Run: `go test ./internal/application/identidade/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/identidade/sessoes.go internal/application/identidade/sessoes_test.go
git commit -m "$(cat <<'EOF'
feat(identidade): casos de uso para listar e revogar sessões

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Aplicação — `CasoEditarPerfilAdmin`

**Files:**
- Create: `internal/application/identidade/editar_perfil_admin.go`
- Test: `internal/application/identidade/editar_perfil_admin_test.go`

**Interfaces:**
- Consumes: `AdminIdentidade.ObterUtilizador`, `dominio.RepositorioUtilizadores` (`ObterPorID`, `GuardarComPapeis`, `AtualizarContacto`), `Auditor`, `dominio.NovoUtilizador`, `dominio.PapeisDe`, `(*Utilizador).AtualizarContacto`, `erros.CategoriaDe`, `paraPerfil` (já em `obter_perfil.go`, mesmo pacote).
- Produces: `CasoEditarPerfilAdmin` com `NovoCasoEditarPerfilAdmin(a AdminIdentidade, r dominio.RepositorioUtilizadores, aud Auditor) *CasoEditarPerfilAdmin` e `Executar(ctx, actor, id string, telefone, bi *string) (Perfil, error)`.

Semântica dos ponteiros (igual ao self-service do Sprint 5): `telefone`/`bi` a `nil` preservam o valor actual; string vazia limpa; valor presente é validado pelos VOs Angola. Se a linha local não existir, hidrata a partir do Keycloak (`ObterUtilizador`) antes de aplicar.

- [ ] **Step 1: Escrever os testes que falham**

Criar `internal/application/identidade/editar_perfil_admin_test.go`:

```go
package identidade_test

import (
	"context"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// fakeRepoPerfil modela a presença (ou ausência) da linha local, para exercitar
// a hidratação JIT a partir do Keycloak.
type fakeRepoPerfil struct {
	existe   bool
	u        *dominio.Utilizador
	guardou  bool
	telefone string
	bi       string
}

func (f *fakeRepoPerfil) ObterPorID(_ context.Context, _ string) (*dominio.Utilizador, error) {
	if !f.existe {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, i18n.T(i18n.MsgUtilizadorNaoEncontrado))
	}
	return f.u, nil
}
func (f *fakeRepoPerfil) GuardarComPapeis(_ context.Context, u *dominio.Utilizador) error {
	f.existe = true
	f.u = u
	f.guardou = true
	return nil
}
func (f *fakeRepoPerfil) AtualizarContacto(_ context.Context, _, telefone, bi string) error {
	f.telefone, f.bi = telefone, bi
	if f.u != nil {
		f.u.Telefone, f.u.BI = telefone, bi
	}
	return nil
}

func TestEditarPerfilAdmin_LinhaExistente(t *testing.T) {
	base, _ := dominio.NovoUtilizador("u1", "Ana Silva", "ana@sgc.ao", "", "", []dominio.Papel{dominio.PapelMedico})
	repo := &fakeRepoPerfil{existe: true, u: base}
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoEditarPerfilAdmin(admin, repo, aud)

	// O agregado normaliza para o formato de apresentação "+244 9XX XXX XXX".
	tel := "+244 923 456 789"
	perfil, err := caso.Executar(context.Background(), "admin-1", "u1", &tel, nil)
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if perfil.Telefone != "+244 923 456 789" {
		t.Fatalf("telefone não persistido: %+v", perfil)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.perfil.actualizado" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
	if aud.registos[0].Actor != "admin-1" || aud.registos[0].EntidadeID != "u1" {
		t.Fatalf("auditoria com dados errados: %+v", aud.registos[0])
	}
}

func TestEditarPerfilAdmin_HidrataDoKeycloak(t *testing.T) {
	repo := &fakeRepoPerfil{existe: false} // sem linha local
	admin := &fakeAdmin{detalhe: appident.DetalheUtilizador{
		ID: "u2", Nome: "Rui Mendes", Email: "rui@sgc.ao", Papeis: []string{"Enfermeiro"},
	}}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoEditarPerfilAdmin(admin, repo, aud)

	tel := "+244 912 000 000"
	perfil, err := caso.Executar(context.Background(), "admin-1", "u2", &tel, nil)
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if !repo.guardou {
		t.Fatal("esperava hidratação (GuardarComPapeis) da linha local a partir do Keycloak")
	}
	if perfil.Nome != "Rui Mendes" || perfil.Telefone != "+244 912 000 000" {
		t.Fatalf("perfil inesperado: %+v", perfil)
	}
}

func TestEditarPerfilAdmin_TelefoneInvalido(t *testing.T) {
	base, _ := dominio.NovoUtilizador("u1", "Ana", "ana@sgc.ao", "", "", nil)
	repo := &fakeRepoPerfil{existe: true, u: base}
	caso := appident.NovoCasoEditarPerfilAdmin(&fakeAdmin{}, repo, &fakeAuditor{})

	mau := "não-é-telefone"
	_, err := caso.Executar(context.Background(), "admin-1", "u1", &mau, nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/identidade/ -run EditarPerfilAdmin`
Expected: FAIL (`NovoCasoEditarPerfilAdmin` indefinido).

- [ ] **Step 3: Implementar o caso de uso**

Criar `internal/application/identidade/editar_perfil_admin.go`:

```go
package identidade

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoEditarPerfilAdmin permite a um administrador actualizar o perfil local
// (telefone/BI) de outro utilizador. Garante a linha local — hidratando-a a
// partir do Keycloak se ainda não existir — aplica a validação de domínio,
// persiste, audita e devolve o perfil actualizado.
type CasoEditarPerfilAdmin struct {
	admin        AdminIdentidade
	utilizadores dominio.RepositorioUtilizadores
	auditor      Auditor
	agora        func() time.Time
}

// NovoCasoEditarPerfilAdmin constrói o caso de uso.
func NovoCasoEditarPerfilAdmin(a AdminIdentidade, r dominio.RepositorioUtilizadores, aud Auditor) *CasoEditarPerfilAdmin {
	return &CasoEditarPerfilAdmin{admin: a, utilizadores: r, auditor: aud, agora: time.Now}
}

// Executar actualiza telefone/BI do utilizador `id`. `telefone`/`bi` a nil
// preservam o valor actual; string vazia limpa; valor presente é validado.
func (c *CasoEditarPerfilAdmin) Executar(ctx context.Context, actor, id string, telefone, bi *string) (Perfil, error) {
	persistido, err := c.garantirLinha(ctx, id)
	if err != nil {
		return Perfil{}, err
	}

	tel := persistido.Telefone
	if telefone != nil {
		tel = *telefone
	}
	doc := persistido.BI
	if bi != nil {
		doc = *bi
	}
	if err := persistido.AtualizarContacto(tel, doc); err != nil {
		return Perfil{}, err
	}
	if err := c.utilizadores.AtualizarContacto(ctx, id, persistido.Telefone, persistido.BI); err != nil {
		return Perfil{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.perfil.actualizado",
		Entidade:   "utilizador",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	}); err != nil {
		return Perfil{}, err
	}

	final, err := c.utilizadores.ObterPorID(ctx, id)
	if err != nil {
		return Perfil{}, err
	}
	return paraPerfil(final), nil
}

// garantirLinha devolve a linha local do utilizador; se não existir, hidrata-a a
// partir do Keycloak (fonte de verdade de nome/email/papéis).
func (c *CasoEditarPerfilAdmin) garantirLinha(ctx context.Context, id string) (*dominio.Utilizador, error) {
	persistido, err := c.utilizadores.ObterPorID(ctx, id)
	if err == nil {
		return persistido, nil
	}
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		return nil, err
	}
	det, err := c.admin.ObterUtilizador(ctx, id)
	if err != nil {
		return nil, err
	}
	base, err := dominio.NovoUtilizador(det.ID, det.Nome, det.Email, "", "", dominio.PapeisDe(det.Papeis))
	if err != nil {
		return nil, err
	}
	if err := c.utilizadores.GuardarComPapeis(ctx, base); err != nil {
		return nil, err
	}
	return c.utilizadores.ObterPorID(ctx, id)
}
```

- [ ] **Step 4: Correr e confirmar que passa**

Run: `go test ./internal/application/identidade/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/application/identidade/editar_perfil_admin.go internal/application/identidade/editar_perfil_admin_test.go
git commit -m "$(cat <<'EOF'
feat(identidade): caso de uso de edição administrativa de perfil (com hidratação JIT)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Porta `Notificador` + adaptador SMTP + fallback no-op

**Files:**
- Modify: `internal/application/identidade/ports.go`
- Create: `internal/adapters/smtp/notificador.go`
- Create: `internal/adapters/smtp/nulo.go`
- Test: `internal/adapters/smtp/notificador_test.go`

**Interfaces:**
- Produces:
  - Porta `appident.Notificador` com `NotificarCriacao(ctx, email, nome, senha string) error` e `NotificarResetPassword(ctx, email, nome, senha string) error`.
  - `smtp.NovoNotificadorSMTP(host, porta, remetente string) *smtp.NotificadorSMTP` (campo público `Enviar EnviarFunc` para testes; default `net/smtp.SendMail`).
  - `smtp.NovoNotificadorNulo(log *slog.Logger) smtp.NotificadorNulo`.

- [ ] **Step 1: Adicionar a porta `Notificador`**

Em `internal/application/identidade/ports.go`, adicionar (a seguir à interface `Auditor`):

```go
// Notificador envia notificações ao utilizador (ex.: email). O envio é
// best-effort na perspectiva do chamador: um erro devolvido aqui é registado
// mas não falha a operação de negócio. Implementado por adapters/smtp.
type Notificador interface {
	NotificarCriacao(ctx context.Context, email, nome, senhaTemporaria string) error
	NotificarResetPassword(ctx context.Context, email, nome, senhaTemporaria string) error
}
```

- [ ] **Step 2: Escrever os testes do adaptador que falham**

Criar `internal/adapters/smtp/notificador_test.go`:

```go
package smtp

import (
	"context"
	"errors"
	nsmtp "net/smtp"
	"strings"
	"testing"
)

func TestNotificadorSMTP_NotificarCriacao_ComponMensagem(t *testing.T) {
	var (
		addr string
		to   []string
		msg  []byte
	)
	n := NovoNotificadorSMTP("mailhog", "1025", "nao-responder@sgc.ao")
	n.Enviar = func(a string, _ nsmtp.Auth, _ string, dest []string, corpo []byte) error {
		addr, to, msg = a, dest, corpo
		return nil
	}

	if err := n.NotificarCriacao(context.Background(), "ana@sgc.ao", "Ana", "senha-secreta-1"); err != nil {
		t.Fatalf("NotificarCriacao: %v", err)
	}
	if addr != "mailhog:1025" {
		t.Fatalf("addr = %q; quer mailhog:1025", addr)
	}
	if len(to) != 1 || to[0] != "ana@sgc.ao" {
		t.Fatalf("to = %v; quer [ana@sgc.ao]", to)
	}
	s := string(msg)
	if !strings.Contains(s, "To: ana@sgc.ao") {
		t.Fatalf("cabeçalho To em falta: %s", s)
	}
	if !strings.Contains(s, "senha-secreta-1") {
		t.Fatalf("senha temporária em falta no corpo: %s", s)
	}
}

func TestNotificadorSMTP_PropagaErro(t *testing.T) {
	n := NovoNotificadorSMTP("mailhog", "1025", "x@sgc.ao")
	n.Enviar = func(string, nsmtp.Auth, string, []string, []byte) error {
		return errors.New("smtp em baixo")
	}
	if err := n.NotificarResetPassword(context.Background(), "a@sgc.ao", "A", "s"); err == nil {
		t.Fatal("esperava erro propagado do envio")
	}
}

func TestNotificadorNulo_DevolveNil(t *testing.T) {
	n := NovoNotificadorNulo(nil)
	if err := n.NotificarCriacao(context.Background(), "a@sgc.ao", "A", "s"); err != nil {
		t.Fatalf("NotificarCriacao nulo devia devolver nil, obtive %v", err)
	}
	if err := n.NotificarResetPassword(context.Background(), "a@sgc.ao", "A", "s"); err != nil {
		t.Fatalf("NotificarResetPassword nulo devia devolver nil, obtive %v", err)
	}
}
```

- [ ] **Step 3: Correr e confirmar que falha**

Run: `go test ./internal/adapters/smtp/`
Expected: FAIL (pacote/tipos inexistentes).

- [ ] **Step 4: Implementar o adaptador SMTP**

Criar `internal/adapters/smtp/notificador.go`:

```go
// Package smtp implementa o Notificador (application/identidade) por email, via
// SMTP (net/smtp da stdlib). Adequado a dev com MailHog (sem autenticação).
// Camada 3 — Adaptadores.
package smtp

import (
	"context"
	"fmt"
	"net"
	nsmtp "net/smtp"
	"strings"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
)

// EnviarFunc é a assinatura de net/smtp.SendMail; extraída para permitir
// substituição em testes.
type EnviarFunc func(addr string, a nsmtp.Auth, from string, to []string, msg []byte) error

// NotificadorSMTP envia notificações por email através de um servidor SMTP.
type NotificadorSMTP struct {
	host      string
	porta     string
	remetente string
	// Enviar é o transporte SMTP; default net/smtp.SendMail. Público para testes.
	Enviar EnviarFunc
}

// NovoNotificadorSMTP constrói o adaptador apontado a host:porta, com o
// remetente indicado.
func NovoNotificadorSMTP(host, porta, remetente string) *NotificadorSMTP {
	return &NotificadorSMTP{host: host, porta: porta, remetente: remetente, Enviar: nsmtp.SendMail}
}

// NotificarCriacao envia o email de conta criada com a senha temporária.
func (n *NotificadorSMTP) NotificarCriacao(_ context.Context, email, nome, senha string) error {
	assunto := "Conta SGC criada"
	corpo := fmt.Sprintf(
		"Olá %s,\r\n\r\nFoi criada uma conta no Sistema de Gestão de Clínicas (SGC).\r\n"+
			"Senha temporária: %s\r\n\r\nSerá pedido para a alterar no primeiro acesso.\r\n",
		nome, senha)
	return n.enviarMensagem(email, assunto, corpo)
}

// NotificarResetPassword envia o email de senha reposta com a nova senha temporária.
func (n *NotificadorSMTP) NotificarResetPassword(_ context.Context, email, nome, senha string) error {
	assunto := "Senha SGC reposta"
	corpo := fmt.Sprintf(
		"Olá %s,\r\n\r\nA sua senha no SGC foi reposta por um administrador.\r\n"+
			"Senha temporária: %s\r\n\r\nSerá pedido para a alterar no próximo acesso.\r\n",
		nome, senha)
	return n.enviarMensagem(email, assunto, corpo)
}

func (n *NotificadorSMTP) enviarMensagem(para, assunto, corpo string) error {
	msg := montarMensagem(n.remetente, para, assunto, corpo)
	return n.Enviar(net.JoinHostPort(n.host, n.porta), nil, n.remetente, []string{para}, msg)
}

// montarMensagem compõe uma mensagem RFC 5322 de texto simples (UTF-8).
func montarMensagem(de, para, assunto, corpo string) []byte {
	var b strings.Builder
	b.WriteString("From: " + de + "\r\n")
	b.WriteString("To: " + para + "\r\n")
	b.WriteString("Subject: " + assunto + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(corpo)
	return []byte(b.String())
}

// Garantia de conformidade com a porta.
var _ appident.Notificador = (*NotificadorSMTP)(nil)
```

- [ ] **Step 5: Implementar o fallback no-op**

Criar `internal/adapters/smtp/nulo.go`:

```go
package smtp

import (
	"context"
	"log/slog"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
)

// NotificadorNulo é usado quando o SMTP não está configurado: não envia nada,
// apenas regista em nível debug. Garante que operações não falham por ausência
// de infra de email.
type NotificadorNulo struct{ log *slog.Logger }

// NovoNotificadorNulo constrói o notificador no-op (log opcional).
func NovoNotificadorNulo(log *slog.Logger) NotificadorNulo {
	return NotificadorNulo{log: log}
}

// NotificarCriacao não envia; regista em debug (sem a senha).
func (n NotificadorNulo) NotificarCriacao(_ context.Context, email, _, _ string) error {
	if n.log != nil {
		n.log.Debug("notificação de criação suprimida (SMTP não configurado)", "email", email)
	}
	return nil
}

// NotificarResetPassword não envia; regista em debug (sem a senha).
func (n NotificadorNulo) NotificarResetPassword(_ context.Context, email, _, _ string) error {
	if n.log != nil {
		n.log.Debug("notificação de reset suprimida (SMTP não configurado)", "email", email)
	}
	return nil
}

// Garantia de conformidade com a porta.
var _ appident.Notificador = NotificadorNulo{}
```

- [ ] **Step 6: Correr e confirmar que passa**

Run: `go test ./internal/adapters/smtp/`
Expected: PASS

- [ ] **Step 7: Confirmar arch-lint (net/smtp é stdlib)**

Run: `go-arch-lint check` (ou `make lint` se disponível)
Expected: sem violações — o pacote `smtp` está sob `internal/adapters/**` e só importa stdlib + `application`.

- [ ] **Step 8: Commit**

```bash
git add internal/application/identidade/ports.go internal/adapters/smtp/
git commit -m "$(cat <<'EOF'
feat(identidade): porta Notificador e adaptador SMTP (net/smtp) com fallback no-op

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Ligar o `Notificador` à criação e ao reset de password

**Files:**
- Modify: `internal/application/identidade/criar_utilizador.go`
- Modify: `internal/application/identidade/reset_credenciais.go`
- Create: `internal/application/identidade/notificador_fake_test.go`
- Modify: `internal/application/identidade/criar_utilizador_test.go` (construtores + 1 teste)
- Modify: `internal/application/identidade/reset_credenciais_test.go` (construtores + 1 teste)

**Interfaces:**
- Consumes: `Notificador`, `AdminIdentidade.ObterUtilizador`.
- Produces (assinaturas alteradas):
  - `NovoCasoCriarUtilizador(a AdminIdentidade, aud Auditor, notif Notificador) *CasoCriarUtilizador`
  - `NovoCasoResetPassword(a AdminIdentidade, aud Auditor, notif Notificador) *CasoResetPassword`

> A partir daqui o pacote `platform` (app.go) deixa de compilar até à Task 8 — é esperado. Verificação desta task é **ao nível do pacote de aplicação**.

- [ ] **Step 1: Criar o fake do notificador (pacote de teste)**

Criar `internal/application/identidade/notificador_fake_test.go`:

```go
package identidade_test

import "context"

// fakeNotificador captura as chamadas de notificação; `err` simula falha de envio.
type fakeNotificador struct {
	criacoes int
	resets   int
	err      error
}

func (f *fakeNotificador) NotificarCriacao(context.Context, string, string, string) error {
	f.criacoes++
	return f.err
}
func (f *fakeNotificador) NotificarResetPassword(context.Context, string, string, string) error {
	f.resets++
	return f.err
}
```

- [ ] **Step 2: Actualizar os testes existentes para a nova assinatura + novos casos**

Em `internal/application/identidade/criar_utilizador_test.go`:
- Em **todas** as chamadas a `appident.NovoCasoCriarUtilizador(admin, aud)` / `NovoCasoCriarUtilizador(&fakeCriador{...}, &fakeAuditor{})`, acrescentar um 3.º argumento `&fakeNotificador{}`. (São 6 chamadas: `TestCriarUtilizador_PapelComum`, `_PapelSensivel_ExigeOTP`, `_EmailInvalido`, `_PapelInvalido`, `_UsernameVazio`, `_PropagaConflito`.)
- Acrescentar dois testes ao fim:

```go
func TestCriarUtilizador_NotificaPorEmail(t *testing.T) {
	notif := &fakeNotificador{}
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{id: "novo-id"}, &fakeAuditor{}, notif)
	if _, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "ana.silva", Nome: "Ana Silva", Email: "ana@sgc.ao",
		Papeis: []dominio.Papel{dominio.PapelMedico},
	}); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if notif.criacoes != 1 {
		t.Fatalf("esperava 1 notificação de criação, obtive %d", notif.criacoes)
	}
}

func TestCriarUtilizador_FalhaEmailNaoFalhaCriacao(t *testing.T) {
	notif := &fakeNotificador{err: errors.New("smtp em baixo")}
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{id: "novo-id"}, &fakeAuditor{}, notif)
	out, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "ana.silva", Nome: "Ana Silva", Email: "ana@sgc.ao",
		Papeis: []dominio.Papel{dominio.PapelMedico},
	})
	if err != nil {
		t.Fatalf("falha de email não deve falhar a criação, obtive %v", err)
	}
	if out.ID != "novo-id" || out.SenhaTemporaria == "" {
		t.Fatalf("criação devia ter tido sucesso: %+v", out)
	}
}
```

Em `internal/application/identidade/reset_credenciais_test.go`:
- Em `TestResetPassword_GeraEAudita` e `TestResetPassword_PropagaErro`, acrescentar `&fakeNotificador{}` como 3.º argumento a `NovoCasoResetPassword(...)`.
- Acrescentar um teste:

```go
func TestResetPassword_FalhaEmailNaoFalha(t *testing.T) {
	admin := &fakeAdmin{}
	notif := &fakeNotificador{err: errors.New("smtp em baixo")}
	caso := appident.NovoCasoResetPassword(admin, &fakeAuditor{}, notif)
	out, err := caso.Executar(context.Background(), "actor-1", "alvo-1")
	if err != nil {
		t.Fatalf("falha de email não deve falhar o reset, obtive %v", err)
	}
	if out.SenhaTemporaria == "" {
		t.Fatal("esperava senha temporária devolvida na mesma")
	}
	if notif.resets != 1 {
		t.Fatalf("esperava 1 tentativa de notificação, obtive %d", notif.resets)
	}
}
```

(Confirmar que `errors` está importado em ambos os ficheiros de teste — `reset_credenciais_test.go` já importa; `criar_utilizador_test.go` já importa `errors`.)

- [ ] **Step 3: Correr e confirmar que falha**

Run: `go test ./internal/application/identidade/ -run 'CriarUtilizador|ResetPassword'`
Expected: FAIL (assinatura do construtor com 2 args; agora chamada com 3).

- [ ] **Step 4: Ligar o notificador na criação**

Em `internal/application/identidade/criar_utilizador.go`:
- Acrescentar `"log/slog"` ao bloco de imports.
- Alterar o struct e o construtor:

```go
type CasoCriarUtilizador struct {
	admin   AdminIdentidade
	auditor Auditor
	notif   Notificador
	agora   func() time.Time
}

// NovoCasoCriarUtilizador constrói o caso de uso.
func NovoCasoCriarUtilizador(a AdminIdentidade, aud Auditor, notif Notificador) *CasoCriarUtilizador {
	return &CasoCriarUtilizador{admin: a, auditor: aud, notif: notif, agora: time.Now}
}
```

- No fim de `Executar`, substituir `return UtilizadorCriado{ID: id, SenhaTemporaria: senha}, nil` por:

```go
	// Notificação best-effort: falha de email não falha a criação nem vaza a senha.
	if err := c.notif.NotificarCriacao(ctx, entrada.Email, entrada.Nome, senha); err != nil {
		slog.Warn("falha ao notificar criação por email", "utilizador", id, "erro", err)
	}

	return UtilizadorCriado{ID: id, SenhaTemporaria: senha}, nil
```

- [ ] **Step 5: Ligar o notificador no reset de password**

Em `internal/application/identidade/reset_credenciais.go`:
- Acrescentar `"log/slog"` ao bloco de imports.
- Alterar o struct e o construtor do `CasoResetPassword`:

```go
type CasoResetPassword struct {
	admin   AdminIdentidade
	auditor Auditor
	notif   Notificador
	agora   func() time.Time
}

// NovoCasoResetPassword constrói o caso de uso.
func NovoCasoResetPassword(a AdminIdentidade, aud Auditor, notif Notificador) *CasoResetPassword {
	return &CasoResetPassword{admin: a, auditor: aud, notif: notif, agora: time.Now}
}
```

- No fim de `Executar`, substituir `return CredencialReposta{SenhaTemporaria: senha}, nil` por:

```go
	// Notificação best-effort: obtém o email do alvo e envia; nada disto falha o reset.
	if det, err := c.admin.ObterUtilizador(ctx, id); err != nil {
		slog.Warn("não obteve o email do utilizador para notificar o reset", "utilizador", id, "erro", err)
	} else if err := c.notif.NotificarResetPassword(ctx, det.Email, det.Nome, senha); err != nil {
		slog.Warn("falha ao notificar reset por email", "utilizador", id, "erro", err)
	}

	return CredencialReposta{SenhaTemporaria: senha}, nil
```

(`CasoResetOTP` não muda.)

- [ ] **Step 6: Correr e confirmar que passa**

Run: `go test ./internal/application/identidade/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/application/identidade/criar_utilizador.go internal/application/identidade/reset_credenciais.go \
  internal/application/identidade/notificador_fake_test.go \
  internal/application/identidade/criar_utilizador_test.go internal/application/identidade/reset_credenciais_test.go
git commit -m "$(cat <<'EOF'
feat(identidade): notificar por email (best-effort) na criação e no reset de password

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: HTTP — rotas de sessões e edição admin de perfil

**Files:**
- Modify: `internal/adapters/http/admin_handler.go`
- Modify: `internal/adapters/http/admin_test.go` (helper `routerAdmin` + 3 fakes + testes)

**Interfaces:**
- Consumes: `appident.SessaoActiva`, `appident.Perfil`, `SessaoDe`, `responderErro`, `RBAC`, `corpoPerfil` (já definido em `identidade_handler.go`, mesmo pacote `http`).
- Produces:
  - Interfaces `ServicoListarSessoes` (`Executar(ctx, id) ([]appident.SessaoActiva, error)`), `ServicoRevogarSessao` (`Executar(ctx, actor, sessionID) error`), `ServicoEditarPerfilAdmin` (`Executar(ctx, actor, id, telefone, bi *string) (appident.Perfil, error)`).
  - `NovoAdministracaoHandler(...)` cresce de 8 para **11** argumentos (novos, por esta ordem, no fim): `sessoesListar ServicoListarSessoes, sessaoRevogar ServicoRevogarSessao, perfilAdmin ServicoEditarPerfilAdmin`.
  - Rotas: `GET /api/v1/identidade/utilizadores/:id/sessoes` (leitura), `PATCH /api/v1/identidade/utilizadores/:id/perfil` (escrita), `DELETE /api/v1/identidade/sessoes/:sessionId` (escrita).

> `platform` (app.go) continua sem compilar até à Task 8. Verificação desta task é ao nível do pacote `internal/adapters/http`.

- [ ] **Step 1: Escrever os testes HTTP que falham**

Em `internal/adapters/http/admin_test.go`:
- Acrescentar os 3 fakes (a seguir a `fakeResetOtp`):

```go
type fakeSessoesListar struct {
	out []appident.SessaoActiva
	err error
}

func (f fakeSessoesListar) Executar(context.Context, string) ([]appident.SessaoActiva, error) {
	return f.out, f.err
}

type fakeSessaoRevogar struct {
	ultimoActor string
	ultimoSID   string
	err         error
}

func (f *fakeSessaoRevogar) Executar(_ context.Context, actor, sessionID string) error {
	f.ultimoActor, f.ultimoSID = actor, sessionID
	return f.err
}

type fakePerfilAdmin struct {
	out appident.Perfil
	err error
}

func (f fakePerfilAdmin) Executar(context.Context, string, string, *string, *string) (appident.Perfil, error) {
	return f.out, f.err
}
```

- Actualizar o helper `routerAdmin` para passar os 3 novos serviços ao construtor (acrescentar ao fim da lista de argumentos de `NovoAdministracaoHandler`):

```go
		fakeResetOtp{},
		fakeSessoesListar{out: []appident.SessaoActiva{{ID: "sess-1", IP: "10.0.0.5"}}},
		&fakeSessaoRevogar{},
		fakePerfilAdmin{out: appident.Perfil{KeycloakID: "u1", Nome: "Ana", Telefone: "+244923456789"}},
```

- Acrescentar os testes ao fim do ficheiro:

```go
func TestAdmin_ListarSessoes_AuditorPermitido(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}}, &fakePapel{})
	w := pedido(r, "GET", "/api/v1/identidade/utilizadores/u1/sessoes", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("Auditor deve poder ver sessões; obtive %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"id":"sess-1"`) {
		t.Fatalf("corpo inesperado: %s", w.Body.String())
	}
}

func TestAdmin_ListarSessoes_MedicoProibido(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, &fakePapel{})
	if w := pedido(r, "GET", "/api/v1/identidade/utilizadores/u1/sessoes", "Bearer xyz"); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Medico não deve ver sessões; obtive %d", w.Code)
	}
}

func TestAdmin_RevogarSessao_AdminOk_204(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Sujeito: "actor-1", Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	if w := pedido(r, "DELETE", "/api/v1/identidade/sessoes/sess-1", "Bearer xyz"); w.Code != nethttp.StatusNoContent {
		t.Fatalf("esperava 204, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestAdmin_RevogarSessao_AuditorProibido(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}}, &fakePapel{})
	if w := pedido(r, "DELETE", "/api/v1/identidade/sessoes/sess-1", "Bearer xyz"); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Auditor não deve revogar sessões; obtive %d", w.Code)
	}
}

func TestAdmin_EditarPerfil_AdminOk_200(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Sujeito: "actor-1", Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedidoCorpo(r, "PATCH", "/api/v1/identidade/utilizadores/u1/perfil", `{"telefone":"+244923456789"}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"telefone":"+244923456789"`) {
		t.Fatalf("corpo inesperado: %s", w.Body.String())
	}
}

func TestAdmin_EditarPerfil_MedicoProibido_403(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, &fakePapel{})
	w := pedidoCorpo(r, "PATCH", "/api/v1/identidade/utilizadores/u1/perfil", `{"telefone":"+244923456789"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("Medico não deve editar perfil de outros; obtive %d", w.Code)
	}
}

func TestAdmin_EditarPerfil_CorpoInvalido_400(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedidoCorpo(r, "PATCH", "/api/v1/identidade/utilizadores/u1/perfil", `{`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400 para JSON inválido; obtive %d", w.Code)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/http/ -run 'ListarSessoes|RevogarSessao|EditarPerfil'`
Expected: FAIL (construtor com 8 args; interfaces/rotas/handlers inexistentes).

- [ ] **Step 3: Adicionar as interfaces de serviço**

Em `internal/adapters/http/admin_handler.go`, dentro do bloco `type ( ... )` (a seguir a `ServicoResetOtp`):

```go
	// ServicoListarSessoes lista as sessões activas de um utilizador.
	ServicoListarSessoes interface {
		Executar(ctx context.Context, id string) ([]appident.SessaoActiva, error)
	}
	// ServicoRevogarSessao revoga uma sessão específica.
	ServicoRevogarSessao interface {
		Executar(ctx context.Context, actor, sessionID string) error
	}
	// ServicoEditarPerfilAdmin actualiza o perfil (telefone/BI) de outro utilizador.
	ServicoEditarPerfilAdmin interface {
		Executar(ctx context.Context, actor, id string, telefone, bi *string) (appident.Perfil, error)
	}
```

- [ ] **Step 4: Ampliar o struct e o construtor**

Em `internal/adapters/http/admin_handler.go`, acrescentar campos ao `AdministracaoHandler` (a seguir a `resetOtp`):

```go
	sessoesListar ServicoListarSessoes
	sessaoRevogar ServicoRevogarSessao
	perfilAdmin   ServicoEditarPerfilAdmin
```

E actualizar `NovoAdministracaoHandler` para aceitar e atribuir os 3 novos parâmetros:

```go
func NovoAdministracaoHandler(
	listar ServicoListar,
	obter ServicoObterUtilizador,
	atribuir ServicoAtribuirPapel,
	revogar ServicoRevogarPapel,
	activar ServicoDefinirActivo,
	criar ServicoCriarUtilizador,
	resetPassword ServicoResetPassword,
	resetOtp ServicoResetOtp,
	sessoesListar ServicoListarSessoes,
	sessaoRevogar ServicoRevogarSessao,
	perfilAdmin ServicoEditarPerfilAdmin,
) *AdministracaoHandler {
	return &AdministracaoHandler{
		listar: listar, obter: obter, atribuir: atribuir, revogar: revogar,
		activar: activar, criar: criar, resetPassword: resetPassword, resetOtp: resetOtp,
		sessoesListar: sessoesListar, sessaoRevogar: sessaoRevogar, perfilAdmin: perfilAdmin,
	}
}
```

- [ ] **Step 5: Registar as rotas**

Em `RegistarAdministracao`, a seguir a `g.POST("/:id/reset-otp", escrita, h.reporOtp)`, acrescentar as duas rotas do grupo de utilizadores e um segundo grupo para a revogação de sessão:

```go
	g.GET("/:id/sessoes", leitura, h.listarSessoes)
	g.PATCH("/:id/perfil", escrita, h.editarPerfilAdmin)

	// Grupo separado: revogar uma sessão por sessionId (não precisa do userId).
	gsessoes := r.Group("/api/v1/identidade/sessoes")
	gsessoes.Use(protecao...)
	gsessoes.DELETE("/:sessionId", escrita, h.revogarSessao)
```

- [ ] **Step 6: Implementar os handlers**

No fim de `internal/adapters/http/admin_handler.go`:

```go
func (h *AdministracaoHandler) listarSessoes(c *gin.Context) {
	out, err := h.sessoesListar.Executar(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *AdministracaoHandler) revogarSessao(c *gin.Context) {
	actor, _ := SessaoDe(c)
	if err := h.sessaoRevogar.Executar(c.Request.Context(), actor.Sujeito, c.Param("sessionId")); err != nil {
		responderErro(c, err)
		return
	}
	c.Status(nethttp.StatusNoContent)
}

func (h *AdministracaoHandler) editarPerfilAdmin(c *gin.Context) {
	var corpo corpoPerfil
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	perfil, err := h.perfilAdmin.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), corpo.Telefone, corpo.Bi)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, perfil)
}
```

(`corpoPerfil`, `erros` e `i18n` já estão no pacote/imports — `corpoPerfil` vem de `identidade_handler.go` e `erros`/`i18n` já são importados em `admin_handler.go`.)

- [ ] **Step 7: Correr e confirmar que passa**

Run: `go test ./internal/adapters/http/`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/http/admin_handler.go internal/adapters/http/admin_test.go
git commit -m "$(cat <<'EOF'
feat(identidade): rotas HTTP de sessões (listar/revogar) e edição admin de perfil

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Composition root — config SMTP, wiring, MailHog

**Files:**
- Modify: `internal/platform/config/config.go`
- Test: `internal/platform/config/config_smtp_test.go` (criar)
- Modify: `internal/platform/app.go`
- Modify: `docker-compose.yml`
- Modify: `.env.example`

**Interfaces:**
- Consumes: tudo o que as tasks anteriores produziram — `appident.Novo*`, `adhttp.NovoAdministracaoHandler` (11 args), `adsmtp.NovoNotificadorSMTP`/`NovoNotificadorNulo`.
- Produces: `Config.SMTPHost`, `Config.SMTPPorta`, `Config.SMTPRemetente`; módulo inteiro a compilar de novo.

- [ ] **Step 1: Escrever o teste de config que falha**

Criar `internal/platform/config/config_smtp_test.go`:

```go
package config

import "testing"

func prepararEnvObrigatorio(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("REDIS_URL", "redis://x")
	t.Setenv("KEYCLOAK_ISSUER", "http://kc/realms/sgc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "sgc-admin")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "segredo")
}

func TestCarregar_SMTPDefaults(t *testing.T) {
	prepararEnvObrigatorio(t)
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("SMTP_FROM", "")

	cfg, err := Carregar()
	if err != nil {
		t.Fatalf("Carregar: %v", err)
	}
	if cfg.SMTPHost != "" {
		t.Fatalf("SMTPHost default devia ser vazio, obtive %q", cfg.SMTPHost)
	}
	if cfg.SMTPPorta != "1025" {
		t.Fatalf("SMTPPorta default = %q; quer 1025", cfg.SMTPPorta)
	}
	if cfg.SMTPRemetente != "nao-responder@sgc.ao" {
		t.Fatalf("SMTPRemetente default = %q; quer nao-responder@sgc.ao", cfg.SMTPRemetente)
	}
}

func TestCarregar_SMTPConfigurado(t *testing.T) {
	prepararEnvObrigatorio(t)
	t.Setenv("SMTP_HOST", "mailhog")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_FROM", "sgc@clinica.ao")

	cfg, err := Carregar()
	if err != nil {
		t.Fatalf("Carregar: %v", err)
	}
	if cfg.SMTPHost != "mailhog" || cfg.SMTPPorta != "2525" || cfg.SMTPRemetente != "sgc@clinica.ao" {
		t.Fatalf("SMTP não lido correctamente: %+v", cfg)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/platform/config/ -run SMTP`
Expected: FAIL (campos inexistentes).

- [ ] **Step 3: Adicionar os campos de config**

Em `internal/platform/config/config.go`:
- No struct `Config`, a seguir a `JanelaTaxa`:

```go
	SMTPHost      string // host SMTP para notificações (vazio → notificador no-op)
	SMTPPorta     string // porta SMTP (default 1025 — MailHog)
	SMTPRemetente string // remetente dos emails (From)
```

- Em `Carregar`, dentro do literal `cfg := Config{...}`, a seguir a `JanelaTaxa: time.Minute,`:

```go
		SMTPHost:      os.Getenv("SMTP_HOST"),
		SMTPPorta:     valorOu("SMTP_PORT", "1025"),
		SMTPRemetente: valorOu("SMTP_FROM", "nao-responder@sgc.ao"),
```

(Sem validação — todos opcionais.)

- [ ] **Step 4: Correr e confirmar que passa**

Run: `go test ./internal/platform/config/`
Expected: PASS

- [ ] **Step 5: Fiar tudo no composition root**

Em `internal/platform/app.go`:
- Acrescentar ao bloco de imports:

```go
	adsmtp "github.com/ivandrosilva12/sgcfinal/internal/adapters/smtp"
```

- A seguir à construção de `adminKC` (linha ~54), construir o notificador:

```go
	var notificador appident.Notificador
	if cfg.SMTPHost == "" {
		notificador = adsmtp.NovoNotificadorNulo(logger)
		logger.Info("notificações por email desactivadas (SMTP_HOST vazio)")
	} else {
		notificador = adsmtp.NovoNotificadorSMTP(cfg.SMTPHost, cfg.SMTPPorta, cfg.SMTPRemetente)
		logger.Info("notificações por email activadas", "smtp", cfg.SMTPHost+":"+cfg.SMTPPorta)
	}
```

- Passar o notificador aos casos que o usam, e construir os novos casos. Substituir as linhas de construção de `casoCriar` e `casoResetPass` e acrescentar os novos casos:

```go
	casoCriar := appident.NovoCasoCriarUtilizador(adminKC, repoAuditoria, notificador)
	casoResetPass := appident.NovoCasoResetPassword(adminKC, repoAuditoria, notificador)
	casoResetOTP := appident.NovoCasoResetOTP(adminKC, repoAuditoria)
	casoListarSessoes := appident.NovoCasoListarSessoes(adminKC)
	casoRevogarSessao := appident.NovoCasoRevogarSessao(adminKC, repoAuditoria)
	casoEditarPerfilAdmin := appident.NovoCasoEditarPerfilAdmin(adminKC, repoUtilizadores, repoAuditoria)
	handlerAdmin := adhttp.NovoAdministracaoHandler(
		casoListar, casoObter, casoAtribuir, casoRevogar, casoActivo, casoCriar,
		casoResetPass, casoResetOTP, casoListarSessoes, casoRevogarSessao, casoEditarPerfilAdmin,
	)
```

(Remover a linha antiga `handlerAdmin := adhttp.NovoAdministracaoHandler(...)` de 8 args.)

- [ ] **Step 6: Adicionar o MailHog ao compose**

Em `docker-compose.yml`, acrescentar um serviço (a seguir ao `redis`, mantendo a indentação de 2 espaços dos serviços):

```yaml
  mailhog:
    image: mailhog/mailhog:v1.0.1
    container_name: sgc-mailhog
    ports:
      - "1025:1025"  # SMTP
      - "8025:8025"  # UI + API (http://localhost:8025)
```

E, no serviço `api`, dentro do bloco `environment:` (junto de `REDIS_URL`), acrescentar:

```yaml
      SMTP_HOST: mailhog
      SMTP_PORT: "1025"
      SMTP_FROM: nao-responder@sgc.ao
```

(Nota: a imagem MailHog é mínima e não traz shell fiável para healthcheck; omitimos o healthcheck de propósito — o notificador é best-effort e a saúde da API não depende do MailHog.)

- [ ] **Step 7: Documentar no `.env.example`**

Em `.env.example`, a seguir às variáveis do Keycloak, acrescentar:

```bash
# --- SMTP (notificações por email) ---
# SMTP_HOST vazio → notificações desactivadas (fallback no-op). Em dev, MailHog.
SMTP_HOST=localhost
SMTP_PORT=1025
SMTP_FROM=nao-responder@sgc.ao
```

- [ ] **Step 8: Verificar o módulo inteiro verde + compose válido**

Run:
```bash
go build ./...
go vet ./...
gofmt -l internal/ cmd/
go test ./...
docker compose config >/dev/null
```
Expected: build OK; vet OK; `gofmt -l` sem output (nenhum ficheiro por formatar); todos os testes PASS; compose válido.

- [ ] **Step 9: Commit**

```bash
git add internal/platform/config/config.go internal/platform/config/config_smtp_test.go \
  internal/platform/app.go docker-compose.yml .env.example
git commit -m "$(cat <<'EOF'
feat(identidade): fiar sessões, edição admin de perfil e notificações; MailHog no compose

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Integração e2e + documentação

**Files:**
- Create: `tests/integration/sessoes_perfil_admin_test.go`
- Create: `adrs/ADR-025-sessoes-perfil-admin-notificacoes.md`
- Modify: `SPRINT.md`
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: os handlers/casos já fiados; a Admin API real; a BD real; o MailHog real.

Os testes de integração usam a tag `//go:build integration` e devem **SKIP** (não falhar) quando a infra não está disponível — seguir o padrão dos testes de integração existentes (`tests/integration/ciclo_vida_test.go`). Usar apenas a stdlib e `pgx` (regra arch-lint do componente `tests`: **não** importar `google/uuid`); onde for preciso um UUID, usar um literal válido fixo.

- [ ] **Step 1: Ler o padrão de integração existente**

Run: `sed -n '1,60p' tests/integration/ciclo_vida_test.go`
Objectivo: reutilizar os helpers de skip/ligação (ex.: obtenção de token de serviço, ligação à BD, base URL do Keycloak) já presentes no pacote de integração. **Não** duplicar helpers — reutilizar os existentes do pacote `integration`.

- [ ] **Step 2: Escrever os testes de integração**

Criar `tests/integration/sessoes_perfil_admin_test.go` com a tag de build e três testes que SKIP sem infra:

```go
//go:build integration

package integration

import (
	"context"
	"testing"
)

// TestListarERevogarSessoes_ViaKeycloak cria um utilizador, autentica-o para
// gerar uma sessão, lista as sessões via Admin API, revoga uma e confirma que
// deixou de aparecer. Faz SKIP se a infra (Keycloak) não estiver disponível.
func TestListarERevogarSessoes_ViaKeycloak(t *testing.T) {
	admin := adminOuSkip(t) // helper existente no pacote integration; ver Step 1
	ctx := context.Background()

	// UUID válido fixo para dados de teste (arch-lint: sem google/uuid).
	const utilizadorTeste = "00000000-0000-4000-8000-0000000000d1"
	_ = ctx
	_ = admin
	_ = utilizadorTeste
	t.Skip("preencher com o fluxo real usando os helpers do pacote integration (ver ciclo_vida_test.go)")
}
```

> **Instrução ao implementador:** substituir o corpo esqueleto pelo fluxo real, seguindo `ciclo_vida_test.go`: (1) obter o `AdminCliente` real (ou SKIP), (2) criar um utilizador de teste com username único e limpá-lo no fim (`ApagarUtilizador`), (3) `ListarSessoes` (pode vir vazio se não houver login — nesse caso afirmar que a chamada não erra e devolve slice; se o padrão existente já faz *direct grant*, gerar uma sessão e afirmar ≥1), (4) se houver sessão, `RevogarSessao` e reconfirmar. Para a **edição admin de perfil**, escrever `TestEditarPerfilAdmin_ViaBD` que insere/garante a linha e chama o caso de uso real contra a BD, afirmando a persistência de telefone/BI. Para o **email**, escrever `TestNotificacaoCriacao_ViaMailHog` que cria um utilizador com SMTP a apontar ao MailHog e consulta `GET http://localhost:8025/api/v2/messages` para confirmar 1 mensagem para o email do utilizador; SKIP se o MailHog não responder. Todos os testes SKIP (não FALHAM) quando a respectiva dependência está em baixo.

- [ ] **Step 3: Verificar que compila com a tag e que passa/SKIP**

Run:
```bash
go build -tags integration ./...
go test -tags integration ./tests/integration/ -run 'Sessoes|PerfilAdmin|MailHog' -v
```
Expected: compila; os testes correm e fazem PASS ou SKIP (nunca FAIL por infra ausente).

- [ ] **Step 4: Escrever a ADR-025**

Criar `adrs/ADR-025-sessoes-perfil-admin-notificacoes.md` (seguir o formato das ADRs existentes — Contexto, Decisão, Consequências), documentando: (a) gestão de sessões activas via Admin API (listar por utilizador; revogar granular por sessionId; auditada); (b) edição administrativa de perfil com hidratação JIT a partir do Keycloak; (c) notificações por email best-effort com fallback no-op (`net/smtp` stdlib, sem dependências novas; senha entregue por email **e** na resposta HTTP; MailHog em dev; senha nunca registada em logs). Referir que fecha os loose-ends dos Sprints 3–5.

- [ ] **Step 5: Actualizar SPRINT.md**

Em `SPRINT.md`, mudar o cabeçalho para Sprint 6 e acrescentar a secção "Sprint 6 — entregue" no topo da lista de sprints:

```markdown
- **Sprint**: 6 (BC Identidade — loose-ends: sessões, perfil admin, notificações) — **entregue**
- **Objectivo**: gestão de sessões activas (listar/revogar granular), edição
  administrativa de perfil (telefone/BI de outros), e notificações por email
  best-effort com fallback no-op. Encerra os loose-ends do BC Identidade.

## Sprint 6 — entregue

- [x] Gestão de sessões activas: listar por utilizador (Admin/Auditor/DPO),
      revogar sessão granular por sessionId (Admin). Auditado.
- [x] Edição administrativa de perfil (`PATCH /utilizadores/:id/perfil`, Admin)
      com hidratação JIT a partir do Keycloak.
- [x] Notificações por email (criação e reset de password) best-effort, com
      fallback no-op quando SMTP não configurado. MailHog no compose.
- [x] ADR-025.
```

(Manter as secções dos Sprints 1–5 abaixo.)

- [ ] **Step 6: Actualizar CLAUDE.md**

Em `CLAUDE.md`, registar a ADR-025 na lista de ADRs e actualizar a indicação de "próximo ADR" para **026** (localizar a linha que hoje diz 025 e mudá-la para 026).

- [ ] **Step 7: Verificação final completa**

Run:
```bash
go build ./...
go test ./...
bash scripts/cobertura.sh
```
Expected: build OK; testes PASS; cobertura com domínio ≥85%, aplicação ≥75%, adaptadores ≥60% (todos OK).

- [ ] **Step 8: Commit**

```bash
git add tests/integration/sessoes_perfil_admin_test.go adrs/ADR-025-sessoes-perfil-admin-notificacoes.md SPRINT.md CLAUDE.md
git commit -m "$(cat <<'EOF'
docs(identidade): ADR-025, testes de integração e reconciliação de docs do Sprint 6

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Verificação (fim a fim)

1. `go build ./...`, `go vet ./...`, `gofmt -l` limpos; `go test ./...` verde; `bash scripts/cobertura.sh` cumpre 85/75/60.
2. Com o compose a correr: `GET /utilizadores/:id/sessoes` como Admin → 200 + sessões; `DELETE /sessoes/:sid` → 204 e a sessão desaparece da Admin API.
3. `PATCH /utilizadores/:id/perfil` como Admin com telefone/BI válidos → 200 + perfil actualizado; inválidos → 400; alvo sem linha local → hidratado do Keycloak.
4. Criar utilizador → 201 com senha na resposta **e** email no MailHog (`http://localhost:8025`). Reset de password → 200 com senha na resposta **e** email no MailHog.
5. Com `SMTP_HOST` vazio → `NotificadorNulo`: operações OK, sem email, sem erro.
6. Auditoria registada para revogação de sessão (`identidade.sessao.revogada`) e edição admin de perfil (`identidade.perfil.actualizado`). Não-Admin nas escritas → 403; leitura de sessões permitida a Admin/Auditor/DPO.

## Notas de execução

- **Ordem obrigatória**: Tasks 1→9 em sequência. O módulo inteiro só recompila na Task 8; verificar por pacote nas Tasks 6–7 (aplicação e http respectivamente).
- **Crescimento de interfaces**: a Task 2 amplia `AdminIdentidade` (+2 métodos) e actualiza **todos** os implementadores no mesmo commit (adaptador real + `fakeAdmin` + `fakeCriador`). A Task 6 troca 2 construtores; a Task 7 troca 1 construtor de handler — cada uma actualiza os respectivos testes no mesmo commit.
- **Sem dependências novas**: confirmar `git diff go.mod go.sum` vazio ao longo de todo o sprint.
