# Imposição uniforme de MFA e factura nascida em RASCUNHO (ADR-042) — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Impor MFA a todos os grupos de rotas de negócio por construção, e impedir que uma `Factura` nasça em estado `EMITIDA`.

**Architecture:** Um único pacote `protecao` de middlewares passado aos 14 grupos em `app.go`, tornando a omissão impossível por esquecimento; os routers de teste passam a espelhar essa cadeia; e uma guarda sobre o próprio `app.go` detecta divergências futuras. Na base de dados, um trigger `BEFORE INSERT` obriga toda a factura a nascer `RASCUNHO`.

**Tech Stack:** Go 1.22+, Gin, PostgreSQL 16, Keycloak 25 (realm de desenvolvimento).

**Spec:** `docs/superpowers/specs/2026-07-19-adr-042-mfa-uniforme-design.md`

## Global Constraints

- **Idioma:** PT-PT angolano **com diacríticos** em código, comentários, documentação e commits. Nunca PT-BR, nunca inglês visível. Várias mensagens sem acentos foram devolvidas nas sprints anteriores — **verifica a tua antes de commitar**.
- **Linguagem ubíqua:** `Factura`, `ItemFactura`, `Papel`, `Sessão`, `Utilizador`, `Série`, `Número`, `Episódio`.
- **Camadas:** `internal/domain/` e `internal/application/` não importam `pgx`, `gin` nem `net/http`.
- **Erros:** nunca `panic()`. Sempre `erros.Novo(erros.Categoria…, "mensagem")`.
- **Migrações forward-only.** **Nunca editar `financeiro/0001`, `0002` nem `0003`** — editar uma migração já aplicada causou um incidente na Sprint 15. A migração desta fatia é a `0004`, nova.
- **`RegistarHealth` NUNCA recebe protecção** — healthchecks e o *scrape* do Prometheus são não-autenticados por desenho.
- **Cobertura:** domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
- **`go-arch-lint check` tem de sair com código 0.** Verificar o **código de saída**, não só o texto.

---

## File Structure

| Ficheiro | Responsabilidade |
|---|---|
| `internal/platform/app.go` (modificar) | pacote `protecao` nos 14 grupos |
| `internal/platform/app_protecao_test.go` (criar) | guarda: toda a chamada `Registar*` usa `protecao...` |
| `internal/adapters/http/identidade_handler.go` (modificar) | `middlewares` → `protecao` |
| `internal/adapters/http/*_test.go` (modificar) | routers espelham a produção; sessões sensíveis ganham 2.º factor |
| `docker/keycloak/realm-sgc.json` (modificar) | OTP no `admin.teste`; `dpo.teste` e `auditor.teste` |
| `migrations/financeiro/0004_facturas_nascem_rascunho.sql` (criar) | trigger `BEFORE INSERT` |
| `tests/integration/facturas_test.go` (modificar) | 3 fixturas passam ao caminho de produção |
| `adrs/ADR-042-mfa-uniforme.md` (criar), `adrs/ADR-040-emissao-factura.md` (modificar) | ADR e resolução do R3/R6 |
| `CLAUDE.md`, `SPRINT.md` (modificar) | marco e sprint |

**Ordem:** a Task 1 (produção) antes da Task 2 (testes espelham), porque a guarda da Task 1 falha enquanto os locais de chamada não estiverem uniformes. A Task 5 (R6) é independente e pode correr a qualquer momento depois da 1.

---

## Task 1: Pacote `protecao` e guarda sobre o `app.go`

**Files:**
- Modify: `internal/platform/app.go` (bloco `registarRotas`, ~linhas 274-289)
- Modify: `internal/adapters/http/identidade_handler.go:37-40`
- Create: `internal/platform/app_protecao_test.go`

**Interfaces:**
- Consumes: `adhttp.LimiteTaxa`, `adhttp.Auth`, `adhttp.MFAObrigatoria` — já existem em `app.go` como `limiteMW`, `authMW`, `mfaMW`.
- Produces: nada para tarefas seguintes em código; a guarda protege o invariante.

