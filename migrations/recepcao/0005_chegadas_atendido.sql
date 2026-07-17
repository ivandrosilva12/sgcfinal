-- Bounded Context: recepcao
-- Migration forward-only. Estende o enum de estado da chegada com ATENDIDO
-- (desfecho pós-consulta: o episódio fechou — ADR-038). O nome
-- chegadas_estado_check é o auto-gerado determinístico da CHECK inline,
-- redefinido pela última vez em 0004.

ALTER TABLE recepcao.chegadas DROP CONSTRAINT chegadas_estado_check;
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_estado_check
    CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU','TRIADO','EM_CONSULTA','ATENDIDO'));
