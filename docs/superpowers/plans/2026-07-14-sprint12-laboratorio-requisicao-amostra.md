# Sprint 12 — BC Laboratório: Catálogo + Requisição + Amostra — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** abrir o marco M3 construindo, no BC Laboratório e só backend/API, o catálogo de Análises, o agregado RequisicaoLab (emitido via ACL sobre o Clínico) e o agregado Resultado com state machine até ao resultado preliminar — que não é visível ao médico.

**Architecture:** DDD + Clean Architecture (Domínio → Aplicação → Adaptadores → Plataforma). Três agregados novos no pacote `internal/domain/laboratorio` (sem subpacotes). Persistência pgx SQL puro (schema `laboratorio`). HTTP Gin com RBAC + RFC 7807. A requisição vive no BC Laboratório e refere o episódio por id, validado por uma ACL (precedente da Receita da Farmácia). Sem FK cross-context.

**Tech Stack:** Go 1.22+, Gin, pgx v5, PostgreSQL 16.

**Spec:** `docs/superpowers/specs/2026-07-14-sprint12-laboratorio-requisicao-amostra-design.md`

## Global Constraints

- Todo o output em **PT-PT angolano** (código, comentários, tags JSON, mensagens de erro, commits). Nunca inglês/PT-BR.
- Domínio puro: só stdlib + Shared Kernel (`erros`, `auditoria`, `evento`). Sem pgx/gin/http/uuid no domínio nem na aplicação. O Laboratório **nunca** importa tipos do domínio Clínico.
- IDs são `string` gerados pela BD: `gen_random_uuid()` + `RETURNING id::text`.
- Erros via `erros.Novo(categoria, msg)` com mensagens literais PT-PT. Categorias: `CategoriaValidacao` (400), `CategoriaProibido` (403), `CategoriaNaoEncontrado` (404), `CategoriaConflito` (409), `CategoriaRegraNegocio` (422).
- HTTP: erros por `responderErro(c, err)` (RFC 7807). Sucesso: `c.JSON(...)`. O actor vem de `SessaoDe(c)` — **nunca do corpo do pedido**.
- Migrações forward-only, schema-qualificadas (`laboratorio.`).
- Sem `panic()` fora de init.
- Conventional Commits PT-PT, terminados **exactamente** com:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- Gates de cobertura: domínio ≥85%, aplicação ≥75%, adaptadores ≥60% (pgrepo excluído do gate unitário — testado por integração, tag `integration`, SKIP sem `DATABASE_URL`).
- O papel validador é `PapelPatologista`; o submissor é `PapelTecnicoLab`. Não existe papel `Biologo`.

## File Structure

**Domínio (`internal/domain/laboratorio/`):**
- Create `analise.go` — VOs `IntervaloReferencia`/`ValorCritico`, agregado `Analise`, `RepositorioAnalises`, `ResumoAnalise`.
- Create `requisicao.go` — `Prioridade`, `EstadoRequisicao`, `ItemRequisicao`, agregado `RequisicaoLab`, `RepositorioRequisicoes`, `ResumoRequisicao`.
- Create `resultado.go` — `EstadoResultado`, agregado `Resultado`, `RepositorioResultados`, `ResumoResultado`.
- Create `eventos.go` — `AmostraColhida`, `AmostraRecusada`, `ResultadoPreliminarSubmetido` (scaffolding).
- Modify `doc.go` — o comentário de pacote deixa de dizer "por implementar".

**Aplicação (`internal/application/laboratorio/`):**
- Create `ports.go` — `Auditor`, `LeitorClinico`, DTOs e reexports.
- Create `analises.go`, `emitir_requisicao.go`, `requisicoes.go`, `amostras.go`, `resultados.go`, `mapa.go`.

**Adaptadores:**
- Create `internal/adapters/pgrepo/analises_repo.go`, `requisicoes_repo.go`, `resultados_repo.go`.
- Create `internal/adapters/laboratorio/leitor_clinico.go` (ACL).
- Create `internal/adapters/http/laboratorio_handler.go`.

**Migrações:** `migrations/laboratorio/0001_catalogo_analises.sql`, `0002_requisicoes_resultados.sql`; Modify `migrations/embed.go`.

**Plataforma / docs:** Modify `internal/platform/app.go`; Create `adrs/ADR-031-bc-laboratorio.md`; Modify `SPRINT.md`, `CLAUDE.md`.

**Testes:** `*_test.go` por camada + `tests/integration/laboratorio_test.go`.

---

### Task 1: Migrações (catálogo de análises, requisições, resultados)

**Files:**
- Create: `migrations/laboratorio/0001_catalogo_analises.sql`
- Create: `migrations/laboratorio/0002_requisicoes_resultados.sql`
- Modify: `migrations/embed.go`

**Interfaces:**
- Produces: schema `laboratorio`; tabelas `analises` (com seed HB/HEMOG/GLIC/CREAT/UREIA), `requisicoes`, `itens_requisicao`, `resultados`.

- [ ] **Step 1: Criar `migrations/laboratorio/0001_catalogo_analises.sql`**

```sql
-- Bounded Context: laboratorio
-- Migration forward-only. Catálogo de análises (dados de referência).
--
-- Os intervalos de referência e os valores críticos são jsonb: são listas de VOs
-- lidas em bloco com o agregado e nunca consultadas isoladamente por SQL, pelo que
-- tabelas-filho só acrescentariam junções sem benefício. Os valores críticos são
-- registados nesta fatia e avaliados no Sprint 13.

CREATE SCHEMA IF NOT EXISTS laboratorio;

CREATE TABLE IF NOT EXISTS laboratorio.analises (
    codigo                text        PRIMARY KEY,
    nome                  text        NOT NULL,
    unidade               text        NOT NULL,
    intervalos_referencia jsonb       NOT NULL DEFAULT '[]'::jsonb,
    valores_criticos      jsonb       NOT NULL DEFAULT '[]'::jsonb,
    activo                boolean     NOT NULL DEFAULT true,
    criado_em             timestamptz NOT NULL DEFAULT now()
);

INSERT INTO laboratorio.analises (codigo, nome, unidade, intervalos_referencia, valores_criticos) VALUES
    ('HB', 'Hemoglobina', 'g/dL',
     '[{"perfil":"ADULTO","sexo":"M","minimo":13.0,"maximo":17.0},
       {"perfil":"ADULTO","sexo":"F","minimo":12.0,"maximo":15.0}]'::jsonb,
     '[{"operador":"<","limite":7.0,"descricao":"anemia grave — contactar o médico requisitante"}]'::jsonb),
    ('GLIC', 'Glicemia em jejum', 'mg/dL',
     '[{"perfil":"ADULTO","sexo":"AMBOS","minimo":70.0,"maximo":110.0}]'::jsonb,
     '[{"operador":"<","limite":50.0,"descricao":"hipoglicemia grave"},
       {"operador":">","limite":400.0,"descricao":"hiperglicemia grave"}]'::jsonb),
    ('CREAT', 'Creatinina sérica', 'mg/dL',
     '[{"perfil":"ADULTO","sexo":"M","minimo":0.7,"maximo":1.3},
       {"perfil":"ADULTO","sexo":"F","minimo":0.6,"maximo":1.1}]'::jsonb,
     '[{"operador":">","limite":5.0,"descricao":"insuficiência renal — contactar o médico"}]'::jsonb),
    ('UREIA', 'Ureia', 'mg/dL',
     '[{"perfil":"ADULTO","sexo":"AMBOS","minimo":15.0,"maximo":45.0}]'::jsonb,
     '[]'::jsonb),
    ('HEMOG', 'Hemograma completo', 'texto',
     '[]'::jsonb, '[]'::jsonb)
ON CONFLICT (codigo) DO NOTHING;

COMMENT ON TABLE laboratorio.analises IS
    'Catálogo de análises. valores_criticos é avaliado na validação (Sprint 13).';
```

- [ ] **Step 2: Criar `migrations/laboratorio/0002_requisicoes_resultados.sql`**

```sql
-- Bounded Context: laboratorio
-- Migration forward-only. Requisição de análises e resultados.
--
-- episodio_id/doente_id/medico_requisitante_id são referências textuais a outros
-- bounded contexts: SEM foreign key (regra de arquitectura). A existência e o estado
-- do episódio são validados pela ACL na camada de aplicação.

CREATE TABLE IF NOT EXISTS laboratorio.requisicoes (
    id                     uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    episodio_id            uuid        NOT NULL,
    doente_id              uuid        NOT NULL,
    medico_requisitante_id uuid        NOT NULL,
    prioridade             text        NOT NULL CHECK (prioridade IN ('ROTINA','URGENTE')),
    estado                 text        NOT NULL CHECK (estado IN ('EMITIDA','CANCELADA')),
    criado_em              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_requisicoes_episodio
    ON laboratorio.requisicoes (episodio_id, criado_em DESC);

CREATE TABLE IF NOT EXISTS laboratorio.itens_requisicao (
    requisicao_id  uuid NOT NULL REFERENCES laboratorio.requisicoes(id) ON DELETE CASCADE,
    codigo_analise text NOT NULL,
    observacoes    text,
    PRIMARY KEY (requisicao_id, codigo_analise)
);

-- Resultado: uma linha por item da requisição, criada em PENDENTE na emissão.
-- As CHECK impõem a coerência estado↔timestamps↔autores (lição do Sprint 11): a base
-- de dados não aceita uma PROCESSADA sem submissor nem valor, nem uma RECUSADA sem
-- motivo. Os estados VALIDADA/CONCLUIDA e a CHECK de segregação já existem aqui,
-- embora a transição só seja implementada no Sprint 13.
CREATE TABLE IF NOT EXISTS laboratorio.resultados (
    id                       uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    requisicao_id            uuid        NOT NULL REFERENCES laboratorio.requisicoes(id),
    codigo_analise           text        NOT NULL,
    valor                    text,
    unidade                  text        NOT NULL,
    observacoes              text,
    motivo_recusa            text,
    estado                   text        NOT NULL CHECK (estado IN
                               ('PENDENTE','COLHIDA','PROCESSADA','VALIDADA','CONCLUIDA','RECUSADA')),
    tecnico_colheita_id      uuid,
    tecnico_submissor_id     uuid,
    patologista_validador_id uuid,
    colhida_em               timestamptz,
    submetida_em             timestamptz,
    validada_em              timestamptz,
    valor_critico            boolean     NOT NULL DEFAULT false,
    criado_em                timestamptz NOT NULL DEFAULT now(),
    CHECK (estado <> 'COLHIDA' OR (colhida_em IS NOT NULL AND tecnico_colheita_id IS NOT NULL)),
    CHECK (estado <> 'PROCESSADA' OR (submetida_em IS NOT NULL AND tecnico_submissor_id IS NOT NULL AND valor IS NOT NULL)),
    CHECK (estado <> 'RECUSADA' OR motivo_recusa IS NOT NULL),
    CHECK (estado NOT IN ('VALIDADA','CONCLUIDA') OR (validada_em IS NOT NULL AND patologista_validador_id IS NOT NULL)),
    -- Segregação de funções (DDM): quem valida nunca é quem submeteu. Defesa em
    -- profundidade — a invariante vive no agregado (Sprint 13), mas a BD também a nega.
    CHECK (patologista_validador_id IS NULL OR patologista_validador_id <> tecnico_submissor_id)
);
CREATE INDEX IF NOT EXISTS idx_resultados_fila       ON laboratorio.resultados (estado, criado_em);
CREATE INDEX IF NOT EXISTS idx_resultados_requisicao ON laboratorio.resultados (requisicao_id);
```

- [ ] **Step 3: Acrescentar `laboratorio` ao embed em `migrations/embed.go`**

Os bounded contexts estão listados um a um na directiva. Sem esta alteração as migrações compilam mas **nunca entram no binário**. Substituir a linha do `go:embed` por:

```go
//go:embed auditoria clinico farmacia identidade laboratorio shared
var FS embed.FS
```

- [ ] **Step 4: Verificar que os ficheiros embebem e o build passa**

Run: `go test ./migrations/... && go build ./...`
Expected: PASS. O runner aplica os BCs por ordem alfabética, pelo que `laboratorio` corre depois de `farmacia` e antes de `shared`; não há dependências entre eles.

- [ ] **Step 5: Commit**

```bash
git add migrations/laboratorio/0001_catalogo_analises.sql migrations/laboratorio/0002_requisicoes_resultados.sql migrations/embed.go
git commit -m "$(printf 'feat(laboratorio): migracoes do catalogo de analises, requisicoes e resultados\n\nSchema laboratorio com seed de analises comuns, requisicao sem FK cross-context\ne resultados com CHECK de coerencia estado-timestamps e de segregacao de funcoes.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 2: Domínio — catálogo de Análises

**Files:**
- Create: `internal/domain/laboratorio/analise.go`
- Test: `internal/domain/laboratorio/analise_test.go`
- Modify: `internal/domain/laboratorio/doc.go`

**Interfaces:**
- Produces: `PerfilReferencia` (`PerfilAdulto`/`PerfilPediatrico`/`PerfilGeriatrico`), `SexoReferencia` (`SexoMasculino`/`SexoFeminino`/`SexoAmbos`), `OperadorCritico` (`CriticoMenor`/`CriticoMaior`/`CriticoMenorIgual`/`CriticoMaiorIgual`); VOs `IntervaloReferencia`, `ValorCritico`; agregado `Analise` com `NovaAnalise(codigo, nome, unidade string, intervalos []IntervaloReferencia, criticos []ValorCritico) (*Analise, error)`, getters `Codigo()/Nome()/Unidade()/Activo()`, `Snapshot() SnapshotAnalise`, `ReconstruirAnalise(SnapshotAnalise) *Analise`; `ResumoAnalise`; `RepositorioAnalises`.

- [ ] **Step 1: Escrever o teste que falha**

```go
package laboratorio_test

import (
	"testing"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func intervaloValido() dominio.IntervaloReferencia {
	return dominio.IntervaloReferencia{
		Perfil: dominio.PerfilAdulto, Sexo: dominio.SexoAmbos, Minimo: 70, Maximo: 110,
	}
}

func TestNovaAnalise_CamposObrigatorios(t *testing.T) {
	casos := []struct{ nome, codigo, nomeAnalise, unidade string }{
		{"código em falta", "", "Glicemia", "mg/dL"},
		{"nome em falta", "GLIC", "", "mg/dL"},
		{"unidade em falta", "GLIC", "Glicemia", ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := dominio.NovaAnalise(c.codigo, c.nomeAnalise, c.unidade, nil, nil)
			if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
				t.Fatalf("esperava Validacao, veio %v", err)
			}
		})
	}
}

func TestNovaAnalise_NormalizaCodigo(t *testing.T) {
	a, err := dominio.NovaAnalise(" glic ", "Glicemia", "mg/dL", []dominio.IntervaloReferencia{intervaloValido()}, nil)
	if err != nil {
		t.Fatalf("análise válida falhou: %v", err)
	}
	if a.Codigo() != "GLIC" {
		t.Fatalf("esperava código normalizado GLIC, veio %q", a.Codigo())
	}
	if !a.Activo() {
		t.Fatalf("uma análise nova devia nascer activa")
	}
}

func TestNovaAnalise_IntervaloInvertido(t *testing.T) {
	mau := dominio.IntervaloReferencia{Perfil: dominio.PerfilAdulto, Sexo: dominio.SexoAmbos, Minimo: 110, Maximo: 70}
	_, err := dominio.NovaAnalise("GLIC", "Glicemia", "mg/dL", []dominio.IntervaloReferencia{mau}, nil)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("mínimo > máximo devia falhar com Validacao, veio %v", err)
	}
}

func TestNovaAnalise_ValorCriticoOperadorInvalido(t *testing.T) {
	mau := dominio.ValorCritico{Operador: "==", Limite: 7, Descricao: "x"}
	_, err := dominio.NovaAnalise("HB", "Hemoglobina", "g/dL", nil, []dominio.ValorCritico{mau})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("operador inválido devia falhar com Validacao, veio %v", err)
	}
}

func TestAnalise_SnapshotEReconstruir(t *testing.T) {
	a, _ := dominio.NovaAnalise("HB", "Hemoglobina", "g/dL",
		[]dominio.IntervaloReferencia{intervaloValido()},
		[]dominio.ValorCritico{{Operador: dominio.CriticoMenor, Limite: 7, Descricao: "anemia grave"}})
	s := a.Snapshot()
	s.Activo = false
	b := dominio.ReconstruirAnalise(s)
	if b.Codigo() != "HB" || b.Activo() {
		t.Fatalf("reconstrução não preservou o snapshot: %+v", b.Snapshot())
	}
	if len(b.Snapshot().ValoresCriticos) != 1 {
		t.Fatalf("esperava 1 valor crítico preservado")
	}
}
```

- [ ] **Step 2: Correr para confirmar que falha**

Run: `go test ./internal/domain/laboratorio/ -run TestNovaAnalise -v`
Expected: FAIL (compilação: `NovaAnalise` indefinido).

- [ ] **Step 3: Implementar `analise.go`**

```go
package laboratorio

import (
	"context"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// PerfilReferencia é o perfil etário a que um intervalo de referência se aplica.
type PerfilReferencia string

const (
	PerfilAdulto     PerfilReferencia = "ADULTO"
	PerfilPediatrico PerfilReferencia = "PEDIATRICO"
	PerfilGeriatrico PerfilReferencia = "GERIATRICO"
)

var perfisValidos = map[PerfilReferencia]bool{
	PerfilAdulto: true, PerfilPediatrico: true, PerfilGeriatrico: true,
}

// SexoReferencia é o sexo a que um intervalo de referência se aplica.
type SexoReferencia string

const (
	SexoMasculino SexoReferencia = "M"
	SexoFeminino  SexoReferencia = "F"
	SexoAmbos     SexoReferencia = "AMBOS"
)

var sexosValidos = map[SexoReferencia]bool{
	SexoMasculino: true, SexoFeminino: true, SexoAmbos: true,
}

// OperadorCritico é o operador de comparação de um valor crítico.
type OperadorCritico string

const (
	CriticoMenor      OperadorCritico = "<"
	CriticoMaior      OperadorCritico = ">"
	CriticoMenorIgual OperadorCritico = "<="
	CriticoMaiorIgual OperadorCritico = ">="
)

var operadoresValidos = map[OperadorCritico]bool{
	CriticoMenor: true, CriticoMaior: true, CriticoMenorIgual: true, CriticoMaiorIgual: true,
}

// IntervaloReferencia é o intervalo normal de uma análise para um perfil e sexo.
type IntervaloReferencia struct {
	Perfil PerfilReferencia `json:"perfil"`
	Sexo   SexoReferencia   `json:"sexo"`
	Minimo float64          `json:"minimo"`
	Maximo float64          `json:"maximo"`
}

// ValorCritico é uma condição que, quando satisfeita, torna o resultado crítico.
// Registado nesta fatia; avaliado na validação (Sprint 13).
type ValorCritico struct {
	Operador  OperadorCritico `json:"operador"`
	Limite    float64         `json:"limite"`
	Descricao string          `json:"descricao"`
}

// Analise é um agregado raiz do BC Laboratório: uma entrada do catálogo de análises.
// A chave é o código (não há id gerado pela BD).
type Analise struct {
	codigo     string
	nome       string
	unidade    string
	intervalos []IntervaloReferencia
	criticos   []ValorCritico
	activo     bool
	criadoEm   time.Time
}

// NovaAnalise valida e constrói uma entrada do catálogo. O código é normalizado
// para maiúsculas — é a chave por que os resultados referenciam a análise.
func NovaAnalise(codigo, nome, unidade string, intervalos []IntervaloReferencia, criticos []ValorCritico) (*Analise, error) {
	codigo = strings.ToUpper(strings.TrimSpace(codigo))
	if codigo == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "código da análise em falta")
	}
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "nome da análise em falta")
	}
	unidade = strings.TrimSpace(unidade)
	if unidade == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "unidade da análise em falta")
	}
	for _, i := range intervalos {
		if !perfisValidos[i.Perfil] {
			return nil, erros.Novo(erros.CategoriaValidacao,
				"perfil do intervalo de referência inválido (esperado ADULTO, PEDIATRICO ou GERIATRICO)")
		}
		if !sexosValidos[i.Sexo] {
			return nil, erros.Novo(erros.CategoriaValidacao,
				"sexo do intervalo de referência inválido (esperado M, F ou AMBOS)")
		}
		if i.Minimo > i.Maximo {
			return nil, erros.Novo(erros.CategoriaValidacao,
				"o mínimo do intervalo de referência não pode exceder o máximo")
		}
	}
	for _, v := range criticos {
		if !operadoresValidos[v.Operador] {
			return nil, erros.Novo(erros.CategoriaValidacao,
				"operador do valor crítico inválido (esperado <, >, <= ou >=)")
		}
		if strings.TrimSpace(v.Descricao) == "" {
			return nil, erros.Novo(erros.CategoriaValidacao, "descrição do valor crítico em falta")
		}
	}
	return &Analise{
		codigo: codigo, nome: nome, unidade: unidade,
		intervalos: intervalos, criticos: criticos, activo: true,
	}, nil
}

