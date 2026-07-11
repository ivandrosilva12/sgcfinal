# Sprint 3 — MFA + Gestão administrativa (BC Identidade) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Impor MFA para papéis sensíveis e adicionar gestão administrativa de utilizadores/papéis (via Admin REST API do Keycloak) ao BC Identidade.

**Architecture:** Fatia vertical Clean Architecture (Domínio → Aplicação → Adaptadores → Plataforma). O Keycloak é a fonte de verdade dos papéis; a BD local é espelho JIT. A imposição de MFA é uma regra de domínio pura (`acr`/`amr` do token → `Sessao.AutenticacaoForte`), aplicada por um middleware HTTP. A gestão administrativa fala com a Admin REST API do Keycloak por um adaptador HTTP puro.

**Tech Stack:** Go 1.25, Gin, go-oidc, pgx v5, Redis, PostgreSQL 16, Keycloak 25. Sem novas dependências (adaptador admin usa `net/http`).

## Global Constraints

- **Idioma**: PT-PT angolano em TODO o output (código, comentários, commits, mensagens, JSON de erro). Nunca EN/PT-BR.
- **Linguagem ubíqua**: Utilizador, Papel, Sessão, Auditoria. Nunca Patient/Role/Session.
- **Regra de dependência**: `internal/domain/**` não importa infra (`pgx`/`gin`/`net/http`/`oidc`). Aplicação importa só domínio. `go-arch-lint` impõe em CI.
- **Sem `panic()`** fora de inicialização — sempre `error` categorizado (`erros.ErroDominio`).
- **Erros HTTP**: RFC 7807 (`application/problem+json`), mensagens via `i18n.T`.
- **Cobertura (gate CI)**: domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
- **Módulo Go**: `github.com/ivandrosilva12/sgcfinal`.
- **Commits**: Conventional Commits PT-PT (`feat(identidade): …`), terminando com:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- **Sem novas migrations** — reutiliza `identidade.utilizadores` e `auditoria.auditoria_eventos`.
- **Módulo de trabalho**: raiz `C:\Users\PC\Documents\RMPRO 2026\Software Clinicas Final`. Comandos `go`/`git` correm a partir da raiz.

---

## Ficheiros (mapa)

| Ação | Ficheiro | Responsabilidade |
|---|---|---|
| Modificar | `internal/domain/shared/erros/erros.go` | Nova categoria `CategoriaMFAObrigatorio` |
| Modificar | `internal/domain/shared/i18n/i18n.go` | Mensagens MFA/papel/utilizador |
| Modificar | `internal/domain/identidade/sessao.go` | Campo `AutenticacaoForte` |
| Criar | `internal/domain/identidade/mfa.go` | Regras puras de MFA |
| Modificar | `internal/domain/identidade/eventos.go` | Eventos de gestão de papéis |
| Criar | `internal/domain/identidade/mfa_test.go` | Testes de domínio MFA |
| Modificar | `internal/application/identidade/ports.go` | Porta `AdminIdentidade` |
| Criar | `internal/application/identidade/gerir_utilizadores.go` | DTOs + 5 casos de uso |
| Criar | `internal/application/identidade/gerir_utilizadores_test.go` | Testes de aplicação |
| Modificar | `internal/adapters/http/middleware_auth.go` | Middleware `MFAObrigatoria` |
| Modificar | `internal/adapters/http/problem.go` | Mapear MFA → 403 + type |
| Criar | `internal/adapters/http/admin_handler.go` | Rotas de administração |
| Criar | `internal/adapters/http/admin_test.go` | Testes HTTP (RBAC + MFA + bodies) |
| Modificar | `internal/adapters/keycloak/cliente.go` | Derivar `AutenticacaoForte` de `acr`/`amr` |
| Modificar | `internal/adapters/keycloak/cliente_internal_test.go` | Teste de força de autenticação |
| Criar | `internal/adapters/keycloak/admin.go` | `AdminCliente` (Admin REST API) |
| Criar | `internal/adapters/keycloak/admin_internal_test.go` | Teste de `dividirIssuer` |
| Modificar | `internal/platform/config/config.go` | Config admin Keycloak + ACR fortes |
| Modificar | `internal/platform/config/config_test.go` | Atualizar testes de config |
| Modificar | `internal/platform/app.go` | Fiar tudo no composition root |
| Modificar | `docker/keycloak/realm-sgc.json` | Cliente `sgc-admin`, user `admin.teste`, OTP |
| Criar | `tests/integration/administracao_test.go` | Smoke tests e2e |
| Criar | `adrs/ADR-022-mfa-gestao-admin.md` | Decisão de arquitetura |
| Modificar | `SPRINT.md`, `CLAUDE.md`, `.env.example` | Docs e config |

---

### Task 1: Domínio — regras de MFA, categoria de erro e eventos

**Files:**
- Modify: `internal/domain/shared/erros/erros.go`
- Modify: `internal/domain/shared/i18n/i18n.go`
- Modify: `internal/domain/identidade/sessao.go`
- Create: `internal/domain/identidade/mfa.go`
- Modify: `internal/domain/identidade/eventos.go`
- Test: `internal/domain/identidade/mfa_test.go`

**Interfaces:**
- Consumes: `Papel`, `EhSensivel` (já existem em `papel.go`); `Sessao` (`sessao.go`); `erros.Novo`, `erros.Categoria`; `i18n.T`.
- Produces:
  - `erros.CategoriaMFAObrigatorio erros.Categoria`
  - `i18n.MsgMFAObrigatoria`, `i18n.MsgPapelInvalido`, `i18n.MsgUtilizadorNaoEncontrado` (chaves `i18n.Chave`)
  - `identidade.Sessao.AutenticacaoForte bool`
  - `identidade.ExigeAutenticacaoForte(papeis []Papel) bool`
  - `identidade.VerificarAutenticacaoForte(s Sessao) error`
  - Eventos `PapelAtribuido{Actor, Alvo string, Papel Papel, Em time.Time}`, `PapelRevogado{…}`, `UtilizadorActivado{Actor, Alvo string, Em time.Time}`, `UtilizadorDesactivado{…}`.

- [ ] **Step 1: Escrever os testes de domínio (falham)**

Create `internal/domain/identidade/mfa_test.go`:

```go
package identidade_test

import (
	"errors"
	"testing"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestExigeAutenticacaoForte(t *testing.T) {
	casos := []struct {
		nome   string
		papeis []dominio.Papel
		quer   bool
	}{
		{"admin exige", []dominio.Papel{dominio.PapelAdmin}, true},
		{"director exige", []dominio.Papel{dominio.PapelMedico, dominio.PapelDirector}, true},
		{"medico nao exige", []dominio.Papel{dominio.PapelMedico}, false},
		{"sem papeis nao exige", nil, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := dominio.ExigeAutenticacaoForte(c.papeis); got != c.quer {
				t.Fatalf("ExigeAutenticacaoForte(%v) = %v; quer %v", c.papeis, got, c.quer)
			}
		})
	}
}

func TestVerificarAutenticacaoForte(t *testing.T) {
	t.Run("papel sensivel sem MFA nega", func(t *testing.T) {
		s := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}, AutenticacaoForte: false}
		err := dominio.VerificarAutenticacaoForte(s)
		if err == nil {
			t.Fatal("esperava erro MFA")
		}
		if erros.CategoriaDe(err) != erros.CategoriaMFAObrigatorio {
			t.Fatalf("categoria = %v; quer MFAObrigatorio", erros.CategoriaDe(err))
		}
	})
	t.Run("papel sensivel com MFA permite", func(t *testing.T) {
		s := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}, AutenticacaoForte: true}
		if err := dominio.VerificarAutenticacaoForte(s); err != nil {
			t.Fatalf("esperava nil, obtive %v", err)
		}
	})
	t.Run("papel comum sem MFA permite", func(t *testing.T) {
		s := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}, AutenticacaoForte: false}
		if err := dominio.VerificarAutenticacaoForte(s); err != nil {
			t.Fatalf("esperava nil, obtive %v", err)
		}
	})
	t.Run("erro nao mascara categoria", func(t *testing.T) {
		s := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelDPO}}
		var ed *erros.ErroDominio
		if !errors.As(dominio.VerificarAutenticacaoForte(s), &ed) {
			t.Fatal("esperava ErroDominio")
		}
	})
}
```

- [ ] **Step 2: Correr os testes e confirmar que falham na compilação**

Run: `go test ./internal/domain/identidade/...`
Expected: FAIL — `CategoriaMFAObrigatorio`, `AutenticacaoForte`, `ExigeAutenticacaoForte`, `VerificarAutenticacaoForte` não existem.

