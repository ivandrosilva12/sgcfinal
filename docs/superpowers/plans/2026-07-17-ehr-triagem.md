# Sinais Vitais da Triagem no EHR (ADR-037) — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Médico/Enfermeiro/Director veem a triagem (prioridade Manchester + sinais vitais) no detalhe do episódio e a cor de Manchester nos resumos (EHR/listagem), lida por ACL sobre a ponte `recepcao.chegadas.episodio_id` — sem migrações, sem tocar na Recepção.

**Architecture:** Porta `LeitorTriagem` na aplicação do Clínico (DTOs próprios, sem importar o domínio Recepção); adaptador na peça de integração existente (`pgrepo.IntegracaoInicioConsulta`) com `JOIN chegadas⋈triagens`; filtragem por papel na projecção (minimização LPDP da ADR-034 — os papéis chegam como `[]string` do handler). Spec: `docs/superpowers/specs/2026-07-17-ehr-triagem-design.md`.

**Tech Stack:** Go 1.22+, Gin, pgx v5, PostgreSQL 16.

## Global Constraints

- **PT-PT angolano em TODA a saída** — código, comentários, commits, mensagens. Nunca PT-BR, nunca EN visível.
- A aplicação do Clínico **não importa** o domínio da Recepção nem o da Identidade — papéis como `[]string`, DTOs próprios da porta.
- Códigos reais dos papéis (verificados em `internal/domain/identidade/papel.go:9-16`): `"Medico"`, `"Enfermeiro"`, `"Director"` — literais exactos.
- Falha do leitor de triagem **propaga** (500); nunca degradar silenciosamente para "sem triagem".
- Papel não autorizado → o leitor **nem é invocado** (asserta-se nos testes).
- Zero migrações; zero alterações a ficheiros do BC Recepção.
- Gates: aplicação ≥75%, adapters ≥60% (pgrepo por integração); testes de integração `//go:build integration`, package `integration_test`, `ligar(t)` SKIPa sem `DATABASE_URL`. Postgres local: `DATABASE_URL="postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable"` (container `sgc-postgres-1`).
- Commits terminam com `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Módulo: `github.com/ivandrosilva12/sgcfinal`.

---

### Task 1: Porta ACL, DTOs e adaptador `LeitorTriagem` + teste de integração

**Files:**
- Modify: `internal/application/clinico/ports.go` (secção da integração, antes de `// --- Consentimento (LPDP) ---`)
- Modify: `internal/domain/clinico/repositorio_episodios.go` (campo passivo em `ResumoEpisodio`)
- Modify: `internal/application/clinico/ports.go` — `DetalheEpisodio` ganha `Triagem`
- Modify: `internal/adapters/pgrepo/integracao_inicio_consulta.go` (implementa a porta)
- Test: `tests/integration/ehr_triagem_test.go` (novo)

**Interfaces:**
- Consumes: `recepcao.chegadas.episodio_id` (migração 0004, ADR-036); helper de integração `criaChegadaTriadaComDoente(t, pool, ctx, bi, telefone)` de `tests/integration/inicio_consulta_test.go` (mesmo pacote — cria chegada TRIADO com prioridade `VERDE`, sinais vitais vazios, enfermeiro `enfInicioConsulta`, médico `medInicioConsulta`); `integ.ConsumirEIniciar` para criar o episódio ligado.
- Produces (contrato das Tasks 2–3):
  - `type SinaisVitaisDTO struct { TensaoSistolica, TensaoDiastolica, FrequenciaCardiaca *int; Temperatura *float64; FrequenciaRespiratoria, SaturacaoO2, Dor, Glicemia *int; Peso *float64 }` (tags json conforme abaixo)
  - `type TriagemDoEpisodio struct { Prioridade string; SinaisVitais SinaisVitaisDTO; Observacoes string; EnfermeiroID string; TriadaEm time.Time }`
  - `type LeitorTriagem interface { TriagemDoEpisodio(ctx, episodioID string) (TriagemDoEpisodio, bool, error); PrioridadesDosEpisodios(ctx, episodioIDs []string) (map[string]string, error) }`
  - `DetalheEpisodio.Triagem *TriagemDoEpisodio` · `ResumoEpisodio.PrioridadeTriagem string`
  - `pgrepo.IntegracaoInicioConsulta` implementa `LeitorTriagem`.

- [ ] **Step 1: Escrever o teste de integração que falha**

Criar `tests/integration/ehr_triagem_test.go`:

