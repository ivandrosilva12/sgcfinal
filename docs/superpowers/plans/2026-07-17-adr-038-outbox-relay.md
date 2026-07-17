# Outbox (ADR-038) — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Construir o mecanismo Outbox (coleta de eventos no domínio rico → escrita transaccional → relay poller in-process → dispatcher por tipo) e ligar o primeiro consumidor real: fechar o episódio transita a chegada da Recepção `EM_CONSULTA → ATENDIDO`, assincronamente e de forma idempotente.

**Architecture:** Monólito modular, Clean Architecture. O agregado rico (`EpisodioClinico`) coleta eventos; o repositório dono da tx persiste-os em `shared.outbox` na mesma transacção do UPDATE de negócio; um relay (Camada 3) arrancado pela Plataforma faz poll com `FOR UPDATE SKIP LOCKED`, despacha a handlers Go registados por BC e marca publicado. Entrega *at-least-once* → o consumidor da Recepção é idempotente (CAS).

**Tech Stack:** Go 1.22+, pgx v5, PostgreSQL 16, Prometheus (só na Plataforma), slog. Sem ORM, sem broker.

## Global Constraints

- **Idioma**: PT-PT angolano em TODA a saída (código, comentários, commits, docs, erros). Nunca PT-BR/EN. Linguagem ubíqua: Episódio, Chegada, Evento, Outbox.
- **Regra de dependência** (`go-arch-lint`): Domínio→(nada de infra); Aplicação→Domínio; Adaptadores→Domínio+Aplicação+Adaptadores (+pgx); Plataforma→tudo. **`prometheus` NÃO está no `canUse` dos adapters** → o relay define uma interface `Observador`, a Plataforma implementa-a.
- **Migrations forward-only**, sem `.down.sql`. Numeradas por BC.
- **Nada de `panic()`** fora de inicialização — sempre `error`.
- **Domínio rico**: regras nas entidades; o domínio não conhece JSON nem SQL.
- **Cobertura**: domínio ≥85%, aplicação ≥75%, adapters ≥60%. Testes de integração com tag `//go:build integration`, pacote `integration_test`, `ligar(t)` (salta sem `DATABASE_URL`).
- **Módulo Go**: `github.com/ivandrosilva12/sgcfinal`.
- **Comandos**: unit `go test -race ./...`; integração `go test -tags integration ./tests/integration/...` (com `DATABASE_URL` apontado ao PG do compose).

---

## Mapa de ficheiros

| Ficheiro | Responsabilidade | Task |
|---|---|---|
| `internal/domain/shared/evento/registo.go` (criar) | Mixin `RegistoEventos` de coleta | 1 |
| `internal/domain/clinico/episodio.go` (modificar) | Embutir mixin; `Fechar` emite `EpisodioFechado` | 1 |
| `internal/domain/recepcao/chegada.go` (modificar) | Estado `ChegAtendido` + transição `Atender()` | 2 |
| `migrations/shared/0002_outbox_tentativas.sql` (criar) | Colunas `tentativas`/`ultimo_erro` | 3 |
| `migrations/recepcao/0005_chegadas_atendido.sql` (criar) | Estado `ATENDIDO` no CHECK | 3 |
| `migrations/embed_test.go` (modificar) | Lista esperada de migrations | 3 |
| `internal/adapters/outbox/codificador.go` (criar) | `Codificar(evento)` → (agregado, payload JSON) | 4 |
| `internal/adapters/pgrepo/outbox_repo.go` (criar) | `inserirEventos(tx, …)` helper na tx | 5 |
| `internal/adapters/pgrepo/episodios_repo.go` (modificar) | `Guardar` escreve eventos na mesma tx | 5 |
| `internal/adapters/outbox/relay.go` (criar) | `Relay`, `Registar`, `ProcessarLote`, `Correr`, `Observador` | 6 |
| `internal/adapters/pgrepo/integracao_pos_consulta.go` (criar) | `MarcarChegadaAtendida` (CAS) | 7 |
| `internal/platform/observ/*.go` (modificar) | Métricas de outbox + métodos | 8 |
| `internal/platform/config/config.go` (modificar) | `OutboxIntervalo`, `OutboxLote` | 8 |
| `internal/platform/app.go` (modificar) | Construir relay, registar handler, arrancar loop | 8 |
| `adrs/ADR-038-outbox.md` (criar) | ADR | 9 |
| `CLAUDE.md`, `SPRINT.md` (modificar) | Marco + índice de ADRs | 9 |

---

### Task 1: Coleta de eventos no domínio + emissão no fecho do episódio

**Files:**
- Create: `internal/domain/shared/evento/registo.go`
- Test: `internal/domain/shared/evento/registo_test.go`
- Modify: `internal/domain/clinico/episodio.go` (struct `EpisodioClinico`, método `Fechar`)
- Test: `internal/domain/clinico/episodio_eventos_emissao_test.go`

**Interfaces:**
- Consumes: `evento.EventoDominio` (existe), `clinico.EpisodioFechado` (existe, campos `EpisodioID, DoenteID string; Em time.Time`).
- Produces: `evento.RegistoEventos` com `RegistarEvento(EventoDominio)`, `EventosPendentes() []EventoDominio`, `LimparEventos()`. `(*EpisodioClinico).EventosPendentes() []evento.EventoDominio` (via embed).

- [ ] **Step 1: Escrever o teste do mixin (falha)**

`internal/domain/shared/evento/registo_test.go`:
```go
package evento_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

type eventoFalso struct{ nome string }

func (e eventoFalso) NomeEvento() string    { return e.nome }
func (e eventoFalso) OcorridoEm() time.Time { return time.Time{} }

func TestRegistoEventos_AcumulaEDevolveNaOrdem(t *testing.T) {
	var r evento.RegistoEventos
	if len(r.EventosPendentes()) != 0 {
		t.Fatalf("registo novo devia estar vazio")
	}
	r.RegistarEvento(eventoFalso{nome: "a"})
	r.RegistarEvento(eventoFalso{nome: "b"})
	pend := r.EventosPendentes()
	if len(pend) != 2 || pend[0].NomeEvento() != "a" || pend[1].NomeEvento() != "b" {
		t.Fatalf("esperava [a b], obtive %v", pend)
	}
}

func TestRegistoEventos_LimparEsvazia(t *testing.T) {
	var r evento.RegistoEventos
	r.RegistarEvento(eventoFalso{nome: "a"})
	r.LimparEventos()
	if len(r.EventosPendentes()) != 0 {
		t.Fatalf("após limpar devia estar vazio, obtive %d", len(r.EventosPendentes()))
	}
}
```

- [ ] **Step 2: Correr o teste — falha por `RegistoEventos` inexistente**

Run: `go test ./internal/domain/shared/evento/...`
Expected: FAIL (`undefined: evento.RegistoEventos`).

- [ ] **Step 3: Implementar o mixin**

