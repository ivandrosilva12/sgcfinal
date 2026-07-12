# Sprint 10 — Farmácia: Stock & Dispensa

**Marco:** M2 — Clínico Core (quarta fatia vertical)
**Data:** 2026-07-12
**Estado:** Aprovado para planeamento

## Contexto

O Sprint 9 entregou o BC Farmácia com o catálogo de Medicamentos e a Receita/Prescrição
(emitida de um episódio, com validação de alergias), fundido em `main`. Falta o **stock** (dar
entrada de medicamentos em lotes) e a **dispensa** da receita (consumir stock por FEFO,
actualizando a receita). Esta fatia entrega o ciclo completo: pôr stock e dispensar uma receita.

**Fonte de verdade do modelo de dados:** DDM-001 v2.0 (extraído verbatim; não inventado).

## Decomposição do M2

- **Sprint 7 (concluído):** Doente + Alergia + AntecedenteClinico.
- **Sprint 8 (concluído):** EpisodioClinico + DiagnosticoCID + EHR.
- **Sprint 9 (concluído):** BC Farmácia — Medicamento + Receita/Prescrição.
- **Sprint 10 (esta fatia):** Farmácia — Stock (Fornecedor, Lote, Movimento, entrada, consulta) +
  Dispensa (FEFO, alergias, não-exceder, estados PARCIAL/DISPENSADA).
- **Diferido:** ajuste manual de stock, alertas (validade/stock-baixo), relatórios de movimentos,
  venda directa OTC, job de expiração automática, psicotrópicos, transferências.

## Princípios herdados (não-negociáveis)

- **Linguagem ubíqua PT-PT angolano** em TODO o output. Nunca inglês nem PT-BR.
- **DDD + Clean Architecture.** `internal/domain/**` só stdlib + Shared Kernel; zero
  `pgx`/`gin`/`http`. **Sem `google/uuid` no domínio nem na aplicação.** O domínio `farmacia` não
  importa o domínio `clinico` (só o adaptador `LeitorClinico`).
- **Sem `panic()`** fora de init. **Migrations forward-only.**
- **Erros de domínio** via `erros.Novo(categoria, msg)` (mensagens PT-PT literais). Categorias:
  `CategoriaValidacao`, `CategoriaNaoEncontrado`, `CategoriaConflito`, `CategoriaRegraNegocio`
  (→422, do Sprint 9). **Erros HTTP** via `responderErro` (RFC 7807).
- **IDs** do domínio são `string`, gerados pela BD (`gen_random_uuid()` + `RETURNING`).
- **Auditoria:** entrada de stock, dispensa e registo de fornecedor auditados; consultas de stock/
  lotes e listagem de fornecedores **não** auditam. Acções: `farmacia.fornecedor.registado`,
  `farmacia.stock.entrada`, `farmacia.receita.dispensada`.
- **Dados de saúde nunca em log.**
- **Cobertura:** domínio ≥85%, aplicação ≥75%, adaptadores ≥60% — mas **`internal/adapters/pgrepo`
  passa a ser excluído do gate unitário** (é coberto por integração, por desenho; ver Testes).
- **Conventional Commits** PT-PT com o trailer
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- **Branch:** `m2-sprint10-farmacia-stock-dispensa`.

---

## Secção 1 — Arquitectura + modelo de dados

No BC Farmácia existente. Agregados novos: **Fornecedor** e **Lote**. O **Movimento** de stock é
um **ledger append-only** (registo, não agregado com comportamento). A **Dispensa** é um caso de
uso que orquestra vários recursos e liga-se à **Receita** do Sprint 9.

### Migration `migrations/farmacia/0002_stock.sql`

Extraída verbatim do DDM-001 (schema `farmacia`):

```sql
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
Acrescenta `farmacia/0002_stock.sql` à lista de `migrations/embed_test.go` (o directório `farmacia`
já está no `//go:embed`).

### Fronteira transaccional da Dispensa (decisão-chave)

