# ADR-039 — BC Financeiro: agregado Factura (RASCUNHO) — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar a primeira fatia vertical do BC Financeiro — o agregado `Factura` rico em estado RASCUNHO (linhas com tipo e snapshot, cálculo de IVA e totais, persistência transaccional, HTTP+RBAC) — precedida da fundação RBAC do papel Tesoureiro (ERRATA-002).

**Architecture:** DDD táctico + Clean Architecture, seguindo os BCs existentes. Domínio rico sem infra; aplicação com casos de uso e portas; adaptadores pgx/Gin; composition root em `platform/app.go`. Montantes em `moeda.AOA` (cêntimos). Sem FK cross-context (`episodio_id`/`operacao_id` uuid nus). Migrações forward-only.

**Tech Stack:** Go 1.22+, Gin, pgx v5, PostgreSQL 16, Keycloak 25 (realm export), testes `go test -race`.

## Global Constraints

- Idioma: **PT-PT angolano** em todo o código, comentários, mensagens, commits. Linguagem ubíqua (Factura, ItemFactura, Cliente, Doente, Episódio). Nunca EN/BR.
- Módulo Go: `github.com/ivandrosilva12/sgcfinal`.
- Erros de domínio via `erros.Novo(erros.Categoria…, "mensagem")` — categorias: `CategoriaValidacao` (400), `CategoriaRegraNegocio`/`CategoriaValidacao` (422), `CategoriaConflito` (409), `CategoriaNaoEncontrado` (404).
- Nada de `panic()` fora de inicialização — sempre `error`.
- Domínio (Camada 1) importa apenas biblioteca-padrão + `shared`. `go-arch-lint` bloqueia infra no domínio.
- Montantes **sempre em cêntimos (int64)** via `moeda.AOA` — nunca vírgula flutuante.
- IVA: 14% standard, isenção configurável por item; arredondamento meia-acima por linha.
- Migrações forward-only, sem `.down.sql`. Migração já aplicada **nunca** se edita — acrescenta-se nova.
- Gates de cobertura: domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
- Auditoria append-only em todas as escritas (`financeiro.factura.*`).
- Snapshot do cliente e das linhas: **sem FK cross-context**.

---

## File Structure

**Criar:**
- `internal/domain/financeiro/factura.go` — agregado `Factura`, `ItemFactura`, VOs (`EstadoFactura`, `TipoLinha`, `RegimeIVA`, `ClienteSnapshot`, `Totais`), `Snapshot`/`Reconstruir`, `ResumoFactura`, porta `RepositorioFacturas`.
- `internal/domain/financeiro/factura_test.go` — testes de domínio.
- `internal/application/financeiro/ports.go` — porta `Auditor`, DTOs, reexports.
- `internal/application/financeiro/mapa.go` — mapeamento domínio→DTO.
- `internal/application/financeiro/facturas.go` — casos de uso.
- `internal/application/financeiro/facturas_test.go` — testes de aplicação.
- `internal/application/financeiro/fakes_test.go` — fakes (repositório, auditor).
- `internal/adapters/pgrepo/facturas_repo.go` — `RepositorioFacturas` pgx.
- `internal/adapters/pgrepo/facturas_repo_test.go` — integração real contra PG.
- `internal/adapters/http/financeiro_handler.go` — handler + rotas + RBAC.
- `internal/adapters/http/financeiro_test.go` — testes HTTP.
- `migrations/financeiro/0001_facturas.sql` — schema + tabelas.
- `migrations/identidade/0005_seed_papel_tesoureiro.sql` — seed do 12.º papel.
- `docs/ERRATA-002-papel-tesoureiro.md` — decisão RBAC.
- `adrs/ADR-039-bc-financeiro-factura.md` — ADR.

**Modificar:**
- `internal/domain/financeiro/doc.go` — actualizar o placeholder.
- `internal/domain/identidade/papel.go` — `PapelTesoureiro` + `papeisValidos` (12).
- `internal/domain/identidade/doc.go` — "11 papéis" → "12 papéis".
- `internal/domain/identidade/identidade_test.go` — caso de teste do Tesoureiro.
- `seeds/papeis.sql` — acrescentar Tesoureiro.
- `docker/keycloak/realm-sgc.json` — realm role `Tesoureiro`.
- `migrations/embed.go` — acrescentar `financeiro` ao `//go:embed`.
- `internal/platform/app.go` — construir repositório/casos/handler e registar rotas.
- `CLAUDE.md` — §6 (marco M4 arranque), índice de ADRs, nota DDM (12 papéis), próximo ADR-040.
- `SPRINT.md` — nova secção do marco M4 / ADR-039.

---

## Task 1: ERRATA-002 — papel Tesoureiro (enum + doc)

Fundação RBAC. Vem primeiro porque o RBAC das rotas financeiras depende dela.

**Files:**
- Modify: `internal/domain/identidade/papel.go`
- Modify: `internal/domain/identidade/doc.go`
- Test: `internal/domain/identidade/identidade_test.go`
- Create: `docs/ERRATA-002-papel-tesoureiro.md`

**Interfaces:**
- Produces: `identidade.PapelTesoureiro identidade.Papel = "Tesoureiro"`; `PapelValido("Tesoureiro") == true`; `EhSensivel(PapelTesoureiro) == false`.

- [ ] **Step 1: Escrever o teste que falha**

Em `internal/domain/identidade/identidade_test.go`, no mapa `casos` de `TestPapelValido`, acrescentar a linha `"Tesoureiro": true,` e, no fim do ficheiro de teste, adicionar:

```go
func TestPapelTesoureiroNaoSensivel(t *testing.T) {
	if !ident.PapelValido(string(ident.PapelTesoureiro)) {
		t.Fatal("PapelTesoureiro devia ser um papel válido")
	}
	if ident.EhSensivel(ident.PapelTesoureiro) {
		t.Error("PapelTesoureiro não é sensível nesta fatia (sem MFA)")
	}
}
```

- [ ] **Step 2: Correr o teste e confirmar que falha**

Run: `go test ./internal/domain/identidade/ -run 'TestPapelValido|TestPapelTesoureiro' -v`
Expected: FAIL — `undefined: ident.PapelTesoureiro`.

- [ ] **Step 3: Implementar o papel**

Em `internal/domain/identidade/papel.go`, acrescentar a constante (a seguir a `PapelAuditor`):

```go
	PapelAuditor            Papel = "Auditor"
	PapelTesoureiro         Papel = "Tesoureiro"
```

E acrescentar ao mapa `papeisValidos`:

```go
	PapelAuditor:            true,
	PapelTesoureiro:         true,
```

Actualizar o comentário do topo do `papel.go`: `Os 11 valores` → `Os 12 valores` e a referência ao ADR: acrescentar `(12.º papel: ERRATA-002)`. Em `internal/domain/identidade/doc.go`, `11 papéis do DDM-001 v2.0` → `12 papéis do DDM-001 v2.0 (+ Tesoureiro, ERRATA-002)`.

- [ ] **Step 4: Correr os testes e confirmar que passam**

Run: `go test ./internal/domain/identidade/ -race`
Expected: PASS (ok).

- [ ] **Step 5: Escrever a ERRATA-002**

Criar `docs/ERRATA-002-papel-tesoureiro.md`:

```markdown
# ERRATA-002 — 12.º papel RBAC: Tesoureiro

- **Data**: 2026-07-18
- **Contexto**: arranque do BC Financeiro (ADR-039, Marco M4).
- **Divergência**: o DDM-001 v2.0 (reconciliado na ERRATA-001) fixou 11 papéis
  RBAC, sem Tesoureiro. O Marco M4 (CCD-M4 v2.0) pressupõe o papel Tesoureiro
  ("Tesoureiro + Director + Auditor assinam UAT").
- **Decisão**: acrescentar `Tesoureiro` como 12.º papel canónico, responsável
  pela facturação (escrita no BC Financeiro). **Não-sensível** nesta fatia (sem
  MFA); a exigência de MFA fica para reavaliação no ADR-040 (emissão de factura).
- **Impacto**: enum `identidade.Papel` (+1), `seeds/papeis.sql`, migração de seed
  `identidade/0005_seed_papel_tesoureiro.sql`, realm Keycloak `realm-sgc.json`,
  documentação (CLAUDE.md, doc.go). As atribuições de papel via JIT/Admin passam a
  aceitar `Tesoureiro`.
- **Rastreabilidade**: DDM-001 v2.0, CCD-M4 v2.0, ADR-039.
```

- [ ] **Step 6: Commit**

```bash
git add internal/domain/identidade/papel.go internal/domain/identidade/doc.go internal/domain/identidade/identidade_test.go docs/ERRATA-002-papel-tesoureiro.md
git commit -m "feat(identidade): 12.º papel Tesoureiro (ERRATA-002) — fundação RBAC do Financeiro (ADR-039)"
```

---

## Task 2: Seed e realm do papel Tesoureiro

Persistência do catálogo de papéis + realm role, para que a atribuição (FK `utilizadores_papeis → papeis`) e o token do Keycloak reconheçam `Tesoureiro`.

**Files:**
- Create: `migrations/identidade/0005_seed_papel_tesoureiro.sql`
- Modify: `seeds/papeis.sql`
- Modify: `docker/keycloak/realm-sgc.json`

**Interfaces:**
- Consumes: tabela `identidade.papeis(codigo, descricao, sensivel)` (migração 0003 já aplicada).

- [ ] **Step 1: Nova migração de seed (forward-only)**

Criar `migrations/identidade/0005_seed_papel_tesoureiro.sql` (a 0004 já foi aplicada — **não editar**; acrescenta-se a 0005):

```sql
-- Bounded Context: identidade
-- Migration forward-only. Acrescenta o 12.º papel RBAC (Tesoureiro) ao catálogo
-- canónico. Ver docs/ERRATA-002-papel-tesoureiro.md (Marco M4 pressupõe Tesoureiro,
-- ausente dos 11 papéis do DDM-001 v2.0/ERRATA-001). Não-sensível nesta fatia.
-- Idempotente: reexecutar actualiza a descrição/sensibilidade sem duplicar.

INSERT INTO identidade.papeis (codigo, descricao, sensivel) VALUES
    ('Tesoureiro', 'Tesoureiro (facturação)', false)
ON CONFLICT (codigo) DO UPDATE
    SET descricao = EXCLUDED.descricao,
        sensivel  = EXCLUDED.sensivel;
```

- [ ] **Step 2: Actualizar o seed idempotente**

Em `seeds/papeis.sql`, dentro do `INSERT ... VALUES`, acrescentar após a linha do `Auditor` (mantendo a vírgula correcta):

```sql
    ('Auditor',            'Auditor',                      true),
    ('Tesoureiro',         'Tesoureiro (facturação)',      false)
```

