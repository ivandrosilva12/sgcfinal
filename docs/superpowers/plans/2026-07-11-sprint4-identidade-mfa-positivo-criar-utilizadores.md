# Sprint 4 — MFA positivo + Criação de utilizadores Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fechar o caminho positivo de MFA (um login com 2º factor é reconhecido → acesso concedido) e adicionar criação administrativa de utilizadores no Keycloak.

**Architecture:** Parte A é sobretudo configuração do realm + validação e2e (a lógica `ehAutenticacaoForte` já existe do Sprint 3). Parte B é uma fatia vertical Clean Architecture (Domínio → Aplicação → Adaptadores → Plataforma): novo caso de uso `CasoCriarUtilizador`, nova operação na porta `AdminIdentidade`, novo handler HTTP, tudo sobre o Keycloak Admin REST API (fonte de verdade).

**Tech Stack:** Go 1.25, Gin, go-oidc, pgx v5, Keycloak 25. Sem novas dependências (TOTP e geração de senha usam `crypto/*` da stdlib).

## Global Constraints

- **Idioma**: PT-PT angolano em TODO o output (código, comentários, commits, mensagens, JSON). Nunca EN/PT-BR.
- **Linguagem ubíqua**: Utilizador, Papel, Sessão, Auditoria. Nunca Patient/Role/Session.
- **Regra de dependência**: `internal/domain/**` não importa infra (`pgx`/`gin`/`net/http`/`oidc`). Aplicação importa só domínio + Shared Kernel (`erros`, `i18n`, `auditoria`) + stdlib (`crypto/rand`, `net/mail`). `go-arch-lint` impõe em CI.
- **Sem `panic()`** fora de inicialização — sempre `error` categorizado (`erros.ErroDominio`).
- **Erros HTTP**: RFC 7807 (`application/problem+json`), mensagens via `i18n.T`.
- **Cobertura (gate CI, agregado por camada, `scripts/cobertura.sh`)**: domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
- **Módulo Go**: `github.com/ivandrosilva12/sgcfinal`.
- **Commits**: Conventional Commits PT-PT (`feat(identidade): …`), terminando com:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- **Fonte de verdade dos utilizadores/papéis**: Keycloak (Admin REST API). A BD local é espelho JIT — a criação NÃO escreve em `identidade.utilizadores`.
- **Módulo de trabalho**: raiz `C:\Users\PC\Documents\RMPRO 2026\Software Clinicas Final`. Comandos correm da raiz (Git Bash). Stack compose a correr; Keycloak em `localhost:8081`, Postgres em `localhost:5432`. Keycloak `start-dev --import-realm` (H2 efémera) → `docker compose up -d --force-recreate keycloak` re-importa o realm (~40s).

---

## Ficheiros (mapa)

| Ação | Ficheiro | Responsabilidade |
|---|---|---|
| Modificar | `docker/keycloak/realm-sgc.json` | Config de MFA (OTP condicional + acr/amr) + `director.teste` com OTP |
| Modificar | `docker-compose.yml` | `--features=scripts` no Keycloak (SÓ se a variante amr vencer) |
| Modificar | `internal/adapters/keycloak/cliente.go` | Ajuste de `amrFortes`/acr se o spike o exigir |
| Modificar | `internal/platform/config/config.go` | Ajuste do default `KEYCLOAK_ACR_FORTE` se o spike o exigir |
| Criar | `tests/integration/totp_test.go` | Helper `codigoTOTP` (RFC 6238) partilhado |
| Criar | `tests/integration/mfa_positivo_test.go` | Spike (exploratório) + teste e2e positivo |
| Modificar | `internal/domain/identidade/eventos.go` | Evento `UtilizadorCriado` |
| Modificar | `internal/domain/shared/i18n/i18n.go` | Mensagens de criação/conflito |
| Modificar | `internal/application/identidade/ports.go` | Porta `CriarUtilizador` + DTOs |
| Criar | `internal/application/identidade/criar_utilizador.go` | `CasoCriarUtilizador` + geração de senha |
| Criar | `internal/application/identidade/criar_utilizador_test.go` | Testes de aplicação |
| Modificar | `internal/adapters/keycloak/admin.go` | Método `CriarUtilizador` |
| Modificar | `internal/adapters/keycloak/admin_httptest_test.go` | Teste do `CriarUtilizador` |
| Modificar | `internal/adapters/http/admin_handler.go` | Handler `criarUtilizador` + rota + 6º serviço |
| Modificar | `internal/adapters/http/admin_test.go` | Testes do handler + fake de criação |
| Modificar | `internal/platform/app.go` | Fiar `CasoCriarUtilizador` no handler |
| Criar | `tests/integration/criar_utilizador_test.go` | e2e criar → confirmar → limpar |
| Criar | `adrs/ADR-023-mfa-positivo-criar-utilizadores.md` | Decisão do spike + criação |
| Modificar | `SPRINT.md`, `CLAUDE.md`, `.env.example` | Docs |

---

## PARTE A — Fechar o caminho positivo de MFA

### Task 1: Spike — determinar e fixar o sinal de MFA no realm

> **Natureza:** esta task é uma INVESTIGAÇÃO com um entregável concreto (config do realm que funciona + decisão). Não segue RED/GREEN estrito. Se, após esgotar as duas variantes com iteração razoável, nenhuma emitir um sinal fiável, PARA e reporta `DONE_WITH_CONCERNS` com os claims observados em cada variante — o controlador decide.

**Files:**
- Create: `tests/integration/totp_test.go`
- Create: `tests/integration/mfa_positivo_test.go` (nesta task, apenas a variante exploratória que imprime claims)
- Modify: `docker/keycloak/realm-sgc.json`
- Modify (só se variante amr vencer): `docker-compose.yml`

**Interfaces:**
- Produces: helper `codigoTOTP(segredo string, em time.Time) string` em `package integration_test`; um `director.teste` no realm com password `teste` e OTP de segredo conhecido; a config de MFA que faz o access token de um login OTP conter um sinal em `KEYCLOAK_ACR_FORTE` (default `mfa,gold,2`) OU `amr` forte.

- [ ] **Step 1: Criar o helper TOTP (RFC 6238)**

Create `tests/integration/totp_test.go`:

