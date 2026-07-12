# Sprint 9 — BC Farmácia: Medicamento (catálogo) + Receita/Prescrição

**Marco:** M2 — Clínico Core (terceira fatia vertical)
**Data:** 2026-07-12
**Estado:** Aprovado para planeamento

## Contexto

O M2 entregou o Doente (Sprint 7) e o Episódio Clínico (Sprint 8), fundidos em `main`. O
DDM-001 coloca a **receita/prescrição no schema `farmacia`**, e cada item da receita referencia
um **catálogo de medicamentos**. A validação de alergias (RN-FAR-04) exige o nome do
medicamento para cruzar com as alergias do doente. Por isso o Sprint 9 introduz o **BC
Farmácia**, com dois agregados: **Medicamento** (catálogo) e **Receita** (prescrição emitida de
um episódio, com validação de alergias). A gestão de stock (lotes, FEFO, movimentos) e a
**dispensa** ficam para uma fatia seguinte.

**Fonte de verdade do modelo de dados:** DDM-001 v2.0 (extraído verbatim; não inventado).

## Decomposição do M2

- **Sprint 7 (concluído):** Doente + Alergia + AntecedenteClinico.
- **Sprint 8 (concluído):** EpisodioClinico + DiagnosticoCID + EHR.
- **Sprint 9 (esta fatia):** BC Farmácia — Medicamento (catálogo) + Receita/Prescrição.
- **Fatia seguinte (Farmácia stock/dispensa):** lotes, FEFO, movimentos, dispensa (RN-FAR-03/04/05).

## Princípios herdados (não-negociáveis)

- **Linguagem ubíqua PT-PT angolano** em TODO o output (código, comentários, commits, JSON,
  mensagens de erro). Nunca inglês nem PT-BR.
- **DDD + Clean Architecture**, dependência para dentro. `internal/domain/**` só stdlib +
  Shared Kernel; zero `pgx`/`gin`/`http`. **Sem `google/uuid` no domínio nem na aplicação.**
- **Sem `panic()`** fora de init. **Migrations forward-only**, sem `.down.sql`.
- **Erros RFC 7807** (`application/problem+json`, PT-PT) via `erros.Novo(categoria, msg)` +
  `responderErro`. Mensagens de erro do domínio são literais PT-PT (padrão do M2).
- **Conventional Commits** em PT-PT, terminando com o trailer
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- **Cobertura** (agregada por camada, `scripts/cobertura.sh`): domínio ≥85%, aplicação ≥75%,
  adapters ≥60%. O gate corre **sem** a tag `integration`.
- **Dados de saúde nunca são registados em log.** A escrita e a consulta individual de receitas
  são auditadas (dados de prescrição são de saúde). O catálogo de medicamentos **não** é dado
  pessoal — as suas leituras não são auditadas.
- **IDs** do domínio são `string`, gerados pela BD (`gen_random_uuid()` DEFAULT + `RETURNING`).

---

## Secção 1 — Arquitectura + modelo de dados

**Novo BC Farmácia** (`internal/{domain,application,adapters}/farmacia/`, schema `farmacia` já
existe). Dois agregados raiz: **Medicamento** (catálogo) e **Receita** (com itens filhos).
Reutiliza a infra do M1/M2: Auth/RBAC/LimiteTaxa, RFC 7807, repositório de auditoria.

### Fronteira Clínico↔Farmácia (anti-corrupção)

A receita referencia `episodio_id`/`doente_id`/`medico_id` do BC Clínico por **id (referência
fraca, sem FK cross-schema)**; só `itens_receita.medicamento_id` tem FK real (para
`farmacia.medicamentos`). Para não acoplar o domínio da Farmácia ao Clínico, a aplicação da
Farmácia define uma **porta de saída própria `LeitorClinico`** com o que precisa. Um adaptador
implementa-a reutilizando os repositórios `clinico` existentes — o domínio/aplicação da Farmácia
nunca importa o domínio do Clínico.

### Migration `migrations/farmacia/0001_medicamentos_receitas.sql`

Extraída verbatim do DDM-001. As tabelas `lotes`/`fornecedores`/`movimentos_stock` ficam fora de
âmbito (fatia de stock).

