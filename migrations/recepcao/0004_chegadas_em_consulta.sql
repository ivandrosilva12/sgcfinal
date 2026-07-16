-- 0004_chegadas_em_consulta.sql — Integração Recepção→Clínico: início da consulta
-- (ADR-036). Estende a chegada com o estado EM_CONSULTA e a referência ao episódio
-- que a consumiu. Forward-only.

-- Estende o enum de estado da chegada com EM_CONSULTA.
ALTER TABLE recepcao.chegadas DROP CONSTRAINT chegadas_estado_check;
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_estado_check
    CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU','TRIADO','EM_CONSULTA'));

-- O episódio que consumiu a chegada (uuid nu — sem FK cross-context; o episódio
-- vive no schema clinico e a integridade é da transacção do adaptador de integração).
ALTER TABLE recepcao.chegadas ADD COLUMN IF NOT EXISTS episodio_id uuid;

-- Uma chegada EM_CONSULTA aponta obrigatoriamente para o seu episódio.
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_em_consulta_episodio_check
    CHECK (estado <> 'EM_CONSULTA' OR episodio_id IS NOT NULL);

-- 1:1 — um episódio consome no máximo uma chegada (defesa em profundidade; a
-- garantia primária é a guarda CAS da transacção única).
CREATE UNIQUE INDEX IF NOT EXISTS chegadas_episodio_id_unico
    ON recepcao.chegadas (episodio_id) WHERE episodio_id IS NOT NULL;
