# Sprint 10 — Farmácia: Stock & Dispensa — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar o stock (Fornecedor, Lote, entrada, consulta) e a dispensa da receita (FEFO, revalidação de alergias, não-exceder, estados PARCIAL/DISPENSADA) no BC Farmácia, do domínio ao HTTP.

**Architecture:** DDD + Clean Architecture no BC `farmacia` já existente. Agregados novos Fornecedor e Lote; a alocação FEFO é uma função pura de domínio (`AlocarFEFO`) alimentada por `SELECT ... FOR UPDATE` no adaptador. A dispensa é atómica através de uma porta `MotorDispensa` implementada por um adaptador transaccional pgx. Estende o agregado Receita do Sprint 9 com `RegistarDispensa`.

**Tech Stack:** Go 1.22+, Gin, pgx v5 (SQL puro, transacções + `FOR UPDATE`), PostgreSQL 16 (NUMERIC, índices parciais FEFO); testes `go test` com fakes; integração com tag `integration`.

## Global Constraints

- **Linguagem ubíqua PT-PT angolano** em TODO o output. Nunca inglês nem PT-BR.
- **Domínio puro:** `internal/domain/**` só stdlib + Shared Kernel. Zero `pgx`/`gin`/`http`. O domínio `farmacia` **não** importa o domínio `clinico`. `google/uuid` proibido no domínio e na aplicação.
- **Sem `panic()`** fora de init. **Migrations forward-only.**
- **Erros de domínio** via `erros.Novo(categoria, mensagem)` (PT-PT literal). Categorias: `CategoriaValidacao`, `CategoriaNaoEncontrado`, `CategoriaConflito`, `CategoriaRegraNegocio` (→422, já existe do Sprint 9). **HTTP** via `responderErro`.
- **IDs** do domínio são `string`, gerados pela BD (`gen_random_uuid()` + `RETURNING id::text`).
- **Auditoria:** entrada de stock, dispensa e registo de fornecedor auditados; consultas de stock/lotes e listagem de fornecedores **não** auditam. Acções: `farmacia.fornecedor.registado`, `farmacia.stock.entrada`, `farmacia.receita.dispensada`.
- **Nunca registar em log** dados de saúde.
- **`preco_unit_custo`** modelado como **decimal validado em texto** (`^[0-9]+(\.[0-9]{1,4})?$`, não-negativo); persistido/lido via cast `::text`↔`::numeric`. Sem float nem dependências novas.
- **Movimentos com sinal:** `ENTRADA` positiva, `SAIDA_DISPENSA` negativa.
- **Cobertura:** domínio ≥85%, aplicação ≥75%, adaptadores ≥60% — **mas `internal/adapters/pgrepo` é excluído do gate unitário dos adaptadores** (Task 1; é integration-only por desenho). Confirmar sempre com a execução real de `bash scripts/cobertura.sh` (+ `staticcheck ./...` se disponível — funções de pacote não usadas falham a CI).
- **Commits** Conventional Commits PT-PT com o trailer:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- **Branch:** `m2-sprint10-farmacia-stock-dispensa` (já criado; a spec já lá está commitada).

### Convenções do BC Farmácia / M2 (seguir tal como estão)

- Agregado com campos privados + factory validante + `Snapshot()`/`ReconstruirX(SnapshotX)`; getters. Entidades-filho / VOs com campos exportados (ver `internal/domain/farmacia/medicamento.go`, `receita.go`).
- Enums `type X string` + consts + `ParseX` quando aplicável.
- **Eventos:** os TIPOS de evento usam prefixo `Evento` quando o nome colidiria com uma const (ex.: `EventoReceitaEmitida` coexiste com a const `ReceitaEmitida`). Segue este padrão — o novo evento da dispensa chama-se **`EventoReceitaDispensada`** (a const `ReceitaDispensada` já existe). Ver `internal/domain/farmacia/eventos.go`.
- Read-models (`ResumoX`/`PaginaX`/`FiltroX`) na interface do repositório, com tags JSON.
- Casos de uso: struct + `NovoCaso...`, relógio `agora func() time.Time` (default `time.Now`), `Executar(ctx, ...)`; auditar **só após** persistir; re-ler via `ObterPorID` para devolver detalhe (ver `internal/application/farmacia/*.go`).
- Repositório pgx: `pool *pgxpool.Pool`, transacção com `defer tx.Rollback`, `errors.Is(err, pgx.ErrNoRows)` → `CategoriaNaoEncontrado`, `NULLIF`/`COALESCE` para texto opcional, `*pgconn.PgError` code `23505` → `CategoriaConflito`. Comparações `uuid` com parâmetro texto no padrão `($n='' OR col=$n)` exigem `col::text` (lição do Sprint 9). Helper `deref(*string) string` já existe no pacote `pgrepo`. Ver `internal/adapters/pgrepo/medicamentos_repo.go`, `receitas_repo.go`.
- Handler: struct + interfaces de serviço + `RegistarX(r gin.IRouter, h, protecao ...gin.HandlerFunc)`, RBAC por rota via `RBAC(dominio.PapelX, ...)` (`dominio` = `internal/domain/identidade`), `actor` via `SessaoDe(c).Sujeito`, `inteiroQuery(c, chave)` já existe. Ver `internal/adapters/http/farmacia_handler.go`.
- Papéis: `PapelMedico`, `PapelEnfermeiro`, `PapelFarmaceutico`, `PapelFarmaceuticoSenior`, `PapelTecnicoLab`, `PapelDirector`, `PapelDPO`, `PapelAuditor`.
- Já existentes e reutilizados: `RepositorioMedicamentos` (`ObterPorID`, `CorrespondeSubstancia` via o agregado `Medicamento`), `RepositorioReceitas` (`ObterPorID`), `LeitorClinico` (porta `ObterContextoDoente`/`EpisodioDoDoente`, adaptador em `internal/adapters/farmacia`), `Auditor`, categoria `CategoriaRegraNegocio`, `paraDetalheReceita`, `limiteDefault`/`limiteMaximo` (em `mapa.go`).

---

### Task 1: Excluir o pgrepo do gate unitário dos adaptadores

**Files:**
- Modify: `scripts/cobertura.sh`

**Interfaces:**
- Consumes: nada.
- Produces: gate de adaptadores mede `internal/adapters/...` **excepto** `internal/adapters/pgrepo` (integration-only por desenho).

**Contexto:** o `scripts/cobertura.sh` corre `go test -covermode=set -coverprofile=... <alvo>` por camada. Os repositórios pgx só são cobertos por testes de integração (que não contam no gate), pelo que arrastam o agregado dos adaptadores para baixo a cada sprint. Esta alteração exclui `internal/adapters/pgrepo` do alvo unitário dos adaptadores.

- [ ] **Step 1: Ler o `scripts/cobertura.sh`**

Confirma a estrutura: uma função `verificar()` que corre `go test ... "$alvo"`, e três chamadas (`domínio`, `aplicação`, `adaptadores`). Vais fazer duas alterações cirúrgicas.

- [ ] **Step 2: Permitir múltiplos pacotes no `verificar`**

Na função `verificar`, na linha do `go test`, **remove as aspas** à volta de `$alvo` para que aceite uma lista de pacotes separada por espaços (as chamadas de domínio/aplicação passam um único padrão sem espaços, pelo que continuam a funcionar):
```bash
    if ! go test -covermode=set -coverprofile="$perfil" $alvo >/dev/null 2>&1; then
```
(era `... "$alvo" >/dev/null ...`).

- [ ] **Step 3: Excluir o pgrepo no alvo dos adaptadores**

Substitui a linha:
```bash
verificar "adaptadores" "./internal/adapters/..."    60
```
por:
```bash
# pgrepo é coberto por testes de integração (sem a tag, aparece a 0%): excluído do gate unitário.
adaptadores_pkgs="$(go list ./internal/adapters/... | grep -v '/pgrepo$' | tr '\n' ' ')"
verificar "adaptadores" "$adaptadores_pkgs" 60
```

- [ ] **Step 4: Correr e confirmar**

Run: `bash scripts/cobertura.sh`
Expected: as três linhas imprimem e o gate passa (`Gate de cobertura OK.`); a linha dos adaptadores usa agora o conjunto sem o pgrepo (o número sobe face ao anterior). Confirma que domínio e aplicação continuam a ser medidos correctamente.

- [ ] **Step 5: Commit**

```bash
git add scripts/cobertura.sh
git commit -m "build(cobertura): exclui o pgrepo do gate unitário dos adaptadores

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Migration do stock (fornecedores, lotes, movimentos)

**Files:**
- Create: `migrations/farmacia/0002_stock.sql`
- Modify: `migrations/embed_test.go` (lista de ficheiros esperados)

**Interfaces:**
- Consumes: runner de migrations; o `//go:embed` já inclui o directório `farmacia` (Sprint 9) — o novo ficheiro é embebido automaticamente, **não alteres `embed.go`**.
- Produces: schema `farmacia` com `fornecedores`, `lotes`, `movimentos_stock` + índices FEFO.

- [ ] **Step 1: Criar `migrations/farmacia/0002_stock.sql`**

```sql
-- Bounded Context: farmacia
-- Migration forward-only. Esquema extraído verbatim do DDM-001 v2.0.
--
-- Gestão de stock: fornecedores, lotes (com FEFO) e movimentos de stock.

CREATE TABLE IF NOT EXISTS farmacia.fornecedores (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    nome        text        NOT NULL,
    nif         text,
    contacto    text,
    activo      boolean     NOT NULL DEFAULT true,
    criado_em   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS farmacia.lotes (
    id                 uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    medicamento_id     uuid          NOT NULL REFERENCES farmacia.medicamentos(id),
    numero_lote        text          NOT NULL,
    validade           date          NOT NULL,
    quantidade_inicial integer       NOT NULL CHECK (quantidade_inicial > 0),
    quantidade_actual  integer       NOT NULL CHECK (quantidade_actual >= 0),
    preco_unit_custo   numeric(14,4) NOT NULL CHECK (preco_unit_custo >= 0),
    fornecedor_id      uuid          REFERENCES farmacia.fornecedores(id),
    entrada_em         timestamptz   NOT NULL DEFAULT now(),
    notas              text,
    UNIQUE (medicamento_id, numero_lote, fornecedor_id)
);
CREATE INDEX IF NOT EXISTS idx_lotes_fefo
    ON farmacia.lotes (medicamento_id, validade ASC) WHERE quantidade_actual > 0;
CREATE INDEX IF NOT EXISTS idx_lotes_validade_proxima
    ON farmacia.lotes (validade) WHERE quantidade_actual > 0 AND validade <= (CURRENT_DATE + INTERVAL '90 days');

COMMENT ON TABLE farmacia.lotes IS
    'Lotes de stock por medicamento. FEFO: consumir primeiro a validade mais próxima (mas válida).';

CREATE TABLE IF NOT EXISTS farmacia.movimentos_stock (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tipo           text        NOT NULL CHECK (tipo IN ('ENTRADA','SAIDA_DISPENSA','SAIDA_VENDA','AJUSTE','EXPIRADO','TRANSFERENCIA')),
    medicamento_id uuid        NOT NULL REFERENCES farmacia.medicamentos(id),
    lote_id        uuid        NOT NULL REFERENCES farmacia.lotes(id),
    quantidade     integer     NOT NULL CHECK (quantidade != 0),
    motivo         text,
    receita_id     uuid,
    factura_id     uuid,
    ajuste_justif  text,
    realizado_por  uuid        NOT NULL,
    realizado_em   timestamptz NOT NULL DEFAULT now(),
    CHECK (tipo != 'AJUSTE' OR ajuste_justif IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS idx_movimentos_lote ON farmacia.movimentos_stock (lote_id, realizado_em DESC);
CREATE INDEX IF NOT EXISTS idx_movimentos_medicamento_data ON farmacia.movimentos_stock (medicamento_id, realizado_em DESC);
```

- [ ] **Step 2: Actualizar `migrations/embed_test.go`**

Lê o ficheiro; acrescenta `"farmacia/0002_stock.sql"` à lista de ficheiros esperados, logo a seguir a `"farmacia/0001_medicamentos_receitas.sql"`.

- [ ] **Step 3: Compilar e correr o teste do embed**

Run: `go test ./migrations/ -v`
Expected: PASS.
Run: `go build ./...`
Expected: sem erros.

- [ ] **Step 4: Commit**

```bash
git add migrations/farmacia/0002_stock.sql migrations/embed_test.go
git commit -m "feat(farmacia): migration do stock (fornecedores, lotes, movimentos)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Domínio — agregados Fornecedor e Lote + repositórios

**Files:**
- Create: `internal/domain/farmacia/fornecedor.go`
- Create: `internal/domain/farmacia/lote.go`
- Create: `internal/domain/farmacia/repositorio_stock.go`
- Test: `internal/domain/farmacia/fornecedor_test.go`
- Test: `internal/domain/farmacia/lote_test.go`

**Interfaces:**
- Consumes: `erros.Novo`; o helper `normalizarOpcional(*string) *string` (já existe em `medicamento.go`, mesmo pacote — **não o redefinas**).
- Produces:
  - `type Fornecedor struct{...}` + `NovoFornecedor(nome string, nif, contacto *string) (*Fornecedor, error)`; getters `ID()`, `Activo()`; `Activar()`/`Desactivar()`; `SnapshotFornecedor`; `Snapshot()`; `ReconstruirFornecedor`.
  - `type Lote struct{...}` + `NovoLote(medicamentoID, numeroLote string, validade time.Time, quantidade int, precoUnitarioCusto string, fornecedorID *string, notas string) (*Lote, error)`; getters `ID()`, `MedicamentoID()`, `QuantidadeActual()`; `Disponivel(agora time.Time) bool`; `SnapshotLote`; `Snapshot()`; `ReconstruirLote`.
  - `FiltroFornecedores`/`ResumoFornecedor`/`PaginaFornecedores`, `RepositorioFornecedores`; `ResumoLote`, `RepositorioLotes`.

- [ ] **Step 1: Escrever os testes que falham**

`fornecedor_test.go`:
```go
package farmacia_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovoFornecedor(t *testing.T) {
	f, err := farmacia.NovoFornecedor("Farmédica Lda", nil, nil)
	if err != nil || !f.Activo() {
		t.Fatalf("fornecedor inesperado: %v", err)
	}
}