Actualizar o comentário de topo: `Seed dos 11 papéis` → `Seed dos 12 papéis`.

- [ ] **Step 3: Acrescentar o realm role no Keycloak**

Em `docker/keycloak/realm-sgc.json`, no array `roles.realm`, acrescentar após a entrada `Auditor` (juntar a vírgula à linha do Auditor):

```json
      { "name": "Auditor", "description": "Auditor" },
      { "name": "Tesoureiro", "description": "Tesoureiro (facturação)" }
```

- [ ] **Step 4: Validar o JSON do realm**

Run: `python -c "import json,sys; json.load(open('docker/keycloak/realm-sgc.json', encoding='utf-8')); print('realm JSON válido')"`
Expected: `realm JSON válido`.

- [ ] **Step 5: Commit**

```bash
git add migrations/identidade/0005_seed_papel_tesoureiro.sql seeds/papeis.sql docker/keycloak/realm-sgc.json
git commit -m "feat(identidade): seed e realm role do papel Tesoureiro (ERRATA-002)"
```

---

## Task 3: Domínio — VOs TipoLinha, RegimeIVA e ClienteSnapshot

Início do agregado. Value Objects primeiro (base para `ItemFactura`).

**Files:**
- Create: `internal/domain/financeiro/factura.go`
- Modify: `internal/domain/financeiro/doc.go`
- Test: `internal/domain/financeiro/factura_test.go`

**Interfaces:**
- Produces:
  - `EstadoFactura string` com `FactRascunho="RASCUNHO"`, `FactEmitida="EMITIDA"`, `FactAnulada="ANULADA"`.
  - `TipoLinha string` com `LinhaConsulta`, `LinhaDispensa`, `LinhaExameAnalise`, `LinhaEstudoImagem`, `LinhaProcedimentoCirurgico`; `func (TipoLinha) Valido() bool`; `func (TipoLinha) ExigeOperacao() bool`.
  - `RegimeIVA string` com `RegimeIsento="ISENTO"`, `RegimeStandard="STANDARD"`; `func (RegimeIVA) Valido() bool`; `func (RegimeIVA) TaxaPercent() int64`.
  - `ClienteSnapshot struct { Nome, NIF, Morada string }`; `func NovoClienteSnapshot(nome, nif, morada string) (ClienteSnapshot, error)`.

- [ ] **Step 1: Escrever os testes que falham**

Criar `internal/domain/financeiro/factura_test.go`:

```go
package financeiro_test

import (
	"testing"

	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
)

func TestTipoLinhaExigeOperacao(t *testing.T) {
	if fin.LinhaConsulta.ExigeOperacao() {
		t.Error("CONSULTA liga-se ao episódio, não exige operacaoID")
	}
	for _, tp := range []fin.TipoLinha{fin.LinhaDispensa, fin.LinhaExameAnalise, fin.LinhaEstudoImagem, fin.LinhaProcedimentoCirurgico} {
		if !tp.ExigeOperacao() {
			t.Errorf("%s devia exigir operacaoID", tp)
		}
	}
	if fin.TipoLinha("XPTO").Valido() {
		t.Error("tipo desconhecido não é válido")
	}
}

func TestRegimeIVATaxa(t *testing.T) {
	if fin.RegimeIsento.TaxaPercent() != 0 {
		t.Error("ISENTO tem taxa 0")
	}
	if fin.RegimeStandard.TaxaPercent() != 14 {
		t.Error("STANDARD tem taxa 14%")
	}
	if fin.RegimeIVA("OUTRO").Valido() {
		t.Error("regime desconhecido não é válido")
	}
}

func TestNovoClienteSnapshot(t *testing.T) {
	if _, err := fin.NovoClienteSnapshot("", "", ""); err == nil {
		t.Error("nome do cliente é obrigatório")
	}
	c, err := fin.NovoClienteSnapshot("  Clínica Sol  ", "", " Rua 1 ")
	if err != nil {
		t.Fatalf("cliente válido devia passar: %v", err)
	}
	if c.Nome != "Clínica Sol" || c.Morada != "Rua 1" {
		t.Errorf("campos não normalizados: %+v", c)
	}
	if _, err := fin.NovoClienteSnapshot("X", "NIF-INVALIDO!!", ""); err == nil {
		t.Error("NIF presente e inválido devia falhar")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/financeiro/ -run 'TestTipoLinha|TestRegimeIVA|TestNovoCliente' -v`
Expected: FAIL — pacote não compila (`undefined: fin.LinhaConsulta`, …).

- [ ] **Step 3: Implementar os VOs**

Criar `internal/domain/financeiro/factura.go`:

```go
// Package financeiro é o Bounded Context Financeiro (Camada 1 — Domínio).
// Esta fatia (ADR-039) entrega o agregado Factura em estado RASCUNHO: linhas com
// tipo e snapshot, cálculo de IVA e totais. A emissão (cadeia hash, numeração,
// imutabilidade) é do ADR-040.
package financeiro

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/identity"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
)

// EstadoFactura é o estado do ciclo de vida de uma factura. Nesta fatia só
// RASCUNHO é alcançável; EMITIDA e ANULADA já figuram no enum (e na CHECK da BD)
// para o ADR-040/041, à imagem do padrão do BC Laboratório.
type EstadoFactura string

const (
	FactRascunho EstadoFactura = "RASCUNHO"
	FactEmitida  EstadoFactura = "EMITIDA"
	FactAnulada  EstadoFactura = "ANULADA"
)

// TipoLinha classifica a operação clínica de origem de uma linha de factura.
type TipoLinha string

const (
	LinhaConsulta              TipoLinha = "CONSULTA"
	LinhaDispensa              TipoLinha = "DISPENSA"
	LinhaExameAnalise          TipoLinha = "EXAME_ANALISE"
	LinhaEstudoImagem          TipoLinha = "ESTUDO_IMAGEM"
	LinhaProcedimentoCirurgico TipoLinha = "PROCEDIMENTO_CIRURGICO"
)

var tiposValidos = map[TipoLinha]bool{
	LinhaConsulta: true, LinhaDispensa: true, LinhaExameAnalise: true,
	LinhaEstudoImagem: true, LinhaProcedimentoCirurgico: true,
}

// Valido indica se o tipo é um dos valores canónicos.
func (t TipoLinha) Valido() bool { return tiposValidos[t] }

// ExigeOperacao indica se a linha tem de referenciar o id lógico da operação de
// origem. A CONSULTA liga-se ao episódio da factura; as restantes referenciam a
// operação concreta (dispensa, requisição, estudo de imagem, procedimento).
func (t TipoLinha) ExigeOperacao() bool { return t != LinhaConsulta }

// RegimeIVA é o regime de IVA de uma linha, configurável por item (CLAUDE.md §8):
// saúde geralmente isenta; produtos/serviços tributados à taxa standard.
type RegimeIVA string

const (
	RegimeIsento   RegimeIVA = "ISENTO"
	RegimeStandard RegimeIVA = "STANDARD"
)

// Valido indica se o regime é conhecido.
func (r RegimeIVA) Valido() bool { return r == RegimeIsento || r == RegimeStandard }

// TaxaPercent devolve a taxa de IVA em pontos percentuais inteiros.
func (r RegimeIVA) TaxaPercent() int64 {
	if r == RegimeStandard {
		return 14
	}
	return 0
}

// ClienteSnapshot é a fotografia dos dados fiscais do cliente no momento da
// factura. É um snapshot imutável — sem FK ao Doente (linguagem ubíqua: Cliente).
type ClienteSnapshot struct {
	Nome   string
	NIF    string
	Morada string
}

// NovoClienteSnapshot valida e normaliza o snapshot do cliente. O nome é
// obrigatório; o NIF, se presente, é validado pelo VO do Shared Kernel.
func NovoClienteSnapshot(nome, nif, morada string) (ClienteSnapshot, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return ClienteSnapshot{}, erros.Novo(erros.CategoriaValidacao, "nome do cliente em falta")
	}
	nif = strings.TrimSpace(nif)
	if nif != "" {
		n, err := identity.NovoNIF(nif)
		if err != nil {
			return ClienteSnapshot{}, err
		}
		nif = n.String()
	}
	return ClienteSnapshot{Nome: nome, NIF: nif, Morada: strings.TrimSpace(morada)}, nil
}

var _ = time.Time{} // usado pelo agregado nas tasks seguintes
```

Actualizar `internal/domain/financeiro/doc.go` — substituir o corpo por um `// Package financeiro …` de uma linha OU eliminar o `doc.go` se o novo `factura.go` já tem o comentário de pacote. **Escolha:** eliminar `doc.go` (o comentário de pacote passa a viver em `factura.go`):

```bash
git rm internal/domain/financeiro/doc.go
```

- [ ] **Step 4: Correr e confirmar que passa**

Run: `go test ./internal/domain/financeiro/ -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/financeiro/factura.go internal/domain/financeiro/factura_test.go
git rm internal/domain/financeiro/doc.go
git commit -m "feat(financeiro): VOs TipoLinha, RegimeIVA e ClienteSnapshot (ADR-039)"
```

---

## Task 4: Domínio — ItemFactura e cálculo de IVA por linha

**Files:**
- Modify: `internal/domain/financeiro/factura.go`
- Test: `internal/domain/financeiro/factura_test.go`

**Interfaces:**
- Consumes: `TipoLinha`, `RegimeIVA`, `moeda.AOA`.
- Produces: `ItemFactura struct { ID, Descricao string; Tipo TipoLinha; OperacaoID string; Quantidade int; PrecoUnitario moeda.AOA; RegimeIVA RegimeIVA }` com `Subtotal() moeda.AOA`, `ValorIVA() moeda.AOA`, `Total() moeda.AOA`.

- [ ] **Step 1: Escrever o teste que falha**

Acrescentar a `factura_test.go`:

```go
func TestItemFacturaCalculo(t *testing.T) {
	// 2 × 1.000,00 Kz = 2.000,00 Kz; IVA 14% = 280,00 Kz.
	it := fin.ItemFactura{
		Descricao: "Consulta", Tipo: fin.LinhaConsulta,
		Quantidade: 2, PrecoUnitario: moeda.DeKwanzas(1000), RegimeIVA: fin.RegimeStandard,
	}
	if got := it.Subtotal().Centimos(); got != 200000 {
		t.Errorf("subtotal = %d; esperava 200000", got)
	}
	if got := it.ValorIVA().Centimos(); got != 28000 {
		t.Errorf("IVA = %d; esperava 28000", got)
	}
	if got := it.Total().Centimos(); got != 228000 {
		t.Errorf("total = %d; esperava 228000", got)
	}

	// Isento: IVA = 0.
	isento := fin.ItemFactura{Quantidade: 1, PrecoUnitario: moeda.DeKwanzas(500), RegimeIVA: fin.RegimeIsento}
	if isento.ValorIVA().Centimos() != 0 {
		t.Error("linha isenta tem IVA 0")
	}

	// Arredondamento meia-acima: 1 × 3,21 Kz (321 cent) × 14% = 44,94 → 44,94? 321*14=4494; (4494+50)/100=45.
	arred := fin.ItemFactura{Quantidade: 1, PrecoUnitario: moeda.DeCentimos(321), RegimeIVA: fin.RegimeStandard}
	if got := arred.ValorIVA().Centimos(); got != 45 {
		t.Errorf("IVA arredondado = %d; esperava 45", got)
	}
}
```