`internal/domain/shared/evento/registo.go`:
```go
package evento

// RegistoEventos é um mixin embutível por agregados que emitem eventos de
// domínio. Acumula os eventos ocorridos durante um comportamento; o adaptador
// de persistência drena-os (EventosPendentes) e escreve-os no Outbox na mesma
// transacção da mudança de estado. Camada 1 — sem infra.
type RegistoEventos struct {
	pendentes []EventoDominio
}

// RegistarEvento acrescenta um evento à lista pendente.
func (r *RegistoEventos) RegistarEvento(e EventoDominio) {
	r.pendentes = append(r.pendentes, e)
}

// EventosPendentes devolve os eventos ainda não persistidos, pela ordem de
// ocorrência.
func (r *RegistoEventos) EventosPendentes() []EventoDominio {
	return r.pendentes
}

// LimparEventos esvazia a lista pendente (após persistência).
func (r *RegistoEventos) LimparEventos() {
	r.pendentes = nil
}
```

- [ ] **Step 4: Correr o teste — passa**

Run: `go test ./internal/domain/shared/evento/...`
Expected: PASS.

- [ ] **Step 5: Escrever o teste de emissão no fecho (falha)**

`internal/domain/clinico/episodio_eventos_emissao_test.go` — usa os construtores existentes. Um episódio ABERTO só fecha com nota completa + ≥1 CID; segue o padrão de `gerir_episodio_test.go`/`iniciar_episodio_test.go` do domínio para montar um episódio válido:
```go
package clinico_test

import (
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func episodioAbertoCompleto(t *testing.T) *dominio.EpisodioClinico {
	t.Helper()
	e, err := dominio.NovoEpisodio("11111111-1111-4111-8111-111111111111",
		dominio.EpisodioConsulta, "22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333", time.Now())
	if err != nil {
		t.Fatalf("novo episódio: %v", err)
	}
	nota := dominio.NovaNotaClinica("queixa", "história", "exame", "diagnóstico", "plano")
	if err := e.ActualizarNota(nota); err != nil {
		t.Fatalf("actualizar nota: %v", err)
	}
	cid, err := dominio.NovoDiagnosticoCID("J06", true) // (cid string, principal bool)
	if err != nil {
		t.Fatalf("cid: %v", err)
	}
	if err := e.DefinirDiagnosticosCID([]dominio.DiagnosticoCID{cid}); err != nil {
		t.Fatalf("definir cid: %v", err)
	}
	return e
}

func TestFechar_EmiteEpisodioFechado(t *testing.T) {
	e := episodioAbertoCompleto(t)
	if len(e.EventosPendentes()) != 0 {
		t.Fatalf("episódio aberto não devia ter eventos pendentes")
	}
	if err := e.Fechar("33333333-3333-4333-8333-333333333333", time.Now()); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	pend := e.EventosPendentes()
	if len(pend) != 1 {
		t.Fatalf("esperava 1 evento, obtive %d", len(pend))
	}
	ev, ok := pend[0].(dominio.EpisodioFechado)
	if !ok {
		t.Fatalf("esperava EpisodioFechado, obtive %T", pend[0])
	}
	if ev.DoenteID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("doente do evento errado: %q", ev.DoenteID)
	}
	if ev.NomeEvento() != "clinico.episodio.fechado" {
		t.Fatalf("nome do evento errado: %q", ev.NomeEvento())
	}
}

func TestFechar_Invalido_NaoEmite(t *testing.T) {
	e := episodioAbertoCompleto(t)
	// fecha uma vez (válido)
	_ = e.Fechar("33333333-3333-4333-8333-333333333333", time.Now())
	// segundo fecho é inválido (já fechado) e não deve acrescentar evento
	if err := e.Fechar("x", time.Now()); err == nil {
		t.Fatalf("segundo fecho devia falhar")
	}
	if len(e.EventosPendentes()) != 1 {
		t.Fatalf("um fecho inválido não devia emitir; esperava 1, obtive %d", len(e.EventosPendentes()))
	}
}
```
> Assinaturas confirmadas no código: `NovaNotaClinica(queixa, historia, exame, diagnostico, plano string) NotaClinica` (sem erro); `NovoDiagnosticoCID(cid string, principal bool) (DiagnosticoCID, error)`; `NovoEpisodio(doenteID string, tipo TipoEpisodio, especialidadeID, medicoID string, inicio time.Time)`.

- [ ] **Step 6: Correr — falha por `EventosPendentes` inexistente no episódio**

Run: `go test ./internal/domain/clinico/...`
Expected: FAIL (`e.EventosPendentes undefined`).

- [ ] **Step 7: Embutir o mixin e emitir no `Fechar`**

Em `internal/domain/clinico/episodio.go`, acrescentar o campo embutido no struct (após os campos existentes de `EpisodioClinico`):
```go
type EpisodioClinico struct {
	id              string
	// ... campos existentes inalterados ...
	evento.RegistoEventos
}
```
Adicionar o import `"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"` se ainda não existir. No fim do caminho de sucesso de `Fechar` (imediatamente antes de `return nil`):
```go
	e.estado = EstadoEpisodioFechado
	e.fim = &em
	e.fechadoEm = &em
	e.fechadoPor = fechadoPor
	e.RegistarEvento(EpisodioFechado{EpisodioID: e.id, DoenteID: e.doenteID, Em: em})
	return nil
```
> `ReconstruirEpisodio`/`Snapshot` **não** mudam: um agregado relido nasce com o registo vazio (o campo embutido é zero-value), logo não re-emite. Confirmar que `Snapshot()` não copia o mixin (não copia — só os campos listados).

- [ ] **Step 8: Correr todos os testes do domínio — passam**

Run: `go test -race ./internal/domain/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/domain/shared/evento/registo.go internal/domain/shared/evento/registo_test.go internal/domain/clinico/episodio.go internal/domain/clinico/episodio_eventos_emissao_test.go
git commit -m "feat(dominio): coleta de eventos no agregado e emissao de EpisodioFechado (ADR-038)"
```

---

### Task 2: Estado ATENDIDO e transição Atender() na Chegada

**Files:**
- Modify: `internal/domain/recepcao/chegada.go` (constante de estado + método)
- Test: `internal/domain/recepcao/chegada_test.go` (acrescentar casos)

**Interfaces:**
- Consumes: `recepcao.ChegEmConsulta` (existe).
- Produces: `recepcao.ChegAtendido EstadoChegada = "ATENDIDO"`; `(*Chegada).Atender(em time.Time) error` (EM_CONSULTA → ATENDIDO).

- [ ] **Step 1: Escrever os testes da transição (falha)**

Acrescentar a `internal/domain/recepcao/chegada_test.go`. Para chegar a EM_CONSULTA há que percorrer a máquina; reutiliza o padrão dos testes existentes no ficheiro (walk-in → Chamar → RegistarTriada → IniciarConsulta). Se já existir um helper para um agregado EM_CONSULTA, usa-o; senão:
```go
func TestAtender_DeEmConsulta_TransitaAtendido(t *testing.T) {
	c := chegadaEmConsulta(t) // helper local: walk-in até EM_CONSULTA
	if err := c.Atender(time.Now()); err != nil {
		t.Fatalf("atender: %v", err)
	}
	if c.Estado() != domrecepcao.ChegAtendido {
		t.Fatalf("esperava ATENDIDO, obtive %q", c.Estado())
	}
}

func TestAtender_DeOutroEstado_Recusa(t *testing.T) {
	c, _ := domrecepcao.NovaChegadaWalkIn(
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222", time.Now())
	// estado AGUARDA — não é EM_CONSULTA
	if err := c.Atender(time.Now()); err == nil {
		t.Fatalf("atender fora de EM_CONSULTA devia recusar")
	}
}
```
> Nota: `chegadaEmConsulta` monta o agregado até EM_CONSULTA usando os construtores/transições já testadas no ficheiro (`NovaChegadaWalkIn`, `Chamar`, atribuição de médico, `RegistarTriada`, `IniciarConsulta`). Confirma as assinaturas reais no ficheiro antes de escrever o helper. O nome do pacote de teste (`domrecepcao` alias ou `package recepcao`) deve seguir o que o ficheiro já usa.

