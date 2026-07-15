# BC Recepção — Marcação — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar o primeiro sub-projecto do BC Recepção — a Marcação de consultas contra a disponibilidade declarada de cada médico, com ciclo de vida (cancelar, remarcar, registar falta) e agenda consultável.

**Architecture:** Novo bounded context `recepcao` com as 4 camadas Clean e schema PostgreSQL próprio. Dois agregados (`JanelaDisponibilidade`, `Marcacao`); a invariante de disponibilidade é uma função de domínio pura alimentada pelos casos de uso; a remarcação preserva histórico por *supersede* (original→`REMARCADA` + nova `MARCADA`). Sem FK cross-context: um adaptador ACL `LeitorDoente` valida o doente contra o BC Clínico. Defesa em profundidade contra sobreposições com uma restrição `EXCLUDE` na base de dados.

**Tech Stack:** Go 1.22+, Gin, pgx v5 (SQL puro, sem ORM), PostgreSQL 16 (extensão `btree_gist`), testes com fakes (não mocks) + integração `//go:build integration`.

## Global Constraints

- **Idioma:** PT-PT angolano em TODO o output (código, comentários, commits, mensagens, JSON de erro). Nunca EN/BR. Termos de negócio em português (Marcação, Janela, Doente, Médico, Especialidade).
- **Module path:** `github.com/ivandrosilva12/sgcfinal`.
- **Sem FK cross-context.** `doente_id`/`medico_id`/`especialidade_id` são referências textuais (uuid) sem FK para outros schemas.
- **Domínio sem infra:** `internal/domain/**` nunca importa `pgx`, `gin`, `net/http`. `go-arch-lint` bloqueia em CI.
- **Migrations forward-only**, sem `.down.sql`.
- **Nada de `panic()`** fora de inicialização — sempre `error` com categoria de `internal/domain/shared/erros`.
- **Actor = sujeito autenticado** (`SessaoDe(c).Sujeito` no handler), nunca um campo do corpo.
- **Auditoria append-only** em todos os comandos; leituras não são auditadas.
- **Cobertura:** domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
- **Categorias de erro** (`erros.Categoria`): `CategoriaValidacao`(400/422), `CategoriaProibido`(403), `CategoriaNaoEncontrado`(404), `CategoriaConflito`(409), `CategoriaRegraNegocio`(422), `CategoriaInterno`(500).
- **Registo de auditoria** (`auditoria.Registo`): campos `Actor, Accao, Entidade, EntidadeID, OcorridoEm, Detalhe`.

---

## Contratos partilhados (definidos ao longo do plano)

Tipos e assinaturas que várias tarefas consomem. Cada um é criado na tarefa indicada.

**Domínio (`internal/domain/recepcao`)** — Tarefas 1–4:
- `type EstadoMarcacao string`; consts `MarcMarcada="MARCADA"`, `MarcCancelada="CANCELADA"`, `MarcRemarcada="REMARCADA"`, `MarcFaltou="FALTOU"`.
- `func NovaJanela(medicoID, especialidadeID string, inicio, fim time.Time) (*JanelaDisponibilidade, error)` + getters `ID/MedicoID/EspecialidadeID/Inicio/Fim`, `Snapshot()/ReconstruirJanela`.
- `func NovaMarcacao(doenteID, medicoID, especialidadeID string, inicio, fim time.Time) (*Marcacao, error)` + `Cancelar/Remarcar/RegistarFalta` + getters + `Snapshot()/ReconstruirMarcacao`.
- `func (m *Marcacao) Remarcar(novoInicio, novoFim, em time.Time) (*Marcacao, error)`.
- `func VerificarDisponibilidade(janelas []JanelaDisponibilidade, activas []Marcacao, especialidadeID string, inicio, fim, agora time.Time) error`.
- `type ResumoJanela struct{ ID, MedicoID, EspecialidadeID string; Inicio, Fim time.Time }`.
- `type ResumoMarcacao struct{ ID, DoenteID, MedicoID, EspecialidadeID, Estado, Motivo string; Inicio, Fim, CriadoEm time.Time }`.
- `type RepositorioJanelas interface` e `type RepositorioMarcacoes interface` (ver Tarefa 4).

**Aplicação (`internal/application/recepcao`)** — Tarefas 4–6:
- `type Auditor interface{ Registar(ctx, auditoria.Registo) error }`.
- `type LeitorDoente interface{ DoenteActivo(ctx context.Context, doenteID string) (bool, error) }`.
- DTOs `DadosDefinirJanela`, `DadosMarcar`, `DadosRemarcar`, `DetalheJanela`, `DetalheMarcacao`, `Agenda`.
- Casos: `NovoCasoDefinirJanela`, `NovoCasoRemoverJanela`, `NovoCasoMarcar`, `NovoCasoRemarcar`, `NovoCasoCancelar`, `NovoCasoRegistarFalta`, `NovoCasoListarAgenda`, `NovoCasoListarMarcacoesDoente`.

---

## Task 1: Domínio — Agregado `JanelaDisponibilidade`

**Files:**
- Create: `internal/domain/recepcao/doc.go`
- Create: `internal/domain/recepcao/janela.go`
- Test: `internal/domain/recepcao/janela_test.go`

**Interfaces:**
- Produces: `NovaJanela(medicoID, especialidadeID string, inicio, fim time.Time) (*JanelaDisponibilidade, error)`; getters `ID() string`, `MedicoID() string`, `EspecialidadeID() string`, `Inicio() time.Time`, `Fim() time.Time`; `type SnapshotJanela struct`; `Snapshot() SnapshotJanela`; `ReconstruirJanela(SnapshotJanela) *JanelaDisponibilidade`.

- [ ] **Step 1: Write the failing test**

```go
// internal/domain/recepcao/janela_test.go
package recepcao_test

import (
	"testing"
	"time"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func inst(hhmm string) time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-07-20T"+hhmm+":00Z")
	return t
}

func TestNovaJanela_Valida(t *testing.T) {
	j, err := recepcao.NovaJanela("med-1", "esp-1", inst("08:00"), inst("13:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if j.MedicoID() != "med-1" || j.EspecialidadeID() != "esp-1" {
		t.Fatalf("campos mal preenchidos: %+v", j)
	}
	if !j.Inicio().Equal(inst("08:00")) || !j.Fim().Equal(inst("13:00")) {
		t.Fatalf("intervalo mal preenchido")
	}
}

func TestNovaJanela_FimNaoPosteriorAoInicio_Erro(t *testing.T) {
	_, err := recepcao.NovaJanela("med-1", "esp-1", inst("13:00"), inst("08:00"))
	if err == nil {
		t.Fatal("esperava erro quando fim <= início")
	}
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestNovaJanela_MedicoEmFalta_Erro(t *testing.T) {
	if _, err := recepcao.NovaJanela("  ", "esp-1", inst("08:00"), inst("13:00")); err == nil {
		t.Fatal("esperava erro com médico em falta")
	}
}

func TestJanela_SnapshotEReconstrucao(t *testing.T) {
	s := recepcao.SnapshotJanela{
		ID: "jan-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("08:00"), Fim: inst("13:00"), CriadoEm: inst("07:00"),
	}
	j := recepcao.ReconstruirJanela(s)
	if j.ID() != "jan-1" || j.Snapshot().MedicoID != "med-1" {
		t.Fatalf("snapshot não redondo: %+v", j.Snapshot())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/recepcao/...`
Expected: FAIL — `package recepcao is not in std` / `undefined: recepcao.NovaJanela`.

- [ ] **Step 3: Write the package doc**

```go
// internal/domain/recepcao/doc.go

// Package recepcao é o BC Recepção (Camada 1 — Domínio): o percurso ambulatório do
// doente antes da consulta. Este sub-projecto cobre a Marcação — a agenda declarada
// de cada médico (JanelaDisponibilidade) e o agendamento de consultas (Marcacao) com
// o seu ciclo de vida. Não importa infra (pgx/gin/net/http). O check-in (Recepção) e
// a Triagem são sub-projectos futuros.
package recepcao
```

- [ ] **Step 4: Write the aggregate**

```go
// internal/domain/recepcao/janela.go
package recepcao

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// JanelaDisponibilidade é a agenda declarada de um médico: um intervalo datado onde
// é possível marcar consultas dessa especialidade. É um agregado sem máquina de
// estados — existe ou é removido. O id é gerado pela base de dados.
type JanelaDisponibilidade struct {
	id              string
	medicoID        string
	especialidadeID string
	inicio          time.Time
	fim             time.Time
	criadoEm        time.Time
}

// NovaJanela valida e constrói uma janela de disponibilidade. Médico, especialidade e
// um intervalo com fim estritamente posterior ao início são obrigatórios.
func NovaJanela(medicoID, especialidadeID string, inicio, fim time.Time) (*JanelaDisponibilidade, error) {
	medicoID = strings.TrimSpace(medicoID)
	if medicoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "médico da janela em falta")
	}
	especialidadeID = strings.TrimSpace(especialidadeID)
	if especialidadeID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "especialidade da janela em falta")
	}
	if inicio.IsZero() || fim.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "intervalo da janela em falta")
	}
	if !fim.After(inicio) {
		return nil, erros.Novo(erros.CategoriaValidacao, "o fim da janela tem de ser posterior ao início")
	}
	return &JanelaDisponibilidade{
		medicoID: medicoID, especialidadeID: especialidadeID, inicio: inicio, fim: fim,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados (vazio se não persistido).
func (j *JanelaDisponibilidade) ID() string { return j.id }

// MedicoID devolve o médico a que a janela pertence.
func (j *JanelaDisponibilidade) MedicoID() string { return j.medicoID }

// EspecialidadeID devolve a especialidade da janela.
func (j *JanelaDisponibilidade) EspecialidadeID() string { return j.especialidadeID }

// Inicio devolve o início da janela.
func (j *JanelaDisponibilidade) Inicio() time.Time { return j.inicio }

// Fim devolve o fim da janela.
func (j *JanelaDisponibilidade) Fim() time.Time { return j.fim }

// SnapshotJanela carrega o estado completo para persistência ou rehidratação.
type SnapshotJanela struct {
	ID              string
	MedicoID        string
	EspecialidadeID string
	Inicio          time.Time
	Fim             time.Time
	CriadoEm        time.Time
}

// Snapshot devolve o estado completo do agregado.
func (j *JanelaDisponibilidade) Snapshot() SnapshotJanela {
	return SnapshotJanela{
		ID: j.id, MedicoID: j.medicoID, EspecialidadeID: j.especialidadeID,
		Inicio: j.inicio, Fim: j.fim, CriadoEm: j.criadoEm,
	}
}

// ReconstruirJanela reconstrói um agregado a partir de um snapshot persistido (dados
// de fonte confiável — não revalida invariantes).
func ReconstruirJanela(s SnapshotJanela) *JanelaDisponibilidade {
	return &JanelaDisponibilidade{
		id: s.ID, medicoID: s.MedicoID, especialidadeID: s.EspecialidadeID,
		inicio: s.Inicio, fim: s.Fim, criadoEm: s.CriadoEm,
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/domain/recepcao/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/recepcao/doc.go internal/domain/recepcao/janela.go internal/domain/recepcao/janela_test.go
git commit -m "feat(recepcao): agregado JanelaDisponibilidade no dominio"
```

---

## Task 2: Domínio — Agregado `Marcacao` (máquina de estados)

**Files:**
- Create: `internal/domain/recepcao/marcacao.go`
- Test: `internal/domain/recepcao/marcacao_test.go`

**Interfaces:**
- Consumes: `erros` (Task 1 já usa o pacote).
- Produces: `type EstadoMarcacao string` + consts `MarcMarcada/MarcCancelada/MarcRemarcada/MarcFaltou`; `NovaMarcacao(doenteID, medicoID, especialidadeID string, inicio, fim time.Time) (*Marcacao, error)`; métodos `Cancelar(motivo string, em time.Time) error`, `Remarcar(novoInicio, novoFim, em time.Time) (*Marcacao, error)`, `RegistarFalta(em time.Time) error`; getters `ID/DoenteID/MedicoID/EspecialidadeID/Inicio/Fim/Estado/RemarcaDe`; `type SnapshotMarcacao struct` (com `EstadoAnterior`); `Snapshot()`; `ReconstruirMarcacao(SnapshotMarcacao) *Marcacao`.

- [ ] **Step 1: Write the failing test**

```go
// internal/domain/recepcao/marcacao_test.go
package recepcao_test

import (
	"testing"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func novaMarcacaoValida(t *testing.T) *recepcao.Marcacao {
	t.Helper()
	m, err := recepcao.NovaMarcacao("doe-1", "med-1", "esp-1", inst("09:00"), inst("09:30"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	return m
}

func TestNovaMarcacao_NasceMarcada(t *testing.T) {
	m := novaMarcacaoValida(t)
	if m.Estado() != recepcao.MarcMarcada {
		t.Fatalf("esperava MARCADA, veio %s", m.Estado())
	}
	if m.DoenteID() != "doe-1" || m.MedicoID() != "med-1" || m.EspecialidadeID() != "esp-1" {
		t.Fatal("campos mal preenchidos")
	}
}

func TestNovaMarcacao_FimNaoPosterior_Erro(t *testing.T) {
	_, err := recepcao.NovaMarcacao("doe-1", "med-1", "esp-1", inst("09:30"), inst("09:00"))
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestCancelar_DeMarcada(t *testing.T) {
	m := novaMarcacaoValida(t)
	if err := m.Cancelar("doente desistiu", inst("08:00")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if m.Estado() != recepcao.MarcCancelada {
		t.Fatalf("esperava CANCELADA, veio %s", m.Estado())
	}
}

func TestCancelar_SemMotivo_Erro(t *testing.T) {
	m := novaMarcacaoValida(t)
	if err := m.Cancelar("  ", inst("08:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao sem motivo, veio %v", erros.CategoriaDe(err))
	}
}

func TestCancelar_JaCancelada_Conflito(t *testing.T) {
	m := novaMarcacaoValida(t)
	_ = m.Cancelar("motivo", inst("08:00"))
	if err := m.Cancelar("outra vez", inst("08:00")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestRemarcar_SupersedePreservandoOriginal(t *testing.T) {
	original := novaMarcacaoValida(t)
	// simula que a original já foi persistida com um id
	original = recepcao.ReconstruirMarcacao(comID(original.Snapshot(), "marc-1"))

	nova, err := original.Remarcar(inst("10:00"), inst("10:30"), inst("08:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if original.Estado() != recepcao.MarcRemarcada {
		t.Fatalf("original devia ficar REMARCADA, veio %s", original.Estado())
	}
	if nova.Estado() != recepcao.MarcMarcada {
		t.Fatalf("nova devia ser MARCADA, veio %s", nova.Estado())
	}
	if nova.RemarcaDe() != "marc-1" {
		t.Fatalf("nova devia apontar para a original (marc-1), veio %q", nova.RemarcaDe())
	}
	if nova.DoenteID() != "doe-1" || nova.MedicoID() != "med-1" || nova.EspecialidadeID() != "esp-1" {
		t.Fatal("a nova marcação devia preservar doente/médico/especialidade")
	}
	if !nova.Inicio().Equal(inst("10:00")) {
		t.Fatal("a nova marcação devia ter o novo início")
	}
}

func TestRegistarFalta_SoAposAHora(t *testing.T) {
	m := novaMarcacaoValida(t) // fim = 09:30
	// antes da hora: recusa
	if err := m.RegistarFalta(inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("falta antes da hora devia dar CategoriaRegraNegocio, veio %v", erros.CategoriaDe(err))
	}
	// depois da hora: aceita
	if err := m.RegistarFalta(inst("10:00")); err != nil {
		t.Fatalf("não esperava erro após a hora: %v", err)
	}
	if m.Estado() != recepcao.MarcFaltou {
		t.Fatalf("esperava FALTOU, veio %s", m.Estado())
	}
}

// comID devolve uma cópia do snapshot com o id preenchido (utilitário de teste).
func comID(s recepcao.SnapshotMarcacao, id string) recepcao.SnapshotMarcacao {
	s.ID = id
	return s
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/recepcao/...`
Expected: FAIL — `undefined: recepcao.NovaMarcacao`.