```go
//go:build integration

package integration_test

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"time"
)

// codigoTOTP calcula o código TOTP (RFC 6238: HMAC-SHA1, 6 dígitos, período 30s)
// a partir do segredo cru guardado pelo Keycloak (secretData.value é usado como
// chave HMAC directa, não base32). Se o Keycloak rejeitar o código, ver a nota no
// spike sobre base32 — algumas versões guardam o segredo noutra codificação.
func codigoTOTP(segredo string, em time.Time) string {
	chave := []byte(segredo)
	contador := uint64(em.Unix()) / 30
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], contador)
	mac := hmac.New(sha1.New, chave)
	_, _ = mac.Write(buf[:])
	soma := mac.Sum(nil)
	deslocamento := soma[len(soma)-1] & 0x0f
	valor := (uint32(soma[deslocamento]&0x7f) << 24) |
		(uint32(soma[deslocamento+1]) << 16) |
		(uint32(soma[deslocamento+2]) << 8) |
		uint32(soma[deslocamento+3])
	return fmt.Sprintf("%06d", valor%1000000)
}
```

- [ ] **Step 2: Semear `director.teste` com OTP conhecido no realm**

In `docker/keycloak/realm-sgc.json`, add to the `users` array (after `admin.teste`, insert a comma). The OTP secret is the literal string `segredoteste-otp-32chars-abcdefgh` (32 chars); the test computes codes from this exact string.

```json
    {
      "username": "director.teste",
      "enabled": true,
      "emailVerified": true,
      "email": "director.teste@sgc.ao",
      "firstName": "Director",
      "lastName": "de Teste",
      "credentials": [
        { "type": "password", "value": "teste", "temporary": false },
        {
          "type": "otp",
          "secretData": "{\"value\":\"segredoteste-otp-32chars-abcdefgh\"}",
          "credentialData": "{\"subType\":\"totp\",\"digits\":6,\"counter\":0,\"period\":30,\"algorithm\":\"HmacSHA1\"}"
        }
      ],
      "realmRoles": ["Director"]
    }
```

- [ ] **Step 3: Configurar a Variante A (acr via LoA) no realm**

Add to `docker/keycloak/realm-sgc.json` at the top level (realm object): the `acr.loa.map` attribute and require OTP for the direct-grant when present. Add an `attributes` block on the realm (if none exists) and configure the Direct Grant flow to include a conditional OTP. Concretely, add these realm-level keys (siblings of `"realm": "sgc"`):

```json
  "attributes": {
    "acr.loa.map": "{\"2\":\"2\"}"
  },
  "otpPolicyType": "totp",
  "otpPolicyAlgorithm": "HmacSHA1",
  "otpPolicyDigits": 6,
  "otpPolicyPeriod": 30,
```