func TestNovoFornecedor_NomeObrigatorio(t *testing.T) {
	if _, err := farmacia.NovoFornecedor("  ", nil, nil); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestFornecedor_ActivarDesactivar(t *testing.T) {
	f, _ := farmacia.NovoFornecedor("X", nil, nil)
	f.Desactivar()
	if f.Activo() {
		t.Fatal("esperava inactivo")
	}
	f.Activar()
	if !f.Activo() {
		t.Fatal("esperava activo")
	}
}

func TestReconstruirFornecedor(t *testing.T) {
	orig, _ := farmacia.NovoFornecedor("Y", nil, nil)
	orig.Desactivar()
	snap := orig.Snapshot()
	snap.ID = "f-1"
	rec := farmacia.ReconstruirFornecedor(snap)
	if rec.ID() != "f-1" || rec.Activo() {
		t.Fatalf("rehidratação perdeu estado: %+v", rec.Snapshot())
	}
}
```

`lote_test.go`:
```go
package farmacia_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func loteValido(t *testing.T) *farmacia.Lote {
	t.Helper()
	l, err := farmacia.NovoLote("med-1", "L001", time.Now().AddDate(1, 0, 0), 100, "12.3456", nil, "")
	if err != nil {
		t.Fatalf("NovoLote: %v", err)
	}
	return l
}

func TestNovoLote_Valido(t *testing.T) {
	l := loteValido(t)
	if l.QuantidadeActual() != 100 || l.MedicamentoID() != "med-1" {
		t.Fatalf("lote inesperado: %+v", l.Snapshot())
	}
	if !l.Disponivel(time.Now()) {
		t.Fatal("esperava disponível")
	}
}

func TestNovoLote_ValidadePassada(t *testing.T) {
	if _, err := farmacia.NovoLote("med-1", "L001", time.Now().AddDate(0, 0, -1), 10, "1", nil, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para validade passada")
	}
}

func TestNovoLote_QuantidadeEPreco(t *testing.T) {
	fut := time.Now().AddDate(1, 0, 0)
	if _, err := farmacia.NovoLote("med-1", "L001", fut, 0, "1", nil, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para quantidade zero")
	}
	if _, err := farmacia.NovoLote("med-1", "L001", fut, 5, "1.23456", nil, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para preço com >4 casas")
	}
	if _, err := farmacia.NovoLote("med-1", "L001", fut, 5, "abc", nil, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para preço não-numérico")
	}
}

func TestReconstruirLote(t *testing.T) {
	orig := loteValido(t)
	snap := orig.Snapshot()
	snap.ID = "lote-1"
	snap.QuantidadeActual = 40
	rec := farmacia.ReconstruirLote(snap)
	if rec.ID() != "lote-1" || rec.QuantidadeActual() != 40 {
		t.Fatalf("rehidratação perdeu estado: %+v", rec.Snapshot())
	}
}
```

- [ ] **Step 2: Correr e confirmar que falham**

Run: `go test ./internal/domain/farmacia/ -run 'Fornecedor|Lote' -v`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Implementar `fornecedor.go`**

```go
package farmacia

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Fornecedor é o agregado de um fornecedor de medicamentos.
type Fornecedor struct {
	id       string
	nome     string
	nif      *string
	contacto *string
	activo   bool
	criadoEm time.Time
}

// NovoFornecedor valida e constrói um fornecedor activo. Nome obrigatório.
func NovoFornecedor(nome string, nif, contacto *string) (*Fornecedor, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "nome do fornecedor em falta")
	}
	return &Fornecedor{nome: nome, nif: normalizarOpcional(nif), contacto: normalizarOpcional(contacto), activo: true}, nil
}

func (f *Fornecedor) ID() string   { return f.id }
func (f *Fornecedor) Activo() bool { return f.activo }
func (f *Fornecedor) Activar()     { f.activo = true }
func (f *Fornecedor) Desactivar()  { f.activo = false }

// SnapshotFornecedor carrega o estado completo para persistência/rehidratação.
type SnapshotFornecedor struct {
	ID       string
	Nome     string
	NIF      *string
	Contacto *string
	Activo   bool
	CriadoEm time.Time
}

func (f *Fornecedor) Snapshot() SnapshotFornecedor {
	return SnapshotFornecedor{ID: f.id, Nome: f.nome, NIF: f.nif, Contacto: f.contacto, Activo: f.activo, CriadoEm: f.criadoEm}
}

func ReconstruirFornecedor(s SnapshotFornecedor) *Fornecedor {
	return &Fornecedor{id: s.ID, nome: s.Nome, nif: s.NIF, contacto: s.Contacto, activo: s.Activo, criadoEm: s.CriadoEm}
}
```

- [ ] **Step 4: Implementar `lote.go`**

```go
package farmacia

import (
	"regexp"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// formatoPreco valida um decimal não-negativo com até 4 casas (NUMERIC(14,4)).
var formatoPreco = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,4})?$`)

// Lote é o agregado de um lote de stock de um medicamento.
type Lote struct {
	id                 string
	medicamentoID      string
	numeroLote         string
	validade           time.Time
	quantidadeInicial  int
	quantidadeActual   int
	precoUnitarioCusto string
	fornecedorID       *string
	entradaEm          time.Time
	notas              string
}

// NovoLote valida e constrói um lote. Medicamento e número obrigatórios;
// quantidade > 0 (RN-FAR-02); validade futura (RN-FAR-01); preço decimal ≥ 0.
func NovoLote(medicamentoID, numeroLote string, validade time.Time, quantidade int, precoUnitarioCusto string, fornecedorID *string, notas string) (*Lote, error) {
	medicamentoID = strings.TrimSpace(medicamentoID)
	numeroLote = strings.TrimSpace(numeroLote)
	if medicamentoID == "" || numeroLote == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "medicamento e número de lote são obrigatórios")
	}
	if quantidade <= 0 {
		return nil, erros.Novo(erros.CategoriaValidacao, "a quantidade do lote deve ser positiva")
	}
	if !validade.After(time.Now()) {
		return nil, erros.Novo(erros.CategoriaValidacao, "a validade do lote tem de ser futura")
	}
	preco := strings.TrimSpace(precoUnitarioCusto)
	if !formatoPreco.MatchString(preco) {
		return nil, erros.Novo(erros.CategoriaValidacao, "preço unitário inválido (decimal não-negativo, até 4 casas)")
	}
	return &Lote{
		medicamentoID: medicamentoID, numeroLote: numeroLote, validade: validade,
		quantidadeInicial: quantidade, quantidadeActual: quantidade, precoUnitarioCusto: preco,
		fornecedorID: normalizarOpcional(fornecedorID), notas: strings.TrimSpace(notas),
	}, nil
}

func (l *Lote) ID() string            { return l.id }
func (l *Lote) MedicamentoID() string { return l.medicamentoID }
func (l *Lote) QuantidadeActual() int { return l.quantidadeActual }

// Disponivel indica se o lote tem stock e ainda está válido.
func (l *Lote) Disponivel(agora time.Time) bool {
	return l.quantidadeActual > 0 && agora.Before(l.validade)
}

// SnapshotLote carrega o estado completo para persistência/rehidratação.
type SnapshotLote struct {
	ID                 string
	MedicamentoID      string
	NumeroLote         string
	Validade           time.Time
	QuantidadeInicial  int
	QuantidadeActual   int
	PrecoUnitarioCusto string
	FornecedorID       *string
	EntradaEm          time.Time
	Notas              string
}

func (l *Lote) Snapshot() SnapshotLote {
	return SnapshotLote{
		ID: l.id, MedicamentoID: l.medicamentoID, NumeroLote: l.numeroLote, Validade: l.validade,
		QuantidadeInicial: l.quantidadeInicial, QuantidadeActual: l.quantidadeActual,
		PrecoUnitarioCusto: l.precoUnitarioCusto, FornecedorID: l.fornecedorID, EntradaEm: l.entradaEm, Notas: l.notas,
	}
}

func ReconstruirLote(s SnapshotLote) *Lote {
	return &Lote{
		id: s.ID, medicamentoID: s.MedicamentoID, numeroLote: s.NumeroLote, validade: s.Validade,
		quantidadeInicial: s.QuantidadeInicial, quantidadeActual: s.QuantidadeActual,
		precoUnitarioCusto: s.PrecoUnitarioCusto, fornecedorID: s.FornecedorID, entradaEm: s.EntradaEm, notas: s.Notas,
	}
}
```

- [ ] **Step 5: Implementar `repositorio_stock.go`**

```go
package farmacia

import (
	"context"
	"time"
)

// FiltroFornecedores parametriza a listagem de fornecedores.
type FiltroFornecedores struct {
	Termo         string
	ApenasActivos bool
	Limite        int
	Deslocamento  int
}

// ResumoFornecedor é o read-model de um fornecedor numa listagem.
type ResumoFornecedor struct {
	ID     string  `json:"id"`
	Nome   string  `json:"nome"`
	NIF    *string `json:"nif,omitempty"`
	Activo bool    `json:"activo"`
}

// PaginaFornecedores é uma página de fornecedores.
type PaginaFornecedores struct {
	Itens        []ResumoFornecedor `json:"itens"`
	Total        int                `json:"total"`
	Limite       int                `json:"limite"`
	Deslocamento int                `json:"deslocamento"`
}

// RepositorioFornecedores é a porta de saída dos fornecedores.
type RepositorioFornecedores interface {
	Guardar(ctx context.Context, f *Fornecedor) (string, error)
	ObterPorID(ctx context.Context, id string) (*Fornecedor, error)
	Listar(ctx context.Context, filtro FiltroFornecedores) (PaginaFornecedores, error)
}

// ResumoLote é o read-model de um lote numa listagem.
type ResumoLote struct {
	ID               string    `json:"id"`
	NumeroLote       string    `json:"numero_lote"`
	Validade         time.Time `json:"validade"`
	QuantidadeActual int       `json:"quantidade_actual"`
	FornecedorID     *string   `json:"fornecedor_id,omitempty"`
}

// RepositorioLotes é a porta de saída dos lotes de stock.
type RepositorioLotes interface {
	// RegistarEntrada persiste o lote e o movimento ENTRADA, atomicamente.
	RegistarEntrada(ctx context.Context, l *Lote, realizadoPor string) (id string, err error)
	ObterPorID(ctx context.Context, id string) (*Lote, error)
	ListarPorMedicamento(ctx context.Context, medicamentoID string, apenasDisponiveis bool) ([]ResumoLote, error)
	StockDisponivel(ctx context.Context, medicamentoID string) (int, error)
}
```

- [ ] **Step 6: Correr os testes e confirmar que passam**

Run: `go test ./internal/domain/farmacia/ -v`
Expected: PASS. `gofmt -l internal/domain/farmacia/` vazio.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/farmacia/fornecedor.go internal/domain/farmacia/lote.go internal/domain/farmacia/repositorio_stock.go internal/domain/farmacia/fornecedor_test.go internal/domain/farmacia/lote_test.go
git commit -m "feat(farmacia): agregados Fornecedor e Lote e portas de stock

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Domínio — FEFO, tipos de movimento, dispensa na Receita e eventos

**Files:**
- Create: `internal/domain/farmacia/fefo.go`
- Modify: `internal/domain/farmacia/enums.go` (acrescentar `TipoMovimento`)
- Modify: `internal/domain/farmacia/receita.go` (acrescentar `RegistarDispensa`)
- Modify: `internal/domain/farmacia/eventos.go` (acrescentar 2 eventos)
- Test: `internal/domain/farmacia/fefo_test.go`
- Test: `internal/domain/farmacia/dispensa_receita_test.go`

**Interfaces:**
- Consumes: `erros`, `evento.EventoDominio`; o agregado `Receita`/`ItemReceita` e as consts `EstadoReceita` (Sprint 9).
- Produces:
  - `type LoteFEFO struct{ LoteID string; Disponivel int }`; `type AlocacaoFEFO struct{ LoteID string; Quantidade int }`; `AlocarFEFO(lotes []LoteFEFO, quantidade int) ([]AlocacaoFEFO, error)`.
  - `type TipoMovimento string` + consts `MovimentoEntrada`/`MovimentoSaidaDispensa`/`MovimentoSaidaVenda`/`MovimentoAjuste`/`MovimentoExpirado`/`MovimentoTransferencia`.
  - `(*Receita) RegistarDispensa(medicamentoID string, quantidade int) error`.
  - Eventos `EventoReceitaDispensada`, `StockEntrado`.

**Nota de colisão (importante):** a const `ReceitaDispensada` já existe (`EstadoReceita`). O evento da dispensa **tem** de se chamar `EventoReceitaDispensada` (prefixo `Evento`), tal como `EventoReceitaEmitida`/`EventoReceitaAnulada` já no `eventos.go`.

