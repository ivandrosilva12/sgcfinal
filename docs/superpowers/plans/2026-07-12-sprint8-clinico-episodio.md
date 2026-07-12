# Sprint 8 — BC Clínico: agregado Episódio Clínico (+ EHR) — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Acrescentar o agregado Episódio Clínico ao BC Clínico — ciclo iniciar → actualizar nota → fechar → cancelar, listagem por doente e projecção de leitura EHR — do domínio ao HTTP.

**Architecture:** DDD + Clean Architecture. Novo agregado raiz `EpisodioClinico` no pacote `clinico` já existente (domínio/aplicação/adapters), sobre o schema `clinico`. Referencia `doente_id` (FK sem cascade). IDs `string` gerados pela BD. Reutiliza o `RepositorioDoentes`, o `Auditor` e a infra HTTP (Auth/RBAC/LimiteTaxa, RFC 7807) do Sprint 7.

**Tech Stack:** Go 1.22+, Gin, pgx v5 (SQL puro), PostgreSQL 16; testes `go test` com fakes; integração com tag `integration`.

## Global Constraints

- **Linguagem ubíqua PT-PT angolano** em TODO o output: código, comentários, mensagens de erro, JSON, commits. Nunca inglês nem PT-BR.
- **Domínio puro:** `internal/domain/**` só stdlib + Shared Kernel. Zero `pgx`/`gin`/`net/http`/`google/uuid`. `google/uuid` também proibido na aplicação.
- **Sem `panic()`** fora de init.
- **Erros de domínio** via `erros.Novo(categoria, mensagem)` com mensagem PT-PT **literal** (padrão do Sprint 7). Categorias usadas: `CategoriaValidacao`, `CategoriaNaoEncontrado`, `CategoriaConflito`.
- **Erros HTTP** via `responderErro(c, err)` (RFC 7807, PT-PT) — já existe em `internal/adapters/http/problem.go`.
- **Modelo de dados** extraído verbatim do DDM-001 v2.0 (ver Task 3). Não inventar colunas.
- **IDs** do domínio são `string`, gerados pela BD (`gen_random_uuid()` DEFAULT + `RETURNING id::text`).
- **Auditoria:** toda a escrita e a consulta individual (episódio, EHR) regista um `auditoria.Registo`. Acções: `clinico.episodio.aberto`, `clinico.episodio.actualizado`, `clinico.episodio.fechado`, `clinico.episodio.cancelado`, `clinico.episodio.consultado`, `clinico.ehr.consultado`. A listagem **não** audita.
- **Nunca registar em log** dados de saúde/identificadores.
- **Cobertura** (`bash scripts/cobertura.sh`, agregado por camada): domínio ≥85%, aplicação ≥75%, adaptadores ≥60%. O gate corre **sem** a tag `integration`.
- **Commits** Conventional Commits em PT-PT, a terminar com:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- **Branch de trabalho:** `m2-sprint8-clinico-episodio` (já criado; a spec já lá está commitada).

### Convenções do BC Clínico (do Sprint 7 — seguir tal como estão)

- Agregado com campos privados + factory validante + `Snapshot()`/`ReconstruirX(SnapshotX)` (ver `internal/domain/clinico/doente.go`).
- Enums `type X string` + consts + `ParseX(string) (X, error)` (ver `grupo_sanguineo.go`).
- Read-models (`ResumoX`/`PaginaX`/`FiltroX`) na interface do repositório, no domínio, com tags JSON nos read-models (ver `internal/domain/clinico/repositorio.go`).
- Casos de uso: struct com dependências por construtor `NovoCaso...`, relógio `agora func() time.Time` (default `time.Now`), método `Executar(ctx, ...)`; auditar **só após** persistir com sucesso; re-ler via `ObterPorID` para devolver o detalhe (ver `internal/application/clinico/registar_doente.go`, `gerir_estado_doente.go`).
- Repositório pgx: `pool *pgxpool.Pool`, transacção com `defer tx.Rollback`, `errors.Is(err, pgx.ErrNoRows)` → `CategoriaNaoEncontrado`, filhos por delete-and-reinsert, `NULLIF(...,'')` para texto opcional (ver `internal/adapters/pgrepo/doentes_repo.go`). O helper `deref(*string) string` já existe no pacote `pgrepo` — reutiliza-o.
- Handler: struct + interfaces de serviço + `RegistarX(r gin.IRouter, h, protecao ...gin.HandlerFunc)`, RBAC por rota via `RBAC(dominio.PapelX, ...)` (`dominio` = `internal/domain/identidade`), `actor` via `SessaoDe(c).Sujeito`, erros via `responderErro`, bind inválido → `erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido))` (ver `internal/adapters/http/doente_handler.go`).
- Testes de aplicação em `package clinico_test`; já existem os fakes `fakeRepo` (doentes), `fakeAuditor`, e os helpers `novoFakeRepo()`, `registarNoRepo(t, repo)`, `dadosBase()`, `ptrS`, `ptr` — **reutiliza-os, não os redefinas**.
- Testes de handler em `package http_test`; já existem `novoRouter()`, `pedido(...)`, `pedidoCorpo(...)`, `fakeAuth{sessao, err}` — reutiliza-os.

---

### Task 1: Enums e Value Objects do Episódio

**Files:**
- Create: `internal/domain/clinico/episodio_enums.go`
- Create: `internal/domain/clinico/nota_clinica.go`
- Create: `internal/domain/clinico/diagnostico_cid.go`
- Test: `internal/domain/clinico/episodio_enums_test.go`
- Test: `internal/domain/clinico/nota_clinica_test.go`
- Test: `internal/domain/clinico/diagnostico_cid_test.go`

**Interfaces:**
- Consumes: `erros.Novo`.
- Produces:
  - `type TipoEpisodio string`; consts `EpisodioConsulta="CONSULTA"`, `EpisodioUrgencia="URGENCIA"`, `EpisodioInternamento="INTERNAMENTO"`; `ParseTipoEpisodio(string) (TipoEpisodio, error)`.
  - `type EstadoEpisodio string`; consts `EstadoEpisodioAberto="ABERTO"`, `EstadoEpisodioFechado="FECHADO"`, `EstadoEpisodioCancelado="CANCELADO"`.
  - `type NotaClinica struct { QueixaPrincipal, HistoriaDoenca, ExameObjectivo, Diagnostico, Plano string }`; `NovaNotaClinica(queixa, historia, exame, diagnostico, plano string) NotaClinica` (apara espaços); método `Completa() bool` (queixa+exame+diagnóstico+plano não-vazios).
  - `type DiagnosticoCID struct { CID string; Principal bool }`; `NovoDiagnosticoCID(cid string, principal bool) (DiagnosticoCID, error)` (CID não-vazio).

- [ ] **Step 1: Escrever os testes que falham**

`episodio_enums_test.go`:
```go
package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func TestParseTipoEpisodio(t *testing.T) {
	if tp, err := clinico.ParseTipoEpisodio("consulta"); err != nil || tp != clinico.EpisodioConsulta {
		t.Fatalf("ParseTipoEpisodio(consulta)=%v,%v", tp, err)
	}
	if _, err := clinico.ParseTipoEpisodio("CIRURGIA"); err == nil {
		t.Fatal("esperava erro para tipo inválido")
	}
}
```

`nota_clinica_test.go`:
```go
package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func TestNotaClinica_Completa(t *testing.T) {
	completa := clinico.NovaNotaClinica("Febre", "", "Temp 39", "Gripe", "Repouso")
	if !completa.Completa() {
		t.Fatal("esperava nota completa (queixa+exame+diagnóstico+plano)")
	}
	semExame := clinico.NovaNotaClinica("Febre", "", "", "Gripe", "Repouso")
	if semExame.Completa() {
		t.Fatal("nota sem exame não devia ser completa")
	}
}

func TestNovaNotaClinica_AparaEspacos(t *testing.T) {
	n := clinico.NovaNotaClinica("  Febre  ", " ", " Temp ", " Gripe ", " Repouso ")
	if n.QueixaPrincipal != "Febre" || n.ExameObjectivo != "Temp" {
		t.Fatalf("não aparou espaços: %+v", n)
	}
}
```

`diagnostico_cid_test.go`:
```go
package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovoDiagnosticoCID(t *testing.T) {
	d, err := clinico.NovoDiagnosticoCID("J11", true)
	if err != nil || d.CID != "J11" || !d.Principal {
		t.Fatalf("diagnóstico inesperado: %+v, %v", d, err)
	}
}

func TestNovoDiagnosticoCID_Vazio(t *testing.T) {
	if _, err := clinico.NovoDiagnosticoCID("  ", false); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falham**

Run: `go test ./internal/domain/clinico/ -run 'TipoEpisodio|NotaClinica|DiagnosticoCID' -v`
Expected: FAIL — símbolos indefinidos.

- [ ] **Step 3: Implementar `episodio_enums.go`**

```go
package clinico

import (
	"strings"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// TipoEpisodio classifica um episódio clínico (DDM-001).
type TipoEpisodio string

const (
	EpisodioConsulta     TipoEpisodio = "CONSULTA"
	EpisodioUrgencia     TipoEpisodio = "URGENCIA"
	EpisodioInternamento TipoEpisodio = "INTERNAMENTO"
)

var tiposEpisodioValidos = map[TipoEpisodio]bool{
	EpisodioConsulta: true, EpisodioUrgencia: true, EpisodioInternamento: true,
}

// ParseTipoEpisodio valida e normaliza um tipo de episódio (aceita minúsculas).
func ParseTipoEpisodio(codigo string) (TipoEpisodio, error) {
	t := TipoEpisodio(strings.ToUpper(strings.TrimSpace(codigo)))
	if !tiposEpisodioValidos[t] {
		return "", erros.Novo(erros.CategoriaValidacao, "tipo de episódio inválido (esperado CONSULTA, URGENCIA ou INTERNAMENTO)")
	}
	return t, nil
}

// EstadoEpisodio é o estado do ciclo de vida de um episódio (DDM-001).
type EstadoEpisodio string

const (
	EstadoEpisodioAberto    EstadoEpisodio = "ABERTO"
	EstadoEpisodioFechado   EstadoEpisodio = "FECHADO"
	EstadoEpisodioCancelado EstadoEpisodio = "CANCELADO"
)
```

- [ ] **Step 4: Implementar `nota_clinica.go`**

```go
package clinico

import "strings"

// NotaClinica é o Value Object da nota clínica estruturada de um episódio
// (queixa, história, exame, diagnóstico, plano). Pode estar incompleta enquanto
// o episódio está ABERTO; o fecho exige-a completa.
type NotaClinica struct {
	QueixaPrincipal string
	HistoriaDoenca  string
	ExameObjectivo  string
	Diagnostico     string
	Plano           string
}

// NovaNotaClinica constrói a nota, aparando espaços. Não impõe obrigatoriedade.
func NovaNotaClinica(queixa, historia, exame, diagnostico, plano string) NotaClinica {
	return NotaClinica{
		QueixaPrincipal: strings.TrimSpace(queixa),
		HistoriaDoenca:  strings.TrimSpace(historia),
		ExameObjectivo:  strings.TrimSpace(exame),
		Diagnostico:     strings.TrimSpace(diagnostico),
		Plano:           strings.TrimSpace(plano),
	}
}

// Completa indica se a nota tem os campos obrigatórios para fechar um episódio:
// queixa principal, exame objectivo, diagnóstico e plano (história é opcional).
func (n NotaClinica) Completa() bool {
	return n.QueixaPrincipal != "" && n.ExameObjectivo != "" && n.Diagnostico != "" && n.Plano != ""
}
```

- [ ] **Step 5: Implementar `diagnostico_cid.go`**

```go
package clinico

import (
	"strings"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// DiagnosticoCID é um código de diagnóstico CID-10 associado a um episódio. Um
// episódio pode ter vários; no máximo um é o principal.
type DiagnosticoCID struct {
	CID       string
	Principal bool
}

// NovoDiagnosticoCID valida e constrói um DiagnosticoCID (CID não-vazio).
func NovoDiagnosticoCID(cid string, principal bool) (DiagnosticoCID, error) {
	cid = strings.TrimSpace(cid)
	if cid == "" {
		return DiagnosticoCID{}, erros.Novo(erros.CategoriaValidacao, "código CID em falta")
	}
	return DiagnosticoCID{CID: cid, Principal: principal}, nil
}
```

- [ ] **Step 6: Correr os testes e confirmar que passam**

Run: `go test ./internal/domain/clinico/ -v`
Expected: PASS (todos, incluindo os do Sprint 7). `gofmt -l internal/domain/clinico/` vazio.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/clinico/episodio_enums.go internal/domain/clinico/nota_clinica.go internal/domain/clinico/diagnostico_cid.go internal/domain/clinico/episodio_enums_test.go internal/domain/clinico/nota_clinica_test.go internal/domain/clinico/diagnostico_cid_test.go
git commit -m "feat(clinico): enums e value objects do episódio clínico

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Agregado Episódio Clínico, eventos e porta de repositório

**Files:**
- Create: `internal/domain/clinico/episodio.go`
- Create: `internal/domain/clinico/episodio_eventos.go`
- Create: `internal/domain/clinico/repositorio_episodios.go`
- Test: `internal/domain/clinico/episodio_test.go`

**Interfaces:**
- Consumes: `TipoEpisodio`/`ParseTipoEpisodio`, `EstadoEpisodio` + consts, `NotaClinica`, `DiagnosticoCID`/`NovoDiagnosticoCID` (Task 1); `erros`; `evento.EventoDominio`.
- Produces:
  - `type EpisodioClinico struct {...}` (campos privados) + getters `ID()`, `DoenteID()`, `Estado()`.
  - `type SnapshotEpisodio struct {...}` (todos os campos exportados).
  - `NovoEpisodio(doenteID string, tipo TipoEpisodio, especialidadeID, medicoID string, inicio time.Time) (*EpisodioClinico, error)`.
  - `ReconstruirEpisodio(s SnapshotEpisodio) *EpisodioClinico`.
  - Método `(*EpisodioClinico) Snapshot() SnapshotEpisodio`.
  - Métodos: `ActualizarNota(NotaClinica) error`, `DefinirDiagnosticosCID([]DiagnosticoCID) error`, `Fechar(fechadoPor string, em time.Time) error`, `Cancelar(em time.Time) error`.
  - `type RepositorioEpisodios interface {...}`; tipos `FiltroEpisodios`, `ResumoEpisodio`, `PaginaEpisodios`.
  - Eventos `EpisodioAberto`, `EpisodioFechado`, `EpisodioCancelado`.

**Regras:** nota e diagnósticos só editáveis em ABERTO. `Fechar` só de ABERTO, exige `nota.Completa()` **e** ≥1 diagnóstico CID e `fechadoPor` não-vazio → FECHADO + `fim`/`fechadoEm`/`fechadoPor`. `Cancelar` só de ABERTO → CANCELADO + `fim`. `DefinirDiagnosticosCID` valida cada CID e no máximo um `Principal`.

- [ ] **Step 1: Escrever o teste que falha (`episodio_test.go`)**

```go
package clinico_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func episodioAberto(t *testing.T) *clinico.EpisodioClinico {
	t.Helper()
	e, err := clinico.NovoEpisodio("doente-1", clinico.EpisodioConsulta, "esp-1", "medico-1", time.Now())
	if err != nil {
		t.Fatalf("NovoEpisodio: %v", err)
	}
	return e
}

func TestNovoEpisodio_EstadoInicialAberto(t *testing.T) {
	e := episodioAberto(t)
	if e.Estado() != clinico.EstadoEpisodioAberto {
		t.Fatalf("estado inicial=%q, esperava ABERTO", e.Estado())
	}
	if e.DoenteID() != "doente-1" {
		t.Fatalf("doente=%q", e.DoenteID())
	}
}

func TestNovoEpisodio_CamposObrigatorios(t *testing.T) {
	if _, err := clinico.NovoEpisodio("", clinico.EpisodioConsulta, "esp-1", "medico-1", time.Now()); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para doente vazio")
	}
	if _, err := clinico.NovoEpisodio("d1", clinico.EpisodioConsulta, "esp-1", "medico-1", time.Time{}); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para início zero")
	}
}

