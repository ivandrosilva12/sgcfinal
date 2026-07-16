# Triagem do BC Recepção — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar a Triagem (última etapa do marco Percurso Ambulatório) — classificação de prioridade Manchester + sinais vitais, transição da chegada para `TRIADO` com atribuição de médico ao walk-in, e a fila clínica ordenada por prioridade.

**Architecture:** Estende o BC `recepcao` com um agregado `Triagem` (imutável, 1:1 com a chegada), dois VOs (`PrioridadeManchester`, `SinaisVitais`) e um estado `TRIADO` na `Chegada`. O registo de triagem é transaccional e cruza dois agregados (transita a chegada + atribui médico + insere a triagem, com guarda compare-and-set). A fila clínica é um read-model ordenado por severidade Manchester. Reutiliza a auditoria e os padrões CAS/transacção já no BC.

**Tech Stack:** Go 1.22+, Gin, pgx v5 (SQL puro), PostgreSQL 16, testes com fakes + integração `//go:build integration`.

## Global Constraints

- **Idioma:** PT-PT angolano em TODO o output (código, comentários, commits, mensagens, JSON). Nunca EN/BR.
- **Module path:** `github.com/ivandrosilva12/sgcfinal`.
- **Sem FK cross-context.** `enfermeiro_id`/`medico_id` são uuid sem FK. Única FK: `triagens.chegada_id → recepcao.chegadas(id)` (interna).
- **Domínio sem infra:** `internal/domain/**` nunca importa `pgx`/`gin`/`net/http` (stdlib como `fmt`/`strconv` é permitido).
- **Camada de Aplicação** importa só o próprio Domínio (+ shared). Nunca outro BC, nunca infra.
- **Migrations forward-only**, sem `.down.sql`.
- **Nada de `panic()`** — sempre `error` via `internal/domain/shared/erros`.
- **Actor = sujeito autenticado** (`SessaoDe(c).Sujeito`), nunca do corpo. Na triagem, o actor é o enfermeiro triador.
- **Auditoria append-only** no comando de registo; leituras não auditadas.
- **Cobertura:** domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
- **Guarda compare-and-set:** transições usam `EstadoAnterior` (só fixado por `Reconstruir…`), nunca o estado novo.
- **Leitura clínica restrita:** rotas de leitura da triagem/fila clínica só a Médico/Enfermeiro/Director (sem Administrativo/Admin).
- **Categorias de erro:** `CategoriaValidacao`(400/422), `CategoriaConflito`(409), `CategoriaNaoEncontrado`(404), `CategoriaProibido`(403).
- **Registo de auditoria** (`auditoria.Registo`): `Actor, Accao, Entidade, EntidadeID, OcorridoEm, Detalhe`.
- **Convenção de testes de integração:** em `tests/integration/` (package `integration_test`, `//go:build integration`), usam `ligar(t)` (SKIP sem `DATABASE_URL`) e `db.AplicarMigracoes(ctx, pool, migrations.FS, logger)`. O helper de tempo do package é `instMarcacao(t, s)` (RFC3339) — NÃO existe `instD`.
- **BD de desenvolvimento disponível:** `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable'` (container `sgc-postgres-1`).
- **Reutilizações no pacote `pgrepo`:** o tipo `querier` e a função `ehUnica`/const `codigoUnicoPG` JÁ EXISTEM (definidos em `marcacoes_repo.go`/`chegadas_repo.go`) — reutiliza-os, NÃO os redefinas.

---

## Contratos partilhados (definidos ao longo do plano)

**Domínio (`internal/domain/recepcao`):**
- `type PrioridadeManchester string`; consts `ManVermelho/ManLaranja/ManAmarelo/ManVerde/ManAzul`; `ParsePrioridade(string) (PrioridadeManchester, error)`; `Severidade() int`; `TempoAlvo() time.Duration` (Task 1).
- `type SinaisVitais struct{...}` (9 campos ponteiro com tags json); `NovosSinaisVitais(SinaisVitais) (SinaisVitais, error)` (Task 2).
- `NovaTriagem(chegadaID, enfermeiroID string, prioridade PrioridadeManchester, sinais SinaisVitais, observacoes string, em time.Time) (*Triagem, error)`; getters; `SnapshotTriagem`; `Snapshot()`; `ReconstruirTriagem`; `type ResumoFilaClinica struct`; `type RepositorioTriagens interface` (Task 3).
- `ChegTriado EstadoChegada = "TRIADO"`; `func (c *Chegada) RegistarTriada(medicoID string, em time.Time) error` (Task 4).

**Aplicação (`internal/application/recepcao`):**
- DTOs `DadosTriagem`, `DetalheTriagem`; reexport `ResumoFilaClinica`; `paraDetalheTriagem` (Task 5).
- `NovoCasoObterTriagem`, `NovoCasoListarFilaClinica` (Task 5); `NovoCasoRegistarTriagem` (Task 6).

**Adaptadores:** `NovoRepositorioTriagens` (Task 8); `NovoRecepcaoTriagemHandler` + `RegistarRecepcaoTriagem` (Task 9).

---

## Task 1: Domínio — VO `PrioridadeManchester`

**Files:**
- Create: `internal/domain/recepcao/prioridade.go`
- Test: `internal/domain/recepcao/prioridade_test.go`

**Interfaces:**
- Produces: `type PrioridadeManchester string` + 5 consts; `ParsePrioridade(codigo string) (PrioridadeManchester, error)`; `func (p PrioridadeManchester) Severidade() int`; `func (p PrioridadeManchester) TempoAlvo() time.Duration`.

- [ ] **Step 1: Write the failing test**

```go
// internal/domain/recepcao/prioridade_test.go
package recepcao_test

import (
	"testing"
	"time"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestParsePrioridade_ValidaENormaliza(t *testing.T) {
	casos := map[string]recepcao.PrioridadeManchester{
		"VERMELHO": recepcao.ManVermelho,
		"laranja":  recepcao.ManLaranja,
		" Amarelo ": recepcao.ManAmarelo,
		"VERDE":    recepcao.ManVerde,
		"azul":     recepcao.ManAzul,
	}
	for entrada, esperado := range casos {
		got, err := recepcao.ParsePrioridade(entrada)
		if err != nil {
			t.Fatalf("ParsePrioridade(%q): erro inesperado %v", entrada, err)
		}
		if got != esperado {
			t.Fatalf("ParsePrioridade(%q) = %q; esperava %q", entrada, got, esperado)
		}
	}
}

func TestParsePrioridade_Invalida(t *testing.T) {
	if _, err := recepcao.ParsePrioridade("ROXO"); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestSeveridade_OrdemDeUrgencia(t *testing.T) {
	// menor severidade = mais urgente
	ordem := []recepcao.PrioridadeManchester{
		recepcao.ManVermelho, recepcao.ManLaranja, recepcao.ManAmarelo, recepcao.ManVerde, recepcao.ManAzul,
	}
	for i := 1; i < len(ordem); i++ {
		if ordem[i-1].Severidade() >= ordem[i].Severidade() {
			t.Fatalf("%s (%d) devia ser mais urgente que %s (%d)",
				ordem[i-1], ordem[i-1].Severidade(), ordem[i], ordem[i].Severidade())
		}
	}
	if recepcao.ManVermelho.Severidade() != 1 || recepcao.ManAzul.Severidade() != 5 {
		t.Fatal("VERMELHO devia ser 1 e AZUL 5")
	}
}

func TestTempoAlvo(t *testing.T) {
	casos := map[recepcao.PrioridadeManchester]time.Duration{
		recepcao.ManVermelho: 0,
		recepcao.ManLaranja:  10 * time.Minute,
		recepcao.ManAmarelo:  60 * time.Minute,
		recepcao.ManVerde:    120 * time.Minute,
		recepcao.ManAzul:     240 * time.Minute,
	}
	for p, esperado := range casos {
		if p.TempoAlvo() != esperado {
			t.Fatalf("%s.TempoAlvo() = %v; esperava %v", p, p.TempoAlvo(), esperado)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/recepcao/... -run Prioridade`
Expected: FAIL — `undefined: recepcao.ParsePrioridade`.

- [ ] **Step 3: Write the VO**

```go
// internal/domain/recepcao/prioridade.go
package recepcao

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// PrioridadeManchester é a classificação de prioridade da triagem pelo Sistema de
// Triagem de Manchester (5 cores, cada uma com um tempo-alvo máximo de espera).
type PrioridadeManchester string

const (
	ManVermelho PrioridadeManchester = "VERMELHO" // Emergente     — 0 min
	ManLaranja  PrioridadeManchester = "LARANJA"  // Muito urgente — 10 min
	ManAmarelo  PrioridadeManchester = "AMARELO"  // Urgente       — 60 min
	ManVerde    PrioridadeManchester = "VERDE"    // Pouco urgente — 120 min
	ManAzul     PrioridadeManchester = "AZUL"     // Não urgente   — 240 min
)

// atributosManchester guarda a severidade (1 = mais urgente) e o tempo-alvo de cada cor.
var atributosManchester = map[PrioridadeManchester]struct {
	severidade int
	tempoAlvo  time.Duration
}{
	ManVermelho: {1, 0},
	ManLaranja:  {2, 10 * time.Minute},
	ManAmarelo:  {3, 60 * time.Minute},
	ManVerde:    {4, 120 * time.Minute},
	ManAzul:     {5, 240 * time.Minute},
}

// ParsePrioridade valida e normaliza uma cor de Manchester (aceita minúsculas e espaços).
func ParsePrioridade(codigo string) (PrioridadeManchester, error) {
	p := PrioridadeManchester(strings.ToUpper(strings.TrimSpace(codigo)))
	if _, ok := atributosManchester[p]; !ok {
		return "", erros.Novo(erros.CategoriaValidacao,
			"prioridade de triagem inválida (esperado VERMELHO, LARANJA, AMARELO, VERDE ou AZUL)")
	}
	return p, nil
}

// Severidade devolve a ordem de urgência: 1 (VERMELHO, mais urgente) a 5 (AZUL). Usada
// para ordenar a fila clínica. Uma cor desconhecida devolve 99 (fica no fim).
func (p PrioridadeManchester) Severidade() int {
	if a, ok := atributosManchester[p]; ok {
		return a.severidade
	}
	return 99
}

// TempoAlvo devolve o tempo máximo de espera recomendado para a cor.
func (p PrioridadeManchester) TempoAlvo() time.Duration {
	return atributosManchester[p].tempoAlvo
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/recepcao/... -cover`
Expected: PASS, cobertura ≥85%.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/recepcao/prioridade.go internal/domain/recepcao/prioridade_test.go
git commit -m "feat(recepcao): VO PrioridadeManchester (5 cores, severidade e tempo-alvo)"
```

---

## Task 2: Domínio — VO `SinaisVitais`

**Files:**
- Create: `internal/domain/recepcao/sinais_vitais.go`
- Test: `internal/domain/recepcao/sinais_vitais_test.go`

**Interfaces:**
- Produces: `type SinaisVitais struct` (9 campos ponteiro com tags json); `NovosSinaisVitais(c SinaisVitais) (SinaisVitais, error)`.

- [ ] **Step 1: Write the failing test**

```go
// internal/domain/recepcao/sinais_vitais_test.go
package recepcao_test

