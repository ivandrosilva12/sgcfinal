# Sprint 8 — BC Clínico: agregado Episódio Clínico (+ EHR de leitura)

**Marco:** M2 — Clínico Core (segunda fatia vertical)
**Data:** 2026-07-12
**Estado:** Aprovado para planeamento

## Contexto

O Sprint 7 entregou o agregado **Doente** (+ Alergia + AntecedenteClinico) do domínio ao
HTTP, fundido em `main`. O Sprint 8 acrescenta o segundo agregado do BC Clínico — o
**EpisodioClinico** — cobrindo o ciclo de vida clínico (iniciar → actualizar nota → fechar →
cancelar), a listagem por doente, e uma projecção de leitura **EHR** (registo clínico
electrónico: identificação + alergias + antecedentes + episódios paginados).

**Fonte de verdade do modelo de dados:** DDM-001 v2.0 (extraído verbatim; não inventado).

## Decomposição do M2

- **Sprint 7 (concluído):** Doente + Alergia + AntecedenteClinico.
- **Sprint 8 (esta fatia):** EpisodioClinico + DiagnosticoCID + vista EHR.
- **Sprint 9:** Receita / Prescrição (+ validação contra alergias do doente).
- **Sprint 10:** Cirurgia ambulatória.

## Princípios herdados (não-negociáveis)

- **Linguagem ubíqua PT-PT angolano** em TODO o output (código, comentários, commits, JSON,
  mensagens de erro). Nunca inglês nem PT-BR.
- **DDD táctico + Clean Architecture**, dependência para dentro. `internal/domain/**` importa
  apenas stdlib + Shared Kernel; zero `pgx`/`gin`/`http`/`oidc`.
- **Sem `google/uuid` no domínio nem na aplicação** — só permitido em adapters (arch-lint).
- **Sem `panic()`** fora de inicialização.
- **Migrations forward-only**, sem `.down.sql`.
- **Erros RFC 7807** (`application/problem+json`, PT-PT) via `erros.Novo(categoria, msg)` +
  `responderErro`. Mensagens de erro do domínio são literais PT-PT (padrão do Sprint 7).
- **Conventional Commits** em PT-PT, terminando com o trailer
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- **Cobertura** (agregada por camada, `scripts/cobertura.sh`): domínio ≥85%, aplicação ≥75%,
  adapters ≥60%. O gate corre **sem** a tag `integration`.
- **Dados de saúde e identificadores nunca são registados em log.** Acessos a dados clínicos
  (episódio individual, EHR) são auditados.
- **IDs** do domínio são `string`, gerados pela BD (`gen_random_uuid()` DEFAULT + `RETURNING`).

---

## Secção 1 — Arquitectura + modelo de dados

**Agregado raiz independente.** Apesar de o DDM-001 lhe chamar "sub-agregado" do Doente, o
`EpisodioClinico` é modelado como **agregado raiz independente** dentro do mesmo BC Clínico,
com o seu próprio repositório, referenciando `doente_id`. Justificação: os episódios crescem
sem limite (carregá-los no agregado Doente seria pesado), têm ciclo de vida próprio, endpoints
próprios (`/api/v1/episodios/:eid`), e a FK do DDM é **sem `ON DELETE CASCADE`** (os episódios
sobrevivem à pseudonimização do doente — integridade clínica/fiscal). O **EHR** é uma
projecção de leitura em runtime (não é entidade), combinando doente + alergias + antecedentes +
episódios paginados.

### Migration `migrations/clinico/0002_episodios.sql`

Extraída verbatim do DDM-001. As tabelas `consultas`/`slots_agenda` (módulo Agendamento) ficam
fora de âmbito.

```sql
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

CREATE TABLE IF NOT EXISTS clinico.diagnosticos_cid (
    episodio_id uuid    NOT NULL REFERENCES clinico.episodios_clinicos(id) ON DELETE CASCADE,
    cid         text    NOT NULL,
    principal   boolean NOT NULL DEFAULT false,
    PRIMARY KEY (episodio_id, cid)
);
```

> **Nota:** o `motivo` de cancelamento não tem coluna no DDM. O cancelamento regista-se na
> auditoria (`clinico.episodio.cancelado`, com o motivo em `Detalhe`); o estado passa a
> `CANCELADO`. Não se acrescentam colunas fora do DDM.

### Decisões transversais

- **Identidade dos registos:** `id` gerado pela BD; o domínio usa `string`.
- **`medico_id`:** keycloak_id do médico responsável, fornecido no corpo do pedido de iniciar
  (o disparo por Agendamento — UC-DOE-06 — fica para quando esse módulo existir).
