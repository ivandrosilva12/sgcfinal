# Design — BC Recepção: Check-in e Fila de Espera

> **Data:** 2026-07-15
> **Estado:** Aprovado (brainstorming) — pendente de plano de implementação
> **ADR associada:** ADR-033 (a redigir no plano)
> **Marco:** Percurso Ambulatório. Segundo sub-projecto — **Check-in**. Sucede à
> Marcação (ADR-032, já entregue). A Triagem fica para o sub-projecto seguinte.

---

## 1. Contexto e Motivação

A Marcação (BC `recepcao`, ADR-032) entrega a agenda e o ciclo de vida da marcação até
`FALTOU`. O próprio código anota que a chegada do doente (`COMPARECEU`) ficaria para o
módulo de Check-in. É este.

O Check-in regista que **o doente está fisicamente na clínica hoje** e coloca-o numa
**fila de espera**, cobrindo dois casos:
- **Com marcação:** check-in que confirma a comparência e fecha o ciclo da marcação.
- **Walk-in:** doente que chega sem marcação prévia.

```
Marcação ──> Check-in (ESTE) ──> Triagem ──> Consulta (Episódio)
(ADR-032)     (Chegada + fila)   (futuro)     (BC Clínico)
```

## 2. Decisões de Arquitectura

- **Estende o BC `recepcao`** (sem novo BC). Novo agregado `Chegada` + extensão pontual
  ao agregado `Marcacao`. Reutiliza a ACL `LeitorDoente`, o padrão compare-and-set (CAS)
  e a auditoria já existentes.
- **`Chegada` é o agregado que unifica os dois casos** (com e sem marcação) e alimenta
  uma fila única. O walk-in não é forçado pela invariante de disponibilidade das
  marcações (`VerificarDisponibilidade`).
- **O check-in de uma marcação é transaccional e cruza dois agregados:** transita a
  `Marcacao` `MARCADA→COMPARECEU` **e** insere a `Chegada` numa única transacção. A
  guarda CAS sobre o estado da marcação impede o check-in duplo (409).
- **Sem FK cross-context.** `doente_id`/`medico_id`/`especialidade_id` são referências
  textuais (uuid) sem FK. A única FK é interna: `chegadas.marcacao_id → recepcao.marcacoes(id)`.

### Layout

```
internal/domain/recepcao/chegada.go          # novo agregado Chegada
internal/domain/recepcao/marcacao.go         # +COMPARECEU +RegistarComparencia
internal/domain/recepcao/repositorio.go      # +RepositorioChegadas +ResumoChegada
internal/application/recepcao/chegadas.go    # casos de uso do check-in
internal/application/recepcao/ports.go       # +DTOs de chegada
internal/adapters/pgrepo/chegadas_repo.go    # repositório pgx
internal/adapters/http/recepcao_handler.go   # +5 rotas de chegada (mesmo handler)
migrations/recepcao/0002_chegadas.sql
```

## 3. Modelo de Domínio

### 3.1 Agregado `Chegada` (raiz)

O doente presente na clínica hoje.

**Campos:** `id`, `doenteID`, `marcacaoID` (vazio no walk-in), `especialidadeID`,
`medicoID` (vazio no walk-in), `horaChegada`, `estado`, `estadoAnterior` (guarda CAS,
só fixado por `ReconstruirChegada`), `criadoEm`, `actualizadoEm`.

**Máquina de estados:**

```
AGUARDA ─┬─ Chamar ─────────► CHAMADO   (ponto de entrega à triagem)
         └─ RegistarDesistencia ► DESISTIU
```

**Construtores:**
- `NovaChegadaAgendada(doenteID, marcacaoID, medicoID, especialidadeID string, hora time.Time) (*Chegada, error)`
  — do check-in de uma marcação; exige todos os campos (o médico e a especialidade vêm
  da marcação). Estado inicial `AGUARDA`.
- `NovaChegadaWalkIn(doenteID, especialidadeID string, hora time.Time) (*Chegada, error)`
  — walk-in; sem `marcacaoID` nem `medicoID`. Exige doente + especialidade + hora.
  Estado inicial `AGUARDA`.

**Comportamento:**
- `Chamar(em time.Time) error` — só de `AGUARDA` (→ `CHAMADO`).
- `RegistarDesistencia(em time.Time) error` — só de `AGUARDA` (→ `DESISTIU`).

Transições inválidas devolvem `CategoriaConflito`.

### 3.2 Extensão de `Marcacao`