```sql
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

CREATE TABLE IF NOT EXISTS farmacia.receitas (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    episodio_id  uuid        NOT NULL,   -- ref clinico.episodios_clinicos (referência fraca)
    doente_id    uuid        NOT NULL,   -- ref clinico.doentes (referência fraca)
    medico_id    uuid        NOT NULL,
    emitida_em   timestamptz NOT NULL DEFAULT now(),
    estado       text        NOT NULL DEFAULT 'EMITIDA'
                 CHECK (estado IN ('EMITIDA','PARCIAL','DISPENSADA','EXPIRADA','ANULADA')),
    notas        text,
    expira_em    date        NOT NULL DEFAULT (CURRENT_DATE + INTERVAL '30 days')
);
CREATE INDEX IF NOT EXISTS idx_receitas_doente ON farmacia.receitas (doente_id, emitida_em DESC);
CREATE INDEX IF NOT EXISTS idx_receitas_episodio ON farmacia.receitas (episodio_id);

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

### Decisões transversais

- **Identidade:** `id` gerado pela BD; o domínio usa `string`.
- **`codigo_interno`:** `MED-{sequencial:05d}` via `SEQUENCE farmacia.seq_codigo_medicamento`
  (`nextval` atómico).
- **`medico_id`:** é o **actor autenticado** (o médico que prescreve) — não vem no corpo.
  `doente_id`/`episodio_id` são validados por leitura cross-BC (`LeitorClinico`).
- **`forma_farmaceutica`/`via_administracao`:** texto validado não-vazio (o DDM guarda-os como
  TEXT sem CHECK — não se inventa vocabulário fechado; fica texto livre até um vocabulário
  controlado futuro).
- Reutiliza middleware Auth/RBAC/LimiteTaxa, RFC 7807 e o repositório de auditoria do M1/M2.

---

## Secção 2 — Domínio (`internal/domain/farmacia/`)

Domínio rico, IDs `string`, mensagens de erro literais PT-PT.

### `medicamento.go` — agregado do catálogo

```go
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
```
- `NovoMedicamento(codigoInterno, nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante string, requerReceita, psicotropico bool, classeATC *string, stockMinimo int) (*Medicamento, error)` — codigoInterno, nome comercial/genérico, forma, dosagem e via obrigatórios (não-vazios), `stockMinimo ≥ 0`; `activo=true`.
- `Actualizar(nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante string, requerReceita, psicotropico bool, classeATC *string, stockMinimo int) error` — revalida os campos.
- `Activar()`, `Desactivar()` — alternam `activo` (soft-delete).
- `CorrespondeSubstancia(substancia string) bool` — verdadeiro se `substancia` (case-insensitive, aparada) estiver contida no nome genérico ou comercial.
- Getters `ID()`, `CodigoInterno()`, `Activo()`; `Snapshot() SnapshotMedicamento`; `ReconstruirMedicamento(SnapshotMedicamento) *Medicamento`.

### `receita.go` — agregado da prescrição

```go
type Receita struct {
    id         string
    episodioID string
    doenteID   string
    medicoID   string
    emitidaEm  time.Time
    estado     EstadoReceita
    notas      string
    expiraEm   time.Time // só data
    itens      []ItemReceita
}
type ItemReceita struct {
    medicamentoID        string
    posologia            string
    duracaoDias          *int
    quantidadePrescrita  int
    quantidadeDispensada int
    notas                string
}
```
- `NovoItemReceita(medicamentoID, posologia string, duracaoDias *int, quantidadePrescrita int, notas string) (ItemReceita, error)` — medicamentoID e posologia não-vazios, `quantidadePrescrita > 0`; `quantidadeDispensada = 0`.
- `NovaReceita(episodioID, doenteID, medicoID string, itens []ItemReceita, notas string, emitidaEm, expiraEm time.Time) (*Receita, error)` — os três ids obrigatórios; **≥1 item**; `expiraEm` posterior a `emitidaEm`; revalida cada item; estado inicial `EMITIDA`.
- `Anular() error` — só de `EMITIDA`/`PARCIAL` → `ANULADA` (senão CategoriaConflito).
- `EstadoEfectivo(agora time.Time) EstadoReceita` — se estado ∈ {EMITIDA, PARCIAL} e `expiraEm` já passou (comparação por data) → `EXPIRADA`; caso contrário o estado persistido. **Calculado na leitura, não persistido.**
- Getters `ID()`, `DoenteID()`, `Estado()`; `Snapshot() SnapshotReceita`; `ReconstruirReceita(SnapshotReceita) *Receita`.

### `enums.go`

`type EstadoReceita string`; consts `ReceitaEmitida="EMITIDA"`, `ReceitaParcial="PARCIAL"`,
`ReceitaDispensada="DISPENSADA"`, `ReceitaExpirada="EXPIRADA"`, `ReceitaAnulada="ANULADA"`.

### `eventos.go`

`MedicamentoRegistado`, `ReceitaEmitida`, `ReceitaAnulada` (sobre `shared/evento`, nomes
`farmacia.medicamento.registado`/`farmacia.receita.emitida`/`farmacia.receita.anulada`).

### `repositorio.go`

```go
type FiltroMedicamentos struct { Termo string; ApenasActivos bool; Limite, Deslocamento int }
type ResumoMedicamento struct { /* json */ ID, CodigoInterno, NomeComercial, NomeGenerico, FormaFarmaceutica, Dosagem string; Activo bool }
type PaginaMedicamentos struct { /* json */ Itens []ResumoMedicamento; Total, Limite, Deslocamento int }
type RepositorioMedicamentos interface {
    Guardar(ctx, *Medicamento) (string, error)
    ObterPorID(ctx, id string) (*Medicamento, error)
    Pesquisar(ctx, FiltroMedicamentos) (PaginaMedicamentos, error)
    ProximoCodigo(ctx) (string, error) // "MED-00001"
}