**Contexto:** as 14 funções `Registar*` de negócio já são **variádicas** e já chamam ao parâmetro `protecao` — 13 das 14 com esse nome exacto. `RegistarIdentidade` chama-lhe `middlewares`. Nenhuma assinatura muda nesta tarefa; muda o que os locais de chamada passam.

- [ ] **Step 1: Escrever a guarda que falha**

Criar `internal/platform/app_protecao_test.go`:

```go
package platform

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A exposição que a ADR-042 corrige (R3 da ADR-040) existiu porque cada local de
// chamada escolhia os seus middlewares: três grupos receberam o mfaMW e onze não,
// e nada tornava isso visível. O pacote `protecao` torna a omissão um desvio
// deliberado — esta guarda torna-a detectável.
//
// O teste lê o código-fonte em vez de exercitar comportamento. É invulgar, e a
// razão está registada na ADR-042 §4.1: o que falhou aqui foi a LIGAÇÃO, não o
// comportamento, e prová-la por comportamento exigiria montar os catorze
// handlers com todos os seus fakes só para verificar uma propriedade de wiring.
//
// RegistarHealth está isento por desenho: healthchecks e o scrape do Prometheus
// são não-autenticados. Isentá-lo aqui é deliberado, não esquecimento.
func TestRegistarRotas_TodasAsRotasDeNegocioUsamOPacoteProteccao(t *testing.T) {
	fonte, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatalf("ler app.go: %v", err)
	}
	corpo := string(fonte)

	inicio := strings.Index(corpo, "registarRotas := func(")
	if inicio < 0 {
		t.Fatal("não encontrei registarRotas em app.go — o teste precisa de ser actualizado")
	}
	fim := strings.Index(corpo[inicio:], "\n\t}")
	if fim < 0 {
		t.Fatal("não encontrei o fim de registarRotas em app.go")
	}
	bloco := corpo[inicio : inicio+fim]

	chamadas := regexp.MustCompile(`adhttp\.(Registar\w+)\(([^\n]*)\)`).FindAllStringSubmatch(bloco, -1)
	if len(chamadas) == 0 {
		t.Fatal("não encontrei chamadas adhttp.Registar* dentro de registarRotas")
	}

	var semProteccao []string
	for _, c := range chamadas {
		nome, argumentos := c[1], c[2]
		if nome == "RegistarHealth" {
			continue // isento por desenho: não-autenticado
		}
		if !strings.HasSuffix(strings.TrimSpace(argumentos), "protecao...") {
			semProteccao = append(semProteccao, nome)
		}
	}
	if len(semProteccao) > 0 {
		t.Errorf("grupos de rotas sem o pacote `protecao`: %v\n"+
			"Todo o grupo de negócio tem de terminar em `protecao...`. Se um grupo "+
			"tiver mesmo de ficar sem MFA, isso exige uma ADR — não uma excepção "+
			"silenciosa aqui.", semProteccao)
	}
	if len(chamadas) < 14 {
		t.Errorf("esperava pelo menos 14 grupos de rotas, encontrei %d — "+
			"se um grupo foi removido, actualiza este número deliberadamente", len(chamadas))
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/platform/ -run TodasAsRotasDeNegocio -v`
Expected: FAIL, listando os 11 grupos sem `protecao` (Doentes, Episodios, Consentimentos, Cirurgia, Farmacia, FarmaciaStock, Laboratorio, Recepcao, RecepcaoChegadas, RecepcaoTriagem, ClinicoConsulta).

- [ ] **Step 3: Implementar o pacote `protecao`**

Em `internal/platform/app.go`, substituir o bloco `registarRotas` por:

```go
	// Pacote único de protecção aplicado a TODOS os grupos de negócio (ADR-042).
	// Antes, cada local de chamada escolhia os seus middlewares: o mfaMW chegava a
	// três grupos e faltava a onze, entre eles Doentes, Consentimentos, Laboratório
	// e Recepção — onde Admin e Director alcançavam dados clínicos sem segundo
	// factor. Um pacote único torna a omissão um desvio deliberado e visível.
	//
	// Custo zero onde não há papéis sensíveis: MFAObrigatoria delega em
	// VerificarAutenticacaoForte, que só rejeita sessões de papel sensível.
	protecao := []gin.HandlerFunc{limiteMW, authMW, mfaMW}

	registarRotas := func(r gin.IRouter) {
		adhttp.RegistarIdentidade(r, handlerIdentidade, protecao...)
		adhttp.RegistarAdministracao(r, handlerAdmin, protecao...)
		adhttp.RegistarDoentes(r, handlerDoentes, protecao...)
		adhttp.RegistarEpisodios(r, handlerEpisodios, protecao...)
		adhttp.RegistarConsentimentos(r, handlerConsentimentos, protecao...)
		adhttp.RegistarCirurgia(r, handlerCirurgia, protecao...)
		adhttp.RegistarFarmacia(r, handlerFarmacia, protecao...)
		adhttp.RegistarFarmaciaStock(r, handlerFarmaciaStock, protecao...)
		adhttp.RegistarLaboratorio(r, handlerLaboratorio, protecao...)
		adhttp.RegistarFinanceiro(r, handlerFinanceiro, protecao...)
		adhttp.RegistarRecepcao(r, handlerRecepcao, protecao...)
		adhttp.RegistarRecepcaoChegadas(r, handlerRecepcaoChegadas, protecao...)
		adhttp.RegistarRecepcaoTriagem(r, handlerRecepcaoTriagem, protecao...)
		adhttp.RegistarClinicoConsulta(r, handlerClinicoConsulta, protecao...)
	}
```

Em `internal/adapters/http/identidade_handler.go:37-40`, uniformizar o nome do parâmetro:

```go
// aplicando ao grupo os middlewares indicados (ex.: rate limit + autenticação + MFA).
func RegistarIdentidade(r gin.IRouter, h *IdentidadeHandler, protecao ...gin.HandlerFunc) {
```

e dentro da função, `grupo.Use(middlewares...)` passa a `grupo.Use(protecao...)`.

- [ ] **Step 4: Correr e confirmar que passa**

Run: `go test ./internal/platform/ -run TodasAsRotasDeNegocio -v`
Expected: PASS

- [ ] **Step 5: Confirmar que a aplicação compila e a suite não regrediu**

Run: `go build ./... && go test ./... -race -count=1`
Expected: PASS. **Nenhum teste HTTP deve falhar aqui** — os routers de teste constroem as suas próprias cadeias e ainda não espelham a produção. Isso é a Task 2. Se algum falhar, investiga antes de prosseguir.

- [ ] **Step 6: Provar a guarda por mutação**

Retirar `protecao...` de uma chamada qualquer (por exemplo `RegistarDoentes`), correr a guarda, confirmar que **falha** nomeando esse grupo, e repor.

Run: `go test ./internal/platform/ -run TodasAsRotasDeNegocio`
Expected: FAIL com `grupos de rotas sem o pacote protecao: [RegistarDoentes]`, e PASS depois de repor. Confirma `git diff` limpo sobre `app.go` antes de continuar.

- [ ] **Step 7: Commit**

```bash
git add internal/platform/app.go internal/platform/app_protecao_test.go internal/adapters/http/identidade_handler.go
git commit -m "feat(segurança): pacote único de protecção em todos os grupos de rotas (ADR-042)"
```

---

## Task 2: Routers de teste espelham a produção

**Files:**
- Modify: todos os `internal/adapters/http/*_test.go` que chamam `adhttp.Registar*` (37 locais, excluindo os 3 de `RegistarHealth`)

**Interfaces:**
- Consumes: `adhttp.MFAObrigatoria()`.
- Produces: routers de teste que exercitam a mesma cadeia que a produção.

**Contexto e porquê:** hoje `routerDoentes` passa só `adhttp.Auth(...)`, enquanto `routerFin` passa `Auth` **e** `MFAObrigatoria()` — a ADR-041 alterou-o. Enquanto os routers de teste não espelharem a produção, **os testes não exercitam a cadeia real** e a exposição do R3 podia repetir-se sem ser detectada.