- [ ] **Step 1: Escrever os testes que falham**

`fefo_test.go`:
```go
package farmacia_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestAlocarFEFO_UmLote(t *testing.T) {
	alocs, err := farmacia.AlocarFEFO([]farmacia.LoteFEFO{{LoteID: "a", Disponivel: 50}}, 20)
	if err != nil || len(alocs) != 1 || alocs[0].Quantidade != 20 {
		t.Fatalf("alocação inesperada: %+v, %v", alocs, err)
	}
}

func TestAlocarFEFO_MultiplosLotes(t *testing.T) {
	// Ordem FEFO: consome 'a' (validade mais próxima) primeiro, depois 'b'.
	alocs, err := farmacia.AlocarFEFO([]farmacia.LoteFEFO{{LoteID: "a", Disponivel: 15}, {LoteID: "b", Disponivel: 30}}, 20)
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if len(alocs) != 2 || alocs[0].LoteID != "a" || alocs[0].Quantidade != 15 || alocs[1].LoteID != "b" || alocs[1].Quantidade != 5 {
		t.Fatalf("alocação FEFO errada: %+v", alocs)
	}
}

func TestAlocarFEFO_Insuficiente(t *testing.T) {
	_, err := farmacia.AlocarFEFO([]farmacia.LoteFEFO{{LoteID: "a", Disponivel: 5}}, 20)
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava RegraNegocio (stock insuficiente), obtive %v", err)
	}
}

func TestAlocarFEFO_QuantidadeInvalida(t *testing.T) {
	if _, err := farmacia.AlocarFEFO(nil, 0); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para quantidade zero")
	}
}
```

`dispensa_receita_test.go`:
```go
package farmacia_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func receitaComItem(t *testing.T, prescrita int) *farmacia.Receita {
	t.Helper()
	it, err := farmacia.NovoItemReceita("med-1", "1 comp 8/8h", nil, prescrita, "")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	emitida := time.Now()
	r, err := farmacia.NovaReceita("ep-1", "doente-1", "medico-1", []farmacia.ItemReceita{it}, "", emitida, emitida.AddDate(0, 0, 30))
	if err != nil {
		t.Fatalf("receita: %v", err)
	}
	return r
}

func TestRegistarDispensa_Parcial(t *testing.T) {
	r := receitaComItem(t, 20)
	if err := r.RegistarDispensa("med-1", 8); err != nil {
		t.Fatalf("dispensar: %v", err)
	}
	if r.Estado() != farmacia.ReceitaParcial {
		t.Fatalf("estado=%q, esperava PARCIAL", r.Estado())
	}
	if r.Snapshot().Itens[0].QuantidadeDispensada != 8 {
		t.Fatalf("quantidade dispensada=%d", r.Snapshot().Itens[0].QuantidadeDispensada)
	}
}

func TestRegistarDispensa_Total(t *testing.T) {
	r := receitaComItem(t, 20)
	if err := r.RegistarDispensa("med-1", 20); err != nil {
		t.Fatalf("dispensar: %v", err)
	}
	if r.Estado() != farmacia.ReceitaDispensada {
		t.Fatalf("estado=%q, esperava DISPENSADA", r.Estado())
	}
}

func TestRegistarDispensa_Excede(t *testing.T) {
	r := receitaComItem(t, 20)
	_ = r.RegistarDispensa("med-1", 15)
	if erros.CategoriaDe(r.RegistarDispensa("med-1", 10)) != erros.CategoriaRegraNegocio {
		t.Fatal("esperava RegraNegocio ao exceder o prescrito cumulativamente")
	}
}

func TestRegistarDispensa_MedicamentoAusente(t *testing.T) {
	r := receitaComItem(t, 20)
	if erros.CategoriaDe(r.RegistarDispensa("med-outro", 1)) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para medicamento fora da receita")
	}
}

func TestRegistarDispensa_ReceitaAnulada(t *testing.T) {
	r := receitaComItem(t, 20)
	_ = r.Anular()
	if erros.CategoriaDe(r.RegistarDispensa("med-1", 1)) != erros.CategoriaConflito {
		t.Fatal("esperava conflito ao dispensar uma receita não emitida/parcial")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falham**

Run: `go test ./internal/domain/farmacia/ -run 'AlocarFEFO|RegistarDispensa' -v`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Implementar `fefo.go`**

```go
package farmacia

import "github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"

// LoteFEFO é um lote candidato à alocação (já ordenado por validade ASC).
type LoteFEFO struct {
	LoteID     string
	Disponivel int
}

// AlocacaoFEFO é a quantidade a retirar de um lote.
type AlocacaoFEFO struct {
	LoteID     string
	Quantidade int
}

// AlocarFEFO aloca `quantidade` a partir dos lotes (já ordenados por validade
// ASC — o mais próximo a expirar primeiro), gulosamente. Devolve RegraNegocio se
// o total disponível não chegar.
func AlocarFEFO(lotes []LoteFEFO, quantidade int) ([]AlocacaoFEFO, error) {
	if quantidade <= 0 {
		return nil, erros.Novo(erros.CategoriaValidacao, "a quantidade a alocar deve ser positiva")
	}
	restante := quantidade
	alocacoes := make([]AlocacaoFEFO, 0, len(lotes))
	for _, l := range lotes {
		if restante == 0 {
			break
		}
		if l.Disponivel <= 0 {
			continue
		}
		usar := l.Disponivel
		if usar > restante {
			usar = restante
		}
		alocacoes = append(alocacoes, AlocacaoFEFO{LoteID: l.LoteID, Quantidade: usar})
		restante -= usar
	}
	if restante > 0 {
		return nil, erros.Novo(erros.CategoriaRegraNegocio, "stock insuficiente para a quantidade pedida")
	}
	return alocacoes, nil
}
```

- [ ] **Step 4: Acrescentar `TipoMovimento` a `enums.go`**

No fim de `internal/domain/farmacia/enums.go`, acrescenta:
```go

// TipoMovimento classifica um movimento de stock (DDM-001).
type TipoMovimento string

const (
	MovimentoEntrada       TipoMovimento = "ENTRADA"
	MovimentoSaidaDispensa TipoMovimento = "SAIDA_DISPENSA"
	MovimentoSaidaVenda    TipoMovimento = "SAIDA_VENDA"
	MovimentoAjuste        TipoMovimento = "AJUSTE"
	MovimentoExpirado      TipoMovimento = "EXPIRADO"
	MovimentoTransferencia TipoMovimento = "TRANSFERENCIA"
)
```

- [ ] **Step 5: Acrescentar `RegistarDispensa` a `receita.go`**

Acrescenta a `internal/domain/farmacia/receita.go` (a seguir ao método `Anular`):
```go
// RegistarDispensa regista a dispensa de `quantidade` de um medicamento da
// receita: valida que não excede o prescrito (cumulativamente) e recalcula o
// estado (DISPENSADA se tudo dispensado, senão PARCIAL). Só de EMITIDA/PARCIAL.
func (r *Receita) RegistarDispensa(medicamentoID string, quantidade int) error {
	if r.estado != ReceitaEmitida && r.estado != ReceitaParcial {
		return erros.Novo(erros.CategoriaConflito, "só é possível dispensar uma receita emitida ou parcial")
	}
	if quantidade <= 0 {
		return erros.Novo(erros.CategoriaValidacao, "a quantidade a dispensar deve ser positiva")
	}
	for i := range r.itens {
		if r.itens[i].MedicamentoID == medicamentoID {
			if r.itens[i].QuantidadeDispensada+quantidade > r.itens[i].QuantidadePrescrita {
				return erros.Novo(erros.CategoriaRegraNegocio, "a quantidade a dispensar excede a prescrita")
			}
			r.itens[i].QuantidadeDispensada += quantidade
			r.recalcularEstadoDispensa()
			return nil
		}
	}
	return erros.Novo(erros.CategoriaValidacao, "o medicamento não consta da receita")
}

// recalcularEstadoDispensa põe a receita em DISPENSADA se todos os itens estão
// totalmente dispensados, senão em PARCIAL.
func (r *Receita) recalcularEstadoDispensa() {
	for _, it := range r.itens {
		if it.QuantidadeDispensada < it.QuantidadePrescrita {
			r.estado = ReceitaParcial
			return
		}
	}
	r.estado = ReceitaDispensada
}
```
(o import `erros` já existe em `receita.go`.)

- [ ] **Step 6: Acrescentar os eventos a `eventos.go`**

Acrescenta a `internal/domain/farmacia/eventos.go` (antes do bloco `var (...)` de garantias, e adiciona as duas novas linhas a esse bloco):
```go
// EventoReceitaDispensada é emitido quando uma receita é (parcial ou totalmente) dispensada.
type EventoReceitaDispensada struct {
	ReceitaID string
	Em        time.Time
}

func (e EventoReceitaDispensada) NomeEvento() string    { return "farmacia.receita.dispensada" }
func (e EventoReceitaDispensada) OcorridoEm() time.Time { return e.Em }

// StockEntrado é emitido quando entra um lote de stock.
type StockEntrado struct {
	LoteID        string
	MedicamentoID string
	Em            time.Time
}

func (e StockEntrado) NomeEvento() string    { return "farmacia.stock.entrada" }
func (e StockEntrado) OcorridoEm() time.Time { return e.Em }
```
E no bloco `var ( _ evento.EventoDominio = ... )` acrescenta:
```go
	_ evento.EventoDominio = EventoReceitaDispensada{}
	_ evento.EventoDominio = StockEntrado{}
```

- [ ] **Step 7: Correr os testes e a cobertura do domínio**

Run: `go test ./internal/domain/farmacia/ -v`
Expected: PASS.
Run: `bash scripts/cobertura.sh` (domínio ≥85% — linha real). Se abaixo, acrescenta casos (ex.: AlocarFEFO com lote de disponível 0 saltado; RegistarDispensa que completa em duas dispensas → DISPENSADA).

- [ ] **Step 8: Commit**

```bash
git add internal/domain/farmacia/fefo.go internal/domain/farmacia/enums.go internal/domain/farmacia/receita.go internal/domain/farmacia/eventos.go internal/domain/farmacia/fefo_test.go internal/domain/farmacia/dispensa_receita_test.go
git commit -m "feat(farmacia): alocação FEFO, tipos de movimento e dispensa na Receita

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Aplicação — portas/DTOs, Fornecedor, entrada de stock e consulta

**Files:**
- Modify: `internal/application/farmacia/ports.go` (acrescentar DTOs de stock + porta `MotorDispensa`)
- Modify: `internal/application/farmacia/mapa.go` (acrescentar `paraDetalheFornecedor`, `paraDetalheLote`)
- Create: `internal/application/farmacia/stock.go`
- Test: `internal/application/farmacia/fakes_stock_test.go`
- Test: `internal/application/farmacia/stock_test.go`

**Interfaces:**
- Consumes: domínio `farmacia` (Tasks 3-4); `auditoria.Registo`; `RepositorioMedicamentos` (existente).
- Produces:
  - Porta `MotorDispensa`; `ItemDispensa`.
  - Reexports `FiltroFornecedores`/`PaginaFornecedores`/`ResumoFornecedor`, `ResumoLote`.
  - DTOs `DadosNovoFornecedor`, `DetalheFornecedor`, `DadosEntradaStock`, `DetalheLote`, `StockDTO`, `ItemDispensaDTO`, `DadosDispensa`.
  - `paraDetalheFornecedor`, `paraDetalheLote`.
  - `CasoRegistarFornecedor`, `CasoListarFornecedores`, `CasoRegistarEntradaStock`, `CasoConsultarStock`, `CasoListarLotes`.

- [ ] **Step 1: Escrever os fakes e os testes que falham**

`fakes_stock_test.go`:
```go
package farmacia_test

import (
	"context"
	"strconv"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeRepoFornecedores em memória.
type fakeRepoFornecedores struct {
	porID  map[string]*farmacia.Fornecedor
	seq    int
	pagina farmacia.PaginaFornecedores
}

func novoFakeRepoFornecedores() *fakeRepoFornecedores {
	return &fakeRepoFornecedores{porID: map[string]*farmacia.Fornecedor{}}
}
func (f *fakeRepoFornecedores) Guardar(_ context.Context, forn *farmacia.Fornecedor) (string, error) {
	snap := forn.Snapshot()
	id := snap.ID
	if id == "" {
		f.seq++
		id = "forn-" + strconv.Itoa(f.seq)
		snap.ID = id
	}
	f.porID[id] = farmacia.ReconstruirFornecedor(snap)
	return id, nil
}
func (f *fakeRepoFornecedores) ObterPorID(_ context.Context, id string) (*farmacia.Fornecedor, error) {
	forn, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "fornecedor não encontrado")
	}
	return forn, nil
}
func (f *fakeRepoFornecedores) Listar(_ context.Context, _ farmacia.FiltroFornecedores) (farmacia.PaginaFornecedores, error) {
	return f.pagina, nil
}

// fakeRepoLotes em memória.
type fakeRepoLotes struct {
	porID   map[string]*farmacia.Lote
	seq     int
	stock   int
	lotes   []farmacia.ResumoLote
	entrErr error
}

func novoFakeRepoLotes() *fakeRepoLotes {
	return &fakeRepoLotes{porID: map[string]*farmacia.Lote{}}
}
func (f *fakeRepoLotes) RegistarEntrada(_ context.Context, l *farmacia.Lote, _ string) (string, error) {
	if f.entrErr != nil {
		return "", f.entrErr
	}
	snap := l.Snapshot()
	f.seq++
	id := "lote-" + strconv.Itoa(f.seq)
	snap.ID = id
	f.porID[id] = farmacia.ReconstruirLote(snap)
	return id, nil
}
func (f *fakeRepoLotes) ObterPorID(_ context.Context, id string) (*farmacia.Lote, error) {
	l, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "lote não encontrado")
	}
	return l, nil
}
func (f *fakeRepoLotes) ListarPorMedicamento(_ context.Context, _ string, _ bool) ([]farmacia.ResumoLote, error) {
	return f.lotes, nil
}
func (f *fakeRepoLotes) StockDisponivel(_ context.Context, _ string) (int, error) {
	return f.stock, nil
}
```