// Codigo devolve o código canónico (maiúsculas).
func (a *Analise) Codigo() string { return a.codigo }

// Nome devolve o nome da análise.
func (a *Analise) Nome() string { return a.nome }

// Unidade devolve a unidade de medida.
func (a *Analise) Unidade() string { return a.unidade }

// Activo indica se a análise pode ser requisitada.
func (a *Analise) Activo() bool { return a.activo }

// SnapshotAnalise carrega o estado completo para persistência ou rehidratação.
type SnapshotAnalise struct {
	Codigo         string
	Nome           string
	Unidade        string
	Intervalos     []IntervaloReferencia
	ValoresCriticos []ValorCritico
	Activo         bool
	CriadoEm       time.Time
}

// Snapshot devolve o estado completo do agregado.
func (a *Analise) Snapshot() SnapshotAnalise {
	return SnapshotAnalise{
		Codigo: a.codigo, Nome: a.nome, Unidade: a.unidade,
		Intervalos: a.intervalos, ValoresCriticos: a.criticos,
		Activo: a.activo, CriadoEm: a.criadoEm,
	}
}

// ReconstruirAnalise reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirAnalise(s SnapshotAnalise) *Analise {
	return &Analise{
		codigo: s.Codigo, nome: s.Nome, unidade: s.Unidade,
		intervalos: s.Intervalos, criticos: s.ValoresCriticos,
		activo: s.Activo, criadoEm: s.CriadoEm,
	}
}

// ResumoAnalise é a projecção de leitura do catálogo.
type ResumoAnalise struct {
	Codigo  string `json:"codigo"`
	Nome    string `json:"nome"`
	Unidade string `json:"unidade"`
	Activo  bool   `json:"activo"`
}

// RepositorioAnalises é a porta de saída de persistência do catálogo.
type RepositorioAnalises interface {
	Guardar(ctx context.Context, a *Analise) error
	ObterPorCodigo(ctx context.Context, codigo string) (*Analise, error)
	Listar(ctx context.Context) ([]ResumoAnalise, error)
}
```

- [ ] **Step 4: Actualizar `doc.go`**

Substituir o conteúdo por:

```go
// Package laboratorio contém o domínio do BC Laboratório: o catálogo de análises,
// a requisição e o resultado com a sua state machine. Camada 1 — Domínio.
// Sem imports de infra e sem tipos do domínio Clínico (a ligação é feita por ACL
// na camada de aplicação).
package laboratorio
```

- [ ] **Step 5: Correr para confirmar que passa**

Run: `go test ./internal/domain/laboratorio/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/laboratorio/analise.go internal/domain/laboratorio/analise_test.go internal/domain/laboratorio/doc.go
git commit -m "$(printf 'feat(laboratorio): agregado Analise (catalogo) com intervalos e valores criticos\n\nVOs IntervaloReferencia e ValorCritico validados, codigo normalizado como chave,\nsnapshot/reconstruir e porta de repositorio do catalogo.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 3: Domínio — agregado RequisicaoLab

**Files:**
- Create: `internal/domain/laboratorio/requisicao.go`
- Test: `internal/domain/laboratorio/requisicao_test.go`

**Interfaces:**
- Produces: `Prioridade` (`PrioridadeRotina`/`PrioridadeUrgente`), `ParsePrioridade(string) (Prioridade, error)`; `EstadoRequisicao` (`RequisicaoEmitida`/`RequisicaoCancelada`); `ItemRequisicao{CodigoAnalise, Observacoes string}`; `DadosNovaRequisicao{EpisodioID, DoenteID, MedicoRequisitanteID string; Prioridade Prioridade; Itens []ItemRequisicao}`; `NovaRequisicao(DadosNovaRequisicao) (*RequisicaoLab, error)`; getters `ID()/EpisodioID()/DoenteID()/Itens()/Estado()`; `Snapshot() SnapshotRequisicao`; `ReconstruirRequisicao(SnapshotRequisicao) *RequisicaoLab`; `ResumoRequisicao`; `RepositorioRequisicoes`.

- [ ] **Step 1: Escrever o teste que falha**

```go
package laboratorio_test

import (
	"testing"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func dadosReq() dominio.DadosNovaRequisicao {
	return dominio.DadosNovaRequisicao{
		EpisodioID: "ep-1", DoenteID: "doente-1", MedicoRequisitanteID: "med-1",
		Prioridade: dominio.PrioridadeRotina,
		Itens:      []dominio.ItemRequisicao{{CodigoAnalise: "HB"}, {CodigoAnalise: "GLIC"}},
	}
}

func TestNovaRequisicao_Valida(t *testing.T) {
	r, err := dominio.NovaRequisicao(dadosReq())
	if err != nil {
		t.Fatalf("requisição válida falhou: %v", err)
	}
	if r.Estado() != dominio.RequisicaoEmitida {
		t.Fatalf("esperava EMITIDA, veio %s", r.Estado())
	}
	if len(r.Itens()) != 2 {
		t.Fatalf("esperava 2 itens, veio %d", len(r.Itens()))
	}
}

func TestNovaRequisicao_SemItens(t *testing.T) {
	d := dadosReq()
	d.Itens = nil
	if _, err := dominio.NovaRequisicao(d); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("requisição sem itens devia falhar com Validacao, veio %v", err)
	}
}

func TestNovaRequisicao_ItensDuplicados(t *testing.T) {
	d := dadosReq()
	d.Itens = []dominio.ItemRequisicao{{CodigoAnalise: "HB"}, {CodigoAnalise: "hb"}}
	if _, err := dominio.NovaRequisicao(d); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("análise repetida (mesmo em minúsculas) devia falhar com Validacao, veio %v", err)
	}
}

func TestNovaRequisicao_CamposObrigatorios(t *testing.T) {
	casos := map[string]func(*dominio.DadosNovaRequisicao){
		"episódio em falta": func(d *dominio.DadosNovaRequisicao) { d.EpisodioID = "" },
		"doente em falta":   func(d *dominio.DadosNovaRequisicao) { d.DoenteID = "" },
		"médico em falta":   func(d *dominio.DadosNovaRequisicao) { d.MedicoRequisitanteID = "" },
	}
	for nome, mutar := range casos {
		t.Run(nome, func(t *testing.T) {
			d := dadosReq()
			mutar(&d)
			if _, err := dominio.NovaRequisicao(d); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
				t.Fatalf("esperava Validacao, veio %v", err)
			}
		})
	}
}

func TestParsePrioridade(t *testing.T) {
	if p, err := dominio.ParsePrioridade("urgente"); err != nil || p != dominio.PrioridadeUrgente {
		t.Fatalf("urgente devia ser válida, veio (%s, %v)", p, err)
	}
	if _, err := dominio.ParsePrioridade("IMEDIATA"); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("IMEDIATA devia falhar com Validacao, veio %v", err)
	}
}
```

- [ ] **Step 2: Correr para confirmar que falha**

Run: `go test ./internal/domain/laboratorio/ -run 'TestNovaRequisicao|TestParsePrioridade' -v`
Expected: FAIL (compilação: `NovaRequisicao` indefinido).

- [ ] **Step 3: Implementar `requisicao.go`**

```go
package laboratorio

import (
	"context"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Prioridade é a urgência de uma requisição de análises.
type Prioridade string

const (
	PrioridadeRotina  Prioridade = "ROTINA"
	PrioridadeUrgente Prioridade = "URGENTE"
)

var prioridadesValidas = map[Prioridade]bool{PrioridadeRotina: true, PrioridadeUrgente: true}

// ParsePrioridade valida e normaliza uma prioridade (aceita minúsculas).
func ParsePrioridade(codigo string) (Prioridade, error) {
	p := Prioridade(strings.ToUpper(strings.TrimSpace(codigo)))
	if !prioridadesValidas[p] {
		return "", erros.Novo(erros.CategoriaValidacao,
			"prioridade da requisição inválida (esperado ROTINA ou URGENTE)")
	}
	return p, nil
}

// EstadoRequisicao é o estado do ciclo de vida da requisição.
type EstadoRequisicao string

const (
	RequisicaoEmitida   EstadoRequisicao = "EMITIDA"
	RequisicaoCancelada EstadoRequisicao = "CANCELADA"
)

// ItemRequisicao é uma análise pedida numa requisição.
type ItemRequisicao struct {
	CodigoAnalise string `json:"codigo_analise"`
	Observacoes   string `json:"observacoes,omitempty"`
}

// RequisicaoLab é um agregado raiz do BC Laboratório: o pedido de análises de um
// médico para um episódio. episodioID/doenteID são referências a outro bounded
// context — validadas pela ACL na aplicação, sem FK.
type RequisicaoLab struct {
	id                   string
	episodioID           string
	doenteID             string
	medicoRequisitanteID string
	prioridade           Prioridade
	itens                []ItemRequisicao
	estado               EstadoRequisicao
	criadoEm             time.Time
}

// DadosNovaRequisicao agrupa os parâmetros de construção.
type DadosNovaRequisicao struct {
	EpisodioID           string
	DoenteID             string
	MedicoRequisitanteID string
	Prioridade           Prioridade
	Itens                []ItemRequisicao
}

// NovaRequisicao valida as invariantes e devolve a requisição EMITIDA. Os códigos
// de análise são normalizados para maiúsculas; repetições são rejeitadas (pedir a
// mesma análise duas vezes na mesma requisição criaria duas linhas na fila do
// técnico para a mesma colheita).
func NovaRequisicao(d DadosNovaRequisicao) (*RequisicaoLab, error) {
	d.EpisodioID = strings.TrimSpace(d.EpisodioID)
	if d.EpisodioID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "episódio da requisição em falta")
	}
	d.DoenteID = strings.TrimSpace(d.DoenteID)
	if d.DoenteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "doente da requisição em falta")
	}
	d.MedicoRequisitanteID = strings.TrimSpace(d.MedicoRequisitanteID)
	if d.MedicoRequisitanteID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "médico requisitante em falta")
	}
	if _, err := ParsePrioridade(string(d.Prioridade)); err != nil {
		return nil, err
	}
	if len(d.Itens) == 0 {
		return nil, erros.Novo(erros.CategoriaValidacao, "a requisição tem de pedir pelo menos uma análise")
	}
	vistos := map[string]bool{}
	itens := make([]ItemRequisicao, 0, len(d.Itens))
	for _, i := range d.Itens {
		codigo := strings.ToUpper(strings.TrimSpace(i.CodigoAnalise))
		if codigo == "" {
			return nil, erros.Novo(erros.CategoriaValidacao, "código de análise da requisição em falta")
		}
		if vistos[codigo] {
			return nil, erros.Novo(erros.CategoriaValidacao, "análise repetida na requisição: "+codigo)
		}
		vistos[codigo] = true
		itens = append(itens, ItemRequisicao{CodigoAnalise: codigo, Observacoes: strings.TrimSpace(i.Observacoes)})
	}
	return &RequisicaoLab{
		episodioID: d.EpisodioID, doenteID: d.DoenteID,
		medicoRequisitanteID: d.MedicoRequisitanteID, prioridade: d.Prioridade,
		itens: itens, estado: RequisicaoEmitida,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados (vazio se não persistida).
func (r *RequisicaoLab) ID() string { return r.id }

// EpisodioID devolve o episódio a que a requisição pertence.
func (r *RequisicaoLab) EpisodioID() string { return r.episodioID }

// DoenteID devolve o doente da requisição.
func (r *RequisicaoLab) DoenteID() string { return r.doenteID }

// Itens devolve os itens pedidos.
func (r *RequisicaoLab) Itens() []ItemRequisicao { return r.itens }

// Estado devolve o estado actual.
func (r *RequisicaoLab) Estado() EstadoRequisicao { return r.estado }

// SnapshotRequisicao carrega o estado completo para persistência ou rehidratação.
type SnapshotRequisicao struct {
	ID                   string
	EpisodioID           string
	DoenteID             string
	MedicoRequisitanteID string
	Prioridade           Prioridade
	Itens                []ItemRequisicao
	Estado               EstadoRequisicao
	CriadoEm             time.Time
}

// Snapshot devolve o estado completo do agregado.
func (r *RequisicaoLab) Snapshot() SnapshotRequisicao {
	return SnapshotRequisicao{
		ID: r.id, EpisodioID: r.episodioID, DoenteID: r.doenteID,
		MedicoRequisitanteID: r.medicoRequisitanteID, Prioridade: r.prioridade,
		Itens: r.itens, Estado: r.estado, CriadoEm: r.criadoEm,
	}
}

// ReconstruirRequisicao reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirRequisicao(s SnapshotRequisicao) *RequisicaoLab {
	return &RequisicaoLab{
		id: s.ID, episodioID: s.EpisodioID, doenteID: s.DoenteID,
		medicoRequisitanteID: s.MedicoRequisitanteID, prioridade: s.Prioridade,
		itens: s.Itens, estado: s.Estado, criadoEm: s.CriadoEm,
	}
}

// ResumoRequisicao é a projecção de leitura de uma requisição.
type ResumoRequisicao struct {
	ID         string    `json:"id"`
	EpisodioID string    `json:"episodio_id"`
	DoenteID   string    `json:"doente_id"`
	Prioridade string    `json:"prioridade"`
	Estado     string    `json:"estado"`
	NumAnalises int      `json:"num_analises"`
	CriadoEm   time.Time `json:"criado_em"`
}

// Nota: a porta RepositorioRequisicoes vive em resultado.go (Task 4) — a sua
// operação de emissão recebe os *Resultado criados com a requisição, e mantê-la
// junto do Resultado evita que este ficheiro dependa de um tipo que ainda não existe.
```

Note que o `import "context"` **não** é usado neste ficheiro (a interface do repositório
está na Task 4). O bloco de imports de `requisicao.go` é apenas:

```go
import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)
```

- [ ] **Step 4: Correr para confirmar que passa**

Run: `go test ./internal/domain/laboratorio/ -run 'TestNovaRequisicao|TestParsePrioridade' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/laboratorio/requisicao.go internal/domain/laboratorio/requisicao_test.go
git commit -m "$(printf 'feat(laboratorio): agregado RequisicaoLab com itens e prioridade\n\nCodigos de analise normalizados e sem repeticoes, referencias ao episodio e ao\ndoente sem FK cross-context, e porta de repositorio com emissao transaccional.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 4: Domínio — agregado Resultado (state machine) + eventos

**Files:**
- Create: `internal/domain/laboratorio/resultado.go`
- Create: `internal/domain/laboratorio/eventos.go`
- Test: `internal/domain/laboratorio/resultado_test.go`

**Interfaces:**
- Consumes: `RequisicaoLab`, `ItemRequisicao` (Task 3).
- Produces: `EstadoResultado` (`ResPendente`/`ResColhida`/`ResProcessada`/`ResValidada`/`ResConcluida`/`ResRecusada`); `NovoResultado(requisicaoID, codigoAnalise, unidade string) (*Resultado, error)`; métodos `ColherAmostra(tecnicoID string, em time.Time) error`, `RecusarAmostra(motivo string, em time.Time) error`, `SubmeterPreliminar(tecnicoID, valor, observacoes string, em time.Time) error`; getters `ID()/RequisicaoID()/Estado()/TecnicoSubmissorID()`; `Snapshot() SnapshotResultado` (com `EstadoAnterior`) / `ReconstruirResultado`; `ResumoResultado`; `RepositorioResultados`; `RepositorioRequisicoes`; eventos `AmostraColhida`, `AmostraRecusada`, `ResultadoPreliminarSubmetido`.

- [ ] **Step 1: Escrever o teste que falha**

```go
package laboratorio_test