Acrescentar o import `"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"` ao bloco de imports do teste.

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/financeiro/ -run TestItemFacturaCalculo -v`
Expected: FAIL — `undefined: fin.ItemFactura`.

- [ ] **Step 3: Implementar o ItemFactura**

Em `factura.go`, substituir a linha `var _ = time.Time{} …` pelo tipo `ItemFactura`:

```go
// ItemFactura é uma linha de factura: entidade-filho do agregado Factura. Guarda
// o snapshot (descrição e preço) da operação de origem — sem FK cross-context.
type ItemFactura struct {
	ID            string
	Descricao     string
	Tipo          TipoLinha
	OperacaoID    string
	Quantidade    int
	PrecoUnitario moeda.AOA
	RegimeIVA     RegimeIVA
}

// Subtotal é preço unitário × quantidade (antes de IVA).
func (i ItemFactura) Subtotal() moeda.AOA {
	return moeda.DeCentimos(i.PrecoUnitario.Centimos() * int64(i.Quantidade))
}

// ValorIVA é o IVA da linha, em aritmética inteira de cêntimos, arredondado
// meia-acima. Linha isenta → 0.
func (i ItemFactura) ValorIVA() moeda.AOA {
	taxa := i.RegimeIVA.TaxaPercent()
	if taxa == 0 {
		return moeda.DeCentimos(0)
	}
	sub := i.Subtotal().Centimos()
	return moeda.DeCentimos((sub*taxa + 50) / 100)
}

// Total é subtotal + IVA da linha.
func (i ItemFactura) Total() moeda.AOA { return i.Subtotal().Somar(i.ValorIVA()) }
```

Remover o import `time` do `factura.go` se ainda não for usado (será reintroduzido na Task 5) — para manter o build limpo, deixar `time` importado só quando a Task 5 o usar. **Nota:** se o compilador reclamar de `time` não usado, remover a linha `"time"` do import neste passo e reintroduzi-la na Task 5.

- [ ] **Step 4: Correr e confirmar que passa**

Run: `go test ./internal/domain/financeiro/ -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/financeiro/factura.go internal/domain/financeiro/factura_test.go
git commit -m "feat(financeiro): ItemFactura e cálculo de IVA por linha (ADR-039)"
```

---

## Task 5: Domínio — agregado Factura (NovaFactura, AdicionarItem, RemoverItem, Totais)

**Files:**
- Modify: `internal/domain/financeiro/factura.go`
- Test: `internal/domain/financeiro/factura_test.go`

**Interfaces:**
- Produces:
  - `Factura` (raiz) com getters `ID() string`, `Estado() EstadoFactura`, `Cliente() ClienteSnapshot`, `EpisodioID() string`, `Itens() []ItemFactura`.
  - `func NovaFactura(cliente ClienteSnapshot, episodioID string) (*Factura, error)`.
  - `func (*Factura) AdicionarItem(descricao string, tipo TipoLinha, operacaoID string, quantidade int, preco moeda.AOA, regime RegimeIVA) error`.
  - `func (*Factura) RemoverItem(itemID string) error`.
  - `Totais struct { Subtotal, TotalIVA, Total moeda.AOA }`; `func (*Factura) Totais() Totais`.

- [ ] **Step 1: Escrever os testes que falham**

Acrescentar a `factura_test.go`:

```go
func novaFacturaValida(t *testing.T) *fin.Factura {
	t.Helper()
	c, err := fin.NovoClienteSnapshot("Clínica Sol", "", "")
	if err != nil {
		t.Fatal(err)
	}
	f, err := fin.NovaFactura(c, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestNovaFacturaArrancaEmRascunho(t *testing.T) {
	f := novaFacturaValida(t)
	if f.Estado() != fin.FactRascunho {
		t.Errorf("estado = %s; esperava RASCUNHO", f.Estado())
	}
	if len(f.Itens()) != 0 {
		t.Error("factura nova não tem itens")
	}
}

func TestAdicionarItemInvariantes(t *testing.T) {
	f := novaFacturaValida(t)
	// Quantidade tem de ser > 0.
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 0, moeda.DeKwanzas(10), fin.RegimeIsento); err == nil {
		t.Error("quantidade 0 devia falhar")
	}
	// DISPENSA exige operacaoID.
	if err := f.AdicionarItem("Paracetamol", fin.LinhaDispensa, "", 1, moeda.DeKwanzas(10), fin.RegimeStandard); err == nil {
		t.Error("DISPENSA sem operacaoID devia falhar")
	}
	// Tipo inválido.
	if err := f.AdicionarItem("X", fin.TipoLinha("X"), "", 1, moeda.DeKwanzas(10), fin.RegimeIsento); err == nil {
		t.Error("tipo inválido devia falhar")
	}
	// Linha válida.
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(5000), fin.RegimeIsento); err != nil {
		t.Fatalf("linha válida devia passar: %v", err)
	}
	if len(f.Itens()) != 1 {
		t.Errorf("esperava 1 item; tem %d", len(f.Itens()))
	}
}

func TestTotaisSomaPorLinha(t *testing.T) {
	f := novaFacturaValida(t)
	_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(5000), fin.RegimeIsento)
	_ = f.AdicionarItem("Medicamento", fin.LinhaDispensa, "22222222-2222-2222-2222-222222222222", 2, moeda.DeKwanzas(1000), fin.RegimeStandard)
	tot := f.Totais()
	// Subtotal: 5000 + 2×1000 = 7000 Kz = 700000 cent.
	if tot.Subtotal.Centimos() != 700000 {
		t.Errorf("subtotal = %d; esperava 700000", tot.Subtotal.Centimos())
	}
	// IVA: consulta isenta (0) + medicamento 14% de 200000 = 28000.
	if tot.TotalIVA.Centimos() != 28000 {
		t.Errorf("IVA = %d; esperava 28000", tot.TotalIVA.Centimos())
	}
	if tot.Total.Centimos() != 728000 {
		t.Errorf("total = %d; esperava 728000", tot.Total.Centimos())
	}
}

func TestRemoverItem(t *testing.T) {
	f := novaFacturaValida(t)
	_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(5000), fin.RegimeIsento)
	// Dar id ao item via reconstrução (simula item já persistido).
	s := f.Snapshot()
	s.Itens[0].ID = "item-1"
	f = fin.ReconstruirFactura(s)
	if err := f.RemoverItem("nao-existe"); err == nil {
		t.Error("remover item inexistente devia falhar")
	}
	if err := f.RemoverItem("item-1"); err != nil {
		t.Fatalf("remover item existente: %v", err)
	}
	if len(f.Itens()) != 0 {
		t.Error("item devia ter sido removido")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/financeiro/ -run 'TestNovaFactura|TestAdicionarItem|TestTotais|TestRemoverItem' -v`
Expected: FAIL — `undefined: fin.NovaFactura` / `Snapshot` / `ReconstruirFactura`.

*(O `Snapshot`/`ReconstruirFactura` são implementados na Task 6, mas o teste `TestRemoverItem` usa-os; para manter TDD por passos pequenos, implementar nesta task o agregado + `Totais` e um `Snapshot`/`ReconstruirFactura` mínimo. A porta e o `ResumoFactura` ficam na Task 6.)*

- [ ] **Step 3: Implementar o agregado**

Garantir que o import `"time"` está presente em `factura.go`. Acrescentar:

```go
// Factura é o agregado raiz do BC Financeiro. Nasce em RASCUNHO; as linhas
// podem ser adicionadas e removidas enquanto está em rascunho. A emissão
// (ADR-040) fixa a factura.
type Factura struct {
	id            string
	estado        EstadoFactura
	cliente       ClienteSnapshot
	episodioID    string
	itens         []ItemFactura
	criadoEm      time.Time
	actualizadoEm time.Time
}

// NovaFactura cria uma factura em RASCUNHO, sem itens. O episodioID é um id
// lógico (uuid) sem FK cross-context.
func NovaFactura(cliente ClienteSnapshot, episodioID string) (*Factura, error) {
	if cliente.Nome == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "cliente da factura em falta")
	}
	episodioID = strings.TrimSpace(episodioID)
	if episodioID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "episódio da factura em falta")
	}
	return &Factura{estado: FactRascunho, cliente: cliente, episodioID: episodioID}, nil
}

// AdicionarItem acrescenta uma linha. Só é permitido em RASCUNHO. O item nasce
// sem id — o repositório atribui-o na persistência.
func (f *Factura) AdicionarItem(descricao string, tipo TipoLinha, operacaoID string, quantidade int, preco moeda.AOA, regime RegimeIVA) error {
	if f.estado != FactRascunho {
		return erros.Novo(erros.CategoriaConflito, "só é possível alterar linhas de uma factura em rascunho")
	}
	descricao = strings.TrimSpace(descricao)
	if descricao == "" {
		return erros.Novo(erros.CategoriaValidacao, "descrição da linha em falta")
	}
	if !tipo.Valido() {
		return erros.Novo(erros.CategoriaValidacao, "tipo de linha inválido")
	}
	operacaoID = strings.TrimSpace(operacaoID)
	if tipo.ExigeOperacao() && operacaoID == "" {
		return erros.Novo(erros.CategoriaValidacao, "operação de origem da linha em falta")
	}
	if quantidade <= 0 {
		return erros.Novo(erros.CategoriaValidacao, "quantidade tem de ser positiva")
	}
	if preco.Negativo() {
		return erros.Novo(erros.CategoriaValidacao, "preço unitário não pode ser negativo")
	}
	if !regime.Valido() {
		return erros.Novo(erros.CategoriaValidacao, "regime de IVA inválido")
	}
	f.itens = append(f.itens, ItemFactura{
		Descricao: descricao, Tipo: tipo, OperacaoID: operacaoID,
		Quantidade: quantidade, PrecoUnitario: preco, RegimeIVA: regime,
	})
	return nil
}

// RemoverItem retira a linha com o id dado. Só é permitido em RASCUNHO.
func (f *Factura) RemoverItem(itemID string) error {
	if f.estado != FactRascunho {
		return erros.Novo(erros.CategoriaConflito, "só é possível alterar linhas de uma factura em rascunho")
	}
	for idx, it := range f.itens {
		if it.ID == itemID && itemID != "" {
			f.itens = append(f.itens[:idx], f.itens[idx+1:]...)
			return nil
		}
	}
	return erros.Novo(erros.CategoriaNaoEncontrado, "linha da factura não encontrada")
}

