# Sprint 9 — BC Farmácia: Medicamento + Receita — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar o BC Farmácia — catálogo de Medicamentos e Receita/Prescrição (emitir de um episódio, com validação de alergias e override auditado) — do domínio ao HTTP.

**Architecture:** DDD + Clean Architecture. Novo BC `farmacia` (domínio/aplicação/adapters), schema `farmacia`. Dois agregados raiz: Medicamento (catálogo) e Receita (com itens). IDs `string` gerados pela BD. A validação de alergias cruza a fronteira Clínico↔Farmácia por uma porta anti-corrupção `LeitorClinico`, implementada por um adaptador que reutiliza os repositórios `clinico`. Nova categoria de erro `RegraNegocio` (422).

**Tech Stack:** Go 1.22+, Gin, pgx v5 (SQL puro), PostgreSQL 16 (pg_trgm, SEQUENCE); testes `go test` com fakes; integração com tag `integration`.

## Global Constraints

- **Linguagem ubíqua PT-PT angolano** em TODO o output: código, comentários, mensagens de erro, JSON, commits. Nunca inglês nem PT-BR.
- **Domínio puro:** `internal/domain/**` só stdlib + Shared Kernel. Zero `pgx`/`gin`/`net/http`/`google/uuid`. O domínio `farmacia` **não** importa o domínio `clinico`. `google/uuid` também proibido na aplicação.
- **Sem `panic()`** fora de init. **Migrations forward-only**.
- **Erros de domínio** via `erros.Novo(categoria, mensagem)` com mensagem PT-PT **literal**. Categorias usadas: `CategoriaValidacao`, `CategoriaNaoEncontrado`, `CategoriaConflito`, e a **nova** `CategoriaRegraNegocio` (Task 1 → 422).
- **Erros HTTP** via `responderErro(c, err)` (RFC 7807, PT-PT) — `internal/adapters/http/problem.go`.
- **Modelo de dados** extraído verbatim do DDM-001 (ver Task 2). Não inventar colunas.
- **IDs** do domínio são `string`, gerados pela BD (`gen_random_uuid()` + `RETURNING id::text`). `codigo_interno` = `MED-{sequencial:05d}` via `SEQUENCE farmacia.seq_codigo_medicamento`.
- **Auditoria:** escrita e consulta individual de **receitas** auditadas (dados de prescrição são de saúde). Acções: `farmacia.medicamento.registado`, `farmacia.medicamento.actualizado`, `farmacia.medicamento.activado`, `farmacia.medicamento.desactivado`, `farmacia.receita.emitida`, `farmacia.receita.anulada`, `farmacia.receita.consultada`. Leituras do **catálogo** (obter/pesquisar medicamentos) e a listagem de receitas **não** auditam.
- **Nunca registar em log** dados de saúde/identificadores.
- **Cobertura** (`bash scripts/cobertura.sh`): domínio ≥85%, aplicação ≥75%, adaptadores ≥60%. O gate corre **sem** a tag `integration`. Confirmar sempre no relatório com a execução real de `bash scripts/cobertura.sh` (e `staticcheck ./...` se disponível — funções de pacote não usadas falham a CI).
- **Commits** Conventional Commits em PT-PT, a terminar com:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- **Branch de trabalho:** `m2-sprint9-farmacia-receita` (já criado; a spec já lá está commitada).

### Convenções do projecto (M2 — seguir tal como estão)

- Agregado com campos privados + factory validante + `Snapshot()`/`ReconstruirX(SnapshotX)` (ver `internal/domain/clinico/doente.go`, `episodio.go`). Entidades-filho como VOs de campos **exportados** com construtor validante (ver `internal/domain/clinico/alergia.go`, `diagnostico_cid.go`).
- Enums `type X string` + consts + `ParseX` quando aplicável (ver `internal/domain/clinico/grupo_sanguineo.go`).
- Read-models (`ResumoX`/`PaginaX`/`FiltroX`) na interface do repositório, no domínio, com tags JSON nos read-models.
- Casos de uso: struct com dependências por construtor `NovoCaso...`, relógio `agora func() time.Time` (default `time.Now`), método `Executar(ctx, ...)`; auditar **só após** persistir; re-ler via `ObterPorID` para devolver o detalhe (ver `internal/application/clinico/*.go`).
- Repositório pgx: `pool *pgxpool.Pool`, transacção com `defer tx.Rollback`, `errors.Is(err, pgx.ErrNoRows)` → `CategoriaNaoEncontrado`, filhos por delete-and-reinsert, `NULLIF(...,'')` na escrita e `COALESCE(...,'')` na leitura de texto opcional; violação de unicidade (`*pgconn.PgError` code `23505`) → `CategoriaConflito`. Helper `deref(*string) string` já existe no pacote `pgrepo`. Ver `internal/adapters/pgrepo/doentes_repo.go` e `episodios_repo.go`.
- Handler: struct + interfaces de serviço + `RegistarX(r gin.IRouter, h, protecao ...gin.HandlerFunc)`, RBAC por rota via `RBAC(dominio.PapelX, ...)` (`dominio` = `internal/domain/identidade`), `actor` via `SessaoDe(c).Sujeito`, erros via `responderErro`, bind inválido → `erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido))`. Helper `inteiroQuery(c, chave)` já existe. Ver `internal/adapters/http/doente_handler.go`, `episodio_handler.go`.
- Papéis (de `internal/domain/identidade`): `PapelMedico`, `PapelEnfermeiro`, `PapelFarmaceutico`, `PapelFarmaceuticoSenior`, `PapelTecnicoLab`, `PapelDirector`, `PapelDPO`, `PapelAuditor`, `PapelAdministrativo`.
- Testes de aplicação em `package farmacia_test`; testes de handler em `package http_test` (reutiliza `novoRouter()`, `pedido`, `pedidoCorpo`, `fakeAuth`).

---

### Task 1: Categoria de erro RegraNegocio (422) no Shared Kernel

**Files:**
- Modify: `internal/domain/shared/erros/erros.go` (acrescentar a categoria)
- Modify: `internal/domain/shared/i18n/i18n.go` (acrescentar a chave/mensagem)
- Modify: `internal/adapters/http/problem.go` (mapear → 422)
- Test: `internal/domain/shared/erros/erros_test.go` (acrescentar caso)
- Test: `internal/adapters/http/problem_regranegocio_test.go` (novo, package `http`)

**Interfaces:**
- Consumes: nada novo.
- Produces: `erros.CategoriaRegraNegocio`; `i18n.MsgRegraNegocio`; mapeamento HTTP 422.

**Contexto:** as categorias são um `iota` em `erros.go`. Acrescenta-se `CategoriaRegraNegocio` **no fim** do bloco const (após `CategoriaInterno`) para não deslocar os valores existentes. O `problem.go` mapeia categoria→estado/título/type.

- [ ] **Step 1: Escrever os testes que falham**

Acrescentar a `internal/domain/shared/erros/erros_test.go`:
```go
func TestCategoriaRegraNegocio_RoundTrip(t *testing.T) {
	err := erros.Novo(erros.CategoriaRegraNegocio, "regra violada")
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava CategoriaRegraNegocio, obtive %v", erros.CategoriaDe(err))
	}
}
```
> **Nota:** confirma o nome do pacote de teste em `erros_test.go` (interno `package erros` ou externo `package erros_test`); usa o mesmo. Se for externo, o `erros.` acima está correcto; se for interno, remove o prefixo `erros.`.

Criar `internal/adapters/http/problem_regranegocio_test.go`:
```go
package http

import (
	nethttp "net/http"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestEstadoDe_RegraNegocio_422(t *testing.T) {
	if got := estadoDe(erros.CategoriaRegraNegocio); got != nethttp.StatusUnprocessableEntity {
		t.Fatalf("estadoDe(RegraNegocio)=%d, esperava 422", got)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falham**

Run: `go test ./internal/domain/shared/erros/ ./internal/adapters/http/ -run 'RegraNegocio' -v`
Expected: FAIL — `CategoriaRegraNegocio` indefinida.

- [ ] **Step 3: Acrescentar a categoria em `erros.go`**

No bloco `const (...)` de `internal/domain/shared/erros/erros.go`, acrescenta a seguir a
`CategoriaInterno`:
```go
	// CategoriaInterno — falha inesperada (→ 500).
	CategoriaInterno
	// CategoriaRegraNegocio — violação de regra de negócio (→ 422).
	CategoriaRegraNegocio
)
```
(Não alteres a ordem das existentes.)

- [ ] **Step 4: Acrescentar a mensagem em `i18n.go`**

Em `internal/domain/shared/i18n/i18n.go`, acrescenta a chave ao bloco const de `Chave`:
```go
	// MsgRegraNegocio — violação de regra de negócio (422).
	MsgRegraNegocio Chave = "erro.regra_negocio"
```
e a entrada ao mapa `mensagensPtAO`:
```go
	MsgRegraNegocio: "Não foi possível processar o pedido por violar uma regra de negócio.",
```

- [ ] **Step 5: Mapear em `problem.go`**

Em `internal/adapters/http/problem.go`, acrescenta os casos:
- em `estadoDe`: `case erros.CategoriaRegraNegocio: return nethttp.StatusUnprocessableEntity`
- em `tituloDe`: `case erros.CategoriaRegraNegocio: return i18n.T(i18n.MsgRegraNegocio)`
(o `tipoDe` mantém-se `about:blank` para esta categoria — não precisa de caso.)

- [ ] **Step 6: Correr os testes e confirmar que passam**

Run: `go test ./internal/domain/shared/erros/ ./internal/adapters/http/ -v`
Expected: PASS (todos). `go build ./...` OK; `gofmt -l internal/` vazio.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/shared/erros/erros.go internal/domain/shared/erros/erros_test.go internal/domain/shared/i18n/i18n.go internal/adapters/http/problem.go internal/adapters/http/problem_regranegocio_test.go
git commit -m "feat(shared): categoria de erro RegraNegocio mapeada para 422

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Migration do BC Farmácia e registo no embed

**Files:**
- Create: `migrations/farmacia/0001_medicamentos_receitas.sql`
- Modify: `migrations/embed.go` (directiva `//go:embed`)
- Modify: `migrations/embed_test.go` (lista de ficheiros esperados)

**Interfaces:**
- Consumes: runner de migrations (descobre bounded contexts por subdirectório).
- Produces: schema `farmacia` com `medicamentos`, `receitas`, `itens_receita`, SEQUENCE e índices.

**Contexto:** o schema `farmacia` já é criado por `docker/postgres/init.sql`, mas a migration usa `CREATE ... IF NOT EXISTS` para ser auto-suficiente. O `//go:embed` actual é `auditoria clinico identidade shared` — passa a incluir `farmacia` em ordem alfabética.

- [ ] **Step 1: Criar `migrations/farmacia/0001_medicamentos_receitas.sql`**

```sql
-- Bounded Context: farmacia
-- Migration forward-only. Esquema extraído verbatim do DDM-001 v2.0.
--
-- Catálogo de medicamentos e receitas/prescrições. As tabelas de stock
-- (lotes, fornecedores, movimentos_stock) ficam para a fatia de stock/dispensa.

CREATE SCHEMA IF NOT EXISTS farmacia;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE SEQUENCE IF NOT EXISTS farmacia.seq_codigo_medicamento;

CREATE TABLE IF NOT EXISTS farmacia.medicamentos (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo_interno     text        NOT NULL UNIQUE,
    nome_comercial     text        NOT NULL,
    nome_generico      text        NOT NULL,
    forma_farmaceutica text        NOT NULL,
    dosagem            text        NOT NULL,
    via_administracao  text        NOT NULL,
    fabricante         text,
    requer_receita     boolean     NOT NULL DEFAULT true,
    psicotropico       boolean     NOT NULL DEFAULT false,
    classe_atc         text,
    stock_minimo       integer     NOT NULL DEFAULT 10 CHECK (stock_minimo >= 0),
    activo             boolean     NOT NULL DEFAULT true,
    criado_em          timestamptz NOT NULL DEFAULT now(),
    actualizado_em     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_medicamentos_nome
    ON farmacia.medicamentos USING gin ((nome_comercial || ' ' || nome_generico) gin_trgm_ops);

COMMENT ON TABLE farmacia.medicamentos IS
    'Catálogo de medicamentos. codigo_interno gerado por SEQUENCE (MED-NNNNN).';

CREATE TABLE IF NOT EXISTS farmacia.receitas (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    episodio_id  uuid        NOT NULL,
    doente_id    uuid        NOT NULL,
    medico_id    uuid        NOT NULL,
    emitida_em   timestamptz NOT NULL DEFAULT now(),
    estado       text        NOT NULL DEFAULT 'EMITIDA'
                 CHECK (estado IN ('EMITIDA','PARCIAL','DISPENSADA','EXPIRADA','ANULADA')),
    notas        text,
    expira_em    date        NOT NULL DEFAULT (CURRENT_DATE + INTERVAL '30 days')
);
CREATE INDEX IF NOT EXISTS idx_receitas_doente ON farmacia.receitas (doente_id, emitida_em DESC);
CREATE INDEX IF NOT EXISTS idx_receitas_episodio ON farmacia.receitas (episodio_id);

COMMENT ON TABLE farmacia.receitas IS
    'Receita/prescrição. episodio_id/doente_id referenciam o BC Clínico por id (sem FK cross-schema).';

CREATE TABLE IF NOT EXISTS farmacia.itens_receita (
    id                    uuid    PRIMARY KEY DEFAULT gen_random_uuid(),
    receita_id            uuid    NOT NULL REFERENCES farmacia.receitas(id) ON DELETE CASCADE,
    medicamento_id        uuid    NOT NULL REFERENCES farmacia.medicamentos(id),
    posologia             text    NOT NULL,
    duracao_dias          integer,
    quantidade_prescrita  integer NOT NULL CHECK (quantidade_prescrita > 0),
    quantidade_dispensada integer NOT NULL DEFAULT 0,
    notas                 text
);
CREATE INDEX IF NOT EXISTS idx_itens_receita_receita ON farmacia.itens_receita (receita_id);
```

- [ ] **Step 2: Actualizar `migrations/embed.go`**

Alterar a directiva `//go:embed` para incluir `farmacia` em ordem alfabética:
```go
//go:embed auditoria clinico farmacia identidade shared
var FS embed.FS
```

- [ ] **Step 3: Actualizar `migrations/embed_test.go`**

Se `embed_test.go` tiver uma lista fixa de ficheiros esperados, acrescenta
`"farmacia/0001_medicamentos_receitas.sql"` na posição alfabeticamente correcta (entre
`clinico/...` e `identidade/...`). Confirma primeiro lendo o ficheiro.

- [ ] **Step 4: Compilar e correr o teste do embed**

Run: `go test ./migrations/ -v`
Expected: PASS.
Run: `go build ./...`
Expected: sem erros.

- [ ] **Step 5: Commit**

```bash
git add migrations/farmacia/0001_medicamentos_receitas.sql migrations/embed.go migrations/embed_test.go
git commit -m "feat(farmacia): migration do catálogo de medicamentos e receitas

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Domínio — agregado Medicamento e porta de repositório

**Files:**
- Create: `internal/domain/farmacia/medicamento.go`
- Create: `internal/domain/farmacia/repositorio_medicamentos.go`
- Test: `internal/domain/farmacia/medicamento_test.go`

**Interfaces:**
- Consumes: `erros.Novo` (Shared Kernel).
- Produces:
  - `type Medicamento struct {...}` (campos privados) + getters `ID()`, `CodigoInterno()`, `Activo()`.
  - `NovoMedicamento(codigoInterno, nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante string, requerReceita, psicotropico bool, classeATC *string, stockMinimo int) (*Medicamento, error)`.
  - `Actualizar(nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante string, requerReceita, psicotropico bool, classeATC *string, stockMinimo int) error`; `Activar()`; `Desactivar()`; `CorrespondeSubstancia(substancia string) bool`.
  - `type SnapshotMedicamento struct {...}` (campos exportados); `Snapshot() SnapshotMedicamento`; `ReconstruirMedicamento(SnapshotMedicamento) *Medicamento`.
  - `type FiltroMedicamentos`, `ResumoMedicamento`, `PaginaMedicamentos`, `RepositorioMedicamentos`.

- [ ] **Step 1: Escrever o teste que falha (`medicamento_test.go`)**

```go
package farmacia_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func medicamentoValido(t *testing.T) *farmacia.Medicamento {
	t.Helper()
	m, err := farmacia.NovoMedicamento("MED-00001", "Amoxil 500mg", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "GSK", true, false, nil, 10)
	if err != nil {
		t.Fatalf("NovoMedicamento: %v", err)
	}
	return m
}