A dispensa tem de ser **atómica** através de vários recursos — decrementar lotes por FEFO, inserir
movimentos `SAIDA_DISPENSA`, e actualizar a receita (`itens_receita.quantidade_dispensada` +
estado) — e o FEFO precisa de **bloqueio de linhas** (`SELECT ... FOR UPDATE`) para ser seguro sob
concorrência. Por isso a dispensa expõe uma **porta `MotorDispensa`** (aplicação), implementada por
um **adaptador transaccional pgx**: numa única transacção faz o `FOR UPDATE` dos lotes válidos por
ordem de validade ASC, aloca (via a função de domínio `AlocarFEFO`), decrementa, insere os
movimentos, e persiste a receita. As validações que não dependem de estado fresco da BD (receita
não-expirada, não-exceder-prescrito, alergias) são feitas na **aplicação antes** de invocar o
motor; o motor revalida só a disponibilidade de stock (com lock) e devolve `RegraNegocio` ("stock
insuficiente") se não houver. A entrada de stock (lote + movimento `ENTRADA`) é igualmente atómica,
via um método transaccional `RepositorioLotes.RegistarEntrada`.

### Decisões transversais

- IDs gerados pela BD; domínio `string`. `realizado_por` = actor autenticado.
- **`preco_unit_custo`** modelado no domínio como **decimal validado em texto** (`^\d+(\.\d{1,4})?$`,
  não-negativo) — o `moeda.AOA` do Shared Kernel guarda cêntimos (2 casas), insuficiente para o
  `NUMERIC(14,4)`. Persistido/lido via cast `::text` (PG converte text↔numeric), sem float nem
  dependências novas.
- **Quantidade dos movimentos com sinal:** `ENTRADA` positiva, `SAIDA_DISPENSA` negativa (coerente
  com o `CHECK (quantidade != 0)` do DDM).
- Reutiliza a infra M1/M2: Auth/RBAC/LimiteTaxa, RFC 7807, auditoria, categoria `RegraNegocio`→422,
  `LeitorClinico`, e os repositórios `RepositorioMedicamentos`/`RepositorioReceitas` do Sprint 9.

---

## Secção 2 — Domínio (`internal/domain/farmacia/`)

Tipos novos, IDs `string`, mensagens PT-PT literais.

### `fornecedor.go` — agregado

`Fornecedor{ id, nome string; nif, contacto *string; activo bool; criadoEm time.Time }`.
`NovoFornecedor(nome string, nif, contacto *string) (*Fornecedor, error)` — nome obrigatório;
activo=true. `Activar()`/`Desactivar()`; getters `ID()`, `Activo()`; `Snapshot()`/
`ReconstruirFornecedor`.

### `lote.go` — agregado

```go
type Lote struct {
    id                 string
    medicamentoID      string
    numeroLote         string
    validade           time.Time // só data
    quantidadeInicial  int
    quantidadeActual   int
    precoUnitarioCusto string // decimal ≤4 casas, ≥0
    fornecedorID       *string
    entradaEm          time.Time
    notas              string
}
```
`NovoLote(medicamentoID, numeroLote string, validade time.Time, quantidade int, precoUnitarioCusto
string, fornecedorID *string, notas string) (*Lote, error)` — medicamento e número de lote
obrigatórios; **quantidade > 0** (RN-FAR-02); **validade futura** (RN-FAR-01,
`validade.After(hoje)`); preço decimal válido e ≥0; `quantidadeActual = quantidadeInicial =
quantidade`. `Disponivel(agora time.Time) bool` (quantidadeActual>0 e ainda válido). Getters `ID()`,
`MedicamentoID()`, `QuantidadeActual()`; `Snapshot()`/`ReconstruirLote`.

### `fefo.go` — função pura de alocação

```go
type LoteFEFO struct { LoteID string; Disponivel int }
type AlocacaoFEFO struct { LoteID string; Quantidade int }
// AlocarFEFO recebe os lotes JÁ ordenados por validade ASC e aloca gulosamente
// a quantidade pedida, do mais próximo a expirar. Devolve RegraNegocio ("stock
// insuficiente") se a soma dos disponíveis não chegar.
func AlocarFEFO(lotes []LoteFEFO, quantidade int) ([]AlocacaoFEFO, error)
```
Pura e testável isoladamente; o adaptador faz o `SELECT ... FOR UPDATE` e passa as linhas
bloqueadas a esta função.

### `movimento.go` + `enums.go` (estender)

`type TipoMovimento string` — consts `MovimentoEntrada="ENTRADA"`, `MovimentoSaidaDispensa=
"SAIDA_DISPENSA"`, `MovimentoSaidaVenda="SAIDA_VENDA"`, `MovimentoAjuste="AJUSTE"`,
`MovimentoExpirado="EXPIRADO"`, `MovimentoTransferencia="TRANSFERENCIA"`. VO `Movimento{ Tipo
TipoMovimento; MedicamentoID, LoteID string; Quantidade int; Motivo string; ReceitaID *string;
RealizadoPor string; RealizadoEm time.Time }` + `NovoMovimento(...)` (quantidade≠0; AJUSTE exige
justificação).

### `receita.go` (estender o agregado do Sprint 9)

`RegistarDispensa(medicamentoID string, quantidade int) error` — localiza o item; valida
**não-exceder o prescrito cumulativamente** (`QuantidadeDispensada + quantidade ≤
QuantidadePrescrita`, RN-FAR-05, senão `RegraNegocio`); incrementa `QuantidadeDispensada`;
**recalcula o estado**: `DISPENSADA` se todos os itens totalmente dispensados, senão `PARCIAL`.
Novos eventos: `ReceitaDispensada`, `StockEntrado` (só definidos).

### Repositórios (`repositorio_*.go`)

- `RepositorioFornecedores{ Guardar(ctx, *Fornecedor)(string,error); ObterPorID(ctx,id); Listar(ctx, FiltroFornecedores)(PaginaFornecedores,error) }`.
- `RepositorioLotes{ RegistarEntrada(ctx, *Lote, realizadoPor string)(id string, err error); ObterPorID(ctx,id); ListarPorMedicamento(ctx, medicamentoID string, apenasDisponiveis bool)([]ResumoLote,error); StockDisponivel(ctx, medicamentoID string)(int,error) }`.
  - `RegistarEntrada` é transaccional (lote + movimento `ENTRADA`).
- Read-models `FiltroFornecedores`/`ResumoFornecedor`/`PaginaFornecedores`, `ResumoLote` (com tags JSON).

---

## Secção 3 — Aplicação (`internal/application/farmacia/`)

Casos de uso sobre portas, relógio injectado.

**Fornecedor:** `CasoRegistarFornecedor` (audita `farmacia.fornecedor.registado`),
`CasoListarFornecedores` (não audita).

**Entrada de stock (UC-FAR-01):** `CasoRegistarEntradaStock.Executar(ctx, actor, dados)` — valida
que o medicamento existe e está activo (`RepositorioMedicamentos`); se `fornecedor_id` indicado,
valida que existe; constrói o `Lote` (RN-FAR-01/02); `RepositorioLotes.RegistarEntrada(ctx, lote,
actor)` (atómico: lote + movimento `ENTRADA`); audita `farmacia.stock.entrada`.

**Consulta de stock (UC-FAR-05):** `CasoConsultarStock` (`StockDisponivel`) e `CasoListarLotes`
(`ListarPorMedicamento`). Não auditam.

**Dispensa (UC-FAR-02):** `CasoDispensarReceita.Executar(ctx, actor, receitaID, dados DadosDispensa)`:
1. Carrega a receita; se `EstadoEfectivo(agora)==EXPIRADA` ou estado ∉ {EMITIDA, PARCIAL} →
   `CategoriaConflito` (RN-FAR-07).
2. Para cada item a dispensar: `receita.RegistarDispensa(medicamentoID, quantidade)` — valida
   **não-exceder** (RN-FAR-05, senão `RegraNegocio`) e actualiza o estado em memória.
3. **Alergias (RN-FAR-04):** `LeitorClinico.ObterContextoDoente(doenteID)` (graves) ×
   `medicamento.CorrespondeSubstancia` (via `RepositorioMedicamentos.ObterPorID` de cada item) →
   bloqueio **422**; override via `ignorar_alerta_alergia` + `justificacao_alerta` (obrigatória,
   auditada).
4. `MotorDispensa.Dispensar(ctx, receita.Snapshot(), itensDispensa, actor)` — porta transaccional:
   FEFO `FOR UPDATE` + `AlocarFEFO` + decrementa lotes + insere movimentos `SAIDA_DISPENSA` +
   persiste a receita (quantidades + estado); `RegraNegocio` se stock insuficiente.
5. Audita `farmacia.receita.dispensada` (Detalhe com override quando aplicável); re-lê via
   `RepositorioReceitas.ObterPorID` e devolve `DetalheReceita`.

**Portas novas (`ports.go`):**
```go
type ItemDispensa struct { MedicamentoID string; Quantidade int }
type MotorDispensa interface {
    Dispensar(ctx context.Context, receita dominio.SnapshotReceita, itens []ItemDispensa, realizadoPor string) ([]dominio.AlocacaoFEFO, error)
}
```
DTOs: `DadosNovoFornecedor`, `DadosEntradaStock` (medicamento_id, numero_lote, validade, quantidade,
preco_unit_custo, fornecedor_id, notas), `DadosDispensa` (`Itens []ItemDispensaDTO{MedicamentoID,
Quantidade}`, `IgnorarAlertaAlergia`, `JustificacaoAlerta`), `DetalheLote`, `StockDTO`,
`ResumoFornecedor` (reexport), `DetalheFornecedor`.

---

## Secção 4 — Adaptadores, HTTP/RBAC e plataforma

### Adaptadores pgx

- `fornecedores_repo.go` — `Guardar` (INSERT/UPDATE), `ObterPorID`, `Listar`.
- `lotes_repo.go` — `RegistarEntrada` **transaccional** (insere lote `RETURNING id` + movimento
  `ENTRADA` na mesma transacção); `ObterPorID`; `ListarPorMedicamento` (opcionalmente só disponíveis,
  `ORDER BY validade ASC`); `StockDisponivel` (`SELECT COALESCE(SUM(quantidade_actual),0) ... WHERE
  quantidade_actual>0 AND validade>CURRENT_DATE`). `preco_unit_custo` escrito como `$n::numeric`,
  lido via `preco_unit_custo::text`. Conflito de unicidade (23505, mesmo lote) → `CategoriaConflito`.
- `motor_dispensa.go` — implementa `MotorDispensa` numa transacção: por item,
  `SELECT id, quantidade_actual FROM farmacia.lotes WHERE medicamento_id=$m AND quantidade_actual>0
  AND validade>CURRENT_DATE ORDER BY validade ASC FOR UPDATE` → `dominio.AlocarFEFO` → por alocação
  `UPDATE farmacia.lotes SET quantidade_actual=quantidade_actual-$q WHERE id=$lote` + `INSERT
  farmacia.movimentos_stock (tipo='SAIDA_DISPENSA', quantidade=-$q, receita_id, realizado_por, ...)`;
  depois `UPDATE farmacia.receitas SET estado=$estado` + `UPDATE farmacia.itens_receita SET
  quantidade_dispensada=$q WHERE receita_id AND medicamento_id`; commit; devolve as alocações.
  Stock insuficiente → rollback + `CategoriaRegraNegocio`.

### `internal/adapters/http/farmacia_stock_handler.go` (novo)

Registado no mesmo grupo `/api/v1/farmacia` (via `RegistarFarmaciaStock`, `Auth`+rate limit):

| Rota | Método | Acção | Papéis |
|---|---|---|---|
| `/api/v1/farmacia/fornecedores` | POST | registar | Farmacêutico, FarmacêuticoSenior |
| `/api/v1/farmacia/fornecedores` | GET | listar | leitura ampla* |
| `/api/v1/farmacia/lotes` | POST | entrada de stock | Farmacêutico, FarmacêuticoSenior |
| `/api/v1/farmacia/medicamentos/:id/stock` | GET | disponível | leitura ampla* |
| `/api/v1/farmacia/medicamentos/:id/lotes` | GET | lotes | leitura ampla* |
| `/api/v1/farmacia/receitas/:id/dispensar` | POST | dispensa | Farmacêutico, FarmacêuticoSenior |

\* **leitura ampla** = Médico, Enfermeiro, Farmacêutico, FarmacêuticoSenior, Director, DPO, Auditor.
`actor`/`realizado_por` = `SessaoDe(c).Sujeito`. Datas (`validade`) em `AAAA-MM-DD`. Bloqueio de
alergia/stock → **422**; a dispensa devolve o `DetalheReceita` actualizado (200).

### Plataforma & docs

- `internal/platform/app.go` — instancia `pgrepo.NovoRepositorioFornecedores(pool)`,
  `NovoRepositorioLotes(pool)`, `NovoMotorDispensa(pool)`, os casos de uso e o handler de stock;
  regista o grupo com `limiteMW, authMW`.
- `migrations/embed_test.go` — inclui `farmacia/0002_stock.sql`. `.go-arch-lint.yml` inalterado.
- **`scripts/cobertura.sh`** — exclui `internal/adapters/pgrepo` do alvo do gate unitário dos
  adaptadores (é integration-only por desenho), medindo `go list ./internal/adapters/... | grep -v
  '/pgrepo$'` (ou equivalente). Isto resolve a dívida estrutural de cobertura sinalizada no Sprint 9.
- `adrs/ADR-029-farmacia-stock-dispensa.md` — decisões: agregados Fornecedor/Lote, Movimento como
  ledger com sinal, FEFO com `FOR UPDATE` + `AlocarFEFO` puro, dispensa transaccional
  (`MotorDispensa`), revalidação de alergias na dispensa com override, extensão do agregado Receita
  (`RegistarDispensa`), preço como decimal-texto, exclusão do pgrepo do gate unitário; e os
  diferimentos.

## Testes & verificação (fim a fim)

1. `go build ./...` e `go build -tags integration ./...` — sem erros.
2. `go test ./...` — PASS. `bash scripts/cobertura.sh` — domínio ≥85%, aplicação ≥75%, adaptadores
   ≥60% (já **sem** o pgrepo no denominador).
3. `make lint` — sem violações; `domain/farmacia` não importa `pgx`/`gin`/`uuid` nem `domain/clinico`.
4. Migration `farmacia/0002` aplica-se; `schema_migrations` regista `farmacia/0002_stock`.
5. **Domínio:** `AlocarFEFO` (alocação simples; múltiplos lotes; insuficiência → RegraNegocio);
   `NovoLote` (validade passada→erro, quantidade≤0→erro, preço inválido→erro); `Receita.
   RegistarDispensa` (não-exceder→RegraNegocio; parcial→PARCIAL; total→DISPENSADA).
6. **Aplicação:** entrada de stock (medicamento inexistente/inactivo→erro); dispensa (receita
   expirada→conflito; alergia sem override→422; override sem justificação→validação; override com
   justificação→dispensa+auditoria; não-exceder→RegraNegocio; stock insuficiente via MotorDispensa
   fake→RegraNegocio).
7. **Integração (`tests/integration/stock_dispensa_test.go`):** registar medicamento → 2 lotes com
   validades diferentes → consultar stock (soma) → emitir receita (Sprint 9) → **dispensa parcial
   que consome primeiro o lote de validade mais próxima (FEFO)** → confirmar: ordem FEFO, stock
   decrementado, `itens_receita.quantidade_dispensada`, receita PARCIAL → segunda dispensa →
   DISPENSADA; movimentos `SAIDA_DISPENSA` registados. SKIP sem `DATABASE_URL`.
8. **HTTP/RBAC:** entrada de stock/dispensar como Médico → 403; dispensar como Farmacêutico → 200;
   ler stock (leitura ampla) → 200.

## Fora de âmbito (fatias futuras)

- Ajuste manual de stock (UC-FAR-08, RN-FAR-09 — justificação >20 chars, Farmacêutico Sénior).
- Alertas de validade (UC-FAR-06) e de stock baixo (UC-FAR-10, RN-FAR-08).
- Relatório de movimentos por período (UC-FAR-11).
- Venda directa OTC (UC-FAR-09) e integração com Facturação.
- Job de expiração automática (UC-FAR-07 — marcar lotes EXPIRADO após validade).
- Psicotrópicos (RN-FAR-06) e transferências de stock.