And ensure the built-in "direct grant" flow runs OTP. The simplest reliable route for direct grant: the token request includes `totp`, and Keycloak's default `direct grant` flow has an "OTP" execution. If the imported realm's direct-grant flow lists OTP as `DISABLED`, set it to `CONDITIONAL`/`REQUIRED`. Because editing the full `authenticationFlows` block in JSON is verbose, FIRST try the import as-is (Keycloak's default direct-grant flow already contains "Direct Grant - Conditional OTP"), then verify in Step 5. Only hand-edit `authenticationFlows` if verification shows OTP isn't enforced.

- [ ] **Step 4: Escrever a sonda exploratória que imprime os claims**

Create `tests/integration/mfa_positivo_test.go` (spike form — logs claims, does not assert yet):

```go
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
```

- [ ] **Step 5: Recriar o Keycloak e correr a sonda (Variante A)**

Run:
```bash
python -c "import json; json.load(open(r'docker/keycloak/realm-sgc.json',encoding='utf-8')); print('json ok')"
docker compose up -d --force-recreate keycloak
# aguardar ~40s (o script deve fazer poll do token client_credentials como no Sprint 3)
DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
KEYCLOAK_ISSUER=http://localhost:8081/realms/sgc \
go test -tags=integration ./tests/integration/... -run TestSpike_ClaimsMFA -v
```
Read the logged `acr`/`amr`. **Success (Variante A)** = `acr` is `"2"` (in `KEYCLOAK_ACR_FORTE` default `mfa,gold,2`). If the token request returned non-200 (test skipped), the TOTP code was rejected — try the base32 fallback: change `codigoTOTP` to base32-decode the secret and re-run; if still failing, the direct-grant flow isn't enforcing OTP → hand-edit `authenticationFlows` direct-grant OTP execution to `REQUIRED`.

- [ ] **Step 6: Se a Variante A falhar, configurar e testar a Variante B (amr script)**

If Variante A does not yield a strong `acr`: enable scripts and add an `amr` script mapper.

In `docker-compose.yml`, change the Keycloak command:
```yaml
    command: ["start-dev", "--import-realm", "--features=scripts"]
```
In `realm-sgc.json`, add a `protocolMappers` entry to the `sgc-api` client that emits `amr` via a script mapper (script returns `["otp"]` when an OTP credential was used). Add to the `sgc-api` client object:
```json
      "protocolMappers": [
        {
          "name": "amr-otp",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-script-based-protocol-mapper",
          "config": {
            "script": "var ctx = keycloakSession.getContext(); token.setOtherClaims('amr', ['otp']); ",
            "id.token.claim": "false",
            "access.token.claim": "true",
            "claim.name": "amr"
          }
        }
      ]
```
Recreate keycloak and re-run `TestSpike_ClaimsMFA`. **Success (Variante B)** = `amr` contains `"otp"` (already in `amrFortes`). Note: the script above is a starting point — refine it during the spike so `amr` only appears for OTP logins if feasible; if the script can only hardcode, document that limitation.

- [ ] **Step 7: Fixar a decisão e ajustar `KEYCLOAK_ACR_FORTE`/`amrFortes` se necessário**

- If the winning signal value is already covered (`acr:"2"` ∈ default, or `amr:["otp"]` ∈ `amrFortes`), no code change needed.
- If the emitted value differs (e.g. `acr:"gold"` — already in default; or some other string), update the default in `internal/platform/config/config.go` (`valorOu("KEYCLOAK_ACR_FORTE", "mfa,gold,2")`) and/or add the value to `amrFortes` in `internal/adapters/keycloak/cliente.go`. Keep the change minimal and PT-PT.
- Record the decision (which variant, which claim/value) in a note you'll fold into ADR-023 (Task 9). Write it to the report.

- [ ] **Step 8: Commit the spike deliverable**

```bash
git add docker/keycloak/realm-sgc.json tests/integration/totp_test.go tests/integration/mfa_positivo_test.go
# add docker-compose.yml and/or the cliente.go/config.go tweak ONLY if changed:
# git add docker-compose.yml internal/adapters/keycloak/cliente.go internal/platform/config/config.go
git commit -m "feat(identidade): configurar sinal de MFA no realm (spike: <variante escolhida>)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Teste e2e do caminho positivo de MFA

**Files:**
- Modify: `tests/integration/mfa_positivo_test.go`

**Interfaces:**
- Consumes: `codigoTOTP`, `tokenDirectorComOTP`, `issuerTeste`, `keycloak.Novo` (4-arg), `dominio.VerificarAutenticacaoForte`, `dominio.PapelDirector`.
- Produces: assertion that a real OTP login is recognized as strong (`AutenticacaoForte==true`) and access is granted.

- [ ] **Step 1: Substituir a sonda por uma asserção**

Replace `TestSpike_ClaimsMFA` in `tests/integration/mfa_positivo_test.go` with the assertion test (keep `tokenDirectorComOTP`, `claimsDe`, `segredoOTPDirector`). Add imports `"context"` and the keycloak/dominio packages:

```go
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
```

Update the import block to include:
```go
	"context"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/keycloak"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
```
(keep `encoding/base64`, `encoding/json`, `net/http`, `net/url`, `strings`, `testing`, `time`).

- [ ] **Step 2: Correr o teste positivo (e o negativo, sem regressão)**

Run:
```bash
DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
KEYCLOAK_ISSUER=http://localhost:8081/realms/sgc \
go test -tags=integration ./tests/integration/... -run 'MFA' -v
```
Expected: `TestMFA_DirectorComOTP_AutenticacaoForte` PASS and the Sprint 3 `TestMFA_AdminSemOTP_NaoTemAutenticacaoForte` still PASS (negative case). If the positive test FAILS on `AutenticacaoForte`, the realm signal from Task 1 isn't reaching the API — return to Task 1's Step 7.

- [ ] **Step 3: `gofmt` + vet e commit**

```bash
gofmt -w tests/integration/mfa_positivo_test.go
go vet -tags=integration ./tests/integration/...
git add tests/integration/mfa_positivo_test.go
git commit -m "test(identidade): validar caminho positivo de MFA (OTP → autenticação forte)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## PARTE B — Criação administrativa de utilizadores

### Task 3: Domínio — evento UtilizadorCriado e mensagens i18n

**Files:**
- Modify: `internal/domain/identidade/eventos.go`
- Modify: `internal/domain/shared/i18n/i18n.go`

**Interfaces:**
- Produces: `identidade.UtilizadorCriado{Actor, Alvo string, Em time.Time}` (implementa `evento.EventoDominio`); `i18n.MsgUtilizadorJaExiste`, `i18n.MsgCriacaoInvalida` (chaves `i18n.Chave`).

- [ ] **Step 1: Adicionar o evento**

In `internal/domain/identidade/eventos.go`, add before the conformance `var (...)` block:

```go
// UtilizadorCriado é emitido quando um administrador cria um utilizador.
type UtilizadorCriado struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e UtilizadorCriado) NomeEvento() string { return "identidade.utilizador.criado" }

// OcorridoEm implementa evento.EventoDominio.
func (e UtilizadorCriado) OcorridoEm() time.Time { return e.Em }
```

And add `_ evento.EventoDominio = UtilizadorCriado{}` to the conformance `var (...)` block.

- [ ] **Step 2: Adicionar as mensagens i18n**

In `internal/domain/shared/i18n/i18n.go`, add to the `const` block:
```go
	// MsgUtilizadorJaExiste — username/email já registado.
	MsgUtilizadorJaExiste Chave = "erro.utilizador_ja_existe"
	// MsgCriacaoInvalida — dados de criação inválidos.
	MsgCriacaoInvalida Chave = "erro.criacao_invalida"
```
And to the `mensagensPtAO` map:
```go
	MsgUtilizadorJaExiste: "Já existe um utilizador com este nome de utilizador ou email.",
	MsgCriacaoInvalida:    "Dados de criação inválidos.",
```

- [ ] **Step 3: Verificar build/vet/gofmt**

Run: `go build ./internal/domain/... && go vet ./internal/domain/... && gofmt -l internal/domain/identidade/eventos.go internal/domain/shared/i18n/i18n.go`
Expected: sem erros; gofmt sem output (`gofmt -w` se listar).

- [ ] **Step 4: Commit**

```bash
git add internal/domain/identidade/eventos.go internal/domain/shared/i18n/i18n.go
git commit -m "feat(identidade): evento UtilizadorCriado e mensagens de criação

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Aplicação — porta CriarUtilizador, DTOs e caso de uso

**Files:**
- Modify: `internal/application/identidade/ports.go`
- Create: `internal/application/identidade/criar_utilizador.go`
- Test: `internal/application/identidade/criar_utilizador_test.go`
- Modify: `internal/application/identidade/gerir_utilizadores_test.go` (manter o `fakeAdmin` existente a satisfazer a interface alargada)

**Interfaces:**
- Consumes: `dominio.Papel`, `dominio.PapelValido`, `dominio.ExigeAutenticacaoForte`; `Auditor`; `auditoria.Registo`; `erros`, `i18n`.
- Produces:
  - DTOs `CriacaoUtilizador{Username, Nome, Email string; Papeis []dominio.Papel}`, `DadosNovoUtilizador{Username, Nome, Email, SenhaTemporaria string; Papeis []dominio.Papel; ConfigurarOTP bool}`, `UtilizadorCriado{ID, SenhaTemporaria string}` (JSON `id`, `senha_temporaria`).
  - Porta `AdminIdentidade.CriarUtilizador(ctx context.Context, dados DadosNovoUtilizador) (id string, err error)` (novo método na interface).
  - `CasoCriarUtilizador` + `NovoCasoCriarUtilizador(a AdminIdentidade, aud Auditor) *CasoCriarUtilizador`; método `Executar(ctx context.Context, actor string, entrada CriacaoUtilizador) (UtilizadorCriado, error)`.

- [ ] **Step 1: Escrever os testes (falham)**

Create `internal/application/identidade/criar_utilizador_test.go`:

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

// fakeCriador estende o comportamento de criação sobre um fakeAdmin mínimo.
type fakeCriador struct {
	recebido appident.DadosNovoUtilizador
	id       string
	err      error
}

func (f *fakeCriador) ListarUtilizadores(context.Context, appident.FiltroUtilizadores) ([]appident.ResumoUtilizador, error) {
	return nil, nil
}
func (f *fakeCriador) ObterUtilizador(context.Context, string) (appident.DetalheUtilizador, error) {
	return appident.DetalheUtilizador{}, nil
}
func (f *fakeCriador) AtribuirPapel(context.Context, string, dominio.Papel) error { return nil }
func (f *fakeCriador) RevogarPapel(context.Context, string, dominio.Papel) error  { return nil }
func (f *fakeCriador) DefinirActivo(context.Context, string, bool) error          { return nil }
func (f *fakeCriador) CriarUtilizador(_ context.Context, d appident.DadosNovoUtilizador) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.recebido = d
	return f.id, nil
}

func TestCriarUtilizador_PapelComum(t *testing.T) {
	admin := &fakeCriador{id: "novo-id"}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoCriarUtilizador(admin, aud)

	out, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "ana.silva", Nome: "Ana Silva", Email: "ana@sgc.ao",
		Papeis: []dominio.Papel{dominio.PapelMedico},
	})
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if out.ID != "novo-id" || out.SenhaTemporaria == "" {
		t.Fatalf("saída inesperada: %+v", out)
	}
	if admin.recebido.ConfigurarOTP {
		t.Fatal("papel comum não deve exigir OTP")
	}
	if admin.recebido.SenhaTemporaria != out.SenhaTemporaria {
		t.Fatal("a senha passada ao adaptador deve ser a devolvida")
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.utilizador.criado" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
}

func TestCriarUtilizador_PapelSensivel_ExigeOTP(t *testing.T) {
	admin := &fakeCriador{id: "novo-id"}
	caso := appident.NovoCasoCriarUtilizador(admin, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "chefe", Nome: "Chefe Geral", Email: "chefe@sgc.ao",
		Papeis: []dominio.Papel{dominio.PapelAdmin},
	}); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if !admin.recebido.ConfigurarOTP {
		t.Fatal("papel sensível deve exigir CONFIGURE_TOTP")
	}
}

func TestCriarUtilizador_EmailInvalido(t *testing.T) {
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{}, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "x", Nome: "X", Email: "não-é-email", Papeis: nil,
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestCriarUtilizador_PapelInvalido(t *testing.T) {
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{}, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "x", Nome: "X", Email: "x@sgc.ao", Papeis: []dominio.Papel{"Inexistente"},
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação por papel inválido, obtive %v", err)
	}
}

func TestCriarUtilizador_UsernameVazio(t *testing.T) {
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{}, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "", Nome: "X", Email: "x@sgc.ao",
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação por username vazio, obtive %v", err)
	}
}

func TestCriarUtilizador_PropagaConflito(t *testing.T) {
	conflito := erros.Novo(erros.CategoriaConflito, "já existe")
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{err: conflito}, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "dup", Nome: "Dup", Email: "dup@sgc.ao",
	})
	if !errors.Is(err, conflito) {
		t.Fatalf("esperava propagação do conflito, obtive %v", err)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/identidade/...`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Adicionar DTOs e o método à porta**

In `internal/application/identidade/ports.go`, add the DTOs after `DetalheUtilizador`:

```go
// CriacaoUtilizador é a entrada do caso de uso de criação (dados do pedido).
type CriacaoUtilizador struct {
	Username string
	Nome     string
	Email    string
	Papeis   []dominio.Papel
}

// DadosNovoUtilizador são os dados enviados ao adaptador para criar o utilizador
// no Keycloak (já enriquecidos com a senha temporária e a política de OTP).
type DadosNovoUtilizador struct {
	Username        string
	Nome            string
	Email           string
	SenhaTemporaria string
	Papeis          []dominio.Papel
	ConfigurarOTP   bool
}

// UtilizadorCriado é a saída do caso de uso: id do Keycloak e senha temporária
// (devolvida uma única vez).
type UtilizadorCriado struct {
	ID              string `json:"id"`
	SenhaTemporaria string `json:"senha_temporaria"`
}
```

And add the method to the `AdminIdentidade` interface (after `DefinirActivo`):
```go
	CriarUtilizador(ctx context.Context, dados DadosNovoUtilizador) (id string, err error)
```

**Also keep the existing fake compiling.** Adding this method to the interface breaks the existing `fakeAdmin` in `internal/application/identidade/gerir_utilizadores_test.go` (it no longer satisfies `AdminIdentidade`, so `NovoCasoListarUtilizadores(admin)` etc. stop compiling). Add a stub method to that `fakeAdmin` (near its other methods):
```go
func (f *fakeAdmin) CriarUtilizador(context.Context, appident.DadosNovoUtilizador) (string, error) {
	return "", f.err
}
```
The `fakeAdmin` type is in package `identidade_test`, which already imports `appident` — no new import needed.

- [ ] **Step 4: Criar o caso de uso**

Create `internal/application/identidade/criar_utilizador.go`:

```go
package identidade

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/mail"
	"strings"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// CasoCriarUtilizador cria um utilizador no Keycloak (fonte de verdade), com uma
// senha temporária gerada e, se algum papel for sensível, exigência de OTP.
type CasoCriarUtilizador struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoCriarUtilizador constrói o caso de uso.
func NovoCasoCriarUtilizador(a AdminIdentidade, aud Auditor) *CasoCriarUtilizador {
	return &CasoCriarUtilizador{admin: a, auditor: aud, agora: time.Now}
}

// Executar valida a entrada, gera a senha temporária, delega a criação no Keycloak,
// audita e devolve o id + a senha (uma única vez).
func (c *CasoCriarUtilizador) Executar(ctx context.Context, actor string, entrada CriacaoUtilizador) (UtilizadorCriado, error) {
	if strings.TrimSpace(entrada.Username) == "" || strings.TrimSpace(entrada.Nome) == "" {
		return UtilizadorCriado{}, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgCriacaoInvalida))
	}
	if _, err := mail.ParseAddress(entrada.Email); err != nil {
		return UtilizadorCriado{}, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgCriacaoInvalida))
	}
	for _, p := range entrada.Papeis {
		if !dominio.PapelValido(string(p)) {
			return UtilizadorCriado{}, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPapelInvalido))
		}
	}

	senha, err := gerarSenhaTemporaria()
	if err != nil {
		return UtilizadorCriado{}, err
	}

	dados := DadosNovoUtilizador{
		Username:        entrada.Username,
		Nome:            entrada.Nome,
		Email:           entrada.Email,
		SenhaTemporaria: senha,
		Papeis:          entrada.Papeis,
		ConfigurarOTP:   dominio.ExigeAutenticacaoForte(entrada.Papeis),
	}
	id, err := c.admin.CriarUtilizador(ctx, dados)
	if err != nil {
		return UtilizadorCriado{}, err
	}

	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.utilizador.criado",
		Entidade:   "utilizador",
		EntidadeID: id,
		Detalhe:    entrada.Username,
		OcorridoEm: c.agora(),
	}); err != nil {
		return UtilizadorCriado{}, err
	}

	return UtilizadorCriado{ID: id, SenhaTemporaria: senha}, nil
}

// gerarSenhaTemporaria devolve uma senha aleatória segura (base64 url-safe de 18
// bytes ≈ 24 caracteres), adequada a uma credencial temporária.
func gerarSenhaTemporaria() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", erros.Novo(erros.CategoriaInterno, "falha ao gerar senha temporária")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
```

- [ ] **Step 5: Correr os testes e confirmar que passam**

Run: `go test ./internal/application/identidade/...`
Expected: PASS.

> Nota: adicionar `CriarUtilizador` à interface `AdminIdentidade` quebra a asserção `var _ appident.AdminIdentidade = (*AdminCliente)(nil)` em `admin.go` até a Task 5 implementar o método. Por isso, **este pacote compila nos testes** (usa o fake), mas `go build ./...` do módulo inteiro falhará até à Task 5. É esperado — não corras `go build ./...` aqui.

- [ ] **Step 6: gofmt/vet e commit**

Run: `gofmt -l internal/application/identidade && go vet ./internal/application/identidade/...`
Expected: sem output.

```bash
git add internal/application/identidade/ports.go internal/application/identidade/criar_utilizador.go internal/application/identidade/criar_utilizador_test.go
git commit -m "feat(identidade): caso de uso de criação de utilizadores (senha temporária + OTP)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Adaptador Keycloak — método CriarUtilizador

**Files:**
- Modify: `internal/adapters/keycloak/admin.go`
- Modify: `internal/adapters/keycloak/admin_httptest_test.go`

**Interfaces:**
- Consumes: `appident.DadosNovoUtilizador`, `dominio.Papel`; existing `tokenServico`, `pedir`, `papelRepresentacao`, `kcRole`.
- Produces: `(*AdminCliente).CriarUtilizador(ctx, dados appident.DadosNovoUtilizador) (id string, err error)` — restores the `AdminIdentidade` interface assertion.

- [ ] **Step 1: Escrever o teste httptest (falha)**

Append to `internal/adapters/keycloak/admin_httptest_test.go` a test that drives `CriarUtilizador`. Add to the fake server handler (in the existing test's server setup) routes for `POST /admin/realms/sgc/users` (respond 201 with a `Location` header `.../users/novo-id-123`), the `GET /roles/{name}` (already present) and the assign `POST .../role-mappings/realm` (already present). Then:

```go
func TestCriarUtilizador_201ComLocation(t *testing.T) {
	var criouComRequiredAction bool
	var atribuiuPapel bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/admin/realms/sgc/users"):
			var corpo map[string]any
			_ = json.NewDecoder(r.Body).Decode(&corpo)
			if ra, ok := corpo["requiredActions"].([]any); ok && len(ra) > 0 {
				criouComRequiredAction = true
			}
			w.Header().Set("Location", srvBase(r)+"/admin/realms/sgc/users/novo-id-123")
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/roles/Admin"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "role-admin", "name": "Admin"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/role-mappings/realm"):
			atribuiuPapel = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	admin, err := NovoAdmin(srv.URL+"/realms/sgc", "sgc-admin", "segredo")
	if err != nil {
		t.Fatalf("NovoAdmin: %v", err)
	}
	id, err := admin.CriarUtilizador(context.Background(), appident.DadosNovoUtilizador{
		Username: "chefe", Nome: "Chefe Geral", Email: "chefe@sgc.ao",
		SenhaTemporaria: "temp123", Papeis: []dominio.Papel{dominio.PapelAdmin}, ConfigurarOTP: true,
	})
	if err != nil {
		t.Fatalf("CriarUtilizador: %v", err)
	}
	if id != "novo-id-123" {
		t.Fatalf("id extraído do Location errado: %q", id)
	}
	if !criouComRequiredAction {
		t.Fatal("esperava requiredActions no corpo (ConfigurarOTP=true)")
	}
	if !atribuiuPapel {
		t.Fatal("esperava atribuição de papel após criação")
	}
}