```go
//go:build integration

// Teste de integração do LeitorTriagem (triagem no EHR, ADR-037) contra a BD
// real: prova a junção recepcao.chegadas ⋈ recepcao.triagens por episodio_id.
// SKIPa (nunca FAIL) quando DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	domclinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestLeitorTriagem_TriagemDoEpisodio(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	doenteID, chegadaID := criaChegadaTriadaComDoente(t, pool, ctx, "00987659LA026", "+244923111777")
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)

	// episódio nascido da fila (ConsumirEIniciar liga chegada→episódio)
	ep, err := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	if err != nil {
		t.Fatalf("construir episódio: %v", err)
	}
	epID, err := integ.ConsumirEIniciar(ctx, chegadaID, medInicioConsulta, ep)
	if err != nil {
		t.Fatalf("consumir e iniciar: %v", err)
	}

	tr, ok, err := integ.TriagemDoEpisodio(ctx, epID)
	if err != nil || !ok {
		t.Fatalf("triagem do episódio: %v (ok=%v)", err, ok)
	}
	if tr.Prioridade != "VERDE" || tr.EnfermeiroID != enfInicioConsulta {
		t.Fatalf("triagem inesperada: %+v", tr)
	}
	if tr.SinaisVitais.Temperatura != nil {
		t.Fatalf("sinais vitais deviam estar vazios (não medidos): %+v", tr.SinaisVitais)
	}
	if tr.TriadaEm.IsZero() {
		t.Fatal("triadaEm em falta")
	}

	// episódio criado pelo endpoint antigo (sem chegada) → ok=false, sem erro
	epAntigo, err := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	if err != nil {
		t.Fatalf("construir episódio antigo: %v", err)
	}
	epAntigoID, err := pgrepo.NovoRepositorioEpisodios(pool).Guardar(ctx, epAntigo)
	if err != nil {
		t.Fatalf("guardar episódio antigo: %v", err)
	}
	if _, ok, err := integ.TriagemDoEpisodio(ctx, epAntigoID); err != nil || ok {
		t.Fatalf("episódio sem fila devia dar ok=false sem erro, veio ok=%v err=%v", ok, err)
	}

	// lote com mistura: só o episódio da fila aparece no mapa
	prioridades, err := integ.PrioridadesDosEpisodios(ctx, []string{epID, epAntigoID})
	if err != nil {
		t.Fatalf("prioridades em lote: %v", err)
	}
	if len(prioridades) != 1 || prioridades[epID] != "VERDE" {
		t.Fatalf("lote inesperado: %+v", prioridades)
	}

	// lote vazio → mapa vazio, sem erro (sem tocar na BD)
	vazio, err := integ.PrioridadesDosEpisodios(ctx, nil)
	if err != nil || len(vazio) != 0 {
		t.Fatalf("lote vazio devia dar mapa vazio: %v (%v)", vazio, err)
	}
}
```

- [ ] **Step 2: Confirmar que falha na compilação**

Run: `go vet -tags integration ./tests/integration/`
Expected: FAIL — `integ.TriagemDoEpisodio undefined` / `integ.PrioridadesDosEpisodios undefined`.

- [ ] **Step 3: Acrescentar DTOs e porta em `ports.go`**

Em `internal/application/clinico/ports.go`, dentro da secção `// --- Integração Recepção→Clínico: início da consulta (ADR-036) ---` (a seguir à interface `ConsumidorChegadas`):

```go
// SinaisVitaisDTO são os sinais vitais da triagem numa resposta clínica.
// Ponteiro nil = não medido (como no VO da Recepção, sem o importar) — ADR-037.
type SinaisVitaisDTO struct {
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

// TriagemDoEpisodio é o retrato da triagem que originou um episódio — DTO da
// porta anti-corrupção, sem tipos do domínio Recepção (ADR-037).
type TriagemDoEpisodio struct {
	Prioridade   string          `json:"prioridade"`
	SinaisVitais SinaisVitaisDTO `json:"sinais_vitais"`
	Observacoes  string          `json:"observacoes,omitempty"`
	EnfermeiroID string          `json:"enfermeiro_id"`
	TriadaEm     time.Time       `json:"triada_em"`
}

// LeitorTriagem é a porta anti-corrupção para leitura da triagem no BC
// Recepção (ADR-037). A junção faz-se pela ponte chegadas.episodio_id (ADR-036).
type LeitorTriagem interface {
	// TriagemDoEpisodio devolve a triagem que originou o episódio; ok=false se
	// o episódio não nasceu da fila clínica (sem chegada associada).
	TriagemDoEpisodio(ctx context.Context, episodioID string) (TriagemDoEpisodio, bool, error)
	// PrioridadesDosEpisodios devolve a cor de Manchester por episódio (lote,
	// para páginas de resumos); ids sem triagem ficam fora do mapa.
	PrioridadesDosEpisodios(ctx context.Context, episodioIDs []string) (map[string]string, error)
}
```

E em `DetalheEpisodio` (mesmo ficheiro), acrescentar o campo no fim do struct, a seguir a `FechadoPor`:

```go
	FechadoPor      string              `json:"fechado_por,omitempty"`
	Triagem         *TriagemDoEpisodio  `json:"triagem,omitempty"`
```

- [ ] **Step 4: Campo passivo no read-model do domínio**

Em `internal/domain/clinico/repositorio_episodios.go`, acrescentar a `ResumoEpisodio` (a seguir a `Estado`):

```go
	Estado          string     `json:"estado"`
	// PrioridadeTriagem é a cor de Manchester da triagem de origem (ADR-037).
	// Preenchida pela camada de aplicação via ACL — o repositório do Clínico
	// não conhece a Recepção; vazia quando o episódio não nasceu da fila.
	PrioridadeTriagem string `json:"prioridade_triagem,omitempty"`
```

- [ ] **Step 5: Implementar a porta no adaptador de integração**

Em `internal/adapters/pgrepo/integracao_inicio_consulta.go`, acrescentar no fim (antes das asserções de conformidade):

```go
// TriagemDoEpisodio devolve a triagem que originou o episódio, pela ponte
// chegadas.episodio_id (ADR-036/ADR-037). ok=false sem erro quando o episódio
// não nasceu da fila clínica.
func (a *IntegracaoInicioConsulta) TriagemDoEpisodio(ctx context.Context, episodioID string) (appclinico.TriagemDoEpisodio, bool, error) {
	const q = `
