# Sprint 11 — Cirurgia Ambulatória + Consentimento (LPDP) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** fechar o critério de saída M2 (ADR-018 pt2) construindo, no BC Clínico e só backend/API, o agregado Consentimento (LPDP), o tipo de episódio CIRURGIA_AMBULATORIA, o agregado ProcedimentoCirurgico com state machine, e o catálogo de procedimentos.

**Architecture:** DDD + Clean Architecture (Domínio → Aplicação → Adaptadores → Plataforma). Dois agregados novos no pacote `internal/domain/clinico` (plano, sem subpacote). Persistência pgx SQL puro (schema `clinico`). HTTP Gin com RBAC + RFC 7807. Consentimento é dependência crítica do procedimento (FK + invariante-estrela).

**Tech Stack:** Go 1.22+, Gin, pgx v5, PostgreSQL 16.

## Global Constraints

- Todo o output em **PT-PT angolano** (código, comentários, tags JSON, mensagens de erro, commits). Nunca inglês/PT-BR.
- Domínio puro: só stdlib + Shared Kernel (`erros`, `auditoria`, `evento`, `identity`). Sem pgx/gin/http/uuid no domínio nem na aplicação.
- IDs são `string` gerados pela BD: `gen_random_uuid()` + `RETURNING id::text`.
- Erros via `erros.Novo(categoria, msg)` com mensagens literais PT-PT. Categorias: `CategoriaValidacao` (400), `CategoriaProibido` (403), `CategoriaNaoEncontrado` (404), `CategoriaConflito` (409), `CategoriaRegraNegocio` (422).
- HTTP: erros por `responderErro(c, err)` (RFC 7807). Sucesso: `c.JSON(...)`.
- Migrações forward-only, schema-qualificadas (`clinico.`).
- Sem `panic()` fora de init.
- Conventional Commits PT-PT, terminados **exactamente** com:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- Gates de cobertura: domínio ≥85%, aplicação ≥75%, adaptadores ≥60% (pgrepo excluído do gate unitário — testado por integração, tag `integration`, SKIP sem `DATABASE_URL`).
- Cancelamento DDM-estrito: `Cancelar` só transita EM_CURSO → CANCELADO.

## File Structure

**Domínio (`internal/domain/clinico/`):**
- Create `consentimento.go` — `Finalidade`, agregado `Consentimento`, `RepositorioConsentimentos`, `FiltroConsentimentos`, `ResumoConsentimento`.
- Create `anestesia.go` — VO `Anestesia`.
- Create `procedimento_enums.go` — `EstadoProcedimento`.
- Create `procedimento_cirurgico.go` — agregado `ProcedimentoCirurgico`, `RepositorioProcedimentos`, `ResumoProcedimento`.
- Create `catalogo_procedimento.go` — read model `CatalogoProcedimento`, `RepositorioCatalogoProcedimentos`.
- Modify `episodio_enums.go` — juntar `EpisodioCirurgiaAmbulatoria`.
- Modify `episodio.go` — juntar getter `Tipo()`.
- Modify `eventos.go` — juntar `ProcedimentoCirurgicoConcluido` (scaffolding).

**Aplicação (`internal/application/clinico/`):**
- Modify `ports.go` — DTOs e reexports novos.
- Create `registar_consentimento.go`, `revogar_consentimento.go`, `listar_consentimentos.go`, `obter_consentimento.go`.
- Create `agendar_procedimento.go`, `iniciar_procedimento.go`, `concluir_procedimento.go`, `cancelar_procedimento.go`, `obter_procedimento.go`, `listar_procedimentos.go`.
- Create `mapa_cirurgia.go` — mapeadores para DTOs.

**Adaptadores:**
- Create `internal/adapters/pgrepo/consentimentos_repo.go`, `procedimentos_repo.go`, `catalogo_procedimentos_repo.go`.
- Create `internal/adapters/http/consentimento_handler.go`, `cirurgia_handler.go`.

**Migrações:** `migrations/clinico/0003_consentimentos.sql`, `0004_tipo_episodio_cirurgia.sql`, `0005_catalogo_procedimentos.sql`, `0006_procedimentos_cirurgicos.sql`.

**Plataforma / docs:** Modify `internal/platform/app.go`; Create `adrs/ADR-030-cirurgia-ambulatoria-consentimento.md`.

**Testes:** `*_test.go` por camada + `tests/integration/cirurgia_test.go`, `tests/integration/consentimento_test.go`.

---

### Task 1: Migrações (consentimentos, tipo episódio, catálogo, procedimentos)

**Files:**
- Create: `migrations/clinico/0003_consentimentos.sql`
- Create: `migrations/clinico/0004_tipo_episodio_cirurgia.sql`
- Create: `migrations/clinico/0005_catalogo_procedimentos.sql`
- Create: `migrations/clinico/0006_procedimentos_cirurgicos.sql`

**Interfaces:**
- Produces: tabelas `clinico.consentimentos`, `clinico.catalogo_procedimentos` (com seed PRC001–PRC007), `clinico.procedimentos_cirurgicos`; CHECK de `episodios_clinicos.tipo` estendida com `CIRURGIA_AMBULATORIA`.

- [ ] **Step 1: Criar `0003_consentimentos.sql`**

```sql
-- Consentimentos LPDP do doente (DDM-001 v2.0). Finalidade CIRURGIA acrescentada
-- pela adenda v2.1; o anexo obrigatório para CIRURGIA é imposto no domínio.
CREATE TABLE IF NOT EXISTS clinico.consentimentos (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id     uuid        NOT NULL REFERENCES clinico.doentes(id),
    finalidade    text        NOT NULL CHECK (finalidade IN
                    ('TRATAMENTO','COMUNICACAO','PARTILHA_SEGURADORA','INVESTIGACAO','CIRURGIA')),
    concedido     boolean     NOT NULL,
    documento_url text,
    concedido_em  date        NOT NULL,
    revogado_em   date,
    criado_em     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_consentimentos_doente
    ON clinico.consentimentos (doente_id, concedido_em DESC);

COMMENT ON TABLE clinico.consentimentos IS
    'Consentimentos LPDP do doente; anexo (documento_url) obrigatório para finalidade CIRURGIA (invariante de domínio).';
```

- [ ] **Step 2: Criar `0004_tipo_episodio_cirurgia.sql`**

```sql
-- Estende a CHECK do tipo de episódio para incluir CIRURGIA_AMBULATORIA (ADR-018 pt2).
ALTER TABLE clinico.episodios_clinicos DROP CONSTRAINT IF EXISTS episodios_clinicos_tipo_check;
ALTER TABLE clinico.episodios_clinicos ADD CONSTRAINT episodios_clinicos_tipo_check
    CHECK (tipo IN ('CONSULTA','URGENCIA','INTERNAMENTO','CIRURGIA_AMBULATORIA'));
```

- [ ] **Step 3: Criar `0005_catalogo_procedimentos.sql`**

```sql
-- Catálogo de procedimentos cirúrgicos (dados de referência). Seed PRC001-PRC007
-- do DDM-001 v2.1 adenda §4.3.
CREATE TABLE IF NOT EXISTS clinico.catalogo_procedimentos (
    codigo               text    PRIMARY KEY,
    descricao            text    NOT NULL,
    especialidade        text,
    duracao_estimada_min integer,
    requer_anestesista   boolean NOT NULL DEFAULT false,
    activo               boolean NOT NULL DEFAULT true
);
INSERT INTO clinico.catalogo_procedimentos (codigo, descricao, especialidade, duracao_estimada_min) VALUES
    ('PRC001','Sutura de ferida superficial','CIRURGIA_GERAL',30),
    ('PRC002','Drenagem de abcesso','CIRURGIA_GERAL',45),
    ('PRC003','Exérese de lesão cutânea','DERMATOLOGIA',30),
    ('PRC004','Biópsia cutânea','DERMATOLOGIA',20),
    ('PRC005','Infiltração articular','ORTOPEDIA',20),
    ('PRC006','Extracção dentária simples','ESTOMATOLOGIA',30),
    ('PRC007','Extracção de corpo estranho ocular','OFTALMOLOGIA',20)
ON CONFLICT (codigo) DO NOTHING;
```

- [ ] **Step 4: Criar `0006_procedimentos_cirurgicos.sql`**

```sql
-- Procedimento cirúrgico ambulatório (DDM-001 v2.1 adenda §4.2). State machine e
-- consistência estado↔timestamps impostas por CHECK. Consentimento obrigatório.
CREATE TABLE IF NOT EXISTS clinico.procedimentos_cirurgicos (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    episodio_id         uuid        NOT NULL REFERENCES clinico.episodios_clinicos(id),
    codigo_procedimento text        NOT NULL,
    descricao           text        NOT NULL,
    sala                text,
    cirurgiao_id        uuid        NOT NULL,
    auxiliar_id         uuid,
    anestesia           text        NOT NULL CHECK (anestesia IN
                          ('NENHUMA','LOCAL','SEDACAO_LIGEIRA','LOCO_REGIONAL')),
    anestesista_id      uuid,
    inicio              timestamptz,
    fim                 timestamptz,
    consentimento_id    uuid        NOT NULL REFERENCES clinico.consentimentos(id),
    complicacoes        text,
    observacoes         text,
    estado              text        NOT NULL CHECK (estado IN
                          ('AGENDADO','EM_CURSO','CONCLUIDO','CANCELADO')),
    criado_em           timestamptz NOT NULL DEFAULT now(),
    CHECK (fim IS NULL OR fim >= inicio),
    CHECK (
        (estado = 'AGENDADO'  AND inicio IS NULL     AND fim IS NULL) OR
        (estado = 'EM_CURSO'  AND inicio IS NOT NULL AND fim IS NULL) OR
        (estado IN ('CONCLUIDO','CANCELADO') AND inicio IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_procedimentos_episodio  ON clinico.procedimentos_cirurgicos (episodio_id);
CREATE INDEX IF NOT EXISTS idx_procedimentos_cirurgiao ON clinico.procedimentos_cirurgicos (cirurgiao_id, criado_em DESC);
CREATE INDEX IF NOT EXISTS idx_procedimentos_codigo    ON clinico.procedimentos_cirurgicos (codigo_procedimento);
```

- [ ] **Step 5: Verificar que os ficheiros embebem e o build passa**

Run: `go test ./migrations/... && go build ./...`
Expected: PASS (os `.sql` de `migrations/clinico/` são embebidos por `embed.FS`; não é preciso registo manual). A aplicação real contra Postgres é verificada na Task 10 (integração).

- [ ] **Step 6: Commit**

```bash
git add migrations/clinico/0003_consentimentos.sql migrations/clinico/0004_tipo_episodio_cirurgia.sql migrations/clinico/0005_catalogo_procedimentos.sql migrations/clinico/0006_procedimentos_cirurgicos.sql
git commit -m "$(printf 'feat(clinico): migracoes de consentimentos, catalogo e procedimentos cirurgicos\n\nDDM-001 v2.0 (consentimentos) + v2.1 adenda (procedimentos, catalogo seed\nPRC001-007) e extensao da CHECK do tipo de episodio para CIRURGIA_AMBULATORIA.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 2: Domínio — agregado Consentimento

**Files:**
- Create: `internal/domain/clinico/consentimento.go`
- Test: `internal/domain/clinico/consentimento_test.go`

**Interfaces:**
- Produces: `Finalidade` (`FinalidadeTratamento`, `FinalidadeComunicacao`, `FinalidadePartilhaSeguradora`, `FinalidadeInvestigacao`, `FinalidadeCirurgia`); `ParseFinalidade(string) (Finalidade, error)`; agregado `Consentimento` com `NovoConsentimento(doenteID string, f Finalidade, concedido bool, documentoURL string, concedidoEm time.Time) (*Consentimento, error)`, getters `ID/DoenteID/Finalidade`, `TemAnexo() bool`, `EstaVigente() bool`, `Revogar(em time.Time) error`, `Snapshot() SnapshotConsentimento`, `ReconstruirConsentimento(SnapshotConsentimento) *Consentimento`; interface `RepositorioConsentimentos`; `FiltroConsentimentos`; `ResumoConsentimento`.

- [ ] **Step 1: Escrever o teste que falha**

```go
package clinico_test

import (
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovoConsentimento_Cirurgia_ExigeAnexoEConcedido(t *testing.T) {
	quando := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	// Sem anexo → RegraNegocio.
	if _, err := dominio.NovoConsentimento("doente-1", dominio.FinalidadeCirurgia, true, "", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("cirurgia sem anexo devia falhar com RegraNegocio, veio %v", err)
	}
	// Não concedido → RegraNegocio.
	if _, err := dominio.NovoConsentimento("doente-1", dominio.FinalidadeCirurgia, false, "s3://doc.pdf", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("cirurgia não concedida devia falhar com RegraNegocio, veio %v", err)
	}
	// Válido.
	c, err := dominio.NovoConsentimento("doente-1", dominio.FinalidadeCirurgia, true, "s3://doc.pdf", quando)
	if err != nil {
		t.Fatalf("consentimento de cirurgia válido não devia falhar: %v", err)
	}
	if !c.TemAnexo() || !c.EstaVigente() {
		t.Fatalf("esperado com anexo e vigente")
	}
}

func TestConsentimento_Revogar(t *testing.T) {
	quando := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	c, _ := dominio.NovoConsentimento("doente-1", dominio.FinalidadeTratamento, true, "", quando)
	if err := c.Revogar(quando); err != nil {
		t.Fatalf("revogar devia funcionar: %v", err)
	}
	if c.EstaVigente() {
		t.Fatalf("consentimento revogado não devia estar vigente")
	}
	if err := c.Revogar(quando); err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("revogar de novo devia falhar com Conflito, veio %v", err)
	}
}

func TestParseFinalidade_Invalida(t *testing.T) {
	if _, err := dominio.ParseFinalidade("QUALQUER"); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("finalidade inválida devia falhar com Validacao, veio %v", err)
	}
}
```

- [ ] **Step 2: Correr para confirmar que falha**

Run: `go test ./internal/domain/clinico/ -run TestNovoConsentimento -v`
Expected: FAIL (compilação: `NovoConsentimento` indefinido).

- [ ] **Step 3: Implementar `consentimento.go`**

```go
package clinico