// Totais soma, por linha, os subtotais e o IVA (arredondar por linha e somar,
// prática fiscal). O total autoritário vive aqui — nunca em SQL.
type Totais struct {
	Subtotal moeda.AOA
	TotalIVA moeda.AOA
	Total    moeda.AOA
}

// Totais calcula os totais da factura.
func (f *Factura) Totais() Totais {
	sub := moeda.DeCentimos(0)
	iva := moeda.DeCentimos(0)
	for _, it := range f.itens {
		sub = sub.Somar(it.Subtotal())
		iva = iva.Somar(it.ValorIVA())
	}
	return Totais{Subtotal: sub, TotalIVA: iva, Total: sub.Somar(iva)}
}

// ID devolve o identificador (vazio antes de persistir).
func (f *Factura) ID() string { return f.id }

// Estado devolve o estado actual.
func (f *Factura) Estado() EstadoFactura { return f.estado }

// Cliente devolve o snapshot do cliente.
func (f *Factura) Cliente() ClienteSnapshot { return f.cliente }

// EpisodioID devolve o id lógico do episódio.
func (f *Factura) EpisodioID() string { return f.episodioID }

// Itens devolve uma cópia das linhas.
func (f *Factura) Itens() []ItemFactura {
	out := make([]ItemFactura, len(f.itens))
	copy(out, f.itens)
	return out
}
```

Acrescentar também o `Snapshot`/`ReconstruirFactura` mínimo (será reutilizado na Task 6):

```go
// SnapshotFactura carrega o estado completo para persistência ou rehidratação.
type SnapshotFactura struct {
	ID            string
	Estado        EstadoFactura
	Cliente       ClienteSnapshot
	EpisodioID    string
	Itens         []ItemFactura
	CriadoEm      time.Time
	ActualizadoEm time.Time
}

// Snapshot devolve o estado completo do agregado.
func (f *Factura) Snapshot() SnapshotFactura {
	itens := make([]ItemFactura, len(f.itens))
	copy(itens, f.itens)
	return SnapshotFactura{
		ID: f.id, Estado: f.estado, Cliente: f.cliente, EpisodioID: f.episodioID,
		Itens: itens, CriadoEm: f.criadoEm, ActualizadoEm: f.actualizadoEm,
	}
}

// ReconstruirFactura reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirFactura(s SnapshotFactura) *Factura {
	itens := make([]ItemFactura, len(s.Itens))
	copy(itens, s.Itens)
	return &Factura{
		id: s.ID, estado: s.Estado, cliente: s.Cliente, episodioID: s.EpisodioID,
		itens: itens, criadoEm: s.CriadoEm, actualizadoEm: s.ActualizadoEm,
	}
}
```

- [ ] **Step 4: Correr e confirmar que passa**

Run: `go test ./internal/domain/financeiro/ -race -cover`
Expected: PASS; cobertura ≥85%.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/financeiro/factura.go internal/domain/financeiro/factura_test.go
git commit -m "feat(financeiro): agregado Factura RASCUNHO — adicionar/remover linhas e totais (ADR-039)"
```

---

## Task 6: Domínio — porta RepositorioFacturas e read model ResumoFactura

**Files:**
- Modify: `internal/domain/financeiro/factura.go`
- Test: `internal/domain/financeiro/factura_test.go`

**Interfaces:**
- Produces:
  - `ResumoFactura struct { ID, Estado, ClienteNome, EpisodioID string; NumItens int; TotalCentimos int64; Total string; CriadoEm time.Time }` (com tags JSON).
  - `RepositorioFacturas interface { Guardar(ctx, *Factura) (string, error); ObterPorID(ctx, id string) (*Factura, error); ListarPorEpisodio(ctx, episodioID string) ([]ResumoFactura, error) }`.

- [ ] **Step 1: Escrever o teste que falha**

Acrescentar a `factura_test.go` (verifica que o read model reflecte o total do domínio):

```go
func TestResumoFacturaCampos(t *testing.T) {
	var _ fin.RepositorioFacturas // a porta tem de existir
	r := fin.ResumoFactura{ID: "f1", Estado: "RASCUNHO", ClienteNome: "Sol", NumItens: 2, TotalCentimos: 728000}
	if r.TotalCentimos != 728000 {
		t.Error("ResumoFactura deve expor o total em cêntimos")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/financeiro/ -run TestResumoFacturaCampos -v`
Expected: FAIL — `undefined: fin.RepositorioFacturas` / `fin.ResumoFactura`.

- [ ] **Step 3: Implementar o read model e a porta**

Em `factura.go`, acrescentar `"context"` ao import e:

```go
// ResumoFactura é a projecção de leitura de uma factura (listagem).
type ResumoFactura struct {
	ID            string    `json:"id"`
	Estado        string    `json:"estado"`
	ClienteNome   string    `json:"cliente_nome"`
	EpisodioID    string    `json:"episodio_id,omitempty"`
	NumItens      int       `json:"num_itens"`
	TotalCentimos int64     `json:"total_centimos"`
	Total         string    `json:"total"`
	CriadoEm      time.Time `json:"criado_em"`
}

// RepositorioFacturas é a porta de saída de persistência de facturas.
//
// Guardar é um upsert transaccional: INSERT da factura (id gerado) quando nova, ou
// UPDATE guardado por estado=RASCUNHO quando existente, reescrevendo as linhas numa
// única transacção. Devolve o id da factura.
type RepositorioFacturas interface {
	Guardar(ctx context.Context, f *Factura) (string, error)
	ObterPorID(ctx context.Context, id string) (*Factura, error)
	ListarPorEpisodio(ctx context.Context, episodioID string) ([]ResumoFactura, error)
}
```

- [ ] **Step 4: Correr e confirmar que passa**

Run: `go test ./internal/domain/financeiro/ -race`
Expected: PASS.

- [ ] **Step 5: Verificar a regra de arquitectura**

Run: `go-arch-lint check` (a partir da raiz do repo, como em CI)
Expected: sem violações (o domínio só importa `shared`).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/financeiro/factura.go internal/domain/financeiro/factura_test.go
git commit -m "feat(financeiro): porta RepositorioFacturas e read model ResumoFactura (ADR-039)"
```

---

## Task 7: Migração SQL do schema financeiro

**Files:**
- Create: `migrations/financeiro/0001_facturas.sql`
- Modify: `migrations/embed.go`

**Interfaces:**
- Produces: schema `financeiro` com `facturas` e `itens_factura`; embebido no binário.

- [ ] **Step 1: Escrever a migração**

Criar `migrations/financeiro/0001_facturas.sql`:

```sql
-- Bounded Context: financeiro
-- Migration forward-only. Arranque do BC Financeiro (ADR-039): factura em RASCUNHO.
-- Sem FK cross-context: episodio_id e operacao_id são uuid lógicos (snapshot + id),
-- nunca FK para outros BCs. A FK itens_factura → facturas é intra-BC (permitida).
-- EMITIDA/ANULADA já figuram na CHECK para o ADR-040/041; nesta fatia só se cria RASCUNHO.

CREATE SCHEMA IF NOT EXISTS financeiro;

CREATE TABLE IF NOT EXISTS financeiro.facturas (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    estado         text        NOT NULL DEFAULT 'RASCUNHO'
                               CHECK (estado IN ('RASCUNHO','EMITIDA','ANULADA')),
    cliente_nome   text        NOT NULL,
    cliente_nif    text,
    cliente_morada text,
    episodio_id    uuid        NOT NULL,
    criado_em      timestamptz NOT NULL DEFAULT now(),
    actualizado_em timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_facturas_episodio ON financeiro.facturas (episodio_id);

CREATE TABLE IF NOT EXISTS financeiro.itens_factura (
    id                      uuid    PRIMARY KEY DEFAULT gen_random_uuid(),
    factura_id              uuid    NOT NULL REFERENCES financeiro.facturas (id) ON DELETE CASCADE,
    descricao               text    NOT NULL,
    tipo                    text    NOT NULL
                                    CHECK (tipo IN ('CONSULTA','DISPENSA','EXAME_ANALISE','ESTUDO_IMAGEM','PROCEDIMENTO_CIRURGICO')),
    operacao_id             uuid,
    quantidade              integer NOT NULL CHECK (quantidade > 0),
    preco_unitario_centimos bigint  NOT NULL CHECK (preco_unitario_centimos >= 0),
    regime_iva              text    NOT NULL CHECK (regime_iva IN ('ISENTO','STANDARD')),
    ordem                   integer NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_itens_factura_factura ON financeiro.itens_factura (factura_id);
```

- [ ] **Step 2: Embeber o novo BC**

Em `migrations/embed.go`, acrescentar `financeiro` (por ordem alfabética) ao `//go:embed`:

```go
//go:embed auditoria clinico farmacia financeiro identidade laboratorio recepcao shared
var FS embed.FS
```

- [ ] **Step 3: Confirmar que o embed compila e a migração é descoberta**

Run: `go build ./...`
Expected: build OK (sem `pattern financeiro: no matching files` — o directório existe com o `.sql`).

- [ ] **Step 4: Commit**

```bash
git add migrations/financeiro/0001_facturas.sql migrations/embed.go
git commit -m "feat(financeiro): migração 0001 — schema facturas + itens (ADR-039)"
```

---

## Task 8: Adaptador pgx — RepositorioFacturas (integração real)

**Files:**
- Create: `internal/adapters/pgrepo/facturas_repo.go`
- Test: `tests/integration/facturas_test.go`

**Interfaces:**
- Consumes: `financeiro.RepositorioFacturas`, `financeiro.Factura`, `financeiro.SnapshotFactura`, `financeiro.ReconstruirFactura`.
- Produces: `func NovoRepositorioFacturas(pool *pgxpool.Pool) *RepositorioFacturas`.

**Nota de infra:** os testes de integração ficam em `tests/integration/`, com build tag `//go:build integration`, pacote `integration_test`, e usam o helper `ligar(t)` (lê `DATABASE_URL`; faz `t.Skip` se ausente). As migrações são aplicadas por `db.AplicarMigracoes(ctx, pool, migrations.FS, logger)` — padrão `migrarLaboratorio` em `laboratorio_test.go`. **Não** se cria teste em `internal/adapters/pgrepo/` (esse pacote só tem testes unitários puros). Corridos com: `go test -tags=integration ./tests/integration/...`. O container `sgc-postgres-1` está a correr; DSN: `postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable`.

- [ ] **Step 1: Escrever o teste de integração que falha**

Criar `tests/integration/facturas_test.go` (segue o padrão de `laboratorio_test.go`: build tag, `ligar(t)`, helper de migração, limpeza registada):

```go
//go:build integration

// Teste de integração do BC Financeiro (ADR-039) contra a BD real. SKIP (nunca
// FAIL) quando DATABASE_URL não está definido. O repositório pgx de facturas fica
// fora do gate de cobertura unitário — é este ficheiro que o cobre, provando o
// upsert transaccional, a reescrita de linhas e o total do read model.
package integration_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// migrarFinanceiro aplica as migrações forward-only (idempotente); ligar(t) só
// liga o pool. Modelada em migrarLaboratorio (laboratorio_test.go).
func migrarFinanceiro(t *testing.T, pool *pgxpool.Pool, ctx context.Context) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
}

// limparFactura remove a factura e as suas linhas (ON DELETE CASCADE trata as linhas).
func limparFactura(t *testing.T, pool *pgxpool.Pool, ctx context.Context, id string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id=$1`, id)
	})
}