- [ ] **Step 2: Correr — falha por `Atender`/`ChegAtendido` inexistentes**

Run: `go test ./internal/domain/recepcao/...`
Expected: FAIL (`undefined: ChegAtendido` / `c.Atender undefined`).

- [ ] **Step 3: Implementar o estado e a transição**

Em `internal/domain/recepcao/chegada.go`, acrescentar à lista de constantes de estado:
```go
	ChegEmConsulta EstadoChegada = "EM_CONSULTA"
	ChegAtendido   EstadoChegada = "ATENDIDO"
```
Actualizar o comentário-diagrama do topo do ficheiro para incluir `EM_CONSULTA ─ Atender ► ATENDIDO`. Acrescentar o método (junto a `IniciarConsulta`):
```go
// Atender transita EM_CONSULTA → ATENDIDO (o episódio fechou; a consulta
// concluiu-se). É o desfecho pós-consulta da chegada, accionado de forma
// assíncrona pelo consumidor do evento clinico.episodio.fechado (ADR-038).
func (c *Chegada) Atender(em time.Time) error {
	if c.estado != ChegEmConsulta {
		return erros.Novo(erros.CategoriaConflito, "só é possível atender uma chegada em consulta")
	}
	c.estado = ChegAtendido
	c.actualizadoEm = em
	return nil
}
```

- [ ] **Step 4: Correr — passa**

Run: `go test -race ./internal/domain/recepcao/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/recepcao/chegada.go internal/domain/recepcao/chegada_test.go
git commit -m "feat(recepcao): estado ATENDIDO e transicao Atender na chegada (ADR-038)"
```

---

### Task 3: Migrações (outbox + estado ATENDIDO)

**Files:**
- Create: `migrations/shared/0002_outbox_tentativas.sql`
- Create: `migrations/recepcao/0005_chegadas_atendido.sql`
- Modify: `migrations/embed_test.go` (lista esperada)
- Test: `tests/integration/migracoes_test.go` (asserção nova — opcional; ver Step 4)

**Interfaces:**
- Produces: colunas `shared.outbox.tentativas int`, `shared.outbox.ultimo_erro text`; CHECK `chegadas_estado_check` a admitir `ATENDIDO`.

- [ ] **Step 1: Escrever a migração do outbox**

`migrations/shared/0002_outbox_tentativas.sql`:
```sql
-- Bounded Context: shared (Shared Kernel)
-- Migration forward-only. Acrescenta contabilidade de reentrega ao Outbox: número
-- de tentativas de entrega e o último erro do handler (diagnóstico de mensagens
-- persistentemente falhadas; dead-lettering fica para marco futuro). ADR-038.

ALTER TABLE shared.outbox ADD COLUMN IF NOT EXISTS tentativas  int  NOT NULL DEFAULT 0;
ALTER TABLE shared.outbox ADD COLUMN IF NOT EXISTS ultimo_erro text;

COMMENT ON COLUMN shared.outbox.tentativas  IS 'Número de tentativas de entrega falhadas (relay).';
COMMENT ON COLUMN shared.outbox.ultimo_erro IS 'Mensagem do último erro de handler, para diagnóstico.';
```

- [ ] **Step 2: Escrever a migração do estado ATENDIDO**

`migrations/recepcao/0005_chegadas_atendido.sql`:
```sql
-- Bounded Context: recepcao
-- Migration forward-only. Estende o enum de estado da chegada com ATENDIDO
-- (desfecho pós-consulta: o episódio fechou — ADR-038). O nome
-- chegadas_estado_check é o auto-gerado determinístico da CHECK inline,
-- redefinido pela última vez em 0004.

ALTER TABLE recepcao.chegadas DROP CONSTRAINT chegadas_estado_check;
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_estado_check
    CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU','TRIADO','EM_CONSULTA','ATENDIDO'));
```

- [ ] **Step 3: Actualizar a lista esperada de migrations embebidas**

Em `migrations/embed_test.go`, acrescentar à lista de caminhos esperados (`TestFS_ContemMigrationsEsperadas`) as duas entradas novas, mantendo o formato existente:
```go
	"shared/0002_outbox_tentativas.sql",
	"recepcao/0005_chegadas_atendido.sql",
```
> Confirmar o nome exacto da slice/variável no ficheiro e inserir na posição coerente (agrupado por BC).

- [ ] **Step 4: Escrever a asserção de integração das colunas**

Acrescentar a `tests/integration/migracoes_test.go` (tag `integration`, usa `ligar` e aplica migrations — segue o padrão do ficheiro):
```go
func TestOutbox_TemColunasDeReentrega(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("aplicar migrations: %v", err)
	}
	var n int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='shared' AND table_name='outbox'
		AND column_name IN ('tentativas','ultimo_erro')`).Scan(&n)
	if err != nil {
		t.Fatalf("consultar colunas: %v", err)
	}
	if n != 2 {
		t.Fatalf("esperava 2 colunas novas no outbox, obtive %d", n)
	}
}
```
> Confirmar os imports já presentes no ficheiro (`io`, `log/slog`, `db`, `migrations`); reutilizar os que já lá estão.

- [ ] **Step 5: Correr unit + embed**

Run: `go test ./migrations/...`
Expected: PASS (a FS embebida contém os novos ficheiros).

- [ ] **Step 6: Correr integração (com PG a correr)**

Run: `go test -tags integration -run 'TestOutbox_TemColunas|TestAplica' ./tests/integration/...`
Expected: PASS (ou SKIP se `DATABASE_URL` não definido).

- [ ] **Step 7: Commit**

```bash
git add migrations/shared/0002_outbox_tentativas.sql migrations/recepcao/0005_chegadas_atendido.sql migrations/embed_test.go tests/integration/migracoes_test.go
git commit -m "feat(migracoes): reentrega no outbox e estado ATENDIDO da chegada (ADR-038)"
```

---

### Task 4: Codificador de eventos (evento → linha de outbox)

**Files:**
- Create: `internal/adapters/outbox/codificador.go`
- Test: `internal/adapters/outbox/codificador_test.go`
- Modify: `internal/adapters/outbox/doc.go` (actualizar o texto de placeholder)

**Interfaces:**
- Consumes: `evento.EventoDominio`, `clinico.EpisodioFechado`.
- Produces: `func Codificar(e evento.EventoDominio) (agregado string, payload []byte, err error)`. Para `EpisodioFechado` → agregado `"episodio"`.

- [ ] **Step 1: Escrever o teste (falha)**

`internal/adapters/outbox/codificador_test.go`:
```go
package outbox_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
	domclinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func TestCodificar_EpisodioFechado(t *testing.T) {
	ev := domclinico.EpisodioFechado{
		EpisodioID: "ep-1", DoenteID: "do-1",
		Em: time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC),
	}
	agregado, payload, err := outbox.Codificar(ev)
	if err != nil {
		t.Fatalf("codificar: %v", err)
	}
	if agregado != "episodio" {
		t.Fatalf("agregado errado: %q", agregado)
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("payload não é JSON: %v", err)
	}
	if m["EpisodioID"] != "ep-1" || m["DoenteID"] != "do-1" {
		t.Fatalf("payload sem os campos esperados: %s", payload)
	}
}