`stock_test.go`:
```go
package farmacia_test

import (
	"context"
	"testing"
	"time"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRegistarFornecedor(t *testing.T) {
	repo := novoFakeRepoFornecedores()
	aud := &fakeAuditor{}
	out, err := appfarmacia.NovoCasoRegistarFornecedor(repo, aud).Executar(context.Background(), "farm-1", appfarmacia.DadosNovoFornecedor{Nome: "Farmédica"})
	if err != nil {
		t.Fatalf("registar: %v", err)
	}
	if out.ID == "" || out.Nome != "Farmédica" {
		t.Fatalf("saída inesperada: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.fornecedor.registado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func dadosEntrada() appfarmacia.DadosEntradaStock {
	return appfarmacia.DadosEntradaStock{
		MedicamentoID: "med-1", NumeroLote: "L001", Validade: time.Now().AddDate(1, 0, 0),
		Quantidade: 100, PrecoUnitarioCusto: "12.5000",
	}
}

func TestRegistarEntradaStock(t *testing.T) {
	repoLotes := novoFakeRepoLotes()
	repoMed := novoFakeRepoMed()
	medID, _ := repoMed.Guardar(context.Background(), medicamentoParaRepo(t))
	repoForn := novoFakeRepoFornecedores()
	aud := &fakeAuditor{}
	caso := appfarmacia.NovoCasoRegistarEntradaStock(repoLotes, repoMed, repoForn, aud)

	dados := dadosEntrada()
	dados.MedicamentoID = medID
	out, err := caso.Executar(context.Background(), "farm-1", dados)
	if err != nil {
		t.Fatalf("entrada: %v", err)
	}
	if out.ID == "" || out.QuantidadeActual != 100 {
		t.Fatalf("saída inesperada: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.stock.entrada" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestRegistarEntradaStock_MedicamentoInexistente(t *testing.T) {
	caso := appfarmacia.NovoCasoRegistarEntradaStock(novoFakeRepoLotes(), novoFakeRepoMed(), novoFakeRepoFornecedores(), &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "farm-1", dadosEntrada()); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestConsultarStock(t *testing.T) {
	repoLotes := novoFakeRepoLotes()
	repoLotes.stock = 250
	out, err := appfarmacia.NovoCasoConsultarStock(repoLotes).Executar(context.Background(), "med-1")
	if err != nil || out.Disponivel != 250 {
		t.Fatalf("stock inesperado: %+v, %v", out, err)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falham**

Run: `go test ./internal/application/farmacia/ -v`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Acrescentar os DTOs e a porta a `ports.go`**

No fim de `internal/application/farmacia/ports.go` acrescenta (o import `time` e `dominio` já existem):
```go
// --- Stock & Dispensa ---

// ItemDispensa é uma linha de dispensa (medicamento + quantidade) passada ao motor.
type ItemDispensa struct {
	MedicamentoID string
	Quantidade    int
}

// MotorDispensa é a porta transaccional da dispensa: aloca stock por FEFO, regista
// os movimentos e persiste a receita, atomicamente.
type MotorDispensa interface {
	Dispensar(ctx context.Context, receita dominio.SnapshotReceita, itens []ItemDispensa, realizadoPor string) ([]dominio.AlocacaoFEFO, error)
}

// Reexports dos read-models de stock.
type (
	FiltroFornecedores = dominio.FiltroFornecedores
	PaginaFornecedores = dominio.PaginaFornecedores
	ResumoFornecedor   = dominio.ResumoFornecedor
	ResumoLote         = dominio.ResumoLote
)

// DadosNovoFornecedor é a entrada do registo de fornecedor.
type DadosNovoFornecedor struct {
	Nome     string  `json:"nome"`
	NIF      *string `json:"nif"`
	Contacto *string `json:"contacto"`
}

// DetalheFornecedor é o detalhe de um fornecedor numa resposta.
type DetalheFornecedor struct {
	ID       string    `json:"id"`
	Nome     string    `json:"nome"`
	NIF      *string   `json:"nif,omitempty"`
	Contacto *string   `json:"contacto,omitempty"`
	Activo   bool      `json:"activo"`
	CriadoEm time.Time `json:"criado_em"`
}

// DadosEntradaStock é a entrada de um lote de stock (UC-FAR-01).
type DadosEntradaStock struct {
	MedicamentoID      string
	NumeroLote         string
	Validade           time.Time
	Quantidade         int
	PrecoUnitarioCusto string
	FornecedorID       *string
	Notas              string
}

// DetalheLote é o detalhe de um lote numa resposta.
type DetalheLote struct {
	ID                 string    `json:"id"`
	MedicamentoID      string    `json:"medicamento_id"`
	NumeroLote         string    `json:"numero_lote"`
	Validade           time.Time `json:"validade"`
	QuantidadeInicial  int       `json:"quantidade_inicial"`
	QuantidadeActual   int       `json:"quantidade_actual"`
	PrecoUnitarioCusto string    `json:"preco_unit_custo"`
	FornecedorID       *string   `json:"fornecedor_id,omitempty"`
	EntradaEm          time.Time `json:"entrada_em"`
	Notas              string    `json:"notas,omitempty"`
}

// StockDTO é o stock disponível total de um medicamento.
type StockDTO struct {
	MedicamentoID string `json:"medicamento_id"`
	Disponivel    int    `json:"disponivel"`
}

// ItemDispensaDTO é um item num pedido de dispensa.
type ItemDispensaDTO struct {
	MedicamentoID string `json:"medicamento_id"`
	Quantidade    int    `json:"quantidade"`
}

// DadosDispensa é a entrada da dispensa de uma receita.
type DadosDispensa struct {
	Itens                []ItemDispensaDTO
	IgnorarAlertaAlergia bool
	JustificacaoAlerta   string
}
```

- [ ] **Step 4: Acrescentar os mapeamentos a `mapa.go`**

No fim de `internal/application/farmacia/mapa.go` acrescenta:
```go
// paraDetalheFornecedor mapeia o agregado Fornecedor para o DTO.
func paraDetalheFornecedor(f *dominio.Fornecedor) DetalheFornecedor {
	s := f.Snapshot()
	return DetalheFornecedor{ID: s.ID, Nome: s.Nome, NIF: s.NIF, Contacto: s.Contacto, Activo: s.Activo, CriadoEm: s.CriadoEm}
}

// paraDetalheLote mapeia o agregado Lote para o DTO.
func paraDetalheLote(l *dominio.Lote) DetalheLote {
	s := l.Snapshot()
	return DetalheLote{
		ID: s.ID, MedicamentoID: s.MedicamentoID, NumeroLote: s.NumeroLote, Validade: s.Validade,
		QuantidadeInicial: s.QuantidadeInicial, QuantidadeActual: s.QuantidadeActual,
		PrecoUnitarioCusto: s.PrecoUnitarioCusto, FornecedorID: s.FornecedorID, EntradaEm: s.EntradaEm, Notas: s.Notas,
	}
}
```

- [ ] **Step 5: Implementar `stock.go`**

```go
package farmacia

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoRegistarFornecedor regista um fornecedor e audita.
type CasoRegistarFornecedor struct {
	repo    dominio.RepositorioFornecedores
	auditor Auditor
	agora   func() time.Time
}

func NovoCasoRegistarFornecedor(r dominio.RepositorioFornecedores, aud Auditor) *CasoRegistarFornecedor {
	return &CasoRegistarFornecedor{repo: r, auditor: aud, agora: time.Now}
}
func (c *CasoRegistarFornecedor) Executar(ctx context.Context, actor string, dados DadosNovoFornecedor) (DetalheFornecedor, error) {
	f, err := dominio.NovoFornecedor(dados.Nome, dados.NIF, dados.Contacto)
	if err != nil {
		return DetalheFornecedor{}, err
	}
	id, err := c.repo.Guardar(ctx, f)
	if err != nil {
		return DetalheFornecedor{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.fornecedor.registado", Entidade: "fornecedor", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheFornecedor{}, err
	}
	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheFornecedor{}, err
	}
	return paraDetalheFornecedor(final), nil
}

// CasoListarFornecedores lista fornecedores (não audita).
type CasoListarFornecedores struct {
	repo dominio.RepositorioFornecedores
}

func NovoCasoListarFornecedores(r dominio.RepositorioFornecedores) *CasoListarFornecedores {
	return &CasoListarFornecedores{repo: r}
}
func (c *CasoListarFornecedores) Executar(ctx context.Context, filtro FiltroFornecedores) (PaginaFornecedores, error) {
	if filtro.Limite <= 0 {
		filtro.Limite = limiteDefault
	}
	if filtro.Limite > limiteMaximo {
		filtro.Limite = limiteMaximo
	}
	if filtro.Deslocamento < 0 {
		filtro.Deslocamento = 0
	}
	return c.repo.Listar(ctx, filtro)
}

// CasoRegistarEntradaStock dá entrada de um lote de stock (UC-FAR-01) e audita.
type CasoRegistarEntradaStock struct {
	lotes        dominio.RepositorioLotes
	medicamentos dominio.RepositorioMedicamentos
	fornecedores dominio.RepositorioFornecedores
	auditor      Auditor
	agora        func() time.Time
}