func TestNovoMedicamento_Valido(t *testing.T) {
	m := medicamentoValido(t)
	if m.CodigoInterno() != "MED-00001" || !m.Activo() {
		t.Fatalf("medicamento inesperado: %+v", m.Snapshot())
	}
}

func TestNovoMedicamento_CamposObrigatorios(t *testing.T) {
	if _, err := farmacia.NovoMedicamento("MED-1", "", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, 10); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para nome comercial vazio")
	}
	if _, err := farmacia.NovoMedicamento("MED-1", "Amoxil", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, -1); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para stock mínimo negativo")
	}
}

func TestCorrespondeSubstancia(t *testing.T) {
	m := medicamentoValido(t)
	if !m.CorrespondeSubstancia("amoxicilina") {
		t.Fatal("esperava correspondência com o nome genérico (case-insensitive)")
	}
	if !m.CorrespondeSubstancia("AMOXIL") {
		t.Fatal("esperava correspondência com o nome comercial")
	}
	if m.CorrespondeSubstancia("Penicilina") {
		t.Fatal("não devia corresponder a substância ausente")
	}
}

func TestMedicamento_ActivarDesactivar(t *testing.T) {
	m := medicamentoValido(t)
	m.Desactivar()
	if m.Activo() {
		t.Fatal("esperava inactivo após Desactivar")
	}
	m.Activar()
	if !m.Activo() {
		t.Fatal("esperava activo após Activar")
	}
}

func TestReconstruirMedicamento_PreservaEstado(t *testing.T) {
	orig := medicamentoValido(t)
	orig.Desactivar()
	snap := orig.Snapshot()
	snap.ID = "id-1"
	rec := farmacia.ReconstruirMedicamento(snap)
	if rec.ID() != "id-1" || rec.Activo() {
		t.Fatalf("rehidratação perdeu estado: id=%q activo=%v", rec.ID(), rec.Activo())
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/farmacia/ -v`
Expected: FAIL — pacote/símbolos inexistentes.

- [ ] **Step 3: Implementar `medicamento.go`**

```go
// Package farmacia é o domínio do Bounded Context Farmácia do SGC Angola: o
// catálogo de medicamentos e as receitas/prescrições. Camada 1 (Domínio):
// importa apenas a biblioteca-padrão e o Shared Kernel — sem infra e sem o
// domínio de outros bounded contexts.
package farmacia

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Medicamento é o agregado raiz do catálogo farmacêutico.
type Medicamento struct {
	id                string
	codigoInterno     string
	nomeComercial     string
	nomeGenerico      string
	formaFarmaceutica string
	dosagem           string
	viaAdministracao  string
	fabricante        string
	requerReceita     bool
	psicotropico      bool
	classeATC         *string
	stockMinimo       int
	activo            bool
	criadoEm          time.Time
	actualizadoEm     time.Time
}

// NovoMedicamento valida e constrói um medicamento activo. codigoInterno, nome
// comercial/genérico, forma, dosagem e via são obrigatórios; stockMinimo ≥ 0.
func NovoMedicamento(codigoInterno, nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante string, requerReceita, psicotropico bool, classeATC *string, stockMinimo int) (*Medicamento, error) {
	m := &Medicamento{
		id:            "",
		codigoInterno: strings.TrimSpace(codigoInterno),
		activo:        true,
	}
	if m.codigoInterno == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "código interno do medicamento em falta")
	}
	if err := m.aplicarCampos(nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante, requerReceita, psicotropico, classeATC, stockMinimo); err != nil {
		return nil, err
	}
	return m, nil
}

// Actualizar revalida e substitui os campos mutáveis do medicamento.
func (m *Medicamento) Actualizar(nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante string, requerReceita, psicotropico bool, classeATC *string, stockMinimo int) error {
	return m.aplicarCampos(nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante, requerReceita, psicotropico, classeATC, stockMinimo)
}

func (m *Medicamento) aplicarCampos(nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante string, requerReceita, psicotropico bool, classeATC *string, stockMinimo int) error {
	nomeComercial = strings.TrimSpace(nomeComercial)
	nomeGenerico = strings.TrimSpace(nomeGenerico)
	formaFarmaceutica = strings.TrimSpace(formaFarmaceutica)
	dosagem = strings.TrimSpace(dosagem)
	viaAdministracao = strings.TrimSpace(viaAdministracao)
	if nomeComercial == "" || nomeGenerico == "" {
		return erros.Novo(erros.CategoriaValidacao, "nome comercial e genérico do medicamento são obrigatórios")
	}
	if formaFarmaceutica == "" || dosagem == "" || viaAdministracao == "" {
		return erros.Novo(erros.CategoriaValidacao, "forma farmacêutica, dosagem e via de administração são obrigatórias")
	}
	if stockMinimo < 0 {
		return erros.Novo(erros.CategoriaValidacao, "stock mínimo não pode ser negativo")
	}
	m.nomeComercial = nomeComercial
	m.nomeGenerico = nomeGenerico
	m.formaFarmaceutica = formaFarmaceutica
	m.dosagem = dosagem
	m.viaAdministracao = viaAdministracao
	m.fabricante = strings.TrimSpace(fabricante)
	m.requerReceita = requerReceita
	m.psicotropico = psicotropico
	m.classeATC = normalizarOpcional(classeATC)
	m.stockMinimo = stockMinimo
	return nil
}

// Activar/Desactivar alternam a disponibilidade do medicamento no catálogo.
func (m *Medicamento) Activar()    { m.activo = true }
func (m *Medicamento) Desactivar() { m.activo = false }

// CorrespondeSubstancia indica se a substância (case-insensitive, aparada) está
// contida no nome genérico ou comercial — heurística de validação de alergias.
func (m *Medicamento) CorrespondeSubstancia(substancia string) bool {
	s := strings.ToLower(strings.TrimSpace(substancia))
	if s == "" {
		return false
	}
	return strings.Contains(strings.ToLower(m.nomeGenerico), s) ||
		strings.Contains(strings.ToLower(m.nomeComercial), s)
}

// ID devolve o identificador atribuído pela base de dados (vazio se não persistido).
func (m *Medicamento) ID() string { return m.id }

// CodigoInterno devolve o código interno (MED-NNNNN).
func (m *Medicamento) CodigoInterno() string { return m.codigoInterno }

// Activo indica se o medicamento está activo no catálogo.
func (m *Medicamento) Activo() bool { return m.activo }

// normalizarOpcional apara espaços e devolve nil se o resultado for vazio.
func normalizarOpcional(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}

// SnapshotMedicamento carrega o estado completo para persistência/rehidratação.
type SnapshotMedicamento struct {
	ID                string
	CodigoInterno     string
	NomeComercial     string
	NomeGenerico      string
	FormaFarmaceutica string
	Dosagem           string
	ViaAdministracao  string
	Fabricante        string
	RequerReceita     bool
	Psicotropico      bool
	ClasseATC         *string
	StockMinimo       int
	Activo            bool
	CriadoEm          time.Time
	ActualizadoEm     time.Time
}

// Snapshot devolve o estado completo do agregado.
func (m *Medicamento) Snapshot() SnapshotMedicamento {
	return SnapshotMedicamento{
		ID: m.id, CodigoInterno: m.codigoInterno, NomeComercial: m.nomeComercial,
		NomeGenerico: m.nomeGenerico, FormaFarmaceutica: m.formaFarmaceutica, Dosagem: m.dosagem,
		ViaAdministracao: m.viaAdministracao, Fabricante: m.fabricante, RequerReceita: m.requerReceita,
		Psicotropico: m.psicotropico, ClasseATC: m.classeATC, StockMinimo: m.stockMinimo,
		Activo: m.activo, CriadoEm: m.criadoEm, ActualizadoEm: m.actualizadoEm,
	}
}

// ReconstruirMedicamento reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirMedicamento(s SnapshotMedicamento) *Medicamento {
	return &Medicamento{
		id: s.ID, codigoInterno: s.CodigoInterno, nomeComercial: s.NomeComercial,
		nomeGenerico: s.NomeGenerico, formaFarmaceutica: s.FormaFarmaceutica, dosagem: s.Dosagem,
		viaAdministracao: s.ViaAdministracao, fabricante: s.Fabricante, requerReceita: s.RequerReceita,
		psicotropico: s.Psicotropico, classeATC: s.ClasseATC, stockMinimo: s.StockMinimo,
		activo: s.Activo, criadoEm: s.CriadoEm, actualizadoEm: s.ActualizadoEm,
	}
}
```

- [ ] **Step 4: Implementar `repositorio_medicamentos.go`**

```go
package farmacia

import "context"

// FiltroMedicamentos parametriza a pesquisa no catálogo.
type FiltroMedicamentos struct {
	Termo         string
	ApenasActivos bool
	Limite        int
	Deslocamento  int
}

// ResumoMedicamento é o read-model de um medicamento numa listagem.
type ResumoMedicamento struct {
	ID                string `json:"id"`
	CodigoInterno     string `json:"codigo_interno"`
	NomeComercial     string `json:"nome_comercial"`
	NomeGenerico      string `json:"nome_generico"`
	FormaFarmaceutica string `json:"forma_farmaceutica"`
	Dosagem           string `json:"dosagem"`
	Activo            bool   `json:"activo"`
}

// PaginaMedicamentos é uma página de resultados do catálogo.
type PaginaMedicamentos struct {
	Itens        []ResumoMedicamento `json:"itens"`
	Total        int                 `json:"total"`
	Limite       int                 `json:"limite"`
	Deslocamento int                 `json:"deslocamento"`
}

// RepositorioMedicamentos é a porta de saída do catálogo. Implementada em pgrepo.
type RepositorioMedicamentos interface {
	Guardar(ctx context.Context, m *Medicamento) (string, error)
	ObterPorID(ctx context.Context, id string) (*Medicamento, error)
	Pesquisar(ctx context.Context, f FiltroMedicamentos) (PaginaMedicamentos, error)
	ProximoCodigo(ctx context.Context) (string, error) // "MED-00001"
}
```

- [ ] **Step 5: Correr os testes e confirmar que passam**

Run: `go test ./internal/domain/farmacia/ -v`
Expected: PASS. `gofmt -l internal/domain/farmacia/` vazio.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/farmacia/medicamento.go internal/domain/farmacia/repositorio_medicamentos.go internal/domain/farmacia/medicamento_test.go
git commit -m "feat(farmacia): agregado Medicamento e porta de repositório do catálogo

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Domínio — agregado Receita, itens, estado, eventos e repositório

**Files:**
- Create: `internal/domain/farmacia/enums.go`
- Create: `internal/domain/farmacia/receita.go`
- Create: `internal/domain/farmacia/eventos.go`
- Create: `internal/domain/farmacia/repositorio_receitas.go`
- Test: `internal/domain/farmacia/receita_test.go`

**Interfaces:**
- Consumes: `erros`; `evento.EventoDominio`; `normalizarOpcional` (Task 3, mesmo pacote).
- Produces:
  - `type EstadoReceita string`; consts `ReceitaEmitida="EMITIDA"`, `ReceitaParcial="PARCIAL"`, `ReceitaDispensada="DISPENSADA"`, `ReceitaExpirada="EXPIRADA"`, `ReceitaAnulada="ANULADA"`.
  - `type ItemReceita struct { MedicamentoID, Posologia string; DuracaoDias *int; QuantidadePrescrita, QuantidadeDispensada int; Notas string }`; `NovoItemReceita(medicamentoID, posologia string, duracaoDias *int, quantidadePrescrita int, notas string) (ItemReceita, error)`.
  - `type Receita struct {...}` (campos privados) + getters `ID()`, `DoenteID()`, `Estado()`.
  - `NovaReceita(episodioID, doenteID, medicoID string, itens []ItemReceita, notas string, emitidaEm, expiraEm time.Time) (*Receita, error)`.
  - `Anular() error`; `EstadoEfectivo(agora time.Time) EstadoReceita`.
  - `type SnapshotReceita struct {...}`; `Snapshot() SnapshotReceita`; `ReconstruirReceita(SnapshotReceita) *Receita`.
  - Eventos `MedicamentoRegistado`, `ReceitaEmitida`, `ReceitaAnulada`.
  - `type FiltroReceitas`, `ResumoReceita`, `PaginaReceitas`, `RepositorioReceitas`.

- [ ] **Step 1: Escrever o teste que falha (`receita_test.go`)**

```go
package farmacia_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func itemValido(t *testing.T) farmacia.ItemReceita {
	t.Helper()
	it, err := farmacia.NovoItemReceita("med-1", "1 comprimido 8/8h", nil, 20, "")
	if err != nil {
		t.Fatalf("NovoItemReceita: %v", err)
	}
	return it
}

func receitaValida(t *testing.T) *farmacia.Receita {
	t.Helper()
	emitida := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	expira := emitida.AddDate(0, 0, 30)
	r, err := farmacia.NovaReceita("ep-1", "doente-1", "medico-1", []farmacia.ItemReceita{itemValido(t)}, "", emitida, expira)
	if err != nil {
		t.Fatalf("NovaReceita: %v", err)
	}
	return r
}

func TestNovaReceita_EstadoInicialEmitida(t *testing.T) {
	r := receitaValida(t)
	if r.Estado() != farmacia.ReceitaEmitida {
		t.Fatalf("estado inicial=%q, esperava EMITIDA", r.Estado())
	}
	if r.DoenteID() != "doente-1" {
		t.Fatalf("doente=%q", r.DoenteID())
	}
}

func TestNovaReceita_ExigePeloMenosUmItem(t *testing.T) {
	emitida := time.Now()
	if _, err := farmacia.NovaReceita("ep-1", "d-1", "m-1", nil, "", emitida, emitida.AddDate(0, 0, 30)); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para receita sem itens")
	}
}

func TestNovoItemReceita_QuantidadeInvalida(t *testing.T) {
	if _, err := farmacia.NovoItemReceita("med-1", "posologia", nil, 0, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para quantidade zero")
	}
	if _, err := farmacia.NovoItemReceita("med-1", "  ", nil, 5, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para posologia vazia")
	}
}

func TestReceita_Anular(t *testing.T) {
	r := receitaValida(t)
	if err := r.Anular(); err != nil {
		t.Fatalf("anular: %v", err)
	}
	if r.Estado() != farmacia.ReceitaAnulada {
		t.Fatalf("estado=%q, esperava ANULADA", r.Estado())
	}
	if erros.CategoriaDe(r.Anular()) != erros.CategoriaConflito {
		t.Fatal("esperava conflito ao anular uma receita já anulada")
	}
}

func TestReceita_EstadoEfectivoExpira(t *testing.T) {
	r := receitaValida(t) // expira em 2026-07-31
	depois := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if r.EstadoEfectivo(depois) != farmacia.ReceitaExpirada {
		t.Fatalf("esperava EXPIRADA após a data de expiração, obtive %q", r.EstadoEfectivo(depois))
	}
	antes := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	if r.EstadoEfectivo(antes) != farmacia.ReceitaEmitida {
		t.Fatalf("esperava EMITIDA antes da expiração, obtive %q", r.EstadoEfectivo(antes))
	}
	// Uma receita anulada não passa a EXPIRADA.
	_ = r.Anular()
	if r.EstadoEfectivo(depois) != farmacia.ReceitaAnulada {
		t.Fatalf("anulada não devia expirar: %q", r.EstadoEfectivo(depois))
	}
}