type eventoDesconhecido struct{}

func (eventoDesconhecido) NomeEvento() string    { return "x.y.z" }
func (eventoDesconhecido) OcorridoEm() time.Time { return time.Time{} }

func TestCodificar_TipoNaoMapeado_Erro(t *testing.T) {
	if _, _, err := outbox.Codificar(eventoDesconhecido{}); err == nil {
		t.Fatalf("evento não mapeado devia devolver erro")
	}
}
```

- [ ] **Step 2: Correr — falha por `Codificar` inexistente**

Run: `go test ./internal/adapters/outbox/...`
Expected: FAIL (`undefined: outbox.Codificar`).

- [ ] **Step 3: Implementar o codificador**

`internal/adapters/outbox/codificador.go`:
```go
// Package outbox implementa o padrão Outbox: a codificação de eventos de domínio
// para persistência e o relay de publicação assíncrona inter-bounded-context.
// Camada 3 — Adaptadores.
package outbox

import (
	"encoding/json"
	"fmt"

	domclinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

// Codificar traduz um evento de domínio numa linha de Outbox: o nome do agregado
// de origem e o payload JSON. O tipo do evento persistido é e.NomeEvento(). Só os
// tipos explicitamente mapeados são aceites — um evento novo tem de ser registado
// aqui (falha explícita em vez de publicação de um payload não contratado).
func Codificar(e evento.EventoDominio) (agregado string, payload []byte, err error) {
	switch e.(type) {
	case domclinico.EpisodioFechado:
		agregado = "episodio"
	default:
		return "", nil, fmt.Errorf("outbox: evento não mapeado para publicação: %s (%T)", e.NomeEvento(), e)
	}
	payload, err = json.Marshal(e)
	if err != nil {
		return "", nil, fmt.Errorf("outbox: serializar evento %s: %w", e.NomeEvento(), err)
	}
	return agregado, payload, nil
}
```
Actualizar `internal/adapters/outbox/doc.go` removendo o texto "Placeholder de M1" (a doc do pacote passa a ser a de `codificador.go`; apagar `doc.go` ou reduzi-lo a um comentário não-duplicado). **Decisão**: apagar `doc.go` (o package comment vive agora em `codificador.go`).

- [ ] **Step 4: Correr — passa**

Run: `go test ./internal/adapters/outbox/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git rm internal/adapters/outbox/doc.go
git add internal/adapters/outbox/codificador.go internal/adapters/outbox/codificador_test.go
git commit -m "feat(outbox): codificador de eventos de dominio para linha de outbox (ADR-038)"
```

---

### Task 5: Escrita transaccional dos eventos no repositório

**Files:**
- Create: `internal/adapters/pgrepo/outbox_repo.go`
- Modify: `internal/adapters/pgrepo/episodios_repo.go` (`Guardar`)
- Test: `tests/integration/outbox_escrita_test.go`

**Interfaces:**
- Consumes: `outbox.Codificar`, `evento.EventoDominio`, `pgx.Tx`.
- Produces: `func inserirEventos(ctx context.Context, tx pgx.Tx, eventos []evento.EventoDominio) error` (pacote `pgrepo`). `Guardar` passa a persistir os eventos pendentes do episódio na mesma tx.

- [ ] **Step 1: Escrever o teste de integração (falha)**

`tests/integration/outbox_escrita_test.go` (tag `integration`, pacote `integration_test`):
```go
//go:build integration

package integration_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
)

// Fecha um episódio via repositório e verifica que a linha de outbox foi escrita
// na MESMA transacção (existe após o commit do Guardar).
func TestGuardar_FechoEscreveOutboxNaMesmaTx(t *testing.T) {
	pool, ctx := ligar(t)
	repo := pgrepo.NovoRepositorioEpisodios(pool)

	// monta e persiste um episódio ABERTO, depois fecha-o.
	ep := episodioAbertoParaTeste(t, pool, ctx) // helper: cria doente + episódio ABERTO, devolve o agregado relido
	if err := ep.Fechar("33333333-3333-4333-8333-333333333333", agora()); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	id, err := repo.Guardar(ctx, ep)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}

	var n int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM shared.outbox
		WHERE tipo_evento='clinico.episodio.fechado'
		AND payload->>'EpisodioID' = $1 AND publicado_em IS NULL`, id).Scan(&n)
	if err != nil {
		t.Fatalf("consultar outbox: %v", err)
	}
	if n != 1 {
		t.Fatalf("esperava 1 linha de outbox pendente, obtive %d", n)
	}
}
```
> Nota: `episodioAbertoParaTeste` e `agora()` — reutiliza os helpers já existentes em `tests/integration/episodios_test.go` para criar um doente activo e um episódio ABERTO com nota+CID; se não houver um helper directo, compõe a partir dos construtores como em `episodios_test.go`. O objectivo é ter um agregado que `Fechar()` aceite.

- [ ] **Step 2: Correr — falha (nenhuma linha escrita)**

Run: `go test -tags integration -run TestGuardar_FechoEscreveOutbox ./tests/integration/...`
Expected: FAIL (`esperava 1 linha … obtive 0`) — ou SKIP sem `DATABASE_URL`.

- [ ] **Step 3: Implementar o helper de inserção**

`internal/adapters/pgrepo/outbox_repo.go`:
```go
package pgrepo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

// inserirEventos persiste os eventos de domínio pendentes de um agregado na
// tabela shared.outbox, dentro da transacção fornecida (mesma tx da mudança de
// estado — garantia atómica do padrão Outbox). Sem eventos, é um no-op.
func inserirEventos(ctx context.Context, tx pgx.Tx, eventos []evento.EventoDominio) error {
	for _, e := range eventos {
		agregado, payload, err := outbox.Codificar(e)
		if err != nil {
			return err
		}
		const q = `INSERT INTO shared.outbox (agregado, tipo_evento, payload)
			VALUES ($1, $2, $3)`
		if _, err := tx.Exec(ctx, q, agregado, e.NomeEvento(), payload); err != nil {
			return fmt.Errorf("inserir evento %s no outbox: %w", e.NomeEvento(), err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Ligar no `Guardar` do episódio**

Em `internal/adapters/pgrepo/episodios_repo.go`, dentro de `Guardar`, após `guardarDiagnosticos` e **antes** de `tx.Commit`:
```go
	if err := r.guardarDiagnosticos(ctx, tx, id, s); err != nil {
		return "", err
	}
	if err := inserirEventos(ctx, tx, e.EventosPendentes()); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar transacção: %w", err)
	}
```
> `e` é o `*dominio.EpisodioClinico` recebido por `Guardar`. Nenhuma outra alteração de assinatura.

- [ ] **Step 5: Correr integração — passa**

Run: `go test -tags integration -run TestGuardar_FechoEscreveOutbox ./tests/integration/...`
Expected: PASS.

- [ ] **Step 6: Correr unit + lint arquitectural**

Run: `go test -race ./... && go-arch-lint check`
Expected: PASS (pgrepo→outbox e pgrepo→domínio são permitidos; sem violações).

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/pgrepo/outbox_repo.go internal/adapters/pgrepo/episodios_repo.go tests/integration/outbox_escrita_test.go
git commit -m "feat(pgrepo): escrita transaccional dos eventos no outbox no fecho do episodio (ADR-038)"
```

