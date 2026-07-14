# Sprint 11 — Cirurgia Ambulatória + Consentimento (LPDP) — Design

**Marco:** M2 (Clínico Core). Fecha o critério de saída M2 relativo a cirurgia
ambulatória (ADR-018 pt2).

**Objectivo:** implementar, no BC Clínico e apenas backend/API:

1. O agregado **Consentimento** (LPDP) — dependência crítica nunca construída
   (`procedimentos_cirurgicos.consentimento_id` referencia-o, e a invariante-estrela
   da cirurgia é "consentimento com anexo obrigatório"). Ciclo LPDP completo.
2. O tipo de episódio **`CIRURGIA_AMBULATORIA`**.
3. O agregado **`ProcedimentoCirurgico`** com state machine
   (AGENDADO → EM_CURSO → CONCLUIDO/CANCELADO) e invariantes de domínio.
4. O **catálogo de procedimentos** (seed PRC001–PRC007).

**Fonte de verdade (nada inventado):**
- **ADR-018 pt2** — decisão da cirurgia ambulatória.
- **DDM-001 v2.0** — schema da tabela `clinico.consentimentos`.
- **DDM-001 v2.1 Adenda §4** — schemas `procedimentos_cirurgicos`,
  `catalogo_procedimentos`, seed, e o modelo de domínio Go de referência.
- **M2 milestone doc** — invariantes obrigatórias da cirurgia.