- [ ] **Step 3: Write the aggregate**

```go
// internal/domain/recepcao/marcacao.go
package recepcao

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// EstadoMarcacao é o estado do ciclo de vida de uma marcação.
//
//	MARCADA ─┬─ Cancelar ─────► CANCELADA
//	         ├─ Remarcar ─────► REMARCADA  (+ nova Marcacao MARCADA)
//	         └─ RegistarFalta ► FALTOU
//
// A chegada do doente (COMPARECEU) não faz parte deste sub-projecto — será marcada
// pelo módulo Recepção (check-in) num ciclo futuro.
type EstadoMarcacao string

const (
	MarcMarcada   EstadoMarcacao = "MARCADA"
	MarcCancelada EstadoMarcacao = "CANCELADA"
	MarcRemarcada EstadoMarcacao = "REMARCADA"
	MarcFaltou    EstadoMarcacao = "FALTOU"
)

// Marcacao é o agregado raiz do BC Recepção: uma consulta agendada para um doente,
// com um médico e uma especialidade, num intervalo. Refere doente/médico/especialidade
// por id (agregados de outros contextos). O id é gerado pela base de dados.
type Marcacao struct {
	id              string
	doenteID        string
	medicoID        string
	especialidadeID string
	inicio          time.Time
	fim             time.Time
	estado          EstadoMarcacao
	estadoAnterior  EstadoMarcacao
	motivo          string
	remarcaDe       string
	criadoEm        time.Time
	actualizadoEm   time.Time
}

// NovaMarcacao valida e constrói uma marcação no estado MARCADA. Doente, médico,
// especialidade e um intervalo com fim posterior ao início são obrigatórios. A
// verificação de disponibilidade (janela livre, sem sobreposição, não no passado) é
// feita pelo caso de uso com VerificarDisponibilidade — não aqui, porque cruza outros
// agregados.
func NovaMarcacao(doenteID, medicoID, especialidadeID string, inicio, fim time.Time) (*Marcacao, error) {
	doenteID = strings.TrimSpace(doenteID)
	if doenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente da marcação em falta")
	}
	medicoID = strings.TrimSpace(medicoID)
	if medicoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "médico da marcação em falta")
	}
	especialidadeID = strings.TrimSpace(especialidadeID)
	if especialidadeID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "especialidade da marcação em falta")
	}
	if inicio.IsZero() || fim.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "intervalo da marcação em falta")
	}
	if !fim.After(inicio) {
		return nil, erros.Novo(erros.CategoriaValidacao, "o fim da marcação tem de ser posterior ao início")
	}
	return &Marcacao{
		doenteID: doenteID, medicoID: medicoID, especialidadeID: especialidadeID,
		inicio: inicio, fim: fim, estado: MarcMarcada,
	}, nil
}

// Cancelar transita MARCADA → CANCELADA. O motivo é obrigatório: um cancelamento sem
// razão registada não é auditável.
func (m *Marcacao) Cancelar(motivo string, em time.Time) error {
	if m.estado != MarcMarcada {
		return erros.Novo(erros.CategoriaConflito, "só é possível cancelar uma marcação em estado MARCADA")
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return erros.Novo(erros.CategoriaValidacao, "motivo do cancelamento em falta")
	}
	m.estado = MarcCancelada
	m.motivo = motivo
	m.actualizadoEm = em
	return nil
}

// Remarcar transita a marcação receptora MARCADA → REMARCADA e devolve uma NOVA
// marcação MARCADA para o novo intervalo, apontando para a original (RemarcaDe). O
// histórico da original é preservado. A disponibilidade do novo intervalo é verificada
// pelo caso de uso. A original tem de já estar persistida (ter id) para que a nova a
// possa referenciar.
func (m *Marcacao) Remarcar(novoInicio, novoFim, em time.Time) (*Marcacao, error) {
	if m.estado != MarcMarcada {
		return nil, erros.Novo(erros.CategoriaConflito, "só é possível remarcar uma marcação em estado MARCADA")
	}
	if m.id == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "a marcação original tem de estar persistida para ser remarcada")
	}
	nova, err := NovaMarcacao(m.doenteID, m.medicoID, m.especialidadeID, novoInicio, novoFim)
	if err != nil {
		return nil, err
	}
	nova.remarcaDe = m.id
	m.estado = MarcRemarcada
	m.actualizadoEm = em
	return nova, nil
}

// RegistarFalta transita MARCADA → FALTOU. Só é possível depois da hora marcada
// (em >= fim): registar falta antes de a consulta acabar não faz sentido.
func (m *Marcacao) RegistarFalta(em time.Time) error {
	if m.estado != MarcMarcada {
		return erros.Novo(erros.CategoriaConflito, "só é possível registar falta de uma marcação em estado MARCADA")
	}
	if em.Before(m.fim) {
		return erros.Novo(erros.CategoriaRegraNegocio, "só é possível registar falta depois da hora marcada")
	}
	m.estado = MarcFaltou
	m.actualizadoEm = em
	return nil
}

// ID devolve o identificador atribuído pela base de dados.
func (m *Marcacao) ID() string { return m.id }

// DoenteID devolve o doente da marcação.
func (m *Marcacao) DoenteID() string { return m.doenteID }

// MedicoID devolve o médico da marcação.
func (m *Marcacao) MedicoID() string { return m.medicoID }

// EspecialidadeID devolve a especialidade da marcação.
func (m *Marcacao) EspecialidadeID() string { return m.especialidadeID }

// Inicio devolve o início da marcação.
func (m *Marcacao) Inicio() time.Time { return m.inicio }

// Fim devolve o fim da marcação.
func (m *Marcacao) Fim() time.Time { return m.fim }

// Estado devolve o estado actual.
func (m *Marcacao) Estado() EstadoMarcacao { return m.estado }

// RemarcaDe devolve o id da marcação original que esta remarca (vazio se não for uma
// remarcação).
func (m *Marcacao) RemarcaDe() string { return m.remarcaDe }

// SnapshotMarcacao carrega o estado completo para persistência ou rehidratação.
//
// EstadoAnterior é o estado lido da base de dados (vazio num agregado novo). O
// repositório usa-o como guarda compare-and-set no UPDATE de transição. É derivado —
// quem reconstrói não o preenche.
type SnapshotMarcacao struct {
	ID              string
	DoenteID        string
	MedicoID        string
	EspecialidadeID string
	Inicio          time.Time
	Fim             time.Time
	Estado          EstadoMarcacao
	EstadoAnterior  EstadoMarcacao
	Motivo          string
	RemarcaDe       string
	CriadoEm        time.Time
	ActualizadoEm   time.Time
}

// Snapshot devolve o estado completo do agregado.
func (m *Marcacao) Snapshot() SnapshotMarcacao {
	return SnapshotMarcacao{
		ID: m.id, DoenteID: m.doenteID, MedicoID: m.medicoID, EspecialidadeID: m.especialidadeID,
		Inicio: m.inicio, Fim: m.fim, Estado: m.estado, EstadoAnterior: m.estadoAnterior,
		Motivo: m.motivo, RemarcaDe: m.remarcaDe, CriadoEm: m.criadoEm, ActualizadoEm: m.actualizadoEm,
	}
}

// ReconstruirMarcacao reconstrói o agregado a partir de um snapshot persistido.
// EstadoAnterior é fixado no estado lido — qualquer transição posterior deixa-o a
// apontar para o estado que está na base de dados.
func ReconstruirMarcacao(s SnapshotMarcacao) *Marcacao {
	return &Marcacao{
		id: s.ID, doenteID: s.DoenteID, medicoID: s.MedicoID, especialidadeID: s.EspecialidadeID,
		inicio: s.Inicio, fim: s.Fim, estado: s.Estado, estadoAnterior: s.Estado,
		motivo: s.Motivo, remarcaDe: s.RemarcaDe, criadoEm: s.CriadoEm, actualizadoEm: s.ActualizadoEm,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/recepcao/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/recepcao/marcacao.go internal/domain/recepcao/marcacao_test.go
git commit -m "feat(recepcao): agregado Marcacao com maquina de estados e remarcacao por supersede"
```

---

## Task 3: Domínio — Função pura `VerificarDisponibilidade`

**Files:**
- Create: `internal/domain/recepcao/disponibilidade.go`
- Test: `internal/domain/recepcao/disponibilidade_test.go`

**Interfaces:**
- Consumes: `JanelaDisponibilidade`, `Marcacao` (Tasks 1–2).
- Produces: `func VerificarDisponibilidade(janelas []JanelaDisponibilidade, activas []Marcacao, especialidadeID string, inicio, fim, agora time.Time) error`.

- [ ] **Step 1: Write the failing test**

```go
// internal/domain/recepcao/disponibilidade_test.go
package recepcao_test

import (
	"testing"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func janela(esp, de, ate string) recepcao.JanelaDisponibilidade {
	j, _ := recepcao.NovaJanela("med-1", esp, inst(de), inst(ate))
	return *j
}

func marcada(de, ate string) recepcao.Marcacao {
	m, _ := recepcao.NovaMarcacao("doe-x", "med-1", "esp-1", inst(de), inst(ate))
	return *m
}

func TestVerificar_CabeNaJanela_SemMarcacoes(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-1", "08:00", "13:00")}
	err := recepcao.VerificarDisponibilidade(janelas, nil, "esp-1", inst("09:00"), inst("09:30"), inst("07:00"))
	if err != nil {
		t.Fatalf("devia aceitar: %v", err)
	}
}

func TestVerificar_ForaDeQualquerJanela_RegraNegocio(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-1", "08:00", "10:00")}
	err := recepcao.VerificarDisponibilidade(janelas, nil, "esp-1", inst("11:00"), inst("11:30"), inst("07:00"))
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio (fora de janela), veio %v", erros.CategoriaDe(err))
	}
}

func TestVerificar_JanelaDeOutraEspecialidade_NaoConta(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-2", "08:00", "13:00")}
	err := recepcao.VerificarDisponibilidade(janelas, nil, "esp-1", inst("09:00"), inst("09:30"), inst("07:00"))
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("janela de outra especialidade não devia servir; veio %v", erros.CategoriaDe(err))
	}
}

func TestVerificar_NoPassado_RegraNegocio(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-1", "08:00", "13:00")}
	// agora depois do início proposto
	err := recepcao.VerificarDisponibilidade(janelas, nil, "esp-1", inst("09:00"), inst("09:30"), inst("09:15"))
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio (passado), veio %v", erros.CategoriaDe(err))
	}
}

func TestVerificar_Sobreposicao_Conflito(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-1", "08:00", "13:00")}
	activas := []recepcao.Marcacao{marcada("09:00", "09:30")}
	// proposta 09:15-09:45 sobrepõe
	err := recepcao.VerificarDisponibilidade(janelas, activas, "esp-1", inst("09:15"), inst("09:45"), inst("07:00"))
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito (sobreposição), veio %v", erros.CategoriaDe(err))
	}
}

func TestVerificar_EncostoExacto_NaoESobreposicao(t *testing.T) {
	janelas := []recepcao.JanelaDisponibilidade{janela("esp-1", "08:00", "13:00")}
	activas := []recepcao.Marcacao{marcada("09:00", "09:30")}
	// proposta 09:30-10:00 encosta exactamente ao fim da anterior — permitido
	err := recepcao.VerificarDisponibilidade(janelas, activas, "esp-1", inst("09:30"), inst("10:00"), inst("07:00"))
	if err != nil {
		t.Fatalf("encosto exacto não devia ser conflito: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/recepcao/...`
Expected: FAIL — `undefined: recepcao.VerificarDisponibilidade`.

- [ ] **Step 3: Write the pure function**

```go
// internal/domain/recepcao/disponibilidade.go
package recepcao

import (
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// VerificarDisponibilidade é a invariante de negócio central da marcação. É uma função
// pura (sem I/O): o caso de uso alimenta-a com as janelas e as marcações activas lidas
// dos repositórios. Assume-se que `janelas` e `activas` são do mesmo médico da proposta.
//
// Verifica, por esta ordem:
//  1. a proposta não está no passado (início >= agora);
//  2. a proposta cabe inteira dentro de uma janela da mesma especialidade;
//  3. a proposta não sobrepõe nenhuma marcação activa (MARCADA) do médico.
//
// Encosto exacto (fim de uma == início da outra) NÃO é sobreposição.
func VerificarDisponibilidade(janelas []JanelaDisponibilidade, activas []Marcacao, especialidadeID string, inicio, fim, agora time.Time) error {
	if inicio.Before(agora) {
		return erros.Novo(erros.CategoriaRegraNegocio, "não é possível marcar no passado")
	}
	if !cabeNumaJanela(janelas, especialidadeID, inicio, fim) {
		return erros.Novo(erros.CategoriaRegraNegocio,
			"não há disponibilidade do médico para essa especialidade e horário")
	}
	for i := range activas {
		if activas[i].estado == MarcMarcada && seSobrepoe(inicio, fim, activas[i].inicio, activas[i].fim) {
			return erros.Novo(erros.CategoriaConflito,
				"o horário sobrepõe outra marcação do médico")
		}
	}
	return nil
}

// cabeNumaJanela indica se [inicio,fim] está inteiramente contido numa janela da
// especialidade dada.
func cabeNumaJanela(janelas []JanelaDisponibilidade, especialidadeID string, inicio, fim time.Time) bool {
	for i := range janelas {
		j := janelas[i]
		if j.especialidadeID != especialidadeID {
			continue
		}
		if !inicio.Before(j.inicio) && !fim.After(j.fim) {
			return true
		}
	}
	return false
}

// seSobrepoe indica se dois intervalos [aDe,aAte] e [bDe,bAte] se sobrepõem. Encosto
// exacto (aAte == bDe ou bAte == aDe) não conta como sobreposição.
func seSobrepoe(aDe, aAte, bDe, bAte time.Time) bool {
	return aDe.Before(bAte) && bDe.Before(aAte)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/recepcao/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/recepcao/disponibilidade.go internal/domain/recepcao/disponibilidade_test.go
git commit -m "feat(recepcao): funcao pura VerificarDisponibilidade (janela, sobreposicao, passado)"
```