func NovoCasoRegistarEntradaStock(lotes dominio.RepositorioLotes, medicamentos dominio.RepositorioMedicamentos, fornecedores dominio.RepositorioFornecedores, aud Auditor) *CasoRegistarEntradaStock {
	return &CasoRegistarEntradaStock{lotes: lotes, medicamentos: medicamentos, fornecedores: fornecedores, auditor: aud, agora: time.Now}
}
func (c *CasoRegistarEntradaStock) Executar(ctx context.Context, actor string, dados DadosEntradaStock) (DetalheLote, error) {
	med, err := c.medicamentos.ObterPorID(ctx, dados.MedicamentoID)
	if err != nil {
		return DetalheLote{}, err
	}
	if !med.Activo() {
		return DetalheLote{}, erros.Novo(erros.CategoriaConflito, "não é possível dar entrada de stock a um medicamento inactivo")
	}
	if dados.FornecedorID != nil && *dados.FornecedorID != "" {
		if _, err := c.fornecedores.ObterPorID(ctx, *dados.FornecedorID); err != nil {
			return DetalheLote{}, err
		}
	}
	lote, err := dominio.NovoLote(dados.MedicamentoID, dados.NumeroLote, dados.Validade, dados.Quantidade, dados.PrecoUnitarioCusto, dados.FornecedorID, dados.Notas)
	if err != nil {
		return DetalheLote{}, err
	}
	id, err := c.lotes.RegistarEntrada(ctx, lote, actor)
	if err != nil {
		return DetalheLote{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.stock.entrada", Entidade: "lote", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheLote{}, err
	}
	final, err := c.lotes.ObterPorID(ctx, id)
	if err != nil {
		return DetalheLote{}, err
	}
	return paraDetalheLote(final), nil
}

// CasoConsultarStock devolve o stock disponível de um medicamento (UC-FAR-05).
type CasoConsultarStock struct {
	lotes dominio.RepositorioLotes
}

func NovoCasoConsultarStock(lotes dominio.RepositorioLotes) *CasoConsultarStock {
	return &CasoConsultarStock{lotes: lotes}
}
func (c *CasoConsultarStock) Executar(ctx context.Context, medicamentoID string) (StockDTO, error) {
	total, err := c.lotes.StockDisponivel(ctx, medicamentoID)
	if err != nil {
		return StockDTO{}, err
	}
	return StockDTO{MedicamentoID: medicamentoID, Disponivel: total}, nil
}

// CasoListarLotes lista os lotes de um medicamento.
type CasoListarLotes struct {
	lotes dominio.RepositorioLotes
}

func NovoCasoListarLotes(lotes dominio.RepositorioLotes) *CasoListarLotes {
	return &CasoListarLotes{lotes: lotes}
}
func (c *CasoListarLotes) Executar(ctx context.Context, medicamentoID string, apenasDisponiveis bool) ([]ResumoLote, error) {
	return c.lotes.ListarPorMedicamento(ctx, medicamentoID, apenasDisponiveis)
}
```

- [ ] **Step 6: Correr os testes e a cobertura**

Run: `go test ./internal/application/farmacia/ -v`
Expected: PASS.
Run: `bash scripts/cobertura.sh` (aplicação ≥75% — linha real); `staticcheck ./internal/application/farmacia/...` (sem avisos).

- [ ] **Step 7: Commit**

```bash
git add internal/application/farmacia/ports.go internal/application/farmacia/mapa.go internal/application/farmacia/stock.go internal/application/farmacia/fakes_stock_test.go internal/application/farmacia/stock_test.go
git commit -m "feat(farmacia): casos de uso de fornecedor, entrada e consulta de stock

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Aplicação — dispensa da receita (FEFO + alergias, transaccional)

**Files:**
- Create: `internal/application/farmacia/dispensa.go`
- Test: `internal/application/farmacia/dispensa_test.go`

**Interfaces:**
- Consumes: `RepositorioReceitas`, `RepositorioMedicamentos`, `LeitorClinico`, `MotorDispensa`, `Auditor`, `paraDetalheReceita` (Sprint 9 + Task 5); domínio `Receita.RegistarDispensa`, `EstadoEfectivo`, `Medicamento.CorrespondeSubstancia`/`CodigoInterno`.
- Produces: `CasoDispensarReceita` — `NovoCasoDispensarReceita(receitas, medicamentos, leitor, motor, aud)`, `Executar(ctx, actor, receitaID string, dados DadosDispensa) (DetalheReceita, error)`.

**Regras (medicoID já na receita; realizado_por = actor):**
1. Carrega a receita; se `EstadoEfectivo(agora)==EXPIRADA` ou estado ∉ {EMITIDA, PARCIAL} → `CategoriaConflito` (RN-FAR-07).
2. Para cada item: `receita.RegistarDispensa(medicamentoID, quantidade)` — valida não-exceder (RN-FAR-05) e actualiza o estado em memória.
3. Alergias (RN-FAR-04): `LeitorClinico.ObterContextoDoente(doenteID)` (graves) × `medicamento.CorrespondeSubstancia` → bloqueio 422; override `IgnorarAlertaAlergia` + `JustificacaoAlerta` (obrigatória).
4. `MotorDispensa.Dispensar(ctx, receita.Snapshot(), itens, actor)` (RegraNegocio se stock insuficiente).
5. Audita `farmacia.receita.dispensada` (Detalhe com override quando aplicável); re-lê e devolve `paraDetalheReceita`.

- [ ] **Step 1: Escrever os fakes e o teste que falha (`dispensa_test.go`)**

```go
package farmacia_test

import (
	"context"
	"testing"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeMotorDispensa simula a persistência transaccional: se tiver um repo,
// guarda nele a receita recebida (como o motor real faria), para que o re-ler do
// caso de uso reflicta o novo estado. Devolve um erro configurável.
type fakeMotorDispensa struct {
	err  error
	repo *fakeRepoReceitas
}

func (m *fakeMotorDispensa) Dispensar(_ context.Context, receita dominio.SnapshotReceita, _ []appfarmacia.ItemDispensa, _ string) ([]dominio.AlocacaoFEFO, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.repo != nil {
		m.repo.porID[receita.ID] = dominio.ReconstruirReceita(receita)
	}
	return []dominio.AlocacaoFEFO{}, nil
}

// prepararDispensa emite uma receita (via o fluxo do Sprint 9) e devolve o que é
// preciso para dispensar. Reutiliza prepararEmissao/dadosReceita de receitas_test.go.
func prepararDispensa(t *testing.T) (*fakeRepoReceitas, *fakeRepoMed, *fakeLeitorClinico, string, string) {
	t.Helper()
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	emitida, err := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{}).Executar(context.Background(), "medico-1", dadosReceita(medID))
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	return repoRec, repoMed, leitor, emitida.ID, medID
}

func dispensa(medID string, qtd int) appfarmacia.DadosDispensa {
	return appfarmacia.DadosDispensa{Itens: []appfarmacia.ItemDispensaDTO{{MedicamentoID: medID, Quantidade: qtd}}}
}

func TestDispensar_Parcial(t *testing.T) {
	repoRec, repoMed, leitor, recID, medID := prepararDispensa(t)
	aud := &fakeAuditor{}
	// O motor persiste o snapshot no repo, para o re-ler reflectir PARCIAL.
	caso := appfarmacia.NovoCasoDispensarReceita(repoRec, repoMed, leitor, &fakeMotorDispensa{repo: repoRec}, aud)
	out, err := caso.Executar(context.Background(), "farm-1", recID, dispensa(medID, 5)) // prescrita=20
	if err != nil {
		t.Fatalf("dispensar: %v", err)
	}
	if out.Estado != "PARCIAL" {
		t.Fatalf("estado=%q, esperava PARCIAL", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.receita.dispensada" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestDispensar_Excede(t *testing.T) {
	repoRec, repoMed, leitor, recID, medID := prepararDispensa(t)
	caso := appfarmacia.NovoCasoDispensarReceita(repoRec, repoMed, leitor, &fakeMotorDispensa{}, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "farm-1", recID, dispensa(medID, 25)); erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava RegraNegocio (excede prescrito), obtive %v", err)
	}
}

func TestDispensar_AlergiaBloqueia(t *testing.T) {
	repoRec, repoMed, leitor, recID, medID := prepararDispensa(t)
	leitor.alergias = []appfarmacia.AlergiaClinica{{Substancia: "Amoxicilina", Severidade: "ANAFILACTICA"}}
	caso := appfarmacia.NovoCasoDispensarReceita(repoRec, repoMed, leitor, &fakeMotorDispensa{}, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "farm-1", recID, dispensa(medID, 5)); erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava RegraNegocio (alergia), obtive %v", err)
	}
}

func TestDispensar_OverrideComJustificacao(t *testing.T) {
	repoRec, repoMed, leitor, recID, medID := prepararDispensa(t)
	leitor.alergias = []appfarmacia.AlergiaClinica{{Substancia: "Amoxicilina", Severidade: "GRAVE"}}
	d := dispensa(medID, 5)
	d.IgnorarAlertaAlergia = true
	d.JustificacaoAlerta = "Doente monitorizado."
	aud := &fakeAuditor{}
	if _, err := appfarmacia.NovoCasoDispensarReceita(repoRec, repoMed, leitor, &fakeMotorDispensa{}, aud).Executar(context.Background(), "farm-1", recID, d); err != nil {
		t.Fatalf("dispensar com override: %v", err)
	}
	if len(aud.registos) != 1 || aud.registos[0].Detalhe == "" {
		t.Fatalf("esperava auditoria com detalhe do override: %+v", aud.registos)
	}
}

func TestDispensar_StockInsuficiente(t *testing.T) {
	repoRec, repoMed, leitor, recID, medID := prepararDispensa(t)
	motor := &fakeMotorDispensa{err: erros.Novo(erros.CategoriaRegraNegocio, "stock insuficiente")}
	caso := appfarmacia.NovoCasoDispensarReceita(repoRec, repoMed, leitor, motor, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "farm-1", recID, dispensa(medID, 5)); erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava RegraNegocio (stock), obtive %v", err)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/farmacia/ -run Dispensar -v`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Implementar `dispensa.go`**

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

// CasoDispensarReceita dispensa (parcial ou totalmente) uma receita: valida
// não-exceder e alergias, e consome stock por FEFO via o MotorDispensa (atómico).
type CasoDispensarReceita struct {
	receitas     dominio.RepositorioReceitas
	medicamentos dominio.RepositorioMedicamentos
	leitor       LeitorClinico
	motor        MotorDispensa
	auditor      Auditor
	agora        func() time.Time
}

func NovoCasoDispensarReceita(receitas dominio.RepositorioReceitas, medicamentos dominio.RepositorioMedicamentos, leitor LeitorClinico, motor MotorDispensa, aud Auditor) *CasoDispensarReceita {
	return &CasoDispensarReceita{receitas: receitas, medicamentos: medicamentos, leitor: leitor, motor: motor, auditor: aud, agora: time.Now}
}

func (c *CasoDispensarReceita) Executar(ctx context.Context, actor, receitaID string, dados DadosDispensa) (DetalheReceita, error) {
	receita, err := c.receitas.ObterPorID(ctx, receitaID)
	if err != nil {
		return DetalheReceita{}, err
	}
	efectivo := receita.EstadoEfectivo(c.agora())
	if efectivo != dominio.ReceitaEmitida && efectivo != dominio.ReceitaParcial {
		return DetalheReceita{}, erros.Novo(erros.CategoriaConflito, "esta receita não pode ser dispensada (expirada, anulada ou já dispensada)")
	}
	if len(dados.Itens) == 0 {
		return DetalheReceita{}, erros.Novo(erros.CategoriaValidacao, "indique pelo menos um item a dispensar")
	}

	for _, it := range dados.Itens {
		if err := receita.RegistarDispensa(it.MedicamentoID, it.Quantidade); err != nil {
			return DetalheReceita{}, err
		}
	}

	_, alergiasGraves, err := c.leitor.ObterContextoDoente(ctx, receita.DoenteID())
	if err != nil {
		return DetalheReceita{}, err
	}
	var alertas []string
	itensMotor := make([]ItemDispensa, 0, len(dados.Itens))
	for _, it := range dados.Itens {
		med, err := c.medicamentos.ObterPorID(ctx, it.MedicamentoID)
		if err != nil {
			return DetalheReceita{}, err
		}
		itensMotor = append(itensMotor, ItemDispensa{MedicamentoID: it.MedicamentoID, Quantidade: it.Quantidade})
		for _, a := range alergiasGraves {
			if med.CorrespondeSubstancia(a.Substancia) {
				alertas = append(alertas, fmt.Sprintf("%s (alergia %s a %s)", med.CodigoInterno(), a.Severidade, a.Substancia))
			}
		}
	}
	if len(alertas) > 0 {
		if !dados.IgnorarAlertaAlergia {
			return DetalheReceita{}, erros.Novo(erros.CategoriaRegraNegocio, "a dispensa colide com alergias graves do doente: "+strings.Join(alertas, "; "))
		}
		if strings.TrimSpace(dados.JustificacaoAlerta) == "" {
			return DetalheReceita{}, erros.Novo(erros.CategoriaValidacao, "é obrigatória uma justificação para ignorar o alerta de alergia")
		}
	}

	if _, err := c.motor.Dispensar(ctx, receita.Snapshot(), itensMotor, actor); err != nil {
		return DetalheReceita{}, err
	}

	detalheAud := ""
	if len(alertas) > 0 {
		detalheAud = "override alergia: " + dados.JustificacaoAlerta + " | alertas: " + strings.Join(alertas, "; ")
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.receita.dispensada", Entidade: "receita", EntidadeID: receitaID, Detalhe: detalheAud, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheReceita{}, err
	}
	final, err := c.receitas.ObterPorID(ctx, receitaID)
	if err != nil {
		return DetalheReceita{}, err
	}
	return paraDetalheReceita(final, c.agora()), nil
}
```

- [ ] **Step 4: Correr os testes e a cobertura**

Run: `go test ./internal/application/farmacia/ -v`
Expected: PASS.
Run: `bash scripts/cobertura.sh` (aplicação ≥75% — linha real); `staticcheck ./internal/application/farmacia/...` (sem avisos).

- [ ] **Step 5: Commit**

```bash
git add internal/application/farmacia/dispensa.go internal/application/farmacia/dispensa_test.go
git commit -m "feat(farmacia): caso de uso de dispensa da receita (FEFO + alergias)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Repositórios pgx de fornecedores e lotes + integração

**Files:**
- Create: `internal/adapters/pgrepo/fornecedores_repo.go`
- Create: `internal/adapters/pgrepo/lotes_repo.go`
- Test: `tests/integration/stock_test.go` (tag `integration`)

**Interfaces:**
- Consumes: domínio `farmacia` (`Fornecedor`/`Snapshot`/`ReconstruirFornecedor`, `Lote`/`Snapshot`/`ReconstruirLote`, `FiltroFornecedores`/`PaginaFornecedores`/`ResumoFornecedor`, `ResumoLote`, `MovimentoEntrada`); `erros`; `pgx`/`pgxpool`/`pgconn`. Helper `deref` já existe.
- Produces: `RepositorioFornecedores` (`NovoRepositorioFornecedores(pool)`) e `RepositorioLotes` (`NovoRepositorioLotes(pool)`).

**Padrão:** ver `medicamentos_repo.go`. `RegistarEntrada` é transaccional (lote + movimento `ENTRADA`). `preco_unit_custo` escrito com `$n::numeric`, lido com `preco_unit_custo::text`. Lote duplicado (23505) → `CategoriaConflito`.

- [ ] **Step 1: Implementar `fornecedores_repo.go`**

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

// RepositorioFornecedores implementa dominio.RepositorioFornecedores com pgx.
type RepositorioFornecedores struct {
	pool *pgxpool.Pool
}

func NovoRepositorioFornecedores(pool *pgxpool.Pool) *RepositorioFornecedores {
	return &RepositorioFornecedores{pool: pool}
}

func (r *RepositorioFornecedores) Guardar(ctx context.Context, f *dominio.Fornecedor) (string, error) {
	s := f.Snapshot()
	if s.ID == "" {
		const q = `INSERT INTO farmacia.fornecedores (nome, nif, contacto, activo)
		           VALUES ($1, NULLIF($2,''), NULLIF($3,''), $4) RETURNING id::text`
		var id string
		if err := r.pool.QueryRow(ctx, q, s.Nome, deref(s.NIF), deref(s.Contacto), s.Activo).Scan(&id); err != nil {
			return "", fmt.Errorf("inserir fornecedor: %w", err)
		}
		return id, nil
	}
	const q = `UPDATE farmacia.fornecedores SET nome=$2, nif=NULLIF($3,''), contacto=NULLIF($4,''), activo=$5 WHERE id=$1`
	ct, err := r.pool.Exec(ctx, q, s.ID, s.Nome, deref(s.NIF), deref(s.Contacto), s.Activo)
	if err != nil {
		return "", fmt.Errorf("actualizar fornecedor: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", erros.Novo(erros.CategoriaNaoEncontrado, "fornecedor não encontrado")
	}
	return s.ID, nil
}

func (r *RepositorioFornecedores) ObterPorID(ctx context.Context, id string) (*dominio.Fornecedor, error) {
	const q = `SELECT id::text, nome, nif, contacto, activo, criado_em FROM farmacia.fornecedores WHERE id=$1`
	var s dominio.SnapshotFornecedor
	if err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.Nome, &s.NIF, &s.Contacto, &s.Activo, &s.CriadoEm); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "fornecedor não encontrado")
		}
		return nil, fmt.Errorf("obter fornecedor: %w", err)
	}
	return dominio.ReconstruirFornecedor(s), nil
}