- [ ] **Step 3: Adicionar a categoria de erro**

In `internal/domain/shared/erros/erros.go`, add the constant to the `const` block, immediately after `CategoriaConflito` and before `CategoriaInterno`:

```go
	// CategoriaConflito — conflito de estado (→ 409).
	CategoriaConflito
	// CategoriaMFAObrigatorio — papel sensível sem segundo fator (→ 403, type próprio).
	CategoriaMFAObrigatorio
	// CategoriaInterno — falha inesperada (→ 500).
	CategoriaInterno
```

- [ ] **Step 4: Adicionar as mensagens i18n**

In `internal/domain/shared/i18n/i18n.go`, add to the `const` block (after `MsgDemasiadosPedidos`):

```go
	// MsgMFAObrigatoria — papel sensível sem segundo fator de autenticação.
	MsgMFAObrigatoria Chave = "erro.mfa_obrigatoria"
	// MsgPapelInvalido — código de papel desconhecido.
	MsgPapelInvalido Chave = "erro.papel_invalido"
	// MsgUtilizadorNaoEncontrado — utilizador inexistente no Keycloak.
	MsgUtilizadorNaoEncontrado Chave = "erro.utilizador_nao_encontrado"
```

And to the `mensagensPtAO` map:

```go
	MsgMFAObrigatoria:          "Autenticação com segundo factor obrigatória para este perfil.",
	MsgPapelInvalido:           "Papel inválido.",
	MsgUtilizadorNaoEncontrado: "Utilizador não encontrado.",
```

- [ ] **Step 5: Adicionar o campo à Sessao**

In `internal/domain/identidade/sessao.go`, add the field to the `Sessao` struct (after `Papeis`):

```go
type Sessao struct {
	Sujeito string // keycloak_id (claim "sub")
	Nome    string
	Email   string
	Papeis  []Papel
	// AutenticacaoForte indica que o token comprova segundo factor (MFA),
	// derivado dos claims acr/amr pela camada de adaptadores.
	AutenticacaoForte bool
}
```

- [ ] **Step 6: Criar as regras de MFA**

Create `internal/domain/identidade/mfa.go`:

```go
package identidade

import (
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// ExigeAutenticacaoForte indica se algum dos papéis exige segundo factor (MFA).
// Alinhado com EhSensivel (Director, Admin, DPO, Auditor).
func ExigeAutenticacaoForte(papeis []Papel) bool {
	for _, p := range papeis {
		if EhSensivel(p) {
			return true
		}
	}
	return false
}

// VerificarAutenticacaoForte devolve um ErroDominio de categoria
// MFAObrigatorio se a sessão tiver um papel sensível mas o token não comprovar
// segundo factor. Caso contrário devolve nil. Função pura (Camada 1).
func VerificarAutenticacaoForte(s Sessao) error {
	if ExigeAutenticacaoForte(s.Papeis) && !s.AutenticacaoForte {
		return erros.Novo(erros.CategoriaMFAObrigatorio, i18n.T(i18n.MsgMFAObrigatoria))
	}
	return nil
}
```

- [ ] **Step 7: Adicionar os eventos de gestão**

In `internal/domain/identidade/eventos.go`, add before the conformance block (`var ( _ evento.EventoDominio = … )`):

```go
// PapelAtribuido é emitido quando um administrador atribui um papel.
type PapelAtribuido struct {
	Actor string
	Alvo  string
	Papel Papel
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e PapelAtribuido) NomeEvento() string { return "identidade.papel.atribuido" }

// OcorridoEm implementa evento.EventoDominio.
func (e PapelAtribuido) OcorridoEm() time.Time { return e.Em }

// PapelRevogado é emitido quando um administrador revoga um papel.
type PapelRevogado struct {
	Actor string
	Alvo  string
	Papel Papel
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e PapelRevogado) NomeEvento() string { return "identidade.papel.revogado" }

// OcorridoEm implementa evento.EventoDominio.
func (e PapelRevogado) OcorridoEm() time.Time { return e.Em }

// UtilizadorActivado é emitido quando um administrador activa uma conta.
type UtilizadorActivado struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e UtilizadorActivado) NomeEvento() string { return "identidade.utilizador.activado" }

// OcorridoEm implementa evento.EventoDominio.
func (e UtilizadorActivado) OcorridoEm() time.Time { return e.Em }

// UtilizadorDesactivado é emitido quando um administrador desactiva uma conta.
type UtilizadorDesactivado struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e UtilizadorDesactivado) NomeEvento() string { return "identidade.utilizador.desactivado" }

// OcorridoEm implementa evento.EventoDominio.
func (e UtilizadorDesactivado) OcorridoEm() time.Time { return e.Em }
```

And extend the conformance block:

```go
var (
	_ evento.EventoDominio = UtilizadorAutenticado{}
	_ evento.EventoDominio = PerfilConsultado{}
	_ evento.EventoDominio = AcessoNegado{}
	_ evento.EventoDominio = PapelAtribuido{}
	_ evento.EventoDominio = PapelRevogado{}
	_ evento.EventoDominio = UtilizadorActivado{}
	_ evento.EventoDominio = UtilizadorDesactivado{}
)
```

- [ ] **Step 8: Correr os testes e confirmar que passam**

Run: `go test ./internal/domain/identidade/... ./internal/domain/shared/...`
Expected: PASS.

- [ ] **Step 9: Verificar formato e vet**

Run: `gofmt -l internal/domain && go vet ./internal/domain/...`
Expected: sem output (limpo).

- [ ] **Step 10: Commit**

```bash
git add internal/domain
git commit -m "feat(identidade): regras de MFA, categoria de erro e eventos de gestão

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Aplicação — porta AdminIdentidade, DTOs e casos de uso

**Files:**
- Modify: `internal/application/identidade/ports.go`
- Create: `internal/application/identidade/gerir_utilizadores.go`
- Test: `internal/application/identidade/gerir_utilizadores_test.go`

**Interfaces:**
- Consumes: `dominio.Papel`, `dominio.PapelValido`; `Auditor` (ports.go); `auditoria.Registo`; `erros`, `i18n`.
- Produces:
  - `FiltroUtilizadores{Termo string; Limite int; Deslocamento int}`
  - `ResumoUtilizador{ID, Nome, Email string; Activo bool; Papeis []string}`
  - `DetalheUtilizador` (alias de `ResumoUtilizador`)
  - Porta `AdminIdentidade` (5 métodos, abaixo)
  - `CasoListarUtilizadores`, `CasoObterUtilizador`, `CasoAtribuirPapel`, `CasoRevogarPapel`, `CasoDefinirActivo` + construtores `Novo…`
  - Cada `Caso…` tem método `Executar` (assinaturas na Step 3).

- [ ] **Step 1: Escrever os testes de aplicação (falham)**

Create `internal/application/identidade/gerir_utilizadores_test.go`:

```go
package identidade_test

