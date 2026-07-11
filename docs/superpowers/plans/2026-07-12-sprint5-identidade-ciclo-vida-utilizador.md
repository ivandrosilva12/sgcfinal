# Sprint 5 — Ciclo de vida do utilizador Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Completar o ciclo de vida do utilizador — reset de password/OTP (admin), edição de perfil self-service (telefone/BI), revogação de sessões na desactivação, e compensação da criação não-atómica.

**Architecture:** Fatia vertical Clean Architecture (Domínio → Aplicação → Adaptadores → Plataforma). Novas operações na porta `AdminIdentidade` (Keycloak Admin REST API, fonte de verdade) e no `RepositorioUtilizadores` (perfil local). Novos casos de uso; handlers HTTP sobre o grupo de administração e o grupo de perfil.

**Tech Stack:** Go 1.25, Gin, go-oidc, pgx v5, Keycloak 25. Sem novas dependências.

## Global Constraints

- **Idioma**: PT-PT angolano em TODO o output (código, comentários, commits, mensagens, JSON). Nunca EN/PT-BR.
- **Linguagem ubíqua**: Utilizador, Papel, Sessão, Perfil, Auditoria. Nunca Patient/Role/Session.
- **Regra de dependência**: `internal/domain/**` não importa infra. Aplicação importa só domínio + Shared Kernel (`erros`, `i18n`, `auditoria`, `identity`) + stdlib. `go-arch-lint` impõe em CI.
- **Sem `panic()`** fora de inicialização — sempre `error` categorizado (`erros.ErroDominio`).
- **Erros HTTP**: RFC 7807 (`application/problem+json`), mensagens via `i18n.T` ou mensagens PT-PT de domínio.
- **Cobertura (gate CI, agregado por camada, `scripts/cobertura.sh`)**: domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
- **Módulo Go**: `github.com/ivandrosilva12/sgcfinal`.
- **Commits**: Conventional Commits PT-PT (`feat(identidade): …`), terminando com:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- **Fonte de verdade**: Keycloak (utilizadores/papéis/credenciais/sessões). BD local guarda o perfil operacional (telefone/BI).
- **Módulo de trabalho**: raiz `C:\Users\PC\Documents\RMPRO 2026\Software Clinicas Final` (Git Bash, Windows). Stack compose a correr; Keycloak `localhost:8081`, Postgres `localhost:5432`. Serviço admin `sgc-admin`/`segredo-admin`.

### Nota sobre crescimento de interfaces (ler antes de começar)
Duas interfaces crescem neste sprint:
- `application/identidade.AdminIdentidade` ganha 4 métodos (`DefinirPasswordTemporaria`, `ResetOTP`, `RevogarSessoes`, `ApagarUtilizador`).
- `domain/identidade.RepositorioUtilizadores` ganha 1 método (`AtualizarContacto`).

Todos os implementadores têm de acompanhar, ou a compilação quebra:
- `AdminIdentidade`: o adaptador real `keycloak.AdminCliente` (Task 4) **e** os fakes de teste `fakeAdmin` (`gerir_utilizadores_test.go`) e `fakeCriador` (`criar_utilizador_test.go`) — Task 2 adiciona-lhes os métodos.
- `RepositorioUtilizadores`: o adaptador real `pgrepo.RepositorioUtilizadores` (Task 5) **e** o fake `fakeRepo` (`identidade_test.go`) — Task 2 adiciona-lhe o método.

Por causa disto, o **build do módulo inteiro só volta a passar na Task 5** (adaptadores completos) e depois na Task 7 (após os handlers mudarem de assinatura na Task 6). Cada task indica exactamente o que correr.

---

## Ficheiros (mapa)

| Ação | Ficheiro | Responsabilidade |
|---|---|---|
| Modificar | `internal/domain/identidade/utilizador.go` | Método `AtualizarContacto` |
| Modificar | `internal/domain/identidade/repositorio.go` | `AtualizarContacto` na interface |
| Modificar | `internal/domain/identidade/eventos.go` | 4 eventos de ciclo de vida |
| Modificar | `internal/application/identidade/ports.go` | 4 métodos `AdminIdentidade` + DTO `CredencialReposta` |
| Criar | `internal/application/identidade/reset_credenciais.go` | `CasoResetPassword`, `CasoResetOTP` |
| Criar | `internal/application/identidade/reset_credenciais_test.go` | Testes |
| Criar | `internal/application/identidade/atualizar_perfil.go` | `CasoAtualizarPerfil` |
| Criar | `internal/application/identidade/atualizar_perfil_test.go` | Testes |
| Modificar | `internal/application/identidade/gerir_utilizadores.go` | `CasoDefinirActivo` revoga sessões |
| Modificar | `internal/application/identidade/gerir_utilizadores_test.go` | Stubs/captura no `fakeAdmin` + teste revogação |
| Modificar | `internal/application/identidade/criar_utilizador_test.go` | Stubs no `fakeCriador` |
| Modificar | `internal/application/identidade/identidade_test.go` | `AtualizarContacto` no `fakeRepo` |
| Modificar | `internal/adapters/keycloak/admin.go` | 4 métodos + compensação no `CriarUtilizador` |
| Modificar | `internal/adapters/keycloak/admin_httptest_test.go` | Testes dos 4 métodos + compensação |
| Modificar | `internal/adapters/pgrepo/identidade_repo.go` | `AtualizarContacto` |
| Modificar | `internal/adapters/http/admin_handler.go` | Handlers reset + rotas |
| Modificar | `internal/adapters/http/admin_test.go` | Testes + fakes de reset |
| Modificar | `internal/adapters/http/identidade_handler.go` | Handler `PATCH /perfil` |
| Modificar | `internal/adapters/http/identidade_test.go` | Teste de edição de perfil |
| Modificar | `internal/platform/app.go` | Fiar novos casos de uso |
| Criar | `tests/integration/ciclo_vida_test.go` | e2e reset/perfil/sessões |
| Criar | `adrs/ADR-024-ciclo-vida-utilizador.md` | Decisão |
| Modificar | `SPRINT.md`, `CLAUDE.md` | Docs |

---

### Task 1: Domínio — AtualizarContacto, interface do repositório e eventos

**Files:**
- Modify: `internal/domain/identidade/utilizador.go`
- Modify: `internal/domain/identidade/repositorio.go`
- Modify: `internal/domain/identidade/eventos.go`
- Test: `internal/domain/identidade/utilizador_test.go` (adicionar casos)

**Interfaces:**
- Consumes: `identity.NovoTelefone`, `identity.NovoBI`, `erros`.
- Produces: `(*Utilizador).AtualizarContacto(telefone, bi string) error`; `RepositorioUtilizadores.AtualizarContacto(ctx, keycloakID, telefone, bi string) error`; eventos `PasswordReposta`/`OtpReposto`/`SessoesRevogadas`{Actor, Alvo string, Em time.Time}, `PerfilActualizado`{Sujeito string, Em time.Time}.

- [ ] **Step 1: Escrever os testes de domínio (falham)**

Append to `internal/domain/identidade/utilizador_test.go` (create the file with this content if it does not exist; if it exists, add these functions and reuse its `package identidade_test` + imports):