func (r *RepositorioFornecedores) Listar(ctx context.Context, f dominio.FiltroFornecedores) (dominio.PaginaFornecedores, error) {
	base := `FROM farmacia.fornecedores WHERE ($1='' OR nome ILIKE '%'||$1||'%') AND ($2 = false OR activo)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+base, f.Termo, f.ApenasActivos).Scan(&total); err != nil {
		return dominio.PaginaFornecedores{}, fmt.Errorf("contar fornecedores: %w", err)
	}
	q := `SELECT id::text, nome, nif, activo ` + base + ` ORDER BY nome LIMIT $3 OFFSET $4`
	linhas, err := r.pool.Query(ctx, q, f.Termo, f.ApenasActivos, f.Limite, f.Deslocamento)
	if err != nil {
		return dominio.PaginaFornecedores{}, fmt.Errorf("listar fornecedores: %w", err)
	}
	defer linhas.Close()
	pagina := dominio.PaginaFornecedores{Total: total, Limite: f.Limite, Deslocamento: f.Deslocamento, Itens: []dominio.ResumoFornecedor{}}
	for linhas.Next() {
		var it dominio.ResumoFornecedor
		if err := linhas.Scan(&it.ID, &it.Nome, &it.NIF, &it.Activo); err != nil {
			return dominio.PaginaFornecedores{}, fmt.Errorf("ler fornecedor: %w", err)
		}
		pagina.Itens = append(pagina.Itens, it)
	}
	return pagina, linhas.Err()
}
```

- [ ] **Step 2: Implementar `lotes_repo.go`**

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

// RepositorioLotes implementa dominio.RepositorioLotes com pgx.
type RepositorioLotes struct {
	pool *pgxpool.Pool
}

func NovoRepositorioLotes(pool *pgxpool.Pool) *RepositorioLotes {
	return &RepositorioLotes{pool: pool}
}

// RegistarEntrada insere o lote e o movimento ENTRADA numa só transacção.
func (r *RepositorioLotes) RegistarEntrada(ctx context.Context, l *dominio.Lote, realizadoPor string) (string, error) {
	s := l.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const qLote = `
INSERT INTO farmacia.lotes (medicamento_id, numero_lote, validade, quantidade_inicial, quantidade_actual, preco_unit_custo, fornecedor_id, notas)
VALUES ($1,$2,$3,$4,$5,$6::numeric,$7,NULLIF($8,'')) RETURNING id::text`
	var id string
	if err := tx.QueryRow(ctx, qLote,
		s.MedicamentoID, s.NumeroLote, s.Validade, s.QuantidadeInicial, s.QuantidadeActual,
		s.PrecoUnitarioCusto, s.FornecedorID, s.Notas,
	).Scan(&id); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", erros.Novo(erros.CategoriaConflito, "já existe um lote com este número para o medicamento e fornecedor")
		}
		return "", fmt.Errorf("inserir lote: %w", err)
	}
	const qMov = `
INSERT INTO farmacia.movimentos_stock (tipo, medicamento_id, lote_id, quantidade, realizado_por)
VALUES ($1,$2,$3,$4,$5)`
	if _, err := tx.Exec(ctx, qMov, string(dominio.MovimentoEntrada), s.MedicamentoID, id, s.QuantidadeInicial, realizadoPor); err != nil {
		return "", fmt.Errorf("registar movimento de entrada: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar transacção: %w", err)
	}
	return id, nil
}

func (r *RepositorioLotes) ObterPorID(ctx context.Context, id string) (*dominio.Lote, error) {
	const q = `
SELECT id::text, medicamento_id::text, numero_lote, validade, quantidade_inicial, quantidade_actual,
       preco_unit_custo::text, fornecedor_id::text, entrada_em, COALESCE(notas,'')
FROM farmacia.lotes WHERE id=$1`
	var s dominio.SnapshotLote
	if err := r.pool.QueryRow(ctx, q, id).Scan(
		&s.ID, &s.MedicamentoID, &s.NumeroLote, &s.Validade, &s.QuantidadeInicial, &s.QuantidadeActual,
		&s.PrecoUnitarioCusto, &s.FornecedorID, &s.EntradaEm, &s.Notas,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "lote não encontrado")
		}
		return nil, fmt.Errorf("obter lote: %w", err)
	}
	return dominio.ReconstruirLote(s), nil
}

func (r *RepositorioLotes) ListarPorMedicamento(ctx context.Context, medicamentoID string, apenasDisponiveis bool) ([]dominio.ResumoLote, error) {
	q := `SELECT id::text, numero_lote, validade, quantidade_actual, fornecedor_id::text
	      FROM farmacia.lotes WHERE medicamento_id=$1 AND ($2 = false OR (quantidade_actual > 0 AND validade > CURRENT_DATE))
	      ORDER BY validade ASC`
	linhas, err := r.pool.Query(ctx, q, medicamentoID, apenasDisponiveis)
	if err != nil {
		return nil, fmt.Errorf("listar lotes: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoLote{}
	for linhas.Next() {
		var it dominio.ResumoLote
		if err := linhas.Scan(&it.ID, &it.NumeroLote, &it.Validade, &it.QuantidadeActual, &it.FornecedorID); err != nil {
			return nil, fmt.Errorf("ler lote: %w", err)
		}
		out = append(out, it)
	}
	return out, linhas.Err()
}

func (r *RepositorioLotes) StockDisponivel(ctx context.Context, medicamentoID string) (int, error) {
	const q = `SELECT COALESCE(SUM(quantidade_actual),0) FROM farmacia.lotes
	           WHERE medicamento_id=$1 AND quantidade_actual > 0 AND validade > CURRENT_DATE`
	var total int
	if err := r.pool.QueryRow(ctx, q, medicamentoID).Scan(&total); err != nil {
		return 0, fmt.Errorf("consultar stock: %w", err)
	}
	return total, nil
}
```

- [ ] **Step 3: Escrever o teste de integração `tests/integration/stock_test.go`**

```go
//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRepositorioStock_EntradaEConsulta(t *testing.T) {
	pool, ctx := ligar(t)
	aplicarMigracoesTeste(t, pool, ctx)
	repoMed := pgrepo.NovoRepositorioMedicamentos(pool)
	repoLotes := pgrepo.NovoRepositorioLotes(pool)

	cod, _ := repoMed.ProximoCodigo(ctx)
	m, _ := dominio.NovoMedicamento(cod, "Amoxil Stock", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, 10)
	medID, err := repoMed.Guardar(ctx, m)
	if err != nil {
		t.Fatalf("guardar medicamento: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.movimentos_stock WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.lotes WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.medicamentos WHERE id=$1`, medID)
	})

	l1, _ := dominio.NovoLote(medID, "L001", time.Now().AddDate(0, 1, 0), 100, "12.5000", nil, "")
	if _, err := repoLotes.RegistarEntrada(ctx, l1, "00000000-0000-4000-8000-0000000000a1"); err != nil {
		t.Fatalf("entrada: %v", err)
	}
	l2, _ := dominio.NovoLote(medID, "L002", time.Now().AddDate(0, 3, 0), 50, "12.5000", nil, "")
	if _, err := repoLotes.RegistarEntrada(ctx, l2, "00000000-0000-4000-8000-0000000000a1"); err != nil {
		t.Fatalf("entrada 2: %v", err)
	}

	total, err := repoLotes.StockDisponivel(ctx, medID)
	if err != nil || total != 150 {
		t.Fatalf("stock=%d, esperava 150 (%v)", total, err)
	}
	lotes, err := repoLotes.ListarPorMedicamento(ctx, medID, true)
	if err != nil || len(lotes) != 2 || lotes[0].NumeroLote != "L001" {
		t.Fatalf("lotes inesperados: %+v (%v)", lotes, err)
	}

	// Lote com o mesmo (medicamento, número, fornecedor) → conflito de unicidade.
	dup, _ := dominio.NovoLote(medID, "L001", time.Now().AddDate(0, 2, 0), 10, "1", nil, "")
	if _, err := repoLotes.RegistarEntrada(ctx, dup, "00000000-0000-4000-8000-0000000000a1"); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito de lote, obtive %v", err)
	}
}
```
> **Nota:** `ligar(t)` e `aplicarMigracoesTeste(t, pool, ctx)` — reutiliza o padrão já usado por `medicamentos_test.go`/`receitas_test.go` (Sprint 9). Se `aplicarMigracoesTeste` não existir como helper partilhado, inline `db.AplicarMigracoes(ctx, pool, migrations.FS, logger)` como nesses ficheiros.

- [ ] **Step 4: Compilar e correr**

Run: `go build ./...` ; `go vet ./internal/adapters/pgrepo/` ; `go vet -tags integration ./tests/integration/` — limpos.
Run: `go test -tags integration ./tests/integration/ -run TestRepositorioStock -v` — PASS (com BD) ou SKIP.
Run: `gofmt -l internal/adapters/pgrepo/ tests/integration/`.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/pgrepo/fornecedores_repo.go internal/adapters/pgrepo/lotes_repo.go tests/integration/stock_test.go
git commit -m "feat(farmacia): repositórios pgx de fornecedores e lotes + integração

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Motor de dispensa transaccional (FEFO) + integração-jóia

**Files:**
- Create: `internal/adapters/pgrepo/motor_dispensa.go`
- Test: `tests/integration/dispensa_test.go` (tag `integration`)

**Interfaces:**
- Consumes: `appfarmacia.MotorDispensa`/`ItemDispensa`; domínio `farmacia` (`SnapshotReceita`, `LoteFEFO`, `AlocacaoFEFO`, `AlocarFEFO`, `MovimentoSaidaDispensa`); `pgx`/`pgxpool`.
- Produces: `MotorDispensa` com `NovoMotorDispensa(pool *pgxpool.Pool) *MotorDispensa`, implementando `appfarmacia.MotorDispensa`.

- [ ] **Step 1: Implementar `motor_dispensa.go`**

```go
package pgrepo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
)

// MotorDispensa implementa appfarmacia.MotorDispensa: aloca stock por FEFO,
// regista os movimentos SAIDA_DISPENSA e persiste a receita, numa transacção.
type MotorDispensa struct {
	pool *pgxpool.Pool
}

func NovoMotorDispensa(pool *pgxpool.Pool) *MotorDispensa {
	return &MotorDispensa{pool: pool}
}