import (
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func novoRes(t *testing.T) *dominio.Resultado {
	t.Helper()
	r, err := dominio.NovoResultado("req-1", "HB", "g/dL")
	if err != nil {
		t.Fatalf("resultado base inválido: %v", err)
	}
	return r
}

func TestNovoResultado_NasceEmPendente(t *testing.T) {
	r := novoRes(t)
	if r.Estado() != dominio.ResPendente {
		t.Fatalf("esperava PENDENTE, veio %s", r.Estado())
	}
	if r.TecnicoSubmissorID() != "" {
		t.Fatalf("um resultado novo não tem submissor")
	}
}

func TestResultado_CicloAteAoPreliminar(t *testing.T) {
	r := novoRes(t)
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	// Submeter sem colher → Conflito.
	if err := r.SubmeterPreliminar("tec-1", "12.5", "", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("submeter sem colher devia falhar com Conflito, veio %v", err)
	}
	if err := r.ColherAmostra("tec-1", quando); err != nil {
		t.Fatalf("colher devia funcionar: %v", err)
	}
	if r.Estado() != dominio.ResColhida {
		t.Fatalf("esperava COLHIDA, veio %s", r.Estado())
	}
	// Colher de novo → Conflito.
	if err := r.ColherAmostra("tec-1", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("colher duas vezes devia falhar com Conflito, veio %v", err)
	}
	// Submeter sem valor → Validacao.
	if err := r.SubmeterPreliminar("tec-1", "  ", "", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("submeter sem valor devia falhar com Validacao, veio %v", err)
	}
	// Submeter sem técnico → Validacao. O submissor é o sujeito autenticado: se
	// chegasse vazio, a segregação do Sprint 13 não teria contra quem comparar.
	if err := r.SubmeterPreliminar("", "12.5", "", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("submeter sem técnico devia falhar com Validacao, veio %v", err)
	}
	if err := r.SubmeterPreliminar("tec-1", "12.5", "amostra hemolisada", quando.Add(time.Hour)); err != nil {
		t.Fatalf("submeter preliminar devia funcionar: %v", err)
	}
	if r.Estado() != dominio.ResProcessada {
		t.Fatalf("esperava PROCESSADA, veio %s", r.Estado())
	}
	if r.TecnicoSubmissorID() != "tec-1" {
		t.Fatalf("esperava submissor tec-1, veio %q", r.TecnicoSubmissorID())
	}
	s := r.Snapshot()
	if s.EstadoAnterior != dominio.ResColhida {
		t.Fatalf("o snapshot devia expor o estado anterior COLHIDA (compare-and-set), veio %s", s.EstadoAnterior)
	}
}

func TestResultado_RecusarAmostra(t *testing.T) {
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	// Sem motivo → Validacao.
	r := novoRes(t)
	if err := r.RecusarAmostra("  ", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("recusar sem motivo devia falhar com Validacao, veio %v", err)
	}
	if err := r.RecusarAmostra("amostra coagulada", quando); err != nil {
		t.Fatalf("recusar em PENDENTE devia funcionar: %v", err)
	}
	if r.Estado() != dominio.ResRecusada {
		t.Fatalf("esperava RECUSADA, veio %s", r.Estado())
	}
	// Recusar de novo → Conflito.
	if err := r.RecusarAmostra("outra razão", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("recusar duas vezes devia falhar com Conflito, veio %v", err)
	}

	// Depois de processada já não se recusa.
	p := novoRes(t)
	_ = p.ColherAmostra("tec-1", quando)
	_ = p.SubmeterPreliminar("tec-1", "12.5", "", quando)
	if err := p.RecusarAmostra("tarde demais", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("recusar uma amostra já processada devia falhar com Conflito, veio %v", err)
	}
}

func TestResultado_ReconstruirPreservaEstado(t *testing.T) {
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	r := novoRes(t)
	_ = r.ColherAmostra("tec-1", quando)
	s := r.Snapshot()
	s.ID = "res-1"
	b := dominio.ReconstruirResultado(s)
	if b.Estado() != dominio.ResColhida || b.ID() != "res-1" {
		t.Fatalf("reconstrução não preservou o snapshot: %+v", b.Snapshot())
	}
	// Um agregado reconstruído tem EstadoAnterior = Estado: ainda não transitou.
	if b.Snapshot().EstadoAnterior != dominio.ResColhida {
		t.Fatalf("o estado anterior de um agregado recém-lido é o próprio estado")
	}
}
```

- [ ] **Step 2: Correr para confirmar que falha**

Run: `go test ./internal/domain/laboratorio/ -run TestResultado -v`
Expected: FAIL (compilação: `NovoResultado` indefinido).

- [ ] **Step 3: Implementar `resultado.go`**

```go
package laboratorio

import (
	"context"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// EstadoResultado é o estado do ciclo de vida de um resultado de análise.
//
//	PENDENTE → COLHIDA → PROCESSADA → VALIDADA → CONCLUIDA
//	    └──────────┴─────► RECUSADA
//
// VALIDADA e CONCLUIDA existem já no enum e nas CHECK da base de dados; a transição
// Validar (com a invariante de segregação submissor ≠ validador) é do Sprint 13.
type EstadoResultado string

const (
	ResPendente   EstadoResultado = "PENDENTE"
	ResColhida    EstadoResultado = "COLHIDA"
	ResProcessada EstadoResultado = "PROCESSADA"
	ResValidada   EstadoResultado = "VALIDADA"
	ResConcluida  EstadoResultado = "CONCLUIDA"
	ResRecusada   EstadoResultado = "RECUSADA"
)

// Resultado é um agregado raiz do BC Laboratório: o resultado de uma análise de uma
// requisição. É criado em PENDENTE, um por item da requisição.
type Resultado struct {
	id                     string
	requisicaoID           string
	codigoAnalise          string
	valor                  string
	unidade                string
	observacoes            string
	motivoRecusa           string
	estado                 EstadoResultado
	estadoAnterior         EstadoResultado
	tecnicoColheitaID      string
	tecnicoSubmissorID     string
	patologistaValidadorID string
	colhidaEm              *time.Time
	submetidaEm            *time.Time
	validadaEm             *time.Time
	valorCritico           bool
	criadoEm               time.Time
}

// NovoResultado cria um resultado em PENDENTE para um item da requisição.
func NovoResultado(requisicaoID, codigoAnalise, unidade string) (*Resultado, error) {
	requisicaoID = strings.TrimSpace(requisicaoID)
	if requisicaoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "requisição do resultado em falta")
	}
	codigoAnalise = strings.ToUpper(strings.TrimSpace(codigoAnalise))
	if codigoAnalise == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "código de análise do resultado em falta")
	}
	unidade = strings.TrimSpace(unidade)
	if unidade == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "unidade do resultado em falta")
	}
	return &Resultado{
		requisicaoID: requisicaoID, codigoAnalise: codigoAnalise, unidade: unidade,
		estado: ResPendente, estadoAnterior: ResPendente,
	}, nil
}

// ColherAmostra transita PENDENTE → COLHIDA. O técnico é o sujeito autenticado.
func (r *Resultado) ColherAmostra(tecnicoID string, em time.Time) error {
	if r.estado != ResPendente {
		return erros.Novo(erros.CategoriaConflito, "só é possível colher a amostra de um resultado pendente")
	}
	tecnicoID = strings.TrimSpace(tecnicoID)
	if tecnicoID == "" {
		return erros.Novo(erros.CategoriaValidacao, "técnico da colheita em falta")
	}
	if em.IsZero() {
		return erros.Novo(erros.CategoriaValidacao, "data da colheita em falta")
	}
	r.estado = ResColhida
	r.tecnicoColheitaID = tecnicoID
	r.colhidaEm = &em
	return nil
}

// RecusarAmostra transita PENDENTE ou COLHIDA → RECUSADA. O motivo é obrigatório:
// uma amostra inviável sem motivo registado não é auditável nem repetível.
func (r *Resultado) RecusarAmostra(motivo string, em time.Time) error {
	if r.estado != ResPendente && r.estado != ResColhida {
		return erros.Novo(erros.CategoriaConflito,
			"só é possível recusar uma amostra pendente ou colhida")
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return erros.Novo(erros.CategoriaValidacao, "motivo da recusa da amostra em falta")
	}
	if em.IsZero() {
		return erros.Novo(erros.CategoriaValidacao, "data da recusa em falta")
	}
	r.estado = ResRecusada
	r.motivoRecusa = motivo
	return nil
}

// SubmeterPreliminar transita COLHIDA → PROCESSADA. O submissor é o sujeito
// autenticado — nunca um campo do pedido: é contra ele que a validação do Sprint 13
// compara o patologista para impor a segregação de funções.
func (r *Resultado) SubmeterPreliminar(tecnicoID, valor, observacoes string, em time.Time) error {
	if r.estado != ResColhida {
		return erros.Novo(erros.CategoriaConflito,
			"só é possível submeter o resultado de uma amostra colhida")
	}
	tecnicoID = strings.TrimSpace(tecnicoID)
	if tecnicoID == "" {
		return erros.Novo(erros.CategoriaValidacao, "técnico submissor em falta")
	}
	valor = strings.TrimSpace(valor)
	if valor == "" {
		return erros.Novo(erros.CategoriaValidacao, "valor do resultado em falta")
	}
	if em.IsZero() {
		return erros.Novo(erros.CategoriaValidacao, "data da submissão em falta")
	}
	r.estado = ResProcessada
	r.tecnicoSubmissorID = tecnicoID
	r.valor = valor
	r.observacoes = strings.TrimSpace(observacoes)
	r.submetidaEm = &em
	return nil
}

// ID devolve o identificador atribuído pela base de dados.
func (r *Resultado) ID() string { return r.id }

// RequisicaoID devolve a requisição a que o resultado pertence.
func (r *Resultado) RequisicaoID() string { return r.requisicaoID }

// Estado devolve o estado actual.
func (r *Resultado) Estado() EstadoResultado { return r.estado }

// TecnicoSubmissorID devolve quem submeteu o preliminar (vazio antes da submissão).
func (r *Resultado) TecnicoSubmissorID() string { return r.tecnicoSubmissorID }

// SnapshotResultado carrega o estado completo para persistência ou rehidratação.
//
// EstadoAnterior é o estado com que o agregado foi lido da base de dados: é o que o
// repositório usa na guarda compare-and-set do UPDATE de transição. Num agregado
// recém-lido (ou recém-criado) é igual a Estado.
type SnapshotResultado struct {
	ID                     string
	RequisicaoID           string
	CodigoAnalise          string
	Valor                  string
	Unidade                string
	Observacoes            string
	MotivoRecusa           string
	Estado                 EstadoResultado
	EstadoAnterior         EstadoResultado
	TecnicoColheitaID      string
	TecnicoSubmissorID     string
	PatologistaValidadorID string
	ColhidaEm              *time.Time
	SubmetidaEm            *time.Time
	ValidadaEm             *time.Time
	ValorCritico           bool
	CriadoEm               time.Time
}

// Snapshot devolve o estado completo do agregado.
func (r *Resultado) Snapshot() SnapshotResultado {
	return SnapshotResultado{
		ID: r.id, RequisicaoID: r.requisicaoID, CodigoAnalise: r.codigoAnalise,
		Valor: r.valor, Unidade: r.unidade, Observacoes: r.observacoes,
		MotivoRecusa: r.motivoRecusa, Estado: r.estado, EstadoAnterior: r.estadoAnterior,
		TecnicoColheitaID: r.tecnicoColheitaID, TecnicoSubmissorID: r.tecnicoSubmissorID,
		PatologistaValidadorID: r.patologistaValidadorID,
		ColhidaEm: r.colhidaEm, SubmetidaEm: r.submetidaEm, ValidadaEm: r.validadaEm,
		ValorCritico: r.valorCritico, CriadoEm: r.criadoEm,
	}
}

// ReconstruirResultado reconstrói o agregado a partir de um snapshot persistido.
// EstadoAnterior é fixado no estado lido — qualquer transição posterior deixa-o a
// apontar para o estado que está na base de dados.
func ReconstruirResultado(s SnapshotResultado) *Resultado {
	return &Resultado{
		id: s.ID, requisicaoID: s.RequisicaoID, codigoAnalise: s.CodigoAnalise,
		valor: s.Valor, unidade: s.Unidade, observacoes: s.Observacoes,
		motivoRecusa: s.MotivoRecusa, estado: s.Estado, estadoAnterior: s.Estado,
		tecnicoColheitaID: s.TecnicoColheitaID, tecnicoSubmissorID: s.TecnicoSubmissorID,
		patologistaValidadorID: s.PatologistaValidadorID,
		colhidaEm: s.ColhidaEm, submetidaEm: s.SubmetidaEm, validadaEm: s.ValidadaEm,
		valorCritico: s.ValorCritico, criadoEm: s.CriadoEm,
	}
}

// ResumoResultado é a projecção de leitura de um resultado.
type ResumoResultado struct {
	ID            string     `json:"id"`
	RequisicaoID  string     `json:"requisicao_id"`
	EpisodioID    string     `json:"episodio_id,omitempty"`
	CodigoAnalise string     `json:"codigo_analise"`
	Valor         string     `json:"valor,omitempty"`
	Unidade       string     `json:"unidade"`
	Estado        string     `json:"estado"`
	ValorCritico  bool       `json:"valor_critico"`
	ColhidaEm     *time.Time `json:"colhida_em,omitempty"`
	SubmetidaEm   *time.Time `json:"submetida_em,omitempty"`
	CriadoEm      time.Time  `json:"criado_em"`
}

// RepositorioResultados é a porta de saída de persistência de resultados.
//
// Transitar aplica a transição com guarda compare-and-set (usa EstadoAnterior do
// snapshot); uma lista de estados vazia em ListarFila/ListarPorEpisodio significa
// "todos os estados".
type RepositorioResultados interface {
	ObterPorID(ctx context.Context, id string) (*Resultado, error)
	Transitar(ctx context.Context, r *Resultado) error
	ListarFila(ctx context.Context, estados []EstadoResultado) ([]ResumoResultado, error)
	ListarPorEpisodio(ctx context.Context, episodioID string, estados []EstadoResultado) ([]ResumoResultado, error)
}

// RepositorioRequisicoes é a porta de saída de persistência de requisições.
//
// Emitir grava a requisição, os seus itens e os resultados PENDENTE numa única
// transacção: uma requisição sem resultados nunca chegaria à fila do laboratório e
// ficaria invisível para toda a gente.
type RepositorioRequisicoes interface {
	Emitir(ctx context.Context, r *RequisicaoLab, resultados []*Resultado) (string, error)
	ObterPorID(ctx context.Context, id string) (*RequisicaoLab, error)
	ListarPorEpisodio(ctx context.Context, episodioID string) ([]ResumoRequisicao, error)
}
```

- [ ] **Step 4: Implementar `eventos.go`**

Scaffolding: definidos agora, emitidos quando o Outbox for ligado (como em Clínico e
Farmácia).

```go
package laboratorio

import "time"

// AmostraColhida é emitido quando o técnico regista a colheita.
type AmostraColhida struct {
	ResultadoID   string
	RequisicaoID  string
	CodigoAnalise string
	Em            time.Time
}

func (e AmostraColhida) NomeEvento() string    { return "laboratorio.amostra.colhida" }
func (e AmostraColhida) OcorridoEm() time.Time { return e.Em }

// AmostraRecusada é emitido quando a amostra é recusada por inviabilidade.
type AmostraRecusada struct {
	ResultadoID  string
	RequisicaoID string
	Motivo       string
	Em           time.Time
}

func (e AmostraRecusada) NomeEvento() string    { return "laboratorio.amostra.recusada" }
func (e AmostraRecusada) OcorridoEm() time.Time { return e.Em }

// ResultadoPreliminarSubmetido é emitido quando o técnico submete o preliminar. O
// resultado ainda NÃO é visível ao médico — só a validação (Sprint 13) o torna.
type ResultadoPreliminarSubmetido struct {
	ResultadoID   string
	RequisicaoID  string
	CodigoAnalise string
	Em            time.Time
}

func (e ResultadoPreliminarSubmetido) NomeEvento() string {
	return "laboratorio.resultado.preliminar_submetido"
}
func (e ResultadoPreliminarSubmetido) OcorridoEm() time.Time { return e.Em }
```

- [ ] **Step 5: Correr para confirmar que passa**

Run: `go test ./internal/domain/laboratorio/ -v && go build ./...`
Expected: PASS (todos os testes do domínio, incluindo os das Tasks 2 e 3).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/laboratorio/resultado.go internal/domain/laboratorio/eventos.go internal/domain/laboratorio/resultado_test.go
git commit -m "$(printf 'feat(laboratorio): agregado Resultado com state machine ate ao preliminar\n\nPENDENTE->COLHIDA->PROCESSADA e recusa da amostra com motivo obrigatorio. O\nsubmissor e sempre explicito (base da segregacao do Sprint 13) e o snapshot expoe\no estado anterior para a guarda compare-and-set. Eventos em scaffolding.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 5: Aplicação — portas, DTOs e casos de uso do catálogo

**Files:**
- Create: `internal/application/laboratorio/ports.go`
- Create: `internal/application/laboratorio/mapa.go`
- Create: `internal/application/laboratorio/analises.go`
- Test: `internal/application/laboratorio/fakes_test.go`
- Test: `internal/application/laboratorio/analises_test.go`

**Interfaces:**
- Consumes: `dominio.RepositorioAnalises` (Task 2).
- Produces: `Auditor`; `LeitorClinico` (com `DoenteActivo`, `EpisodioAbertoDoDoente`); DTOs `DadosNovaAnalise`, `DetalheAnalise`, `DadosEmitirRequisicao`, `ItemPedido`, `DetalheRequisicao`, `DadosSubmeterPreliminar`, `DetalheResultado`; reexports `ResumoAnalise`/`ResumoRequisicao`/`ResumoResultado`; `CasoRegistarAnalise`/`CasoListarAnalises` (cada um com `NovoCaso…` e `Executar`); mapeadores `paraDetalheAnalise`, `paraDetalheRequisicao`, `paraDetalheResultado`.

- [ ] **Step 1: Criar `ports.go`**

```go
// Package laboratorio contém os casos de uso do BC Laboratório (Camada 2 — Aplicação).
package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// Auditor persiste registos de auditoria de forma append-only.
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}

// LeitorClinico é a porta anti-corrupção para leitura de dados do BC Clínico. O
// Laboratório nunca importa tipos do domínio Clínico: só faz estas duas perguntas.
type LeitorClinico interface {
	// DoenteActivo indica se o doente existe e está activo.
	DoenteActivo(ctx context.Context, doenteID string) (bool, error)
	// EpisodioAbertoDoDoente indica se o episódio existe, pertence ao doente e
	// está ABERTO.
	EpisodioAbertoDoDoente(ctx context.Context, episodioID, doenteID string) (bool, error)
}

// Reexports dos read-models do domínio.
type (
	ResumoAnalise    = dominio.ResumoAnalise
	ResumoRequisicao = dominio.ResumoRequisicao
	ResumoResultado  = dominio.ResumoResultado
)

// DadosNovaAnalise é a entrada do registo de uma análise no catálogo.
type DadosNovaAnalise struct {
	Codigo          string                        `json:"codigo"`
	Nome            string                        `json:"nome"`
	Unidade         string                        `json:"unidade"`
	Intervalos      []dominio.IntervaloReferencia `json:"intervalos_referencia"`
	ValoresCriticos []dominio.ValorCritico        `json:"valores_criticos"`
}

// DetalheAnalise é o detalhe de uma análise numa resposta.
type DetalheAnalise struct {
	Codigo          string                        `json:"codigo"`
	Nome            string                        `json:"nome"`
	Unidade         string                        `json:"unidade"`
	Intervalos      []dominio.IntervaloReferencia `json:"intervalos_referencia"`
	ValoresCriticos []dominio.ValorCritico        `json:"valores_criticos"`
	Activo          bool                          `json:"activo"`
}

// ItemPedido é uma análise pedida numa requisição.
type ItemPedido struct {
	CodigoAnalise string `json:"codigo_analise"`
	Observacoes   string `json:"observacoes"`
}

// DadosEmitirRequisicao é a entrada da emissão de uma requisição. O EpisodioID vem
// do caminho; o MedicoRequisitanteID é o sujeito autenticado, não um campo do corpo.
type DadosEmitirRequisicao struct {
	EpisodioID string
	DoenteID   string       `json:"doente_id"`
	Prioridade string       `json:"prioridade"`
	Itens      []ItemPedido `json:"itens"`
}

// DetalheRequisicao é o detalhe de uma requisição numa resposta.
type DetalheRequisicao struct {
	ID                   string                  `json:"id"`
	EpisodioID           string                  `json:"episodio_id"`
	DoenteID             string                  `json:"doente_id"`
	MedicoRequisitanteID string                  `json:"medico_requisitante_id"`
	Prioridade           string                  `json:"prioridade"`
	Estado               string                  `json:"estado"`
	Itens                []dominio.ItemRequisicao `json:"itens"`
	CriadoEm             time.Time               `json:"criado_em"`
}

// DadosSubmeterPreliminar é a entrada da submissão do resultado preliminar.
type DadosSubmeterPreliminar struct {
	Valor       string `json:"valor"`
	Observacoes string `json:"observacoes"`
}

// DetalheResultado é o detalhe de um resultado numa resposta.
type DetalheResultado struct {
	ID                 string     `json:"id"`
	RequisicaoID       string     `json:"requisicao_id"`
	CodigoAnalise      string     `json:"codigo_analise"`
	Valor              string     `json:"valor,omitempty"`
	Unidade            string     `json:"unidade"`
	Observacoes        string     `json:"observacoes,omitempty"`
	MotivoRecusa       string     `json:"motivo_recusa,omitempty"`
	Estado             string     `json:"estado"`
	TecnicoSubmissorID string     `json:"tecnico_submissor_id,omitempty"`
	ColhidaEm          *time.Time `json:"colhida_em,omitempty"`
	SubmetidaEm        *time.Time `json:"submetida_em,omitempty"`
	ValorCritico       bool       `json:"valor_critico"`
}

// EstadosVisiveisAoMedico são os únicos estados que a leitura clínica devolve: o
// resultado preliminar (PROCESSADA) não é visível ao médico — só a validação do
// patologista o torna visível (critério de saída do marco).
var EstadosVisiveisAoMedico = []dominio.EstadoResultado{dominio.ResValidada, dominio.ResConcluida}
```

- [ ] **Step 2: Criar `mapa.go`**

```go
package laboratorio

import dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"