- **`especialidade_id`:** identificador opaco (string) fornecido pelo chamador, guardado tal
  como vem (sem FK — o módulo Admin/Especialidades não existe ainda).
- **`inicio`:** opcional no pedido (RFC 3339); se omitido, assume o momento da criação.
- Reutiliza a infra do M1/Sprint 7: middleware Auth/RBAC/LimiteTaxa, RFC 7807, repositório de
  auditoria, e o `RepositorioDoentes` (para validar existência/estado do doente ao iniciar).

---

## Secção 2 — Domínio (`internal/domain/clinico/`)

Domínio rico, IDs `string`, mensagens de erro literais PT-PT. Ficheiros novos:

### `episodio.go` — agregado raiz

```go
type EpisodioClinico struct {
    id              string
    doenteID        string
    tipo            TipoEpisodio      // CONSULTA|URGENCIA|INTERNAMENTO
    especialidadeID string            // identificador opaco
    medicoID        string            // keycloak_id do médico responsável
    inicio          time.Time
    fim             *time.Time
    nota            NotaClinica
    diagnosticosCID []DiagnosticoCID
    estado          EstadoEpisodio    // ABERTO|FECHADO|CANCELADO
    criadoEm        time.Time
    actualizadoEm   time.Time
    fechadoEm       *time.Time
    fechadoPor      string
}
```

- Factory `NovoEpisodio(doenteID string, tipo TipoEpisodio, especialidadeID, medicoID string, inicio time.Time) (*EpisodioClinico, error)` — valida obrigatórios (doenteID/especialidadeID/medicoID não-vazios, tipo válido, inicio não-zero); estado inicial `ABERTO`.
- `ActualizarNota(n NotaClinica) error` — só em ABERTO (senão CategoriaConflito).
- `DefinirDiagnosticosCID(cids []DiagnosticoCID) error` — só em ABERTO; no máximo um com `Principal=true`; cada CID não-vazio.
- `Fechar(fechadoPor string, em time.Time) error` — só de ABERTO; **exige** `nota.Completa()` (queixa+exame+diagnóstico+plano não-vazios) **e** ≥1 diagnóstico CID; põe FECHADO, `fim=&em`, `fechadoEm=&em`, `fechadoPor`.
- `Cancelar(em time.Time) error` — só de ABERTO; põe CANCELADO (o motivo é auditado na aplicação).
- Getters `ID()`, `DoenteID()`, `Estado()`; `Snapshot() SnapshotEpisodio` e `ReconstruirEpisodio(SnapshotEpisodio) *EpisodioClinico` (padrão do Doente — todos os campos nos dois sentidos).

### `nota_clinica.go` — VO

```go
type NotaClinica struct {
    QueixaPrincipal string
    HistoriaDoenca  string
    ExameObjectivo  string
    Diagnostico     string
    Plano           string
}
```
`NovaNotaClinica(...)` apara espaços (sem obrigatoriedade — a nota pode estar incompleta
enquanto ABERTO). Método `Completa() bool` — queixa+exame+diagnóstico+plano não-vazios
(historia opcional).

### `diagnostico_cid.go` — VO

```go
type DiagnosticoCID struct {
    CID       string
    Principal bool
}
```
`NovoDiagnosticoCID(cid string, principal bool) (DiagnosticoCID, error)` — CID não-vazio
(aparado).

### `episodio_enums.go`

- `type TipoEpisodio string`; consts `EpisodioConsulta="CONSULTA"`, `EpisodioUrgencia="URGENCIA"`, `EpisodioInternamento="INTERNAMENTO"`; `ParseTipoEpisodio(string) (TipoEpisodio, error)`.
- `type EstadoEpisodio string`; consts `EpisodioAbertoEstado="ABERTO"`, `EpisodioFechado="FECHADO"`, `EpisodioCancelado="CANCELADO"`.

### `episodio_eventos.go`

Eventos `EpisodioAberto`, `EpisodioFechado`, `EpisodioCancelado` (sobre `shared/evento`,
nomes estáveis `clinico.episodio.aberto`/`.fechado`/`.cancelado`), com asserções de
conformidade com `evento.EventoDominio`.

### `repositorio_episodios.go`

