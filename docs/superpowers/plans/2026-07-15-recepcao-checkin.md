# Check-in do BC Recepção — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar o Check-in do marco Percurso Ambulatório — a chegada do doente (com marcação ou walk-in), a transição da marcação para `COMPARECEU`, e a fila de espera.

**Architecture:** Estende o BC `recepcao` existente com um novo agregado `Chegada` (unifica check-in de marcação e walk-in) e um estado `COMPARECEU` na `Marcacao`. O check-in de uma marcação é transaccional e cruza os dois agregados (transita a marcação + insere a chegada, com guarda compare-and-set). A fila é um read-model das chegadas em `AGUARDA`. Reutiliza a ACL `LeitorDoente`, a auditoria e os padrões CAS já no BC.

**Tech Stack:** Go 1.22+, Gin, pgx v5 (SQL puro), PostgreSQL 16, testes com fakes + integração `//go:build integration`.

## Global Constraints

- **Idioma:** PT-PT angolano em TODO o output (código, comentários, commits, mensagens, JSON). Nunca EN/BR.
- **Module path:** `github.com/ivandrosilva12/sgcfinal`.
- **Sem FK cross-context.** `doente_id`/`medico_id`/`especialidade_id` são uuid sem FK. Única FK: `chegadas.marcacao_id → recepcao.marcacoes(id)` (interna ao schema).
- **Domínio sem infra:** `internal/domain/**` nunca importa `pgx`/`gin`/`net/http`.
- **Camada de Aplicação** importa só o próprio Domínio (+ shared). Nunca outro BC, nunca infra.
- **Migrations forward-only**, sem `.down.sql`.
- **Nada de `panic()`** — sempre `error` via `internal/domain/shared/erros`.
- **Actor = sujeito autenticado** (`SessaoDe(c).Sujeito`), nunca do corpo.
- **Auditoria append-only** em todos os comandos; leituras não auditadas.
- **Cobertura:** domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
- **Guarda compare-and-set:** transições usam `EstadoAnterior` (só fixado por `Reconstruir…`), nunca o estado novo.
- **Categorias de erro:** `CategoriaValidacao`(400/422), `CategoriaConflito`(409), `CategoriaRegraNegocio`(422), `CategoriaNaoEncontrado`(404), `CategoriaProibido`(403).
- **Registo de auditoria** (`auditoria.Registo`): `Actor, Accao, Entidade, EntidadeID, OcorridoEm, Detalhe`.
- **Convenção de testes de integração:** vivem em `tests/integration/` (package `integration_test`, `//go:build integration`), usam o helper `ligar(t)` (SKIP sem `DATABASE_URL`) e `db.AplicarMigracoes(ctx, pool, migrations.FS, logger)`. NÃO usar helpers inexistentes.
- **BD de desenvolvimento disponível:** `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable'` (container `sgc-postgres-1`).

---

## Contratos partilhados (definidos ao longo do plano)

**Domínio (`internal/domain/recepcao`):**
- `MarcCompareceu EstadoMarcacao = "COMPARECEU"` (Task 1); `func (m *Marcacao) RegistarComparencia(em time.Time) error` (MARCADA→COMPARECEU).
- `type EstadoChegada string`; `ChegAguarda="AGUARDA"`, `ChegChamado="CHAMADO"`, `ChegDesistiu="DESISTIU"` (Task 2).
- `NovaChegadaAgendada(doenteID, marcacaoID, medicoID, especialidadeID string, hora time.Time) (*Chegada, error)`; `NovaChegadaWalkIn(doenteID, especialidadeID string, hora time.Time) (*Chegada, error)`; `Chamar/RegistarDesistencia(em) error`; getters `ID/DoenteID/MarcacaoID/MedicoID/EspecialidadeID/HoraChegada/Estado`; `SnapshotChegada`; `Snapshot()`; `ReconstruirChegada` (Task 2).
- `type ResumoChegada struct{ ID, DoenteID, MarcacaoID, MedicoID, EspecialidadeID, Estado string; HoraChegada time.Time }`; `type RepositorioChegadas interface` (Task 3).

**Aplicação (`internal/application/recepcao`):**
- DTOs `DadosWalkIn`, `DetalheChegada`; reexport `ResumoChegada`; `paraDetalheChegada` (Task 3).
- `NovoCasoRegistarWalkIn`, `NovoCasoChamar`, `NovoCasoRegistarDesistencia`, `NovoCasoListarFila` (Task 3); `NovoCasoRegistarChegada` (Task 4).

**Adaptadores:** `NovoRepositorioChegadas` (Task 6); `NovoRecepcaoChegadasHandler` + `RegistarRecepcaoChegadas` (Task 7).

---

## Task 1: Domínio — estado `COMPARECEU` na `Marcacao`

**Files:**
- Modify: `internal/domain/recepcao/marcacao.go`
- Test: `internal/domain/recepcao/marcacao_test.go`

**Interfaces:**
- Produces: `MarcCompareceu EstadoMarcacao = "COMPARECEU"`; `func (m *Marcacao) RegistarComparencia(em time.Time) error`.

- [ ] **Step 1: Write the failing test**

Acrescenta a `internal/domain/recepcao/marcacao_test.go`:

```go
func TestRegistarComparencia_DeMarcada(t *testing.T) {
	m := novaMarcacaoValida(t)
	if err := m.RegistarComparencia(inst("09:00")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if m.Estado() != recepcao.MarcCompareceu {
		t.Fatalf("esperava COMPARECEU, veio %s", m.Estado())
	}
}

func TestRegistarComparencia_NaoMarcada_Conflito(t *testing.T) {
	m := novaMarcacaoValida(t)
	_ = m.Cancelar("motivo", inst("08:00"))
	if err := m.RegistarComparencia(inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestTransicoes_RecusamAPartirDeCompareceu(t *testing.T) {
	// Depois de comparecer, a marcação já não pode ser cancelada, remarcada nem dar falta.
	base := novaMarcacaoValida(t)
	base = recepcao.ReconstruirMarcacao(comID(base.Snapshot(), "marc-1"))
	_ = base.RegistarComparencia(inst("09:00"))

	if err := base.Cancelar("x", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("Cancelar a partir de COMPARECEU devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
	if _, err := base.Remarcar(inst("10:00"), inst("10:30"), inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("Remarcar a partir de COMPARECEU devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
	if err := base.RegistarFalta(inst("12:00")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("RegistarFalta a partir de COMPARECEU devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/recepcao/... -run Comparencia`
Expected: FAIL — `undefined: recepcao.MarcCompareceu`.

- [ ] **Step 3: Add the state constant**

Em `internal/domain/recepcao/marcacao.go`, no bloco `const`, acrescenta `MarcCompareceu` e actualiza o comentário do enum:

```go
// EstadoMarcacao é o estado do ciclo de vida de uma marcação.
//
//	MARCADA ─┬─ Cancelar ──────────► CANCELADA
//	         ├─ Remarcar ──────────► REMARCADA  (+ nova Marcacao MARCADA)
//	         ├─ RegistarFalta ─────► FALTOU
//	         └─ RegistarComparencia► COMPARECEU (check-in do doente)
type EstadoMarcacao string

const (
	MarcMarcada    EstadoMarcacao = "MARCADA"
	MarcCancelada  EstadoMarcacao = "CANCELADA"
	MarcRemarcada  EstadoMarcacao = "REMARCADA"
	MarcFaltou     EstadoMarcacao = "FALTOU"
	MarcCompareceu EstadoMarcacao = "COMPARECEU"
)
```

- [ ] **Step 4: Add the method**

Em `internal/domain/recepcao/marcacao.go`, a seguir a `RegistarFalta`:

```go
// RegistarComparencia transita MARCADA → COMPARECEU (o doente chegou e fez check-in).
// Desfecho simétrico ao FALTOU: depois de comparecer, a marcação já não pode ser
// cancelada, remarcada nem dar falta (essas transições continuam a exigir MARCADA).
func (m *Marcacao) RegistarComparencia(em time.Time) error {
	if m.estado != MarcMarcada {
		return erros.Novo(erros.CategoriaConflito, "só é possível registar a comparência de uma marcação em estado MARCADA")
	}
	m.estado = MarcCompareceu
	m.actualizadoEm = em
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/domain/recepcao/... -cover`
Expected: PASS, cobertura ≥85%.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/recepcao/marcacao.go internal/domain/recepcao/marcacao_test.go
git commit -m "feat(recepcao): estado COMPARECEU e RegistarComparencia na Marcacao"
```

---

## Task 2: Domínio — Agregado `Chegada`

**Files:**
- Create: `internal/domain/recepcao/chegada.go`
- Test: `internal/domain/recepcao/chegada_test.go`

**Interfaces:**
- Produces: `type EstadoChegada string` + consts `ChegAguarda/ChegChamado/ChegDesistiu`; `NovaChegadaAgendada(doenteID, marcacaoID, medicoID, especialidadeID string, hora time.Time) (*Chegada, error)`; `NovaChegadaWalkIn(doenteID, especialidadeID string, hora time.Time) (*Chegada, error)`; métodos `Chamar(em)/RegistarDesistencia(em) error`; getters `ID/DoenteID/MarcacaoID/MedicoID/EspecialidadeID/HoraChegada/Estado`; `type SnapshotChegada struct` (com `EstadoAnterior`); `Snapshot()`; `ReconstruirChegada(SnapshotChegada) *Chegada`.

- [ ] **Step 1: Write the failing test**

```go
// internal/domain/recepcao/chegada_test.go
package recepcao_test