---

### Task 6: Relay (poll + dispatch)

**Files:**
- Create: `internal/adapters/outbox/relay.go`
- Test: `internal/adapters/outbox/relay_test.go` (unidade, com fakes)
- Test: `tests/integration/outbox_relay_test.go` (SKIP LOCKED + marcação real)

**Interfaces:**
- Consumes: `*pgxpool.Pool`, `pgx`.
- Produces:
  - `type Handler func(ctx context.Context, ev EventoEntregue) error`
  - `type EventoEntregue struct { ID int64; Agregado, TipoEvento string; Payload []byte }`
  - `type Observador interface { Pendentes(n int); Publicado(); FalhaHandler(tipoEvento string) }`
  - `type ObservadorNulo struct{}` (no-op, satisfaz `Observador`)
  - `func NovoRelay(pool *pgxpool.Pool, lote int, obs Observador, log *slog.Logger) *Relay`
  - `func (r *Relay) Registar(tipoEvento string, h Handler)`
  - `func (r *Relay) ProcessarLote(ctx context.Context) (processados int, err error)`
  - `func (r *Relay) Correr(ctx context.Context, intervalo time.Duration)`

- [ ] **Step 1: Escrever os testes de unidade do dispatch (falha)**

O `ProcessarLote` toca a BD; a lógica de *dispatch* é testável isoladamente extraindo `despachar(ctx, ev)`. Testa o registo/despacho e o `ObservadorNulo`:
`internal/adapters/outbox/relay_test.go`:
```go
package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
)

func relayVazio() *outbox.Relay {
	return outbox.NovoRelay(nil, 100, outbox.ObservadorNulo{},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestDespachar_ChamaHandlersDoTipo(t *testing.T) {
	r := relayVazio()
	var visto string
	r.Registar("clinico.episodio.fechado", func(_ context.Context, ev outbox.EventoEntregue) error {
		visto = string(ev.Payload)
		return nil
	})
	err := r.Despachar(context.Background(), outbox.EventoEntregue{
		TipoEvento: "clinico.episodio.fechado", Payload: []byte(`{"x":1}`)})
	if err != nil {
		t.Fatalf("despachar: %v", err)
	}
	if visto != `{"x":1}` {
		t.Fatalf("handler não recebeu o payload, obtive %q", visto)
	}
}

func TestDespachar_SemHandler_NaoErra(t *testing.T) {
	r := relayVazio()
	if err := r.Despachar(context.Background(), outbox.EventoEntregue{TipoEvento: "n.a"}); err != nil {
		t.Fatalf("sem handler devia ser no-op sem erro, obtive %v", err)
	}
}

func TestDespachar_HandlerFalha_Propaga(t *testing.T) {
	r := relayVazio()
	r.Registar("t", func(context.Context, outbox.EventoEntregue) error { return errors.New("falhou") })
	if err := r.Despachar(context.Background(), outbox.EventoEntregue{TipoEvento: "t"}); err == nil {
		t.Fatalf("erro do handler devia propagar")
	}
}
```

- [ ] **Step 2: Correr — falha (`Relay`/`Despachar` inexistentes)**

Run: `go test ./internal/adapters/outbox/...`
Expected: FAIL (`undefined: outbox.Relay`).

- [ ] **Step 3: Implementar o relay**

`internal/adapters/outbox/relay.go`:
```go
package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventoEntregue é a forma persistida de um evento que o relay entrega aos
// handlers. O consumidor desserializa Payload no que precisar — fica desacoplado
// do struct do produtor.
type EventoEntregue struct {
	ID         int64
	Agregado   string
	TipoEvento string
	Payload    []byte
}

// Handler processa um evento entregue. Deve ser idempotente: a entrega é
// at-least-once (uma linha pode ser reprocessada após uma falha tardia).
type Handler func(ctx context.Context, ev EventoEntregue) error

// Observador recebe sinais de instrumentação do relay (métricas). Definido aqui
// (Camada 3) porque a regra de dependência proíbe o adaptador de importar o
// Prometheus da Plataforma; esta é implementada na Camada 4.
type Observador interface {
	Pendentes(n int)
	Publicado()
	FalhaHandler(tipoEvento string)
}

// ObservadorNulo é a implementação no-op (testes e omissão).
type ObservadorNulo struct{}

func (ObservadorNulo) Pendentes(int)        {}
func (ObservadorNulo) Publicado()           {}
func (ObservadorNulo) FalhaHandler(string)  {}

// Relay faz poll da tabela shared.outbox e despacha os eventos pendentes aos
// handlers registados por tipo. Camada 3 — Adaptadores.
type Relay struct {
	pool    *pgxpool.Pool
	lote    int
	obs     Observador
	log     *slog.Logger
	handlers map[string][]Handler
}

// NovoRelay constrói o relay. lote é o máximo de eventos por passagem.
func NovoRelay(pool *pgxpool.Pool, lote int, obs Observador, log *slog.Logger) *Relay {
	if lote <= 0 {
		lote = 100
	}
	return &Relay{pool: pool, lote: lote, obs: obs, log: log, handlers: map[string][]Handler{}}
}

// Registar liga um handler a um tipo de evento (vários handlers por tipo).
func (r *Relay) Registar(tipoEvento string, h Handler) {
	r.handlers[tipoEvento] = append(r.handlers[tipoEvento], h)
}

// Despachar chama todos os handlers do tipo do evento. Sem handler é no-op. O
// primeiro erro interrompe e propaga (a linha não é marcada publicada).
func (r *Relay) Despachar(ctx context.Context, ev EventoEntregue) error {
	for _, h := range r.handlers[ev.TipoEvento] {
		if err := h(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

// ProcessarLote faz uma passagem: selecciona até `lote` eventos pendentes com
// FOR UPDATE SKIP LOCKED (uma linha-veneno não bloqueia as sãs), despacha cada um
// e marca publicado em sucesso; em falha incrementa tentativas e grava ultimo_erro.
// Devolve o número de linhas processadas com sucesso.
func (r *Relay) ProcessarLote(ctx context.Context) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	const sel = `SELECT id, agregado, tipo_evento, payload
		FROM shared.outbox WHERE publicado_em IS NULL
		ORDER BY id FOR UPDATE SKIP LOCKED LIMIT $1`
	rows, err := tx.Query(ctx, sel, r.lote)
	if err != nil {
		return 0, err
	}
	var lote []EventoEntregue
	for rows.Next() {
		var ev EventoEntregue
		if err := rows.Scan(&ev.ID, &ev.Agregado, &ev.TipoEvento, &ev.Payload); err != nil {
			rows.Close()
			return 0, err
		}
		lote = append(lote, ev)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	publicados := 0
	for _, ev := range lote {
		if err := r.Despachar(ctx, ev); err != nil {
			r.obs.FalhaHandler(ev.TipoEvento)
			r.log.Warn("falha ao entregar evento do outbox", "id", ev.ID,
				"tipo_evento", ev.TipoEvento, "erro", err)
			if _, e := tx.Exec(ctx, `UPDATE shared.outbox
				SET tentativas = tentativas + 1, ultimo_erro = $2 WHERE id = $1`,
				ev.ID, err.Error()); e != nil {
				return publicados, e
			}
			continue
		}
		if _, e := tx.Exec(ctx, `UPDATE shared.outbox SET publicado_em = now() WHERE id = $1`, ev.ID); e != nil {
			return publicados, e
		}
		r.obs.Publicado()
		publicados++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return publicados, nil
}

// Correr executa ProcessarLote em ciclo, no intervalo dado, até ctx ser
// cancelado (drena a passagem em curso antes de sair — shutdown gracioso).
func (r *Relay) Correr(ctx context.Context, intervalo time.Duration) {
	t := time.NewTicker(intervalo)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := r.ProcessarLote(ctx); err != nil && ctx.Err() == nil {
				r.log.Error("erro no ciclo do relay de outbox", "erro", err)
			}
		}
	}
}

var _ = pgx.ErrNoRows // pgx importado para tipos de scan; manter o import estável
```
> Remover a linha `var _ = pgx.ErrNoRows` se o `pgx` já for referenciado por outro motivo (não é, neste ficheiro os tipos vêm de `pgxpool`); em alternativa, apagar o import `pgx` e a linha. **Decisão**: apagar o import `pgx` e a linha `var _` — só `pgxpool` é usado.