SELECT t.prioridade, t.tensao_sistolica, t.tensao_diastolica, t.frequencia_cardiaca,
       t.temperatura, t.frequencia_respiratoria, t.saturacao_o2, t.dor, t.glicemia,
       t.peso, COALESCE(t.observacoes,''), t.enfermeiro_id::text, t.triada_em
FROM recepcao.chegadas c
JOIN recepcao.triagens t ON t.chegada_id = c.id
WHERE c.episodio_id = $1`
	var tr appclinico.TriagemDoEpisodio
	sv := &tr.SinaisVitais
	err := a.pool.QueryRow(ctx, q, episodioID).Scan(&tr.Prioridade,
		&sv.TensaoSistolica, &sv.TensaoDiastolica, &sv.FrequenciaCardiaca, &sv.Temperatura,
		&sv.FrequenciaRespiratoria, &sv.SaturacaoO2, &sv.Dor, &sv.Glicemia, &sv.Peso,
		&tr.Observacoes, &tr.EnfermeiroID, &tr.TriadaEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appclinico.TriagemDoEpisodio{}, false, nil
		}
		return appclinico.TriagemDoEpisodio{}, false, fmt.Errorf("obter triagem do episódio: %w", err)
	}
	return tr, true, nil
}

// PrioridadesDosEpisodios devolve a cor de Manchester por episódio (lote).
func (a *IntegracaoInicioConsulta) PrioridadesDosEpisodios(ctx context.Context, episodioIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(episodioIDs) == 0 {
		return out, nil
	}
	const q = `
SELECT c.episodio_id::text, t.prioridade
FROM recepcao.chegadas c
JOIN recepcao.triagens t ON t.chegada_id = c.id
WHERE c.episodio_id = ANY($1::uuid[])`
	linhas, err := a.pool.Query(ctx, q, episodioIDs)
	if err != nil {
		return nil, fmt.Errorf("listar prioridades de triagem: %w", err)
	}
	defer linhas.Close()
	for linhas.Next() {
		var id, prioridade string
		if err := linhas.Scan(&id, &prioridade); err != nil {
			return nil, fmt.Errorf("ler prioridade de triagem: %w", err)
		}
		out[id] = prioridade
	}
	return out, linhas.Err()
}
```

E acrescentar à lista de garantias de conformidade no fim do ficheiro:

```go
	_ appclinico.LeitorTriagem      = (*IntegracaoInicioConsulta)(nil)
```

- [ ] **Step 6: Compilar, vet e correr a integração**

Run: `go build ./... && go vet -tags integration ./tests/integration/`
Expected: OK.

Run: `DATABASE_URL="postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable" go test -count=1 -tags integration ./tests/integration/ -run LeitorTriagem -v`
Expected: PASS. (Sem `DATABASE_URL`: SKIP.)

- [ ] **Step 7: Commit**

```bash
git add internal/application/clinico/ports.go internal/domain/clinico/repositorio_episodios.go internal/adapters/pgrepo/integracao_inicio_consulta.go tests/integration/ehr_triagem_test.go
git commit -m "feat(clinico): porta LeitorTriagem sobre a ponte episodio_id (ADR-037)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Casos de uso — filtragem por papel e enriquecimento

**Files:**
- Create: `internal/application/clinico/triagem_projeccao.go`
- Modify: `internal/application/clinico/obter_episodio.go`
- Modify: `internal/application/clinico/obter_ehr.go`
- Modify: `internal/application/clinico/listar_episodios.go`
- Test: `internal/application/clinico/triagem_projeccao_test.go` (novo)
- Modify: call sites nos testes existentes do pacote (`gerir_episodio_test.go`, `obter_listar_ehr_test.go`, `fakes_episodio_test.go` — actualização mecânica de assinaturas)

**Interfaces:**
- Consumes: `LeitorTriagem`, `TriagemDoEpisodio`, `DetalheEpisodio.Triagem`, `ResumoEpisodio.PrioridadeTriagem` (Task 1).
- Produces (contrato da Task 3):
  - `NovoCasoObterEpisodio(ep dominio.RepositorioEpisodios, triagem LeitorTriagem, aud Auditor)`; `Executar(ctx, actor string, papeis []string, id string) (DetalheEpisodio, error)`
  - `NovoCasoListarEpisodios(ep dominio.RepositorioEpisodios, triagem LeitorTriagem)`; `Executar(ctx, doenteID string, papeis []string, filtro FiltroEpisodios) (PaginaEpisodios, error)`
  - `NovoCasoObterEHR(doentes, ep, triagem LeitorTriagem, aud)`; `Executar(ctx, actor string, papeis []string, doenteID string, filtro) (EHR, error)`

- [ ] **Step 1: Escrever os testes que falham**

Criar `internal/application/clinico/triagem_projeccao_test.go`:

```go
package clinico_test

import (
	"context"
	"errors"
	"testing"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeLeitorTriagem serve triagens por episódio e conta invocações — permite
// assertar que papéis não autorizados nem sequer invocam a porta.
type fakeLeitorTriagem struct {
	porEpisodio map[string]appclinico.TriagemDoEpisodio
	err         error
	chamadas    int
}

func (f *fakeLeitorTriagem) TriagemDoEpisodio(_ context.Context, id string) (appclinico.TriagemDoEpisodio, bool, error) {
	f.chamadas++
	if f.err != nil {
		return appclinico.TriagemDoEpisodio{}, false, f.err
	}
	tr, ok := f.porEpisodio[id]
	return tr, ok, nil
}

func (f *fakeLeitorTriagem) PrioridadesDosEpisodios(_ context.Context, ids []string) (map[string]string, error) {
	f.chamadas++
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]string{}
	for _, id := range ids {
		if tr, ok := f.porEpisodio[id]; ok {
			out[id] = tr.Prioridade
		}
	}
	return out, nil
}