import (
	"context"
	"errors"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes ---
// NOTA: reutiliza o `fakeAuditor` já definido em identidade_test.go (mesmo
// pacote de teste `identidade_test`) — NÃO o redefinir aqui (colidiria).

type fakeAdmin struct {
	lista     []appident.ResumoUtilizador
	detalhe   appident.DetalheUtilizador
	err       error
	atribuido []string // "alvo:papel"
	revogado  []string
	activo    map[string]bool
}

func (f *fakeAdmin) ListarUtilizadores(context.Context, appident.FiltroUtilizadores) ([]appident.ResumoUtilizador, error) {
	return f.lista, f.err
}
func (f *fakeAdmin) ObterUtilizador(context.Context, string) (appident.DetalheUtilizador, error) {
	return f.detalhe, f.err
}
func (f *fakeAdmin) AtribuirPapel(_ context.Context, id string, p dominio.Papel) error {
	if f.err != nil {
		return f.err
	}
	f.atribuido = append(f.atribuido, id+":"+string(p))
	return nil
}
func (f *fakeAdmin) RevogarPapel(_ context.Context, id string, p dominio.Papel) error {
	if f.err != nil {
		return f.err
	}
	f.revogado = append(f.revogado, id+":"+string(p))
	return nil
}
func (f *fakeAdmin) DefinirActivo(_ context.Context, id string, activo bool) error {
	if f.err != nil {
		return f.err
	}
	if f.activo == nil {
		f.activo = map[string]bool{}
	}
	f.activo[id] = activo
	return nil
}

// --- Testes ---

func TestAtribuirPapel_ValidaEAudita(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoAtribuirPapel(admin, aud)

	if err := caso.Executar(context.Background(), "actor-1", "alvo-1", dominio.PapelMedico); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(admin.atribuido) != 1 || admin.atribuido[0] != "alvo-1:Medico" {
		t.Fatalf("atribuição não delegada: %v", admin.atribuido)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.papel.atribuido" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
	if aud.registos[0].Actor != "actor-1" || aud.registos[0].EntidadeID != "alvo-1" {
		t.Fatalf("auditoria com dados errados: %+v", aud.registos[0])
	}
}

func TestAtribuirPapel_PapelInvalido(t *testing.T) {
	admin := &fakeAdmin{}
	caso := appident.NovoCasoAtribuirPapel(admin, &fakeAuditor{})
	err := caso.Executar(context.Background(), "actor-1", "alvo-1", dominio.Papel("Inexistente"))
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
	if len(admin.atribuido) != 0 {
		t.Fatal("não devia ter delegado com papel inválido")
	}
}

func TestRevogarPapel_Audita(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoRevogarPapel(admin, aud)
	if err := caso.Executar(context.Background(), "actor-1", "alvo-1", dominio.PapelAdmin); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.papel.revogado" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
}

func TestDefinirActivo_AuditaAccaoCorrecta(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoDefinirActivo(admin, aud)

	if err := caso.Executar(context.Background(), "actor-1", "alvo-1", false); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if admin.activo["alvo-1"] != false {
		t.Fatal("estado activo não aplicado")
	}
	if aud.registos[0].Accao != "identidade.utilizador.desactivado" {
		t.Fatalf("acção errada: %q", aud.registos[0].Accao)
	}
}

func TestListarUtilizadores_Delega(t *testing.T) {
	admin := &fakeAdmin{lista: []appident.ResumoUtilizador{{ID: "u1", Nome: "Ana"}}}
	caso := appident.NovoCasoListarUtilizadores(admin)
	out, err := caso.Executar(context.Background(), appident.FiltroUtilizadores{})
	if err != nil || len(out) != 1 || out[0].ID != "u1" {
		t.Fatalf("listagem inesperada: %v, %v", out, err)
	}
}

func TestObterUtilizador_PropagaErro(t *testing.T) {
	admin := &fakeAdmin{err: errors.New("falha")}
	caso := appident.NovoCasoObterUtilizador(admin)
	if _, err := caso.Executar(context.Background(), "u1"); err == nil {
		t.Fatal("esperava erro propagado")
	}
}
```

- [ ] **Step 2: Correr os testes e confirmar que falham**

Run: `go test ./internal/application/identidade/...`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Adicionar a porta AdminIdentidade**

In `internal/application/identidade/ports.go`, add after the `Auditor` interface:

```go
// AdminIdentidade é a porta de saída para a gestão de utilizadores/papéis no
// Keycloak (fonte de verdade). Implementada por adapters/keycloak.AdminCliente.
type AdminIdentidade interface {
	ListarUtilizadores(ctx context.Context, filtro FiltroUtilizadores) ([]ResumoUtilizador, error)
	ObterUtilizador(ctx context.Context, id string) (DetalheUtilizador, error)
	AtribuirPapel(ctx context.Context, id string, papel dominio.Papel) error
	RevogarPapel(ctx context.Context, id string, papel dominio.Papel) error
	DefinirActivo(ctx context.Context, id string, activo bool) error
}
```

- [ ] **Step 4: Criar DTOs e casos de uso**

Create `internal/application/identidade/gerir_utilizadores.go`:

```go
package identidade

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// FiltroUtilizadores parametriza a listagem de utilizadores.
type FiltroUtilizadores struct {
	Termo        string // pesquisa por nome/email/username (opcional)
	Limite       int    // máximo de resultados (0 = default do adaptador)
	Deslocamento int    // paginação
}

// ResumoUtilizador é o DTO de um utilizador na gestão administrativa.
type ResumoUtilizador struct {
	ID     string   `json:"id"`
	Nome   string   `json:"nome"`
	Email  string   `json:"email"`
	Activo bool     `json:"activo"`
	Papeis []string `json:"papeis"`
}

// DetalheUtilizador é o detalhe de um utilizador (mesma forma que o resumo).
type DetalheUtilizador = ResumoUtilizador

// CasoListarUtilizadores lista utilizadores (leitura; sem auditoria).
type CasoListarUtilizadores struct{ admin AdminIdentidade }

// NovoCasoListarUtilizadores constrói o caso de uso.
func NovoCasoListarUtilizadores(a AdminIdentidade) *CasoListarUtilizadores {
	return &CasoListarUtilizadores{admin: a}
}

// Executar devolve a lista de utilizadores segundo o filtro.
func (c *CasoListarUtilizadores) Executar(ctx context.Context, f FiltroUtilizadores) ([]ResumoUtilizador, error) {
	return c.admin.ListarUtilizadores(ctx, f)
}

// CasoObterUtilizador devolve o detalhe de um utilizador (leitura).
type CasoObterUtilizador struct{ admin AdminIdentidade }

// NovoCasoObterUtilizador constrói o caso de uso.
func NovoCasoObterUtilizador(a AdminIdentidade) *CasoObterUtilizador {
	return &CasoObterUtilizador{admin: a}
}

// Executar devolve o detalhe do utilizador com o id indicado.
func (c *CasoObterUtilizador) Executar(ctx context.Context, id string) (DetalheUtilizador, error) {
	return c.admin.ObterUtilizador(ctx, id)
}

// CasoAtribuirPapel atribui um papel a um utilizador e audita a acção.
type CasoAtribuirPapel struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoAtribuirPapel constrói o caso de uso.
func NovoCasoAtribuirPapel(a AdminIdentidade, aud Auditor) *CasoAtribuirPapel {
	return &CasoAtribuirPapel{admin: a, auditor: aud, agora: time.Now}
}

// Executar valida o papel, delega no Keycloak e regista a auditoria.
func (c *CasoAtribuirPapel) Executar(ctx context.Context, actor, id string, papel dominio.Papel) error {
	if !dominio.PapelValido(string(papel)) {
		return erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPapelInvalido))
	}
	if err := c.admin.AtribuirPapel(ctx, id, papel); err != nil {
		return err
	}
	return c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.papel.atribuido",
		Entidade:   "utilizador",
		EntidadeID: id,
		Detalhe:    string(papel),
		OcorridoEm: c.agora(),
	})
}

// CasoRevogarPapel revoga um papel de um utilizador e audita a acção.
type CasoRevogarPapel struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoRevogarPapel constrói o caso de uso.
func NovoCasoRevogarPapel(a AdminIdentidade, aud Auditor) *CasoRevogarPapel {
	return &CasoRevogarPapel{admin: a, auditor: aud, agora: time.Now}
}

// Executar valida o papel, delega no Keycloak e regista a auditoria.
func (c *CasoRevogarPapel) Executar(ctx context.Context, actor, id string, papel dominio.Papel) error {
	if !dominio.PapelValido(string(papel)) {
		return erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPapelInvalido))
	}
	if err := c.admin.RevogarPapel(ctx, id, papel); err != nil {
		return err
	}
	return c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.papel.revogado",
		Entidade:   "utilizador",
		EntidadeID: id,
		Detalhe:    string(papel),
		OcorridoEm: c.agora(),
	})
}

// CasoDefinirActivo activa/desactiva um utilizador e audita a acção.
type CasoDefinirActivo struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoDefinirActivo constrói o caso de uso.
func NovoCasoDefinirActivo(a AdminIdentidade, aud Auditor) *CasoDefinirActivo {
	return &CasoDefinirActivo{admin: a, auditor: aud, agora: time.Now}
}

// Executar aplica o estado no Keycloak e regista a auditoria com a acção
// correspondente (activado/desactivado).
func (c *CasoDefinirActivo) Executar(ctx context.Context, actor, id string, activo bool) error {
	if err := c.admin.DefinirActivo(ctx, id, activo); err != nil {
		return err
	}
	accao := "identidade.utilizador.desactivado"
	if activo {
		accao = "identidade.utilizador.activado"
	}
	return c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      accao,
		Entidade:   "utilizador",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	})
}
```

- [ ] **Step 5: Correr os testes e confirmar que passam**

Run: `go test ./internal/application/identidade/...`
Expected: PASS.

- [ ] **Step 6: Verificar formato/vet**

Run: `gofmt -l internal/application && go vet ./internal/application/...`
Expected: sem output.

- [ ] **Step 7: Commit**

```bash
git add internal/application/identidade
git commit -m "feat(identidade): casos de uso de gestão administrativa de utilizadores

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: HTTP — middleware MFAObrigatoria e mapeamento RFC 7807