func TestReconstruirReceita_PreservaEstado(t *testing.T) {
	orig := receitaValida(t)
	_ = orig.Anular()
	snap := orig.Snapshot()
	snap.ID = "rec-1"
	rec := farmacia.ReconstruirReceita(snap)
	if rec.ID() != "rec-1" || rec.Estado() != farmacia.ReceitaAnulada || len(rec.Snapshot().Itens) != 1 {
		t.Fatalf("rehidratação perdeu estado: %+v", rec.Snapshot())
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/farmacia/ -run 'Receita|Item' -v`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Implementar `enums.go`**

```go
package farmacia

// EstadoReceita é o estado do ciclo de vida de uma receita (DDM-001).
type EstadoReceita string

const (
	ReceitaEmitida    EstadoReceita = "EMITIDA"
	ReceitaParcial    EstadoReceita = "PARCIAL"
	ReceitaDispensada EstadoReceita = "DISPENSADA"
	ReceitaExpirada   EstadoReceita = "EXPIRADA"
	ReceitaAnulada    EstadoReceita = "ANULADA"
)
```

- [ ] **Step 4: Implementar `receita.go`**

```go
package farmacia

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// ItemReceita é uma linha de prescrição (medicamento + posologia + quantidade).
// Campos exportados; construído por NovoItemReceita, que valida.
type ItemReceita struct {
	MedicamentoID        string
	Posologia            string
	DuracaoDias          *int
	QuantidadePrescrita  int
	QuantidadeDispensada int
	Notas                string
}

// NovoItemReceita valida e constrói um item. MedicamentoID e posologia não-vazios;
// quantidade prescrita > 0; quantidade dispensada inicial 0.
func NovoItemReceita(medicamentoID, posologia string, duracaoDias *int, quantidadePrescrita int, notas string) (ItemReceita, error) {
	medicamentoID = strings.TrimSpace(medicamentoID)
	if medicamentoID == "" {
		return ItemReceita{}, erros.Novo(erros.CategoriaValidacao, "medicamento do item da receita em falta")
	}
	posologia = strings.TrimSpace(posologia)
	if posologia == "" {
		return ItemReceita{}, erros.Novo(erros.CategoriaValidacao, "posologia do item da receita em falta")
	}
	if quantidadePrescrita <= 0 {
		return ItemReceita{}, erros.Novo(erros.CategoriaValidacao, "quantidade prescrita deve ser positiva")
	}
	return ItemReceita{
		MedicamentoID:        medicamentoID,
		Posologia:            posologia,
		DuracaoDias:          duracaoDias,
		QuantidadePrescrita:  quantidadePrescrita,
		QuantidadeDispensada: 0,
		Notas:                strings.TrimSpace(notas),
	}, nil
}

// Receita é o agregado raiz da prescrição, emitida num episódio clínico.
type Receita struct {
	id         string
	episodioID string
	doenteID   string
	medicoID   string
	emitidaEm  time.Time
	estado     EstadoReceita
	notas      string
	expiraEm   time.Time
	itens      []ItemReceita
}

// NovaReceita valida e constrói uma receita no estado EMITIDA. Os três ids são
// obrigatórios; exige pelo menos um item; expiraEm posterior a emitidaEm.
func NovaReceita(episodioID, doenteID, medicoID string, itens []ItemReceita, notas string, emitidaEm, expiraEm time.Time) (*Receita, error) {
	episodioID = strings.TrimSpace(episodioID)
	doenteID = strings.TrimSpace(doenteID)
	medicoID = strings.TrimSpace(medicoID)
	if episodioID == "" || doenteID == "" || medicoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "episódio, doente e médico da receita são obrigatórios")
	}
	if len(itens) == 0 {
		return nil, erros.Novo(erros.CategoriaValidacao, "a receita tem de ter pelo menos um item")
	}
	for _, it := range itens {
		if _, err := NovoItemReceita(it.MedicamentoID, it.Posologia, it.DuracaoDias, it.QuantidadePrescrita, it.Notas); err != nil {
			return nil, err
		}
	}
	if !expiraEm.After(emitidaEm) {
		return nil, erros.Novo(erros.CategoriaValidacao, "a data de expiração tem de ser posterior à emissão")
	}
	return &Receita{
		episodioID: episodioID,
		doenteID:   doenteID,
		medicoID:   medicoID,
		emitidaEm:  emitidaEm,
		estado:     ReceitaEmitida,
		notas:      strings.TrimSpace(notas),
		expiraEm:   expiraEm,
		itens:      itens,
	}, nil
}

// ID/DoenteID/Estado — getters.
func (r *Receita) ID() string          { return r.id }
func (r *Receita) DoenteID() string    { return r.doenteID }
func (r *Receita) Estado() EstadoReceita { return r.estado }

// Anular passa a receita a ANULADA. Só de EMITIDA/PARCIAL.
func (r *Receita) Anular() error {
	if r.estado != ReceitaEmitida && r.estado != ReceitaParcial {
		return erros.Novo(erros.CategoriaConflito, "só é possível anular uma receita emitida ou parcial")
	}
	r.estado = ReceitaAnulada
	return nil
}

// EstadoEfectivo devolve o estado tendo em conta a expiração: se EMITIDA/PARCIAL e
// a data de expiração já passou, devolve EXPIRADA (não persistido — calculado na
// leitura). Compara por data.
func (r *Receita) EstadoEfectivo(agora time.Time) EstadoReceita {
	if (r.estado == ReceitaEmitida || r.estado == ReceitaParcial) && agora.Truncate(24*time.Hour).After(r.expiraEm.Truncate(24 * time.Hour)) {
		return ReceitaExpirada
	}
	return r.estado
}

// SnapshotReceita carrega o estado completo para persistência/rehidratação.
type SnapshotReceita struct {
	ID         string
	EpisodioID string
	DoenteID   string
	MedicoID   string
	EmitidaEm  time.Time
	Estado     EstadoReceita
	Notas      string
	ExpiraEm   time.Time
	Itens      []ItemReceita
}

// Snapshot devolve o estado completo do agregado.
func (r *Receita) Snapshot() SnapshotReceita {
	return SnapshotReceita{
		ID: r.id, EpisodioID: r.episodioID, DoenteID: r.doenteID, MedicoID: r.medicoID,
		EmitidaEm: r.emitidaEm, Estado: r.estado, Notas: r.notas, ExpiraEm: r.expiraEm, Itens: r.itens,
	}
}

// ReconstruirReceita reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirReceita(s SnapshotReceita) *Receita {
	return &Receita{
		id: s.ID, episodioID: s.EpisodioID, doenteID: s.DoenteID, medicoID: s.MedicoID,
		emitidaEm: s.EmitidaEm, estado: s.Estado, notas: s.Notas, expiraEm: s.ExpiraEm, itens: s.Itens,
	}
}
```

- [ ] **Step 5: Implementar `eventos.go`**

```go
package farmacia

import (
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

// MedicamentoRegistado é emitido quando um medicamento é adicionado ao catálogo.
type MedicamentoRegistado struct {
	MedicamentoID string
	Em            time.Time
}

func (e MedicamentoRegistado) NomeEvento() string    { return "farmacia.medicamento.registado" }
func (e MedicamentoRegistado) OcorridoEm() time.Time { return e.Em }

// ReceitaEmitida é emitido quando uma receita é emitida.
type ReceitaEmitida struct {
	ReceitaID string
	DoenteID  string
	Em        time.Time
}

func (e ReceitaEmitida) NomeEvento() string    { return "farmacia.receita.emitida" }
func (e ReceitaEmitida) OcorridoEm() time.Time { return e.Em }

// ReceitaAnulada é emitido quando uma receita é anulada.
type ReceitaAnulada struct {
	ReceitaID string
	Em        time.Time
}

func (e ReceitaAnulada) NomeEvento() string    { return "farmacia.receita.anulada" }
func (e ReceitaAnulada) OcorridoEm() time.Time { return e.Em }

// Garantias de conformidade com a interface de evento de domínio.
var (
	_ evento.EventoDominio = MedicamentoRegistado{}
	_ evento.EventoDominio = ReceitaEmitida{}
	_ evento.EventoDominio = ReceitaAnulada{}
)
```

- [ ] **Step 6: Implementar `repositorio_receitas.go`**

```go
package farmacia

import (
	"context"
	"time"
)

// FiltroReceitas parametriza a listagem de receitas de um doente.
type FiltroReceitas struct {
	DoenteID     string
	EpisodioID   string // filtro opcional
	Estado       string // filtro opcional
	Limite       int
	Deslocamento int
}

// ResumoReceita é o read-model de uma receita numa listagem.
type ResumoReceita struct {
	ID         string    `json:"id"`
	EpisodioID string    `json:"episodio_id"`
	MedicoID   string    `json:"medico_id"`
	EmitidaEm  time.Time `json:"emitida_em"`
	Estado     string    `json:"estado"`
	ExpiraEm   time.Time `json:"expira_em"`
	NumItens   int       `json:"num_itens"`
}

// PaginaReceitas é uma página de receitas.
type PaginaReceitas struct {
	Itens        []ResumoReceita `json:"itens"`
	Total        int             `json:"total"`
	Limite       int             `json:"limite"`
	Deslocamento int             `json:"deslocamento"`
}

// RepositorioReceitas é a porta de saída das receitas. Implementada em pgrepo.
type RepositorioReceitas interface {
	Guardar(ctx context.Context, r *Receita) (string, error)
	ObterPorID(ctx context.Context, id string) (*Receita, error)
	ListarPorDoente(ctx context.Context, f FiltroReceitas) (PaginaReceitas, error)
}
```

- [ ] **Step 7: Correr os testes e a cobertura do domínio**

Run: `go test ./internal/domain/farmacia/ -v`
Expected: PASS.
Run: `bash scripts/cobertura.sh` (secção domínio ≥85%). Se abaixo, acrescenta casos (ex.: NovaReceita com item inválido; EstadoEfectivo de DISPENSADA não expira; ReconstruirReceita simetria de todos os campos).

- [ ] **Step 8: Commit**

```bash
git add internal/domain/farmacia/enums.go internal/domain/farmacia/receita.go internal/domain/farmacia/eventos.go internal/domain/farmacia/repositorio_receitas.go internal/domain/farmacia/receita_test.go
git commit -m "feat(farmacia): agregado Receita com itens, estado, eventos e repositório

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Aplicação — portas/DTOs, mapeamento e casos de uso do Medicamento

**Files:**
- Create: `internal/application/farmacia/ports.go`
- Create: `internal/application/farmacia/mapa.go`
- Create: `internal/application/farmacia/medicamentos.go`
- Test: `internal/application/farmacia/fakes_test.go`
- Test: `internal/application/farmacia/medicamentos_test.go`

**Interfaces:**
- Consumes: domínio `farmacia` (Tasks 3-4); `auditoria.Registo`; `erros`.
- Produces:
  - Porta `Auditor`; porta `LeitorClinico` + `AlergiaClinica`.
  - Reexports `FiltroMedicamentos`/`PaginaMedicamentos`/`ResumoMedicamento`, `FiltroReceitas`/`PaginaReceitas`/`ResumoReceita`.
  - DTOs: `DadosNovoMedicamento`, `DadosActualizarMedicamento`, `DetalheMedicamento`; `DadosItemReceita`, `DadosNovaReceita`, `DadosAnularReceita`, `ItemReceitaDTO`, `DetalheReceita`.
  - `paraDetalheMedicamento(*dominio.Medicamento) DetalheMedicamento`; `paraDetalheReceita(*dominio.Receita, agora time.Time) DetalheReceita`.
  - `CasoRegistarMedicamento`, `CasoActualizarMedicamento`, `CasoDefinirEstadoMedicamento`, `CasoObterMedicamento`, `CasoPesquisarMedicamentos` (com construtores `NovoCaso...`).
  - Constantes `limiteDefault=20`, `limiteMaximo=100`.

- [ ] **Step 1: Escrever os fakes e o teste que falha**

`fakes_test.go` (define aqui: `fakeRepoMed`, `novoFakeRepoMed`, `leftPad`, `fakeAuditor`, e o helper `medicamentoParaRepo` da nota abaixo):
```go
package farmacia_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeRepoMed é um repositório de medicamentos em memória.
type fakeRepoMed struct {
	porID      map[string]*farmacia.Medicamento
	seq        int
	guardarErr error
	pagina     farmacia.PaginaMedicamentos
	ultimoFilt farmacia.FiltroMedicamentos
}

func novoFakeRepoMed() *fakeRepoMed { return &fakeRepoMed{porID: map[string]*farmacia.Medicamento{}} }

func (f *fakeRepoMed) Guardar(_ context.Context, m *farmacia.Medicamento) (string, error) {
	if f.guardarErr != nil {
		return "", f.guardarErr
	}
	snap := m.Snapshot()
	id := snap.ID
	if id == "" {
		f.seq++
		id = "med-" + strconv.Itoa(f.seq)
		snap.ID = id
	}
	f.porID[id] = farmacia.ReconstruirMedicamento(snap)
	return id, nil
}
func (f *fakeRepoMed) ObterPorID(_ context.Context, id string) (*farmacia.Medicamento, error) {
	m, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "medicamento não encontrado")
	}
	return m, nil
}
func (f *fakeRepoMed) Pesquisar(_ context.Context, filt farmacia.FiltroMedicamentos) (farmacia.PaginaMedicamentos, error) {
	f.ultimoFilt = filt
	return f.pagina, nil
}
func (f *fakeRepoMed) ProximoCodigo(_ context.Context) (string, error) {
	f.seq++
	return "MED-" + leftPad(f.seq), nil
}

func leftPad(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 5 {
		s = "0" + s
	}
	return s
}

// fakeAuditor recolhe os registos de auditoria.
type fakeAuditor struct{ registos []auditoria.Registo }

func (a *fakeAuditor) Registar(_ context.Context, r auditoria.Registo) error {
	a.registos = append(a.registos, r)
	return nil
}
```

`medicamentos_test.go`:
```go
package farmacia_test

import (
	"context"
	"testing"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
)

func dadosMedBase() appfarmacia.DadosNovoMedicamento {
	return appfarmacia.DadosNovoMedicamento{
		NomeComercial: "Amoxil 500mg", NomeGenerico: "Amoxicilina",
		FormaFarmaceutica: "COMPRIMIDO", Dosagem: "500 mg", ViaAdministracao: "ORAL",
		RequerReceita: true, StockMinimo: 10,
	}
}

func TestRegistarMedicamento_GeraCodigoEAudita(t *testing.T) {
	repo := novoFakeRepoMed()
	aud := &fakeAuditor{}
	caso := appfarmacia.NovoCasoRegistarMedicamento(repo, aud)
	out, err := caso.Executar(context.Background(), "farm-1", dadosMedBase())
	if err != nil {
		t.Fatalf("registar: %v", err)
	}
	if out.ID == "" || out.CodigoInterno == "" {
		t.Fatalf("saída incompleta: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.medicamento.registado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestRegistarMedicamento_Invalido(t *testing.T) {
	repo := novoFakeRepoMed()
	caso := appfarmacia.NovoCasoRegistarMedicamento(repo, &fakeAuditor{})
	dados := dadosMedBase()
	dados.NomeComercial = ""
	if _, err := caso.Executar(context.Background(), "farm-1", dados); err == nil {
		t.Fatal("esperava erro de validação")
	}
}

func TestDesactivarMedicamento(t *testing.T) {
	repo := novoFakeRepoMed()
	id, _ := repo.Guardar(context.Background(), medicamentoParaRepo(t))
	caso := appfarmacia.NovoCasoDefinirEstadoMedicamento(repo, &fakeAuditor{})
	out, err := caso.Desactivar(context.Background(), "farm-1", id)
	if err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	if out.Activo {
		t.Fatal("esperava inactivo")
	}
}

func TestPesquisarMedicamentos_LimiteDefault(t *testing.T) {
	repo := novoFakeRepoMed()
	caso := appfarmacia.NovoCasoPesquisarMedicamentos(repo)
	if _, err := caso.Executar(context.Background(), appfarmacia.FiltroMedicamentos{Termo: "amox"}); err != nil {
		t.Fatalf("pesquisar: %v", err)
	}
	if repo.ultimoFilt.Limite != 20 {
		t.Fatalf("limite default=%d, esperava 20", repo.ultimoFilt.Limite)
	}
}
```
> **Nota ao implementador:** define o helper partilhado `medicamentoParaRepo(t)` em `fakes_test.go` (é reutilizado pela Task 6). Constrói um `*farmacia.Medicamento` válido: `func medicamentoParaRepo(t *testing.T) *farmacia.Medicamento { t.Helper(); m, err := farmacia.NovoMedicamento("MED-00001","Amoxil","Amoxicilina","COMPRIMIDO","500 mg","ORAL","",true,false,nil,10); if err != nil { t.Fatal(err) }; return m }` (com o import `farmacia "…/domain/farmacia"` e `"testing"` em `fakes_test.go`).

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/farmacia/ -v`
Expected: FAIL — pacote/símbolos inexistentes.

- [ ] **Step 3: Implementar `ports.go`**

```go
// Package farmacia contém os casos de uso do BC Farmácia (Camada 2 — Aplicação).
package farmacia

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// Auditor persiste registos de auditoria de forma append-only.
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}

// AlergiaClinica é a projecção de uma alergia do doente vista pela Farmácia.
type AlergiaClinica struct {
	Substancia string
	Severidade string
}