---

## Task 4: Aplicação — ports, interfaces de repositório e casos de Janela

**Files:**
- Create: `internal/domain/recepcao/repositorio.go`
- Create: `internal/application/recepcao/ports.go`
- Create: `internal/application/recepcao/janelas.go`
- Create: `internal/application/recepcao/fakes_test.go`
- Test: `internal/application/recepcao/janelas_test.go`

**Interfaces:**
- Consumes: agregados e read-models do domínio (Tasks 1–3).
- Produces:
  - Domínio: `type RepositorioJanelas interface`, `type RepositorioMarcacoes interface`, `type ResumoJanela struct`, `type ResumoMarcacao struct`.
  - Aplicação: `type Auditor interface`, `type LeitorDoente interface`, `type DadosDefinirJanela struct`, `type DetalheJanela struct`, `NovoCasoDefinirJanela(RepositorioJanelas, Auditor) *CasoDefinirJanela` com `Executar(ctx, actor string, dados DadosDefinirJanela) (DetalheJanela, error)`; `NovoCasoRemoverJanela(RepositorioJanelas, RepositorioMarcacoes, Auditor) *CasoRemoverJanela` com `Executar(ctx, actor, janelaID string) error`.

- [ ] **Step 1: Write the domain repository interfaces + read-models**

```go
// internal/domain/recepcao/repositorio.go
package recepcao

import (
	"context"
	"time"
)

// ResumoJanela é a projecção de leitura de uma janela de disponibilidade.
type ResumoJanela struct {
	ID              string    `json:"id"`
	MedicoID        string    `json:"medico_id"`
	EspecialidadeID string    `json:"especialidade_id"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
}

// ResumoMarcacao é a projecção de leitura de uma marcação.
type ResumoMarcacao struct {
	ID              string    `json:"id"`
	DoenteID        string    `json:"doente_id"`
	MedicoID        string    `json:"medico_id"`
	EspecialidadeID string    `json:"especialidade_id"`
	Estado          string    `json:"estado"`
	Motivo          string    `json:"motivo,omitempty"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
	CriadoEm        time.Time `json:"criado_em"`
}

// RepositorioJanelas é a porta de saída de persistência de janelas de disponibilidade.
// ListarPorMedicoIntervalo devolve as janelas do médico que SE SOBREPÕEM ao intervalo
// [de,ate] (não apenas as inteiramente contidas): é sobre essas que
// VerificarDisponibilidade decide o encaixe.
type RepositorioJanelas interface {
	Guardar(ctx context.Context, j *JanelaDisponibilidade) (string, error)
	ObterPorID(ctx context.Context, id string) (*JanelaDisponibilidade, error)
	ListarPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]JanelaDisponibilidade, error)
	Remover(ctx context.Context, id string) error
}

// RepositorioMarcacoes é a porta de saída de persistência de marcações.
//
// Transitar aplica a transição de estado com guarda compare-and-set (usa
// EstadoAnterior do snapshot). Remarcar grava, numa única transacção, a original a
// passar a REMARCADA e a nova MARCADA — uma marcação remarcada sem a nova deixaria o
// doente sem consulta.
//
// ListarActivasPorMedicoIntervalo devolve os agregados das marcações MARCADA do médico
// que se sobrepõem ao intervalo (para VerificarDisponibilidade). ListarPorMedicoIntervalo
// e ListarPorDoente devolvem read-models de TODOS os estados (para a agenda/consulta).
type RepositorioMarcacoes interface {
	Guardar(ctx context.Context, m *Marcacao) (string, error)
	ObterPorID(ctx context.Context, id string) (*Marcacao, error)
	Transitar(ctx context.Context, m *Marcacao) error
	Remarcar(ctx context.Context, original, nova *Marcacao) (string, error)
	ListarActivasPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]Marcacao, error)
	ListarPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]ResumoMarcacao, error)
	ListarPorDoente(ctx context.Context, doenteID string) ([]ResumoMarcacao, error)
}
```

- [ ] **Step 2: Write the application ports**

```go
// Package recepcao contém os casos de uso do BC Recepção (Camada 2 — Aplicação).
package recepcao

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// Auditor persiste registos de auditoria de forma append-only.
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}

// LeitorDoente é a porta anti-corrupção para leitura do BC Clínico. A Recepção nunca
// importa tipos do domínio Clínico: só faz esta pergunta booleana.
type LeitorDoente interface {
	// DoenteActivo indica se o doente existe e está activo.
	DoenteActivo(ctx context.Context, doenteID string) (bool, error)
}

// Reexports dos read-models do domínio.
type (
	ResumoJanela   = dominio.ResumoJanela
	ResumoMarcacao = dominio.ResumoMarcacao
)

// DadosDefinirJanela é a entrada da definição de uma janela. O MedicoID vem do
// caminho (:mid); a especialidade e o intervalo vêm do corpo.
type DadosDefinirJanela struct {
	MedicoID        string
	EspecialidadeID string    `json:"especialidade_id"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
}

// DetalheJanela é o detalhe de uma janela numa resposta.
type DetalheJanela struct {
	ID              string    `json:"id"`
	MedicoID        string    `json:"medico_id"`
	EspecialidadeID string    `json:"especialidade_id"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
}

// DadosMarcar é a entrada de uma marcação. Todos os ids vêm do corpo; o actor
// (quem marca) vem da sessão, não daqui.
type DadosMarcar struct {
	DoenteID        string    `json:"doente_id"`
	MedicoID        string    `json:"medico_id"`
	EspecialidadeID string    `json:"especialidade_id"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
}

// DadosRemarcar é a entrada de uma remarcação (novo intervalo).
type DadosRemarcar struct {
	Inicio time.Time `json:"inicio"`
	Fim    time.Time `json:"fim"`
}