func (m *MotorDispensa) Dispensar(ctx context.Context, receita dominio.SnapshotReceita, itens []appfarmacia.ItemDispensa, realizadoPor string) ([]dominio.AlocacaoFEFO, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var todas []dominio.AlocacaoFEFO
	for _, it := range itens {
		// Lotes válidos, ordenados por FEFO, bloqueados.
		linhas, err := tx.Query(ctx,
			`SELECT id::text, quantidade_actual FROM farmacia.lotes
			 WHERE medicamento_id=$1 AND quantidade_actual > 0 AND validade > CURRENT_DATE
			 ORDER BY validade ASC FOR UPDATE`, it.MedicamentoID)
		if err != nil {
			return nil, fmt.Errorf("bloquear lotes: %w", err)
		}
		var lotesFEFO []dominio.LoteFEFO
		for linhas.Next() {
			var lf dominio.LoteFEFO
			if err := linhas.Scan(&lf.LoteID, &lf.Disponivel); err != nil {
				linhas.Close()
				return nil, fmt.Errorf("ler lote: %w", err)
			}
			lotesFEFO = append(lotesFEFO, lf)
		}
		linhas.Close()
		if err := linhas.Err(); err != nil {
			return nil, err
		}

		alocs, err := dominio.AlocarFEFO(lotesFEFO, it.Quantidade)
		if err != nil {
			return nil, err // RegraNegocio (stock insuficiente) — rollback pelo defer
		}
		for _, a := range alocs {
			if _, err := tx.Exec(ctx,
				`UPDATE farmacia.lotes SET quantidade_actual = quantidade_actual - $2 WHERE id=$1`,
				a.LoteID, a.Quantidade); err != nil {
				return nil, fmt.Errorf("decrementar lote: %w", err)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO farmacia.movimentos_stock (tipo, medicamento_id, lote_id, quantidade, receita_id, realizado_por)
				 VALUES ($1,$2,$3,$4,$5,$6)`,
				string(dominio.MovimentoSaidaDispensa), it.MedicamentoID, a.LoteID, -a.Quantidade, receita.ID, realizadoPor); err != nil {
				return nil, fmt.Errorf("registar movimento de saída: %w", err)
			}
		}
		todas = append(todas, alocs...)
	}

	// Persistir a receita (estado + quantidades dispensadas) a partir do snapshot.
	if _, err := tx.Exec(ctx, `UPDATE farmacia.receitas SET estado=$2 WHERE id=$1`, receita.ID, string(receita.Estado)); err != nil {
		return nil, fmt.Errorf("actualizar estado da receita: %w", err)
	}
	for _, it := range receita.Itens {
		if _, err := tx.Exec(ctx,
			`UPDATE farmacia.itens_receita SET quantidade_dispensada=$3 WHERE receita_id=$1 AND medicamento_id=$2`,
			receita.ID, it.MedicamentoID, it.QuantidadeDispensada); err != nil {
			return nil, fmt.Errorf("actualizar item da receita: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("confirmar transacção: %w", err)
	}
	return todas, nil
}

// Garantia de conformidade com a porta.
var _ appfarmacia.MotorDispensa = (*MotorDispensa)(nil)
```

- [ ] **Step 2: Escrever o teste de integração `tests/integration/dispensa_test.go` (o teste-jóia)**

```go
//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
)

func TestMotorDispensa_FEFO(t *testing.T) {
	pool, ctx := ligar(t)
	aplicarMigracoesTeste(t, pool, ctx)
	repoMed := pgrepo.NovoRepositorioMedicamentos(pool)
	repoLotes := pgrepo.NovoRepositorioLotes(pool)
	repoReceitas := pgrepo.NovoRepositorioReceitas(pool)
	motor := pgrepo.NovoMotorDispensa(pool)

	cod, _ := repoMed.ProximoCodigo(ctx)
	m, _ := dominio.NovoMedicamento(cod, "Amoxil Disp", "Amoxicilina", "COMPRIMIDO", "500 mg", "ORAL", "", true, false, nil, 10)
	medID, _ := repoMed.Guardar(ctx, m)

	// Lote A expira antes (deve ser consumido primeiro), Lote B depois.
	loteA, _ := dominio.NovoLote(medID, "A", time.Now().AddDate(0, 1, 0), 15, "1", nil, "")
	loteAID, _ := repoLotes.RegistarEntrada(ctx, loteA, "00000000-0000-4000-8000-0000000000b1")
	loteB, _ := dominio.NovoLote(medID, "B", time.Now().AddDate(0, 6, 0), 30, "1", nil, "")
	loteBID, _ := repoLotes.RegistarEntrada(ctx, loteB, "00000000-0000-4000-8000-0000000000b1")

	// Receita com 1 item prescrito 40, do medicamento.
	item, _ := dominio.NovoItemReceita(medID, "1 comp", nil, 40, "")
	const doenteID = "00000000-0000-4000-8000-0000000000b2"
	const episodioID = "00000000-0000-4000-8000-0000000000b3"
	rec, _ := dominio.NovaReceita(episodioID, doenteID, "00000000-0000-4000-8000-0000000000b4", []dominio.ItemReceita{item}, "", time.Now(), time.Now().AddDate(0, 0, 30))
	recID, _ := repoReceitas.Guardar(ctx, rec)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.receitas WHERE id=$1`, recID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.movimentos_stock WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.lotes WHERE medicamento_id=$1`, medID)
		_, _ = pool.Exec(ctx, `DELETE FROM farmacia.medicamentos WHERE id=$1`, medID)
	})

	// Dispensa parcial de 20: FEFO → 15 de A + 5 de B; receita fica PARCIAL.
	lido, _ := repoReceitas.ObterPorID(ctx, recID)
	_ = lido.RegistarDispensa(medID, 20)
	snap := lido.Snapshot()
	snap.ID = recID
	if _, err := motor.Dispensar(ctx, snap, []appfarmacia.ItemDispensa{{MedicamentoID: medID, Quantidade: 20}}, "00000000-0000-4000-8000-0000000000b5"); err != nil {
		t.Fatalf("dispensar: %v", err)
	}

	// Confirmar FEFO: A esgotado (0), B com 25.
	la, _ := repoLotes.ObterPorID(ctx, loteAID)
	lb, _ := repoLotes.ObterPorID(ctx, loteBID)
	if la.QuantidadeActual() != 0 || lb.QuantidadeActual() != 25 {
		t.Fatalf("FEFO errado: A=%d (esperava 0), B=%d (esperava 25)", la.QuantidadeActual(), lb.QuantidadeActual())
	}
	final, _ := repoReceitas.ObterPorID(ctx, recID)
	if final.Estado() != dominio.ReceitaParcial || final.Snapshot().Itens[0].QuantidadeDispensada != 20 {
		t.Fatalf("receita não ficou PARCIAL/20: estado=%v qtd=%d", final.Estado(), final.Snapshot().Itens[0].QuantidadeDispensada)
	}

	// Segunda dispensa de 20 → total 40 → DISPENSADA.
	lido2, _ := repoReceitas.ObterPorID(ctx, recID)
	_ = lido2.RegistarDispensa(medID, 20)
	snap2 := lido2.Snapshot()
	if _, err := motor.Dispensar(ctx, snap2, []appfarmacia.ItemDispensa{{MedicamentoID: medID, Quantidade: 20}}, "00000000-0000-4000-8000-0000000000b5"); err != nil {
		t.Fatalf("segunda dispensa: %v", err)
	}
	final2, _ := repoReceitas.ObterPorID(ctx, recID)
	if final2.Estado() != dominio.ReceitaDispensada {
		t.Fatalf("esperava DISPENSADA, obtive %v", final2.Estado())
	}
}
```
> **Nota:** este teste usa `pgrepo.NovoRepositorioReceitas` (Sprint 9) para semear a receita. `ligar`/`aplicarMigracoesTeste` como nos outros testes de integração.

- [ ] **Step 3: Compilar e correr**

Run: `go build ./...` ; `go vet ./internal/adapters/pgrepo/` ; `go vet -tags integration ./tests/integration/` — limpos.
Run: `go test -tags integration ./tests/integration/ -run 'TestMotorDispensa|TestRepositorioStock' -v` — PASS (com BD) ou SKIP; se tiveres a BD do docker-compose, corre contra ela.
Run: `gofmt -l internal/adapters/pgrepo/ tests/integration/`.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/pgrepo/motor_dispensa.go tests/integration/dispensa_test.go
git commit -m "feat(farmacia): motor de dispensa transaccional (FEFO) e teste de integração

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Handler HTTP de stock e dispensa com RBAC e testes

**Files:**
- Create: `internal/adapters/http/farmacia_stock_handler.go`
- Test: `internal/adapters/http/farmacia_stock_test.go`

**Interfaces:**
- Consumes: casos de uso `application/farmacia` (Tasks 5-6) via interfaces de serviço locais; `SessaoDe`, `RBAC`, `responderErro`, `i18n`, `erros`, `inteiroQuery`; `dominio "internal/domain/identidade"` (papéis + `Sessao`).
- Produces: `FarmaciaStockHandler`, `NovoFarmaciaStockHandler(...)`, `RegistarFarmaciaStock(r gin.IRouter, h *FarmaciaStockHandler, protecao ...gin.HandlerFunc)`.

**Rotas e RBAC** (grupo `/api/v1/farmacia`, protegido por `protecao...`):

| Rota | Método | Papéis |
|---|---|---|
| `/farmacia/fornecedores` | POST | Farmacêutico, FarmacêuticoSenior |
| `/farmacia/fornecedores` | GET | leitura ampla* |
| `/farmacia/lotes` | POST | Farmacêutico, FarmacêuticoSenior |
| `/farmacia/medicamentos/:id/stock` | GET | leitura ampla* |
| `/farmacia/medicamentos/:id/lotes` | GET | leitura ampla* |
| `/farmacia/receitas/:id/dispensar` | POST | Farmacêutico, FarmacêuticoSenior |

\* leitura ampla = Médico, Enfermeiro, Farmacêutico, FarmacêuticoSenior, Director, DPO, Auditor.

- [ ] **Step 1: Escrever o teste que falha (`farmacia_stock_test.go`)**

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

type fakeRegistarForn struct{ out appfarmacia.DetalheFornecedor; err error }

func (f fakeRegistarForn) Executar(_ ctxT, _ string, _ appfarmacia.DadosNovoFornecedor) (appfarmacia.DetalheFornecedor, error) {
	return f.out, f.err
}

type fakeListarForn struct{ out appfarmacia.PaginaFornecedores; err error }

func (f fakeListarForn) Executar(_ ctxT, _ appfarmacia.FiltroFornecedores) (appfarmacia.PaginaFornecedores, error) {
	return f.out, f.err
}

type fakeEntradaStock struct{ out appfarmacia.DetalheLote; err error }

func (f fakeEntradaStock) Executar(_ ctxT, _ string, _ appfarmacia.DadosEntradaStock) (appfarmacia.DetalheLote, error) {
	return f.out, f.err
}

type fakeConsultarStock struct{ out appfarmacia.StockDTO; err error }

func (f fakeConsultarStock) Executar(_ ctxT, _ string) (appfarmacia.StockDTO, error) {
	return f.out, f.err
}

type fakeListarLotes struct{ out []appfarmacia.ResumoLote; err error }

func (f fakeListarLotes) Executar(_ ctxT, _ string, _ bool) ([]appfarmacia.ResumoLote, error) {
	return f.out, f.err
}

type fakeDispensar struct{ out appfarmacia.DetalheReceita; err error }

func (f fakeDispensar) Executar(_ ctxT, _, _ string, _ appfarmacia.DadosDispensa) (appfarmacia.DetalheReceita, error) {
	return f.out, f.err
}

func routerFarmaciaStock(sessao dominio.Sessao, dispensar fakeDispensar) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoFarmaciaStockHandler(
		fakeRegistarForn{out: appfarmacia.DetalheFornecedor{ID: "forn-1", Nome: "Farmédica"}},
		fakeListarForn{out: appfarmacia.PaginaFornecedores{Total: 0}},
		fakeEntradaStock{out: appfarmacia.DetalheLote{ID: "lote-1", QuantidadeActual: 100}},
		fakeConsultarStock{out: appfarmacia.StockDTO{MedicamentoID: "med-1", Disponivel: 150}},
		fakeListarLotes{out: []appfarmacia.ResumoLote{}},
		dispensar,
	)
	adhttp.RegistarFarmaciaStock(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

const corpoEntrada = `{"medicamento_id":"med-1","numero_lote":"L001","validade":"2027-01-01","quantidade":100,"preco_unit_custo":"12.5"}`
const corpoDispensa = `{"itens":[{"medicamento_id":"med-1","quantidade":5}]}`

func TestFarmaciaStock_RegistarFornecedor_Farmaceutico(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeDispensar{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/fornecedores", `{"nome":"Farmédica"}`); w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmaciaStock_RegistarFornecedor_MedicoProibido(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeDispensar{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/fornecedores", `{"nome":"X"}`); w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestFarmaciaStock_EntradaStock_Farmaceutico(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceuticoSenior}}, fakeDispensar{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/lotes", corpoEntrada); w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmaciaStock_EntradaStock_ValidadeInvalida_400(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeDispensar{})
	corpo := `{"medicamento_id":"med-1","numero_lote":"L001","validade":"01-01-2027","quantidade":100,"preco_unit_custo":"12.5"}`
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/lotes", corpo); w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestFarmaciaStock_ConsultarStock_LeituraAmpla(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeDispensar{})
	if w := pedido(r, "GET", "/api/v1/farmacia/medicamentos/med-1/stock", "Bearer x"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestFarmaciaStock_Dispensar_Farmaceutico(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, fakeDispensar{out: appfarmacia.DetalheReceita{ID: "rec-1", Estado: "PARCIAL"}})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas/rec-1/dispensar", corpoDispensa); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmaciaStock_Dispensar_MedicoProibido(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}, fakeDispensar{})
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas/rec-1/dispensar", corpoDispensa); w.Code != nethttp.StatusForbidden {
		t.Fatalf("Médico não devia dispensar: obtive %d", w.Code)
	}
}

func TestFarmaciaStock_Dispensar_Alergia_422(t *testing.T) {
	disp := fakeDispensar{err: erros.Novo(erros.CategoriaRegraNegocio, "colide com alergia")}
	r := routerFarmaciaStock(dominio.Sessao{Sujeito: "f1", Papeis: []dominio.Papel{dominio.PapelFarmaceutico}}, disp)
	if w := pedidoCorpo(r, "POST", "/api/v1/farmacia/receitas/rec-1/dispensar", corpoDispensa); w.Code != nethttp.StatusUnprocessableEntity {
		t.Fatalf("esperava 422, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestFarmaciaStock_ListarLotes_LeituraAmpla(t *testing.T) {
	r := routerFarmaciaStock(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}}, fakeDispensar{})
	if w := pedido(r, "GET", "/api/v1/farmacia/medicamentos/med-1/lotes?apenas_disponiveis=true", "Bearer x"); w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}
```
> **Nota:** o alias `ctxT` é só para encurtar os fakes no plano. No ficheiro real importa `"context"` e usa `context.Context`.

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/http/ -run FarmaciaStock -v`
Expected: FAIL — `NovoFarmaciaStockHandler`/`RegistarFarmaciaStock` indefinidos.

- [ ] **Step 3: Implementar `farmacia_stock_handler.go`**

```go
package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso de stock/dispensa.
type (
	ServicoRegistarFornecedor interface {
		Executar(ctx context.Context, actor string, dados appfarmacia.DadosNovoFornecedor) (appfarmacia.DetalheFornecedor, error)
	}
	ServicoListarFornecedores interface {
		Executar(ctx context.Context, filtro appfarmacia.FiltroFornecedores) (appfarmacia.PaginaFornecedores, error)
	}
	ServicoRegistarEntradaStock interface {
		Executar(ctx context.Context, actor string, dados appfarmacia.DadosEntradaStock) (appfarmacia.DetalheLote, error)
	}
	ServicoConsultarStock interface {
		Executar(ctx context.Context, medicamentoID string) (appfarmacia.StockDTO, error)
	}
	ServicoListarLotes interface {
		Executar(ctx context.Context, medicamentoID string, apenasDisponiveis bool) ([]appfarmacia.ResumoLote, error)
	}
	ServicoDispensarReceita interface {
		Executar(ctx context.Context, actor, receitaID string, dados appfarmacia.DadosDispensa) (appfarmacia.DetalheReceita, error)
	}
)

// FarmaciaStockHandler expõe os endpoints de stock e dispensa.
type FarmaciaStockHandler struct {
	registarForn ServicoRegistarFornecedor
	listarForn   ServicoListarFornecedores
	entrada      ServicoRegistarEntradaStock
	stock        ServicoConsultarStock
	lotes        ServicoListarLotes
	dispensar    ServicoDispensarReceita
}

func NovoFarmaciaStockHandler(
	registarForn ServicoRegistarFornecedor,
	listarForn ServicoListarFornecedores,
	entrada ServicoRegistarEntradaStock,
	stock ServicoConsultarStock,
	lotes ServicoListarLotes,
	dispensar ServicoDispensarReceita,
) *FarmaciaStockHandler {
	return &FarmaciaStockHandler{
		registarForn: registarForn, listarForn: listarForn, entrada: entrada,
		stock: stock, lotes: lotes, dispensar: dispensar,
	}
}

// RegistarFarmaciaStock regista as rotas de stock/dispensa no grupo /api/v1/farmacia.
func RegistarFarmaciaStock(r gin.IRouter, h *FarmaciaStockHandler, protecao ...gin.HandlerFunc) {
	g := r.Group("/api/v1/farmacia")
	g.Use(protecao...)

	leitura := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelFarmaceutico,
		dominio.PapelFarmaceuticoSenior, dominio.PapelDirector, dominio.PapelDPO, dominio.PapelAuditor)
	stockEscrita := RBAC(dominio.PapelFarmaceutico, dominio.PapelFarmaceuticoSenior)

	g.POST("/fornecedores", stockEscrita, h.registarFornecedor)
	g.GET("/fornecedores", leitura, h.listarFornecedores)
	g.POST("/lotes", stockEscrita, h.registarEntrada)
	g.GET("/medicamentos/:id/stock", leitura, h.consultarStock)
	g.GET("/medicamentos/:id/lotes", leitura, h.listarLotes)
	g.POST("/receitas/:id/dispensar", stockEscrita, h.dispensarReceita)
}

const formatoDataStock = "2006-01-02"

type corpoFornecedor struct {
	Nome     string  `json:"nome"`
	NIF      *string `json:"nif"`
	Contacto *string `json:"contacto"`
}

func (h *FarmaciaStockHandler) registarFornecedor(c *gin.Context) {
	var corpo corpoFornecedor
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registarForn.Executar(c.Request.Context(), actor.Sujeito, appfarmacia.DadosNovoFornecedor{
		Nome: corpo.Nome, NIF: corpo.NIF, Contacto: corpo.Contacto,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *FarmaciaStockHandler) listarFornecedores(c *gin.Context) {
	filtro := appfarmacia.FiltroFornecedores{
		Termo:         c.Query("termo"),
		ApenasActivos: c.Query("apenas_activos") == "true",
		Limite:        inteiroQuery(c, "limite"),
		Deslocamento:  inteiroQuery(c, "deslocamento"),
	}
	out, err := h.listarForn.Executar(c.Request.Context(), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoEntradaStock struct {
	MedicamentoID      string  `json:"medicamento_id"`
	NumeroLote         string  `json:"numero_lote"`
	Validade           string  `json:"validade"`
	Quantidade         int     `json:"quantidade"`
	PrecoUnitarioCusto string  `json:"preco_unit_custo"`
	FornecedorID       *string `json:"fornecedor_id"`
	Notas              string  `json:"notas"`
}

func (h *FarmaciaStockHandler) registarEntrada(c *gin.Context) {
	var corpo corpoEntradaStock
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	validade, err := time.Parse(formatoDataStock, corpo.Validade)
	if err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "validade inválida (formato esperado AAAA-MM-DD)"))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.entrada.Executar(c.Request.Context(), actor.Sujeito, appfarmacia.DadosEntradaStock{
		MedicamentoID: corpo.MedicamentoID, NumeroLote: corpo.NumeroLote, Validade: validade,
		Quantidade: corpo.Quantidade, PrecoUnitarioCusto: corpo.PrecoUnitarioCusto,
		FornecedorID: corpo.FornecedorID, Notas: corpo.Notas,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *FarmaciaStockHandler) consultarStock(c *gin.Context) {
	out, err := h.stock.Executar(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FarmaciaStockHandler) listarLotes(c *gin.Context) {
	out, err := h.lotes.Executar(c.Request.Context(), c.Param("id"), c.Query("apenas_disponiveis") == "true")
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoItemDispensa struct {
	MedicamentoID string `json:"medicamento_id"`
	Quantidade    int    `json:"quantidade"`
}

type corpoDispensa struct {
	Itens                []corpoItemDispensa `json:"itens"`
	IgnorarAlertaAlergia bool                `json:"ignorar_alerta_alergia"`
	JustificacaoAlerta   string              `json:"justificacao_alerta"`
}

func (h *FarmaciaStockHandler) dispensarReceita(c *gin.Context) {
	var corpo corpoDispensa
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	itens := make([]appfarmacia.ItemDispensaDTO, 0, len(corpo.Itens))
	for _, it := range corpo.Itens {
		itens = append(itens, appfarmacia.ItemDispensaDTO{MedicamentoID: it.MedicamentoID, Quantidade: it.Quantidade})
	}
	actor, _ := SessaoDe(c)
	out, err := h.dispensar.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), appfarmacia.DadosDispensa{
		Itens: itens, IgnorarAlertaAlergia: corpo.IgnorarAlertaAlergia, JustificacaoAlerta: corpo.JustificacaoAlerta,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
```

- [ ] **Step 4: Correr os testes e a cobertura dos adaptadores**

Run: `go test ./internal/adapters/http/ -v` → PASS (todos, incl. pré-existentes).
Run: `bash scripts/cobertura.sh` (adaptadores ≥60% — já sem o pgrepo no denominador; linha real). `staticcheck ./internal/adapters/http/...` (sem avisos).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/farmacia_stock_handler.go internal/adapters/http/farmacia_stock_test.go
git commit -m "feat(farmacia): handler HTTP de stock e dispensa com RBAC

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Wiring no composition root e ADR-029

**Files:**
- Modify: `internal/platform/app.go`
- Create: `adrs/ADR-029-farmacia-stock-dispensa.md`

**Interfaces:**
- Consumes: `pgrepo.NovoRepositorioFornecedores`/`NovoRepositorioLotes`/`NovoMotorDispensa` (Tasks 7-8), casos de uso `application/farmacia` de stock/dispensa (Tasks 5-6), `adhttp.NovoFarmaciaStockHandler`/`RegistarFarmaciaStock` (Task 9); `pool`, `repoMedicamentos`, `repoReceitas`, `leitorClinico`, `repoAuditoria`, `limiteMW`, `authMW` já existentes em `app.go`.

**Contexto:** `ExecutarServidor` já constrói `pool`, `repoAuditoria`, `repoMedicamentos`, `repoReceitas`, `leitorClinico`, `limiteMW`, `authMW`, e o closure `registarRotas` (que já chama `RegistarFarmacia`). Acrescentar o handler de stock com `limiteMW`+`authMW`.

- [ ] **Step 1: Acrescentar a construção do handler de stock**

Em `internal/platform/app.go`, a seguir ao bloco que constrói `handlerFarmacia := adhttp.NovoFarmaciaHandler(...)`, acrescentar:
```go
	// BC Farmácia: stock e dispensa.
	repoFornecedores := pgrepo.NovoRepositorioFornecedores(pool)
	repoLotes := pgrepo.NovoRepositorioLotes(pool)
	motorDispensa := pgrepo.NovoMotorDispensa(pool)
	handlerFarmaciaStock := adhttp.NovoFarmaciaStockHandler(
		appfarmacia.NovoCasoRegistarFornecedor(repoFornecedores, repoAuditoria),
		appfarmacia.NovoCasoListarFornecedores(repoFornecedores),
		appfarmacia.NovoCasoRegistarEntradaStock(repoLotes, repoMedicamentos, repoFornecedores, repoAuditoria),
		appfarmacia.NovoCasoConsultarStock(repoLotes),
		appfarmacia.NovoCasoListarLotes(repoLotes),
		appfarmacia.NovoCasoDispensarReceita(repoReceitas, repoMedicamentos, leitorClinico, motorDispensa, repoAuditoria),
	)
```
(`appfarmacia` já está importado; `repoMedicamentos`, `repoReceitas`, `leitorClinico`, `repoAuditoria` já existem.)

- [ ] **Step 2: Registar as rotas no closure `registarRotas`**

Acrescentar (a seguir a `adhttp.RegistarFarmacia(...)`):
```go
		adhttp.RegistarFarmaciaStock(r, handlerFarmaciaStock, limiteMW, authMW)
```

- [ ] **Step 3: Compilar e correr a suite completa**

Run: `go build ./...` ; `go build -tags integration ./...` — sem erros.
Run: `go test ./...` — PASS.
Run: `bash scripts/cobertura.sh` — domínio ≥85%, aplicação ≥75%, adaptadores ≥60% (linha real).
Run: `gofmt -l internal/platform/` ; `go vet ./...` ; se disponível `staticcheck ./...`.

- [ ] **Step 4: Escrever `adrs/ADR-029-farmacia-stock-dispensa.md`**

```markdown
# ADR-029 — Farmácia: Stock & Dispensa

- **Estado:** Aceite
- **Data:** 2026-07-12
- **Marco:** M2 — Clínico Core (Sprint 10)
- **Contexto de spec:** docs/superpowers/specs/2026-07-12-sprint10-farmacia-stock-dispensa-design.md

## Contexto

O Sprint 10 completa o ciclo da receita: dar entrada de stock em lotes e dispensar uma receita,
consumindo stock por FEFO. O modelo de dados foi extraído verbatim do DDM-001.

## Decisões

1. **Agregados Fornecedor e Lote; Movimento como ledger append-only** (com quantidade sinalizada:
   ENTRADA positiva, SAIDA_DISPENSA negativa).
2. **FEFO puro + `FOR UPDATE`.** A alocação é uma função de domínio `AlocarFEFO` (testável
   isoladamente), alimentada pelo adaptador com os lotes válidos bloqueados por
   `SELECT ... FOR UPDATE ORDER BY validade ASC` (seguro sob concorrência).
3. **Dispensa transaccional via porta `MotorDispensa`.** Numa só transacção: decrementa lotes,
   insere movimentos SAIDA_DISPENSA e persiste a receita (quantidades + estado). As validações
   independentes de estado fresco (não-expirada, não-exceder, alergias) são feitas na aplicação
   antes; o motor revalida só o stock (com lock).
4. **Revalidação de alergias na dispensa** (RN-FAR-04) com override do farmacêutico
   (flag + justificação, auditado) — dupla barreira face à emissão (Sprint 9).
5. **Extensão do agregado Receita** (`RegistarDispensa`): não-exceder o prescrito (RN-FAR-05) e
   estados PARCIAL/DISPENSADA.
6. **`preco_unit_custo` como decimal-texto** (NUMERIC(14,4) via `::text`) — o `moeda.AOA` do Shared
   Kernel guarda cêntimos (2 casas), insuficiente.
7. **`pgrepo` excluído do gate unitário de cobertura** (é integration-only por desenho) — resolve
   a dívida estrutural que crescia a cada sprint.

## Diferimentos

- Ajuste manual de stock (UC-FAR-08, RN-FAR-09), alertas de validade/stock-baixo (UC-FAR-06/10),
  relatório de movimentos (UC-FAR-11), venda directa OTC (UC-FAR-09), job de expiração automática
  (UC-FAR-07), psicotrópicos (RN-FAR-06), transferências.

## Consequências

- O ciclo da receita fica completo (emitir → dispensar). A base de stock/movimentos sustenta os
  relatórios e alertas futuros.
```

- [ ] **Step 5: Commits**

```bash
git add internal/platform/app.go
git commit -m "feat(farmacia): liga o stock e a dispensa ao composition root

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"

git add adrs/ADR-029-farmacia-stock-dispensa.md
git commit -m "docs(farmacia): ADR-029 com as decisões de stock e dispensa

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Verificação final (fim a fim)

1. `go build ./...` e `go build -tags integration ./...` — sem erros.
2. `go test ./...` — PASS. `bash scripts/cobertura.sh` — 85/75/60 (adaptadores já sem o pgrepo).
3. `make lint` — sem violações; `domain/farmacia` não importa `pgx`/`gin`/`uuid` nem `domain/clinico`.
4. `gofmt -l internal/ migrations/ tests/` — vazio. `staticcheck ./...` — sem avisos.
5. Migrations `farmacia/0002` aplicam-se.
6. `go test -tags integration ./tests/integration/ -run 'Stock|MotorDispensa'` — PASS (com BD) ou SKIP.
7. Fluxo HTTP (token de Farmacêutico): registar fornecedor → 201; entrada de stock → 201; consultar stock → 200; emitir receita (Sprint 9) → dispensar (FEFO) → 200 PARCIAL; dispensar de novo → DISPENSADA; dispensar como Médico → 403; entrada como Médico → 403.

## Fora de âmbito (fatias futuras)

Ajuste de stock, alertas, relatórios, venda directa, expiração-batch, psicotrópicos, transferências
(ver ADR-029).
