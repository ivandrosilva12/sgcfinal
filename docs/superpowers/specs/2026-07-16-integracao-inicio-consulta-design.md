# Design — Integração Recepção→Clínico: Início da Consulta

> **Data:** 2026-07-16
> **Estado:** Aprovado (brainstorming) — pendente de plano de implementação
> **ADR associada:** ADR-036 (a redigir no plano)
> **Marco:** Integração Percurso Ambulatório → Clínico. Consome a `Chegada` `TRIADO`
> (ADR-034) e cria o `EpisodioClinico` (ADR-027) — a integração diferida no fecho do
> marco Percurso Ambulatório.

---

## 1. Contexto e Motivação

A Triagem (ADR-034) fechou o Percurso Ambulatório com o doente na fila clínica, ordenada
por prioridade Manchester e filtrável por médico. Mas a `Chegada` `TRIADO` é hoje um
estado terminal: o doente nunca sai da fila e o `EpisodioClinico` só nasce por um endpoint
desligado do percurso (`POST /doentes/:id/episodios`). Esta entrega liga as duas pontas —
o médico inicia a consulta a partir da fila e recebe o episódio aberto na resposta.

```
Marcação → Check-in → Triagem → Início da Consulta (ESTE) → EpisodioClinico (ABERTO)
(ADR-032)  (ADR-033)  (ADR-034)  (TRIADO → EM_CONSULTA)      (BC Clínico, ADR-027)
```

**Âmbito:** só o início da consulta. A ligação dos sinais vitais da triagem ao EHR fica
fora de âmbito (ver §Fora de Âmbito).

## 2. Decisões de Arquitectura

- **Primeira escrita cross-BC do sistema.** Até hoje as portas ACL só leem (`LeitorClinico`
  do Lab, `LeitorDoente` da Recepção). Iniciar a consulta escreve nos dois contextos:
  transita a `Chegada` (recepcao) e cria o `EpisodioClinico` (clinico).
- **Consistência por transacção única num adaptador de integração** (Camada 3, que pode
  importar ambos os domínios): `INSERT` do episódio + `UPDATE` CAS da chegada numa só
  transacção PG. Sem estados intermédios nem compensação. Estende o precedente do
  `RegistarTriagem` (transaccional cross-agregado, ADR-034) para cross-BC, contido num
  único adaptador. Rejeitados: orquestração com compensação (janelas de inconsistência,
  episódio órfão se a compensação falhar) e Outbox assíncrono (o médico espera o episódio
  na resposta; o Outbox continua por implementar — dívida).
- **O caso de uso vive no BC Clínico** (`CasoIniciarConsulta`): é o Clínico que consome a
  chegada, como previsto na ADR-034. A aplicação do Clínico não importa o domínio da
  Recepção — fala por portas ACL com DTOs próprios.
- **Só o médico atribuído inicia a consulta.** Toda a chegada `TRIADO` tem médico
  (herdado da marcação ou atribuído na triagem ao walk-in). O actor autenticado tem de
  ser esse médico — guarda no domínio (`CategoriaProibido` → 403) re-verificada no CAS.
- **A proveniência fica do lado consumido.** A `Chegada` ganha `episodio_id` (uuid sem FK
  — cross-context); o `EpisodioClinico` não ganha nenhuma coluna — não sabe que nasceu de
  uma fila. Tipo do episódio fixo: `CONSULTA`.
- **Sem FK cross-context.** `episodio_id` na chegada é uuid nu; a integridade 1:1 é
  garantida por `UNIQUE` parcial (defesa em profundidade) e pela transacção.

### Layout

```
internal/domain/recepcao/chegada.go                  # +estado EM_CONSULTA +IniciarConsulta +episodioID
internal/application/clinico/iniciar_consulta.go     # caso de uso CasoIniciarConsulta
internal/application/clinico/ports.go                # +LeitorRecepcao +ConsumidorChegadas +DTOs
internal/adapters/pgrepo/integracao_consulta_repo.go # adaptador de integração (transacção única)
internal/adapters/http/clinico_consulta_handler.go   # POST /chegadas/:cid/iniciar-consulta
migrations/recepcao/0004_chegadas_em_consulta.sql
```

