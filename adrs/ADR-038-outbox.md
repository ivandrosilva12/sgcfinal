# ADR-038 — Outbox: propagação assíncrona de factos inter-BC

- **Estado:** Aceite
- **Data:** 2026-07-17
- **Marco/Sprint:** pós-Percurso Ambulatório (preparação da propagação de factos cross-BC)
- **Fontes:** design em `docs/superpowers/specs/2026-07-17-adr-038-outbox-relay-design.md`;
  ADR-036 (a ponte `episodio_id`); ADR-034 (estado da chegada).

## Contexto

A comunicação inter-BC está definida desde o início como **eventos via Outbox
(assíncrono) + ACL** (CLAUDE.md §3), mas o mecanismo nunca foi construído: a tabela
`shared.outbox` existe desde M1 sem relay e sem write-path, e os eventos de domínio
(`EpisodioFechado`, `EpisodioAberto`, etc.) são structs conformes a
`evento.EventoDominio` que nenhum caso de uso alguma vez instanciava ou persistia.

A ADR-036 resolveu a primeira escrita cross-BC (síncrona, na mesma transacção,
porque era acção interactiva — o médico espera o episódio na resposta) e deixou
explícito o critério: propagação de factos *consumados* entre BCs fica para quando o
Outbox existir. Essa dívida tornou-se concreta: quando o médico fecha a consulta, a
chegada da Recepção fica presa em `EM_CONSULTA` — terminal desde a ADR-036, sem
desfecho pós-consulta. Este é o primeiro facto cross-BC que precisa de propagação
assíncrona, e serve de consumidor de prova para o mecanismo.

A dívida "auditoria pós-commit" (registada nas revisões da ADR-036/037) não é
resolvida por esta fatia — ver secção Diferido.

## Decisão

1. **Relay = poller + dispatcher in-process, sem broker.** A tabela `shared.outbox`
   já é a fila; um `Relay` (Camada 3, `internal/adapters/outbox/relay.go`) faz
   `SELECT ... FOR UPDATE SKIP LOCKED` periódico e despacha a handlers Go registados
   por `tipo_evento`. Entrega **at-least-once** — os handlers têm de ser idempotentes.
   Rejeitados: **LISTEN/NOTIFY** (optimização de latência válida no futuro, mas o
   poll continua a ser a rede de segurança contra notificações perdidas — não
   substitui o relay, só o complementaria); **broker externo (Redis/fila dedicada)**
   (nova peça de infra e novo modo de falha para um único consumidor; contraria o
   princípio de plataforma mínima on-premise, ADR-004/CLAUDE.md §2).
2. **Escrita transaccional no repositório dono da tx, não unit-of-work na Camada 2.**
   `RepositorioEpisodios.Guardar` lê `EventosPendentes()` do agregado e insere cada
   evento codificado em `shared.outbox` na mesma transacção do UPDATE/INSERT do
   episódio, via helper partilhado `pgrepo.inserirEventos` — episódio-fechado-sem-
   evento e evento-sem-episódio ficam impossíveis por construção. Rejeitado:
   introduzir uma abstracção de unit-of-work na Camada 2 só para coordenar a escrita
   do outbox — sobre-engenharia para um único repositório escritor; o padrão de
   "repositório dono da tx" já existia desde a ADR-036 e mantém-se consistente.
3. **Coleta de eventos no agregado rico, não em serviço de aplicação.** Mixin
   `evento.RegistoEventos` (Camada 1, `internal/domain/shared/evento`) embutido em
   `EpisodioClinico`; `Fechar()` chama `RegistarEvento(EpisodioFechado{...})` no fim
   do caminho de sucesso. `Snapshot`/`Reconstruir` não repõem eventos pendentes — um
   agregado relido não re-emite. O domínio não conhece JSON nem SQL; a serialização
   (`outbox.Codificar`, type switch por struct concreto → `agregado`+`payload` JSON)
   vive na Camada 3.