import (
	"context"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Finalidade classifica a finalidade LPDP de um consentimento (DDM-001 v2.0;
// CIRURGIA acrescentada pela adenda v2.1).
type Finalidade string

const (
	FinalidadeTratamento         Finalidade = "TRATAMENTO"
	FinalidadeComunicacao        Finalidade = "COMUNICACAO"
	FinalidadePartilhaSeguradora Finalidade = "PARTILHA_SEGURADORA"
	FinalidadeInvestigacao       Finalidade = "INVESTIGACAO"
	FinalidadeCirurgia           Finalidade = "CIRURGIA"
)

var finalidadesValidas = map[Finalidade]bool{
	FinalidadeTratamento: true, FinalidadeComunicacao: true,
	FinalidadePartilhaSeguradora: true, FinalidadeInvestigacao: true,
	FinalidadeCirurgia: true,
}

// ParseFinalidade valida e normaliza uma finalidade (aceita minúsculas).
func ParseFinalidade(codigo string) (Finalidade, error) {
	f := Finalidade(strings.ToUpper(strings.TrimSpace(codigo)))
	if !finalidadesValidas[f] {
		return "", erros.Novo(erros.CategoriaValidacao,
			"finalidade de consentimento inválida (esperado TRATAMENTO, COMUNICACAO, PARTILHA_SEGURADORA, INVESTIGACAO ou CIRURGIA)")
	}
	return f, nil
}

// Consentimento é um agregado raiz do BC Clínico: o consentimento LPDP de um
// doente para uma finalidade. O id é gerado pela base de dados.
type Consentimento struct {
	id           string
	doenteID     string
	finalidade   Finalidade
	concedido    bool
	documentoURL string
	concedidoEm  time.Time
	revogadoEm   *time.Time
	criadoEm     time.Time
}

// NovoConsentimento valida e constrói um consentimento. Para a finalidade
// CIRURGIA impõe a invariante-estrela: tem de estar concedido e com anexo.
func NovoConsentimento(doenteID string, f Finalidade, concedido bool, documentoURL string, concedidoEm time.Time) (*Consentimento, error) {
	doenteID = strings.TrimSpace(doenteID)
	if doenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente do consentimento em falta")
	}
	if _, err := ParseFinalidade(string(f)); err != nil {
		return nil, err
	}
	if concedidoEm.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "data de concessão do consentimento em falta")
	}
	documentoURL = strings.TrimSpace(documentoURL)
	if f == FinalidadeCirurgia {
		if !concedido {
			return nil, erros.Novo(erros.CategoriaRegraNegocio, "o consentimento de cirurgia tem de estar concedido")
		}
		if documentoURL == "" {
			return nil, erros.Novo(erros.CategoriaRegraNegocio, "o consentimento de cirurgia exige documento anexado")
		}
	}
	return &Consentimento{
		doenteID: doenteID, finalidade: f, concedido: concedido,
		documentoURL: documentoURL, concedidoEm: concedidoEm,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados (vazio se não persistido).
func (c *Consentimento) ID() string { return c.id }

// DoenteID devolve o id do doente a que o consentimento pertence.
func (c *Consentimento) DoenteID() string { return c.doenteID }

// Finalidade devolve a finalidade LPDP.
func (c *Consentimento) Finalidade() Finalidade { return c.finalidade }

// TemAnexo indica se há documento anexado.
func (c *Consentimento) TemAnexo() bool { return c.documentoURL != "" }

// EstaVigente indica se o consentimento está concedido e não revogado.
func (c *Consentimento) EstaVigente() bool { return c.concedido && c.revogadoEm == nil }

// Revogar revoga o consentimento. Só de um consentimento concedido e não revogado.
func (c *Consentimento) Revogar(em time.Time) error {
	if !c.concedido {
		return erros.Novo(erros.CategoriaConflito, "não é possível revogar um consentimento que não foi concedido")
	}
	if c.revogadoEm != nil {
		return erros.Novo(erros.CategoriaConflito, "o consentimento já foi revogado")
	}
	c.revogadoEm = &em
	return nil
}

// SnapshotConsentimento carrega o estado completo para persistência ou rehidratação.
type SnapshotConsentimento struct {
	ID           string
	DoenteID     string
	Finalidade   Finalidade
	Concedido    bool
	DocumentoURL string
	ConcedidoEm  time.Time
	RevogadoEm   *time.Time
	CriadoEm     time.Time
}

// Snapshot devolve o estado completo do agregado.
func (c *Consentimento) Snapshot() SnapshotConsentimento {
	return SnapshotConsentimento{
		ID: c.id, DoenteID: c.doenteID, Finalidade: c.finalidade,
		Concedido: c.concedido, DocumentoURL: c.documentoURL,
		ConcedidoEm: c.concedidoEm, RevogadoEm: c.revogadoEm, CriadoEm: c.criadoEm,
	}
}

// ReconstruirConsentimento reconstrói um agregado a partir de um snapshot persistido.
func ReconstruirConsentimento(s SnapshotConsentimento) *Consentimento {
	return &Consentimento{
		id: s.ID, doenteID: s.DoenteID, finalidade: s.Finalidade,
		concedido: s.Concedido, documentoURL: s.DocumentoURL,
		concedidoEm: s.ConcedidoEm, revogadoEm: s.RevogadoEm, criadoEm: s.CriadoEm,
	}
}

// FiltroConsentimentos filtra a listagem de consentimentos de um doente.
type FiltroConsentimentos struct {
	Finalidade     string
	ApenasVigentes bool
}

// ResumoConsentimento é a projecção de leitura de um consentimento.
type ResumoConsentimento struct {
	ID           string     `json:"id"`
	DoenteID     string     `json:"doente_id"`
	Finalidade   string     `json:"finalidade"`
	Concedido    bool       `json:"concedido"`
	DocumentoURL string     `json:"documento_url,omitempty"`
	ConcedidoEm  time.Time  `json:"concedido_em"`
	RevogadoEm   *time.Time `json:"revogado_em,omitempty"`
	Vigente      bool       `json:"vigente"`
}

// RepositorioConsentimentos é a porta de saída de persistência de consentimentos.
type RepositorioConsentimentos interface {
	Guardar(ctx context.Context, c *Consentimento) (string, error)
	ObterPorID(ctx context.Context, id string) (*Consentimento, error)
	ListarPorDoente(ctx context.Context, doenteID string, filtro FiltroConsentimentos) ([]ResumoConsentimento, error)
}
```

- [ ] **Step 4: Correr para confirmar que passa**

Run: `go test ./internal/domain/clinico/ -run 'TestNovoConsentimento|TestConsentimento_Revogar|TestParseFinalidade' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/clinico/consentimento.go internal/domain/clinico/consentimento_test.go
git commit -m "$(printf 'feat(clinico): agregado Consentimento (LPDP) com invariante de cirurgia\n\nFinalidade (5 valores incl. CIRURGIA), NovoConsentimento (CIRURGIA exige anexo\ne concedido), Revogar, vigencia, snapshot/reconstruir e porta de repositorio.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 3: Domínio — VO Anestesia + tipo de episódio CIRURGIA_AMBULATORIA

**Files:**
- Create: `internal/domain/clinico/anestesia.go`
- Test: `internal/domain/clinico/anestesia_test.go`
- Modify: `internal/domain/clinico/episodio_enums.go` (juntar `EpisodioCirurgiaAmbulatoria`)
- Modify: `internal/domain/clinico/episodio.go` (juntar getter `Tipo()`)
- Test: `internal/domain/clinico/episodio_enums_test.go` (juntar caso do novo tipo)

**Interfaces:**
- Produces: `Anestesia` (`AnestesiaNenhuma`, `AnestesiaLocal`, `AnestesiaSedacaoLigeira`, `AnestesiaLocoRegional`); `ParseAnestesia(string) (Anestesia, error)`; `(Anestesia).RequerAnestesista() bool`; `EpisodioCirurgiaAmbulatoria TipoEpisodio`; `(*EpisodioClinico).Tipo() TipoEpisodio`.

- [ ] **Step 1: Escrever o teste que falha (`anestesia_test.go`)**

```go
package clinico_test

import (
	"testing"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestAnestesia_RequerAnestesista(t *testing.T) {
	if dominio.AnestesiaNenhuma.RequerAnestesista() {
		t.Fatalf("NENHUMA não devia exigir anestesista")
	}
	for _, a := range []dominio.Anestesia{dominio.AnestesiaLocal, dominio.AnestesiaSedacaoLigeira, dominio.AnestesiaLocoRegional} {
		if !a.RequerAnestesista() {
			t.Fatalf("%s devia exigir anestesista", a)
		}
	}
}

func TestParseAnestesia(t *testing.T) {
	if _, err := dominio.ParseAnestesia("sedacao_ligeira"); err != nil {
		t.Fatalf("sedacao_ligeira devia ser válida: %v", err)
	}
	if _, err := dominio.ParseAnestesia("GERAL"); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("GERAL devia falhar com Validacao, veio %v", err)
	}
}
```

- [ ] **Step 2: Juntar teste do novo tipo de episódio em `episodio_enums_test.go`**

Acrescentar esta função ao ficheiro existente:

```go
func TestParseTipoEpisodio_CirurgiaAmbulatoria(t *testing.T) {
	tipo, err := clinico.ParseTipoEpisodio("cirurgia_ambulatoria")
	if err != nil {
		t.Fatalf("CIRURGIA_AMBULATORIA devia ser válida: %v", err)
	}
	if tipo != clinico.EpisodioCirurgiaAmbulatoria {
		t.Fatalf("esperado EpisodioCirurgiaAmbulatoria, veio %s", tipo)
	}
}
```

- [ ] **Step 3: Correr para confirmar que falha**

Run: `go test ./internal/domain/clinico/ -run 'TestAnestesia|TestParseAnestesia|TestParseTipoEpisodio_Cirurgia' -v`
Expected: FAIL (compilação: identificadores indefinidos).

- [ ] **Step 4: Implementar `anestesia.go`**

```go
package clinico

import (
	"strings"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Anestesia é o tipo de anestesia de um procedimento cirúrgico (DDM-001 v2.1).
type Anestesia string

const (
	AnestesiaNenhuma        Anestesia = "NENHUMA"
	AnestesiaLocal          Anestesia = "LOCAL"
	AnestesiaSedacaoLigeira Anestesia = "SEDACAO_LIGEIRA"
	AnestesiaLocoRegional   Anestesia = "LOCO_REGIONAL"
)

var anestesiasValidas = map[Anestesia]bool{
	AnestesiaNenhuma: true, AnestesiaLocal: true,
	AnestesiaSedacaoLigeira: true, AnestesiaLocoRegional: true,
}

// ParseAnestesia valida e normaliza um tipo de anestesia (aceita minúsculas).
func ParseAnestesia(codigo string) (Anestesia, error) {
	a := Anestesia(strings.ToUpper(strings.TrimSpace(codigo)))
	if !anestesiasValidas[a] {
		return "", erros.Novo(erros.CategoriaValidacao,
			"tipo de anestesia inválido (esperado NENHUMA, LOCAL, SEDACAO_LIGEIRA ou LOCO_REGIONAL)")
	}
	return a, nil
}

// RequerAnestesista indica se este tipo de anestesia obriga a um anestesista.
func (a Anestesia) RequerAnestesista() bool { return a != AnestesiaNenhuma }
```

- [ ] **Step 5: Estender `episodio_enums.go`**

No bloco `const` juntar a constante e no mapa `tiposEpisodioValidos` juntar a entrada; actualizar a mensagem de erro de `ParseTipoEpisodio`:

```go
const (
	EpisodioConsulta            TipoEpisodio = "CONSULTA"
	EpisodioUrgencia            TipoEpisodio = "URGENCIA"
	EpisodioInternamento        TipoEpisodio = "INTERNAMENTO"
	EpisodioCirurgiaAmbulatoria TipoEpisodio = "CIRURGIA_AMBULATORIA"
)

var tiposEpisodioValidos = map[TipoEpisodio]bool{
	EpisodioConsulta: true, EpisodioUrgencia: true, EpisodioInternamento: true,
	EpisodioCirurgiaAmbulatoria: true,
}
```

E na `ParseTipoEpisodio`, trocar a mensagem para:

```go
		return "", erros.Novo(erros.CategoriaValidacao, "tipo de episódio inválido (esperado CONSULTA, URGENCIA, INTERNAMENTO ou CIRURGIA_AMBULATORIA)")
```

- [ ] **Step 6: Juntar getter `Tipo()` em `episodio.go`**

Logo a seguir ao getter `DoenteID`:

```go
// Tipo devolve o tipo do episódio.
func (e *EpisodioClinico) Tipo() TipoEpisodio { return e.tipo }
```

- [ ] **Step 7: Correr para confirmar que passa**

Run: `go test ./internal/domain/clinico/ -run 'TestAnestesia|TestParseAnestesia|TestParseTipoEpisodio' -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/clinico/anestesia.go internal/domain/clinico/anestesia_test.go internal/domain/clinico/episodio_enums.go internal/domain/clinico/episodio_enums_test.go internal/domain/clinico/episodio.go
git commit -m "$(printf 'feat(clinico): VO Anestesia e tipo de episodio CIRURGIA_AMBULATORIA\n\nAnestesia (4 valores) com RequerAnestesista, novo tipo de episodio e getter\nTipo() no agregado EpisodioClinico.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 4: Domínio — agregado ProcedimentoCirurgico + catálogo (read model)

**Files:**
- Create: `internal/domain/clinico/procedimento_enums.go`
- Create: `internal/domain/clinico/procedimento_cirurgico.go`
- Create: `internal/domain/clinico/catalogo_procedimento.go`
- Modify: `internal/domain/clinico/eventos.go` (juntar `ProcedimentoCirurgicoConcluido`)
- Test: `internal/domain/clinico/procedimento_cirurgico_test.go`

**Interfaces:**
- Consumes: `Anestesia`, `Consentimento` (Task 2/3).
- Produces: `EstadoProcedimento` (`ProcAgendado`, `ProcEmCurso`, `ProcConcluido`, `ProcCancelado`); `DadosNovoProcedimento`; `NovoProcedimento(DadosNovoProcedimento, *Consentimento) (*ProcedimentoCirurgico, error)`; métodos `Iniciar(time.Time) error`, `Concluir(time.Time, complicacoes, observacoes string) error`, `Cancelar(time.Time, motivo string) error`; getters `ID/EpisodioID/Estado/ConsentimentoID`; `Snapshot() SnapshotProcedimento` / `ReconstruirProcedimento`; `RepositorioProcedimentos`; `ResumoProcedimento`; `CatalogoProcedimento`; `RepositorioCatalogoProcedimentos`.

- [ ] **Step 1: Escrever o teste que falha**

```go
package clinico_test

import (
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func consentimentoCirurgiaValido(t *testing.T) *dominio.Consentimento {
	t.Helper()
	c, err := dominio.NovoConsentimento("doente-1", dominio.FinalidadeCirurgia, true, "s3://c.pdf",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("consentimento base inválido: %v", err)
	}
	return c
}

func dadosProc() dominio.DadosNovoProcedimento {
	return dominio.DadosNovoProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura",
		CirurgiaoID: "cir-1", Anestesia: dominio.AnestesiaLocal, AnestesistaID: "an-1",
	}
}

func TestNovoProcedimento_ConsentimentoInvalido(t *testing.T) {
	// Consentimento de tratamento (não cirurgia) → RegraNegocio.
	cons, _ := dominio.NovoConsentimento("doente-1", dominio.FinalidadeTratamento, true, "",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	if _, err := dominio.NovoProcedimento(dadosProc(), cons); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("consentimento não-cirurgia devia falhar com RegraNegocio, veio %v", err)
	}
	// Consentimento nil → RegraNegocio.
	if _, err := dominio.NovoProcedimento(dadosProc(), nil); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("consentimento nil devia falhar com RegraNegocio, veio %v", err)
	}
}

func TestNovoProcedimento_AnestesistaObrigatorio(t *testing.T) {
	d := dadosProc()
	d.AnestesistaID = ""
	if _, err := dominio.NovoProcedimento(d, consentimentoCirurgiaValido(t)); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("anestesia≠NENHUMA sem anestesista devia falhar com Validacao, veio %v", err)
	}
	// Com NENHUMA não é preciso anestesista.
	d.Anestesia = dominio.AnestesiaNenhuma
	if _, err := dominio.NovoProcedimento(d, consentimentoCirurgiaValido(t)); err != nil {
		t.Fatalf("NENHUMA sem anestesista devia ser válido: %v", err)
	}
}