```go
type FiltroEpisodios struct {
    DoenteID     string
    Estado       string // opcional
    Limite       int
    Deslocamento int
}
type ResumoEpisodio struct {
    ID              string    `json:"id"`
    Tipo            string    `json:"tipo"`
    EspecialidadeID string    `json:"especialidade_id"`
    MedicoID        string    `json:"medico_id"`
    Inicio          time.Time `json:"inicio"`
    Fim             *time.Time `json:"fim,omitempty"`
    Estado          string    `json:"estado"`
}
type PaginaEpisodios struct {
    Itens        []ResumoEpisodio `json:"itens"`
    Total        int              `json:"total"`
    Limite       int              `json:"limite"`
    Deslocamento int              `json:"deslocamento"`
}
type RepositorioEpisodios interface {
    Guardar(ctx context.Context, e *EpisodioClinico) (string, error)
    ObterPorID(ctx context.Context, id string) (*EpisodioClinico, error)
    ListarPorDoente(ctx context.Context, f FiltroEpisodios) (PaginaEpisodios, error)
}
```

---

## Secção 3 — Aplicação + projecção EHR

Casos de uso em `internal/application/clinico/`, sobre portas, relógio injectado, cada um audita
(excepto a listagem). Reutiliza a porta `Auditor` e o `RepositorioDoentes` existentes; adiciona
o reexport de `RepositorioEpisodios` e novos DTOs.

- **`iniciar_episodio.go`** — `CasoIniciarEpisodio`: carrega o doente via
  `RepositorioDoentes.ObterPorID`; **exige que exista e esteja ACTIVO** (FALECIDO/APAGADO/
  INACTIVO → CategoriaConflito, "não é possível abrir episódio a este doente"); constrói o
  episódio (inicio = fornecido ou `agora()`); `Guardar`; audita `clinico.episodio.aberto`;
  re-lê e devolve `DetalheEpisodio`.
- **`actualizar_episodio.go`** — hidrata; se `dados.Nota != nil` aplica `ActualizarNota`; se
  `dados.DiagnosticosCID != nil` aplica `DefinirDiagnosticosCID`; `Guardar`; audita
  `clinico.episodio.actualizado`; re-lê e devolve o detalhe.
- **`fechar_episodio.go`** — hidrata; `Fechar(actor, agora())` (validação nota-completa + ≥1
  CID no domínio); `Guardar`; audita `clinico.episodio.fechado`.
- **`cancelar_episodio.go`** — hidrata; `Cancelar(agora())`; `Guardar`; audita
  `clinico.episodio.cancelado` (motivo em `Detalhe`).
- **`obter_episodio.go`** — por id; audita `clinico.episodio.consultado`.
- **`listar_episodios.go`** — por doente, filtro por estado + paginação (limite default 20,
  máximo 100); **não** audita.
- **`obter_ehr.go`** — `CasoObterEHR`: monta `EHR{Doente DetalheDoente, Episodios
  PaginaEpisodios}` combinando `RepositorioDoentes.ObterPorID` (traz alergias/antecedentes) +
  `RepositorioEpisodios.ListarPorDoente` (paginado); audita `clinico.ehr.consultado`.

DTOs novos em `ports.go`:
- `DadosNovoEpisodio{ DoenteID string; Tipo string; EspecialidadeID string; MedicoID string; Inicio *time.Time }`.
- `DadosActualizarEpisodio{ Nota *DadosNotaClinica; DiagnosticosCID *[]DadosDiagnosticoCID }`.
- `DadosNotaClinica{ QueixaPrincipal, HistoriaDoenca, ExameObjectivo, Diagnostico, Plano string }`.
- `DadosDiagnosticoCID{ CID string; Principal bool }`.
- `DadosCancelarEpisodio{ Motivo string }`.
- `DetalheEpisodio` (json: id, doente_id, tipo, especialidade_id, medico_id, inicio, fim,
  nota {queixa/historia/exame/diagnostico/plano}, diagnosticos_cid [], estado, criado_em,
  actualizado_em, fechado_em, fechado_por).
- Reexports `FiltroEpisodios`/`PaginaEpisodios`/`ResumoEpisodio` do domínio.
- `EHR{ Doente DetalheDoente `json:"doente"`; Episodios PaginaEpisodios `json:"episodios"` }`.

Mapeamento `paraDetalheEpisodio(*dominio.EpisodioClinico) DetalheEpisodio` via `Snapshot()`,
com as listas inicializadas (não nil).

---

## Secção 4 — Adaptadores, HTTP/RBAC e plataforma

### `internal/adapters/pgrepo/episodios_repo.go`

Implementa `RepositorioEpisodios` com pgx (padrão do `doentes_repo`):
- `Guardar` — `INSERT ... RETURNING id::text` (id vazio) ou `UPDATE` numa transacção;
  `diagnosticos_cid` por delete-and-reinsert dentro da transacção.
- `ObterPorID` — SELECT do episódio + carregamento dos CIDs; `pgx.ErrNoRows` →
  `CategoriaNaoEncontrado`.