## 3. Modelo de Domínio

### 3.1 Extensão de `Chegada` (BC Recepção)

- Novo estado `ChegEmConsulta = "EM_CONSULTA"`. A máquina fica:

```
AGUARDA ─┬─ Chamar ────────────► CHAMADO ─ RegistarTriada ► TRIADO ─ IniciarConsulta ► EM_CONSULTA
         └─ RegistarDesistencia ► DESISTIU
```

- Novo campo `episodioID string` (+ snapshot/reconstrução) — o episódio que consumiu a
  chegada. O domínio nunca o atribui (o id do episódio é gerado pela BD na mesma
  transacção): é escrito pelo adaptador no `UPDATE` e entra no agregado apenas por
  reconstrução.
- Novo método `IniciarConsulta(medicoID string, em time.Time) error` (`TRIADO→EM_CONSULTA`):
  - só de `TRIADO` (senão `CategoriaConflito`, "só é possível iniciar a consulta de uma
    chegada triada");
  - `medicoID` (o actor) obrigatório e **igual ao `medico_id` da chegada** (senão
    `CategoriaProibido`, "só o médico atribuído pode iniciar a consulta");
  - transita para `EM_CONSULTA`.

A fila clínica não muda: o read-model já filtra `TRIADO`, pelo que o doente sai da fila
automaticamente ao entrar em consulta.

### 3.2 Coordenação — portas ACL no BC Clínico

A aplicação do Clínico ganha duas portas (em `internal/application/clinico/ports.go`),
implementadas por adaptadores que conhecem a Recepção:

```
// ChegadaTriada é o retrato mínimo de uma chegada TRIADO — DTO da porta, sem
// tipos do domínio Recepção.
type ChegadaTriada struct {
    DoenteID        string
    MedicoID        string
    EspecialidadeID string
}

// LeitorRecepcao é a porta anti-corrupção para leitura do BC Recepção.
type LeitorRecepcao interface {
    // ChegadaTriada devolve a chegada se existir e estiver TRIADO
    // (CategoriaNaoEncontrado caso contrário).
    ChegadaTriada(ctx context.Context, chegadaID string) (ChegadaTriada, error)
}

// ConsumidorChegadas consome a chegada TRIADO e cria o episódio, atomicamente.
type ConsumidorChegadas interface {
    // ConsumirEIniciar insere o episódio e transita a chegada
    // TRIADO→EM_CONSULTA (CAS por estado e médico) numa única transacção.
    // Devolve o id do episódio criado.
    ConsumirEIniciar(ctx context.Context, chegadaID, medicoID string, episodio *dominio.EpisodioClinico) (string, error)
}
```

## 4. Camada de Aplicação

### 4.1 Caso de uso `CasoIniciarConsulta` (BC Clínico)

```
Executar(ctx, actor, chegadaID) (DetalheEpisodio, error):
  1. ch := LeitorRecepcao.ChegadaTriada(chegadaID)       // 404 se não existe/não TRIADO
  2. se actor != ch.MedicoID → CategoriaProibido (403)   // guarda de dono
  3. doente := RepositorioDoentes.ObterPorID(ch.DoenteID)
     se doente.Estado() != ACTIVO → CategoriaConflito (409)   // igual ao CasoIniciarEpisodio
  4. episodio := dominio.NovoEpisodio(ch.DoenteID, CONSULTA, ch.EspecialidadeID, actor, agora())
  5. id := ConsumidorChegadas.ConsumirEIniciar(ctx, chegadaID, actor, episodio)
     // CAS a 0 linhas → o adaptador distingue 404/409/403 por releitura (§5.1)
     // e devolve o erro já categorizado; o caso de uso só propaga
  6. audita "clinico.episodio.aberto" (entidade episodio, id)
     audita "recepcao.chegada.consulta_iniciada" (entidade chegada, chegadaID)
  7. devolve DetalheEpisodio (releitura por RepositorioEpisodios.ObterPorID)
```

**Actor** = `SessaoDe(ctx).Sujeito` (o médico), nunca do corpo. A guarda de dono corre
duas vezes: no passo 2 (erro claro 403) e no CAS do passo 5 (fecha a corrida — entre a
leitura e a escrita a chegada pode ter mudado).

Entre a leitura (1) e o CAS (5) não há lock: é o CAS que decide. Se outro pedido consumiu
a chegada entretanto, o CAS afecta 0 linhas → 409.

### 4.2 Auditoria

| Acção | Entidade | Quando |
|---|---|---|
| `clinico.episodio.aberto` | `episodio` | sempre que o episódio nasce (reutiliza a acção existente) |
| `recepcao.chegada.consulta_iniciada` | `chegada` | a chegada foi consumida pela consulta |

Ambas com actor = médico, append-only, 10 anos.

## 5. Adaptadores

### 5.1 Adaptador de integração — `integracao_consulta_repo.go` (pgrepo)

Implementa `ConsumidorChegadas` e `LeitorRecepcao`. É o único componente que conhece os
dois contextos (Camada 3 — permitido; a regra de dependência proíbe infra no domínio, não
adaptadores multi-contexto).

```sql
BEGIN;
  INSERT INTO clinico.episodios (doente_id, tipo, especialidade_id, medico_id,
                                 inicio, estado, ...)
  VALUES ($..., 'CONSULTA', ..., 'ABERTO') RETURNING id;    -- via snapshot do agregado

  UPDATE recepcao.chegadas
     SET estado = 'EM_CONSULTA', episodio_id = $id, actualizado_em = now()
   WHERE id = $chegadaID AND estado = 'TRIADO' AND medico_id = $medicoID;  -- CAS + dono
  -- 0 linhas afectadas → ROLLBACK. Releitura fora da tx para distinguir:
  --   não existe → 404 · estado ≠ TRIADO → 409 · médico ≠ actor → 403
COMMIT;
```

Tudo ou nada: se o CAS falhar, o episódio não fica criado. Duplo-clique/corrida → o
segundo pedido falha no CAS → 409. Violações das restrições da migração 0004
(`23505`/`23514`) traduzidas para `CategoriaConflito`.

### 5.2 HTTP — `clinico_consulta_handler.go`

Handler novo e separado (padrão dos handlers enxutos). Regista a rota no grupo das
chegadas mas injecta o caso de uso do Clínico:

| Método + rota | Caso de uso | RBAC | Resposta |
|---|---|---|---|
| `POST /api/v1/chegadas/:cid/iniciar-consulta` | IniciarConsulta | **Médico** (só) | `201` + `DetalheEpisodio` |

Sem corpo no pedido (tudo vem da chegada e da sessão). O Enfermeiro pode iniciar
episódios genéricos (ADR-027), mas **não** consumir a fila — iniciar a consulta é acto do
médico atribuído. Erros: 401 sem sessão, 403 papel/dono, 404 chegada, 409 estado/doente
inactivo.

### 5.3 Persistência — `migrations/recepcao/0004_chegadas_em_consulta.sql` (forward-only)

```sql
-- Estende o enum de estado da chegada com EM_CONSULTA.
ALTER TABLE recepcao.chegadas DROP CONSTRAINT chegadas_estado_check;
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_estado_check
    CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU','TRIADO','EM_CONSULTA'));

-- O episódio que consumiu a chegada (uuid nu — sem FK cross-context).
ALTER TABLE recepcao.chegadas ADD COLUMN episodio_id uuid;

-- Uma chegada EM_CONSULTA aponta obrigatoriamente para o seu episódio.
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_em_consulta_episodio_check
    CHECK (estado <> 'EM_CONSULTA' OR episodio_id IS NOT NULL);

-- 1:1 — um episódio consome no máximo uma chegada (defesa em profundidade;
-- a garantia primária é a transacção do adaptador de integração).
CREATE UNIQUE INDEX IF NOT EXISTS chegadas_episodio_id_unico
    ON recepcao.chegadas (episodio_id) WHERE episodio_id IS NOT NULL;
```

### 5.4 Composition root

`internal/platform/app.go` liga o adaptador de integração ao caso de uso e o handler ao
router — único sítio que conhece os tipos concretos (padrão de DI existente).

## 6. Erros

Categorias existentes, sem novas: `CategoriaNaoEncontrado` (404, chegada inexistente ou
não `TRIADO` na leitura), `CategoriaProibido` (403, actor não é o médico atribuído),
`CategoriaConflito` (409, chegada já consumida na corrida / doente não activo),
`CategoriaValidacao` (400, id malformado). RFC 7807 no handler, mensagens PT-PT.

## 7. Outbox — fora de âmbito (dívida mantém-se)

O evento de integração `recepcao.chegada.consulta_iniciada` via Outbox fica na dívida
existente (a par dos eventos do Lab, ADR-035). Esta entrega não precisa dele: a
consistência é transaccional e síncrona. Quando o Outbox for implementado, esta acção é
candidata a evento.

## 8. Testes e Cobertura

- **Domínio ≥85%:** `Chegada.IniciarConsulta` — transição de `TRIADO`; conflito a partir
  de cada estado ≠ `TRIADO`; médico errado → `CategoriaProibido`; médico vazio;
  snapshot/reconstrução com `episodioID`.
- **Aplicação ≥75%:** fakes das duas portas; fluxo feliz (episódio CONSULTA, médico =
  actor, auditorias registadas); chegada inexistente/não-TRIADO → 404; actor ≠ médico →
  403; doente inactivo → 409; falha do consumidor propaga sem auditar.
- **Adaptadores ≥60%:** teste HTTP com duplos (RBAC — Enfermeiro/Administrativo → 403;
  actor da sessão; códigos de estado); integração `//go:build integration` contra
  Postgres real — atomicidade (CAS a falhar → episódio não existe), corrida (2.º pedido →
  409), médico errado no CAS → 0 linhas, restrições da migração 0004 (`23505` no índice
  único, `23514` no CHECK de episódio obrigatório).

## 9. ADR e Critérios de Saída

- **ADR-036 — "Integração Recepção→Clínico: início da consulta"** (próximo número livre;
  a redigir como task do plano). Regista: estado `EM_CONSULTA`, guarda de dono (médico
  atribuído), caso de uso no Clínico com portas ACL, e — a decisão estruturante — o padrão
  de **escrita cross-BC síncrona por transacção única num adaptador de integração**, com
  os rejeitados (compensação, Outbox) e o critério de quando usar cada um.

**Critérios de saída:**
1. O médico atribuído inicia a consulta a partir da fila e recebe o episódio `ABERTO`
   (tipo `CONSULTA`) na resposta (`201`).
2. A chegada transita `TRIADO→EM_CONSULTA` e sai da fila clínica; regista o
   `episodio_id` que a consumiu.
3. Transição + criação são atómicas: nunca existe episódio sem chegada consumida nem
   chegada consumida sem episódio.
4. Só o médico atribuído pode iniciar (403 no domínio e no CAS); duplo-clique/corrida →
   409; o episódio não sabe que nasceu de uma fila (zero colunas novas no Clínico).
5. Sem FK cross-context; 1:1 chegada↔episódio garantido por `UNIQUE` parcial.
6. Comando auditado nos dois contextos; cobertura nos limiares.

---

## Fora de Âmbito (futuro)

- **Sinais vitais da triagem no EHR** do episódio (o segundo diferimento da ADR-034).
- Evento de integração via **Outbox** (dívida existente, ADR-035).
- Desfazer o início da consulta / devolver o doente à fila (`EM_CONSULTA` é terminal na
  Recepção; o ciclo de vida continua no episódio — fechar/cancelar, ADR-027).
- Estados posteriores da chegada (p. ex. `ATENDIDO` no fecho do episódio) — a Recepção
  não acompanha a consulta; saberá dela por eventos quando o Outbox existir.
- Marcação da chegada agendada como "compareceu" na `Marcacao` (fica como está).
