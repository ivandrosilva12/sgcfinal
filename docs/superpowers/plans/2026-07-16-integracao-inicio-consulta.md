# Integração Recepção→Clínico: Início da Consulta — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** O médico atribuído consome a `Chegada` `TRIADO` da fila clínica e recebe o `EpisodioClinico` `ABERTO` na resposta — transição + criação atómicas (primeira escrita cross-BC do sistema).

**Architecture:** Novo estado `EM_CONSULTA` na `Chegada` (domínio recepcao) com guarda de dono; caso de uso `CasoIniciarConsulta` no BC Clínico falando por portas ACL (`LeitorRecepcao`, `ConsumidorChegadas`); adaptador de integração em `pgrepo` (Camada 3, importa ambos os domínios) que executa `INSERT` do episódio + `UPDATE` CAS da chegada numa única transacção PG. Spec: `docs/superpowers/specs/2026-07-16-integracao-inicio-consulta-design.md`.

**Tech Stack:** Go 1.22+, Gin, pgx v5 (SQL puro, sem ORM), PostgreSQL 16, migrações forward-only.

## Global Constraints

- **PT-PT angolano em TODA a saída** — código, comentários, commits, mensagens de erro. Nunca PT-BR, nunca EN visível.
- **Sem FK cross-context**: `recepcao.chegadas.episodio_id` é `uuid` nu.
- **Sem `panic()`** fora de inicialização — sempre `error` com categoria (`internal/domain/shared/erros`).
- **Domínio rico**: guardas de estado e de dono no agregado, não no SQL apenas.
- **Migrações forward-only**, sem `.down.sql`.
- **Cobertura**: domínio ≥85%, aplicação ≥75%, adapters ≥60% (pgrepo coberto por integração).
- **Fakes > mocks** nos testes de aplicação.
- Módulo Go: `github.com/ivandrosilva12/sgcfinal`. Tabela real do episódio: `clinico.episodios_clinicos` (não `clinico.episodios`).
- Commits terminam com `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Testes de integração: `//go:build integration`, package `integration_test`, usam `ligar(t)` (SKIPa sem `DATABASE_URL`) e `db.AplicarMigracoes`.

---

### Task 1: Domínio Recepção — estado `EM_CONSULTA`, método `IniciarConsulta`, campo `episodioID`

**Files:**
- Modify: `internal/domain/recepcao/chegada.go`
- Test: `internal/domain/recepcao/chegada_test.go` (acrescentar no fim)

**Interfaces:**
- Consumes: helpers de teste existentes `inst(hhmm string) time.Time` e `chegadaChamada(t *testing.T, walkin bool) *recepcao.Chegada` (já em `chegada_test.go`/pacote de teste).
- Produces (usado pelas Tasks 2 e 4):
  - `const ChegEmConsulta EstadoChegada = "EM_CONSULTA"`
  - `func (c *Chegada) IniciarConsulta(medicoID string, em time.Time) error` — Conflito se estado ≠ TRIADO; Proibido se `medicoID` (com trim) ≠ médico da chegada.
  - `func (c *Chegada) EpisodioID() string`
  - `SnapshotChegada.EpisodioID string` (novo campo, preservado por `Snapshot`/`ReconstruirChegada`).

- [ ] **Step 1: Escrever os testes que falham**

Acrescentar no fim de `internal/domain/recepcao/chegada_test.go`:

```go
// --- IniciarConsulta (integração início da consulta, ADR-036) ---

func chegadaTriadaTeste(t *testing.T) *recepcao.Chegada {
	t.Helper()
	c := chegadaChamada(t, false) // agendada, com med-1
	if err := c.RegistarTriada("", inst("09:10")); err != nil {
		t.Fatalf("registar triada: %v", err)
	}
	return c
}

func TestIniciarConsulta_DeTriado_PeloMedicoAtribuido(t *testing.T) {
	c := chegadaTriadaTeste(t)
	if err := c.IniciarConsulta("med-1", inst("09:30")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.Estado() != recepcao.ChegEmConsulta {
		t.Fatalf("esperava EM_CONSULTA, veio %s", c.Estado())
	}
	if !c.Snapshot().ActualizadoEm.Equal(inst("09:30")) {
		t.Fatal("actualizadoEm devia ser a hora do início da consulta")
	}
}

func TestIniciarConsulta_MedicoErrado_Proibido(t *testing.T) {
	c := chegadaTriadaTeste(t)
	if err := c.IniciarConsulta("med-9", inst("09:30")); erros.CategoriaDe(err) != erros.CategoriaProibido {
		t.Fatalf("médico não atribuído devia dar CategoriaProibido, veio %v", erros.CategoriaDe(err))
	}
	if c.Estado() != recepcao.ChegTriado {
		t.Fatalf("o estado não devia mudar, veio %s", c.Estado())
	}
}

func TestIniciarConsulta_MedicoVazio_Proibido(t *testing.T) {
	c := chegadaTriadaTeste(t)
	if err := c.IniciarConsulta("   ", inst("09:30")); erros.CategoriaDe(err) != erros.CategoriaProibido {
		t.Fatalf("médico vazio devia dar CategoriaProibido, veio %v", erros.CategoriaDe(err))
	}
}

func TestIniciarConsulta_ForaDeTriado_Conflito(t *testing.T) {
	casos := []struct {
		nome    string
		chegada func(t *testing.T) *recepcao.Chegada
	}{
		{"AGUARDA", func(t *testing.T) *recepcao.Chegada {
			c, _ := recepcao.NovaChegadaAgendada("doe-1", "marc-1", "med-1", "esp-1", inst("09:00"))
			return c
		}},
		{"CHAMADO", func(t *testing.T) *recepcao.Chegada { return chegadaChamada(t, false) }},
		{"DESISTIU", func(t *testing.T) *recepcao.Chegada {
			c, _ := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
			_ = c.RegistarDesistencia(inst("09:05"))
			return c
		}},
		{"EM_CONSULTA (duplo início)", func(t *testing.T) *recepcao.Chegada {
			c := chegadaTriadaTeste(t)
			if err := c.IniciarConsulta("med-1", inst("09:30")); err != nil {
				t.Fatalf("primeiro início devia suceder: %v", err)
			}
			return c
		}},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			c := caso.chegada(t)
			if err := c.IniciarConsulta("med-1", inst("09:40")); erros.CategoriaDe(err) != erros.CategoriaConflito {
				t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
			}
		})
	}
}

func TestChegada_SnapshotReconstrucao_EpisodioID(t *testing.T) {
	s := recepcao.SnapshotChegada{
		ID: "cheg-3", DoenteID: "doe-3", MedicoID: "med-3", EspecialidadeID: "esp-3",
		HoraChegada: inst("09:00"), Estado: recepcao.ChegEmConsulta, EpisodioID: "ep-1",
	}
	c := recepcao.ReconstruirChegada(s)
	if c.EpisodioID() != "ep-1" {
		t.Fatalf("EpisodioID mal reconstruído: %q", c.EpisodioID())
	}
	if c.Snapshot().EpisodioID != "ep-1" {
		t.Fatalf("EpisodioID perdido no snapshot: %+v", c.Snapshot())
	}
}
```

- [ ] **Step 2: Correr os testes e confirmar que falham**

Run: `go test ./internal/domain/recepcao/ -run "IniciarConsulta|EpisodioID" -v`
Expected: FAIL — erros de compilação (`c.IniciarConsulta undefined`, `recepcao.ChegEmConsulta undefined`, `unknown field EpisodioID`).

- [ ] **Step 3: Implementar no agregado**

Em `internal/domain/recepcao/chegada.go`:

3a. Actualizar o diagrama de estados no comentário de `EstadoChegada` e acrescentar a constante:

```go
// EstadoChegada é o estado do ciclo de vida de uma chegada (o doente na fila).
//
//	AGUARDA ─┬─ Chamar ──────────► CHAMADO ─ RegistarTriada ► TRIADO ─ IniciarConsulta ► EM_CONSULTA
//	         └─ RegistarDesistencia► DESISTIU
type EstadoChegada string

const (
	ChegAguarda    EstadoChegada = "AGUARDA"
	ChegChamado    EstadoChegada = "CHAMADO"
	ChegDesistiu   EstadoChegada = "DESISTIU"
	ChegTriado     EstadoChegada = "TRIADO"
	ChegEmConsulta EstadoChegada = "EM_CONSULTA"
)
```

3b. Novo campo na struct `Chegada` (a seguir a `medicoID`):

```go
	medicoID        string
	episodioID      string
```

3c. Novo método (a seguir a `RegistarTriada`):

```go
// IniciarConsulta transita TRIADO → EM_CONSULTA (o médico consome a chegada da
// fila clínica e abre a consulta — ADR-036). Só o médico atribuído pode iniciar.
// O episodioID não é atribuído aqui: o id do episódio é gerado pela BD na mesma
// transacção, escrito pelo adaptador de integração, e entra no agregado apenas
// por reconstrução.
func (c *Chegada) IniciarConsulta(medicoID string, em time.Time) error {
	if c.estado != ChegTriado {
		return erros.Novo(erros.CategoriaConflito, "só é possível iniciar a consulta de uma chegada triada")
	}
	if strings.TrimSpace(medicoID) != c.medicoID {
		return erros.Novo(erros.CategoriaProibido, "só o médico atribuído pode iniciar a consulta")
	}
	c.estado = ChegEmConsulta
	c.actualizadoEm = em
	return nil
}
```

3d. Getter (junto aos restantes):

```go
// EpisodioID devolve o episódio que consumiu a chegada (vazio até EM_CONSULTA).
func (c *Chegada) EpisodioID() string { return c.episodioID }
```

3e. `SnapshotChegada`: acrescentar `EpisodioID string` a seguir a `MedicoID`; em `Snapshot()` acrescentar `EpisodioID: c.episodioID,`; em `ReconstruirChegada` acrescentar `episodioID: s.EpisodioID,`.

- [ ] **Step 4: Correr os testes e confirmar que passam**

Run: `go test ./internal/domain/recepcao/ -v`
Expected: PASS (todos, incluindo os pré-existentes).

- [ ] **Step 5: Commit**

```bash
git add internal/domain/recepcao/chegada.go internal/domain/recepcao/chegada_test.go
git commit -m "feat(recepcao): estado EM_CONSULTA e IniciarConsulta com guarda de dono (ADR-036)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Migração `recepcao/0004` + leitura de `episodio_id` no repositório de chegadas

**Files:**
- Create: `migrations/recepcao/0004_chegadas_em_consulta.sql`
- Modify: `internal/adapters/pgrepo/chegadas_repo.go` (constante `colunasChegada` + scan de `ObterPorID`)

**Interfaces:**
- Consumes: `SnapshotChegada.EpisodioID` (Task 1).
- Produces: coluna `recepcao.chegadas.episodio_id uuid` (sem FK), `CHECK` do estado com `EM_CONSULTA`, `CHECK` de episódio obrigatório em `EM_CONSULTA`, índice único parcial `chegadas_episodio_id_unico`; `colunasChegada` passa a devolver `episodio_id` (posição: entre `medico_id` e `hora_chegada` — a Task 4 usa esta ordem no scan).

- [ ] **Step 1: Escrever a migração**

Criar `migrations/recepcao/0004_chegadas_em_consulta.sql`:

```sql
-- 0004_chegadas_em_consulta.sql — Integração Recepção→Clínico: início da consulta
-- (ADR-036). Estende a chegada com o estado EM_CONSULTA e a referência ao episódio
-- que a consumiu. Forward-only.

-- Estende o enum de estado da chegada com EM_CONSULTA.
ALTER TABLE recepcao.chegadas DROP CONSTRAINT chegadas_estado_check;
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_estado_check
    CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU','TRIADO','EM_CONSULTA'));

-- O episódio que consumiu a chegada (uuid nu — sem FK cross-context; o episódio
-- vive no schema clinico e a integridade é da transacção do adaptador de integração).
ALTER TABLE recepcao.chegadas ADD COLUMN IF NOT EXISTS episodio_id uuid;

-- Uma chegada EM_CONSULTA aponta obrigatoriamente para o seu episódio.
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_em_consulta_episodio_check
    CHECK (estado <> 'EM_CONSULTA' OR episodio_id IS NOT NULL);

-- 1:1 — um episódio consome no máximo uma chegada (defesa em profundidade; a
-- garantia primária é a guarda CAS da transacção única).
CREATE UNIQUE INDEX IF NOT EXISTS chegadas_episodio_id_unico
    ON recepcao.chegadas (episodio_id) WHERE episodio_id IS NOT NULL;
```

- [ ] **Step 2: Expor `episodio_id` na leitura do repositório de chegadas**

Em `internal/adapters/pgrepo/chegadas_repo.go`, substituir a constante:

```go
const colunasChegada = `id::text, doente_id::text, COALESCE(marcacao_id::text,''),
       especialidade_id::text, COALESCE(medico_id::text,''), COALESCE(episodio_id::text,''),
       hora_chegada, estado, criado_em, actualizado_em`
```

E em `ObterPorID`, actualizar o scan para a nova ordem:

```go
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.DoenteID, &s.MarcacaoID, &s.EspecialidadeID,
		&s.MedicoID, &s.EpisodioID, &s.HoraChegada, &estado, &s.CriadoEm, &s.ActualizadoEm)
```

(`Guardar`/`inserir`, `Transitar` e `ListarFila` não mudam: chegadas novas nunca têm episódio e o `episodio_id` só é escrito pelo adaptador de integração.)

- [ ] **Step 3: Compilar e correr os testes unitários**

Run: `go build ./... && go test ./internal/adapters/pgrepo/`
Expected: build OK; testes unitários (helpers) PASS.

- [ ] **Step 4: Provar a migração contra a BD real (se disponível)**

Run (PowerShell, com o Postgres do docker compose a correr): `go test -tags integration ./tests/integration/ -run Migracoes -v`
Expected: PASS (aplica todas as migrações incluindo a 0004). Sem `DATABASE_URL`: SKIP — aceitável; o CI prova.

- [ ] **Step 5: Commit**

```bash
git add migrations/recepcao/0004_chegadas_em_consulta.sql internal/adapters/pgrepo/chegadas_repo.go
git commit -m "feat(recepcao): migração 0004 — EM_CONSULTA e episodio_id na chegada (ADR-036)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Aplicação Clínico — portas ACL e `CasoIniciarConsulta`

**Files:**
- Modify: `internal/application/clinico/ports.go` (acrescentar na secção `--- Episódio Clínico ---`, antes de `--- Consentimento (LPDP) ---`)
- Create: `internal/application/clinico/iniciar_consulta.go`
- Test: `internal/application/clinico/iniciar_consulta_test.go`