`RegistarHealth` **não leva** `MFAObrigatoria()` — os três locais em `health_handler_test.go` ficam como estão.

- [ ] **Step 1: Acrescentar `MFAObrigatoria()` a todos os registos de negócio**

Em cada `adhttp.Registar<X>(r, h, adhttp.Auth(fakeAuth{...}))`, acrescentar `, adhttp.MFAObrigatoria()` como último argumento, ficando:

```go
adhttp.RegistarDoentes(r, h, adhttp.Auth(fakeAuth{sessao: sessao}), adhttp.MFAObrigatoria())
```

O `routerFin` (`financeiro_test.go:139`) **já está assim** — serve de modelo e não muda.

- [ ] **Step 2: Correr e ver o que parte**

Run: `go test ./internal/adapters/http/ -count=1`
Expected: FAIL em vários testes, com 403 onde se esperava 2xx. São as sessões de papel sensível sem segundo factor. **Anota a lista** — é o trabalho do passo seguinte.

- [ ] **Step 3: Dar segundo factor às sessões de papel sensível**

Em cada sessão construída com um papel **sensível** (`PapelDirector`, `PapelAdmin`, `PapelDPO`, `PapelAuditor`, `PapelTesoureiro`) que o teste espera que **prossiga**, acrescentar `AutenticacaoForte: true`:

```go
dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}, AutenticacaoForte: true}
```

Sessões com papéis não-sensíveis (`PapelMedico`, `PapelEnfermeiro`, `PapelRecepcionista`, …) **não mudam** — o `MFAObrigatoria` é um no-op para elas.

**⚠️ A armadilha desta tarefa, e é subtil.** Há testes que asseream **403 por RBAC** — por exemplo "um `Auditor` não pode escrever". Com o MFA na cadeia, esses testes passam a receber 403 **do MFA**, antes de o RBAC sequer correr: continuam verdes, mas **deixam de provar o que dizem provar**. Um teste que passa pela razão errada é pior do que um teste que falha.

Regra: **toda a sessão de papel sensível leva `AutenticacaoForte: true`, mesmo nos testes que esperam 403.** Assim o 403 continua a vir do RBAC, que é o que esses testes existem para verificar. Só os testes que verificam explicitamente o MFA é que usam sessão sensível sem segundo factor — e esses são os da Task 3.

Os dois testes de `admin_test.go` que exercitam o `MFAObrigatoria` directamente (`TestMFAObrigatoria_PapelSensivelSemMFA_403` com `AutenticacaoForte: false`, e `..._ComMFA_Prossegue` com `true`) **não mudam** — são precisamente os que devem manter o `false`.

- [ ] **Step 4: Correr até a suite ficar verde**

Run: `go test ./internal/adapters/http/ -race -count=1`
Expected: PASS

- [ ] **Step 5: Correr a suite completa**