func TestRepositorioFacturas_GuardarEObter(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	cli, _ := fin.NovoClienteSnapshot("Clínica Sol", "", "")
	f, _ := fin.NovaFactura(cli, "11111111-1111-1111-1111-111111111111")
	_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(5000), fin.RegimeIsento)
	_ = f.AdicionarItem("Medicamento", fin.LinhaDispensa, "22222222-2222-2222-2222-222222222222", 2, moeda.DeKwanzas(1000), fin.RegimeStandard)

	id, err := repo.Guardar(ctx, f)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if id == "" {
		t.Fatal("id gerado em falta")
	}
	limparFactura(t, pool, ctx, id)

	lida, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if lida.Estado() != fin.FactRascunho || len(lida.Itens()) != 2 {
		t.Errorf("factura mal lida: estado=%s itens=%d", lida.Estado(), len(lida.Itens()))
	}
	if lida.Totais().Total.Centimos() != 728000 {
		t.Errorf("total = %d; esperava 728000", lida.Totais().Total.Centimos())
	}

	// Listar por episódio devolve o total do domínio.
	resumos, err := repo.ListarPorEpisodio(ctx, "11111111-1111-1111-1111-111111111111")
	if err != nil || len(resumos) != 1 {
		t.Fatalf("listar: err=%v n=%d", err, len(resumos))
	}
	if resumos[0].TotalCentimos != 728000 || resumos[0].NumItens != 2 {
		t.Errorf("resumo errado: %+v", resumos[0])
	}
}

func TestRepositorioFacturas_ReescreveItens(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	cli, _ := fin.NovoClienteSnapshot("Sol", "", "")
	f, _ := fin.NovaFactura(cli, "33333333-3333-3333-3333-333333333333")
	_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(5000), fin.RegimeIsento)
	id, _ := repo.Guardar(ctx, f)
	limparFactura(t, pool, ctx, id)

	lida, _ := repo.ObterPorID(ctx, id)
	item0 := lida.Itens()[0].ID
	if err := lida.RemoverItem(item0); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Guardar(ctx, lida); err != nil {
		t.Fatalf("reguardar: %v", err)
	}
	rel, _ := repo.ObterPorID(ctx, id)
	if len(rel.Itens()) != 0 {
		t.Errorf("esperava 0 itens após remoção; tem %d", len(rel.Itens()))
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run TestRepositorioFacturas -v`
Expected: FAIL (compilação) — `undefined: pgrepo.NovoRepositorioFacturas`.

- [ ] **Step 3: Implementar o repositório**

Criar `internal/adapters/pgrepo/facturas_repo.go`:

```go
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
)

// RepositorioFacturas implementa fin.RepositorioFacturas com pgx.
type RepositorioFacturas struct {
	pool *pgxpool.Pool
}

// NovoRepositorioFacturas constrói o repositório sobre o pool pgx.
func NovoRepositorioFacturas(pool *pgxpool.Pool) *RepositorioFacturas {
	return &RepositorioFacturas{pool: pool}
}