// LeitorClinico é a porta anti-corrupção para leitura de dados do BC Clínico.
type LeitorClinico interface {
	// ObterContextoDoente devolve se o doente existe e está activo, e as suas
	// alergias GRAVE/ANAFILÁCTICA.
	ObterContextoDoente(ctx context.Context, doenteID string) (activo bool, alergiasGraves []AlergiaClinica, err error)
	// EpisodioDoDoente indica se o episódio existe e pertence ao doente.
	EpisodioDoDoente(ctx context.Context, episodioID, doenteID string) (bool, error)
}

// Reexports dos read-models do domínio.
type (
	FiltroMedicamentos = dominio.FiltroMedicamentos
	PaginaMedicamentos = dominio.PaginaMedicamentos
	ResumoMedicamento  = dominio.ResumoMedicamento
	FiltroReceitas     = dominio.FiltroReceitas
	PaginaReceitas     = dominio.PaginaReceitas
	ResumoReceita      = dominio.ResumoReceita
)

// DadosNovoMedicamento é a entrada do registo de medicamento.
type DadosNovoMedicamento struct {
	NomeComercial     string  `json:"nome_comercial"`
	NomeGenerico      string  `json:"nome_generico"`
	FormaFarmaceutica string  `json:"forma_farmaceutica"`
	Dosagem           string  `json:"dosagem"`
	ViaAdministracao  string  `json:"via_administracao"`
	Fabricante        string  `json:"fabricante"`
	RequerReceita     bool    `json:"requer_receita"`
	Psicotropico      bool    `json:"psicotropico"`
	ClasseATC         *string `json:"classe_atc"`
	StockMinimo       int     `json:"stock_minimo"`
}

// DadosActualizarMedicamento tem a mesma forma (substituição integral dos campos mutáveis).
type DadosActualizarMedicamento = DadosNovoMedicamento

// DetalheMedicamento é o detalhe de um medicamento numa resposta.
type DetalheMedicamento struct {
	ID                string    `json:"id"`
	CodigoInterno     string    `json:"codigo_interno"`
	NomeComercial     string    `json:"nome_comercial"`
	NomeGenerico      string    `json:"nome_generico"`
	FormaFarmaceutica string    `json:"forma_farmaceutica"`
	Dosagem           string    `json:"dosagem"`
	ViaAdministracao  string    `json:"via_administracao"`
	Fabricante        string    `json:"fabricante,omitempty"`
	RequerReceita     bool      `json:"requer_receita"`
	Psicotropico      bool      `json:"psicotropico"`
	ClasseATC         *string   `json:"classe_atc,omitempty"`
	StockMinimo       int       `json:"stock_minimo"`
	Activo            bool      `json:"activo"`
	CriadoEm          time.Time `json:"criado_em"`
	ActualizadoEm     time.Time `json:"actualizado_em"`
}

// DadosItemReceita é um item num pedido de emissão.
type DadosItemReceita struct {
	MedicamentoID       string `json:"medicamento_id"`
	Posologia           string `json:"posologia"`
	DuracaoDias         *int   `json:"duracao_dias"`
	QuantidadePrescrita int    `json:"quantidade_prescrita"`
	Notas               string `json:"notas"`
}

// DadosNovaReceita é a entrada da emissão. MedicoID = actor autenticado.
type DadosNovaReceita struct {
	EpisodioID           string
	DoenteID             string
	Itens                []DadosItemReceita
	Notas                string
	IgnorarAlertaAlergia bool
	JustificacaoAlerta   string
}

// DadosAnularReceita é a entrada da anulação.
type DadosAnularReceita struct {
	Motivo string
}

// ItemReceitaDTO é um item numa resposta.
type ItemReceitaDTO struct {
	MedicamentoID        string `json:"medicamento_id"`
	Posologia            string `json:"posologia"`
	DuracaoDias          *int   `json:"duracao_dias,omitempty"`
	QuantidadePrescrita  int    `json:"quantidade_prescrita"`
	QuantidadeDispensada int    `json:"quantidade_dispensada"`
	Notas                string `json:"notas,omitempty"`
}

// DetalheReceita é o detalhe de uma receita numa resposta.
type DetalheReceita struct {
	ID         string           `json:"id"`
	EpisodioID string           `json:"episodio_id"`
	DoenteID   string           `json:"doente_id"`
	MedicoID   string           `json:"medico_id"`
	EmitidaEm  time.Time        `json:"emitida_em"`
	Estado     string           `json:"estado"`
	Notas      string           `json:"notas,omitempty"`
	ExpiraEm   time.Time        `json:"expira_em"`
	Itens      []ItemReceitaDTO `json:"itens"`
}
```

- [ ] **Step 4: Implementar `mapa.go`**

```go
package farmacia

import (
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
)

const (
	limiteDefault = 20
	limiteMaximo  = 100
)

// paraDetalheMedicamento mapeia o agregado Medicamento para o DTO de detalhe.
func paraDetalheMedicamento(m *dominio.Medicamento) DetalheMedicamento {
	s := m.Snapshot()
	return DetalheMedicamento{
		ID: s.ID, CodigoInterno: s.CodigoInterno, NomeComercial: s.NomeComercial,
		NomeGenerico: s.NomeGenerico, FormaFarmaceutica: s.FormaFarmaceutica, Dosagem: s.Dosagem,
		ViaAdministracao: s.ViaAdministracao, Fabricante: s.Fabricante, RequerReceita: s.RequerReceita,
		Psicotropico: s.Psicotropico, ClasseATC: s.ClasseATC, StockMinimo: s.StockMinimo,
		Activo: s.Activo, CriadoEm: s.CriadoEm, ActualizadoEm: s.ActualizadoEm,
	}
}

// paraDetalheReceita mapeia o agregado Receita para o DTO, com o estado efectivo
// (considera a expiração calculada em `agora`).
func paraDetalheReceita(r *dominio.Receita, agora time.Time) DetalheReceita {
	s := r.Snapshot()
	det := DetalheReceita{
		ID: s.ID, EpisodioID: s.EpisodioID, DoenteID: s.DoenteID, MedicoID: s.MedicoID,
		EmitidaEm: s.EmitidaEm, Estado: string(r.EstadoEfectivo(agora)), Notas: s.Notas,
		ExpiraEm: s.ExpiraEm, Itens: []ItemReceitaDTO{},
	}
	for _, it := range s.Itens {
		det.Itens = append(det.Itens, ItemReceitaDTO{
			MedicamentoID: it.MedicamentoID, Posologia: it.Posologia, DuracaoDias: it.DuracaoDias,
			QuantidadePrescrita: it.QuantidadePrescrita, QuantidadeDispensada: it.QuantidadeDispensada, Notas: it.Notas,
		})
	}
	return det
}
```

- [ ] **Step 5: Implementar `medicamentos.go`**

```go
package farmacia

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarMedicamento regista um medicamento no catálogo e audita.
type CasoRegistarMedicamento struct {
	repo    dominio.RepositorioMedicamentos
	auditor Auditor
	agora   func() time.Time
}

func NovoCasoRegistarMedicamento(r dominio.RepositorioMedicamentos, aud Auditor) *CasoRegistarMedicamento {
	return &CasoRegistarMedicamento{repo: r, auditor: aud, agora: time.Now}
}

func (c *CasoRegistarMedicamento) Executar(ctx context.Context, actor string, dados DadosNovoMedicamento) (DetalheMedicamento, error) {
	codigo, err := c.repo.ProximoCodigo(ctx)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	m, err := dominio.NovoMedicamento(codigo, dados.NomeComercial, dados.NomeGenerico, dados.FormaFarmaceutica, dados.Dosagem, dados.ViaAdministracao, dados.Fabricante, dados.RequerReceita, dados.Psicotropico, dados.ClasseATC, dados.StockMinimo)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	id, err := c.repo.Guardar(ctx, m)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.medicamento.registado", Entidade: "medicamento", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheMedicamento{}, err
	}
	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	return paraDetalheMedicamento(final), nil
}

// CasoActualizarMedicamento actualiza os campos de um medicamento e audita.
type CasoActualizarMedicamento struct {
	repo    dominio.RepositorioMedicamentos
	auditor Auditor
	agora   func() time.Time
}

func NovoCasoActualizarMedicamento(r dominio.RepositorioMedicamentos, aud Auditor) *CasoActualizarMedicamento {
	return &CasoActualizarMedicamento{repo: r, auditor: aud, agora: time.Now}
}