**Files:**
- Modify: `internal/adapters/http/middleware_auth.go`
- Modify: `internal/adapters/http/problem.go`
- Test: `internal/adapters/http/admin_test.go` (criado aqui; expandido na Task 4)

**Interfaces:**
- Consumes: `SessaoDe`, `responderErro` (mesmo pacote); `dominio.VerificarAutenticacaoForte`; `erros.CategoriaMFAObrigatorio`; `i18n.MsgMFAObrigatoria`.
- Produces: `MFAObrigatoria() gin.HandlerFunc`; mapeamento `CategoriaMFAObrigatorio` → 403 com `type: /erros/mfa-obrigatorio` em `problem.go`.

- [ ] **Step 1: Escrever os testes do middleware (falham)**

Create `internal/adapters/http/admin_test.go`:

```go
package http_test

import (
	nethttp "net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

func TestMFAObrigatoria_PapelSensivelSemMFA_403(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	sessao := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}, AutenticacaoForte: false}
	r.Use(adhttp.Auth(fakeAuth{sessao: sessao}))
	r.Use(adhttp.MFAObrigatoria())
	r.GET("/x", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	w := pedido(r, "GET", "/x", "Bearer xyz")
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "mfa-obrigatorio") {
		t.Fatalf("esperava type mfa-obrigatorio: %s", w.Body.String())
	}
}

func TestMFAObrigatoria_PapelSensivelComMFA_Prossegue(t *testing.T) {
	r := novoRouter()
	sessao := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}, AutenticacaoForte: true}
	r.Use(adhttp.Auth(fakeAuth{sessao: sessao}))
	r.Use(adhttp.MFAObrigatoria())
	r.GET("/x", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	if w := pedido(r, "GET", "/x", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestMFAObrigatoria_PapelComum_Prossegue(t *testing.T) {
	r := novoRouter()
	sessao := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}
	r.Use(adhttp.Auth(fakeAuth{sessao: sessao}))
	r.Use(adhttp.MFAObrigatoria())
	r.GET("/x", func(c *gin.Context) { c.Status(nethttp.StatusOK) })

	if w := pedido(r, "GET", "/x", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/http/ -run MFAObrigatoria`
Expected: FAIL — `adhttp.MFAObrigatoria` não existe.

- [ ] **Step 3: Implementar o middleware**

In `internal/adapters/http/middleware_auth.go`, add after the `RBAC` function:

```go
// MFAObrigatoria rejeita (403, type mfa-obrigatorio) qualquer sessão com papel
// sensível cujo token não comprove segundo factor. Aplicar logo a seguir a Auth.
func MFAObrigatoria() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessao, ok := SessaoDe(c)
		if !ok {
			responderErro(c, erros.Novo(erros.CategoriaNaoAutorizado, i18n.T(i18n.MsgNaoAutenticado)))
			return
		}
		if err := dominio.VerificarAutenticacaoForte(sessao); err != nil {
			responderErro(c, err)
			return
		}
		c.Next()
	}
}
```

- [ ] **Step 4: Mapear a categoria MFA em problem.go**

In `internal/adapters/http/problem.go`:

Add a `tipoDe` helper and use it in `responderProblema`. Change `responderProblema` signature to accept the category-derived type. Simplest non-breaking approach — compute the type inside `responderErro` and add an internal variant:

Replace the body of `responderErro` with:

```go
func responderErro(c *gin.Context, err error) {
	cat := erros.CategoriaDe(err)
	detalhe := err.Error()
	if cat == erros.CategoriaInterno {
		detalhe = i18n.T(i18n.MsgErroInterno)
	}
	responderProblemaTipo(c, estadoDe(cat), tipoDe(cat), tituloDe(cat), detalhe)
}
```

Add `responderProblemaTipo` (and make `responderProblema` delegate to it so the rate-limit caller keeps working):

```go
// responderProblema mantém a assinatura usada pelo rate limiter (type genérico).
func responderProblema(c *gin.Context, status int, titulo, detalhe string) {
	responderProblemaTipo(c, status, "about:blank", titulo, detalhe)
}

// responderProblemaTipo escreve uma resposta RFC 7807 com um type específico.
func responderProblemaTipo(c *gin.Context, status int, tipo, titulo, detalhe string) {
	instancia := ""
	if v, ok := c.Get("request_id"); ok {
		if s, ok := v.(string); ok {
			instancia = s
		}
	}
	corpo, _ := json.Marshal(Problema{
		Type:     tipo,
		Title:    titulo,
		Status:   status,
		Detail:   detalhe,
		Instance: instancia,
	})
	c.Data(status, "application/problem+json; charset=utf-8", corpo)
	c.Abort()
}
```

Add the `CategoriaMFAObrigatorio` case to `estadoDe` (before `default`):

```go
	case erros.CategoriaMFAObrigatorio:
		return nethttp.StatusForbidden
```

Add the `tituloDe` case (before `default`):

```go
	case erros.CategoriaMFAObrigatorio:
		return i18n.T(i18n.MsgMFAObrigatoria)
```

Add the new `tipoDe` function:

```go
// tipoDe devolve o URI de tipo RFC 7807 para a categoria. Distingue o caso MFA;
// as restantes categorias usam "about:blank" (o título já as identifica).
func tipoDe(cat erros.Categoria) string {
	if cat == erros.CategoriaMFAObrigatorio {
		return "/erros/mfa-obrigatorio"
	}
	return "about:blank"
}
```

- [ ] **Step 5: Correr os testes e confirmar que passam**

Run: `go test ./internal/adapters/http/ -run MFAObrigatoria`
Expected: PASS.

- [ ] **Step 6: Correr toda a suite HTTP (não regrediu)**

Run: `go test ./internal/adapters/http/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/http/middleware_auth.go internal/adapters/http/problem.go internal/adapters/http/admin_test.go
git commit -m "feat(identidade): middleware de imposição de MFA e type RFC 7807 dedicado

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: HTTP — handler de administração

**Files:**
- Create: `internal/adapters/http/admin_handler.go`
- Test: `internal/adapters/http/admin_test.go` (expandir)

**Interfaces:**
- Consumes: `appident.FiltroUtilizadores`, `appident.ResumoUtilizador`, `appident.DetalheUtilizador`; `dominio.Papel`, `dominio.PapelAdmin/Auditor/DPO`; `SessaoDe`, `RBAC`, `responderErro`; `erros`, `i18n`.
- Produces:
  - Interfaces `ServicoListar`, `ServicoObterUtilizador`, `ServicoAtribuirPapel`, `ServicoRevogarPapel`, `ServicoDefinirActivo` (uma por caso de uso).
  - `AdministracaoHandler` + `NovoAdministracaoHandler(listar ServicoListar, obter ServicoObterUtilizador, atribuir ServicoAtribuirPapel, revogar ServicoRevogarPapel, activar ServicoDefinirActivo) *AdministracaoHandler`.
  - `RegistarAdministracao(r gin.IRouter, h *AdministracaoHandler, protecao ...gin.HandlerFunc)`.

- [ ] **Step 1: Escrever os testes do handler (falham)**

Append to `internal/adapters/http/admin_test.go` (add imports `"context"`, `appident "…/internal/application/identidade"` to the existing import block):

```go
// --- Fakes dos serviços de administração ---

type fakeListar struct {
	out []appident.ResumoUtilizador
	err error
}

func (f fakeListar) Executar(context.Context, appident.FiltroUtilizadores) ([]appident.ResumoUtilizador, error) {
	return f.out, f.err
}

type fakeObter struct {
	out appident.DetalheUtilizador
	err error
}

func (f fakeObter) Executar(context.Context, string) (appident.DetalheUtilizador, error) {
	return f.out, f.err
}

type fakePapel struct {
	ultimoActor string
	ultimoAlvo  string
	ultimoPapel dominio.Papel
	err         error
}

func (f *fakePapel) Executar(_ context.Context, actor, id string, p dominio.Papel) error {
	f.ultimoActor, f.ultimoAlvo, f.ultimoPapel = actor, id, p
	return f.err
}

type fakeActivo struct {
	ultimoActivo bool
	err          error
}

func (f *fakeActivo) Executar(_ context.Context, _, _ string, activo bool) error {
	f.ultimoActivo = activo
	return f.err
}