func episodioNoRepo(t *testing.T, repo *fakeRepoEpisodios, doenteID string) string {
	t.Helper()
	ep, err := clinico.NovoEpisodio(doenteID, clinico.EpisodioConsulta, "esp-1", "medico-1", instanteTeste())
	if err != nil {
		t.Fatalf("construir episódio: %v", err)
	}
	id, err := repo.Guardar(context.Background(), ep)
	if err != nil {
		t.Fatalf("guardar episódio: %v", err)
	}
	return id
}

func TestObterEpisodio_MedicoVeTriagem(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	epID := episodioNoRepo(t, repoEp, "doe-1")
	leitor := &fakeLeitorTriagem{porEpisodio: map[string]appclinico.TriagemDoEpisodio{
		epID: {Prioridade: "AMARELO", EnfermeiroID: "enf-1"},
	}}
	caso := appclinico.NovoCasoObterEpisodio(repoEp, leitor, &fakeAuditor{})

	out, err := caso.Executar(context.Background(), "medico-1", []string{"Medico"}, epID)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if out.Triagem == nil || out.Triagem.Prioridade != "AMARELO" {
		t.Fatalf("triagem devia vir preenchida: %+v", out.Triagem)
	}
}

func TestObterEpisodio_FarmaceuticoNaoVeTriagem_LeitorNaoInvocado(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	epID := episodioNoRepo(t, repoEp, "doe-1")
	leitor := &fakeLeitorTriagem{porEpisodio: map[string]appclinico.TriagemDoEpisodio{
		epID: {Prioridade: "AMARELO"},
	}}
	caso := appclinico.NovoCasoObterEpisodio(repoEp, leitor, &fakeAuditor{})

	out, err := caso.Executar(context.Background(), "farm-1", []string{"Farmaceutico"}, epID)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if out.Triagem != nil {
		t.Fatalf("farmacêutico não devia ver a triagem: %+v", out.Triagem)
	}
	if leitor.chamadas != 0 {
		t.Fatal("o leitor de triagem não devia ser invocado sem papel autorizado")
	}
}

func TestObterEpisodio_SemTriagem_OmitidoSemErro(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	epID := episodioNoRepo(t, repoEp, "doe-1")
	caso := appclinico.NovoCasoObterEpisodio(repoEp, &fakeLeitorTriagem{}, &fakeAuditor{})

	out, err := caso.Executar(context.Background(), "medico-1", []string{"Medico"}, epID)
	if err != nil || out.Triagem != nil {
		t.Fatalf("episódio sem triagem devia vir sem bloco e sem erro: %v (%+v)", err, out.Triagem)
	}
}

func TestObterEpisodio_FalhaDoLeitor_Propaga(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	epID := episodioNoRepo(t, repoEp, "doe-1")
	falha := erros.Novo(erros.CategoriaInterno, "recepção indisponível")
	caso := appclinico.NovoCasoObterEpisodio(repoEp, &fakeLeitorTriagem{err: falha}, &fakeAuditor{})

	if _, err := caso.Executar(context.Background(), "medico-1", []string{"Medico"}, epID); !errors.Is(err, falha) {
		t.Fatalf("a falha do leitor devia propagar, veio %v", err)
	}
}

func TestListarEpisodios_LotePreenchePrioridades(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	repoEp.pagina = clinico.PaginaEpisodios{Itens: []clinico.ResumoEpisodio{
		{ID: "ep-1"}, {ID: "ep-2"},
	}, Total: 2}
	leitor := &fakeLeitorTriagem{porEpisodio: map[string]appclinico.TriagemDoEpisodio{
		"ep-1": {Prioridade: "VERMELHO"},
	}}
	caso := appclinico.NovoCasoListarEpisodios(repoEp, leitor)

	pagina, err := caso.Executar(context.Background(), "doe-1", []string{"Enfermeiro"}, appclinico.FiltroEpisodios{})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if pagina.Itens[0].PrioridadeTriagem != "VERMELHO" || pagina.Itens[1].PrioridadeTriagem != "" {
		t.Fatalf("prioridades mal preenchidas: %+v", pagina.Itens)
	}
}

func TestListarEpisodios_SemPapel_LeitorNaoInvocado(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	repoEp.pagina = clinico.PaginaEpisodios{Itens: []clinico.ResumoEpisodio{{ID: "ep-1"}}, Total: 1}
	leitor := &fakeLeitorTriagem{porEpisodio: map[string]appclinico.TriagemDoEpisodio{
		"ep-1": {Prioridade: "VERMELHO"},
	}}
	caso := appclinico.NovoCasoListarEpisodios(repoEp, leitor)

	pagina, err := caso.Executar(context.Background(), "doe-1", []string{"TecnicoLab"}, appclinico.FiltroEpisodios{})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if pagina.Itens[0].PrioridadeTriagem != "" || leitor.chamadas != 0 {
		t.Fatalf("sem papel: prioridade devia ficar vazia e o leitor por invocar (%+v, chamadas=%d)", pagina.Itens, leitor.chamadas)
	}
}