func TestProcedimento_StateMachine(t *testing.T) {
	p, err := dominio.NovoProcedimento(dadosProc(), consentimentoCirurgiaValido(t))
	if err != nil {
		t.Fatalf("construção válida falhou: %v", err)
	}
	if p.Estado() != dominio.ProcAgendado {
		t.Fatalf("esperado AGENDADO, veio %s", p.Estado())
	}
	// Concluir sem iniciar → Conflito.
	if err := p.Concluir(time.Now(), "", ""); err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("concluir sem iniciar devia falhar com Conflito, veio %v", err)
	}
	inicio := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	if err := p.Iniciar(inicio); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}
	if p.Estado() != dominio.ProcEmCurso {
		t.Fatalf("esperado EM_CURSO, veio %s", p.Estado())
	}
	// Iniciar de novo → Conflito.
	if err := p.Iniciar(inicio); err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("iniciar de novo devia falhar com Conflito, veio %v", err)
	}
	// Concluir com fim antes do início → Validacao.
	if err := p.Concluir(inicio.Add(-time.Hour), "", ""); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("fim antes do início devia falhar com Validacao, veio %v", err)
	}
	if err := p.Concluir(inicio.Add(time.Hour), "sem complicações", ""); err != nil {
		t.Fatalf("concluir devia funcionar: %v", err)
	}
	if p.Estado() != dominio.ProcConcluido {
		t.Fatalf("esperado CONCLUIDO, veio %s", p.Estado())
	}
}

func TestProcedimento_Cancelar_SoEmCurso(t *testing.T) {
	p, _ := dominio.NovoProcedimento(dadosProc(), consentimentoCirurgiaValido(t))
	// Cancelar em AGENDADO → Conflito (DDM estrito).
	if err := p.Cancelar(time.Now(), "desmarcado"); err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("cancelar AGENDADO devia falhar com Conflito, veio %v", err)
	}
	inicio := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	_ = p.Iniciar(inicio)
	if err := p.Cancelar(inicio.Add(time.Minute), "complicação intra-op"); err != nil {
		t.Fatalf("cancelar EM_CURSO devia funcionar: %v", err)
	}
	if p.Estado() != dominio.ProcCancelado {
		t.Fatalf("esperado CANCELADO, veio %s", p.Estado())
	}
}
```

- [ ] **Step 2: Correr para confirmar que falha**

Run: `go test ./internal/domain/clinico/ -run TestProcedimento -v`
Expected: FAIL (compilação: `NovoProcedimento` indefinido).

- [ ] **Step 3: Implementar `procedimento_enums.go`**

```go
package clinico

// EstadoProcedimento é o estado do ciclo de vida de um procedimento cirúrgico.
type EstadoProcedimento string

const (
	ProcAgendado  EstadoProcedimento = "AGENDADO"
	ProcEmCurso   EstadoProcedimento = "EM_CURSO"
	ProcConcluido EstadoProcedimento = "CONCLUIDO"
	ProcCancelado EstadoProcedimento = "CANCELADO"
)
```

- [ ] **Step 4: Implementar `procedimento_cirurgico.go`**

```go
package clinico

import (
	"context"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// ProcedimentoCirurgico é um agregado raiz do BC Clínico: um procedimento
// cirúrgico ambulatório de um episódio. O id é gerado pela base de dados.
type ProcedimentoCirurgico struct {
	id              string
	episodioID      string
	codigo          string
	descricao       string
	sala            string
	cirurgiaoID     string
	auxiliarID      string
	anestesia       Anestesia
	anestesistaID   string
	inicio          *time.Time
	fim             *time.Time
	consentimentoID string
	complicacoes    string
	observacoes     string
	estado          EstadoProcedimento
	criadoEm        time.Time
}

// DadosNovoProcedimento agrupa os parâmetros de construção.
type DadosNovoProcedimento struct {
	EpisodioID    string
	Codigo        string
	Descricao     string
	Sala          string
	CirurgiaoID   string
	AuxiliarID    string
	Anestesia     Anestesia
	AnestesistaID string
	Observacoes   string
}

// NovoProcedimento valida as invariantes e devolve o agregado em AGENDADO.
// Recebe o Consentimento (não só o id) para impor a invariante-estrela: só há
// procedimento com consentimento de finalidade CIRURGIA, anexado e vigente.
func NovoProcedimento(d DadosNovoProcedimento, consentimento *Consentimento) (*ProcedimentoCirurgico, error) {
	d.EpisodioID = strings.TrimSpace(d.EpisodioID)
	if d.EpisodioID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "episódio do procedimento em falta")
	}
	d.Codigo = strings.TrimSpace(d.Codigo)
	if d.Codigo == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "código do procedimento em falta")
	}
	d.Descricao = strings.TrimSpace(d.Descricao)
	if d.Descricao == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "descrição do procedimento em falta")
	}
	d.CirurgiaoID = strings.TrimSpace(d.CirurgiaoID)
	if d.CirurgiaoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "cirurgião do procedimento em falta")
	}
	if _, err := ParseAnestesia(string(d.Anestesia)); err != nil {
		return nil, err
	}
	d.AnestesistaID = strings.TrimSpace(d.AnestesistaID)
	if d.Anestesia.RequerAnestesista() && d.AnestesistaID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "anestesista obrigatório quando há anestesia")
	}
	if consentimento == nil {
		return nil, erros.Novo(erros.CategoriaRegraNegocio, "consentimento cirúrgico em falta")
	}
	if consentimento.Finalidade() != FinalidadeCirurgia || !consentimento.TemAnexo() || !consentimento.EstaVigente() {
		return nil, erros.Novo(erros.CategoriaRegraNegocio,
			"consentimento cirúrgico inválido (exige finalidade CIRURGIA, anexo e estar vigente)")
	}
	return &ProcedimentoCirurgico{
		episodioID: d.EpisodioID, codigo: d.Codigo, descricao: d.Descricao,
		sala: strings.TrimSpace(d.Sala), cirurgiaoID: d.CirurgiaoID,
		auxiliarID: strings.TrimSpace(d.AuxiliarID), anestesia: d.Anestesia,
		anestesistaID: d.AnestesistaID, consentimentoID: consentimento.ID(),
		observacoes: strings.TrimSpace(d.Observacoes), estado: ProcAgendado,
	}, nil
}

// Iniciar transita AGENDADO → EM_CURSO.
func (p *ProcedimentoCirurgico) Iniciar(em time.Time) error {
	if p.estado != ProcAgendado {
		return erros.Novo(erros.CategoriaConflito, "só é possível iniciar um procedimento agendado")
	}
	if em.IsZero() {
		return erros.Novo(erros.CategoriaValidacao, "início do procedimento em falta")
	}
	p.estado = ProcEmCurso
	p.inicio = &em
	return nil
}

// Concluir transita EM_CURSO → CONCLUIDO. O fim não pode ser anterior ao início.
func (p *ProcedimentoCirurgico) Concluir(em time.Time, complicacoes, observacoes string) error {
	if p.estado != ProcEmCurso {
		return erros.Novo(erros.CategoriaConflito, "só é possível concluir um procedimento em curso")
	}
	if em.Before(*p.inicio) {
		return erros.Novo(erros.CategoriaValidacao, "o fim do procedimento não pode ser anterior ao início")
	}
	p.estado = ProcConcluido
	p.fim = &em
	p.complicacoes = strings.TrimSpace(complicacoes)
	if obs := strings.TrimSpace(observacoes); obs != "" {
		p.observacoes = obs
	}
	return nil
}

// Cancelar transita EM_CURSO → CANCELADO (cancelamento intra-operatório, DDM
// estrito). O motivo é guardado nas observações e auditado na aplicação.
func (p *ProcedimentoCirurgico) Cancelar(em time.Time, motivo string) error {
	if p.estado != ProcEmCurso {
		return erros.Novo(erros.CategoriaConflito, "só é possível cancelar um procedimento em curso")
	}
	if em.Before(*p.inicio) {
		return erros.Novo(erros.CategoriaValidacao, "o fim do procedimento não pode ser anterior ao início")
	}
	p.estado = ProcCancelado
	p.fim = &em
	if m := strings.TrimSpace(motivo); m != "" {
		p.observacoes = m
	}
	return nil
}

// ID devolve o identificador atribuído pela base de dados.
func (p *ProcedimentoCirurgico) ID() string { return p.id }

// EpisodioID devolve o id do episódio a que o procedimento pertence.
func (p *ProcedimentoCirurgico) EpisodioID() string { return p.episodioID }

// Estado devolve o estado actual.
func (p *ProcedimentoCirurgico) Estado() EstadoProcedimento { return p.estado }

// ConsentimentoID devolve o id do consentimento associado.
func (p *ProcedimentoCirurgico) ConsentimentoID() string { return p.consentimentoID }

// SnapshotProcedimento carrega o estado completo para persistência ou rehidratação.
type SnapshotProcedimento struct {
	ID              string
	EpisodioID      string
	Codigo          string
	Descricao       string
	Sala            string
	CirurgiaoID     string
	AuxiliarID      string
	Anestesia       Anestesia
	AnestesistaID   string
	Inicio          *time.Time
	Fim             *time.Time
	ConsentimentoID string
	Complicacoes    string
	Observacoes     string
	Estado          EstadoProcedimento
	CriadoEm        time.Time
}

// Snapshot devolve o estado completo do agregado.
func (p *ProcedimentoCirurgico) Snapshot() SnapshotProcedimento {
	return SnapshotProcedimento{
		ID: p.id, EpisodioID: p.episodioID, Codigo: p.codigo, Descricao: p.descricao,
		Sala: p.sala, CirurgiaoID: p.cirurgiaoID, AuxiliarID: p.auxiliarID,
		Anestesia: p.anestesia, AnestesistaID: p.anestesistaID, Inicio: p.inicio, Fim: p.fim,
		ConsentimentoID: p.consentimentoID, Complicacoes: p.complicacoes,
		Observacoes: p.observacoes, Estado: p.estado, CriadoEm: p.criadoEm,
	}
}

// ReconstruirProcedimento reconstrói um agregado a partir de um snapshot persistido.
func ReconstruirProcedimento(s SnapshotProcedimento) *ProcedimentoCirurgico {
	return &ProcedimentoCirurgico{
		id: s.ID, episodioID: s.EpisodioID, codigo: s.Codigo, descricao: s.Descricao,
		sala: s.Sala, cirurgiaoID: s.CirurgiaoID, auxiliarID: s.AuxiliarID,
		anestesia: s.Anestesia, anestesistaID: s.AnestesistaID, inicio: s.Inicio, fim: s.Fim,
		consentimentoID: s.ConsentimentoID, complicacoes: s.Complicacoes,
		observacoes: s.Observacoes, estado: s.Estado, criadoEm: s.CriadoEm,
	}
}

// ResumoProcedimento é a projecção de leitura de um procedimento.
type ResumoProcedimento struct {
	ID         string     `json:"id"`
	EpisodioID string     `json:"episodio_id"`
	Codigo     string     `json:"codigo_procedimento"`
	Descricao  string     `json:"descricao"`
	Estado     string     `json:"estado"`
	Anestesia  string     `json:"anestesia"`
	Inicio     *time.Time `json:"inicio,omitempty"`
	Fim        *time.Time `json:"fim,omitempty"`
	CriadoEm   time.Time  `json:"criado_em"`
}

// RepositorioProcedimentos é a porta de saída de persistência de procedimentos.
type RepositorioProcedimentos interface {
	Guardar(ctx context.Context, p *ProcedimentoCirurgico) (string, error)
	ObterPorID(ctx context.Context, id string) (*ProcedimentoCirurgico, error)
	ListarPorEpisodio(ctx context.Context, episodioID string) ([]ResumoProcedimento, error)
}
```

- [ ] **Step 5: Implementar `catalogo_procedimento.go`**

```go
package clinico

import "context"

// CatalogoProcedimento é a projecção de leitura de uma entrada do catálogo de
// procedimentos cirúrgicos (dados de referência).
type CatalogoProcedimento struct {
	Codigo             string `json:"codigo"`
	Descricao          string `json:"descricao"`
	Especialidade      string `json:"especialidade,omitempty"`
	DuracaoEstimadaMin int    `json:"duracao_estimada_min,omitempty"`
	RequerAnestesista  bool   `json:"requer_anestesista"`
	Activo             bool   `json:"activo"`
}

// RepositorioCatalogoProcedimentos é a porta de leitura do catálogo.
type RepositorioCatalogoProcedimentos interface {
	ObterPorCodigo(ctx context.Context, codigo string) (*CatalogoProcedimento, error)
}
```

- [ ] **Step 6: Juntar o evento em `eventos.go`**

Acrescentar (scaffolding — definido para consumo futuro por Financeiro/reporting, não emitido nesta fatia, coerente com Sprint 9/10). Seguir o padrão dos eventos existentes (implementam `NomeEvento()`/`OcorridoEm()`; `eventos.go` já importa `time`):

```go
// ProcedimentoCirurgicoConcluido é emitido quando um procedimento é concluído.
// Consumido futuramente por Financeiro (linha de factura) e reporting (MINSA).
type ProcedimentoCirurgicoConcluido struct {
	ProcedimentoID string
	EpisodioID     string
	Codigo         string
	Em             time.Time
}

func (e ProcedimentoCirurgicoConcluido) NomeEvento() string    { return "clinico.procedimento.concluido" }
func (e ProcedimentoCirurgicoConcluido) OcorridoEm() time.Time { return e.Em }
```

- [ ] **Step 7: Correr para confirmar que passa**

Run: `go test ./internal/domain/clinico/ -run TestProcedimento -v && go build ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/clinico/procedimento_enums.go internal/domain/clinico/procedimento_cirurgico.go internal/domain/clinico/catalogo_procedimento.go internal/domain/clinico/eventos.go internal/domain/clinico/procedimento_cirurgico_test.go
git commit -m "$(printf 'feat(clinico): agregado ProcedimentoCirurgico com state machine\n\nAGENDADO->EM_CURSO->CONCLUIDO/CANCELADO (DDM estrito), invariantes de\nconsentimento cirurgico e anestesista, catalogo (read model) e evento\nscaffolding ProcedimentoCirurgicoConcluido.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 5: Aplicação — casos de uso de Consentimento

**Files:**
- Modify: `internal/application/clinico/ports.go` (juntar DTOs/reexports de consentimento)
- Create: `internal/application/clinico/registar_consentimento.go`
- Create: `internal/application/clinico/revogar_consentimento.go`
- Create: `internal/application/clinico/listar_consentimentos.go`
- Create: `internal/application/clinico/obter_consentimento.go`
- Create: `internal/application/clinico/mapa_cirurgia.go` (mapeadores partilhados por Task 5 e 6)
- Test: `internal/application/clinico/consentimento_test.go`
- Test: `internal/application/clinico/fakes_cirurgia_test.go` (fakes partilhados por Task 5 e 6)