**Interfaces:**
- Consumes: `dominio.RepositorioDoentes`, `dominio.RepositorioEpisodios`, `Auditor`, `paraDetalheEpisodio`, `dominio.EpisodioConsulta`; fakes de teste existentes `novoFakeRepo()`, `registarNoRepo(t, repo) string`, `novoFakeRepoEpisodios() *fakeRepoEpisodios`, `fakeAuditor{}` (campo `registos`); `(*dominio.Doente).Desactivar(motivo string, em time.Time) error`.
- Produces (usado pelas Tasks 4, 5 e 6):
  - `type ChegadaTriada struct { DoenteID, MedicoID, EspecialidadeID string }`
  - `type LeitorRecepcao interface { ChegadaTriada(ctx context.Context, chegadaID string) (ChegadaTriada, error) }`
  - `type ConsumidorChegadas interface { ConsumirEIniciar(ctx context.Context, chegadaID, medicoID string, episodio *dominio.EpisodioClinico) (string, error) }`
  - `func NovoCasoIniciarConsulta(recepcao LeitorRecepcao, consumidor ConsumidorChegadas, doentes dominio.RepositorioDoentes, episodios dominio.RepositorioEpisodios, aud Auditor) *CasoIniciarConsulta`
  - `func (c *CasoIniciarConsulta) Executar(ctx context.Context, actor, chegadaID string) (DetalheEpisodio, error)`

- [ ] **Step 1: Escrever os testes que falham**

Criar `internal/application/clinico/iniciar_consulta_test.go`:

```go
package clinico_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes das portas de integração (ADR-036) ---

type fakeLeitorRecepcao struct {
	out appclinico.ChegadaTriada
	err error
}

func (f fakeLeitorRecepcao) ChegadaTriada(_ context.Context, _ string) (appclinico.ChegadaTriada, error) {
	return f.out, f.err
}

// fakeConsumidorChegadas delega no fakeRepoEpisodios para o episódio ficar
// disponível na releitura final do caso de uso.
type fakeConsumidorChegadas struct {
	repo      *fakeRepoEpisodios
	err       error
	chegadaID string
	medicoID  string
	chamadas  int
}

func (f *fakeConsumidorChegadas) ConsumirEIniciar(ctx context.Context, chegadaID, medicoID string, e *clinico.EpisodioClinico) (string, error) {
	f.chamadas++
	if f.err != nil {
		return "", f.err
	}
	f.chegadaID, f.medicoID = chegadaID, medicoID
	return f.repo.Guardar(ctx, e)
}

func casoIniciarConsultaTeste(t *testing.T) (*appclinico.CasoIniciarConsulta, string, *fakeConsumidorChegadas, *fakeAuditor) {
	t.Helper()
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes) // doente ACTIVO
	repoEp := novoFakeRepoEpisodios()
	consumidor := &fakeConsumidorChegadas{repo: repoEp}
	leitor := fakeLeitorRecepcao{out: appclinico.ChegadaTriada{
		DoenteID: doenteID, MedicoID: "medico-1", EspecialidadeID: "esp-1",
	}}
	aud := &fakeAuditor{}
	return appclinico.NovoCasoIniciarConsulta(leitor, consumidor, repoDoentes, repoEp, aud), doenteID, consumidor, aud
}

func TestIniciarConsulta_FluxoFeliz(t *testing.T) {
	caso, doenteID, consumidor, aud := casoIniciarConsultaTeste(t)

	out, err := caso.Executar(context.Background(), "medico-1", "cheg-1")
	if err != nil {
		t.Fatalf("iniciar consulta: %v", err)
	}
	if out.ID == "" || out.Estado != "ABERTO" || out.Tipo != "CONSULTA" {
		t.Fatalf("episódio inesperado: %+v", out)
	}
	if out.DoenteID != doenteID || out.MedicoID != "medico-1" || out.EspecialidadeID != "esp-1" {
		t.Fatalf("dados da chegada mal propagados: %+v", out)
	}
	if consumidor.chegadaID != "cheg-1" || consumidor.medicoID != "medico-1" {
		t.Fatalf("consumidor mal invocado: %+v", consumidor)
	}
	if len(aud.registos) != 2 ||
		aud.registos[0].Accao != "clinico.episodio.aberto" ||
		aud.registos[1].Accao != "recepcao.chegada.consulta_iniciada" {
		t.Fatalf("auditoria inesperada: %+v", aud.registos)
	}
	if aud.registos[1].EntidadeID != "cheg-1" || aud.registos[1].Entidade != "chegada" {
		t.Fatalf("auditoria da chegada mal preenchida: %+v", aud.registos[1])
	}
}

func TestIniciarConsulta_ChegadaNaoEncontrada_Propaga(t *testing.T) {
	caso, _, consumidor, _ := casoIniciarConsultaTeste(t)
	// substitui o leitor por um que devolve 404 — reconstruir o caso
	repoDoentes := novoFakeRepo()
	repoEp := novoFakeRepoEpisodios()
	caso = appclinico.NovoCasoIniciarConsulta(
		fakeLeitorRecepcao{err: erros.Novo(erros.CategoriaNaoEncontrado, "chegada triada não encontrada")},
		consumidor, repoDoentes, repoEp, &fakeAuditor{})

	if _, err := caso.Executar(context.Background(), "medico-1", "cheg-x"); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava CategoriaNaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

func TestIniciarConsulta_ActorNaoEOMedico_Proibido(t *testing.T) {
	caso, _, consumidor, aud := casoIniciarConsultaTeste(t)

	_, err := caso.Executar(context.Background(), "medico-2", "cheg-1")
	if erros.CategoriaDe(err) != erros.CategoriaProibido {
		t.Fatalf("esperava CategoriaProibido, veio %v", erros.CategoriaDe(err))
	}
	if consumidor.chamadas != 0 {
		t.Fatal("o consumidor não devia ser invocado quando o actor não é o médico")
	}
	if len(aud.registos) != 0 {
		t.Fatalf("não devia haver auditoria: %+v", aud.registos)
	}
}

func TestIniciarConsulta_DoenteInactivo_Conflito(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	d, err := repoDoentes.ObterPorID(context.Background(), doenteID)
	if err != nil {
		t.Fatalf("obter doente: %v", err)
	}
	if err := d.Desactivar("mudou de clínica", time.Now()); err != nil {
		t.Fatalf("desactivar doente: %v", err)
	}
	repoEp := novoFakeRepoEpisodios()
	consumidor := &fakeConsumidorChegadas{repo: repoEp}
	caso := appclinico.NovoCasoIniciarConsulta(
		fakeLeitorRecepcao{out: appclinico.ChegadaTriada{DoenteID: doenteID, MedicoID: "medico-1", EspecialidadeID: "esp-1"}},
		consumidor, repoDoentes, repoEp, &fakeAuditor{})

	if _, err := caso.Executar(context.Background(), "medico-1", "cheg-1"); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
	if consumidor.chamadas != 0 {
		t.Fatal("o consumidor não devia ser invocado com o doente inactivo")
	}
}

func TestIniciarConsulta_FalhaDoConsumidor_PropagaSemAuditar(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	falha := erros.Novo(erros.CategoriaConflito, "o estado da chegada mudou entretanto; recarregue e repita a operação")
	consumidor := &fakeConsumidorChegadas{repo: repoEp, err: falha}
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoIniciarConsulta(
		fakeLeitorRecepcao{out: appclinico.ChegadaTriada{DoenteID: doenteID, MedicoID: "medico-1", EspecialidadeID: "esp-1"}},
		consumidor, repoDoentes, repoEp, aud)

	_, err := caso.Executar(context.Background(), "medico-1", "cheg-1")
	if !errors.Is(err, falha) {
		t.Fatalf("esperava a falha do consumidor, veio %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("não devia haver auditoria após falha: %+v", aud.registos)
	}
}
```

- [ ] **Step 2: Correr os testes e confirmar que falham**