// DetalheMarcacao é o detalhe de uma marcação numa resposta.
type DetalheMarcacao struct {
	ID              string    `json:"id"`
	DoenteID        string    `json:"doente_id"`
	MedicoID        string    `json:"medico_id"`
	EspecialidadeID string    `json:"especialidade_id"`
	Estado          string    `json:"estado"`
	Motivo          string    `json:"motivo,omitempty"`
	RemarcaDe       string    `json:"remarca_de,omitempty"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
}

// Agenda é a leitura combinada da agenda de um médico num intervalo.
type Agenda struct {
	Janelas   []DetalheJanela  `json:"janelas"`
	Marcacoes []ResumoMarcacao `json:"marcacoes"`
}

// paraDetalheJanela projecta o agregado para o read-model de resposta.
func paraDetalheJanela(j *dominio.JanelaDisponibilidade) DetalheJanela {
	s := j.Snapshot()
	return DetalheJanela{
		ID: s.ID, MedicoID: s.MedicoID, EspecialidadeID: s.EspecialidadeID,
		Inicio: s.Inicio, Fim: s.Fim,
	}
}

// paraDetalheMarcacao projecta o agregado para o read-model de resposta.
func paraDetalheMarcacao(m *dominio.Marcacao) DetalheMarcacao {
	s := m.Snapshot()
	return DetalheMarcacao{
		ID: s.ID, DoenteID: s.DoenteID, MedicoID: s.MedicoID, EspecialidadeID: s.EspecialidadeID,
		Estado: string(s.Estado), Motivo: s.Motivo, RemarcaDe: s.RemarcaDe,
		Inicio: s.Inicio, Fim: s.Fim,
	}
}
```

**Nota ao implementador:** este ficheiro é `internal/application/recepcao/ports.go`. As funções `paraDetalheJanela`/`paraDetalheMarcacao` ficam aqui e são consumidas pelas Tasks 4–6.

- [ ] **Step 3: Write the fakes (shared test doubles)**

```go
// internal/application/recepcao/fakes_test.go
package recepcao_test

import (
	"context"
	"time"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func inst(hhmm string) time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-07-20T"+hhmm+":00Z")
	return t
}

// agoraFixo devolve um relógio de teste fixo.
func agoraFixo(hhmm string) func() time.Time {
	return func() time.Time { return inst(hhmm) }
}

// fakeJanelas guarda janelas em memória, indexadas por id.
type fakeJanelas struct {
	dados    map[string]*dominio.JanelaDisponibilidade
	seq      int
	removido []string
}

func novoFakeJanelas() *fakeJanelas {
	return &fakeJanelas{dados: map[string]*dominio.JanelaDisponibilidade{}}
}

func (f *fakeJanelas) Guardar(_ context.Context, j *dominio.JanelaDisponibilidade) (string, error) {
	f.seq++
	id := "jan-" + itoa(f.seq)
	s := j.Snapshot()
	s.ID = id
	f.dados[id] = dominio.ReconstruirJanela(s)
	return id, nil
}

func (f *fakeJanelas) ObterPorID(_ context.Context, id string) (*dominio.JanelaDisponibilidade, error) {
	j, ok := f.dados[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "janela não encontrada")
	}
	return j, nil
}

func (f *fakeJanelas) ListarPorMedicoIntervalo(_ context.Context, medicoID string, de, ate time.Time) ([]dominio.JanelaDisponibilidade, error) {
	var out []dominio.JanelaDisponibilidade
	for _, j := range f.dados {
		if j.MedicoID() == medicoID && j.Inicio().Before(ate) && de.Before(j.Fim()) {
			out = append(out, *j)
		}
	}
	return out, nil
}

func (f *fakeJanelas) Remover(_ context.Context, id string) error {
	if _, ok := f.dados[id]; !ok {
		return erros.Novo(erros.CategoriaNaoEncontrado, "janela não encontrada")
	}
	delete(f.dados, id)
	f.removido = append(f.removido, id)
	return nil
}

// fakeMarcacoes guarda marcações em memória.
type fakeMarcacoes struct {
	dados map[string]*dominio.Marcacao
	seq   int
}

func novoFakeMarcacoes() *fakeMarcacoes {
	return &fakeMarcacoes{dados: map[string]*dominio.Marcacao{}}
}

func (f *fakeMarcacoes) Guardar(_ context.Context, m *dominio.Marcacao) (string, error) {
	f.seq++
	id := "marc-" + itoa(f.seq)
	s := m.Snapshot()
	s.ID = id
	f.dados[id] = dominio.ReconstruirMarcacao(s)
	return id, nil
}

func (f *fakeMarcacoes) ObterPorID(_ context.Context, id string) (*dominio.Marcacao, error) {
	m, ok := f.dados[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
	}
	// devolve uma cópia rehidratada (EstadoAnterior fixado no estado persistido)
	return dominio.ReconstruirMarcacao(m.Snapshot()), nil
}

func (f *fakeMarcacoes) Transitar(_ context.Context, m *dominio.Marcacao) error {
	s := m.Snapshot()
	cur, ok := f.dados[s.ID]
	if !ok {
		return erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
	}
	if cur.Estado() != s.EstadoAnterior {
		return erros.Novo(erros.CategoriaConflito, "o estado da marcação mudou entretanto")
	}
	f.dados[s.ID] = dominio.ReconstruirMarcacao(s)
	return nil
}

func (f *fakeMarcacoes) Remarcar(_ context.Context, original, nova *dominio.Marcacao) (string, error) {
	so := original.Snapshot()
	cur, ok := f.dados[so.ID]
	if !ok {
		return "", erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
	}
	if cur.Estado() != so.EstadoAnterior {
		return "", erros.Novo(erros.CategoriaConflito, "o estado da marcação mudou entretanto")
	}
	f.dados[so.ID] = dominio.ReconstruirMarcacao(so)
	return f.Guardar(context.Background(), nova)
}

func (f *fakeMarcacoes) ListarActivasPorMedicoIntervalo(_ context.Context, medicoID string, de, ate time.Time) ([]dominio.Marcacao, error) {
	var out []dominio.Marcacao
	for _, m := range f.dados {
		if m.MedicoID() == medicoID && m.Estado() == dominio.MarcMarcada &&
			m.Inicio().Before(ate) && de.Before(m.Fim()) {
			out = append(out, *dominio.ReconstruirMarcacao(m.Snapshot()))
		}
	}
	return out, nil
}

func (f *fakeMarcacoes) ListarPorMedicoIntervalo(_ context.Context, medicoID string, de, ate time.Time) ([]dominio.ResumoMarcacao, error) {
	var out []dominio.ResumoMarcacao
	for _, m := range f.dados {
		if m.MedicoID() == medicoID && m.Inicio().Before(ate) && de.Before(m.Fim()) {
			out = append(out, resumo(m))
		}
	}
	return out, nil
}

func (f *fakeMarcacoes) ListarPorDoente(_ context.Context, doenteID string) ([]dominio.ResumoMarcacao, error) {
	var out []dominio.ResumoMarcacao
	for _, m := range f.dados {
		if m.DoenteID() == doenteID {
			out = append(out, resumo(m))
		}
	}
	return out, nil
}

func resumo(m *dominio.Marcacao) dominio.ResumoMarcacao {
	s := m.Snapshot()
	return dominio.ResumoMarcacao{
		ID: s.ID, DoenteID: s.DoenteID, MedicoID: s.MedicoID, EspecialidadeID: s.EspecialidadeID,
		Estado: string(s.Estado), Motivo: s.Motivo, Inicio: s.Inicio, Fim: s.Fim, CriadoEm: s.CriadoEm,
	}
}

// fakeLeitorDoente responde à ACL sobre o Clínico.
type fakeLeitorDoente struct {
	activos map[string]bool
	erro    error
}

func (f fakeLeitorDoente) DoenteActivo(_ context.Context, doenteID string) (bool, error) {
	if f.erro != nil {
		return false, f.erro
	}
	return f.activos[doenteID], nil
}

// fakeAuditor acumula os registos e permite consultá-los por acção.
type fakeAuditor struct {
	registos []auditoria.Registo
}

func (f *fakeAuditor) Registar(_ context.Context, r auditoria.Registo) error {
	f.registos = append(f.registos, r)
	return nil
}

func (f *fakeAuditor) tem(accao string) bool {
	for _, r := range f.registos {
		if r.Accao == accao {
			return true
		}
	}
	return false
}

// itoa é um Itoa mínimo para evitar importar strconv nos fakes.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Garantias de conformidade com as portas.
var (
	_ dominio.RepositorioJanelas   = (*fakeJanelas)(nil)
	_ dominio.RepositorioMarcacoes = (*fakeMarcacoes)(nil)
	_ app.LeitorDoente             = fakeLeitorDoente{}
	_ app.Auditor                  = (*fakeAuditor)(nil)
)
```

- [ ] **Step 4: Write the failing test for the Janela use cases**

```go
// internal/application/recepcao/janelas_test.go
package recepcao_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestDefinirJanela_CriaEAudita(t *testing.T) {
	janelas := novoFakeJanelas()
	aud := &fakeAuditor{}
	uc := app.NovoCasoDefinirJanela(janelas, aud)

	out, err := uc.Executar(context.Background(), "adm-1", app.DadosDefinirJanela{
		MedicoID: "med-1", EspecialidadeID: "esp-1", Inicio: inst("08:00"), Fim: inst("13:00"),
	})
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.ID == "" || out.MedicoID != "med-1" {
		t.Fatalf("detalhe mal preenchido: %+v", out)
	}
	if !aud.tem("recepcao.janela.definida") {
		t.Fatal("esperava auditoria recepcao.janela.definida")
	}
}

func TestDefinirJanela_IntervaloInvalido_Erro(t *testing.T) {
	uc := app.NovoCasoDefinirJanela(novoFakeJanelas(), &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "adm-1", app.DadosDefinirJanela{
		MedicoID: "med-1", EspecialidadeID: "esp-1", Inicio: inst("13:00"), Fim: inst("08:00"),
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestRemoverJanela_SemMarcacoes_RemoveEAudita(t *testing.T) {
	janelas := novoFakeJanelas()
	marc := novoFakeMarcacoes()
	aud := &fakeAuditor{}
	id, _ := janelas.Guardar(context.Background(), janelaAgregada(t, "med-1", "esp-1", "08:00", "13:00"))

	uc := app.NovoCasoRemoverJanela(janelas, marc, aud)
	if err := uc.Executar(context.Background(), "adm-1", id); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if _, err := janelas.ObterPorID(context.Background(), id); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatal("a janela devia ter sido removida")
	}
	if !aud.tem("recepcao.janela.removida") {
		t.Fatal("esperava auditoria recepcao.janela.removida")
	}
}

func TestRemoverJanela_ComMarcacaoActiva_Conflito(t *testing.T) {
	janelas := novoFakeJanelas()
	marc := novoFakeMarcacoes()
	id, _ := janelas.Guardar(context.Background(), janelaAgregada(t, "med-1", "esp-1", "08:00", "13:00"))
	// marcação activa dentro da janela
	_, _ = marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-1", "med-1", "esp-1", "09:00", "09:30"))

	uc := app.NovoCasoRemoverJanela(janelas, marc, &fakeAuditor{})
	if err := uc.Executar(context.Background(), "adm-1", id); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}
```

**Nota:** `janelaAgregada` e `marcacaoAgregada` são helpers construídos a partir dos construtores do domínio; adicioná-los ao `fakes_test.go`:

```go
// (adicionar ao fim de internal/application/recepcao/fakes_test.go)
import (
	// já importado acima: dominio, testing
)

func janelaAgregada(t *testing.T, medico, esp, de, ate string) *dominio.JanelaDisponibilidade {
	t.Helper()
	j, err := dominio.NovaJanela(medico, esp, inst(de), inst(ate))
	if err != nil {
		t.Fatalf("janela inválida no teste: %v", err)
	}
	return j
}

func marcacaoAgregada(t *testing.T, doe, medico, esp, de, ate string) *dominio.Marcacao {
	t.Helper()
	m, err := dominio.NovaMarcacao(doe, medico, esp, inst(de), inst(ate))
	if err != nil {
		t.Fatalf("marcação inválida no teste: %v", err)
	}
	return m
}
```

(Coloca estes dois helpers no `fakes_test.go`, não repitas o bloco `import` — o ficheiro já importa `testing` e `dominio`.)

- [ ] **Step 5: Run test to verify it fails**

Run: `go test ./internal/application/recepcao/...`
Expected: FAIL — `undefined: app.NovoCasoDefinirJanela`.

- [ ] **Step 6: Write the Janela use cases**

```go
// internal/application/recepcao/janelas.go
package recepcao

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoDefinirJanela cria uma janela de disponibilidade de um médico.
type CasoDefinirJanela struct {
	janelas dominio.RepositorioJanelas
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoDefinirJanela constrói o caso de uso.
func NovoCasoDefinirJanela(j dominio.RepositorioJanelas, a Auditor) *CasoDefinirJanela {
	return &CasoDefinirJanela{janelas: j, auditor: a, agora: time.Now}
}

// Executar valida e persiste a janela, e audita. O actor (quem define) é o sujeito
// autenticado.
func (uc *CasoDefinirJanela) Executar(ctx context.Context, actor string, dados DadosDefinirJanela) (DetalheJanela, error) {
	j, err := dominio.NovaJanela(dados.MedicoID, dados.EspecialidadeID, dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheJanela{}, err
	}
	id, err := uc.janelas.Guardar(ctx, j)
	if err != nil {
		return DetalheJanela{}, err
	}
	j = dominio.ReconstruirJanela(comIDJanela(j.Snapshot(), id))
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.janela.definida",
		Entidade: "janela", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheJanela{}, err
	}
	return paraDetalheJanela(j), nil
}

// CasoRemoverJanela remove uma janela, desde que não tenha marcações activas dentro.
type CasoRemoverJanela struct {
	janelas   dominio.RepositorioJanelas
	marcacoes dominio.RepositorioMarcacoes
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoRemoverJanela constrói o caso de uso.
func NovoCasoRemoverJanela(j dominio.RepositorioJanelas, m dominio.RepositorioMarcacoes, a Auditor) *CasoRemoverJanela {
	return &CasoRemoverJanela{janelas: j, marcacoes: m, auditor: a, agora: time.Now}
}

// Executar remove a janela se não houver marcações MARCADA no seu intervalo. Remover
// uma janela com marcações activas deixaria consultas sem cobertura de agenda.
func (uc *CasoRemoverJanela) Executar(ctx context.Context, actor, janelaID string) error {
	j, err := uc.janelas.ObterPorID(ctx, janelaID)
	if err != nil {
		return err
	}
	activas, err := uc.marcacoes.ListarActivasPorMedicoIntervalo(ctx, j.MedicoID(), j.Inicio(), j.Fim())
	if err != nil {
		return err
	}
	if len(activas) > 0 {
		return erros.Novo(erros.CategoriaConflito,
			"a janela tem marcações activas e não pode ser removida")
	}
	if err := uc.janelas.Remover(ctx, janelaID); err != nil {
		return err
	}
	return uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.janela.removida",
		Entidade: "janela", EntidadeID: janelaID, OcorridoEm: uc.agora(),
	})
}

// comIDJanela devolve uma cópia do snapshot com o id preenchido.
func comIDJanela(s dominio.SnapshotJanela, id string) dominio.SnapshotJanela {
	s.ID = id
	return s
}
```

- [ ] **Step 7: Run test to verify it passes**

Run: `go test ./internal/application/recepcao/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/recepcao/repositorio.go internal/application/recepcao/ports.go internal/application/recepcao/janelas.go internal/application/recepcao/fakes_test.go internal/application/recepcao/janelas_test.go
git commit -m "feat(recepcao): ports, interfaces de repositorio e casos DefinirJanela/RemoverJanela"
```

---

## Task 5: Aplicação — casos de Marcação (marcar, remarcar, cancelar, falta)

**Files:**
- Create: `internal/application/recepcao/marcacoes.go`
- Test: `internal/application/recepcao/marcacoes_test.go`

**Interfaces:**
- Consumes: `RepositorioJanelas`, `RepositorioMarcacoes`, `LeitorDoente`, `Auditor`, `VerificarDisponibilidade`, `paraDetalheMarcacao`, DTOs `DadosMarcar/DadosRemarcar/DetalheMarcacao`.
- Produces: `NovoCasoMarcar(RepositorioMarcacoes, RepositorioJanelas, LeitorDoente, Auditor) *CasoMarcar` (`Executar(ctx, actor string, dados DadosMarcar) (DetalheMarcacao, error)`); `NovoCasoRemarcar(RepositorioMarcacoes, RepositorioJanelas, Auditor) *CasoRemarcar` (`Executar(ctx, actor, marcacaoID string, dados DadosRemarcar) (DetalheMarcacao, error)`); `NovoCasoCancelar(RepositorioMarcacoes, Auditor) *CasoCancelar` (`Executar(ctx, actor, marcacaoID, motivo string) (DetalheMarcacao, error)`); `NovoCasoRegistarFalta(RepositorioMarcacoes, Auditor) *CasoRegistarFalta` (`Executar(ctx, actor, marcacaoID string) (DetalheMarcacao, error)`).

- [ ] **Step 1: Write the failing test**

```go
// internal/application/recepcao/marcacoes_test.go
package recepcao_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func cenarioComJanela(t *testing.T) (*fakeJanelas, *fakeMarcacoes) {
	t.Helper()
	janelas := novoFakeJanelas()
	marc := novoFakeMarcacoes()
	_, _ = janelas.Guardar(context.Background(), janelaAgregada(t, "med-1", "esp-1", "08:00", "13:00"))
	return janelas, marc
}

func TestMarcar_DentroDaJanela_CriaEAudita(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	aud := &fakeAuditor{}
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	uc := app.NovoCasoMarcar(marc, janelas, leitor, aud)
	uc.DefinirRelogio(agoraFixo("07:00"))

	out, err := uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.ID == "" || out.Estado != string(dominio.MarcMarcada) {
		t.Fatalf("detalhe mal preenchido: %+v", out)
	}
	if !aud.tem("recepcao.marcacao.criada") {
		t.Fatal("esperava auditoria recepcao.marcacao.criada")
	}
}

func TestMarcar_DoenteInactivo_RegraNegocio(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{}} // doe-1 não activo
	uc := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))

	_, err := uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio (doente inactivo), veio %v", erros.CategoriaDe(err))
	}
}

func TestMarcar_ForaDaJanela_RegraNegocio(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	uc := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))

	_, err := uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("14:00"), Fim: inst("14:30"), // fora da janela 08-13
	})
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio (fora de janela), veio %v", erros.CategoriaDe(err))
	}
}

func TestMarcar_Sobreposicao_Conflito(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true, "doe-2": true}}
	uc := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("07:00"))
	_, _ = uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})
	_, err := uc.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-2", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:15"), Fim: inst("09:45"),
	})
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestRemarcar_SupersedeEAudita(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	marcar := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	marcar.DefinirRelogio(agoraFixo("07:00"))
	criada, _ := marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})

	aud := &fakeAuditor{}
	uc := app.NovoCasoRemarcar(marc, janelas, aud)
	uc.DefinirRelogio(agoraFixo("07:00"))
	nova, err := uc.Executar(context.Background(), "adm-1", criada.ID, app.DadosRemarcar{
		Inicio: inst("10:00"), Fim: inst("10:30"),
	})
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if nova.RemarcaDe != criada.ID {
		t.Fatalf("a nova devia apontar para %s, veio %q", criada.ID, nova.RemarcaDe)
	}
	original, _ := marc.ObterPorID(context.Background(), criada.ID)
	if original.Estado() != dominio.MarcRemarcada {
		t.Fatalf("a original devia estar REMARCADA, veio %s", original.Estado())
	}
	if !aud.tem("recepcao.marcacao.remarcada") {
		t.Fatal("esperava auditoria recepcao.marcacao.remarcada")
	}
}

func TestCancelar_ComMotivo_Audita(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	marcar := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	marcar.DefinirRelogio(agoraFixo("07:00"))
	criada, _ := marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})

	aud := &fakeAuditor{}
	uc := app.NovoCasoCancelar(marc, aud)
	uc.DefinirRelogio(agoraFixo("07:00"))
	out, err := uc.Executar(context.Background(), "adm-1", criada.ID, "doente desistiu")
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.Estado != string(dominio.MarcCancelada) {
		t.Fatalf("esperava CANCELADA, veio %s", out.Estado)
	}
	if !aud.tem("recepcao.marcacao.cancelada") {
		t.Fatal("esperava auditoria recepcao.marcacao.cancelada")
	}
}

func TestRegistarFalta_AposHora_Audita(t *testing.T) {
	janelas, marc := cenarioComJanela(t)
	leitor := fakeLeitorDoente{activos: map[string]bool{"doe-1": true}}
	marcar := app.NovoCasoMarcar(marc, janelas, leitor, &fakeAuditor{})
	marcar.DefinirRelogio(agoraFixo("07:00"))
	criada, _ := marcar.Executar(context.Background(), "adm-1", app.DadosMarcar{
		DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"),
	})

	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarFalta(marc, aud)
	uc.DefinirRelogio(agoraFixo("10:00")) // depois do fim
	out, err := uc.Executar(context.Background(), "adm-1", criada.ID)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.Estado != string(dominio.MarcFaltou) {
		t.Fatalf("esperava FALTOU, veio %s", out.Estado)
	}
	if !aud.tem("recepcao.marcacao.faltou") {
		t.Fatal("esperava auditoria recepcao.marcacao.faltou")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/recepcao/...`
Expected: FAIL — `undefined: app.NovoCasoMarcar`.

- [ ] **Step 3: Write the use cases**

```go
// internal/application/recepcao/marcacoes.go
package recepcao

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoMarcar cria uma marcação dentro de uma janela livre.
type CasoMarcar struct {
	marcacoes dominio.RepositorioMarcacoes
	janelas   dominio.RepositorioJanelas
	doentes   LeitorDoente
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoMarcar constrói o caso de uso.
func NovoCasoMarcar(m dominio.RepositorioMarcacoes, j dominio.RepositorioJanelas, d LeitorDoente, a Auditor) *CasoMarcar {
	return &CasoMarcar{marcacoes: m, janelas: j, doentes: d, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste (usado só em testes).
func (uc *CasoMarcar) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar valida o doente (ACL), verifica a disponibilidade e persiste a marcação. O
// actor (quem marca) é o sujeito autenticado e vai no registo de auditoria — não é
// guardado na marcação.
func (uc *CasoMarcar) Executar(ctx context.Context, actor string, dados DadosMarcar) (DetalheMarcacao, error) {
	activo, err := uc.doentes.DoenteActivo(ctx, dados.DoenteID)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	if !activo {
		return DetalheMarcacao{}, erros.Novo(erros.CategoriaRegraNegocio,
			"o doente não existe ou não está activo")
	}
	m, err := dominio.NovaMarcacao(dados.DoenteID, dados.MedicoID, dados.EspecialidadeID, dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	janelas, err := uc.janelas.ListarPorMedicoIntervalo(ctx, dados.MedicoID, dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	activas, err := uc.marcacoes.ListarActivasPorMedicoIntervalo(ctx, dados.MedicoID, dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	if err := dominio.VerificarDisponibilidade(janelas, activas, dados.EspecialidadeID, dados.Inicio, dados.Fim, uc.agora()); err != nil {
		return DetalheMarcacao{}, err
	}
	id, err := uc.marcacoes.Guardar(ctx, m)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	m = dominio.ReconstruirMarcacao(comIDMarcacao(m.Snapshot(), id))
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.marcacao.criada",
		Entidade: "marcacao", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheMarcacao{}, err
	}
	return paraDetalheMarcacao(m), nil
}

// CasoRemarcar remarca uma marcação para um novo intervalo, preservando a original.
type CasoRemarcar struct {
	marcacoes dominio.RepositorioMarcacoes
	janelas   dominio.RepositorioJanelas
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoRemarcar constrói o caso de uso.
func NovoCasoRemarcar(m dominio.RepositorioMarcacoes, j dominio.RepositorioJanelas, a Auditor) *CasoRemarcar {
	return &CasoRemarcar{marcacoes: m, janelas: j, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoRemarcar) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar transita a original para REMARCADA e cria uma nova MARCADA no novo horário,
// numa única transacção. A disponibilidade do novo horário é verificada excluindo a
// própria original (senão a marcação colidiria consigo mesma).
func (uc *CasoRemarcar) Executar(ctx context.Context, actor, marcacaoID string, dados DadosRemarcar) (DetalheMarcacao, error) {
	original, err := uc.marcacoes.ObterPorID(ctx, marcacaoID)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	nova, err := original.Remarcar(dados.Inicio, dados.Fim, uc.agora())
	if err != nil {
		return DetalheMarcacao{}, err
	}
	janelas, err := uc.janelas.ListarPorMedicoIntervalo(ctx, nova.MedicoID(), dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	activas, err := uc.marcacoes.ListarActivasPorMedicoIntervalo(ctx, nova.MedicoID(), dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	activas = semAMarcacao(activas, marcacaoID)
	if err := dominio.VerificarDisponibilidade(janelas, activas, nova.EspecialidadeID(), dados.Inicio, dados.Fim, uc.agora()); err != nil {
		return DetalheMarcacao{}, err
	}
	novoID, err := uc.marcacoes.Remarcar(ctx, original, nova)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	nova = dominio.ReconstruirMarcacao(comIDMarcacao(nova.Snapshot(), novoID))
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.marcacao.remarcada",
		Entidade: "marcacao", EntidadeID: novoID, OcorridoEm: uc.agora(),
		Detalhe: "remarca_de: " + marcacaoID,
	}); err != nil {
		return DetalheMarcacao{}, err
	}
	return paraDetalheMarcacao(nova), nil
}

// CasoCancelar cancela uma marcação com motivo.
type CasoCancelar struct {
	marcacoes dominio.RepositorioMarcacoes
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoCancelar constrói o caso de uso.
func NovoCasoCancelar(m dominio.RepositorioMarcacoes, a Auditor) *CasoCancelar {
	return &CasoCancelar{marcacoes: m, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoCancelar) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar cancela a marcação e audita (o motivo vai no detalhe: um cancelamento sem
// razão registada não é auditável).
func (uc *CasoCancelar) Executar(ctx context.Context, actor, marcacaoID, motivo string) (DetalheMarcacao, error) {
	m, err := uc.marcacoes.ObterPorID(ctx, marcacaoID)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	if err := m.Cancelar(motivo, uc.agora()); err != nil {
		return DetalheMarcacao{}, err
	}
	if err := uc.marcacoes.Transitar(ctx, m); err != nil {
		return DetalheMarcacao{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.marcacao.cancelada",
		Entidade: "marcacao", EntidadeID: marcacaoID, OcorridoEm: uc.agora(),
		Detalhe: "motivo: " + motivo,
	}); err != nil {
		return DetalheMarcacao{}, err
	}
	return paraDetalheMarcacao(m), nil
}

// CasoRegistarFalta regista a falta (no-show) de uma marcação.
type CasoRegistarFalta struct {
	marcacoes dominio.RepositorioMarcacoes
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoRegistarFalta constrói o caso de uso.
func NovoCasoRegistarFalta(m dominio.RepositorioMarcacoes, a Auditor) *CasoRegistarFalta {
	return &CasoRegistarFalta{marcacoes: m, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoRegistarFalta) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar regista a falta (só depois da hora, invariante do agregado) e audita.
func (uc *CasoRegistarFalta) Executar(ctx context.Context, actor, marcacaoID string) (DetalheMarcacao, error) {
	m, err := uc.marcacoes.ObterPorID(ctx, marcacaoID)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	if err := m.RegistarFalta(uc.agora()); err != nil {
		return DetalheMarcacao{}, err
	}
	if err := uc.marcacoes.Transitar(ctx, m); err != nil {
		return DetalheMarcacao{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.marcacao.faltou",
		Entidade: "marcacao", EntidadeID: marcacaoID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheMarcacao{}, err
	}
	return paraDetalheMarcacao(m), nil
}

// comIDMarcacao devolve uma cópia do snapshot com o id preenchido.
func comIDMarcacao(s dominio.SnapshotMarcacao, id string) dominio.SnapshotMarcacao {
	s.ID = id
	return s
}

// semAMarcacao devolve a lista sem a marcação de id dado (usada na remarcação para não
// contar a própria original como sobreposição).
func semAMarcacao(activas []dominio.Marcacao, id string) []dominio.Marcacao {
	out := activas[:0]
	for i := range activas {
		if activas[i].ID() != id {
			out = append(out, activas[i])
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/application/recepcao/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/application/recepcao/marcacoes.go internal/application/recepcao/marcacoes_test.go
git commit -m "feat(recepcao): casos Marcar, Remarcar, Cancelar e RegistarFalta"
```

---

## Task 6: Aplicação — consultas de leitura (agenda e marcações do doente)

**Files:**
- Create: `internal/application/recepcao/consultas.go`
- Test: `internal/application/recepcao/consultas_test.go`

**Interfaces:**
- Consumes: `RepositorioJanelas`, `RepositorioMarcacoes`, `Agenda`, `paraDetalheJanela`.
- Produces: `NovoCasoListarAgenda(RepositorioJanelas, RepositorioMarcacoes) *CasoListarAgenda` (`Executar(ctx, medicoID string, de, ate time.Time) (Agenda, error)`); `NovoCasoListarMarcacoesDoente(RepositorioMarcacoes) *CasoListarMarcacoesDoente` (`Executar(ctx, doenteID string) ([]ResumoMarcacao, error)`).

- [ ] **Step 1: Write the failing test**

```go
// internal/application/recepcao/consultas_test.go
package recepcao_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
)

func TestListarAgenda_DevolveJanelasEMarcacoes(t *testing.T) {
	janelas := novoFakeJanelas()
	marc := novoFakeMarcacoes()
	_, _ = janelas.Guardar(context.Background(), janelaAgregada(t, "med-1", "esp-1", "08:00", "13:00"))
	_, _ = marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-1", "med-1", "esp-1", "09:00", "09:30"))

	uc := app.NovoCasoListarAgenda(janelas, marc)
	ag, err := uc.Executar(context.Background(), "med-1", inst("00:00"), inst("23:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if len(ag.Janelas) != 1 || len(ag.Marcacoes) != 1 {
		t.Fatalf("esperava 1 janela e 1 marcação, veio %d/%d", len(ag.Janelas), len(ag.Marcacoes))
	}
}

func TestListarMarcacoesDoente(t *testing.T) {
	marc := novoFakeMarcacoes()
	_, _ = marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-1", "med-1", "esp-1", "09:00", "09:30"))
	_, _ = marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-2", "med-1", "esp-1", "10:00", "10:30"))

	uc := app.NovoCasoListarMarcacoesDoente(marc)
	out, err := uc.Executar(context.Background(), "doe-1")
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if len(out) != 1 || out[0].DoenteID != "doe-1" {
		t.Fatalf("esperava só as marcações do doe-1, veio %+v", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/recepcao/...`
Expected: FAIL — `undefined: app.NovoCasoListarAgenda`.

- [ ] **Step 3: Write the use cases**

```go
// internal/application/recepcao/consultas.go
package recepcao

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
)

// CasoListarAgenda lê a agenda de um médico num intervalo: janelas de disponibilidade
// e marcações (de todos os estados).
type CasoListarAgenda struct {
	janelas   dominio.RepositorioJanelas
	marcacoes dominio.RepositorioMarcacoes
}

// NovoCasoListarAgenda constrói o caso de uso.
func NovoCasoListarAgenda(j dominio.RepositorioJanelas, m dominio.RepositorioMarcacoes) *CasoListarAgenda {
	return &CasoListarAgenda{janelas: j, marcacoes: m}
}

// Executar devolve a agenda do médico entre `de` e `ate`.
func (uc *CasoListarAgenda) Executar(ctx context.Context, medicoID string, de, ate time.Time) (Agenda, error) {
	js, err := uc.janelas.ListarPorMedicoIntervalo(ctx, medicoID, de, ate)
	if err != nil {
		return Agenda{}, err
	}
	ms, err := uc.marcacoes.ListarPorMedicoIntervalo(ctx, medicoID, de, ate)
	if err != nil {
		return Agenda{}, err
	}
	detalhes := make([]DetalheJanela, 0, len(js))
	for i := range js {
		detalhes = append(detalhes, paraDetalheJanela(&js[i]))
	}
	return Agenda{Janelas: detalhes, Marcacoes: ms}, nil
}

// CasoListarMarcacoesDoente lê as marcações de um doente.
type CasoListarMarcacoesDoente struct {
	marcacoes dominio.RepositorioMarcacoes
}

// NovoCasoListarMarcacoesDoente constrói o caso de uso.
func NovoCasoListarMarcacoesDoente(m dominio.RepositorioMarcacoes) *CasoListarMarcacoesDoente {
	return &CasoListarMarcacoesDoente{marcacoes: m}
}

// Executar devolve as marcações do doente.
func (uc *CasoListarMarcacoesDoente) Executar(ctx context.Context, doenteID string) ([]ResumoMarcacao, error) {
	return uc.marcacoes.ListarPorDoente(ctx, doenteID)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/application/recepcao/...`
Expected: PASS.

- [ ] **Step 5: Verify application coverage ≥75%**

Run: `go test -cover ./internal/application/recepcao/...`
Expected: PASS com cobertura ≥ 75.0%.

- [ ] **Step 6: Commit**

```bash
git add internal/application/recepcao/consultas.go internal/application/recepcao/consultas_test.go
git commit -m "feat(recepcao): consultas de leitura ListarAgenda e ListarMarcacoesDoente"
```

---

## Task 7: Adaptador ACL — `LeitorDoente` sobre o BC Clínico

**Files:**
- Create: `internal/adapters/recepcao/leitor_doente.go`
- Test: `internal/adapters/recepcao/leitor_doente_test.go`

**Interfaces:**
- Consumes: `clinico.RepositorioDoentes` (`ObterPorID(ctx, id) (*clinico.Doente, error)`), `clinico.EstadoActivo`, `apprecepcao.LeitorDoente`.
- Produces: `NovoLeitorDoente(clinico.RepositorioDoentes) *LeitorDoente` (`DoenteActivo(ctx, doenteID string) (bool, error)`).

- [ ] **Step 1: Write the failing test**

```go
// internal/adapters/recepcao/leitor_doente_test.go
package recepcao_test

import (
	"context"
	"testing"

	adrecepcao "github.com/ivandrosilva12/sgcfinal/internal/adapters/recepcao"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeDoentes é um duplo mínimo de clinico.RepositorioDoentes; só ObterPorID é usado.
type fakeDoentes struct {
	doente *clinico.Doente
	erro   error
}

func (f fakeDoentes) Guardar(context.Context, *clinico.Doente) (string, error) { return "", nil }
func (f fakeDoentes) ObterPorID(context.Context, string) (*clinico.Doente, error) {
	return f.doente, f.erro
}
func (f fakeDoentes) ObterPorNumProcesso(context.Context, string) (*clinico.Doente, error) {
	return nil, nil
}
func (f fakeDoentes) Pesquisar(context.Context, clinico.FiltroDoentes) (clinico.PaginaDoentes, error) {
	return clinico.PaginaDoentes{}, nil
}
func (f fakeDoentes) ProximoNumeroProcesso(context.Context, int) (string, error) { return "", nil }

func TestDoenteActivo_Inexistente_FalseSemErro(t *testing.T) {
	l := adrecepcao.NovoLeitorDoente(fakeDoentes{erro: erros.Novo(erros.CategoriaNaoEncontrado, "não existe")})
	ok, err := l.DoenteActivo(context.Background(), "doe-x")
	if err != nil || ok {
		t.Fatalf("doente inexistente devia dar (false, nil), veio (%v, %v)", ok, err)
	}
}
```

**Nota:** para o caso "activo → true" seria preciso construir um `clinico.Doente` activo. Como o construtor do Doente exige muitos campos válidos, o teste unitário cobre o ramo inexistente (o mais crítico para a ACL); o ramo activo é exercido pelo teste de integração da marcação (Task 12 usa doentes reais). Mantém-se só este teste unitário.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/recepcao/...`
Expected: FAIL — `undefined: adrecepcao.NovoLeitorDoente`.

- [ ] **Step 3: Write the ACL adapter**

```go
// Package recepcao (adaptadores) contém adaptadores de saída do BC Recepção.
// Camada 3 — Adaptadores.
package recepcao

import (
	"context"

	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// LeitorDoente implementa a porta anti-corrupção apprecepcao.LeitorDoente, lendo o BC
// Clínico através do seu repositório e traduzindo o que interessa à Recepção (uma
// pergunta booleana) — sem deixar passar tipos do Clínico.
type LeitorDoente struct {
	doentes clinico.RepositorioDoentes
}

// NovoLeitorDoente constrói o adaptador sobre o repositório de doentes.
func NovoLeitorDoente(doentes clinico.RepositorioDoentes) *LeitorDoente {
	return &LeitorDoente{doentes: doentes}
}

// DoenteActivo indica se o doente existe e está activo. Um doente inexistente devolve
// false sem erro — para a Recepção, "não existe" e "não pode ser marcado" são a mesma
// resposta.
func (l *LeitorDoente) DoenteActivo(ctx context.Context, doenteID string) (bool, error) {
	d, err := l.doentes.ObterPorID(ctx, doenteID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return false, nil
		}
		return false, err
	}
	return d.Estado() == clinico.EstadoActivo, nil
}

// Garantia de conformidade com a porta.
var _ apprecepcao.LeitorDoente = (*LeitorDoente)(nil)
```

**Nota ao implementador:** confirma que `clinico.Doente` expõe `Estado() clinico.EstadoDoente` (é o padrão usado por `adapters/laboratorio/leitor_clinico.go:37`). Se o getter tiver outro nome, ajusta a chamada.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/recepcao/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/recepcao/leitor_doente.go internal/adapters/recepcao/leitor_doente_test.go
git commit -m "feat(recepcao): adaptador ACL LeitorDoente sobre o BC Clinico"
```

---

## Task 8: Migração SQL + registo no embed

**Files:**
- Create: `migrations/recepcao/0001_agenda_marcacoes.sql`
- Modify: `migrations/embed.go:10` (adicionar `recepcao` à directiva `//go:embed`)

**Interfaces:**
- Produces: schema `recepcao` com tabelas `janelas` e `marcacoes` (colunas e restrições consumidas pelos repositórios da Task 9–10).

- [ ] **Step 1: Write the migration**

```sql
-- migrations/recepcao/0001_agenda_marcacoes.sql
-- Bounded Context: recepcao
-- Migration forward-only. Marcação: janelas de disponibilidade e marcações.
--
-- doente_id/medico_id/especialidade_id são referências textuais a outros bounded
-- contexts: SEM foreign key (regra de arquitectura). A existência/estado do doente é
-- validada pela ACL na camada de aplicação.

CREATE SCHEMA IF NOT EXISTS recepcao;

-- btree_gist é preciso para a restrição EXCLUDE que combina uma igualdade (medico_id)
-- com uma sobreposição de intervalo (tstzrange) no mesmo índice.
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS recepcao.janelas (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    medico_id        uuid        NOT NULL,
    especialidade_id uuid        NOT NULL,
    inicio           timestamptz NOT NULL,
    fim              timestamptz NOT NULL,
    criado_em        timestamptz NOT NULL DEFAULT now(),
    CHECK (fim > inicio)
);
CREATE INDEX IF NOT EXISTS idx_janelas_medico
    ON recepcao.janelas (medico_id, inicio);

-- Marcação: uma consulta agendada. A CHECK impõe a coerência estado↔motivo (uma
-- CANCELADA sem motivo é recusada pela base de dados). A EXCLUDE é defesa em
-- profundidade: a invariante de não-sobreposição vive no agregado
-- (VerificarDisponibilidade), mas a base de dados também nega marcações MARCADA
-- sobrepostas do mesmo médico — o único guarda à prova de corridas concorrentes.
CREATE TABLE IF NOT EXISTS recepcao.marcacoes (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id        uuid        NOT NULL,
    medico_id        uuid        NOT NULL,
    especialidade_id uuid        NOT NULL,
    inicio           timestamptz NOT NULL,
    fim              timestamptz NOT NULL,
    estado           text        NOT NULL CHECK (estado IN
                       ('MARCADA','CANCELADA','REMARCADA','FALTOU')),
    motivo           text,
    remarca_de       uuid        REFERENCES recepcao.marcacoes(id),
    criado_em        timestamptz NOT NULL DEFAULT now(),
    actualizado_em   timestamptz NOT NULL DEFAULT now(),
    CHECK (fim > inicio),
    CHECK (estado <> 'CANCELADA' OR motivo IS NOT NULL),
    EXCLUDE USING gist (
        medico_id WITH =,
        tstzrange(inicio, fim) WITH &&
    ) WHERE (estado = 'MARCADA')
);
CREATE INDEX IF NOT EXISTS idx_marcacoes_doente ON recepcao.marcacoes (doente_id);
CREATE INDEX IF NOT EXISTS idx_marcacoes_medico ON recepcao.marcacoes (medico_id, inicio);
```

- [ ] **Step 2: Register the new bounded context in the embed directive**

Modify `migrations/embed.go` line 10 — adicionar `recepcao` (por ordem alfabética, depois de `laboratorio`):

```go
//go:embed auditoria clinico farmacia identidade laboratorio recepcao shared
var FS embed.FS
```

- [ ] **Step 3: Verify the embed compiles and the file is included**

Run: `go build ./migrations/... && go test ./migrations/...`
Expected: PASS (o `embed_test.go` valida que a FS embebida contém as migrations; a nova directiva compila só se o directório `recepcao` existir e tiver o `.sql`).

- [ ] **Step 4: Commit**

```bash
git add migrations/recepcao/0001_agenda_marcacoes.sql migrations/embed.go
git commit -m "feat(recepcao): migration 0001 (janelas, marcacoes, EXCLUDE anti-sobreposicao)"
```

---

## Task 9: Repositório pgx — `JanelasRepo`

**Files:**
- Create: `internal/adapters/pgrepo/janelas_repo.go`
- Test: `internal/adapters/pgrepo/janelas_repo_integration_test.go`

**Interfaces:**
- Consumes: `dominio.RepositorioJanelas` (Task 4), `pgxpool.Pool`.
- Produces: `NovoRepositorioJanelas(*pgxpool.Pool) *RepositorioJanelas` (implementa `dominio.RepositorioJanelas`).

- [ ] **Step 1: Write the failing integration test**

```go
// internal/adapters/pgrepo/janelas_repo_integration_test.go
//go:build integration

package pgrepo_test

import (
	"context"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
)

func instD(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("data inválida: %v", err)
	}
	return v
}

func TestJanelasRepo_GuardarObterListarRemover(t *testing.T) {
	pool := poolTeste(t) // helper de integração já existente no pacote pgrepo_test
	repo := pgrepo.NovoRepositorioJanelas(pool)
	ctx := context.Background()

	j, _ := dominio.NovaJanela(uuidTeste(t), uuidTeste(t),
		instD(t, "2026-08-01T08:00:00Z"), instD(t, "2026-08-01T13:00:00Z"))
	id, err := repo.Guardar(ctx, j)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}

	got, err := repo.ObterPorID(ctx, id)
	if err != nil || got.ID() != id {
		t.Fatalf("obter: %v (%+v)", err, got)
	}

	lista, err := repo.ListarPorMedicoIntervalo(ctx, j.MedicoID(),
		instD(t, "2026-08-01T09:00:00Z"), instD(t, "2026-08-01T10:00:00Z"))
	if err != nil || len(lista) != 1 {
		t.Fatalf("listar sobreposição: %v (n=%d)", err, len(lista))
	}

	if err := repo.Remover(ctx, id); err != nil {
		t.Fatalf("remover: %v", err)
	}
}
```

**Nota:** `poolTeste` e `uuidTeste` são helpers do pacote de testes de integração `pgrepo_test`. Se ainda não existirem com estes nomes, reutiliza os que o pacote já usa para os outros repositórios de integração (procura em `internal/adapters/pgrepo/*_integration_test.go`) e ajusta os nomes das chamadas.

- [ ] **Step 2: Run test to verify it fails (or SKIPs without DB)**

Run: `go test -tags integration ./internal/adapters/pgrepo/... -run TestJanelasRepo`
Expected: FAIL a compilar — `undefined: pgrepo.NovoRepositorioJanelas` (ou SKIP se `DATABASE_URL` não estiver definida, conforme o helper `poolTeste`).

- [ ] **Step 3: Write the repository**

```go
// internal/adapters/pgrepo/janelas_repo.go
package pgrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioJanelas implementa dominio.RepositorioJanelas com pgx.
type RepositorioJanelas struct {
	pool *pgxpool.Pool
}

// NovoRepositorioJanelas constrói o repositório sobre o pool pgx.
func NovoRepositorioJanelas(pool *pgxpool.Pool) *RepositorioJanelas {
	return &RepositorioJanelas{pool: pool}
}

// Guardar insere a janela e devolve o id gerado.
func (r *RepositorioJanelas) Guardar(ctx context.Context, j *dominio.JanelaDisponibilidade) (string, error) {
	s := j.Snapshot()
	const q = `
INSERT INTO recepcao.janelas (medico_id, especialidade_id, inicio, fim)
VALUES ($1::uuid, $2::uuid, $3, $4)
RETURNING id::text`
	var id string
	if err := r.pool.QueryRow(ctx, q, s.MedicoID, s.EspecialidadeID, s.Inicio, s.Fim).Scan(&id); err != nil {
		return "", fmt.Errorf("guardar janela: %w", err)
	}
	return id, nil
}

// ObterPorID reconstrói a janela. NaoEncontrado se não existir.
func (r *RepositorioJanelas) ObterPorID(ctx context.Context, id string) (*dominio.JanelaDisponibilidade, error) {
	const q = `
SELECT id::text, medico_id::text, especialidade_id::text, inicio, fim, criado_em
FROM recepcao.janelas WHERE id=$1`
	var s dominio.SnapshotJanela
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.MedicoID, &s.EspecialidadeID, &s.Inicio, &s.Fim, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "janela não encontrada")
		}
		return nil, fmt.Errorf("obter janela: %w", err)
	}
	return dominio.ReconstruirJanela(s), nil
}

// ListarPorMedicoIntervalo devolve as janelas do médico que se sobrepõem a [de,ate].
func (r *RepositorioJanelas) ListarPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]dominio.JanelaDisponibilidade, error) {
	const q = `
SELECT id::text, medico_id::text, especialidade_id::text, inicio, fim, criado_em
FROM recepcao.janelas
WHERE medico_id=$1::uuid AND inicio < $3 AND $2 < fim
ORDER BY inicio`
	linhas, err := r.pool.Query(ctx, q, medicoID, de, ate)
	if err != nil {
		return nil, fmt.Errorf("listar janelas: %w", err)
	}
	defer linhas.Close()
	var out []dominio.JanelaDisponibilidade
	for linhas.Next() {
		var s dominio.SnapshotJanela
		if err := linhas.Scan(&s.ID, &s.MedicoID, &s.EspecialidadeID, &s.Inicio, &s.Fim, &s.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler janela: %w", err)
		}
		out = append(out, *dominio.ReconstruirJanela(s))
	}
	return out, linhas.Err()
}

// Remover apaga a janela. NaoEncontrado se não existir.
func (r *RepositorioJanelas) Remover(ctx context.Context, id string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM recepcao.janelas WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("remover janela: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return erros.Novo(erros.CategoriaNaoEncontrado, "janela não encontrada")
	}
	return nil
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioJanelas = (*RepositorioJanelas)(nil)
```

- [ ] **Step 4: Run test to verify it passes (with DB)**

Run: `DATABASE_URL=... go test -tags integration ./internal/adapters/pgrepo/... -run TestJanelasRepo`
Expected: PASS (aplica as migrations e exercita o CRUD). Sem `DATABASE_URL`: SKIP.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/pgrepo/janelas_repo.go internal/adapters/pgrepo/janelas_repo_integration_test.go
git commit -m "feat(recepcao): repositorio pgx JanelasRepo"
```

---

## Task 10: Repositório pgx — `MarcacoesRepo` (Transitar + Remarcar)

**Files:**
- Create: `internal/adapters/pgrepo/marcacoes_repo.go`
- Test: `internal/adapters/pgrepo/marcacoes_repo_integration_test.go`

**Interfaces:**
- Consumes: `dominio.RepositorioMarcacoes` (Task 4), `pgxpool.Pool`.
- Produces: `NovoRepositorioMarcacoes(*pgxpool.Pool) *RepositorioMarcacoes` (implementa `dominio.RepositorioMarcacoes`).

- [ ] **Step 1: Write the failing integration test**

```go
// internal/adapters/pgrepo/marcacoes_repo_integration_test.go
//go:build integration

package pgrepo_test

import (
	"context"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestMarcacoesRepo_GuardarTransitarRemarcar(t *testing.T) {
	pool := poolTeste(t)
	repo := pgrepo.NovoRepositorioMarcacoes(pool)
	ctx := context.Background()
	medico := uuidTeste(t)

	m, _ := dominio.NovaMarcacao(uuidTeste(t), medico, uuidTeste(t),
		instD(t, "2026-08-02T09:00:00Z"), instD(t, "2026-08-02T09:30:00Z"))
	id, err := repo.Guardar(ctx, m)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}

	// remarcar: original → REMARCADA + nova MARCADA
	original, _ := repo.ObterPorID(ctx, id)
	nova, err := original.Remarcar(instD(t, "2026-08-02T10:00:00Z"), instD(t, "2026-08-02T10:30:00Z"), instD(t, "2026-08-01T00:00:00Z"))
	if err != nil {
		t.Fatalf("remarcar (dominio): %v", err)
	}
	novoID, err := repo.Remarcar(ctx, original, nova)
	if err != nil {
		t.Fatalf("remarcar (repo): %v", err)
	}
	recarregada, _ := repo.ObterPorID(ctx, id)
	if recarregada.Estado() != dominio.MarcRemarcada {
		t.Fatalf("original devia estar REMARCADA, veio %s", recarregada.Estado())
	}
	nv, _ := repo.ObterPorID(ctx, novoID)
	if nv.Estado() != dominio.MarcMarcada || nv.RemarcaDe() != id {
		t.Fatalf("nova mal gravada: estado=%s remarca_de=%s", nv.Estado(), nv.RemarcaDe())
	}
}

func TestMarcacoesRepo_ExcludeNegaSobreposicao(t *testing.T) {
	pool := poolTeste(t)
	repo := pgrepo.NovoRepositorioMarcacoes(pool)
	ctx := context.Background()
	medico := uuidTeste(t)

	m1, _ := dominio.NovaMarcacao(uuidTeste(t), medico, uuidTeste(t),
		instD(t, "2026-08-03T09:00:00Z"), instD(t, "2026-08-03T09:30:00Z"))
	if _, err := repo.Guardar(ctx, m1); err != nil {
		t.Fatalf("guardar m1: %v", err)
	}
	// sobreposta, mesmo médico, ambas MARCADA → a EXCLUDE tem de negar
	m2, _ := dominio.NovaMarcacao(uuidTeste(t), medico, uuidTeste(t),
		instD(t, "2026-08-03T09:15:00Z"), instD(t, "2026-08-03T09:45:00Z"))
	_, err := repo.Guardar(ctx, m2)
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("EXCLUDE devia negar com CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags integration ./internal/adapters/pgrepo/... -run TestMarcacoesRepo`
Expected: FAIL a compilar — `undefined: pgrepo.NovoRepositorioMarcacoes`.

- [ ] **Step 3: Write the repository**

```go
// internal/adapters/pgrepo/marcacoes_repo.go
package pgrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// codigoExclusaoPG é o SQLSTATE de violação de restrição EXCLUDE (sobreposição).
const codigoExclusaoPG = "23P01"

// RepositorioMarcacoes implementa dominio.RepositorioMarcacoes com pgx.
type RepositorioMarcacoes struct {
	pool *pgxpool.Pool
}

// NovoRepositorioMarcacoes constrói o repositório sobre o pool pgx.
func NovoRepositorioMarcacoes(pool *pgxpool.Pool) *RepositorioMarcacoes {
	return &RepositorioMarcacoes{pool: pool}
}

// colunasMarcacao é a lista SELECT reutilizada para reconstruir agregados.
const colunasMarcacao = `id::text, doente_id::text, medico_id::text, especialidade_id::text,
       inicio, fim, estado, COALESCE(motivo,''), COALESCE(remarca_de::text,''),
       criado_em, actualizado_em`

// Guardar insere a marcação e devolve o id gerado. Uma sobreposição negada pela
// EXCLUDE devolve Conflito.
func (r *RepositorioMarcacoes) Guardar(ctx context.Context, m *dominio.Marcacao) (string, error) {
	return r.inserir(ctx, r.pool, m)
}

// inserir insere uma marcação numa dada querier (pool ou tx).
func (r *RepositorioMarcacoes) inserir(ctx context.Context, q querier, m *dominio.Marcacao) (string, error) {
	s := m.Snapshot()
	const sql = `
INSERT INTO recepcao.marcacoes
    (doente_id, medico_id, especialidade_id, inicio, fim, estado, motivo, remarca_de)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, NULLIF($7,''), NULLIF($8,'')::uuid)
RETURNING id::text`
	var id string
	err := q.QueryRow(ctx, sql, s.DoenteID, s.MedicoID, s.EspecialidadeID, s.Inicio, s.Fim,
		string(s.Estado), s.Motivo, s.RemarcaDe).Scan(&id)
	if err != nil {
		if ehExclusao(err) {
			return "", erros.Novo(erros.CategoriaConflito, "o horário sobrepõe outra marcação do médico")
		}
		return "", fmt.Errorf("guardar marcação: %w", err)
	}
	return id, nil
}

// ObterPorID reconstrói a marcação. NaoEncontrado se não existir.
func (r *RepositorioMarcacoes) ObterPorID(ctx context.Context, id string) (*dominio.Marcacao, error) {
	q := `SELECT ` + colunasMarcacao + ` FROM recepcao.marcacoes WHERE id=$1`
	m, err := r.scanUma(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
		}
		return nil, err
	}
	return m, nil
}

// Transitar aplica a transição de estado com guarda compare-and-set: o UPDATE só se
// aplica se a linha ainda estiver no estado com que o agregado foi lido.
func (r *RepositorioMarcacoes) Transitar(ctx context.Context, m *dominio.Marcacao) error {
	s := m.Snapshot()
	if s.ID == "" {
		return erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
	}
	const q = `
UPDATE recepcao.marcacoes
SET estado=$2, motivo=NULLIF($3,''), actualizado_em=$4
WHERE id=$1 AND estado=$5`
	ct, err := r.pool.Exec(ctx, q, s.ID, string(s.Estado), s.Motivo, s.ActualizadoEm, string(s.EstadoAnterior))
	if err != nil {
		return fmt.Errorf("actualizar marcação: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return r.erroTransicaoFalhada(ctx, s.ID)
	}
	return nil
}

// Remarcar grava, numa única transacção, a original a passar a REMARCADA (compare-and-set)
// e a nova MARCADA. Devolve o id da nova.
func (r *RepositorioMarcacoes) Remarcar(ctx context.Context, original, nova *dominio.Marcacao) (string, error) {
	so := original.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção de remarcação: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	const upd = `UPDATE recepcao.marcacoes SET estado=$2, actualizado_em=$3 WHERE id=$1 AND estado=$4`
	ct, err := tx.Exec(ctx, upd, so.ID, string(so.Estado), so.ActualizadoEm, string(so.EstadoAnterior))
	if err != nil {
		return "", fmt.Errorf("marcar original como remarcada: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", r.erroTransicaoFalhada(ctx, so.ID)
	}
	novoID, err := r.inserir(ctx, tx, nova)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar remarcação: %w", err)
	}
	return novoID, nil
}

// ListarActivasPorMedicoIntervalo devolve os agregados das marcações MARCADA do médico
// que se sobrepõem a [de,ate].
func (r *RepositorioMarcacoes) ListarActivasPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]dominio.Marcacao, error) {
	q := `SELECT ` + colunasMarcacao + `
FROM recepcao.marcacoes
WHERE medico_id=$1::uuid AND estado='MARCADA' AND inicio < $3 AND $2 < fim
ORDER BY inicio`
	linhas, err := r.pool.Query(ctx, q, medicoID, de, ate)
	if err != nil {
		return nil, fmt.Errorf("listar marcações activas: %w", err)
	}
	defer linhas.Close()
	var out []dominio.Marcacao
	for linhas.Next() {
		m, err := r.scanUma(linhas)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, linhas.Err()
}

// ListarPorMedicoIntervalo devolve read-models de TODAS as marcações do médico que se
// sobrepõem a [de,ate].
func (r *RepositorioMarcacoes) ListarPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]dominio.ResumoMarcacao, error) {
	const q = `
SELECT id::text, doente_id::text, medico_id::text, especialidade_id::text, estado,
       COALESCE(motivo,''), inicio, fim, criado_em
FROM recepcao.marcacoes
WHERE medico_id=$1::uuid AND inicio < $3 AND $2 < fim
ORDER BY inicio`
	return r.consultarResumos(ctx, q, medicoID, de, ate)
}

// ListarPorDoente devolve read-models das marcações de um doente.
func (r *RepositorioMarcacoes) ListarPorDoente(ctx context.Context, doenteID string) ([]dominio.ResumoMarcacao, error) {
	const q = `
SELECT id::text, doente_id::text, medico_id::text, especialidade_id::text, estado,
       COALESCE(motivo,''), inicio, fim, criado_em
FROM recepcao.marcacoes
WHERE doente_id=$1::uuid
ORDER BY inicio DESC`
	return r.consultarResumos(ctx, q, doenteID)
}

func (r *RepositorioMarcacoes) consultarResumos(ctx context.Context, q string, args ...any) ([]dominio.ResumoMarcacao, error) {
	linhas, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listar marcações: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoMarcacao{}
	for linhas.Next() {
		var rm dominio.ResumoMarcacao
		if err := linhas.Scan(&rm.ID, &rm.DoenteID, &rm.MedicoID, &rm.EspecialidadeID, &rm.Estado,
			&rm.Motivo, &rm.Inicio, &rm.Fim, &rm.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler marcação: %w", err)
		}
		out = append(out, rm)
	}
	return out, linhas.Err()
}

// erroTransicaoFalhada distingue 404 (linha inexistente) de 409 (estado mudou).
func (r *RepositorioMarcacoes) erroTransicaoFalhada(ctx context.Context, id string) error {
	var existe bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM recepcao.marcacoes WHERE id=$1)`, id).Scan(&existe); err != nil {
		return fmt.Errorf("verificar marcação: %w", err)
	}
	if !existe {
		return erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
	}
	return erros.Novo(erros.CategoriaConflito,
		"o estado da marcação mudou entretanto; recarregue a marcação e repita a operação")
}

