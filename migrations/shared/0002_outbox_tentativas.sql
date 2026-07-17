-- Bounded Context: shared (Shared Kernel)
-- Migration forward-only. Acrescenta contabilidade de reentrega ao Outbox: número
-- de tentativas de entrega e o último erro do handler (diagnóstico de mensagens
-- persistentemente falhadas; dead-lettering fica para marco futuro). ADR-038.

ALTER TABLE shared.outbox ADD COLUMN IF NOT EXISTS tentativas  int  NOT NULL DEFAULT 0;
ALTER TABLE shared.outbox ADD COLUMN IF NOT EXISTS ultimo_erro text;

COMMENT ON COLUMN shared.outbox.tentativas  IS 'Número de tentativas de entrega falhadas (relay).';
COMMENT ON COLUMN shared.outbox.ultimo_erro IS 'Mensagem do último erro de handler, para diagnóstico.';