- [ ] **Step 4: Correr unit — passa**

Run: `go test -race ./internal/adapters/outbox/...`
Expected: PASS.

- [ ] **Step 5: Escrever o teste de integração do relay (falha antes de existir a marcação real)**

`tests/integration/outbox_relay_test.go` (tag `integration`): insere duas linhas na `shared.outbox`, regista um handler contador, corre `ProcessarLote` e verifica que ambas ficam `publicado_em NOT NULL` e o handler viu 2; e que uma segunda passagem processa 0 (idempotência da marcação):
```go
//go:build integration

package integration_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
)

func TestRelay_ProcessarLote_MarcaPublicado(t *testing.T) {
	pool, ctx := ligar(t)
	// limpa e semeia duas linhas pendentes
	if _, err := pool.Exec(ctx, `DELETE FROM shared.outbox`); err != nil {
		t.Fatalf("limpar outbox: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO shared.outbox (agregado, tipo_evento, payload)
			VALUES ('teste','t.evento','{}'::jsonb)`); err != nil {
			t.Fatalf("semear: %v", err)
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := outbox.NovoRelay(pool, 100, outbox.ObservadorNulo{}, log)
	var vistos int
	r.Registar("t.evento", func(context.Context, outbox.EventoEntregue) error { vistos++; return nil })

	n, err := r.ProcessarLote(ctx)
	if err != nil {
		t.Fatalf("processar: %v", err)
	}
	if n != 2 || vistos != 2 {
		t.Fatalf("esperava 2 processados e 2 vistos, obtive n=%d vistos=%d", n, vistos)
	}
	// segunda passagem: nada pendente
	n2, _ := r.ProcessarLote(ctx)
	if n2 != 0 {
		t.Fatalf("segunda passagem devia processar 0, obtive %d", n2)
	}
}
```

- [ ] **Step 6: Correr integração — passa**

Run: `go test -tags integration -run TestRelay_ProcessarLote ./tests/integration/...`
Expected: PASS (ou SKIP sem `DATABASE_URL`).

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/outbox/relay.go internal/adapters/outbox/relay_test.go tests/integration/outbox_relay_test.go
git commit -m "feat(outbox): relay poller com dispatch por tipo e SKIP LOCKED (ADR-038)"
```

---

### Task 7: Consumidor da Recepção (chegada → ATENDIDO)

**Files:**
- Create: `internal/adapters/pgrepo/integracao_pos_consulta.go`
- Test: `tests/integration/pos_consulta_test.go`

**Interfaces:**
- Consumes: `*pgxpool.Pool`, `outbox.EventoEntregue`.
- Produces:
  - `type IntegracaoPosConsulta struct { … }` + `func NovaIntegracaoPosConsulta(pool *pgxpool.Pool) *IntegracaoPosConsulta`
  - `func (a *IntegracaoPosConsulta) MarcarChegadaAtendida(ctx context.Context, episodioID string) error` (CAS EM_CONSULTA→ATENDIDO; 0 linhas = no-op)
  - `func (a *IntegracaoPosConsulta) HandlerEpisodioFechado(ctx context.Context, ev outbox.EventoEntregue) error` (desserializa `EpisodioID` e chama `MarcarChegadaAtendida`)

- [ ] **Step 1: Escrever o teste de integração (falha)**

`tests/integration/pos_consulta_test.go` (tag `integration`). Reutiliza `criaChegadaTriadaComDoente` (de `inicio_consulta_test.go`) e o adaptador de início da consulta para pôr uma chegada em EM_CONSULTA com um episódio real:
```go
//go:build integration

package integration_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
)

func TestMarcarChegadaAtendida_TransitaEIdempotente(t *testing.T) {
	pool, ctx := ligar(t)
	// põe uma chegada EM_CONSULTA com episódio (via o adaptador ADR-036)
	episodioID, chegadaID := chegadaEmConsultaComEpisodio(t, pool, ctx) // helper (ver nota)

	pos := pgrepo.NovaIntegracaoPosConsulta(pool)
	if err := pos.MarcarChegadaAtendida(ctx, episodioID); err != nil {
		t.Fatalf("marcar atendida: %v", err)
	}
	var estado string
	if err := pool.QueryRow(ctx, `SELECT estado FROM recepcao.chegadas WHERE id=$1`, chegadaID).Scan(&estado); err != nil {
		t.Fatalf("ler chegada: %v", err)
	}
	if estado != "ATENDIDO" {
		t.Fatalf("esperava ATENDIDO, obtive %q", estado)
	}
	// idempotência: segunda chamada é no-op sem erro
	if err := pos.MarcarChegadaAtendida(ctx, episodioID); err != nil {
		t.Fatalf("segunda marcação devia ser no-op, obtive %v", err)
	}
}

func TestMarcarChegadaAtendida_SemChegada_NoOp(t *testing.T) {
	pool, ctx := ligar(t)
	pos := pgrepo.NovaIntegracaoPosConsulta(pool)
	// episódio que não nasceu da fila → nenhuma chegada com este episodio_id
	if err := pos.MarcarChegadaAtendida(ctx, "00000000-0000-4000-8000-0000000000ff"); err != nil {
		t.Fatalf("episódio sem chegada devia ser no-op, obtive %v", err)
	}
}
```
> Nota: `chegadaEmConsultaComEpisodio` compõe o cenário reutilizando `criaChegadaTriadaComDoente` + `pgrepo.NovaIntegracaoInicioConsulta(pool).ConsumirEIniciar(...)` (o caminho real que grava `episodio_id` na chegada e a põe EM_CONSULTA). Devolve o `episodioID` retornado por `ConsumirEIniciar` e o `chegadaID`. Segue exactamente o padrão de `inicio_consulta_test.go`.

- [ ] **Step 2: Correr — falha (`NovaIntegracaoPosConsulta` inexistente)**

Run: `go test -tags integration -run TestMarcarChegadaAtendida ./tests/integration/...`
Expected: FAIL/compile error — ou SKIP sem `DATABASE_URL`.