// scanUma reconstrói uma Marcacao a partir de uma linha (QueryRow ou Rows).
func (r *RepositorioMarcacoes) scanUma(linha pgx.Row) (*dominio.Marcacao, error) {
	var s dominio.SnapshotMarcacao
	var estado string
	if err := linha.Scan(&s.ID, &s.DoenteID, &s.MedicoID, &s.EspecialidadeID,
		&s.Inicio, &s.Fim, &estado, &s.Motivo, &s.RemarcaDe, &s.CriadoEm, &s.ActualizadoEm); err != nil {
		return nil, err
	}
	s.Estado = dominio.EstadoMarcacao(estado)
	return dominio.ReconstruirMarcacao(s), nil
}

// ehExclusao indica se o erro é uma violação da restrição EXCLUDE (sobreposição).
func ehExclusao(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoExclusaoPG
}

// querier abstrai o que pool e tx têm em comum (QueryRow), para reutilizar `inserir`.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioMarcacoes = (*RepositorioMarcacoes)(nil)
```

**Nota ao implementador:** se o pacote `pgrepo` já definir um tipo `querier` equivalente (procura `type querier` no pacote), reutiliza-o e apaga esta definição para não duplicar. `pgx.Tx` e `*pgxpool.Pool` satisfazem ambos `QueryRow(ctx, sql, args...) pgx.Row`.

- [ ] **Step 4: Run test to verify it passes (with DB)**

Run: `DATABASE_URL=... go test -tags integration ./internal/adapters/pgrepo/... -run TestMarcacoesRepo`
Expected: PASS (remarcação transaccional e EXCLUDE a negar sobreposição). Sem `DATABASE_URL`: SKIP.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/pgrepo/marcacoes_repo.go internal/adapters/pgrepo/marcacoes_repo_integration_test.go
git commit -m "feat(recepcao): repositorio pgx MarcacoesRepo (Transitar compare-and-set, Remarcar transaccional)"
```

