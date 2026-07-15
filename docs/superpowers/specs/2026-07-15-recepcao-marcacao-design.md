# Design — BC Recepção: Marcação e Agenda por Disponibilidade

> **Data:** 2026-07-15
> **Estado:** Aprovado (brainstorming) — pendente de plano de implementação
> **ADR associada:** ADR-032 (a redigir no plano)
> **Marco:** Novo marco "Percurso Ambulatório" (Recepção). Este spec cobre **apenas o
> primeiro sub-projecto: Marcação**. Recepção (check-in) e Triagem terão spec/plano próprios.

---

## 1. Contexto e Motivação

Hoje o `EpisodioClinico` (BC `clinico`) nasce diretamente em `ABERTO`: `NovoEpisodio`
recebe já doente + especialidade + médico + início, assumindo que **o doente já está à
frente do médico**. Falta modelar tudo o que acontece *antes* da consulta — o percurso
ambulatório do doente na clínica:

```
Marcação ──> Chegada/Check-in ──> Triagem ──> Consulta (Episódio, BC Clínico)
(agendar)     (Recepção)          (prioridade)   (existente)
```

Este design trata **a Marcação**: agendar consultas contra a disponibilidade declarada
de cada médico, com o ciclo de vida da marcação (cancelar, remarcar, registar falta) e a
agenda consultável. As etapas seguintes (check-in e triagem) ficam para ciclos próprios.

## 2. Decisões de Arquitectura

- **Novo Bounded Context `recepcao`**, com as 4 camadas Clean e schema PostgreSQL próprio
  `recepcao`, alinhado com os 5 BCs existentes (`clinico`, `farmacia`, `laboratorio`,
  `financeiro`, `identidade`). Mantém o BC Clínico focado no ato clínico; o agendamento
  administrativo vive fora dele.
- **Sem FK cross-context.** Um adaptador ACL `LeitorDoente` (importa o domínio `clinico`,
  permitido em adaptadores) valida que o doente existe e está activo antes de marcar.
  `medicoID`/`especialidadeID` são referências textuais validadas por ACL leve, como já se
  faz no Laboratório.
- **Remarcação por *supersede*** (mesmo padrão da correcção de resultados do Laboratório,
  Sprint 13): a marcação original passa a `REMARCADA` e nasce uma nova `MARCADA` que
  aponta para a original (`remarca_de`), preservando o histórico.
- **Invariante de disponibilidade como função de domínio pura**, alimentada pelo caso de
  uso com dados dos repositórios (mesmo padrão do FEFO/ACL já no código). Defesa em
  profundidade: a BD nega sobreposições com uma restrição `EXCLUDE`.

### Layout

```
internal/domain/recepcao/          # Camada 1 — DDD
internal/application/recepcao/     # Camada 2 — casos de uso + ports
internal/adapters/http/            # recepcao_handler.go (rota + RBAC)
internal/adapters/pgrepo/          # janelas_repo.go, marcacoes_repo.go
internal/adapters/recepcao/        # leitor_doente.go (ACL sobre clinico)
migrations/recepcao/0001_agenda_marcacoes.sql
```

## 3. Modelo de Domínio

### 3.1 Agregado `JanelaDisponibilidade`

A agenda declarada de um médico: um intervalo datado onde é possível marcar.

**Campos:** `id`, `medicoID`, `especialidadeID`, `inicio` (timestamptz), `fim`, `criadoEm`.

**Invariantes:**
- `medicoID`, `especialidadeID` obrigatórios;
- `inicio` e `fim` não-zero e `fim > inicio`.

**Comportamento:**
- `NovaJanela(medicoID, especialidadeID, inicio, fim)` → valida e constrói.
- Remoção é orquestrada pelo caso de uso (só se não houver marcação activa dentro dela);
  o agregado não guarda referência às marcações.

**Modelo de recorrência:** janelas com **data concreta** (sem motor de recorrência, sem
fatiar automático em slots). Para uma semana repetida criam-se várias janelas. (YAGNI —
recorrência semanal fica para evolução futura, se pedida.)