func TestObterEHR_LotePreenchePrioridades(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	repoEp.pagina = clinico.PaginaEpisodios{Itens: []clinico.ResumoEpisodio{
		{ID: "ep-1"}, {ID: "ep-2"},
	}, Total: 2}
	leitor := &fakeLeitorTriagem{porEpisodio: map[string]appclinico.TriagemDoEpisodio{
		"ep-2": {Prioridade: "LARANJA"},
	}}
	caso := appclinico.NovoCasoObterEHR(repoDoentes, repoEp, leitor, &fakeAuditor{})

	ehr, err := caso.Executar(context.Background(), "medico-1", []string{"Director"}, doenteID, appclinico.FiltroEpisodios{})
	if err != nil {
		t.Fatalf("ehr: %v", err)
	}
	if ehr.Episodios.Itens[0].PrioridadeTriagem != "" || ehr.Episodios.Itens[1].PrioridadeTriagem != "LARANJA" {
		t.Fatalf("prioridades do EHR mal preenchidas: %+v", ehr.Episodios.Itens)
	}
}

func TestObterEHR_FalhaDoLote_Propaga(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	repoEp.pagina = clinico.PaginaEpisodios{Itens: []clinico.ResumoEpisodio{{ID: "ep-1"}}, Total: 1}
	falha := erros.Novo(erros.CategoriaInterno, "recepção indisponível")
	caso := appclinico.NovoCasoObterEHR(repoDoentes, repoEp, &fakeLeitorTriagem{err: falha}, &fakeAuditor{})

	if _, err := caso.Executar(context.Background(), "medico-1", []string{"Medico"}, doenteID, appclinico.FiltroEpisodios{}); !errors.Is(err, falha) {
		t.Fatalf("a falha do lote devia propagar, veio %v", err)
	}
}
```

Nota: se o helper `instanteTeste()` não existir no pacote de teste, usar `time.Now()` (import `time`) — verificar o idioma dos ficheiros vizinhos e seguir o que existir.

- [ ] **Step 2: Confirmar que falham**

Run: `go test ./internal/application/clinico/ -run "Triagem|MedicoVeTriagem|LotePreenche" -v`
Expected: FAIL — compilação (construtores com aridade antiga, `fakeRepoEpisodios.pagina` já existe mas `NovoCasoObterEpisodio` não aceita leitor).

- [ ] **Step 3: Criar `triagem_projeccao.go`**

```go
package clinico

import "context"

// papeisLeituraTriagem são os papéis que veem a triagem na projecção clínica
// (minimização LPDP, ADR-034/ADR-037): Médico, Enfermeiro e Director. Literais
// iguais aos códigos do BC Identidade — a Camada 2 do Clínico não o importa.
var papeisLeituraTriagem = map[string]bool{
	"Medico": true, "Enfermeiro": true, "Director": true,
}

// temPapelLeituraTriagem indica se algum papel do actor autoriza ver a triagem.
func temPapelLeituraTriagem(papeis []string) bool {
	for _, p := range papeis {
		if papeisLeituraTriagem[p] {
			return true
		}
	}
	return false
}

// preencherPrioridadesTriagem anota os resumos com a cor de Manchester da
// triagem de origem, quando o actor a pode ver. Ids sem triagem ficam vazios.
func preencherPrioridadesTriagem(ctx context.Context, leitor LeitorTriagem, papeis []string, itens []ResumoEpisodio) error {
	if len(itens) == 0 || !temPapelLeituraTriagem(papeis) {
		return nil
	}
	ids := make([]string, 0, len(itens))
	for _, it := range itens {
		ids = append(ids, it.ID)
	}
	prioridades, err := leitor.PrioridadesDosEpisodios(ctx, ids)
	if err != nil {
		return err
	}
	for i := range itens {
		itens[i].PrioridadeTriagem = prioridades[itens[i].ID]
	}
	return nil
}
```

- [ ] **Step 4: Actualizar os três casos de uso**

`obter_episodio.go` — struct ganha `triagem LeitorTriagem`; construtor `NovoCasoObterEpisodio(ep dominio.RepositorioEpisodios, triagem LeitorTriagem, aud Auditor)`; `Executar(ctx context.Context, actor string, papeis []string, id string)` e, depois da auditoria e antes do return:

```go
	det := paraDetalheEpisodio(episodio)
	if temPapelLeituraTriagem(papeis) {
		tr, ok, err := c.triagem.TriagemDoEpisodio(ctx, id)
		if err != nil {
			return DetalheEpisodio{}, err
		}
		if ok {
			det.Triagem = &tr
		}
	}
	return det, nil
```

`listar_episodios.go` — struct ganha `triagem LeitorTriagem`; construtor `NovoCasoListarEpisodios(ep dominio.RepositorioEpisodios, triagem LeitorTriagem)`; `Executar(ctx context.Context, doenteID string, papeis []string, filtro FiltroEpisodios)`; após obter a página:

```go
	pagina, err := c.episodios.ListarPorDoente(ctx, filtro)
	if err != nil {
		return PaginaEpisodios{}, err
	}
	if err := preencherPrioridadesTriagem(ctx, c.triagem, papeis, pagina.Itens); err != nil {
		return PaginaEpisodios{}, err
	}
	return pagina, nil
