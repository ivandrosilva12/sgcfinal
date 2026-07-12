# ADR-029 — Farmácia: Stock & Dispensa

- **Estado:** Aceite
- **Data:** 2026-07-12
- **Marco:** M2 — Clínico Core (Sprint 10)
- **Contexto de spec:** docs/superpowers/specs/2026-07-12-sprint10-farmacia-stock-dispensa-design.md

## Contexto

O Sprint 10 completa o ciclo da receita: dar entrada de stock em lotes e dispensar uma receita,
consumindo stock por FEFO. O modelo de dados foi extraído verbatim do DDM-001.

## Decisões

1. **Agregados Fornecedor e Lote; Movimento como ledger append-only** (com quantidade sinalizada:
   ENTRADA positiva, SAIDA_DISPENSA negativa).
2. **FEFO puro + `FOR UPDATE`.** A alocação é uma função de domínio `AlocarFEFO` (testável
   isoladamente), alimentada pelo adaptador com os lotes válidos bloqueados por
   `SELECT ... FOR UPDATE ORDER BY validade ASC` (seguro sob concorrência).
3. **Dispensa transaccional via porta `MotorDispensa`.** Numa só transacção: decrementa lotes,
   insere movimentos SAIDA_DISPENSA e persiste a receita (quantidades + estado). As validações
   independentes de estado fresco (não-expirada, não-exceder, alergias) são feitas na aplicação
   antes; o motor revalida só o stock (com lock).
4. **Revalidação de alergias na dispensa** (RN-FAR-04) com override do farmacêutico
   (flag + justificação, auditado) — dupla barreira face à emissão (Sprint 9).
5. **Extensão do agregado Receita** (`RegistarDispensa`): não-exceder o prescrito (RN-FAR-05) e
   estados PARCIAL/DISPENSADA.
6. **`preco_unit_custo` como decimal-texto** (NUMERIC(14,4) via `::text`) — o `moeda.AOA` do Shared
   Kernel guarda cêntimos (2 casas), insuficiente.
7. **`pgrepo` excluído do gate unitário de cobertura** (é integration-only por desenho) — resolve
   a dívida estrutural que crescia a cada sprint.

## Diferimentos

- Ajuste manual de stock (UC-FAR-08, RN-FAR-09), alertas de validade/stock-baixo (UC-FAR-06/10),
  relatório de movimentos (UC-FAR-11), venda directa OTC (UC-FAR-09), job de expiração automática
  (UC-FAR-07), psicotrópicos (RN-FAR-06), transferências.

## Consequências

- O ciclo da receita fica completo (emitir → dispensar). A base de stock/movimentos sustenta os
  relatórios e alertas futuros.
