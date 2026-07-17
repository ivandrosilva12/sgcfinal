# ERRATA-002 — 12.º papel RBAC: Tesoureiro

- **Data**: 2026-07-18
- **Contexto**: arranque do BC Financeiro (ADR-039, Marco M4).
- **Divergência**: o DDM-001 v2.0 (reconciliado na ERRATA-001) fixou 11 papéis
  RBAC, sem Tesoureiro. O Marco M4 (CCD-M4 v2.0) pressupõe o papel Tesoureiro
  ("Tesoureiro + Director + Auditor assinam UAT").
- **Decisão**: acrescentar `Tesoureiro` como 12.º papel canónico, responsável
  pela facturação (escrita no BC Financeiro). **Não-sensível** nesta fatia (sem
  MFA); a exigência de MFA fica para reavaliação no ADR-040 (emissão de factura).
- **Impacto**: enum `identidade.Papel` (+1), `seeds/papeis.sql`, migração de seed
  `identidade/0005_seed_papel_tesoureiro.sql`, realm Keycloak `realm-sgc.json`,
  documentação (CLAUDE.md, doc.go). As atribuições de papel via JIT/Admin passam a
  aceitar `Tesoureiro`.
- **Rastreabilidade**: DDM-001 v2.0, CCD-M4 v2.0, ADR-039.