func routerAdmin(sessao dominio.Sessao, atribuir *fakePapel) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoAdministracaoHandler(
		fakeListar{out: []appident.ResumoUtilizador{{ID: "u1", Nome: "Ana"}}},
		fakeObter{out: appident.DetalheUtilizador{ID: "u1", Nome: "Ana"}},
		atribuir,
		&fakePapel{},
		&fakeActivo{},
	)
	adhttp.RegistarAdministracao(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func pedidoCorpo(r nethttp.Handler, metodo, caminho, corpo string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(metodo, caminho, strings.NewReader(corpo))
	req.Header.Set("Authorization", "Bearer xyz")
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestAdmin_Listar_AdminPermitido(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedido(r, "GET", "/api/v1/identidade/utilizadores", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"id":"u1"`) {
		t.Fatalf("corpo inesperado: %s", w.Body.String())
	}
}

func TestAdmin_Listar_AuditorPermitido(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}}, &fakePapel{})
	if w := pedido(r, "GET", "/api/v1/identidade/utilizadores", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("Auditor deve poder listar; obtive %d", w.Code)
	}
}

func TestAdmin_Listar_MedicoProibido(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, &fakePapel{})
	if w := pedido(r, "GET", "/api/v1/identidade/utilizadores", "Bearer xyz"); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Medico não deve listar; obtive %d", w.Code)
	}
}

func TestAdmin_AtribuirPapel_AdminOk(t *testing.T) {
	atribuir := &fakePapel{}
	r := routerAdmin(dominio.Sessao{Sujeito: "actor-1", Papeis: []dominio.Papel{dominio.PapelAdmin}}, atribuir)
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores/u1/papeis", `{"papel":"Medico"}`)
	if w.Code != nethttp.StatusNoContent {
		t.Fatalf("esperava 204, obtive %d (%s)", w.Code, w.Body.String())
	}
	if atribuir.ultimoActor != "actor-1" || atribuir.ultimoAlvo != "u1" || atribuir.ultimoPapel != dominio.PapelMedico {
		t.Fatalf("delegação errada: %+v", atribuir)
	}
}

func TestAdmin_AtribuirPapel_AuditorProibido(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}}, &fakePapel{})
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores/u1/papeis", `{"papel":"Medico"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("Auditor não deve escrever; obtive %d", w.Code)
	}
}

func TestAdmin_AtribuirPapel_CorpoInvalido_400(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores/u1/papeis", `{}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400 para papel em falta; obtive %d", w.Code)
	}
}

func TestAdmin_DesactivarUtilizador_204(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedidoCorpo(r, "PATCH", "/api/v1/identidade/utilizadores/u1", `{"activo":false}`)
	if w.Code != nethttp.StatusNoContent {
		t.Fatalf("esperava 204, obtive %d (%s)", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/http/ -run Admin`
Expected: FAIL — `NovoAdministracaoHandler`/`RegistarAdministracao` não existem.

- [ ] **Step 3: Implementar o handler**

Create `internal/adapters/http/admin_handler.go`:

```go
package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Serviços de administração (casos de uso de application/identidade).
type (
	// ServicoListar lista utilizadores.
	ServicoListar interface {
		Executar(ctx context.Context, f appident.FiltroUtilizadores) ([]appident.ResumoUtilizador, error)
	}
	// ServicoObterUtilizador devolve o detalhe de um utilizador.
	ServicoObterUtilizador interface {
		Executar(ctx context.Context, id string) (appident.DetalheUtilizador, error)
	}
	// ServicoAtribuirPapel atribui um papel.
	ServicoAtribuirPapel interface {
		Executar(ctx context.Context, actor, id string, papel dominio.Papel) error
	}
	// ServicoRevogarPapel revoga um papel.
	ServicoRevogarPapel interface {
		Executar(ctx context.Context, actor, id string, papel dominio.Papel) error
	}
	// ServicoDefinirActivo activa/desactiva um utilizador.
	ServicoDefinirActivo interface {
		Executar(ctx context.Context, actor, id string, activo bool) error
	}
)

// AdministracaoHandler expõe os endpoints de gestão de utilizadores/papéis.
type AdministracaoHandler struct {
	listar   ServicoListar
	obter    ServicoObterUtilizador
	atribuir ServicoAtribuirPapel
	revogar  ServicoRevogarPapel
	activar  ServicoDefinirActivo
}

// NovoAdministracaoHandler constrói o handler com os casos de uso.
func NovoAdministracaoHandler(
	listar ServicoListar,
	obter ServicoObterUtilizador,
	atribuir ServicoAtribuirPapel,
	revogar ServicoRevogarPapel,
	activar ServicoDefinirActivo,
) *AdministracaoHandler {
	return &AdministracaoHandler{listar: listar, obter: obter, atribuir: atribuir, revogar: revogar, activar: activar}
}

// RegistarAdministracao regista as rotas sob /api/v1/identidade/utilizadores. Os
// middlewares `protecao` (rate limit + Auth + MFAObrigatoria) aplicam-se ao grupo;
// o RBAC por papel é aplicado por rota (escrita: Admin; leitura: Admin/Auditor/DPO).
func RegistarAdministracao(r gin.IRouter, h *AdministracaoHandler, protecao ...gin.HandlerFunc) {
	g := r.Group("/api/v1/identidade/utilizadores")
	g.Use(protecao...)

	leitura := RBAC(dominio.PapelAdmin, dominio.PapelAuditor, dominio.PapelDPO)
	escrita := RBAC(dominio.PapelAdmin)

	g.GET("", leitura, h.listarUtilizadores)
	g.GET("/:id", leitura, h.obterUtilizador)
	g.POST("/:id/papeis", escrita, h.atribuirPapel)
	g.DELETE("/:id/papeis/:papel", escrita, h.revogarPapel)
	g.PATCH("/:id", escrita, h.definirActivo)
}

func (h *AdministracaoHandler) listarUtilizadores(c *gin.Context) {
	out, err := h.listar.Executar(c.Request.Context(), appident.FiltroUtilizadores{Termo: c.Query("q")})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *AdministracaoHandler) obterUtilizador(c *gin.Context) {
	out, err := h.obter.Executar(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoPapel struct {
	Papel string `json:"papel"`
}

func (h *AdministracaoHandler) atribuirPapel(c *gin.Context) {
	var corpo corpoPapel
	if err := c.ShouldBindJSON(&corpo); err != nil || corpo.Papel == "" {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPapelInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	if err := h.atribuir.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), dominio.Papel(corpo.Papel)); err != nil {
		responderErro(c, err)
		return
	}
	c.Status(nethttp.StatusNoContent)
}

func (h *AdministracaoHandler) revogarPapel(c *gin.Context) {
	actor, _ := SessaoDe(c)
	if err := h.revogar.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), dominio.Papel(c.Param("papel"))); err != nil {
		responderErro(c, err)
		return
	}
	c.Status(nethttp.StatusNoContent)
}

type corpoActivo struct {
	Activo *bool `json:"activo"`
}

func (h *AdministracaoHandler) definirActivo(c *gin.Context) {
	var corpo corpoActivo
	if err := c.ShouldBindJSON(&corpo); err != nil || corpo.Activo == nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	if err := h.activar.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), *corpo.Activo); err != nil {
		responderErro(c, err)
		return
	}
	c.Status(nethttp.StatusNoContent)
}
```

- [ ] **Step 4: Correr os testes e confirmar que passam**

Run: `go test ./internal/adapters/http/...`
Expected: PASS.

- [ ] **Step 5: Verificar formato/vet**

Run: `gofmt -l internal/adapters/http && go vet ./internal/adapters/http/...`
Expected: sem output.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/http/admin_handler.go internal/adapters/http/admin_test.go
git commit -m "feat(identidade): endpoints HTTP de gestão administrativa com RBAC por rota

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Adaptador Keycloak — derivar AutenticacaoForte de acr/amr

**Files:**
- Modify: `internal/adapters/keycloak/cliente.go`
- Modify: `internal/adapters/keycloak/cliente_internal_test.go`

**Interfaces:**
- Consumes: `dominio.Sessao` (novo campo `AutenticacaoForte`).
- Produces: `keycloak.Novo(ctx, issuer, audiencia string, acrFortes []string) (*Cliente, error)` (**assinatura alterada** — +1 parâmetro); `Cliente.Verificar` passa a preencher `AutenticacaoForte`.

- [ ] **Step 1: Escrever o teste interno (falha)**

Append to `internal/adapters/keycloak/cliente_internal_test.go`:

```go
func TestEhAutenticacaoForte(t *testing.T) {
	c := &Cliente{acrFortes: map[string]bool{"gold": true, "2": true}}
	casos := []struct {
		nome string
		acr  string
		amr  []string
		quer bool
	}{
		{"amr otp", "1", []string{"pwd", "otp"}, true},
		{"amr mfa", "", []string{"mfa"}, true},
		{"acr forte configurado", "gold", nil, true},
		{"acr numerico configurado", "2", nil, true},
		{"so password", "1", []string{"pwd"}, false},
		{"vazio", "", nil, false},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			if got := c.ehAutenticacaoForte(tc.acr, tc.amr); got != tc.quer {
				t.Fatalf("ehAutenticacaoForte(%q,%v)=%v; quer %v", tc.acr, tc.amr, got, tc.quer)
			}
		})
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/keycloak/ -run EhAutenticacaoForte`
Expected: FAIL — `acrFortes`/`ehAutenticacaoForte` não existem.

- [ ] **Step 3: Adaptar o cliente**

In `internal/adapters/keycloak/cliente.go`:

Add fields to the `Cliente` struct:

```go
type Cliente struct {
	verifier  *oidc.IDTokenVerifier
	audiencia string
	discovery string
	http      *nethttp.Client
	acrFortes map[string]bool
}
```

Add `Acr`/`Amr` to `claims`:

```go
type claims struct {
	Sub         string   `json:"sub"`
	Nome        string   `json:"name"`
	Email       string   `json:"email"`
	Azp         string   `json:"azp"`
	Aud         claimAud `json:"aud"`
	Acr         string   `json:"acr"`
	Amr         []string `json:"amr"`
	RealmAccess struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}
```

Change `Novo` to accept and store `acrFortes`:

```go
func Novo(ctx context.Context, issuer, audiencia string, acrFortes []string) (*Cliente, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discovery OIDC em %q: %w", issuer, err)
	}
	verifier := provider.Verifier(&oidc.Config{SkipClientIDCheck: true})
	fortes := make(map[string]bool, len(acrFortes))
	for _, a := range acrFortes {
		fortes[a] = true
	}
	return &Cliente{
		verifier:  verifier,
		audiencia: audiencia,
		discovery: issuer + "/.well-known/openid-configuration",
		http:      &nethttp.Client{Timeout: 5 * time.Second},
		acrFortes: fortes,
	}, nil
}
```

In `Verificar`, set the field when building the `Sessao`:

```go
	return dominio.Sessao{
		Sujeito:           cl.Sub,
		Nome:              cl.Nome,
		Email:             cl.Email,
		Papeis:            dominio.PapeisDe(cl.RealmAccess.Roles),
		AutenticacaoForte: c.ehAutenticacaoForte(cl.Acr, cl.Amr),
	}, nil
