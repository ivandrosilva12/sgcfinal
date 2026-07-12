# ADR-026 — BC Clínico: agregado Doente

- **Estado:** Aceite
- **Data:** 2026-07-12
- **Marco:** M2 — Clínico Core (Sprint 7)
- **Contexto de spec:** docs/superpowers/specs/2026-07-12-sprint7-clinico-doente-design.md

## Contexto

O M2 abre o Bounded Context Clínico. A primeira fatia vertical é o agregado
Doente (com Alergia e AntecedenteClinico), do domínio ao HTTP. O modelo de dados
foi extraído verbatim do DDM-001 v2.0.

## Decisões

1. **Identidade gerada pela base de dados.** O `id` dos doentes (e filhos) é
   `uuid` com `DEFAULT gen_random_uuid()`, obtido por `RETURNING`. O domínio usa
   `string` e nunca gera IDs — mantém-se puro (sem `google/uuid` no domínio nem na
   aplicação, conforme o arch-lint).

2. **Número de processo híbrido.** Se o pedido trouxer um número, é usado (unicidade
   garantida por `UNIQUE`; colisão → 409). Caso contrário, é gerado
   `P-{ano}-{sequencial:06d}` a partir de um contador por ano
   (`clinico.processo_sequencia`), incrementado atomicamente por
   `INSERT ... ON CONFLICT (ano) DO UPDATE ... RETURNING`.

3. **RBAC clínico vs administrativo.** Escrita de demografia/contactos/estado:
   Administrativo, Médico, Enfermeiro. Escrita de dados clínicos
   (alergias/antecedentes): apenas Médico e Enfermeiro. Leitura: ampla (Médico,
   Enfermeiro, Administrativo, Farmacêutico, TecnicoLab, Director, DPO, Auditor).

4. **Auditoria de acesso a dados de saúde.** Além da escrita, a consulta individual
   de um doente é auditada (`clinico.doente.consultado`). A pesquisa (listagem) não
   é auditada, para evitar ruído.

5. **Rehidratação por snapshot.** O agregado Doente tem campos privados e expõe
   `Snapshot()`/`ReconstruirDoente(SnapshotDoente)`; a construção validante
   (`NovoDoente`) fica separada da rehidratação a partir da BD (dados confiáveis).

6. **Validador de NIF no Shared Kernel.** Acrescentado `identity.NovoNIF` (10
   caracteres: 10 dígitos ou 9 dígitos + 1 letra), a par de `NovoBI`/`NovoTelefone`.

7. **Pesquisa por trigram.** Índice `gin (nome_completo gin_trgm_ops)` sustenta o
   ILIKE fuzzy por nome; BI, número de processo e telefone são pesquisados por
   igualdade. Paginação por `limite` (default 20, máximo 100) e `deslocamento`.

## Diferimentos

- **LPDP:** o estado `APAGADO` e a pseudonimização (apagamento com retenção legal)
  ficam para uma fatia dedicada. A coluna `apagado_em` e o estado já existem no
  esquema, mas o fluxo de apagamento não é implementado neste sprint.
- **Consentimentos** e **episódios clínicos** (tabelas do DDM) ficam fora de âmbito
  (Sprint 8+).
- **Telefone fixo:** o validador cobre apenas o telemóvel angolano (+244 9XX).

## Consequências

- Base sólida e auditável para os episódios clínicos (Sprint 8), que referenciarão
  `clinico.doentes(id)`.
- O contador por ano exige atenção em cenários multi-instância (o
  `ON CONFLICT ... RETURNING` é atómico, pelo que é seguro sob concorrência).