### 3.2 Agregado `Marcacao` (raiz)

**Campos:** `id`, `doenteID`, `medicoID`, `especialidadeID`, `inicio`, `fim`, `estado`,
`motivo` (preenchido no cancelamento), `remarcaDe *string`, `criadoEm`, `actualizadoEm`.

**Máquina de estados:**

```
             ┌─ Cancelar(motivo) ──> CANCELADA
   MARCADA ──┼─ Remarcar ─────────> REMARCADA   (+ nova Marcacao MARCADA)
             └─ RegistarFalta ─────> FALTOU
```

`COMPARECEU` (chegada) **não** faz parte deste sub-projecto — será marcado pelo módulo
Recepção num ciclo futuro.

**Comportamento (regras no agregado):**
- `NovaMarcacao(doenteID, medicoID, especialidadeID, inicio, fim)` → estado `MARCADA`;
  valida obrigatórios e `fim > inicio`.
- `Cancelar(motivo, em)` — só de `MARCADA`; `motivo` obrigatório (→ `CANCELADA`).
- `Remarcar(novoInicio, novoFim, em)` — só de `MARCADA`; marca a receptora `REMARCADA` e
  **devolve uma nova** `Marcacao` `MARCADA` com `remarcaDe = original.id`, preservando
  `doenteID`/`medicoID`/`especialidadeID`.
- `RegistarFalta(em)` — só de `MARCADA` e só depois da hora (`em >= fim`) (→ `FALTOU`).

Transições inválidas devolvem `CategoriaConflito` (409); pré-condições de dados
(motivo em falta, falta antes da hora) devolvem `CategoriaValidacao`/`CategoriaRegraNegocio`.

### 3.3 Regra que cruza agregados — `VerificarDisponibilidade`

Função de domínio **pura** (sem I/O), invariante de negócio central da marcação:

```
VerificarDisponibilidade(janelas []JanelaDisponibilidade,
                         marcacoesActivas []Marcacao,
                         doProposta, ateProposta time.Time,
                         medicoID, especialidadeID string,
                         agora time.Time) error
```

Verifica, por esta ordem:
1. **Não no passado:** `doProposta >= agora` (senão `CategoriaRegraNegocio`).
2. **Cabe inteira numa janela** do mesmo médico+especialidade (`janela.inicio <= do` e
   `ate <= janela.fim`) (senão `CategoriaRegraNegocio`, "fora de janela").
3. **Não sobrepõe** nenhuma marcação `MARCADA` do mesmo médico
   (`do < m.fim && m.inicio < ate`) (senão `CategoriaConflito`).

O caso de uso alimenta a função com dados lidos dos repositórios. Encosto exacto
(`ate == m.inicio` ou `do == m.fim`) **não** é sobreposição.

## 4. Camada de Aplicação

### 4.1 Casos de Uso

| Caso de uso | Comportamento | Acção de auditoria |
|---|---|---|
| `NovoCasoDefinirJanela` | Valida médico/especialidade; cria janela. | `recepcao.janela.definida` |
| `NovoCasoRemoverJanela` | Remove janela se não tiver marcação activa lá dentro (senão 409). | `recepcao.janela.removida` |
| `NovoCasoMarcar` | ACL valida doente activo; `VerificarDisponibilidade`; persiste `MARCADA`. | `recepcao.marcacao.criada` |
| `NovoCasoRemarcar` | Revalida disponibilidade da nova data; supersede (original→`REMARCADA` + nova `MARCADA`). | `recepcao.marcacao.remarcada` |
| `NovoCasoCancelar` | `MARCADA`→`CANCELADA` com motivo. | `recepcao.marcacao.cancelada` |
| `NovoCasoRegistarFalta` | `MARCADA`→`FALTOU` (só após a hora). | `recepcao.marcacao.faltou` |
| `NovoCasoListarAgenda` | Lê agenda por médico + intervalo (janelas + marcações). | — (leitura) |
| `NovoCasoListarMarcacoesDoente` | Lê marcações de um doente. | — (leitura) |