4. **Interface `Observador` definida no pacote `outbox` (Camada 3), implementada na
   Plataforma (Camada 4).** O relay precisa de emitir métricas
   (`Pendentes`/`Publicado`/`FalhaHandler`), mas a regra de dependência
   (`go-arch-lint`) proíbe o adaptador de importar o Prometheus da Plataforma — a
   interface inverte a dependência, tal como as portas ACL das ADR-036/037.
5. **Primeiro consumidor real: `clinico.episodio.fechado` → chegada `ATENDIDO`.**
   Novo estado terminal `ATENDIDO` na `Chegada` (alinhado com o padrão
   masculino/particípio: AGUARDA, CHAMADO, TRIADO, EM_CONSULTA), com transição de
   domínio `Chegada.Atender()` (`EM_CONSULTA → ATENDIDO`) testada em unidade. O
   handler (`pgrepo.IntegracaoPosConsulta`, adaptador de integração que importa os
   dois domínios, padrão ADR-036) aplica o CAS directo no `WHERE estado =
   'EM_CONSULTA'` — 0 linhas afectadas é no-op, cobrindo tanto a reentrega
   at-least-once de um evento já processado como um episódio que não nasceu da fila
   clínica (mesmo contrato de "episódio sem chegada" da ADR-037). Cancelamento de
   episódio → chegada fica pendente (candidato ADR-039, ver Diferido).
6. **Colunas `tentativas`/`ultimo_erro` em vez de dead-lettering.**
   `migrations/shared/0002` acrescenta as duas colunas ao outbox: falha de handler
   incrementa `tentativas` e grava `ultimo_erro`, sem marcar `publicado_em` — a linha
   é retentada no ciclo seguinte. `SKIP LOCKED` isola uma linha-veneno das sãs. Sem
   limite de tentativas nem quarentena nesta fatia — diagnóstico manual chega para o
   volume actual (um único tipo de evento).

## Consequências

**Positivas**

- O mecanismo de propagação assíncrona entre BCs, definido desde a arquitectura
  inicial (CLAUDE.md §3) mas nunca construído, existe e está provado ponta a ponta.
- O percurso ambulatório ganha desfecho: a chegada já não fica presa em
  `EM_CONSULTA` para sempre — transita a `ATENDIDO` quando a consulta fecha.
- Futuros produtores só precisam de `RegistarEvento` no agregado e de o repositório
  chamar `inserirEventos`; futuros consumidores só precisam de `Relay.Registar`. O
  padrão está pronto a reutilizar por qualquer BC.
- Isolamento por linha (`SKIP LOCKED`) significa que uma falha de um tipo de evento
  nunca bloqueia o processamento dos restantes.

**Negativas**

- Latência de propagação limitada ao intervalo do ticker (`OUTBOX_INTERVALO_MS`,
  default 2000 ms) — a Recepção só vê `ATENDIDO` alguns segundos depois do fecho,
  nunca instantaneamente. Aceitável: não é acção interactiva (ninguém espera na
  resposta HTTP), ao contrário da ADR-036.
- Sem dead-lettering: uma linha que falhe indefinidamente fica retentada para
  sempre, visível só por `tentativas`/`ultimo_erro` e pelo log WARN — exige
  vigilância operacional manual até existir quarentena.
- Auditoria continua fora da transacção do outbox (dívida pré-existente, não
  agravada nem resolvida aqui).

## Diferido

- **Candidato ADR-039**: cancelamento de episódio → chegada (evento
  `clinico.episodio.cancelado`), e/ou auditoria-na-tx via outbox (encaminhar o
  registo de auditoria pelo mesmo mecanismo mexe em toda a base de casos de uso e
  fica para uma ADR própria).
- LISTEN/NOTIFY como optimização de latência sobre o poll existente.
- Dead-lettering/quarentena por limite de tentativas.
- Consumidores out-of-process (fora do binário `cmd/api`).
- Métricas de latência de propagação (tempo entre `criado_em` e `publicado_em`).