```

`obter_ehr.go` — struct ganha `triagem LeitorTriagem`; construtor `NovoCasoObterEHR(doentes dominio.RepositorioDoentes, ep dominio.RepositorioEpisodios, triagem LeitorTriagem, aud Auditor)`; `Executar(ctx context.Context, actor string, papeis []string, doenteID string, filtroEpisodios FiltroEpisodios)`; após obter a página (antes da auditoria):

```go
	if err := preencherPrioridadesTriagem(ctx, c.triagem, papeis, pagina.Itens); err != nil {
		return EHR{}, err
	}
```

- [ ] **Step 5: Actualizar mecanicamente os call sites dos testes existentes**

Todos os `NovoCasoObterEpisodio(...)`/`NovoCasoListarEpisodios(...)`/`NovoCasoObterEHR(...)` e respectivos `Executar` no pacote de teste ganham `&fakeLeitorTriagem{}` no construtor e `nil` (ou `[]string{"Medico"}` onde o teste já é clínico) como papéis — comportamento idêntico ao anterior (leitor vazio → sem triagem). Ficheiros afectados: `gerir_episodio_test.go`, `obter_listar_ehr_test.go` (incluindo os testes de dívida D1). Não alterar as asserções existentes.

- [ ] **Step 6: Correr e confirmar verde**

Run: `go test ./internal/application/clinico/ -count=1` e depois `go test -cover ./internal/application/clinico/`
Expected: PASS; cobertura ≥ a anterior (~94%) — regista o valor.

- [ ] **Step 7: Commit**

```bash
git add internal/application/clinico/
git commit -m "feat(clinico): triagem na projecção clínica com filtragem por papel (ADR-037)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: HTTP (papéis da sessão) + composition root + verificação global

**Files:**
- Modify: `internal/adapters/http/episodio_handler.go` (interfaces + 3 handlers + helper `papeisDe`)
- Modify: `internal/adapters/http/episodio_test.go` (fakes com assinaturas novas + 2 testes novos)
- Modify: `internal/platform/app.go` (mover `integracaoConsulta` para cima + injectar nos 3 casos)

**Interfaces:**
- Consumes: assinaturas da Task 2; `SessaoDe(c)` devolve `dominio.Sessao` com `Papeis []dominio.Papel` (strings tipadas: `"Medico"`, `"Enfermeiro"`, `"Director"`, ...).
- Produces: rotas inalteradas; casos de uso recebem os papéis reais da sessão.

- [ ] **Step 1: Escrever/actualizar os testes que falham**

Em `internal/adapters/http/episodio_test.go`:

1a. Actualizar os fakes às novas assinaturas, gravando os papéis recebidos:

```go
type fakeObterEpisodio struct {
	out    appclinico.DetalheEpisodio
	err    error
	papeis []string
}

func (f *fakeObterEpisodio) Executar(_ context.Context, _ string, papeis []string, _ string) (appclinico.DetalheEpisodio, error) {
	f.papeis = papeis
	return f.out, f.err
}

type fakeListarEpisodios struct {
	out    appclinico.PaginaEpisodios
	err    error
	papeis []string
}

func (f *fakeListarEpisodios) Executar(_ context.Context, _ string, papeis []string, _ appclinico.FiltroEpisodios) (appclinico.PaginaEpisodios, error) {
	f.papeis = papeis
	return f.out, f.err
}

type fakeObterEHR struct {
	out    appclinico.EHR
	err    error
	papeis []string
}

func (f *fakeObterEHR) Executar(_ context.Context, _ string, papeis []string, _ string, _ appclinico.FiltroEpisodios) (appclinico.EHR, error) {
	f.papeis = papeis
	return f.out, f.err
}
```

(os fakes passam a ponteiros — ajustar `routerEpisodios` para os construir com `&` e, onde os testes novos precisarem de inspeccionar, aceitar os fakes como parâmetros ou construí-los antes do router; segue o padrão de `clinico_consulta_test.go` que já injecta o fake por ponteiro.)

1b. Dois testes novos:

```go
func TestEpisodios_Obter_PassaPapeisDaSessao(t *testing.T) {
	f := &fakeObterEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1"}}
	r := routerEpisodiosComFakes(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, f)
	w := pedido(r, "GET", "/api/v1/episodios/ep-1", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, veio %d", w.Code)
	}
	if len(f.papeis) != 1 || f.papeis[0] != "Medico" {
		t.Fatalf("papéis da sessão mal passados: %v", f.papeis)
	}
}

func TestEpisodios_EHR_PassaPapeisDaSessao(t *testing.T) {
	f := &fakeObterEHR{out: appclinico.EHR{}}
	r := routerEpisodiosComFakesEHR(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, f)
	w := pedido(r, "GET", "/api/v1/doentes/d1/ehr", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, veio %d", w.Code)
	}
	if len(f.papeis) != 1 || f.papeis[0] != "Farmaceutico" {
		t.Fatalf("papéis da sessão mal passados: %v", f.papeis)
	}
}
```

(implementar `routerEpisodiosComFakes`/`routerEpisodiosComFakesEHR` como variantes de `routerEpisodios` que aceitam o fake a inspeccionar; ou refactorizar `routerEpisodios` para aceitar os 7 fakes — escolher o que mantiver os testes existentes com menos churn.)

- [ ] **Step 2: Confirmar que falham na compilação**

