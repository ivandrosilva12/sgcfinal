# Sprint 7 — BC Clínico: agregado Doente (+ Alergia + Antecedente)

**Marco:** M2 — Clínico Core (primeira fatia vertical)
**Data:** 2026-07-12
**Estado:** Aprovado para planeamento

## Contexto

O M1 (BC Identidade) está completo e fundido em `main`: autenticação Keycloak
(OIDC/RS256), RBAC pelos 11 papéis, auditoria append-only, gestão administrativa de
utilizadores, MFA, sessões, notificações. O M2 abre o **BC Clínico**, cuja primeira fatia
vertical natural é o agregado **Doente**, incluindo os filhos **Alergia** e
**AntecedenteClinico**.

Esta fatia entrega o CRUD completo do domínio ao HTTP: registo de doentes com número de
processo, identificação (BI/NIF/passaporte angolanos), contactos e morada, grupo sanguíneo,
alergias, antecedentes clínicos, pesquisa e ciclo de vida (desactivação e óbito). A apagamento
LPDP (estado `APAGADO` + pseudonimização) e a tabela `consentimentos` ficam **diferidos** para
uma fatia dedicada, tal como os `episodios_clinicos` (Sprint 8).

**Fonte de verdade do modelo de dados:** DDM-001 v2.0 (extraído verbatim; não inventado).

## Decomposição do M2

- **Sprint 7 (esta fatia):** Doente + Alergia + AntecedenteClinico.
- **Sprint 8:** Episódio Clínico.
- **Sprint 9:** Receita / Prescrição (+ validação contra alergias do doente).
- **Sprint 10:** Cirurgia ambulatória.

## Princípios herdados (não-negociáveis)

- **Linguagem ubíqua PT-PT angolano** em TODO o output (código, comentários, commits, JSON,
  mensagens de erro).
- **DDD táctico + Clean Architecture**, dependência para dentro. `internal/domain/**` importa
  apenas stdlib + Shared Kernel; zero `pgx`/`gin`/`http`/`oidc`.
- **Sem `google/uuid` no domínio nem na aplicação** — só permitido em adapters (arch-lint).
- **Sem `panic()`** fora de inicialização.
- **Migrations forward-only**, sem `.down.sql`.
- **Erros RFC 7807** (`application/problem+json`, PT-PT) via `erros.Novo(categoria, msg)` +
  `responderErro`.
- **Conventional Commits** em PT-PT, terminando com o trailer
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- **Cobertura** (agregada por camada, `scripts/cobertura.sh`): domínio ≥85%, aplicação ≥75%,
  adapters ≥60%.
- **Dados de saúde e identificadores nunca são registados em log.** Acessos a dados de doentes
  são auditados.

---

## Secção 1 — Arquitectura + modelo de dados

Novo BC Clínico em `internal/{domain,application,adapters}/clinico/`. Schema `clinico` já
existe (`docker/postgres/init.sql`). Reutiliza a infra do M1: middleware
`Auth`/`RBAC`/`LimiteTaxa`, respostas RFC 7807, repositório de auditoria
(`pgrepo.NovoRepositorioAuditoria`, BC-agnóstico), validadores Angola do Shared Kernel.

### Migration `migrations/clinico/0001_doentes.sql`