**Interfaces:**
- Consumes: `dominio.RepositorioConsentimentos`, `dominio.RepositorioDoentes`, `Auditor`.
- Produces: `CasoRegistarConsentimento`/`CasoRevogarConsentimento`/`CasoListarConsentimentos`/`CasoObterConsentimento` (cada um com `NovoCaso...` e método `Executar`); DTOs `DadosNovoConsentimento`, `DetalheConsentimento`; reexports `FiltroConsentimentos`, `ResumoConsentimento`; `paraDetalheConsentimento(*dominio.Consentimento) DetalheConsentimento`.

- [ ] **Step 1: Juntar DTOs/reexports em `ports.go`**

No fim do ficheiro (mantendo o `import "time"` já presente):

```go
// --- Consentimento (LPDP) ---

// Reexports dos read-models de consentimento.
type (
	FiltroConsentimentos = dominio.FiltroConsentimentos
	ResumoConsentimento  = dominio.ResumoConsentimento
)

// DadosNovoConsentimento é a entrada do registo de consentimento. DoenteID vem do
// caminho; ConcedidoEm é opcional (default: momento do registo).
type DadosNovoConsentimento struct {
	DoenteID     string
	Finalidade   string
	Concedido    bool
	DocumentoURL string
	ConcedidoEm  *time.Time
}

// DetalheConsentimento é o detalhe de um consentimento numa resposta.
type DetalheConsentimento struct {
	ID           string     `json:"id"`
	DoenteID     string     `json:"doente_id"`
	Finalidade   string     `json:"finalidade"`
	Concedido    bool       `json:"concedido"`
	DocumentoURL string     `json:"documento_url,omitempty"`
	ConcedidoEm  time.Time  `json:"concedido_em"`
	RevogadoEm   *time.Time `json:"revogado_em,omitempty"`
	Vigente      bool       `json:"vigente"`
}
```

- [ ] **Step 2: Criar `mapa_cirurgia.go`**

```go
package clinico

import dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"

// paraDetalheConsentimento projecta o agregado num DTO de resposta.
func paraDetalheConsentimento(c *dominio.Consentimento) DetalheConsentimento {
	s := c.Snapshot()
	return DetalheConsentimento{
		ID: s.ID, DoenteID: s.DoenteID, Finalidade: string(s.Finalidade),
		Concedido: s.Concedido, DocumentoURL: s.DocumentoURL,
		ConcedidoEm: s.ConcedidoEm, RevogadoEm: s.RevogadoEm,
		Vigente: c.EstaVigente(),
	}
}
```

- [ ] **Step 3: Escrever `fakes_cirurgia_test.go` (fakes em memória)**

```go
package clinico_test

import (
	"context"
	"strconv"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeConsentimentos é um RepositorioConsentimentos em memória.
type fakeConsentimentos struct {
	porID map[string]*clinico.Consentimento
	seq   int
	lista []clinico.ResumoConsentimento
}

func novoFakeConsentimentos() *fakeConsentimentos {
	return &fakeConsentimentos{porID: map[string]*clinico.Consentimento{}}
}

func (f *fakeConsentimentos) Guardar(_ context.Context, c *clinico.Consentimento) (string, error) {
	s := c.Snapshot()
	id := s.ID
	if id == "" {
		f.seq++
		id = "cons-" + strconv.Itoa(f.seq)
		s.ID = id
	}
	f.porID[id] = clinico.ReconstruirConsentimento(s)
	return id, nil
}

func (f *fakeConsentimentos) ObterPorID(_ context.Context, id string) (*clinico.Consentimento, error) {
	c, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "consentimento não encontrado")
	}
	return c, nil
}

func (f *fakeConsentimentos) ListarPorDoente(_ context.Context, _ string, _ clinico.FiltroConsentimentos) ([]clinico.ResumoConsentimento, error) {
	return f.lista, nil
}

// fakeProcedimentos é um RepositorioProcedimentos em memória.
type fakeProcedimentos struct {
	porID map[string]*clinico.ProcedimentoCirurgico
	seq   int
	lista []clinico.ResumoProcedimento
}

func novoFakeProcedimentos() *fakeProcedimentos {
	return &fakeProcedimentos{porID: map[string]*clinico.ProcedimentoCirurgico{}}
}

func (f *fakeProcedimentos) Guardar(_ context.Context, p *clinico.ProcedimentoCirurgico) (string, error) {
	s := p.Snapshot()
	id := s.ID
	if id == "" {
		f.seq++
		id = "proc-" + strconv.Itoa(f.seq)
		s.ID = id
	}
	f.porID[id] = clinico.ReconstruirProcedimento(s)
	return id, nil
}

func (f *fakeProcedimentos) ObterPorID(_ context.Context, id string) (*clinico.ProcedimentoCirurgico, error) {
	p, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "procedimento não encontrado")
	}
	return p, nil
}

func (f *fakeProcedimentos) ListarPorEpisodio(_ context.Context, _ string) ([]clinico.ResumoProcedimento, error) {
	return f.lista, nil
}

// fakeCatalogo é um RepositorioCatalogoProcedimentos em memória.
type fakeCatalogo struct {
	porCodigo map[string]*clinico.CatalogoProcedimento
}

func novoFakeCatalogo() *fakeCatalogo {
	return &fakeCatalogo{porCodigo: map[string]*clinico.CatalogoProcedimento{
		"PRC001": {Codigo: "PRC001", Descricao: "Sutura", Activo: true},
	}}
}

func (f *fakeCatalogo) ObterPorCodigo(_ context.Context, codigo string) (*clinico.CatalogoProcedimento, error) {
	c, ok := f.porCodigo[codigo]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "procedimento do catálogo não encontrado")
	}
	return c, nil
}

// fakeAuditorCir regista as acções auditadas.
type fakeAuditorCir struct{ accoes []string }

func (f *fakeAuditorCir) Registar(_ context.Context, r auditoria.Registo) error {
	f.accoes = append(f.accoes, r.Accao)
	return nil
}
```

> Nota ao implementador: se já existir no pacote de testes um fake `Auditor`
> reutilizável (ex.: em `fakes_test.go`/`fakes_episodio_test.go`), usa-o em vez de
> `fakeAuditorCir` para não duplicar; remove `fakeAuditorCir` nesse caso. O fake de
> doentes existente (`fakeRepo`) é reutilizável para `RepositorioDoentes`.

- [ ] **Step 4: Escrever o teste que falha (`consentimento_test.go`)**

```go
package clinico_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRegistarConsentimento_CirurgiaSemAnexo(t *testing.T) {
	repoC := novoFakeConsentimentos()
	repoD := novoFakeRepo()
	repoD.porID["doente-1"] = novoDoenteValido(t) // helper existente nos testes de doente
	aud := &fakeAuditorCir{}
	uc := app.NovoCasoRegistarConsentimento(repoC, repoD, aud)

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosNovoConsentimento{
		DoenteID: "doente-1", Finalidade: "CIRURGIA", Concedido: true, DocumentoURL: "",
	})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("cirurgia sem anexo devia falhar com RegraNegocio, veio %v", err)
	}
}

func TestRegistarConsentimento_Sucesso(t *testing.T) {
	repoC := novoFakeConsentimentos()
	repoD := novoFakeRepo()
	repoD.porID["doente-1"] = novoDoenteValido(t)
	aud := &fakeAuditorCir{}
	uc := app.NovoCasoRegistarConsentimento(repoC, repoD, aud)

	out, err := uc.Executar(context.Background(), "actor-1", app.DadosNovoConsentimento{
		DoenteID: "doente-1", Finalidade: "TRATAMENTO", Concedido: true,
	})
	if err != nil {
		t.Fatalf("registo devia funcionar: %v", err)
	}
	if out.ID == "" || !out.Vigente {
		t.Fatalf("esperado consentimento vigente com id, veio %+v", out)
	}
	if len(aud.accoes) != 1 || aud.accoes[0] != "clinico.consentimento.registado" {
		t.Fatalf("esperada auditoria clinico.consentimento.registado, veio %v", aud.accoes)
	}
}

func TestRevogarConsentimento(t *testing.T) {
	repoC := novoFakeConsentimentos()
	c, _ := clinico.NovoConsentimento("doente-1", clinico.FinalidadeTratamento, true, "", nowUTC())
	id, _ := repoC.Guardar(context.Background(), c)
	aud := &fakeAuditorCir{}
	uc := app.NovoCasoRevogarConsentimento(repoC, aud)

	out, err := uc.Executar(context.Background(), "actor-1", id)
	if err != nil {
		t.Fatalf("revogar devia funcionar: %v", err)
	}
	if out.Vigente {
		t.Fatalf("consentimento revogado não devia estar vigente")
	}
	if len(aud.accoes) != 1 || aud.accoes[0] != "clinico.consentimento.revogado" {
		t.Fatalf("esperada auditoria clinico.consentimento.revogado, veio %v", aud.accoes)
	}
}
```

> Nota: `novoDoenteValido(t)` e `nowUTC()` — se não existirem helpers equivalentes
> nos testes do pacote, cria-os no `fakes_cirurgia_test.go`: `nowUTC()` devolve
> `time.Date(2026,7,13,0,0,0,0,time.UTC)`; `novoDoenteValido(t)` constrói um
> `*clinico.Doente` mínimo via `ReconstruirDoente` (usa o helper/factory já usado
> pelos testes de doente existentes se disponível).

- [ ] **Step 5: Correr para confirmar que falha**

Run: `go test ./internal/application/clinico/ -run 'Consentimento' -v`
Expected: FAIL (compilação: `NovoCasoRegistarConsentimento` indefinido).

- [ ] **Step 6: Implementar `registar_consentimento.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarConsentimento regista um consentimento LPDP de um doente e audita.
type CasoRegistarConsentimento struct {
	consentimentos dominio.RepositorioConsentimentos
	doentes        dominio.RepositorioDoentes
	auditor        Auditor
	agora          func() time.Time
}

// NovoCasoRegistarConsentimento constrói o caso de uso.
func NovoCasoRegistarConsentimento(c dominio.RepositorioConsentimentos, d dominio.RepositorioDoentes, a Auditor) *CasoRegistarConsentimento {
	return &CasoRegistarConsentimento{consentimentos: c, doentes: d, auditor: a, agora: time.Now}
}

// Executar valida o doente, cria o consentimento, persiste e audita.
func (uc *CasoRegistarConsentimento) Executar(ctx context.Context, actor string, dados DadosNovoConsentimento) (DetalheConsentimento, error) {
	if _, err := uc.doentes.ObterPorID(ctx, dados.DoenteID); err != nil {
		return DetalheConsentimento{}, err
	}
	fin, err := dominio.ParseFinalidade(dados.Finalidade)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	quando := uc.agora()
	if dados.ConcedidoEm != nil {
		quando = *dados.ConcedidoEm
	}
	c, err := dominio.NovoConsentimento(dados.DoenteID, fin, dados.Concedido, dados.DocumentoURL, quando)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	id, err := uc.consentimentos.Guardar(ctx, c)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.consentimento.registado",
		Entidade: "consentimento", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheConsentimento{}, err
	}
	final, err := uc.consentimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	return paraDetalheConsentimento(final), nil
}
```

- [ ] **Step 7: Implementar `revogar_consentimento.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRevogarConsentimento revoga um consentimento e audita.
type CasoRevogarConsentimento struct {
	consentimentos dominio.RepositorioConsentimentos
	auditor        Auditor
	agora          func() time.Time
}

// NovoCasoRevogarConsentimento constrói o caso de uso.
func NovoCasoRevogarConsentimento(c dominio.RepositorioConsentimentos, a Auditor) *CasoRevogarConsentimento {
	return &CasoRevogarConsentimento{consentimentos: c, auditor: a, agora: time.Now}
}

// Executar carrega, revoga, persiste e audita.
func (uc *CasoRevogarConsentimento) Executar(ctx context.Context, actor, id string) (DetalheConsentimento, error) {
	c, err := uc.consentimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	if err := c.Revogar(uc.agora()); err != nil {
		return DetalheConsentimento{}, err
	}
	if _, err := uc.consentimentos.Guardar(ctx, c); err != nil {
		return DetalheConsentimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.consentimento.revogado",
		Entidade: "consentimento", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheConsentimento{}, err
	}
	final, err := uc.consentimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	return paraDetalheConsentimento(final), nil
}
```

- [ ] **Step 8: Implementar `listar_consentimentos.go` e `obter_consentimento.go`**

`listar_consentimentos.go`:

```go
package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

// CasoListarConsentimentos lista os consentimentos de um doente (não audita — leitura).
type CasoListarConsentimentos struct {
	consentimentos dominio.RepositorioConsentimentos
}

// NovoCasoListarConsentimentos constrói o caso de uso.
func NovoCasoListarConsentimentos(c dominio.RepositorioConsentimentos) *CasoListarConsentimentos {
	return &CasoListarConsentimentos{consentimentos: c}
}

// Executar devolve os consentimentos do doente segundo o filtro.
func (uc *CasoListarConsentimentos) Executar(ctx context.Context, doenteID string, filtro FiltroConsentimentos) ([]ResumoConsentimento, error) {
	return uc.consentimentos.ListarPorDoente(ctx, doenteID, filtro)
}
```

`obter_consentimento.go`:

```go
package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

// CasoObterConsentimento devolve o detalhe de um consentimento (não audita — leitura).
type CasoObterConsentimento struct {
	consentimentos dominio.RepositorioConsentimentos
}

// NovoCasoObterConsentimento constrói o caso de uso.
func NovoCasoObterConsentimento(c dominio.RepositorioConsentimentos) *CasoObterConsentimento {
	return &CasoObterConsentimento{consentimentos: c}
}

// Executar carrega e projecta o consentimento.
func (uc *CasoObterConsentimento) Executar(ctx context.Context, id string) (DetalheConsentimento, error) {
	c, err := uc.consentimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	return paraDetalheConsentimento(c), nil
}
```

- [ ] **Step 9: Correr para confirmar que passa**

Run: `go test ./internal/application/clinico/ -run 'Consentimento' -v`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/application/clinico/ports.go internal/application/clinico/registar_consentimento.go internal/application/clinico/revogar_consentimento.go internal/application/clinico/listar_consentimentos.go internal/application/clinico/obter_consentimento.go internal/application/clinico/mapa_cirurgia.go internal/application/clinico/consentimento_test.go internal/application/clinico/fakes_cirurgia_test.go
git commit -m "$(printf 'feat(clinico): casos de uso de consentimento (registar/revogar/listar/obter)\n\nRegisto valida doente e finalidade; revogar audita; leituras nao auditam.\nDTOs, reexports e mapeador de detalhe.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 6: Aplicação — casos de uso de Procedimento Cirúrgico

**Files:**
- Modify: `internal/application/clinico/ports.go` (juntar DTOs/reexports de procedimento)
- Modify: `internal/application/clinico/mapa_cirurgia.go` (juntar `paraDetalheProcedimento`)
- Create: `internal/application/clinico/agendar_procedimento.go`
- Create: `internal/application/clinico/iniciar_procedimento.go`
- Create: `internal/application/clinico/concluir_procedimento.go`
- Create: `internal/application/clinico/cancelar_procedimento.go`
- Create: `internal/application/clinico/obter_procedimento.go`
- Create: `internal/application/clinico/listar_procedimentos.go`
- Test: `internal/application/clinico/procedimento_test.go`