---

## Task 11: Handler HTTP + rotas + RBAC

**Files:**
- Create: `internal/adapters/http/recepcao_handler.go`
- Test: `internal/adapters/http/recepcao_test.go`

**Interfaces:**
- Consumes: os casos de uso (Tasks 4–6) via interfaces de serviço; `SessaoDe`, `RBAC`, `responderErro`, `Auth`, `i18n`, `dominio.Papel*` (padrões existentes em `internal/adapters/http`).
- Produces: `NovoRecepcaoHandler(...)` e `RegistarRecepcao(r gin.IRouter, h *RecepcaoHandler, protecao ...gin.HandlerFunc)`.

- [ ] **Step 1: Write the failing test**

```go
// internal/adapters/http/recepcao_test.go
package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

// duploMarcar guarda o actor recebido e devolve uma marcação fixa.
type duploMarcar struct{ actorRecebido string }

func (d *duploMarcar) Executar(_ context.Context, actor string, _ apprecepcao.DadosMarcar) (apprecepcao.DetalheMarcacao, error) {
	d.actorRecebido = actor
	return apprecepcao.DetalheMarcacao{ID: "marc-1", Estado: "MARCADA"}, nil
}

type duploDefinirJanela struct{}

func (duploDefinirJanela) Executar(_ context.Context, _ string, _ apprecepcao.DadosDefinirJanela) (apprecepcao.DetalheJanela, error) {
	return apprecepcao.DetalheJanela{ID: "jan-1"}, nil
}

type duploRemoverJanela struct{}

func (duploRemoverJanela) Executar(_ context.Context, _, _ string) error { return nil }

type duploRemarcar struct{}

func (duploRemarcar) Executar(_ context.Context, _, _ string, _ apprecepcao.DadosRemarcar) (apprecepcao.DetalheMarcacao, error) {
	return apprecepcao.DetalheMarcacao{ID: "marc-2", Estado: "MARCADA", RemarcaDe: "marc-1"}, nil
}

type duploCancelar struct{}

func (duploCancelar) Executar(_ context.Context, _, _, _ string) (apprecepcao.DetalheMarcacao, error) {
	return apprecepcao.DetalheMarcacao{ID: "marc-1", Estado: "CANCELADA"}, nil
}

type duploRegistarFalta struct{}

func (duploRegistarFalta) Executar(_ context.Context, _, _ string) (apprecepcao.DetalheMarcacao, error) {
	return apprecepcao.DetalheMarcacao{ID: "marc-1", Estado: "FALTOU"}, nil
}

type duploListarAgenda struct{}

func (duploListarAgenda) Executar(_ context.Context, _ string, _, _ time.Time) (apprecepcao.Agenda, error) {
	return apprecepcao.Agenda{}, nil
}

type duploListarMarcacoesDoente struct{}

func (duploListarMarcacoesDoente) Executar(_ context.Context, _ string) ([]apprecepcao.ResumoMarcacao, error) {
	return []apprecepcao.ResumoMarcacao{}, nil
}

func routerRecepcao(t *testing.T, marcar *duploMarcar, sessao identidade.Sessao) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := adhttp.NovoRecepcaoHandler(
		duploDefinirJanela{}, duploRemoverJanela{},
		marcar, duploRemarcar{}, duploCancelar{}, duploRegistarFalta{},
		duploListarAgenda{}, duploListarMarcacoesDoente{},
	)
	adhttp.RegistarRecepcao(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func sessaoRecepcaoDe(sujeito string, papel identidade.Papel) identidade.Sessao {
	return identidade.Sessao{Sujeito: sujeito, Papeis: []identidade.Papel{papel}}
}

func TestMarcar_UsaOSujeitoAutenticado(t *testing.T) {
	marcar := &duploMarcar{}
	r := routerRecepcao(t, marcar, sessaoRecepcaoDe("adm-9", identidade.PapelAdministrativo))
	corpo, _ := json.Marshal(map[string]any{
		"doente_id": "doe-1", "medico_id": "med-1", "especialidade_id": "esp-1",
		"inicio": "2026-08-01T09:00:00Z", "fim": "2026-08-01T09:30:00Z",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	if marcar.actorRecebido != "adm-9" {
		t.Fatalf("o actor devia vir da sessão (adm-9), veio %q", marcar.actorRecebido)
	}
}

func TestMarcar_Medico_Proibido(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	corpo, _ := json.Marshal(map[string]any{"doente_id": "doe-1", "medico_id": "med-1", "especialidade_id": "esp-1", "inicio": "2026-08-01T09:00:00Z", "fim": "2026-08-01T09:30:00Z"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("um médico não marca: esperava 403, veio %d", w.Code)
	}
}

func TestMarcar_CorpoMalformado_400(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/marcacoes", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

func TestListarAgenda_MedicoPodeLer(t *testing.T) {
	r := routerRecepcao(t, &duploMarcar{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/agenda?medico=med-1&de=2026-08-01T00:00:00Z&ate=2026-08-01T23:00:00Z", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("o médico devia poder ler a agenda: esperava 200, veio %d", w.Code)
	}
}
```