Run: `go test ./... -race -count=1 && gofmt -l . && go vet ./...`
Expected: PASS, sem saída em `gofmt` e `go vet`.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/http/
git commit -m "test(segurança): routers de teste espelham a cadeia de protecção da produção (ADR-042)"
```

---

## Task 3: Provas de MFA por família de rotas

**Files:**
- Modify: `internal/adapters/http/doente_test.go`, `consentimento_test.go`, `laboratorio_test.go`, `recepcao_test.go`

**Interfaces:**
- Consumes: os routers de teste da Task 2, já com `MFAObrigatoria()`.

**Contexto:** estas são as quatro famílias cujo RBAC admite papéis sensíveis e que estavam sem MFA. As restantes não expõem papéis sensíveis, pelo que não há nada a provar nelas.

- [ ] **Step 1: Escrever os testes que falham**

Para **cada uma** das quatro famílias, acrescentar um par de testes. Exemplo para Doentes (`doente_test.go`) — adaptar o nome do router, do papel sensível e de uma rota existente a cada ficheiro:

```go
// ADR-042 (R3 da ADR-040): antes desta fatia, o grupo de Doentes não recebia o
// mfaMW, pelo que um Admin alcançava dados clínicos sem segundo factor.
func TestDoentes_PapelSensivelSemSegundoFactor_403(t *testing.T) {
	r := routerDoentes(dominio.Sessao{
		Sujeito: "adm-1",
		Papeis:  []dominio.Papel{dominio.PapelAdmin},
		// sem AutenticacaoForte: é este o ponto do teste
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/doentes/id-1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("código = %d, queria 403", w.Code)
	}
	// Asserir o tipo do problema, e não só o 403: sem isto, o teste não distingue
	// o 403 do MFA do 403 do RBAC, e passaria a verde pela razão errada se o RBAC
	// mudasse.
	if corpo := w.Body.String(); !strings.Contains(corpo, "mfa-obrigatorio") {
		t.Errorf("corpo = %s, queria type mfa-obrigatorio", corpo)
	}
}

func TestDoentes_PapelSensivelComSegundoFactor_Prossegue(t *testing.T) {
	r := routerDoentes(dominio.Sessao{
		Sujeito:           "adm-1",
		Papeis:            []dominio.Papel{dominio.PapelAdmin},
		AutenticacaoForte: true,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/doentes/id-1", nil)
	r.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Errorf("com segundo factor não devia dar 403; corpo = %s", w.Body.String())
	}
}
```

Confirma a rota e o papel sensível certos para cada família antes de escrever — usa uma rota que já exista nos testes desse ficheiro. Para Laboratório e Recepção o papel sensível pode ser `PapelDirector`; para Consentimentos, `PapelAdmin`.

- [ ] **Step 2: Correr**

Run: `go test ./internal/adapters/http/ -run 'SemSegundoFactor|ComSegundoFactor' -v`
Expected: PASS (a Task 2 já ligou o `MFAObrigatoria` aos routers). Se algum **falhar**, é sinal de que o router dessa família não recebeu o `MFAObrigatoria()` na Task 2 — corrige lá.

- [ ] **Step 3: Provar por mutação que os testes mordem**

Retirar `adhttp.MFAObrigatoria()` do router de **uma** família, correr os testes dessa família, confirmar que o teste `_403` **falha** (passa a 2xx), e repor. Confirma `git diff` limpo.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/http/
git commit -m "test(segurança): prova de MFA nas quatro famílias com papéis sensíveis (ADR-042)"
```

---

## Task 4: Credenciais OTP no realm de desenvolvimento

**Files:**
- Modify: `docker/keycloak/realm-sgc.json`

**Contexto:** estado actual dos utilizadores do realm:

| Utilizador | Papéis | Credenciais |
|---|---|---|
| `medico.teste` | `Medico` | password |
| `admin.teste` | `Admin` | password |
| `director.teste` | `Director` | password, **otp** |
| `tesoureiro.teste` | `Tesoureiro` | password, **otp** |

`Admin` é sensível e não tem OTP; não existem utilizadores `DPO` nem `Auditor`. Sem corrigir isto, **o percurso positivo do MFA nunca é exercitado para esses papéis** — o realm passaria nos testes sem provar nada, que é o modo de falha que esta fatia existe para evitar.

- [ ] **Step 1: Acrescentar OTP ao `admin.teste` e criar `dpo.teste` e `auditor.teste`**

Espelhar exactamente a estrutura de `director.teste` (mesmas chaves, mesma forma de credenciais `password` + `otp`), mudando apenas `username`, `email`/`firstName`/`lastName` se existirem, e `realmRoles`.

- [ ] **Step 2: Confirmar que o JSON continua válido**

Run:
```bash
python -c "
import json
d=json.load(open('docker/keycloak/realm-sgc.json',encoding='utf-8'))
for u in d.get('users',[]):
    print(u.get('username'), u.get('realmRoles'), [c.get('type') for c in u.get('credentials',[])])
"
```
Expected: JSON parseia; `admin.teste`, `director.teste`, `tesoureiro.teste`, `dpo.teste` e `auditor.teste` têm todos `otp`.

- [ ] **Step 3: Confirmar que todos os papéis sensíveis têm utilizador com OTP**

Os papéis sensíveis são `Director`, `Admin`, `DPO`, `Auditor` e `Tesoureiro` (`internal/domain/identidade/papel.go`). Confirma que cada um tem pelo menos um utilizador de teste com credencial `otp`.

- [ ] **Step 4: Commit**

```bash
git add docker/keycloak/realm-sgc.json
git commit -m "chore(identidade): credenciais OTP para todos os papéis sensíveis no realm de desenvolvimento (ADR-042)"
```

---

## Task 5: A factura nasce RASCUNHO (R6)

**Files:**
- Create: `migrations/financeiro/0004_facturas_nascem_rascunho.sql`
- Modify: `tests/integration/facturas_test.go` (3 fixturas)

**Contexto:** `trg_facturas_imutaveis` é `BEFORE UPDATE OR DELETE` e não cobre `INSERT`, pelo que um `INSERT` directo com `estado='EMITIDA'` é aceite. O trigger irmão de `itens_factura` já cobre o `INSERT` desde a ADR-040.

**Precedente a seguir:** `migrations/financeiro/0003_itens_factura_imutaveis_insert.sql` faz exactamente esta operação para a outra tabela — lê-o e espelha o padrão (`CREATE OR REPLACE FUNCTION`, `DROP TRIGGER IF EXISTS` + `CREATE TRIGGER`, `RAISE EXCEPTION … USING ERRCODE = 'restrict_violation'`).

**SQLSTATE:** `restrict_violation` produz **`23001`**, não `2F004`.

- [ ] **Step 1: Escrever o teste de integração que falha**

Acrescentar a `tests/integration/facturas_test.go`:

```go
// ADR-042 (R6 da ADR-040): toda a factura tem de nascer RASCUNHO. Um INSERT
// directo já em EMITIDA é fabricação — não altera nada já selado, mas cria um
// documento que nunca passou pelo caminho de emissão.
func TestFacturaNaoPodeNascerEmitida(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)

	_, err := pool.Exec(ctx, `
INSERT INTO financeiro.facturas (estado, cliente_nome, episodio_id, numero, serie, sequencial,
                                 data_emissao, hash, hash_anterior)
VALUES ('EMITIDA','Cliente Fabricado',gen_random_uuid(),$1,'2026',9999996,now(),'hash-falso','')`,
		"FAC 2026/09999996")
	if err == nil {
		t.Fatal("INSERT de uma factura nascida EMITIDA tinha de falhar")
	}
	if !strings.Contains(err.Error(), "23001") {
		t.Errorf("erro = %v, queria SQLSTATE 23001 (restrict_violation)", err)
	}
}

// O caminho normal continua aberto: a factura nasce RASCUNHO e só depois é
// promovida. Fechar isto seria pior do que o buraco que o trigger fecha.
func TestFacturaNasceRascunhoContinuaAPassar(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)

	var id string
	if err := pool.QueryRow(ctx, `
INSERT INTO financeiro.facturas (estado, cliente_nome, episodio_id)
VALUES ('RASCUNHO','Cliente Normal',gen_random_uuid()) RETURNING id::text`).Scan(&id); err != nil {
		t.Fatalf("INSERT de rascunho tinha de passar: %v", err)
	}
	limparFactura(t, pool, ctx, id)
}
```

- [ ] **Step 2: Correr e confirmar que o primeiro falha**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run 'NaoPodeNascerEmitida|NasceRascunho' -v -count=1`
Expected: `TestFacturaNaoPodeNascerEmitida` FAIL com "INSERT de uma factura nascida EMITIDA tinha de falhar"; `TestFacturaNasceRascunhoContinuaAPassar` PASS.

- [ ] **Step 3: Criar a migração**

`migrations/financeiro/0004_facturas_nascem_rascunho.sql`, espelhando o padrão da `0003`:

```sql
-- ADR-042 (R6 da ADR-040): trg_facturas_imutaveis é BEFORE UPDATE OR DELETE e não
-- cobre o INSERT, pelo que uma factura podia nascer já EMITIDA. Isso é fabricação,
-- não mutação: nada do que está selado muda, mas cria-se um documento que nunca
-- passou pelo caminho de emissão e que fica órfão da cadeia (a emissão legítima
-- seguinte lê series.ultimo_hash, não a linha fabricada).
--
-- Nota honesta de âmbito: enquanto o R7 estiver aberto — o papel da aplicação é
-- dono desta tabela e pode correr ALTER TABLE ... DISABLE TRIGGER — este trigger
-- é defesa contra erro e contra SQL directo de terceiros, NÃO contra a aplicação
-- comprometida.
CREATE OR REPLACE FUNCTION financeiro.impedir_factura_nascer_emitida() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'a factura tem de nascer em RASCUNHO: estado % não permitido no INSERT', NEW.estado
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_facturas_nascem_rascunho ON financeiro.facturas;
CREATE TRIGGER trg_facturas_nascem_rascunho
    BEFORE INSERT ON financeiro.facturas
    FOR EACH ROW
    WHEN (NEW.estado <> 'RASCUNHO')
    EXECUTE FUNCTION financeiro.impedir_factura_nascer_emitida();
```

- [ ] **Step 4: Correr e confirmar que ambos passam**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run 'NaoPodeNascerEmitida|NasceRascunho' -v -count=1`
Expected: PASS nos dois.

- [ ] **Step 5: Reescrever as três fixturas que semeiam facturas nascidas EMITIDA**

A suite completa vai agora falhar nas fixturas que fazem `INSERT` directo de `EMITIDA` — linhas ~225, ~440 e ~728 de `tests/integration/facturas_test.go`.

Reescrever cada uma para o **caminho de produção**: `INSERT` em `RASCUNHO`, depois `UPDATE` para `EMITIDA` com os campos de emissão. A ADR-041 já fez exactamente isto a uma quarta fixtura (procura o `SET estado='EMITIDA'` existente no ficheiro e segue esse padrão).

O argumento, que vale a pena repetir no comentário de cada uma: **a rota nova é a real** — nenhum caminho do sistema produz uma factura nascida EMITIDA, pelo que a fixtura antiga é que era artificial.

Atenção ao `ON CONFLICT`: `uq_facturas_numero` é um índice único **parcial** (`WHERE numero IS NOT NULL`); qualquer `ON CONFLICT (numero)` tem de repetir o predicado, senão o Postgres recusa com **42P10**.

- [ ] **Step 6: Correr a suite de integração completa, duas vezes**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/... -count=1` (duas vezes seguidas)
Expected: PASS nas duas.

- [ ] **Step 7: Correr contra uma base de dados criada de raiz**

```bash
docker exec sgc-postgres-1 psql -U sgc -d postgres -c "CREATE DATABASE sgc_adr042;"
DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc_adr042?sslmode=disable' go test -tags=integration ./tests/integration/... -count=1
DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc_adr042?sslmode=disable' go test -tags=integration ./tests/integration/... -count=1
docker exec sgc-postgres-1 psql -U sgc -d postgres -c "DROP DATABASE sgc_adr042;"
```
Expected: PASS nas duas corridas. Apagar a base no fim.

- [ ] **Step 8: Commit**

```bash
git add migrations/financeiro/0004_facturas_nascem_rascunho.sql tests/integration/facturas_test.go
git commit -m "feat(financeiro): a factura passa a nascer obrigatoriamente em RASCUNHO (ADR-042)"
```

---

## Task 6: ADR-042, resolução do R3/R6 e actualização de marco

**Files:**
- Create: `adrs/ADR-042-mfa-uniforme.md`
- Modify: `adrs/ADR-040-emissao-factura.md` (R3 e R6)
- Modify: `CLAUDE.md`, `SPRINT.md`

- [ ] **Step 1: Escrever a ADR-042**

Seguir o formato de `adrs/ADR-041-selagem-canonica.md`. **Cada afirmação tem de ser verdadeira face ao código** — verifica antes de escrever.

Conteúdo obrigatório:

1. **A exposição medida do R3**, por família (Doentes/Admin, Consentimentos/Admin, Laboratório/Admin+Director, Recepção/Admin+Director), e que se tratava de **dados clínicos e consentimentos LPDP**, não só matéria fiscal.
2. **A causa real:** as `Registar*` já eram variádicas e já chamavam ao parâmetro `protecao`; falhou a aplicação uniforme nos locais de chamada. Mesma classe de defeito que a ADR-041 corrigiu no hash — um juízo caso a caso que corre mal em silêncio.
3. **A lacuna de detecção:** os routers de teste construíam cadeias próprias, pelo que nada verificava a ligação em `app.go`. Registar que uma versão anterior do desenho afirmou, sem medir, que ~12 ficheiros de teste passariam a dar 403 — **era falso**, e a correcção é ela própria matéria de registo.
4. **A guarda sobre o `app.go`** e a justificação de ler código-fonte em vez de comportamento, com a alternativa aceitável (extrair `registarRotas` para função testável).
5. **`RegistarHealth` isento por desenho** — healthchecks e *scrape* do Prometheus são não-autenticados; um "aplicar a todos" ingénuo partiria a observabilidade.
6. **O R6** e, obrigatoriamente, a frase de âmbito: **enquanto o R7 estiver aberto, o trigger é contornável pela própria aplicação**, que é dona da tabela. É defesa contra erro e contra SQL de terceiros, não contra a aplicação comprometida.
7. **Riscos e dívida:** o R7 continua aberto, em fatia própria, com a razão (exige separar a credencial de migração da de runtime).

**A ADR NÃO pode afirmar** que o R7 está resolvido, que a imutabilidade é absoluta, nem que a anulação, o SAF-T-AO ou a certificação AGT existem.

- [ ] **Step 2: Marcar o R3 e o R6 da ADR-040 como resolvidos, aditivamente**

**Sem apagar o texto original** — é registo do que se sabia na altura. Acrescentar a cada um uma nota de resolução a apontar para a ADR-042. Para o R6, a nota deve dizer que o trigger existe **mas** que a garantia continua condicionada pelo R7.

- [ ] **Step 3: Actualizar `CLAUDE.md`**

§6 (marco actual), índice de ADRs registadas (acrescentar a ADR-042), e `Próximo ADR:` passa a **ADR-043**.

- [ ] **Step 4: Actualizar `SPRINT.md`**

Acrescentar a Sprint 17 no formato das anteriores.

- [ ] **Step 5: Correr todos os gates**

```bash
go build ./... ; echo "build=$?"
go vet ./... ; echo "vet=$?"
gofmt -l . ; echo "gofmt=$?"
go test ./... -race -count=1 ; echo "test=$?"
DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/... -count=1 ; echo "integracao=$?"
golangci-lint run ; echo "golangci=$?"
go-arch-lint check ; echo "archlint=$?"
```
Expected: **todos com código de saída 0.** Verificar o código de saída, não só o texto — na Sprint 15 um `notice` do `go-arch-lint` foi tratado como benigno enquanto o comando saía com 1, e teria partido o CI.

- [ ] **Step 6: Confirmar a cobertura**

Run: `go test ./internal/domain/... ./internal/application/... -cover -count=1`
Expected: domínio ≥85%, aplicação ≥75%.

- [ ] **Step 7: Commit**

```bash
git add adrs/ CLAUDE.md SPRINT.md
git commit -m "docs(segurança): ADR-042 e resolução do R3/R6 da ADR-040 (ADR-042)"
```

---

## Verificação final

- [ ] `graphify update .` para manter o grafo actual.
- [ ] Confirmar que só a `0004` foi acrescentada em migrações: `git diff main --name-only -- migrations/` deve mostrar apenas o ficheiro novo.
- [ ] Confirmar que `RegistarHealth` continua sem protecção: `grep -n "RegistarHealth" internal/platform/app.go internal/adapters/http/health_handler_test.go`.