**Interfaces:**
- Consumes: `dominio.RepositorioProcedimentos`, `dominio.RepositorioConsentimentos`, `dominio.RepositorioCatalogoProcedimentos`, `dominio.RepositorioEpisodios`, `Auditor` (fakes da Task 5).
- Produces: `CasoAgendarProcedimento`/`CasoIniciarProcedimento`/`CasoConcluirProcedimento`/`CasoCancelarProcedimento`/`CasoObterProcedimento`/`CasoListarProcedimentos` (cada um com `NovoCaso...` e `Executar`); DTOs `DadosAgendarProcedimento`, `DadosConcluirProcedimento`, `DetalheProcedimento`; reexport `ResumoProcedimento`; `paraDetalheProcedimento(*dominio.ProcedimentoCirurgico) DetalheProcedimento`.

- [ ] **Step 1: Juntar DTOs/reexports em `ports.go`**

```go
// --- Procedimento Cirúrgico ---

// Reexport do read-model de procedimento.
type ResumoProcedimento = dominio.ResumoProcedimento

// DadosAgendarProcedimento é a entrada do agendamento. EpisodioID vem do caminho.
type DadosAgendarProcedimento struct {
	EpisodioID      string
	Codigo          string
	Descricao       string
	Sala            string
	CirurgiaoID     string
	AuxiliarID      string
	Anestesia       string
	AnestesistaID   string
	ConsentimentoID string
	Observacoes     string
}

// DadosConcluirProcedimento é a entrada da conclusão.
type DadosConcluirProcedimento struct {
	Complicacoes string
	Observacoes  string
}

// DetalheProcedimento é o detalhe de um procedimento numa resposta.
type DetalheProcedimento struct {
	ID              string     `json:"id"`
	EpisodioID      string     `json:"episodio_id"`
	Codigo          string     `json:"codigo_procedimento"`
	Descricao       string     `json:"descricao"`
	Sala            string     `json:"sala,omitempty"`
	CirurgiaoID     string     `json:"cirurgiao_id"`
	AuxiliarID      string     `json:"auxiliar_id,omitempty"`
	Anestesia       string     `json:"anestesia"`
	AnestesistaID   string     `json:"anestesista_id,omitempty"`
	ConsentimentoID string     `json:"consentimento_id"`
	Inicio          *time.Time `json:"inicio,omitempty"`
	Fim             *time.Time `json:"fim,omitempty"`
	Complicacoes    string     `json:"complicacoes,omitempty"`
	Observacoes     string     `json:"observacoes,omitempty"`
	Estado          string     `json:"estado"`
	CriadoEm        time.Time  `json:"criado_em"`
}
```

- [ ] **Step 2: Juntar `paraDetalheProcedimento` em `mapa_cirurgia.go`**

```go
// paraDetalheProcedimento projecta o agregado num DTO de resposta.
func paraDetalheProcedimento(p *dominio.ProcedimentoCirurgico) DetalheProcedimento {
	s := p.Snapshot()
	return DetalheProcedimento{
		ID: s.ID, EpisodioID: s.EpisodioID, Codigo: s.Codigo, Descricao: s.Descricao,
		Sala: s.Sala, CirurgiaoID: s.CirurgiaoID, AuxiliarID: s.AuxiliarID,
		Anestesia: string(s.Anestesia), AnestesistaID: s.AnestesistaID,
		ConsentimentoID: s.ConsentimentoID, Inicio: s.Inicio, Fim: s.Fim,
		Complicacoes: s.Complicacoes, Observacoes: s.Observacoes,
		Estado: string(s.Estado), CriadoEm: s.CriadoEm,
	}
}
```

- [ ] **Step 3: Escrever o teste que falha (`procedimento_test.go`)**

```go
package clinico_test

import (
	"context"
	"testing"
	"time"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// episodioCirurgico devolve um episódio ABERTO de cirurgia ambulatória para os fakes.
func episodioCirurgico(doenteID string) *clinico.EpisodioClinico {
	return clinico.ReconstruirEpisodio(clinico.SnapshotEpisodio{
		ID: "ep-1", DoenteID: doenteID, Tipo: clinico.EpisodioCirurgiaAmbulatoria,
		EspecialidadeID: "esp-1", MedicoID: "med-1", Inicio: nowUTC(),
		Estado: clinico.EstadoEpisodioAberto,
	})
}

func consentimentoGuardado(t *testing.T, repo *fakeConsentimentos, doenteID string) string {
	t.Helper()
	c, err := clinico.NovoConsentimento(doenteID, clinico.FinalidadeCirurgia, true, "s3://c.pdf", nowUTC())
	if err != nil {
		t.Fatalf("consentimento inválido: %v", err)
	}
	id, _ := repo.Guardar(context.Background(), c)
	return id
}

func TestAgendarProcedimento_EpisodioNaoCirurgico(t *testing.T) {
	repoE := novoFakeEpisodios()
	repoE.porID["ep-2"] = clinico.ReconstruirEpisodio(clinico.SnapshotEpisodio{
		ID: "ep-2", DoenteID: "doente-1", Tipo: clinico.EpisodioConsulta,
		EspecialidadeID: "esp-1", MedicoID: "med-1", Inicio: nowUTC(), Estado: clinico.EstadoEpisodioAberto,
	})
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, novoFakeCatalogo(), &fakeAuditorCir{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-2", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("episódio não-cirúrgico devia falhar com Conflito, veio %v", err)
	}
}

func TestAgendarProcedimento_ConsentimentoDeOutroDoente(t *testing.T) {
	repoE := novoFakeEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-OUTRO")
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, novoFakeCatalogo(), &fakeAuditorCir{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("consentimento de outro doente devia falhar com Validacao, veio %v", err)
	}
}

func TestProcedimento_CicloCompleto(t *testing.T) {
	repoE := novoFakeEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditorCir{}

	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	if det.Estado != "AGENDADO" {
		t.Fatalf("esperado AGENDADO, veio %s", det.Estado)
	}

	iniciar := app.NovoCasoIniciarProcedimento(repoP, aud)
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}

	concluir := app.NovoCasoConcluirProcedimento(repoP, aud)
	fim, err := concluir.Executar(context.Background(), "actor-1", det.ID, app.DadosConcluirProcedimento{Complicacoes: "nenhuma"})
	if err != nil {
		t.Fatalf("concluir devia funcionar: %v", err)
	}
	if fim.Estado != "CONCLUIDO" {
		t.Fatalf("esperado CONCLUIDO, veio %s", fim.Estado)
	}
	esperadas := []string{"clinico.procedimento.agendado", "clinico.procedimento.iniciado", "clinico.procedimento.concluido"}
	for i, a := range esperadas {
		if i >= len(aud.accoes) || aud.accoes[i] != a {
			t.Fatalf("auditoria esperada %v, veio %v", esperadas, aud.accoes)
		}
	}
	_ = time.Now
}
```

> Nota: `novoFakeEpisodios()` — se não existir já um fake de `RepositorioEpisodios`
> nos testes (`fakes_episodio_test.go`), cria-o em `fakes_cirurgia_test.go` com um
> `map[string]*clinico.EpisodioClinico` e os métodos da interface
> (`Guardar`/`ObterPorID`/`Listar...`); reutiliza o existente se disponível.

- [ ] **Step 4: Correr para confirmar que falha**

Run: `go test ./internal/application/clinico/ -run 'Procedimento' -v`
Expected: FAIL (compilação: casos indefinidos).

- [ ] **Step 5: Implementar `agendar_procedimento.go`**

```go
package clinico

import (
	"context"
	"strings"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoAgendarProcedimento agenda um procedimento cirúrgico num episódio de
// cirurgia ambulatória aberto, validando catálogo e consentimento.
type CasoAgendarProcedimento struct {
	procedimentos  dominio.RepositorioProcedimentos
	episodios      dominio.RepositorioEpisodios
	consentimentos dominio.RepositorioConsentimentos
	catalogo       dominio.RepositorioCatalogoProcedimentos
	auditor        Auditor
	agora          func() time.Time
}

// NovoCasoAgendarProcedimento constrói o caso de uso.
func NovoCasoAgendarProcedimento(
	p dominio.RepositorioProcedimentos, e dominio.RepositorioEpisodios,
	c dominio.RepositorioConsentimentos, cat dominio.RepositorioCatalogoProcedimentos, a Auditor,
) *CasoAgendarProcedimento {
	return &CasoAgendarProcedimento{procedimentos: p, episodios: e, consentimentos: c, catalogo: cat, auditor: a, agora: time.Now}
}

// Executar valida episódio/catálogo/consentimento, cria o procedimento e audita.
func (uc *CasoAgendarProcedimento) Executar(ctx context.Context, actor string, dados DadosAgendarProcedimento) (DetalheProcedimento, error) {
	ep, err := uc.episodios.ObterPorID(ctx, dados.EpisodioID)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if ep.Tipo() != dominio.EpisodioCirurgiaAmbulatoria {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaConflito, "o episódio não é de cirurgia ambulatória")
	}
	if ep.Estado() != dominio.EstadoEpisodioAberto {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaConflito, "só é possível agendar procedimentos num episódio aberto")
	}
	cat, err := uc.catalogo.ObterPorCodigo(ctx, dados.Codigo)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if !cat.Activo {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaValidacao, "procedimento do catálogo inactivo")
	}
	cons, err := uc.consentimentos.ObterPorID(ctx, dados.ConsentimentoID)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if cons.DoenteID() != ep.DoenteID() {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaValidacao, "o consentimento não pertence ao doente do episódio")
	}
	anestesia, err := dominio.ParseAnestesia(dados.Anestesia)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if cat.RequerAnestesista && strings.TrimSpace(dados.AnestesistaID) == "" {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaValidacao, "este procedimento exige anestesista")
	}
	proc, err := dominio.NovoProcedimento(dominio.DadosNovoProcedimento{
		EpisodioID: dados.EpisodioID, Codigo: dados.Codigo, Descricao: dados.Descricao,
		Sala: dados.Sala, CirurgiaoID: dados.CirurgiaoID, AuxiliarID: dados.AuxiliarID,
		Anestesia: anestesia, AnestesistaID: dados.AnestesistaID, Observacoes: dados.Observacoes,
	}, cons)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	id, err := uc.procedimentos.Guardar(ctx, proc)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.procedimento.agendado",
		Entidade: "procedimento", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheProcedimento{}, err
	}
	final, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	return paraDetalheProcedimento(final), nil
}
```

- [ ] **Step 6: Implementar `iniciar_procedimento.go`, `concluir_procedimento.go`, `cancelar_procedimento.go`**

`iniciar_procedimento.go`:

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoIniciarProcedimento transita um procedimento AGENDADO para EM_CURSO.
type CasoIniciarProcedimento struct {
	procedimentos dominio.RepositorioProcedimentos
	auditor       Auditor
	agora         func() time.Time
}

// NovoCasoIniciarProcedimento constrói o caso de uso.
func NovoCasoIniciarProcedimento(p dominio.RepositorioProcedimentos, a Auditor) *CasoIniciarProcedimento {
	return &CasoIniciarProcedimento{procedimentos: p, auditor: a, agora: time.Now}
}

// Executar carrega, inicia, persiste e audita.
func (uc *CasoIniciarProcedimento) Executar(ctx context.Context, actor, id string) (DetalheProcedimento, error) {
	p, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if err := p.Iniciar(uc.agora()); err != nil {
		return DetalheProcedimento{}, err
	}
	if _, err := uc.procedimentos.Guardar(ctx, p); err != nil {
		return DetalheProcedimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.procedimento.iniciado",
		Entidade: "procedimento", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheProcedimento{}, err
	}
	final, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	return paraDetalheProcedimento(final), nil
}
```

`concluir_procedimento.go`:

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoConcluirProcedimento transita um procedimento EM_CURSO para CONCLUIDO.
type CasoConcluirProcedimento struct {
	procedimentos dominio.RepositorioProcedimentos
	auditor       Auditor
	agora         func() time.Time
}

// NovoCasoConcluirProcedimento constrói o caso de uso.
func NovoCasoConcluirProcedimento(p dominio.RepositorioProcedimentos, a Auditor) *CasoConcluirProcedimento {
	return &CasoConcluirProcedimento{procedimentos: p, auditor: a, agora: time.Now}
}

// Executar carrega, conclui, persiste e audita.
func (uc *CasoConcluirProcedimento) Executar(ctx context.Context, actor, id string, dados DadosConcluirProcedimento) (DetalheProcedimento, error) {
	p, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if err := p.Concluir(uc.agora(), dados.Complicacoes, dados.Observacoes); err != nil {
		return DetalheProcedimento{}, err
	}
	if _, err := uc.procedimentos.Guardar(ctx, p); err != nil {
		return DetalheProcedimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.procedimento.concluido",
		Entidade: "procedimento", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheProcedimento{}, err
	}
	final, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	return paraDetalheProcedimento(final), nil
}
```

`cancelar_procedimento.go`:

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoCancelarProcedimento transita um procedimento EM_CURSO para CANCELADO.
type CasoCancelarProcedimento struct {
	procedimentos dominio.RepositorioProcedimentos
	auditor       Auditor
	agora         func() time.Time
}

// NovoCasoCancelarProcedimento constrói o caso de uso.
func NovoCasoCancelarProcedimento(p dominio.RepositorioProcedimentos, a Auditor) *CasoCancelarProcedimento {
	return &CasoCancelarProcedimento{procedimentos: p, auditor: a, agora: time.Now}
}

// Executar carrega, cancela (com motivo), persiste e audita.
func (uc *CasoCancelarProcedimento) Executar(ctx context.Context, actor, id, motivo string) (DetalheProcedimento, error) {
	p, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if err := p.Cancelar(uc.agora(), motivo); err != nil {
		return DetalheProcedimento{}, err
	}
	if _, err := uc.procedimentos.Guardar(ctx, p); err != nil {
		return DetalheProcedimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.procedimento.cancelado",
		Entidade: "procedimento", EntidadeID: id, OcorridoEm: uc.agora(), Detalhe: motivo,
	}); err != nil {
		return DetalheProcedimento{}, err
	}
	final, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	return paraDetalheProcedimento(final), nil
}
```

- [ ] **Step 7: Implementar `obter_procedimento.go` e `listar_procedimentos.go`**

`obter_procedimento.go`:

```go
package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

// CasoObterProcedimento devolve o detalhe de um procedimento (não audita).
type CasoObterProcedimento struct {
	procedimentos dominio.RepositorioProcedimentos
}

// NovoCasoObterProcedimento constrói o caso de uso.
func NovoCasoObterProcedimento(p dominio.RepositorioProcedimentos) *CasoObterProcedimento {
	return &CasoObterProcedimento{procedimentos: p}
}

// Executar carrega e projecta o procedimento.
func (uc *CasoObterProcedimento) Executar(ctx context.Context, id string) (DetalheProcedimento, error) {
	p, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	return paraDetalheProcedimento(p), nil
}
```

`listar_procedimentos.go`:

```go
package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

// CasoListarProcedimentos lista os procedimentos de um episódio (não audita).
type CasoListarProcedimentos struct {
	procedimentos dominio.RepositorioProcedimentos
}

// NovoCasoListarProcedimentos constrói o caso de uso.
func NovoCasoListarProcedimentos(p dominio.RepositorioProcedimentos) *CasoListarProcedimentos {
	return &CasoListarProcedimentos{procedimentos: p}
}

