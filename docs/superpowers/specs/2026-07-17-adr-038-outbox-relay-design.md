# Desenho — ADR-038: Outbox (propagação assíncrona de factos inter-BC)

- **Data**: 2026-07-17
- **Marco**: pós-Percurso Ambulatório / preparação M4
- **ADR**: ADR-038 (a registar)
- **Âmbito**: mecanismo Outbox completo (escrita transaccional + relay + dispatcher
  in-process) **e** o primeiro consumidor real de negócio: `clinico.episodio.fechado`
  → a chegada da Recepção transita `EM_CONSULTA → ATENDIDO`.

---

## 1. Contexto e problema

O sistema é um monólito modular com 5 bounded contexts. A comunicação inter-BC
está definida como **eventos via Outbox (assíncrono) + ACL** (CLAUDE.md §3), mas o
mecanismo nunca foi construído:

- A tabela `shared.outbox` existe desde M1 (`migrations/shared/0001_outbox.sql`),
  mas **sem relay e sem write-path**. `internal/adapters/outbox/doc.go` é um
  placeholder.
- Os eventos de domínio estão **definidos mas nunca emitidos** — `EpisodioFechado`,
  `EpisodioAberto`, `ProcedimentoCirurgicoConcluido`, `ReceitaEmitida`, `StockEntrado`,
  etc. são structs que cumprem `evento.EventoDominio` (com vars de conformidade),
  mas nenhum caso de uso os instancia ou persiste. É código à espera desta fatia.
- Consequência funcional concreta: quando o médico fecha a consulta, a chegada da
  Recepção fica presa em `EM_CONSULTA` (terminal na Recepção desde ADR-036). Não há
  desfecho pós-consulta. Este é o primeiro facto cross-BC que precisa de propagação
  assíncrona — e por isso o consumidor de prova desta ADR.

A dívida "auditoria pós-commit" (registada nas revisões ADR-036/037) **não é
resolvida aqui** — ver §8.

## 2. Decisões (fixadas no brainstorming)

1. **Relay = poller + dispatcher in-process** (abordagem A). Sem broker: o outbox já
   é a fila; entrega a handlers Go registados por BC. *At-least-once* → handlers
   idempotentes. LISTEN/NOTIFY fica como optimização futura trivial (o poll é a rede
   de segurança).
2. **Consumidor desta fatia**: só `clinico.episodio.fechado`. Cancelamento de
   episódio → chegada fica pendente futuro.
3. **Novo estado terminal da chegada**: `ATENDIDO` (alinha com o padrão masculino/
   particípio dos estados existentes: AGUARDA, CHAMADO, TRIADO, EM_CONSULTA).
4. **Escrita atómica no repositório dono da tx** — sem unit-of-work na Camada 2.
5. **Migração `shared/0002`** acrescenta `tentativas`/`ultimo_erro` ao outbox.

## 3. Arquitectura por camadas

### Camada 1 — Domínio (`internal/domain/shared/evento`)

Mixin de coleta de eventos embutível em agregados ricos, sem infra:

```go
// RegistoEventos é embutido por agregados que emitem eventos de domínio.
type RegistoEventos struct {
    pendentes []EventoDominio
}
func (r *RegistoEventos) RegistarEvento(e EventoDominio) { r.pendentes = append(r.pendentes, e) }
func (r *RegistoEventos) EventosPendentes() []EventoDominio { return r.pendentes }
func (r *RegistoEventos) LimparEventos() { r.pendentes = nil }
```

- `EpisodioClinico` embute `evento.RegistoEventos`. `Fechar(actor, agora)` acrescenta
  `RegistarEvento(EpisodioFechado{EpisodioID: <id>, DoenteID: <id>, Em: agora})` no
  fim do caminho de sucesso. Nenhum outro comportamento muda.
- O domínio **não** conhece JSON nem SQL.
- Nota de reconstrução: `ReconstruirEpisodio`/snapshot **não** repõem eventos
  pendentes (um agregado relido não re-emite). Só as transições de comportamento
  emitem.

### Camada 3 — Adaptadores

**Serialização** (`internal/adapters/outbox/codificador.go`): traduz
`evento.EventoDominio` → linha de outbox. `agregado` (ex.: `"episodio"`),
`tipo_evento = e.NomeEvento()`, `payload` = JSON dos campos do evento (encoding/json
sobre o struct concreto via type switch ou marshaller registado por tipo).