Extraída verbatim do DDM-001, com três tabelas + infra de suporte. Tabelas
`consentimentos` e `episodios_clinicos` do DDM ficam **fora de âmbito** desta fatia.

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE clinico.doentes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    num_processo        TEXT NOT NULL UNIQUE,
    nome_completo       TEXT NOT NULL,
    data_nascimento     DATE NOT NULL,
    sexo                CHAR(1) NOT NULL CHECK (sexo IN ('M','F','O')),
    bi                  TEXT,
    nif                 TEXT,
    passaporte          TEXT,
    nacionalidade       TEXT NOT NULL DEFAULT 'AO',
    telefone            TEXT NOT NULL,
    email               TEXT,
    morada_provincia    TEXT, morada_municipio TEXT, morada_comuna TEXT,
    morada_bairro       TEXT, morada_rua TEXT, morada_casa TEXT, morada_referencia TEXT,
    grupo_sanguineo     TEXT CHECK (grupo_sanguineo IN ('A+','A-','B+','B-','AB+','AB-','O+','O-')),
    estado              TEXT NOT NULL DEFAULT 'ACTIVO'
                        CHECK (estado IN ('ACTIVO','INACTIVO','FALECIDO','APAGADO')),
    falecido_em         DATE,
    causa_morte_cid     TEXT,
    criado_em           TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_em      TIMESTAMPTZ NOT NULL DEFAULT now(),
    desactivado_em      TIMESTAMPTZ,
    desactivado_motivo  TEXT,
    apagado_em          TIMESTAMPTZ,
    CONSTRAINT doc_identificacao CHECK (bi IS NOT NULL OR passaporte IS NOT NULL)
);
CREATE UNIQUE INDEX uq_doentes_bi ON clinico.doentes(bi) WHERE bi IS NOT NULL AND apagado_em IS NULL;
CREATE INDEX idx_doentes_nome ON clinico.doentes USING gin (nome_completo gin_trgm_ops);
CREATE INDEX idx_doentes_telefone ON clinico.doentes(telefone);
CREATE INDEX idx_doentes_estado ON clinico.doentes(estado) WHERE desactivado_em IS NULL;