// Executar devolve os procedimentos do episódio.
func (uc *CasoListarProcedimentos) Executar(ctx context.Context, episodioID string) ([]ResumoProcedimento, error) {
	return uc.procedimentos.ListarPorEpisodio(ctx, episodioID)
}
```

- [ ] **Step 8: Correr para confirmar que passa**

Run: `go test ./internal/application/clinico/ -run 'Procedimento' -v && go build ./...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/application/clinico/ports.go internal/application/clinico/mapa_cirurgia.go internal/application/clinico/agendar_procedimento.go internal/application/clinico/iniciar_procedimento.go internal/application/clinico/concluir_procedimento.go internal/application/clinico/cancelar_procedimento.go internal/application/clinico/obter_procedimento.go internal/application/clinico/listar_procedimentos.go internal/application/clinico/procedimento_test.go
git commit -m "$(printf 'feat(clinico): casos de uso de procedimento cirurgico\n\nAgendar (valida episodio cirurgico aberto, catalogo activo, consentimento do\ndoente, requer_anestesista), iniciar, concluir, cancelar, obter e listar.\nAuditoria de cada transicao.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 7: Adaptadores — pgrepo (consentimentos, procedimentos, catálogo)

**Files:**
- Create: `internal/adapters/pgrepo/consentimentos_repo.go`
- Create: `internal/adapters/pgrepo/procedimentos_repo.go`
- Create: `internal/adapters/pgrepo/catalogo_procedimentos_repo.go`

**Interfaces:**
- Consumes: `dominio.Consentimento`/`ProcedimentoCirurgico`/`CatalogoProcedimento` (snapshots/reconstruir), `dominio.RepositorioConsentimentos`/`RepositorioProcedimentos`/`RepositorioCatalogoProcedimentos`.
- Produces: `NovoRepositorioConsentimentos(*pgxpool.Pool)`, `NovoRepositorioProcedimentos(*pgxpool.Pool)`, `NovoRepositorioCatalogoProcedimentos(*pgxpool.Pool)`.

Nota: pgrepo é integração-only (excluído do gate unitário de adaptadores). Sem testes unitários próprios; validado na Task 10.

- [ ] **Step 1: Implementar `consentimentos_repo.go`**

```go
package pgrepo

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioConsentimentos implementa dominio.RepositorioConsentimentos com pgx.
type RepositorioConsentimentos struct {
	pool *pgxpool.Pool
}

// NovoRepositorioConsentimentos constrói o repositório sobre o pool pgx.
func NovoRepositorioConsentimentos(pool *pgxpool.Pool) *RepositorioConsentimentos {
	return &RepositorioConsentimentos{pool: pool}
}

// Guardar insere (id vazio) ou actualiza a revogação (id presente).
func (r *RepositorioConsentimentos) Guardar(ctx context.Context, c *dominio.Consentimento) (string, error) {
	s := c.Snapshot()
	if s.ID == "" {
		const q = `
INSERT INTO clinico.consentimentos (doente_id, finalidade, concedido, documento_url, concedido_em, revogado_em)
VALUES ($1,$2,$3,NULLIF($4,''),$5,$6) RETURNING id::text`
		var id string
		err := r.pool.QueryRow(ctx, q, s.DoenteID, string(s.Finalidade), s.Concedido, s.DocumentoURL, s.ConcedidoEm, s.RevogadoEm).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("inserir consentimento: %w", err)
		}
		return id, nil
	}
	const q = `UPDATE clinico.consentimentos SET revogado_em=$2 WHERE id=$1`
	ct, err := r.pool.Exec(ctx, q, s.ID, s.RevogadoEm)
	if err != nil {
		return "", fmt.Errorf("actualizar consentimento: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", erros.Novo(erros.CategoriaNaoEncontrado, "consentimento não encontrado")
	}
	return s.ID, nil
}

// ObterPorID reconstrói o consentimento. NaoEncontrado se não existir.
func (r *RepositorioConsentimentos) ObterPorID(ctx context.Context, id string) (*dominio.Consentimento, error) {
	const q = `
SELECT id::text, doente_id::text, finalidade, concedido, COALESCE(documento_url,''),
       concedido_em, revogado_em, criado_em
FROM clinico.consentimentos WHERE id=$1`
	var s dominio.SnapshotConsentimento
	var fin string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.DoenteID, &fin, &s.Concedido, &s.DocumentoURL,
		&s.ConcedidoEm, &s.RevogadoEm, &s.CriadoEm)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "consentimento não encontrado")
		}
		return nil, fmt.Errorf("obter consentimento: %w", err)
	}
	s.Finalidade = dominio.Finalidade(fin)
	return dominio.ReconstruirConsentimento(s), nil
}

// ListarPorDoente devolve os consentimentos do doente segundo o filtro.
func (r *RepositorioConsentimentos) ListarPorDoente(ctx context.Context, doenteID string, filtro dominio.FiltroConsentimentos) ([]dominio.ResumoConsentimento, error) {
	q := `
SELECT id::text, doente_id::text, finalidade, concedido, COALESCE(documento_url,''),
       concedido_em, revogado_em, (concedido AND revogado_em IS NULL) AS vigente
FROM clinico.consentimentos WHERE doente_id=$1`
	args := []any{doenteID}
	if f := strings.TrimSpace(filtro.Finalidade); f != "" {
		args = append(args, strings.ToUpper(f))
		q += fmt.Sprintf(" AND finalidade=$%d", len(args))
	}
	if filtro.ApenasVigentes {
		q += " AND concedido AND revogado_em IS NULL"
	}
	q += " ORDER BY concedido_em DESC"
	linhas, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listar consentimentos: %w", err)
	}
	defer linhas.Close()
	var out []dominio.ResumoConsentimento
	for linhas.Next() {
		var rc dominio.ResumoConsentimento
		if err := linhas.Scan(&rc.ID, &rc.DoenteID, &rc.Finalidade, &rc.Concedido, &rc.DocumentoURL,
			&rc.ConcedidoEm, &rc.RevogadoEm, &rc.Vigente); err != nil {
			return nil, fmt.Errorf("ler consentimento: %w", err)
		}
		out = append(out, rc)
	}
	return out, linhas.Err()
}
```

- [ ] **Step 2: Implementar `procedimentos_repo.go`**

```go
package pgrepo

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioProcedimentos implementa dominio.RepositorioProcedimentos com pgx.
type RepositorioProcedimentos struct {
	pool *pgxpool.Pool
}

// NovoRepositorioProcedimentos constrói o repositório sobre o pool pgx.
func NovoRepositorioProcedimentos(pool *pgxpool.Pool) *RepositorioProcedimentos {
	return &RepositorioProcedimentos{pool: pool}
}

// Guardar insere (id vazio) ou actualiza a transição de estado (id presente).
func (r *RepositorioProcedimentos) Guardar(ctx context.Context, p *dominio.ProcedimentoCirurgico) (string, error) {
	s := p.Snapshot()
	if s.ID == "" {
		const q = `
INSERT INTO clinico.procedimentos_cirurgicos (
    episodio_id, codigo_procedimento, descricao, sala, cirurgiao_id, auxiliar_id,
    anestesia, anestesista_id, consentimento_id, observacoes, estado
) VALUES (
    $1,$2,$3,NULLIF($4,''),$5,NULLIF($6,'')::uuid,
    $7,NULLIF($8,'')::uuid,$9,NULLIF($10,''),$11
) RETURNING id::text`
		var id string
		err := r.pool.QueryRow(ctx, q,
			s.EpisodioID, s.Codigo, s.Descricao, s.Sala, s.CirurgiaoID, s.AuxiliarID,
			string(s.Anestesia), s.AnestesistaID, s.ConsentimentoID, s.Observacoes, string(s.Estado),
		).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("inserir procedimento: %w", err)
		}
		return id, nil
	}
	const q = `
UPDATE clinico.procedimentos_cirurgicos SET
    estado=$2, inicio=$3, fim=$4, complicacoes=NULLIF($5,''), observacoes=NULLIF($6,'')
WHERE id=$1`
	ct, err := r.pool.Exec(ctx, q, s.ID, string(s.Estado), s.Inicio, s.Fim, s.Complicacoes, s.Observacoes)
	if err != nil {
		return "", fmt.Errorf("actualizar procedimento: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", erros.Novo(erros.CategoriaNaoEncontrado, "procedimento não encontrado")
	}
	return s.ID, nil
}

// ObterPorID reconstrói o procedimento. NaoEncontrado se não existir.
func (r *RepositorioProcedimentos) ObterPorID(ctx context.Context, id string) (*dominio.ProcedimentoCirurgico, error) {
	const q = `
SELECT id::text, episodio_id::text, codigo_procedimento, descricao, COALESCE(sala,''),
       cirurgiao_id::text, COALESCE(auxiliar_id::text,''), anestesia, COALESCE(anestesista_id::text,''),
       inicio, fim, consentimento_id::text, COALESCE(complicacoes,''), COALESCE(observacoes,''),
       estado, criado_em
FROM clinico.procedimentos_cirurgicos WHERE id=$1`
	var s dominio.SnapshotProcedimento
	var anestesia, estado string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.EpisodioID, &s.Codigo, &s.Descricao, &s.Sala,
		&s.CirurgiaoID, &s.AuxiliarID, &anestesia, &s.AnestesistaID, &s.Inicio, &s.Fim,
		&s.ConsentimentoID, &s.Complicacoes, &s.Observacoes, &estado, &s.CriadoEm)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "procedimento não encontrado")
		}
		return nil, fmt.Errorf("obter procedimento: %w", err)
	}
	s.Anestesia = dominio.Anestesia(anestesia)
	s.Estado = dominio.EstadoProcedimento(estado)
	return dominio.ReconstruirProcedimento(s), nil
}

// ListarPorEpisodio devolve os procedimentos do episódio (mais recentes primeiro).
func (r *RepositorioProcedimentos) ListarPorEpisodio(ctx context.Context, episodioID string) ([]dominio.ResumoProcedimento, error) {
	const q = `
SELECT id::text, episodio_id::text, codigo_procedimento, descricao, estado, anestesia,
       inicio, fim, criado_em
FROM clinico.procedimentos_cirurgicos WHERE episodio_id=$1 ORDER BY criado_em DESC`
	linhas, err := r.pool.Query(ctx, q, episodioID)
	if err != nil {
		return nil, fmt.Errorf("listar procedimentos: %w", err)
	}
	defer linhas.Close()
	var out []dominio.ResumoProcedimento
	for linhas.Next() {
		var rp dominio.ResumoProcedimento
		if err := linhas.Scan(&rp.ID, &rp.EpisodioID, &rp.Codigo, &rp.Descricao, &rp.Estado, &rp.Anestesia,
			&rp.Inicio, &rp.Fim, &rp.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler procedimento: %w", err)
		}
		out = append(out, rp)
	}
	return out, linhas.Err()
}
```

- [ ] **Step 3: Implementar `catalogo_procedimentos_repo.go`**

```go
package pgrepo

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioCatalogoProcedimentos implementa a leitura do catálogo com pgx.
type RepositorioCatalogoProcedimentos struct {
	pool *pgxpool.Pool
}

// NovoRepositorioCatalogoProcedimentos constrói o repositório sobre o pool pgx.
func NovoRepositorioCatalogoProcedimentos(pool *pgxpool.Pool) *RepositorioCatalogoProcedimentos {
	return &RepositorioCatalogoProcedimentos{pool: pool}
}

// ObterPorCodigo devolve a entrada do catálogo. NaoEncontrado se não existir.
func (r *RepositorioCatalogoProcedimentos) ObterPorCodigo(ctx context.Context, codigo string) (*dominio.CatalogoProcedimento, error) {
	const q = `
SELECT codigo, descricao, COALESCE(especialidade,''), COALESCE(duracao_estimada_min,0),
       requer_anestesista, activo
FROM clinico.catalogo_procedimentos WHERE codigo=$1`
	var c dominio.CatalogoProcedimento
	err := r.pool.QueryRow(ctx, q, strings.ToUpper(strings.TrimSpace(codigo))).Scan(
		&c.Codigo, &c.Descricao, &c.Especialidade, &c.DuracaoEstimadaMin, &c.RequerAnestesista, &c.Activo)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "procedimento do catálogo não encontrado")
		}
		return nil, fmt.Errorf("obter procedimento do catálogo: %w", err)
	}
	return &c, nil
}
```

- [ ] **Step 4: Confirmar que compila**

Run: `go build ./... && go vet ./internal/adapters/pgrepo/`
Expected: sem erros. (Verificação funcional contra Postgres na Task 10.)

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/pgrepo/consentimentos_repo.go internal/adapters/pgrepo/procedimentos_repo.go internal/adapters/pgrepo/catalogo_procedimentos_repo.go
git commit -m "$(printf 'feat(clinico): repositorios pgx de consentimentos, procedimentos e catalogo\n\nConsentimentos (insert/update revogacao/listar), procedimentos (insert +\nupdate de transicao de estado/timestamps) e leitura do catalogo.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 8: Adaptadores — handlers HTTP (consentimento + cirurgia)

**Files:**
- Create: `internal/adapters/http/consentimento_handler.go`
- Create: `internal/adapters/http/cirurgia_handler.go`
- Test: `internal/adapters/http/consentimento_test.go`
- Test: `internal/adapters/http/cirurgia_test.go`

**Interfaces:**
- Consumes: DTOs de `appclinico` (Task 5/6); helpers do pacote http (`responderErro`, `SessaoDe`, `RBAC`, `Auth`, `inteiroQuery`, e nos testes `novoRouter`, `fakeAuth`, `pedidoCorpo`).
- Produces: `NovoConsentimentosHandler(...)`/`RegistarConsentimentos(r, h, protecao...)`; `NovoCirurgiaHandler(...)`/`RegistarCirurgia(r, h, protecao...)`.

- [ ] **Step 1: Implementar `consentimento_handler.go`**