import (
	"testing"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovaChegadaAgendada_NasceAguarda(t *testing.T) {
	c, err := recepcao.NovaChegadaAgendada("doe-1", "marc-1", "med-1", "esp-1", inst("09:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.Estado() != recepcao.ChegAguarda {
		t.Fatalf("esperava AGUARDA, veio %s", c.Estado())
	}
	if c.DoenteID() != "doe-1" || c.MarcacaoID() != "marc-1" || c.MedicoID() != "med-1" || c.EspecialidadeID() != "esp-1" {
		t.Fatal("campos mal preenchidos")
	}
}

func TestNovaChegadaAgendada_CamposObrigatorios(t *testing.T) {
	casos := []struct {
		nome                            string
		doente, marc, medico, esp string
	}{
		{"sem doente", "", "marc-1", "med-1", "esp-1"},
		{"sem marcacao", "doe-1", "", "med-1", "esp-1"},
		{"sem medico", "doe-1", "marc-1", "", "esp-1"},
		{"sem especialidade", "doe-1", "marc-1", "med-1", ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := recepcao.NovaChegadaAgendada(c.doente, c.marc, c.medico, c.esp, inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
				t.Fatalf("%s: esperava CategoriaValidacao, veio %v", c.nome, erros.CategoriaDe(err))
			}
		})
	}
}

func TestNovaChegadaWalkIn_SemMarcacaoNemMedico(t *testing.T) {
	c, err := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.Estado() != recepcao.ChegAguarda || c.MarcacaoID() != "" || c.MedicoID() != "" {
		t.Fatalf("walk-in mal construído: %+v", c.Snapshot())
	}
	if c.DoenteID() != "doe-1" || c.EspecialidadeID() != "esp-1" {
		t.Fatal("doente/especialidade mal preenchidos")
	}
}

func TestNovaChegadaWalkIn_CamposObrigatorios(t *testing.T) {
	if _, err := recepcao.NovaChegadaWalkIn("", "esp-1", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("sem doente: esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
	if _, err := recepcao.NovaChegadaWalkIn("doe-1", "", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("sem especialidade: esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestChamar_DeAguarda(t *testing.T) {
	c, _ := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	if err := c.Chamar(inst("09:10")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.Estado() != recepcao.ChegChamado {
		t.Fatalf("esperava CHAMADO, veio %s", c.Estado())
	}
}

func TestRegistarDesistencia_DeAguarda(t *testing.T) {
	c, _ := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	if err := c.RegistarDesistencia(inst("09:10")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.Estado() != recepcao.ChegDesistiu {
		t.Fatalf("esperava DESISTIU, veio %s", c.Estado())
	}
}

func TestChegada_TransicoesInvalidas_Conflito(t *testing.T) {
	c, _ := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	_ = c.Chamar(inst("09:10"))
	if err := c.Chamar(inst("09:20")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("chamar duas vezes devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
	if err := c.RegistarDesistencia(inst("09:20")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("desistir depois de chamado devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestChegada_SnapshotEstadoAnterior_NaoMutaAposTransicao(t *testing.T) {
	rehidratada := recepcao.ReconstruirChegada(recepcao.SnapshotChegada{
		ID: "cheg-1", DoenteID: "doe-1", EspecialidadeID: "esp-1",
		Estado: recepcao.ChegAguarda, HoraChegada: inst("09:00"),
	})
	if err := rehidratada.Chamar(inst("09:10")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if rehidratada.Snapshot().EstadoAnterior != recepcao.ChegAguarda {
		t.Fatalf("EstadoAnterior devia continuar AGUARDA, veio %s", rehidratada.Snapshot().EstadoAnterior)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/recepcao/... -run Chegada`
Expected: FAIL — `undefined: recepcao.NovaChegadaAgendada`.

- [ ] **Step 3: Write the aggregate**

```go
// internal/domain/recepcao/chegada.go
package recepcao

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// EstadoChegada é o estado do ciclo de vida de uma chegada (o doente na fila).
//
//	AGUARDA ─┬─ Chamar ──────────► CHAMADO   (entrega à triagem/consulta)
//	         └─ RegistarDesistencia► DESISTIU
type EstadoChegada string

const (
	ChegAguarda  EstadoChegada = "AGUARDA"
	ChegChamado  EstadoChegada = "CHAMADO"
	ChegDesistiu EstadoChegada = "DESISTIU"
)

// Chegada é um agregado raiz do BC Recepção: o doente presente na clínica hoje, à
// espera de ser atendido. Nasce de um check-in de marcação (com marcacaoID e médico) ou
// de um walk-in (sem marcação nem médico). Refere doente/marcação/médico/especialidade
// por id. O id é gerado pela base de dados.
type Chegada struct {
	id              string
	doenteID        string
	marcacaoID      string
	especialidadeID string
	medicoID        string
	horaChegada     time.Time
	estado          EstadoChegada
	estadoAnterior  EstadoChegada
	criadoEm        time.Time
	actualizadoEm   time.Time
}

// NovaChegadaAgendada constrói a chegada de um doente com marcação (check-in). Doente,
// marcação, médico, especialidade e hora são todos obrigatórios (o médico e a
// especialidade vêm da marcação). Estado inicial AGUARDA.
func NovaChegadaAgendada(doenteID, marcacaoID, medicoID, especialidadeID string, hora time.Time) (*Chegada, error) {
	doenteID = strings.TrimSpace(doenteID)
	if doenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente da chegada em falta")
	}
	marcacaoID = strings.TrimSpace(marcacaoID)
	if marcacaoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "marcação da chegada em falta")
	}
	medicoID = strings.TrimSpace(medicoID)
	if medicoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "médico da chegada em falta")
	}
	especialidadeID = strings.TrimSpace(especialidadeID)
	if especialidadeID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "especialidade da chegada em falta")
	}
	if hora.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "hora da chegada em falta")
	}
	return &Chegada{
		doenteID: doenteID, marcacaoID: marcacaoID, medicoID: medicoID,
		especialidadeID: especialidadeID, horaChegada: hora, estado: ChegAguarda,
	}, nil
}

// NovaChegadaWalkIn constrói a chegada de um doente sem marcação (walk-in). Só o doente,
// a especialidade e a hora são obrigatórios; o médico fica por atribuir. Estado inicial
// AGUARDA.
func NovaChegadaWalkIn(doenteID, especialidadeID string, hora time.Time) (*Chegada, error) {
	doenteID = strings.TrimSpace(doenteID)
	if doenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente da chegada em falta")
	}
	especialidadeID = strings.TrimSpace(especialidadeID)
	if especialidadeID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "especialidade da chegada em falta")
	}
	if hora.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "hora da chegada em falta")
	}
	return &Chegada{
		doenteID: doenteID, especialidadeID: especialidadeID, horaChegada: hora,
		estado: ChegAguarda,
	}, nil
}

// Chamar transita AGUARDA → CHAMADO (o doente é chamado para ser atendido).
func (c *Chegada) Chamar(em time.Time) error {
	if c.estado != ChegAguarda {
		return erros.Novo(erros.CategoriaConflito, "só é possível chamar uma chegada em espera")
	}
	c.estado = ChegChamado
	c.actualizadoEm = em
	return nil
}

// RegistarDesistencia transita AGUARDA → DESISTIU (o doente foi embora antes de ser
// chamado).
func (c *Chegada) RegistarDesistencia(em time.Time) error {
	if c.estado != ChegAguarda {
		return erros.Novo(erros.CategoriaConflito, "só é possível registar a desistência de uma chegada em espera")
	}
	c.estado = ChegDesistiu
	c.actualizadoEm = em
	return nil
}

// ID devolve o identificador atribuído pela base de dados.
func (c *Chegada) ID() string { return c.id }

// DoenteID devolve o doente da chegada.
func (c *Chegada) DoenteID() string { return c.doenteID }

// MarcacaoID devolve a marcação de origem (vazio no walk-in).
func (c *Chegada) MarcacaoID() string { return c.marcacaoID }

// MedicoID devolve o médico (vazio no walk-in).
func (c *Chegada) MedicoID() string { return c.medicoID }

// EspecialidadeID devolve a especialidade da chegada.
func (c *Chegada) EspecialidadeID() string { return c.especialidadeID }

// HoraChegada devolve o instante da chegada.
func (c *Chegada) HoraChegada() time.Time { return c.horaChegada }

// Estado devolve o estado actual.
func (c *Chegada) Estado() EstadoChegada { return c.estado }

// SnapshotChegada carrega o estado completo para persistência ou rehidratação.
//
// EstadoAnterior é o estado lido da base de dados (vazio num agregado novo); o
// repositório usa-o como guarda compare-and-set. É derivado — quem reconstrói não o
// preenche.
type SnapshotChegada struct {
	ID              string
	DoenteID        string
	MarcacaoID      string
	EspecialidadeID string
	MedicoID        string
	HoraChegada     time.Time
	Estado          EstadoChegada
	EstadoAnterior  EstadoChegada
	CriadoEm        time.Time
	ActualizadoEm   time.Time
}

// Snapshot devolve o estado completo do agregado.
func (c *Chegada) Snapshot() SnapshotChegada {
	return SnapshotChegada{
		ID: c.id, DoenteID: c.doenteID, MarcacaoID: c.marcacaoID,
		EspecialidadeID: c.especialidadeID, MedicoID: c.medicoID, HoraChegada: c.horaChegada,
		Estado: c.estado, EstadoAnterior: c.estadoAnterior,
		CriadoEm: c.criadoEm, ActualizadoEm: c.actualizadoEm,
	}
}