- [ ] **Step 3: Implementar o consumidor**

`internal/adapters/pgrepo/integracao_pos_consulta.go`:
```go
// internal/adapters/pgrepo/integracao_pos_consulta.go
//
// Consumidor do evento clinico.episodio.fechado (ADR-038): transita a chegada da
// Recepção que originou o episódio para ATENDIDO. Adaptador de integração — o
// único ponto que conhece a ponte episodio_id entre o Clínico e a Recepção. A
// entrega é at-least-once, por isso a operação é idempotente (CAS por estado).
package pgrepo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
)

// IntegracaoPosConsulta implementa o desfecho pós-consulta da chegada.
type IntegracaoPosConsulta struct {
	pool *pgxpool.Pool
}

// NovaIntegracaoPosConsulta constrói o adaptador sobre o pool pgx.
func NovaIntegracaoPosConsulta(pool *pgxpool.Pool) *IntegracaoPosConsulta {
	return &IntegracaoPosConsulta{pool: pool}
}

// MarcarChegadaAtendida transita a chegada EM_CONSULTA→ATENDIDO pela ponte
// episodio_id. Idempotente: 0 linhas afectadas (já ATENDIDO por reentrega, ou
// episódio sem chegada associada) é sucesso/no-op. A guarda de estado no WHERE é
// o CAS que fecha corridas — espelha a máquina de estados do domínio (Atender).
func (a *IntegracaoPosConsulta) MarcarChegadaAtendida(ctx context.Context, episodioID string) error {
	const q = `UPDATE recepcao.chegadas
		SET estado = 'ATENDIDO', actualizado_em = now()
		WHERE episodio_id = $1 AND estado = 'EM_CONSULTA'`
	if _, err := a.pool.Exec(ctx, q, episodioID); err != nil {
		return fmt.Errorf("marcar chegada atendida (episódio %s): %w", episodioID, err)
	}
	return nil
}

// HandlerEpisodioFechado é o Handler de relay para clinico.episodio.fechado.
func (a *IntegracaoPosConsulta) HandlerEpisodioFechado(ctx context.Context, ev outbox.EventoEntregue) error {
	var p struct {
		EpisodioID string `json:"EpisodioID"`
	}
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return fmt.Errorf("payload de episodio.fechado inválido: %w", err)
	}
	if p.EpisodioID == "" {
		return fmt.Errorf("payload de episodio.fechado sem EpisodioID")
	}
	return a.MarcarChegadaAtendida(ctx, p.EpisodioID)
}
```

- [ ] **Step 4: Correr integração — passa**

Run: `go test -tags integration -run TestMarcarChegadaAtendida ./tests/integration/...`
Expected: PASS.

- [ ] **Step 5: Correr lint arquitectural**

Run: `go-arch-lint check`
Expected: PASS (pgrepo→outbox permitido).

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/pgrepo/integracao_pos_consulta.go tests/integration/pos_consulta_test.go
git commit -m "feat(pgrepo): consumidor episodio.fechado transita chegada para ATENDIDO (ADR-038)"
```

---

### Task 8: Ligação na Plataforma (config, métricas, arranque do relay)

**Files:**
- Modify: `internal/platform/observ/` (ficheiro das métricas — acrescentar colectores + métodos)
- Test: `internal/platform/observ/observ_test.go` (se existir; senão criar um mínimo)
- Modify: `internal/platform/config/config.go`
- Test: `internal/platform/config/config_test.go` (se existir; acrescentar caso)
- Modify: `internal/platform/app.go`
- Test: `tests/integration/outbox_e2e_test.go`

**Interfaces:**
- Consumes: `outbox.NovoRelay`, `outbox.Relay.Correr/Registar`, `pgrepo.NovaIntegracaoPosConsulta`.
- Produces: `config.Config.OutboxIntervalo time.Duration`, `config.Config.OutboxLote int`; `(*observ.Metricas)` satisfaz `outbox.Observador` (métodos `Pendentes(int)`, `Publicado()`, `FalhaHandler(string)`).

- [ ] **Step 1: Config — acrescentar campos e defaults (teste primeiro)**

Em `internal/platform/config/config_test.go` (criar se não existir, `package config`), verificar os defaults:
```go
func TestCarregar_OutboxDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("REDIS_URL", "redis://x")
	t.Setenv("KEYCLOAK_ISSUER", "http://kc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "id")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "s")
	cfg, err := Carregar()
	if err != nil {
		t.Fatalf("carregar: %v", err)
	}
	if cfg.OutboxIntervalo != 2*time.Second {
		t.Fatalf("intervalo default errado: %v", cfg.OutboxIntervalo)
	}
	if cfg.OutboxLote != 100 {
		t.Fatalf("lote default errado: %d", cfg.OutboxLote)
	}
}
```
Implementar em `config.go`: adicionar ao struct `Config` os campos `OutboxIntervalo time.Duration` e `OutboxLote int`, e no `Carregar`:
```go
		OutboxIntervalo: time.Duration(inteiroOu("OUTBOX_INTERVALO_MS", 2000)) * time.Millisecond,
		OutboxLote:      inteiroOu("OUTBOX_LOTE", 100),
```
Run: `go test ./internal/platform/config/...` → PASS.

- [ ] **Step 2: Métricas — acrescentar colectores de outbox (teste primeiro)**

Em `internal/platform/observ/observ_test.go` (criar se não existir, `package observ`):
```go
func TestMetricas_SatisfazObservadorOutbox(t *testing.T) {
	m := Novo()
	// não deve entrar em pânico nem falhar ao registar/observar
	m.Pendentes(3)
	m.Publicado()
	m.FalhaHandler("clinico.episodio.fechado")
}
```
Implementar no ficheiro das métricas: acrescentar ao struct `Metricas` três colectores e registá-los em `Novo()`:
```go
	outboxPendentes *prometheus.GaugeVec // ou Gauge
	outboxPublicados prometheus.Counter
	outboxFalhas     *prometheus.CounterVec
```
Em `Novo()`, construir e `reg.MustRegister(...)`:
```go
	outboxPendentes := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sgc_outbox_pendentes", Help: "Eventos de outbox por publicar."})
	outboxPublicados := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "sgc_outbox_publicados_total", Help: "Eventos de outbox publicados."})
	outboxFalhas := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sgc_outbox_falhas_handler_total", Help: "Falhas de handler por tipo de evento."},
		[]string{"tipo_evento"})
	reg.MustRegister(outboxPendentes, outboxPublicados, outboxFalhas)
```
Atribuir aos campos do struct devolvido. Métodos (satisfazem `outbox.Observador` por estrutura, sem importar o pacote outbox):
```go
func (m *Metricas) Pendentes(n int)             { m.outboxPendentes.Set(float64(n)) }
func (m *Metricas) Publicado()                  { m.outboxPublicados.Inc() }
func (m *Metricas) FalhaHandler(tipo string)    { m.outboxFalhas.WithLabelValues(tipo).Inc() }
```
Run: `go test ./internal/platform/observ/...` → PASS.

- [ ] **Step 3: Ligar o relay no `app.go` (composition root)**

Em `internal/platform/app.go`, dentro de `ExecutarServidor`, **antes** de `return srv.Iniciar(ctx)` (o `metricas` já existe nesse escopo; `pool` e `logger` também):
```go
	// Relay do Outbox (ADR-038): publica eventos de domínio inter-BC. Handlers
	// registados por tipo; o loop pára com o ctx do shutdown gracioso.
	relay := outbox.NovoRelay(pool, cfg.OutboxLote, metricas, logger)
	posConsulta := pgrepo.NovaIntegracaoPosConsulta(pool)
	relay.Registar("clinico.episodio.fechado", posConsulta.HandlerEpisodioFechado)
	go relay.Correr(ctx, cfg.OutboxIntervalo)