type FiltroReceitas struct { DoenteID, EpisodioID, Estado string; Limite, Deslocamento int }
type ResumoReceita struct { /* json */ ID, EpisodioID, MedicoID string; EmitidaEm time.Time; Estado string; ExpiraEm time.Time; NumItens int }
type PaginaReceitas struct { /* json */ Itens []ResumoReceita; Total, Limite, Deslocamento int }
type RepositorioReceitas interface {
    Guardar(ctx, *Receita) (string, error)
    ObterPorID(ctx, id string) (*Receita, error)
    ListarPorDoente(ctx, FiltroReceitas) (PaginaReceitas, error)
}
```

---

## Secção 3 — Aplicação + validação de alergias

Casos de uso em `internal/application/farmacia/`, sobre portas, relógio injectado.

### Portas (`ports.go`)

- `Auditor` (padrão do M1/M2).
- Porta anti-corrupção `LeitorClinico`:
  ```go
  type AlergiaClinica struct { Substancia, Severidade string }
  type LeitorClinico interface {
      ObterContextoDoente(ctx context.Context, doenteID string) (activo bool, alergiasGraves []AlergiaClinica, err error)
      EpisodioDoDoente(ctx context.Context, episodioID, doenteID string) (bool, error)
  }
  ```
- Reexports `FiltroMedicamentos`/`PaginaMedicamentos`/`ResumoMedicamento` e
  `FiltroReceitas`/`PaginaReceitas`/`ResumoReceita`.
- DTOs: `DadosNovoMedicamento`, `DadosActualizarMedicamento`, `DetalheMedicamento`;
  `DadosNovaReceita{ EpisodioID, DoenteID string; Itens []DadosItemReceita; Notas string; IgnorarAlertaAlergia bool; JustificacaoAlerta string }`,
  `DadosItemReceita{ MedicamentoID, Posologia string; DuracaoDias *int; QuantidadePrescrita int; Notas string }`,
  `DadosAnularReceita{ Motivo string }`, `DetalheReceita` (+ `ItemReceitaDTO`).

### Medicamento

- `CasoRegistarMedicamento` — gera `codigo_interno` via `repo.ProximoCodigo`; `NovoMedicamento`;
  `Guardar`; audita `farmacia.medicamento.registado`.
- `CasoActualizarMedicamento` — hidrata, `Actualizar`, `Guardar`, audita.
- `CasoDefinirEstadoMedicamento` — `Activar`/`Desactivar`, `Guardar`, audita.
- `CasoObterMedicamento`, `CasoPesquisarMedicamentos` — **não** auditam (catálogo não é dado
  pessoal); pesquisa normaliza limites (default 20, máximo 100).

### Receita

- `CasoEmitirReceita.Executar(ctx, actor string, dados DadosNovaReceita) (DetalheReceita, error)`
  — `medico_id` = `actor`:
  1. `LeitorClinico.ObterContextoDoente(doenteID)` → não existe/activo → `CategoriaConflito`;
     retém as alergias graves.
  2. `LeitorClinico.EpisodioDoDoente(episodioID, doenteID)` → falso → `CategoriaValidacao`.
  3. Para cada item: `repoMedicamentos.ObterPorID(medicamentoID)` (tem de existir e estar
     `activo`; senão `CategoriaNaoEncontrado`/`CategoriaConflito`); constrói `ItemReceita`.
  4. **Alergias:** para cada medicamento × cada alergia grave,
     `medicamento.CorrespondeSubstancia(a.Substancia)` → acumula alertas.
  5. Se há alertas **e** `!IgnorarAlertaAlergia` → `erros.Novo(CategoriaRegraNegocio, <lista>)`
     (422). Se `IgnorarAlertaAlergia` → exige `JustificacaoAlerta` não-vazia (senão
     `CategoriaValidacao`) e prossegue.
  6. `NovaReceita` (emitidaEm=`agora()`, expiraEm=`agora()+30 dias`); `Guardar`; audita
     `farmacia.receita.emitida` (com a justificação de override + alertas no `Detalhe`, quando
     aplicável).
- `CasoAnularReceita` — hidrata, `Anular`, `Guardar`, audita `farmacia.receita.anulada`
  (motivo em `Detalhe`).
- `CasoObterReceita` — audita `farmacia.receita.consultada`; devolve o **estado efectivo**
  (`EstadoEfectivo(agora)`).
- `CasoListarReceitas` — por doente, filtro episódio/estado + paginação; **não** audita.

### Shared Kernel

Acrescenta a categoria `CategoriaRegraNegocio` a `erros` → mapeada em `problem.go` para **422**
(Unprocessable Entity) com mensagem i18n `MsgRegraNegocio` — para violações de regra de negócio
(bloqueio de alergia agora; FEFO/stock no futuro).

---

## Secção 4 — Adaptadores, HTTP/RBAC e plataforma

### Adaptadores

- `internal/adapters/pgrepo/medicamentos_repo.go` — `Guardar` (INSERT `RETURNING id::text` /
  UPDATE numa transacção); `ObterPorID`; `Pesquisar` (`WHERE (nome_comercial||' '||nome_generico)
  ILIKE '%'||termo||'%'` sustentado pelo índice trigram, filtro `activo`, `COUNT` + `LIMIT/OFFSET`);
  `ProximoCodigo` (`SELECT nextval('farmacia.seq_codigo_medicamento')` → `fmt.Sprintf("MED-%05d", n)`);
  `codigo_interno` duplicado (23505) → `CategoriaConflito`.
- `internal/adapters/pgrepo/receitas_repo.go` — `Guardar` (receita + itens por
  delete-and-reinsert numa transacção); `ObterPorID` (receita + itens); `ListarPorDoente` (filtro
  doente/episódio/estado, `ORDER BY emitida_em DESC`, `COUNT` + `LIMIT/OFFSET`). Texto opcional com
  `NULLIF(...,'')` na escrita e `COALESCE(...,'')` na leitura (lição do M2); `duracao_dias`/`fim`
  anuláveis via ponteiro.
- `internal/adapters/farmacia/leitor_clinico.go` — implementa `LeitorClinico` reutilizando
  `pgrepo.RepositorioDoentes`/`RepositorioEpisodios`: `ObterContextoDoente` (activo = estado
  `ACTIVO`; filtra alergias `GRAVE`/`ANAFILACTICA` → `AlergiaClinica`); `EpisodioDoDoente`
  (existe + `DoenteID == doenteID`).

### `internal/adapters/http/farmacia_handler.go`

Grupo `/api/v1/farmacia`, com `Auth` + rate limit. DTOs JSON PT-PT; erros via `responderErro`;
`actor = SessaoDe(c).Sujeito` (também `medico_id` da receita).

| Rota | Método | Acção | Papéis |
|---|---|---|---|
| `/api/v1/farmacia/medicamentos` | POST | registar | Farmacêutico, FarmacêuticoSenior |
| `/api/v1/farmacia/medicamentos` | GET | pesquisar | leitura ampla* |
| `/api/v1/farmacia/medicamentos/:id` | GET | detalhe | leitura ampla* |
| `/api/v1/farmacia/medicamentos/:id` | PATCH | actualizar | Farmacêutico, FarmacêuticoSenior |
| `/api/v1/farmacia/medicamentos/:id/activar` | POST | activar | Farmacêutico, FarmacêuticoSenior |
| `/api/v1/farmacia/medicamentos/:id/desactivar` | POST | desactivar | Farmacêutico, FarmacêuticoSenior |
| `/api/v1/farmacia/receitas` | POST | emitir | **só Médico** |
| `/api/v1/farmacia/receitas` | GET | listar por doente | leitura ampla* |
| `/api/v1/farmacia/receitas/:id` | GET | detalhe | leitura ampla* |
| `/api/v1/farmacia/receitas/:id/anular` | POST | anular | **só Médico** |

\* **leitura ampla** = Médico, Enfermeiro, Farmacêutico, FarmacêuticoSenior, Director, DPO,
Auditor. Bloqueio de alergia → **422** com a lista de conflitos; override via
`ignorar_alerta_alergia` + `justificacao_alerta` no corpo do POST de emissão.

### Plataforma & docs

- `internal/domain/shared/erros/erros.go` — nova categoria `CategoriaRegraNegocio`.
- `internal/domain/shared/i18n/i18n.go` — nova chave `MsgRegraNegocio`.
- `internal/adapters/http/problem.go` — `CategoriaRegraNegocio` → 422 + título i18n + type.
- `internal/platform/app.go` — instancia `pgrepo.NovoRepositorioMedicamentos(pool)`,
  `NovoRepositorioReceitas(pool)`, o `LeitorClinico` (sobre os repos `clinico` já criados), os
  casos de uso e o handler; regista o grupo com `limiteMW, authMW` (sem MFA).
- `migrations/embed.go` — passa a incluir `farmacia` (ordem alfabética:
  `auditoria clinico farmacia identidade shared`); actualiza a lista de `embed_test.go`.
- `.go-arch-lint.yml` inalterado (componentes por caminho).
- `adrs/ADR-028-bc-farmacia-receita.md` — decisões: novo BC Farmácia, porta anti-corrupção
  `LeitorClinico`, `codigo_interno` por SEQUENCE, validação de alergias com override auditado,
  matching textual case-insensitive, categoria 422 `RegraNegocio`, estado EXPIRADA calculado na
  leitura; diferimentos (stock/lotes/FEFO/dispensa, fornecedores, venda directa, psicotrópicos,
  batch de expiração).

## Testes & verificação (fim a fim)

1. `go build ./...` e `make test` verdes; `make cover` cumpre 85/75/60.
2. `make lint` sem violações; `domain/farmacia` não importa `pgx`/`gin`/`uuid` nem o domínio
   `clinico`.
3. Migration `farmacia/0001` aplica-se; `schema_migrations` regista `farmacia/0001_...`.
4. Registar medicamento → recebe `MED-00001`; código duplicado → 409.
5. Pesquisar medicamento por parte do nome (trgm) → 200; filtro `apenas_activos`.
6. Emitir receita a doente com alergia GRAVE ao medicamento **sem** override → 422 (RFC 7807
   PT-PT com a lista); **com** `ignorar_alerta_alergia=true` + justificação → 201 + auditoria do
   override; sem alergia → 201.
7. Emitir a doente inexistente/inactivo → 409; episódio de outro doente → 400; medicamento
   inexistente/inactivo → 404/409; item com quantidade ≤ 0 → 400.
8. Anular receita EMITIDA → 200 + ANULADA; anular já anulada → 409.
9. Obter receita expirada (expira_em < hoje) → estado efectivo EXPIRADA.
10. RBAC: emitir/anular como Farmacêutico → 403; registar medicamento como Médico → 403; ler
    catálogo/receitas como leitura ampla → 200.
11. Cada escrita e consulta individual de receita gera evento em `auditoria.auditoria_eventos`;
    leituras do catálogo não geram.
12. `tests/integration/receitas_test.go` (tag `integration`): registar medicamento → criar
    doente+alergia grave+episódio → emitir bloqueada → emitir com override → anular; SKIP sem
    `DATABASE_URL`.

## Fora de âmbito (fatias futuras)

- **Stock:** lotes, FEFO (RN-FAR-03), movimentos de stock, fornecedores, entrada de stock,
  alertas de stock mínimo.
- **Dispensa** (UC-FAR-02, RN-FAR-04/05) — decremento por FEFO, estados PARCIAL/DISPENSADA.
- **Venda directa** OTC (UC-FAR-09) e a integração com Facturação.
- **Psicotrópicos** (RN-FAR-06) — registo especial adicional.
- **Batch de expiração** (transição automática EMITIDA→EXPIRADA persistida) — nesta fatia a
  expiração é calculada na leitura.
- Vocabulário controlado de forma farmacêutica / via de administração.