// paraDetalheAnalise projecta o agregado do catálogo num DTO de resposta.
func paraDetalheAnalise(a *dominio.Analise) DetalheAnalise {
	s := a.Snapshot()
	return DetalheAnalise{
		Codigo: s.Codigo, Nome: s.Nome, Unidade: s.Unidade,
		Intervalos: s.Intervalos, ValoresCriticos: s.ValoresCriticos, Activo: s.Activo,
	}
}

// paraDetalheRequisicao projecta a requisição num DTO de resposta.
func paraDetalheRequisicao(r *dominio.RequisicaoLab) DetalheRequisicao {
	s := r.Snapshot()
	return DetalheRequisicao{
		ID: s.ID, EpisodioID: s.EpisodioID, DoenteID: s.DoenteID,
		MedicoRequisitanteID: s.MedicoRequisitanteID, Prioridade: string(s.Prioridade),
		Estado: string(s.Estado), Itens: s.Itens, CriadoEm: s.CriadoEm,
	}
}

// paraDetalheResultado projecta o resultado num DTO de resposta.
func paraDetalheResultado(r *dominio.Resultado) DetalheResultado {
	s := r.Snapshot()
	return DetalheResultado{
		ID: s.ID, RequisicaoID: s.RequisicaoID, CodigoAnalise: s.CodigoAnalise,
		Valor: s.Valor, Unidade: s.Unidade, Observacoes: s.Observacoes,
		MotivoRecusa: s.MotivoRecusa, Estado: string(s.Estado),
		TecnicoSubmissorID: s.TecnicoSubmissorID,
		ColhidaEm: s.ColhidaEm, SubmetidaEm: s.SubmetidaEm, ValorCritico: s.ValorCritico,
	}
}
```

- [ ] **Step 3: Escrever os fakes (`fakes_test.go`)**

Fakes em memória (o projecto usa fakes, não mocks — princípio 6 do CLAUDE.md).

```go
package laboratorio_test

import (
	"context"
	"strconv"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeAnalises é um RepositorioAnalises em memória.
type fakeAnalises struct {
	porCodigo map[string]*laboratorio.Analise
}

func novoFakeAnalises() *fakeAnalises {
	return &fakeAnalises{porCodigo: map[string]*laboratorio.Analise{}}
}

func (f *fakeAnalises) Guardar(_ context.Context, a *laboratorio.Analise) error {
	if _, existe := f.porCodigo[a.Codigo()]; existe {
		return erros.Novo(erros.CategoriaConflito, "já existe uma análise com este código")
	}
	f.porCodigo[a.Codigo()] = a
	return nil
}

func (f *fakeAnalises) ObterPorCodigo(_ context.Context, codigo string) (*laboratorio.Analise, error) {
	a, ok := f.porCodigo[codigo]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "análise não encontrada")
	}
	return a, nil
}

func (f *fakeAnalises) Listar(_ context.Context) ([]laboratorio.ResumoAnalise, error) {
	out := []laboratorio.ResumoAnalise{}
	for _, a := range f.porCodigo {
		s := a.Snapshot()
		out = append(out, laboratorio.ResumoAnalise{
			Codigo: s.Codigo, Nome: s.Nome, Unidade: s.Unidade, Activo: s.Activo,
		})
	}
	return out, nil
}

// fakeRequisicoes é um RepositorioRequisicoes em memória. Emitir guarda a requisição
// e os resultados (o fake não simula transacções — a atomicidade é do pgrepo).
type fakeRequisicoes struct {
	porID       map[string]*laboratorio.RequisicaoLab
	resultados  *fakeResultados
	seq         int
}

func novoFakeRequisicoes(res *fakeResultados) *fakeRequisicoes {
	return &fakeRequisicoes{porID: map[string]*laboratorio.RequisicaoLab{}, resultados: res}
}

func (f *fakeRequisicoes) Emitir(_ context.Context, r *laboratorio.RequisicaoLab, resultados []*laboratorio.Resultado) (string, error) {
	f.seq++
	id := "req-" + strconv.Itoa(f.seq)
	s := r.Snapshot()
	s.ID = id
	f.porID[id] = laboratorio.ReconstruirRequisicao(s)
	// O fake regista o episódio da requisição para que ListarPorEpisodio dos
	// resultados possa fazer a junção que, no repositório real, é um JOIN SQL.
	f.resultados.episodioDe[id] = s.EpisodioID
	for _, res := range resultados {
		sr := res.Snapshot()
		sr.RequisicaoID = id
		f.resultados.inserir(sr)
	}
	return id, nil
}

func (f *fakeRequisicoes) ObterPorID(_ context.Context, id string) (*laboratorio.RequisicaoLab, error) {
	r, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "requisição não encontrada")
	}
	return r, nil
}

func (f *fakeRequisicoes) ListarPorEpisodio(_ context.Context, episodioID string) ([]laboratorio.ResumoRequisicao, error) {
	out := []laboratorio.ResumoRequisicao{}
	for _, r := range f.porID {
		s := r.Snapshot()
		if s.EpisodioID != episodioID {
			continue
		}
		out = append(out, laboratorio.ResumoRequisicao{
			ID: s.ID, EpisodioID: s.EpisodioID, DoenteID: s.DoenteID,
			Prioridade: string(s.Prioridade), Estado: string(s.Estado),
			NumAnalises: len(s.Itens), CriadoEm: s.CriadoEm,
		})
	}
	return out, nil
}

// fakeResultados é um RepositorioResultados em memória.
type fakeResultados struct {
	porID       map[string]laboratorio.SnapshotResultado
	episodioDe  map[string]string // requisicaoID → episodioID
	seq         int
}

func novoFakeResultados() *fakeResultados {
	return &fakeResultados{
		porID:      map[string]laboratorio.SnapshotResultado{},
		episodioDe: map[string]string{},
	}
}

func (f *fakeResultados) inserir(s laboratorio.SnapshotResultado) string {
	f.seq++
	id := "res-" + strconv.Itoa(f.seq)
	s.ID = id
	f.porID[id] = s
	return id
}

func (f *fakeResultados) ObterPorID(_ context.Context, id string) (*laboratorio.Resultado, error) {
	s, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
	}
	return laboratorio.ReconstruirResultado(s), nil
}

func (f *fakeResultados) Transitar(_ context.Context, r *laboratorio.Resultado) error {
	s := r.Snapshot()
	actual, ok := f.porID[s.ID]
	if !ok {
		return erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
	}
	// Compare-and-set, como o repositório real.
	if actual.Estado != s.EstadoAnterior {
		return erros.Novo(erros.CategoriaConflito, "o estado do resultado mudou entretanto")
	}
	f.porID[s.ID] = s
	return nil
}

func (f *fakeResultados) contem(estados []laboratorio.EstadoResultado, e laboratorio.EstadoResultado) bool {
	if len(estados) == 0 {
		return true
	}
	for _, x := range estados {
		if x == e {
			return true
		}
	}
	return false
}

func (f *fakeResultados) ListarFila(_ context.Context, estados []laboratorio.EstadoResultado) ([]laboratorio.ResumoResultado, error) {
	out := []laboratorio.ResumoResultado{}
	for _, s := range f.porID {
		if !f.contem(estados, s.Estado) {
			continue
		}
		out = append(out, laboratorio.ResumoResultado{
			ID: s.ID, RequisicaoID: s.RequisicaoID, CodigoAnalise: s.CodigoAnalise,
			Valor: s.Valor, Unidade: s.Unidade, Estado: string(s.Estado),
			ValorCritico: s.ValorCritico, ColhidaEm: s.ColhidaEm, SubmetidaEm: s.SubmetidaEm,
		})
	}
	return out, nil
}

func (f *fakeResultados) ListarPorEpisodio(_ context.Context, episodioID string, estados []laboratorio.EstadoResultado) ([]laboratorio.ResumoResultado, error) {
	out := []laboratorio.ResumoResultado{}
	for _, s := range f.porID {
		if f.episodioDe[s.RequisicaoID] != episodioID {
			continue
		}
		if !f.contem(estados, s.Estado) {
			continue
		}
		out = append(out, laboratorio.ResumoResultado{
			ID: s.ID, RequisicaoID: s.RequisicaoID, EpisodioID: episodioID,
			CodigoAnalise: s.CodigoAnalise, Valor: s.Valor, Unidade: s.Unidade,
			Estado: string(s.Estado), ValorCritico: s.ValorCritico,
		})
	}
	return out, nil
}

// fakeLeitorClinico é a ACL em memória.
type fakeLeitorClinico struct {
	doenteActivo   bool
	episodioAberto bool
}

func (f *fakeLeitorClinico) DoenteActivo(_ context.Context, _ string) (bool, error) {
	return f.doenteActivo, nil
}

func (f *fakeLeitorClinico) EpisodioAbertoDoDoente(_ context.Context, _, _ string) (bool, error) {
	return f.episodioAberto, nil
}

// fakeAuditor recolhe os registos de auditoria.
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
```

- [ ] **Step 4: Escrever o teste do catálogo (`analises_test.go`)**

```go
package laboratorio_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRegistarAnalise(t *testing.T) {
	repo := novoFakeAnalises()
	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarAnalise(repo, aud)

	out, err := uc.Executar(context.Background(), "admin-1", app.DadosNovaAnalise{
		Codigo: "glic", Nome: "Glicemia", Unidade: "mg/dL",
		Intervalos: []dominio.IntervaloReferencia{
			{Perfil: dominio.PerfilAdulto, Sexo: dominio.SexoAmbos, Minimo: 70, Maximo: 110},
		},
	})
	if err != nil {
		t.Fatalf("registar análise: %v", err)
	}
	if out.Codigo != "GLIC" {
		t.Fatalf("esperava código normalizado GLIC, veio %q", out.Codigo)
	}
	if !aud.tem("laboratorio.analise.registada") {
		t.Fatalf("esperava auditoria do registo: %+v", aud.registos)
	}

	// Duplicado → Conflito.
	_, err = uc.Executar(context.Background(), "admin-1", app.DadosNovaAnalise{
		Codigo: "GLIC", Nome: "Glicemia", Unidade: "mg/dL",
	})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("código duplicado devia falhar com Conflito, veio %v", err)
	}
}