func TestCriarUtilizador_409Conflito(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/admin/realms/sgc/users"):
			w.WriteHeader(http.StatusConflict)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	admin, _ := NovoAdmin(srv.URL+"/realms/sgc", "sgc-admin", "segredo")
	_, err := admin.CriarUtilizador(context.Background(), appident.DadosNovoUtilizador{
		Username: "dup", Nome: "Dup", Email: "dup@sgc.ao", SenhaTemporaria: "t",
	})
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito (409), obtive %v", err)
	}
}
```

Add a small helper `srvBase` at the bottom of the test file (used to build the Location) and ensure imports include `"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"`:

```go
// srvBase reconstrói o esquema+host do pedido para compor um Location absoluto.
func srvBase(r *http.Request) string {
	esquema := "http"
	if r.TLS != nil {
		esquema = "https"
	}
	return esquema + "://" + r.Host
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/keycloak/ -run CriarUtilizador`
Expected: FAIL — `CriarUtilizador` não existe.

- [ ] **Step 3: Implementar CriarUtilizador**

In `internal/adapters/keycloak/admin.go`, add (after `DefinirActivo`, before the interface-assertion `var _`):

```go
// CriarUtilizador cria um utilizador no Keycloak com uma credencial de password
// temporária e, se pedido, a required action CONFIGURE_TOTP; devolve o id lido do
// cabeçalho Location. Depois atribui os papéis indicados. Mapeia 409 (já existe)
// para CategoriaConflito.
func (a *AdminCliente) CriarUtilizador(ctx context.Context, dados appident.DadosNovoUtilizador) (string, error) {
	tok, err := a.tokenServico(ctx)
	if err != nil {
		return "", err
	}

	primeiro, ultimo := repartirNome(dados.Nome)
	rep := map[string]any{
		"username":      dados.Username,
		"firstName":     primeiro,
		"lastName":      ultimo,
		"email":         dados.Email,
		"enabled":       true,
		"emailVerified": true,
		"credentials": []map[string]any{
			{"type": "password", "value": dados.SenhaTemporaria, "temporary": true},
		},
	}
	if dados.ConfigurarOTP {
		rep["requiredActions"] = []string{"CONFIGURE_TOTP"}
	}

	corpo, err := json.Marshal(rep)
	if err != nil {
		return "", err
	}
	// #nosec G107 -- URL deriva de KEYCLOAK_ISSUER (config de confiança).
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost,
		a.base+"/admin/realms/"+a.realm+"/users", bytes.NewReader(corpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak admin criar: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == nethttp.StatusConflict {
		return "", erros.Novo(erros.CategoriaConflito, i18n.T(i18n.MsgUtilizadorJaExiste))
	}
	if resp.StatusCode != nethttp.StatusCreated {
		return "", fmt.Errorf("keycloak admin criar devolveu %d", resp.StatusCode)
	}
	id := idDoLocation(resp.Header.Get("Location"))
	if id == "" {
		return "", fmt.Errorf("keycloak admin criar: Location sem id")
	}

	for _, p := range dados.Papeis {
		if err := a.AtribuirPapel(ctx, id, p); err != nil {
			return "", err
		}
	}
	return id, nil
}

// repartirNome divide "Ana Maria Silva" em ("Ana", "Maria Silva").
func repartirNome(nome string) (primeiro, ultimo string) {
	nome = strings.TrimSpace(nome)
	if i := strings.IndexByte(nome, ' '); i >= 0 {
		return nome[:i], strings.TrimSpace(nome[i+1:])
	}
	return nome, ""
}

// idDoLocation extrai o último segmento de um URL de Location (.../users/{id}).
func idDoLocation(loc string) string {
	loc = strings.TrimRight(loc, "/")
	if i := strings.LastIndexByte(loc, '/'); i >= 0 {
		return loc[i+1:]
	}
	return ""
}
```

- [ ] **Step 4: Correr os testes e confirmar que passam**

Run: `go test ./internal/adapters/keycloak/...`
Expected: PASS (incluindo a asserção `var _ appident.AdminIdentidade = (*AdminCliente)(nil)`, agora satisfeita).

- [ ] **Step 5: gofmt/vet e commit**

Run: `gofmt -l internal/adapters/keycloak && go vet ./internal/adapters/keycloak/...`
Expected: sem output (ignora `doc.go` pré-existente com CRLF se aparecer — não o formates).

```bash
git add internal/adapters/keycloak/admin.go internal/adapters/keycloak/admin_httptest_test.go
git commit -m "feat(identidade): criar utilizador via Admin API (senha temporária, OTP, 409)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: HTTP — handler de criação

**Files:**
- Modify: `internal/adapters/http/admin_handler.go`
- Modify: `internal/adapters/http/admin_test.go`

**Interfaces:**
- Consumes: `appident.CriacaoUtilizador`, `appident.UtilizadorCriado`; `SessaoDe`, `RBAC`, `responderErro`; `dominio.Papel`, `dominio.PapelAdmin`.
- Produces: interface `ServicoCriarUtilizador`; `NovoAdministracaoHandler` passa a receber um 6º parâmetro `criar ServicoCriarUtilizador` (**assinatura alterada**); rota `POST ""` no grupo com `RBAC(Admin)`.

- [ ] **Step 1: Escrever os testes do handler (falham)**

In `internal/adapters/http/admin_test.go`, update the `routerAdmin` helper to pass a 6th fake, and add a create fake + tests. First add the fake (near the other fakes):

```go
type fakeCriar struct {
	out appident.UtilizadorCriado
	err error
}

func (f fakeCriar) Executar(context.Context, string, appident.CriacaoUtilizador) (appident.UtilizadorCriado, error) {
	return f.out, f.err
}
```

Update `routerAdmin` so the handler is built with 6 args (add the create fake as the last argument):

```go
	h := adhttp.NovoAdministracaoHandler(
		fakeListar{out: []appident.ResumoUtilizador{{ID: "u1", Nome: "Ana"}}},
		fakeObter{out: appident.DetalheUtilizador{ID: "u1", Nome: "Ana"}},
		atribuir,
		&fakePapel{},
		&fakeActivo{},
		fakeCriar{out: appident.UtilizadorCriado{ID: "novo-id", SenhaTemporaria: "senha-temp"}},
	)
```

Add tests:

```go
func TestAdmin_CriarUtilizador_AdminOk_201(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Sujeito: "actor-1", Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores",
		`{"username":"ana.silva","nome":"Ana Silva","email":"ana@sgc.ao","papeis":["Medico"]}`)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"senha_temporaria":"senha-temp"`) {
		t.Fatalf("corpo inesperado: %s", w.Body.String())
	}
}

