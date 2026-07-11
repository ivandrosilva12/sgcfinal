# ERRATA-001 — Divergência no número de papéis (RBAC)

- **Data**: 2026-07-11
- **Estado**: aberta (aguarda validação do Tech Lead)
- **Âmbito**: BC Identidade — enum `Papel` e seed `papeis`.

## Inconsistência detectada

Durante a fundação de M1, ao extrair o modelo de dados do **DDM-001 v2.0** (`unzip` do
`.docx`), verificou-se divergência com os prompts de coordenação:

| Fonte | Nº papéis | Papéis |
|-------|-----------|--------|
| `Prompts\marcos\m1-fundacoes.md` / `CLAUDE.md` (blueprint) | 8 | Director, Médico, Enfermeiro, Farmacêutico, Lab, Financeiro, Recepção, Sysadmin |
| **`DDM-001 v2.0` (modelo de dados)** | **11** | Medico, Enfermeiro, Administrativo, Farmaceutico, FarmaceuticoSenior, TecnicoLab, Patologista, Director, Admin, DPO, Auditor |

Papéis presentes no DDM-001 e ausentes na lista de "8": `FarmaceuticoSenior`,
`Patologista`, `DPO`, `Auditor`. A lista de "8" inclui `Financeiro` e `Recepção`, que não
aparecem com esse nome no DDM-001 (possivelmente mapeados para `Administrativo`).

## Decisão adoptada

**Fonte de verdade: `DDM-001` (11 papéis).** O modelo de dados é o artefacto que a BD
reflecte directamente e é mais recente. O enum `internal/domain/identidade/papel.go` e o
seed `seeds/papeis.sql` alinham-se pelos 11 valores.

## Acção pendente

- [ ] Validação humana (Tech Lead) da lista definitiva e do mapeamento
      `Financeiro`/`Recepção` ↔ `Administrativo`.
- [ ] Actualizar `CLAUDE.md` e `m1-fundacoes.md` do blueprint após validação (ou reconciliar
      via nova versão do DDM/ADR).
- [ ] Registar decisão final em ADR se implicar alteração de escopo de permissões.