func TestListarAnalises(t *testing.T) {
	repo := novoFakeAnalises()
	a, _ := dominio.NovaAnalise("HB", "Hemoglobina", "g/dL", nil, nil)
	_ = repo.Guardar(context.Background(), a)

	out, err := app.NovoCasoListarAnalises(repo).Executar(context.Background())
	if err != nil {
		t.Fatalf("listar análises: %v", err)
	}
	if len(out) != 1 || out[0].Codigo != "HB" {
		t.Fatalf("esperava a análise HB, veio %+v", out)
	}
}
```

- [ ] **Step 5: Correr para confirmar que falha**

Run: `go test ./internal/application/laboratorio/ -v`
Expected: FAIL (compilação: `NovoCasoRegistarAnalise` indefinido).

- [ ] **Step 6: Implementar `analises.go`**

```go
package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarAnalise regista uma análise no catálogo.
type CasoRegistarAnalise struct {
	analises dominio.RepositorioAnalises
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoRegistarAnalise constrói o caso de uso.
func NovoCasoRegistarAnalise(a dominio.RepositorioAnalises, aud Auditor) *CasoRegistarAnalise {
	return &CasoRegistarAnalise{analises: a, auditor: aud, agora: time.Now}
}

// Executar valida, persiste e audita o registo da análise.
func (uc *CasoRegistarAnalise) Executar(ctx context.Context, actor string, dados DadosNovaAnalise) (DetalheAnalise, error) {
	a, err := dominio.NovaAnalise(dados.Codigo, dados.Nome, dados.Unidade, dados.Intervalos, dados.ValoresCriticos)
	if err != nil {
		return DetalheAnalise{}, err
	}
	if err := uc.analises.Guardar(ctx, a); err != nil {
		return DetalheAnalise{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.analise.registada",
		Entidade: "analise", EntidadeID: a.Codigo(), OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheAnalise{}, err
	}
	return paraDetalheAnalise(a), nil
}

// CasoListarAnalises lista o catálogo.
type CasoListarAnalises struct {
	analises dominio.RepositorioAnalises
}

// NovoCasoListarAnalises constrói o caso de uso.
func NovoCasoListarAnalises(a dominio.RepositorioAnalises) *CasoListarAnalises {
	return &CasoListarAnalises{analises: a}
}

// Executar devolve o catálogo de análises.
func (uc *CasoListarAnalises) Executar(ctx context.Context) ([]ResumoAnalise, error) {
	return uc.analises.Listar(ctx)
}
```

- [ ] **Step 7: Correr para confirmar que passa**

Run: `go test ./internal/application/laboratorio/ -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/application/laboratorio/ports.go internal/application/laboratorio/mapa.go internal/application/laboratorio/analises.go internal/application/laboratorio/fakes_test.go internal/application/laboratorio/analises_test.go
git commit -m "$(printf 'feat(laboratorio): portas, DTOs e casos de uso do catalogo de analises\n\nPorta anti-corrupcao LeitorClinico (o Laboratorio nunca importa o dominio Clinico),\nDTOs, mapeadores e registo/listagem do catalogo com auditoria. Fixa os estados\nvisiveis ao medico (VALIDADA/CONCLUIDA).\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 6: Aplicação — emitir e ler requisições

**Files:**
- Create: `internal/application/laboratorio/emitir_requisicao.go`
- Create: `internal/application/laboratorio/requisicoes.go`
- Test: `internal/application/laboratorio/requisicoes_test.go`

**Interfaces:**
- Consumes: `LeitorClinico`, `Auditor` (Task 5); `dominio.RepositorioRequisicoes`, `dominio.RepositorioAnalises`, `dominio.NovaRequisicao`, `dominio.NovoResultado`.
- Produces: `CasoEmitirRequisicao` com `Executar(ctx, actor string, dados DadosEmitirRequisicao) (DetalheRequisicao, error)`; `CasoObterRequisicao` com `Executar(ctx, id string) (DetalheRequisicao, error)`; `CasoListarRequisicoesDoEpisodio` com `Executar(ctx, episodioID string) ([]ResumoRequisicao, error)`.

- [ ] **Step 1: Escrever o teste que falha**

```go
package laboratorio_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// cenario monta os fakes com o catálogo já povoado (HB, GLIC) e a ACL a aceitar.
type cenario struct {
	analises    *fakeAnalises
	requisicoes *fakeRequisicoes
	resultados  *fakeResultados
	leitor      *fakeLeitorClinico
	auditor     *fakeAuditor
}

func novoCenario(t *testing.T) *cenario {
	t.Helper()
	an := novoFakeAnalises()
	for _, a := range []struct{ codigo, nome, unidade string }{
		{"HB", "Hemoglobina", "g/dL"},
		{"GLIC", "Glicemia", "mg/dL"},
	} {
		x, err := dominio.NovaAnalise(a.codigo, a.nome, a.unidade, nil, nil)
		if err != nil {
			t.Fatalf("análise base inválida: %v", err)
		}
		if err := an.Guardar(context.Background(), x); err != nil {
			t.Fatalf("guardar análise base: %v", err)
		}
	}
	res := novoFakeResultados()
	return &cenario{
		analises: an, resultados: res, requisicoes: novoFakeRequisicoes(res),
		leitor:  &fakeLeitorClinico{doenteActivo: true, episodioAberto: true},
		auditor: &fakeAuditor{},
	}
}

func (c *cenario) emitir() *app.CasoEmitirRequisicao {
	return app.NovoCasoEmitirRequisicao(c.requisicoes, c.analises, c.leitor, c.auditor)
}

func dadosEmitir() app.DadosEmitirRequisicao {
	return app.DadosEmitirRequisicao{
		EpisodioID: "ep-1", DoenteID: "doente-1", Prioridade: "ROTINA",
		Itens: []app.ItemPedido{{CodigoAnalise: "HB"}, {CodigoAnalise: "GLIC"}},
	}
}

func TestEmitirRequisicao_CriaUmResultadoPendentePorItem(t *testing.T) {
	c := novoCenario(t)
	out, err := c.emitir().Executar(context.Background(), "med-1", dadosEmitir())
	if err != nil {
		t.Fatalf("emitir requisição: %v", err)
	}
	if out.MedicoRequisitanteID != "med-1" {
		t.Fatalf("o requisitante tem de ser o sujeito autenticado, veio %q", out.MedicoRequisitanteID)
	}
	fila, err := c.resultados.ListarFila(context.Background(), []dominio.EstadoResultado{dominio.ResPendente})
	if err != nil {
		t.Fatalf("listar fila: %v", err)
	}
	if len(fila) != 2 {
		t.Fatalf("esperava 2 resultados PENDENTE (um por item), veio %d", len(fila))
	}
	if !c.auditor.tem("laboratorio.requisicao.emitida") {
		t.Fatalf("esperava auditoria da emissão: %+v", c.auditor.registos)
	}
}

func TestEmitirRequisicao_EpisodioFechado(t *testing.T) {
	c := novoCenario(t)
	c.leitor.episodioAberto = false
	_, err := c.emitir().Executar(context.Background(), "med-1", dadosEmitir())
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("episódio fechado devia falhar com Conflito, veio %v", err)
	}
}

func TestEmitirRequisicao_DoenteInactivo(t *testing.T) {
	c := novoCenario(t)
	c.leitor.doenteActivo = false
	_, err := c.emitir().Executar(context.Background(), "med-1", dadosEmitir())
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("doente inactivo devia falhar com RegraNegocio, veio %v", err)
	}
}

func TestEmitirRequisicao_AnaliseInexistente(t *testing.T) {
	c := novoCenario(t)
	d := dadosEmitir()
	d.Itens = []app.ItemPedido{{CodigoAnalise: "NAOEXISTE"}}
	_, err := c.emitir().Executar(context.Background(), "med-1", d)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("análise inexistente devia falhar com NaoEncontrado, veio %v", err)
	}
}

func TestObterEListarRequisicoes(t *testing.T) {
	c := novoCenario(t)
	criada, err := c.emitir().Executar(context.Background(), "med-1", dadosEmitir())
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	obtida, err := app.NovoCasoObterRequisicao(c.requisicoes).Executar(context.Background(), criada.ID)
	if err != nil {
		t.Fatalf("obter requisição: %v", err)
	}
	if obtida.ID != criada.ID || len(obtida.Itens) != 2 {
		t.Fatalf("requisição obtida não bate certo: %+v", obtida)
	}
	lista, err := app.NovoCasoListarRequisicoesDoEpisodio(c.requisicoes).Executar(context.Background(), "ep-1")
	if err != nil {
		t.Fatalf("listar requisições: %v", err)
	}
	if len(lista) != 1 || lista[0].NumAnalises != 2 {
		t.Fatalf("esperava 1 requisição com 2 análises, veio %+v", lista)
	}
}
```

- [ ] **Step 2: Correr para confirmar que falha**

Run: `go test ./internal/application/laboratorio/ -run 'TestEmitirRequisicao|TestObterEListar' -v`
Expected: FAIL (compilação: `NovoCasoEmitirRequisicao` indefinido).

- [ ] **Step 3: Implementar `emitir_requisicao.go`**

```go
package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoEmitirRequisicao emite uma requisição de análises para um episódio aberto e
// cria um resultado PENDENTE por análise pedida (é o que povoa a fila do laboratório).
type CasoEmitirRequisicao struct {
	requisicoes dominio.RepositorioRequisicoes
	analises    dominio.RepositorioAnalises
	leitor      LeitorClinico
	auditor     Auditor
	agora       func() time.Time
}

// NovoCasoEmitirRequisicao constrói o caso de uso.
func NovoCasoEmitirRequisicao(
	r dominio.RepositorioRequisicoes, a dominio.RepositorioAnalises,
	l LeitorClinico, aud Auditor,
) *CasoEmitirRequisicao {
	return &CasoEmitirRequisicao{requisicoes: r, analises: a, leitor: l, auditor: aud, agora: time.Now}
}

// Executar valida o doente e o episódio (ACL), valida cada código contra o catálogo,
// e grava requisição + resultados numa só transacção. O médico requisitante é o
// actor autenticado — nunca um campo do corpo do pedido.
func (uc *CasoEmitirRequisicao) Executar(ctx context.Context, actor string, dados DadosEmitirRequisicao) (DetalheRequisicao, error) {
	activo, err := uc.leitor.DoenteActivo(ctx, dados.DoenteID)
	if err != nil {
		return DetalheRequisicao{}, err
	}
	if !activo {
		return DetalheRequisicao{}, erros.Novo(erros.CategoriaRegraNegocio,
			"não é possível requisitar análises para um doente inexistente ou inactivo")
	}
	aberto, err := uc.leitor.EpisodioAbertoDoDoente(ctx, dados.EpisodioID, dados.DoenteID)
	if err != nil {
		return DetalheRequisicao{}, err
	}
	if !aberto {
		return DetalheRequisicao{}, erros.Novo(erros.CategoriaConflito,
			"só é possível requisitar análises num episódio aberto do doente")
	}
	prioridade, err := dominio.ParsePrioridade(dados.Prioridade)
	if err != nil {
		return DetalheRequisicao{}, err
	}

	itens := make([]dominio.ItemRequisicao, 0, len(dados.Itens))
	for _, i := range dados.Itens {
		itens = append(itens, dominio.ItemRequisicao{
			CodigoAnalise: i.CodigoAnalise, Observacoes: i.Observacoes,
		})
	}
	req, err := dominio.NovaRequisicao(dominio.DadosNovaRequisicao{
		EpisodioID: dados.EpisodioID, DoenteID: dados.DoenteID,
		MedicoRequisitanteID: actor, Prioridade: prioridade, Itens: itens,
	})
	if err != nil {
		return DetalheRequisicao{}, err
	}

	// Um resultado PENDENTE por item. A unidade é a do catálogo (fonte de verdade):
	// o resultado guarda-a para que a leitura clínica não dependa de o catálogo ser
	// alterado mais tarde. Códigos inexistentes ou inactivos são rejeitados aqui —
	// já normalizados pelo agregado, pelo que a pesquisa é pelo código canónico.
	resultados := make([]*dominio.Resultado, 0, len(req.Itens()))
	for _, item := range req.Itens() {
		analise, err := uc.analises.ObterPorCodigo(ctx, item.CodigoAnalise)
		if err != nil {
			return DetalheRequisicao{}, err
		}
		if !analise.Activo() {
			return DetalheRequisicao{}, erros.Novo(erros.CategoriaValidacao,
				"análise inactiva no catálogo: "+item.CodigoAnalise)
		}
		res, err := dominio.NovoResultado("pendente-de-id", item.CodigoAnalise, analise.Unidade())
		if err != nil {
			return DetalheRequisicao{}, err
		}
		resultados = append(resultados, res)
	}

	id, err := uc.requisicoes.Emitir(ctx, req, resultados)
	if err != nil {
		return DetalheRequisicao{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.requisicao.emitida",
		Entidade: "requisicao", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheRequisicao{}, err
	}
	final, err := uc.requisicoes.ObterPorID(ctx, id)
	if err != nil {
		return DetalheRequisicao{}, err
	}
	return paraDetalheRequisicao(final), nil
}
```

**Nota sobre `"pendente-de-id"`:** o `NovoResultado` exige uma requisição não vazia,
mas o id só existe depois do INSERT. O repositório (Task 8) ignora este valor e usa o
id real da requisição que acabou de inserir, dentro da mesma transacção. O fake faz o
mesmo (`sr.RequisicaoID = id`).

- [ ] **Step 4: Implementar `requisicoes.go`**

```go
package laboratorio

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
)

// CasoObterRequisicao devolve o detalhe de uma requisição.
type CasoObterRequisicao struct {
	requisicoes dominio.RepositorioRequisicoes
}

// NovoCasoObterRequisicao constrói o caso de uso.
func NovoCasoObterRequisicao(r dominio.RepositorioRequisicoes) *CasoObterRequisicao {
	return &CasoObterRequisicao{requisicoes: r}
}

// Executar devolve a requisição ou NaoEncontrado.
func (uc *CasoObterRequisicao) Executar(ctx context.Context, id string) (DetalheRequisicao, error) {
	r, err := uc.requisicoes.ObterPorID(ctx, id)
	if err != nil {
		return DetalheRequisicao{}, err
	}
	return paraDetalheRequisicao(r), nil
}

// CasoListarRequisicoesDoEpisodio lista as requisições de um episódio.
type CasoListarRequisicoesDoEpisodio struct {
	requisicoes dominio.RepositorioRequisicoes
}

// NovoCasoListarRequisicoesDoEpisodio constrói o caso de uso.
func NovoCasoListarRequisicoesDoEpisodio(r dominio.RepositorioRequisicoes) *CasoListarRequisicoesDoEpisodio {
	return &CasoListarRequisicoesDoEpisodio{requisicoes: r}
}

// Executar devolve as requisições do episódio.
func (uc *CasoListarRequisicoesDoEpisodio) Executar(ctx context.Context, episodioID string) ([]ResumoRequisicao, error) {
	return uc.requisicoes.ListarPorEpisodio(ctx, episodioID)
}
```

- [ ] **Step 5: Correr para confirmar que passa**

Run: `go test ./internal/application/laboratorio/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/application/laboratorio/emitir_requisicao.go internal/application/laboratorio/requisicoes.go internal/application/laboratorio/requisicoes_test.go
git commit -m "$(printf 'feat(laboratorio): caso de uso de emissao de requisicao (ACL + catalogo)\n\nValida doente activo e episodio aberto pela ACL, valida cada codigo contra o\ncatalogo e cria um resultado PENDENTE por analise. O medico requisitante e o\nsujeito autenticado. Auditado.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 7: Aplicação — amostras, fila e a regra de visibilidade

**Files:**
- Create: `internal/application/laboratorio/amostras.go`
- Create: `internal/application/laboratorio/resultados.go`
- Test: `internal/application/laboratorio/amostras_test.go`

**Interfaces:**
- Consumes: `dominio.RepositorioResultados`, `Auditor` (Tasks 4/5).
- Produces: `CasoColherAmostra`/`CasoRecusarAmostra`/`CasoSubmeterPreliminar` (cada um com `NovoCaso…` e `Executar(ctx, actor, resultadoID string, …) (DetalheResultado, error)`); `CasoListarFila` com `Executar(ctx, estados []dominio.EstadoResultado) ([]ResumoResultado, error)`; `CasoListarResultadosDoEpisodio` com `Executar(ctx, episodioID string) ([]ResumoResultado, error)`.

- [ ] **Step 1: Escrever o teste que falha**

```go
package laboratorio_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// primeiroPendente emite uma requisição e devolve o id de um resultado PENDENTE.
func primeiroPendente(t *testing.T, c *cenario) string {
	t.Helper()
	if _, err := c.emitir().Executar(context.Background(), "med-1", dadosEmitir()); err != nil {
		t.Fatalf("emitir requisição: %v", err)
	}
	fila, err := c.resultados.ListarFila(context.Background(), []dominio.EstadoResultado{dominio.ResPendente})
	if err != nil || len(fila) == 0 {
		t.Fatalf("esperava resultados na fila, veio (%+v, %v)", fila, err)
	}
	return fila[0].ID
}

func TestColherESubmeter_GravaOSujeitoAutenticado(t *testing.T) {
	c := novoCenario(t)
	id := primeiroPendente(t, c)

	if _, err := app.NovoCasoColherAmostra(c.resultados, c.auditor).
		Executar(context.Background(), "tec-1", id); err != nil {
		t.Fatalf("colher amostra: %v", err)
	}
	out, err := app.NovoCasoSubmeterPreliminar(c.resultados, c.auditor).
		Executar(context.Background(), "tec-1", id, app.DadosSubmeterPreliminar{Valor: "12.5"})
	if err != nil {
		t.Fatalf("submeter preliminar: %v", err)
	}
	if out.Estado != string(dominio.ResProcessada) {
		t.Fatalf("esperava PROCESSADA, veio %s", out.Estado)
	}
	// O submissor é o actor, não um campo do pedido — é contra ele que o Sprint 13
	// comparará o patologista para impor a segregação de funções.
	if out.TecnicoSubmissorID != "tec-1" {
		t.Fatalf("esperava submissor tec-1, veio %q", out.TecnicoSubmissorID)
	}
	if !c.auditor.tem("laboratorio.amostra.colhida") || !c.auditor.tem("laboratorio.resultado.preliminar_submetido") {
		t.Fatalf("esperava auditoria da colheita e da submissão: %+v", c.auditor.registos)
	}
}

func TestSubmeterPreliminar_SemColher(t *testing.T) {
	c := novoCenario(t)
	id := primeiroPendente(t, c)
	_, err := app.NovoCasoSubmeterPreliminar(c.resultados, c.auditor).
		Executar(context.Background(), "tec-1", id, app.DadosSubmeterPreliminar{Valor: "12.5"})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("submeter sem colher devia falhar com Conflito, veio %v", err)
	}
}

func TestRecusarAmostra_ExigeMotivo(t *testing.T) {
	c := novoCenario(t)
	id := primeiroPendente(t, c)
	uc := app.NovoCasoRecusarAmostra(c.resultados, c.auditor)

	if _, err := uc.Executar(context.Background(), "tec-1", id, "  "); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("recusar sem motivo devia falhar com Validacao, veio %v", err)
	}
	out, err := uc.Executar(context.Background(), "tec-1", id, "amostra coagulada")
	if err != nil {
		t.Fatalf("recusar amostra: %v", err)
	}
	if out.Estado != string(dominio.ResRecusada) || out.MotivoRecusa != "amostra coagulada" {
		t.Fatalf("recusa não registada: %+v", out)
	}
	if !c.auditor.tem("laboratorio.amostra.recusada") {
		t.Fatalf("esperava auditoria da recusa: %+v", c.auditor.registos)
	}
}

// TestPreliminarNaoEVisivelAoMedico é o critério de saída do marco: o resultado
// submetido pelo técnico não aparece na leitura clínica; a fila do laboratório vê-o.
func TestPreliminarNaoEVisivelAoMedico(t *testing.T) {
	c := novoCenario(t)
	id := primeiroPendente(t, c)
	if _, err := app.NovoCasoColherAmostra(c.resultados, c.auditor).
		Executar(context.Background(), "tec-1", id); err != nil {
		t.Fatalf("colher: %v", err)
	}
	if _, err := app.NovoCasoSubmeterPreliminar(c.resultados, c.auditor).
		Executar(context.Background(), "tec-1", id, app.DadosSubmeterPreliminar{Valor: "12.5"}); err != nil {
		t.Fatalf("submeter: %v", err)
	}

	// Visão clínica: nada — nenhum resultado está VALIDADA/CONCLUIDA.
	clinica, err := app.NovoCasoListarResultadosDoEpisodio(c.resultados).
		Executar(context.Background(), "ep-1")
	if err != nil {
		t.Fatalf("listar resultados do episódio: %v", err)
	}
	if len(clinica) != 0 {
		t.Fatalf("o preliminar NÃO pode ser visível ao médico, veio %+v", clinica)
	}

	// Fila do laboratório: vê o PROCESSADA.
	fila, err := app.NovoCasoListarFila(c.resultados).
		Executar(context.Background(), []dominio.EstadoResultado{dominio.ResProcessada})
	if err != nil {
		t.Fatalf("listar fila: %v", err)
	}
	if len(fila) != 1 || fila[0].Estado != string(dominio.ResProcessada) {
		t.Fatalf("a fila do laboratório devia ver o preliminar, veio %+v", fila)
	}
}
```

- [ ] **Step 2: Correr para confirmar que falha**

Run: `go test ./internal/application/laboratorio/ -run 'TestColher|TestSubmeter|TestRecusar|TestPreliminar' -v`
Expected: FAIL (compilação: `NovoCasoColherAmostra` indefinido).

- [ ] **Step 3: Implementar `amostras.go`**

```go
package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoColherAmostra regista a colheita da amostra (PENDENTE → COLHIDA).
type CasoColherAmostra struct {
	resultados dominio.RepositorioResultados
	auditor    Auditor
	agora      func() time.Time
}

// NovoCasoColherAmostra constrói o caso de uso.
func NovoCasoColherAmostra(r dominio.RepositorioResultados, a Auditor) *CasoColherAmostra {
	return &CasoColherAmostra{resultados: r, auditor: a, agora: time.Now}
}

// Executar transita o resultado e audita. O técnico é o sujeito autenticado.
func (uc *CasoColherAmostra) Executar(ctx context.Context, actor, resultadoID string) (DetalheResultado, error) {
	res, err := uc.resultados.ObterPorID(ctx, resultadoID)
	if err != nil {
		return DetalheResultado{}, err
	}
	if err := res.ColherAmostra(actor, uc.agora()); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.resultados.Transitar(ctx, res); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.amostra.colhida",
		Entidade: "resultado", EntidadeID: resultadoID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheResultado{}, err
	}
	return paraDetalheResultado(res), nil
}

// CasoRecusarAmostra recusa a amostra por inviabilidade (→ RECUSADA).
type CasoRecusarAmostra struct {
	resultados dominio.RepositorioResultados
	auditor    Auditor
	agora      func() time.Time
}

// NovoCasoRecusarAmostra constrói o caso de uso.
func NovoCasoRecusarAmostra(r dominio.RepositorioResultados, a Auditor) *CasoRecusarAmostra {
	return &CasoRecusarAmostra{resultados: r, auditor: a, agora: time.Now}
}

// Executar recusa a amostra com motivo e audita (o motivo vai no detalhe do registo:
// uma recusa sem razão registada não é auditável nem repetível).
func (uc *CasoRecusarAmostra) Executar(ctx context.Context, actor, resultadoID, motivo string) (DetalheResultado, error) {
	res, err := uc.resultados.ObterPorID(ctx, resultadoID)
	if err != nil {
		return DetalheResultado{}, err
	}
	if err := res.RecusarAmostra(motivo, uc.agora()); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.resultados.Transitar(ctx, res); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.amostra.recusada",
		Entidade: "resultado", EntidadeID: resultadoID, OcorridoEm: uc.agora(),
		Detalhe: "motivo: " + motivo,
	}); err != nil {
		return DetalheResultado{}, err
	}
	return paraDetalheResultado(res), nil
}

// CasoSubmeterPreliminar submete o resultado preliminar (COLHIDA → PROCESSADA).
type CasoSubmeterPreliminar struct {
	resultados dominio.RepositorioResultados
	auditor    Auditor
	agora      func() time.Time
}

// NovoCasoSubmeterPreliminar constrói o caso de uso.
func NovoCasoSubmeterPreliminar(r dominio.RepositorioResultados, a Auditor) *CasoSubmeterPreliminar {
	return &CasoSubmeterPreliminar{resultados: r, auditor: a, agora: time.Now}
}

// Executar submete o preliminar e audita. O submissor gravado é o actor autenticado
// — nunca um campo do corpo: é contra ele que a validação (Sprint 13) compara o
// patologista para impor a segregação de funções.
func (uc *CasoSubmeterPreliminar) Executar(ctx context.Context, actor, resultadoID string, dados DadosSubmeterPreliminar) (DetalheResultado, error) {
	res, err := uc.resultados.ObterPorID(ctx, resultadoID)
	if err != nil {
		return DetalheResultado{}, err
	}
	if err := res.SubmeterPreliminar(actor, dados.Valor, dados.Observacoes, uc.agora()); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.resultados.Transitar(ctx, res); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.resultado.preliminar_submetido",
		Entidade: "resultado", EntidadeID: resultadoID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheResultado{}, err
	}
	return paraDetalheResultado(res), nil
}
```

- [ ] **Step 4: Implementar `resultados.go` (a regra de visibilidade)**

```go
package laboratorio

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
)

// CasoListarFila lista a fila de trabalho do laboratório (técnico e patologista).
// Vê todos os estados: é a fila de quem executa o trabalho.
type CasoListarFila struct {
	resultados dominio.RepositorioResultados
}

// NovoCasoListarFila constrói o caso de uso.
func NovoCasoListarFila(r dominio.RepositorioResultados) *CasoListarFila {
	return &CasoListarFila{resultados: r}
}

// Executar devolve a fila; uma lista de estados vazia devolve todos.
func (uc *CasoListarFila) Executar(ctx context.Context, estados []dominio.EstadoResultado) ([]ResumoResultado, error) {
	return uc.resultados.ListarFila(ctx, estados)
}

// CasoListarResultadosDoEpisodio é a leitura clínica dos resultados de um episódio.
//
// Impõe a regra de visibilidade do marco: o resultado preliminar (PROCESSADA) NÃO é
// visível ao médico — só o que o patologista validou. A regra vive aqui, e não no
// RBAC de rota, porque o RBAC não chegaria: o médico tem de poder ver os validados,
// pelo que a distinção é pelo estado, não pelo papel. Enquanto a validação não
// existir (Sprint 13), esta listagem devolve vazio — e é isso que se espera.
type CasoListarResultadosDoEpisodio struct {
	resultados dominio.RepositorioResultados
}

// NovoCasoListarResultadosDoEpisodio constrói o caso de uso.
func NovoCasoListarResultadosDoEpisodio(r dominio.RepositorioResultados) *CasoListarResultadosDoEpisodio {
	return &CasoListarResultadosDoEpisodio{resultados: r}
}

// Executar devolve apenas os resultados visíveis ao médico.
func (uc *CasoListarResultadosDoEpisodio) Executar(ctx context.Context, episodioID string) ([]ResumoResultado, error) {
	return uc.resultados.ListarPorEpisodio(ctx, episodioID, EstadosVisiveisAoMedico)
}
```

- [ ] **Step 5: Correr para confirmar que passa**

Run: `go test ./internal/application/laboratorio/ -v`
Expected: PASS (todos, incluindo `TestPreliminarNaoEVisivelAoMedico`).

- [ ] **Step 6: Commit**

```bash
git add internal/application/laboratorio/amostras.go internal/application/laboratorio/resultados.go internal/application/laboratorio/amostras_test.go
git commit -m "$(printf 'feat(laboratorio): colheita, recusa e submissao do preliminar + visibilidade\n\nO tecnico gravado e sempre o sujeito autenticado (base da segregacao do Sprint 13).\nA leitura clinica so devolve VALIDADA/CONCLUIDA: o preliminar nao e visivel ao\nmedico. Escritas auditadas.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 8: Adaptadores — repositórios pgx

**Files:**
- Create: `internal/adapters/pgrepo/analises_repo.go`
- Create: `internal/adapters/pgrepo/requisicoes_repo.go`
- Create: `internal/adapters/pgrepo/resultados_repo.go`

**Interfaces:**
- Consumes: as portas do domínio (Tasks 2/3/4).
- Produces: `NovoRepositorioAnalises(pool *pgxpool.Pool) *RepositorioAnalises`, `NovoRepositorioRequisicoes(pool) *RepositorioRequisicoes`, `NovoRepositorioResultados(pool) *RepositorioResultados`.

O pgrepo está excluído do gate unitário de cobertura — é coberto pelos testes de
integração da Task 11. Não escrever testes unitários com base de dados a fingir.

- [ ] **Step 1: Implementar `analises_repo.go`**