func (c *CasoActualizarMedicamento) Executar(ctx context.Context, actor, id string, dados DadosActualizarMedicamento) (DetalheMedicamento, error) {
	m, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	if err := m.Actualizar(dados.NomeComercial, dados.NomeGenerico, dados.FormaFarmaceutica, dados.Dosagem, dados.ViaAdministracao, dados.Fabricante, dados.RequerReceita, dados.Psicotropico, dados.ClasseATC, dados.StockMinimo); err != nil {
		return DetalheMedicamento{}, err
	}
	if _, err := c.repo.Guardar(ctx, m); err != nil {
		return DetalheMedicamento{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.medicamento.actualizado", Entidade: "medicamento", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheMedicamento{}, err
	}
	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	return paraDetalheMedicamento(final), nil
}

// CasoDefinirEstadoMedicamento activa/desactiva um medicamento e audita.
type CasoDefinirEstadoMedicamento struct {
	repo    dominio.RepositorioMedicamentos
	auditor Auditor
	agora   func() time.Time
}

func NovoCasoDefinirEstadoMedicamento(r dominio.RepositorioMedicamentos, aud Auditor) *CasoDefinirEstadoMedicamento {
	return &CasoDefinirEstadoMedicamento{repo: r, auditor: aud, agora: time.Now}
}

func (c *CasoDefinirEstadoMedicamento) Activar(ctx context.Context, actor, id string) (DetalheMedicamento, error) {
	return c.aplicar(ctx, actor, id, true)
}
func (c *CasoDefinirEstadoMedicamento) Desactivar(ctx context.Context, actor, id string) (DetalheMedicamento, error) {
	return c.aplicar(ctx, actor, id, false)
}
func (c *CasoDefinirEstadoMedicamento) aplicar(ctx context.Context, actor, id string, activar bool) (DetalheMedicamento, error) {
	m, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	accao := "farmacia.medicamento.desactivado"
	if activar {
		m.Activar()
		accao = "farmacia.medicamento.activado"
	} else {
		m.Desactivar()
	}
	if _, err := c.repo.Guardar(ctx, m); err != nil {
		return DetalheMedicamento{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: accao, Entidade: "medicamento", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheMedicamento{}, err
	}
	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	return paraDetalheMedicamento(final), nil
}

// CasoObterMedicamento devolve o detalhe de um medicamento (não audita — catálogo).
type CasoObterMedicamento struct {
	repo dominio.RepositorioMedicamentos
}

func NovoCasoObterMedicamento(r dominio.RepositorioMedicamentos) *CasoObterMedicamento {
	return &CasoObterMedicamento{repo: r}
}
func (c *CasoObterMedicamento) Executar(ctx context.Context, id string) (DetalheMedicamento, error) {
	m, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	return paraDetalheMedicamento(m), nil
}

// CasoPesquisarMedicamentos pesquisa o catálogo (não audita).
type CasoPesquisarMedicamentos struct {
	repo dominio.RepositorioMedicamentos
}

func NovoCasoPesquisarMedicamentos(r dominio.RepositorioMedicamentos) *CasoPesquisarMedicamentos {
	return &CasoPesquisarMedicamentos{repo: r}
}
func (c *CasoPesquisarMedicamentos) Executar(ctx context.Context, filtro FiltroMedicamentos) (PaginaMedicamentos, error) {
	if filtro.Limite <= 0 {
		filtro.Limite = limiteDefault
	}
	if filtro.Limite > limiteMaximo {
		filtro.Limite = limiteMaximo
	}
	if filtro.Deslocamento < 0 {
		filtro.Deslocamento = 0
	}
	return c.repo.Pesquisar(ctx, filtro)
}
```

- [ ] **Step 6: Correr os testes e confirmar que passam**

Run: `go test ./internal/application/farmacia/ -v`
Expected: PASS. Corre `bash scripts/cobertura.sh` e regista a linha real da aplicação; se disponível, `staticcheck ./internal/application/farmacia/...` (nota: `paraDetalheReceita` só será chamado na Task 6 — se o `staticcheck` acusar `unused` nesse ponto, acrescenta um teste interno mínimo `package farmacia` que o exercite, tal como no padrão do Sprint 8).

- [ ] **Step 7: Commit**

```bash
git add internal/application/farmacia/ports.go internal/application/farmacia/mapa.go internal/application/farmacia/medicamentos.go internal/application/farmacia/fakes_test.go internal/application/farmacia/medicamentos_test.go
git commit -m "feat(farmacia): portas, DTOs e casos de uso do catálogo de medicamentos

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Aplicação — casos de uso da Receita (emissão com validação de alergias)

**Files:**
- Create: `internal/application/farmacia/receitas.go`
- Test: `internal/application/farmacia/receitas_test.go`

**Interfaces:**
- Consumes: `RepositorioReceitas`, `RepositorioMedicamentos`, `LeitorClinico`, `Auditor`, `paraDetalheReceita`, `limiteDefault`/`limiteMaximo` (Task 5); domínio `NovaReceita`/`NovoItemReceita`/`Anular`/`CorrespondeSubstancia`.
- Produces:
  - `CasoEmitirReceita` — `NovoCasoEmitirReceita(repoReceitas dominio.RepositorioReceitas, repoMedicamentos dominio.RepositorioMedicamentos, leitor LeitorClinico, aud Auditor)`, `Executar(ctx, actor string, dados DadosNovaReceita) (DetalheReceita, error)`.
  - `CasoAnularReceita` — `NovoCasoAnularReceita(repoReceitas, aud)`, `Executar(ctx, actor, id, motivo string) (DetalheReceita, error)`.
  - `CasoObterReceita` — `NovoCasoObterReceita(repoReceitas, aud)`, `Executar(ctx, actor, id string) (DetalheReceita, error)`.
  - `CasoListarReceitas` — `NovoCasoListarReceitas(repoReceitas)`, `Executar(ctx, filtro FiltroReceitas) (PaginaReceitas, error)`.

**Regras da emissão** (medicoID = actor):
1. `LeitorClinico.ObterContextoDoente(doenteID)` → se `!activo` → `CategoriaConflito`.
2. `LeitorClinico.EpisodioDoDoente(episodioID, doenteID)` → se falso → `CategoriaValidacao`.
3. Para cada item: `repoMedicamentos.ObterPorID(medicamentoID)` (existe; se `!Activo()` → `CategoriaConflito`); constrói `ItemReceita` via `NovoItemReceita`.
4. Alergias: para cada medicamento × cada alergia grave, `medicamento.CorrespondeSubstancia(a.Substancia)` → acumula alertas.
5. Se há alertas e `!IgnorarAlertaAlergia` → `erros.Novo(CategoriaRegraNegocio, <mensagem com a lista>)`. Se `IgnorarAlertaAlergia` → exige `JustificacaoAlerta` não-vazia (senão `CategoriaValidacao`).
6. `NovaReceita` (emitidaEm=`agora()`, expiraEm=`agora()+30 dias`); `Guardar`; audita `farmacia.receita.emitida` (Detalhe com a justificação/alertas quando override); devolve `paraDetalheReceita(final, agora())`.

- [ ] **Step 1: Escrever o teste que falha (`receitas_test.go`)**

```go
package farmacia_test

import (
	"context"
	"strconv"
	"testing"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeRepoReceitas é um repositório de receitas em memória (usado só nesta task).
type fakeRepoReceitas struct {
	porID      map[string]*farmacia.Receita
	seq        int
	pagina     farmacia.PaginaReceitas
	ultimoFilt farmacia.FiltroReceitas
}

func novoFakeRepoReceitas() *fakeRepoReceitas {
	return &fakeRepoReceitas{porID: map[string]*farmacia.Receita{}}
}
func (f *fakeRepoReceitas) Guardar(_ context.Context, r *farmacia.Receita) (string, error) {
	snap := r.Snapshot()
	id := snap.ID
	if id == "" {
		f.seq++
		id = "rec-" + strconv.Itoa(f.seq)
		snap.ID = id
	}
	f.porID[id] = farmacia.ReconstruirReceita(snap)
	return id, nil
}
func (f *fakeRepoReceitas) ObterPorID(_ context.Context, id string) (*farmacia.Receita, error) {
	r, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "receita não encontrada")
	}
	return r, nil
}
func (f *fakeRepoReceitas) ListarPorDoente(_ context.Context, filt farmacia.FiltroReceitas) (farmacia.PaginaReceitas, error) {
	f.ultimoFilt = filt
	return f.pagina, nil
}

// fakeLeitorClinico simula a porta anti-corrupção do BC Clínico.
type fakeLeitorClinico struct {
	activo    bool
	alergias  []appfarmacia.AlergiaClinica
	episodios map[string]string // episodioID -> doenteID
	err       error
}

func (f *fakeLeitorClinico) ObterContextoDoente(_ context.Context, _ string) (bool, []appfarmacia.AlergiaClinica, error) {
	return f.activo, f.alergias, f.err
}
func (f *fakeLeitorClinico) EpisodioDoDoente(_ context.Context, episodioID, doenteID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.episodios[episodioID] == doenteID, nil
}

func prepararEmissao(t *testing.T) (*fakeRepoReceitas, *fakeRepoMed, *fakeLeitorClinico, string) {
	t.Helper()
	repoMed := novoFakeRepoMed()
	medID, _ := repoMed.Guardar(context.Background(), medicamentoParaRepo(t)) // Amoxicilina, activo
	leitor := &fakeLeitorClinico{activo: true, episodios: map[string]string{"ep-1": "doente-1"}}
	return novoFakeRepoReceitas(), repoMed, leitor, medID
}

func dadosReceita(medID string) appfarmacia.DadosNovaReceita {
	return appfarmacia.DadosNovaReceita{
		EpisodioID: "ep-1", DoenteID: "doente-1",
		Itens: []appfarmacia.DadosItemReceita{{MedicamentoID: medID, Posologia: "1 comp 8/8h", QuantidadePrescrita: 20}},
	}
}

func TestEmitirReceita_SemAlergia(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	aud := &fakeAuditor{}
	caso := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, aud)
	out, err := caso.Executar(context.Background(), "medico-1", dadosReceita(medID))
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if out.ID == "" || out.MedicoID != "medico-1" || out.Estado != "EMITIDA" {
		t.Fatalf("saída inesperada: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.receita.emitida" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestEmitirReceita_AlergiaBloqueia(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	leitor.alergias = []appfarmacia.AlergiaClinica{{Substancia: "Amoxicilina", Severidade: "ANAFILACTICA"}}
	caso := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", dadosReceita(medID))
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava RegraNegocio (422), obtive %v", err)
	}
}

func TestEmitirReceita_OverrideSemJustificacao(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	leitor.alergias = []appfarmacia.AlergiaClinica{{Substancia: "Amoxicilina", Severidade: "GRAVE"}}
	dados := dadosReceita(medID)
	dados.IgnorarAlertaAlergia = true // sem justificação
	caso := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "medico-1", dados); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação (falta justificação), obtive %v", err)
	}
}

func TestEmitirReceita_OverrideComJustificacao(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	leitor.alergias = []appfarmacia.AlergiaClinica{{Substancia: "Amoxicilina", Severidade: "GRAVE"}}
	dados := dadosReceita(medID)
	dados.IgnorarAlertaAlergia = true
	dados.JustificacaoAlerta = "Benefício supera o risco; doente monitorizado."
	aud := &fakeAuditor{}
	out, err := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, aud).Executar(context.Background(), "medico-1", dados)
	if err != nil {
		t.Fatalf("emitir com override: %v", err)
	}
	if out.ID == "" {
		t.Fatal("esperava receita emitida com override")
	}
	if len(aud.registos) != 1 || aud.registos[0].Detalhe == "" {
		t.Fatalf("esperava auditoria com detalhe do override: %+v", aud.registos)
	}
}

func TestEmitirReceita_DoenteInactivo(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	leitor.activo = false
	caso := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "medico-1", dadosReceita(medID)); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito (doente inactivo), obtive %v", err)
	}
}

func TestEmitirReceita_EpisodioDeOutroDoente(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	leitor.episodios = map[string]string{"ep-1": "outro-doente"}
	caso := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "medico-1", dadosReceita(medID)); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação (episódio de outro doente), obtive %v", err)
	}
}

func TestAnularReceita(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	emitida, _ := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{}).Executar(context.Background(), "medico-1", dadosReceita(medID))
	aud := &fakeAuditor{}
	out, err := appfarmacia.NovoCasoAnularReceita(repoRec, aud).Executar(context.Background(), "medico-1", emitida.ID, "erro de prescrição")
	if err != nil {
		t.Fatalf("anular: %v", err)
	}
	if out.Estado != "ANULADA" {
		t.Fatalf("estado=%q, esperava ANULADA", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.receita.anulada" || aud.registos[0].Detalhe == "" {
		t.Fatalf("auditoria em falta ou sem motivo: %+v", aud.registos)
	}
}

func TestObterReceita_Audita(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	emitida, _ := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{}).Executar(context.Background(), "medico-1", dadosReceita(medID))
	aud := &fakeAuditor{}
	if _, err := appfarmacia.NovoCasoObterReceita(repoRec, aud).Executar(context.Background(), "medico-1", emitida.ID); err != nil {
		t.Fatalf("obter: %v", err)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.receita.consultada" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/farmacia/ -run 'Receita' -v`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Implementar `receitas.go`**

```go
package farmacia

import (
	"context"
	"fmt"
	"strings"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

const validadeReceitaDias = 30

// CasoEmitirReceita emite uma receita de um episódio, validando as alergias do
// doente (bloqueio com override auditado).
type CasoEmitirReceita struct {
	receitas     dominio.RepositorioReceitas
	medicamentos dominio.RepositorioMedicamentos
	leitor       LeitorClinico
	auditor      Auditor
	agora        func() time.Time
}

func NovoCasoEmitirReceita(receitas dominio.RepositorioReceitas, medicamentos dominio.RepositorioMedicamentos, leitor LeitorClinico, aud Auditor) *CasoEmitirReceita {
	return &CasoEmitirReceita{receitas: receitas, medicamentos: medicamentos, leitor: leitor, auditor: aud, agora: time.Now}
}

func (c *CasoEmitirReceita) Executar(ctx context.Context, actor string, dados DadosNovaReceita) (DetalheReceita, error) {
	activo, alergiasGraves, err := c.leitor.ObterContextoDoente(ctx, dados.DoenteID)
	if err != nil {
		return DetalheReceita{}, err
	}
	if !activo {
		return DetalheReceita{}, erros.Novo(erros.CategoriaConflito, "não é possível emitir uma receita a um doente que não está activo")
	}
	pertence, err := c.leitor.EpisodioDoDoente(ctx, dados.EpisodioID, dados.DoenteID)
	if err != nil {
		return DetalheReceita{}, err
	}
	if !pertence {
		return DetalheReceita{}, erros.Novo(erros.CategoriaValidacao, "o episódio indicado não pertence ao doente")
	}

	itens := make([]dominio.ItemReceita, 0, len(dados.Itens))
	var alertas []string
	for _, di := range dados.Itens {
		med, err := c.medicamentos.ObterPorID(ctx, di.MedicamentoID)
		if err != nil {
			return DetalheReceita{}, err
		}
		if !med.Activo() {
			return DetalheReceita{}, erros.Novo(erros.CategoriaConflito, "o medicamento "+med.CodigoInterno()+" está inactivo")
		}
		item, err := dominio.NovoItemReceita(di.MedicamentoID, di.Posologia, di.DuracaoDias, di.QuantidadePrescrita, di.Notas)
		if err != nil {
			return DetalheReceita{}, err
		}
		itens = append(itens, item)
		for _, a := range alergiasGraves {
			if med.CorrespondeSubstancia(a.Substancia) {
				alertas = append(alertas, fmt.Sprintf("%s (alergia %s a %s)", med.CodigoInterno(), a.Severidade, a.Substancia))
			}
		}
	}

	if len(alertas) > 0 {
		if !dados.IgnorarAlertaAlergia {
			return DetalheReceita{}, erros.Novo(erros.CategoriaRegraNegocio, "a prescrição colide com alergias graves do doente: "+strings.Join(alertas, "; "))
		}
		if strings.TrimSpace(dados.JustificacaoAlerta) == "" {
			return DetalheReceita{}, erros.Novo(erros.CategoriaValidacao, "é obrigatória uma justificação para ignorar o alerta de alergia")
		}
	}

	agora := c.agora()
	expira := agora.AddDate(0, 0, validadeReceitaDias)
	receita, err := dominio.NovaReceita(dados.EpisodioID, dados.DoenteID, actor, itens, dados.Notas, agora, expira)
	if err != nil {
		return DetalheReceita{}, err
	}
	id, err := c.receitas.Guardar(ctx, receita)
	if err != nil {
		return DetalheReceita{}, err
	}
	detalheAud := ""
	if len(alertas) > 0 {
		detalheAud = "override alergia: " + dados.JustificacaoAlerta + " | alertas: " + strings.Join(alertas, "; ")
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.receita.emitida", Entidade: "receita", EntidadeID: id, Detalhe: detalheAud, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheReceita{}, err
	}
	final, err := c.receitas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheReceita{}, err
	}
	return paraDetalheReceita(final, c.agora()), nil
}

// CasoAnularReceita anula uma receita e audita (motivo em Detalhe).
type CasoAnularReceita struct {
	receitas dominio.RepositorioReceitas
	auditor  Auditor
	agora    func() time.Time
}

func NovoCasoAnularReceita(receitas dominio.RepositorioReceitas, aud Auditor) *CasoAnularReceita {
	return &CasoAnularReceita{receitas: receitas, auditor: aud, agora: time.Now}
}
func (c *CasoAnularReceita) Executar(ctx context.Context, actor, id, motivo string) (DetalheReceita, error) {
	receita, err := c.receitas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheReceita{}, err
	}
	if err := receita.Anular(); err != nil {
		return DetalheReceita{}, err
	}
	if _, err := c.receitas.Guardar(ctx, receita); err != nil {
		return DetalheReceita{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.receita.anulada", Entidade: "receita", EntidadeID: id, Detalhe: motivo, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheReceita{}, err
	}
	final, err := c.receitas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheReceita{}, err
	}
	return paraDetalheReceita(final, c.agora()), nil
}

// CasoObterReceita devolve o detalhe de uma receita (com estado efectivo) e audita.
type CasoObterReceita struct {
	receitas dominio.RepositorioReceitas
	auditor  Auditor
	agora    func() time.Time
}

func NovoCasoObterReceita(receitas dominio.RepositorioReceitas, aud Auditor) *CasoObterReceita {
	return &CasoObterReceita{receitas: receitas, auditor: aud, agora: time.Now}
}
func (c *CasoObterReceita) Executar(ctx context.Context, actor, id string) (DetalheReceita, error) {
	receita, err := c.receitas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheReceita{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.receita.consultada", Entidade: "receita", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheReceita{}, err
	}
	return paraDetalheReceita(receita, c.agora()), nil
}

// CasoListarReceitas lista as receitas de um doente (não audita).
type CasoListarReceitas struct {
	receitas dominio.RepositorioReceitas
}

func NovoCasoListarReceitas(receitas dominio.RepositorioReceitas) *CasoListarReceitas {
	return &CasoListarReceitas{receitas: receitas}
}
func (c *CasoListarReceitas) Executar(ctx context.Context, filtro FiltroReceitas) (PaginaReceitas, error) {
	if filtro.Limite <= 0 {
		filtro.Limite = limiteDefault
	}
	if filtro.Limite > limiteMaximo {
		filtro.Limite = limiteMaximo
	}
	if filtro.Deslocamento < 0 {
		filtro.Deslocamento = 0
	}
	return c.receitas.ListarPorDoente(ctx, filtro)
}
```

- [ ] **Step 4: Correr os testes e a cobertura da aplicação**

Run: `go test ./internal/application/farmacia/ -v`
Expected: PASS.
Run: `bash scripts/cobertura.sh` (secção aplicação ≥75% — linha real); se disponível `staticcheck ./internal/application/farmacia/...` (sem avisos). Se abaixo de 75%, acrescenta casos (ex.: listar aplica limites; medicamento inexistente na emissão propaga NaoEncontrado).

- [ ] **Step 5: Commit**

```bash
git add internal/application/farmacia/receitas.go internal/application/farmacia/receitas_test.go
git commit -m "feat(farmacia): casos de uso da receita com validação de alergias e override

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Repositório PostgreSQL de medicamentos + integração

**Files:**
- Create: `internal/adapters/pgrepo/medicamentos_repo.go`
- Test: `tests/integration/medicamentos_test.go` (tag `integration`)

**Interfaces:**
- Consumes: domínio `farmacia` (`Medicamento`, `Snapshot`, `SnapshotMedicamento`, `ReconstruirMedicamento`, `FiltroMedicamentos`/`PaginaMedicamentos`/`ResumoMedicamento`); `erros`; `pgx`/`pgxpool`/`pgconn`. Helper `deref(*string) string` já existe no pacote `pgrepo`.
- Produces: `RepositorioMedicamentos` com `NovoRepositorioMedicamentos(pool *pgxpool.Pool) *RepositorioMedicamentos`, implementando `farmacia.RepositorioMedicamentos`.

**Padrão:** ver `internal/adapters/pgrepo/doentes_repo.go`. `codigo_interno` duplicado (23505) → `CategoriaConflito`. `fabricante` é `string` no domínio (lido com `COALESCE(...,'')`); `classe_atc` é `*string` (lido para `*string`).

- [ ] **Step 1: Implementar `medicamentos_repo.go`**

```go
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioMedicamentos implementa dominio.RepositorioMedicamentos com pgx.
type RepositorioMedicamentos struct {
	pool *pgxpool.Pool
}

// NovoRepositorioMedicamentos constrói o repositório sobre o pool pgx.
func NovoRepositorioMedicamentos(pool *pgxpool.Pool) *RepositorioMedicamentos {
	return &RepositorioMedicamentos{pool: pool}
}

// ProximoCodigo reserva atomicamente o próximo código do catálogo (MED-NNNNN).
func (r *RepositorioMedicamentos) ProximoCodigo(ctx context.Context) (string, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT nextval('farmacia.seq_codigo_medicamento')`).Scan(&n); err != nil {
		return "", fmt.Errorf("reservar código de medicamento: %w", err)
	}
	return fmt.Sprintf("MED-%05d", n), nil
}

// Guardar persiste o medicamento (INSERT se id vazio, senão UPDATE). Código
// interno duplicado → CategoriaConflito.
func (r *RepositorioMedicamentos) Guardar(ctx context.Context, m *dominio.Medicamento) (string, error) {
	s := m.Snapshot()
	if s.ID == "" {
		return r.inserir(ctx, s)
	}
	return s.ID, r.actualizar(ctx, s)
}

func (r *RepositorioMedicamentos) inserir(ctx context.Context, s dominio.SnapshotMedicamento) (string, error) {
	const q = `
INSERT INTO farmacia.medicamentos (
    codigo_interno, nome_comercial, nome_generico, forma_farmaceutica, dosagem,
    via_administracao, fabricante, requer_receita, psicotropico, classe_atc, stock_minimo, activo
) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,$12) RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q,
		s.CodigoInterno, s.NomeComercial, s.NomeGenerico, s.FormaFarmaceutica, s.Dosagem,
		s.ViaAdministracao, s.Fabricante, s.RequerReceita, s.Psicotropico, s.ClasseATC, s.StockMinimo, s.Activo,
	).Scan(&id)
	if err != nil {
		return "", traduzUnicidadeMedicamento(err)
	}
	return id, nil
}

func (r *RepositorioMedicamentos) actualizar(ctx context.Context, s dominio.SnapshotMedicamento) error {
	const q = `
UPDATE farmacia.medicamentos SET
    nome_comercial=$2, nome_generico=$3, forma_farmaceutica=$4, dosagem=$5,
    via_administracao=$6, fabricante=NULLIF($7,''), requer_receita=$8, psicotropico=$9,
    classe_atc=$10, stock_minimo=$11, activo=$12, actualizado_em=now()
WHERE id=$1`
	ct, err := r.pool.Exec(ctx, q, s.ID,
		s.NomeComercial, s.NomeGenerico, s.FormaFarmaceutica, s.Dosagem,
		s.ViaAdministracao, s.Fabricante, s.RequerReceita, s.Psicotropico, s.ClasseATC, s.StockMinimo, s.Activo,
	)
	if err != nil {
		return traduzUnicidadeMedicamento(err)
	}
	if ct.RowsAffected() == 0 {
		return erros.Novo(erros.CategoriaNaoEncontrado, "medicamento não encontrado")
	}
	return nil
}