```go
package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso de consentimento.
type (
	// ServicoRegistarConsentimento regista um consentimento.
	ServicoRegistarConsentimento interface {
		Executar(ctx context.Context, actor string, dados appclinico.DadosNovoConsentimento) (appclinico.DetalheConsentimento, error)
	}
	// ServicoRevogarConsentimento revoga um consentimento.
	ServicoRevogarConsentimento interface {
		Executar(ctx context.Context, actor, id string) (appclinico.DetalheConsentimento, error)
	}
	// ServicoListarConsentimentos lista os consentimentos de um doente.
	ServicoListarConsentimentos interface {
		Executar(ctx context.Context, doenteID string, filtro appclinico.FiltroConsentimentos) ([]appclinico.ResumoConsentimento, error)
	}
	// ServicoObterConsentimento devolve o detalhe de um consentimento.
	ServicoObterConsentimento interface {
		Executar(ctx context.Context, id string) (appclinico.DetalheConsentimento, error)
	}
)

// ConsentimentosHandler expõe os endpoints HTTP de consentimentos LPDP.
type ConsentimentosHandler struct {
	registar ServicoRegistarConsentimento
	revogar  ServicoRevogarConsentimento
	listar   ServicoListarConsentimentos
	obter    ServicoObterConsentimento
}

// NovoConsentimentosHandler constrói o handler.
func NovoConsentimentosHandler(
	registar ServicoRegistarConsentimento, revogar ServicoRevogarConsentimento,
	listar ServicoListarConsentimentos, obter ServicoObterConsentimento,
) *ConsentimentosHandler {
	return &ConsentimentosHandler{registar: registar, revogar: revogar, listar: listar, obter: obter}
}

// RegistarConsentimentos regista as rotas, aplicando `protecao` e o RBAC por rota.
func RegistarConsentimentos(r gin.IRouter, h *ConsentimentosHandler, protecao ...gin.HandlerFunc) {
	leituraClinica := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelAdministrativo,
		dominio.PapelDirector, dominio.PapelDPO, dominio.PapelAuditor)
	escritaConsent := RBAC(dominio.PapelMedico, dominio.PapelAdministrativo)

	gd := r.Group("/api/v1/doentes")
	gd.Use(protecao...)
	gd.POST("/:id/consentimentos", escritaConsent, h.registarConsentimento)
	gd.GET("/:id/consentimentos", leituraClinica, h.listarConsentimentos)

	gc := r.Group("/api/v1/consentimentos")
	gc.Use(protecao...)
	gc.GET("/:cid", leituraClinica, h.obterConsentimento)
	gc.POST("/:cid/revogar", escritaConsent, h.revogarConsentimento)
}

type corpoConsentimento struct {
	Finalidade   string `json:"finalidade"`
	Concedido    bool   `json:"concedido"`
	DocumentoURL string `json:"documento_url"`
}

func (h *ConsentimentosHandler) registarConsentimento(c *gin.Context) {
	var corpo corpoConsentimento
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registar.Executar(c.Request.Context(), actor.Sujeito, appclinico.DadosNovoConsentimento{
		DoenteID: c.Param("id"), Finalidade: corpo.Finalidade,
		Concedido: corpo.Concedido, DocumentoURL: corpo.DocumentoURL,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *ConsentimentosHandler) listarConsentimentos(c *gin.Context) {
	out, err := h.listar.Executar(c.Request.Context(), c.Param("id"), appclinico.FiltroConsentimentos{
		Finalidade:     c.Query("finalidade"),
		ApenasVigentes: c.Query("vigentes") == "true",
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

func (h *ConsentimentosHandler) obterConsentimento(c *gin.Context) {
	out, err := h.obter.Executar(c.Request.Context(), c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *ConsentimentosHandler) revogarConsentimento(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.revogar.Executar(c.Request.Context(), actor.Sujeito, c.Param("cid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
```

- [ ] **Step 2: Implementar `cirurgia_handler.go`**

```go
package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso de procedimento cirúrgico.
type (
	// ServicoAgendarProcedimento agenda um procedimento.
	ServicoAgendarProcedimento interface {
		Executar(ctx context.Context, actor string, dados appclinico.DadosAgendarProcedimento) (appclinico.DetalheProcedimento, error)
	}
	// ServicoIniciarProcedimento inicia um procedimento.
	ServicoIniciarProcedimento interface {
		Executar(ctx context.Context, actor, id string) (appclinico.DetalheProcedimento, error)
	}
	// ServicoConcluirProcedimento conclui um procedimento.
	ServicoConcluirProcedimento interface {
		Executar(ctx context.Context, actor, id string, dados appclinico.DadosConcluirProcedimento) (appclinico.DetalheProcedimento, error)
	}
	// ServicoCancelarProcedimento cancela um procedimento.
	ServicoCancelarProcedimento interface {
		Executar(ctx context.Context, actor, id, motivo string) (appclinico.DetalheProcedimento, error)
	}
	// ServicoObterProcedimento devolve o detalhe de um procedimento.
	ServicoObterProcedimento interface {
		Executar(ctx context.Context, id string) (appclinico.DetalheProcedimento, error)
	}
	// ServicoListarProcedimentos lista os procedimentos de um episódio.
	ServicoListarProcedimentos interface {
		Executar(ctx context.Context, episodioID string) ([]appclinico.ResumoProcedimento, error)
	}
)

// CirurgiaHandler expõe os endpoints HTTP de procedimentos cirúrgicos.
type CirurgiaHandler struct {
	agendar  ServicoAgendarProcedimento
	iniciar  ServicoIniciarProcedimento
	concluir ServicoConcluirProcedimento
	cancelar ServicoCancelarProcedimento
	obter    ServicoObterProcedimento
	listar   ServicoListarProcedimentos
}

// NovoCirurgiaHandler constrói o handler.
func NovoCirurgiaHandler(
	agendar ServicoAgendarProcedimento, iniciar ServicoIniciarProcedimento,
	concluir ServicoConcluirProcedimento, cancelar ServicoCancelarProcedimento,
	obter ServicoObterProcedimento, listar ServicoListarProcedimentos,
) *CirurgiaHandler {
	return &CirurgiaHandler{agendar: agendar, iniciar: iniciar, concluir: concluir, cancelar: cancelar, obter: obter, listar: listar}
}

// RegistarCirurgia regista as rotas, aplicando `protecao` e o RBAC por rota.
func RegistarCirurgia(r gin.IRouter, h *CirurgiaHandler, protecao ...gin.HandlerFunc) {
	leituraClinica := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelAdministrativo,
		dominio.PapelDirector, dominio.PapelDPO, dominio.PapelAuditor)
	soMedico := RBAC(dominio.PapelMedico)

	ge := r.Group("/api/v1/episodios")
	ge.Use(protecao...)
	ge.POST("/:eid/procedimentos", soMedico, h.agendarProcedimento)
	ge.GET("/:eid/procedimentos", leituraClinica, h.listarProcedimentos)

	gp := r.Group("/api/v1/procedimentos")
	gp.Use(protecao...)
	gp.GET("/:pid", leituraClinica, h.obterProcedimento)
	gp.POST("/:pid/iniciar", soMedico, h.iniciarProcedimento)
	gp.POST("/:pid/concluir", soMedico, h.concluirProcedimento)
	gp.POST("/:pid/cancelar", soMedico, h.cancelarProcedimento)
}

type corpoAgendar struct {
	Codigo          string `json:"codigo_procedimento"`
	Descricao       string `json:"descricao"`
	Sala            string `json:"sala"`
	CirurgiaoID     string `json:"cirurgiao_id"`
	AuxiliarID      string `json:"auxiliar_id"`
	Anestesia       string `json:"anestesia"`
	AnestesistaID   string `json:"anestesista_id"`
	ConsentimentoID string `json:"consentimento_id"`
	Observacoes     string `json:"observacoes"`
}

type corpoConcluir struct {
	Complicacoes string `json:"complicacoes"`
	Observacoes  string `json:"observacoes"`
}

type corpoCancelar struct {
	Motivo string `json:"motivo"`
}

func (h *CirurgiaHandler) agendarProcedimento(c *gin.Context) {
	var corpo corpoAgendar
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.agendar.Executar(c.Request.Context(), actor.Sujeito, appclinico.DadosAgendarProcedimento{
		EpisodioID: c.Param("eid"), Codigo: corpo.Codigo, Descricao: corpo.Descricao,
		Sala: corpo.Sala, CirurgiaoID: corpo.CirurgiaoID, AuxiliarID: corpo.AuxiliarID,
		Anestesia: corpo.Anestesia, AnestesistaID: corpo.AnestesistaID,
		ConsentimentoID: corpo.ConsentimentoID, Observacoes: corpo.Observacoes,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *CirurgiaHandler) listarProcedimentos(c *gin.Context) {
	out, err := h.listar.Executar(c.Request.Context(), c.Param("eid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

func (h *CirurgiaHandler) obterProcedimento(c *gin.Context) {
	out, err := h.obter.Executar(c.Request.Context(), c.Param("pid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *CirurgiaHandler) iniciarProcedimento(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.iniciar.Executar(c.Request.Context(), actor.Sujeito, c.Param("pid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *CirurgiaHandler) concluirProcedimento(c *gin.Context) {
	var corpo corpoConcluir
	// Corpo opcional: um pedido vazio é aceite.
	_ = c.ShouldBindJSON(&corpo)
	actor, _ := SessaoDe(c)
	out, err := h.concluir.Executar(c.Request.Context(), actor.Sujeito, c.Param("pid"), appclinico.DadosConcluirProcedimento{
		Complicacoes: corpo.Complicacoes, Observacoes: corpo.Observacoes,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *CirurgiaHandler) cancelarProcedimento(c *gin.Context) {
	var corpo corpoCancelar
	_ = c.ShouldBindJSON(&corpo)
	actor, _ := SessaoDe(c)
	out, err := h.cancelar.Executar(c.Request.Context(), actor.Sujeito, c.Param("pid"), corpo.Motivo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
```

- [ ] **Step 3: Escrever os testes (`consentimento_test.go` e `cirurgia_test.go`)**

`consentimento_test.go`:

```go
package http_test

import (
	"context"
	nethttp "net/http"
	"testing"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

type fakeRegistarConsent struct {
	out appclinico.DetalheConsentimento
	err error
}

func (f fakeRegistarConsent) Executar(context.Context, string, appclinico.DadosNovoConsentimento) (appclinico.DetalheConsentimento, error) {
	return f.out, f.err
}

type fakeRevogarConsent struct{ out appclinico.DetalheConsentimento }

func (f fakeRevogarConsent) Executar(context.Context, string, string) (appclinico.DetalheConsentimento, error) {
	return f.out, nil
}

type fakeListarConsent struct{ out []appclinico.ResumoConsentimento }

func (f fakeListarConsent) Executar(context.Context, string, appclinico.FiltroConsentimentos) ([]appclinico.ResumoConsentimento, error) {
	return f.out, nil
}

type fakeObterConsent struct{ out appclinico.DetalheConsentimento }

func (f fakeObterConsent) Executar(context.Context, string) (appclinico.DetalheConsentimento, error) {
	return f.out, nil
}

func routerConsent(sessao dominio.Sessao) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoConsentimentosHandler(
		fakeRegistarConsent{out: appclinico.DetalheConsentimento{ID: "cons-1", Vigente: true}},
		fakeRevogarConsent{out: appclinico.DetalheConsentimento{ID: "cons-1"}},
		fakeListarConsent{},
		fakeObterConsent{out: appclinico.DetalheConsentimento{ID: "cons-1"}},
	)
	adhttp.RegistarConsentimentos(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestConsentimentos_Registar_Administrativo_201(t *testing.T) {
	r := routerConsent(dominio.Sessao{Sujeito: "a1", Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/consentimentos", `{"finalidade":"TRATAMENTO","concedido":true}`)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestConsentimentos_Registar_Enfermeiro_Proibido(t *testing.T) {
	r := routerConsent(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/d1/consentimentos", `{"finalidade":"TRATAMENTO","concedido":true}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}