- Novo estado terminal `COMPARECEU` (acrescentado ao enum `EstadoMarcacao`).
- Novo método `RegistarComparencia(em time.Time) error` — `MARCADA→COMPARECEU`; só de
  `MARCADA`. Desfecho simétrico ao `FALTOU`: depois de comparecer, a marcação já não
  pode ser cancelada, remarcada nem dar falta (essas transições continuam a exigir
  `MARCADA`, logo recusam a partir de `COMPARECEU` com `CategoriaConflito`).

### 3.3 Coordenação do check-in agendado

Cruza dois agregados numa transacção (padrão do `Emitir`/`Remarcar` já no BC):
1. o caso de uso obtém a marcação, chama `RegistarComparencia` (valida estado no domínio);
2. constrói a `Chegada` agendada com `doente/médico/especialidade` da marcação;
3. persiste ambos com `RepositorioChegadas.RegistarChegadaAgendada(ctx, chegada, marcacao)`:
   `UPDATE recepcao.marcacoes SET estado='COMPARECEU', actualizado_em=$ WHERE id=$ AND
   estado='MARCADA'` (0 linhas → `erroTransicaoFalhada`: 404 se não existe, 409 se já
   não está `MARCADA` — check-in duplo) seguido de `INSERT` da chegada, na mesma `tx`.

Defesa em profundidade: `UNIQUE` parcial sobre `chegadas.marcacao_id` (uma chegada por
marcação) — apanha a corrida concorrente que ambas as guardas do domínio passariam.

## 4. Camada de Aplicação

### 4.1 Casos de Uso

| Caso de uso | Comportamento | Acção de auditoria |
|---|---|---|
| `NovoCasoRegistarChegada` | Obtém a marcação; `RegistarComparencia`; cria `Chegada` agendada; persiste na transacção coordenada. | `recepcao.chegada.registada` |
| `NovoCasoRegistarWalkIn` | Valida doente activo (ACL); cria `Chegada` walk-in; guarda. | `recepcao.chegada.walkin` |
| `NovoCasoChamar` | `AGUARDA→CHAMADO`. | `recepcao.chegada.chamada` |
| `NovoCasoRegistarDesistencia` | `AGUARDA→DESISTIU`. | `recepcao.chegada.desistiu` |
| `NovoCasoListarFila` | Lê a fila (`AGUARDA`) por especialidade. | — (leitura) |

**Actor** sempre de `SessaoDe(ctx).Sujeito`, nunca do corpo.

### 4.2 Ports

Reutiliza `LeitorDoente` (`DoenteActivo`) e `Auditor`. DTOs novos em `ports.go`:
`DadosWalkIn{ DoenteID, EspecialidadeID }`, `DetalheChegada`, reexport `ResumoChegada`.

Nova interface de domínio em `repositorio.go`:

```
type RepositorioChegadas interface {
    Guardar(ctx, *Chegada) (string, error)                          // walk-in
    RegistarChegadaAgendada(ctx, chegada *Chegada, marcacao *Marcacao) (string, error) // transaccional
    ObterPorID(ctx, id string) (*Chegada, error)
    Transitar(ctx, *Chegada) error                                  // CAS (Chamar/Desistir)
    ListarFila(ctx, especialidadeID string) ([]ResumoChegada, error) // AGUARDA, FIFO
}
```

`ResumoChegada{ ID, DoenteID, MarcacaoID, MedicoID, EspecialidadeID, Estado string; HoraChegada time.Time }`.

## 5. Adaptadores

### 5.1 HTTP — acrescentado ao `recepcao_handler.go`

Novos grupos RBAC: `chamada = {Enfermeiro, Medico, Administrativo}`,
`filaLeitura = {Administrativo, Enfermeiro, Medico}`. Reutiliza `soAdministrativo`.

| Método + rota | Caso de uso | RBAC |
|---|---|---|
| `POST /api/v1/marcacoes/:mid/chegada` | RegistarChegada | soAdministrativo |
| `POST /api/v1/chegadas` | RegistarWalkIn | soAdministrativo |
| `POST /api/v1/chegadas/:cid/chamada` | Chamar | chamada |
| `POST /api/v1/chegadas/:cid/desistencia` | RegistarDesistencia | soAdministrativo |
| `GET /api/v1/recepcao/fila?especialidade=` | ListarFila | filaLeitura |

### 5.2 Persistência — `migrations/recepcao/0002_chegadas.sql` (forward-only)