**Stack / convenções não-negociáveis (herdadas):** Go/Gin, pgx v5 SQL puro,
PostgreSQL 16 (schema `clinico`), DDD + Clean Architecture (Domínio sem infra),
tudo em **PT-PT angolano**, IDs `string` gerados pela BD (`gen_random_uuid()` +
`RETURNING id::text`), migrações forward-only, erros via
`erros.Novo(categoria, msg)` com mensagens literais PT-PT, HTTP via RFC 7807
(`responderErro`), Conventional Commits PT-PT terminados **exactamente** com
`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
Gates de cobertura: domínio ≥85%, aplicação ≥75%, adaptadores ≥60% (pgrepo
excluído do gate unitário de adaptadores, testado por integração).

---

## Decisões fixadas (com o utilizador)

- **Consentimento — ciclo LPDP completo:** tabela + agregado + conceder/revogar +
  listar por doente (filtros por finalidade e estado) + endpoints próprios +
  auditoria de cada alteração.
- **Cancelamento — DDM estrito:** a CHECK do DDM obriga `CANCELADO` a ter `inicio`
  não-nulo. Logo `Cancelar` só transita **EM_CURSO → CANCELADO** (cancelamento
  intra-operatório). Um procedimento AGENDADO que não se realiza resolve-se
  cancelando o episódio (fatia existente). A CHECK do DDM fica intacta.

## Desvios ao blueprint (registados no ADR-030)

1. **Pacote plano:** `ProcedimentoCirurgico` e `Consentimento` ficam no pacote
   `internal/domain/clinico` (como `episodio`/`doente`), não num subpacote
   `clinico/cirurgia`. Consistência com a base de código existente; evita o
   acoplamento pai↔filho de pacotes.
2. **Erros idiomáticos:** `erros.Novo(categoria, msg)` PT-PT em vez de sentinelas
   `ErrConsentimentoCirurgicoInvalido` do exemplo do DDM.
3. **Sem `ON DELETE CASCADE`:** o DDM v2.0 declara `consentimentos.doente_id ...
   ON DELETE CASCADE`, mas o doente é soft-delete (nunca se apaga a linha); a FK
   fica sem cascade, como as restantes FKs de `clinico`.
4. **Sem FK `codigo_procedimento` → catálogo:** o DDM não a declara; a validação do
   código contra o catálogo (existência + activo) faz-se na aplicação, o que
   também fornece a flag `requer_anestesista`.

---

## Secção 1 — Migrações (`migrations/clinico/`, forward-only)

### 0003_consentimentos.sql
Schema exacto do DDM-001 v2.0, schema-qualificado e adaptado às convenções:

```sql
CREATE TABLE IF NOT EXISTS clinico.consentimentos (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id     uuid        NOT NULL REFERENCES clinico.doentes(id),
    finalidade    text        NOT NULL CHECK (finalidade IN
                    ('TRATAMENTO','COMUNICACAO','PARTILHA_SEGURADORA','INVESTIGACAO','CIRURGIA')),
    concedido     boolean     NOT NULL,
    documento_url text,        -- referência ao anexo digitalizado (obrigatório p/ CIRURGIA, imposto no domínio)
    concedido_em  date        NOT NULL,
    revogado_em   date,
    criado_em     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_consentimentos_doente
    ON clinico.consentimentos (doente_id, concedido_em DESC);
```

### 0004_tipo_episodio_cirurgia.sql
Estende a CHECK de `episodios_clinicos.tipo` (que hoje admite
CONSULTA/URGENCIA/INTERNAMENTO) para incluir `CIRURGIA_AMBULATORIA`:

```sql
ALTER TABLE clinico.episodios_clinicos DROP CONSTRAINT IF EXISTS episodios_clinicos_tipo_check;
ALTER TABLE clinico.episodios_clinicos ADD CONSTRAINT episodios_clinicos_tipo_check
    CHECK (tipo IN ('CONSULTA','URGENCIA','INTERNAMENTO','CIRURGIA_AMBULATORIA'));
```
(O nome exacto da constraint será confirmado durante a implementação via `\d`; se
diferente, ajustar o `DROP`.)

### 0005_catalogo_procedimentos.sql
Tabela de dados de referência + seed PRC001–PRC007 (excerto do DDM v2.1 §4.3):

```sql
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

### 0006_procedimentos_cirurgicos.sql
Schema exacto do DDM v2.1 §4.2 (todas as CHECK preservadas):

```sql
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

`cirurgiao_id`/`auxiliar_id`/`anestesista_id` são `keycloak_id` (identidade); sem FK
a `identidade.utilizadores` (BC diferente; o DDM referencia mas mantemos o
identificador opaco, como `medico_id` no episódio).

---

## Secção 2 — Domínio (`internal/domain/clinico/`)

Domínio rico, IDs `string`, mensagens literais PT-PT, erros por `erros.Novo`.

### consentimento.go
```go
type Finalidade string
const (
    FinalidadeTratamento         Finalidade = "TRATAMENTO"
    FinalidadeComunicacao        Finalidade = "COMUNICACAO"
    FinalidadePartilhaSeguradora Finalidade = "PARTILHA_SEGURADORA"
    FinalidadeInvestigacao       Finalidade = "INVESTIGACAO"
    FinalidadeCirurgia           Finalidade = "CIRURGIA"
)
func ParseFinalidade(codigo string) (Finalidade, error) // valida, normaliza maiúsculas

type Consentimento struct { /* campos privados */ }

// NovoConsentimento valida e constrói. Regras:
//  - doenteID obrigatório; finalidade válida; concedidoEm não-zero.
//  - CIRURGIA exige documentoURL não-vazio E concedido=true (não há cirurgia sem
//    consentimento concedido e anexado) → erros.CategoriaRegraNegocio.
func NovoConsentimento(doenteID string, f Finalidade, concedido bool, documentoURL string, concedidoEm time.Time) (*Consentimento, error)

func (c *Consentimento) ID() string
func (c *Consentimento) DoenteID() string
func (c *Consentimento) Finalidade() Finalidade
func (c *Consentimento) TemAnexo() bool         // documentoURL != ""
func (c *Consentimento) EstaVigente() bool       // concedido && revogadoEm == nil
func (c *Consentimento) Revogar(em time.Time) error // erro se não concedido ou já revogado (CategoriaConflito)

type SnapshotConsentimento struct { /* ... campos exportados ... */ }
func (c *Consentimento) Snapshot() SnapshotConsentimento
func ReconstruirConsentimento(s SnapshotConsentimento) *Consentimento
```

### anestesia.go
```go
type Anestesia string
const (
    AnestesiaNenhuma       Anestesia = "NENHUMA"
    AnestesiaLocal         Anestesia = "LOCAL"
    AnestesiaSedacaoLigeira Anestesia = "SEDACAO_LIGEIRA"
    AnestesiaLocoRegional  Anestesia = "LOCO_REGIONAL"
)
func ParseAnestesia(codigo string) (Anestesia, error)
func (a Anestesia) RequerAnestesista() bool // != NENHUMA
```

### procedimento_enums.go
```go
type EstadoProcedimento string
const (
    ProcAgendado  EstadoProcedimento = "AGENDADO"
    ProcEmCurso   EstadoProcedimento = "EM_CURSO"
    ProcConcluido EstadoProcedimento = "CONCLUIDO"
    ProcCancelado EstadoProcedimento = "CANCELADO"
)
```

### procedimento_cirurgico.go
```go
type ProcedimentoCirurgico struct { /* campos privados */ }

// DadosNovoProcedimento agrupa os parâmetros de construção (evita assinatura longa).
type DadosNovoProcedimento struct {
    EpisodioID   string
    Codigo       string
    Descricao    string
    Sala         string // opcional
    CirurgiaoID  string
    AuxiliarID   string // opcional
    Anestesia    Anestesia
    AnestesistaID string // opcional, mas obrigatório se Anestesia.RequerAnestesista()
    Observacoes  string
}

// NovoProcedimento valida invariantes e devolve o agregado em AGENDADO.
// Recebe o Consentimento (não só o id) para validar a invariante-estrela.
// Invariantes:
//   - EpisodioID, Codigo, Descricao, CirurgiaoID obrigatórios (CategoriaValidacao)
//   - Anestesia válida; se RequerAnestesista() ⇒ AnestesistaID obrigatório (CategoriaValidacao)
//   - consentimento.Finalidade == CIRURGIA && TemAnexo() && EstaVigente()
//       senão CategoriaRegraNegocio ("consentimento cirúrgico inválido ou em falta")
//   - consentimento.DoenteID deve corresponder ao doente do episódio → validado na aplicação
func NovoProcedimento(d DadosNovoProcedimento, consentimento *Consentimento) (*ProcedimentoCirurgico, error)

func (p *ProcedimentoCirurgico) Iniciar(em time.Time) error
//   AGENDADO → EM_CURSO; senão CategoriaConflito. Define inicio.
func (p *ProcedimentoCirurgico) Concluir(em time.Time, complicacoes, observacoes string) error
//   EM_CURSO → CONCLUIDO; em >= inicio senão CategoriaValidacao ("fim antes do início"). Define fim.
func (p *ProcedimentoCirurgico) Cancelar(em time.Time, motivo string) error
//   EM_CURSO → CANCELADO (DDM estrito); em >= inicio. Define fim. motivo → observacoes/auditoria.

// getters: ID, EpisodioID, Estado, ConsentimentoID, ...
type SnapshotProcedimento struct { /* campos exportados, incl. *time.Time Inicio/Fim */ }
func (p *ProcedimentoCirurgico) Snapshot() SnapshotProcedimento
func ReconstruirProcedimento(s SnapshotProcedimento) *ProcedimentoCirurgico
```

Evento `ProcedimentoCirurgicoConcluido` (em `eventos.go`): definido para consumo
futuro por Financeiro/reporting — **scaffolding** (não emitido), coerente com os
eventos de Sprint 9/10.

### catalogo_procedimento.go
```go
type CatalogoProcedimento struct {
    Codigo             string
    Descricao          string
    Especialidade      string
    DuracaoEstimadaMin int
    RequerAnestesista  bool
    Activo             bool
}
```
(Read model; a validação de existência/activo faz-se na aplicação.)

### episodio_enums.go (estender)
Acrescentar `EpisodioCirurgiaAmbulatoria TipoEpisodio = "CIRURGIA_AMBULATORIA"` a
`tiposEpisodioValidos` e à mensagem de erro de `ParseTipoEpisodio`.

### repositorio.go (estender) — interfaces de saída
```go
type RepositorioConsentimentos interface {
    Guardar(ctx, *Consentimento) (id string, err error)
    ObterPorID(ctx, id string) (*Consentimento, error)
    ListarPorDoente(ctx, doenteID string, filtro FiltroConsentimentos) ([]ResumoConsentimento, error)
}
type RepositorioProcedimentos interface {
    Guardar(ctx, *ProcedimentoCirurgico) (id string, err error)
    ObterPorID(ctx, id string) (*ProcedimentoCirurgico, error)
    ListarPorEpisodio(ctx, episodioID string) ([]ResumoProcedimento, error)
}
type RepositorioCatalogoProcedimentos interface {
    ObterPorCodigo(ctx, codigo string) (*CatalogoProcedimento, error) // NaoEncontrado se ausente
}
type FiltroConsentimentos struct { Finalidade string; ApenasVigentes bool }
type ResumoConsentimento struct { /* json */ ID, DoenteID, Finalidade string; Concedido bool; DocumentoURL string; ConcedidoEm time.Time; RevogadoEm *time.Time; Vigente bool }
type ResumoProcedimento struct { /* json */ ID, EpisodioID, Codigo, Descricao, Estado, Anestesia string; Inicio, Fim *time.Time; CriadoEm time.Time }
```

---

## Secção 3 — Aplicação (`internal/application/clinico/`)

Novas portas em `ports.go` (reexports dos read-models + tipos de entrada/DTO).
Reutiliza `RepositorioDoentes`, `RepositorioEpisodios`, `Auditor`.

### Consentimento
- **`registar_consentimento.go`** — `CasoRegistarConsentimento`: valida doente
  existe; `NovoConsentimento`; `Guardar`; audita `clinico.consentimento.registado`.
- **`revogar_consentimento.go`** — `CasoRevogarConsentimento`: `ObterPorID`;
  `Revogar(agora)`; `Guardar`; audita `clinico.consentimento.revogado`.
- **`listar_consentimentos.go`** — `CasoListarConsentimentos` por doente com
  `FiltroConsentimentos` (não audita — leitura).
- **`obter_consentimento.go`** — `CasoObterConsentimento` (detalhe; não audita).

### Cirurgia
- **`agendar_procedimento.go`** — `CasoAgendarProcedimento`:
  1. `ObterPorID` do episódio; deve existir, `Tipo == CIRURGIA_AMBULATORIA` e
     estado ABERTO (senão `CategoriaConflito`).
  2. `catalogo.ObterPorCodigo`; deve existir e `Activo` (senão
     `CategoriaValidacao`/`NaoEncontrado`).
  3. `consentimentos.ObterPorID`; valida `DoenteID == episodio.DoenteID`.
  4. Se `catalogo.RequerAnestesista` ⇒ exige `AnestesistaID` (reforço além do
     domínio, que já exige por `Anestesia != NENHUMA`).
  5. `NovoProcedimento(dados, consentimento)`; `Guardar`; audita
     `clinico.procedimento.agendado`.
- **`iniciar_procedimento.go`** — `Iniciar`; audita `clinico.procedimento.iniciado`.
- **`concluir_procedimento.go`** — `Concluir`; audita
  `clinico.procedimento.concluido`. (Evento scaffolding não emitido.)
- **`cancelar_procedimento.go`** — `Cancelar`; audita
  `clinico.procedimento.cancelado` (motivo em `Detalhe`).
- **`obter_procedimento.go`** / **`listar_procedimentos.go`** — leitura por id / por
  episódio.

Mapeadores `mapa_*` para os DTOs (padrão `paraDetalheEpisodio`).

---

## Secção 4 — Adaptadores

### pgrepo
- **`consentimentos_repo.go`** — `Guardar` (`INSERT ... RETURNING id::text`),
  `ObterPorID` (reconstrói via `ReconstruirConsentimento`), `ListarPorDoente`
  (filtros; `revogado_em IS NULL` para vigentes). Datas `date` lidas/escritas como
  `time.Time`.
- **`procedimentos_repo.go`** — `Guardar` faz `INSERT` no primeiro persist e, para
  agregados já com id (transições de estado), `UPDATE` do estado/inicio/fim/
  complicacoes/observacoes guardado por `id`. `ObterPorID`/`ListarPorEpisodio`
  reconstroem via `ReconstruirProcedimento`.
- **`catalogo_procedimentos_repo.go`** — `ObterPorCodigo` (SELECT; `NaoEncontrado`
  se ausente).
- Excluídos do gate unitário de adaptadores (integração-only), como os demais
  pgrepo.

### http
- **`consentimento_handler.go`** — corpos de pedido/resposta com tags JSON PT-PT;
  `responderErro` (RFC 7807). Rotas registadas por
  `RegistarConsentimentos(r, h, protecao...)`.
- **`cirurgia_handler.go`** — idem, `RegistarCirurgia(r, h, protecao...)`.
- **RBAC:** escritas de cirurgia (agendar/iniciar/concluir/cancelar) =
  `PapelMedico`; consentimento (registar/revogar) = `PapelMedico` +
  `PapelAdministrativo`; leituras = leque clínico
  (Médico/Enfermeiro/Administrativo). Alinhado com os handlers de episódio/doente.

Rotas sob `/api/v1/clinico`:
```
POST   /doentes/:id/consentimentos          registar consentimento
GET    /doentes/:id/consentimentos          listar (query: finalidade, vigentes)
POST   /consentimentos/:id/revogar          revogar
GET    /consentimentos/:id                  detalhe
POST   /episodios/:id/procedimentos         agendar procedimento
GET    /episodios/:id/procedimentos         listar por episódio
GET    /procedimentos/:id                   detalhe
POST   /procedimentos/:id/iniciar           iniciar (AGENDADO→EM_CURSO)
POST   /procedimentos/:id/concluir          concluir (EM_CURSO→CONCLUIDO)
POST   /procedimentos/:id/cancelar          cancelar (EM_CURSO→CANCELADO)
```

---

## Secção 5 — Plataforma

- **`app.go`** — instanciar os 3 repos novos sobre o pool; os casos de uso; os 2
  handlers; registar `RegistarConsentimentos` e `RegistarCirurgia` com os
  middlewares (limite + auth). Embed automático das novas migrações `clinico/`
  (o runner já percorre a pasta).
- **`adrs/ADR-030-cirurgia-ambulatoria-consentimento.md`** — decisões: dependência
  Consentimento, state machine DDM-estrita, invariante consentimento CIRURGIA+anexo,
  os 4 desvios ao blueprint, RBAC.

---

## Secção 6 — Testes (gates 85/75/60)

- **Domínio:** `consentimento_test.go` (CIRURGIA exige anexo+concedido; revogar;
  vigência), `anestesia_test.go` (RequerAnestesista), `procedimento_cirurgico_test.go`
  (construtor: anestesista obrigatório; consentimento inválido bloqueia; state
  machine completa incl. transições inválidas → Conflito; fim≥inicio),
  `episodio_enums_test.go` (novo tipo válido).
- **Aplicação:** casos com fakes (fake repos de consentimento/procedimento/catálogo/
  doente/episódio + fake Auditor). Cobrir: agendar bloqueia episódio não-cirúrgico,
  catálogo inactivo, consentimento de doente errado, requer_anestesista; ciclo
  agendar→iniciar→concluir; revogar consentimento; asserção dos eventos de auditoria.
- **Adaptadores:** handlers via `httptest` + fakes (401/403/estados; mapeamento
  RFC 7807); ratelimit já coberto.
- **Integração** (`tests/integration/`, tag `integration`, SKIP sem `DATABASE_URL`):
  `cirurgia_test.go` — aplicar migrações; criar doente+episódio CIRURGIA_AMBULATORIA;
  registar consentimento CIRURGIA com documento_url; agendar→iniciar→concluir; e o
  caminho negativo (agendar sem consentimento válido → RegraNegocio; sem anexo →
  RegraNegocio). `consentimento_test.go` — ciclo registar/listar/revogar.

## Verificação (fim a fim)

1. `go build ./...` + `go test ./...` verdes; `-race` limpo.
2. `scripts/cobertura.sh` cumpre 85/75/60.
3. `staticcheck`/`go vet`/`gofmt` limpos; `go-arch-lint` sem violações (domínio sem
   infra; farmácia intocada).
4. Integração contra PG real: migrações aplicam; ciclo cirúrgico completo persiste
   com os estados/timestamps coerentes; bloqueios de consentimento devolvem 422.

## Fora de âmbito (fatias futuras)

- Upload binário do anexo (MinIO); hoje `documento_url` é referência textual.
- Integração com Facturação (linha por procedimento — evento consumido em M4).
- Relatório MINSA de contagem de cirurgias.
- Cancelar procedimento AGENDADO (modelo DDM não o permite; resolve-se no episódio).
- Subpacote `clinico/cirurgia` (mantido plano).