import (
	"testing"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func iptr(v int) *int         { return &v }
func fptr(v float64) *float64 { return &v }

func TestNovosSinaisVitais_VazioEValido(t *testing.T) {
	sv, err := recepcao.NovosSinaisVitais(recepcao.SinaisVitais{})
	if err != nil {
		t.Fatalf("um conjunto vazio devia ser válido: %v", err)
	}
	if sv.TensaoSistolica != nil || sv.Peso != nil {
		t.Fatal("campos não medidos deviam ficar nil")
	}
}

func TestNovosSinaisVitais_ValoresPlausiveis(t *testing.T) {
	_, err := recepcao.NovosSinaisVitais(recepcao.SinaisVitais{
		TensaoSistolica: iptr(120), TensaoDiastolica: iptr(80), FrequenciaCardiaca: iptr(72),
		Temperatura: fptr(36.6), FrequenciaRespiratoria: iptr(16), SaturacaoO2: iptr(98),
		Dor: iptr(3), Glicemia: iptr(95), Peso: fptr(70.5),
	})
	if err != nil {
		t.Fatalf("valores plausíveis não deviam falhar: %v", err)
	}
}

func TestNovosSinaisVitais_ForaDeIntervalo(t *testing.T) {
	casos := []recepcao.SinaisVitais{
		{TensaoSistolica: iptr(400)},        // > 300
		{TensaoDiastolica: iptr(10)},        // < 30
		{FrequenciaCardiaca: iptr(500)},     // > 300
		{Temperatura: fptr(50)},             // > 45
		{FrequenciaRespiratoria: iptr(2)},   // < 5
		{SaturacaoO2: iptr(40)},             // < 50
		{Dor: iptr(11)},                     // > 10
		{Glicemia: iptr(5)},                 // < 20
		{Peso: fptr(0.1)},                   // < 0.5
	}
	for i, c := range casos {
		if _, err := recepcao.NovosSinaisVitais(c); erros.CategoriaDe(err) != erros.CategoriaValidacao {
			t.Fatalf("caso %d: esperava CategoriaValidacao, veio %v", i, erros.CategoriaDe(err))
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/recepcao/... -run SinaisVitais`
Expected: FAIL — `undefined: recepcao.NovosSinaisVitais`.

- [ ] **Step 3: Write the VO**

```go
// internal/domain/recepcao/sinais_vitais.go
package recepcao

import (
	"fmt"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// SinaisVitais é um value object com os sinais vitais medidos na triagem. Todos os
// campos são opcionais (ponteiro nil = não medido). Os intervalos validados são limites
// de sanidade (rejeitam erros de digitação), não intervalos de normalidade clínica.
type SinaisVitais struct {
	TensaoSistolica        *int     `json:"tensao_sistolica,omitempty"`
	TensaoDiastolica       *int     `json:"tensao_diastolica,omitempty"`
	FrequenciaCardiaca     *int     `json:"frequencia_cardiaca,omitempty"`
	Temperatura            *float64 `json:"temperatura,omitempty"`
	FrequenciaRespiratoria *int     `json:"frequencia_respiratoria,omitempty"`
	SaturacaoO2            *int     `json:"saturacao_o2,omitempty"`
	Dor                    *int     `json:"dor,omitempty"`
	Glicemia               *int     `json:"glicemia,omitempty"`
	Peso                   *float64 `json:"peso,omitempty"`
}

// NovosSinaisVitais valida os campos presentes e devolve o VO. Um valor fora do
// intervalo plausível devolve CategoriaValidacao; um conjunto vazio é válido.
func NovosSinaisVitais(c SinaisVitais) (SinaisVitais, error) {
	if err := intNoIntervalo("tensão sistólica", c.TensaoSistolica, 50, 300); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("tensão diastólica", c.TensaoDiastolica, 30, 200); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("frequência cardíaca", c.FrequenciaCardiaca, 20, 300); err != nil {
		return SinaisVitais{}, err
	}
	if err := floatNoIntervalo("temperatura", c.Temperatura, 30, 45); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("frequência respiratória", c.FrequenciaRespiratoria, 5, 80); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("saturação de O2", c.SaturacaoO2, 50, 100); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("escala de dor", c.Dor, 0, 10); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("glicemia", c.Glicemia, 20, 600); err != nil {
		return SinaisVitais{}, err
	}
	if err := floatNoIntervalo("peso", c.Peso, 0.5, 400); err != nil {
		return SinaisVitais{}, err
	}
	return c, nil
}

func intNoIntervalo(nome string, v *int, min, max int) error {
	if v == nil {
		return nil
	}
	if *v < min || *v > max {
		return erros.Novo(erros.CategoriaValidacao,
			fmt.Sprintf("%s fora do intervalo plausível (%d–%d)", nome, min, max))
	}
	return nil
}

func floatNoIntervalo(nome string, v *float64, min, max float64) error {
	if v == nil {
		return nil
	}
	if *v < min || *v > max {
		return erros.Novo(erros.CategoriaValidacao,
			fmt.Sprintf("%s fora do intervalo plausível (%g–%g)", nome, min, max))
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/recepcao/... -cover`
Expected: PASS, cobertura ≥85%.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/recepcao/sinais_vitais.go internal/domain/recepcao/sinais_vitais_test.go
git commit -m "feat(recepcao): VO SinaisVitais com validacao de intervalos plausiveis"
```

---

## Task 3: Domínio — Agregado `Triagem` + porta e read-model

**Files:**
- Create: `internal/domain/recepcao/triagem.go`
- Test: `internal/domain/recepcao/triagem_test.go`

**Interfaces:**
- Consumes: `PrioridadeManchester`, `SinaisVitais` (Tasks 1–2), `Chegada` (existente).
- Produces: `NovaTriagem(chegadaID, enfermeiroID string, prioridade PrioridadeManchester, sinais SinaisVitais, observacoes string, em time.Time) (*Triagem, error)`; getters `ID/ChegadaID/Prioridade/SinaisVitais/Observacoes/EnfermeiroID/TriadaEm`; `type SnapshotTriagem struct`; `Snapshot()`; `ReconstruirTriagem(SnapshotTriagem) *Triagem`; `type ResumoFilaClinica struct`; `type RepositorioTriagens interface`.

- [ ] **Step 1: Write the failing test**

```go
// internal/domain/recepcao/triagem_test.go
package recepcao_test

import (
	"testing"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovaTriagem_Valida(t *testing.T) {
	sv, _ := recepcao.NovosSinaisVitais(recepcao.SinaisVitais{Temperatura: fptr(37.0)})
	tr, err := recepcao.NovaTriagem("cheg-1", "enf-1", recepcao.ManAmarelo, sv, "cefaleia", inst("09:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if tr.ChegadaID() != "cheg-1" || tr.EnfermeiroID() != "enf-1" || tr.Prioridade() != recepcao.ManAmarelo {
		t.Fatalf("campos mal preenchidos: %+v", tr.Snapshot())
	}
	if tr.SinaisVitais().Temperatura == nil || *tr.SinaisVitais().Temperatura != 37.0 {
		t.Fatal("sinais vitais mal preenchidos")
	}
}

func TestNovaTriagem_CamposObrigatorios(t *testing.T) {
	sv := recepcao.SinaisVitais{}
	if _, err := recepcao.NovaTriagem("", "enf-1", recepcao.ManVerde, sv, "", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("sem chegada: esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
	if _, err := recepcao.NovaTriagem("cheg-1", "  ", recepcao.ManVerde, sv, "", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("sem enfermeiro: esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestNovaTriagem_PrioridadeInvalida(t *testing.T) {
	if _, err := recepcao.NovaTriagem("cheg-1", "enf-1", recepcao.PrioridadeManchester("ROXO"), recepcao.SinaisVitais{}, "", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestTriagem_RoundTrip(t *testing.T) {
	sv, _ := recepcao.NovosSinaisVitais(recepcao.SinaisVitais{Dor: iptr(5)})
	s := recepcao.SnapshotTriagem{
		ID: "tri-1", ChegadaID: "cheg-1", EnfermeiroID: "enf-1",
		Prioridade: recepcao.ManLaranja, SinaisVitais: sv, Observacoes: "x", TriadaEm: inst("09:00"),
	}
	tr := recepcao.ReconstruirTriagem(s)
	got := tr.Snapshot()
	if got.ID != "tri-1" || got.Prioridade != recepcao.ManLaranja || got.SinaisVitais.Dor == nil || *got.SinaisVitais.Dor != 5 {
		t.Fatalf("round-trip não preserva: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/recepcao/... -run Triagem`
Expected: FAIL — `undefined: recepcao.NovaTriagem`.

- [ ] **Step 3: Write the aggregate + port + read-model**

```go
// internal/domain/recepcao/triagem.go
package recepcao

import (
	"context"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Triagem é um agregado raiz do BC Recepção: o registo clínico da triagem de uma
// chegada — a prioridade de Manchester e os sinais vitais. É imutável após criação (não
// tem máquina de estados). Refere a chegada e o enfermeiro por id.
type Triagem struct {
	id           string
	chegadaID    string
	prioridade   PrioridadeManchester
	sinaisVitais SinaisVitais
	observacoes  string
	enfermeiroID string
	triadaEm     time.Time
	criadoEm     time.Time
}

// NovaTriagem valida e constrói uma triagem. Chegada, enfermeiro, uma prioridade válida
// e o instante são obrigatórios; os sinais vitais assumem-se já validados (VO
// SinaisVitais). O enfermeiro é o sujeito autenticado (na aplicação), nunca do corpo.
func NovaTriagem(chegadaID, enfermeiroID string, prioridade PrioridadeManchester, sinais SinaisVitais, observacoes string, em time.Time) (*Triagem, error) {
	chegadaID = strings.TrimSpace(chegadaID)
	if chegadaID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "chegada da triagem em falta")
	}
	enfermeiroID = strings.TrimSpace(enfermeiroID)
	if enfermeiroID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "enfermeiro da triagem em falta")
	}
	p, err := ParsePrioridade(string(prioridade))
	if err != nil {
		return nil, err
	}
	if em.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "instante da triagem em falta")
	}
	return &Triagem{
		chegadaID: chegadaID, enfermeiroID: enfermeiroID, prioridade: p,
		sinaisVitais: sinais, observacoes: strings.TrimSpace(observacoes), triadaEm: em,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados.
func (t *Triagem) ID() string { return t.id }

// ChegadaID devolve a chegada triada.
func (t *Triagem) ChegadaID() string { return t.chegadaID }

// Prioridade devolve a cor de Manchester.
func (t *Triagem) Prioridade() PrioridadeManchester { return t.prioridade }

// SinaisVitais devolve os sinais vitais registados.
func (t *Triagem) SinaisVitais() SinaisVitais { return t.sinaisVitais }

// Observacoes devolve as observações livres (vazio se não houver).
func (t *Triagem) Observacoes() string { return t.observacoes }

// EnfermeiroID devolve o enfermeiro triador.
func (t *Triagem) EnfermeiroID() string { return t.enfermeiroID }

// TriadaEm devolve o instante da triagem.
func (t *Triagem) TriadaEm() time.Time { return t.triadaEm }

// SnapshotTriagem carrega o estado completo para persistência ou rehidratação.
type SnapshotTriagem struct {
	ID           string
	ChegadaID    string
	Prioridade   PrioridadeManchester
	SinaisVitais SinaisVitais
	Observacoes  string
	EnfermeiroID string
	TriadaEm     time.Time
	CriadoEm     time.Time
}

// Snapshot devolve o estado completo do agregado.
func (t *Triagem) Snapshot() SnapshotTriagem {
	return SnapshotTriagem{
		ID: t.id, ChegadaID: t.chegadaID, Prioridade: t.prioridade,
		SinaisVitais: t.sinaisVitais, Observacoes: t.observacoes,
		EnfermeiroID: t.enfermeiroID, TriadaEm: t.triadaEm, CriadoEm: t.criadoEm,
	}
}

// ReconstruirTriagem reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirTriagem(s SnapshotTriagem) *Triagem {
	return &Triagem{
		id: s.ID, chegadaID: s.ChegadaID, prioridade: s.Prioridade,
		sinaisVitais: s.SinaisVitais, observacoes: s.Observacoes,
		enfermeiroID: s.EnfermeiroID, triadaEm: s.TriadaEm, criadoEm: s.CriadoEm,
	}
}

// ResumoFilaClinica é a projecção de leitura de uma linha da fila clínica (chegada
// triada à espera do médico).
type ResumoFilaClinica struct {
	ChegadaID       string    `json:"chegada_id"`
	TriagemID       string    `json:"triagem_id"`
	DoenteID        string    `json:"doente_id"`
	MedicoID        string    `json:"medico_id"`
	EspecialidadeID string    `json:"especialidade_id"`
	Prioridade      string    `json:"prioridade"`
	HoraChegada     time.Time `json:"hora_chegada"`
	TriadaEm        time.Time `json:"triada_em"`
}

// RepositorioTriagens é a porta de saída de persistência de triagens.
//
// RegistarTriagem grava, numa única transacção, a chegada a passar a TRIADO (guarda
// compare-and-set sobre CHAMADO, com o médico atribuído) e a nova triagem — um registo
// que transitasse a chegada sem criar a triagem (ou vice-versa) deixaria a recepção
// incoerente. ListarFilaClinica devolve as chegadas TRIADO ordenadas por severidade de
// Manchester (mais urgente primeiro) e depois por hora de chegada; médico vazio = todos.
type RepositorioTriagens interface {
	RegistarTriagem(ctx context.Context, triagem *Triagem, chegada *Chegada) (string, error)
	ObterPorChegada(ctx context.Context, chegadaID string) (*Triagem, error)
	ListarFilaClinica(ctx context.Context, medicoID string) ([]ResumoFilaClinica, error)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/recepcao/... -cover`
Expected: PASS, cobertura ≥85%.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/recepcao/triagem.go internal/domain/recepcao/triagem_test.go
git commit -m "feat(recepcao): agregado Triagem (imutavel) + porta RepositorioTriagens e fila clinica"
```

---

## Task 4: Domínio — estado `TRIADO` e `RegistarTriada` na `Chegada`

**Files:**
- Modify: `internal/domain/recepcao/chegada.go`
- Test: `internal/domain/recepcao/chegada_test.go`

**Interfaces:**
- Produces: `ChegTriado EstadoChegada = "TRIADO"`; `func (c *Chegada) RegistarTriada(medicoID string, em time.Time) error`.

- [ ] **Step 1: Write the failing test**

Acrescenta a `internal/domain/recepcao/chegada_test.go`:

```go
func chegadaChamada(t *testing.T, walkin bool) *recepcao.Chegada {
	t.Helper()
	var c *recepcao.Chegada
	var err error
	if walkin {
		c, err = recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	} else {
		c, err = recepcao.NovaChegadaAgendada("doe-1", "marc-1", "med-1", "esp-1", inst("09:00"))
	}
	if err != nil {
		t.Fatalf("chegada inválida: %v", err)
	}
	if err := c.Chamar(inst("09:05")); err != nil {
		t.Fatalf("chamar: %v", err)
	}
	return c
}

func TestRegistarTriada_WalkIn_AtribuiMedico(t *testing.T) {
	c := chegadaChamada(t, true) // walk-in, sem médico
	if err := c.RegistarTriada("med-9", inst("09:10")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.Estado() != recepcao.ChegTriado {
		t.Fatalf("esperava TRIADO, veio %s", c.Estado())
	}
	if c.MedicoID() != "med-9" {
		t.Fatalf("o médico do walk-in devia ser atribuído, veio %q", c.MedicoID())
	}
}

func TestRegistarTriada_WalkIn_SemMedico_Validacao(t *testing.T) {
	c := chegadaChamada(t, true)
	if err := c.RegistarTriada("", inst("09:10")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("walk-in sem médico devia dar CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestRegistarTriada_Agendada_HerdaMedico(t *testing.T) {
	c := chegadaChamada(t, false) // agendada, já com med-1
	if err := c.RegistarTriada("", inst("09:10")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.MedicoID() != "med-1" {
		t.Fatalf("devia herdar o médico da marcação, veio %q", c.MedicoID())
	}
}

func TestRegistarTriada_Agendada_MedicoIndevido_Validacao(t *testing.T) {
	c := chegadaChamada(t, false)
	if err := c.RegistarTriada("med-9", inst("09:10")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("re-atribuir médico a chegada agendada devia dar CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestRegistarTriada_ForaDeChamado_Conflito(t *testing.T) {
	c, _ := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00")) // AGUARDA, não CHAMADO
	if err := c.RegistarTriada("med-9", inst("09:10")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("triar uma chegada não chamada devia dar CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/recepcao/... -run Triada`
Expected: FAIL — `undefined: recepcao.ChegTriado`.

- [ ] **Step 3: Add the state constant and method**

Em `internal/domain/recepcao/chegada.go`, acrescenta `ChegTriado` ao bloco `const` e actualiza o comentário do enum:

```go
// EstadoChegada é o estado do ciclo de vida de uma chegada (o doente na fila).
//
//	AGUARDA ─┬─ Chamar ──────────► CHAMADO ─ RegistarTriada ► TRIADO (fila clínica)
//	         └─ RegistarDesistencia► DESISTIU
type EstadoChegada string

const (
	ChegAguarda  EstadoChegada = "AGUARDA"
	ChegChamado  EstadoChegada = "CHAMADO"
	ChegDesistiu EstadoChegada = "DESISTIU"
	ChegTriado   EstadoChegada = "TRIADO"
)
```

E, a seguir a `RegistarDesistencia`, acrescenta:

```go
// RegistarTriada transita CHAMADO → TRIADO (a triagem foi registada). Numa chegada sem
// médico (walk-in) o medicoID é obrigatório e fica atribuído; numa chegada que já tem
// médico (agendada) o medicoID tem de vir vazio — herda-se o médico da marcação, não se
// re-atribui.
func (c *Chegada) RegistarTriada(medicoID string, em time.Time) error {
	if c.estado != ChegChamado {
		return erros.Novo(erros.CategoriaConflito, "só é possível triar uma chegada que foi chamada")
	}
	medicoID = strings.TrimSpace(medicoID)
	if c.medicoID == "" {
		if medicoID == "" {
			return erros.Novo(erros.CategoriaValidacao, "médico a atribuir ao walk-in em falta")
		}
		c.medicoID = medicoID
	} else if medicoID != "" {
		return erros.Novo(erros.CategoriaValidacao, "a chegada já tem médico atribuído")
	}
	c.estado = ChegTriado
	c.actualizadoEm = em
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/recepcao/... -cover`
Expected: PASS, cobertura ≥85%.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/recepcao/chegada.go internal/domain/recepcao/chegada_test.go
git commit -m "feat(recepcao): estado TRIADO e RegistarTriada na Chegada (atribui medico ao walk-in)"
```

---

## Task 5: Aplicação — ports, fakes e casos de leitura (obter triagem, fila clínica)

**Files:**
- Modify: `internal/application/recepcao/ports.go` (DTOs `DadosTriagem`/`DetalheTriagem`, reexport, `paraDetalheTriagem`)
- Create: `internal/application/recepcao/triagens.go`
- Modify: `internal/application/recepcao/fakes_test.go` (`fakeTriagens`)
- Test: `internal/application/recepcao/triagens_test.go`

**Interfaces:**
- Consumes: `RepositorioTriagens`, `ResumoFilaClinica`, `Triagem`, `SinaisVitais` (Tasks 1–3), `Auditor` (existente).
- Produces: `type DadosTriagem struct`; `type DetalheTriagem struct`; reexport `ResumoFilaClinica`; `paraDetalheTriagem(*dominio.Triagem) DetalheTriagem`; `NovoCasoObterTriagem(dominio.RepositorioTriagens)` (`Executar(ctx, chegadaID string) (DetalheTriagem, error)`); `NovoCasoListarFilaClinica(dominio.RepositorioTriagens)` (`Executar(ctx, medicoID string) ([]ResumoFilaClinica, error)`).

- [ ] **Step 1: Add the DTOs + mapper to ports.go**

No bloco de reexports de `internal/application/recepcao/ports.go` (junto de `ResumoMarcacao`/`ResumoChegada`):

```go
	ResumoFilaClinica = dominio.ResumoFilaClinica
```

E no fim de `ports.go`:

```go
// DadosTriagem é a entrada do registo de uma triagem. Os sinais vitais são opcionais.
// O MedicoID só é usado no walk-in (atribuição). O enfermeiro triador vem da sessão.
type DadosTriagem struct {
	Prioridade             string   `json:"prioridade"`
	TensaoSistolica        *int     `json:"tensao_sistolica"`
	TensaoDiastolica       *int     `json:"tensao_diastolica"`
	FrequenciaCardiaca     *int     `json:"frequencia_cardiaca"`
	Temperatura            *float64 `json:"temperatura"`
	FrequenciaRespiratoria *int     `json:"frequencia_respiratoria"`
	SaturacaoO2            *int     `json:"saturacao_o2"`
	Dor                    *int     `json:"dor"`
	Glicemia               *int     `json:"glicemia"`
	Peso                   *float64 `json:"peso"`
	Observacoes            string   `json:"observacoes"`
	MedicoID               string   `json:"medico_id"`
}

// DetalheTriagem é o detalhe de uma triagem numa resposta.
type DetalheTriagem struct {
	ID           string               `json:"id"`
	ChegadaID    string               `json:"chegada_id"`
	EnfermeiroID string               `json:"enfermeiro_id"`
	Prioridade   string               `json:"prioridade"`
	SinaisVitais dominio.SinaisVitais `json:"sinais_vitais"`
	Observacoes  string               `json:"observacoes,omitempty"`
	TriadaEm     time.Time            `json:"triada_em"`
}

// paraDetalheTriagem projecta o agregado para o read-model de resposta.
func paraDetalheTriagem(t *dominio.Triagem) DetalheTriagem {
	s := t.Snapshot()
	return DetalheTriagem{
		ID: s.ID, ChegadaID: s.ChegadaID, EnfermeiroID: s.EnfermeiroID,
		Prioridade: string(s.Prioridade), SinaisVitais: s.SinaisVitais,
		Observacoes: s.Observacoes, TriadaEm: s.TriadaEm,
	}
}
```

**Nota ao implementador:** confirma que `ports.go` já importa `time` e `dominio` (importa — são usados pelos DTOs existentes).

- [ ] **Step 2: Add the fake to fakes_test.go**

Acrescenta a `internal/application/recepcao/fakes_test.go`:

```go
// fakeTriagens guarda triagens em memória. RegistarTriagem também transita a chegada no
// fakeChegadas injectado (coordenação cross-agregado).
type fakeTriagens struct {
	dados    map[string]*dominio.Triagem     // por id
	porCheg  map[string]*dominio.Triagem     // por chegadaID
	seq      int
	chegadas *fakeChegadas
}

func novoFakeTriagens(c *fakeChegadas) *fakeTriagens {
	return &fakeTriagens{dados: map[string]*dominio.Triagem{}, porCheg: map[string]*dominio.Triagem{}, chegadas: c}
}

func (f *fakeTriagens) RegistarTriagem(ctx context.Context, triagem *dominio.Triagem, chegada *dominio.Chegada) (string, error) {
	if err := f.chegadas.Transitar(ctx, chegada); err != nil {
		return "", err
	}
	f.seq++
	id := "tri-" + itoa(f.seq)
	s := triagem.Snapshot()
	s.ID = id
	guardada := dominio.ReconstruirTriagem(s)
	f.dados[id] = guardada
	f.porCheg[s.ChegadaID] = guardada
	return id, nil
}

func (f *fakeTriagens) ObterPorChegada(_ context.Context, chegadaID string) (*dominio.Triagem, error) {
	t, ok := f.porCheg[chegadaID]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "triagem não encontrada")
	}
	return dominio.ReconstruirTriagem(t.Snapshot()), nil
}

func (f *fakeTriagens) ListarFilaClinica(_ context.Context, medicoID string) ([]dominio.ResumoFilaClinica, error) {
	var out []dominio.ResumoFilaClinica
	for _, t := range f.dados {
		s := t.Snapshot()
		ch, err := f.chegadas.ObterPorID(context.Background(), s.ChegadaID)
		if err != nil {
			continue
		}
		cs := ch.Snapshot()
		if cs.Estado != dominio.ChegTriado {
			continue
		}
		if medicoID != "" && cs.MedicoID != medicoID {
			continue
		}
		out = append(out, dominio.ResumoFilaClinica{
			ChegadaID: cs.ID, TriagemID: s.ID, DoenteID: cs.DoenteID, MedicoID: cs.MedicoID,
			EspecialidadeID: cs.EspecialidadeID, Prioridade: string(s.Prioridade),
			HoraChegada: cs.HoraChegada, TriadaEm: s.TriadaEm,
		})
	}
	// ordena por severidade de Manchester, depois por hora de chegada
	sortFilaClinica(out)
	return out, nil
}

func sortFilaClinica(fila []dominio.ResumoFilaClinica) {
	for i := 1; i < len(fila); i++ {
		for j := i; j > 0 && maisUrgente(fila[j], fila[j-1]); j-- {
			fila[j], fila[j-1] = fila[j-1], fila[j]
		}
	}
}

func maisUrgente(a, b dominio.ResumoFilaClinica) bool {
	sa := dominio.PrioridadeManchester(a.Prioridade).Severidade()
	sb := dominio.PrioridadeManchester(b.Prioridade).Severidade()
	if sa != sb {
		return sa < sb
	}
	return a.HoraChegada.Before(b.HoraChegada)
}

var _ dominio.RepositorioTriagens = (*fakeTriagens)(nil)
```

- [ ] **Step 3: Write the failing test**

```go
// internal/application/recepcao/triagens_test.go
package recepcao_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// triagemAgregada cria uma triagem de domínio para semear os fakes nos testes de leitura.
func triagemAgregada(t *testing.T, chegadaID, enfermeiro string, p dominio.PrioridadeManchester) *dominio.Triagem {
	t.Helper()
	tr, err := dominio.NovaTriagem(chegadaID, enfermeiro, p, dominio.SinaisVitais{}, "", inst("09:10"))
	if err != nil {
		t.Fatalf("triagem inválida no teste: %v", err)
	}
	return tr
}

func TestObterTriagem_DevolveDetalhe(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	// semeia uma chegada TRIADO e a sua triagem
	cid, _ := chegadas.Guardar(context.Background(), chegadaTriadaSemear(t, chegadas, "doe-1", "med-1", "esp-1"))
	_, _ = triagens.RegistarTriagem(context.Background(), triagemAgregada(t, cid, "enf-1", dominio.ManAmarelo), reconstruirTriada(t, chegadas, cid))

	uc := app.NovoCasoObterTriagem(triagens)
	out, err := uc.Executar(context.Background(), cid)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.ChegadaID != cid || out.Prioridade != string(dominio.ManAmarelo) {
		t.Fatalf("detalhe mal preenchido: %+v", out)
	}
}

func TestListarFilaClinica_OrdenadaPorPrioridade(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	// duas chegadas TRIADO do mesmo médico, prioridades diferentes
	c1 := semearChegadaTriada(t, chegadas, "doe-1", "med-1", "esp-1", "09:00")
	c2 := semearChegadaTriada(t, chegadas, "doe-2", "med-1", "esp-1", "08:00")
	_, _ = triagens.RegistarTriagem(context.Background(), triagemAgregada(t, c1, "enf-1", dominio.ManVerde), reconstruirTriada(t, chegadas, c1))
	_, _ = triagens.RegistarTriagem(context.Background(), triagemAgregada(t, c2, "enf-1", dominio.ManVermelho), reconstruirTriada(t, chegadas, c2))

	uc := app.NovoCasoListarFilaClinica(triagens)
	fila, err := uc.Executar(context.Background(), "med-1")
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if len(fila) != 2 {
		t.Fatalf("esperava 2 na fila, veio %d", len(fila))
	}
	// o VERMELHO (c2) tem de vir primeiro, apesar de ter chegado antes
	if fila[0].ChegadaID != c2 {
		t.Fatalf("o VERMELHO devia vir primeiro na fila, veio %+v", fila)
	}
}

func TestObterTriagem_Inexistente_NaoEncontrado(t *testing.T) {
	triagens := novoFakeTriagens(novoFakeChegadas(novoFakeMarcacoes()))
	uc := app.NovoCasoObterTriagem(triagens)
	if _, err := uc.Executar(context.Background(), "cheg-x"); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava CategoriaNaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}
```

Acrescenta ao `fakes_test.go` os helpers de sementeira:

```go
// semearChegadaTriada cria e persiste uma chegada agendada, chama-a e devolve o id
// (para os testes de leitura da triagem que precisam de uma chegada TRIADO).
func semearChegadaTriada(t *testing.T, f *fakeChegadas, doe, medico, esp, hora string) string {
	t.Helper()
	c, err := dominio.NovaChegadaAgendada(doe, "marc-"+doe, medico, esp, inst(hora))
	if err != nil {
		t.Fatalf("chegada inválida: %v", err)
	}
	_ = c.Chamar(inst(hora))
	id, _ := f.Guardar(context.Background(), c)
	return id
}

func chegadaTriadaSemear(t *testing.T, f *fakeChegadas, doe, medico, esp string) *dominio.Chegada {
	t.Helper()
	c, err := dominio.NovaChegadaAgendada(doe, "marc-"+doe, medico, esp, inst("09:00"))
	if err != nil {
		t.Fatalf("chegada inválida: %v", err)
	}
	_ = c.Chamar(inst("09:05"))
	return c
}

// reconstruirTriada lê a chegada guardada, chama-a de novo em memória e transita-a a
// TRIADO para o fakeTriagens.RegistarTriagem poder gravar a transição.
func reconstruirTriada(t *testing.T, f *fakeChegadas, chegadaID string) *dominio.Chegada {
	t.Helper()
	c, err := f.ObterPorID(context.Background(), chegadaID)
	if err != nil {
		t.Fatalf("obter chegada: %v", err)
	}
	if err := c.RegistarTriada("", inst("09:10")); err != nil {
		t.Fatalf("registar triada: %v", err)
	}
	return c
}
```

**Nota ao implementador:** os helpers de sementeira acima existem para os testes de leitura poderem montar o estado. `semearChegadaTriada` deixa a chegada em `CHAMADO` (para depois `reconstruirTriada` a transitar). Confirma que os nomes não colidem com helpers já no `fakes_test.go`.

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/application/recepcao/... -run "ObterTriagem|FilaClinica"`
Expected: FAIL — `undefined: app.NovoCasoObterTriagem`.

- [ ] **Step 5: Write the read use cases**

```go
// internal/application/recepcao/triagens.go
package recepcao

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
)

// CasoObterTriagem lê a triagem de uma chegada.
type CasoObterTriagem struct {
	triagens dominio.RepositorioTriagens
}

// NovoCasoObterTriagem constrói o caso de uso.
func NovoCasoObterTriagem(t dominio.RepositorioTriagens) *CasoObterTriagem {
	return &CasoObterTriagem{triagens: t}
}

// Executar devolve o detalhe da triagem de uma chegada.
func (uc *CasoObterTriagem) Executar(ctx context.Context, chegadaID string) (DetalheTriagem, error) {
	t, err := uc.triagens.ObterPorChegada(ctx, chegadaID)
	if err != nil {
		return DetalheTriagem{}, err
	}
	return paraDetalheTriagem(t), nil
}

// CasoListarFilaClinica lê a fila clínica (chegadas TRIADO por prioridade).
type CasoListarFilaClinica struct {
	triagens dominio.RepositorioTriagens
}

// NovoCasoListarFilaClinica constrói o caso de uso.
func NovoCasoListarFilaClinica(t dominio.RepositorioTriagens) *CasoListarFilaClinica {
	return &CasoListarFilaClinica{triagens: t}
}

// Executar devolve a fila clínica; médico vazio = todos.
func (uc *CasoListarFilaClinica) Executar(ctx context.Context, medicoID string) ([]ResumoFilaClinica, error) {
	return uc.triagens.ListarFilaClinica(ctx, medicoID)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/application/recepcao/... -cover`
Expected: PASS, cobertura ≥75%.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/recepcao internal/application/recepcao/ports.go internal/application/recepcao/triagens.go internal/application/recepcao/fakes_test.go internal/application/recepcao/triagens_test.go
git commit -m "feat(recepcao): DTOs de triagem, fake e casos de leitura (obter triagem, fila clinica)"
```

---

## Task 6: Aplicação — registo de triagem (coordenação transaccional)

**Files:**
- Modify: `internal/application/recepcao/triagens.go` (adicionar `CasoRegistarTriagem`)
- Test: `internal/application/recepcao/triagens_test.go` (adicionar testes)

**Interfaces:**
- Consumes: `RepositorioTriagens.RegistarTriagem`, `RepositorioChegadas.ObterPorID`, `Chegada.RegistarTriada` (Task 4), `dominio.NovosSinaisVitais`, `dominio.NovaTriagem`, `dominio.ParsePrioridade`.
- Produces: `NovoCasoRegistarTriagem(dominio.RepositorioTriagens, dominio.RepositorioChegadas, Auditor)` (`Executar(ctx, actor, chegadaID string, dados DadosTriagem) (DetalheTriagem, error)`).

- [ ] **Step 1: Write the failing test**

Acrescenta a `internal/application/recepcao/triagens_test.go`:

```go
func TestRegistarTriagem_WalkIn_TransitaAtribuiMedicoECria(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	// walk-in chamado (sem médico)
	c, _ := dominio.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	_ = c.Chamar(inst("09:05"))
	cid, _ := chegadas.Guardar(context.Background(), c)

	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarTriagem(triagens, chegadas, aud)
	uc.DefinirRelogio(agoraFixo("09:10"))
	out, err := uc.Executar(context.Background(), "enf-1", cid, app.DadosTriagem{
		Prioridade: "AMARELO", Temperatura: fptr(37.5), MedicoID: "med-9",
	})
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.EnfermeiroID != "enf-1" || out.Prioridade != "AMARELO" {
		t.Fatalf("detalhe mal preenchido: %+v", out)
	}
	// a chegada ficou TRIADO com o médico atribuído
	ch, _ := chegadas.ObterPorID(context.Background(), cid)
	if ch.Estado() != dominio.ChegTriado || ch.MedicoID() != "med-9" {
		t.Fatalf("chegada mal transitada: estado=%s medico=%s", ch.Estado(), ch.MedicoID())
	}
	if !aud.tem("recepcao.triagem.registada") {
		t.Fatal("esperava auditoria recepcao.triagem.registada")
	}
}

func TestRegistarTriagem_SinaisForaDeIntervalo_Validacao(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	c, _ := dominio.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	_ = c.Chamar(inst("09:05"))
	cid, _ := chegadas.Guardar(context.Background(), c)

	uc := app.NovoCasoRegistarTriagem(triagens, chegadas, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("09:10"))
	_, err := uc.Executar(context.Background(), "enf-1", cid, app.DadosTriagem{
		Prioridade: "VERDE", Temperatura: fptr(60), MedicoID: "med-9", // temperatura absurda
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
	// nada foi transitado
	ch, _ := chegadas.ObterPorID(context.Background(), cid)
	if ch.Estado() != dominio.ChegChamado {
		t.Fatalf("a chegada não devia ter transitado, veio %s", ch.Estado())
	}
}

func TestRegistarTriagem_ChegadaNaoChamada_Conflito(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	c, _ := dominio.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00")) // AGUARDA, não chamada
	cid, _ := chegadas.Guardar(context.Background(), c)

	uc := app.NovoCasoRegistarTriagem(triagens, chegadas, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("09:10"))
	_, err := uc.Executar(context.Background(), "enf-1", cid, app.DadosTriagem{Prioridade: "VERDE", MedicoID: "med-9"})
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestRegistarTriagem_Duplicada_Conflito(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	c, _ := dominio.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	_ = c.Chamar(inst("09:05"))
	cid, _ := chegadas.Guardar(context.Background(), c)

	uc := app.NovoCasoRegistarTriagem(triagens, chegadas, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("09:10"))
	if _, err := uc.Executar(context.Background(), "enf-1", cid, app.DadosTriagem{Prioridade: "VERDE", MedicoID: "med-9"}); err != nil {
		t.Fatalf("primeira triagem não devia falhar: %v", err)
	}
	// segunda: a chegada já não está CHAMADO → Conflito
	_, err := uc.Executar(context.Background(), "enf-1", cid, app.DadosTriagem{Prioridade: "VERDE"})
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("triagem duplicada devia dar CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/recepcao/... -run RegistarTriagem`
Expected: FAIL — `undefined: app.NovoCasoRegistarTriagem`.

- [ ] **Step 3: Write the use case**

Acrescenta a `internal/application/recepcao/triagens.go` (e o import de `time`, `auditoria`):

```go
// (ajusta o bloco de imports de triagens.go para:)
import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarTriagem regista a triagem de uma chegada chamada: valida os sinais vitais
// e a prioridade, transita a chegada CHAMADO→TRIADO (atribuindo o médico ao walk-in) e
// cria a triagem, atomicamente.
type CasoRegistarTriagem struct {
	triagens dominio.RepositorioTriagens
	chegadas dominio.RepositorioChegadas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoRegistarTriagem constrói o caso de uso.
func NovoCasoRegistarTriagem(t dominio.RepositorioTriagens, c dominio.RepositorioChegadas, a Auditor) *CasoRegistarTriagem {
	return &CasoRegistarTriagem{triagens: t, chegadas: c, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoRegistarTriagem) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar valida os dados clínicos, transita a chegada e regista a triagem numa
// transacção coordenada. O actor é o enfermeiro triador. Uma triagem sobre uma chegada
// que já não está CHAMADO (segunda triagem) falha com Conflito.
func (uc *CasoRegistarTriagem) Executar(ctx context.Context, actor, chegadaID string, dados DadosTriagem) (DetalheTriagem, error) {
	chegada, err := uc.chegadas.ObterPorID(ctx, chegadaID)
	if err != nil {
		return DetalheTriagem{}, err
	}
	// valida os dados clínicos ANTES de tocar no estado da chegada
	sinais, err := dominio.NovosSinaisVitais(dominio.SinaisVitais{
		TensaoSistolica: dados.TensaoSistolica, TensaoDiastolica: dados.TensaoDiastolica,
		FrequenciaCardiaca: dados.FrequenciaCardiaca, Temperatura: dados.Temperatura,
		FrequenciaRespiratoria: dados.FrequenciaRespiratoria, SaturacaoO2: dados.SaturacaoO2,
		Dor: dados.Dor, Glicemia: dados.Glicemia, Peso: dados.Peso,
	})
	if err != nil {
		return DetalheTriagem{}, err
	}
	triagem, err := dominio.NovaTriagem(chegadaID, actor, dominio.PrioridadeManchester(dados.Prioridade), sinais, dados.Observacoes, uc.agora())
	if err != nil {
		return DetalheTriagem{}, err
	}
	if err := chegada.RegistarTriada(dados.MedicoID, uc.agora()); err != nil {
		return DetalheTriagem{}, err
	}
	id, err := uc.triagens.RegistarTriagem(ctx, triagem, chegada)
	if err != nil {
		return DetalheTriagem{}, err
	}
	triagem = dominio.ReconstruirTriagem(comIDTriagem(triagem.Snapshot(), id))
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.triagem.registada",
		Entidade: "triagem", EntidadeID: id, OcorridoEm: uc.agora(),
		Detalhe: "chegada: " + chegadaID + " prioridade: " + dados.Prioridade,
	}); err != nil {
		return DetalheTriagem{}, err
	}
	return paraDetalheTriagem(triagem), nil
}

// comIDTriagem devolve uma cópia do snapshot com o id preenchido.
func comIDTriagem(s dominio.SnapshotTriagem, id string) dominio.SnapshotTriagem {
	s.ID = id
	return s
}
```

**Nota ao implementador:** `NovaTriagem` valida a prioridade (via `ParsePrioridade`), por isso uma prioridade inválida em `dados.Prioridade` devolve `CategoriaValidacao` neste caminho. Constrói-se a triagem ANTES de `RegistarTriada` para que dados clínicos inválidos não deixem a chegada meio-transitada em memória.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/application/recepcao/... -cover`
Expected: PASS, cobertura ≥75%.

- [ ] **Step 5: Commit**

```bash
git add internal/application/recepcao/triagens.go internal/application/recepcao/triagens_test.go
git commit -m "feat(recepcao): registo de triagem com coordenacao transaccional (TRIADO + medico + triagem)"
```

---

## Task 7: Migração SQL — `0003_triagens.sql`

**Files:**
- Create: `migrations/recepcao/0003_triagens.sql`

**Interfaces:**
- Produces: estado `TRIADO` aceite em `recepcao.chegadas`; tabela `recepcao.triagens` (colunas consumidas pela Task 8).

- [ ] **Step 1: Write the migration**

```sql
-- migrations/recepcao/0003_triagens.sql
-- Bounded Context: recepcao
-- Migration forward-only. Triagem: prioridade de Manchester e sinais vitais.

-- Estende o enum de estado da chegada com TRIADO. A CHECK inline de 0002 tem o nome
-- auto-gerado determinístico chegadas_estado_check (só referencia a coluna estado).
ALTER TABLE recepcao.chegadas DROP CONSTRAINT chegadas_estado_check;
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_estado_check
    CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU','TRIADO'));

-- Triagem: registo clínico imutável, 1:1 com uma chegada. chegada_id é FK interna ao
-- schema; enfermeiro_id/medico_id são referências textuais sem FK. Os sinais vitais são
-- NULL quando não medidos; as CHECK só se aplicam a valores presentes.
CREATE TABLE IF NOT EXISTS recepcao.triagens (
    id                       uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    chegada_id               uuid         NOT NULL REFERENCES recepcao.chegadas(id),
    enfermeiro_id            uuid         NOT NULL,
    prioridade               text         NOT NULL CHECK (prioridade IN
                               ('VERMELHO','LARANJA','AMARELO','VERDE','AZUL')),
    tensao_sistolica         int          CHECK (tensao_sistolica        BETWEEN 50 AND 300),
    tensao_diastolica        int          CHECK (tensao_diastolica       BETWEEN 30 AND 200),
    frequencia_cardiaca      int          CHECK (frequencia_cardiaca     BETWEEN 20 AND 300),
    temperatura              numeric(4,1) CHECK (temperatura             BETWEEN 30 AND 45),
    frequencia_respiratoria  int          CHECK (frequencia_respiratoria BETWEEN 5 AND 80),
    saturacao_o2             int          CHECK (saturacao_o2            BETWEEN 50 AND 100),
    dor                      int          CHECK (dor                     BETWEEN 0 AND 10),
    glicemia                 int          CHECK (glicemia                BETWEEN 20 AND 600),
    peso                     numeric(5,1) CHECK (peso                    BETWEEN 0.5 AND 400),
    observacoes              text,
    triada_em                timestamptz  NOT NULL,
    criado_em                timestamptz  NOT NULL DEFAULT now(),
    -- Uma triagem por chegada (o duplicado é negado também pela guarda CAS do domínio; a
    -- BD fecha a corrida concorrente).
    UNIQUE (chegada_id)
);
CREATE INDEX IF NOT EXISTS idx_triagens_chegada ON recepcao.triagens (chegada_id);
```

- [ ] **Step 2: Verify it embeds and applies against the real DB**

Run: `go test ./migrations/...`
Expected: PASS (o directório `recepcao` já está no `//go:embed`; esta é a 3.ª migração do BC).

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags integration ./tests/integration/... -run Migracoes -count=1 -v`
Expected: PASS — prova que a cadeia 0001→0002→0003 aplica sem erro (o `DROP CONSTRAINT chegadas_estado_check` inclusive). **Se o `DROP CONSTRAINT` falhar por nome, é aqui que aparece** — nesse caso, consulta o nome real com `\d recepcao.chegadas`.

- [ ] **Step 3: Commit**

```bash
git add migrations/recepcao/0003_triagens.sql
git commit -m "feat(recepcao): migration 0003 (TRIADO na chegada, tabela triagens, UNIQUE por chegada)"
```

---

## Task 8: Repositório pgx — `TriagensRepo`

**Files:**
- Create: `internal/adapters/pgrepo/triagens_repo.go`
- Test: `tests/integration/recepcao_triagens_test.go`

**Interfaces:**
- Consumes: `dominio.RepositorioTriagens` (Task 3), `pgxpool.Pool`; reutiliza `querier` e `ehUnica` do pacote.
- Produces: `NovoRepositorioTriagens(*pgxpool.Pool) *RepositorioTriagens` (implementa `dominio.RepositorioTriagens`).

- [ ] **Step 1: Write the failing integration test**

```go
// tests/integration/recepcao_triagens_test.go
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

func TestRecepcaoTriagensRepo_RegistarObterFila(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	chegRepo := pgrepo.NovoRepositorioChegadas(pool)
	triRepo := pgrepo.NovoRepositorioTriagens(pool)
	medico := "77777777-7777-7777-7777-777777777777"
	esp := "88888888-8888-8888-8888-888888888888"

	// walk-in chamado (sem médico)
	c, _ := dominio.NovaChegadaWalkIn("99999999-9999-9999-9999-999999999999", esp, instMarcacao(t, "2026-08-20T09:00:00Z"))
	_ = c.Chamar(instMarcacao(t, "2026-08-20T09:05:00Z"))
	cid, err := chegRepo.Guardar(ctx, c)
	if err != nil {
		t.Fatalf("guardar chegada: %v", err)
	}

	// registar triagem: transita CHAMADO→TRIADO + atribui médico + insere triagem
	obtida, _ := chegRepo.ObterPorID(ctx, cid)
	if err := obtida.RegistarTriada(medico, instMarcacao(t, "2026-08-20T09:10:00Z")); err != nil {
		t.Fatalf("registar triada (domínio): %v", err)
	}
	sinais, _ := dominio.NovosSinaisVitais(dominio.SinaisVitais{Temperatura: fptrI(37.5), SaturacaoO2: iptrI(98)})
	tr, _ := dominio.NovaTriagem(cid, "aaaaaaaa-0000-0000-0000-000000000001", dominio.ManAmarelo, sinais, "cefaleia", instMarcacao(t, "2026-08-20T09:10:00Z"))
	tid, err := triRepo.RegistarTriagem(ctx, tr, obtida)
	if err != nil {
		t.Fatalf("registar triagem (repo): %v", err)
	}

	// a chegada ficou TRIADO com o médico
	rec, _ := chegRepo.ObterPorID(ctx, cid)
	if rec.Estado() != dominio.ChegTriado || rec.MedicoID() != medico {
		t.Fatalf("chegada mal transitada: estado=%s medico=%s", rec.Estado(), rec.MedicoID())
	}

	// obter por chegada devolve a triagem com os sinais vitais
	got, err := triRepo.ObterPorChegada(ctx, cid)
	if err != nil || got.ID() != tid {
		t.Fatalf("obter por chegada: %v (%v)", err, got)
	}
	if got.SinaisVitais().Temperatura == nil || *got.SinaisVitais().Temperatura != 37.5 {
		t.Fatalf("temperatura mal persistida: %+v", got.SinaisVitais())
	}

	// fila clínica do médico devolve esta chegada
	fila, err := triRepo.ListarFilaClinica(ctx, medico)
	if err != nil || len(fila) != 1 || fila[0].ChegadaID != cid {
		t.Fatalf("fila clínica: %v (n=%d)", err, len(fila))
	}

	// segunda triagem da mesma chegada (já TRIADO) → Conflito pela guarda CAS
	obtida2, _ := chegRepo.ObterPorID(ctx, cid) // TRIADO
	segunda := recarregadaComoChamada(t, obtida2, medico)
	tr2, _ := dominio.NovaTriagem(cid, "aaaaaaaa-0000-0000-0000-000000000001", dominio.ManVerde, dominio.SinaisVitais{}, "", instMarcacao(t, "2026-08-20T09:20:00Z"))
	if _, err := triRepo.RegistarTriagem(ctx, tr2, segunda); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("triagem duplicada devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestRecepcaoTriagensRepo_FilaOrdenadaPorPrioridade(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	chegRepo := pgrepo.NovoRepositorioChegadas(pool)
	triRepo := pgrepo.NovoRepositorioTriagens(pool)
	medico := "66666666-6666-6666-6666-666666666666"
	esp := "88888888-8888-8888-8888-888888888888"

	triar := func(doente, hora string, p dominio.PrioridadeManchester) string {
		c, _ := dominio.NovaChegadaWalkIn(doente, esp, instMarcacao(t, hora))
		_ = c.Chamar(instMarcacao(t, hora))
		cid, _ := chegRepo.Guardar(ctx, c)
		obt, _ := chegRepo.ObterPorID(ctx, cid)
		_ = obt.RegistarTriada(medico, instMarcacao(t, hora))
		tr, _ := dominio.NovaTriagem(cid, "aaaaaaaa-0000-0000-0000-000000000001", p, dominio.SinaisVitais{}, "", instMarcacao(t, hora))
		_, _ = triRepo.RegistarTriagem(ctx, tr, obt)
		return cid
	}
	// o VERDE chega primeiro; o VERMELHO chega depois mas é mais urgente
	_ = triar("11111111-2222-2222-2222-222222222222", "2026-08-21T08:00:00Z", dominio.ManVerde)
	vermelho := triar("11111111-3333-3333-3333-333333333333", "2026-08-21T09:00:00Z", dominio.ManVermelho)

	fila, err := triRepo.ListarFilaClinica(ctx, medico)
	if err != nil || len(fila) < 2 {
		t.Fatalf("fila clínica: %v (n=%d)", err, len(fila))
	}
	if fila[0].ChegadaID != vermelho {
		t.Fatalf("o VERMELHO devia vir primeiro, veio %+v", fila[0])
	}
}

// recarregadaComoChamada reconstrói a chegada como se ainda estivesse CHAMADO (o
// EstadoAnterior fica CHAMADO), para exercitar a guarda CAS do repositório contra a
// linha que já está TRIADO na BD.
func recarregadaComoChamada(t *testing.T, c *dominio.Chegada, medico string) *dominio.Chegada {
	t.Helper()
	s := c.Snapshot()
	s.Estado = dominio.ChegTriado
	s.EstadoAnterior = dominio.ChegChamado
	return dominio.ReconstruirChegada(s)
}

func iptrI(v int) *int         { return &v }
func fptrI(v float64) *float64 { return &v }
```

**Nota ao implementador:** o helper de tempo do package `integration_test` é `instMarcacao(t, s)` (RFC3339) — reutiliza-o, NÃO uses `instD` (não existe). `iptrI`/`fptrI` são helpers locais deste ficheiro (nomes distintos dos `iptr`/`fptr` dos testes de domínio, que estão noutro package). Confirma por grep que `iptrI`/`fptrI`/`recarregadaComoChamada` não colidem no package `integration_test`. **Sobre `recarregadaComoChamada`:** como `ReconstruirChegada` re-deriva sempre `estadoAnterior = Estado`, a guarda `EstadoAnterior=CHAMADO` forjada não é honrada; a segunda triagem falha na mesma com Conflito, mas — tal como no check-in — a rejeição vem da guarda CAS `WHERE estado='CHAMADO'` (0 linhas, a BD está TRIADO), retornada ANTES do INSERT. Se preferires provar a CAS de forma inequívoca (recomendado), usa DUAS leituras da chegada CHAMADO **antes** da primeira triagem (`obtidaA`, `obtidaB`), tria com `obtidaA`, e tenta a segunda com `obtidaB` (que ainda "pensa" CHAMADO) — nesse caso apaga `recarregadaComoChamada`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags integration ./tests/integration/... -run RecepcaoTriagens`
Expected: FAIL a compilar — `undefined: pgrepo.NovoRepositorioTriagens`.

- [ ] **Step 3: Write the repository**

```go
// internal/adapters/pgrepo/triagens_repo.go
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioTriagens implementa dominio.RepositorioTriagens com pgx.
type RepositorioTriagens struct {
	pool *pgxpool.Pool
}

// NovoRepositorioTriagens constrói o repositório sobre o pool pgx.
func NovoRepositorioTriagens(pool *pgxpool.Pool) *RepositorioTriagens {
	return &RepositorioTriagens{pool: pool}
}

const colunasTriagem = `id::text, chegada_id::text, enfermeiro_id::text, prioridade,
       tensao_sistolica, tensao_diastolica, frequencia_cardiaca, temperatura,
       frequencia_respiratoria, saturacao_o2, dor, glicemia, peso,
       COALESCE(observacoes,''), triada_em, criado_em`

// RegistarTriagem grava, numa única transacção, a chegada a passar a TRIADO (guarda
// compare-and-set sobre CHAMADO, com o médico atribuído) e a nova triagem.
func (r *RepositorioTriagens) RegistarTriagem(ctx context.Context, triagem *dominio.Triagem, chegada *dominio.Chegada) (string, error) {
	sc := chegada.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção de triagem: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	const upd = `UPDATE recepcao.chegadas SET estado=$2, medico_id=NULLIF($3,'')::uuid, actualizado_em=$4
WHERE id=$1 AND estado=$5`
	ct, err := tx.Exec(ctx, upd, sc.ID, string(sc.Estado), sc.MedicoID, sc.ActualizadoEm, string(sc.EstadoAnterior))
	if err != nil {
		return "", fmt.Errorf("transitar chegada para triada: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", r.erroChegada(ctx, sc.ID)
	}

	st := triagem.Snapshot()
	const ins = `
INSERT INTO recepcao.triagens
    (chegada_id, enfermeiro_id, prioridade, tensao_sistolica, tensao_diastolica,
     frequencia_cardiaca, temperatura, frequencia_respiratoria, saturacao_o2, dor,
     glicemia, peso, observacoes, triada_em)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NULLIF($13,''), $14)
RETURNING id::text`
	sv := st.SinaisVitais
	var id string
	err = tx.QueryRow(ctx, ins, st.ChegadaID, st.EnfermeiroID, string(st.Prioridade),
		sv.TensaoSistolica, sv.TensaoDiastolica, sv.FrequenciaCardiaca, sv.Temperatura,
		sv.FrequenciaRespiratoria, sv.SaturacaoO2, sv.Dor, sv.Glicemia, sv.Peso,
		st.Observacoes, st.TriadaEm).Scan(&id)
	if err != nil {
		if ehUnica(err) {
			return "", erros.Novo(erros.CategoriaConflito, "já existe uma triagem para esta chegada")
		}
		return "", fmt.Errorf("guardar triagem: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar triagem: %w", err)
	}
	return id, nil
}

// ObterPorChegada reconstrói a triagem de uma chegada. NaoEncontrado se não existir.
func (r *RepositorioTriagens) ObterPorChegada(ctx context.Context, chegadaID string) (*dominio.Triagem, error) {
	q := `SELECT ` + colunasTriagem + ` FROM recepcao.triagens WHERE chegada_id=$1`
	var s dominio.SnapshotTriagem
	var prioridade string
	var sv dominio.SinaisVitais
	err := r.pool.QueryRow(ctx, q, chegadaID).Scan(&s.ID, &s.ChegadaID, &s.EnfermeiroID, &prioridade,
		&sv.TensaoSistolica, &sv.TensaoDiastolica, &sv.FrequenciaCardiaca, &sv.Temperatura,
		&sv.FrequenciaRespiratoria, &sv.SaturacaoO2, &sv.Dor, &sv.Glicemia, &sv.Peso,
		&s.Observacoes, &s.TriadaEm, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "triagem não encontrada")
		}
		return nil, fmt.Errorf("obter triagem: %w", err)
	}
	s.Prioridade = dominio.PrioridadeManchester(prioridade)
	s.SinaisVitais = sv
	return dominio.ReconstruirTriagem(s), nil
}

// ListarFilaClinica devolve as chegadas TRIADO com a sua triagem, ordenadas por
// severidade de Manchester (mais urgente primeiro) e depois por hora de chegada. Médico
// vazio = todos.
func (r *RepositorioTriagens) ListarFilaClinica(ctx context.Context, medicoID string) ([]dominio.ResumoFilaClinica, error) {
	const q = `
SELECT c.id::text, t.id::text, c.doente_id::text, COALESCE(c.medico_id::text,''),
       c.especialidade_id::text, t.prioridade, c.hora_chegada, t.triada_em
FROM recepcao.triagens t
JOIN recepcao.chegadas c ON c.id = t.chegada_id
WHERE c.estado='TRIADO' AND ($1='' OR c.medico_id=NULLIF($1,'')::uuid)
ORDER BY CASE t.prioridade
    WHEN 'VERMELHO' THEN 1 WHEN 'LARANJA' THEN 2 WHEN 'AMARELO' THEN 3
    WHEN 'VERDE' THEN 4 WHEN 'AZUL' THEN 5 ELSE 9 END,
    c.hora_chegada, c.id`
	linhas, err := r.pool.Query(ctx, q, medicoID)
	if err != nil {
		return nil, fmt.Errorf("listar fila clínica: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoFilaClinica{}
	for linhas.Next() {
		var rc dominio.ResumoFilaClinica
		if err := linhas.Scan(&rc.ChegadaID, &rc.TriagemID, &rc.DoenteID, &rc.MedicoID,
			&rc.EspecialidadeID, &rc.Prioridade, &rc.HoraChegada, &rc.TriadaEm); err != nil {
			return nil, fmt.Errorf("ler fila clínica: %w", err)
		}
		out = append(out, rc)
	}
	return out, linhas.Err()
}

// erroChegada distingue 404 (chegada inexistente) de 409 (estado mudou) na transição.
func (r *RepositorioTriagens) erroChegada(ctx context.Context, id string) error {
	var existe bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM recepcao.chegadas WHERE id=$1)`, id).Scan(&existe); err != nil {
		return fmt.Errorf("verificar chegada: %w", err)
	}
	if !existe {
		return erros.Novo(erros.CategoriaNaoEncontrado, "chegada não encontrada")
	}
	return erros.Novo(erros.CategoriaConflito,
		"o estado da chegada mudou entretanto; recarregue e repita a operação")
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioTriagens = (*RepositorioTriagens)(nil)
```

**Nota ao implementador:** os campos de sinais vitais são `*int`/`*float64`; o pgx v5 aceita-os como argumentos de INSERT (nil → NULL) e no `Scan` (NULL → nil, valor → aloca). Para `temperatura`/`peso` (`numeric` na BD) o pgx faz o mapeamento para `*float64`. Se algum `Scan` de numeric falhar, o teste de integração apanha-o — nesse caso, confirma o codec (mantém `*float64`; o pgx v5 suporta numeric→float64). `ehUnica`/`querier` já existem no pacote — não os redefinas.

- [ ] **Step 4: Run tests to verify they pass (against the real DB)**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags integration ./tests/integration/... -run RecepcaoTriagens -count=1 -v`
Expected: PASS (registo transaccional, sinais vitais numeric round-trip, fila ordenada por Manchester, duplicado negado). Sem `DATABASE_URL`: SKIP.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/pgrepo/triagens_repo.go tests/integration/recepcao_triagens_test.go
git commit -m "feat(recepcao): repositorio pgx TriagensRepo (registo transaccional, fila clinica Manchester)"
```

---

## Task 9: Handler HTTP — `recepcao_triagem_handler.go`

**Files:**
- Create: `internal/adapters/http/recepcao_triagem_handler.go`
- Test: `internal/adapters/http/recepcao_triagem_test.go`

**Interfaces:**
- Consumes: os 3 casos de uso (Tasks 5–6); `SessaoDe`, `RBAC`, `responderErro`, `Auth`, `i18n.MsgPedidoInvalido`, `dominio.Papel*`.
- Produces: `NovoRecepcaoTriagemHandler(...)` e `RegistarRecepcaoTriagem(r gin.IRouter, h *RecepcaoTriagemHandler, protecao ...gin.HandlerFunc)`.

Handler separado (3.º do BC Recepção) — não estende os handlers de marcação/check-in.

- [ ] **Step 1: Write the failing test**

```go
// internal/adapters/http/recepcao_triagem_test.go
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

type duploRegistarTriagem struct{ actorRecebido string }

func (d *duploRegistarTriagem) Executar(_ context.Context, actor, _ string, _ apprecepcao.DadosTriagem) (apprecepcao.DetalheTriagem, error) {
	d.actorRecebido = actor
	return apprecepcao.DetalheTriagem{ID: "tri-1", ChegadaID: "cheg-1", Prioridade: "AMARELO"}, nil
}

type duploObterTriagem struct{}

func (duploObterTriagem) Executar(_ context.Context, _ string) (apprecepcao.DetalheTriagem, error) {
	return apprecepcao.DetalheTriagem{ID: "tri-1", ChegadaID: "cheg-1", Prioridade: "VERDE"}, nil
}

type duploListarFilaClinica struct{}

func (duploListarFilaClinica) Executar(_ context.Context, _ string) ([]apprecepcao.ResumoFilaClinica, error) {
	return []apprecepcao.ResumoFilaClinica{}, nil
}

func routerTriagem(t *testing.T, registar *duploRegistarTriagem, sessao identidade.Sessao) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := adhttp.NovoRecepcaoTriagemHandler(registar, duploObterTriagem{}, duploListarFilaClinica{})
	adhttp.RegistarRecepcaoTriagem(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestRegistarTriagem_UsaOSujeitoAutenticado(t *testing.T) {
	reg := &duploRegistarTriagem{}
	r := routerTriagem(t, reg, sessaoRecepcaoDe("enf-9", identidade.PapelEnfermeiro))
	corpo, _ := json.Marshal(map[string]any{"prioridade": "AMARELO", "medico_id": "med-1"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/triagem", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	if reg.actorRecebido != "enf-9" {
		t.Fatalf("o enfermeiro devia vir da sessão, veio %q", reg.actorRecebido)
	}
}

func TestRegistarTriagem_Administrativo_Proibido(t *testing.T) {
	r := routerTriagem(t, &duploRegistarTriagem{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	corpo, _ := json.Marshal(map[string]any{"prioridade": "AMARELO"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/triagem", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("o Administrativo não tria: esperava 403, veio %d", w.Code)
	}
}

func TestRegistarTriagem_CorpoMalformado_400(t *testing.T) {
	r := routerTriagem(t, &duploRegistarTriagem{}, sessaoRecepcaoDe("enf-1", identidade.PapelEnfermeiro))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/chegadas/cheg-1/triagem", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}

func TestObterTriagem_Administrativo_Proibido(t *testing.T) {
	r := routerTriagem(t, &duploRegistarTriagem{}, sessaoRecepcaoDe("adm-1", identidade.PapelAdministrativo))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/chegadas/cheg-1/triagem", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("leitura clínica proibida ao Administrativo: esperava 403, veio %d", w.Code)
	}
}

func TestFilaClinica_MedicoPodeLer(t *testing.T) {
	r := routerTriagem(t, &duploRegistarTriagem{}, sessaoRecepcaoDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/recepcao/fila-clinica?medico=med-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("o médico pode ler a fila clínica: esperava 200, veio %d", w.Code)
	}
}
```

**Nota:** `fakeAuth`, `sessaoRecepcaoDe` já existem no pacote `http_test`. Reutiliza-os.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/http/... -run Triagem`
Expected: FAIL — `undefined: adhttp.NovoRecepcaoTriagemHandler`.

- [ ] **Step 3: Write the handler**

```go
// internal/adapters/http/recepcao_triagem_handler.go
//
// Package http (adaptadores) — este ficheiro expõe a Triagem do BC Recepção. Handler
// separado dos de marcação/check-in para manter os construtores enxutos.
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

// Interfaces dos casos de uso da Triagem.
type (
	// ServicoRegistarTriagem regista a triagem de uma chegada.
	ServicoRegistarTriagem interface {
		Executar(ctx context.Context, actor, chegadaID string, dados apprecepcao.DadosTriagem) (apprecepcao.DetalheTriagem, error)
	}
	// ServicoObterTriagem devolve a triagem de uma chegada.
	ServicoObterTriagem interface {
		Executar(ctx context.Context, chegadaID string) (apprecepcao.DetalheTriagem, error)
	}
	// ServicoListarFilaClinica devolve a fila clínica por médico.
	ServicoListarFilaClinica interface {
		Executar(ctx context.Context, medicoID string) ([]apprecepcao.ResumoFilaClinica, error)
	}
)

// RecepcaoTriagemHandler expõe os endpoints HTTP da Triagem.
type RecepcaoTriagemHandler struct {
	registar   ServicoRegistarTriagem
	obter      ServicoObterTriagem
	filaClinica ServicoListarFilaClinica
}

// NovoRecepcaoTriagemHandler constrói o handler.
func NovoRecepcaoTriagemHandler(
	registar ServicoRegistarTriagem, obter ServicoObterTriagem, filaClinica ServicoListarFilaClinica,
) *RecepcaoTriagemHandler {
	return &RecepcaoTriagemHandler{registar: registar, obter: obter, filaClinica: filaClinica}
}

// RegistarRecepcaoTriagem regista as rotas da Triagem. Registar a triagem é de quem a faz
// (Enfermeiro/Médico); a leitura da triagem e da fila clínica é clínica (Médico/Enfermeiro/
// Director) — sem Administrativo/Admin, porque os sinais vitais e a prioridade derivada são
// dado clínico (minimização LPDP).
func RegistarRecepcaoTriagem(r gin.IRouter, h *RecepcaoTriagemHandler, protecao ...gin.HandlerFunc) {
	triagemEscrita := RBAC(dominio.PapelEnfermeiro, dominio.PapelMedico)
	leituraClinica := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelDirector)

	gc := r.Group("/api/v1/chegadas")
	gc.Use(protecao...)
	gc.POST("/:cid/triagem", triagemEscrita, h.registarTriagemHTTP)
	gc.GET("/:cid/triagem", leituraClinica, h.obterTriagemHTTP)

	gf := r.Group("/api/v1/recepcao")
	gf.Use(protecao...)
	gf.GET("/fila-clinica", leituraClinica, h.filaClinicaHTTP)
}

func (h *RecepcaoTriagemHandler) registarTriagemHTTP(c *gin.Context) {
	var dados apprecepcao.DadosTriagem
	if err := c.ShouldBindJSON(&dados); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registar.Executar(c.Request.Context(), actor.Sujeito, c.Param("cid"), dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *RecepcaoTriagemHandler) obterTriagemHTTP(c *gin.Context) {
	out, err := h.obter.Executar(c.Request.Context(), c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *RecepcaoTriagemHandler) filaClinicaHTTP(c *gin.Context) {
	out, err := h.filaClinica.Executar(c.Request.Context(), c.Query("medico"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}
```

**Nota ao implementador:** confirma por grep que os nomes de tipo/interface novos (`RecepcaoTriagemHandler`, `NovoRecepcaoTriagemHandler`, `RegistarRecepcaoTriagem`, `ServicoRegistarTriagem`, `ServicoObterTriagem`, `ServicoListarFilaClinica`) não colidem com nenhum já existente no pacote `http` (ex.: `ServicoListarFila` do laboratório e `ServicoListarFilaChegadas` do check-in existem — `ServicoListarFilaClinica` é distinto). Se colidir, renomeia com sufixo e avisa.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/http/... -cover`
Expected: PASS; cobertura agregada do pacote ≥60%.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/recepcao_triagem_handler.go internal/adapters/http/recepcao_triagem_test.go
git commit -m "feat(recepcao): handler HTTP da triagem (registar, obter, fila clinica) com leitura restrita"
```

---

## Task 10: Composição, ADR-034 e CLAUDE.md

**Files:**
- Modify: `internal/platform/app.go` (montar os repos/casos/handler da triagem + registar rotas)
- Create: `adrs/ADR-034-bc-recepcao-triagem.md`
- Modify: `CLAUDE.md` (lista de ADRs + próximo número; marco fechado)

**Interfaces:**
- Consumes: tudo o que as Tasks 1–9 produziram.

- [ ] **Step 1: Wire the triagem in the composition root**

Em `internal/platform/app.go`, a seguir ao bloco do Check-in (a seguir a `handlerRecepcaoChegadas := ...`, antes de "Middlewares transversais"), acrescenta:

```go
	// BC Recepção — Triagem (prioridade Manchester, sinais vitais, fila clínica).
	repoTriagens := pgrepo.NovoRepositorioTriagens(pool)
	handlerRecepcaoTriagem := adhttp.NovoRecepcaoTriagemHandler(
		apprecepcao.NovoCasoRegistarTriagem(repoTriagens, repoChegadas, repoAuditoria),
		apprecepcao.NovoCasoObterTriagem(repoTriagens),
		apprecepcao.NovoCasoListarFilaClinica(repoTriagens),
	)
```

E em `registarRotas`, a seguir a `adhttp.RegistarRecepcaoChegadas(r, handlerRecepcaoChegadas, limiteMW, authMW)`:

```go
		adhttp.RegistarRecepcaoTriagem(r, handlerRecepcaoTriagem, limiteMW, authMW)
```

- [ ] **Step 2: Verify build and full unit suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS (unitários; integração fica SKIP sem `DATABASE_URL`).

- [ ] **Step 3: Verify the architectural linter**

Run: `go-arch-lint check`
Expected: `OK - No warnings found`.

- [ ] **Step 4: Write ADR-034**

```markdown
# ADR-034 — BC Recepção: Triagem e fila clínica

- **Estado:** Aceite
- **Data:** 2026-07-15
- **Marco/Sprint:** Percurso Ambulatório / sub-projecto Triagem (fecha o marco)
- **Fontes:** design em `docs/superpowers/specs/2026-07-15-recepcao-triagem-design.md`; ADR-033 (Check-in, precedente do mesmo BC).

## Contexto

O Check-in (ADR-033) coloca o doente na fila e permite chamá-lo (`Chegada` `CHAMADO`).
Faltava a avaliação clínica que classifica a prioridade e regista os sinais vitais, e que
ordena a fila a partir da qual o médico atende. Este sub-projecto entrega a Triagem e
fecha o marco Percurso Ambulatório.

## Decisão

1. **Novo agregado `Triagem`** no BC `recepcao`, 1:1 com a chegada, **imutável** após
   criação (sem máquina de estados) — é um registo clínico.
2. **Prioridade pelo Sistema de Manchester** (VO `PrioridadeManchester`: 5 cores com
   severidade e tempo-alvo). Os **sinais vitais** são um VO (`SinaisVitais`) de 9 campos
   opcionais com validação de intervalos plausíveis (limites de sanidade, não normais
   clínicos).
3. **Novo estado `TRIADO` na `Chegada`** (`RegistarTriada`, CHAMADO→TRIADO). É aqui que o
   **walk-in** recebe o médico atribuído (obrigatório); a chegada agendada herda o médico
   da marcação (não se re-atribui).
4. **Registo de triagem transaccional e cross-agregado:**
   `RepositorioTriagens.RegistarTriagem` transita a chegada para TRIADO (guarda
   compare-and-set sobre CHAMADO, com o médico) e insere a triagem na mesma transacção. O
   registo duplicado falha na guarda CAS (a chegada já não está CHAMADO) → Conflito, com
   defesa em profundidade pela restrição `UNIQUE (chegada_id)`.
5. **Fila clínica** — read-model das chegadas TRIADO com a sua triagem, ordenadas por
   severidade de Manchester (mais urgente primeiro) e depois por hora de chegada,
   filtráveis por médico.
6. **Leitura clínica restrita** (Médico/Enfermeiro/Director; sem Administrativo/Admin),
   porque os sinais vitais e a prioridade derivada são dado clínico (minimização LPDP,
   como no Laboratório). O registo é do Enfermeiro/Médico. Handler HTTP separado.

## Consequências

- O marco Percurso Ambulatório fica fechado: marcação → check-in → triagem → (consulta).
- O início da consulta (consumir a `Chegada` TRIADO → criar o `EpisodioClinico` no BC
  Clínico) e a ligação dos sinais vitais ao EHR ficam para um marco futuro de integração.
- A triagem é imutável neste marco (sem re-triagem/correcção).
```

- [ ] **Step 5: Update CLAUDE.md**

Na secção "Convenções-fonte" de `CLAUDE.md`, acrescenta a ADR-034 e actualiza o próximo número:

```
`adrs/ADR-033-bc-recepcao-checkin.md`,
`adrs/ADR-034-bc-recepcao-triagem.md`.
Próximo ADR: **ADR-035**.
```

Na secção "6. Marco Actual", actualiza a nota do Marco Percurso Ambulatório para o marcar
como **fechado**:

```
**Marco Percurso Ambulatório** (entregue, a par do M3): o percurso do doente antes da
consulta. Sub-projectos: **Marcação** (ADR-032), **Check-in** (ADR-033) e **Triagem** (BC
`recepcao` — prioridade Manchester, sinais vitais, fila clínica; ver ADR-034). O início da
consulta (Chegada TRIADO → Episódio no BC Clínico) fica para integração futura.
```

- [ ] **Step 6: Final build + integration suite + commit**

Run: `go build ./... && go test ./...`
Expected: PASS.

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags integration ./tests/integration/... -count=1`
Expected: verde (os testes de Recepção passam; skips só de Keycloak/MailHog).

```bash
git add internal/platform/app.go adrs/ADR-034-bc-recepcao-triagem.md CLAUDE.md
git commit -m "feat(recepcao): liga a Triagem ao composition root + ADR-034 (fecha o marco)"
```

---

## Self-Review (autor)

**1. Cobertura do spec:**
- §2/3.1 VO `PrioridadeManchester` → Task 1. ✓
- §3.2 VO `SinaisVitais` (9 campos, intervalos) → Task 2. ✓
- §3.3 agregado `Triagem` imutável + porta + read-model → Task 3. ✓
- §3.4 `TRIADO` + `RegistarTriada` (walk-in exige médico, agendada herda) → Task 4. ✓
- §3.5 coordenação transaccional (CAS + UNIQUE) → Tasks 6 (aplicação) e 8 (pgrepo). ✓
- §4.1 3 casos de uso + acção de auditoria → Tasks 5–6. ✓
- §4.2 `RepositorioTriagens`, DTOs → Tasks 3, 5. ✓
- §5.1 HTTP + RBAC (triagemEscrita/leituraClinica) → Task 9. ✓
- §5.2 migração (ALTER CHECK + triagens + intervalos + UNIQUE) → Task 7. ✓
- §6 erros/auditoria → categorias usadas; auditoria no registo. ✓
- §7 cobertura (domínio Tasks 1–4, aplicação Tasks 5–6, adaptadores Tasks 8–9). ✓
- §8 ADR-034 + fecho do marco → Task 10. ✓

**2. Placeholders:** nenhum "TBD"/"TODO"; todos os passos com código completo. As notas de
reutilização (querier, ehUnica, instMarcacao, fakeAuth, fakeChegadas) são instruções
contra o código real. A nota sobre `recarregadaComoChamada` (Task 8) dá ao implementador a
alternativa recomendada (duas leituras) para provar a CAS de forma inequívoca.

**3. Consistência de tipos:** `PrioridadeManchester`/consts, `SinaisVitais` (9 campos
idênticos entre VO, DTO e migração), `NovaTriagem`/`RegistarTriada`/`RegistarTriagem`,
`ResumoFilaClinica`, e as acções de auditoria (`recepcao.triagem.registada`) coerentes
entre domínio, aplicação, adaptadores e ADR. O padrão `comIDTriagem`/`ReconstruirTriagem`
é uniforme com o resto do BC.