**Actor** vem sempre de `SessaoDe(ctx).Sujeito`, nunca do corpo do pedido.

### 4.2 Ports (`internal/application/recepcao/ports.go`)

- `LeitorDoente` (ACL): `DoenteActivo(ctx, doenteID string) (ok bool, err error)`.
- `RepositorioJanelas`: `Guardar`, `ObterPorID`, `ListarPorMedicoIntervalo`, `Remover`.
  (O `RemoverJanela` verifica marcações activas reutilizando
  `RepositorioMarcacoes.ListarActivasPorMedicoIntervalo` sobre o intervalo da janela.)
- `RepositorioMarcacoes`: `Guardar`, `ObterPorID`, `Transitar` (compare-and-set no estado;
  `RowsAffected()==0` distingue 404 de 409), `Remarcar` (transaccional: UPDATE original +
  INSERT nova), `ListarActivasPorMedicoIntervalo`, `ListarPorDoente`.
- `Auditor`: reutiliza a interface existente.

Fakes (não mocks) para todos os ports nos testes de aplicação.

## 5. Adaptadores

### 5.1 HTTP — `recepcao_handler.go`

Construtor posicional (padrão do handler do Laboratório). Grupos RBAC:
`soAdministrativo` = `{Administrativo, Director, Admin}`; leitura de agenda também para
`Medico`.

| Método + rota | Caso de uso | RBAC |
|---|---|---|
| `POST /medicos/:mid/janelas` | DefinirJanela | soAdministrativo |
| `DELETE /janelas/:jid` | RemoverJanela | soAdministrativo |
| `POST /marcacoes` | Marcar | soAdministrativo |
| `POST /marcacoes/:mid/remarcacao` | Remarcar | soAdministrativo |
| `POST /marcacoes/:mid/cancelamento` | Cancelar | soAdministrativo |
| `POST /marcacoes/:mid/falta` | RegistarFalta | soAdministrativo |
| `GET /agenda?medico=&de=&ate=` | ListarAgenda | Administrativo, Medico |
| `GET /doentes/:did/marcacoes` | ListarMarcacoesDoente | Administrativo, Medico |

Registo via `RegistarRecepcao(r, handler, limiteMW, authMW)`, montado no composition root
(`internal/platform/app.go`).

### 5.2 Persistência — `migrations/recepcao/0001_agenda_marcacoes.sql`

Forward-only, sem `.down.sql`. Requer extensão `btree_gist`.

```sql
CREATE SCHEMA IF NOT EXISTS recepcao;
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS recepcao.janelas (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    medico_id        uuid        NOT NULL,
    especialidade_id uuid        NOT NULL,
    inicio           timestamptz NOT NULL,
    fim              timestamptz NOT NULL,
    criado_em        timestamptz NOT NULL DEFAULT now(),
    CHECK (fim > inicio)
);
CREATE INDEX IF NOT EXISTS idx_janelas_medico
    ON recepcao.janelas (medico_id, inicio);

CREATE TABLE IF NOT EXISTS recepcao.marcacoes (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id        uuid        NOT NULL,
    medico_id        uuid        NOT NULL,
    especialidade_id uuid        NOT NULL,
    inicio           timestamptz NOT NULL,
    fim              timestamptz NOT NULL,
    estado           text        NOT NULL CHECK (estado IN
                       ('MARCADA','CANCELADA','REMARCADA','FALTOU')),
    motivo           text,
    remarca_de       uuid        REFERENCES recepcao.marcacoes(id),
    criado_em        timestamptz NOT NULL DEFAULT now(),
    actualizado_em   timestamptz NOT NULL DEFAULT now(),
    CHECK (fim > inicio),
    CHECK (estado <> 'CANCELADA' OR motivo IS NOT NULL),
    -- Defesa em profundidade: a BD nega marcações MARCADA sobrepostas do mesmo médico.
    EXCLUDE USING gist (
        medico_id WITH =,
        tstzrange(inicio, fim) WITH &&
    ) WHERE (estado = 'MARCADA')
);
CREATE INDEX IF NOT EXISTS idx_marcacoes_doente ON recepcao.marcacoes (doente_id);
CREATE INDEX IF NOT EXISTS idx_marcacoes_medico ON recepcao.marcacoes (medico_id, inicio);
```

