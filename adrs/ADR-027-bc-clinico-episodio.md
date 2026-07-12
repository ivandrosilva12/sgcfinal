# ADR-027 — BC Clínico: agregado Episódio Clínico

- **Estado:** Aceite
- **Data:** 2026-07-12
- **Marco:** M2 — Clínico Core (Sprint 8)
- **Contexto de spec:** docs/superpowers/specs/2026-07-12-sprint8-clinico-episodio-design.md

## Contexto

O Sprint 8 acrescenta o segundo agregado do BC Clínico — o EpisodioClinico —
cobrindo o ciclo de vida clínico e uma projecção de leitura EHR. O modelo de dados
foi extraído verbatim do DDM-001 v2.0.

## Decisões

1. **Agregado raiz independente.** Apesar de o DDM lhe chamar "sub-agregado" do
   Doente, o EpisodioClinico é um agregado raiz próprio, com repositório próprio,
   referenciando `doente_id`. Motivos: os episódios crescem sem limite; têm ciclo
   de vida próprio; a FK `doente_id` é sem `ON DELETE CASCADE` (sobrevivem à
   pseudonimização do doente).

2. **EHR como projecção de leitura.** O EHR não é entidade — é montado em runtime
   combinando o doente (com alergias/antecedentes) e os episódios paginados.

3. **`especialidade_id` opaco.** Guardado como identificador (uuid) fornecido pelo
   chamador, sem FK — o módulo Admin/Especialidades não existe ainda.

4. **`medico_id` no pedido.** O médico responsável é indicado no corpo de iniciar
   (o disparo por Agendamento — UC-DOE-06 — fica para quando esse módulo existir).

5. **Fecho exige nota completa + ≥1 CID.** Fechar um episódio requer queixa, exame,
   diagnóstico e plano preenchidos, e pelo menos um diagnóstico CID codificado.
   Nota e diagnósticos só são editáveis enquanto ABERTO.

6. **RBAC.** Iniciar/actualizar: Médico + Enfermeiro. Fechar/cancelar: só Médico
   (o diagnóstico é acto médico). Leitura de episódios/EHR: leitura clínica
   (exclui Administrativo, que vê a demografia mas não as notas clínicas).

7. **Auditoria.** Escrita e consulta individual (episódio, EHR) auditadas; a
   listagem não é auditada (evita ruído).

## Diferimentos

- **RN-DOE-05:** episódio ABERTO bloquear a actualização de nome/BI do doente.
- **RN-DOE-03:** acesso ao EHR exigir relação clínica activa (depende de Agendamento).
- **Prescrições** (Sprint 9) e **requisições de laboratório** (módulo Laboratório).
- **UC-DOE-08:** declarar óbito cancelar os episódios ABERTOS (interacção
  Doente↔Episódio) — quando a fatia de óbito/LPDP consolidada for feita.

## Consequências

- Base para a Facturação (consome `clinico.episodio.fechado`) e o Laboratório
  (consome `clinico.episodio.aberto`) em marcos futuros.
- Tal como no agregado Doente, os diagnósticos CID são persistidos por
  delete-and-reinsert em cada `Guardar` (regenera nada — a PK é natural
  (episodio_id, cid) — mas reescreve o conjunto).