Run: `go test ./internal/adapters/http/ -run Episodios -v`
Expected: FAIL — assinaturas das interfaces `ServicoObterEpisodio`/`ServicoListarEpisodios`/`ServicoObterEHR` ainda antigas.

- [ ] **Step 3: Actualizar o handler**

Em `internal/adapters/http/episodio_handler.go`:

3a. Interfaces:

```go
	// ServicoObterEpisodio devolve o detalhe de um episódio.
	ServicoObterEpisodio interface {
		Executar(ctx context.Context, actor string, papeis []string, id string) (appclinico.DetalheEpisodio, error)
	}
	// ServicoListarEpisodios lista os episódios de um doente.
	ServicoListarEpisodios interface {
		Executar(ctx context.Context, doenteID string, papeis []string, filtro appclinico.FiltroEpisodios) (appclinico.PaginaEpisodios, error)
	}
	// ServicoObterEHR devolve a projecção EHR de um doente.
	ServicoObterEHR interface {
		Executar(ctx context.Context, actor string, papeis []string, doenteID string, filtro appclinico.FiltroEpisodios) (appclinico.EHR, error)
	}
```

3b. Helper (no mesmo ficheiro, junto aos handlers):

```go
// papeisDe converte os papéis da sessão para os literais esperados pela
// aplicação (a Camada 2 não importa o domínio Identidade — ADR-037).
func papeisDe(s dominio.Sessao) []string {
	out := make([]string, 0, len(s.Papeis))
	for _, p := range s.Papeis {
		out = append(out, string(p))
	}
	return out
}
```

3c. Handlers:

```go
func (h *EpisodiosHandler) obterEpisodio(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.obter.Executar(c.Request.Context(), actor.Sujeito, papeisDe(actor), c.Param("eid"))
	...
}

func (h *EpisodiosHandler) listarEpisodios(c *gin.Context) {
	actor, _ := SessaoDe(c)
	filtro := ...  // inalterado
	out, err := h.listar.Executar(c.Request.Context(), c.Param("id"), papeisDe(actor), filtro)
	...
}

func (h *EpisodiosHandler) obterEHR(c *gin.Context) {
	actor, _ := SessaoDe(c)
	filtro := ...  // inalterado
	out, err := h.ehr.Executar(c.Request.Context(), actor.Sujeito, papeisDe(actor), c.Param("id"), filtro)
	...
}
```

- [ ] **Step 4: Composition root**

Em `internal/platform/app.go`:

4a. **Mover** a linha `integracaoConsulta := pgrepo.NovaIntegracaoInicioConsulta(pool)` (e o seu comentário) do bloco da integração (após o `handlerRecepcaoTriagem`) para **imediatamente a seguir a** `repoEpisodios := pgrepo.NovoRepositorioEpisodios(pool)` — o leitor passa a ser preciso pelos casos de uso dos episódios, construídos antes. O bloco original da ADR-036 mantém `handlerClinicoConsulta := ...` usando a variável já construída (remover só a linha duplicada da construção).

4b. Injectar nos três casos:

```go
	handlerEpisodios := adhttp.NovoEpisodiosHandler(
		appclinico.NovoCasoIniciarEpisodio(repoEpisodios, repoDoentes, repoAuditoria),
		appclinico.NovoCasoObterEpisodio(repoEpisodios, integracaoConsulta, repoAuditoria),
		appclinico.NovoCasoListarEpisodios(repoEpisodios, integracaoConsulta),
		appclinico.NovoCasoActualizarEpisodio(repoEpisodios, repoAuditoria),
		appclinico.NovoCasoFecharEpisodio(repoEpisodios, repoAuditoria),
		appclinico.NovoCasoCancelarEpisodio(repoEpisodios, repoAuditoria),
		appclinico.NovoCasoObterEHR(repoDoentes, repoEpisodios, integracaoConsulta, repoAuditoria),
	)
```

- [ ] **Step 5: Verificação global**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: tudo verde.

Run: `golangci-lint run ./...`
Expected: 0 issues.

Run (com Postgres): `DATABASE_URL="postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable" go test -count=1 -tags integration ./tests/integration/`
Expected: PASS completo.

Run: `go test -cover ./internal/application/clinico/ ./internal/adapters/http/`
Expected: application/clinico ≥75% (partimos de 94%), http ≥60%.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/http/episodio_handler.go internal/adapters/http/episodio_test.go internal/platform/app.go
git commit -m "feat(http): papéis da sessão na leitura de episódios/EHR e ligação do LeitorTriagem (ADR-037)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: ADR-037, SPRINT.md e CLAUDE.md

**Files:**
- Create: `adrs/ADR-037-ehr-triagem.md`
- Modify: `SPRINT.md`
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: nada — documenta as Tasks 1–3.
- Produces: decisão registada; docs mestres actualizados.

- [ ] **Step 1: Redigir a ADR-037**

Criar `adrs/ADR-037-ehr-triagem.md`:

```markdown
# ADR-037 — EHR: triagem por leitura ACL com filtragem por papel

- **Estado:** Aceite
- **Data:** 2026-07-17
- **Marco/Sprint:** Integração Percurso Ambulatório → Clínico, 2.ª fatia (fecha o
  último diferimento da ADR-034)
- **Fontes:** design em `docs/superpowers/specs/2026-07-17-ehr-triagem-design.md`;
  ADR-034 (Triagem), ADR-036 (início da consulta — a ponte `episodio_id`).

## Contexto

A triagem regista a prioridade de Manchester e os sinais vitais (ADR-034), e o início
da consulta liga a chegada ao episódio (`recepcao.chegadas.episodio_id`, ADR-036) —
mas o médico que abria o EHR não via nada disso. Faltava expor a triagem na projecção
clínica sem violar a minimização LPDP da ADR-034 (leitura da triagem restrita a
Médico/Enfermeiro/Director, enquanto o EHR é legível por mais papéis).

## Decisão

1. **Leitura via ACL, não snapshot.** Porta `LeitorTriagem` na aplicação do Clínico
   (DTOs próprios — `TriagemDoEpisodio`, `SinaisVitaisDTO`); o adaptador é a peça de
   integração existente (`pgrepo.IntegracaoInicioConsulta`), com
   `JOIN recepcao.chegadas ⋈ recepcao.triagens` por `episodio_id`. Fonte única de
   verdade na Recepção; zero migrações; zero alterações ao BC Recepção. Rejeitado:
   snapshot na criação do episódio (duplicaria dado clínico, exigiria migração e
   alargaria a transacção da ADR-036).
2. **Filtragem por papel na projecção.** A triagem só entra na resposta quando o actor
   tem papel Médico/Enfermeiro/Director (literais do BC Identidade, guardados como
   `[]string` — a Camada 2 do Clínico não importa Identidade). Sem papel, a resposta é
   a de sempre e a porta nem é invocada.
3. **Superfície:** `GET /episodios/:eid` ganha o bloco `triagem` completo (prioridade,
   9 sinais vitais, observações, enfermeiro, instante); os resumos de episódio (EHR e
   listagem) ganham só `prioridade_triagem` (leitura em lote). Episódios que não
   nasceram da fila ficam como hoje (sem bloco, sem erro).
4. **Falha franca:** erro do leitor propaga (500) — nunca degradar silenciosamente
   para "sem triagem".

## Consequências

- O percurso ambulatório fica clinicamente visível de ponta a ponta: a triagem que
  originou a consulta acompanha o episódio no EHR.
- O campo `ResumoEpisodio.PrioridadeTriagem` é preenchido pela aplicação (ACL), nunca
  pelo repositório do Clínico — a fronteira BC mantém-se.
- Correcção futura da triagem (hoje imutável) propagaria automaticamente ao EHR.
- Diferimentos: tendências de sinais vitais; sinais medidos durante a consulta;
  eventos via Outbox (ADR-038).
```

- [ ] **Step 2: Actualizar o SPRINT.md**

Inserir imediatamente a seguir à secção `## Critérios de saída — Integração Início da Consulta (ADR-036)`:

```markdown
## Critérios de saída — Triagem no EHR (ADR-037)

- [x] Médico/Enfermeiro/Director veem no detalhe do episódio o bloco triagem
      (prioridade, sinais vitais, observações, enfermeiro, instante) quando o
      episódio nasceu da fila clínica.
- [x] Os resumos de episódio (EHR e listagem) mostram a cor de Manchester a esses
      papéis (leitura em lote).
- [x] Farmacêutico/Técnico de Lab/DPO/Auditor recebem as respostas de sempre — e o
      leitor de triagem nem é invocado (minimização LPDP, ADR-034).
- [x] Episódios sem chegada associada ficam exactamente como antes (sem bloco, sem
      erro); falha do leitor propaga (nunca degrada em silêncio).
- [x] Zero migrações; zero alterações ao BC Recepção; fonte única de verdade.
- [x] Cobertura nos limiares; integração prova a junção real por episodio_id.
```

- [ ] **Step 3: Actualizar o CLAUDE.md**

3a. Na secção `## 6. Marco Actual`, substituir:

```
consulta (Chegada TRIADO → Episódio no BC Clínico) foi entregue como **Integração
Recepção→Clínico** (ADR-036): transacção única no adaptador de integração, estado
EM_CONSULTA, só o médico atribuído.
```

por:

```
consulta (Chegada TRIADO → Episódio no BC Clínico) foi entregue como **Integração
Recepção→Clínico** (ADR-036): transacção única no adaptador de integração, estado
EM_CONSULTA, só o médico atribuído. A triagem ficou visível no EHR (ADR-037):
leitura ACL pela ponte episodio_id, filtrada por papel (minimização LPDP).
```

3b. Na lista de ADRs, mudar o final de `adrs/ADR-036-integracao-inicio-consulta.md`.
para `adrs/ADR-036-integracao-inicio-consulta.md`, (vírgula) e acrescentar
`adrs/ADR-037-ehr-triagem.md`. (com ponto final).

3c. Substituir `Próximo ADR: **ADR-037**.` por `Próximo ADR: **ADR-038**.`

- [ ] **Step 4: Commit**

```bash
git add adrs/ADR-037-ehr-triagem.md SPRINT.md CLAUDE.md
git commit -m "docs(clinico): ADR-037 — triagem no EHR por leitura ACL filtrada por papel

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Verificação final (após todas as tasks)

1. `go build ./... && go vet ./... && go test ./...` — tudo verde.
2. `golangci-lint run ./...` — 0 issues.
3. Com Postgres: `go test -count=1 -tags integration ./tests/integration/` — tudo verde.
4. Suites da Recepção intactas (nenhum ficheiro do BC Recepção alterado): `git diff --stat main` não toca em `internal/domain/recepcao/`, `internal/application/recepcao/` nem `migrations/recepcao/`.
