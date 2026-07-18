-- Bounded Context: identidade
-- Migration forward-only. O papel Tesoureiro passa a sensível (exige MFA).
-- Ver docs/ERRATA-002-papel-tesoureiro.md, bloco "Revisão (2026-07-18, ADR-040)":
-- com a entrega da emissão de factura, o Tesoureiro pratica um acto irreversível
-- com efeito fiscal (numeração sequencial e cadeia hash), pelo que se junta a
-- Director, Admin, DPO e Auditor no conjunto dos papéis sensíveis.
-- A migração 0005 (que o semeou como não-sensível) NÃO é editada: o intervalo em
-- que o papel esteve não-sensível é matéria de auditoria e fica no registo.
-- Idempotente: reexecutar não altera nada.

UPDATE identidade.papeis
   SET sensivel = true
 WHERE codigo = 'Tesoureiro';
