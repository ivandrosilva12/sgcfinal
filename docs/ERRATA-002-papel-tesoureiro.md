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
- **Revisão (2026-07-18, ADR-040)**: com a entrega da emissão, o Tesoureiro passa
  a praticar um acto irreversível com efeito fiscal. `Tesoureiro` passa a **papel
  sensível**, exigindo MFA, ao lado de Director, Admin, DPO e Auditor. Fecha-se a
  reavaliação prevista na Decisão original, que se mantém acima como registo do
  estado anterior — o intervalo em que o papel esteve não-sensível é ele próprio
  matéria de auditoria e não deve ser apagado.
- **Impacto da revisão**: `identidade.papeisSensiveis`, `TestPapelTesoureiroSensivel`,
  `seeds/papeis.sql`, `docker/keycloak/realm-sgc.json`, CLAUDE.md §6. Acresce a
  migração forward-only `identidade/0006_papel_tesoureiro_sensivel.sql` (a 0005,
  que o semeou como não-sensível, não é editada) e a imposição efectiva de MFA nas
  rotas do BC Financeiro: `RegistarFinanceiro` passa a receber `MFAObrigatoria()`
  em `internal/platform/app.go`, sem o que a marcação do papel não teria efeito
  prático. Prova: `TestFinanceiro_Emitir_TesoureiroSemMFA_403` e
  `TestFinanceiro_Emitir_TesoureiroComMFA_Prossegue`.
- **Correcção de segurança por direito próprio**: a falta de `MFAObrigatoria()`
  nas rotas do BC Financeiro não abria brecha só para o Tesoureiro. O Director e
  o Auditor já eram papéis sensíveis desde o Sprint 3 e tinham leitura no
  Financeiro (consulta de facturas, verificação da cadeia); sem a imposição
  agora acrescentada, ambos passavam sem segundo factor nessas rotas. A
  correcção fecha esse buraco mais antigo, não apenas o do Tesoureiro. Prova:
  `TestFinanceiro_ObterFactura_DirectorSemMFA_403`.