```go
package pgrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioAnalises implementa dominio.RepositorioAnalises com pgx.
type RepositorioAnalises struct {
	pool *pgxpool.Pool
}

// NovoRepositorioAnalises constrói o repositório sobre o pool pgx.
func NovoRepositorioAnalises(pool *pgxpool.Pool) *RepositorioAnalises {
	return &RepositorioAnalises{pool: pool}
}

// Guardar insere a análise. Código duplicado → Conflito (violação de PK).
func (r *RepositorioAnalises) Guardar(ctx context.Context, a *dominio.Analise) error {
	s := a.Snapshot()
	intervalos, err := json.Marshal(naoNil(s.Intervalos))
	if err != nil {
		return fmt.Errorf("serializar intervalos de referência: %w", err)
	}
	criticos, err := json.Marshal(naoNilCriticos(s.ValoresCriticos))
	if err != nil {
		return fmt.Errorf("serializar valores críticos: %w", err)
	}
	const q = `
INSERT INTO laboratorio.analises (codigo, nome, unidade, intervalos_referencia, valores_criticos, activo)
VALUES ($1,$2,$3,$4,$5,$6)`
	if _, err := r.pool.Exec(ctx, q, s.Codigo, s.Nome, s.Unidade, intervalos, criticos, s.Activo); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return erros.Novo(erros.CategoriaConflito, "já existe uma análise com este código")
		}
		return fmt.Errorf("inserir análise: %w", err)
	}
	return nil
}

// naoNil garante `[]` em vez de `null` no jsonb (a coluna é NOT NULL).
func naoNil(v []dominio.IntervaloReferencia) []dominio.IntervaloReferencia {
	if v == nil {
		return []dominio.IntervaloReferencia{}
	}
	return v
}

func naoNilCriticos(v []dominio.ValorCritico) []dominio.ValorCritico {
	if v == nil {
		return []dominio.ValorCritico{}
	}
	return v
}

// ObterPorCodigo reconstrói a análise. NaoEncontrado se não existir.
func (r *RepositorioAnalises) ObterPorCodigo(ctx context.Context, codigo string) (*dominio.Analise, error) {
	const q = `
SELECT codigo, nome, unidade, intervalos_referencia, valores_criticos, activo, criado_em
FROM laboratorio.analises WHERE codigo=$1`
	var s dominio.SnapshotAnalise
	var intervalos, criticos []byte
	err := r.pool.QueryRow(ctx, q, codigo).Scan(&s.Codigo, &s.Nome, &s.Unidade,
		&intervalos, &criticos, &s.Activo, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "análise não encontrada: "+codigo)
		}
		return nil, fmt.Errorf("obter análise: %w", err)
	}
	if err := json.Unmarshal(intervalos, &s.Intervalos); err != nil {
		return nil, fmt.Errorf("ler intervalos de referência: %w", err)
	}
	if err := json.Unmarshal(criticos, &s.ValoresCriticos); err != nil {
		return nil, fmt.Errorf("ler valores críticos: %w", err)
	}
	return dominio.ReconstruirAnalise(s), nil
}

// Listar devolve o catálogo por ordem de código.
func (r *RepositorioAnalises) Listar(ctx context.Context) ([]dominio.ResumoAnalise, error) {
	const q = `SELECT codigo, nome, unidade, activo FROM laboratorio.analises ORDER BY codigo`
	linhas, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("listar análises: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoAnalise{}
	for linhas.Next() {
		var a dominio.ResumoAnalise
		if err := linhas.Scan(&a.Codigo, &a.Nome, &a.Unidade, &a.Activo); err != nil {
			return nil, fmt.Errorf("ler análise: %w", err)
		}
		out = append(out, a)
	}
	return out, linhas.Err()
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioAnalises = (*RepositorioAnalises)(nil)
```

- [ ] **Step 2: Implementar `requisicoes_repo.go`**

```go
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioRequisicoes implementa dominio.RepositorioRequisicoes com pgx.
type RepositorioRequisicoes struct {
	pool *pgxpool.Pool
}

// NovoRepositorioRequisicoes constrói o repositório sobre o pool pgx.
func NovoRepositorioRequisicoes(pool *pgxpool.Pool) *RepositorioRequisicoes {
	return &RepositorioRequisicoes{pool: pool}
}

// Emitir grava a requisição, os seus itens e os resultados PENDENTE numa única
// transacção. Se qualquer INSERT falhar, nada fica escrito: uma requisição sem
// resultados nunca apareceria na fila do laboratório e ficaria invisível para todos.
// O RequisicaoID dos resultados vem do id acabado de gerar — o valor que o caso de
// uso lá pôs é ignorado (na altura ainda não havia id).
func (r *RepositorioRequisicoes) Emitir(ctx context.Context, req *dominio.RequisicaoLab, resultados []*dominio.Resultado) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção da requisição: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	s := req.Snapshot()
	const qReq = `
INSERT INTO laboratorio.requisicoes (episodio_id, doente_id, medico_requisitante_id, prioridade, estado)
VALUES ($1,$2,$3,$4,$5) RETURNING id::text`
	var id string
	if err := tx.QueryRow(ctx, qReq, s.EpisodioID, s.DoenteID, s.MedicoRequisitanteID,
		string(s.Prioridade), string(s.Estado)).Scan(&id); err != nil {
		return "", fmt.Errorf("inserir requisição: %w", err)
	}

	const qItem = `
INSERT INTO laboratorio.itens_requisicao (requisicao_id, codigo_analise, observacoes)
VALUES ($1,$2,NULLIF($3,''))`
	for _, item := range s.Itens {
		if _, err := tx.Exec(ctx, qItem, id, item.CodigoAnalise, item.Observacoes); err != nil {
			return "", fmt.Errorf("inserir item da requisição: %w", err)
		}
	}

	const qRes = `
INSERT INTO laboratorio.resultados (requisicao_id, codigo_analise, unidade, estado)
VALUES ($1,$2,$3,$4)`
	for _, res := range resultados {
		sr := res.Snapshot()
		if _, err := tx.Exec(ctx, qRes, id, sr.CodigoAnalise, sr.Unidade, string(sr.Estado)); err != nil {
			return "", fmt.Errorf("inserir resultado pendente: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar a emissão da requisição: %w", err)
	}
	return id, nil
}

// ObterPorID reconstrói a requisição com os seus itens.
func (r *RepositorioRequisicoes) ObterPorID(ctx context.Context, id string) (*dominio.RequisicaoLab, error) {
	const q = `
SELECT id::text, episodio_id::text, doente_id::text, medico_requisitante_id::text,
       prioridade, estado, criado_em
FROM laboratorio.requisicoes WHERE id=$1`
	var s dominio.SnapshotRequisicao
	var prioridade, estado string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.EpisodioID, &s.DoenteID,
		&s.MedicoRequisitanteID, &prioridade, &estado, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "requisição não encontrada")
		}
		return nil, fmt.Errorf("obter requisição: %w", err)
	}
	s.Prioridade = dominio.Prioridade(prioridade)
	s.Estado = dominio.EstadoRequisicao(estado)

	const qItens = `
SELECT codigo_analise, COALESCE(observacoes,'')
FROM laboratorio.itens_requisicao WHERE requisicao_id=$1 ORDER BY codigo_analise`
	linhas, err := r.pool.Query(ctx, qItens, id)
	if err != nil {
		return nil, fmt.Errorf("listar itens da requisição: %w", err)
	}
	defer linhas.Close()
	for linhas.Next() {
		var it dominio.ItemRequisicao
		if err := linhas.Scan(&it.CodigoAnalise, &it.Observacoes); err != nil {
			return nil, fmt.Errorf("ler item da requisição: %w", err)
		}
		s.Itens = append(s.Itens, it)
	}
	if err := linhas.Err(); err != nil {
		return nil, fmt.Errorf("ler itens da requisição: %w", err)
	}
	return dominio.ReconstruirRequisicao(s), nil
}

// ListarPorEpisodio devolve as requisições do episódio (mais recentes primeiro).
func (r *RepositorioRequisicoes) ListarPorEpisodio(ctx context.Context, episodioID string) ([]dominio.ResumoRequisicao, error) {
	const q = `
SELECT r.id::text, r.episodio_id::text, r.doente_id::text, r.prioridade, r.estado,
       (SELECT count(*) FROM laboratorio.itens_requisicao i WHERE i.requisicao_id = r.id),
       r.criado_em
FROM laboratorio.requisicoes r
WHERE r.episodio_id=$1
ORDER BY r.criado_em DESC`
	linhas, err := r.pool.Query(ctx, q, episodioID)
	if err != nil {
		return nil, fmt.Errorf("listar requisições: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoRequisicao{}
	for linhas.Next() {
		var rr dominio.ResumoRequisicao
		if err := linhas.Scan(&rr.ID, &rr.EpisodioID, &rr.DoenteID, &rr.Prioridade,
			&rr.Estado, &rr.NumAnalises, &rr.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler requisição: %w", err)
		}
		out = append(out, rr)
	}
	return out, linhas.Err()
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioRequisicoes = (*RepositorioRequisicoes)(nil)
```

- [ ] **Step 3: Implementar `resultados_repo.go`**

```go
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioResultados implementa dominio.RepositorioResultados com pgx.
type RepositorioResultados struct {
	pool *pgxpool.Pool
}

// NovoRepositorioResultados constrói o repositório sobre o pool pgx.
func NovoRepositorioResultados(pool *pgxpool.Pool) *RepositorioResultados {
	return &RepositorioResultados{pool: pool}
}

// ObterPorID reconstrói o resultado. NaoEncontrado se não existir.
func (r *RepositorioResultados) ObterPorID(ctx context.Context, id string) (*dominio.Resultado, error) {
	const q = `
SELECT id::text, requisicao_id::text, codigo_analise, COALESCE(valor,''), unidade,
       COALESCE(observacoes,''), COALESCE(motivo_recusa,''), estado,
       COALESCE(tecnico_colheita_id::text,''), COALESCE(tecnico_submissor_id::text,''),
       COALESCE(patologista_validador_id::text,''),
       colhida_em, submetida_em, validada_em, valor_critico, criado_em
FROM laboratorio.resultados WHERE id=$1`
	var s dominio.SnapshotResultado
	var estado string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.RequisicaoID, &s.CodigoAnalise,
		&s.Valor, &s.Unidade, &s.Observacoes, &s.MotivoRecusa, &estado,
		&s.TecnicoColheitaID, &s.TecnicoSubmissorID, &s.PatologistaValidadorID,
		&s.ColhidaEm, &s.SubmetidaEm, &s.ValidadaEm, &s.ValorCritico, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
		}
		return nil, fmt.Errorf("obter resultado: %w", err)
	}
	s.Estado = dominio.EstadoResultado(estado)
	return dominio.ReconstruirResultado(s), nil
}

// Transitar aplica a transição de estado com guarda compare-and-set: o UPDATE só se
// aplica se a linha ainda estiver no estado com que o agregado foi lido. Duas
// transições concorrentes a partir do mesmo estado (dois técnicos a colher a mesma
// amostra, duplo-clique em submeter) passam ambas as guardas do domínio — mas só uma
// escreve; a outra perde a corrida e recebe Conflito.
//
// Escreve apenas as colunas que uma transição altera — nunca as de identidade do
// resultado (requisicao_id, codigo_analise, unidade).
func (r *RepositorioResultados) Transitar(ctx context.Context, res *dominio.Resultado) error {
	s := res.Snapshot()
	const q = `
UPDATE laboratorio.resultados SET
    estado=$2, valor=NULLIF($3,''), observacoes=NULLIF($4,''), motivo_recusa=NULLIF($5,''),
    tecnico_colheita_id=NULLIF($6,'')::uuid, tecnico_submissor_id=NULLIF($7,'')::uuid,
    colhida_em=$8, submetida_em=$9
WHERE id=$1 AND estado=$10`
	ct, err := r.pool.Exec(ctx, q, s.ID, string(s.Estado), s.Valor, s.Observacoes, s.MotivoRecusa,
		s.TecnicoColheitaID, s.TecnicoSubmissorID, s.ColhidaEm, s.SubmetidaEm,
		string(s.EstadoAnterior))
	if err != nil {
		return fmt.Errorf("actualizar resultado: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return r.erroTransicaoFalhada(ctx, s.ID)
	}
	return nil
}

// erroTransicaoFalhada distingue "a linha não existe" (NaoEncontrado/404) de "a linha
// existe mas já não está no estado esperado" (Conflito/409, corrida perdida).
func (r *RepositorioResultados) erroTransicaoFalhada(ctx context.Context, id string) error {
	const q = `SELECT EXISTS (SELECT 1 FROM laboratorio.resultados WHERE id=$1)`
	var existe bool
	if err := r.pool.QueryRow(ctx, q, id).Scan(&existe); err != nil {
		return fmt.Errorf("verificar resultado: %w", err)
	}
	if !existe {
		return erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
	}
	return erros.Novo(erros.CategoriaConflito,
		"o estado do resultado mudou entretanto; recarregue o resultado e repita a operação")
}

// estadosTexto converte a lista de estados para texto (nil = todos).
func estadosTexto(estados []dominio.EstadoResultado) []string {
	if len(estados) == 0 {
		return nil
	}
	out := make([]string, 0, len(estados))
	for _, e := range estados {
		out = append(out, string(e))
	}
	return out
}

// ListarFila devolve a fila de trabalho do laboratório. Lista de estados vazia = todos.
func (r *RepositorioResultados) ListarFila(ctx context.Context, estados []dominio.EstadoResultado) ([]dominio.ResumoResultado, error) {
	const q = `
SELECT res.id::text, res.requisicao_id::text, req.episodio_id::text, res.codigo_analise,
       COALESCE(res.valor,''), res.unidade, res.estado, res.valor_critico,
       res.colhida_em, res.submetida_em, res.criado_em
FROM laboratorio.resultados res
JOIN laboratorio.requisicoes req ON req.id = res.requisicao_id
WHERE ($1::text[] IS NULL OR res.estado = ANY($1))
ORDER BY res.criado_em`
	return r.consultar(ctx, q, estadosTexto(estados))
}

// ListarPorEpisodio devolve os resultados de um episódio nos estados dados.
func (r *RepositorioResultados) ListarPorEpisodio(ctx context.Context, episodioID string, estados []dominio.EstadoResultado) ([]dominio.ResumoResultado, error) {
	const q = `
SELECT res.id::text, res.requisicao_id::text, req.episodio_id::text, res.codigo_analise,
       COALESCE(res.valor,''), res.unidade, res.estado, res.valor_critico,
       res.colhida_em, res.submetida_em, res.criado_em
FROM laboratorio.resultados res
JOIN laboratorio.requisicoes req ON req.id = res.requisicao_id
WHERE req.episodio_id = $2 AND ($1::text[] IS NULL OR res.estado = ANY($1))
ORDER BY res.criado_em`
	return r.consultar(ctx, q, estadosTexto(estados), episodioID)
}

// consultar corre uma das duas queries de listagem e mapeia as linhas.
func (r *RepositorioResultados) consultar(ctx context.Context, q string, args ...any) ([]dominio.ResumoResultado, error) {
	linhas, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listar resultados: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoResultado{}
	for linhas.Next() {
		var rr dominio.ResumoResultado
		if err := linhas.Scan(&rr.ID, &rr.RequisicaoID, &rr.EpisodioID, &rr.CodigoAnalise,
			&rr.Valor, &rr.Unidade, &rr.Estado, &rr.ValorCritico,
			&rr.ColhidaEm, &rr.SubmetidaEm, &rr.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler resultado: %w", err)
		}
		out = append(out, rr)
	}
	return out, linhas.Err()
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioResultados = (*RepositorioResultados)(nil)
```

- [ ] **Step 4: Verificar que compila e o vet passa**

Run: `go build ./... && go vet ./internal/adapters/pgrepo/ && gofmt -l internal/adapters/pgrepo/`
Expected: sem saída (o comportamento real é verificado na Task 11, contra Postgres).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/pgrepo/analises_repo.go internal/adapters/pgrepo/requisicoes_repo.go internal/adapters/pgrepo/resultados_repo.go
git commit -m "$(printf 'feat(laboratorio): repositorios pgx do catalogo, requisicoes e resultados\n\nEmissao transaccional (requisicao + itens + resultados pendentes) e guarda\ncompare-and-set no UPDATE de transicao, que so escreve as colunas da transicao.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 9: Adaptadores — ACL Laboratório→Clínico + handler HTTP

**Files:**
- Create: `internal/adapters/laboratorio/leitor_clinico.go`
- Create: `internal/adapters/http/laboratorio_handler.go`
- Test: `internal/adapters/http/laboratorio_test.go`

**Interfaces:**
- Consumes: `applaboratorio.LeitorClinico` (Task 5); `clinico.RepositorioDoentes`, `clinico.RepositorioEpisodios` (BC Clínico, já existentes); os casos de uso das Tasks 5/6/7.
- Produces: `NovoLeitorClinico(doentes clinico.RepositorioDoentes, episodios clinico.RepositorioEpisodios) *LeitorClinico`; `NovoLaboratorioHandler(...) *LaboratorioHandler`; `RegistarLaboratorio(r gin.IRouter, h *LaboratorioHandler, protecao ...gin.HandlerFunc)`.

- [ ] **Step 1: Implementar a ACL (`internal/adapters/laboratorio/leitor_clinico.go`)**

Espelha `internal/adapters/farmacia/leitor_clinico.go`: é **aqui**, na camada de
adaptadores, que os dois domínios se tocam — o domínio e a aplicação do Laboratório
nunca importam o Clínico.

```go
// Package laboratorio (adaptadores) contém adaptadores de saída do BC Laboratório.
// Camada 3 — Adaptadores.
package laboratorio

import (
	"context"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// LeitorClinico implementa a porta anti-corrupção applaboratorio.LeitorClinico,
// lendo o BC Clínico através dos seus repositórios e traduzindo o que interessa ao
// Laboratório (duas perguntas booleanas) — sem deixar passar tipos do Clínico.
type LeitorClinico struct {
	doentes   clinico.RepositorioDoentes
	episodios clinico.RepositorioEpisodios
}

// NovoLeitorClinico constrói o adaptador sobre os repositórios clínicos.
func NovoLeitorClinico(doentes clinico.RepositorioDoentes, episodios clinico.RepositorioEpisodios) *LeitorClinico {
	return &LeitorClinico{doentes: doentes, episodios: episodios}
}

// DoenteActivo indica se o doente existe e está activo. Um doente inexistente
// devolve false sem erro — para o Laboratório, "não existe" e "não pode" são a
// mesma resposta.
func (l *LeitorClinico) DoenteActivo(ctx context.Context, doenteID string) (bool, error) {
	d, err := l.doentes.ObterPorID(ctx, doenteID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return false, nil
		}
		return false, err
	}
	return d.Estado() == clinico.EstadoActivo, nil
}

// EpisodioAbertoDoDoente indica se o episódio existe, pertence ao doente e está
// ABERTO. Requisitar análises para um episódio fechado deixaria resultados órfãos na
// fila, sem consulta onde os devolver.
func (l *LeitorClinico) EpisodioAbertoDoDoente(ctx context.Context, episodioID, doenteID string) (bool, error) {
	ep, err := l.episodios.ObterPorID(ctx, episodioID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return false, nil
		}
		return false, err
	}
	return ep.DoenteID() == doenteID && ep.Estado() == clinico.EstadoEpisodioAberto, nil
}

// Garantia de conformidade com a porta.
var _ applaboratorio.LeitorClinico = (*LeitorClinico)(nil)
```

- [ ] **Step 2: Escrever o teste do handler (`internal/adapters/http/laboratorio_test.go`)**

Segue o padrão de `cirurgia_test.go`: servidor Gin em modo de teste, casos de uso
substituídos por duplos, e verificação dos códigos HTTP e do RBAC.