**Escrita transaccional** (`RepositorioEpisodios.Guardar`, já dono da tx):
após o UPDATE/INSERT do episódio e dos diagnósticos, e **antes do commit**, lê
`e.EventosPendentes()`, codifica cada um e faz `INSERT INTO shared.outbox
(agregado, tipo_evento, payload)`. Um helper partilhado `inserirEventos(ctx, tx,
[]EventoDominio)` no pacote `pgrepo` evita duplicação entre repositórios futuros.
Mesma tx ⇒ episódio-fechado-sem-evento e evento-sem-episódio são impossíveis.

**Relay** (`internal/adapters/outbox/relay.go`):

```go
type Handler func(ctx context.Context, ev EventoEntregue) error
type Relay struct { /* pool, registo map[string][]Handler, lote int, log */ }
func (r *Relay) Registar(tipoEvento string, h Handler)
func (r *Relay) ProcessarLote(ctx context.Context) (processados int, err error)
```

- `ProcessarLote` é uma passagem pura e testável: numa tx,
  `SELECT id, agregado, tipo_evento, payload FROM shared.outbox
   WHERE publicado_em IS NULL ORDER BY id FOR UPDATE SKIP LOCKED LIMIT :lote`.
- Para cada linha: despacha a todos os handlers de `tipo_evento`.
  - **Sucesso** (ou nenhum handler registado): `UPDATE … SET publicado_em = now()`.
  - **Falha**: `UPDATE … SET tentativas = tentativas + 1, ultimo_erro = $err`
    (não marca publicado). `SKIP LOCKED` garante que uma linha-veneno não bloqueia
    as outras; é retentada no ciclo seguinte.
- `EventoEntregue` expõe `TipoEvento`, `Agregado`, `Payload []byte` (o handler
  desserializa o que precisa — desacopla o consumidor do struct do produtor).

**Consumidor Recepção** — a lógica cross-BC vive num adaptador de integração
(padrão ADR-036: um adaptador pode importar dois domínios). Reutiliza/estende
`pgrepo` com `MarcarChegadaAtendida(ctx, episodioID) error`:

```sql
UPDATE recepcao.chegadas SET estado = 'ATENDIDO', actualizado_em = now()
WHERE episodio_id = $1 AND estado = 'EM_CONSULTA';
```

- **Idempotente**: 0 linhas afectadas = sucesso/no-op. Cobre (a) reentrega
  at-least-once de um evento já processado (chegada já `ATENDIDO`) e (b) episódio
  que não nasceu da fila clínica (sem chegada com aquele `episodio_id`) — mesmo
  contrato de "episódio sem chegada" da ADR-037.
- A regra de transição vive no domínio da Recepção (`Chegada.Atender()`,
  `EM_CONSULTA → ATENDIDO`); o CAS do UPDATE fecha a corrida. O handler constrói o
  agregado a partir do estado lido e aplica `Atender()` antes do UPDATE — ou, dado
  que o predicado é o próprio `WHERE estado='EM_CONSULTA'`, aplica o CAS directo e
  trata 0-linhas como no-op. **Decisão**: CAS directo com a guarda no `WHERE`,
  espelhando `ConsumirEIniciar`; a transição de domínio `Atender()` existe e é
  testada em unidade, para manter a máquina de estados como fonte de verdade das
  transições legais.

### Camada 4 — Plataforma (`internal/platform`)

- Composition root (`app.go`): constrói o `Relay`, regista o handler
  `clinico.episodio.fechado → MarcarChegadaAtendida`, e arranca o **loop do relay**
  numa goroutine com `time.Ticker` de intervalo configurável, cancelado pelo
  `context` do shutdown gracioso já existente (drena o lote em curso antes de sair).
- `config`: `OUTBOX_INTERVALO_MS` (default 2000) e `OUTBOX_LOTE` (default 100),
  validados como o resto da config.

## 4. Fluxo ponta a ponta

```
Médico fecha consulta
  └─ CasoFecharEpisodio.Executar
       ├─ episodio.Fechar()           → agrega EpisodioFechado (domínio)
       ├─ episodios.Guardar()         → UPDATE episódio + INSERT shared.outbox (1 tx)
       └─ auditor.Registar()          → auditoria (inalterada)
  ... (assíncrono) ...
Relay (ticker)
  └─ ProcessarLote
       ├─ SELECT … FOR UPDATE SKIP LOCKED
       ├─ handler(clinico.episodio.fechado) → MarcarChegadaAtendida(episodioID)
       │     └─ UPDATE recepcao.chegadas EM_CONSULTA→ATENDIDO (CAS, idempotente)
       └─ UPDATE shared.outbox SET publicado_em = now()
```

## 5. Migrações (forward-only)