Run: `go test ./internal/application/clinico/ -run IniciarConsulta -v`
Expected: FAIL — compilação (`appclinico.ChegadaTriada undefined`, `NovoCasoIniciarConsulta undefined`).

- [ ] **Step 3: Acrescentar as portas em `ports.go`**

Em `internal/application/clinico/ports.go`, imediatamente antes de `// --- Consentimento (LPDP) ---`:

```go
// --- Integração Recepção→Clínico: início da consulta (ADR-036) ---

// ChegadaTriada é o retrato mínimo de uma chegada TRIADO — DTO da porta
// anti-corrupção, sem tipos do domínio Recepção.
type ChegadaTriada struct {
	DoenteID        string
	MedicoID        string
	EspecialidadeID string
}

// LeitorRecepcao é a porta anti-corrupção para leitura do BC Recepção. O Clínico
// nunca importa tipos do domínio Recepção: só faz esta pergunta.
type LeitorRecepcao interface {
	// ChegadaTriada devolve a chegada se existir e estiver TRIADO
	// (CategoriaNaoEncontrado caso contrário).
	ChegadaTriada(ctx context.Context, chegadaID string) (ChegadaTriada, error)
}

// ConsumidorChegadas consome a chegada TRIADO e cria o episódio, atomicamente:
// insere o episódio e transita a chegada TRIADO→EM_CONSULTA (guarda CAS por
// estado e médico) numa única transacção. Devolve o id do episódio criado.
type ConsumidorChegadas interface {
	ConsumirEIniciar(ctx context.Context, chegadaID, medicoID string, episodio *dominio.EpisodioClinico) (string, error)
}
```

- [ ] **Step 4: Implementar o caso de uso**

Criar `internal/application/clinico/iniciar_consulta.go`:

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoIniciarConsulta consome uma chegada TRIADO da fila clínica (BC Recepção) e
// abre o episódio CONSULTA correspondente, atomicamente (ADR-036). Só o médico
// atribuído à chegada pode iniciar; a guarda corre aqui (erro claro) e novamente
// no CAS da transacção (fecha a corrida entre a leitura e a escrita).
type CasoIniciarConsulta struct {
	recepcao   LeitorRecepcao
	consumidor ConsumidorChegadas
	doentes    dominio.RepositorioDoentes
	episodios  dominio.RepositorioEpisodios
	auditor    Auditor
	agora      func() time.Time
}

// NovoCasoIniciarConsulta constrói o caso de uso.
func NovoCasoIniciarConsulta(recepcao LeitorRecepcao, consumidor ConsumidorChegadas,
	doentes dominio.RepositorioDoentes, episodios dominio.RepositorioEpisodios, aud Auditor) *CasoIniciarConsulta {
	return &CasoIniciarConsulta{recepcao: recepcao, consumidor: consumidor,
		doentes: doentes, episodios: episodios, auditor: aud, agora: time.Now}
}

// Executar valida a chegada (TRIADO, do médico actor) e o doente (activo), cria o
// episódio CONSULTA e consome a chegada na mesma transacção, audita nos dois
// contextos e devolve o episódio aberto.
func (c *CasoIniciarConsulta) Executar(ctx context.Context, actor, chegadaID string) (DetalheEpisodio, error) {
	ch, err := c.recepcao.ChegadaTriada(ctx, chegadaID)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if actor != ch.MedicoID {
		return DetalheEpisodio{}, erros.Novo(erros.CategoriaProibido, "só o médico atribuído pode iniciar a consulta")
	}
	doente, err := c.doentes.ObterPorID(ctx, ch.DoenteID)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if doente.Estado() != dominio.EstadoActivo {
		return DetalheEpisodio{}, erros.Novo(erros.CategoriaConflito, "não é possível abrir um episódio a um doente que não está activo")
	}
	episodio, err := dominio.NovoEpisodio(ch.DoenteID, dominio.EpisodioConsulta, ch.EspecialidadeID, actor, c.agora())
	if err != nil {
		return DetalheEpisodio{}, err
	}
	id, err := c.consumidor.ConsumirEIniciar(ctx, chegadaID, actor, episodio)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.aberto",
		Entidade: "episodio", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.chegada.consulta_iniciada",
		Entidade: "chegada", EntidadeID: chegadaID, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheEpisodio{}, err
	}
	final, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	return paraDetalheEpisodio(final), nil
}
```

- [ ] **Step 5: Correr os testes e confirmar que passam**

Run: `go test ./internal/application/clinico/ -v -run IniciarConsulta`
Expected: PASS (os 5 testes novos). Depois: `go test ./internal/application/clinico/` — PASS completo.

- [ ] **Step 6: Commit**

```bash
git add internal/application/clinico/ports.go internal/application/clinico/iniciar_consulta.go internal/application/clinico/iniciar_consulta_test.go
git commit -m "feat(clinico): CasoIniciarConsulta com portas ACL sobre a Recepção (ADR-036)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Adaptador de integração pgrepo — transacção única + teste de integração

**Files:**
- Create: `internal/adapters/pgrepo/integracao_inicio_consulta.go`
- Test: `tests/integration/inicio_consulta_test.go`

**Interfaces:**
- Consumes: `colunasChegada` (Task 2 — ordem: id, doente, marcacao, especialidade, medico, **episodio**, hora, estado, criado, actualizado), `(*RepositorioEpisodios).inserirEpisodio(ctx, tx, s)` (mesmo pacote), `Chegada.IniciarConsulta` (Task 1), portas `appclinico.LeitorRecepcao`/`appclinico.ConsumidorChegadas` (Task 3); helpers de integração `ligar(t)`, `db.AplicarMigracoes`.
- Produces (usado pela Task 6): `func NovaIntegracaoInicioConsulta(pool *pgxpool.Pool) *IntegracaoInicioConsulta` — implementa AMBAS as portas.

- [ ] **Step 1: Escrever o teste de integração que falha**

Criar `tests/integration/inicio_consulta_test.go`:

```go
//go:build integration

// Teste de integração do adaptador de integração Recepção→Clínico (início da
// consulta, ADR-036) contra a BD real. Prova a atomicidade da transacção única
// (INSERT do episódio + CAS da chegada), a recusa do médico errado e do duplo
// início, e as restrições de defesa em profundidade da migração recepcao/0004.
// SKIPa (nunca FAIL) quando DATABASE_URL não está definido.
package integration_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	domclinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	domrecepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

const (
	espInicioConsulta = "00000000-0000-4000-8000-000000000031"
	medInicioConsulta = "00000000-0000-4000-8000-000000000032"
	enfInicioConsulta = "00000000-0000-4000-8000-000000000033"
)

// criaChegadaTriadaComDoente cria um doente ACTIVO (FK do episódio) e uma chegada
// walk-in TRIADO atribuída a medInicioConsulta (via triagem transaccional, o único
// caminho real para TRIADO). Regista a limpeza.
func criaChegadaTriadaComDoente(t *testing.T, pool *pgxpool.Pool, ctx context.Context, bi, telefone string) (doenteID, chegadaID string) {
	t.Helper()
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	nasc := time.Date(1988, 2, 10, 0, 0, 0, 0, time.UTC)
	num, _ := repoDoentes.ProximoNumeroProcesso(ctx, 2026)
	ident, _ := domclinico.NovaIdentificacao("Rui Consulta", nasc, domclinico.SexoMasculino, &bi, nil, nil)
	ct, _ := domclinico.NovosContactos(telefone, nil, nil)
	doente, _ := domclinico.NovoDoente(num, ident, ct, "AO")
	var err error
	doenteID, err = repoDoentes.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar doente: %v", err)
	}

	chegRepo := pgrepo.NovoRepositorioChegadas(pool)
	triRepo := pgrepo.NovoRepositorioTriagens(pool)
	c, err := domrecepcao.NovaChegadaWalkIn(doenteID, espInicioConsulta, time.Now())
	if err != nil {
		t.Fatalf("construir chegada: %v", err)
	}
	if err := c.Chamar(time.Now()); err != nil {
		t.Fatalf("chamar (domínio): %v", err)
	}
	chegadaID, err = chegRepo.Guardar(ctx, c)
	if err != nil {
		t.Fatalf("guardar chegada: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.episodios_clinicos WHERE doente_id=$1`, doenteID)
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.triagens WHERE chegada_id=$1`, chegadaID)
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.chegadas WHERE id=$1`, chegadaID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})

	obt, err := chegRepo.ObterPorID(ctx, chegadaID)
	if err != nil {
		t.Fatalf("obter chegada: %v", err)
	}
	if err := obt.RegistarTriada(medInicioConsulta, time.Now()); err != nil {
		t.Fatalf("registar triada (domínio): %v", err)
	}
	tr, err := domrecepcao.NovaTriagem(chegadaID, enfInicioConsulta, domrecepcao.ManVerde, domrecepcao.SinaisVitais{}, "", time.Now())
	if err != nil {
		t.Fatalf("construir triagem: %v", err)
	}
	if _, err := triRepo.RegistarTriagem(ctx, tr, obt); err != nil {
		t.Fatalf("registar triagem: %v", err)
	}
	return doenteID, chegadaID
}