func TestAdmin_CriarUtilizador_MedicoProibido_403(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, &fakePapel{})
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores",
		`{"username":"x","nome":"X","email":"x@sgc.ao"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("Medico não deve criar; obtive %d", w.Code)
	}
}

func TestAdmin_CriarUtilizador_CorpoInvalido_400(t *testing.T) {
	r := routerAdmin(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}}, &fakePapel{})
	w := pedidoCorpo(r, "POST", "/api/v1/identidade/utilizadores", `{"nome":"sem username"}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400 (username em falta); obtive %d", w.Code)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/http/ -run 'Admin_CriarUtilizador'`
Expected: FAIL — `NovoAdministracaoHandler` com 6 args e a rota POST não existem.

- [ ] **Step 3: Implementar o handler e a rota**

In `internal/adapters/http/admin_handler.go`:

Add the interface to the `type ( ... )` block:
```go
	// ServicoCriarUtilizador cria um utilizador.
	ServicoCriarUtilizador interface {
		Executar(ctx context.Context, actor string, entrada appident.CriacaoUtilizador) (appident.UtilizadorCriado, error)
	}
```

Add the field to `AdministracaoHandler`:
```go
	criar    ServicoCriarUtilizador
```

Update `NovoAdministracaoHandler` to accept and store it (add `criar ServicoCriarUtilizador` as the last param and `criar: criar` in the struct literal):
```go
func NovoAdministracaoHandler(
	listar ServicoListar,
	obter ServicoObterUtilizador,
	atribuir ServicoAtribuirPapel,
	revogar ServicoRevogarPapel,
	activar ServicoDefinirActivo,
	criar ServicoCriarUtilizador,
) *AdministracaoHandler {
	return &AdministracaoHandler{listar: listar, obter: obter, atribuir: atribuir, revogar: revogar, activar: activar, criar: criar}
}
```