```sql
-- Estende o enum de estado da marcação com COMPARECEU (desfecho do check-in).
ALTER TABLE recepcao.marcacoes DROP CONSTRAINT marcacoes_estado_check;
ALTER TABLE recepcao.marcacoes ADD CONSTRAINT marcacoes_estado_check
    CHECK (estado IN ('MARCADA','CANCELADA','REMARCADA','FALTOU','COMPARECEU'));

CREATE TABLE IF NOT EXISTS recepcao.chegadas (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id        uuid        NOT NULL,
    marcacao_id      uuid        REFERENCES recepcao.marcacoes(id),
    especialidade_id uuid        NOT NULL,
    medico_id        uuid,
    hora_chegada     timestamptz NOT NULL,
    estado           text        NOT NULL CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU')),
    criado_em        timestamptz NOT NULL DEFAULT now(),
    actualizado_em   timestamptz NOT NULL DEFAULT now(),
    -- Uma chegada agendada tem sempre médico (herdado da marcação); o walk-in não tem
    -- marcação nem médico. A coerência é: se há marcação, há médico.
    CHECK (marcacao_id IS NULL OR medico_id IS NOT NULL)
);
-- Defesa em profundidade: uma chegada por marcação (o check-in duplo é negado também
-- pela guarda CAS do domínio, mas a BD fecha a corrida concorrente).
CREATE UNIQUE INDEX IF NOT EXISTS idx_chegadas_marcacao
    ON recepcao.chegadas (marcacao_id) WHERE marcacao_id IS NOT NULL;
-- Índice da fila: AGUARDA por especialidade, ordem FIFO por chegada.
CREATE INDEX IF NOT EXISTS idx_chegadas_fila
    ON recepcao.chegadas (estado, especialidade_id, hora_chegada);
```

`marcacao_id` é FK interna ao schema `recepcao` (como `remarca_de`) — permitida.
`doente_id`/`medico_id`/`especialidade_id` são uuid sem FK (regra de arquitectura).

## 6. Erros e Auditoria

Categorias existentes (`internal/domain/shared/erros`): `CategoriaConflito` (409, check-in
duplo / transição inválida), `CategoriaValidacao` (400), `CategoriaNaoEncontrado` (404),
`CategoriaRegraNegocio` (422, doente inactivo no walk-in). Todos os comandos auditados
(append-only, 10 anos). As leituras (fila) não são auditadas.

## 7. Testes e Cobertura

- **Domínio ≥85%:** máquina de estados da `Chegada` (transições válidas e as inválidas
  → 409); os dois construtores (campos obrigatórios); `RegistarComparencia` na `Marcacao`
  e a confirmação de que `Cancelar`/`Remarcar`/`RegistarFalta` recusam a partir de
  `COMPARECEU`.
- **Aplicação ≥75%:** fakes; a coordenação do check-in (transita a marcação **e** cria a
  chegada; check-in duplo → 409); a ACL do walk-in (doente inactivo → 422); auditoria em
  cada comando.
- **Adaptadores ≥60%:** teste HTTP com duplos (RBAC das 5 rotas; actor da sessão); teste
  de integração `//go:build integration` contra Postgres real — check-in transaccional
  (marcação passa a `COMPARECEU` **e** chegada criada, ou nada em caso de falha), o
  `UNIQUE` a negar check-in duplo, e a fila FIFO ordenada.

## 8. ADR e Critérios de Saída

- **ADR-033 — "BC Recepção — Check-in e fila de espera"** (próximo número livre; a
  redigir como task do plano). Regista: agregado `Chegada` unificando check-in e walk-in,
  `COMPARECEU` na marcação, coordenação transaccional com guarda CAS + `UNIQUE`, RBAC
  balcão/clínico.

**Critérios de saída (sub-projecto Check-in):**
1. Check-in de uma marcação: transita a marcação para `COMPARECEU` **e** cria a chegada
   na fila, atomicamente.
2. Walk-in: cria a chegada (doente validado por ACL + especialidade), sem marcação/médico.
3. Chamar (`AGUARDA→CHAMADO`) e registar desistência (`AGUARDA→DESISTIU`).
4. Fila consultável por especialidade, FIFO por hora de chegada.
5. Check-in duplo negado pelo agregado (guarda CAS) **e** pela BD (`UNIQUE`).
6. Sem FK cross-context; doente do walk-in validado por ACL sobre o Clínico.
7. Todos os comandos auditados; cobertura nos limiares.

---

## Fora de Âmbito (marco seguinte)

- **Triagem:** classificação de prioridade clínica + sinais vitais; consumo dos `CHAMADO`.
- Atribuição de médico ao walk-in (fica para a triagem/chamada).
- Validação de que a marcação do check-in é de hoje (o check-in aceita qualquer marcação
  `MARCADA`; a data não é imposta — decisão operacional da recepção).
- Reabertura de uma `DESISTIU` ou de uma marcação `COMPARECEU`.
