# ERRATA-001 — Divergência no número de papéis (RBAC)

- **Data**: 2026-07-11
- **Estado**: **resolvida** (validada; fonte de verdade = DDM-001, 11 papéis)
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

## Resolução (2026-07-11)

Decisão validada em M1/Sprint 1. Fonte de verdade fixada nos **11 papéis do DDM-001**.
`Financeiro` e `Recepção` da lista antiga mapeiam para `Administrativo`.

- [x] Validação da lista definitiva e do mapeamento `Financeiro`/`Recepção` ↔ `Administrativo`.
- [x] Reconciliar `CLAUDE.md` e `README.md` (deixavam de dizer "8 papéis" → "11 papéis").
- [x] Registar a decisão em `adrs/ADR-020-fundacao-m1.md`.
- [x] Aplicar nos artefactos: `seeds/papeis.sql`, `migrations/identidade/0003_papeis.sql`
      e `docker/keycloak/realm-sgc.json` (11 papéis).