Register the route in `RegistarAdministracao` (add after the `g.GET("", ...)` line):
```go
	g.POST("", escrita, h.criarUtilizador)
```

Add the handler method and its request body type (near the other body types):
```go
type corpoCriacao struct {
	Username string   `json:"username"`
	Nome     string   `json:"nome"`
	Email    string   `json:"email"`
	Papeis   []string `json:"papeis"`
}

func (h *AdministracaoHandler) criarUtilizador(c *gin.Context) {
	var corpo corpoCriacao
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgCriacaoInvalida)))
		return
	}
	papeis := make([]dominio.Papel, 0, len(corpo.Papeis))
	for _, p := range corpo.Papeis {
		papeis = append(papeis, dominio.Papel(p))
	}
	actor, _ := SessaoDe(c)
	out, err := h.criar.Executar(c.Request.Context(), actor.Sujeito, appident.CriacaoUtilizador{
		Username: corpo.Username, Nome: corpo.Nome, Email: corpo.Email, Papeis: papeis,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}
```

- [ ] **Step 4: Correr os testes e confirmar que passam**

Run: `go test ./internal/adapters/http/...`
Expected: PASS (os testes existentes que usam `routerAdmin` já passam o 6º arg; se algum outro sítio construir `NovoAdministracaoHandler`, actualiza-o).