```go
package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// duploEmitir devolve a requisição que lhe pedirem, guardando o actor recebido.
type duploEmitir struct {
	actorRecebido string
	erro          error
}

func (d *duploEmitir) Executar(_ context.Context, actor string, _ applaboratorio.DadosEmitirRequisicao) (applaboratorio.DetalheRequisicao, error) {
	d.actorRecebido = actor
	if d.erro != nil {
		return applaboratorio.DetalheRequisicao{}, d.erro
	}
	return applaboratorio.DetalheRequisicao{ID: "req-1", MedicoRequisitanteID: actor}, nil
}

type duploSubmeter struct {
	actorRecebido string
	valorRecebido string
}

func (d *duploSubmeter) Executar(_ context.Context, actor, _ string, dados applaboratorio.DadosSubmeterPreliminar) (applaboratorio.DetalheResultado, error) {
	d.actorRecebido = actor
	d.valorRecebido = dados.Valor
	return applaboratorio.DetalheResultado{ID: "res-1", Estado: string(dominio.ResProcessada), TecnicoSubmissorID: actor}, nil
}

// Os restantes casos de uso não são exercitados por estes testes: duplos mínimos.
type duploColher struct{}

func (duploColher) Executar(_ context.Context, actor, id string) (applaboratorio.DetalheResultado, error) {
	return applaboratorio.DetalheResultado{ID: id, Estado: string(dominio.ResColhida)}, nil
}

type duploRecusar struct{}

func (duploRecusar) Executar(_ context.Context, _, id, motivo string) (applaboratorio.DetalheResultado, error) {
	if motivo == "" {
		return applaboratorio.DetalheResultado{}, erros.Novo(erros.CategoriaValidacao, "motivo em falta")
	}
	return applaboratorio.DetalheResultado{ID: id, Estado: string(dominio.ResRecusada)}, nil
}

type duploRegistarAnalise struct{}

func (duploRegistarAnalise) Executar(_ context.Context, _ string, d applaboratorio.DadosNovaAnalise) (applaboratorio.DetalheAnalise, error) {
	return applaboratorio.DetalheAnalise{Codigo: d.Codigo}, nil
}

type duploListarAnalises struct{}

func (duploListarAnalises) Executar(_ context.Context) ([]applaboratorio.ResumoAnalise, error) {
	return []applaboratorio.ResumoAnalise{{Codigo: "HB"}}, nil
}

type duploObterRequisicao struct{}

func (duploObterRequisicao) Executar(_ context.Context, id string) (applaboratorio.DetalheRequisicao, error) {
	return applaboratorio.DetalheRequisicao{ID: id}, nil
}

type duploListarRequisicoes struct{}

func (duploListarRequisicoes) Executar(_ context.Context, _ string) ([]applaboratorio.ResumoRequisicao, error) {
	return []applaboratorio.ResumoRequisicao{}, nil
}

type duploListarFila struct{}

func (duploListarFila) Executar(_ context.Context, _ []dominio.EstadoResultado) ([]applaboratorio.ResumoResultado, error) {
	return []applaboratorio.ResumoResultado{{ID: "res-1", Estado: string(dominio.ResPendente)}}, nil
}

type duploListarResultadosEpisodio struct{}

func (duploListarResultadosEpisodio) Executar(_ context.Context, _ string) ([]applaboratorio.ResumoResultado, error) {
	return []applaboratorio.ResumoResultado{}, nil
}

// routerLab monta o router com os duplos e uma sessão fixa. Usa o `fakeAuth` que já
// existe no pacote de testes (ver `cirurgia_test.go`) — não criar outro.
func routerLab(t *testing.T, emitir *duploEmitir, submeter *duploSubmeter, sessao identidade.Sessao) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := adhttp.NovoLaboratorioHandler(
		duploRegistarAnalise{}, duploListarAnalises{},
		emitir, duploObterRequisicao{}, duploListarRequisicoes{},
		duploColher{}, duploRecusar{}, submeter,
		duploListarFila{}, duploListarResultadosEpisodio{},
	)
	adhttp.RegistarLaboratorio(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

// sessaoDe constrói uma sessão de teste com um papel.
func sessaoDe(sujeito string, papel identidade.Papel) identidade.Sessao {
	return identidade.Sessao{Sujeito: sujeito, Papeis: []identidade.Papel{papel}}
}

func TestEmitirRequisicao_UsaOSujeitoAutenticado(t *testing.T) {
	emitir := &duploEmitir{}
	r := routerLab(t, emitir, &duploSubmeter{}, sessaoDe("med-99", identidade.PapelMedico))

	corpo, _ := json.Marshal(map[string]any{
		"doente_id":  "doente-1",
		"prioridade": "ROTINA",
		"itens":      []map[string]string{{"codigo_analise": "HB"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/episodios/ep-1/requisicoes", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	// O requisitante nunca vem do corpo: vem da sessão.
	if emitir.actorRecebido != "med-99" {
		t.Fatalf("esperava o actor da sessão (med-99), veio %q", emitir.actorRecebido)
	}
}

func TestSubmeterPreliminar_SoTecnicoLab(t *testing.T) {
	// Um médico não submete resultados: 403.
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoDe("med-1", identidade.PapelMedico))
	corpo, _ := json.Marshal(map[string]string{"valor": "12.5"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/preliminar", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403 para o Medico, veio %d", w.Code)
	}

	// O técnico submete, e o submissor é o sujeito da sessão.
	submeter := &duploSubmeter{}
	rt := routerLab(t, &duploEmitir{}, submeter, sessaoDe("tec-7", identidade.PapelTecnicoLab))
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/v1/resultados/res-1/preliminar", bytes.NewReader(corpo))
	req2.Header.Set("Content-Type", "application/json")
	rt.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("esperava 200 para o TecnicoLab, veio %d (%s)", w2.Code, w2.Body.String())
	}
	if submeter.actorRecebido != "tec-7" || submeter.valorRecebido != "12.5" {
		t.Fatalf("submissão não usou a sessão/corpo esperados: actor=%q valor=%q",
			submeter.actorRecebido, submeter.valorRecebido)
	}
}

func TestSubmeterPreliminar_CorpoMalformado(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, sessaoDe("tec-1", identidade.PapelTecnicoLab))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/preliminar", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}
```

**Nota:** `fakeAuth` já existe no pacote `http_test` (usado por `cirurgia_test.go`,
que monta o router com `adhttp.Auth(fakeAuth{sessao: identidade.Sessao{Sujeito: "m1",
Papeis: []identidade.Papel{identidade.PapelMedico}}})`). Reutilizar esse tipo — não
criar outro nem duplicar o helper.

- [ ] **Step 3: Correr para confirmar que falha**

Run: `go test ./internal/adapters/http/ -run 'TestEmitirRequisicao|TestSubmeterPreliminar' -v`
Expected: FAIL (compilação: `NovoLaboratorioHandler` indefinido).

- [ ] **Step 4: Implementar `internal/adapters/http/laboratorio_handler.go`**

```go
package http

import (
	"context"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	dominiolab "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso do BC Laboratório.
type (
	// ServicoRegistarAnalise regista uma análise no catálogo.
	ServicoRegistarAnalise interface {
		Executar(ctx context.Context, actor string, dados applaboratorio.DadosNovaAnalise) (applaboratorio.DetalheAnalise, error)
	}
	// ServicoListarAnalises lista o catálogo.
	ServicoListarAnalises interface {
		Executar(ctx context.Context) ([]applaboratorio.ResumoAnalise, error)
	}
	// ServicoEmitirRequisicao emite uma requisição de análises.
	ServicoEmitirRequisicao interface {
		Executar(ctx context.Context, actor string, dados applaboratorio.DadosEmitirRequisicao) (applaboratorio.DetalheRequisicao, error)
	}
	// ServicoObterRequisicao devolve o detalhe de uma requisição.
	ServicoObterRequisicao interface {
		Executar(ctx context.Context, id string) (applaboratorio.DetalheRequisicao, error)
	}
	// ServicoListarRequisicoes lista as requisições de um episódio.
	ServicoListarRequisicoes interface {
		Executar(ctx context.Context, episodioID string) ([]applaboratorio.ResumoRequisicao, error)
	}
	// ServicoColherAmostra regista a colheita.
	ServicoColherAmostra interface {
		Executar(ctx context.Context, actor, resultadoID string) (applaboratorio.DetalheResultado, error)
	}
	// ServicoRecusarAmostra recusa a amostra.
	ServicoRecusarAmostra interface {
		Executar(ctx context.Context, actor, resultadoID, motivo string) (applaboratorio.DetalheResultado, error)
	}
	// ServicoSubmeterPreliminar submete o resultado preliminar.
	ServicoSubmeterPreliminar interface {
		Executar(ctx context.Context, actor, resultadoID string, dados applaboratorio.DadosSubmeterPreliminar) (applaboratorio.DetalheResultado, error)
	}
	// ServicoListarFila lista a fila de trabalho do laboratório.
	ServicoListarFila interface {
		Executar(ctx context.Context, estados []dominiolab.EstadoResultado) ([]applaboratorio.ResumoResultado, error)
	}
	// ServicoListarResultadosDoEpisodio é a leitura clínica dos resultados.
	ServicoListarResultadosDoEpisodio interface {
		Executar(ctx context.Context, episodioID string) ([]applaboratorio.ResumoResultado, error)
	}
)

// LaboratorioHandler expõe os endpoints HTTP do BC Laboratório.
type LaboratorioHandler struct {
	registarAnalise    ServicoRegistarAnalise
	listarAnalises     ServicoListarAnalises
	emitir             ServicoEmitirRequisicao
	obterRequisicao    ServicoObterRequisicao
	listarRequisicoes  ServicoListarRequisicoes
	colher             ServicoColherAmostra
	recusar            ServicoRecusarAmostra
	submeter           ServicoSubmeterPreliminar
	listarFila         ServicoListarFila
	resultadosEpisodio ServicoListarResultadosDoEpisodio
}

// NovoLaboratorioHandler constrói o handler.
func NovoLaboratorioHandler(
	registarAnalise ServicoRegistarAnalise, listarAnalises ServicoListarAnalises,
	emitir ServicoEmitirRequisicao, obterRequisicao ServicoObterRequisicao,
	listarRequisicoes ServicoListarRequisicoes, colher ServicoColherAmostra,
	recusar ServicoRecusarAmostra, submeter ServicoSubmeterPreliminar,
	listarFila ServicoListarFila, resultadosEpisodio ServicoListarResultadosDoEpisodio,
) *LaboratorioHandler {
	return &LaboratorioHandler{
		registarAnalise: registarAnalise, listarAnalises: listarAnalises,
		emitir: emitir, obterRequisicao: obterRequisicao, listarRequisicoes: listarRequisicoes,
		colher: colher, recusar: recusar, submeter: submeter,
		listarFila: listarFila, resultadosEpisodio: resultadosEpisodio,
	}
}

// RegistarLaboratorio regista as rotas, aplicando `protecao` e o RBAC por rota.
//
// A separação das rotas é o que dá corpo à regra de visibilidade: a fila do
// laboratório (todos os estados) é do Técnico/Patologista; os resultados do episódio
// (só os validados, filtrados na aplicação) são do pessoal clínico.
func RegistarLaboratorio(r gin.IRouter, h *LaboratorioHandler, protecao ...gin.HandlerFunc) {
	leituraClinica := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelDirector,
		dominio.PapelTecnicoLab, dominio.PapelPatologista, dominio.PapelAdmin)
	catalogoEscrita := RBAC(dominio.PapelAdmin, dominio.PapelDirector)
	soMedico := RBAC(dominio.PapelMedico)
	soTecnico := RBAC(dominio.PapelTecnicoLab)
	filaLab := RBAC(dominio.PapelTecnicoLab, dominio.PapelPatologista, dominio.PapelDirector)

	ga := r.Group("/api/v1/analises")
	ga.Use(protecao...)
	ga.POST("", catalogoEscrita, h.registarAnaliseHTTP)
	ga.GET("", leituraClinica, h.listarAnalisesHTTP)

	ge := r.Group("/api/v1/episodios")
	ge.Use(protecao...)
	ge.POST("/:eid/requisicoes", soMedico, h.emitirRequisicaoHTTP)
	ge.GET("/:eid/requisicoes", leituraClinica, h.listarRequisicoesHTTP)
	ge.GET("/:eid/resultados", leituraClinica, h.listarResultadosDoEpisodioHTTP)

	gr := r.Group("/api/v1/requisicoes")
	gr.Use(protecao...)
	gr.GET("/:rid", leituraClinica, h.obterRequisicaoHTTP)

	gl := r.Group("/api/v1/laboratorio")
	gl.Use(protecao...)
	gl.GET("/fila", filaLab, h.listarFilaHTTP)

	gres := r.Group("/api/v1/resultados")
	gres.Use(protecao...)
	gres.POST("/:rid/colheita", soTecnico, h.colherAmostraHTTP)
	gres.POST("/:rid/recusa", soTecnico, h.recusarAmostraHTTP)
	gres.POST("/:rid/preliminar", soTecnico, h.submeterPreliminarHTTP)
}

type corpoEmitirRequisicao struct {
	DoenteID   string                     `json:"doente_id"`
	Prioridade string                     `json:"prioridade"`
	Itens      []applaboratorio.ItemPedido `json:"itens"`
}

type corpoRecusa struct {
	Motivo string `json:"motivo"`
}

type corpoPreliminar struct {
	Valor       string `json:"valor"`
	Observacoes string `json:"observacoes"`
}

func (h *LaboratorioHandler) registarAnaliseHTTP(c *gin.Context) {
	var corpo applaboratorio.DadosNovaAnalise
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.registarAnalise.Executar(c.Request.Context(), actor.Sujeito, corpo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *LaboratorioHandler) listarAnalisesHTTP(c *gin.Context) {
	out, err := h.listarAnalises.Executar(c.Request.Context())
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

func (h *LaboratorioHandler) emitirRequisicaoHTTP(c *gin.Context) {
	var corpo corpoEmitirRequisicao
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.emitir.Executar(c.Request.Context(), actor.Sujeito, applaboratorio.DadosEmitirRequisicao{
		EpisodioID: c.Param("eid"), DoenteID: corpo.DoenteID,
		Prioridade: corpo.Prioridade, Itens: corpo.Itens,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *LaboratorioHandler) obterRequisicaoHTTP(c *gin.Context) {
	out, err := h.obterRequisicao.Executar(c.Request.Context(), c.Param("rid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *LaboratorioHandler) listarRequisicoesHTTP(c *gin.Context) {
	out, err := h.listarRequisicoes.Executar(c.Request.Context(), c.Param("eid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

// listarFilaHTTP aceita ?estado=PENDENTE&estado=COLHIDA; sem filtro devolve todos.
func (h *LaboratorioHandler) listarFilaHTTP(c *gin.Context) {
	var estados []dominiolab.EstadoResultado
	for _, e := range c.QueryArray("estado") {
		estados = append(estados, dominiolab.EstadoResultado(e))
	}
	out, err := h.listarFila.Executar(c.Request.Context(), estados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

// listarResultadosDoEpisodioHTTP é a leitura clínica: o caso de uso filtra os estados
// visíveis (o preliminar não aparece aqui).
func (h *LaboratorioHandler) listarResultadosDoEpisodioHTTP(c *gin.Context) {
	out, err := h.resultadosEpisodio.Executar(c.Request.Context(), c.Param("eid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"itens": out})
}

func (h *LaboratorioHandler) colherAmostraHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.colher.Executar(c.Request.Context(), actor.Sujeito, c.Param("rid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *LaboratorioHandler) recusarAmostraHTTP(c *gin.Context) {
	var corpo corpoRecusa
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.recusar.Executar(c.Request.Context(), actor.Sujeito, c.Param("rid"), corpo.Motivo)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *LaboratorioHandler) submeterPreliminarHTTP(c *gin.Context) {
	var corpo corpoPreliminar
	// O corpo é obrigatório: sem valor não há resultado. Um corpo malformado tem de
	// falhar com 400 — aceitá-lo em silêncio devolveria 200 a confirmar um resultado
	// que na verdade não foi gravado.
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.submeter.Executar(c.Request.Context(), actor.Sujeito, c.Param("rid"),
		applaboratorio.DadosSubmeterPreliminar{Valor: corpo.Valor, Observacoes: corpo.Observacoes})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
```

- [ ] **Step 5: Correr para confirmar que passa**

Run: `go test ./internal/adapters/... -v && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/laboratorio/leitor_clinico.go internal/adapters/http/laboratorio_handler.go internal/adapters/http/laboratorio_test.go
git commit -m "$(printf 'feat(laboratorio): ACL sobre o Clinico e handlers HTTP com RBAC\n\nLeitorClinico responde duas perguntas booleanas (doente activo, episodio aberto do\ndoente) sem deixar passar tipos do Clinico. Rotas separadas: fila do laboratorio\n(Tecnico/Patologista) e resultados do episodio (clinico, so validados).\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 10: Plataforma — wiring no `app.go` + ADR-031

**Files:**
- Modify: `internal/platform/app.go`
- Create: `adrs/ADR-031-bc-laboratorio.md`
- Modify: `SPRINT.md`
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: tudo o que as Tasks 5–9 produzem.
- Produces: rotas do BC Laboratório servidas pela aplicação real.

- [ ] **Step 1: Ligar os repositórios e os casos de uso em `app.go`**

O ficheiro já instancia os repositórios do Clínico (`repoDoentes`, `repoEpisodios`) e
os da Farmácia. A seguir ao bloco da Farmácia (depois de `handlerFarmaciaStock` ser
construído e antes do registo das rotas), acrescentar:

```go
	// --- BC Laboratório (M3) ---
	repoAnalises := pgrepo.NovoRepositorioAnalises(pool)
	repoRequisicoes := pgrepo.NovoRepositorioRequisicoes(pool)
	repoResultados := pgrepo.NovoRepositorioResultados(pool)
	// ACL: o Laboratório lê o Clínico apenas através deste adaptador.
	leitorClinicoLab := adlaboratorio.NovoLeitorClinico(repoDoentes, repoEpisodios)

	handlerLaboratorio := adhttp.NovoLaboratorioHandler(
		applaboratorio.NovoCasoRegistarAnalise(repoAnalises, repoAuditoria),
		applaboratorio.NovoCasoListarAnalises(repoAnalises),
		applaboratorio.NovoCasoEmitirRequisicao(repoRequisicoes, repoAnalises, leitorClinicoLab, repoAuditoria),
		applaboratorio.NovoCasoObterRequisicao(repoRequisicoes),
		applaboratorio.NovoCasoListarRequisicoesDoEpisodio(repoRequisicoes),
		applaboratorio.NovoCasoColherAmostra(repoResultados, repoAuditoria),
		applaboratorio.NovoCasoRecusarAmostra(repoResultados, repoAuditoria),
		applaboratorio.NovoCasoSubmeterPreliminar(repoResultados, repoAuditoria),
		applaboratorio.NovoCasoListarFila(repoResultados),
		applaboratorio.NovoCasoListarResultadosDoEpisodio(repoResultados),
	)
```

E, no bloco onde as rotas são registadas (a seguir a `adhttp.RegistarFarmaciaStock(...)`):

```go
		adhttp.RegistarLaboratorio(r, handlerLaboratorio, limiteMW, authMW)
```

Acrescentar os imports em falta no topo do ficheiro (junto dos existentes):

```go
	adlaboratorio "github.com/ivandrosilva12/sgcfinal/internal/adapters/laboratorio"
	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
```

- [ ] **Step 2: Confirmar que o linter arquitectural continua verde**

Run: `go build ./... && go test ./... && go-arch-lint check`
Expected: PASS. O `go-arch-lint` tem de continuar sem violações: o novo pacote
`internal/adapters/laboratorio` é da camada 3 e pode importar Domínio e Aplicação. Se
o `.go-arch-lint.yml` listar os componentes de adaptadores um a um, acrescentar lá o
novo pacote (seguir o padrão da entrada de `internal/adapters/farmacia`).

- [ ] **Step 3: Criar `adrs/ADR-031-bc-laboratorio.md`**

```markdown
# ADR-031 — BC Laboratório: requisição, amostra e resultado preliminar