**`migrations/shared/0002_outbox_tentativas.sql`**
```sql
ALTER TABLE shared.outbox ADD COLUMN IF NOT EXISTS tentativas int NOT NULL DEFAULT 0;
ALTER TABLE shared.outbox ADD COLUMN IF NOT EXISTS ultimo_erro text;
```

**`migrations/recepcao/0005_chegadas_atendido.sql`**
```sql
-- Estende o enum de estado da chegada com ATENDIDO (desfecho pós-consulta, ADR-038).
ALTER TABLE recepcao.chegadas DROP CONSTRAINT chegadas_estado_check;
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_estado_check
    CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU','TRIADO','EM_CONSULTA','ATENDIDO'));
```
(O nome `chegadas_estado_check` é o auto-gerado determinístico, confirmado em 0004.)

## 6. Observabilidade

Métricas Prometheus (pacote `observ` existente):
- `sgc_outbox_pendentes` (gauge) — linhas com `publicado_em IS NULL` por ciclo.
- `sgc_outbox_publicados_total` (counter).
- `sgc_outbox_falhas_handler_total` (counter, label `tipo_evento`).

Logs `slog`: falha de handler ao nível WARN com `id`, `tipo_evento`, `tentativas`.

## 7. Testes

- **Domínio** (≥85%): `RegistoEventos` (registar/limpar/pendentes);
  `EpisodioClinico.Fechar` emite exactamente um `EpisodioFechado`; relido não
  re-emite; `Chegada.Atender()` (EM_CONSULTA→ATENDIDO; recusa de outros estados).
- **Aplicação** (≥75%): `CasoFecharEpisodio` continua verde; fake de repositório
  verifica que os eventos pendentes chegam ao Guardar.
- **Relay** (unidade, com fakes): `ProcessarLote` marca publicado em sucesso;
  incrementa `tentativas`/`ultimo_erro` e não publica em falha; sem handler ⇒
  publica; despacho por `tipo_evento`.
- **Integração** (PG real, ≥60% adapters): escrita episódio+outbox na mesma tx
  (rollback deixa zero linhas em ambos); `ProcessarLote` real consome a linha e
  transita a chegada; **idempotência** (segundo ProcessarLote da mesma linha
  reentregue é no-op); episódio sem chegada ⇒ no-op sem erro; `SKIP LOCKED` não
  bloqueia linhas sãs perante uma veneno.

## 8. Fronteiras (fora de âmbito — YAGNI)

- **Auditoria-na-tx não é resolvida aqui.** Esta fatia prova o mecanismo de
  escrita-na-mesma-tx; encaminhar a auditoria pelo outbox mexe em toda a base de
  casos de uso e fica para ADR futura.
- Cancelamento de episódio → chegada (evento `clinico.episodio.cancelado`).
- LISTEN/NOTIFY; dead-lettering / limite de tentativas com quarentena; consumidores
  out-of-process; métricas de latência de propagação.
- Tendências/re-triagem e restantes pendentes do Percurso Ambulatório.

## 9. Riscos e mitigações

- **Reentrega duplicada** (at-least-once): mitigada por handler idempotente (CAS).
- **Linha-veneno** bloqueia a fila: mitigada por `SKIP LOCKED` (isolamento por
  linha) + `tentativas`/`ultimo_erro` para diagnóstico; quarentena fica futura.
- **Ordenação**: `ORDER BY id` dá FIFO aproximado; este fluxo tem um só tipo de
  evento sem dependência de ordem entre agregados.
- **Regra de dependência** (`go-arch-lint`): o handler cross-BC vive na Camada 3
  (adaptador de integração), nunca no domínio; o relay é Camada 3, arrancado pela
  Camada 4. Sem novas violações.

## 10. Critérios de saída

- [ ] `EpisodioClinico.Fechar` emite `EpisodioFechado`; `Guardar` persiste episódio
      + evento na mesma tx (provado por rollback).
- [ ] Relay in-process arranca na Plataforma, processa em lote com `SKIP LOCKED`,
      marca publicados e regista falhas em `tentativas`/`ultimo_erro`.
- [ ] Fecho de consulta transita a chegada `EM_CONSULTA → ATENDIDO`
      assincronamente; idempotente; episódio sem chegada é no-op.
- [ ] Migrações `shared/0002` e `recepcao/0005` forward-only aplicadas.
- [ ] Métricas de outbox expostas; shutdown gracioso drena o lote.
- [ ] Gates de cobertura verdes (85/75/60); `go-arch-lint` sem violações.
- [ ] ADR-038 registada; CLAUDE.md §6 e o índice de ADRs actualizados.