```

> Nota: `gin.Engine`/`gin` — importar `"github.com/gin-gonic/gin"` no teque de teste
> (o `routerEpisodios` existente já usa `*gin.Engine`; segue o mesmo import).

`cirurgia_test.go`:

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
)

type fakeAgendar struct {
	out appclinico.DetalheProcedimento
	err error
}

func (f fakeAgendar) Executar(context.Context, string, appclinico.DadosAgendarProcedimento) (appclinico.DetalheProcedimento, error) {
	return f.out, f.err
}

type fakeIniciarProc struct{ out appclinico.DetalheProcedimento }

func (f fakeIniciarProc) Executar(context.Context, string, string) (appclinico.DetalheProcedimento, error) {
	return f.out, nil
}

type fakeConcluirProc struct{ out appclinico.DetalheProcedimento }

func (f fakeConcluirProc) Executar(context.Context, string, string, appclinico.DadosConcluirProcedimento) (appclinico.DetalheProcedimento, error) {
	return f.out, nil
}

type fakeCancelarProc struct{ out appclinico.DetalheProcedimento }

func (f fakeCancelarProc) Executar(context.Context, string, string, string) (appclinico.DetalheProcedimento, error) {
	return f.out, nil
}

type fakeObterProc struct{ out appclinico.DetalheProcedimento }

func (f fakeObterProc) Executar(context.Context, string) (appclinico.DetalheProcedimento, error) {
	return f.out, nil
}

type fakeListarProc struct{ out []appclinico.ResumoProcedimento }

func (f fakeListarProc) Executar(context.Context, string) ([]appclinico.ResumoProcedimento, error) {
	return f.out, nil
}

func routerCirurgia(sessao dominio.Sessao) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoCirurgiaHandler(
		fakeAgendar{out: appclinico.DetalheProcedimento{ID: "proc-1", Estado: "AGENDADO"}},
		fakeIniciarProc{out: appclinico.DetalheProcedimento{ID: "proc-1", Estado: "EM_CURSO"}},
		fakeConcluirProc{out: appclinico.DetalheProcedimento{ID: "proc-1", Estado: "CONCLUIDO"}},
		fakeCancelarProc{out: appclinico.DetalheProcedimento{ID: "proc-1", Estado: "CANCELADO"}},
		fakeObterProc{out: appclinico.DetalheProcedimento{ID: "proc-1"}},
		fakeListarProc{},
	)
	adhttp.RegistarCirurgia(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestCirurgia_Agendar_Medico_201(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep1/procedimentos",
		`{"codigo_procedimento":"PRC001","descricao":"Sutura","cirurgiao_id":"c1","anestesia":"NENHUMA","consentimento_id":"cons-1"}`)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestCirurgia_Agendar_Enfermeiro_Proibido(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelEnfermeiro}})
	w := pedidoCorpo(r, "POST", "/api/v1/episodios/ep1/procedimentos", `{"codigo_procedimento":"PRC001"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestCirurgia_Concluir_SemCorpo_200(t *testing.T) {
	r := routerCirurgia(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/procedimentos/proc-1/concluir", ``)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 4: Correr os testes**

Run: `go test ./internal/adapters/http/ -run 'Consentimentos|Cirurgia' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/consentimento_handler.go internal/adapters/http/cirurgia_handler.go internal/adapters/http/consentimento_test.go internal/adapters/http/cirurgia_test.go
git commit -m "$(printf 'feat(clinico): handlers HTTP de consentimento e cirurgia com RBAC\n\nRotas de consentimento (escrita Medico+Administrativo) e de procedimento\n(escrita Medico), leitura pelo leque clinico; RFC 7807 e tags JSON PT-PT.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 9: Plataforma — wiring em `app.go` + ADR-030

**Files:**
- Modify: `internal/platform/app.go`
- Create: `adrs/ADR-030-cirurgia-ambulatoria-consentimento.md`

**Interfaces:**
- Consumes: `pgrepo.NovoRepositorioConsentimentos/Procedimentos/CatalogoProcedimentos`, os `appclinico.NovoCaso...` das Tasks 5/6, os handlers da Task 8, `repoDoentes`/`repoEpisodios`/`repoAuditoria` já existentes, `limiteMW`/`authMW`.

- [ ] **Step 1: Instanciar repos + casos + handlers em `app.go`**

A seguir ao bloco do `handlerEpisodios` (após a linha que constrói `handlerEpisodios := adhttp.NovoEpisodiosHandler(...)`, por volta da linha 114) juntar:

```go
	// BC Clínico: cirurgia ambulatória + consentimentos (LPDP).
	repoConsentimentos := pgrepo.NovoRepositorioConsentimentos(pool)
	repoProcedimentos := pgrepo.NovoRepositorioProcedimentos(pool)
	repoCatalogo := pgrepo.NovoRepositorioCatalogoProcedimentos(pool)
	handlerConsentimentos := adhttp.NovoConsentimentosHandler(
		appclinico.NovoCasoRegistarConsentimento(repoConsentimentos, repoDoentes, repoAuditoria),
		appclinico.NovoCasoRevogarConsentimento(repoConsentimentos, repoAuditoria),
		appclinico.NovoCasoListarConsentimentos(repoConsentimentos),
		appclinico.NovoCasoObterConsentimento(repoConsentimentos),
	)
	handlerCirurgia := adhttp.NovoCirurgiaHandler(
		appclinico.NovoCasoAgendarProcedimento(repoProcedimentos, repoEpisodios, repoConsentimentos, repoCatalogo, repoAuditoria),
		appclinico.NovoCasoIniciarProcedimento(repoProcedimentos, repoAuditoria),
		appclinico.NovoCasoConcluirProcedimento(repoProcedimentos, repoAuditoria),
		appclinico.NovoCasoCancelarProcedimento(repoProcedimentos, repoAuditoria),
		appclinico.NovoCasoObterProcedimento(repoProcedimentos),
		appclinico.NovoCasoListarProcedimentos(repoProcedimentos),
	)
```

> Nota: confirmar os nomes exactos das variáveis `repoDoentes`/`repoEpisodios`/`repoAuditoria` no ficheiro (linhas ~93-114) e reutilizá-las; não instanciar repos duplicados.

- [ ] **Step 2: Registar as rotas**

No bloco onde estão as chamadas `adhttp.Registar...` (por volta da linha 162-165), a seguir a `adhttp.RegistarEpisodios(r, handlerEpisodios, limiteMW, authMW)` juntar:

```go
		adhttp.RegistarConsentimentos(r, handlerConsentimentos, limiteMW, authMW)
		adhttp.RegistarCirurgia(r, handlerCirurgia, limiteMW, authMW)
```

- [ ] **Step 3: Confirmar build**

Run: `go build ./... && go vet ./internal/platform/`
Expected: sem erros.

- [ ] **Step 4: Criar `adrs/ADR-030-cirurgia-ambulatoria-consentimento.md`**

```markdown
# ADR-030 — Cirurgia Ambulatória e Consentimento (LPDP)

- **Estado:** Aceite
- **Data:** 2026-07-13
- **Marco/Sprint:** M2 / Sprint 11
- **Fontes:** ADR-018 pt2, DDM-001 v2.0 (consentimentos), DDM-001 v2.1 adenda §4.

## Contexto

O critério de saída M2 exige cirurgia ambulatória: tipo de episódio dedicado,
agregado `ProcedimentoCirurgico` com state machine e consentimento cirúrgico com
anexo obrigatório. A tabela `clinico.consentimentos` (DDM v2.0) nunca fora criada
(a migração dos doentes adiou-a explicitamente) — é dependência crítica da FK
`procedimentos_cirurgicos.consentimento_id` e da invariante-estrela.

## Decisão

1. **Consentimento (LPDP) com ciclo completo:** tabela + agregado `Consentimento`
   (finalidades TRATAMENTO/COMUNICACAO/PARTILHA_SEGURADORA/INVESTIGACAO/CIRURGIA),
   registar/revogar/listar/obter, todos auditados nas escritas.
2. **Invariante-estrela:** um consentimento de finalidade CIRURGIA exige estar
   concedido e ter `documento_url` (anexo). `NovoProcedimento` só aceita um
   consentimento CIRURGIA, com anexo e vigente (senão RegraNegocio/422).
3. **State machine DDM-estrita:** AGENDADO → EM_CURSO (Iniciar) → CONCLUIDO
   (Concluir) ou → CANCELADO (Cancelar). `Cancelar` só de EM_CURSO — a CHECK do
   DDM obriga `inicio` não-nulo em CANCELADO. Um AGENDADO que não se realiza
   resolve-se cancelando o episódio.
4. **Anestesia:** VO com NENHUMA/LOCAL/SEDACAO_LIGEIRA/LOCO_REGIONAL; anestesista
   obrigatório se anestesia ≠ NENHUMA; a flag `requer_anestesista` do catálogo
   reforça a exigência na aplicação.
5. **RBAC:** escrita de cirurgia = Médico; escrita de consentimento = Médico +
   Administrativo; leitura = leque clínico.

## Desvios ao blueprint (conscientes)

- `ProcedimentoCirurgico`/`Consentimento` no pacote `clinico` (plano), não em
  subpacote `clinico/cirurgia` — consistência com `episodio`/`doente`.
- Erros por `erros.Novo(categoria, msg)` PT-PT em vez de sentinelas `ErrXxx`.
- FK de consentimento sem `ON DELETE CASCADE` (doente é soft-delete).
- Sem FK `codigo_procedimento` → catálogo; validação na aplicação (dá também o
  `requer_anestesista`).

## Consequências

- Fecha o critério de saída M2 de cirurgia ambulatória.
- `documento_url` é referência textual ao anexo; o upload binário (MinIO) fica
  para fatia futura.
- Evento `ProcedimentoCirurgicoConcluido` definido mas não emitido (scaffolding),
  para consumo por Financeiro/reporting em marcos posteriores.

## Dívida registada (não-bloqueante)

- Auditoria fora da transacção das escritas (janela sem trilho se a auditoria
  falhar), como nos sprints anteriores.
- Upload binário do anexo de consentimento.
- Integração com Facturação (linha por procedimento) e relatório MINSA.
```

- [ ] **Step 5: Commit**

```bash
git add internal/platform/app.go adrs/ADR-030-cirurgia-ambulatoria-consentimento.md
git commit -m "$(printf 'feat(clinico): liga cirurgia e consentimentos ao composition root + ADR-030\n\nInstancia repos, casos de uso e handlers; regista as rotas com limite+auth.\nADR-030 documenta decisoes, invariante-estrela, state machine e desvios.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 10: Testes de integração (contra Postgres real)

**Files:**
- Create: `tests/integration/consentimento_test.go`
- Create: `tests/integration/cirurgia_test.go`

**Interfaces:**
- Consumes: helper `ligar(t)`, `db.AplicarMigracoes`, `migrations.FS`, os repos da Task 7.

Nota: tag `integration`; SKIP (nunca FAIL) sem `DATABASE_URL`.

- [ ] **Step 1: Escrever `consentimento_test.go`**

```go
//go:build integration

// Teste de integração dos consentimentos LPDP contra a BD real. SKIP (nunca
// FAIL) quando DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// criarDoenteSQL insere um doente mínimo directamente e devolve o id.
func criarDoenteSQL(t *testing.T, pool interface {
	QueryRow(ctx any, sql string, args ...any) any
}) string {
	t.Helper()
	return "" // substituído abaixo pelo helper real
}

func TestRepositorioConsentimentos_CicloCompleto(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	// Doente mínimo via SQL (evita a factory completa).
	var doenteID string
	err := pool.QueryRow(ctx, `
INSERT INTO clinico.doentes (num_processo, nome_completo, data_nascimento, sexo, nacionalidade, telefone, estado)
VALUES ($1,'Teste Consentimento','1990-01-01','M','Angolana','923000000','ACTIVO') RETURNING id::text`,
		"PROC-CONS-"+time.Now().Format("150405")).Scan(&doenteID)
	if err != nil {
		t.Fatalf("inserir doente: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.consentimentos WHERE doente_id=$1`, doenteID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})

	repo := pgrepo.NovoRepositorioConsentimentos(pool)
	c, err := dominio.NovoConsentimento(doenteID, dominio.FinalidadeCirurgia, true, "s3://consent.pdf",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("consentimento inválido: %v", err)
	}
	id, err := repo.Guardar(ctx, c)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}

	obtido, err := repo.ObterPorID(ctx, id)
	if err != nil || !obtido.EstaVigente() || !obtido.TemAnexo() {
		t.Fatalf("esperado vigente com anexo, veio %+v err=%v", obtido, err)
	}

	if err := obtido.Revogar(time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("revogar domínio: %v", err)
	}
	if _, err := repo.Guardar(ctx, obtido); err != nil {
		t.Fatalf("guardar revogação: %v", err)
	}
	depois, _ := repo.ObterPorID(ctx, id)
	if depois.EstaVigente() {
		t.Fatalf("consentimento revogado não devia estar vigente")
	}

	lista, err := repo.ListarPorDoente(ctx, doenteID, dominio.FiltroConsentimentos{})
	if err != nil || len(lista) != 1 {
		t.Fatalf("esperava 1 consentimento listado, veio %d err=%v", len(lista), err)
	}
}
```

> Nota ao implementador: a assinatura real de `ligar(t)` devolve
> `(*pgxpool.Pool, context.Context)` (ver `ligar` no pacote). Usa esse tipo — o
> stub `criarDoenteSQL`/`interface{...}` acima é ilustrativo; **remove-o** e usa
> `pool.QueryRow(ctx, ...)` directamente como no corpo do teste. Ajusta as colunas
> do INSERT de `clinico.doentes` às reais (ver `migrations/clinico/0001_doentes.sql`
> — usar as colunas NOT NULL existentes; as opcionais podem ficar de fora).

- [ ] **Step 2: Escrever `cirurgia_test.go`**

```go
//go:build integration

// Teste de integração da cirurgia ambulatória contra a BD real. SKIP (nunca
// FAIL) quando DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestCirurgia_CicloEProibicoes(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	// Doente + episódio de cirurgia ambulatória via SQL.
	var doenteID, episodioID string
	if err := pool.QueryRow(ctx, `
INSERT INTO clinico.doentes (num_processo, nome_completo, data_nascimento, sexo, nacionalidade, telefone, estado)
VALUES ($1,'Teste Cirurgia','1985-05-05','F','Angolana','923111111','ACTIVO') RETURNING id::text`,
		"PROC-CIR-"+time.Now().Format("150405")).Scan(&doenteID); err != nil {
		t.Fatalf("inserir doente: %v", err)
	}
	if err := pool.QueryRow(ctx, `
INSERT INTO clinico.episodios_clinicos (doente_id, tipo, especialidade_id, medico_id, inicio, estado)
VALUES ($1,'CIRURGIA_AMBULATORIA',gen_random_uuid(),gen_random_uuid(),now(),'ABERTO') RETURNING id::text`,
		doenteID).Scan(&episodioID); err != nil {
		t.Fatalf("inserir episódio: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.procedimentos_cirurgicos WHERE episodio_id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.consentimentos WHERE doente_id=$1`, doenteID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.episodios_clinicos WHERE id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})

	repoCons := pgrepo.NovoRepositorioConsentimentos(pool)
	repoProc := pgrepo.NovoRepositorioProcedimentos(pool)
	repoCat := pgrepo.NovoRepositorioCatalogoProcedimentos(pool)

	// O catálogo tem PRC001 do seed.
	cat, err := repoCat.ObterPorCodigo(ctx, "PRC001")
	if err != nil || cat.Codigo != "PRC001" {
		t.Fatalf("catálogo PRC001 devia existir do seed: %v", err)
	}

	cons, _ := dominio.NovoConsentimento(doenteID, dominio.FinalidadeCirurgia, true, "s3://c.pdf",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	consID, err := repoCons.Guardar(ctx, cons)
	if err != nil {
		t.Fatalf("guardar consentimento: %v", err)
	}
	consPersistido, _ := repoCons.ObterPorID(ctx, consID)

	proc, err := dominio.NovoProcedimento(dominio.DadosNovoProcedimento{
		EpisodioID: episodioID, Codigo: "PRC001", Descricao: "Sutura",
		CirurgiaoID: "00000000-0000-4000-8000-0000000000c1", Anestesia: dominio.AnestesiaLocal,
		AnestesistaID: "00000000-0000-4000-8000-0000000000a1",
	}, consPersistido)
	if err != nil {
		t.Fatalf("novo procedimento: %v", err)
	}
	procID, err := repoProc.Guardar(ctx, proc)
	if err != nil {
		t.Fatalf("guardar procedimento: %v", err)
	}

	// Ciclo: iniciar → concluir, persistido e coerente.
	p, _ := repoProc.ObterPorID(ctx, procID)
	if err := p.Iniciar(time.Now()); err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if _, err := repoProc.Guardar(ctx, p); err != nil {
		t.Fatalf("persistir início: %v", err)
	}
	p2, _ := repoProc.ObterPorID(ctx, procID)
	if err := p2.Concluir(time.Now().Add(time.Hour), "sem complicações", ""); err != nil {
		t.Fatalf("concluir: %v", err)
	}
	if _, err := repoProc.Guardar(ctx, p2); err != nil {
		t.Fatalf("persistir conclusão: %v", err)
	}
	final, _ := repoProc.ObterPorID(ctx, procID)
	if final.Estado() != dominio.ProcConcluido {
		t.Fatalf("esperado CONCLUIDO, veio %s", final.Estado())
	}

	// Proibição: consentimento não-cirúrgico bloqueia a construção do procedimento.
	consTrat, _ := dominio.NovoConsentimento(doenteID, dominio.FinalidadeTratamento, true, "",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	if _, err := dominio.NovoProcedimento(dominio.DadosNovoProcedimento{
		EpisodioID: episodioID, Codigo: "PRC001", Descricao: "X",
		CirurgiaoID: "c1", Anestesia: dominio.AnestesiaNenhuma,
	}, consTrat); err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("consentimento não-cirúrgico devia bloquear com RegraNegocio, veio %v", err)
	}
}
```

- [ ] **Step 3: Correr a integração (se houver Postgres)**

Run (com Postgres a correr):
```
DATABASE_URL="postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable" go test -tags integration ./tests/integration/ -run 'Consentimentos|Cirurgia' -v
```
Expected: PASS (ou SKIP se `DATABASE_URL` não estiver definido). Sem Postgres, confirmar que compila: `go test -tags integration ./tests/integration/ -run NADA`.

- [ ] **Step 4: Verificação final de qualidade**

Run:
```
go build ./... && go test ./... && gofmt -l internal/ && go vet ./...
bash scripts/cobertura.sh
```
Expected: build+testes verdes; `gofmt -l` sem saída; vet limpo; cobertura domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.

- [ ] **Step 5: Commit**

```bash
git add tests/integration/consentimento_test.go tests/integration/cirurgia_test.go
git commit -m "$(printf 'test(clinico): integracao de consentimentos e cirurgia contra Postgres\n\nCiclo completo de consentimento (guardar/obter/revogar/listar) e de\nprocedimento (agendar/iniciar/concluir), e o bloqueio por consentimento\nnao-cirurgico. SKIP sem DATABASE_URL.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Revisão final (whole-branch)

Após a Task 10, dispatch da revisão de branch completa no modelo mais capaz
(padrão dos sprints anteriores): `scripts/review-package <merge-base> HEAD` e
revisão focada em (1) a invariante-estrela do consentimento cirúrgico em todas as
entradas, (2) coerência estado↔timestamps do procedimento contra as CHECK do DDM,
(3) o `UPDATE` de transição no `procedimentos_repo` não corromper campos, (4)
gates de cobertura, (5) trailers de commit correctos. Triar os Minor registados
antes do merge.

