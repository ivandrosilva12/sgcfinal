# ADR-034 — BC Recepção: Triagem e fila clínica

- **Estado:** Aceite
- **Data:** 2026-07-15
- **Marco/Sprint:** Percurso Ambulatório / sub-projecto Triagem (fecha o marco)
- **Fontes:** design em `docs/superpowers/specs/2026-07-15-recepcao-triagem-design.md`; ADR-033 (Check-in, precedente do mesmo BC).

## Contexto

O Check-in (ADR-033) coloca o doente na fila e permite chamá-lo (`Chegada` `CHAMADO`).
Faltava a avaliação clínica que classifica a prioridade e regista os sinais vitais, e que
ordena a fila a partir da qual o médico atende. Este sub-projecto entrega a Triagem e
fecha o marco Percurso Ambulatório.

## Decisão

1. **Novo agregado `Triagem`** no BC `recepcao`, 1:1 com a chegada, **imutável** após
   criação (sem máquina de estados) — é um registo clínico.
2. **Prioridade pelo Sistema de Manchester** (VO `PrioridadeManchester`: 5 cores com
   severidade e tempo-alvo). Os **sinais vitais** são um VO (`SinaisVitais`) de 9 campos
   opcionais com validação de intervalos plausíveis (limites de sanidade, não normais
   clínicos).
3. **Novo estado `TRIADO` na `Chegada`** (`RegistarTriada`, CHAMADO→TRIADO). É aqui que o
   **walk-in** recebe o médico atribuído (obrigatório); a chegada agendada herda o médico
   da marcação (não se re-atribui).
4. **Registo de triagem transaccional e cross-agregado:**
   `RepositorioTriagens.RegistarTriagem` transita a chegada para TRIADO (guarda
   compare-and-set sobre CHAMADO, com o médico) e insere a triagem na mesma transacção. O
   registo duplicado falha na guarda CAS (a chegada já não está CHAMADO) → Conflito, com
   defesa em profundidade pela restrição `UNIQUE (chegada_id)`.
5. **Fila clínica** — read-model das chegadas TRIADO com a sua triagem, ordenadas por
   severidade de Manchester (mais urgente primeiro) e depois por hora de chegada,
   filtráveis por médico.
6. **Leitura clínica restrita** (Médico/Enfermeiro/Director; sem Administrativo/Admin),
   porque os sinais vitais e a prioridade derivada são dado clínico (minimização LPDP,
   como no Laboratório). O registo é do Enfermeiro/Médico. Handler HTTP separado.

## Consequências

- O marco Percurso Ambulatório fica fechado: marcação → check-in → triagem → (consulta).
- O início da consulta (consumir a `Chegada` TRIADO → criar o `EpisodioClinico` no BC
  Clínico) e a ligação dos sinais vitais ao EHR ficam para um marco futuro de integração.
- A triagem é imutável neste marco (sem re-triagem/correcção).