```
Adicionar o import `"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"` (o `pgrepo` já está importado). `metricas` é passado como `outbox.Observador` (satisfaz por estrutura).

- [ ] **Step 4: Compilar e correr unit**

Run: `go build ./... && go test -race ./...`
Expected: PASS (compila; `metricas` satisfaz a interface).

- [ ] **Step 5: Teste e2e de integração (fecho real → chegada ATENDIDO via relay)**

`tests/integration/outbox_e2e_test.go` (tag `integration`): põe uma chegada EM_CONSULTA com episódio; fecha o episódio via `CasoFecharEpisodio` (ou via `RepositorioEpisodios.Guardar` com o agregado fechado, como na Task 5) — o que escreve o outbox; corre um `Relay` com o handler `pos.HandlerEpisodioFechado` registado; assere que a chegada ficou ATENDIDO:
```go
//go:build integration

package integration_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
)

func TestOutbox_E2E_FechoTransitaChegada(t *testing.T) {
	pool, ctx := ligar(t)
	episodioID, chegadaID := chegadaEmConsultaComEpisodio(t, pool, ctx)

	// fecha o episódio pelo repositório (escreve o outbox na mesma tx)
	repo := pgrepo.NovoRepositorioEpisodios(pool)
	ep, err := repo.ObterPorID(ctx, episodioID)
	if err != nil {
		t.Fatalf("obter episódio: %v", err)
	}
	prepararEpisodioParaFecho(t, ep) // nota completa + CID (helper local, ver nota)
	if err := ep.Fechar("33333333-3333-4333-8333-333333333333", agora()); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if _, err := repo.Guardar(ctx, ep); err != nil {
		t.Fatalf("guardar: %v", err)
	}

	// relay entrega ao consumidor
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	relay := outbox.NovoRelay(pool, 100, outbox.ObservadorNulo{}, log)
	pos := pgrepo.NovaIntegracaoPosConsulta(pool)
	relay.Registar("clinico.episodio.fechado", pos.HandlerEpisodioFechado)
	if _, err := relay.ProcessarLote(ctx); err != nil {
		t.Fatalf("processar lote: %v", err)
	}

	var estado string
	if err := pool.QueryRow(ctx, `SELECT estado FROM recepcao.chegadas WHERE id=$1`, chegadaID).Scan(&estado); err != nil {
		t.Fatalf("ler chegada: %v", err)
	}
	if estado != "ATENDIDO" {
		t.Fatalf("esperava ATENDIDO após o relay, obtive %q", estado)
	}
	_ = context.Background
}
```
> Nota: `prepararEpisodioParaFecho` garante que o episódio relido tem nota completa + ≥1 CID (o cenário de início da consulta cria o episódio ABERTO sem nota). Reutiliza os setters do agregado (`ActualizarNota`, `DefinirDiagnosticosCID`) como no helper da Task 1. Se o helper `chegadaEmConsultaComEpisodio` já criar o episódio com nota+CID, dispensa este passo.

- [ ] **Step 6: Correr integração completa**

Run: `go test -tags integration ./tests/integration/...`
Expected: PASS (ou SKIP sem `DATABASE_URL`).

- [ ] **Step 7: Cobertura + lint**

Run: `bash scripts/cobertura.sh && go-arch-lint check && golangci-lint run`
Expected: gates 85/75/60 verdes; sem violações arquitecturais; lint limpo.

- [ ] **Step 8: Commit**

```bash
git add internal/platform/observ/ internal/platform/config/ internal/platform/app.go tests/integration/outbox_e2e_test.go
git commit -m "feat(plataforma): arranque do relay de outbox, config e metricas (ADR-038)"
```

---

### Task 9: ADR-038 + documentação de marco

**Files:**
- Create: `adrs/ADR-038-outbox.md`
- Modify: `CLAUDE.md` (secção 6 — marco; lista de ADRs; "Próximo ADR: ADR-039")
- Modify: `SPRINT.md` (secção com critérios de saída do Outbox)

**Interfaces:** documentação — sem código.

- [ ] **Step 1: Escrever a ADR-038**

`adrs/ADR-038-outbox.md`, seguindo o formato das ADRs existentes (contexto, decisão, consequências). Conteúdo essencial: padrão Outbox in-process (poll + `SKIP LOCKED`, sem broker); escrita transaccional no repositório dono da tx; coleta de eventos no agregado rico; entrega at-least-once → consumidores idempotentes; primeiro consumidor `clinico.episodio.fechado` → chegada `ATENDIDO`; colunas `tentativas`/`ultimo_erro` (dead-lettering diferido); auditoria-na-tx **fora de âmbito**; interface `Observador` para respeitar a regra de dependência (adapters sem Prometheus). Registar alternativas rejeitadas (LISTEN/NOTIFY, Redis) e o candidato seguinte (cancelamento → chegada).

- [ ] **Step 2: Actualizar o índice de ADRs no CLAUDE.md**

Em `CLAUDE.md`, acrescentar `adrs/ADR-038-outbox.md` à lista e mudar "Próximo ADR: **ADR-038**" para "**ADR-039**". Na secção 6 (Marco Actual), acrescentar uma linha a registar a entrega do Outbox (mecanismo + `episodio.fechado → ATENDIDO`).

- [ ] **Step 3: Actualizar o SPRINT.md**

Acrescentar a secção "Critérios de saída — Outbox (ADR-038)" com os critérios da §10 da spec, todos `[x]`.

- [ ] **Step 4: Verificação final**

Run: `go build ./... && go test -race ./... && go test -tags integration ./tests/integration/... && go-arch-lint check`
Expected: tudo PASS/SKIP coerente.

- [ ] **Step 5: Commit**

```bash
git add adrs/ADR-038-outbox.md CLAUDE.md SPRINT.md
git commit -m "docs(shared): ADR-038 do Outbox e actualizacao de marco (ADR-038)"
```

---

## Notas de execução

- **Ordem sugerida**: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9. As Tasks 1–4 são independentes entre si (podem paralelizar); 5 depende de 1+4; 7 depende de 2+3; 8 depende de 5+6+7.
- **Integração precisa de PG**: `docker compose up -d postgres` e `export DATABASE_URL=postgres://…` antes das tasks com testes de integração; sem isso os testes **saltam** (não falham), mas a task não fica provada — subir o PG é obrigatório para dar a task por concluída.
- **DRY**: reutilizar helpers de integração existentes (`ligar`, `criaChegadaTriadaComDoente`, helpers de `episodios_test.go`) em vez de duplicar setup.
- **Sem placeholders no código entregue**: os `> Nota ao implementador` deste plano são para confirmar assinaturas reais (nomes de construtores de `NotaClinica`/`DiagnosticoCID`, helpers de teste), não código por preencher.