```

Add the helper (near `audienceValida`):

```go
// amrFortes são métodos de autenticação que, por si, comprovam segundo factor.
var amrFortes = map[string]bool{
	"otp": true, "totp": true, "mfa": true, "hwk": true, "sms": true, "swk": true,
}

// ehAutenticacaoForte decide se o token comprova segundo factor: qualquer método
// forte em "amr", ou um valor de "acr" na lista configurada (KEYCLOAK_ACR_FORTE).
func (c *Cliente) ehAutenticacaoForte(acr string, amr []string) bool {
	for _, m := range amr {
		if amrFortes[m] {
			return true
		}
	}
	return acr != "" && c.acrFortes[acr]
}
```

- [ ] **Step 4: Correr os testes e confirmar que passam**

Run: `go test ./internal/adapters/keycloak/...`
Expected: PASS (o pacote compila; nota: `platform/app.go` e o teste de integração ainda usam a assinatura antiga — serão actualizados nas Tasks 8 e 10; a compilação desses pacotes só é exercida lá).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/keycloak/cliente.go internal/adapters/keycloak/cliente_internal_test.go
git commit -m "feat(identidade): derivar autenticação forte (MFA) dos claims acr/amr

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Adaptador Keycloak — AdminCliente (Admin REST API)

**Files:**
- Create: `internal/adapters/keycloak/admin.go`
- Test: `internal/adapters/keycloak/admin_internal_test.go`

**Interfaces:**
- Consumes: `dominio.Papel`; `appident.FiltroUtilizadores`, `appident.ResumoUtilizador`, `appident.DetalheUtilizador`; `erros`, `i18n`.
- Produces: `keycloak.NovoAdmin(issuer, clientID, clientSecret string) (*AdminCliente, error)`; `AdminCliente` implementa `appident.AdminIdentidade`; helper interno `dividirIssuer(issuer string) (base, realm string, ok bool)`.

> **Nota de arquitetura:** este adaptador importa `application/identidade` (para os DTOs `ResumoUtilizador`/`FiltroUtilizadores` e satisfazer a porta). Isto é permitido — `adapters` `mayDependOn: application` no `.go-arch-lint.yml`. Sem novo vendor (usa `net/http`, `encoding/json`, `net/url`).

- [ ] **Step 1: Escrever o teste interno (falha)**

Create `internal/adapters/keycloak/admin_internal_test.go`:

```go
package keycloak

import "testing"

func TestDividirIssuer(t *testing.T) {
	casos := []struct {
		issuer    string
		wantBase  string
		wantRealm string
		wantOK    bool
	}{
		{"http://localhost:8081/realms/sgc", "http://localhost:8081", "sgc", true},
		{"https://kc.exemplo.ao/auth/realms/sgc", "https://kc.exemplo.ao/auth", "sgc", true},
		{"http://localhost:8081/realms/", "", "", false},
		{"http://localhost:8081/sem-realm", "", "", false},
		{"", "", "", false},
	}
	for _, c := range casos {
		base, realm, ok := dividirIssuer(c.issuer)
		if base != c.wantBase || realm != c.wantRealm || ok != c.wantOK {
			t.Fatalf("dividirIssuer(%q) = (%q,%q,%v); quer (%q,%q,%v)",
				c.issuer, base, realm, ok, c.wantBase, c.wantRealm, c.wantOK)
		}
	}
}