// ObterPorID devolve o medicamento. NaoEncontrado se não existir.
func (r *RepositorioMedicamentos) ObterPorID(ctx context.Context, id string) (*dominio.Medicamento, error) {
	const q = `
SELECT id::text, codigo_interno, nome_comercial, nome_generico, forma_farmaceutica, dosagem,
       via_administracao, COALESCE(fabricante,''), requer_receita, psicotropico, classe_atc,
       stock_minimo, activo, criado_em, actualizado_em
FROM farmacia.medicamentos WHERE id=$1`
	var s dominio.SnapshotMedicamento
	if err := r.pool.QueryRow(ctx, q, id).Scan(
		&s.ID, &s.CodigoInterno, &s.NomeComercial, &s.NomeGenerico, &s.FormaFarmaceutica, &s.Dosagem,
		&s.ViaAdministracao, &s.Fabricante, &s.RequerReceita, &s.Psicotropico, &s.ClasseATC,
		&s.StockMinimo, &s.Activo, &s.CriadoEm, &s.ActualizadoEm,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "medicamento não encontrado")
		}
		return nil, fmt.Errorf("obter medicamento: %w", err)
	}
	return dominio.ReconstruirMedicamento(s), nil
}

// Pesquisar devolve uma página do catálogo (nome via trigram na expressão indexada).
func (r *RepositorioMedicamentos) Pesquisar(ctx context.Context, f dominio.FiltroMedicamentos) (dominio.PaginaMedicamentos, error) {
	base := `FROM farmacia.medicamentos WHERE ($1='' OR (nome_comercial || ' ' || nome_generico) ILIKE '%'||$1||'%') AND ($2 = false OR activo)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+base, f.Termo, f.ApenasActivos).Scan(&total); err != nil {
		return dominio.PaginaMedicamentos{}, fmt.Errorf("contar medicamentos: %w", err)
	}
	q := `SELECT id::text, codigo_interno, nome_comercial, nome_generico, forma_farmaceutica, dosagem, activo ` +
		base + ` ORDER BY nome_comercial LIMIT $3 OFFSET $4`
	linhas, err := r.pool.Query(ctx, q, f.Termo, f.ApenasActivos, f.Limite, f.Deslocamento)
	if err != nil {
		return dominio.PaginaMedicamentos{}, fmt.Errorf("pesquisar medicamentos: %w", err)
	}
	defer linhas.Close()
	pagina := dominio.PaginaMedicamentos{Total: total, Limite: f.Limite, Deslocamento: f.Deslocamento, Itens: []dominio.ResumoMedicamento{}}
	for linhas.Next() {
		var it dominio.ResumoMedicamento
		if err := linhas.Scan(&it.ID, &it.CodigoInterno, &it.NomeComercial, &it.NomeGenerico, &it.FormaFarmaceutica, &it.Dosagem, &it.Activo); err != nil {
			return dominio.PaginaMedicamentos{}, fmt.Errorf("ler medicamento: %w", err)
		}
		pagina.Itens = append(pagina.Itens, it)
	}
	return pagina, linhas.Err()
}

// traduzUnicidadeMedicamento mapeia 23505 (código interno duplicado) para conflito.
func traduzUnicidadeMedicamento(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return erros.Novo(erros.CategoriaConflito, "já existe um medicamento com este código interno")
	}
	return fmt.Errorf("guardar medicamento: %w", err)
}
```

- [ ] **Step 2: Escrever o teste de integração `tests/integration/medicamentos_test.go`**

```go
//go:build integration

package integration_test

import (
	"context"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRepositorioMedicamentos_CicloCompleto(t *testing.T) {
	pool, ctx := ligar(t)
	aplicarMigracoesTeste(t, pool, ctx)
	repo := pgrepo.NovoRepositorioMedicamentos(pool)

	cod, err := repo.ProximoCodigo(ctx)
	if err != nil || cod[:4] != "MED-" {
		t.Fatalf("próximo código: %v (%q)", err, cod)
	}
	m, _ := dominio.NovoMedicamento(cod, "Amoxil Integração 500", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "GSK", true, false, nil, 10)
	id, err := repo.Guardar(ctx, m)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM farmacia.medicamentos WHERE id=$1`, id) })

	lido, err := repo.ObterPorID(ctx, id)
	if err != nil || lido.CodigoInterno() != cod {
		t.Fatalf("obter falhou: %v", err)
	}

	pag, err := repo.Pesquisar(ctx, dominio.FiltroMedicamentos{Termo: "Integração", ApenasActivos: true, Limite: 10})
	if err != nil || pag.Total < 1 {
		t.Fatalf("pesquisar falhou: %v (total=%d)", err, pag.Total)
	}

	// Código duplicado → conflito.
	dup, _ := dominio.NovoMedicamento(cod, "Outro", "Outro", "COMPRIMIDO", "1 g", "ORAL", "", true, false, nil, 5)
	if _, err := repo.Guardar(ctx, dup); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito de código, obtive %v", err)
	}
}
```
> **Nota:** `ligar(t)` e `aplicarMigracoesTeste(t, pool, ctx)` já existem no pacote `integration_test` (Sprints 7-8) — reutiliza-os.

- [ ] **Step 3: Compilar e correr**

Run: `go build ./...` ; `go vet ./internal/adapters/pgrepo/` ; `go vet -tags integration ./tests/integration/` — limpos.
Run: `go test -tags integration ./tests/integration/ -run TestRepositorioMedicamentos -v` — PASS (com BD) ou SKIP (sem `DATABASE_URL`); se tiveres o docker-compose com Postgres, corre-o contra a BD real.
Run: `gofmt -l internal/adapters/pgrepo/ tests/integration/` — vazio.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/pgrepo/medicamentos_repo.go tests/integration/medicamentos_test.go
git commit -m "feat(farmacia): repositório pgx do catálogo de medicamentos e integração

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Repositório PostgreSQL de receitas + integração

**Files:**
- Create: `internal/adapters/pgrepo/receitas_repo.go`
- Test: `tests/integration/receitas_test.go` (tag `integration`)

**Interfaces:**
- Consumes: domínio `farmacia` (`Receita`, `Snapshot`, `SnapshotReceita`, `ReconstruirReceita`, `ItemReceita`, `EstadoReceita`, `FiltroReceitas`/`PaginaReceitas`/`ResumoReceita`); `erros`; `pgx`/`pgxpool`. Helper `deref` existe.
- Produces: `RepositorioReceitas` com `NovoRepositorioReceitas(pool *pgxpool.Pool) *RepositorioReceitas`, implementando `farmacia.RepositorioReceitas`.

**Padrão:** transacção; itens por delete-and-reinsert; `notas` opcional com `NULLIF`/`COALESCE`; `duracao_dias` anulável via `*int`.

- [ ] **Step 1: Implementar `receitas_repo.go`**

```go
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioReceitas implementa dominio.RepositorioReceitas com pgx.
type RepositorioReceitas struct {
	pool *pgxpool.Pool
}

// NovoRepositorioReceitas constrói o repositório sobre o pool pgx.
func NovoRepositorioReceitas(pool *pgxpool.Pool) *RepositorioReceitas {
	return &RepositorioReceitas{pool: pool}
}

// Guardar persiste a receita e os seus itens numa transacção.
func (r *RepositorioReceitas) Guardar(ctx context.Context, rec *dominio.Receita) (string, error) {
	s := rec.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := s.ID
	if id == "" {
		const q = `
INSERT INTO farmacia.receitas (episodio_id, doente_id, medico_id, emitida_em, estado, notas, expira_em)
VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7) RETURNING id::text`
		if err := tx.QueryRow(ctx, q, s.EpisodioID, s.DoenteID, s.MedicoID, s.EmitidaEm, string(s.Estado), s.Notas, s.ExpiraEm).Scan(&id); err != nil {
			return "", fmt.Errorf("inserir receita: %w", err)
		}
	} else {
		const q = `
UPDATE farmacia.receitas SET episodio_id=$2, doente_id=$3, medico_id=$4, emitida_em=$5,
    estado=$6, notas=NULLIF($7,''), expira_em=$8 WHERE id=$1`
		ct, err := tx.Exec(ctx, q, id, s.EpisodioID, s.DoenteID, s.MedicoID, s.EmitidaEm, string(s.Estado), s.Notas, s.ExpiraEm)
		if err != nil {
			return "", fmt.Errorf("actualizar receita: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return "", erros.Novo(erros.CategoriaNaoEncontrado, "receita não encontrada")
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM farmacia.itens_receita WHERE receita_id=$1`, id); err != nil {
		return "", fmt.Errorf("limpar itens: %w", err)
	}
	for _, it := range s.Itens {
		if _, err := tx.Exec(ctx,
			`INSERT INTO farmacia.itens_receita (receita_id, medicamento_id, posologia, duracao_dias, quantidade_prescrita, quantidade_dispensada, notas)
			 VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''))`,
			id, it.MedicamentoID, it.Posologia, it.DuracaoDias, it.QuantidadePrescrita, it.QuantidadeDispensada, it.Notas); err != nil {
			return "", fmt.Errorf("inserir item: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar transacção: %w", err)
	}
	return id, nil
}

// ObterPorID devolve a receita com os itens. NaoEncontrado se não existir.
func (r *RepositorioReceitas) ObterPorID(ctx context.Context, id string) (*dominio.Receita, error) {
	const q = `
SELECT id::text, episodio_id::text, doente_id::text, medico_id::text, emitida_em, estado,
       COALESCE(notas,''), expira_em
FROM farmacia.receitas WHERE id=$1`
	var s dominio.SnapshotReceita
	var estado string
	if err := r.pool.QueryRow(ctx, q, id).Scan(
		&s.ID, &s.EpisodioID, &s.DoenteID, &s.MedicoID, &s.EmitidaEm, &estado, &s.Notas, &s.ExpiraEm,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "receita não encontrada")
		}
		return nil, fmt.Errorf("obter receita: %w", err)
	}
	s.Estado = dominio.EstadoReceita(estado)
	itens, err := r.carregarItens(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	s.Itens = itens
	return dominio.ReconstruirReceita(s), nil
}

func (r *RepositorioReceitas) carregarItens(ctx context.Context, id string) ([]dominio.ItemReceita, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT medicamento_id::text, posologia, duracao_dias, quantidade_prescrita, quantidade_dispensada, COALESCE(notas,'')
		 FROM farmacia.itens_receita WHERE receita_id=$1 ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("carregar itens: %w", err)
	}
	defer linhas.Close()
	var out []dominio.ItemReceita
	for linhas.Next() {
		var it dominio.ItemReceita
		if err := linhas.Scan(&it.MedicamentoID, &it.Posologia, &it.DuracaoDias, &it.QuantidadePrescrita, &it.QuantidadeDispensada, &it.Notas); err != nil {
			return nil, fmt.Errorf("ler item: %w", err)
		}
		out = append(out, it)
	}
	return out, linhas.Err()
}

// ListarPorDoente devolve uma página das receitas do doente, mais recentes primeiro.
func (r *RepositorioReceitas) ListarPorDoente(ctx context.Context, f dominio.FiltroReceitas) (dominio.PaginaReceitas, error) {
	base := `FROM farmacia.receitas r WHERE r.doente_id=$1 AND ($2='' OR r.episodio_id=$2) AND ($3='' OR r.estado=$3)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+base, f.DoenteID, f.EpisodioID, f.Estado).Scan(&total); err != nil {
		return dominio.PaginaReceitas{}, fmt.Errorf("contar receitas: %w", err)
	}
	q := `SELECT r.id::text, r.episodio_id::text, r.medico_id::text, r.emitida_em, r.estado, r.expira_em,
	         (SELECT count(*) FROM farmacia.itens_receita i WHERE i.receita_id=r.id) ` +
		base + ` ORDER BY r.emitida_em DESC LIMIT $4 OFFSET $5`
	linhas, err := r.pool.Query(ctx, q, f.DoenteID, f.EpisodioID, f.Estado, f.Limite, f.Deslocamento)
	if err != nil {
		return dominio.PaginaReceitas{}, fmt.Errorf("listar receitas: %w", err)
	}
	defer linhas.Close()
	pagina := dominio.PaginaReceitas{Total: total, Limite: f.Limite, Deslocamento: f.Deslocamento, Itens: []dominio.ResumoReceita{}}
	for linhas.Next() {
		var it dominio.ResumoReceita
		if err := linhas.Scan(&it.ID, &it.EpisodioID, &it.MedicoID, &it.EmitidaEm, &it.Estado, &it.ExpiraEm, &it.NumItens); err != nil {
			return dominio.PaginaReceitas{}, fmt.Errorf("ler resumo de receita: %w", err)
		}
		pagina.Itens = append(pagina.Itens, it)
	}
	return pagina, linhas.Err()
}
```

- [ ] **Step 2: Escrever o teste de integração `tests/integration/receitas_test.go`**

```go
//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
)

func TestRepositorioReceitas_CicloCompleto(t *testing.T) {
	pool, ctx := ligar(t)
	aplicarMigracoesTeste(t, pool, ctx)
	repoMed := pgrepo.NovoRepositorioMedicamentos(pool)
	repoRec := pgrepo.NovoRepositorioReceitas(pool)

	cod, _ := repoMed.ProximoCodigo(ctx)
	m, _ := dominio.NovoMedicamento(cod, "Amoxil Rec", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, 10)
	medID, err := repoMed.Guardar(ctx, m)
	if err != nil {
		t.Fatalf("guardar medicamento: %v", err)
	}

	emitida := time.Now()
	item, _ := dominio.NovoItemReceita(medID, "1 comp 8/8h", nil, 20, "")
	const doenteID = "00000000-0000-4000-8000-0000000000f1"
	const episodioID = "00000000-0000-4000-8000-0000000000f2"
	const medicoID = "00000000-0000-4000-8000-0000000000f3"
	rec, _ := dominio.NovaReceita(episodioID, doenteID, medicoID, []dominio.ItemReceita{item}, "notas", emitida, emitida.AddDate(0, 0, 30))
	recID, err := repoRec.Guardar(ctx, rec)
	if err != nil {
		t.Fatalf("guardar receita: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.receitas WHERE id=$1`, recID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.medicamentos WHERE id=$1`, medID)
	})

	lido, err := repoRec.ObterPorID(ctx, recID)
	if err != nil || len(lido.Snapshot().Itens) != 1 {
		t.Fatalf("obter receita falhou: %v", err)
	}

	pag, err := repoRec.ListarPorDoente(ctx, dominio.FiltroReceitas{DoenteID: doenteID, Limite: 10})
	if err != nil || pag.Total < 1 || pag.Itens[0].NumItens != 1 {
		t.Fatalf("listar falhou: %v (%+v)", err, pag)
	}
}
```

- [ ] **Step 3: Compilar e correr**

Run: `go build ./...` ; `go vet ./internal/adapters/pgrepo/` ; `go vet -tags integration ./tests/integration/` — limpos.
Run: `go test -tags integration ./tests/integration/ -run 'TestRepositorioReceitas|TestRepositorioMedicamentos' -v` — PASS (com BD) ou SKIP.
Run: `gofmt -l internal/adapters/pgrepo/ tests/integration/` — vazio.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/pgrepo/receitas_repo.go tests/integration/receitas_test.go
git commit -m "feat(farmacia): repositório pgx de receitas e integração

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Adaptador LeitorClinico (anti-corrupção sobre o BC Clínico)

**Files:**
- Create: `internal/adapters/farmacia/leitor_clinico.go`
- Test: `internal/adapters/farmacia/leitor_clinico_test.go`

**Interfaces:**
- Consumes: `clinico.RepositorioDoentes`/`RepositorioEpisodios` (interfaces do domínio clínico), `clinico.EstadoActivo`, `clinico.SeveridadeGrave`/`SeveridadeAnafilactica`, os agregados `clinico.Doente`/`EpisodioClinico`; `appfarmacia.AlergiaClinica`; `erros.CategoriaDe`/`CategoriaNaoEncontrado`.
- Produces: `LeitorClinico` com `NovoLeitorClinico(doentes clinico.RepositorioDoentes, episodios clinico.RepositorioEpisodios) *LeitorClinico`, implementando `appfarmacia.LeitorClinico`.

- [ ] **Step 1: Escrever o teste que falha (`leitor_clinico_test.go`)**

```go
package farmacia_test