CREATE TABLE clinico.alergias (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id UUID NOT NULL REFERENCES clinico.doentes(id) ON DELETE CASCADE,
    substancia TEXT NOT NULL,
    severidade TEXT NOT NULL CHECK (severidade IN ('LEVE','MODERADA','GRAVE','ANAFILACTICA')),
    reaccao_tipica TEXT, confirmada_em DATE, notas TEXT,
    criada_em TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_alergias_doente ON clinico.alergias(doente_id);

CREATE TABLE clinico.antecedentes_clinicos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id UUID NOT NULL REFERENCES clinico.doentes(id) ON DELETE CASCADE,
    tipo TEXT NOT NULL CHECK (tipo IN ('PESSOAL','FAMILIAR','CIRURGICO','OBSTETRICO')),
    descricao TEXT NOT NULL, cid TEXT, data_inicio DATE,
    activo BOOLEAN NOT NULL DEFAULT true, notas TEXT,
    criado_em TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_antecedentes_doente ON clinico.antecedentes_clinicos(doente_id);

-- Contador por ano para o número de processo automático.
CREATE TABLE clinico.processo_sequencia (
    ano    INT PRIMARY KEY,
    ultimo INT NOT NULL DEFAULT 0
);
```

### Decisões transversais

- **Identidade dos registos:** `id` gerado pela BD (`gen_random_uuid()` DEFAULT + `RETURNING`);
  o domínio usa `string`. Mantém a pureza da camada de domínio (sem `google/uuid`).
- **Número de processo (híbrido):** se o pedido trouxer um número, usa-o (unicidade garantida
  por `UNIQUE`; colisão → 409). Senão, gera `P-{ANO}-{sequencial:06d}` (ex.: `P-2026-000001`)
  via incremento atómico em `processo_sequencia` na transacção de criação.
- **Pesquisa:** índice trigram (`gin_trgm_ops`) em `nome_completo` sustenta ILIKE fuzzy;
  também pesquisa exacta por BI / num_processo / telefone; paginação por `limite`/`deslocamento`.
- **Endpoints:** base `/api/v1/doentes` (convenção de nomes do DDM).

---

## Secção 2 — Domínio (`internal/domain/clinico/`)

Domínio rico, zero infra, IDs `string`. Todos os erros via
`erros.Novo(erros.CategoriaValidacao, i18n.T(...))` com mensagens PT-PT novas no catálogo.

### `doente.go` — agregado raiz

```go
type Doente struct {
    id                 string        // gerado pela BD; vazio antes de persistir
    numProcesso        string        // "P-2026-000001" ou manual
    identificacao      Identificacao
    contactos          Contactos
    nacionalidade      string        // ISO-3166 alfa-2; default "AO"
    grupoSanguineo     *GrupoSanguineo
    estado             EstadoDoente  // ACTIVO|INACTIVO|FALECIDO|APAGADO
    alergias           []Alergia
    antecedentes       []AntecedenteClinico
    criadoEm           time.Time
    actualizadoEm      time.Time
    desactivadoEm      *time.Time
    desactivadoMotivo  string
    falecidoEm         *time.Time
    causaMorteCID      string
}
```

- Factory `NovoDoente(numProcesso string, ident Identificacao, ct Contactos, nacionalidade string) (*Doente, error)` — valida identificação e contactos; nacionalidade default `"AO"` se vazia; estado inicial `ACTIVO`.
- `AdicionarAlergia(a Alergia) error`, `AdicionarAntecedente(a AntecedenteClinico) error`.
- `Desactivar(motivo string) error` — exige motivo não-vazio; proíbe se já `FALECIDO`/`APAGADO`; define estado `INACTIVO` + `desactivadoEm`/`desactivadoMotivo`.
- `Reactivar() error` — só de `INACTIVO` para `ACTIVO`; limpa `desactivadoEm`/motivo.
- `DeclararFalecido(data time.Time, causaCID string) error` — exige data não-futura; proíbe se `APAGADO`; estado `FALECIDO` + `falecidoEm`/`causaMorteCID`.
- `AtualizarIdentificacao(Identificacao) error`, `AtualizarContactos(Contactos) error`, `DefinirGrupoSanguineo(*GrupoSanguineo)`.
- Getters para todos os campos (domínio encapsulado).

### `identificacao.go` — VO

```go
type Identificacao struct {
    NomeCompleto    string
    DataNascimento  time.Time // só data (sem horas)
    Sexo            Sexo      // M|F|O
    BI              *string
    NIF             *string
    Passaporte      *string
}
```
`NovaIdentificacao(...)` valida: nome não-vazio; data não-futura; sexo válido; **BI ou
Passaporte obrigatório** (invariante DDM `doc_identificacao`); BI via `identity.NovoBI` se
presente; NIF via `identity.NovoNIF` se presente. `Sexo` é enum tipado com parser.

### `contactos.go` — VO

```go
type Contactos struct {
    Telefone string   // obrigatório, normalizado por identity.NovoTelefone
    Email    *string  // validado se presente
    Morada   *Morada
}
type Morada struct {
    Provincia, Municipio, Comuna, Bairro, Rua string
    Casa, Referencia *string
}
```
`NovosContactos(...)` valida telefone (obrigatório) e email (se presente).

### `alergia.go` / `antecedente.go` — VOs

```go
type Alergia struct {
    Substancia    string
    Severidade    Severidade // LEVE|MODERADA|GRAVE|ANAFILACTICA
    ReaccaoTipica string
    ConfirmadaEm  *time.Time
    Notas         string
}
type AntecedenteClinico struct {
    Tipo       TipoAntecedente // PESSOAL|FAMILIAR|CIRURGICO|OBSTETRICO
    Descricao  string
    CID        string
    DataInicio *time.Time
    Activo     bool
    Notas      string
}
```
Factories validam substância/descrição não-vazias e enums.

### Outros ficheiros

- `grupo_sanguineo.go` — enum `GrupoSanguineo` (8 valores) + `ParseGrupoSanguineo`.
- `estado.go` — `EstadoDoente` + verificação de transições válidas.
- `sexo.go`, `severidade.go`, `tipo_antecedente.go` — enums tipados com parsers (podem
  coabitar no ficheiro do VO respectivo se pequenos).
- `eventos.go` — `DoenteRegistado`, `DoenteDesactivado`, `DoenteFalecido`, `AlergiaRegistada`
  (sobre `shared/evento`).
- `repositorio.go` — interface:
  ```go
  type RepositorioDoentes interface {
      Guardar(ctx context.Context, d *Doente) (string, error) // devolve id
      ObterPorID(ctx context.Context, id string) (*Doente, error)
      ObterPorNumProcesso(ctx context.Context, num string) (*Doente, error)
      Pesquisar(ctx context.Context, f FiltroDoentes) (PaginaDoentes, error)
      ProximoNumeroProcesso(ctx context.Context, ano int) (string, error)
  }
  ```
  `FiltroDoentes{Termo, Estado string; Limite, Deslocamento int}` e
  `PaginaDoentes{Itens []ResumoDoente; Total, Limite, Deslocamento int}` vivem **no domínio**
  (tipos de leitura simples), pois a interface do repositório referencia-os; a aplicação
  reexporta-os para não obrigar os handlers a importar o domínio directamente. `ResumoDoente`
  também é definido no domínio para que `PaginaDoentes` não dependa da aplicação.

---

## Secção 3 — Aplicação + Shared Kernel

### Shared Kernel — `internal/domain/shared/identity/nif.go` (novo)

`NovoNIF(bruto string) (string, error)` — NIF angolano de 10 caracteres: pessoa singular =
9 dígitos + 1 letra final; pessoa colectiva = 10 dígitos. Normaliza (remove espaços,
maiúsculas), valida comprimento e formato, devolve `erros.Novo(CategoriaValidacao, ...)` PT-PT.
Testes acompanham (`nif_test.go`), contando para o gate de domínio ≥85%.

### Aplicação — `internal/application/clinico/`

Casos de uso sobre portas, sem infra. Cada caso recebe dependências por construtor
(`NovoCaso...`), devolve DTO + erro.

- **`ports.go`** — reexporta `RepositorioDoentes`, `FiltroDoentes`, `PaginaDoentes` e
  `ResumoDoente` do domínio; define `Auditor` (padrão do M1); DTOs de entrada/saída próprios da
  aplicação: `DadosNovoDoente`, `DadosIdentificacao`, `DadosContactos`, `DadosMorada`,
  `DadosAlergia`, `DadosAntecedente`, `DetalheDoente`.
- **`registar_doente.go`** — `CasoRegistarDoente`: se `numProcesso` vazio →
  `repo.ProximoNumeroProcesso(ano)`; constrói agregado; `repo.Guardar`; audita
  `clinico.doente.registado`. Conflito de num_processo/BI → erro de conflito (→409).
- **`obter_doente.go`** — `CasoObterDoente` por ID e por num_processo; audita
  `clinico.doente.consultado` (acesso a dados de saúde).
- **`pesquisar_doentes.go`** — `CasoPesquisarDoentes`: delega `repo.Pesquisar(filtro)`; aplica
  limites por omissão/máximos ao `Limite`.
- **`actualizar_doente.go`** — `CasoActualizarDoente`: hidrata → aplica identificação/contactos/
  grupo sanguíneo → guarda → audita `clinico.doente.actualizado`.
- **`gerir_estado_doente.go`** — `CasoGerirEstadoDoente`: `Desactivar(motivo)` e
  `DeclararFalecido(data, causaCID)`; auditados (`clinico.doente.desactivado`,
  `clinico.doente.falecido`).
- **`registar_alergia.go` / `registar_antecedente.go`** — adicionam ao agregado e persistem;
  auditados (`clinico.alergia.registada`, `clinico.antecedente.registado`).

Testes com fakes (fake repo, fake auditor), gate aplicação ≥75%.

---

## Secção 4 — Adaptadores, HTTP/RBAC e plataforma

### `internal/adapters/pgrepo/doentes_repo.go`

Implementa `RepositorioDoentes` com pgx:

- **`Guardar`** — `INSERT ... RETURNING id` (novo, `id` vazio) ou `UPDATE` (existente) numa
  transacção; persiste filhos (alergias/antecedentes) por delete-and-reinsert dentro da mesma
  transacção. Traduz `unique_violation` do pg (SQLSTATE `23505`, BI ou num_processo) para
  `erros.CategoriaConflito`.
- **`ProximoNumeroProcesso(ctx, ano)`** —
  `INSERT INTO clinico.processo_sequencia(ano, ultimo) VALUES ($1, 1)
   ON CONFLICT (ano) DO UPDATE SET ultimo = clinico.processo_sequencia.ultimo + 1 RETURNING ultimo`;
  formata `P-{ano}-{ultimo:06d}`.
- **`ObterPorID` / `ObterPorNumProcesso`** — SELECT do doente + carregamento dos filhos.
- **`Pesquisar`** —
  `WHERE (nome_completo ILIKE '%'||$termo||'%' OR bi = $termo OR num_processo = $termo OR telefone = $termo)`
  com filtro de estado opcional, `ORDER BY nome_completo`, `LIMIT/OFFSET`, e `COUNT(*)` para o
  total. Sustentado pelo índice trigram.

### `internal/adapters/http/doente_handler.go`

Grupo `/api/v1/doentes`, todas as rotas com `Auth` + `RBAC`. DTOs de request/response em JSON
PT-PT; erros via `responderErro` (RFC 7807 PT-PT); anotações swag.

| Rota | Método | Acção | Papéis |
|---|---|---|---|
| `/api/v1/doentes` | POST | registar | Administrativo, Médico, Enfermeiro |
| `/api/v1/doentes` | GET | pesquisar | leitura ampla* |
| `/api/v1/doentes/:id` | GET | detalhe | leitura ampla* |
| `/api/v1/doentes/:id` | PATCH | actualizar identificação/contactos/grupo | Administrativo, Médico, Enfermeiro |
| `/api/v1/doentes/:id/estado` | POST | desactivar / declarar falecido | Administrativo, Médico, Enfermeiro |
| `/api/v1/doentes/:id/alergias` | POST | registar alergia | **Médico, Enfermeiro** |
| `/api/v1/doentes/:id/antecedentes` | POST | registar antecedente | **Médico, Enfermeiro** |

\* **leitura ampla** = Médico, Enfermeiro, Administrativo, Farmacêutico, TecnicoLab, Director,
DPO, Auditor.

### Plataforma

- **`config/config.go`** — sem novas variáveis obrigatórias.
- **`platform/app.go`** — instancia `pgrepo.NovoRepositorioDoentes(pool)`, os casos de uso, o
  handler, e regista o grupo no router. Reutiliza middleware Auth/RBAC/LimiteTaxa e o
  repositório de auditoria do M1.
- **`.go-arch-lint.yml`** — adiciona o componente `clinico` (domínio/aplicação/adapters) com as
  mesmas regras de dependência dos componentes de Identidade.

### ADR

- **`adrs/ADR-026-bc-clinico-doente.md`** — decisões: IDs gerados pela BD (pureza do domínio),
  número de processo híbrido (auto/manual, contador por ano), RBAC clínico (alergias/antecedentes
  só Médico/Enfermeiro) vs administrativo (demografia), diferimento LPDP e consentimentos.

---

## Testes & verificação (fim a fim)

1. `go build ./...` e `make test` verdes; `make cover` cumpre 85/75/60.
2. `make lint` (golangci + `go-arch-lint`) sem violações — confirma que `domain/clinico` não
   importa `pgx`/`gin`/`uuid`.
3. Migration `clinico/0001` aplica-se; `schema_migrations` populado.
4. Registar doente sem número → recebe `P-2026-000001`; registar com número manual → aceita e
   garante unicidade (segundo registo com o mesmo número → 409).
5. Pesquisa por parte do nome (trgm) devolve o doente; pesquisa por BI/num_processo/telefone
   exacta funciona; paginação respeita `limite`/`deslocamento`.
6. Adicionar alergia como Médico → 200; como Administrativo → 403 (RFC 7807 PT-PT).
7. Desactivar (com motivo) → estado `INACTIVO`; declarar falecido → `FALECIDO`; transições
   inválidas → erro de validação.
8. Cada operação de escrita/consulta gera evento em `auditoria.auditoria_eventos`.
9. `tests/integration/doentes_test.go` (tag `integration`) exercita o fluxo real contra a BD;
   SKIP (nunca FAIL) se `DATABASE_URL` não estiver definido.

## Fora de âmbito (fatias futuras)

- Apagamento LPDP (estado `APAGADO` + pseudonimização) e tabela `consentimentos` — fatia
  dedicada.
- `episodios_clinicos` — Sprint 8.
- Suporte a telefone fixo (o validador actual cobre apenas o móvel Angola +244 9XX).
