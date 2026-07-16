# ADR-036 — Integração Recepção→Clínico: início da consulta

- **Estado:** Aceite
- **Data:** 2026-07-16
- **Marco/Sprint:** Integração Percurso Ambulatório → Clínico (fecha o diferimento da ADR-034)
- **Fontes:** design em `docs/superpowers/specs/2026-07-16-integracao-inicio-consulta-design.md`; ADR-034 (Triagem), ADR-027 (Episódio Clínico).

## Contexto

A Triagem (ADR-034) deixou o doente na fila clínica com a `Chegada` em `TRIADO` —
um estado terminal: o doente nunca saía da fila e o `EpisodioClinico` nascia por um
endpoint desligado do percurso. Faltava a integração diferida: consumir a chegada
TRIADO e criar o episódio. É a primeira escrita cross-BC do sistema (as ACLs
existentes — Lab→Clínico, Recepção→Clínico — só leem).

## Decisão

1. **Novo estado `EM_CONSULTA` na `Chegada`** (`IniciarConsulta`, TRIADO→EM_CONSULTA),
   com **guarda de dono no domínio**: só o médico atribuído pode iniciar
   (`CategoriaProibido` → 403). A chegada regista o `episodio_id` que a consumiu
   (uuid **sem FK** — cross-context); o episódio não ganha nenhuma coluna.
2. **Caso de uso no BC Clínico** (`CasoIniciarConsulta`): é o Clínico que consome a
   chegada, por portas ACL com DTOs próprios (`LeitorRecepcao`, `ConsumidorChegadas`)
   — a aplicação do Clínico não importa o domínio da Recepção. Tipo fixo `CONSULTA`;
   médico do episódio = actor autenticado.
3. **Escrita cross-BC síncrona por transacção única num adaptador de integração**
   (Camada 3, importa ambos os domínios): `INSERT` do episódio + `UPDATE` CAS da
   chegada (estado + médico) numa só transacção PG. As regras correm no domínio da
   Recepção dentro da transacção (releitura em tx → `IniciarConsulta`); o CAS fecha
   a corrida (0 linhas → 409). Rejeitados: orquestração com compensação (janelas de
   inconsistência, episódio órfão) e Outbox assíncrono (acção interactiva — o médico
   espera o episódio na resposta; o Outbox continua por implementar).
   **Critério para o futuro:** escrita cross-BC iniciada por acção interactiva e na
   mesma BD → transacção única no adaptador; propagação de factos consumados entre
   BCs → eventos via Outbox quando existir.
4. **Defesa em profundidade na BD** (migração `recepcao/0004`): `CHECK` de estado
   com `EM_CONSULTA`, `CHECK (estado <> 'EM_CONSULTA' OR episodio_id IS NOT NULL)`
   e `UNIQUE` parcial sobre `episodio_id` (1:1 chegada↔episódio).
5. **HTTP/RBAC:** `POST /api/v1/chegadas/:cid/iniciar-consulta` → 201 com o
   episódio; papel **Médico** apenas. Auditoria dupla: `clinico.episodio.aberto` +
   `recepcao.chegada.consulta_iniciada`.

## Consequências

- O percurso ambulatório fica ligado de ponta a ponta: marcação → check-in →
  triagem → consulta (episódio ABERTO). A chegada EM_CONSULTA sai da fila clínica
  (o read-model filtra TRIADO).
- `EM_CONSULTA` é terminal na Recepção: o ciclo de vida continua no episódio
  (fechar/cancelar, ADR-027). A Recepção saberá do desfecho por eventos, quando o
  Outbox existir.
- Diferimentos: sinais vitais da triagem no EHR; evento de integração via Outbox;
  desfazer o início da consulta.