import (
	"context"
	"testing"
	"time"

	adfarmacia "github.com/ivandrosilva12/sgcfinal/internal/adapters/farmacia"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakes dos repositórios clínicos (só os métodos usados pelo LeitorClinico).
type fakeDoentes struct{ d *clinico.Doente }

func (f fakeDoentes) Guardar(context.Context, *clinico.Doente) (string, error) { return "", nil }
func (f fakeDoentes) ObterPorID(_ context.Context, _ string) (*clinico.Doente, error) {
	if f.d == nil {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
	}
	return f.d, nil
}
func (f fakeDoentes) ObterPorNumProcesso(context.Context, string) (*clinico.Doente, error) {
	return nil, erros.Novo(erros.CategoriaNaoEncontrado, "n/d")
}
func (f fakeDoentes) Pesquisar(context.Context, clinico.FiltroDoentes) (clinico.PaginaDoentes, error) {
	return clinico.PaginaDoentes{}, nil
}
func (f fakeDoentes) ProximoNumeroProcesso(context.Context, int) (string, error) { return "", nil }

type fakeEpisodios struct{ e *clinico.EpisodioClinico }

func (f fakeEpisodios) Guardar(context.Context, *clinico.EpisodioClinico) (string, error) {
	return "", nil
}
func (f fakeEpisodios) ObterPorID(_ context.Context, _ string) (*clinico.EpisodioClinico, error) {
	if f.e == nil {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "episódio não encontrado")
	}
	return f.e, nil
}
func (f fakeEpisodios) ListarPorDoente(context.Context, clinico.FiltroEpisodios) (clinico.PaginaEpisodios, error) {
	return clinico.PaginaEpisodios{}, nil
}

func doenteComAlergiaGrave(t *testing.T) *clinico.Doente {
	t.Helper()
	snap := clinico.SnapshotDoente{
		ID:     "d-1",
		Estado: clinico.EstadoActivo,
		Alergias: []clinico.Alergia{
			{Substancia: "Penicilina", Severidade: clinico.SeveridadeGrave},
			{Substancia: "Pó", Severidade: clinico.SeveridadeLeve},
		},
	}
	return clinico.ReconstruirDoente(snap)
}

func TestObterContextoDoente_FiltraAlergiasGraves(t *testing.T) {
	leitor := adfarmacia.NovoLeitorClinico(fakeDoentes{d: doenteComAlergiaGrave(t)}, fakeEpisodios{})
	activo, alergias, err := leitor.ObterContextoDoente(context.Background(), "d-1")
	if err != nil || !activo {
		t.Fatalf("esperava activo: %v", err)
	}
	if len(alergias) != 1 || alergias[0].Substancia != "Penicilina" {
		t.Fatalf("esperava só a alergia grave: %+v", alergias)
	}
}

func TestObterContextoDoente_Inexistente(t *testing.T) {
	leitor := adfarmacia.NovoLeitorClinico(fakeDoentes{}, fakeEpisodios{})
	activo, _, err := leitor.ObterContextoDoente(context.Background(), "x")
	if err != nil || activo {
		t.Fatalf("doente inexistente devia dar activo=false sem erro; got activo=%v err=%v", activo, err)
	}
}