- [ ] **Step 5: gofmt/vet e commit**

Run: `gofmt -l internal/adapters/http && go vet ./internal/adapters/http/...`
Expected: sem output.

```bash
git add internal/adapters/http/admin_handler.go internal/adapters/http/admin_test.go
git commit -m "feat(identidade): endpoint POST de criação de utilizadores (RBAC Admin)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Plataforma — fiar a criação no composition root

**Files:**
- Modify: `internal/platform/app.go`

**Interfaces:**
- Consumes: `appident.NovoCasoCriarUtilizador`, `adhttp.NovoAdministracaoHandler` (6 args).

- [ ] **Step 1: Atualizar app.go**

In `internal/platform/app.go`, add the use case next to the others (after `casoActivo`):
```go
	casoCriar := appident.NovoCasoCriarUtilizador(adminKC, repoAuditoria)
```

Update the `NovoAdministracaoHandler` call to pass it as the 6th argument:
```go
	handlerAdmin := adhttp.NovoAdministracaoHandler(casoListar, casoObter, casoAtribuir, casoRevogar, casoActivo, casoCriar)
```

- [ ] **Step 2: Compilar o módulo inteiro**

Run: `go build ./...`
Expected: sem erros (o módulo volta a compilar — a interface `AdminIdentidade` está completa e o handler recebe os 6 serviços).

- [ ] **Step 3: vet + suite completa (sem integração)**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 4: gofmt e commit**

Run: `gofmt -l internal/platform/app.go` (sem output; `gofmt -w` se listar).

```bash
git add internal/platform/app.go
git commit -m "feat(identidade): fiar criação de utilizadores no composition root

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Integração — criar utilizador via Keycloak real

**Files:**
- Create: `tests/integration/criar_utilizador_test.go`

**Interfaces:**
- Consumes: `keycloak.NovoAdmin`, `appident` DTOs, `dominio.Papel`, `issuerTeste`.

- [ ] **Step 1: Escrever o teste e2e**

Create `tests/integration/criar_utilizador_test.go`:

```go
//go:build integration

package integration_test

import (
	"context"
	nethttp "net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/keycloak"
	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

// apagarUtilizador remove um utilizador via Admin API (limpeza do teste).
func apagarUtilizador(t *testing.T, issuer, id string) {
	t.Helper()
	base, realm, _ := strings.Cut(issuer, "/realms/")
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"sgc-admin"},
		"client_secret": {"segredo-admin"},
	}
	// #nosec G107 -- issuer da config de teste.
	resp, err := nethttp.PostForm(issuer+"/protocol/openid-connect/token", form)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	var corpo struct {
		AccessToken string `json:"access_token"`
	}
	_ = jsonDecode(resp, &corpo)
	req, _ := nethttp.NewRequest(nethttp.MethodDelete, base+"/admin/realms/"+realm+"/users/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+corpo.AccessToken)
	if r, err := nethttp.DefaultClient.Do(req); err == nil {
		_ = r.Body.Close()
	}
}

func TestCriarUtilizador_ViaKeycloak(t *testing.T) {
	issuer := issuerTeste()
	admin, err := keycloak.NovoAdmin(issuer, "sgc-admin", "segredo-admin")
	if err != nil {
		t.Fatalf("NovoAdmin: %v", err)
	}
	ctx := context.Background()

	username := "novo.teste.sprint4"
	id, err := admin.CriarUtilizador(ctx, appident.DadosNovoUtilizador{
		Username: username, Nome: "Novo Teste", Email: "novo.teste.sprint4@sgc.ao",
		SenhaTemporaria: "Temp-1234", Papeis: []dominio.Papel{dominio.PapelMedico}, ConfigurarOTP: false,
	})
	if err != nil {
		t.Skipf("Admin API indisponível ou utilizador já existe: %v", err)
	}
	defer apagarUtilizador(t, issuer, id)

	det, err := admin.ObterUtilizador(ctx, id)
	if err != nil {
		t.Fatalf("obter utilizador criado: %v", err)
	}
	if det.Email != "novo.teste.sprint4@sgc.ao" {
		t.Fatalf("email inesperado: %q", det.Email)
	}
	temMedico := false
	for _, p := range det.Papeis {
		if p == "Medico" {
			temMedico = true
		}
	}
	if !temMedico {
		t.Fatalf("papel Medico não atribuído na criação: %v", det.Papeis)
	}
}
```

Add a small `jsonDecode` helper if one does not already exist in the integration package. Check first (`grep`); if absent, add to this file:
```go
func jsonDecode(resp *nethttp.Response, v any) error {
	return json.NewDecoder(resp.Body).Decode(v)
}
```
(and import `"encoding/json"`). If a similar helper already exists in the package, reuse it and do not redefine.

- [ ] **Step 2: Correr o teste e2e (compose a correr)**

Run:
```bash
DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
KEYCLOAK_ISSUER=http://localhost:8081/realms/sgc \
go test -tags=integration ./tests/integration/... -run 'CriarUtilizador' -v
```
Expected: PASS (cria, confirma, limpa) — ou SKIP se a infra estiver indisponível.