func TestNovoAdmin_IssuerInvalido(t *testing.T) {
	if _, err := NovoAdmin("http://x/sem-realm", "sgc-admin", "segredo"); err == nil {
		t.Fatal("esperava erro para issuer sem /realms/")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/keycloak/ -run DividirIssuer`
Expected: FAIL — `dividirIssuer`/`NovoAdmin` não existem.

- [ ] **Step 3: Implementar o AdminCliente**

Create `internal/adapters/keycloak/admin.go`:

```go
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
```

- [ ] **Step 4: Correr os testes e confirmar que passam**

Run: `go test ./internal/adapters/keycloak/...`
Expected: PASS.

- [ ] **Step 5: Verificar formato/vet**

Run: `gofmt -l internal/adapters/keycloak && go vet ./internal/adapters/keycloak/...`
Expected: sem output.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/keycloak/admin.go internal/adapters/keycloak/admin_internal_test.go
git commit -m "feat(identidade): cliente da Admin REST API do Keycloak (HTTP puro)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Plataforma — configuração do Keycloak Admin e ACR fortes

**Files:**
- Modify: `internal/platform/config/config.go`
- Modify: `internal/platform/config/config_test.go`

**Interfaces:**
- Produces: `config.Config` ganha `KeycloakAdminClientID string`, `KeycloakAdminClientSecret string`, `KeycloakACRFortes []string`. `KEYCLOAK_ADMIN_CLIENT_ID` e `KEYCLOAK_ADMIN_CLIENT_SECRET` passam a obrigatórios.

- [ ] **Step 1: Atualizar os testes de config (falham)**

In `internal/platform/config/config_test.go`:

Update `TestCarregar_Valido` to set the new required vars and assert defaults:

```go
func TestCarregar_Valido(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/sgc")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("KEYCLOAK_ISSUER", "http://localhost:8081/realms/sgc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "sgc-admin")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "segredo-admin")
	t.Setenv("APP_ENV", "dev")

	cfg, err := config.Carregar()
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if cfg.PortaHTTP != "8080" {
		t.Fatalf("porta por omissão errada: %q", cfg.PortaHTTP)
	}
	if cfg.EmProducao() {
		t.Fatal("dev não deve ser produção")
	}
	if len(cfg.OrigensCORS) != 1 || cfg.OrigensCORS[0] != "*" {
		t.Fatalf("CORS por omissão em dev errado: %v", cfg.OrigensCORS)
	}
	if cfg.LimiteTaxaIP != 100 {
		t.Fatalf("limite de taxa por omissão errado: %d", cfg.LimiteTaxaIP)
	}
	if cfg.KeycloakAdminClientID != "sgc-admin" {
		t.Fatalf("admin client id errado: %q", cfg.KeycloakAdminClientID)
	}
	if len(cfg.KeycloakACRFortes) == 0 {
		t.Fatal("esperava lista de ACR fortes por omissão")
	}
}

func TestCarregar_FaltaAdminClient(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/sgc")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("KEYCLOAK_ISSUER", "http://localhost:8081/realms/sgc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "")
	t.Setenv("APP_ENV", "dev")
	if _, err := config.Carregar(); err == nil {
		t.Fatal("esperava erro por faltar KEYCLOAK_ADMIN_CLIENT_ID/SECRET")
	}
}
```

Also update `TestCarregar_FaltaKeycloakIssuer` and `TestCarregar_AmbienteInvalido` to set the admin vars so they fail only for the reason under test:

```go
func TestCarregar_FaltaKeycloakIssuer(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/sgc")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("KEYCLOAK_ISSUER", "")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "sgc-admin")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "segredo-admin")
	t.Setenv("APP_ENV", "dev")
	if _, err := config.Carregar(); err == nil {
		t.Fatal("esperava erro por faltar KEYCLOAK_ISSUER")
	}
}

func TestCarregar_AmbienteInvalido(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/sgc")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("KEYCLOAK_ISSUER", "http://localhost:8081/realms/sgc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "sgc-admin")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "segredo-admin")
	t.Setenv("APP_ENV", "producao-errada")
	if _, err := config.Carregar(); err == nil {
		t.Fatal("esperava erro por APP_ENV inválido")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/platform/config/...`
Expected: FAIL — campos e validação inexistentes.

- [ ] **Step 3: Adicionar os campos e a validação**

In `internal/platform/config/config.go`:

Add fields to `Config`:

```go
	KeycloakAdminClientID     string   // client confidencial para a Admin API (sgc-admin)
	KeycloakAdminClientSecret string   // segredo do client admin
	KeycloakACRFortes         []string // valores de acr considerados MFA
```

In `Carregar`, populate them:

```go
		KeycloakAdminClientID:     os.Getenv("KEYCLOAK_ADMIN_CLIENT_ID"),
		KeycloakAdminClientSecret: os.Getenv("KEYCLOAK_ADMIN_CLIENT_SECRET"),
		KeycloakACRFortes:         parseLista(valorOu("KEYCLOAK_ACR_FORTE", "mfa,gold,2")),
```

Add validation (after the `KEYCLOAK_ISSUER` check):

```go
	if cfg.KeycloakAdminClientID == "" {
		erro.faltam = append(erro.faltam, "KEYCLOAK_ADMIN_CLIENT_ID")
	}
	if cfg.KeycloakAdminClientSecret == "" {
		erro.faltam = append(erro.faltam, "KEYCLOAK_ADMIN_CLIENT_SECRET")
	}
```

Add the `parseLista` helper (near `parseCORS`):

```go
// parseLista interpreta uma lista separada por vírgulas, ignorando vazios.
func parseLista(raw string) []string {
	partes := strings.Split(raw, ",")
	out := make([]string, 0, len(partes))
	for _, p := range partes {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
```

- [ ] **Step 4: Correr os testes e confirmar que passam**

Run: `go test ./internal/platform/config/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/config
git commit -m "feat(identidade): configuração do client admin do Keycloak e ACR fortes

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Plataforma — fiar tudo no composition root

**Files:**
- Modify: `internal/platform/app.go`

**Interfaces:**
- Consumes: tudo produzido acima (`keycloak.Novo` nova assinatura, `keycloak.NovoAdmin`, casos de uso de gestão, `adhttp.NovoAdministracaoHandler`, `adhttp.RegistarAdministracao`, `adhttp.MFAObrigatoria`).

- [ ] **Step 1: Atualizar app.go**

In `internal/platform/app.go`, inside `ExecutarServidor`:

Change the `keycloak.Novo` call to pass ACR fortes:

```go
	verificador, err := keycloak.Novo(ctx, cfg.KeycloakIssuer, cfg.KeycloakAudNome, cfg.KeycloakACRFortes)
	if err != nil {
		return fmt.Errorf("inicializar Keycloak: %w", err)
	}

	adminKC, err := keycloak.NovoAdmin(cfg.KeycloakIssuer, cfg.KeycloakAdminClientID, cfg.KeycloakAdminClientSecret)
	if err != nil {
		return fmt.Errorf("inicializar Keycloak admin: %w", err)
	}
```

After the existing use cases (`casoPerfil`), add the gestão use cases and handlers:

```go
	casoListar := appident.NovoCasoListarUtilizadores(adminKC)
	casoObter := appident.NovoCasoObterUtilizador(adminKC)
	casoAtribuir := appident.NovoCasoAtribuirPapel(adminKC, repoAuditoria)
	casoRevogar := appident.NovoCasoRevogarPapel(adminKC, repoAuditoria)
	casoActivo := appident.NovoCasoDefinirActivo(adminKC, repoAuditoria)
	handlerAdmin := adhttp.NovoAdministracaoHandler(casoListar, casoObter, casoAtribuir, casoRevogar, casoActivo)
```

Add the MFA middleware next to the others:

```go
	mfaMW := adhttp.MFAObrigatoria()
```

Update `registarRotas` to protect the profile with MFA and register administration:

```go
	registarRotas := func(r gin.IRouter) {
		adhttp.RegistarIdentidade(r, handlerIdentidade, limiteMW, authMW, mfaMW)
		adhttp.RegistarAdministracao(r, handlerAdmin, limiteMW, authMW, mfaMW)
	}
```

- [ ] **Step 2: Compilar todo o módulo**

Run: `go build ./...`
Expected: sem erros.

- [ ] **Step 3: Vet e teste completo (sem integração)**

Run: `go vet ./... && go test ./...`
Expected: PASS (os testes com tag `integration` não correm aqui).

- [ ] **Step 4: Commit**

```bash
git add internal/platform/app.go
git commit -m "feat(identidade): fiar gestão administrativa e MFA no composition root

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Infra — realm Keycloak (client admin, user de teste, OTP)

**Files:**
- Modify: `docker/keycloak/realm-sgc.json`

**Interfaces:**
- Produces: client confidencial `sgc-admin` (service account + roles `realm-management`), user `admin.teste` (papel Admin, sem OTP), e configuração base para OTP condicional.

> **Nota:** o `admin.teste` **sem OTP** é intencional — permite ao smoke test verificar que um papel sensível sem segundo factor recebe 403. O segredo do `sgc-admin` (`segredo-admin`) corresponde ao `.env.example`/compose.

- [ ] **Step 1: Adicionar o client sgc-admin**

In `docker/keycloak/realm-sgc.json`, add to the `clients` array (after the `sgc-api` object — insert a comma):

```json
    {
      "clientId": "sgc-admin",
      "name": "SGC Admin (service account)",
      "enabled": true,
      "protocol": "openid-connect",
      "publicClient": false,
      "secret": "segredo-admin",
      "standardFlowEnabled": false,
      "directAccessGrantsEnabled": false,
      "serviceAccountsEnabled": true
    }
```

- [ ] **Step 2: Conceder ao service account os roles de gestão**

Add a top-level `serviceAccountClientRoles`-equivalent via realm `users` — Keycloak realm import expects the service account user under `users` with `serviceAccountClientId`. Add to the `users` array (after `medico.teste` — insert a comma):

```json
    {
      "username": "service-account-sgc-admin",
      "enabled": true,
      "serviceAccountClientId": "sgc-admin",
      "clientRoles": {
        "realm-management": ["view-users", "query-users", "manage-users"]
      }
    },
    {
      "username": "admin.teste",
      "enabled": true,
      "emailVerified": true,
      "email": "admin.teste@sgc.ao",
      "firstName": "Admin",
      "lastName": "de Teste",
      "credentials": [
        { "type": "password", "value": "teste", "temporary": false }
      ],
      "realmRoles": ["Admin"]
    }
```

- [ ] **Step 3: Validar o JSON**

Run: `python -c "import json,sys; json.load(open(r'docker/keycloak/realm-sgc.json',encoding='utf-8')); print('json ok')"`
Expected: `json ok`.

- [ ] **Step 4: Recriar o Keycloak com o novo realm**

Run:
```bash
docker compose up -d --force-recreate keycloak
```
Wait for health, then confirm the admin client works (obtain a service token):
```bash
curl -s -d 'grant_type=client_credentials&client_id=sgc-admin&client_secret=segredo-admin' \
  http://localhost:8081/realms/sgc/protocol/openid-connect/token | grep -o '"access_token"'
```
Expected: `"access_token"` presente.

> Nota ambiental: o Apache do host ocupa a porta 8080; o Keycloak está mapeado em 8081 (ver `.env.example`/compose). Ajustar o URL se o mapeamento diferir.

- [ ] **Step 5: Commit**

```bash
git add docker/keycloak/realm-sgc.json
git commit -m "feat(identidade): client admin e utilizador Admin de teste no realm sgc

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Integração — smoke tests e2e de login e gestão

**Files:**
- Modify: `tests/integration/identidade_test.go` (atualizar chamada `keycloak.Novo`)
- Create: `tests/integration/administracao_test.go`

**Interfaces:**
- Consumes: `keycloak.Novo` (nova assinatura), `keycloak.NovoAdmin`, `appident` use cases, helpers `issuerTeste`, `ligar` (já existentes nos testes de integração).

- [ ] **Step 0: Corrigir a chamada `keycloak.Novo` no teste de integração existente**

The Sprint 2 test `tests/integration/identidade_test.go` still calls `keycloak.Novo` with 3 args and won't compile under `-tags=integration` after Task 5. Update the call (around line 75):

```go
	verificador, err := keycloak.Novo(ctx, issuer, "sgc-api", []string{"mfa", "gold", "2"})
```

- [ ] **Step 1: Escrever os smoke tests**

Create `tests/integration/administracao_test.go`:

```go
//go:build integration

package integration_test

import (
	"context"
	"net/url"
	nethttp "net/http"
	"encoding/json"
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
```

- [ ] **Step 2: Correr os smoke tests (compose a correr)**

Run:
```bash
DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
KEYCLOAK_ISSUER=http://localhost:8081/realms/sgc \
go test -tags=integration ./tests/integration/... -run 'MFA|Admin' -v
```
Expected: PASS (ou SKIP se o Keycloak/PG não estiverem acessíveis — nunca FAIL por indisponibilidade).

- [ ] **Step 3: Commit**

```bash
git add tests/integration/administracao_test.go
git commit -m "test(identidade): smoke tests e2e de MFA e gestão via Keycloak

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Documentação, config de exemplo e verificação final

**Files:**
- Create: `adrs/ADR-022-mfa-gestao-admin.md`
- Modify: `SPRINT.md`
- Modify: `CLAUDE.md`
- Modify: `.env.example`

**Interfaces:** nenhuma (documentação/config).

- [ ] **Step 1: Escrever a ADR-022**

Create `adrs/ADR-022-mfa-gestao-admin.md`:

```markdown
# ADR-022 — MFA para papéis sensíveis e gestão administrativa via Keycloak

- **Estado**: Aceite
- **Data**: 2026-07-11
- **Marco/Sprint**: M1 / Sprint 3
- **Contexto BC**: Identidade

## Contexto

O Sprint 2 estabeleceu autenticação OIDC, RBAC e auditoria. Faltavam, para os
critérios de saída M1: imposição de MFA para papéis sensíveis e gestão
administrativa de utilizadores/papéis.

## Decisão

1. **Fonte de verdade dos papéis: Keycloak (Admin REST API).** Os endpoints de
   gestão escrevem no Keycloak via um client confidencial (`sgc-admin`,
   `client_credentials`). A BD local mantém-se espelho JIT.
2. **Imposição de MFA em dupla camada.** O realm exige OTP para papéis sensíveis;
   a API rejeita (403, `type: /erros/mfa-obrigatorio`) qualquer sessão com papel
   sensível cujo token não comprove segundo factor. A força de autenticação é
   derivada dos claims `amr` (métodos: otp/totp/mfa/hwk/sms/swk) ou `acr` (lista
   configurável `KEYCLOAK_ACR_FORTE`).
3. **Autorização dos endpoints de gestão.** Escrita: apenas `Admin`. Leitura:
   `Admin`, `Auditor`, `DPO` (auditoria/conformidade, menor privilégio).
4. **Adaptador HTTP puro** (`net/http`), sem nova dependência.

## Consequências

- Coerência com o Sprint 2 (Keycloak como fonte única); a atribuição de papel só
  reflecte no espelho local no próximo login/consulta do visado.
- O `admin.teste` sem OTP serve de caso negativo verificável nos smoke tests.
- Alternativas descartadas: gestão de papéis na BD local (divergiria do token);
  MFA só por configuração de realm (não testável na API); biblioteca gocloak
  (dependência desnecessária).
```

- [ ] **Step 2: Atualizar SPRINT.md**

In `SPRINT.md`, change the header line `- **Sprint**: 2 …` to reference Sprint 3 as entregue and add a "Sprint 3 — entregue" section above "Sprint 2 — entregue":

```markdown
- **Sprint**: 3 (BC Identidade — MFA + gestão administrativa) — **entregue**

## Sprint 3 — entregue

- [x] Imposição de MFA para papéis sensíveis (Director, Admin, DPO, Auditor):
      `Sessao.AutenticacaoForte` derivada de `acr`/`amr`, middleware
      `MFAObrigatoria` → 403 (`type: /erros/mfa-obrigatorio`).
- [x] Gestão administrativa via Admin REST API do Keycloak: listar, ver,
      atribuir/revogar papel, activar/desactivar (adaptador HTTP puro).
- [x] RBAC por rota: escrita só Admin; leitura Admin/Auditor/DPO. Auditoria de
      todas as escritas.
- [x] Realm: client `sgc-admin` (service account) + utilizador `admin.teste`.
- [x] Smoke tests e2e (MFA negativo + fluxo de atribuição via Keycloak).
- [x] ADR-022.
```

Also update the M1 exit criterion line for MFA:

```markdown
- [x] Identidade Keycloak operacional (login, 11 papéis, MFA para papéis sensíveis). — Sprint 2/3
```

- [ ] **Step 3: Atualizar CLAUDE.md**

In `CLAUDE.md`, in the final "Convenções-fonte" block, add ADR-022 to the registered list and bump the next ADR:

```markdown
80 documentos + 19 ADRs). ADRs registadas: `adrs/ADR-020-fundacao-m1.md`,
`adrs/ADR-021-identidade-oidc-rbac.md`, `adrs/ADR-022-mfa-gestao-admin.md`.
Próximo ADR: **ADR-023**.
```

- [ ] **Step 4: Atualizar .env.example**

In `.env.example`, after the `KEYCLOAK_AUDIENCE` line, add:

```bash
# Client confidencial para a Admin REST API do Keycloak (gestão de utilizadores/
# papéis). Service account com roles realm-management (view-users, manage-users).
KEYCLOAK_ADMIN_CLIENT_ID=sgc-admin
KEYCLOAK_ADMIN_CLIENT_SECRET=segredo-admin

# Valores do claim "acr" considerados autenticação forte (MFA), separados por
# vírgulas. Por omissão: mfa,gold,2.
KEYCLOAK_ACR_FORTE=mfa,gold,2
```

- [ ] **Step 5: Verificação final completa**

Run each and confirm:
```bash
gofmt -l internal cmd tests            # sem output
go vet ./...                            # sem erros
go build ./...                          # sem erros
go test ./...                           # PASS
```

- [ ] **Step 6: Verificar os gates de cobertura**

Run:
```bash
go test -cover ./internal/domain/identidade/... ./internal/application/identidade/... ./internal/adapters/http/... ./internal/adapters/keycloak/...
```
Expected: domínio ≥85%, aplicação ≥75%, adaptadores ≥60%. Se algum adaptador ficar <60%, acrescentar um teste de tabela ao pacote em falta antes de continuar.

- [ ] **Step 7: Commit final**

```bash
git add adrs/ADR-022-mfa-gestao-admin.md SPRINT.md CLAUDE.md .env.example
git commit -m "docs(identidade): ADR-022 e reconciliação de docs do Sprint 3

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Notas de verificação global (após todas as tasks)

1. `go build ./...`, `go vet ./...`, `gofmt -l` limpos; `go test ./...` verde.
2. Cobertura cumpre 85/75/60.
3. Com o compose a correr: token `medico.teste` → `/perfil` 200; token `admin.teste`
   (sem OTP) → qualquer rota protegida 403 MFA.
4. `POST /utilizadores/:id/papeis` como Admin → 204 + `auditoria_eventos`
   (`identidade.papel.atribuido`); leitura confirma o papel.
5. Auditor: `GET /utilizadores` 200; `POST .../papeis` 403.
6. `go-arch-lint` (em CI/Linux) sem violações — sem vendor novo; domínio sem infra.
7. `gosec` sem findings (os `#nosec G107` do adaptador admin estão justificados).
```