func TestFechar_ExigeNotaCompletaEDiagnostico(t *testing.T) {
	e := episodioAberto(t)
	// Sem nota completa → erro de validação.
	if erros.CategoriaDe(e.Fechar("medico-1", time.Now())) != erros.CategoriaValidacao {
		t.Fatal("esperava validação: nota incompleta")
	}
	_ = e.ActualizarNota(clinico.NovaNotaClinica("Febre", "", "Temp 39", "Gripe", "Repouso"))
	// Nota completa mas sem CID → erro de validação.
	if erros.CategoriaDe(e.Fechar("medico-1", time.Now())) != erros.CategoriaValidacao {
		t.Fatal("esperava validação: sem diagnóstico CID")
	}
	cid, _ := clinico.NovoDiagnosticoCID("J11", true)
	_ = e.DefinirDiagnosticosCID([]clinico.DiagnosticoCID{cid})
	if err := e.Fechar("medico-1", time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if e.Estado() != clinico.EstadoEpisodioFechado {
		t.Fatalf("estado=%q, esperava FECHADO", e.Estado())
	}
	if e.Snapshot().FechadoPor != "medico-1" || e.Snapshot().Fim == nil {
		t.Fatalf("campos de fecho não preenchidos: %+v", e.Snapshot())
	}
}

func TestActualizarNota_ProibidaSeNaoAberto(t *testing.T) {
	e := episodioAberto(t)
	_ = e.ActualizarNota(clinico.NovaNotaClinica("Q", "", "E", "D", "P"))
	cid, _ := clinico.NovoDiagnosticoCID("J11", false)
	_ = e.DefinirDiagnosticosCID([]clinico.DiagnosticoCID{cid})
	_ = e.Fechar("medico-1", time.Now())
	if erros.CategoriaDe(e.ActualizarNota(clinico.NovaNotaClinica("X", "", "Y", "Z", "W"))) != erros.CategoriaConflito {
		t.Fatal("esperava conflito ao alterar nota de episódio fechado")
	}
}

func TestDefinirDiagnosticos_MaximoUmPrincipal(t *testing.T) {
	e := episodioAberto(t)
	c1, _ := clinico.NovoDiagnosticoCID("J11", true)
	c2, _ := clinico.NovoDiagnosticoCID("J12", true)
	if erros.CategoriaDe(e.DefinirDiagnosticosCID([]clinico.DiagnosticoCID{c1, c2})) != erros.CategoriaValidacao {
		t.Fatal("esperava validação: dois diagnósticos principais")
	}
}

func TestCancelar(t *testing.T) {
	e := episodioAberto(t)
	if err := e.Cancelar(time.Now()); err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	if e.Estado() != clinico.EstadoEpisodioCancelado {
		t.Fatalf("estado=%q, esperava CANCELADO", e.Estado())
	}
	// Cancelar de novo (já cancelado) → conflito.
	if erros.CategoriaDe(e.Cancelar(time.Now())) != erros.CategoriaConflito {
		t.Fatal("esperava conflito ao cancelar um episódio não aberto")
	}
}

func TestReconstruirEpisodio_PreservaEstado(t *testing.T) {
	orig := episodioAberto(t)
	_ = orig.Cancelar(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	snap := orig.Snapshot()
	snap.ID = "ep-1"
	rec := clinico.ReconstruirEpisodio(snap)
	if rec.ID() != "ep-1" || rec.Estado() != clinico.EstadoEpisodioCancelado {
		t.Fatalf("rehidratação perdeu estado: id=%q estado=%q", rec.ID(), rec.Estado())
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/clinico/ -run Episodio -v`
Expected: FAIL — símbolos indefinidos.

- [ ] **Step 3: Implementar `episodio.go`**

```go
package clinico

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// EpisodioClinico é um agregado raiz do BC Clínico: um episódio de cuidados
// (consulta, urgência ou internamento) de um doente. Referencia o doente por id
// (agregado independente). O id é gerado pela base de dados.
type EpisodioClinico struct {
	id              string
	doenteID        string
	tipo            TipoEpisodio
	especialidadeID string
	medicoID        string
	inicio          time.Time
	fim             *time.Time
	nota            NotaClinica
	diagnosticosCID []DiagnosticoCID
	estado          EstadoEpisodio
	criadoEm        time.Time
	actualizadoEm   time.Time
	fechadoEm       *time.Time
	fechadoPor      string
}

// NovoEpisodio valida e constrói um episódio no estado ABERTO. O doente,
// a especialidade, o médico e o início são obrigatórios.
func NovoEpisodio(doenteID string, tipo TipoEpisodio, especialidadeID, medicoID string, inicio time.Time) (*EpisodioClinico, error) {
	doenteID = strings.TrimSpace(doenteID)
	if doenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente do episódio em falta")
	}
	if _, err := ParseTipoEpisodio(string(tipo)); err != nil {
		return nil, err
	}
	especialidadeID = strings.TrimSpace(especialidadeID)
	if especialidadeID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "especialidade do episódio em falta")
	}
	medicoID = strings.TrimSpace(medicoID)
	if medicoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "médico do episódio em falta")
	}
	if inicio.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "início do episódio em falta")
	}
	return &EpisodioClinico{
		doenteID:        doenteID,
		tipo:            tipo,
		especialidadeID: especialidadeID,
		medicoID:        medicoID,
		inicio:          inicio,
		estado:          EstadoEpisodioAberto,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados (vazio se não persistido).
func (e *EpisodioClinico) ID() string { return e.id }

// DoenteID devolve o id do doente a que o episódio pertence.
func (e *EpisodioClinico) DoenteID() string { return e.doenteID }

// Estado devolve o estado actual do episódio.
func (e *EpisodioClinico) Estado() EstadoEpisodio { return e.estado }

// ActualizarNota substitui a nota clínica. Só permitido em ABERTO.
func (e *EpisodioClinico) ActualizarNota(n NotaClinica) error {
	if e.estado != EstadoEpisodioAberto {
		return erros.Novo(erros.CategoriaConflito, "só é possível alterar um episódio aberto")
	}
	e.nota = n
	return nil
}

// DefinirDiagnosticosCID substitui a lista de diagnósticos CID. Só permitido em
// ABERTO; valida cada CID e admite no máximo um principal.
func (e *EpisodioClinico) DefinirDiagnosticosCID(cids []DiagnosticoCID) error {
	if e.estado != EstadoEpisodioAberto {
		return erros.Novo(erros.CategoriaConflito, "só é possível alterar um episódio aberto")
	}
	principais := 0
	for _, c := range cids {
		if _, err := NovoDiagnosticoCID(c.CID, c.Principal); err != nil {
			return err
		}
		if c.Principal {
			principais++
		}
	}
	if principais > 1 {
		return erros.Novo(erros.CategoriaValidacao, "só pode existir um diagnóstico principal")
	}
	e.diagnosticosCID = cids
	return nil
}

// Fechar encerra o episódio. Só de ABERTO; exige nota clínica completa e pelo
// menos um diagnóstico CID.
func (e *EpisodioClinico) Fechar(fechadoPor string, em time.Time) error {
	if e.estado != EstadoEpisodioAberto {
		return erros.Novo(erros.CategoriaConflito, "só é possível fechar um episódio aberto")
	}
	if !e.nota.Completa() {
		return erros.Novo(erros.CategoriaValidacao, "a nota clínica (queixa, exame, diagnóstico e plano) é obrigatória para fechar o episódio")
	}
	if len(e.diagnosticosCID) == 0 {
		return erros.Novo(erros.CategoriaValidacao, "é obrigatório pelo menos um diagnóstico CID para fechar o episódio")
	}
	fechadoPor = strings.TrimSpace(fechadoPor)
	if fechadoPor == "" {
		return erros.Novo(erros.CategoriaValidacao, "autor do fecho em falta")
	}
	e.estado = EstadoEpisodioFechado
	e.fim = &em
	e.fechadoEm = &em
	e.fechadoPor = fechadoPor
	return nil
}

// Cancelar cancela o episódio. Só de ABERTO. O motivo é auditado na aplicação.
func (e *EpisodioClinico) Cancelar(em time.Time) error {
	if e.estado != EstadoEpisodioAberto {
		return erros.Novo(erros.CategoriaConflito, "só é possível cancelar um episódio aberto")
	}
	e.estado = EstadoEpisodioCancelado
	e.fim = &em
	return nil
}

// SnapshotEpisodio carrega o estado completo de um episódio para persistência ou
// rehidratação (dados de fonte confiável — não revalida invariantes).
type SnapshotEpisodio struct {
	ID              string
	DoenteID        string
	Tipo            TipoEpisodio
	EspecialidadeID string
	MedicoID        string
	Inicio          time.Time
	Fim             *time.Time
	Nota            NotaClinica
	DiagnosticosCID []DiagnosticoCID
	Estado          EstadoEpisodio
	CriadoEm        time.Time
	ActualizadoEm   time.Time
	FechadoEm       *time.Time
	FechadoPor      string
}

// Snapshot devolve o estado completo do agregado.
func (e *EpisodioClinico) Snapshot() SnapshotEpisodio {
	return SnapshotEpisodio{
		ID:              e.id,
		DoenteID:        e.doenteID,
		Tipo:            e.tipo,
		EspecialidadeID: e.especialidadeID,
		MedicoID:        e.medicoID,
		Inicio:          e.inicio,
		Fim:             e.fim,
		Nota:            e.nota,
		DiagnosticosCID: e.diagnosticosCID,
		Estado:          e.estado,
		CriadoEm:        e.criadoEm,
		ActualizadoEm:   e.actualizadoEm,
		FechadoEm:       e.fechadoEm,
		FechadoPor:      e.fechadoPor,
	}
}

// ReconstruirEpisodio reconstrói um agregado a partir de um snapshot persistido.
func ReconstruirEpisodio(s SnapshotEpisodio) *EpisodioClinico {
	return &EpisodioClinico{
		id:              s.ID,
		doenteID:        s.DoenteID,
		tipo:            s.Tipo,
		especialidadeID: s.EspecialidadeID,
		medicoID:        s.MedicoID,
		inicio:          s.Inicio,
		fim:             s.Fim,
		nota:            s.Nota,
		diagnosticosCID: s.DiagnosticosCID,
		estado:          s.Estado,
		criadoEm:        s.CriadoEm,
		actualizadoEm:   s.ActualizadoEm,
		fechadoEm:       s.FechadoEm,
		fechadoPor:      s.FechadoPor,
	}
}
```

- [ ] **Step 4: Implementar `repositorio_episodios.go`**

```go
package clinico

import (
	"context"
	"time"
)

// FiltroEpisodios parametriza a listagem de episódios de um doente.
type FiltroEpisodios struct {
	DoenteID     string
	Estado       string // filtro opcional por estado
	Limite       int
	Deslocamento int
}

// ResumoEpisodio é o read-model de um episódio numa listagem.
type ResumoEpisodio struct {
	ID              string     `json:"id"`
	Tipo            string     `json:"tipo"`
	EspecialidadeID string     `json:"especialidade_id"`
	MedicoID        string     `json:"medico_id"`
	Inicio          time.Time  `json:"inicio"`
	Fim             *time.Time `json:"fim,omitempty"`
	Estado          string     `json:"estado"`
}

// PaginaEpisodios é uma página de episódios.
type PaginaEpisodios struct {
	Itens        []ResumoEpisodio `json:"itens"`
	Total        int              `json:"total"`
	Limite       int              `json:"limite"`
	Deslocamento int              `json:"deslocamento"`
}

// RepositorioEpisodios é a porta de saída para persistência do agregado
// EpisodioClinico. A implementação vive em adapters/pgrepo.
type RepositorioEpisodios interface {
	Guardar(ctx context.Context, e *EpisodioClinico) (string, error)
	ObterPorID(ctx context.Context, id string) (*EpisodioClinico, error)
	ListarPorDoente(ctx context.Context, f FiltroEpisodios) (PaginaEpisodios, error)
}
```

- [ ] **Step 5: Implementar `episodio_eventos.go`**

```go
package clinico

import (
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

// EpisodioAberto é emitido quando um episódio é iniciado.
type EpisodioAberto struct {
	EpisodioID string
	DoenteID   string
	Em         time.Time
}

func (e EpisodioAberto) NomeEvento() string    { return "clinico.episodio.aberto" }
func (e EpisodioAberto) OcorridoEm() time.Time { return e.Em }

// EpisodioFechado é emitido quando um episódio é fechado.
type EpisodioFechado struct {
	EpisodioID string
	DoenteID   string
	Em         time.Time
}

func (e EpisodioFechado) NomeEvento() string    { return "clinico.episodio.fechado" }
func (e EpisodioFechado) OcorridoEm() time.Time { return e.Em }

// EpisodioCancelado é emitido quando um episódio é cancelado.
type EpisodioCancelado struct {
	EpisodioID string
	DoenteID   string
	Em         time.Time
}

func (e EpisodioCancelado) NomeEvento() string    { return "clinico.episodio.cancelado" }
func (e EpisodioCancelado) OcorridoEm() time.Time { return e.Em }

// Garantias de conformidade com a interface de evento de domínio.
var (
	_ evento.EventoDominio = EpisodioAberto{}
	_ evento.EventoDominio = EpisodioFechado{}
	_ evento.EventoDominio = EpisodioCancelado{}
)
```

- [ ] **Step 6: Correr os testes e a cobertura do domínio**

Run: `go test ./internal/domain/clinico/ -v`
Expected: PASS.
Run: `bash scripts/cobertura.sh` (secção domínio ≥85%). Se abaixo, acrescenta casos ao `episodio_test.go` (ex.: DefinirDiagnosticosCID com CID vazio, Fechar sem fechadoPor, ActualizarNota em ABERTO com sucesso, ReconstruirEpisodio simetria de todos os campos).

- [ ] **Step 7: Commit**

```bash
git add internal/domain/clinico/episodio.go internal/domain/clinico/episodio_eventos.go internal/domain/clinico/repositorio_episodios.go internal/domain/clinico/episodio_test.go
git commit -m "feat(clinico): agregado Episódio Clínico com estados, eventos e porta de repositório

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Migration do episódio clínico

**Files:**
- Create: `migrations/clinico/0002_episodios.sql`

**Interfaces:**
- Consumes: runner de migrations existente; o `//go:embed` já inclui o directório `clinico` (Sprint 7), pelo que o novo ficheiro é embebido automaticamente — **não é preciso alterar `migrations/embed.go`**.
- Produces: tabelas `clinico.episodios_clinicos` e `clinico.diagnosticos_cid` + índices.

**Contexto:** esquema extraído verbatim do DDM-001. A FK `doente_id` é **sem `ON DELETE CASCADE`** (os episódios sobrevivem); a FK `episodio_id` de `diagnosticos_cid` é **com** `ON DELETE CASCADE`.

- [ ] **Step 1: Criar `migrations/clinico/0002_episodios.sql`**

```sql
-- Bounded Context: clinico
-- Migration forward-only. Esquema extraído verbatim do DDM-001 v2.0.
--
-- Episódio clínico (agregado raiz independente) e diagnósticos CID associados.

CREATE TABLE IF NOT EXISTS clinico.episodios_clinicos (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id        uuid        NOT NULL REFERENCES clinico.doentes(id),
    tipo             text        NOT NULL CHECK (tipo IN ('CONSULTA','URGENCIA','INTERNAMENTO')),
    especialidade_id uuid        NOT NULL,
    medico_id        uuid        NOT NULL,
    inicio           timestamptz NOT NULL,
    fim              timestamptz,
    queixa_principal text,
    historia_doenca  text,
    exame_objectivo  text,
    diagnostico      text,
    plano            text,
    estado           text        NOT NULL DEFAULT 'ABERTO'
                     CHECK (estado IN ('ABERTO','FECHADO','CANCELADO')),
    criado_em        timestamptz NOT NULL DEFAULT now(),
    actualizado_em   timestamptz NOT NULL DEFAULT now(),
    fechado_em       timestamptz,
    fechado_por      uuid
);

CREATE INDEX IF NOT EXISTS idx_episodios_doente ON clinico.episodios_clinicos (doente_id, inicio DESC);
CREATE INDEX IF NOT EXISTS idx_episodios_medico ON clinico.episodios_clinicos (medico_id, inicio DESC);
CREATE INDEX IF NOT EXISTS idx_episodios_estado ON clinico.episodios_clinicos (estado) WHERE estado = 'ABERTO';

COMMENT ON TABLE clinico.episodios_clinicos IS
    'Episódio clínico (agregado raiz). FK doente_id sem cascade — os episódios sobrevivem à pseudonimização do doente.';

CREATE TABLE IF NOT EXISTS clinico.diagnosticos_cid (
    episodio_id uuid    NOT NULL REFERENCES clinico.episodios_clinicos(id) ON DELETE CASCADE,
    cid         text    NOT NULL,
    principal   boolean NOT NULL DEFAULT false,
    PRIMARY KEY (episodio_id, cid)
);
```

- [ ] **Step 2: Confirmar embed e migrations**

Run: `go test ./migrations/ -v`
Expected: PASS (o embed inclui automaticamente `clinico/0002_episodios.sql`).
Run: `go build ./...`
Expected: sem erros.

> Se o `migrations/embed_test.go` tiver uma lista fixa de ficheiros esperados, acrescenta `clinico/0002_episodios.sql` a essa lista. Caso contrário, não alteres o teste.

- [ ] **Step 3: Commit**

```bash
git add migrations/clinico/0002_episodios.sql
git commit -m "feat(clinico): migration do episódio clínico e diagnósticos CID

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Portas/DTOs, mapeamento e caso de uso Iniciar Episódio

**Files:**
- Modify: `internal/application/clinico/ports.go` (acrescentar DTOs e reexports do episódio — **não alterar** os existentes)
- Create: `internal/application/clinico/mapa_episodio.go`
- Create: `internal/application/clinico/iniciar_episodio.go`
- Test: `internal/application/clinico/fakes_episodio_test.go`
- Test: `internal/application/clinico/iniciar_episodio_test.go`

**Interfaces:**
- Consumes: domínio `EpisodioClinico`, `NovoEpisodio`, `ParseTipoEpisodio`, `NotaClinica`/`NovaNotaClinica`, `DiagnosticoCID`/`NovoDiagnosticoCID`, `RepositorioEpisodios`, `RepositorioDoentes`, `EstadoActivo`, read-models; porta `Auditor` (existente).
- Produces:
  - DTOs em `ports.go`: `DadosNovoEpisodio`, `DadosNotaClinica`, `DadosDiagnosticoCID`, `DadosActualizarEpisodio`, `NotaClinicaDTO`, `DiagnosticoCIDDTO`, `DetalheEpisodio`, `EHR`; reexports `FiltroEpisodios = dominio.FiltroEpisodios`, `PaginaEpisodios = dominio.PaginaEpisodios`, `ResumoEpisodio = dominio.ResumoEpisodio`.
  - `mapa_episodio.go`: `paraDetalheEpisodio(*dominio.EpisodioClinico) DetalheEpisodio`, `construirNota(DadosNotaClinica) dominio.NotaClinica`, `construirDiagnosticos([]DadosDiagnosticoCID) []dominio.DiagnosticoCID`.
  - `CasoIniciarEpisodio` com `NovoCasoIniciarEpisodio(ep dominio.RepositorioEpisodios, doentes dominio.RepositorioDoentes, aud Auditor)` e `Executar(ctx, actor string, dados DadosNovoEpisodio) (DetalheEpisodio, error)`.

**Regras Iniciar:** carrega o doente via `doentes.ObterPorID(dados.DoenteID)` (NaoEncontrado propaga → 404); se `doente.Estado() != dominio.EstadoActivo` → `CategoriaConflito`; `inicio` = `dados.Inicio` se presente, senão `agora()`; `NovoEpisodio`; `episodios.Guardar`; audita `clinico.episodio.aberto` (Entidade "episodio", EntidadeID = id); re-lê via `episodios.ObterPorID` e devolve `DetalheEpisodio`.

- [ ] **Step 1: Escrever os fakes e o teste que falha**

`fakes_episodio_test.go`:
```go
package clinico_test

import (
	"context"
	"strconv"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeRepoEpisodios é um repositório de episódios em memória para os testes.
type fakeRepoEpisodios struct {
	porID      map[string]*clinico.EpisodioClinico
	seq        int
	guardarErr error
	pagina     clinico.PaginaEpisodios
	ultimoFilt clinico.FiltroEpisodios
}

func novoFakeRepoEpisodios() *fakeRepoEpisodios {
	return &fakeRepoEpisodios{porID: map[string]*clinico.EpisodioClinico{}}
}

func (f *fakeRepoEpisodios) Guardar(_ context.Context, e *clinico.EpisodioClinico) (string, error) {
	if f.guardarErr != nil {
		return "", f.guardarErr
	}
	snap := e.Snapshot()
	id := snap.ID
	if id == "" {
		f.seq++
		id = "ep-" + strconv.Itoa(f.seq)
		snap.ID = id
	}
	f.porID[id] = clinico.ReconstruirEpisodio(snap)
	return id, nil
}

func (f *fakeRepoEpisodios) ObterPorID(_ context.Context, id string) (*clinico.EpisodioClinico, error) {
	e, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "episódio não encontrado")
	}
	return e, nil
}

func (f *fakeRepoEpisodios) ListarPorDoente(_ context.Context, filt clinico.FiltroEpisodios) (clinico.PaginaEpisodios, error) {
	f.ultimoFilt = filt
	return f.pagina, nil
}
```

`iniciar_episodio_test.go`:
```go
package clinico_test

import (
	"context"
	"testing"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func dadosEpisodioBase(doenteID string) appclinico.DadosNovoEpisodio {
	return appclinico.DadosNovoEpisodio{
		DoenteID: doenteID, Tipo: "CONSULTA", EspecialidadeID: "esp-1", MedicoID: "medico-1",
	}
}

func TestIniciarEpisodio_DoenteActivo(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes) // cria um doente ACTIVO
	repoEp := novoFakeRepoEpisodios()
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoIniciarEpisodio(repoEp, repoDoentes, aud)

	out, err := caso.Executar(context.Background(), "medico-1", dadosEpisodioBase(doenteID))
	if err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if out.ID == "" || out.Estado != "ABERTO" {
		t.Fatalf("saída inesperada: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.episodio.aberto" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestIniciarEpisodio_DoenteNaoEncontrado(t *testing.T) {
	caso := appclinico.NovoCasoIniciarEpisodio(novoFakeRepoEpisodios(), novoFakeRepo(), &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", dadosEpisodioBase("inexistente"))
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestIniciarEpisodio_DoenteNaoActivo(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	// Desactiva o doente directamente no fake.
	d, _ := repoDoentes.ObterPorID(context.Background(), doenteID)
	_ = d.Desactivar("teste", timeFixa())
	_, _ = repoDoentes.Guardar(context.Background(), d)

	caso := appclinico.NovoCasoIniciarEpisodio(novoFakeRepoEpisodios(), repoDoentes, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", dadosEpisodioBase(doenteID))
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito (doente não activo), obtive %v", err)
	}
}
```

> **Nota ao implementador:** o helper `timeFixa()` acima — se ainda não existir no pacote `clinico_test`, define-o num dos ficheiros de teste desta task (`func timeFixa() time.Time { return time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC) }`), com o import `"time"`. Se já existir (procura antes), reutiliza-o.

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/clinico/ -run IniciarEpisodio -v`
Expected: FAIL — pacote/símbolos inexistentes.

- [ ] **Step 3: Acrescentar os DTOs a `ports.go`**

Acrescentar ao fim de `internal/application/clinico/ports.go` (sem tocar no que existe):

```go
// --- Episódio Clínico ---

// Reexports dos read-models de episódio.
type (
	FiltroEpisodios = dominio.FiltroEpisodios
	PaginaEpisodios = dominio.PaginaEpisodios
	ResumoEpisodio  = dominio.ResumoEpisodio
)

// DadosNovoEpisodio é a entrada do caso de uso de iniciar episódio. DoenteID vem
// do caminho do pedido; Inicio é opcional (default: momento da criação).
type DadosNovoEpisodio struct {
	DoenteID        string
	Tipo            string
	EspecialidadeID string
	MedicoID        string
	Inicio          *time.Time
}

// DadosNotaClinica é a nota clínica num pedido de actualização.
type DadosNotaClinica struct {
	QueixaPrincipal string `json:"queixa_principal"`
	HistoriaDoenca  string `json:"historia_doenca"`
	ExameObjectivo  string `json:"exame_objectivo"`
	Diagnostico     string `json:"diagnostico"`
	Plano           string `json:"plano"`
}

// DadosDiagnosticoCID é um diagnóstico CID num pedido.
type DadosDiagnosticoCID struct {
	CID       string `json:"cid"`
	Principal bool   `json:"principal"`
}

// DadosActualizarEpisodio é a entrada da actualização (campos a nil ignorados).
type DadosActualizarEpisodio struct {
	Nota            *DadosNotaClinica
	DiagnosticosCID *[]DadosDiagnosticoCID
}

// NotaClinicaDTO é a nota clínica numa resposta.
type NotaClinicaDTO struct {
	QueixaPrincipal string `json:"queixa_principal,omitempty"`
	HistoriaDoenca  string `json:"historia_doenca,omitempty"`
	ExameObjectivo  string `json:"exame_objectivo,omitempty"`
	Diagnostico     string `json:"diagnostico,omitempty"`
	Plano           string `json:"plano,omitempty"`
}

// DiagnosticoCIDDTO é um diagnóstico CID numa resposta.
type DiagnosticoCIDDTO struct {
	CID       string `json:"cid"`
	Principal bool   `json:"principal"`
}

// DetalheEpisodio é o detalhe completo de um episódio numa resposta.
type DetalheEpisodio struct {
	ID              string              `json:"id"`
	DoenteID        string              `json:"doente_id"`
	Tipo            string              `json:"tipo"`
	EspecialidadeID string              `json:"especialidade_id"`
	MedicoID        string              `json:"medico_id"`
	Inicio          time.Time           `json:"inicio"`
	Fim             *time.Time          `json:"fim,omitempty"`
	Nota            NotaClinicaDTO      `json:"nota"`
	DiagnosticosCID []DiagnosticoCIDDTO `json:"diagnosticos_cid"`
	Estado          string              `json:"estado"`
	CriadoEm        time.Time           `json:"criado_em"`
	ActualizadoEm   time.Time           `json:"actualizado_em"`
	FechadoEm       *time.Time          `json:"fechado_em,omitempty"`
	FechadoPor      string              `json:"fechado_por,omitempty"`
}

// EHR é a projecção de leitura do registo clínico: doente (com alergias e
// antecedentes) + episódios paginados.
type EHR struct {
	Doente    DetalheDoente   `json:"doente"`
	Episodios PaginaEpisodios `json:"episodios"`
}
```

- [ ] **Step 4: Implementar `mapa_episodio.go`**

```go
package clinico

import dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"

// construirNota converte o DTO de nota clínica no VO de domínio.
func construirNota(d DadosNotaClinica) dominio.NotaClinica {
	return dominio.NovaNotaClinica(d.QueixaPrincipal, d.HistoriaDoenca, d.ExameObjectivo, d.Diagnostico, d.Plano)
}

// construirDiagnosticos converte os DTOs de diagnóstico nos VOs de domínio (sem
// validar aqui — a validação vive em EpisodioClinico.DefinirDiagnosticosCID).
func construirDiagnosticos(ds []DadosDiagnosticoCID) []dominio.DiagnosticoCID {
	out := make([]dominio.DiagnosticoCID, 0, len(ds))
	for _, d := range ds {
		out = append(out, dominio.DiagnosticoCID{CID: d.CID, Principal: d.Principal})
	}
	return out
}

// paraDetalheEpisodio mapeia um agregado EpisodioClinico para o DTO de detalhe.
func paraDetalheEpisodio(e *dominio.EpisodioClinico) DetalheEpisodio {
	s := e.Snapshot()
	det := DetalheEpisodio{
		ID:              s.ID,
		DoenteID:        s.DoenteID,
		Tipo:            string(s.Tipo),
		EspecialidadeID: s.EspecialidadeID,
		MedicoID:        s.MedicoID,
		Inicio:          s.Inicio,
		Fim:             s.Fim,
		Nota: NotaClinicaDTO{
			QueixaPrincipal: s.Nota.QueixaPrincipal,
			HistoriaDoenca:  s.Nota.HistoriaDoenca,
			ExameObjectivo:  s.Nota.ExameObjectivo,
			Diagnostico:     s.Nota.Diagnostico,
			Plano:           s.Nota.Plano,
		},
		DiagnosticosCID: []DiagnosticoCIDDTO{},
		Estado:          string(s.Estado),
		CriadoEm:        s.CriadoEm,
		ActualizadoEm:   s.ActualizadoEm,
		FechadoEm:       s.FechadoEm,
		FechadoPor:      s.FechadoPor,
	}
	for _, c := range s.DiagnosticosCID {
		det.DiagnosticosCID = append(det.DiagnosticosCID, DiagnosticoCIDDTO{CID: c.CID, Principal: c.Principal})
	}
	return det
}
```

- [ ] **Step 5: Implementar `iniciar_episodio.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoIniciarEpisodio inicia um episódio clínico para um doente activo e audita.
type CasoIniciarEpisodio struct {
	episodios dominio.RepositorioEpisodios
	doentes   dominio.RepositorioDoentes
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoIniciarEpisodio constrói o caso de uso.
func NovoCasoIniciarEpisodio(ep dominio.RepositorioEpisodios, doentes dominio.RepositorioDoentes, aud Auditor) *CasoIniciarEpisodio {
	return &CasoIniciarEpisodio{episodios: ep, doentes: doentes, auditor: aud, agora: time.Now}
}

// Executar valida o doente (existe e activo), cria o episódio, persiste e audita.
func (c *CasoIniciarEpisodio) Executar(ctx context.Context, actor string, dados DadosNovoEpisodio) (DetalheEpisodio, error) {
	doente, err := c.doentes.ObterPorID(ctx, dados.DoenteID)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if doente.Estado() != dominio.EstadoActivo {
		return DetalheEpisodio{}, erros.Novo(erros.CategoriaConflito, "não é possível abrir um episódio a um doente que não está activo")
	}
	tipo, err := dominio.ParseTipoEpisodio(dados.Tipo)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	inicio := c.agora()
	if dados.Inicio != nil {
		inicio = *dados.Inicio
	}
	episodio, err := dominio.NovoEpisodio(dados.DoenteID, tipo, dados.EspecialidadeID, dados.MedicoID, inicio)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	id, err := c.episodios.Guardar(ctx, episodio)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.aberto",
		Entidade: "episodio", EntidadeID: id, OcorridoEm: c.agora(),
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

- [ ] **Step 6: Correr os testes e confirmar que passam**

Run: `go test ./internal/application/clinico/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/application/clinico/ports.go internal/application/clinico/mapa_episodio.go internal/application/clinico/iniciar_episodio.go internal/application/clinico/fakes_episodio_test.go internal/application/clinico/iniciar_episodio_test.go
git commit -m "feat(clinico): portas, DTOs e caso de uso de iniciar episódio

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Casos de uso Actualizar, Fechar e Cancelar Episódio

**Files:**
- Create: `internal/application/clinico/actualizar_episodio.go`
- Create: `internal/application/clinico/fechar_episodio.go`
- Create: `internal/application/clinico/cancelar_episodio.go`
- Test: `internal/application/clinico/gerir_episodio_test.go`

**Interfaces:**
- Consumes: `RepositorioEpisodios`, `Auditor`, `construirNota`, `construirDiagnosticos`, `paraDetalheEpisodio` (Task 4); domínio `ActualizarNota`/`DefinirDiagnosticosCID`/`Fechar`/`Cancelar`.
- Produces:
  - `CasoActualizarEpisodio` — `NovoCasoActualizarEpisodio(ep, aud)`, `Executar(ctx, actor, id string, dados DadosActualizarEpisodio) (DetalheEpisodio, error)`.
  - `CasoFecharEpisodio` — `NovoCasoFecharEpisodio(ep, aud)`, `Executar(ctx, actor, id string) (DetalheEpisodio, error)`.
  - `CasoCancelarEpisodio` — `NovoCasoCancelarEpisodio(ep, aud)`, `Executar(ctx, actor, id, motivo string) (DetalheEpisodio, error)`.

**Regras:** cada método hidrata (`ObterPorID`), aplica a transição de domínio, `Guardar`, audita, re-lê e devolve o detalhe. Actualizar: se `dados.Nota != nil` aplica `ActualizarNota(construirNota(...))`; se `dados.DiagnosticosCID != nil` aplica `DefinirDiagnosticosCID(construirDiagnosticos(...))`. Fechar: `Fechar(actor, agora())`. Cancelar: `Cancelar(agora())` e audita com o `motivo` em `Detalhe`.

- [ ] **Step 1: Escrever o teste que falha (`gerir_episodio_test.go`)**

```go
package clinico_test

import (
	"context"
	"testing"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// iniciarNoRepo cria um episódio ABERTO no fake e devolve o seu id.
func iniciarNoRepo(t *testing.T, repoEp *fakeRepoEpisodios, repoDoentes *fakeRepo) string {
	t.Helper()
	doenteID := registarNoRepo(t, repoDoentes)
	caso := appclinico.NovoCasoIniciarEpisodio(repoEp, repoDoentes, &fakeAuditor{})
	out, err := caso.Executar(context.Background(), "medico-1", dadosEpisodioBase(doenteID))
	if err != nil {
		t.Fatalf("preparar episódio: %v", err)
	}
	return out.ID
}

func TestActualizarEpisodio_NotaEDiagnosticos(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoActualizarEpisodio(repoEp, aud)

	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre", ExameObjectivo: "Temp 39", Diagnostico: "Gripe", Plano: "Repouso"}
	cids := &[]appclinico.DadosDiagnosticoCID{{CID: "J11", Principal: true}}
	out, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: nota, DiagnosticosCID: cids})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.Nota.Diagnostico != "Gripe" || len(out.DiagnosticosCID) != 1 {
		t.Fatalf("actualização não reflectida: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.episodio.actualizado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestFecharEpisodio_SemNota_Erro(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	caso := appclinico.NovoCasoFecharEpisodio(repoEp, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", id)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação (nota incompleta), obtive %v", err)
	}
}

func TestFecharEpisodio_Completo(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	repoDoentes := novoFakeRepo()
	id := iniciarNoRepo(t, repoEp, repoDoentes)
	// Preenche nota + CID.
	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre", ExameObjectivo: "Temp 39", Diagnostico: "Gripe", Plano: "Repouso"}
	cids := &[]appclinico.DadosDiagnosticoCID{{CID: "J11", Principal: true}}
	_, _ = appclinico.NovoCasoActualizarEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: nota, DiagnosticosCID: cids})

	aud := &fakeAuditor{}
	out, err := appclinico.NovoCasoFecharEpisodio(repoEp, aud).Executar(context.Background(), "medico-1", id)
	if err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if out.Estado != "FECHADO" || out.FechadoPor != "medico-1" {
		t.Fatalf("fecho inesperado: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.episodio.fechado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestCancelarEpisodio(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	aud := &fakeAuditor{}
	out, err := appclinico.NovoCasoCancelarEpisodio(repoEp, aud).Executar(context.Background(), "medico-1", id, "duplicado")
	if err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	if out.Estado != "CANCELADO" {
		t.Fatalf("estado=%q, esperava CANCELADO", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.episodio.cancelado" || aud.registos[0].Detalhe == "" {
		t.Fatalf("auditoria em falta ou sem motivo: %+v", aud.registos)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/clinico/ -run 'ActualizarEpisodio|FecharEpisodio|CancelarEpisodio' -v`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Implementar `actualizar_episodio.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoActualizarEpisodio actualiza a nota clínica e/ou os diagnósticos CID de um
// episódio aberto e audita.
type CasoActualizarEpisodio struct {
	episodios dominio.RepositorioEpisodios
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoActualizarEpisodio constrói o caso de uso.
func NovoCasoActualizarEpisodio(ep dominio.RepositorioEpisodios, aud Auditor) *CasoActualizarEpisodio {
	return &CasoActualizarEpisodio{episodios: ep, auditor: aud, agora: time.Now}
}

// Executar aplica as alterações fornecidas (campos a nil ficam inalterados).
func (c *CasoActualizarEpisodio) Executar(ctx context.Context, actor, id string, dados DadosActualizarEpisodio) (DetalheEpisodio, error) {
	episodio, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if dados.Nota != nil {
		if err := episodio.ActualizarNota(construirNota(*dados.Nota)); err != nil {
			return DetalheEpisodio{}, err
		}
	}
	if dados.DiagnosticosCID != nil {
		if err := episodio.DefinirDiagnosticosCID(construirDiagnosticos(*dados.DiagnosticosCID)); err != nil {
			return DetalheEpisodio{}, err
		}
	}
	if _, err := c.episodios.Guardar(ctx, episodio); err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.actualizado",
		Entidade: "episodio", EntidadeID: id, OcorridoEm: c.agora(),
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

- [ ] **Step 4: Implementar `fechar_episodio.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoFecharEpisodio fecha um episódio (exige nota completa + ≥1 CID no domínio)
// e audita.
type CasoFecharEpisodio struct {
	episodios dominio.RepositorioEpisodios
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoFecharEpisodio constrói o caso de uso.
func NovoCasoFecharEpisodio(ep dominio.RepositorioEpisodios, aud Auditor) *CasoFecharEpisodio {
	return &CasoFecharEpisodio{episodios: ep, auditor: aud, agora: time.Now}
}

// Executar fecha o episódio identificado, registando o actor como fechado_por.
func (c *CasoFecharEpisodio) Executar(ctx context.Context, actor, id string) (DetalheEpisodio, error) {
	episodio, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if err := episodio.Fechar(actor, c.agora()); err != nil {
		return DetalheEpisodio{}, err
	}
	if _, err := c.episodios.Guardar(ctx, episodio); err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.fechado",
		Entidade: "episodio", EntidadeID: id, OcorridoEm: c.agora(),
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

- [ ] **Step 5: Implementar `cancelar_episodio.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoCancelarEpisodio cancela um episódio aberto e audita (motivo em Detalhe).
type CasoCancelarEpisodio struct {
	episodios dominio.RepositorioEpisodios
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoCancelarEpisodio constrói o caso de uso.
func NovoCasoCancelarEpisodio(ep dominio.RepositorioEpisodios, aud Auditor) *CasoCancelarEpisodio {
	return &CasoCancelarEpisodio{episodios: ep, auditor: aud, agora: time.Now}
}

// Executar cancela o episódio identificado.
func (c *CasoCancelarEpisodio) Executar(ctx context.Context, actor, id, motivo string) (DetalheEpisodio, error) {
	episodio, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if err := episodio.Cancelar(c.agora()); err != nil {
		return DetalheEpisodio{}, err
	}
	if _, err := c.episodios.Guardar(ctx, episodio); err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.cancelado",
		Entidade: "episodio", EntidadeID: id, Detalhe: motivo, OcorridoEm: c.agora(),
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

- [ ] **Step 6: Correr os testes e confirmar que passam**

Run: `go test ./internal/application/clinico/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/application/clinico/actualizar_episodio.go internal/application/clinico/fechar_episodio.go internal/application/clinico/cancelar_episodio.go internal/application/clinico/gerir_episodio_test.go
git commit -m "feat(clinico): casos de uso de actualização, fecho e cancelamento de episódio

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Casos de uso Obter, Listar e EHR

**Files:**
- Create: `internal/application/clinico/obter_episodio.go`
- Create: `internal/application/clinico/listar_episodios.go`
- Create: `internal/application/clinico/obter_ehr.go`
- Test: `internal/application/clinico/obter_listar_ehr_test.go`

**Interfaces:**
- Consumes: `RepositorioEpisodios`, `RepositorioDoentes`, `Auditor`, `paraDetalheEpisodio`, `paraDetalhe` (do Sprint 7), `FiltroEpisodios`/`PaginaEpisodios`, `EHR`.
- Produces:
  - `CasoObterEpisodio` — `NovoCasoObterEpisodio(ep, aud)`, `Executar(ctx, actor, id string) (DetalheEpisodio, error)` (audita `clinico.episodio.consultado`).
  - `CasoListarEpisodios` — `NovoCasoListarEpisodios(ep)`, `Executar(ctx, doenteID string, filtro FiltroEpisodios) (PaginaEpisodios, error)` (normaliza limites 20/100; **não** audita).
  - `CasoObterEHR` — `NovoCasoObterEHR(doentes, ep, aud)`, `Executar(ctx, actor, doenteID string, filtroEpisodios FiltroEpisodios) (EHR, error)` (audita `clinico.ehr.consultado`).

**Regras:** Obter audita a consulta. Listar aplica `DoenteID` do argumento ao filtro, limite default 20 / máximo 100, deslocamento ≥0, delega `ListarPorDoente`. EHR: `doentes.ObterPorID` (traz alergias/antecedentes) → `paraDetalhe`; `episodios.ListarPorDoente` (com limites normalizados) → monta `EHR`; audita.

- [ ] **Step 1: Escrever o teste que falha (`obter_listar_ehr_test.go`)**

```go
package clinico_test

import (
	"context"
	"testing"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestObterEpisodio_Audita(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	aud := &fakeAuditor{}
	out, err := appclinico.NovoCasoObterEpisodio(repoEp, aud).Executar(context.Background(), "medico-1", id)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if out.ID != id {
		t.Fatalf("id inesperado: %q", out.ID)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.episodio.consultado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestObterEpisodio_NaoEncontrado(t *testing.T) {
	_, err := appclinico.NovoCasoObterEpisodio(novoFakeRepoEpisodios(), &fakeAuditor{}).Executar(context.Background(), "m", "inexistente")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestListarEpisodios_AplicaDoenteELimite(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	caso := appclinico.NovoCasoListarEpisodios(repoEp)
	if _, err := caso.Executar(context.Background(), "doente-9", appclinico.FiltroEpisodios{}); err != nil {
		t.Fatalf("listar: %v", err)
	}
	if repoEp.ultimoFilt.DoenteID != "doente-9" || repoEp.ultimoFilt.Limite != 20 {
		t.Fatalf("filtro inesperado: %+v", repoEp.ultimoFilt)
	}
	if _, err := caso.Executar(context.Background(), "doente-9", appclinico.FiltroEpisodios{Limite: 5000}); err != nil {
		t.Fatalf("listar: %v", err)
	}
	if repoEp.ultimoFilt.Limite != 100 {
		t.Fatalf("limite máximo=%d, esperava 100", repoEp.ultimoFilt.Limite)
	}
}

func TestObterEHR_CombinaDoenteEEpisodios(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	repoEp.pagina = clinico.PaginaEpisodios{Total: 1, Itens: []clinico.ResumoEpisodio{{ID: "ep-1", Estado: "ABERTO"}}}
	aud := &fakeAuditor{}

	ehr, err := appclinico.NovoCasoObterEHR(repoDoentes, repoEp, aud).Executar(context.Background(), "medico-1", doenteID, appclinico.FiltroEpisodios{})
	if err != nil {
		t.Fatalf("ehr: %v", err)
	}
	if ehr.Doente.ID != doenteID || ehr.Episodios.Total != 1 {
		t.Fatalf("EHR inesperado: %+v", ehr)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.ehr.consultado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/clinico/ -run 'ObterEpisodio|ListarEpisodios|ObterEHR' -v`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Implementar `obter_episodio.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoObterEpisodio devolve o detalhe de um episódio e audita o acesso.
type CasoObterEpisodio struct {
	episodios dominio.RepositorioEpisodios
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoObterEpisodio constrói o caso de uso.
func NovoCasoObterEpisodio(ep dominio.RepositorioEpisodios, aud Auditor) *CasoObterEpisodio {
	return &CasoObterEpisodio{episodios: ep, auditor: aud, agora: time.Now}
}

// Executar carrega o episódio, audita a consulta e devolve o detalhe.
func (c *CasoObterEpisodio) Executar(ctx context.Context, actor, id string) (DetalheEpisodio, error) {
	episodio, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.consultado",
		Entidade: "episodio", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheEpisodio{}, err
	}
	return paraDetalheEpisodio(episodio), nil
}
```

- [ ] **Step 4: Implementar `listar_episodios.go`**

```go
package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

// CasoListarEpisodios lista os episódios de um doente, com paginação.
type CasoListarEpisodios struct {
	episodios dominio.RepositorioEpisodios
}

// NovoCasoListarEpisodios constrói o caso de uso.
func NovoCasoListarEpisodios(ep dominio.RepositorioEpisodios) *CasoListarEpisodios {
	return &CasoListarEpisodios{episodios: ep}
}

// Executar normaliza os limites e delega a listagem ao repositório.
func (c *CasoListarEpisodios) Executar(ctx context.Context, doenteID string, filtro FiltroEpisodios) (PaginaEpisodios, error) {
	filtro.DoenteID = doenteID
	if filtro.Limite <= 0 {
		filtro.Limite = limiteDefault
	}
	if filtro.Limite > limiteMaximo {
		filtro.Limite = limiteMaximo
	}
	if filtro.Deslocamento < 0 {
		filtro.Deslocamento = 0
	}
	return c.episodios.ListarPorDoente(ctx, filtro)
}
```

> **Nota:** `limiteDefault` (20) e `limiteMaximo` (100) já estão declarados em `pesquisar_doentes.go` (mesmo pacote) — reutiliza-os, **não os redeclares**.

- [ ] **Step 5: Implementar `obter_ehr.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoObterEHR monta a projecção de leitura EHR (doente + alergias/antecedentes +
// episódios paginados) e audita o acesso.
type CasoObterEHR struct {
	doentes   dominio.RepositorioDoentes
	episodios dominio.RepositorioEpisodios
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoObterEHR constrói o caso de uso.
func NovoCasoObterEHR(doentes dominio.RepositorioDoentes, ep dominio.RepositorioEpisodios, aud Auditor) *CasoObterEHR {
	return &CasoObterEHR{doentes: doentes, episodios: ep, auditor: aud, agora: time.Now}
}

// Executar carrega o doente e os seus episódios paginados, audita e devolve o EHR.
func (c *CasoObterEHR) Executar(ctx context.Context, actor, doenteID string, filtroEpisodios FiltroEpisodios) (EHR, error) {
	doente, err := c.doentes.ObterPorID(ctx, doenteID)
	if err != nil {
		return EHR{}, err
	}
	filtroEpisodios.DoenteID = doenteID
	if filtroEpisodios.Limite <= 0 {
		filtroEpisodios.Limite = limiteDefault
	}
	if filtroEpisodios.Limite > limiteMaximo {
		filtroEpisodios.Limite = limiteMaximo
	}
	if filtroEpisodios.Deslocamento < 0 {
		filtroEpisodios.Deslocamento = 0
	}
	pagina, err := c.episodios.ListarPorDoente(ctx, filtroEpisodios)
	if err != nil {
		return EHR{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.ehr.consultado",
		Entidade: "doente", EntidadeID: doenteID, OcorridoEm: c.agora(),
	}); err != nil {
		return EHR{}, err
	}
	return EHR{Doente: paraDetalhe(doente), Episodios: pagina}, nil
}
```

- [ ] **Step 6: Correr os testes e a cobertura da aplicação**

Run: `go test ./internal/application/clinico/ -v`
Expected: PASS.
Run: `bash scripts/cobertura.sh` (secção aplicação ≥75%). Se abaixo, acrescenta casos (ex.: ObterEHR com doente não encontrado propaga; ListarEpisodios com deslocamento negativo).

- [ ] **Step 7: Commit**

```bash
git add internal/application/clinico/obter_episodio.go internal/application/clinico/listar_episodios.go internal/application/clinico/obter_ehr.go internal/application/clinico/obter_listar_ehr_test.go
git commit -m "feat(clinico): casos de uso obter/listar episódios e projecção EHR

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Repositório PostgreSQL de episódios e teste de integração

**Files:**
- Create: `internal/adapters/pgrepo/episodios_repo.go`
- Test: `tests/integration/episodios_test.go` (tag `integration`)

**Interfaces:**
- Consumes: domínio `EpisodioClinico`, `SnapshotEpisodio`, `ReconstruirEpisodio`, `NotaClinica`, `DiagnosticoCID`, `TipoEpisodio`, `EstadoEpisodio`, `FiltroEpisodios`/`PaginaEpisodios`/`ResumoEpisodio`; `erros`; `pgx`/`pgxpool`. O helper `deref(*string) string` já existe no pacote `pgrepo` (reutiliza).
- Produces: `RepositorioEpisodios` com `NovoRepositorioEpisodios(pool *pgxpool.Pool) *RepositorioEpisodios`, implementando `clinico.RepositorioEpisodios`.

**Padrão:** seguir `internal/adapters/pgrepo/doentes_repo.go` (transacção com defer rollback; `errors.Is(err, pgx.ErrNoRows)` → `CategoriaNaoEncontrado`; filhos por delete-and-reinsert; `NULLIF(...,'')` para texto opcional no INSERT/UPDATE e `COALESCE(...,'')` na leitura). O gate de cobertura corre sem a tag `integration`, por isso este repo fica coberto **apenas** pelo teste de integração.

- [ ] **Step 1: Implementar `episodios_repo.go`**

```go
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioEpisodios implementa dominio.RepositorioEpisodios com pgx.
type RepositorioEpisodios struct {
	pool *pgxpool.Pool
}

// NovoRepositorioEpisodios constrói o repositório sobre o pool pgx.
func NovoRepositorioEpisodios(pool *pgxpool.Pool) *RepositorioEpisodios {
	return &RepositorioEpisodios{pool: pool}
}

// Guardar persiste o episódio (INSERT se id vazio, senão UPDATE) e os seus
// diagnósticos CID, numa única transacção.
func (r *RepositorioEpisodios) Guardar(ctx context.Context, e *dominio.EpisodioClinico) (string, error) {
	s := e.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := s.ID
	if id == "" {
		id, err = r.inserirEpisodio(ctx, tx, s)
	} else {
		err = r.actualizarEpisodio(ctx, tx, s)
	}
	if err != nil {
		return "", err
	}
	if err := r.guardarDiagnosticos(ctx, tx, id, s); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar transacção: %w", err)
	}
	return id, nil
}

func (r *RepositorioEpisodios) inserirEpisodio(ctx context.Context, tx pgx.Tx, s dominio.SnapshotEpisodio) (string, error) {
	const q = `
INSERT INTO clinico.episodios_clinicos (
    doente_id, tipo, especialidade_id, medico_id, inicio, fim,
    queixa_principal, historia_doenca, exame_objectivo, diagnostico, plano,
    estado, fechado_em, fechado_por
) VALUES (
    $1,$2,$3,$4,$5,$6,
    NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),
    $12,$13,NULLIF($14,'')
) RETURNING id::text`
	var id string
	err := tx.QueryRow(ctx, q,
		s.DoenteID, string(s.Tipo), s.EspecialidadeID, s.MedicoID, s.Inicio, s.Fim,
		s.Nota.QueixaPrincipal, s.Nota.HistoriaDoenca, s.Nota.ExameObjectivo, s.Nota.Diagnostico, s.Nota.Plano,
		string(s.Estado), s.FechadoEm, s.FechadoPor,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserir episódio: %w", err)
	}
	return id, nil
}

func (r *RepositorioEpisodios) actualizarEpisodio(ctx context.Context, tx pgx.Tx, s dominio.SnapshotEpisodio) error {
	const q = `
UPDATE clinico.episodios_clinicos SET
    tipo=$2, especialidade_id=$3, medico_id=$4, inicio=$5, fim=$6,
    queixa_principal=NULLIF($7,''), historia_doenca=NULLIF($8,''), exame_objectivo=NULLIF($9,''),
    diagnostico=NULLIF($10,''), plano=NULLIF($11,''),
    estado=$12, fechado_em=$13, fechado_por=NULLIF($14,''), actualizado_em=now()
WHERE id=$1`
	ct, err := tx.Exec(ctx, q, s.ID,
		string(s.Tipo), s.EspecialidadeID, s.MedicoID, s.Inicio, s.Fim,
		s.Nota.QueixaPrincipal, s.Nota.HistoriaDoenca, s.Nota.ExameObjectivo, s.Nota.Diagnostico, s.Nota.Plano,
		string(s.Estado), s.FechadoEm, s.FechadoPor,
	)
	if err != nil {
		return fmt.Errorf("actualizar episódio: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return erros.Novo(erros.CategoriaNaoEncontrado, "episódio não encontrado")
	}
	return nil
}

func (r *RepositorioEpisodios) guardarDiagnosticos(ctx context.Context, tx pgx.Tx, id string, s dominio.SnapshotEpisodio) error {
	if _, err := tx.Exec(ctx, `DELETE FROM clinico.diagnosticos_cid WHERE episodio_id=$1`, id); err != nil {
		return fmt.Errorf("limpar diagnósticos: %w", err)
	}
	for _, d := range s.DiagnosticosCID {
		if _, err := tx.Exec(ctx,
			`INSERT INTO clinico.diagnosticos_cid (episodio_id, cid, principal) VALUES ($1,$2,$3)`,
			id, d.CID, d.Principal); err != nil {
			return fmt.Errorf("inserir diagnóstico: %w", err)
		}
	}
	return nil
}

// ObterPorID devolve o episódio com os diagnósticos. NaoEncontrado se não existir.
func (r *RepositorioEpisodios) ObterPorID(ctx context.Context, id string) (*dominio.EpisodioClinico, error) {
	const q = `
SELECT id::text, doente_id::text, tipo, especialidade_id::text, medico_id::text, inicio, fim,
       COALESCE(queixa_principal,''), COALESCE(historia_doenca,''), COALESCE(exame_objectivo,''),
       COALESCE(diagnostico,''), COALESCE(plano,''), estado, criado_em, actualizado_em,
       fechado_em, fechado_por::text
FROM clinico.episodios_clinicos WHERE id=$1`
	var s dominio.SnapshotEpisodio
	var tipo, estado string
	var queixa, historia, exame, diag, plano string
	var fechadoPor *string
	if err := r.pool.QueryRow(ctx, q, id).Scan(
		&s.ID, &s.DoenteID, &tipo, &s.EspecialidadeID, &s.MedicoID, &s.Inicio, &s.Fim,
		&queixa, &historia, &exame, &diag, &plano, &estado, &s.CriadoEm, &s.ActualizadoEm,
		&s.FechadoEm, &fechadoPor,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "episódio não encontrado")
		}
		return nil, fmt.Errorf("obter episódio: %w", err)
	}
	s.Tipo = dominio.TipoEpisodio(tipo)
	s.Estado = dominio.EstadoEpisodio(estado)
	s.Nota = dominio.NotaClinica{
		QueixaPrincipal: queixa, HistoriaDoenca: historia, ExameObjectivo: exame,
		Diagnostico: diag, Plano: plano,
	}
	s.FechadoPor = deref(fechadoPor)

	diags, err := r.carregarDiagnosticos(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	s.DiagnosticosCID = diags
	return dominio.ReconstruirEpisodio(s), nil
}

func (r *RepositorioEpisodios) carregarDiagnosticos(ctx context.Context, id string) ([]dominio.DiagnosticoCID, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT cid, principal FROM clinico.diagnosticos_cid WHERE episodio_id=$1 ORDER BY cid`, id)
	if err != nil {
		return nil, fmt.Errorf("carregar diagnósticos: %w", err)
	}
	defer linhas.Close()
	var out []dominio.DiagnosticoCID
	for linhas.Next() {
		var d dominio.DiagnosticoCID
		if err := linhas.Scan(&d.CID, &d.Principal); err != nil {
			return nil, fmt.Errorf("ler diagnóstico: %w", err)
		}
		out = append(out, d)
	}
	return out, linhas.Err()
}

// ListarPorDoente devolve uma página de episódios do doente, mais recentes primeiro.
func (r *RepositorioEpisodios) ListarPorDoente(ctx context.Context, f dominio.FiltroEpisodios) (dominio.PaginaEpisodios, error) {
	base := `FROM clinico.episodios_clinicos WHERE doente_id=$1 AND ($2='' OR estado=$2)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+base, f.DoenteID, f.Estado).Scan(&total); err != nil {
		return dominio.PaginaEpisodios{}, fmt.Errorf("contar episódios: %w", err)
	}
	q := `SELECT id::text, tipo, especialidade_id::text, medico_id::text, inicio, fim, estado ` +
		base + ` ORDER BY inicio DESC LIMIT $3 OFFSET $4`
	linhas, err := r.pool.Query(ctx, q, f.DoenteID, f.Estado, f.Limite, f.Deslocamento)
	if err != nil {
		return dominio.PaginaEpisodios{}, fmt.Errorf("listar episódios: %w", err)
	}
	defer linhas.Close()
	pagina := dominio.PaginaEpisodios{Total: total, Limite: f.Limite, Deslocamento: f.Deslocamento, Itens: []dominio.ResumoEpisodio{}}
	for linhas.Next() {
		var it dominio.ResumoEpisodio
		if err := linhas.Scan(&it.ID, &it.Tipo, &it.EspecialidadeID, &it.MedicoID, &it.Inicio, &it.Fim, &it.Estado); err != nil {
			return dominio.PaginaEpisodios{}, fmt.Errorf("ler resumo de episódio: %w", err)
		}
		pagina.Itens = append(pagina.Itens, it)
	}
	return pagina, linhas.Err()
}
```

- [ ] **Step 2: Escrever o teste de integração `tests/integration/episodios_test.go`**

```go
//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func TestRepositorioEpisodios_CicloCompleto(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	aplicarMigracoesTeste(t, pool, ctx)
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	repoEp := pgrepo.NovoRepositorioEpisodios(pool)

	// Cria um doente (FK).
	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	bi := "00123456LA042"
	num, _ := repoDoentes.ProximoNumeroProcesso(ctx, 2026)
	ident, _ := dominio.NovaIdentificacao("Ana Episódio", nasc, dominio.SexoFeminino, &bi, nil, nil)
	ct, _ := dominio.NovosContactos("+244923456789", nil, nil)
	doente, _ := dominio.NovoDoente(num, ident, ct, "AO")
	doenteID, err := repoDoentes.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar doente: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.episodios_clinicos WHERE doente_id=$1`, doenteID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})

	// Iniciar episódio (medico/especialidade são uuid — usa valores uuid válidos).
	const espID = "00000000-0000-4000-8000-000000000001"
	const medID = "00000000-0000-4000-8000-000000000002"
	ep, _ := dominio.NovoEpisodio(doenteID, dominio.EpisodioConsulta, espID, medID, time.Now())
	epID, err := repoEp.Guardar(ctx, ep)
	if err != nil {
		t.Fatalf("guardar episódio: %v", err)
	}

	// Preencher nota + CID e fechar.
	lido, err := repoEp.ObterPorID(ctx, epID)
	if err != nil {
		t.Fatalf("obter episódio: %v", err)
	}
	_ = lido.ActualizarNota(dominio.NovaNotaClinica("Febre", "", "Temp 39", "Gripe", "Repouso"))
	cid, _ := dominio.NovoDiagnosticoCID("J11", true)
	_ = lido.DefinirDiagnosticosCID([]dominio.DiagnosticoCID{cid})
	if err := lido.Fechar(medID, time.Now()); err != nil {
		t.Fatalf("fechar (domínio): %v", err)
	}
	if _, err := repoEp.Guardar(ctx, snapshotComID(lido, epID)); err != nil {
		t.Fatalf("guardar fecho: %v", err)
	}

	// Reler e confirmar fecho + CID persistido.
	final, err := repoEp.ObterPorID(ctx, epID)
	if err != nil || final.Estado() != dominio.EstadoEpisodioFechado {
		t.Fatalf("episódio não fechou: %v (estado=%v)", err, final.Estado())
	}
	if len(final.Snapshot().DiagnosticosCID) != 1 {
		t.Fatalf("diagnóstico não persistido: %d", len(final.Snapshot().DiagnosticosCID))
	}

	// Listar por doente.
	pag, err := repoEp.ListarPorDoente(ctx, dominio.FiltroEpisodios{DoenteID: doenteID, Limite: 10})
	if err != nil || pag.Total < 1 {
		t.Fatalf("listar falhou: %v (total=%d)", err, pag.Total)
	}
}

// snapshotComID reconstrói o episódio com o id atribuído pela BD.
func snapshotComID(e *dominio.EpisodioClinico, id string) *dominio.EpisodioClinico {
	s := e.Snapshot()
	s.ID = id
	return dominio.ReconstruirEpisodio(s)
}
```

> **Nota ao implementador:** o helper `aplicarMigracoesTeste(t, pool, ctx)` foi introduzido/reutilizado no teste de integração de doentes (Sprint 7, Task 10). Se existir no pacote `integration_test`, reutiliza-o; senão, chama directamente `db.AplicarMigracoes(ctx, pool, migrations.FS, logger)` (com um `slog` para stderr), como em `TestEditarPerfilAdmin_ViaBD`. `ligar(t)` já existe — não o redefinas.

- [ ] **Step 3: Compilar e correr**

Run: `go build ./...` → sem erros.
Run: `go vet ./internal/adapters/pgrepo/` → limpo.
Run: `go build -tags integration ./tests/integration/` → compila.
Run: `go test -tags integration ./tests/integration/ -run TestRepositorioEpisodios -v` → PASS (com BD) ou SKIP (sem `DATABASE_URL`).
Run: `gofmt -l internal/adapters/pgrepo/ tests/integration/` → vazio.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/pgrepo/episodios_repo.go tests/integration/episodios_test.go
git commit -m "feat(clinico): repositório pgx de episódios e teste de integração

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Handler HTTP de episódios (+ EHR) com RBAC e testes

**Files:**
- Create: `internal/adapters/http/episodio_handler.go`
- Test: `internal/adapters/http/episodio_test.go`

**Interfaces:**
- Consumes: casos de uso da aplicação (Tasks 4-6) via interfaces de serviço locais; `SessaoDe`, `RBAC`, `responderErro`, `i18n`, `erros`; `dominio "internal/domain/identidade"` (papéis + `Sessao`).
- Produces: `EpisodiosHandler`, `NovoEpisodiosHandler(...)`, `RegistarEpisodios(r gin.IRouter, h *EpisodiosHandler, protecao ...gin.HandlerFunc)`.

**Rotas e RBAC:**

| Rota | Método | Papéis |
|---|---|---|
| `/api/v1/doentes/:id/episodios` | POST (iniciar) | Médico, Enfermeiro |
| `/api/v1/doentes/:id/episodios` | GET (listar) | leitura clínica* |
| `/api/v1/doentes/:id/ehr` | GET | leitura clínica* |
| `/api/v1/episodios/:eid` | GET | leitura clínica* |
| `/api/v1/episodios/:eid` | PATCH | Médico, Enfermeiro |
| `/api/v1/episodios/:eid/fechar` | POST | **Médico** |
| `/api/v1/episodios/:eid/cancelar` | POST | **Médico** |

\* **leitura clínica** = Médico, Enfermeiro, Farmacêutico, TecnicoLab, Director, DPO, Auditor (exclui Administrativo).

**Datas:** `inicio` (iniciar) opcional, RFC 3339, parse com `time.Parse(time.RFC3339, ...)`; inválido → 400.

- [ ] **Step 1: Escrever o teste que falha (`episodio_test.go`)**

```go
package http_test

import (
	nethttp "net/http"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes dos serviços de episódio ---

type fakeIniciarEpisodio struct {
	out appclinico.DetalheEpisodio
	err error
}

func (f fakeIniciarEpisodio) Executar(_ ctxT, _ string, _ appclinico.DadosNovoEpisodio) (appclinico.DetalheEpisodio, error) {
	return f.out, f.err
}

type fakeObterEpisodio struct {
	out appclinico.DetalheEpisodio
	err error
}

func (f fakeObterEpisodio) Executar(_ ctxT, _, _ string) (appclinico.DetalheEpisodio, error) {
	return f.out, f.err
}

type fakeListarEpisodios struct {
	out appclinico.PaginaEpisodios
	err error
}

func (f fakeListarEpisodios) Executar(_ ctxT, _ string, _ appclinico.FiltroEpisodios) (appclinico.PaginaEpisodios, error) {
	return f.out, f.err
}

type fakeActualizarEpisodio struct {
	out appclinico.DetalheEpisodio
	err error
}

func (f fakeActualizarEpisodio) Executar(_ ctxT, _, _ string, _ appclinico.DadosActualizarEpisodio) (appclinico.DetalheEpisodio, error) {
	return f.out, f.err
}

type fakeFecharEpisodio struct {
	out appclinico.DetalheEpisodio
	err error
}

func (f fakeFecharEpisodio) Executar(_ ctxT, _, _ string) (appclinico.DetalheEpisodio, error) {
	return f.out, f.err
}

type fakeCancelarEpisodio struct {
	out appclinico.DetalheEpisodio
	err error
}

func (f fakeCancelarEpisodio) Executar(_ ctxT, _, _, _ string) (appclinico.DetalheEpisodio, error) {
	return f.out, f.err
}

type fakeObterEHR struct {
	out appclinico.EHR
	err error
}

func (f fakeObterEHR) Executar(_ ctxT, _, _ string, _ appclinico.FiltroEpisodios) (appclinico.EHR, error) {
	return f.out, f.err
}

func routerEpisodios(sessao dominio.Sessao) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoEpisodiosHandler(
		fakeIniciarEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1", Estado: "ABERTO"}},
		fakeObterEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1"}},
		fakeListarEpisodios{out: appclinico.PaginaEpisodios{Total: 0}},
		fakeActualizarEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1"}},
		fakeFecharEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1", Estado: "FECHADO"}},
		fakeCancelarEpisodio{out: appclinico.DetalheEpisodio{ID: "ep-1", Estado: "CANCELADO"}},
		fakeObterEHR{out: appclinico.EHR{}},
	)
	adhttp.RegistarEpisodios(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestEpisodios_Iniciar_MedicoPermitido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/episodios", `{"tipo":"CONSULTA","especialidade_id":"e1","medico_id":"m1"}`)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Iniciar_AdministrativoProibido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/episodios", `{"tipo":"CONSULTA","especialidade_id":"e1","medico_id":"m1"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestEpisodios_Iniciar_InicioInvalido_400(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/episodios", `{"tipo":"CONSULTA","especialidade_id":"e1","medico_id":"m1","inicio":"ontem"}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestEpisodios_Fechar_SoMedico(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	if w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep-1/fechar", ``); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
	r2 := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	if w := pedidoCorpo(r2, "POST", "/api/v1/episodios/ep-1/fechar", ``); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Enfermeiro não devia fechar: obtive %d", w.Code)
	}
}

func TestEpisodios_Actualizar_Clinicos(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "e1", Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedidoCorpo(r, "PATCH", "/api/v1/episodios/ep-1", `{"nota":{"queixa_principal":"Febre"}}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Cancelar_SoMedico(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep-1/cancelar", `{"motivo":"duplicado"}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Listar_LeituraClinica(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}})
	if w := pedido(r, "GET", "/api/v1/doentes/d1/episodios", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestEpisodios_EHR_AdministrativoProibido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	if w := pedido(r, "GET", "/api/v1/doentes/d1/ehr", "Bearer xyz"); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Administrativo não devia ler EHR: obtive %d", w.Code)
	}
}

func TestEpisodios_EHR_MedicoPermitido(t *testing.T) {
	r := routerEpisodios(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	if w := pedido(r, "GET", "/api/v1/doentes/d1/ehr", "Bearer xyz"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestEpisodios_Obter_ErroMapeado_404(t *testing.T) {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoEpisodiosHandler(
		fakeIniciarEpisodio{}, fakeObterEpisodio{err: erros.Novo(erros.CategoriaNaoEncontrado, "episódio não encontrado")},
		fakeListarEpisodios{}, fakeActualizarEpisodio{}, fakeFecharEpisodio{}, fakeCancelarEpisodio{}, fakeObterEHR{},
	)
	adhttp.RegistarEpisodios(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}))
	if w := pedido(r, "GET", "/api/v1/episodios/inexistente", "Bearer xyz"); w.Code != nethttp.StatusNotFound {
		t.Fatalf("esperava 404, obtive %d", w.Code)
	}
}
```

> **Nota ao implementador:** o alias `ctxT` acima é só para encurtar as assinaturas dos fakes no plano. No ficheiro real importa `"context"` e usa `context.Context` (remove `ctxT`).

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/http/ -run Episodios -v`
Expected: FAIL — `NovoEpisodiosHandler`/`RegistarEpisodios` indefinidos.

- [ ] **Step 3: Implementar `episodio_handler.go`**

```go
package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso de episódio (application/clinico).
type (
	// ServicoIniciarEpisodio inicia um episódio.
	ServicoIniciarEpisodio interface {
		Executar(ctx context.Context, actor string, dados appclinico.DadosNovoEpisodio) (appclinico.DetalheEpisodio, error)
	}
	// ServicoObterEpisodio devolve o detalhe de um episódio.
	ServicoObterEpisodio interface {
		Executar(ctx context.Context, actor, id string) (appclinico.DetalheEpisodio, error)
	}
	// ServicoListarEpisodios lista os episódios de um doente.
	ServicoListarEpisodios interface {
		Executar(ctx context.Context, doenteID string, filtro appclinico.FiltroEpisodios) (appclinico.PaginaEpisodios, error)
	}
	// ServicoActualizarEpisodio actualiza a nota/diagnósticos.
	ServicoActualizarEpisodio interface {
		Executar(ctx context.Context, actor, id string, dados appclinico.DadosActualizarEpisodio) (appclinico.DetalheEpisodio, error)
	}
	// ServicoFecharEpisodio fecha um episódio.
	ServicoFecharEpisodio interface {
		Executar(ctx context.Context, actor, id string) (appclinico.DetalheEpisodio, error)
	}
	// ServicoCancelarEpisodio cancela um episódio.
	ServicoCancelarEpisodio interface {
		Executar(ctx context.Context, actor, id, motivo string) (appclinico.DetalheEpisodio, error)
	}
	// ServicoObterEHR devolve a projecção EHR de um doente.
	ServicoObterEHR interface {
		Executar(ctx context.Context, actor, doenteID string, filtro appclinico.FiltroEpisodios) (appclinico.EHR, error)
	}
)

// EpisodiosHandler expõe os endpoints HTTP do episódio clínico e do EHR.
type EpisodiosHandler struct {
	iniciar    ServicoIniciarEpisodio
	obter      ServicoObterEpisodio
	listar     ServicoListarEpisodios
	actualizar ServicoActualizarEpisodio
	fechar     ServicoFecharEpisodio
	cancelar   ServicoCancelarEpisodio
	ehr        ServicoObterEHR
}

// NovoEpisodiosHandler constrói o handler com os casos de uso.
func NovoEpisodiosHandler(
	iniciar ServicoIniciarEpisodio,
	obter ServicoObterEpisodio,
	listar ServicoListarEpisodios,
	actualizar ServicoActualizarEpisodio,
	fechar ServicoFecharEpisodio,
	cancelar ServicoCancelarEpisodio,
	ehr ServicoObterEHR,
) *EpisodiosHandler {
	return &EpisodiosHandler{
		iniciar: iniciar, obter: obter, listar: listar, actualizar: actualizar,
		fechar: fechar, cancelar: cancelar, ehr: ehr,
	}
}

// RegistarEpisodios regista as rotas de episódio e EHR, aplicando `protecao`
// (rate limit + Auth) e o RBAC por rota.
func RegistarEpisodios(r gin.IRouter, h *EpisodiosHandler, protecao ...gin.HandlerFunc) {
	leituraClinica := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelFarmaceutico,
		dominio.PapelTecnicoLab, dominio.PapelDirector, dominio.PapelDPO, dominio.PapelAuditor)
	clinicos := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro)
	soMedico := RBAC(dominio.PapelMedico)

	gd := r.Group("/api/v1/doentes")
	gd.Use(protecao...)
	gd.POST("/:id/episodios", clinicos, h.iniciarEpisodio)
	gd.GET("/:id/episodios", leituraClinica, h.listarEpisodios)
	gd.GET("/:id/ehr", leituraClinica, h.obterEHR)

	ge := r.Group("/api/v1/episodios")
	ge.Use(protecao...)
	ge.GET("/:eid", leituraClinica, h.obterEpisodio)
	ge.PATCH("/:eid", clinicos, h.actualizarEpisodio)
	ge.POST("/:eid/fechar", soMedico, h.fecharEpisodio)
	ge.POST("/:eid/cancelar", soMedico, h.cancelarEpisodio)
}

type corpoIniciarEpisodio struct {
	Tipo            string  `json:"tipo"`
	EspecialidadeID string  `json:"especialidade_id"`
	MedicoID        string  `json:"medico_id"`
	Inicio          *string `json:"inicio"` // RFC 3339 opcional
}

func (h *EpisodiosHandler) iniciarEpisodio(c *gin.Context) {
	var corpo corpoIniciarEpisodio
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	var inicio *time.Time
	if corpo.Inicio != nil && *corpo.Inicio != "" {
		t, err := time.Parse(time.RFC3339, *corpo.Inicio)
		if err != nil {
			responderErro(c, erros.Novo(erros.CategoriaValidacao, "início inválido (formato esperado RFC 3339)"))
			return
		}
		inicio = &t
	}
	actor, _ := SessaoDe(c)
	out, err := h.iniciar.Executar(c.Request.Context(), actor.Sujeito, appclinico.DadosNovoEpisodio{
		DoenteID: c.Param("id"), Tipo: corpo.Tipo, EspecialidadeID: corpo.EspecialidadeID,
		MedicoID: corpo.MedicoID, Inicio: inicio,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *EpisodiosHandler) listarEpisodios(c *gin.Context) {
	filtro := appclinico.FiltroEpisodios{
		Estado:       c.Query("estado"),
		Limite:       inteiroQuery(c, "limite"),
		Deslocamento: inteiroQuery(c, "deslocamento"),
	}
	out, err := h.listar.Executar(c.Request.Context(), c.Param("id"), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *EpisodiosHandler) obterEHR(c *gin.Context) {
	actor, _ := SessaoDe(c)
	filtro := appclinico.FiltroEpisodios{
		Estado:       c.Query("estado"),
		Limite:       inteiroQuery(c, "limite"),
		Deslocamento: inteiroQuery(c, "deslocamento"),
	}
	out, err := h.ehr.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *EpisodiosHandler) obterEpisodio(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.obter.Executar(c.Request.Context(), actor.Sujeito, c.Param("eid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoActualizarEpisodio struct {
	Nota *struct {
		QueixaPrincipal string `json:"queixa_principal"`
		HistoriaDoenca  string `json:"historia_doenca"`
		ExameObjectivo  string `json:"exame_objectivo"`
		Diagnostico     string `json:"diagnostico"`
		Plano           string `json:"plano"`
	} `json:"nota"`
	DiagnosticosCID *[]struct {
		CID       string `json:"cid"`
		Principal bool   `json:"principal"`
	} `json:"diagnosticos_cid"`
}

func (h *EpisodiosHandler) actualizarEpisodio(c *gin.Context) {
	var corpo corpoActualizarEpisodio
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	var dados appclinico.DadosActualizarEpisodio
	if corpo.Nota != nil {
		dados.Nota = &appclinico.DadosNotaClinica{
			QueixaPrincipal: corpo.Nota.QueixaPrincipal, HistoriaDoenca: corpo.Nota.HistoriaDoenca,
			ExameObjectivo: corpo.Nota.ExameObjectivo, Diagnostico: corpo.Nota.Diagnostico, Plano: corpo.Nota.Plano,
		}
	}
	if corpo.DiagnosticosCID != nil {
		lista := make([]appclinico.DadosDiagnosticoCID, 0, len(*corpo.DiagnosticosCID))
		for _, d := range *corpo.DiagnosticosCID {
			lista = append(lista, appclinico.DadosDiagnosticoCID{CID: d.CID, Principal: d.Principal})
		}
		dados.DiagnosticosCID = &lista
	}
	actor, _ := SessaoDe(c)
	out, err := h.actualizar.Executar(c.Request.Context(), actor.Sujeito, c.Param("eid"), dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *EpisodiosHandler) fecharEpisodio(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.fechar.Executar(c.Request.Context(), actor.Sujeito, c.Param("eid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoCancelarEpisodio struct {
	Motivo string `json:"motivo"`
}

func (h *EpisodiosHandler) cancelarEpisodio(c *gin.Context) {
	var corpo corpoCancelarEpisodio
	// O corpo é opcional; um motivo em falta é aceitável.
	_ = c.ShouldBindJSON(&corpo)
	actor, _ := SessaoDe(c)
	out, err := h.cancelar.Executar(c.Request.Context(), actor.Sujeito, c.Param("eid"), corpo.Motivo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
```

> **Nota:** `inteiroQuery(c, chave)` já existe em `doente_handler.go` (mesmo pacote `http`) — reutiliza, não redeclares.

- [ ] **Step 4: Correr os testes e a cobertura dos adaptadores**

Run: `go test ./internal/adapters/http/ -v` → PASS (todos, incl. pré-existentes).
Run: `bash scripts/cobertura.sh` (adaptadores ≥60%). Se abaixo, acrescenta casos ao `episodio_test.go` (ex.: actualizar com diagnósticos → 200; listar com erro 500; obter EHR 200 com Director; cancelar sem corpo → 200).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/episodio_handler.go internal/adapters/http/episodio_test.go
git commit -m "feat(clinico): handler HTTP de episódios e EHR com RBAC clínico

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Wiring no composition root e ADR-027

**Files:**
- Modify: `internal/platform/app.go`
- Create: `adrs/ADR-027-bc-clinico-episodio.md`

**Interfaces:**
- Consumes: `pgrepo.NovoRepositorioEpisodios` (Task 7), casos de uso `application/clinico` de episódio (Tasks 4-6), `adhttp.NovoEpisodiosHandler`/`RegistarEpisodios` (Task 8); `pool`, `repoDoentes`, `repoAuditoria`, `limiteMW`, `authMW` já existentes em `app.go`.

**Contexto:** `ExecutarServidor` já constrói `pool`, `repoDoentes := pgrepo.NovoRepositorioDoentes(pool)`, `repoAuditoria`, `limiteMW`, `authMW`, e o closure `registarRotas` (que já chama `RegistarDoentes`). Acrescentar o episódio com **`limiteMW` + `authMW`** (sem MFA, como o handler de doentes).

- [ ] **Step 1: Acrescentar a construção do handler de episódios**

Em `internal/platform/app.go`, a seguir ao bloco que constrói `handlerDoentes := adhttp.NovoDoentesHandler(...)`, acrescentar:

```go
	// BC Clínico: episódios e EHR.
	repoEpisodios := pgrepo.NovoRepositorioEpisodios(pool)
	handlerEpisodios := adhttp.NovoEpisodiosHandler(
		appclinico.NovoCasoIniciarEpisodio(repoEpisodios, repoDoentes, repoAuditoria),
		appclinico.NovoCasoObterEpisodio(repoEpisodios, repoAuditoria),
		appclinico.NovoCasoListarEpisodios(repoEpisodios),
		appclinico.NovoCasoActualizarEpisodio(repoEpisodios, repoAuditoria),
		appclinico.NovoCasoFecharEpisodio(repoEpisodios, repoAuditoria),
		appclinico.NovoCasoCancelarEpisodio(repoEpisodios, repoAuditoria),
		appclinico.NovoCasoObterEHR(repoDoentes, repoEpisodios, repoAuditoria),
	)
```

> `appclinico` já está importado (Sprint 7). `repoDoentes` já existe. Se o nome da variável do repositório de doentes no `app.go` for diferente, usa o existente (confirma lendo o ficheiro).

- [ ] **Step 2: Registar as rotas no closure `registarRotas`**

Acrescentar a linha ao closure (a seguir a `adhttp.RegistarDoentes(...)`):

```go
		adhttp.RegistarEpisodios(r, handlerEpisodios, limiteMW, authMW)
```

- [ ] **Step 3: Compilar e correr a suite completa**

Run: `go build ./...` → sem erros.
Run: `go build -tags integration ./...` → compila.
Run: `go test ./...` → PASS.
Run: `bash scripts/cobertura.sh` → domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
Run: `gofmt -l internal/platform/` → vazio. `go vet ./...` → limpo.

- [ ] **Step 4: Escrever `adrs/ADR-027-bc-clinico-episodio.md`**

```markdown
# ADR-027 — BC Clínico: agregado Episódio Clínico

- **Estado:** Aceite
- **Data:** 2026-07-12
- **Marco:** M2 — Clínico Core (Sprint 8)
- **Contexto de spec:** docs/superpowers/specs/2026-07-12-sprint8-clinico-episodio-design.md

## Contexto

O Sprint 8 acrescenta o segundo agregado do BC Clínico — o EpisodioClinico —
cobrindo o ciclo de vida clínico e uma projecção de leitura EHR. O modelo de dados
foi extraído verbatim do DDM-001 v2.0.

## Decisões

1. **Agregado raiz independente.** Apesar de o DDM lhe chamar "sub-agregado" do
   Doente, o EpisodioClinico é um agregado raiz próprio, com repositório próprio,
   referenciando `doente_id`. Motivos: os episódios crescem sem limite; têm ciclo
   de vida próprio; a FK `doente_id` é sem `ON DELETE CASCADE` (sobrevivem à
   pseudonimização do doente).

2. **EHR como projecção de leitura.** O EHR não é entidade — é montado em runtime
   combinando o doente (com alergias/antecedentes) e os episódios paginados.

3. **`especialidade_id` opaco.** Guardado como identificador (uuid) fornecido pelo
   chamador, sem FK — o módulo Admin/Especialidades não existe ainda.

4. **`medico_id` no pedido.** O médico responsável é indicado no corpo de iniciar
   (o disparo por Agendamento — UC-DOE-06 — fica para quando esse módulo existir).

5. **Fecho exige nota completa + ≥1 CID.** Fechar um episódio requer queixa, exame,
   diagnóstico e plano preenchidos, e pelo menos um diagnóstico CID codificado.
   Nota e diagnósticos só são editáveis enquanto ABERTO.

6. **RBAC.** Iniciar/actualizar: Médico + Enfermeiro. Fechar/cancelar: só Médico
   (o diagnóstico é acto médico). Leitura de episódios/EHR: leitura clínica
   (exclui Administrativo, que vê a demografia mas não as notas clínicas).

7. **Auditoria.** Escrita e consulta individual (episódio, EHR) auditadas; a
   listagem não é auditada (evita ruído).

## Diferimentos

- **RN-DOE-05:** episódio ABERTO bloquear a actualização de nome/BI do doente.
- **RN-DOE-03:** acesso ao EHR exigir relação clínica activa (depende de Agendamento).
- **Prescrições** (Sprint 9) e **requisições de laboratório** (módulo Laboratório).
- **UC-DOE-08:** declarar óbito cancelar os episódios ABERTOS (interacção
  Doente↔Episódio) — quando a fatia de óbito/LPDP consolidada for feita.

## Consequências

- Base para a Facturação (consome `clinico.episodio.fechado`) e o Laboratório
  (consome `clinico.episodio.aberto`) em marcos futuros.
- Tal como no agregado Doente, os diagnósticos CID são persistidos por
  delete-and-reinsert em cada `Guardar` (regenera nada — a PK é natural
  (episodio_id, cid) — mas reescreve o conjunto).
```

- [ ] **Step 5: Commits**

```bash
git add internal/platform/app.go
git commit -m "feat(clinico): liga episódios e EHR ao composition root

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"

git add adrs/ADR-027-bc-clinico-episodio.md
git commit -m "docs(clinico): ADR-027 com as decisões do episódio clínico

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Verificação final (fim a fim)

1. `go build ./...` e `go build -tags integration ./...` — sem erros.
2. `go test ./...` — PASS.
3. `bash scripts/cobertura.sh` — domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
4. `make lint` — sem violações; `domain/clinico` não importa `pgx`/`gin`/`uuid`.
5. `gofmt -l internal/ migrations/ tests/` — vazio.
6. Migration `clinico/0002` aplica-se; `schema_migrations` regista `clinico/0002_episodios`.
7. `go test -tags integration ./tests/integration/ -run TestRepositorioEpisodios` — PASS (com BD) ou SKIP.
8. Fluxo HTTP (token de Médico): iniciar → actualizar (nota+CID) → fechar → listar → EHR; fechar como Enfermeiro → 403; EHR como Administrativo → 403.

## Fora de âmbito (fatias futuras)

- RN-DOE-05, RN-DOE-03, prescrições, requisições-lab, integração Agendamento, óbito-cancela-episódios (ver ADR-027).
