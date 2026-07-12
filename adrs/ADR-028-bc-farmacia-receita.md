# ADR-028 — BC Farmácia: Medicamento (catálogo) e Receita/Prescrição

- **Estado:** Aceite
- **Data:** 2026-07-12
- **Marco:** M2 — Clínico Core (Sprint 9)
- **Contexto de spec:** docs/superpowers/specs/2026-07-12-sprint9-farmacia-receita-design.md

## Contexto

O Sprint 9 introduz o BC Farmácia, com o catálogo de medicamentos e a receita/prescrição
(emitida de um episódio, com validação de alergias). O modelo de dados foi extraído verbatim do
DDM-001. A gestão de stock (lotes, FEFO, movimentos) e a dispensa ficam para uma fatia seguinte.

## Decisões

1. **Dois agregados no BC Farmácia:** Medicamento (catálogo) e Receita (com itens).
2. **Porta anti-corrupção `LeitorClinico`.** A receita referencia o BC Clínico por id (sem FK
   cross-schema). O domínio/aplicação da Farmácia não importa o domínio do Clínico; um adaptador
   (`internal/adapters/farmacia/leitor_clinico.go`) implementa a porta reutilizando os
   repositórios `clinico`.
3. **`codigo_interno` por SEQUENCE.** `MED-{sequencial:05d}` via
   `farmacia.seq_codigo_medicamento` (nextval atómico).
4. **`medico_id` da receita = actor autenticado** (o prescritor). `doente_id`/`episodio_id` são
   validados por leitura cross-BC.
5. **Validação de alergias com override auditado.** Na emissão, cada medicamento é cruzado
   (texto case-insensitive: substância da alergia contida no nome genérico/comercial) com as
   alergias GRAVE/ANAFILÁCTICA do doente. Havendo colisão, a emissão é **bloqueada (422)**; o
   médico pode forçar com `ignorar_alerta_alergia` + `justificacao_alerta` (registados na
   auditoria). O bloqueio na dispensa (RN-FAR-04) fica para a fatia de stock.
6. **Categoria de erro `RegraNegocio` → 422.** Nova categoria no Shared Kernel para violações de
   regra de negócio (alergia agora; FEFO/stock no futuro).
7. **Estado EXPIRADA calculado na leitura** (expira_em < hoje) — sem batch de transição
   persistida nesta fatia. `expira_em` = emissão + 30 dias (RN-FAR-07).

## Diferimentos

- Stock: lotes, FEFO (RN-FAR-03), movimentos, fornecedores, entrada de stock, alertas de mínimo.
- Dispensa (UC-FAR-02, RN-FAR-04/05) — estados PARCIAL/DISPENSADA.
- Venda directa OTC (UC-FAR-09) e integração com Facturação.
- Psicotrópicos (RN-FAR-06) — registo especial.
- Batch de expiração persistida.
- Vocabulário controlado de forma farmacêutica / via de administração.

## Consequências

- Base para a fatia de stock/dispensa, que consumirá o catálogo e as receitas EMITIDA/PARCIAL.
- Tal como nos agregados do M2, os itens da receita são persistidos por delete-and-reinsert em
  cada `Guardar`.