### 5.3 ACL — `internal/adapters/recepcao/leitor_doente.go`

Implementa `LeitorDoente` lendo o repositório de doentes do BC `clinico` (importa o
domínio `clinico`, permitido em adaptadores). Devolve `ok=false` se o doente não existir
ou não estiver activo.

## 6. Erros e Auditoria

Categorias já existentes em `internal/domain/shared/erros`:
`CategoriaValidacao`(400), `CategoriaConflito`(409, sobreposição/estado),
`CategoriaRegraNegocio`(422, fora de janela/passado), `CategoriaNaoEncontrado`(404),
`CategoriaProibido`(403), `CategoriaInterno`(500).

Todos os comandos (definir/remover janela, marcar, remarcar, cancelar, registar falta)
são auditados no log append-only (retenção 10 anos). As leituras não são auditadas.

## 7. Testes e Cobertura

- **Domínio ≥85%:** máquina de estados da `Marcacao` (todas as transições válidas e as
  inválidas → 409); `VerificarDisponibilidade` com casos de borda — encosto exacto (não é
  conflito), sobreposição parcial, proposta fora de qualquer janela, proposta no passado,
  janela de outra especialidade não conta.
- **Aplicação ≥75%:** fakes (não mocks) para `RepositorioJanelas`, `RepositorioMarcacoes`,
  `LeitorDoente`, `Auditor`. Testa: ACL recusa doente inexistente; auditoria emitida em
  cada comando; supersede na remarcação (original `REMARCADA`, nova `MARCADA`).
- **Adaptadores ≥60%:** teste HTTP com duplos (verifica RBAC, mapeamento de estados HTTP,
  actor da sessão). Teste de integração `//go:build integration` contra Postgres real
  (SKIP sem `DATABASE_URL`), cobrindo o ciclo marcar→remarcar→cancelar e a rejeição da
  `EXCLUDE` em sobreposição concorrente.

## 8. ADR e Critérios de Saída

- **ADR-032 — "BC Recepção — Marcação e agenda por disponibilidade"** (próximo número
  livre; a redigir como task do plano). Regista: novo BC, remarcação por supersede,
  invariante de disponibilidade em função pura + `EXCLUDE` na BD, ACL sobre o Clínico.

**Critérios de saída (sub-projecto Marcação):**
1. Definir/remover janelas de disponibilidade por médico (Administrativo).
2. Marcar dentro de uma janela livre; **recusar** fora de janela, sobreposta ou no passado.
3. Remarcar preservando o histórico (original `REMARCADA`, nova `MARCADA` ligada).
4. Cancelar com motivo; registar falta só após a hora.
5. Agenda consultável por médico+intervalo e marcações por doente.
6. Doente validado por ACL sobre o Clínico; sem FK cross-context.
7. Todos os comandos auditados; sobreposição negada também pela BD (`EXCLUDE`).
8. Cobertura nos limiares (domínio ≥85%, aplicação ≥75%, adaptadores ≥60%).

---

## Fora de Âmbito (ciclos futuros)

- **Recepção/Check-in:** chegada do doente, estado `COMPARECEU`, fila de espera, doente
  sem marcação prévia (walk-in).
- **Triagem:** classificação de prioridade + sinais vitais; entrega o doente pronto para o
  episódio clínico.
- Recorrência semanal de agenda, confirmação por SMS, duração fixa por especialidade,
  gestão de salas/equipamentos.