```go
func TestAtualizarContacto_Valido(t *testing.T) {
	u := &dominio.Utilizador{KeycloakID: "id-1", Nome: "Ana", Email: "ana@sgc.ao", Activo: true}
	if err := u.AtualizarContacto("+244 923 456 789", "00123456LA042"); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if u.Telefone != "+244 923 456 789" {
		t.Fatalf("telefone normalizado errado: %q", u.Telefone)
	}
	if u.BI != "00123456LA042" {
		t.Fatalf("BI normalizado errado: %q", u.BI)
	}
}

func TestAtualizarContacto_LimpaComVazio(t *testing.T) {
	u := &dominio.Utilizador{KeycloakID: "id-1", Telefone: "+244 923 456 789", BI: "00123456LA042"}
	if err := u.AtualizarContacto("", ""); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if u.Telefone != "" || u.BI != "" {
		t.Fatalf("esperava campos limpos, obtive tel=%q bi=%q", u.Telefone, u.BI)
	}
}

func TestAtualizarContacto_TelefoneInvalido(t *testing.T) {
	u := &dominio.Utilizador{KeycloakID: "id-1"}
	err := u.AtualizarContacto("123", "")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestAtualizarContacto_BIInvalido(t *testing.T) {
	u := &dominio.Utilizador{KeycloakID: "id-1"}
	err := u.AtualizarContacto("", "invalido")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}
```

If you create the file, use this header:
```go
package identidade_test

import (
	"testing"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/identidade/ -run AtualizarContacto`
Expected: FAIL — `AtualizarContacto` não existe.

- [ ] **Step 3: Implementar AtualizarContacto no agregado**

In `internal/domain/identidade/utilizador.go`, add after `TemAlgumPapel`:

```go
// AtualizarContacto valida e define o telefone e o BI (campos de perfil local).
// Uma string vazia limpa o campo correspondente. Devolve um ErroDominio de
// categoria Validação se algum valor for inválido. Não altera nome/email/papéis.
func (u *Utilizador) AtualizarContacto(telefone, bi string) error {
	tel := ""
	if s := strings.TrimSpace(telefone); s != "" {
		t, err := identity.NovoTelefone(s)
		if err != nil {
			return erros.Novo(erros.CategoriaValidacao, "telefone inválido")
		}
		tel = t.String()
	}
	doc := ""
	if s := strings.TrimSpace(bi); s != "" {
		b, err := identity.NovoBI(s)
		if err != nil {
			return erros.Novo(erros.CategoriaValidacao, "bilhete de identidade inválido")
		}
		doc = b.String()
	}
	u.Telefone = tel
	u.BI = doc
	return nil
}
```

(`utilizador.go` já importa `strings`, `identity` e `erros` — nenhum import novo.)

- [ ] **Step 4: Adicionar AtualizarContacto à interface do repositório**

In `internal/domain/identidade/repositorio.go`, add to the `RepositorioUtilizadores` interface (after `GuardarComPapeis`):

```go
	// AtualizarContacto persiste os campos de perfil local (telefone/BI) do
	// utilizador com o keycloak_id indicado. Devolve NaoEncontrado se a linha
	// não existir. Strings vazias limpam o campo.
	AtualizarContacto(ctx context.Context, keycloakID, telefone, bi string) error
```

- [ ] **Step 5: Adicionar os eventos**

In `internal/domain/identidade/eventos.go`, add before the conformance `var (...)` block:

```go
// PasswordReposta é emitido quando um administrador repõe a password.
type PasswordReposta struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e PasswordReposta) NomeEvento() string { return "identidade.password.reposta" }

// OcorridoEm implementa evento.EventoDominio.
func (e PasswordReposta) OcorridoEm() time.Time { return e.Em }

// OtpReposto é emitido quando um administrador repõe o OTP.
type OtpReposto struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e OtpReposto) NomeEvento() string { return "identidade.otp.reposto" }

// OcorridoEm implementa evento.EventoDominio.
func (e OtpReposto) OcorridoEm() time.Time { return e.Em }

// SessoesRevogadas é emitido quando as sessões de um utilizador são revogadas.
type SessoesRevogadas struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e SessoesRevogadas) NomeEvento() string { return "identidade.sessoes.revogadas" }

// OcorridoEm implementa evento.EventoDominio.
func (e SessoesRevogadas) OcorridoEm() time.Time { return e.Em }

// PerfilActualizado é emitido quando um utilizador actualiza o seu perfil.
type PerfilActualizado struct {
	Sujeito string
	Em      time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e PerfilActualizado) NomeEvento() string { return "identidade.perfil.actualizado" }

// OcorridoEm implementa evento.EventoDominio.
func (e PerfilActualizado) OcorridoEm() time.Time { return e.Em }
```

And extend the conformance `var (...)` block with:
```go
	_ evento.EventoDominio = PasswordReposta{}
	_ evento.EventoDominio = OtpReposto{}
	_ evento.EventoDominio = SessoesRevogadas{}
	_ evento.EventoDominio = PerfilActualizado{}
```

- [ ] **Step 6: Correr os testes de domínio**

Run: `go test ./internal/domain/identidade/... ./internal/domain/shared/...`
Expected: PASS.

- [ ] **Step 7: gofmt/vet e commit**

Run: `gofmt -l internal/domain/identidade && go vet ./internal/domain/identidade/...`
Expected: sem output.

> Nota: adicionar `AtualizarContacto` à interface `RepositorioUtilizadores` quebra o build do módulo (o `fakeRepo` e o `pgrepo` ainda não o implementam). É esperado; corrige-se nas Tasks 2 e 5. Aqui verifica-se só o domínio.

```bash
git add internal/domain/identidade
git commit -m "feat(identidade): AtualizarContacto no agregado/repositório e eventos de ciclo de vida

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Aplicação — porta AdminIdentidade, reset de credenciais e stubs de fakes

**Files:**
- Modify: `internal/application/identidade/ports.go`
- Create: `internal/application/identidade/reset_credenciais.go`
- Test: `internal/application/identidade/reset_credenciais_test.go`
- Modify: `internal/application/identidade/gerir_utilizadores_test.go` (stubs+captura no `fakeAdmin`)
- Modify: `internal/application/identidade/criar_utilizador_test.go` (stubs no `fakeCriador`)
- Modify: `internal/application/identidade/identidade_test.go` (`AtualizarContacto` no `fakeRepo`)

**Interfaces:**
- Consumes: `AdminIdentidade`, `Auditor`, `gerarSenhaTemporaria` (Sprint 4), `auditoria.Registo`.
- Produces:
  - `AdminIdentidade` ganha: `DefinirPasswordTemporaria(ctx, id, senha string) error`, `ResetOTP(ctx, id string) error`, `RevogarSessoes(ctx, id string) error`, `ApagarUtilizador(ctx, id string) error`.
  - DTO `CredencialReposta{SenhaTemporaria string}` (json `senha_temporaria`).
  - `CasoResetPassword` + `NovoCasoResetPassword(a AdminIdentidade, aud Auditor)`; `Executar(ctx, actor, id string) (CredencialReposta, error)`.
  - `CasoResetOTP` + `NovoCasoResetOTP(a AdminIdentidade, aud Auditor)`; `Executar(ctx, actor, id string) error`.

- [ ] **Step 1: Escrever os testes de reset (falham)**

Create `internal/application/identidade/reset_credenciais_test.go`:

```go
package identidade_test

import (
	"context"
	"errors"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
)