// ReconstruirChegada reconstrói o agregado a partir de um snapshot persistido.
// EstadoAnterior é fixado no estado lido.
func ReconstruirChegada(s SnapshotChegada) *Chegada {
	return &Chegada{
		id: s.ID, doenteID: s.DoenteID, marcacaoID: s.MarcacaoID,
		especialidadeID: s.EspecialidadeID, medicoID: s.MedicoID, horaChegada: s.HoraChegada,
		estado: s.Estado, estadoAnterior: s.Estado,
		criadoEm: s.CriadoEm, actualizadoEm: s.ActualizadoEm,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/recepcao/... -cover`
Expected: PASS, cobertura ≥85%.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/recepcao/chegada.go internal/domain/recepcao/chegada_test.go
git commit -m "feat(recepcao): agregado Chegada com maquina de estados AGUARDA/CHAMADO/DESISTIU"
```

---

## Task 3: Aplicação — port de chegadas + casos de walk-in, chamar, desistir e fila

**Files:**
- Modify: `internal/domain/recepcao/repositorio.go` (adicionar `ResumoChegada` + `RepositorioChegadas`)
- Modify: `internal/application/recepcao/ports.go` (adicionar DTOs + `paraDetalheChegada`)
- Create: `internal/application/recepcao/chegadas.go`
- Modify: `internal/application/recepcao/fakes_test.go` (adicionar `fakeChegadas`)
- Test: `internal/application/recepcao/chegadas_test.go`

**Interfaces:**
- Consumes: `LeitorDoente`, `Auditor` (existentes); agregado `Chegada` (Task 2).
- Produces:
  - Domínio: `type ResumoChegada struct{ ID, DoenteID, MarcacaoID, MedicoID, EspecialidadeID, Estado string; HoraChegada time.Time }`; `type RepositorioChegadas interface { Guardar(ctx, *Chegada) (string, error); RegistarChegadaAgendada(ctx, chegada *Chegada, marcacao *Marcacao) (string, error); ObterPorID(ctx, id string) (*Chegada, error); Transitar(ctx, *Chegada) error; ListarFila(ctx, especialidadeID string) ([]ResumoChegada, error) }`.
  - Aplicação: `type DadosWalkIn struct{ DoenteID, EspecialidadeID string }`; `type DetalheChegada struct{...}`; reexport `ResumoChegada`; `NovoCasoRegistarWalkIn(RepositorioChegadas, LeitorDoente, Auditor)` (`Executar(ctx, actor string, dados DadosWalkIn) (DetalheChegada, error)`); `NovoCasoChamar(RepositorioChegadas, Auditor)` (`Executar(ctx, actor, chegadaID string) (DetalheChegada, error)`); `NovoCasoRegistarDesistencia(RepositorioChegadas, Auditor)` (`Executar(ctx, actor, chegadaID string) (DetalheChegada, error)`); `NovoCasoListarFila(RepositorioChegadas)` (`Executar(ctx, especialidadeID string) ([]ResumoChegada, error)`).

- [ ] **Step 1: Add the domain read-model + repository interface**

Acrescenta a `internal/domain/recepcao/repositorio.go` (a seguir a `ResumoMarcacao`):

```go
// ResumoChegada é a projecção de leitura de uma chegada (linha da fila).
type ResumoChegada struct {
	ID              string    `json:"id"`
	DoenteID        string    `json:"doente_id"`
	MarcacaoID      string    `json:"marcacao_id,omitempty"`
	MedicoID        string    `json:"medico_id,omitempty"`
	EspecialidadeID string    `json:"especialidade_id"`
	Estado          string    `json:"estado"`
	HoraChegada     time.Time `json:"hora_chegada"`
}
```

E, no fim do ficheiro, a interface:

```go
// RepositorioChegadas é a porta de saída de persistência de chegadas.
//
// RegistarChegadaAgendada grava, numa única transacção, a marcação a passar a
// COMPARECEU (guarda compare-and-set sobre MARCADA) e a nova chegada — um check-in que
// transitasse a marcação sem criar a chegada (ou vice-versa) deixaria a recepção
// incoerente. Transitar aplica a transição de estado da chegada (CAS). ListarFila
// devolve as chegadas em AGUARDA (fila), ordenadas por hora de chegada; especialidade
// vazia = todas.
type RepositorioChegadas interface {
	Guardar(ctx context.Context, c *Chegada) (string, error)
	RegistarChegadaAgendada(ctx context.Context, chegada *Chegada, marcacao *Marcacao) (string, error)
	ObterPorID(ctx context.Context, id string) (*Chegada, error)
	Transitar(ctx context.Context, c *Chegada) error
	ListarFila(ctx context.Context, especialidadeID string) ([]ResumoChegada, error)
}
```

- [ ] **Step 2: Add the application DTOs + mapper**

Acrescenta a `internal/application/recepcao/ports.go`. No bloco de reexports:

```go
// Reexports dos read-models do domínio.
type (
	ResumoMarcacao = dominio.ResumoMarcacao
	ResumoChegada  = dominio.ResumoChegada
)
```

E no fim do ficheiro:

```go
// DadosWalkIn é a entrada de um walk-in (doente sem marcação). O actor vem da sessão.
type DadosWalkIn struct {
	DoenteID        string `json:"doente_id"`
	EspecialidadeID string `json:"especialidade_id"`
}

// DetalheChegada é o detalhe de uma chegada numa resposta.
type DetalheChegada struct {
	ID              string    `json:"id"`
	DoenteID        string    `json:"doente_id"`
	MarcacaoID      string    `json:"marcacao_id,omitempty"`
	MedicoID        string    `json:"medico_id,omitempty"`
	EspecialidadeID string    `json:"especialidade_id"`
	Estado          string    `json:"estado"`
	HoraChegada     time.Time `json:"hora_chegada"`
}

// paraDetalheChegada projecta o agregado para o read-model de resposta.
func paraDetalheChegada(c *dominio.Chegada) DetalheChegada {
	s := c.Snapshot()
	return DetalheChegada{
		ID: s.ID, DoenteID: s.DoenteID, MarcacaoID: s.MarcacaoID, MedicoID: s.MedicoID,
		EspecialidadeID: s.EspecialidadeID, Estado: string(s.Estado), HoraChegada: s.HoraChegada,
	}
}
```

- [ ] **Step 3: Add the fake to fakes_test.go**

Acrescenta a `internal/application/recepcao/fakes_test.go` um `fakeChegadas`. Reutiliza o `itoa` e o `agoraFixo` já existentes no ficheiro:

```go
// fakeChegadas guarda chegadas em memória. RegistarChegadaAgendada também transita a
// marcação no fakeMarcacoes injectado (coordenação cross-agregado).
type fakeChegadas struct {
	dados     map[string]*dominio.Chegada
	seq       int
	marcacoes *fakeMarcacoes // para a coordenação transaccional
}

func novoFakeChegadas(m *fakeMarcacoes) *fakeChegadas {
	return &fakeChegadas{dados: map[string]*dominio.Chegada{}, marcacoes: m}
}

func (f *fakeChegadas) Guardar(_ context.Context, c *dominio.Chegada) (string, error) {
	f.seq++
	id := "cheg-" + itoa(f.seq)
	s := c.Snapshot()
	s.ID = id
	f.dados[id] = dominio.ReconstruirChegada(s)
	return id, nil
}

func (f *fakeChegadas) RegistarChegadaAgendada(ctx context.Context, chegada *dominio.Chegada, marcacao *dominio.Marcacao) (string, error) {
	// Transita a marcação (guarda CAS via o fakeMarcacoes) e só depois grava a chegada.
	if err := f.marcacoes.Transitar(ctx, marcacao); err != nil {
		return "", err
	}
	return f.Guardar(ctx, chegada)
}

func (f *fakeChegadas) ObterPorID(_ context.Context, id string) (*dominio.Chegada, error) {
	c, ok := f.dados[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "chegada não encontrada")
	}
	return dominio.ReconstruirChegada(c.Snapshot()), nil
}

func (f *fakeChegadas) Transitar(_ context.Context, c *dominio.Chegada) error {
	s := c.Snapshot()
	cur, ok := f.dados[s.ID]
	if !ok {
		return erros.Novo(erros.CategoriaNaoEncontrado, "chegada não encontrada")
	}
	if cur.Estado() != s.EstadoAnterior {
		return erros.Novo(erros.CategoriaConflito, "o estado da chegada mudou entretanto")
	}
	f.dados[s.ID] = dominio.ReconstruirChegada(s)
	return nil
}

func (f *fakeChegadas) ListarFila(_ context.Context, especialidadeID string) ([]dominio.ResumoChegada, error) {
	var out []dominio.ResumoChegada
	for _, c := range f.dados {
		s := c.Snapshot()
		if s.Estado != dominio.ChegAguarda {
			continue
		}
		if especialidadeID != "" && s.EspecialidadeID != especialidadeID {
			continue
		}
		out = append(out, dominio.ResumoChegada{
			ID: s.ID, DoenteID: s.DoenteID, MarcacaoID: s.MarcacaoID, MedicoID: s.MedicoID,
			EspecialidadeID: s.EspecialidadeID, Estado: string(s.Estado), HoraChegada: s.HoraChegada,
		})
	}
	return out, nil
}

var _ dominio.RepositorioChegadas = (*fakeChegadas)(nil)
```

- [ ] **Step 4: Write the failing test**

```go
// internal/application/recepcao/chegadas_test.go
package recepcao_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRegistarWalkIn_CriaEAudita(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	aud := &fakeAuditor{}
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	uc := app.NovoCasoRegistarWalkIn(chegadas, leitor, aud)

	out, err := uc.Executar(context.Background(), "adm-1", app.DadosWalkIn{DoenteID: "doe-1", EspecialidadeID: "esp-1"})
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.ID == "" || out.Estado != string(dominio.ChegAguarda) || out.MarcacaoID != "" {
		t.Fatalf("detalhe mal preenchido: %+v", out)
	}
	if !aud.tem("recepcao.chegada.walkin") {
		t.Fatal("esperava auditoria recepcao.chegada.walkin")
	}
}

func TestRegistarWalkIn_DoenteInactivo_RegraNegocio(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	leitor := fakeLeitorDoente{activos: map[string]bool{}}
	uc := app.NovoCasoRegistarWalkIn(chegadas, leitor, &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "adm-1", app.DadosWalkIn{DoenteID: "doe-1", EspecialidadeID: "esp-1"})
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio, veio %v", erros.CategoriaDe(err))
	}
}

func TestChamar_TransitaEAudita(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	id, _ := chegadas.Guardar(context.Background(), chegadaWalkIn(t, "doe-1", "esp-1", "09:00"))
	aud := &fakeAuditor{}
	uc := app.NovoCasoChamar(chegadas, aud)
	uc.DefinirRelogio(agoraFixo("09:10"))

	out, err := uc.Executar(context.Background(), "enf-1", id)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.Estado != string(dominio.ChegChamado) {
		t.Fatalf("esperava CHAMADO, veio %s", out.Estado)
	}
	if !aud.tem("recepcao.chegada.chamada") {
		t.Fatal("esperava auditoria recepcao.chegada.chamada")
	}
}

func TestRegistarDesistencia_TransitaEAudita(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	id, _ := chegadas.Guardar(context.Background(), chegadaWalkIn(t, "doe-1", "esp-1", "09:00"))
	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarDesistencia(chegadas, aud)
	uc.DefinirRelogio(agoraFixo("09:10"))

	out, err := uc.Executar(context.Background(), "adm-1", id)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.Estado != string(dominio.ChegDesistiu) {
		t.Fatalf("esperava DESISTIU, veio %s", out.Estado)
	}
	if !aud.tem("recepcao.chegada.desistiu") {
		t.Fatal("esperava auditoria recepcao.chegada.desistiu")
	}
}

func TestListarFila_SoAguardaFiltradoPorEspecialidade(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	_, _ = chegadas.Guardar(context.Background(), chegadaWalkIn(t, "doe-1", "esp-1", "09:00"))
	_, _ = chegadas.Guardar(context.Background(), chegadaWalkIn(t, "doe-2", "esp-2", "09:05"))
	chamada := chegadaWalkIn(t, "doe-3", "esp-1", "09:10")
	_ = chamada.Chamar(inst("09:12"))
	_, _ = chegadas.Guardar(context.Background(), chamada) // CHAMADO não entra na fila

	uc := app.NovoCasoListarFila(chegadas)
	out, err := uc.Executar(context.Background(), "esp-1")
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if len(out) != 1 || out[0].DoenteID != "doe-1" {
		t.Fatalf("esperava só o doe-1 em AGUARDA de esp-1, veio %+v", out)
	}
}
```

Acrescenta o helper ao `fakes_test.go`:

```go
func chegadaWalkIn(t *testing.T, doe, esp, hora string) *dominio.Chegada {
	t.Helper()
	c, err := dominio.NovaChegadaWalkIn(doe, esp, inst(hora))
	if err != nil {
		t.Fatalf("chegada inválida no teste: %v", err)
	}
	return c
}
```

- [ ] **Step 5: Run test to verify it fails**

Run: `go test ./internal/application/recepcao/... -run "WalkIn|Chamar|Desistencia|Fila"`
Expected: FAIL — `undefined: app.NovoCasoRegistarWalkIn`.

- [ ] **Step 6: Write the use cases**

```go
// internal/application/recepcao/chegadas.go
package recepcao

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoRegistarWalkIn regista a chegada de um doente sem marcação.
type CasoRegistarWalkIn struct {
	chegadas dominio.RepositorioChegadas
	doentes  LeitorDoente
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoRegistarWalkIn constrói o caso de uso.
func NovoCasoRegistarWalkIn(c dominio.RepositorioChegadas, d LeitorDoente, a Auditor) *CasoRegistarWalkIn {
	return &CasoRegistarWalkIn{chegadas: c, doentes: d, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoRegistarWalkIn) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar valida o doente (ACL) e regista a chegada walk-in. O actor vai na auditoria.
func (uc *CasoRegistarWalkIn) Executar(ctx context.Context, actor string, dados DadosWalkIn) (DetalheChegada, error) {
	activo, err := uc.doentes.DoenteActivo(ctx, dados.DoenteID)
	if err != nil {
		return DetalheChegada{}, err
	}
	if !activo {
		return DetalheChegada{}, erros.Novo(erros.CategoriaRegraNegocio, "o doente não existe ou não está activo")
	}
	c, err := dominio.NovaChegadaWalkIn(dados.DoenteID, dados.EspecialidadeID, uc.agora())
	if err != nil {
		return DetalheChegada{}, err
	}
	id, err := uc.chegadas.Guardar(ctx, c)
	if err != nil {
		return DetalheChegada{}, err
	}
	c = dominio.ReconstruirChegada(comIDChegada(c.Snapshot(), id))
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.chegada.walkin",
		Entidade: "chegada", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheChegada{}, err
	}
	return paraDetalheChegada(c), nil
}

// CasoChamar chama uma chegada da fila (AGUARDA → CHAMADO).
type CasoChamar struct {
	chegadas dominio.RepositorioChegadas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoChamar constrói o caso de uso.
func NovoCasoChamar(c dominio.RepositorioChegadas, a Auditor) *CasoChamar {
	return &CasoChamar{chegadas: c, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoChamar) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar chama a chegada e audita.
func (uc *CasoChamar) Executar(ctx context.Context, actor, chegadaID string) (DetalheChegada, error) {
	return transitarChegada(ctx, uc.chegadas, uc.auditor, chegadaID, actor, "recepcao.chegada.chamada", uc.agora(),
		func(c *dominio.Chegada) error { return c.Chamar(uc.agora()) })
}

// CasoRegistarDesistencia regista a desistência de uma chegada (AGUARDA → DESISTIU).
type CasoRegistarDesistencia struct {
	chegadas dominio.RepositorioChegadas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoRegistarDesistencia constrói o caso de uso.
func NovoCasoRegistarDesistencia(c dominio.RepositorioChegadas, a Auditor) *CasoRegistarDesistencia {
	return &CasoRegistarDesistencia{chegadas: c, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoRegistarDesistencia) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar regista a desistência e audita.
func (uc *CasoRegistarDesistencia) Executar(ctx context.Context, actor, chegadaID string) (DetalheChegada, error) {
	return transitarChegada(ctx, uc.chegadas, uc.auditor, chegadaID, actor, "recepcao.chegada.desistiu", uc.agora(),
		func(c *dominio.Chegada) error { return c.RegistarDesistencia(uc.agora()) })
}

// CasoListarFila lê a fila de espera (chegadas em AGUARDA) por especialidade.
type CasoListarFila struct {
	chegadas dominio.RepositorioChegadas
}

// NovoCasoListarFila constrói o caso de uso.
func NovoCasoListarFila(c dominio.RepositorioChegadas) *CasoListarFila {
	return &CasoListarFila{chegadas: c}
}

// Executar devolve a fila; especialidade vazia = todas.
func (uc *CasoListarFila) Executar(ctx context.Context, especialidadeID string) ([]ResumoChegada, error) {
	return uc.chegadas.ListarFila(ctx, especialidadeID)
}

// transitarChegada é o padrão comum de Chamar/Desistir: obter → transição de domínio →
// Transitar (CAS) → auditar.
func transitarChegada(ctx context.Context, chegadas dominio.RepositorioChegadas, auditor Auditor,
	chegadaID, actor, accao string, em time.Time, transicao func(*dominio.Chegada) error) (DetalheChegada, error) {
	c, err := chegadas.ObterPorID(ctx, chegadaID)
	if err != nil {
		return DetalheChegada{}, err
	}
	if err := transicao(c); err != nil {
		return DetalheChegada{}, err
	}
	if err := chegadas.Transitar(ctx, c); err != nil {
		return DetalheChegada{}, err
	}
	if err := auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: accao,
		Entidade: "chegada", EntidadeID: chegadaID, OcorridoEm: em,
	}); err != nil {
		return DetalheChegada{}, err
	}
	return paraDetalheChegada(c), nil
}

// comIDChegada devolve uma cópia do snapshot com o id preenchido.
func comIDChegada(s dominio.SnapshotChegada, id string) dominio.SnapshotChegada {
	s.ID = id
	return s
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/application/recepcao/... -cover`
Expected: PASS, cobertura ≥75%.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/recepcao/repositorio.go internal/application/recepcao/ports.go internal/application/recepcao/chegadas.go internal/application/recepcao/fakes_test.go internal/application/recepcao/chegadas_test.go
git commit -m "feat(recepcao): port de chegadas e casos walk-in, chamar, desistir e fila"
```

---

## Task 4: Aplicação — check-in de marcação (coordenação transaccional)

**Files:**
- Modify: `internal/application/recepcao/chegadas.go` (adicionar `CasoRegistarChegada`)
- Test: `internal/application/recepcao/chegadas_test.go` (adicionar testes)

**Interfaces:**
- Consumes: `RepositorioChegadas.RegistarChegadaAgendada` (Task 3), `RepositorioMarcacoes.ObterPorID` (existente), `Marcacao.RegistarComparencia` (Task 1), `dominio.NovaChegadaAgendada` (Task 2).
- Produces: `NovoCasoRegistarChegada(RepositorioChegadas, RepositorioMarcacoes, Auditor)` (`Executar(ctx, actor, marcacaoID string) (DetalheChegada, error)`).

- [ ] **Step 1: Write the failing test**

Acrescenta a `internal/application/recepcao/chegadas_test.go`:

```go
func TestRegistarChegada_TransitaMarcacaoECriaChegada(t *testing.T) {
	marc := novoFakeMarcacoes()
	// marcação MARCADA persistida
	mid, _ := marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-1", "med-1", "esp-1", "09:00", "09:30"))
	chegadas := novoFakeChegadas(marc)
	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarChegada(chegadas, marc, aud)
	uc.DefinirRelogio(agoraFixo("08:50"))

	out, err := uc.Executar(context.Background(), "adm-1", mid)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.ID == "" || out.MarcacaoID != mid || out.MedicoID != "med-1" || out.Estado != string(dominio.ChegAguarda) {
		t.Fatalf("chegada mal preenchida: %+v", out)
	}
	// a marcação passou a COMPARECEU
	m, _ := marc.ObterPorID(context.Background(), mid)
	if m.Estado() != dominio.MarcCompareceu {
		t.Fatalf("a marcação devia estar COMPARECEU, veio %s", m.Estado())
	}
	if !aud.tem("recepcao.chegada.registada") {
		t.Fatal("esperava auditoria recepcao.chegada.registada")
	}
}

func TestRegistarChegada_MarcacaoInexistente_NaoEncontrado(t *testing.T) {
	marc := novoFakeMarcacoes()
	uc := app.NovoCasoRegistarChegada(novoFakeChegadas(marc), marc, &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "adm-1", "marc-inexistente")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava CategoriaNaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

func TestRegistarChegada_Duplicado_Conflito(t *testing.T) {
	marc := novoFakeMarcacoes()
	mid, _ := marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-1", "med-1", "esp-1", "09:00", "09:30"))
	chegadas := novoFakeChegadas(marc)
	uc := app.NovoCasoRegistarChegada(chegadas, marc, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("08:50"))
	if _, err := uc.Executar(context.Background(), "adm-1", mid); err != nil {
		t.Fatalf("primeiro check-in não devia falhar: %v", err)
	}
	// segundo check-in da mesma marcação: já não está MARCADA → Conflito
	_, err := uc.Executar(context.Background(), "adm-1", mid)
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("check-in duplo devia dar CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/recepcao/... -run RegistarChegada`
Expected: FAIL — `undefined: app.NovoCasoRegistarChegada`.

- [ ] **Step 3: Write the use case**

Acrescenta a `internal/application/recepcao/chegadas.go`:

```go
// CasoRegistarChegada faz o check-in de uma marcação: transita a marcação para
// COMPARECEU e cria a chegada na fila, atomicamente.
type CasoRegistarChegada struct {
	chegadas  dominio.RepositorioChegadas
	marcacoes dominio.RepositorioMarcacoes
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoRegistarChegada constrói o caso de uso.
func NovoCasoRegistarChegada(c dominio.RepositorioChegadas, m dominio.RepositorioMarcacoes, a Auditor) *CasoRegistarChegada {
	return &CasoRegistarChegada{chegadas: c, marcacoes: m, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoRegistarChegada) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar obtém a marcação, regista a comparência (MARCADA→COMPARECEU) e cria a
// chegada agendada, persistindo ambos numa transacção coordenada. Um check-in duplo
// falha em RegistarComparencia (a marcação já não está MARCADA) → Conflito.
func (uc *CasoRegistarChegada) Executar(ctx context.Context, actor, marcacaoID string) (DetalheChegada, error) {
	m, err := uc.marcacoes.ObterPorID(ctx, marcacaoID)
	if err != nil {
		return DetalheChegada{}, err
	}
	if err := m.RegistarComparencia(uc.agora()); err != nil {
		return DetalheChegada{}, err
	}
	c, err := dominio.NovaChegadaAgendada(m.DoenteID(), m.ID(), m.MedicoID(), m.EspecialidadeID(), uc.agora())
	if err != nil {
		return DetalheChegada{}, err
	}
	id, err := uc.chegadas.RegistarChegadaAgendada(ctx, c, m)
	if err != nil {
		return DetalheChegada{}, err
	}
	c = dominio.ReconstruirChegada(comIDChegada(c.Snapshot(), id))
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.chegada.registada",
		Entidade: "chegada", EntidadeID: id, OcorridoEm: uc.agora(),
		Detalhe: "marcacao: " + marcacaoID,
	}); err != nil {
		return DetalheChegada{}, err
	}
	return paraDetalheChegada(c), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/application/recepcao/... -cover`
Expected: PASS, cobertura ≥75%.

- [ ] **Step 5: Commit**

```bash
git add internal/application/recepcao/chegadas.go internal/application/recepcao/chegadas_test.go
git commit -m "feat(recepcao): check-in de marcacao com coordenacao transaccional (COMPARECEU + chegada)"
```

---

## Task 5: Migração SQL — `0002_chegadas.sql`

**Files:**
- Create: `migrations/recepcao/0002_chegadas.sql`

**Interfaces:**
- Produces: coluna de estado `COMPARECEU` aceite em `recepcao.marcacoes`; tabela `recepcao.chegadas` (colunas consumidas pela Task 6).

- [ ] **Step 1: Write the migration**

```sql
-- migrations/recepcao/0002_chegadas.sql
-- Bounded Context: recepcao
-- Migration forward-only. Check-in: chegada do doente e fila de espera.

-- Estende o enum de estado da marcação com COMPARECEU (desfecho do check-in). A CHECK
-- inline de 0001 tem o nome auto-gerado determinístico marcacoes_estado_check (só
-- referencia a coluna estado).
ALTER TABLE recepcao.marcacoes DROP CONSTRAINT marcacoes_estado_check;
ALTER TABLE recepcao.marcacoes ADD CONSTRAINT marcacoes_estado_check
    CHECK (estado IN ('MARCADA','CANCELADA','REMARCADA','FALTOU','COMPARECEU'));

-- Chegada: o doente presente na clínica. marcacao_id é FK interna ao schema (como
-- remarca_de em 0001); doente_id/medico_id/especialidade_id são referências textuais a
-- outros bounded contexts, SEM foreign key.
CREATE TABLE IF NOT EXISTS recepcao.chegadas (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id        uuid        NOT NULL,
    marcacao_id      uuid        REFERENCES recepcao.marcacoes(id),
    especialidade_id uuid        NOT NULL,
    medico_id        uuid,
    hora_chegada     timestamptz NOT NULL,
    estado           text        NOT NULL CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU')),
    criado_em        timestamptz NOT NULL DEFAULT now(),
    actualizado_em   timestamptz NOT NULL DEFAULT now(),
    -- Coerência: uma chegada com marcação tem sempre médico (herdado da marcação); o
    -- walk-in não tem marcação nem médico.
    CHECK (marcacao_id IS NULL OR medico_id IS NOT NULL)
);
-- Defesa em profundidade: uma chegada por marcação (o check-in duplo é negado também
-- pela guarda CAS do domínio; a BD fecha a corrida concorrente).
CREATE UNIQUE INDEX IF NOT EXISTS idx_chegadas_marcacao
    ON recepcao.chegadas (marcacao_id) WHERE marcacao_id IS NOT NULL;
-- Índice da fila: AGUARDA por especialidade, ordem FIFO por chegada.
CREATE INDEX IF NOT EXISTS idx_chegadas_fila
    ON recepcao.chegadas (estado, especialidade_id, hora_chegada);
```

- [ ] **Step 2: Verify it compiles/embeds and applies against the real DB**

Run: `go test ./migrations/...`
Expected: PASS (o directório `recepcao` já está na directiva `//go:embed` desde o sub-projecto Marcação; esta é a 2.ª migração do BC).

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags integration ./tests/integration/... -run Migracoes -count=1`
Expected: PASS — prova que a cadeia completa de migrações (incluindo o `DROP CONSTRAINT marcacoes_estado_check`) aplica sem erro contra Postgres real. **Se o `DROP CONSTRAINT` falhar por nome, é aqui que aparece** — nesse caso, consulta o nome real com `\d recepcao.marcacoes` e corrige a migração.

- [ ] **Step 3: Commit**

```bash
git add migrations/recepcao/0002_chegadas.sql
git commit -m "feat(recepcao): migration 0002 (COMPARECEU na marcacao, tabela chegadas, UNIQUE parcial)"
```

---

## Task 6: Repositório pgx — `ChegadasRepo`

**Files:**
- Create: `internal/adapters/pgrepo/chegadas_repo.go`
- Test: `tests/integration/recepcao_chegadas_test.go`

**Interfaces:**
- Consumes: `dominio.RepositorioChegadas` (Task 3), `pgxpool.Pool`.
- Produces: `NovoRepositorioChegadas(*pgxpool.Pool) *RepositorioChegadas` (implementa `dominio.RepositorioChegadas`).

- [ ] **Step 1: Write the failing integration test**

```go
// tests/integration/recepcao_chegadas_test.go
//go:build integration

package integration_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func instCheg(t *testing.T, s string) (out interface{ IsZero() bool }) { return nil } // não usado; ver instD abaixo

func TestRecepcaoChegadasRepo_WalkInTransitarFila(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repo := pgrepo.NovoRepositorioChegadas(pool)
	esp := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	c, _ := dominio.NovaChegadaWalkIn(
		"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", esp, instD(t, "2026-08-10T09:00:00Z"))
	id, err := repo.Guardar(ctx, c)
	if err != nil {
		t.Fatalf("guardar walk-in: %v", err)
	}

	fila, err := repo.ListarFila(ctx, esp)
	if err != nil || len(fila) != 1 || fila[0].ID != id {
		t.Fatalf("fila: %v (n=%d)", err, len(fila))
	}

	obtida, _ := repo.ObterPorID(ctx, id)
	if err := obtida.Chamar(instD(t, "2026-08-10T09:10:00Z")); err != nil {
		t.Fatalf("chamar (domínio): %v", err)
	}
	if err := repo.Transitar(ctx, obtida); err != nil {
		t.Fatalf("transitar: %v", err)
	}
	// CHAMADO já não aparece na fila
	if fila2, _ := repo.ListarFila(ctx, esp); len(fila2) != 0 {
		t.Fatalf("CHAMADO não devia estar na fila, veio n=%d", len(fila2))
	}
}

func TestRecepcaoChegadasRepo_CheckinTransaccionalEUnique(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	marc := pgrepo.NovoRepositorioMarcacoes(pool)
	cheg := pgrepo.NovoRepositorioChegadas(pool)
	medico := "cccccccc-cccc-cccc-cccc-cccccccccccc"

	// marcação MARCADA
	m, _ := dominio.NovaMarcacao(
		"dddddddd-dddd-dddd-dddd-dddddddddddd", medico, "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee",
		instD(t, "2026-08-11T09:00:00Z"), instD(t, "2026-08-11T09:30:00Z"))
	mid, _ := marc.Guardar(ctx, m)

	// check-in: transita a marcação + cria a chegada, atomicamente
	original, _ := marc.ObterPorID(ctx, mid)
	_ = original.RegistarComparencia(instD(t, "2026-08-11T08:50:00Z"))
	ch, _ := dominio.NovaChegadaAgendada(original.DoenteID(), original.ID(), original.MedicoID(),
		original.EspecialidadeID(), instD(t, "2026-08-11T08:50:00Z"))
	if _, err := cheg.RegistarChegadaAgendada(ctx, ch, original); err != nil {
		t.Fatalf("check-in transaccional: %v", err)
	}
	// a marcação ficou COMPARECEU
	recarregada, _ := marc.ObterPorID(ctx, mid)
	if recarregada.Estado() != dominio.MarcCompareceu {
		t.Fatalf("marcação devia estar COMPARECEU, veio %s", recarregada.Estado())
	}

	// segundo check-in da MESMA marcação: a guarda CAS (marcação já não MARCADA) nega
	original2, _ := marc.ObterPorID(ctx, mid) // está COMPARECEU
	// forçar uma tentativa como se ainda estivesse MARCADA não é possível pelo domínio;
	// aqui exercitamos a guarda do repositório directamente construindo o cenário:
	if _, err := cheg.RegistarChegadaAgendada(ctx, ch, recarregadaComoMarcada(t, original2)); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("check-in duplo devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
}

// recarregadaComoMarcada reconstrói a marcação como se ainda estivesse MARCADA (o
// EstadoAnterior fica MARCADA), para exercitar a guarda CAS do repositório contra a
// linha que já está COMPARECEU na BD.
func recarregadaComoMarcada(t *testing.T, m *dominio.Marcacao) *dominio.Marcacao {
	t.Helper()
	s := m.Snapshot()
	s.Estado = dominio.MarcCompareceu // o que se quer escrever
	s.EstadoAnterior = dominio.MarcMarcada // a guarda que já não bate (a BD tem COMPARECEU)
	return dominio.ReconstruirMarcacao(s)
}
```

**Nota ao implementador:** apaga a linha `func instCheg(...)` acima — foi um lapso; o helper correcto é `instD(t, s)`, **que já existe** em `tests/integration/recepcao_janelas_test.go` (mesmo package `integration_test`). NÃO redefinas `instD`. Reutiliza também o `ligar(t)` de `migracoes_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags integration ./tests/integration/... -run RecepcaoChegadas`
Expected: FAIL a compilar — `undefined: pgrepo.NovoRepositorioChegadas`.

- [ ] **Step 3: Write the repository**

```go
// internal/adapters/pgrepo/chegadas_repo.go
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// codigoUnicoPG é o SQLSTATE de violação de restrição UNIQUE.
const codigoUnicoPG = "23505"

// RepositorioChegadas implementa dominio.RepositorioChegadas com pgx.
type RepositorioChegadas struct {
	pool *pgxpool.Pool
}

// NovoRepositorioChegadas constrói o repositório sobre o pool pgx.
func NovoRepositorioChegadas(pool *pgxpool.Pool) *RepositorioChegadas {
	return &RepositorioChegadas{pool: pool}
}

const colunasChegada = `id::text, doente_id::text, COALESCE(marcacao_id::text,''),
       especialidade_id::text, COALESCE(medico_id::text,''), hora_chegada, estado,
       criado_em, actualizado_em`

// Guardar insere uma chegada (walk-in) e devolve o id gerado.
func (r *RepositorioChegadas) Guardar(ctx context.Context, c *dominio.Chegada) (string, error) {
	return r.inserir(ctx, r.pool, c)
}

func (r *RepositorioChegadas) inserir(ctx context.Context, q querier, c *dominio.Chegada) (string, error) {
	s := c.Snapshot()
	const sql = `
INSERT INTO recepcao.chegadas
    (doente_id, marcacao_id, especialidade_id, medico_id, hora_chegada, estado)
VALUES ($1::uuid, NULLIF($2,'')::uuid, $3::uuid, NULLIF($4,'')::uuid, $5, $6)
RETURNING id::text`
	var id string
	err := q.QueryRow(ctx, sql, s.DoenteID, s.MarcacaoID, s.EspecialidadeID, s.MedicoID,
		s.HoraChegada, string(s.Estado)).Scan(&id)
	if err != nil {
		if ehUnica(err) {
			return "", erros.Novo(erros.CategoriaConflito, "já existe uma chegada para esta marcação")
		}
		return "", fmt.Errorf("guardar chegada: %w", err)
	}
	return id, nil
}

// RegistarChegadaAgendada grava, numa única transacção, a marcação a passar a
// COMPARECEU (guarda compare-and-set sobre MARCADA) e a nova chegada.
func (r *RepositorioChegadas) RegistarChegadaAgendada(ctx context.Context, chegada *dominio.Chegada, marcacao *dominio.Marcacao) (string, error) {
	sm := marcacao.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção de check-in: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	const upd = `UPDATE recepcao.marcacoes SET estado=$2, actualizado_em=$3 WHERE id=$1 AND estado=$4`
	ct, err := tx.Exec(ctx, upd, sm.ID, string(sm.Estado), sm.ActualizadoEm, string(sm.EstadoAnterior))
	if err != nil {
		return "", fmt.Errorf("marcar comparência: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", r.erroTransicaoMarcacao(ctx, sm.ID)
	}
	id, err := r.inserir(ctx, tx, chegada)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar check-in: %w", err)
	}
	return id, nil
}

// ObterPorID reconstrói a chegada. NaoEncontrado se não existir.
func (r *RepositorioChegadas) ObterPorID(ctx context.Context, id string) (*dominio.Chegada, error) {
	q := `SELECT ` + colunasChegada + ` FROM recepcao.chegadas WHERE id=$1`
	var s dominio.SnapshotChegada
	var estado string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.DoenteID, &s.MarcacaoID, &s.EspecialidadeID,
		&s.MedicoID, &s.HoraChegada, &estado, &s.CriadoEm, &s.ActualizadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "chegada não encontrada")
		}
		return nil, fmt.Errorf("obter chegada: %w", err)
	}
	s.Estado = dominio.EstadoChegada(estado)
	return dominio.ReconstruirChegada(s), nil
}

// Transitar aplica a transição de estado da chegada com guarda compare-and-set.
func (r *RepositorioChegadas) Transitar(ctx context.Context, c *dominio.Chegada) error {
	s := c.Snapshot()
	if s.ID == "" {
		return erros.Novo(erros.CategoriaNaoEncontrado, "chegada não encontrada")
	}
	const q = `UPDATE recepcao.chegadas SET estado=$2, actualizado_em=$3 WHERE id=$1 AND estado=$4`
	ct, err := r.pool.Exec(ctx, q, s.ID, string(s.Estado), s.ActualizadoEm, string(s.EstadoAnterior))
	if err != nil {
		return fmt.Errorf("actualizar chegada: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return r.erroTransicaoChegada(ctx, s.ID)
	}
	return nil
}

// ListarFila devolve as chegadas em AGUARDA, ordenadas por hora de chegada (FIFO).
// Especialidade vazia = todas.
func (r *RepositorioChegadas) ListarFila(ctx context.Context, especialidadeID string) ([]dominio.ResumoChegada, error) {
	const q = `
SELECT id::text, doente_id::text, COALESCE(marcacao_id::text,''), COALESCE(medico_id::text,''),
       especialidade_id::text, estado, hora_chegada
FROM recepcao.chegadas
WHERE estado='AGUARDA' AND ($1='' OR especialidade_id=NULLIF($1,'')::uuid)
ORDER BY hora_chegada, id`
	linhas, err := r.pool.Query(ctx, q, especialidadeID)
	if err != nil {
		return nil, fmt.Errorf("listar fila: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoChegada{}
	for linhas.Next() {
		var rc dominio.ResumoChegada
		if err := linhas.Scan(&rc.ID, &rc.DoenteID, &rc.MarcacaoID, &rc.MedicoID,
			&rc.EspecialidadeID, &rc.Estado, &rc.HoraChegada); err != nil {
			return nil, fmt.Errorf("ler chegada: %w", err)
		}
		out = append(out, rc)
	}
	return out, linhas.Err()
}

func (r *RepositorioChegadas) erroTransicaoChegada(ctx context.Context, id string) error {
	return r.erroTransicao(ctx, "recepcao.chegadas", "chegada", id)
}

func (r *RepositorioChegadas) erroTransicaoMarcacao(ctx context.Context, id string) error {
	return r.erroTransicao(ctx, "recepcao.marcacoes", "marcação", id)
}

// erroTransicao distingue 404 (linha inexistente) de 409 (estado mudou).
func (r *RepositorioChegadas) erroTransicao(ctx context.Context, tabela, substantivo, id string) error {
	var existe bool
	q := `SELECT EXISTS (SELECT 1 FROM ` + tabela + ` WHERE id=$1)`
	if err := r.pool.QueryRow(ctx, q, id).Scan(&existe); err != nil {
		return fmt.Errorf("verificar %s: %w", substantivo, err)
	}
	if !existe {
		return erros.Novo(erros.CategoriaNaoEncontrado, substantivo+" não encontrada")
	}
	return erros.Novo(erros.CategoriaConflito,
		"o estado da "+substantivo+" mudou entretanto; recarregue e repita a operação")
}

// ehUnica indica se o erro é uma violação de restrição UNIQUE.
func ehUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoUnicoPG
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioChegadas = (*RepositorioChegadas)(nil)
```

**Nota ao implementador:** o tipo `querier` já existe em `internal/adapters/pgrepo/marcacoes_repo.go` (`interface{ QueryRow(ctx, sql, args...) pgx.Row }`) — reutiliza-o, NÃO o redefinas (dava erro de redeclaração no pacote). `pgconn` já é importado por outros repos do pacote.

- [ ] **Step 4: Run tests to verify they pass (against the real DB)**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags integration ./tests/integration/... -run RecepcaoChegadas -count=1 -v`
Expected: PASS (walk-in→fila→transitar; check-in transaccional→COMPARECEU; guarda CAS a negar o duplo). Sem `DATABASE_URL`: SKIP.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/pgrepo/chegadas_repo.go tests/integration/recepcao_chegadas_test.go
git commit -m "feat(recepcao): repositorio pgx ChegadasRepo (check-in transaccional, fila FIFO)"
```

---

## Task 7: Handler HTTP — `recepcao_chegadas_handler.go`

**Files:**
- Create: `internal/adapters/http/recepcao_chegadas_handler.go`
- Test: `internal/adapters/http/recepcao_chegadas_test.go`

**Interfaces:**
- Consumes: os 5 casos de uso (Tasks 3–4) via interfaces de serviço; `SessaoDe`, `RBAC`, `responderErro`, `Auth`, `i18n.MsgPedidoInvalido`, `dominio.Papel*`.
- Produces: `NovoRecepcaoChegadasHandler(...)` e `RegistarRecepcaoChegadas(r gin.IRouter, h *RecepcaoChegadasHandler, protecao ...gin.HandlerFunc)`.

Handler separado (não estende o `NovoRecepcaoHandler` de 8 parâmetros do sub-projecto Marcação — evita um construtor de 13 argumentos e não toca no handler/teste/wiring já existentes). As rotas não colidem com as de marcação (paths distintos).

- [ ] **Step 1: Write the failing test**

```go
// internal/adapters/http/recepcao_chegadas_test.go
package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

type duploRegistarChegada struct{ actorRecebido string }

func (d *duploRegistarChegada) Executar(_ context.Context, actor, _ string) (apprecepcao.DetalheChegada, error) {
	d.actorRecebido = actor
	return apprecepcao.DetalheChegada{ID: "cheg-1", Estado: "AGUARDA", MarcacaoID: "marc-1"}, nil
}

type duploRegistarWalkIn struct{ actorRecebido string }

func (d *duploRegistarWalkIn) Executar(_ context.Context, actor string, _ apprecepcao.DadosWalkIn) (apprecepcao.DetalheChegada, error) {
	d.actorRecebido = actor
	return apprecepcao.DetalheChegada{ID: "cheg-2", Estado: "AGUARDA"}, nil
}

type duploChamar struct{}

func (duploChamar) Executar(_ context.Context, _, _ string) (apprecepcao.DetalheChegada, error) {
	return apprecepcao.DetalheChegada{ID: "cheg-1", Estado: "CHAMADO"}, nil
}

type duploDesistencia struct{}

func (duploDesistencia) Executar(_ context.Context, _, _ string) (apprecepcao.DetalheChegada, error) {
	return apprecepcao.DetalheChegada{ID: "cheg-1", Estado: "DESISTIU"}, nil
}

type duploListarFila struct{}

func (duploListarFila) Executar(_ context.Context, _ string) ([]apprecepcao.ResumoChegada, error) {
	return []apprecepcao.ResumoChegada{}, nil
}

func routerChegadas(t *testing.T, chegar *duploRegistarChegada, walkin *duploRegistarWalkIn, sessao identidade.Sessao) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := adhttp.NovoRecepcaoChegadasHandler(chegar, walkin, duploChamar{}, duploDesistencia{}, duploListarFila{})
	adhttp.RegistarRecepcaoChegadas(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestRegistarChegada_UsaOSujeitoAutenticado(t *testing.T) {
	chegar := &duploRegistarChegada{}
	r := routerChegadas(t, chegar, &duploRegistarWalkIn{}, sessaoRecepcaoDe("adm-9", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/chegada", nil)
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	if chegar.actorRecebido != "adm-9" {
		t.Fatalf("o actor devia vir da sessão, veio %q", chegar.actorRecebido)
	}
}

func TestRegistarChegada_Medico_Proibido(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes/marc-1/chegada", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("um médico não faz check-in: esperava 403, veio %d", w.Code)
	}
}

func TestWalkIn_CorpoMalformado_400(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

func TestChamar_Enfermeiro_Permitido(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("enf-1", identidade.PapelEnfermeiro))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/chamada", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("o enfermeiro pode chamar: esperava 200, veio %d", w.Code)
	}
}

func TestFila_MedicoPodeLer(t *testing.T) {
	r := routerChegadas(t, &duploRegistarChegada{}, &duploRegistarWalkIn{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/fila?especialidade=esp-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("o médico pode ver a fila: esperava 200, veio %d", w.Code)
	}
}
```

**Nota:** `fakeAuth`, `sessaoRecepcaoDe` já existem no pacote `http_test` (de `identidade_test.go`/`recepcao_test.go`). Reutiliza-os.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/http/... -run "Chegada|WalkIn|Chamar|Fila"`
Expected: FAIL — `undefined: adhttp.NovoRecepcaoChegadasHandler`.

- [ ] **Step 3: Write the handler**

```go
// internal/adapters/http/recepcao_chegadas_handler.go
//
// Package http (adaptadores) — este ficheiro expõe o Check-in do BC Recepção (chegadas
// e fila). Handler separado do de marcações para manter os construtores enxutos.
package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso do Check-in.
type (
	// ServicoRegistarChegada faz o check-in de uma marcação.
	ServicoRegistarChegada interface {
		Executar(ctx context.Context, actor, marcacaoID string) (apprecepcao.DetalheChegada, error)
	}
	// ServicoRegistarWalkIn regista a chegada de um doente sem marcação.
	ServicoRegistarWalkIn interface {
		Executar(ctx context.Context, actor string, dados apprecepcao.DadosWalkIn) (apprecepcao.DetalheChegada, error)
	}
	// ServicoChamar chama uma chegada da fila.
	ServicoChamar interface {
		Executar(ctx context.Context, actor, chegadaID string) (apprecepcao.DetalheChegada, error)
	}
	// ServicoRegistarDesistencia regista a desistência de uma chegada.
	ServicoRegistarDesistencia interface {
		Executar(ctx context.Context, actor, chegadaID string) (apprecepcao.DetalheChegada, error)
	}
	// ServicoListarFila devolve a fila de espera por especialidade.
	ServicoListarFila interface {
		Executar(ctx context.Context, especialidadeID string) ([]apprecepcao.ResumoChegada, error)
	}
)

// RecepcaoChegadasHandler expõe os endpoints HTTP do Check-in.
type RecepcaoChegadasHandler struct {
	registarChegada ServicoRegistarChegada
	registarWalkIn  ServicoRegistarWalkIn
	chamar          ServicoChamar
	desistencia     ServicoRegistarDesistencia
	listarFila      ServicoListarFila
}

// NovoRecepcaoChegadasHandler constrói o handler.
func NovoRecepcaoChegadasHandler(
	registarChegada ServicoRegistarChegada, registarWalkIn ServicoRegistarWalkIn,
	chamar ServicoChamar, desistencia ServicoRegistarDesistencia, listarFila ServicoListarFila,
) *RecepcaoChegadasHandler {
	return &RecepcaoChegadasHandler{
		registarChegada: registarChegada, registarWalkIn: registarWalkIn,
		chamar: chamar, desistencia: desistencia, listarFila: listarFila,
	}
}

// RegistarRecepcaoChegadas regista as rotas do Check-in. O check-in, o walk-in e a
// desistência são função do Administrativo (balcão); chamar o próximo é de quem vai
// atender (Enfermeiro/Médico) e também do Administrativo; a fila é visível ao pessoal
// de balcão e clínico.
func RegistarRecepcaoChegadas(r gin.IRouter, h *RecepcaoChegadasHandler, protecao ...gin.HandlerFunc) {
	soAdministrativo := RBAC(dominio.PapelAdministrativo, dominio.PapelDirector, dominio.PapelAdmin)
	chamada := RBAC(dominio.PapelEnfermeiro, dominio.PapelMedico, dominio.PapelAdministrativo)
	filaLeitura := RBAC(dominio.PapelAdministrativo, dominio.PapelEnfermeiro, dominio.PapelMedico)

	gmar := r.Group("/api/v1/marcacoes")
	gmar.Use(protecao...)
	gmar.POST("/:mid/chegada", soAdministrativo, h.registarChegadaHTTP)

	gc := r.Group("/api/v1/chegadas")
	gc.Use(protecao...)
	gc.POST("", soAdministrativo, h.registarWalkInHTTP)
	gc.POST("/:cid/chamada", chamada, h.chamarHTTP)
	gc.POST("/:cid/desistencia", soAdministrativo, h.desistenciaHTTP)

	gf := r.Group("/api/v1/recepcao")
	gf.Use(protecao...)
	gf.GET("/fila", filaLeitura, h.listarFilaHTTP)
}

func (h *RecepcaoChegadasHandler) registarChegadaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.registarChegada.Executar(c.Request.Context(), actor.Sujeito, c.Param("mid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoChegadasHandler) registarWalkInHTTP(c *gin.Context) {
	var dados apprecepcao.DadosWalkIn
	if err := c.ShouldBindJSON(&dados); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registarWalkIn.Executar(c.Request.Context(), actor.Sujeito, dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoChegadasHandler) chamarHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.chamar.Executar(c.Request.Context(), actor.Sujeito, c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoChegadasHandler) desistenciaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.desistencia.Executar(c.Request.Context(), actor.Sujeito, c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoChegadasHandler) listarFilaHTTP(c *gin.Context) {
	out, err := h.listarFila.Executar(c.Request.Context(), c.Query("especialidade"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/http/... -cover`
Expected: PASS; cobertura agregada do pacote ≥60%.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/recepcao_chegadas_handler.go internal/adapters/http/recepcao_chegadas_test.go
git commit -m "feat(recepcao): handler HTTP do check-in (chegada, walk-in, chamar, desistir, fila)"
```

---

## Task 8: Composição, ADR-033 e CLAUDE.md

**Files:**
- Modify: `internal/platform/app.go` (montar os repos/casos/handler do check-in + registar rotas)
- Create: `adrs/ADR-033-bc-recepcao-checkin.md`
- Modify: `CLAUDE.md` (lista de ADRs + próximo número; nota do sub-projecto)

**Interfaces:**
- Consumes: tudo o que as Tasks 1–7 produziram.

- [ ] **Step 1: Wire the check-in in the composition root**

Em `internal/platform/app.go`, a seguir ao bloco do handler de Recepção (a seguir a `handlerRecepcao := ...`, antes de "Middlewares transversais"), acrescenta:

```go
	// BC Recepção — Check-in (chegada, fila de espera).
	repoChegadas := pgrepo.NovoRepositorioChegadas(pool)
	handlerRecepcaoChegadas := adhttp.NovoRecepcaoChegadasHandler(
		apprecepcao.NovoCasoRegistarChegada(repoChegadas, repoMarcacoes, repoAuditoria),
		apprecepcao.NovoCasoRegistarWalkIn(repoChegadas, leitorDoenteRec, repoAuditoria),
		apprecepcao.NovoCasoChamar(repoChegadas, repoAuditoria),
		apprecepcao.NovoCasoRegistarDesistencia(repoChegadas, repoAuditoria),
		apprecepcao.NovoCasoListarFila(repoChegadas),
	)
```

E em `registarRotas`, a seguir a `adhttp.RegistarRecepcao(r, handlerRecepcao, limiteMW, authMW)`:

```go
		adhttp.RegistarRecepcaoChegadas(r, handlerRecepcaoChegadas, limiteMW, authMW)
```

- [ ] **Step 2: Verify build and full unit suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS (unitários; integração fica SKIP sem `DATABASE_URL`).

- [ ] **Step 3: Verify the architectural linter**

Run: `go-arch-lint check`
Expected: `OK - No warnings found` — `internal/domain/recepcao` e `internal/application/recepcao` continuam sem importar infra nem outro BC.

- [ ] **Step 4: Write ADR-033**

```markdown
# ADR-033 — BC Recepção: Check-in e fila de espera

- **Estado:** Aceite
- **Data:** 2026-07-15
- **Marco/Sprint:** Percurso Ambulatório / sub-projecto Check-in
- **Fontes:** design em `docs/superpowers/specs/2026-07-15-recepcao-checkin-design.md`; ADR-032 (Marcação, precedente do mesmo BC).

## Contexto

A Marcação (ADR-032) entrega a agenda e o ciclo da marcação até FALTOU. Faltava a
chegada do doente à clínica e a fila de espera. Este sub-projecto entrega o Check-in.

## Decisão

1. **Novo agregado `Chegada`** no BC `recepcao`, que unifica o check-in de uma marcação
   e o walk-in (sem marcação) numa só entidade e numa só fila. O walk-in não é forçado
   pela invariante de disponibilidade das marcações.
2. **Novo estado `COMPARECEU` na `Marcacao`** (método `RegistarComparencia`, MARCADA→
   COMPARECEU) — desfecho simétrico ao FALTOU; depois de comparecer, a marcação já não
   pode ser cancelada, remarcada nem dar falta.
3. **Check-in de marcação é transaccional e cruza dois agregados:**
   `RepositorioChegadas.RegistarChegadaAgendada` transita a marcação para COMPARECEU
   (guarda compare-and-set sobre MARCADA) e insere a chegada na mesma transacção. O
   check-in duplo falha na guarda CAS (a marcação já não está MARCADA) → Conflito.
   Defesa em profundidade: índice `UNIQUE` parcial sobre `chegadas.marcacao_id`.
4. **Walk-in** valida o doente pela ACL `LeitorDoente` (doente registado, como na
   marcação) e regista a chegada sem marcação nem médico (o médico é atribuído depois).
5. **Fila** = chegadas em `AGUARDA`, ordenadas por hora de chegada (FIFO), filtráveis por
   especialidade. A prioridade clínica fica para a Triagem.
6. **RBAC:** check-in, walk-in e desistência são do `Administrativo` (balcão); chamar o
   próximo (`AGUARDA→CHAMADO`) é de quem atende (`Enfermeiro`/`Médico`) e do
   `Administrativo`; a fila é visível ao pessoal de balcão e clínico. Handler HTTP
   separado do de marcações para manter os construtores enxutos.

## Consequências

- O percurso ambulatório fica pronto para a Triagem (que consumirá os `CHAMADO`).
- A atribuição de médico ao walk-in e a prioridade clínica ficam para a Triagem.
- O check-in aceita qualquer marcação `MARCADA` (a data não é imposta — decisão
  operacional da recepção).
```

- [ ] **Step 5: Update CLAUDE.md**

Na secção "Convenções-fonte" de `CLAUDE.md`, acrescenta a ADR-033 e actualiza o próximo número:

```
`adrs/ADR-032-bc-recepcao-marcacao.md`,
`adrs/ADR-033-bc-recepcao-checkin.md`.
Próximo ADR: **ADR-034**.
```

Na secção "6. Marco Actual", actualiza a nota do Marco Percurso Ambulatório para registar
o Check-in como entregue e a Triagem como o pendente:

```
**Marco Percurso Ambulatório** (em curso, a par do M3): abre o percurso do doente antes
da consulta. Sub-projectos entregues: **Marcação** (ADR-032) e **Check-in** (BC `recepcao`
— chegada, fila de espera, estado COMPARECEU; ver ADR-033). Pendente: Triagem.
```

- [ ] **Step 6: Final build + integration suite + commit**

Run: `go build ./... && go test ./...`
Expected: PASS.

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags integration ./tests/integration/... -count=1`
Expected: verde (os testes de Recepção passam; skips só de Keycloak/MailHog).

```bash
git add internal/platform/app.go adrs/ADR-033-bc-recepcao-checkin.md CLAUDE.md
git commit -m "feat(recepcao): liga o Check-in ao composition root + ADR-033"
```

---

## Self-Review (autor)

**1. Cobertura do spec:**
- §2 novo agregado `Chegada` + `COMPARECEU` na `Marcacao` → Tasks 1–2. ✓
- §3.1 `Chegada` (estados, dois construtores, Chamar/Desistir, snapshot) → Task 2. ✓
- §3.2 `RegistarComparencia` + recusa das transições a partir de COMPARECEU → Task 1. ✓
- §3.3 coordenação transaccional (CAS + UNIQUE) → Tasks 4 (aplicação) e 6 (pgrepo). ✓
- §4.1 5 casos de uso + acções de auditoria → Tasks 3–4. ✓
- §4.2 `RepositorioChegadas`, DTOs → Tasks 3. ✓
- §5.1 HTTP + RBAC (soAdministrativo/chamada/filaLeitura) → Task 7. ✓
- §5.2 migração (ALTER CHECK + chegadas + UNIQUE parcial + índice fila) → Task 5. ✓
- §6 erros/auditoria → categorias usadas; auditoria em cada comando. ✓
- §7 cobertura (domínio Tasks 1–2, aplicação Tasks 3–4, adaptadores Tasks 6–7). ✓
- §8 ADR-033 + critérios de saída → Task 8. ✓

**2. Placeholders:** nenhum "TBD"/"TODO"; todos os passos com código completo. As notas de "reutiliza o helper existente" (querier, instD, ligar, fakeAuth) são instruções contra o código real, não lacunas. O `func instCheg` no teste da Task 6 está marcado explicitamente para apagar (lapso deliberadamente assinalado com a correcção ao lado).

**3. Consistência de tipos:** `EstadoChegada`/consts, `NovaChegadaAgendada`/`NovaChegadaWalkIn`, `RepositorioChegadas` (5 métodos, incl. `RegistarChegadaAgendada(chegada, marcacao)`), `ResumoChegada`/`DetalheChegada`, `MarcCompareceu`/`RegistarComparencia`, e as acções de auditoria (`recepcao.chegada.registada/walkin/chamada/desistiu`) coerentes entre domínio, aplicação, adaptadores e ADR. O padrão `comIDChegada`/`ReconstruirChegada` para rehidratar com id é uniforme com o resto do BC.