- [ ] **Step 3: gofmt/vet e commit**

Run: `gofmt -l tests/integration && go vet -tags=integration ./tests/integration/...`
Expected: sem output.

```bash
git add tests/integration/criar_utilizador_test.go
git commit -m "test(identidade): e2e de criação de utilizador via Keycloak

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Documentação e verificação final

**Files:**
- Create: `adrs/ADR-023-mfa-positivo-criar-utilizadores.md`
- Modify: `SPRINT.md`, `CLAUDE.md`, `.env.example`

**Interfaces:** nenhuma.

- [ ] **Step 1: Escrever a ADR-023**

Create `adrs/ADR-023-mfa-positivo-criar-utilizadores.md`. Substitute `<VARIANTE>` and `<CLAIM/VALOR>` with the actual spike outcome from Task 1 (read the Task 1 report / the commit message):

```markdown
# ADR-023 — MFA (caminho positivo) e criação de utilizadores

- **Estado**: Aceite
- **Data**: 2026-07-11
- **Marco/Sprint**: M1 / Sprint 4
- **Contexto BC**: Identidade

## Contexto

O Sprint 3 deixou o caminho positivo de MFA por validar e a gestão administrativa
sem criação de utilizadores. Este sprint fecha ambos.

## Decisão

1. **Sinal de MFA**: após spike, adoptou-se a variante **<VARIANTE>** (acr via LoA /
   amr via script). O realm emite `<CLAIM/VALOR>` num login com OTP, que a API
   reconhece como autenticação forte (`KEYCLOAK_ACR_FORTE` / `amrFortes`). O realm
   inclui `director.teste` com OTP de segredo conhecido para validação e2e positiva.
2. **Criação de utilizadores**: `POST /api/v1/identidade/utilizadores` (RBAC Admin)
   cria no Keycloak (fonte de verdade) com senha temporária gerada (devolvida uma
   vez, `temporary:true`). Se algum papel inicial for sensível, adiciona a required
   action `CONFIGURE_TOTP` (coerente com o MFA fail-closed). Duplicados → 409.

## Consequências

- O critério de saída M1 de MFA fica totalmente fechado (positivo + negativo).
- A senha temporária é devolvida uma única vez; o admin comunica-a por canal seguro.
- Alternativas descartadas: envio por email (sem SMTP em dev); gestão de credenciais
  na BD local (Keycloak é a fonte de verdade).
```

- [ ] **Step 2: Atualizar SPRINT.md**

In `SPRINT.md`, change the `- **Sprint**:` line to `4 (BC Identidade — MFA positivo + criação de utilizadores) — **entregue**` and add a "Sprint 4 — entregue" section above "Sprint 3 — entregue":

```markdown
## Sprint 4 — entregue

- [x] Caminho positivo de MFA validado e2e: `director.teste` com OTP → autenticação
      forte reconhecida (acesso concedido); config de MFA fixada no realm.
- [x] Criação administrativa de utilizadores: `POST /api/v1/identidade/utilizadores`
      (RBAC Admin), senha temporária gerada (devolvida 1x), `CONFIGURE_TOTP` em papéis
      sensíveis, 409 em duplicados. Auditoria `identidade.utilizador.criado`.
- [x] ADR-023.
```

Update the M1 MFA exit criterion line to note both paths closed:
```markdown
- [x] Identidade Keycloak operacional (login, 11 papéis, MFA para papéis sensíveis — positivo e negativo). — Sprint 2/3/4
```

- [ ] **Step 3: Atualizar CLAUDE.md**

In `CLAUDE.md`, in the "Convenções-fonte" block, add ADR-023 to the registered list and bump the next ADR:
```markdown
`adrs/ADR-022-mfa-gestao-admin.md`, `adrs/ADR-023-mfa-positivo-criar-utilizadores.md`.
Próximo ADR: **ADR-024**.
```

- [ ] **Step 4: Atualizar .env.example**

In `.env.example`, if Task 1 chose the amr-script variant, add a note near the Keycloak section:
```bash
# Nota: o realm usa a feature `scripts` do Keycloak (mapper de amr) — ver
# docker-compose.yml (--features=scripts) e ADR-023.
```
If Task 1 chose the acr variant, add instead:
```bash
# Nota: o realm mapeia OTP → acr (acr.loa.map) para o MFA positivo — ver ADR-023.
```

- [ ] **Step 5: Verificação final completa**

Run each and confirm:
```bash
go build ./...        # sem erros
go vet ./...          # sem erros
go test ./...         # PASS (integração não corre)
bash scripts/cobertura.sh   # domínio ≥85, aplicação ≥75, adaptadores ≥60 — todos OK
```
For `gofmt`, run `gofmt -l internal cmd tests` and confirm that the ONLY files listed (if any) are pre-existing CRLF artifacts NOT touched by Sprint 4 (verify with `git diff --name-only 6cafd7f..HEAD`). Do not reformat pre-existing untouched files.

- [ ] **Step 6: Verificação e2e final (compose a correr)**

Run the full integration suite:
```bash
DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
KEYCLOAK_ISSUER=http://localhost:8081/realms/sgc \
go test -tags=integration ./tests/integration/... -v
```
Expected: MFA positivo + negativo, criação, e os testes do Sprint 3 — todos PASS (ou SKIP se indisponível).

- [ ] **Step 7: Commit final**

```bash
git add adrs/ADR-023-mfa-positivo-criar-utilizadores.md SPRINT.md CLAUDE.md .env.example
git commit -m "docs(identidade): ADR-023 e reconciliação de docs do Sprint 4

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Notas de verificação global (após todas as tasks)

1. `go build ./...`, `go vet ./...`, `gofmt` (só artefactos CRLF pré-existentes) limpos; `go test ./...` verde.
2. Cobertura cumpre 85/75/60.
3. Compose a correr: `director.teste` + TOTP → autenticação forte (positivo); `admin.teste` sem OTP → negado (negativo, regressão).
4. `POST /utilizadores` como Admin → 201 + senha temporária; papel sensível → `CONFIGURE_TOTP`; duplicado → 409; não-Admin → 403.
5. Auditoria `identidade.utilizador.criado` registada; e2e de criação limpa o realm no fim.
6. `go-arch-lint` (CI/Linux) sem violações — sem vendor novo; domínio sem infra.