// Guardar faz o upsert transaccional da factura e reescreve as suas linhas.
func (r *RepositorioFacturas) Guardar(ctx context.Context, f *fin.Factura) (string, error) {
	s := f.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção da factura: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := s.ID
	if id == "" {
		const qIns = `
INSERT INTO financeiro.facturas (estado, cliente_nome, cliente_nif, cliente_morada, episodio_id)
VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5::uuid) RETURNING id::text`
		if err := tx.QueryRow(ctx, qIns, string(s.Estado), s.Cliente.Nome, s.Cliente.NIF,
			s.Cliente.Morada, s.EpisodioID).Scan(&id); err != nil {
			return "", fmt.Errorf("inserir factura: %w", err)
		}
	} else {
		const qUpd = `
UPDATE financeiro.facturas
SET cliente_nome=$2, cliente_nif=NULLIF($3,''), cliente_morada=NULLIF($4,''), actualizado_em=now()
WHERE id=$1 AND estado='RASCUNHO'`
		ct, err := tx.Exec(ctx, qUpd, id, s.Cliente.Nome, s.Cliente.NIF, s.Cliente.Morada)
		if err != nil {
			return "", fmt.Errorf("actualizar factura: %w", err)
		}
		if ct.RowsAffected() != 1 {
			return "", erros.Novo(erros.CategoriaConflito, "a factura já não está em rascunho ou não existe")
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM financeiro.itens_factura WHERE factura_id=$1`, id); err != nil {
		return "", fmt.Errorf("limpar linhas da factura: %w", err)
	}
	const qItem = `
INSERT INTO financeiro.itens_factura
    (id, factura_id, descricao, tipo, operacao_id, quantidade, preco_unitario_centimos, regime_iva, ordem)
VALUES (COALESCE(NULLIF($1,'')::uuid, gen_random_uuid()), $2, $3, $4, NULLIF($5,'')::uuid, $6, $7, $8, $9)`
	for ordem, it := range s.Itens {
		if _, err := tx.Exec(ctx, qItem, it.ID, id, it.Descricao, string(it.Tipo),
			it.OperacaoID, it.Quantidade, it.PrecoUnitario.Centimos(), string(it.RegimeIVA), ordem); err != nil {
			return "", fmt.Errorf("inserir linha da factura: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar a gravação da factura: %w", err)
	}
	return id, nil
}

// ObterPorID reconstrói a factura com as suas linhas.
func (r *RepositorioFacturas) ObterPorID(ctx context.Context, id string) (*fin.Factura, error) {
	const q = `
SELECT id::text, estado, cliente_nome, COALESCE(cliente_nif,''), COALESCE(cliente_morada,''),
       episodio_id::text, criado_em, actualizado_em
FROM financeiro.facturas WHERE id=$1`
	var s fin.SnapshotFactura
	var estado string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &estado, &s.Cliente.Nome, &s.Cliente.NIF,
		&s.Cliente.Morada, &s.EpisodioID, &s.CriadoEm, &s.ActualizadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "factura não encontrada")
		}
		return nil, fmt.Errorf("obter factura: %w", err)
	}
	s.Estado = fin.EstadoFactura(estado)

	const qItens = `
SELECT id::text, descricao, tipo, COALESCE(operacao_id::text,''), quantidade, preco_unitario_centimos, regime_iva
FROM financeiro.itens_factura WHERE factura_id=$1 ORDER BY ordem`
	linhas, err := r.pool.Query(ctx, qItens, id)
	if err != nil {
		return nil, fmt.Errorf("listar linhas da factura: %w", err)
	}
	defer linhas.Close()
	for linhas.Next() {
		var it fin.ItemFactura
		var tipo, regime string
		var centimos int64
		if err := linhas.Scan(&it.ID, &it.Descricao, &tipo, &it.OperacaoID, &it.Quantidade, &centimos, &regime); err != nil {
			return nil, fmt.Errorf("ler linha da factura: %w", err)
		}
		it.Tipo = fin.TipoLinha(tipo)
		it.RegimeIVA = fin.RegimeIVA(regime)
		it.PrecoUnitario = moeda.DeCentimos(centimos)
		s.Itens = append(s.Itens, it)
	}
	if err := linhas.Err(); err != nil {
		return nil, fmt.Errorf("ler linhas da factura: %w", err)
	}
	return fin.ReconstruirFactura(s), nil
}

// ListarPorEpisodio devolve os resumos das facturas do episódio (recentes primeiro).
// O total replica, em aritmética inteira, a fórmula de IVA do domínio
// (ItemFactura.ValorIVA): o read model é uma projecção; o cálculo autoritário é do
// domínio.
func (r *RepositorioFacturas) ListarPorEpisodio(ctx context.Context, episodioID string) ([]fin.ResumoFactura, error) {
	const q = `
SELECT f.id::text, f.estado, f.cliente_nome, f.episodio_id::text,
       (SELECT count(*) FROM financeiro.itens_factura i WHERE i.factura_id=f.id),
       COALESCE((SELECT sum(i.preco_unitario_centimos*i.quantidade
                 + CASE i.regime_iva WHEN 'STANDARD'
                       THEN (i.preco_unitario_centimos*i.quantidade*14 + 50)/100 ELSE 0 END)
                 FROM financeiro.itens_factura i WHERE i.factura_id=f.id), 0),
       f.criado_em
FROM financeiro.facturas f
WHERE f.episodio_id=$1
ORDER BY f.criado_em DESC`
	linhas, err := r.pool.Query(ctx, q, episodioID)
	if err != nil {
		return nil, fmt.Errorf("listar facturas: %w", err)
	}
	defer linhas.Close()
	out := []fin.ResumoFactura{}
	for linhas.Next() {
		var rf fin.ResumoFactura
		if err := linhas.Scan(&rf.ID, &rf.Estado, &rf.ClienteNome, &rf.EpisodioID,
			&rf.NumItens, &rf.TotalCentimos, &rf.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler factura: %w", err)
		}
		rf.Total = moeda.DeCentimos(rf.TotalCentimos).String()
		out = append(out, rf)
	}
	return out, linhas.Err()
}

// Garantia de conformidade com a porta.
var _ fin.RepositorioFacturas = (*RepositorioFacturas)(nil)
```

- [ ] **Step 4: Correr e confirmar que passa**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run TestRepositorioFacturas -v`
Expected: PASS (o container `sgc-postgres-1` está a correr).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/pgrepo/facturas_repo.go tests/integration/facturas_test.go
git commit -m "feat(pgrepo): RepositorioFacturas — upsert transaccional + listagem (ADR-039)"
```

---

## Task 9: Aplicação — portas, DTOs e mapeamento

**Files:**
- Create: `internal/application/financeiro/ports.go`
- Create: `internal/application/financeiro/mapa.go`

**Interfaces:**
- Produces:
  - `Auditor interface { Registar(ctx, auditoria.Registo) error }`.
  - `ResumoFactura = dominio.ResumoFactura` (reexport).
  - `DadosNovaFactura`, `DadosNovoItem`, `LinhaDetalhe`, `DetalheFactura` (structs com tags JSON — ver corpo).
  - `func paraDetalheFactura(f *dominio.Factura) DetalheFactura`.

- [ ] **Step 1: Escrever ports.go**

Criar `internal/application/financeiro/ports.go`:

```go
// Package financeiro contém os casos de uso do BC Financeiro (Camada 2 — Aplicação).
package financeiro

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// Auditor persiste registos de auditoria de forma append-only.
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}

// Reexport do read model do domínio.
type ResumoFactura = dominio.ResumoFactura

// DadosNovaFactura é a entrada da criação de uma factura em rascunho.
type DadosNovaFactura struct {
	EpisodioID    string `json:"episodio_id"`
	ClienteNome   string `json:"cliente_nome"`
	ClienteNIF    string `json:"cliente_nif"`
	ClienteMorada string `json:"cliente_morada"`
}

// DadosNovoItem é a entrada da adição de uma linha. FacturaID vem do caminho.
type DadosNovoItem struct {
	FacturaID             string
	Descricao             string `json:"descricao"`
	Tipo                  string `json:"tipo"`
	OperacaoID            string `json:"operacao_id"`
	Quantidade            int    `json:"quantidade"`
	PrecoUnitarioCentimos int64  `json:"preco_unitario_centimos"`
	RegimeIVA             string `json:"regime_iva"`
}

// LinhaDetalhe é uma linha de factura numa resposta.
type LinhaDetalhe struct {
	ID                    string `json:"id"`
	Descricao             string `json:"descricao"`
	Tipo                  string `json:"tipo"`
	OperacaoID            string `json:"operacao_id,omitempty"`
	Quantidade            int    `json:"quantidade"`
	PrecoUnitarioCentimos int64  `json:"preco_unitario_centimos"`
	RegimeIVA             string `json:"regime_iva"`
	SubtotalCentimos      int64  `json:"subtotal_centimos"`
	ValorIVACentimos      int64  `json:"valor_iva_centimos"`
	TotalCentimos         int64  `json:"total_centimos"`
}

// DetalheFactura é o detalhe de uma factura numa resposta.
type DetalheFactura struct {
	ID               string         `json:"id"`
	Estado           string         `json:"estado"`
	ClienteNome      string         `json:"cliente_nome"`
	ClienteNIF       string         `json:"cliente_nif,omitempty"`
	ClienteMorada    string         `json:"cliente_morada,omitempty"`
	EpisodioID       string         `json:"episodio_id,omitempty"`
	Itens            []LinhaDetalhe `json:"itens"`
	SubtotalCentimos int64          `json:"subtotal_centimos"`
	TotalIVACentimos int64          `json:"total_iva_centimos"`
	TotalCentimos    int64          `json:"total_centimos"`
	Total            string         `json:"total"`
	CriadoEm         time.Time      `json:"criado_em"`
}
```

- [ ] **Step 2: Escrever mapa.go**

Criar `internal/application/financeiro/mapa.go`:

```go
package financeiro

import dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"

// paraDetalheFactura projecta o agregado para o DTO de resposta. Os totais vêm do
// domínio (fonte autoritária do cálculo de IVA).
func paraDetalheFactura(f *dominio.Factura) DetalheFactura {
	c := f.Cliente()
	tot := f.Totais()
	itens := make([]LinhaDetalhe, 0, len(f.Itens()))
	for _, it := range f.Itens() {
		itens = append(itens, LinhaDetalhe{
			ID: it.ID, Descricao: it.Descricao, Tipo: string(it.Tipo), OperacaoID: it.OperacaoID,
			Quantidade: it.Quantidade, PrecoUnitarioCentimos: it.PrecoUnitario.Centimos(),
			RegimeIVA:        string(it.RegimeIVA),
			SubtotalCentimos: it.Subtotal().Centimos(),
			ValorIVACentimos: it.ValorIVA().Centimos(),
			TotalCentimos:    it.Total().Centimos(),
		})
	}
	return DetalheFactura{
		ID: f.ID(), Estado: string(f.Estado()), ClienteNome: c.Nome, ClienteNIF: c.NIF,
		ClienteMorada: c.Morada, EpisodioID: f.EpisodioID(), Itens: itens,
		SubtotalCentimos: tot.Subtotal.Centimos(), TotalIVACentimos: tot.TotalIVA.Centimos(),
		TotalCentimos: tot.Total.Centimos(), Total: tot.Total.String(),
	}
}
```

- [ ] **Step 3: Confirmar que compila**

Run: `go build ./internal/application/financeiro/`
Expected: build OK.

- [ ] **Step 4: Commit**

```bash
git add internal/application/financeiro/ports.go internal/application/financeiro/mapa.go
git commit -m "feat(app-financeiro): portas, DTOs e mapeamento da factura (ADR-039)"
```

---

## Task 10: Aplicação — casos de uso (criar, adicionar/remover linha, obter, listar)

**Files:**
- Create: `internal/application/financeiro/facturas.go`
- Create: `internal/application/financeiro/fakes_test.go`
- Test: `internal/application/financeiro/facturas_test.go`

**Interfaces:**
- Consumes: `dominio.RepositorioFacturas`, `Auditor`, DTOs da Task 9.
- Produces:
  - `func NovoCasoCriarFactura(dominio.RepositorioFacturas, Auditor) *CasoCriarFactura` — `Executar(ctx, actor string, d DadosNovaFactura) (DetalheFactura, error)`.
  - `func NovoCasoAdicionarItem(dominio.RepositorioFacturas, Auditor) *CasoAdicionarItem` — `Executar(ctx, actor string, d DadosNovoItem) (DetalheFactura, error)`.
  - `func NovoCasoRemoverItem(dominio.RepositorioFacturas, Auditor) *CasoRemoverItem` — `Executar(ctx, actor, facturaID, itemID string) (DetalheFactura, error)`.
  - `func NovoCasoObterFactura(dominio.RepositorioFacturas) *CasoObterFactura` — `Executar(ctx, id string) (DetalheFactura, error)`.
  - `func NovoCasoListarFacturasPorEpisodio(dominio.RepositorioFacturas) *CasoListarFacturasPorEpisodio` — `Executar(ctx, episodioID string) ([]ResumoFactura, error)`.

- [ ] **Step 1: Escrever os fakes**

Criar `internal/application/financeiro/fakes_test.go`:

```go
package financeiro_test

import (
	"context"
	"strconv"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func agoraFixo() time.Time { return time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC) }

// fakeFacturas é um RepositorioFacturas em memória.
type fakeFacturas struct {
	porID map[string]dominio.SnapshotFactura
	seq   int
}

func novoFakeFacturas() *fakeFacturas {
	return &fakeFacturas{porID: map[string]dominio.SnapshotFactura{}}
}

func (f *fakeFacturas) Guardar(_ context.Context, fa *dominio.Factura) (string, error) {
	s := fa.Snapshot()
	if s.ID == "" {
		f.seq++
		s.ID = "fac-" + strconv.Itoa(f.seq)
	}
	// Atribui ids às linhas sem id (como o pgrepo).
	for i := range s.Itens {
		if s.Itens[i].ID == "" {
			f.seq++
			s.Itens[i].ID = "item-" + strconv.Itoa(f.seq)
		}
	}
	f.porID[s.ID] = s
	return s.ID, nil
}

func (f *fakeFacturas) ObterPorID(_ context.Context, id string) (*dominio.Factura, error) {
	s, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "factura não encontrada")
	}
	return dominio.ReconstruirFactura(s), nil
}

func (f *fakeFacturas) ListarPorEpisodio(_ context.Context, episodioID string) ([]dominio.ResumoFactura, error) {
	out := []dominio.ResumoFactura{}
	for _, s := range f.porID {
		if s.EpisodioID != episodioID {
			continue
		}
		fa := dominio.ReconstruirFactura(s)
		out = append(out, dominio.ResumoFactura{
			ID: s.ID, Estado: string(s.Estado), ClienteNome: s.Cliente.Nome,
			EpisodioID: s.EpisodioID, NumItens: len(s.Itens),
			TotalCentimos: fa.Totais().Total.Centimos(), CriadoEm: s.CriadoEm,
		})
	}
	return out, nil
}

// fakeAuditor recolhe os registos de auditoria.
type fakeAuditor struct{ registos []auditoria.Registo }

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
```

- [ ] **Step 2: Escrever os testes que falham**

Criar `internal/application/financeiro/facturas_test.go`:

```go
package financeiro_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/financeiro"
)

func TestCriarFacturaAuditada(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	uc := app.NovoCasoCriarFactura(repo, aud)
	out, err := uc.Executar(context.Background(), "u-1", app.DadosNovaFactura{
		EpisodioID: "11111111-1111-1111-1111-111111111111", ClienteNome: "Clínica Sol",
	})
	if err != nil {
		t.Fatalf("criar: %v", err)
	}
	if out.ID == "" || out.Estado != "RASCUNHO" {
		t.Errorf("factura mal criada: %+v", out)
	}
	if !aud.tem("financeiro.factura.criada") {
		t.Error("criação devia ser auditada")
	}
}

func TestAdicionarERemoverLinha(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	criar := app.NovoCasoCriarFactura(repo, aud)
	f, _ := criar.Executar(context.Background(), "u-1", app.DadosNovaFactura{
		EpisodioID: "11111111-1111-1111-1111-111111111111", ClienteNome: "Sol",
	})

	adicionar := app.NovoCasoAdicionarItem(repo, aud)
	det, err := adicionar.Executar(context.Background(), "u-1", app.DadosNovoItem{
		FacturaID: f.ID, Descricao: "Medicamento", Tipo: "DISPENSA",
		OperacaoID: "22222222-2222-2222-2222-222222222222", Quantidade: 2,
		PrecoUnitarioCentimos: 100000, RegimeIVA: "STANDARD",
	})
	if err != nil {
		t.Fatalf("adicionar: %v", err)
	}
	if len(det.Itens) != 1 || det.TotalCentimos != 228000 {
		t.Errorf("linha/total errados: %+v", det)
	}
	if !aud.tem("financeiro.factura.item.adicionado") {
		t.Error("adição de linha devia ser auditada")
	}

	remover := app.NovoCasoRemoverItem(repo, aud)
	det2, err := remover.Executar(context.Background(), "u-1", f.ID, det.Itens[0].ID)
	if err != nil {
		t.Fatalf("remover: %v", err)
	}
	if len(det2.Itens) != 0 {
		t.Error("linha devia ter sido removida")
	}
	if !aud.tem("financeiro.factura.item.removido") {
		t.Error("remoção de linha devia ser auditada")
	}
}

func TestListarPorEpisodio(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	criar := app.NovoCasoCriarFactura(repo, aud)
	_, _ = criar.Executar(context.Background(), "u-1", app.DadosNovaFactura{
		EpisodioID: "ep-x", ClienteNome: "Sol",
	})
	listar := app.NovoCasoListarFacturasPorEpisodio(repo)
	res, err := listar.Executar(context.Background(), "ep-x")
	if err != nil || len(res) != 1 {
		t.Fatalf("listar: err=%v n=%d", err, len(res))
	}
}
```

- [ ] **Step 3: Correr e confirmar que falha**

Run: `go test ./internal/application/financeiro/ -v`
Expected: FAIL — `undefined: app.NovoCasoCriarFactura`.

- [ ] **Step 4: Implementar os casos de uso**

Criar `internal/application/financeiro/facturas.go`:

```go
package financeiro

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
)

// CasoCriarFactura cria uma factura em rascunho.
type CasoCriarFactura struct {
	facturas dominio.RepositorioFacturas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoCriarFactura constrói o caso de uso.
func NovoCasoCriarFactura(f dominio.RepositorioFacturas, aud Auditor) *CasoCriarFactura {
	return &CasoCriarFactura{facturas: f, auditor: aud, agora: time.Now}
}

// Executar valida, persiste e audita a criação da factura.
func (uc *CasoCriarFactura) Executar(ctx context.Context, actor string, d DadosNovaFactura) (DetalheFactura, error) {
	cliente, err := dominio.NovoClienteSnapshot(d.ClienteNome, d.ClienteNIF, d.ClienteMorada)
	if err != nil {
		return DetalheFactura{}, err
	}
	f, err := dominio.NovaFactura(cliente, d.EpisodioID)
	if err != nil {
		return DetalheFactura{}, err
	}
	id, err := uc.facturas.Guardar(ctx, f)
	if err != nil {
		return DetalheFactura{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "financeiro.factura.criada",
		Entidade: "factura", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheFactura{}, err
	}
	return uc.obter(ctx, id)
}

func (uc *CasoCriarFactura) obter(ctx context.Context, id string) (DetalheFactura, error) {
	f, err := uc.facturas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheFactura{}, err
	}
	return paraDetalheFactura(f), nil
}

// CasoAdicionarItem acrescenta uma linha a uma factura em rascunho.
type CasoAdicionarItem struct {
	facturas dominio.RepositorioFacturas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoAdicionarItem constrói o caso de uso.
func NovoCasoAdicionarItem(f dominio.RepositorioFacturas, aud Auditor) *CasoAdicionarItem {
	return &CasoAdicionarItem{facturas: f, auditor: aud, agora: time.Now}
}

// Executar carrega a factura, acrescenta a linha, persiste e audita.
func (uc *CasoAdicionarItem) Executar(ctx context.Context, actor string, d DadosNovoItem) (DetalheFactura, error) {
	f, err := uc.facturas.ObterPorID(ctx, d.FacturaID)
	if err != nil {
		return DetalheFactura{}, err
	}
	if err := f.AdicionarItem(d.Descricao, dominio.TipoLinha(d.Tipo), d.OperacaoID,
		d.Quantidade, moeda.DeCentimos(d.PrecoUnitarioCentimos), dominio.RegimeIVA(d.RegimeIVA)); err != nil {
		return DetalheFactura{}, err
	}
	if _, err := uc.facturas.Guardar(ctx, f); err != nil {
		return DetalheFactura{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "financeiro.factura.item.adicionado",
		Entidade: "factura", EntidadeID: d.FacturaID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheFactura{}, err
	}
	recarregada, err := uc.facturas.ObterPorID(ctx, d.FacturaID)
	if err != nil {
		return DetalheFactura{}, err
	}
	return paraDetalheFactura(recarregada), nil
}

// CasoRemoverItem retira uma linha de uma factura em rascunho.
type CasoRemoverItem struct {
	facturas dominio.RepositorioFacturas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoRemoverItem constrói o caso de uso.
func NovoCasoRemoverItem(f dominio.RepositorioFacturas, aud Auditor) *CasoRemoverItem {
	return &CasoRemoverItem{facturas: f, auditor: aud, agora: time.Now}
}

// Executar carrega a factura, remove a linha, persiste e audita.
func (uc *CasoRemoverItem) Executar(ctx context.Context, actor, facturaID, itemID string) (DetalheFactura, error) {
	f, err := uc.facturas.ObterPorID(ctx, facturaID)
	if err != nil {
		return DetalheFactura{}, err
	}
	if err := f.RemoverItem(itemID); err != nil {
		return DetalheFactura{}, err
	}
	if _, err := uc.facturas.Guardar(ctx, f); err != nil {
		return DetalheFactura{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "financeiro.factura.item.removido",
		Entidade: "factura", EntidadeID: facturaID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheFactura{}, err
	}
	recarregada, err := uc.facturas.ObterPorID(ctx, facturaID)
	if err != nil {
		return DetalheFactura{}, err
	}
	return paraDetalheFactura(recarregada), nil
}

// CasoObterFactura devolve o detalhe de uma factura.
type CasoObterFactura struct {
	facturas dominio.RepositorioFacturas
}

// NovoCasoObterFactura constrói o caso de uso.
func NovoCasoObterFactura(f dominio.RepositorioFacturas) *CasoObterFactura {
	return &CasoObterFactura{facturas: f}
}

// Executar devolve o detalhe da factura.
func (uc *CasoObterFactura) Executar(ctx context.Context, id string) (DetalheFactura, error) {
	f, err := uc.facturas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheFactura{}, err
	}
	return paraDetalheFactura(f), nil
}

// CasoListarFacturasPorEpisodio lista as facturas de um episódio.
type CasoListarFacturasPorEpisodio struct {
	facturas dominio.RepositorioFacturas
}

// NovoCasoListarFacturasPorEpisodio constrói o caso de uso.
func NovoCasoListarFacturasPorEpisodio(f dominio.RepositorioFacturas) *CasoListarFacturasPorEpisodio {
	return &CasoListarFacturasPorEpisodio{facturas: f}
}

// Executar devolve os resumos das facturas do episódio.
func (uc *CasoListarFacturasPorEpisodio) Executar(ctx context.Context, episodioID string) ([]ResumoFactura, error) {
	return uc.facturas.ListarPorEpisodio(ctx, episodioID)
}
```

- [ ] **Step 5: Correr e confirmar que passa**

Run: `go test ./internal/application/financeiro/ -race -cover`
Expected: PASS; cobertura ≥75%.

- [ ] **Step 6: Commit**

```bash
git add internal/application/financeiro/facturas.go internal/application/financeiro/facturas_test.go internal/application/financeiro/fakes_test.go
git commit -m "feat(app-financeiro): casos de uso de factura RASCUNHO — criar, linhas, obter, listar (ADR-039)"
```

---

## Task 11: Adaptador HTTP — handler, rotas e RBAC

**Files:**
- Create: `internal/adapters/http/financeiro_handler.go`
- Test: `internal/adapters/http/financeiro_test.go`

**Interfaces:**
- Consumes: casos de uso da Task 10 (via interfaces de serviço), `RBAC`, `SessaoDe`, `responderErro`, `i18n` (padrão do pacote http).
- Produces: `func NovoFinanceiroHandler(criar ServicoCriarFactura, adicionar ServicoAdicionarItem, remover ServicoRemoverItem, obter ServicoObterFactura, listar ServicoListarFacturas) *FinanceiroHandler`; `func RegistarFinanceiro(r gin.IRouter, h *FinanceiroHandler, protecao ...gin.HandlerFunc)`.

- [ ] **Step 1: Escrever o handler**

Criar `internal/adapters/http/financeiro_handler.go`:

```go
// Package http (adaptadores) — este ficheiro expõe o BC Financeiro. Camada 3.
package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	appfinanceiro "github.com/ivandrosilva12/sgcfinal/internal/application/financeiro"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso do BC Financeiro.
type (
	// ServicoCriarFactura cria uma factura em rascunho.
	ServicoCriarFactura interface {
		Executar(ctx context.Context, actor string, d appfinanceiro.DadosNovaFactura) (appfinanceiro.DetalheFactura, error)
	}
	// ServicoAdicionarItem acrescenta uma linha.
	ServicoAdicionarItem interface {
		Executar(ctx context.Context, actor string, d appfinanceiro.DadosNovoItem) (appfinanceiro.DetalheFactura, error)
	}
	// ServicoRemoverItem retira uma linha.
	ServicoRemoverItem interface {
		Executar(ctx context.Context, actor, facturaID, itemID string) (appfinanceiro.DetalheFactura, error)
	}
	// ServicoObterFactura devolve o detalhe de uma factura.
	ServicoObterFactura interface {
		Executar(ctx context.Context, id string) (appfinanceiro.DetalheFactura, error)
	}
	// ServicoListarFacturas lista as facturas de um episódio.
	ServicoListarFacturas interface {
		Executar(ctx context.Context, episodioID string) ([]appfinanceiro.ResumoFactura, error)
	}
)

// FinanceiroHandler expõe os endpoints HTTP do BC Financeiro.
type FinanceiroHandler struct {
	criar     ServicoCriarFactura
	adicionar ServicoAdicionarItem
	remover   ServicoRemoverItem
	obter     ServicoObterFactura
	listar    ServicoListarFacturas
}

// NovoFinanceiroHandler constrói o handler.
func NovoFinanceiroHandler(criar ServicoCriarFactura, adicionar ServicoAdicionarItem,
	remover ServicoRemoverItem, obter ServicoObterFactura, listar ServicoListarFacturas) *FinanceiroHandler {
	return &FinanceiroHandler{criar: criar, adicionar: adicionar, remover: remover, obter: obter, listar: listar}
}

// RegistarFinanceiro regista as rotas, aplicando `protecao` e o RBAC por rota. A
// escrita (facturação) é do Tesoureiro; a leitura abre também ao Director e Auditor.
func RegistarFinanceiro(r gin.IRouter, h *FinanceiroHandler, protecao ...gin.HandlerFunc) {
	escrita := RBAC(dominio.PapelTesoureiro)
	leitura := RBAC(dominio.PapelTesoureiro, dominio.PapelDirector, dominio.PapelAuditor)

	g := r.Group("/api/v1/financeiro/facturas")
	g.Use(protecao...)
	g.POST("", escrita, h.criarFacturaHTTP)
	g.GET("", leitura, h.listarFacturasHTTP)
	g.GET("/:fid", leitura, h.obterFacturaHTTP)
	g.POST("/:fid/itens", escrita, h.adicionarItemHTTP)
	g.DELETE("/:fid/itens/:itemID", escrita, h.removerItemHTTP)
}

type corpoNovaFactura struct {
	EpisodioID    string `json:"episodio_id"`
	ClienteNome   string `json:"cliente_nome"`
	ClienteNIF    string `json:"cliente_nif"`
	ClienteMorada string `json:"cliente_morada"`
}

type corpoNovoItem struct {
	Descricao             string `json:"descricao"`
	Tipo                  string `json:"tipo"`
	OperacaoID            string `json:"operacao_id"`
	Quantidade            int    `json:"quantidade"`
	PrecoUnitarioCentimos int64  `json:"preco_unitario_centimos"`
	RegimeIVA             string `json:"regime_iva"`
}

func (h *FinanceiroHandler) criarFacturaHTTP(c *gin.Context) {
	var corpo corpoNovaFactura
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.criar.Executar(c.Request.Context(), actor.Sujeito, appfinanceiro.DadosNovaFactura{
		EpisodioID: corpo.EpisodioID, ClienteNome: corpo.ClienteNome,
		ClienteNIF: corpo.ClienteNIF, ClienteMorada: corpo.ClienteMorada,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *FinanceiroHandler) listarFacturasHTTP(c *gin.Context) {
	out, err := h.listar.Executar(c.Request.Context(), c.Query("episodio_id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

func (h *FinanceiroHandler) obterFacturaHTTP(c *gin.Context) {
	out, err := h.obter.Executar(c.Request.Context(), c.Param("fid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FinanceiroHandler) adicionarItemHTTP(c *gin.Context) {
	var corpo corpoNovoItem
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.adicionar.Executar(c.Request.Context(), actor.Sujeito, appfinanceiro.DadosNovoItem{
		FacturaID: c.Param("fid"), Descricao: corpo.Descricao, Tipo: corpo.Tipo,
		OperacaoID: corpo.OperacaoID, Quantidade: corpo.Quantidade,
		PrecoUnitarioCentimos: corpo.PrecoUnitarioCentimos, RegimeIVA: corpo.RegimeIVA,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *FinanceiroHandler) removerItemHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.remover.Executar(c.Request.Context(), actor.Sujeito, c.Param("fid"), c.Param("itemID"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
```

**Nota de rota:** o `:itemID` não é uuid validado a nível de engine porque a validação global `ValidarUUIDs` só isenta `:papel`; os ids de linha são uuid, pelo que passam na validação. Se algum teste falhar por 400 em `:itemID`, confirmar que o id usado no teste é um uuid válido.

- [ ] **Step 2: Escrever os testes HTTP**

Criar `internal/adapters/http/financeiro_test.go` seguindo o padrão de `laboratorio_test.go` (montar router com sessão fake de papel Tesoureiro, exercitar criar → adicionar → obter). Usar os helpers de teste existentes do pacote (`novoRouterDeTeste`/`comSessao` — usar exactamente os que `laboratorio_test.go` usa). Casos mínimos:

```go
// Esboço — adaptar aos helpers reais do pacote http (ver laboratorio_test.go):
// 1. POST /api/v1/financeiro/facturas com papel Tesoureiro → 201, estado RASCUNHO.
// 2. POST .../:fid/itens (DISPENSA, qtd 2, 100000 cent, STANDARD) → 201, total 228000.
// 3. GET .../:fid com papel Director → 200.
// 4. POST /api/v1/financeiro/facturas com papel Medico → 403 (RBAC).
// 5. POST /api/v1/financeiro/facturas com corpo inválido → 400.
```

Implementar os 5 casos com fakes de serviço (structs que satisfazem `ServicoCriarFactura` etc.), à imagem dos testes de handler existentes.

- [ ] **Step 3: Correr e confirmar que passa**

Run: `go test ./internal/adapters/http/ -run Financeiro -race`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/http/financeiro_handler.go internal/adapters/http/financeiro_test.go
git commit -m "feat(http): endpoints do BC Financeiro (factura RASCUNHO) + RBAC Tesoureiro (ADR-039)"
```

---

## Task 12: Composition root — ligar o BC Financeiro

**Files:**
- Modify: `internal/platform/app.go`

**Interfaces:**
- Consumes: `pgrepo.NovoRepositorioFacturas`, `appfinanceiro.NovoCaso*`, `adhttp.NovoFinanceiroHandler`, `adhttp.RegistarFinanceiro`.

- [ ] **Step 1: Construir repositório, casos e handler**

Em `internal/platform/app.go`, após o bloco do BC Laboratório (a seguir à linha 204, antes do bloco "BC Recepção"), acrescentar:

```go
	// BC Financeiro (M4, ADR-039): agregado Factura em RASCUNHO.
	repoFacturas := pgrepo.NovoRepositorioFacturas(pool)
	handlerFinanceiro := adhttp.NovoFinanceiroHandler(
		appfinanceiro.NovoCasoCriarFactura(repoFacturas, repoAuditoria),
		appfinanceiro.NovoCasoAdicionarItem(repoFacturas, repoAuditoria),
		appfinanceiro.NovoCasoRemoverItem(repoFacturas, repoAuditoria),
		appfinanceiro.NovoCasoObterFactura(repoFacturas),
		appfinanceiro.NovoCasoListarFacturasPorEpisodio(repoFacturas),
	)
```

Acrescentar o import do pacote de aplicação no topo do ficheiro (junto dos outros `app…`):

```go
	appfinanceiro "github.com/ivandrosilva12/sgcfinal/internal/application/financeiro"
```

- [ ] **Step 2: Registar as rotas**

Na função `registarRotas`, acrescentar (a seguir a `RegistarLaboratorio`):

```go
		adhttp.RegistarFinanceiro(r, handlerFinanceiro, limiteMW, authMW)
```

- [ ] **Step 3: Confirmar que compila e o binário arranca a composição**

Run: `go build ./...`
Expected: build OK.

- [ ] **Step 4: Correr toda a suite (unit + race)**

Run: `go test ./... -race`
Expected: PASS (os testes de integração pgrepo requerem PG; correr como em CI).

- [ ] **Step 5: Commit**

```bash
git add internal/platform/app.go
git commit -m "feat(plataforma): liga o BC Financeiro (factura RASCUNHO) à composição e rotas (ADR-039)"
```

---

## Task 13: ADR-039 e actualização de docs

**Files:**
- Create: `adrs/ADR-039-bc-financeiro-factura.md`
- Modify: `CLAUDE.md`
- Modify: `SPRINT.md`

- [ ] **Step 1: Escrever a ADR-039**

Criar `adrs/ADR-039-bc-financeiro-factura.md` documentando: contexto (arranque M4), decisão (agregado Factura RASCUNHO, corte Opção A, papel Tesoureiro via ERRATA-002, snapshot de linha no pedido/ACL deferido, IVA por item com arredondamento meia-acima por linha, sem FK cross-context), consequências (emissão/hash no ADR-040), estado "Aceite". Seguir o formato das ADR-036/037/038 existentes.

- [ ] **Step 2: Actualizar o CLAUDE.md**

Em `CLAUDE.md`:
- §6 (Marco Actual): acrescentar parágrafo de arranque do **M4 — Financeiro (Sprint 14)**: agregado Factura em RASCUNHO + papel Tesoureiro (ERRATA-002).
- Índice de ADRs: acrescentar `adrs/ADR-039-bc-financeiro-factura.md`; alterar "Próximo ADR: **ADR-039**" → "**ADR-040**".
- Nota DDM/§8: referir 12 papéis (Tesoureiro, ERRATA-002); mencionar `docs/ERRATA-002-papel-tesoureiro.md`.

- [ ] **Step 3: Actualizar o SPRINT.md**

Em `SPRINT.md`, acrescentar a secção do marco/critérios de saída do ADR-039 (agregado Factura RASCUNHO; papel Tesoureiro; IVA/totais; RBAC; migração financeiro/0001; gates verdes).

- [ ] **Step 4: Commit**

```bash
git add adrs/ADR-039-bc-financeiro-factura.md CLAUDE.md SPRINT.md
git commit -m "docs(financeiro): ADR-039 e actualização de marco/índice (ADR-039)"
```

---

## Task 14: Verificação final (gates, arch-lint, lint)

**Files:** nenhum (só verificação).

- [ ] **Step 1: Gates de cobertura**

Run: `go test ./internal/domain/financeiro/ ./internal/application/financeiro/ -cover`
Expected: domínio ≥85%, aplicação ≥75%. Se abaixo, acrescentar testes aos ramos em falta (ex.: preço negativo, factura fora de RASCUNHO, NIF inválido).

- [ ] **Step 2: Regra de arquitectura**

Run: `go-arch-lint check`
Expected: sem violações (o novo domínio só importa `shared`).

- [ ] **Step 3: Lint e vet**

Run: `golangci-lint run ./internal/domain/financeiro/... ./internal/application/financeiro/... ./internal/adapters/...`
Expected: sem erros.

- [ ] **Step 4: Suite completa com corrida de dados**

Run: `go test ./... -race`
Expected: PASS (testes não-integração; os de integração estão atrás do build tag).

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/...`
Expected: PASS (inclui `TestRepositorioFacturas_*`; requer `sgc-postgres-1`).

- [ ] **Step 5: Verificação de comportamento (skill `verify`)**

Aplicar as migrações num PG de dev e exercitar o fluxo real: `POST` factura (Tesoureiro) → `POST` linha → `GET` factura confirma total; `POST` com papel não-Tesoureiro → 403. (Usar o método de arranque do projecto; ver skill `run`.)

- [ ] **Step 6: Commit final (se algo foi ajustado)**

```bash
git add -A
git commit -m "test(financeiro): fecha gates de cobertura do BC Financeiro (ADR-039)"
```

---

## Self-Review (preenchido pelo autor do plano)

**Spec coverage:** cada secção do spec tem task — RBAC Tesoureiro (T1-T2), domínio Factura/ItemFactura/VOs/IVA (T3-T6), migração (T7), pgrepo (T8), aplicação (T9-T10), HTTP+RBAC (T11), wiring (T12), docs/ADR (T13), gates (T14). Fronteiras deferidas (emissão/hash, ACL, MFA) explicitamente fora de âmbito.

**Placeholder scan:** os testes HTTP da T11 estão em esboço deliberado (dependem dos helpers reais do pacote `http`, que o implementador deve reutilizar de `laboratorio_test.go`); todos os outros passos têm código completo. O esboço da T11 lista os 5 casos concretos a implementar.

**Type consistency:** assinaturas verificadas entre tasks — `RepositorioFacturas.Guardar` devolve `(string, error)` e é usado assim no pgrepo (T8), fakes (T10) e casos de uso (T10); `paraDetalheFactura(*dominio.Factura)` consistente entre mapa (T9) e casos de uso (T10); nomes de papel `PapelTesoureiro` consistentes (T1, T11); `moeda.DeCentimos`/`Centimos`/`Somar`/`Negativo` conforme a API real do VO.