func contaEpisodios(t *testing.T, pool *pgxpool.Pool, ctx context.Context, doenteID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM clinico.episodios_clinicos WHERE doente_id=$1`, doenteID).Scan(&n); err != nil {
		t.Fatalf("contar episódios: %v", err)
	}
	return n
}

func TestIntegracaoInicioConsulta_FluxoFeliz(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	doenteID, chegadaID := criaChegadaTriadaComDoente(t, pool, ctx, "00987654LA021", "+244923111222")
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)

	// o leitor devolve o retrato da chegada TRIADO
	ct, err := integ.ChegadaTriada(ctx, chegadaID)
	if err != nil || ct.DoenteID != doenteID || ct.MedicoID != medInicioConsulta || ct.EspecialidadeID != espInicioConsulta {
		t.Fatalf("chegada triada: %v (%+v)", err, ct)
	}

	// consumir e iniciar: episódio criado + chegada EM_CONSULTA, atomicamente
	ep, err := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	if err != nil {
		t.Fatalf("construir episódio: %v", err)
	}
	epID, err := integ.ConsumirEIniciar(ctx, chegadaID, medInicioConsulta, ep)
	if err != nil {
		t.Fatalf("consumir e iniciar: %v", err)
	}
	lido, err := pgrepo.NovoRepositorioEpisodios(pool).ObterPorID(ctx, epID)
	if err != nil || lido.Estado() != domclinico.EstadoEpisodioAberto {
		t.Fatalf("episódio não ficou ABERTO: %v", err)
	}
	rec, err := pgrepo.NovoRepositorioChegadas(pool).ObterPorID(ctx, chegadaID)
	if err != nil || rec.Estado() != domrecepcao.ChegEmConsulta || rec.EpisodioID() != epID {
		t.Fatalf("chegada mal consumida: %v estado=%s episodio=%q", err, rec.Estado(), rec.EpisodioID())
	}

	// o leitor deixa de devolver a chegada (saiu da fila clínica)
	if _, err := integ.ChegadaTriada(ctx, chegadaID); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("chegada consumida devia dar NaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

func TestIntegracaoInicioConsulta_ChegadaInexistente_NaoEncontrado(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)
	if _, err := integ.ChegadaTriada(ctx, "00000000-0000-4000-8000-00000000dead"); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

func TestIntegracaoInicioConsulta_MedicoErrado_ProibidoENadaMuda(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	doenteID, chegadaID := criaChegadaTriadaComDoente(t, pool, ctx, "00987655LA022", "+244923111333")
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)

	outro := "00000000-0000-4000-8000-000000000099"
	ep, _ := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, outro, time.Now())
	if _, err := integ.ConsumirEIniciar(ctx, chegadaID, outro, ep); erros.CategoriaDe(err) != erros.CategoriaProibido {
		t.Fatalf("médico errado devia dar Proibido, veio %v", erros.CategoriaDe(err))
	}
	// atomicidade: nem episódio criado, nem chegada consumida
	if n := contaEpisodios(t, pool, ctx, doenteID); n != 0 {
		t.Fatalf("não devia existir episódio, existem %d", n)
	}
	rec, _ := pgrepo.NovoRepositorioChegadas(pool).ObterPorID(ctx, chegadaID)
	if rec.Estado() != domrecepcao.ChegTriado {
		t.Fatalf("a chegada devia continuar TRIADO, veio %s", rec.Estado())
	}
}

func TestIntegracaoInicioConsulta_DuploInicio_Conflito(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	doenteID, chegadaID := criaChegadaTriadaComDoente(t, pool, ctx, "00987656LA023", "+244923111444")
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)

	ep1, _ := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	if _, err := integ.ConsumirEIniciar(ctx, chegadaID, medInicioConsulta, ep1); err != nil {
		t.Fatalf("primeiro início: %v", err)
	}
	ep2, _ := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	if _, err := integ.ConsumirEIniciar(ctx, chegadaID, medInicioConsulta, ep2); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("duplo início devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
	// atomicidade: só o primeiro episódio existe
	if n := contaEpisodios(t, pool, ctx, doenteID); n != 1 {
		t.Fatalf("devia existir exactamente 1 episódio, existem %d", n)
	}
}

func TestIntegracaoInicioConsulta_RestricoesMigracao0004(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	doenteID, chegadaA := criaChegadaTriadaComDoente(t, pool, ctx, "00987657LA024", "+244923111555")
	_, chegadaB := criaChegadaTriadaComDoente(t, pool, ctx, "00987658LA025", "+244923111666")
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)

	// CHECK: EM_CONSULTA sem episodio_id → 23514
	var pgErr *pgconn.PgError
	_, err := pool.Exec(ctx, `UPDATE recepcao.chegadas SET estado='EM_CONSULTA' WHERE id=$1`, chegadaB)
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("EM_CONSULTA sem episódio devia violar o CHECK (23514), veio %v", err)
	}

	// UNIQUE parcial: duas chegadas com o mesmo episodio_id → 23505
	ep, _ := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	epID, err := integ.ConsumirEIniciar(ctx, chegadaA, medInicioConsulta, ep)
	if err != nil {
		t.Fatalf("iniciar consulta da chegada A: %v", err)
	}
	_, err = pool.Exec(ctx,
		`UPDATE recepcao.chegadas SET estado='EM_CONSULTA', episodio_id=$2 WHERE id=$1`, chegadaB, epID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("episodio_id repetido devia violar o UNIQUE (23505), veio %v", err)
	}
}
```

- [ ] **Step 2: Correr o teste e confirmar que falha**

Run: `go vet -tags integration ./tests/integration/`
Expected: FAIL — `pgrepo.NovaIntegracaoInicioConsulta undefined`. (Com `DATABASE_URL` definido, `go test -tags integration ./tests/integration/ -run IntegracaoInicioConsulta` também falha na compilação.)

- [ ] **Step 3: Implementar o adaptador de integração**

Criar `internal/adapters/pgrepo/integracao_inicio_consulta.go`:

```go
// internal/adapters/pgrepo/integracao_inicio_consulta.go
//
// Adaptador de integração Recepção→Clínico (ADR-036): o único componente que
// conhece os dois contextos. Implementa as portas appclinico.LeitorRecepcao e
// appclinico.ConsumidorChegadas — a primeira escrita cross-BC do sistema, numa
// única transacção PG (INSERT do episódio + CAS da chegada). Camada 3: um
// adaptador pode importar ambos os domínios; a regra de dependência proíbe
// infra no domínio, não adaptadores multi-contexto.
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	domclinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	domrecepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// IntegracaoInicioConsulta implementa as portas de integração do início da consulta.
type IntegracaoInicioConsulta struct {
	pool      *pgxpool.Pool
	episodios *RepositorioEpisodios
}

// NovaIntegracaoInicioConsulta constrói o adaptador sobre o pool pgx.
func NovaIntegracaoInicioConsulta(pool *pgxpool.Pool) *IntegracaoInicioConsulta {
	return &IntegracaoInicioConsulta{pool: pool, episodios: NovoRepositorioEpisodios(pool)}
}

// ChegadaTriada devolve o retrato mínimo de uma chegada TRIADO. NaoEncontrado se
// a chegada não existir ou não estiver TRIADO (para o Clínico é a mesma resposta:
// não há nada na fila para consumir com este id).
func (a *IntegracaoInicioConsulta) ChegadaTriada(ctx context.Context, chegadaID string) (appclinico.ChegadaTriada, error) {
	const q = `SELECT doente_id::text, COALESCE(medico_id::text,''), especialidade_id::text
FROM recepcao.chegadas WHERE id=$1 AND estado='TRIADO'`
	var ct appclinico.ChegadaTriada
	err := a.pool.QueryRow(ctx, q, chegadaID).Scan(&ct.DoenteID, &ct.MedicoID, &ct.EspecialidadeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appclinico.ChegadaTriada{}, erros.Novo(erros.CategoriaNaoEncontrado, "chegada triada não encontrada")
		}
		return appclinico.ChegadaTriada{}, fmt.Errorf("obter chegada triada: %w", err)
	}
	return ct, nil
}

// ConsumirEIniciar insere o episódio e transita a chegada TRIADO→EM_CONSULTA numa
// única transacção. As regras correm no domínio da Recepção (estado + médico, com
// a categoria certa: 409/403); a guarda CAS do UPDATE fecha a corrida entre a
// leitura em transacção e a escrita.
func (a *IntegracaoInicioConsulta) ConsumirEIniciar(ctx context.Context, chegadaID, medicoID string, episodio *domclinico.EpisodioClinico) (string, error) {
	se := episodio.Snapshot()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção de início de consulta: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	// lê a chegada dentro da transacção e aplica as regras no domínio
	q := `SELECT ` + colunasChegada + ` FROM recepcao.chegadas WHERE id=$1`
	var sc domrecepcao.SnapshotChegada
	var estado string
	err = tx.QueryRow(ctx, q, chegadaID).Scan(&sc.ID, &sc.DoenteID, &sc.MarcacaoID,
		&sc.EspecialidadeID, &sc.MedicoID, &sc.EpisodioID, &sc.HoraChegada, &estado,
		&sc.CriadoEm, &sc.ActualizadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", erros.Novo(erros.CategoriaNaoEncontrado, "chegada não encontrada")
		}
		return "", fmt.Errorf("obter chegada: %w", err)
	}
	sc.Estado = domrecepcao.EstadoChegada(estado)
	chegada := domrecepcao.ReconstruirChegada(sc)
	if err := chegada.IniciarConsulta(medicoID, se.Inicio); err != nil {
		return "", err
	}

	epID, err := a.episodios.inserirEpisodio(ctx, tx, se)
	if err != nil {
		return "", err
	}

	scDepois := chegada.Snapshot()
	const upd = `UPDATE recepcao.chegadas
SET estado=$2, episodio_id=$3::uuid, actualizado_em=$4
WHERE id=$1 AND estado=$5 AND medico_id=$6::uuid`
	ct, err := tx.Exec(ctx, upd, scDepois.ID, string(scDepois.Estado), epID,
		scDepois.ActualizadoEm, string(scDepois.EstadoAnterior), medicoID)
	if err != nil {
		return "", fmt.Errorf("consumir chegada: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", erros.Novo(erros.CategoriaConflito,
			"o estado da chegada mudou entretanto; recarregue e repita a operação")
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar início de consulta: %w", err)
	}
	return epID, nil
}

// Garantias de conformidade com as portas.
var (
	_ appclinico.LeitorRecepcao     = (*IntegracaoInicioConsulta)(nil)
	_ appclinico.ConsumidorChegadas = (*IntegracaoInicioConsulta)(nil)
)
```

- [ ] **Step 4: Compilar, vet e correr a integração**

Run: `go build ./... && go vet -tags integration ./tests/integration/`
Expected: OK, sem erros.

Run (com `DATABASE_URL` definido — Postgres do docker compose): `go test -tags integration ./tests/integration/ -run IntegracaoInicioConsulta -v`
Expected: PASS (5 testes). Sem `DATABASE_URL`: SKIP — correr então no CI.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/pgrepo/integracao_inicio_consulta.go tests/integration/inicio_consulta_test.go
git commit -m "feat(integracao): adaptador Recepção→Clínico com transacção única (ADR-036)

Primeira escrita cross-BC: INSERT do episódio + CAS da chegada TRIADO→EM_CONSULTA
numa só transacção PG, com as regras no domínio da Recepção e prova de
atomicidade, corrida e restrições da migração 0004 em integração.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Handler HTTP `POST /api/v1/chegadas/:cid/iniciar-consulta`

**Files:**
- Create: `internal/adapters/http/clinico_consulta_handler.go`
- Test: `internal/adapters/http/clinico_consulta_test.go`

**Interfaces:**
- Consumes: `appclinico.DetalheEpisodio` (Task 3), helpers `RBAC`, `SessaoDe`, `responderErro`, `Auth`, e nos testes `novoRouter()`, `fakeAuth{sessao}`, `pedidoCorpo(r, método, rota, corpo)`.
- Produces (usado pela Task 6):
  - `type ServicoIniciarConsulta interface { Executar(ctx context.Context, actor, chegadaID string) (appclinico.DetalheEpisodio, error) }`
  - `func NovoClinicoConsultaHandler(iniciar ServicoIniciarConsulta) *ClinicoConsultaHandler`
  - `func RegistarClinicoConsulta(r gin.IRouter, h *ClinicoConsultaHandler, protecao ...gin.HandlerFunc)`

- [ ] **Step 1: Escrever os testes que falham**

Criar `internal/adapters/http/clinico_consulta_test.go`:

```go
package http_test

import (
	"context"
	nethttp "net/http"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

type fakeIniciarConsulta struct {
	out       appclinico.DetalheEpisodio
	err       error
	actor     string
	chegadaID string
}

func (f *fakeIniciarConsulta) Executar(_ context.Context, actor, chegadaID string) (appclinico.DetalheEpisodio, error) {
	f.actor, f.chegadaID = actor, chegadaID
	return f.out, f.err
}

func routerConsulta(sessao dominio.Sessao, f *fakeIniciarConsulta) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoClinicoConsultaHandler(f)
	adhttp.RegistarClinicoConsulta(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestIniciarConsultaHTTP_Medico_201(t *testing.T) {
	f := &fakeIniciarConsulta{out: appclinico.DetalheEpisodio{ID: "ep-1", Estado: "ABERTO", Tipo: "CONSULTA"}}
	r := routerConsulta(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, f)
	w := pedidoCorpo(r, "POST", "/api/v1/chegadas/c1/iniciar-consulta", ``)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	if f.actor != "m1" || f.chegadaID != "c1" {
		t.Fatalf("actor/chegada mal passados ao caso de uso: %q %q", f.actor, f.chegadaID)
	}
}

func TestIniciarConsultaHTTP_Enfermeiro_403(t *testing.T) {
	f := &fakeIniciarConsulta{}
	r := routerConsulta(dominio.Sessao{Sujeito: "e1", Papeis: []dominio.Papel{dominio.PapelEnfermeiro}}, f)
	w := pedidoCorpo(r, "POST", "/api/v1/chegadas/c1/iniciar-consulta", ``)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("enfermeiro devia receber 403, veio %d", w.Code)
	}
}

func TestIniciarConsultaHTTP_Administrativo_403(t *testing.T) {
	f := &fakeIniciarConsulta{}
	r := routerConsulta(dominio.Sessao{Sujeito: "a1", Papeis: []dominio.Papel{dominio.PapelAdministrativo}}, f)
	w := pedidoCorpo(r, "POST", "/api/v1/chegadas/c1/iniciar-consulta", ``)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("administrativo devia receber 403, veio %d", w.Code)
	}
}

func TestIniciarConsultaHTTP_ErrosPropagados(t *testing.T) {
	casos := []struct {
		nome   string
		err    error
		codigo int
	}{
		{"chegada não encontrada", erros.Novo(erros.CategoriaNaoEncontrado, "chegada triada não encontrada"), nethttp.StatusNotFound},
		{"médico não atribuído", erros.Novo(erros.CategoriaProibido, "só o médico atribuído pode iniciar a consulta"), nethttp.StatusForbidden},
		{"chegada já consumida", erros.Novo(erros.CategoriaConflito, "o estado da chegada mudou entretanto; recarregue e repita a operação"), nethttp.StatusConflict},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			f := &fakeIniciarConsulta{err: caso.err}
			r := routerConsulta(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, f)
			w := pedidoCorpo(r, "POST", "/api/v1/chegadas/c1/iniciar-consulta", ``)
			if w.Code != caso.codigo {
				t.Fatalf("esperava %d, veio %d (%s)", caso.codigo, w.Code, w.Body.String())
			}
		})
	}
}
```

- [ ] **Step 2: Correr os testes e confirmar que falham**

Run: `go test ./internal/adapters/http/ -run IniciarConsultaHTTP -v`
Expected: FAIL — compilação (`adhttp.NovoClinicoConsultaHandler undefined`).

- [ ] **Step 3: Implementar o handler**

Criar `internal/adapters/http/clinico_consulta_handler.go`:

```go
// internal/adapters/http/clinico_consulta_handler.go
//
// Package http (adaptadores) — este ficheiro expõe o início da consulta
// (integração Recepção→Clínico, ADR-036). Handler separado para manter os
// construtores enxutos: a rota vive no grupo das chegadas, mas o caso de uso é
// do BC Clínico.
package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

// ServicoIniciarConsulta consome uma chegada TRIADO e abre o episódio CONSULTA.
type ServicoIniciarConsulta interface {
	Executar(ctx context.Context, actor, chegadaID string) (appclinico.DetalheEpisodio, error)
}

// ClinicoConsultaHandler expõe o endpoint HTTP do início da consulta.
type ClinicoConsultaHandler struct {
	iniciar ServicoIniciarConsulta
}

// NovoClinicoConsultaHandler constrói o handler.
func NovoClinicoConsultaHandler(iniciar ServicoIniciarConsulta) *ClinicoConsultaHandler {
	return &ClinicoConsultaHandler{iniciar: iniciar}
}

// RegistarClinicoConsulta regista a rota do início da consulta. Só Médico:
// iniciar a consulta é acto do médico atribuído (a guarda de dono corre no
// domínio e no CAS); o Enfermeiro pode iniciar episódios genéricos, mas não
// consumir a fila clínica.
func RegistarClinicoConsulta(r gin.IRouter, h *ClinicoConsultaHandler, protecao ...gin.HandlerFunc) {
	soMedico := RBAC(dominio.PapelMedico)

	gc := r.Group("/api/v1/chegadas")
	gc.Use(protecao...)
	gc.POST("/:cid/iniciar-consulta", soMedico, h.iniciarConsultaHTTP)
}

// iniciarConsultaHTTP não tem corpo: tudo vem da chegada e da sessão.
func (h *ClinicoConsultaHandler) iniciarConsultaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.iniciar.Executar(c.Request.Context(), actor.Sujeito, c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}
```

- [ ] **Step 4: Correr os testes e confirmar que passam**

Run: `go test ./internal/adapters/http/`
Expected: PASS (novos e pré-existentes — atenção a colisões de rota gin: o grupo `/api/v1/chegadas` já existe noutros handlers com o mesmo nome de parâmetro `:cid`, pelo que não há pânico de rota).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/clinico_consulta_handler.go internal/adapters/http/clinico_consulta_test.go
git commit -m "feat(http): POST /chegadas/:cid/iniciar-consulta, só Médico (ADR-036)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Composition root + verificação global

**Files:**
- Modify: `internal/platform/app.go` (duas inserções)

**Interfaces:**
- Consumes: `pgrepo.NovaIntegracaoInicioConsulta` (Task 4), `appclinico.NovoCasoIniciarConsulta` (Task 3), `adhttp.NovoClinicoConsultaHandler`/`adhttp.RegistarClinicoConsulta` (Task 5), e os já existentes `repoDoentes`, `repoEpisodios`, `repoAuditoria`, `limiteMW`, `authMW`.
- Produces: aplicação completa com a rota ligada.

- [ ] **Step 1: Ligar no composition root**

Em `internal/platform/app.go`, logo a seguir ao bloco do `handlerRecepcaoTriagem` (que termina perto da linha 232), acrescentar:

```go
	// Integração Recepção→Clínico — início da consulta (ADR-036). O adaptador de
	// integração implementa as duas portas (leitor + consumidor transaccional).
	integracaoConsulta := pgrepo.NovaIntegracaoInicioConsulta(pool)
	handlerClinicoConsulta := adhttp.NovoClinicoConsultaHandler(
		appclinico.NovoCasoIniciarConsulta(integracaoConsulta, integracaoConsulta,
			repoDoentes, repoEpisodios, repoAuditoria),
	)
```

E no bloco de registo de rotas, logo a seguir a `adhttp.RegistarRecepcaoTriagem(r, handlerRecepcaoTriagem, limiteMW, authMW)`:

```go
		adhttp.RegistarClinicoConsulta(r, handlerClinicoConsulta, limiteMW, authMW)
```

- [ ] **Step 2: Verificação global**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: build OK, vet limpo, todos os testes unitários PASS.

Run: `golangci-lint run ./...`
Expected: 0 issues. (Instalado localmente: v2.5.0; se falhar com "built with go1.24", reinstalar com `GOTOOLCHAIN=go1.25.0 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0`.)

Run (se `DATABASE_URL` definido): `go test -tags integration ./tests/integration/`
Expected: PASS completo.

Run (gates de cobertura das camadas tocadas):
`go test -cover ./internal/domain/recepcao/ ./internal/application/clinico/ ./internal/adapters/http/`
Expected: recepcao ≥85%, application/clinico ≥75%, adapters/http ≥60%.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/app.go
git commit -m "feat(plataforma): liga o início da consulta no composition root (ADR-036)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: ADR-036, SPRINT.md e CLAUDE.md

**Files:**
- Create: `adrs/ADR-036-integracao-inicio-consulta.md`
- Modify: `SPRINT.md` (nova secção de critérios de saída)
- Modify: `CLAUDE.md` (marco, lista de ADRs, próximo ADR)

**Interfaces:**
- Consumes: nada de código — documenta as Tasks 1–6.
- Produces: registo formal da decisão; docs mestres actualizados.

- [ ] **Step 1: Redigir a ADR-036**

Criar `adrs/ADR-036-integracao-inicio-consulta.md`:

```markdown
# ADR-036 — Integração Recepção→Clínico: início da consulta

- **Estado:** Aceite
- **Data:** 2026-07-16
- **Marco/Sprint:** Integração Percurso Ambulatório → Clínico (fecha o diferimento da ADR-034)
- **Fontes:** design em `docs/superpowers/specs/2026-07-16-integracao-inicio-consulta-design.md`; ADR-034 (Triagem), ADR-027 (Episódio Clínico).

## Contexto

A Triagem (ADR-034) deixou o doente na fila clínica com a `Chegada` em `TRIADO` —
um estado terminal: o doente nunca saía da fila e o `EpisodioClinico` nascia por um
endpoint desligado do percurso. Faltava a integração diferida: consumir a chegada
TRIADO e criar o episódio. É a primeira escrita cross-BC do sistema (as ACLs
existentes — Lab→Clínico, Recepção→Clínico — só leem).

## Decisão

1. **Novo estado `EM_CONSULTA` na `Chegada`** (`IniciarConsulta`, TRIADO→EM_CONSULTA),
   com **guarda de dono no domínio**: só o médico atribuído pode iniciar
   (`CategoriaProibido` → 403). A chegada regista o `episodio_id` que a consumiu
   (uuid **sem FK** — cross-context); o episódio não ganha nenhuma coluna.
2. **Caso de uso no BC Clínico** (`CasoIniciarConsulta`): é o Clínico que consome a
   chegada, por portas ACL com DTOs próprios (`LeitorRecepcao`, `ConsumidorChegadas`)
   — a aplicação do Clínico não importa o domínio da Recepção. Tipo fixo `CONSULTA`;
   médico do episódio = actor autenticado.
3. **Escrita cross-BC síncrona por transacção única num adaptador de integração**
   (Camada 3, importa ambos os domínios): `INSERT` do episódio + `UPDATE` CAS da
   chegada (estado + médico) numa só transacção PG. As regras correm no domínio da
   Recepção dentro da transacção (releitura em tx → `IniciarConsulta`); o CAS fecha
   a corrida (0 linhas → 409). Rejeitados: orquestração com compensação (janelas de
   inconsistência, episódio órfão) e Outbox assíncrono (acção interactiva — o médico
   espera o episódio na resposta; o Outbox continua por implementar).
   **Critério para o futuro:** escrita cross-BC iniciada por acção interactiva e na
   mesma BD → transacção única no adaptador; propagação de factos consumados entre
   BCs → eventos via Outbox quando existir.
4. **Defesa em profundidade na BD** (migração `recepcao/0004`): `CHECK` de estado
   com `EM_CONSULTA`, `CHECK (estado <> 'EM_CONSULTA' OR episodio_id IS NOT NULL)`
   e `UNIQUE` parcial sobre `episodio_id` (1:1 chegada↔episódio).
5. **HTTP/RBAC:** `POST /api/v1/chegadas/:cid/iniciar-consulta` → 201 com o
   episódio; papel **Médico** apenas. Auditoria dupla: `clinico.episodio.aberto` +
   `recepcao.chegada.consulta_iniciada`.

## Consequências

- O percurso ambulatório fica ligado de ponta a ponta: marcação → check-in →
  triagem → consulta (episódio ABERTO). A chegada EM_CONSULTA sai da fila clínica
  (o read-model filtra TRIADO).
- `EM_CONSULTA` é terminal na Recepção: o ciclo de vida continua no episódio
  (fechar/cancelar, ADR-027). A Recepção saberá do desfecho por eventos, quando o
  Outbox existir.
- Diferimentos: sinais vitais da triagem no EHR; evento de integração via Outbox;
  desfazer o início da consulta.
```

- [ ] **Step 2: Actualizar o SPRINT.md**

Em `SPRINT.md`, inserir imediatamente a seguir à secção `## Critérios de saída M3 — Laboratório` (depois do último item dessa secção):

```markdown
## Critérios de saída — Integração Início da Consulta (ADR-036)

- [x] O médico atribuído inicia a consulta a partir da fila e recebe o episódio
      ABERTO (tipo CONSULTA) na resposta (201).
- [x] A chegada transita TRIADO→EM_CONSULTA, sai da fila clínica e regista o
      episodio_id que a consumiu (uuid sem FK cross-context).
- [x] Transição + criação atómicas (transacção única no adaptador de integração):
      nunca existe episódio sem chegada consumida nem chegada consumida sem episódio.
- [x] Só o médico atribuído pode iniciar (403 no domínio e no CAS); duplo
      início/corrida → 409; zero colunas novas no BC Clínico.
- [x] 1:1 chegada↔episódio garantido por UNIQUE parcial + CHECK (migração
      recepcao/0004), provado em integração (23505/23514).
- [x] Comando auditado nos dois contextos; cobertura nos limiares.
```

(Também actualizar o cabeçalho/estado do marco no topo do `SPRINT.md`, se existir referência ao "início da consulta" como pendente — marcar como entregue.)

- [ ] **Step 3: Actualizar o CLAUDE.md**

Em `CLAUDE.md`:

3a. Na secção `## 6. Marco Actual`, substituir:

```
**Marco Percurso Ambulatório** (entregue, a par do M3): o percurso do doente antes da
consulta. Sub-projectos: **Marcação** (ADR-032), **Check-in** (ADR-033) e **Triagem** (BC
`recepcao` — prioridade Manchester, sinais vitais, fila clínica; ver ADR-034). O início da
consulta (Chegada TRIADO → Episódio no BC Clínico) fica para integração futura.
```

por:

```
**Marco Percurso Ambulatório** (entregue, a par do M3): o percurso do doente antes da
consulta. Sub-projectos: **Marcação** (ADR-032), **Check-in** (ADR-033) e **Triagem** (BC
`recepcao` — prioridade Manchester, sinais vitais, fila clínica; ver ADR-034). O início da
consulta (Chegada TRIADO → Episódio no BC Clínico) foi entregue como **Integração
Recepção→Clínico** (ADR-036): transacção única no adaptador de integração, estado
EM_CONSULTA, só o médico atribuído.
```

3b. Na lista de ADRs do rodapé, acrescentar após `adrs/ADR-035-laboratorio-validacao-correccao.md`:

```
`adrs/ADR-036-integracao-inicio-consulta.md`.
```

(ajustar a pontuação da linha anterior de `.` para `,`)

3c. Substituir `Próximo ADR: **ADR-036**.` por `Próximo ADR: **ADR-037**.`

- [ ] **Step 4: Commit**

```bash
git add adrs/ADR-036-integracao-inicio-consulta.md SPRINT.md CLAUDE.md
git commit -m "docs(integracao): ADR-036 e fecho da integração início da consulta

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Verificação final (após todas as tasks)

1. `go build ./... && go vet ./... && go test ./...` — tudo verde.
2. `golangci-lint run ./...` — 0 issues.
3. Com Postgres: `go test -tags integration ./tests/integration/` — tudo verde.
4. `git log --oneline -8` — 6 commits da entrega + histórico limpo em `main` (ou branch, conforme o fluxo escolhido na execução).