- **Estado:** Aceite
- **Data:** 2026-07-14
- **Marco/Sprint:** M3 / Sprint 12
- **Fontes:** CCD-M3 (blueprint), DDM-001, ADR-028/029 (precedente da ACL da Farmácia).

## Contexto

O BC Laboratório era o último bounded context clínico por abrir. O blueprint (CCD-M3)
descreve-o em duas fatias: até ao resultado preliminar, e depois a validação com
segregação de funções e valores críticos. Esta ADR cobre a primeira.

## Decisão

1. **A requisição vive no BC Laboratório**, não no Clínico. Refere `episodio_id` e
   `doente_id` por id, sem FK cross-context; a existência e o estado do episódio são
   validados por uma ACL (`LeitorClinico`) na camada de aplicação, com o adaptador em
   `internal/adapters/laboratorio/`. É exactamente o desenho da Receita (ADR-028): um
   segundo padrão para o mesmo problema só criaria confusão.
2. **Emitir uma requisição cria um `Resultado` PENDENTE por análise pedida**, na mesma
   transacção (`RepositorioRequisicoes.Emitir`). Uma requisição sem resultados nunca
   apareceria na fila do laboratório e ficaria invisível para toda a gente.
3. **O submissor é o sujeito autenticado, nunca um campo do corpo do pedido.** Esta é
   a decisão que torna real a segregação de funções do Sprint 13: se o cliente pudesse
   declarar quem submeteu, poderia validar o seu próprio resultado declarando outro
   nome. A mesma regra valerá para o validador.
4. **O resultado preliminar não é visível ao médico.** A regra vive na aplicação
   (`CasoListarResultadosDoEpisodio` filtra por `EstadosVisiveisAoMedico` =
   VALIDADA/CONCLUIDA), e não apenas no RBAC de rota — o RBAC não chegaria, porque o
   médico *tem* de ver os resultados validados: a distinção é pelo estado, não pelo
   papel. Enquanto a validação não existir (Sprint 13), esta leitura devolve vazio —
   é o comportamento esperado, não um defeito.
5. **Coerência estado↔timestamps imposta por CHECK** e **guarda compare-and-set nos
   UPDATE de transição** (lições do Sprint 11): a BD recusa uma PROCESSADA sem
   submissor nem valor, e duas colheitas concorrentes da mesma amostra não se
   atropelam — a segunda perde a corrida e recebe Conflito/409.
6. **A CHECK de segregação (`patologista_validador_id <> tecnico_submissor_id`) já
   existe na tabela**, embora a transição de validação seja do Sprint 13: defesa em
   profundidade barata de escrever agora.

## Desvios ao blueprint (conscientes)

- **Numeração de marcos.** O CCD-M3 do blueprint é "Farmácia + Laboratório". Neste
  repositório a Farmácia foi entregue no M2 (Sprints 9–10), pelo que o M3 é apenas o
  Laboratório (Sprints 12–13). Os marcos do blueprint continuam a valer como
  referência de âmbito, não de numeração.
- **O "biólogo validador" é o papel `Patologista`.** Os 11 papéis do DDM-001 não
  incluem `Biologo` (ver `docs/ERRATA-001-papeis.md`).
- **Intervalos de referência e valores críticos em `jsonb`**, não em tabelas-filho:
  são lidos em bloco com o agregado e nunca consultados isoladamente por SQL.

## Consequências

- Abre o marco M3 e prepara o Sprint 13 (validação, segregação, valores críticos).
- O endpoint de leitura clínica de resultados existe e devolve vazio até haver
  validação — é o critério de saída do blueprint ("o médico tenta ver o preliminar e
  não vê").

## Dívida registada (não-bloqueante)

- **Override de alergia da Farmácia sem dupla aprovação.** O CCD-M3 exige aprovação do
  médico prescritor **e** do Director Clínico; o que existe é um override de actor
  único com justificação auditada (`internal/application/farmacia/dispensa.go`). Não se
  resolve dentro de um sprint de Laboratório.
- Auditoria fora da transacção das escritas (como nos sprints anteriores).
- Cancelamento de requisição (estado CANCELADA existe no schema, sem caso de uso).
- Eventos definidos mas não emitidos (scaffolding, à espera do Outbox).
```

- [ ] **Step 4: Actualizar `SPRINT.md` e `CLAUDE.md`**

No `SPRINT.md`, substituir o cabeçalho e acrescentar a secção do Sprint 12 no topo das
sprints entregues:

```markdown
# SPRINT ACTUAL

- **Marco**: M3 — Laboratório
- **Sprint**: 12 (BC Laboratório — catálogo, requisição, amostra, preliminar) — **entregue**
- **Objectivo**: abrir o BC Laboratório até ao resultado preliminar submetido pelo
  técnico — que não é visível ao médico. A validação com segregação de funções e os
  valores críticos são o Sprint 13.

## Sprint 12 — entregue

- [x] Catálogo de análises (intervalos de referência, valores críticos) com seed.
- [x] Requisição no BC Laboratório via ACL sobre o Clínico (doente activo, episódio
      aberto); um resultado PENDENTE por análise, em transacção única.
- [x] Colheita, recusa (motivo obrigatório) e submissão do preliminar; o técnico
      gravado é sempre o sujeito autenticado.
- [x] O preliminar **não** é visível ao médico: a leitura clínica filtra por
      VALIDADA/CONCLUIDA na aplicação.
- [x] Guarda compare-and-set nas transições; CHECK de coerência estado↔timestamps.
- [x] ADR-031.
```

E acrescentar os critérios de saída do marco, a seguir aos do M2:

```markdown
## Critérios de saída M3 — Laboratório

- [x] Médico requisita análises para um episódio aberto. — Sprint 12
- [x] Técnico colhe/recusa amostra e submete resultado preliminar. — Sprint 12
- [x] O preliminar não é visível ao médico. — Sprint 12
- [ ] Validação pelo patologista com segregação (submissor ≠ validador). — Sprint 13
- [ ] Valores críticos detectados e notificados (SMS auditado). — Sprint 13
- [ ] Correcção cria novo resultado preservando o original. — Sprint 13
```

No `CLAUDE.md`, na secção **6. Marco Actual**, substituir o parágrafo do M2 por (e
manter o resto):

```markdown
**M3 — Laboratório** (Sprints 12–13; ver `SPRINT.md`). Entrega o BC Laboratório:
catálogo de análises, requisição (via ACL sobre o Clínico), amostra e resultado com
state machine; a validação com segregação de funções (técnico ≠ patologista) e os
valores críticos fecham o marco no Sprint 13.

**M2 — Clínico Core** (entregue; Sprints 7–11): BC Clínico (doente, episódio + EHR,
cirurgia ambulatória com consentimento LPDP) e BC Farmácia (catálogo, receita, stock
e dispensa FEFO).
```

E, no rodapé de ADRs registadas, acrescentar `adrs/ADR-031-bc-laboratorio.md` e passar
"Próximo ADR" para **ADR-032**.

- [ ] **Step 5: Correr a verificação completa**

Run: `go build ./... && go test ./... && gofmt -l internal/ && go vet ./...`
Expected: build+testes verdes; `gofmt -l` sem saída; vet limpo.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/app.go adrs/ADR-031-bc-laboratorio.md SPRINT.md CLAUDE.md
git commit -m "$(printf 'feat(laboratorio): liga o BC Laboratorio ao composition root + ADR-031\n\nInstancia repos, ACL, casos de uso e handler; regista as rotas com limite+auth.\nADR-031 documenta a requisicao via ACL, o submissor autenticado, a regra de\nvisibilidade e as divergencias de numeracao face ao blueprint.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

### Task 11: Testes de integração (contra Postgres real)

**Files:**
- Create: `tests/integration/laboratorio_test.go`

**Interfaces:**
- Consumes: o helper `ligar(t) (*pgxpool.Pool, context.Context)` (em `tests/integration/migracoes_test.go` — salta o teste sem `DATABASE_URL` e aplica as migrações) e os repositórios da Task 8.
- Produces: `fixturaLaboratorio(t, pool, ctx, bi, nome) (doenteID, episodioID string)` — modelada em `fixturaCirurgia` (`tests/integration/cirurgia_test.go:346`), mas com um episódio de **CONSULTA** aberto e limpeza das tabelas do laboratório.

Atenção aos nomes reais: `ligar(t)` devolve **dois** valores (`pool, ctx`), e o pacote
de integração **não** tem `criarDoente`/`criarEpisodioAberto`/`agora()`. Reutilizar o
que existe; a fixtura nova é a única coisa a acrescentar.

- [ ] **Step 1: Escrever o teste de integração**

```go
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Ids fixos de pessoal (o BC Laboratório não valida o pessoal — é do Identidade).
const (
	medicoLabID  = "00000000-0000-4000-8000-0000000000f1"
	tecnicoLabID = "00000000-0000-4000-8000-0000000000f2"
	outroTecnico = "00000000-0000-4000-8000-0000000000f3"
)

// fixturaLaboratorio cria um doente e um episódio de CONSULTA ABERTO na BD real, com
// limpeza registada (inclui as tabelas do laboratório, que referenciam a requisição).
func fixturaLaboratorio(t *testing.T, pool *pgxpool.Pool, ctx context.Context, bi, nome string) (string, string) {
	t.Helper()
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	repoEp := pgrepo.NovoRepositorioEpisodios(pool)

	num, err := repoDoentes.ProximoNumeroProcesso(ctx, 2026)
	if err != nil {
		t.Fatalf("próximo número: %v", err)
	}
	ident, _ := clinico.NovaIdentificacao(nome, time.Date(1985, 5, 5, 0, 0, 0, 0, time.UTC),
		clinico.SexoFeminino, &bi, nil, nil)
	ct, _ := clinico.NovosContactos("+244923111113", nil, nil)
	doente, _ := clinico.NovoDoente(num, ident, ct, "AO")
	doenteID, err := repoDoentes.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar doente: %v", err)
	}
	const espID = "00000000-0000-4000-8000-0000000000e1"
	ep, _ := clinico.NovoEpisodio(doenteID, clinico.EpisodioConsulta, espID, medicoLabID, time.Now())
	episodioID, err := repoEp.Guardar(ctx, ep)
	if err != nil {
		t.Fatalf("guardar episódio: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `
DELETE FROM laboratorio.resultados WHERE requisicao_id IN
    (SELECT id FROM laboratorio.requisicoes WHERE episodio_id=$1)`, episodioID)
		_, _ = pool.Exec(ctx, `
DELETE FROM laboratorio.itens_requisicao WHERE requisicao_id IN
    (SELECT id FROM laboratorio.requisicoes WHERE episodio_id=$1)`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM laboratorio.requisicoes WHERE episodio_id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.episodios_clinicos WHERE id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})
	return doenteID, episodioID
}

// TestLaboratorio_CicloRequisicaoAtePreliminar cobre o ciclo completo contra Postgres:
// emissão transaccional (requisição + itens + resultados PENDENTE), colheita,
// submissão do preliminar, e a regra de visibilidade.
func TestLaboratorio_CicloRequisicaoAtePreliminar(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio; aplica as migrações

	repoAnalises := pgrepo.NovoRepositorioAnalises(pool)
	repoRequisicoes := pgrepo.NovoRepositorioRequisicoes(pool)
	repoResultados := pgrepo.NovoRepositorioResultados(pool)

	// O seed da migration 0001 tem de estar lá.
	hb, err := repoAnalises.ObterPorCodigo(ctx, "HB")
	if err != nil {
		t.Fatalf("o seed do catálogo devia conter HB: %v", err)
	}
	if hb.Unidade() != "g/dL" {
		t.Fatalf("esperava unidade g/dL, veio %q", hb.Unidade())
	}

	doenteID, episodioID := fixturaLaboratorio(t, pool, ctx, "12345678LA123", "Ana Laboratório")

	req, err := dominio.NovaRequisicao(dominio.DadosNovaRequisicao{
		EpisodioID: episodioID, DoenteID: doenteID, MedicoRequisitanteID: medicoLabID,
		Prioridade: dominio.PrioridadeRotina,
		Itens:      []dominio.ItemRequisicao{{CodigoAnalise: "HB"}, {CodigoAnalise: "GLIC"}},
	})
	if err != nil {
		t.Fatalf("construir requisição: %v", err)
	}
	glic, err := repoAnalises.ObterPorCodigo(ctx, "GLIC")
	if err != nil {
		t.Fatalf("obter GLIC: %v", err)
	}
	resHB, _ := dominio.NovoResultado("por-atribuir", "HB", hb.Unidade())
	resGLIC, _ := dominio.NovoResultado("por-atribuir", "GLIC", glic.Unidade())

	reqID, err := repoRequisicoes.Emitir(ctx, req, []*dominio.Resultado{resHB, resGLIC})
	if err != nil {
		t.Fatalf("emitir requisição: %v", err)
	}

	// A emissão criou dois resultados PENDENTE.
	fila, err := repoResultados.ListarFila(ctx, []dominio.EstadoResultado{dominio.ResPendente})
	if err != nil {
		t.Fatalf("listar fila: %v", err)
	}
	var meus []string
	for _, r := range fila {
		if r.RequisicaoID == reqID {
			meus = append(meus, r.ID)
		}
	}
	if len(meus) != 2 {
		t.Fatalf("esperava 2 resultados PENDENTE da requisição, veio %d", len(meus))
	}

	// Colher e submeter o preliminar do primeiro.
	res, err := repoResultados.ObterPorID(ctx, meus[0])
	if err != nil {
		t.Fatalf("obter resultado: %v", err)
	}
	if err := res.ColherAmostra(tecnicoLabID, time.Now()); err != nil {
		t.Fatalf("colher: %v", err)
	}
	if err := repoResultados.Transitar(ctx, res); err != nil {
		t.Fatalf("gravar colheita: %v", err)
	}
	res, _ = repoResultados.ObterPorID(ctx, meus[0])
	if err := res.SubmeterPreliminar(tecnicoLabID, "12.5", "", time.Now()); err != nil {
		t.Fatalf("submeter: %v", err)
	}
	if err := repoResultados.Transitar(ctx, res); err != nil {
		t.Fatalf("gravar submissão: %v", err)
	}

	// Visibilidade: o preliminar NÃO aparece na leitura clínica.
	visiveis, err := repoResultados.ListarPorEpisodio(ctx, episodioID,
		[]dominio.EstadoResultado{dominio.ResValidada, dominio.ResConcluida})
	if err != nil {
		t.Fatalf("listar resultados visíveis: %v", err)
	}
	if len(visiveis) != 0 {
		t.Fatalf("o preliminar não pode ser visível ao médico, veio %+v", visiveis)
	}
	// Mas a fila do laboratório vê-o.
	processados, err := repoResultados.ListarPorEpisodio(ctx, episodioID,
		[]dominio.EstadoResultado{dominio.ResProcessada})
	if err != nil {
		t.Fatalf("listar processados: %v", err)
	}
	if len(processados) != 1 || processados[0].Valor != "12.5" {
		t.Fatalf("esperava 1 resultado PROCESSADA com valor 12.5, veio %+v", processados)
	}
}

// TestLaboratorio_TransicaoConcorrentePerdeACorrida verifica a guarda compare-and-set:
// dois agregados lidos no mesmo estado, duas transições — só a primeira escreve.
func TestLaboratorio_TransicaoConcorrentePerdeACorrida(t *testing.T) {
	pool, ctx := ligar(t)

	repoAnalises := pgrepo.NovoRepositorioAnalises(pool)
	repoRequisicoes := pgrepo.NovoRepositorioRequisicoes(pool)
	repoResultados := pgrepo.NovoRepositorioResultados(pool)

	hb, err := repoAnalises.ObterPorCodigo(ctx, "HB")
	if err != nil {
		t.Fatalf("obter HB: %v", err)
	}
	doenteID, episodioID := fixturaLaboratorio(t, pool, ctx, "12345678LA124", "Bia Laboratório")
	req, _ := dominio.NovaRequisicao(dominio.DadosNovaRequisicao{
		EpisodioID: episodioID, DoenteID: doenteID, MedicoRequisitanteID: medicoLabID,
		Prioridade: dominio.PrioridadeRotina,
		Itens:      []dominio.ItemRequisicao{{CodigoAnalise: "HB"}},
	})
	res, _ := dominio.NovoResultado("por-atribuir", "HB", hb.Unidade())
	reqID, err := repoRequisicoes.Emitir(ctx, req, []*dominio.Resultado{res})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	fila, _ := repoResultados.ListarFila(ctx, []dominio.EstadoResultado{dominio.ResPendente})
	var id string
	for _, r := range fila {
		if r.RequisicaoID == reqID {
			id = r.ID
		}
	}

	// Dois técnicos lêem o mesmo resultado PENDENTE.
	a, _ := repoResultados.ObterPorID(ctx, id)
	b, _ := repoResultados.ObterPorID(ctx, id)
	_ = a.ColherAmostra(tecnicoLabID, time.Now())
	_ = b.ColherAmostra(outroTecnico, time.Now())

	if err := repoResultados.Transitar(ctx, a); err != nil {
		t.Fatalf("a primeira colheita devia escrever: %v", err)
	}
	err = repoResultados.Transitar(ctx, b)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("a segunda colheita devia perder a corrida com Conflito, veio %v", err)
	}
}
```

**Nota sobre `"por-atribuir"`:** o `NovoResultado` exige uma requisição não vazia, mas
o id só existe depois do INSERT. O repositório ignora este valor e usa o id real da
requisição que acabou de inserir, dentro da mesma transacção (ver Task 8).

- [ ] **Step 2: Confirmar que compila sem Postgres**

Run: `go test -tags integration ./tests/integration/ -run NADA`
Expected: PASS (nenhum teste corre; só se verifica a compilação).

- [ ] **Step 3: Correr a integração (com Postgres)**

Run:
```
DATABASE_URL="postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable" go test -tags integration ./tests/integration/ -run Laboratorio -v
```
Expected: PASS (ou SKIP se `DATABASE_URL` não estiver definido).

- [ ] **Step 4: Verificação final de qualidade**

Run:
```
go build ./... && go test ./... && gofmt -l internal/ && go vet ./...
bash scripts/cobertura.sh
```
Expected: build+testes verdes; `gofmt -l` sem saída; vet limpo; cobertura domínio
≥85%, aplicação ≥75%, adaptadores ≥60%.

- [ ] **Step 5: Commit**

```bash
git add tests/integration/laboratorio_test.go
git commit -m "$(printf 'test(laboratorio): integracao do ciclo requisicao-colheita-preliminar\n\nEmissao transaccional (requisicao + itens + resultados pendentes), submissao do\npreliminar, a regra de visibilidade e a guarda compare-and-set a rejeitar a\ncolheita concorrente. SKIP sem DATABASE_URL.\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>')"
```

---

## Revisão final (whole-branch)

Após a Task 11, dispatch da revisão de branch completa no modelo mais capaz (padrão
dos sprints anteriores): `scripts/review-package <merge-base> HEAD`, com foco em:

1. **O submissor nunca vem do corpo do pedido** — em todas as entradas (handler, caso
   de uso, repositório). É a fundação da segregação de funções do Sprint 13; se falhar
   aqui, o Sprint 13 constrói uma invariante sobre areia.
2. **A regra de visibilidade** — nenhum caminho de leitura clínica devolve PROCESSADA.
3. **A emissão é mesmo atómica** — não há caminho que grave a requisição sem os
   resultados (e o rollback é feito em todos os erros).
4. **A guarda compare-and-set** nos UPDATE de transição, e o `Transitar` a não escrever
   colunas de identidade do resultado.
5. **CHECK da BD coerentes com as invariantes do domínio** (nenhuma delas contornável
   pelos caminhos da aplicação).
6. Gates de cobertura e trailers de commit correctos.

Triar os Minor registados antes do merge.