- `ListarPorDoente` — `WHERE doente_id=$1 AND ($2='' OR estado=$2)`, `ORDER BY inicio DESC`,
  `COUNT(*)` + `LIMIT/OFFSET`.

Campos anuláveis lidos para `*string`/`*time.Time`; campos de texto opcionais gravados com
`NULLIF(...,'')` (simetria INSERT/UPDATE, lição do Sprint 7). Não regista dados clínicos em log.

### `internal/adapters/http/episodio_handler.go`

Dois grupos de rotas, ambos com `Auth` + rate limit. DTOs JSON PT-PT; erros via `responderErro`;
`actor = SessaoDe(c).Sujeito` (também usado como `fechado_por`).

| Rota | Método | Acção | Papéis |
|---|---|---|---|
| `/api/v1/doentes/:id/episodios` | POST | iniciar | Médico, Enfermeiro |
| `/api/v1/doentes/:id/episodios` | GET | listar por doente | leitura clínica* |
| `/api/v1/doentes/:id/ehr` | GET | vista EHR | leitura clínica* |
| `/api/v1/episodios/:eid` | GET | obter detalhe | leitura clínica* |
| `/api/v1/episodios/:eid` | PATCH | actualizar nota + CID | Médico, Enfermeiro |
| `/api/v1/episodios/:eid/fechar` | POST | fechar | **Médico** |
| `/api/v1/episodios/:eid/cancelar` | POST | cancelar | **Médico** |

\* **leitura clínica** = Médico, Enfermeiro, Farmacêutico, TecnicoLab, Director, DPO, Auditor
(**exclui Administrativo** — vê a demografia do doente mas não as notas clínicas/EHR).

Datas: `inicio` (iniciar) opcional RFC 3339 → default momento da criação; parse com
`time.Parse(time.RFC3339, ...)`; data inválida → 400.

### Plataforma & docs

- `platform/app.go` — instancia `pgrepo.NovoRepositorioEpisodios(pool)`, os 7 casos de uso e o
  handler; regista os dois grupos com `limiteMW, authMW` (sem MFA, como o handler de doentes).
- Sem novas variáveis de config. Sem alteração ao `.go-arch-lint.yml` (componentes por caminho).
- `adrs/ADR-027-bc-clinico-episodio.md` — decisões: agregado independente (vs sub-agregado do
  DDM), `especialidade_id` opaco, nota completa + ≥1 CID obrigatórios no fecho, EHR como
  projecção de leitura, e diferimentos (RN-DOE-05, prescrições, requisições-lab, relação
  clínica RN-DOE-03, integração Agendamento).

## Testes & verificação (fim a fim)

1. `go build ./...` e `make test` verdes; `make cover` cumpre 85/75/60.
2. `make lint` sem violações — `domain/clinico` não importa `pgx`/`gin`/`uuid`.
3. Migration `clinico/0002` aplica-se; `schema_migrations` regista `clinico/0002_episodios`.
4. Iniciar episódio a doente ACTIVO → 201; a doente FALECIDO/inexistente → 409/404.
5. Actualizar nota + CID (episódio ABERTO) → 200; fechar sem nota completa ou sem CID → erro de
   validação; fechar completo → 200 + estado FECHADO; actualizar após fechar → conflito.
6. Cancelar (ABERTO) → 200 + CANCELADO; cancelar um já fechado → conflito.
7. Listar por doente com `?estado=ABERTO` + paginação → 200.
8. `GET /api/v1/doentes/:id/ehr` → 200 com doente (alergias/antecedentes) + episódios paginados.
9. RBAC: fechar como Enfermeiro → 403; iniciar como Administrativo → 403; ler EHR como
   Administrativo → 403; ler EHR como Médico → 200.
10. Cada escrita/consulta individual/EHR gera evento em `auditoria.auditoria_eventos`.
11. `tests/integration/episodios_test.go` (tag `integration`): iniciar → actualizar → fechar →
    listar → EHR contra a BD; SKIP sem `DATABASE_URL`.

## Fora de âmbito (fatias futuras)

- **RN-DOE-05** (episódio ABERTO bloqueia actualização de nome/BI do doente) — follow-up.
- **RN-DOE-03** (acesso ao EHR exige relação clínica activa) — depende de Agendamento.
- **Prescrições** (Sprint 9) e **requisições de laboratório** (módulo Laboratório).
- **Integração Agendamento** (UC-DOE-06 disparado ao iniciar consulta).
- **Declarar óbito cancela episódios ABERTOS** (UC-DOE-08) — interacção Doente↔Episódio; quando
  a fatia LPDP/óbito consolidada for feita.