**Nota:** `fakeAuth` já existe no pacote de testes (`identidade_test.go`), reutilizado por outros handlers. Não criar outro.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/http/... -run Recepcao`
Expected: FAIL — `undefined: adhttp.NovoRecepcaoHandler`.

- [ ] **Step 3: Write the handler**

```go
// internal/adapters/http/recepcao_handler.go
//
// Package http (adaptadores) — este ficheiro expõe o BC Recepção. Camada 3.
package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso do BC Recepção.
type (
	ServicoDefinirJanela interface {
		Executar(ctx context.Context, actor string, dados apprecepcao.DadosDefinirJanela) (apprecepcao.DetalheJanela, error)
	}
	ServicoRemoverJanela interface {
		Executar(ctx context.Context, actor, janelaID string) error
	}
	ServicoMarcar interface {
		Executar(ctx context.Context, actor string, dados apprecepcao.DadosMarcar) (apprecepcao.DetalheMarcacao, error)
	}
	ServicoRemarcar interface {
		Executar(ctx context.Context, actor, marcacaoID string, dados apprecepcao.DadosRemarcar) (apprecepcao.DetalheMarcacao, error)
	}
	ServicoCancelar interface {
		Executar(ctx context.Context, actor, marcacaoID, motivo string) (apprecepcao.DetalheMarcacao, error)
	}
	ServicoRegistarFalta interface {
		Executar(ctx context.Context, actor, marcacaoID string) (apprecepcao.DetalheMarcacao, error)
	}
	ServicoListarAgenda interface {
		Executar(ctx context.Context, medicoID string, de, ate time.Time) (apprecepcao.Agenda, error)
	}
	ServicoListarMarcacoesDoente interface {
		Executar(ctx context.Context, doenteID string) ([]apprecepcao.ResumoMarcacao, error)
	}
)

// RecepcaoHandler expõe os endpoints HTTP do BC Recepção.
type RecepcaoHandler struct {
	definirJanela  ServicoDefinirJanela
	removerJanela  ServicoRemoverJanela
	marcar         ServicoMarcar
	remarcar       ServicoRemarcar
	cancelar       ServicoCancelar
	registarFalta  ServicoRegistarFalta
	listarAgenda   ServicoListarAgenda
	marcacoesDoente ServicoListarMarcacoesDoente
}

// NovoRecepcaoHandler constrói o handler.
func NovoRecepcaoHandler(
	definirJanela ServicoDefinirJanela, removerJanela ServicoRemoverJanela,
	marcar ServicoMarcar, remarcar ServicoRemarcar, cancelar ServicoCancelar,
	registarFalta ServicoRegistarFalta, listarAgenda ServicoListarAgenda,
	marcacoesDoente ServicoListarMarcacoesDoente,
) *RecepcaoHandler {
	return &RecepcaoHandler{
		definirJanela: definirJanela, removerJanela: removerJanela,
		marcar: marcar, remarcar: remarcar, cancelar: cancelar,
		registarFalta: registarFalta, listarAgenda: listarAgenda,
		marcacoesDoente: marcacoesDoente,
	}
}