func TestResetPassword_GeraEAudita(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoResetPassword(admin, aud)

	out, err := caso.Executar(context.Background(), "actor-1", "alvo-1")
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if out.SenhaTemporaria == "" {
		t.Fatal("esperava senha temporária não vazia")
	}
	if admin.passwordDefinida["alvo-1"] != out.SenhaTemporaria {
		t.Fatalf("senha passada ao adaptador != devolvida: %v", admin.passwordDefinida)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.password.reposta" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
}

func TestResetPassword_PropagaErro(t *testing.T) {
	admin := &fakeAdmin{err: errors.New("kc down")}
	caso := appident.NovoCasoResetPassword(admin, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "actor-1", "alvo-1"); err == nil {
		t.Fatal("esperava erro propagado")
	}
}

func TestResetOTP_Audita(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoResetOTP(admin, aud)
	if err := caso.Executar(context.Background(), "actor-1", "alvo-1"); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if !admin.otpReposto["alvo-1"] {
		t.Fatal("esperava reset de OTP delegado")
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.otp.reposto" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/identidade/...`
Expected: FAIL — símbolos inexistentes E o pacote não compila (fakes não satisfazem as interfaces crescidas). Ambos serão resolvidos nos passos seguintes.

- [ ] **Step 3: Crescer a porta e adicionar o DTO**

In `internal/application/identidade/ports.go`, add to the `AdminIdentidade` interface (after `CriarUtilizador`):

```go
	DefinirPasswordTemporaria(ctx context.Context, id, senha string) error
	ResetOTP(ctx context.Context, id string) error
	RevogarSessoes(ctx context.Context, id string) error
	ApagarUtilizador(ctx context.Context, id string) error
```

And add the DTO (near the other DTOs):
```go
// CredencialReposta é a saída de um reset de password: a nova senha temporária,
// devolvida uma única vez.
type CredencialReposta struct {
	SenhaTemporaria string `json:"senha_temporaria"`
}
```

- [ ] **Step 4: Criar os casos de uso de reset**

Create `internal/application/identidade/reset_credenciais.go`:

```go
package identidade

import (
	"context"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoResetPassword repõe a password de um utilizador com uma nova senha
// temporária gerada, delegando no Keycloak e auditando.
type CasoResetPassword struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoResetPassword constrói o caso de uso.
func NovoCasoResetPassword(a AdminIdentidade, aud Auditor) *CasoResetPassword {
	return &CasoResetPassword{admin: a, auditor: aud, agora: time.Now}
}

// Executar gera uma nova senha temporária, define-a no Keycloak, audita e
// devolve-a (uma única vez).
func (c *CasoResetPassword) Executar(ctx context.Context, actor, id string) (CredencialReposta, error) {
	senha, err := gerarSenhaTemporaria()
	if err != nil {
		return CredencialReposta{}, err
	}
	if err := c.admin.DefinirPasswordTemporaria(ctx, id, senha); err != nil {
		return CredencialReposta{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.password.reposta",
		Entidade:   "utilizador",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	}); err != nil {
		return CredencialReposta{}, err
	}
	return CredencialReposta{SenhaTemporaria: senha}, nil
}

// CasoResetOTP remove o 2º factor de um utilizador (re-inscrição forçada) e audita.
type CasoResetOTP struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoResetOTP constrói o caso de uso.
func NovoCasoResetOTP(a AdminIdentidade, aud Auditor) *CasoResetOTP {
	return &CasoResetOTP{admin: a, auditor: aud, agora: time.Now}
}

// Executar delega a reposição de OTP no Keycloak e regista a auditoria.
func (c *CasoResetOTP) Executar(ctx context.Context, actor, id string) error {
	if err := c.admin.ResetOTP(ctx, id); err != nil {
		return err
	}
	return c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.otp.reposto",
		Entidade:   "utilizador",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	})
}
```

- [ ] **Step 5: Adicionar métodos (com captura) ao `fakeAdmin`**

In `internal/application/identidade/gerir_utilizadores_test.go`, add capture fields to the `fakeAdmin` struct (extend the struct literal fields) and the 4 methods. Add these fields to the struct:
```go
	passwordDefinida map[string]string
	otpReposto       map[string]bool
	sessoesRevogadas []string
	apagados         []string
```
And these methods:
```go
func (f *fakeAdmin) DefinirPasswordTemporaria(_ context.Context, id, senha string) error {
	if f.err != nil {
		return f.err
	}
	if f.passwordDefinida == nil {
		f.passwordDefinida = map[string]string{}
	}
	f.passwordDefinida[id] = senha
	return nil
}
func (f *fakeAdmin) ResetOTP(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	if f.otpReposto == nil {
		f.otpReposto = map[string]bool{}
	}
	f.otpReposto[id] = true
	return nil
}
func (f *fakeAdmin) RevogarSessoes(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.sessoesRevogadas = append(f.sessoesRevogadas, id)
	return nil
}
func (f *fakeAdmin) ApagarUtilizador(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.apagados = append(f.apagados, id)
	return nil
}
```

- [ ] **Step 6: Adicionar stubs ao `fakeCriador`**

In `internal/application/identidade/criar_utilizador_test.go`, add to `fakeCriador` the 4 no-op stubs (it only needs to satisfy the interface):
```go
func (f *fakeCriador) DefinirPasswordTemporaria(context.Context, string, string) error { return nil }
func (f *fakeCriador) ResetOTP(context.Context, string) error                          { return nil }
func (f *fakeCriador) RevogarSessoes(context.Context, string) error                    { return nil }
func (f *fakeCriador) ApagarUtilizador(context.Context, string) error                  { return nil }
```

- [ ] **Step 7: Adicionar AtualizarContacto (com captura) ao `fakeRepo`**

In `internal/application/identidade/identidade_test.go`, add to `fakeRepo` a capture field and the method:
```go
	// (adicionar ao struct fakeRepo)
	atualizarErr error
```
```go
func (f *fakeRepo) AtualizarContacto(_ context.Context, _, telefone, bi string) error {
	if f.atualizarErr != nil {
		return f.atualizarErr
	}
	if f.guardado != nil {
		f.guardado.Telefone = telefone
		f.guardado.BI = bi
	}
	return nil
}
```

- [ ] **Step 8: Correr os testes e confirmar que passam**

Run: `go test ./internal/application/identidade/...`
Expected: PASS (reset tests + existentes; o pacote compila com as interfaces crescidas via stubs).

> Nota: `go build ./...` do módulo inteiro AINDA falha (o `keycloak.AdminCliente` e o `pgrepo` não implementam os novos métodos — Tasks 4 e 5). Verifica só o pacote de aplicação.

- [ ] **Step 9: gofmt/vet e commit**

Run: `gofmt -l internal/application/identidade && go vet ./internal/application/identidade/...`
Expected: sem output.

```bash
git add internal/application/identidade/ports.go internal/application/identidade/reset_credenciais.go internal/application/identidade/reset_credenciais_test.go internal/application/identidade/gerir_utilizadores_test.go internal/application/identidade/criar_utilizador_test.go internal/application/identidade/identidade_test.go
git commit -m "feat(identidade): porta e casos de uso de reset de credenciais

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Aplicação — edição de perfil e revogação de sessões na desactivação

**Files:**
- Create: `internal/application/identidade/atualizar_perfil.go`
- Test: `internal/application/identidade/atualizar_perfil_test.go`
- Modify: `internal/application/identidade/gerir_utilizadores.go`
- Test: `internal/application/identidade/gerir_utilizadores_test.go` (asserção de revogação)

**Interfaces:**
- Consumes: `dominio.RepositorioUtilizadores`, `Auditor`, `dominio.NovoUtilizador`, `paraPerfil` (obter_perfil.go), `Perfil`.
- Produces: `CasoAtualizarPerfil` + `NovoCasoAtualizarPerfil(r dominio.RepositorioUtilizadores, a Auditor)`; `Executar(ctx, s dominio.Sessao, telefone, bi *string) (Perfil, error)`.

- [ ] **Step 1: Escrever os testes (falham)**

Create `internal/application/identidade/atualizar_perfil_test.go`:

```go
package identidade_test

import (
	"context"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func ptr(s string) *string { return &s }

func TestAtualizarPerfil_DefineContacto(t *testing.T) {
	repo := &fakeRepo{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoAtualizarPerfil(repo, aud)

	perfil, err := caso.Executar(context.Background(), novaSessao(), ptr("+244 923 456 789"), ptr("00123456LA042"))
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if perfil.Telefone != "+244 923 456 789" || perfil.BI != "00123456LA042" {
		t.Fatalf("perfil não reflecte o contacto: %+v", perfil)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.perfil.actualizado" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
}

func TestAtualizarPerfil_TelefoneInvalido(t *testing.T) {
	caso := appident.NovoCasoAtualizarPerfil(&fakeRepo{}, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), novaSessao(), ptr("123"), nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestAtualizarPerfil_OmitidoPreserva(t *testing.T) {
	repo := &fakeRepo{}
	caso := appident.NovoCasoAtualizarPerfil(repo, &fakeAuditor{})
	// Primeiro define um telefone.
	if _, err := caso.Executar(context.Background(), novaSessao(), ptr("+244 923 456 789"), nil); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Agora actualiza só o BI; telefone omitido (nil) deve preservar-se.
	perfil, err := caso.Executar(context.Background(), novaSessao(), nil, ptr("00123456LA042"))
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if perfil.Telefone != "+244 923 456 789" {
		t.Fatalf("telefone omitido devia preservar-se, obtive %q", perfil.Telefone)
	}
}
```

(reutiliza `novaSessao()` de `identidade_test.go`.)

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/identidade/ -run AtualizarPerfil`
Expected: FAIL — `NovoCasoAtualizarPerfil` não existe.

- [ ] **Step 3: Criar o caso de uso**

Create `internal/application/identidade/atualizar_perfil.go`:

```go
package identidade

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoAtualizarPerfil permite ao próprio utilizador actualizar o seu perfil local
// (telefone/BI). Garante a linha local (JIT a partir da sessão), aplica a
// validação de domínio, persiste, audita e devolve o perfil actualizado.
type CasoAtualizarPerfil struct {
	utilizadores dominio.RepositorioUtilizadores
	auditor      Auditor
	agora        func() time.Time
}

// NovoCasoAtualizarPerfil constrói o caso de uso.
func NovoCasoAtualizarPerfil(r dominio.RepositorioUtilizadores, a Auditor) *CasoAtualizarPerfil {
	return &CasoAtualizarPerfil{utilizadores: r, auditor: a, agora: time.Now}
}

// Executar actualiza telefone/BI do próprio. `telefone`/`bi` nil preservam o valor
// actual; string vazia limpa; valor presente é validado. Devolve o perfil final.
func (c *CasoAtualizarPerfil) Executar(ctx context.Context, s dominio.Sessao, telefone, bi *string) (Perfil, error) {
	// JIT: garantir a linha local a partir da sessão (fonte de verdade Keycloak).
	base, err := dominio.NovoUtilizador(s.Sujeito, s.Nome, s.Email, "", "", s.Papeis)
	if err != nil {
		return Perfil{}, err
	}
	if err := c.utilizadores.GuardarComPapeis(ctx, base); err != nil {
		return Perfil{}, err
	}

	persistido, err := c.utilizadores.ObterPorID(ctx, s.Sujeito)
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
	if err := c.utilizadores.AtualizarContacto(ctx, s.Sujeito, persistido.Telefone, persistido.BI); err != nil {
		return Perfil{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      s.Sujeito,
		Accao:      "identidade.perfil.actualizado",
		Entidade:   "utilizador",
		EntidadeID: s.Sujeito,
		OcorridoEm: c.agora(),
	}); err != nil {
		return Perfil{}, err
	}

	final, err := c.utilizadores.ObterPorID(ctx, s.Sujeito)
	if err != nil {
		return Perfil{}, err
	}
	return paraPerfil(final), nil
}
```

- [ ] **Step 4: Alterar CasoDefinirActivo para revogar sessões**

In `internal/application/identidade/gerir_utilizadores.go`, replace the body of `CasoDefinirActivo.Executar` with a version that revokes sessions on deactivation:

```go
// Executar aplica o estado no Keycloak e regista a auditoria. Ao desactivar,
// revoga também as sessões activas do utilizador (deixa de poder renovar tokens).
func (c *CasoDefinirActivo) Executar(ctx context.Context, actor, id string, activo bool) error {
	if err := c.admin.DefinirActivo(ctx, id, activo); err != nil {
		return err
	}
	accao := "identidade.utilizador.desactivado"
	if activo {
		accao = "identidade.utilizador.activado"
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      accao,
		Entidade:   "utilizador",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	}); err != nil {
		return err
	}
	if !activo {
		if err := c.admin.RevogarSessoes(ctx, id); err != nil {
			return err
		}
		return c.auditor.Registar(ctx, auditoria.Registo{
			Actor:      actor,
			Accao:      "identidade.sessoes.revogadas",
			Entidade:   "utilizador",
			EntidadeID: id,
			OcorridoEm: c.agora(),
		})
	}
	return nil
}
```

- [ ] **Step 5: Adicionar a asserção de revogação ao teste de DefinirActivo**

In `internal/application/identidade/gerir_utilizadores_test.go`, add tests:

```go
func TestDefinirActivo_DesactivarRevogaSessoes(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoDefinirActivo(admin, aud)
	if err := caso.Executar(context.Background(), "actor-1", "alvo-1", false); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(admin.sessoesRevogadas) != 1 || admin.sessoesRevogadas[0] != "alvo-1" {
		t.Fatalf("esperava revogação de sessões ao desactivar: %v", admin.sessoesRevogadas)
	}
}

func TestDefinirActivo_ActivarNaoRevoga(t *testing.T) {
	admin := &fakeAdmin{}
	caso := appident.NovoCasoDefinirActivo(admin, &fakeAuditor{})
	if err := caso.Executar(context.Background(), "actor-1", "alvo-1", true); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(admin.sessoesRevogadas) != 0 {
		t.Fatalf("activar não deve revogar sessões: %v", admin.sessoesRevogadas)
	}
}
```

- [ ] **Step 6: Correr os testes e confirmar que passam**

Run: `go test ./internal/application/identidade/...`
Expected: PASS.

- [ ] **Step 7: gofmt/vet e commit**

Run: `gofmt -l internal/application/identidade && go vet ./internal/application/identidade/...`
Expected: sem output.

```bash
git add internal/application/identidade/atualizar_perfil.go internal/application/identidade/atualizar_perfil_test.go internal/application/identidade/gerir_utilizadores.go internal/application/identidade/gerir_utilizadores_test.go
git commit -m "feat(identidade): edição de perfil self-service e revogação de sessões ao desactivar

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Adaptador Keycloak — 4 métodos + compensação da criação

**Files:**
- Modify: `internal/adapters/keycloak/admin.go`
- Modify: `internal/adapters/keycloak/admin_httptest_test.go`

**Interfaces:**
- Consumes: existing `pedir`, `tokenServico`; `appident.DadosNovoUtilizador`.
- Produces: `(*AdminCliente)` implements `DefinirPasswordTemporaria`, `ResetOTP`, `RevogarSessoes`, `ApagarUtilizador` — restores the `AdminIdentidade` interface assertion; `CriarUtilizador` compensates on role-assign failure.

- [ ] **Step 1: Escrever os testes httptest (falham)**

Append to `internal/adapters/keycloak/admin_httptest_test.go`:

```go
func TestDefinirPasswordTemporaria_PUT(t *testing.T) {
	var recebeuTemporary bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/users/u1/reset-password"):
			var corpo map[string]any
			_ = json.NewDecoder(r.Body).Decode(&corpo)
			recebeuTemporary, _ = corpo["temporary"].(bool)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	admin, _ := NovoAdmin(srv.URL+"/realms/sgc", "sgc-admin", "segredo")
	if err := admin.DefinirPasswordTemporaria(context.Background(), "u1", "nova-senha"); err != nil {
		t.Fatalf("DefinirPasswordTemporaria: %v", err)
	}
	if !recebeuTemporary {
		t.Fatal("esperava temporary:true no corpo")
	}
}

func TestResetOTP_ApagaCredsEExigeConfigure(t *testing.T) {
	var apagouOTP bool
	var exigiuConfigure bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/u1/credentials"):
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "cred-otp", "type": "otp"},
				{"id": "cred-pwd", "type": "password"},
			})
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/users/u1/credentials/cred-otp"):
			apagouOTP = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/users/u1"):
			var corpo map[string]any
			_ = json.NewDecoder(r.Body).Decode(&corpo)
			if ra, ok := corpo["requiredActions"].([]any); ok {
				for _, a := range ra {
					if a == "CONFIGURE_TOTP" {
						exigiuConfigure = true
					}
				}
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	admin, _ := NovoAdmin(srv.URL+"/realms/sgc", "sgc-admin", "segredo")
	if err := admin.ResetOTP(context.Background(), "u1"); err != nil {
		t.Fatalf("ResetOTP: %v", err)
	}
	if !apagouOTP {
		t.Fatal("esperava DELETE da credencial otp")
	}
	if !exigiuConfigure {
		t.Fatal("esperava requiredActions com CONFIGURE_TOTP")
	}
}

func TestRevogarSessoes_POSTLogout(t *testing.T) {
	var fezLogout bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/users/u1/logout"):
			fezLogout = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	admin, _ := NovoAdmin(srv.URL+"/realms/sgc", "sgc-admin", "segredo")
	if err := admin.RevogarSessoes(context.Background(), "u1"); err != nil {
		t.Fatalf("RevogarSessoes: %v", err)
	}
	if !fezLogout {
		t.Fatal("esperava POST logout")
	}
}

func TestApagarUtilizador_DELETE(t *testing.T) {
	var apagou bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/users/u1"):
			apagou = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	admin, _ := NovoAdmin(srv.URL+"/realms/sgc", "sgc-admin", "segredo")
	if err := admin.ApagarUtilizador(context.Background(), "u1"); err != nil {
		t.Fatalf("ApagarUtilizador: %v", err)
	}
	if !apagou {
		t.Fatal("esperava DELETE do utilizador")
	}
}

func TestCriarUtilizador_CompensaFalhaDePapel(t *testing.T) {
	var apagou bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/admin/realms/sgc/users"):
			w.Header().Set("Location", srvBase(r)+"/admin/realms/sgc/users/novo-id")
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/roles/Medico"):
			w.WriteHeader(http.StatusInternalServerError) // falha ao obter o papel
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/users/novo-id"):
			apagou = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	admin, _ := NovoAdmin(srv.URL+"/realms/sgc", "sgc-admin", "segredo")
	_, err := admin.CriarUtilizador(context.Background(), appident.DadosNovoUtilizador{
		Username: "x", Nome: "X Y", Email: "x@sgc.ao", SenhaTemporaria: "t",
		Papeis: []dominio.Papel{dominio.PapelMedico},
	})
	if err == nil {
		t.Fatal("esperava erro na atribuição de papel")
	}
	if !apagou {
		t.Fatal("esperava compensação (DELETE do utilizador criado)")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/keycloak/ -run 'DefinirPassword|ResetOTP|RevogarSessoes|ApagarUtilizador|CompensaFalha'`
Expected: FAIL — métodos inexistentes.

- [ ] **Step 3: Implementar os 4 métodos + compensação**

In `internal/adapters/keycloak/admin.go`, add (after `DefinirActivo`, before the interface assertion `var _`):

```go
// DefinirPasswordTemporaria define uma nova password temporária (o utilizador é
// forçado a mudá-la no próximo login).
func (a *AdminCliente) DefinirPasswordTemporaria(ctx context.Context, id, senha string) error {
	corpo := map[string]any{"type": "password", "value": senha, "temporary": true}
	return a.pedir(ctx, nethttp.MethodPut, "/users/"+url.PathEscape(id)+"/reset-password", corpo, nil)
}

// ResetOTP remove as credenciais OTP do utilizador e exige a re-inscrição
// (required action CONFIGURE_TOTP) no próximo login.
func (a *AdminCliente) ResetOTP(ctx context.Context, id string) error {
	var creds []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := a.pedir(ctx, nethttp.MethodGet, "/users/"+url.PathEscape(id)+"/credentials", nil, &creds); err != nil {
		return err
	}
	for _, c := range creds {
		if c.Type == "otp" {
			if err := a.pedir(ctx, nethttp.MethodDelete,
				"/users/"+url.PathEscape(id)+"/credentials/"+url.PathEscape(c.ID), nil, nil); err != nil {
				return err
			}
		}
	}
	return a.pedir(ctx, nethttp.MethodPut, "/users/"+url.PathEscape(id),
		map[string]any{"requiredActions": []string{"CONFIGURE_TOTP"}}, nil)
}

// RevogarSessoes termina todas as sessões activas do utilizador.
func (a *AdminCliente) RevogarSessoes(ctx context.Context, id string) error {
	return a.pedir(ctx, nethttp.MethodPost, "/users/"+url.PathEscape(id)+"/logout", nil, nil)
}

// ApagarUtilizador remove o utilizador do realm.
func (a *AdminCliente) ApagarUtilizador(ctx context.Context, id string) error {
	return a.pedir(ctx, nethttp.MethodDelete, "/users/"+url.PathEscape(id), nil, nil)
}
```

And update the role-assignment loop in `CriarUtilizador` to compensate on failure. Replace the existing loop:
```go
	for _, p := range dados.Papeis {
		if err := a.AtribuirPapel(ctx, id, p); err != nil {
			return "", err
		}
	}
```
with:
```go
	for _, p := range dados.Papeis {
		if err := a.AtribuirPapel(ctx, id, p); err != nil {
			// Compensação best-effort: apagar o utilizador criado para que uma
			// nova tentativa não bata em 409 (ver ADR-023/024).
			_ = a.ApagarUtilizador(ctx, id)
			return "", err
		}
	}
```

- [ ] **Step 4: Correr os testes e confirmar que passam**

Run: `go test ./internal/adapters/keycloak/...`
Expected: PASS (incl. a asserção `var _ appident.AdminIdentidade = (*AdminCliente)(nil)`, agora satisfeita).

- [ ] **Step 5: gofmt/vet e commit**

Run: `gofmt -l internal/adapters/keycloak && go vet ./internal/adapters/keycloak/...`
Expected: sem output (ignora `doc.go` CRLF pré-existente).

```bash
git add internal/adapters/keycloak/admin.go internal/adapters/keycloak/admin_httptest_test.go
git commit -m "feat(identidade): reset de password/OTP, revogação de sessões, apagar e compensação

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Adaptador pgrepo — AtualizarContacto

**Files:**
- Modify: `internal/adapters/pgrepo/identidade_repo.go`

**Interfaces:**
- Produces: `(*RepositorioUtilizadores).AtualizarContacto(ctx, keycloakID, telefone, bi string) error` — completes the `dominio.RepositorioUtilizadores` interface (restores the module build).

- [ ] **Step 1: Implementar AtualizarContacto**

In `internal/adapters/pgrepo/identidade_repo.go`, add (after `GuardarComPapeis`):

```go
// AtualizarContacto persiste telefone/BI do utilizador (perfil local). Strings
// vazias limpam o campo. Devolve NaoEncontrado se a linha não existir.
func (r *RepositorioUtilizadores) AtualizarContacto(ctx context.Context, keycloakID, telefone, bi string) error {
	const q = `
UPDATE identidade.utilizadores
SET telefone = NULLIF($2, ''), bi = NULLIF($3, ''), actualizado_em = now()
WHERE keycloak_id = $1`

	ct, err := r.pool.Exec(ctx, q, keycloakID, telefone, bi)
	if err != nil {
		return fmt.Errorf("actualizar contacto: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return erros.Novo(erros.CategoriaNaoEncontrado, "utilizador não encontrado")
	}
	return nil
}
```

(`identidade_repo.go` já importa `context`, `fmt` e `erros` — nenhum import novo.)

- [ ] **Step 2: Compilar o pacote e o módulo**

Run: `go build ./...`
Expected: sem erros — todas as interfaces estão agora implementadas (AdminCliente na Task 4, pgrepo aqui). O módulo volta a compilar.

- [ ] **Step 3: vet + suite (sem integração)**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 4: gofmt e commit**

Run: `gofmt -l internal/adapters/pgrepo/identidade_repo.go` (sem output; `gofmt -w` se listar).

```bash
git add internal/adapters/pgrepo/identidade_repo.go
git commit -m "feat(identidade): persistência de telefone/BI (AtualizarContacto) no repositório

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: HTTP — handlers de reset e edição de perfil

**Files:**
- Modify: `internal/adapters/http/admin_handler.go`
- Modify: `internal/adapters/http/admin_test.go`
- Modify: `internal/adapters/http/identidade_handler.go`
- Modify: `internal/adapters/http/identidade_test.go`

**Interfaces:**
- Consumes: `appident.CredencialReposta`, `appident.Perfil`; `SessaoDe`, `RBAC`, `responderErro`; `dominio.Sessao`, `dominio.PapelAdmin`.
- Produces:
  - `admin_handler.go`: interfaces `ServicoResetPassword`/`ServicoResetOtp`; `NovoAdministracaoHandler` ganha 2 params (7º `resetPassword`, 8º `resetOtp`); rotas `POST /:id/reset-password`, `POST /:id/reset-otp`.
  - `identidade_handler.go`: interface `ServicoAtualizarPerfil`; `NovoIdentidadeHandler` ganha um 2º param (`atualizar`); rota `PATCH /perfil`.

- [ ] **Step 1: Escrever os testes HTTP (falham)**

In `internal/adapters/http/admin_test.go`, update `routerAdmin` to build the handler with 8 args (add two fakes) and add fakes + tests. Add the fakes:
```go
type fakeResetPass struct {
	out appident.CredencialReposta
	err error
}

func (f fakeResetPass) Executar(context.Context, string, string) (appident.CredencialReposta, error) {
	return f.out, f.err
}

type fakeResetOtp struct{ err error }

func (f fakeResetOtp) Executar(context.Context, string, string) error { return f.err }
```
Update the `NovoAdministracaoHandler(...)` call inside `routerAdmin` to pass the two extra args at the end:
```go
		fakeCriar{out: appident.UtilizadorCriado{ID: "novo-id", SenhaTemporaria: "senha-temp"}},
		fakeResetPass{out: appident.CredencialReposta{SenhaTemporaria: "nova-senha"}},
		fakeResetOtp{},
	)
```
Add tests:
```go
func TestAdmin_ResetPassword_AdminOk_200(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Sujeito: "actor-1", Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedido(r, "POST", "/api/v1/identidade/utilizadores/u1/reset-password", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"senha_temporaria":"nova-senha"`) {
		t.Fatalf("corpo inesperado: %s", w.Body.String())
	}
}

func TestAdmin_ResetPassword_MedicoProibido_403(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, &fakePapel{})
	if w := pedido(r, "POST", "/api/v1/identidade/utilizadores/u1/reset-password", "Bearer xyz"); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Medico não deve repor password; obtive %d", w.Code)
	}
}

func TestAdmin_ResetOtp_AdminOk_204(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Sujeito: "actor-1", Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	if w := pedido(r, "POST", "/api/v1/identidade/utilizadores/u1/reset-otp", "Bearer xyz"); w.Code != nethttp.StatusNoContent {
		t.Fatalf("esperava 204, obtive %d", w.Code)
	}
}
```

In `internal/adapters/http/identidade_test.go`, add a fake + test for the profile update. Add the fake near the others:
```go
type fakeAtualizarPerfil struct {
	perfil appident.Perfil
	err    error
}

func (f fakeAtualizarPerfil) Executar(context.Context, dominio.Sessao, *string, *string) (appident.Perfil, error) {
	return f.perfil, f.err
}
```
And a test (the existing `TestRegistarIdentidade_Perfil_200` uses `NovoIdentidadeHandler(fakePerfil{...})` — update that call and the `SemToken` one to pass the second arg `fakeAtualizarPerfil{}`):
```go
func TestRegistarIdentidade_AtualizarPerfil_200(t *testing.T) {
	r := novoRouter()
	sessao := dominio.Sessao{Sujeito: "uuid-1", Papeis: []dominio.Papel{dominio.PapelMedico}}
	perfil := appident.Perfil{KeycloakID: "uuid-1", Nome: "Ana", Email: "ana@sgc.ao", Telefone: "+244 923 456 789", Activo: true, Papeis: []string{"Medico"}}
	h := adhttp.NovoIdentidadeHandler(fakePerfil{}, fakeAtualizarPerfil{perfil: perfil})
	adhttp.RegistarIdentidade(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/v1/identidade/perfil", strings.NewReader(`{"telefone":"+244 923 456 789"}`))
	req.Header.Set("Authorization", "Bearer xyz")
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"telefone":"+244 923 456 789"`) {
		t.Fatalf("corpo inesperado: %s", w.Body.String())
	}
}
```
Update the two existing `NovoIdentidadeHandler(...)` calls in this file to pass `fakeAtualizarPerfil{}` as the second argument (e.g. `adhttp.NovoIdentidadeHandler(fakePerfil{perfil: perfil}, fakeAtualizarPerfil{})`).

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/http/ -run 'ResetPassword|ResetOtp|AtualizarPerfil'`
Expected: FAIL — construtores com mais args e rotas inexistentes.

- [ ] **Step 3: Implementar os handlers de reset (admin_handler.go)**

In `internal/adapters/http/admin_handler.go`:

Add to the `type ( ... )` interface block:
```go
	// ServicoResetPassword repõe a password de um utilizador.
	ServicoResetPassword interface {
		Executar(ctx context.Context, actor, id string) (appident.CredencialReposta, error)
	}
	// ServicoResetOtp repõe o OTP de um utilizador.
	ServicoResetOtp interface {
		Executar(ctx context.Context, actor, id string) error
	}
```

Add fields to `AdministracaoHandler`:
```go
	resetPassword ServicoResetPassword
	resetOtp      ServicoResetOtp
```

Update `NovoAdministracaoHandler` to accept and store them (add as the last two params `resetPassword ServicoResetPassword, resetOtp ServicoResetOtp` and `resetPassword: resetPassword, resetOtp: resetOtp` in the literal).

Register routes in `RegistarAdministracao` (after `g.POST("", ...)`). Note: the handler methods are named `reporPassword`/`reporOtp` (distinct from the struct fields `resetPassword`/`resetOtp` — a Go struct field and a method cannot share a name):
```go
	g.POST("/:id/reset-password", escrita, h.reporPassword)
	g.POST("/:id/reset-otp", escrita, h.reporOtp)
```

Add the handler methods:
```go
func (h *AdministracaoHandler) reporPassword(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.resetPassword.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *AdministracaoHandler) reporOtp(c *gin.Context) {
	actor, _ := SessaoDe(c)
	if err := h.resetOtp.Executar(c.Request.Context(), actor.Sujeito, c.Param("id")); err != nil {
		responderErro(c, err)
		return
	}
	c.Status(nethttp.StatusNoContent)
}
```

- [ ] **Step 4: Implementar o handler de perfil (identidade_handler.go)**

In `internal/adapters/http/identidade_handler.go`:

Add the interface after `ServicoPerfil`:
```go
// ServicoAtualizarPerfil actualiza o perfil (telefone/BI) do próprio utilizador.
type ServicoAtualizarPerfil interface {
	Executar(ctx context.Context, s dominio.Sessao, telefone, bi *string) (appident.Perfil, error)
}
```

Add the field to `IdentidadeHandler`:
```go
	atualizar ServicoAtualizarPerfil
```

Update `NovoIdentidadeHandler` to accept and store it:
```go
func NovoIdentidadeHandler(p ServicoPerfil, atualizar ServicoAtualizarPerfil) *IdentidadeHandler {
	return &IdentidadeHandler{perfil: p, atualizar: atualizar}
}
```

Register the route in `RegistarIdentidade` (after `grupo.GET("/perfil", ...)`):
```go
	grupo.PATCH("/perfil", h.atualizarPerfil)
```

Add the request type and handler:
```go
type corpoPerfil struct {
	Telefone *string `json:"telefone"`
	Bi       *string `json:"bi"`
}

func (h *IdentidadeHandler) atualizarPerfil(c *gin.Context) {
	sessao, ok := SessaoDe(c)
	if !ok {
		responderErro(c, erros.Novo(erros.CategoriaNaoAutorizado, i18n.T(i18n.MsgNaoAutenticado)))
		return
	}
	var corpo corpoPerfil
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	perfil, err := h.atualizar.Executar(c.Request.Context(), sessao, corpo.Telefone, corpo.Bi)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, perfil)
}
```

- [ ] **Step 5: Correr os testes e confirmar que passam**

Run: `go test ./internal/adapters/http/...`
Expected: PASS (todos, incl. os testes existentes actualizados para os novos construtores).

> Nota: `go build ./...` do módulo falha agora (o `app.go` chama `NovoAdministracaoHandler`/`NovoIdentidadeHandler` com a assinatura antiga) — corrige-se na Task 7. Verifica só o pacote http.

- [ ] **Step 6: gofmt/vet e commit**

Run: `gofmt -l internal/adapters/http && go vet ./internal/adapters/http/...`
Expected: sem output.

```bash
git add internal/adapters/http/admin_handler.go internal/adapters/http/admin_test.go internal/adapters/http/identidade_handler.go internal/adapters/http/identidade_test.go
git commit -m "feat(identidade): endpoints de reset (admin) e PATCH de perfil (self-service)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Plataforma — fiar os novos casos de uso

**Files:**
- Modify: `internal/platform/app.go`

**Interfaces:**
- Consumes: `appident.NovoCasoResetPassword`, `appident.NovoCasoResetOTP`, `appident.NovoCasoAtualizarPerfil`, `adhttp.NovoAdministracaoHandler` (8 args), `adhttp.NovoIdentidadeHandler` (2 args).

- [ ] **Step 1: Atualizar app.go**

In `internal/platform/app.go`:

Add the use cases near the others:
```go
	casoResetPass := appident.NovoCasoResetPassword(adminKC, repoAuditoria)
	casoResetOTP := appident.NovoCasoResetOTP(adminKC, repoAuditoria)
	casoAtualizarPerfil := appident.NovoCasoAtualizarPerfil(repoUtilizadores, repoAuditoria)
```

Update the identidade handler construction to pass the profile-update use case:
```go
	handlerIdentidade := adhttp.NovoIdentidadeHandler(casoPerfil, casoAtualizarPerfil)
```

Update the administration handler construction to pass the two reset use cases (as the 7th and 8th args):
```go
	handlerAdmin := adhttp.NovoAdministracaoHandler(casoListar, casoObter, casoAtribuir, casoRevogar, casoActivo, casoCriar, casoResetPass, casoResetOTP)
```

- [ ] **Step 2: Compilar o módulo inteiro**

Run: `go build ./...`
Expected: sem erros.

- [ ] **Step 3: vet + suite completa (sem integração)**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 4: gofmt e commit**

Run: `gofmt -l internal/platform/app.go` (sem output; `gofmt -w` se listar).

```bash
git add internal/platform/app.go
git commit -m "feat(identidade): fiar reset de credenciais e edição de perfil no composition root

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Integração — reset, perfil e revogação de sessões via Keycloak/BD reais

**Files:**
- Create: `tests/integration/ciclo_vida_test.go`

**Interfaces:**
- Consumes: `keycloak.NovoAdmin`, `pgrepo.NovoRepositorioUtilizadores`, `appident` DTOs, `issuerTeste`, `ligar` (helpers existentes).

- [ ] **Step 1: Escrever os testes e2e**

Create `tests/integration/ciclo_vida_test.go`:

```go
//go:build integration

package integration_test

import (
	"context"
	"testing"

	"log/slog"
	"os"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/keycloak"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// TestResetPasswordEOTP_ViaKeycloak cria um utilizador, repõe a password e o OTP,
// e limpa. Exercita DefinirPasswordTemporaria/ResetOTP contra o Keycloak real.
func TestResetPasswordEOTP_ViaKeycloak(t *testing.T) {
	issuer := issuerTeste()
	admin, err := keycloak.NovoAdmin(issuer, "sgc-admin", "segredo-admin")
	if err != nil {
		t.Fatalf("NovoAdmin: %v", err)
	}
	ctx := context.Background()

	id, err := admin.CriarUtilizador(ctx, appident.DadosNovoUtilizador{
		Username: "reset.teste.sprint5", Nome: "Reset Teste", Email: "reset.teste.sprint5@sgc.ao",
		SenhaTemporaria: "Temp-1234", Papeis: []dominio.Papel{dominio.PapelMedico}, ConfigurarOTP: false,
	})
	if err != nil {
		t.Skipf("Admin API indisponível ou já existe: %v", err)
	}
	defer apagarUtilizador(t, issuer, id)

	if err := admin.DefinirPasswordTemporaria(ctx, id, "Nova-Senha-9"); err != nil {
		t.Fatalf("DefinirPasswordTemporaria: %v", err)
	}
	if err := admin.ResetOTP(ctx, id); err != nil {
		t.Fatalf("ResetOTP: %v", err)
	}
	if err := admin.RevogarSessoes(ctx, id); err != nil {
		t.Fatalf("RevogarSessoes: %v", err)
	}
}

// TestAtualizarPerfil_ViaBD exercita o CasoAtualizarPerfil contra a BD real:
// garante a linha (JIT) e persiste telefone/BI.
func TestAtualizarPerfil_ViaBD(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repo := pgrepo.NovoRepositorioUtilizadores(pool)
	repoAud := pgrepo.NovoRepositorioAuditoria(pool)
	caso := appident.NovoCasoAtualizarPerfil(repo, repoAud)

	sessao := dominio.Sessao{Sujeito: "perfil-teste-sprint5", Nome: "Perfil Teste", Email: "perfil.teste@sgc.ao", Papeis: []dominio.Papel{dominio.PapelMedico}}
	tel := "+244 923 456 789"
	perfil, err := caso.Executar(ctx, sessao, &tel, nil)
	if err != nil {
		t.Fatalf("actualizar perfil: %v", err)
	}
	if perfil.Telefone != "+244 923 456 789" {
		t.Fatalf("telefone não persistido: %q", perfil.Telefone)
	}

	// Limpeza da linha local criada.
	_, _ = pool.Exec(ctx, `DELETE FROM identidade.utilizadores WHERE keycloak_id = $1`, sessao.Sujeito)
}
```

> Confirma que `apagarUtilizador`, `issuerTeste`, `ligar` já existem no pacote de integração (Sprints 3/4) — não os redefinas. Corre `gofmt -w` no ficheiro no fim (a ordenação dos imports é normalizada por gofmt).

- [ ] **Step 2: Correr os testes e2e (compose a correr)**

Run:
```bash
DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
KEYCLOAK_ISSUER=http://localhost:8081/realms/sgc \
go test -tags=integration ./tests/integration/... -run 'ResetPasswordEOTP|AtualizarPerfil_ViaBD' -v
```
Expected: PASS (ou SKIP se infra indisponível). Se FALHAR por razão real, investiga e reporta.

- [ ] **Step 3: Confirmar build/test sem tag + gofmt/vet**

Run: `go build ./... && go test ./...`
Then: `gofmt -l tests/integration/ciclo_vida_test.go && go vet -tags=integration ./tests/integration/...`
Expected: build/test PASS; gofmt sem output.

- [ ] **Step 4: Commit**

```bash
git add tests/integration/ciclo_vida_test.go
git commit -m "test(identidade): e2e de reset de credenciais e edição de perfil

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Documentação e verificação final

**Files:**
- Create: `adrs/ADR-024-ciclo-vida-utilizador.md`
- Modify: `SPRINT.md`, `CLAUDE.md`

**Interfaces:** nenhuma.

- [ ] **Step 1: Escrever a ADR-024**

Create `adrs/ADR-024-ciclo-vida-utilizador.md`:

```markdown
# ADR-024 — Ciclo de vida do utilizador

- **Estado**: Aceite
- **Data**: 2026-07-12
- **Marco/Sprint**: M1 / Sprint 5
- **Contexto BC**: Identidade

## Contexto

Completar a gestão de utilizadores dos Sprints 3-4 com as operações de ciclo de vida
em falta.

## Decisão

1. **Reset de password (admin)**: gera nova senha temporária (`temporary:true`),
   devolvida uma única vez. Coerente com a criação (Sprint 4).
2. **Reset de OTP (admin)**: remove as credenciais OTP e exige `CONFIGURE_TOTP`
   (re-inscrição no próximo login).
3. **Edição de perfil (self-service)**: `PATCH /api/v1/identidade/perfil` — o próprio
   actualiza telefone/BI (campos locais, validados pelos VOs Angola). Campos omitidos
   preservam-se; vazios limpam.
4. **Revogação de sessões na desactivação**: ao desactivar, revogam-se as sessões
   Keycloak do utilizador (deixa de poder renovar tokens).
5. **Compensação da criação não-atómica**: se a atribuição de papel falhar após a
   criação, o utilizador criado é apagado (best-effort), fechando a limitação da
   ADR-023.

## Consequências

- A gestão de utilizadores fica completa para o M1; auditoria em todas as operações.
- Reset de password/OTP e revogação de sessões usam a Admin REST API (Keycloak fonte
  de verdade); o perfil telefone/BI vive na BD local.
- Alternativas descartadas: reset por email (sem SMTP em dev); edição administrativa
  do perfil de outros (mantém-se self-service — fica para Sprint 6+).
```

- [ ] **Step 2: Atualizar SPRINT.md**

In `SPRINT.md`, change the `- **Sprint**:` line to `5 (BC Identidade — ciclo de vida do utilizador) — **entregue**` and add a "Sprint 5 — entregue" section above "Sprint 4 — entregue":

```markdown
## Sprint 5 — entregue

- [x] Reset de password (admin): nova senha temporária devolvida 1x; reset de OTP
      (remove credenciais + `CONFIGURE_TOTP`). Auditados.
- [x] Edição de perfil self-service (`PATCH /perfil`): telefone/BI validados pelos VOs
      Angola; omitido preserva, vazio limpa.
- [x] Revogação de sessões na desactivação; compensação da criação não-atómica
      (ADR-023 fechada).
- [x] ADR-024.
```

- [ ] **Step 3: Atualizar CLAUDE.md**

In `CLAUDE.md`, in the "Convenções-fonte" block, add ADR-024 to the registered list and bump the next ADR:
```markdown
`adrs/ADR-023-mfa-positivo-criar-utilizadores.md`, `adrs/ADR-024-ciclo-vida-utilizador.md`.
Próximo ADR: **ADR-025**.
```

- [ ] **Step 4: Verificação final completa**

Run each and confirm:
```bash
go build ./...        # sem erros
go vet ./...          # sem erros
go test ./...         # PASS
bash scripts/cobertura.sh   # domínio ≥85, aplicação ≥75, adaptadores ≥60 — todos OK
```
For `gofmt`, run `gofmt -l internal cmd tests` and confirm the ONLY files listed are pre-existing CRLF artifacts NOT touched by Sprint 5 (verify with `git diff --name-only 9c41617..HEAD`). Do not reformat pre-existing untouched files.

- [ ] **Step 5: Verificação e2e final (compose a correr)**

Run the full integration suite:
```bash
DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
KEYCLOAK_ISSUER=http://localhost:8081/realms/sgc \
go test -tags=integration ./tests/integration/... -v
```
Expected: todos os testes (MFA, criação, ciclo de vida, Sprint 3) PASS ou SKIP.

- [ ] **Step 6: Commit final**

```bash
git add adrs/ADR-024-ciclo-vida-utilizador.md SPRINT.md CLAUDE.md
git commit -m "docs(identidade): ADR-024 e reconciliação de docs do Sprint 5

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Notas de verificação global (após todas as tasks)

1. `go build ./...`, `go vet ./...`, `gofmt` (só CRLF pré-existentes) limpos; `go test ./...` verde.
2. Cobertura cumpre 85/75/60.
3. Compose a correr: reset-password como Admin → 200 + nova senha; reset-otp → 204; `PATCH /perfil` telefone/BI válido → 200, inválido → 400; desactivar → sessões revogadas.
4. Auditoria em todas as operações; não-Admin nos resets → 403.
5. `go-arch-lint` (CI/Linux) sem violações — domínio sem infra.