func TestEpisodioDoDoente(t *testing.T) {
	// Reconstrói um episódio do doente d-1.
	ep := clinico.ReconstruirEpisodio(clinico.SnapshotEpisodio{ID: "e-1", DoenteID: "d-1", Estado: clinico.EstadoEpisodioAberto, Inicio: time.Now()})
	leitor := adfarmacia.NovoLeitorClinico(fakeDoentes{}, fakeEpisodios{e: ep})
	ok, err := leitor.EpisodioDoDoente(context.Background(), "e-1", "d-1")
	if err != nil || !ok {
		t.Fatalf("esperava pertença: ok=%v err=%v", ok, err)
	}
	nok, _ := leitor.EpisodioDoDoente(context.Background(), "e-1", "outro")
	if nok {
		t.Fatal("não devia pertencer a outro doente")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/farmacia/ -v`
Expected: FAIL — pacote/símbolos inexistentes.

- [ ] **Step 3: Implementar `leitor_clinico.go`**

```go
// Package farmacia (adaptadores) contém adaptadores de saída do BC Farmácia.
// Camada 3 — Adaptadores.
package farmacia

import (
	"context"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// LeitorClinico implementa a porta anti-corrupção appfarmacia.LeitorClinico,
// lendo o BC Clínico através dos seus repositórios.
type LeitorClinico struct {
	doentes   clinico.RepositorioDoentes
	episodios clinico.RepositorioEpisodios
}

// NovoLeitorClinico constrói o adaptador sobre os repositórios clínicos.
func NovoLeitorClinico(doentes clinico.RepositorioDoentes, episodios clinico.RepositorioEpisodios) *LeitorClinico {
	return &LeitorClinico{doentes: doentes, episodios: episodios}
}

// ObterContextoDoente devolve se o doente existe e está activo, e as suas alergias
// GRAVE/ANAFILÁCTICA. Um doente inexistente devolve activo=false sem erro.
func (l *LeitorClinico) ObterContextoDoente(ctx context.Context, doenteID string) (bool, []appfarmacia.AlergiaClinica, error) {
	d, err := l.doentes.ObterPorID(ctx, doenteID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return false, nil, nil
		}
		return false, nil, err
	}
	s := d.Snapshot()
	var graves []appfarmacia.AlergiaClinica
	for _, a := range s.Alergias {
		if a.Severidade == clinico.SeveridadeGrave || a.Severidade == clinico.SeveridadeAnafilactica {
			graves = append(graves, appfarmacia.AlergiaClinica{Substancia: a.Substancia, Severidade: string(a.Severidade)})
		}
	}
	return d.Estado() == clinico.EstadoActivo, graves, nil
}

// EpisodioDoDoente indica se o episódio existe e pertence ao doente.
func (l *LeitorClinico) EpisodioDoDoente(ctx context.Context, episodioID, doenteID string) (bool, error) {
	ep, err := l.episodios.ObterPorID(ctx, episodioID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return false, nil
		}
		return false, err
	}
	return ep.DoenteID() == doenteID, nil
}

// Garantia de conformidade com a porta.
var _ appfarmacia.LeitorClinico = (*LeitorClinico)(nil)
```

- [ ] **Step 4: Correr os testes e a cobertura dos adaptadores**

Run: `go test ./internal/adapters/farmacia/ -v` → PASS.
Run: `go build ./...` ; `gofmt -l internal/adapters/farmacia/` (vazio); `go vet ./internal/adapters/farmacia/`. Se disponível, `staticcheck ./internal/adapters/farmacia/...`.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/farmacia/leitor_clinico.go internal/adapters/farmacia/leitor_clinico_test.go
git commit -m "feat(farmacia): adaptador LeitorClinico (anti-corrupção sobre o BC Clínico)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Handler HTTP do BC Farmácia com RBAC e testes

**Files:**
- Create: `internal/adapters/http/farmacia_handler.go`
- Test: `internal/adapters/http/farmacia_test.go`

**Interfaces:**
- Consumes: casos de uso `application/farmacia` (Tasks 5-6) via interfaces de serviço locais; `SessaoDe`, `RBAC`, `responderErro`, `i18n`, `erros`, `inteiroQuery`; `dominio "internal/domain/identidade"` (papéis + `Sessao`).
- Produces: `FarmaciaHandler`, `NovoFarmaciaHandler(...)`, `RegistarFarmacia(r gin.IRouter, h *FarmaciaHandler, protecao ...gin.HandlerFunc)`.

**Rotas e RBAC** (grupo `/api/v1/farmacia`, protegido por `protecao...`):

| Rota | Método | Papéis |
|---|---|---|
| `/farmacia/medicamentos` | POST | Farmacêutico, FarmacêuticoSenior |
| `/farmacia/medicamentos` | GET | leitura ampla* |
| `/farmacia/medicamentos/:id` | GET | leitura ampla* |
| `/farmacia/medicamentos/:id` | PATCH | Farmacêutico, FarmacêuticoSenior |
| `/farmacia/medicamentos/:id/activar` | POST | Farmacêutico, FarmacêuticoSenior |
| `/farmacia/medicamentos/:id/desactivar` | POST | Farmacêutico, FarmacêuticoSenior |
| `/farmacia/receitas` | POST | **só Médico** |
| `/farmacia/receitas` | GET | leitura ampla* |
| `/farmacia/receitas/:id` | GET | leitura ampla* |
| `/farmacia/receitas/:id/anular` | POST | **só Médico** |

\* leitura ampla = Médico, Enfermeiro, Farmacêutico, FarmacêuticoSenior, Director, DPO, Auditor.

- [ ] **Step 1: Escrever o teste que falha (`farmacia_test.go`)**

```go
package http_test

import (
	nethttp "net/http"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes dos serviços de farmácia ---

type fakeRegistarMed struct {
	out appfarmacia.DetalheMedicamento
	err error
}

func (f fakeRegistarMed) Executar(_ ctxT, _ string, _ appfarmacia.DadosNovoMedicamento) (appfarmacia.DetalheMedicamento, error) {
	return f.out, f.err
}

type fakeActualizarMed struct{ out appfarmacia.DetalheMedicamento; err error }

func (f fakeActualizarMed) Executar(_ ctxT, _, _ string, _ appfarmacia.DadosActualizarMedicamento) (appfarmacia.DetalheMedicamento, error) {
	return f.out, f.err
}

type fakeEstadoMed struct{ out appfarmacia.DetalheMedicamento; err error }

func (f fakeEstadoMed) Activar(_ ctxT, _, _ string) (appfarmacia.DetalheMedicamento, error) {
	return f.out, f.err
}
func (f fakeEstadoMed) Desactivar(_ ctxT, _, _ string) (appfarmacia.DetalheMedicamento, error) {
	return f.out, f.err
}

type fakeObterMed struct{ out appfarmacia.DetalheMedicamento; err error }

func (f fakeObterMed) Executar(_ ctxT, _ string) (appfarmacia.DetalheMedicamento, error) {
	return f.out, f.err
}

type fakePesquisarMed struct{ out appfarmacia.PaginaMedicamentos; err error }

func (f fakePesquisarMed) Executar(_ ctxT, _ appfarmacia.FiltroMedicamentos) (appfarmacia.PaginaMedicamentos, error) {
	return f.out, f.err
}

type fakeEmitirReceita struct{ out appfarmacia.DetalheReceita; err error }

func (f fakeEmitirReceita) Executar(_ ctxT, _ string, _ appfarmacia.DadosNovaReceita) (appfarmacia.DetalheReceita, error) {
	return f.out, f.err
}

type fakeAnularReceita struct{ out appfarmacia.DetalheReceita; err error }

func (f fakeAnularReceita) Executar(_ ctxT, _, _, _ string) (appfarmacia.DetalheReceita, error) {
	return f.out, f.err
}

type fakeObterReceita struct{ out appfarmacia.DetalheReceita; err error }

func (f fakeObterReceita) Executar(_ ctxT, _, _ string) (appfarmacia.DetalheReceita, error) {
	return f.out, f.err
}

type fakeListarReceitas struct{ out appfarmacia.PaginaReceitas; err error }

func (f fakeListarReceitas) Executar(_ ctxT, _ appfarmacia.FiltroReceitas) (appfarmacia.PaginaReceitas, error) {
	return f.out, f.err
}

func routerFarmacia(sessao dominio.Sessao, emitir fakeEmitirReceita) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoFarmaciaHandler(
		fakeRegistarMed{out: appfarmacia.DetalheMedicamento{ID: "med-1", CodigoInterno: "MED-00001"}},
		fakeActualizarMed{out: appfarmacia.DetalheMedicamento{ID: "med-1"}},
		fakeEstadoMed{out: appfarmacia.DetalheMedicamento{ID: "med-1", Activo: false}},
		fakeObterMed{out: appfarmacia.DetalheMedicamento{ID: "med-1"}},
		fakePesquisarMed{out: appfarmacia.PaginaMedicamentos{Total: 0}},
		emitir,
		fakeAnularReceita{out: appfarmacia.DetalheReceita{ID: "rec-1", Estado: "ANULADA"}},
		fakeObterReceita{out: appfarmacia.DetalheReceita{ID: "rec-1"}},
		fakeListarReceitas{out: appfarmacia.PaginaReceitas{Total: 0}},
	)
	adhttp.RegistarFarmacia(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

const corpoMed = `{"nome_comercial":"Amoxil","nome_generico":"Amoxicilina","forma_farmaceutica":"COMPRIMIDO","dosagem":"500 mg","via_administracao":"ORAL","requer_receita":true,"stock_minimo":10}`
const corpoReceita = `{"episodio_id":"ep-1","doente_id":"d-1","itens":[{"medicamento_id":"med-1","posologia":"1 comp 8/8h","quantidade_prescrita":20}]}`

func TestFarmacia_RegistarMedicamento_FarmaceuticoPermitido(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeEmitirReceita{})
	w := pedidoCorpo(r, "POST", "/api/v1/farmacia/medicamentos", corpoMed)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_RegistarMedicamento_MedicoProibido(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/medicamentos", corpoMed); w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestFarmacia_PesquisarMedicamentos_LeituraAmpla(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeEmitirReceita{})
	if w := pedido(r, "GET", "/api/v1/farmacia/medicamentos?termo=amox", "Bearer x"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestFarmacia_EmitirReceita_MedicoPermitido(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeEmitirReceita{out: appfarmacia.DetalheReceita{ID: "rec-1", Estado: "EMITIDA"}})
	w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas", corpoReceita)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_EmitirReceita_FarmaceuticoProibido(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas", corpoReceita); w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestFarmacia_EmitirReceita_Alergia_422(t *testing.T) {
	emitir := fakeEmitirReceita{err: erros.Novo(erros.CategoriaRegraNegocio, "colide com alergia grave")}
	r := routerFarmacia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, emitir)
	w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas", corpoReceita)
	if w.Code != nethttp.StatusUnprocessableEntity {
		t.Fatalf("esperava 422, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmacia_AnularReceita_SoMedico(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas/rec-1/anular", `{"motivo":"erro"}`); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
	r2 := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r2, "POST", "/api/v1/farmacia/receitas/rec-1/anular", `{"motivo":"erro"}`); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Farmacêutico não devia anular: obtive %d", w.Code)
	}
}

func TestFarmacia_ListarReceitas_LeituraAmpla(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeEmitirReceita{})
	if w := pedido(r, "GET", "/api/v1/farmacia/receitas?doente_id=d-1", "Bearer x"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestFarmacia_Desactivar_Farmaceutico(t *testing.T) {
	r := routerFarmacia(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceuticoSenior}}, fakeEmitirReceita{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/medicamentos/med-1/desactivar", ``); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}
```
> **Nota ao implementador:** o alias `ctxT` é só para encurtar as assinaturas dos fakes no plano. No ficheiro real importa `"context"` e usa `context.Context` (remove `ctxT`).

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/http/ -run Farmacia -v`
Expected: FAIL — `NovoFarmaciaHandler`/`RegistarFarmacia` indefinidos.

- [ ] **Step 3: Implementar `farmacia_handler.go`**

```go
package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso do BC Farmácia.
type (
	ServicoRegistarMedicamento interface {
		Executar(ctx context.Context, actor string, dados appfarmacia.DadosNovoMedicamento) (appfarmacia.DetalheMedicamento, error)
	}
	ServicoActualizarMedicamento interface {
		Executar(ctx context.Context, actor, id string, dados appfarmacia.DadosActualizarMedicamento) (appfarmacia.DetalheMedicamento, error)
	}
	ServicoDefinirEstadoMedicamento interface {
		Activar(ctx context.Context, actor, id string) (appfarmacia.DetalheMedicamento, error)
		Desactivar(ctx context.Context, actor, id string) (appfarmacia.DetalheMedicamento, error)
	}
	ServicoObterMedicamento interface {
		Executar(ctx context.Context, id string) (appfarmacia.DetalheMedicamento, error)
	}
	ServicoPesquisarMedicamentos interface {
		Executar(ctx context.Context, filtro appfarmacia.FiltroMedicamentos) (appfarmacia.PaginaMedicamentos, error)
	}
	ServicoEmitirReceita interface {
		Executar(ctx context.Context, actor string, dados appfarmacia.DadosNovaReceita) (appfarmacia.DetalheReceita, error)
	}
	ServicoAnularReceita interface {
		Executar(ctx context.Context, actor, id, motivo string) (appfarmacia.DetalheReceita, error)
	}
	ServicoObterReceita interface {
		Executar(ctx context.Context, actor, id string) (appfarmacia.DetalheReceita, error)
	}
	ServicoListarReceitas interface {
		Executar(ctx context.Context, filtro appfarmacia.FiltroReceitas) (appfarmacia.PaginaReceitas, error)
	}
)

// FarmaciaHandler expõe os endpoints HTTP do BC Farmácia.
type FarmaciaHandler struct {
	registarMed   ServicoRegistarMedicamento
	actualizarMed ServicoActualizarMedicamento
	estadoMed     ServicoDefinirEstadoMedicamento
	obterMed      ServicoObterMedicamento
	pesquisarMed  ServicoPesquisarMedicamentos
	emitir        ServicoEmitirReceita
	anular        ServicoAnularReceita
	obterReceita  ServicoObterReceita
	listarReceita ServicoListarReceitas
}

// NovoFarmaciaHandler constrói o handler com os casos de uso.
func NovoFarmaciaHandler(
	registarMed ServicoRegistarMedicamento,
	actualizarMed ServicoActualizarMedicamento,
	estadoMed ServicoDefinirEstadoMedicamento,
	obterMed ServicoObterMedicamento,
	pesquisarMed ServicoPesquisarMedicamentos,
	emitir ServicoEmitirReceita,
	anular ServicoAnularReceita,
	obterReceita ServicoObterReceita,
	listarReceita ServicoListarReceitas,
) *FarmaciaHandler {
	return &FarmaciaHandler{
		registarMed: registarMed, actualizarMed: actualizarMed, estadoMed: estadoMed,
		obterMed: obterMed, pesquisarMed: pesquisarMed, emitir: emitir, anular: anular,
		obterReceita: obterReceita, listarReceita: listarReceita,
	}
}

// RegistarFarmacia regista as rotas sob /api/v1/farmacia.
func RegistarFarmacia(r gin.IRouter, h *FarmaciaHandler, protecao ...gin.HandlerFunc) {
	g := r.Group("/api/v1/farmacia")
	g.Use(protecao...)

	leitura := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelFarmaceutico,
		dominio.PapelFarmaceuticoSenior, dominio.PapelDirector, dominio.PapelDPO, dominio.PapelAuditor)
	catalogo := RBAC(dominio.PapelFarmaceutico, dominio.PapelFarmaceuticoSenior)
	soMedico := RBAC(dominio.PapelMedico)

	g.POST("/medicamentos", catalogo, h.registarMedicamento)
	g.GET("/medicamentos", leitura, h.pesquisarMedicamentos)
	g.GET("/medicamentos/:id", leitura, h.obterMedicamento)
	g.PATCH("/medicamentos/:id", catalogo, h.actualizarMedicamento)
	g.POST("/medicamentos/:id/activar", catalogo, h.activarMedicamento)
	g.POST("/medicamentos/:id/desactivar", catalogo, h.desactivarMedicamento)

	g.POST("/receitas", soMedico, h.emitirReceita)
	g.GET("/receitas", leitura, h.listarReceitas)
	g.GET("/receitas/:id", leitura, h.obterReceitaHandler)
	g.POST("/receitas/:id/anular", soMedico, h.anularReceita)
}

type corpoMedicamento struct {
	NomeComercial     string  `json:"nome_comercial"`
	NomeGenerico      string  `json:"nome_generico"`
	FormaFarmaceutica string  `json:"forma_farmaceutica"`
	Dosagem           string  `json:"dosagem"`
	ViaAdministracao  string  `json:"via_administracao"`
	Fabricante        string  `json:"fabricante"`
	RequerReceita     bool    `json:"requer_receita"`
	Psicotropico      bool    `json:"psicotropico"`
	ClasseATC         *string `json:"classe_atc"`
	StockMinimo       int     `json:"stock_minimo"`
}

func (c corpoMedicamento) paraDados() appfarmacia.DadosNovoMedicamento {
	return appfarmacia.DadosNovoMedicamento{
		NomeComercial: c.NomeComercial, NomeGenerico: c.NomeGenerico, FormaFarmaceutica: c.FormaFarmaceutica,
		Dosagem: c.Dosagem, ViaAdministracao: c.ViaAdministracao, Fabricante: c.Fabricante,
		RequerReceita: c.RequerReceita, Psicotropico: c.Psicotropico, ClasseATC: c.ClasseATC, StockMinimo: c.StockMinimo,
	}
}

func (h *FarmaciaHandler) registarMedicamento(c *gin.Context) {
	var corpo corpoMedicamento
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registarMed.Executar(c.Request.Context(), actor.Sujeito, corpo.paraDados())
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *FarmaciaHandler) actualizarMedicamento(c *gin.Context) {
	var corpo corpoMedicamento
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.actualizarMed.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), corpo.paraDados())
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) activarMedicamento(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.estadoMed.Activar(c.Request.Context(), actor.Sujeito, c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) desactivarMedicamento(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.estadoMed.Desactivar(c.Request.Context(), actor.Sujeito, c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) obterMedicamento(c *gin.Context) {
	out, err := h.obterMed.Executar(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) pesquisarMedicamentos(c *gin.Context) {
	filtro := appfarmacia.FiltroMedicamentos{
		Termo:         c.Query("termo"),
		ApenasActivos: c.Query("apenas_activos") == "true",
		Limite:        inteiroQuery(c, "limite"),
		Deslocamento:  inteiroQuery(c, "deslocamento"),
	}
	out, err := h.pesquisarMed.Executar(c.Request.Context(), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoItemReceita struct {
	MedicamentoID       string `json:"medicamento_id"`
	Posologia           string `json:"posologia"`
	DuracaoDias         *int   `json:"duracao_dias"`
	QuantidadePrescrita int    `json:"quantidade_prescrita"`
	Notas               string `json:"notas"`
}

type corpoEmitirReceita struct {
	EpisodioID           string             `json:"episodio_id"`
	DoenteID             string             `json:"doente_id"`
	Itens                []corpoItemReceita `json:"itens"`
	Notas                string             `json:"notas"`
	IgnorarAlertaAlergia bool               `json:"ignorar_alerta_alergia"`
	JustificacaoAlerta   string             `json:"justificacao_alerta"`
}

func (h *FarmaciaHandler) emitirReceita(c *gin.Context) {
	var corpo corpoEmitirReceita
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	itens := make([]appfarmacia.DadosItemReceita, 0, len(corpo.Itens))
	for _, it := range corpo.Itens {
		itens = append(itens, appfarmacia.DadosItemReceita{
			MedicamentoID: it.MedicamentoID, Posologia: it.Posologia, DuracaoDias: it.DuracaoDias,
			QuantidadePrescrita: it.QuantidadePrescrita, Notas: it.Notas,
		})
	}
	actor, _ := SessaoDe(c)
	out, err := h.emitir.Executar(c.Request.Context(), actor.Sujeito, appfarmacia.DadosNovaReceita{
		EpisodioID: corpo.EpisodioID, DoenteID: corpo.DoenteID, Itens: itens, Notas: corpo.Notas,
		IgnorarAlertaAlergia: corpo.IgnorarAlertaAlergia, JustificacaoAlerta: corpo.JustificacaoAlerta,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

type corpoAnularReceita struct {
	Motivo string `json:"motivo"`
}

func (h *FarmaciaHandler) anularReceita(c *gin.Context) {
	var corpo corpoAnularReceita
	_ = c.ShouldBindJSON(&corpo) // motivo é opcional
	actor, _ := SessaoDe(c)
	out, err := h.anular.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), corpo.Motivo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) obterReceitaHandler(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.obterReceita.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaHandler) listarReceitas(c *gin.Context) {
	filtro := appfarmacia.FiltroReceitas{
		DoenteID:     c.Query("doente_id"),
		EpisodioID:   c.Query("episodio_id"),
		Estado:       c.Query("estado"),
		Limite:       inteiroQuery(c, "limite"),
		Deslocamento: inteiroQuery(c, "deslocamento"),
	}
	out, err := h.listarReceita.Executar(c.Request.Context(), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
```

- [ ] **Step 4: Correr os testes e a cobertura dos adaptadores**

Run: `go test ./internal/adapters/http/ -v` → PASS (todos, incl. pré-existentes).
Run: `bash scripts/cobertura.sh` (adaptadores ≥60% — linha real); se disponível `staticcheck ./internal/adapters/http/...`. Se abaixo de 60%, acrescenta casos (ex.: obter medicamento 200; obter receita 200; actualizar 200; pesquisar com erro 500).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/farmacia_handler.go internal/adapters/http/farmacia_test.go
git commit -m "feat(farmacia): handler HTTP do catálogo e receitas com RBAC

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Wiring no composition root e ADR-028

**Files:**
- Modify: `internal/platform/app.go`
- Create: `adrs/ADR-028-bc-farmacia-receita.md`

**Interfaces:**
- Consumes: `pgrepo.NovoRepositorioMedicamentos`/`NovoRepositorioReceitas` (Tasks 7-8), `adfarmacia.NovoLeitorClinico` (Task 9), casos de uso `application/farmacia` (Tasks 5-6), `adhttp.NovoFarmaciaHandler`/`RegistarFarmacia` (Task 10); `pool`, `repoDoentes`, `repoEpisodios`, `repoAuditoria`, `limiteMW`, `authMW` já existentes em `app.go`.

**Contexto:** `ExecutarServidor` já constrói `pool`, `repoAuditoria`, `repoDoentes`, `repoEpisodios`, `limiteMW`, `authMW`, e o closure `registarRotas`. Acrescentar o BC Farmácia com `limiteMW`+`authMW` (sem MFA).

- [ ] **Step 1: Acrescentar imports e a construção do BC Farmácia**

Em `internal/platform/app.go`, acrescentar aos imports:
```go
	adfarmacia "github.com/ivandrosilva12/sgcfinal/internal/adapters/farmacia"
	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
```
A seguir ao bloco que constrói `handlerEpisodios`, acrescentar:
```go
	// BC Farmácia: catálogo de medicamentos e receitas.
	repoMedicamentos := pgrepo.NovoRepositorioMedicamentos(pool)
	repoReceitas := pgrepo.NovoRepositorioReceitas(pool)
	leitorClinico := adfarmacia.NovoLeitorClinico(repoDoentes, repoEpisodios)
	handlerFarmacia := adhttp.NovoFarmaciaHandler(
		appfarmacia.NovoCasoRegistarMedicamento(repoMedicamentos, repoAuditoria),
		appfarmacia.NovoCasoActualizarMedicamento(repoMedicamentos, repoAuditoria),
		appfarmacia.NovoCasoDefinirEstadoMedicamento(repoMedicamentos, repoAuditoria),
		appfarmacia.NovoCasoObterMedicamento(repoMedicamentos),
		appfarmacia.NovoCasoPesquisarMedicamentos(repoMedicamentos),
		appfarmacia.NovoCasoEmitirReceita(repoReceitas, repoMedicamentos, leitorClinico, repoAuditoria),
		appfarmacia.NovoCasoAnularReceita(repoReceitas, repoAuditoria),
		appfarmacia.NovoCasoObterReceita(repoReceitas, repoAuditoria),
		appfarmacia.NovoCasoListarReceitas(repoReceitas),
	)
```

- [ ] **Step 2: Registar as rotas no closure `registarRotas`**

Acrescentar a linha (a seguir a `adhttp.RegistarEpisodios(...)`):
```go
		adhttp.RegistarFarmacia(r, handlerFarmacia, limiteMW, authMW)
```

- [ ] **Step 3: Compilar e correr a suite completa**

Run: `go build ./...` ; `go build -tags integration ./...` — sem erros.
Run: `go test ./...` — PASS.
Run: `bash scripts/cobertura.sh` — domínio ≥85%, aplicação ≥75%, adaptadores ≥60% (linha real).
Run: `gofmt -l internal/platform/` (vazio) ; `go vet ./...` ; se disponível `staticcheck ./...`.

- [ ] **Step 4: Escrever `adrs/ADR-028-bc-farmacia-receita.md`**

```markdown
# ADR-028 — BC Farmácia: Medicamento (catálogo) e Receita/Prescrição

- **Estado:** Aceite
- **Data:** 2026-07-12
- **Marco:** M2 — Clínico Core (Sprint 9)
- **Contexto de spec:** docs/superpowers/specs/2026-07-12-sprint9-farmacia-receita-design.md

## Contexto

O Sprint 9 introduz o BC Farmácia, com o catálogo de medicamentos e a receita/prescrição
(emitida de um episódio, com validação de alergias). O modelo de dados foi extraído verbatim do
DDM-001. A gestão de stock (lotes, FEFO, movimentos) e a dispensa ficam para uma fatia seguinte.

## Decisões

1. **Dois agregados no BC Farmácia:** Medicamento (catálogo) e Receita (com itens).
2. **Porta anti-corrupção `LeitorClinico`.** A receita referencia o BC Clínico por id (sem FK
   cross-schema). O domínio/aplicação da Farmácia não importa o domínio do Clínico; um adaptador
   (`internal/adapters/farmacia/leitor_clinico.go`) implementa a porta reutilizando os
   repositórios `clinico`.
3. **`codigo_interno` por SEQUENCE.** `MED-{sequencial:05d}` via
   `farmacia.seq_codigo_medicamento` (nextval atómico).
4. **`medico_id` da receita = actor autenticado** (o prescritor). `doente_id`/`episodio_id` são
   validados por leitura cross-BC.
5. **Validação de alergias com override auditado.** Na emissão, cada medicamento é cruzado
   (texto case-insensitive: substância da alergia contida no nome genérico/comercial) com as
   alergias GRAVE/ANAFILÁCTICA do doente. Havendo colisão, a emissão é **bloqueada (422)**; o
   médico pode forçar com `ignorar_alerta_alergia` + `justificacao_alerta` (registados na
   auditoria). O bloqueio na dispensa (RN-FAR-04) fica para a fatia de stock.
6. **Categoria de erro `RegraNegocio` → 422.** Nova categoria no Shared Kernel para violações de
   regra de negócio (alergia agora; FEFO/stock no futuro).
7. **Estado EXPIRADA calculado na leitura** (expira_em < hoje) — sem batch de transição
   persistida nesta fatia. `expira_em` = emissão + 30 dias (RN-FAR-07).

## Diferimentos

- Stock: lotes, FEFO (RN-FAR-03), movimentos, fornecedores, entrada de stock, alertas de mínimo.
- Dispensa (UC-FAR-02, RN-FAR-04/05) — estados PARCIAL/DISPENSADA.
- Venda directa OTC (UC-FAR-09) e integração com Facturação.
- Psicotrópicos (RN-FAR-06) — registo especial.
- Batch de expiração persistida.
- Vocabulário controlado de forma farmacêutica / via de administração.

## Consequências

- Base para a fatia de stock/dispensa, que consumirá o catálogo e as receitas EMITIDA/PARCIAL.
- Tal como nos agregados do M2, os itens da receita são persistidos por delete-and-reinsert em
  cada `Guardar`.
```

- [ ] **Step 5: Commits**

```bash
git add internal/platform/app.go
git commit -m "feat(farmacia): liga o BC Farmácia ao composition root

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"

git add adrs/ADR-028-bc-farmacia-receita.md
git commit -m "docs(farmacia): ADR-028 com as decisões do BC Farmácia

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Verificação final (fim a fim)

1. `go build ./...` e `go build -tags integration ./...` — sem erros.
2. `go test ./...` — PASS. `bash scripts/cobertura.sh` — 85/75/60 cumpridos.
3. `make lint` — sem violações; `domain/farmacia` não importa `pgx`/`gin`/`uuid` nem `domain/clinico`.
4. `gofmt -l internal/ migrations/ tests/` — vazio. `staticcheck ./...` — sem avisos.
5. Migration `farmacia/0001` aplica-se; `schema_migrations` regista `farmacia/0001_medicamentos_receitas`.
6. `go test -tags integration ./tests/integration/ -run 'Medicamentos|Receitas'` — PASS (com BD) ou SKIP.
7. Fluxo HTTP: registar medicamento (Farmacêutico) → 201 `MED-00001`; emitir receita a doente com alergia grave sem override → 422; com override+justificação → 201; anular → 200; ler catálogo/receitas (leitura ampla) → 200; emitir como Farmacêutico → 403.

## Fora de âmbito (fatias futuras)

Stock/lotes/FEFO/dispensa, fornecedores, venda directa, psicotrópicos, batch de expiração,
vocabulário controlado (ver ADR-028).