// RegistarRecepcao regista as rotas, aplicando `protecao` e o RBAC por rota. A gestão
// de agenda e as marcações são função do Administrativo (secretaria/recepção), com
// supervisão do Director/Admin. A leitura da agenda é aberta também ao Médico, que
// precisa de a consultar; a escrita nunca.
func RegistarRecepcao(r gin.IRouter, h *RecepcaoHandler, protecao ...gin.HandlerFunc) {
	soAdministrativo := RBAC(dominio.PapelAdministrativo, dominio.PapelDirector, dominio.PapelAdmin)
	leituraAgenda := RBAC(dominio.PapelAdministrativo, dominio.PapelDirector, dominio.PapelAdmin, dominio.PapelMedico)

	gm := r.Group("/api/v1/medicos")
	gm.Use(protecao...)
	gm.POST("/:mid/janelas", soAdministrativo, h.definirJanelaHTTP)

	gj := r.Group("/api/v1/janelas")
	gj.Use(protecao...)
	gj.DELETE("/:jid", soAdministrativo, h.removerJanelaHTTP)

	gmar := r.Group("/api/v1/marcacoes")
	gmar.Use(protecao...)
	gmar.POST("", soAdministrativo, h.marcarHTTP)
	gmar.POST("/:mid/remarcacao", soAdministrativo, h.remarcarHTTP)
	gmar.POST("/:mid/cancelamento", soAdministrativo, h.cancelarHTTP)
	gmar.POST("/:mid/falta", soAdministrativo, h.registarFaltaHTTP)

	gr := r.Group("/api/v1/recepcao")
	gr.Use(protecao...)
	gr.GET("/agenda", leituraAgenda, h.listarAgendaHTTP)

	gd := r.Group("/api/v1/doentes")
	gd.Use(protecao...)
	gd.GET("/:did/marcacoes", leituraAgenda, h.listarMarcacoesDoenteHTTP)
}

type corpoDefinirJanela struct {
	EspecialidadeID string    `json:"especialidade_id"`
	Inicio          time.Time `json:"inicio"`
	Fim             time.Time `json:"fim"`
}

type corpoRemarcar struct {
	Inicio time.Time `json:"inicio"`
	Fim    time.Time `json:"fim"`
}

type corpoCancelar struct {
	Motivo string `json:"motivo"`
}

func (h *RecepcaoHandler) definirJanelaHTTP(c *gin.Context) {
	var corpo corpoDefinirJanela
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.definirJanela.Executar(c.Request.Context(), actor.Sujeito, apprecepcao.DadosDefinirJanela{
		MedicoID: c.Param("mid"), EspecialidadeID: corpo.EspecialidadeID,
		Inicio: corpo.Inicio, Fim: corpo.Fim,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoHandler) removerJanelaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	if err := h.removerJanela.Executar(c.Request.Context(), actor.Sujeito, c.Param("jid")); err != nil {
		responderErro(c, err)
		return
	}
	c.Status(nethttp.StatusNoContent)
}

func (h *RecepcaoHandler) marcarHTTP(c *gin.Context) {
	var dados apprecepcao.DadosMarcar
	if err := c.ShouldBindJSON(&dados); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.marcar.Executar(c.Request.Context(), actor.Sujeito, dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoHandler) remarcarHTTP(c *gin.Context) {
	var corpo corpoRemarcar
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.remarcar.Executar(c.Request.Context(), actor.Sujeito, c.Param("mid"),
		apprecepcao.DadosRemarcar{Inicio: corpo.Inicio, Fim: corpo.Fim})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoHandler) cancelarHTTP(c *gin.Context) {
	var corpo corpoCancelar
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.cancelar.Executar(c.Request.Context(), actor.Sujeito, c.Param("mid"), corpo.Motivo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoHandler) registarFaltaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.registarFalta.Executar(c.Request.Context(), actor.Sujeito, c.Param("mid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoHandler) listarAgendaHTTP(c *gin.Context) {
	de, err := time.Parse(time.RFC3339, c.Query("de"))
	if err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "parâmetro 'de' inválido (esperado RFC3339)"))
		return
	}
	ate, err := time.Parse(time.RFC3339, c.Query("ate"))
	if err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "parâmetro 'ate' inválido (esperado RFC3339)"))
		return
	}
	out, err := h.listarAgenda.Executar(c.Request.Context(), c.Query("medico"), de, ate)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoHandler) listarMarcacoesDoenteHTTP(c *gin.Context) {
	out, err := h.marcacoesDoente.Executar(c.Request.Context(), c.Param("did"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}
```

**Nota ao implementador:** confirma que existe a constante `i18n.MsgPedidoInvalido` (usada por `laboratorio_handler.go`). Confirma também o nome exacto do papel: `dominio.PapelAdministrativo` (ver `internal/domain/identidade/papel.go:11`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/http/... -run Recepcao`
Expected: PASS.

- [ ] **Step 5: Verify adapters coverage ≥60% for the new handler**

Run: `go test -cover ./internal/adapters/http/...`
Expected: PASS (a cobertura agregada do pacote mantém-se ≥60%).

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/http/recepcao_handler.go internal/adapters/http/recepcao_test.go
git commit -m "feat(recepcao): handler HTTP com rotas, RBAC e actor da sessao"
```

---

## Task 12: Composição, ADR-032 e actualização do CLAUDE.md

**Files:**
- Modify: `internal/platform/app.go` (adicionar imports + montar o BC Recepção + registar rotas)
- Create: `adrs/ADR-032-bc-recepcao-marcacao.md`
- Modify: `CLAUDE.md` (lista de ADRs + próximo número; nota do marco)
- Test: `go build ./...` + toda a suite unitária

**Interfaces:**
- Consumes: tudo o que as Tasks 1–11 produziram.

- [ ] **Step 1: Wire the BC in the composition root**

Em `internal/platform/app.go`, adicionar aos imports (junto dos outros adaptadores e aplicações):

```go
	adrecepcao "github.com/ivandrosilva12/sgcfinal/internal/adapters/recepcao"
	apprecepcao "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
```

Depois do bloco do BC Laboratório (a seguir à linha 184, antes de "Middlewares transversais"), acrescentar:

```go
	// BC Recepção (marco Percurso Ambulatório): marcação e agenda por disponibilidade.
	repoJanelas := pgrepo.NovoRepositorioJanelas(pool)
	repoMarcacoes := pgrepo.NovoRepositorioMarcacoes(pool)
	// ACL: a Recepção lê o Clínico apenas através deste adaptador.
	leitorDoenteRec := adrecepcao.NovoLeitorDoente(repoDoentes)
	handlerRecepcao := adhttp.NovoRecepcaoHandler(
		apprecepcao.NovoCasoDefinirJanela(repoJanelas, repoAuditoria),
		apprecepcao.NovoCasoRemoverJanela(repoJanelas, repoMarcacoes, repoAuditoria),
		apprecepcao.NovoCasoMarcar(repoMarcacoes, repoJanelas, leitorDoenteRec, repoAuditoria),
		apprecepcao.NovoCasoRemarcar(repoMarcacoes, repoJanelas, repoAuditoria),
		apprecepcao.NovoCasoCancelar(repoMarcacoes, repoAuditoria),
		apprecepcao.NovoCasoRegistarFalta(repoMarcacoes, repoAuditoria),
		apprecepcao.NovoCasoListarAgenda(repoJanelas, repoMarcacoes),
		apprecepcao.NovoCasoListarMarcacoesDoente(repoMarcacoes),
	)
```

E dentro de `registarRotas`, a seguir a `adhttp.RegistarLaboratorio(...)`:

```go
		adhttp.RegistarRecepcao(r, handlerRecepcao, limiteMW, authMW)
```

- [ ] **Step 2: Verify the whole build and unit suite**

Run: `go build ./... && go test ./...`
Expected: PASS (todos os testes unitários; os de integração ficam SKIP sem `DATABASE_URL`).

- [ ] **Step 3: Verify the architectural linter (dependency rule)**

Run: `go-arch-lint check` (se disponível localmente; senão será validado em CI)
Expected: sem violações — `internal/domain/recepcao` não importa infra; o adaptador ACL `internal/adapters/recepcao` é a única fronteira que importa `internal/domain/clinico`.

- [ ] **Step 4: Write ADR-032**

```markdown
# ADR-032 — BC Recepção: Marcação e agenda por disponibilidade

- **Estado:** Aceite
- **Data:** 2026-07-15
- **Marco/Sprint:** Marco "Percurso Ambulatório" / sub-projecto Marcação
- **Fontes:** design em `docs/superpowers/specs/2026-07-15-recepcao-marcacao-design.md`; DDM-001 (papéis); ADR-031 (precedente da ACL do Laboratório).

## Contexto

O `EpisodioClinico` nasce diretamente em ABERTO, assumindo que o doente já está à
frente do médico. Faltava modelar o percurso ambulatório antes da consulta: marcação,
recepção/check-in e triagem. Este marco abre esse percurso; este primeiro sub-projecto
entrega a **Marcação**.

## Decisão

1. **Novo BC `recepcao`**, com schema PostgreSQL próprio e as 4 camadas Clean, em vez
   de engrossar o BC Clínico. O agendamento administrativo é uma responsabilidade
   distinta do ato clínico. Recepção/check-in e Triagem são sub-projectos futuros do
   mesmo marco.
2. **Dois agregados:** `JanelaDisponibilidade` (agenda declarada por médico, com data
   concreta — sem motor de recorrência, YAGNI) e `Marcacao` (raiz, com a máquina de
   estados MARCADA → CANCELADA/REMARCADA/FALTOU). A chegada (COMPARECEU) pertence ao
   futuro módulo Recepção.
3. **Remarcação por *supersede*** (mesmo padrão da correcção de resultados do
   Laboratório): a original passa a REMARCADA e nasce uma nova MARCADA que a
   referencia (`remarca_de`), preservando o histórico, numa única transacção.
4. **Invariante de disponibilidade como função de domínio pura**
   (`VerificarDisponibilidade`): não no passado, cabe numa janela da especialidade, não
   sobrepõe outra marcação MARCADA. O caso de uso alimenta-a com dados dos repositórios.
5. **Defesa em profundidade na base de dados:** restrição `EXCLUDE USING gist` (com
   `btree_gist`) que nega marcações MARCADA sobrepostas do mesmo médico — o único
   guarda à prova de corridas concorrentes; o adaptador traduz o SQLSTATE 23P01 em
   Conflito (409).
6. **Sem FK cross-context.** Um adaptador ACL `LeitorDoente`
   (`internal/adapters/recepcao/leitor_doente.go`) valida o doente contra o BC Clínico;
   o domínio e a aplicação da Recepção nunca importam `clinico`.
7. **RBAC:** definir/remover janelas e marcar/remarcar/cancelar/registar falta são do
   `Administrativo` (supervisão `Director`/`Admin`); a leitura da agenda é aberta também
   ao `Medico`. O actor é sempre o sujeito autenticado.

## Consequências

- O BC Recepção fica pronto a receber os sub-projectos Recepção/Check-in e Triagem.
- A agenda por data concreta exige criar várias janelas para horários repetidos; a
  recorrência semanal fica para evolução futura, se pedida.
- O episódio clínico continua a nascer em ABERTO; a ligação marcação → episódio (abrir
  a consulta a partir da marcação) será desenhada quando o módulo Recepção existir.
```

- [ ] **Step 5: Update CLAUDE.md**

No fim da secção "Convenções-fonte" de `CLAUDE.md`, acrescentar a ADR-031 a linha da ADR-032 e actualizar o próximo número:

```
`adrs/ADR-031-bc-laboratorio.md`,
`adrs/ADR-032-bc-recepcao-marcacao.md`.
Próximo ADR: **ADR-033**.
```

E na secção "6. Marco Actual", acrescentar uma linha a registar o novo marco em curso (depois do parágrafo do M3):

```
**Marco Percurso Ambulatório** (em curso, a par do M3): abre o percurso do doente antes
da consulta. Sub-projecto entregue: **Marcação** (BC `recepcao` — agenda por
disponibilidade, ciclo de vida da marcação, ACL sobre o Clínico; ver ADR-032). Pendentes:
Recepção/check-in e Triagem.
```

- [ ] **Step 6: Final build + commit**

Run: `go build ./... && go test ./...`
Expected: PASS.

```bash
git add internal/platform/app.go adrs/ADR-032-bc-recepcao-marcacao.md CLAUDE.md
git commit -m "feat(recepcao): liga o BC Recepcao ao composition root + ADR-032"
```

---

## Self-Review (autor)

**1. Cobertura do spec:**
- §2 Novo BC `recepcao`, 4 camadas, sem FK → Tasks 1–12; ACL Task 7. ✓
- §3.1 `JanelaDisponibilidade` → Task 1. ✓
- §3.2 `Marcacao` + máquina de estados + remarcação supersede → Task 2. ✓
- §3.3 `VerificarDisponibilidade` (passado, janela, sobreposição, encosto exacto) → Task 3. ✓
- §4.1 8 casos de uso + acções de auditoria → Tasks 4–6. ✓
- §4.2 Ports (`LeitorDoente`, `RepositorioJanelas`, `RepositorioMarcacoes`, `Auditor`) → Tasks 4, 7. ✓
- §5.1 HTTP + RBAC (`soAdministrativo`, leitura ao Médico) + actor da sessão → Task 11. ✓
- §5.2 Migração + `EXCLUDE`/`btree_gist` → Task 8; repos Tasks 9–10. ✓
- §5.3 ACL `LeitorDoente` → Task 7. ✓
- §6 Erros/auditoria → categorias usadas em todos os casos; auditoria em cada comando. ✓
- §7 Cobertura (domínio ≥85% Tasks 1–3, aplicação ≥75% Task 6 step 5, adaptadores ≥60% Task 11 step 5). ✓
- §8 ADR-032 + critérios de saída → Task 12. ✓

**2. Placeholders:** nenhum "TBD"/"TODO"; todos os passos com código completo. As duas notas de "confirma o nome" (getter `Estado()`, `i18n.MsgPedidoInvalido`, `querier`) são verificações contra o código existente, não lacunas do plano.

**3. Consistência de tipos:** `EstadoMarcacao`/consts, `VerificarDisponibilidade(janelas, activas, especialidadeID, inicio, fim, agora)`, `RepositorioMarcacoes.Remarcar(original, nova)`, `ResumoMarcacao`/`DetalheMarcacao`, acções de auditoria (`recepcao.janela.definida/removida`, `recepcao.marcacao.criada/remarcada/cancelada/faltou`) coerentes entre domínio, aplicação, adaptadores e ADR. Fluxo de ids (`Guardar` devolve id → `comIDMarcacao`/`comIDJanela` rehidrata para o detalhe) uniforme.
