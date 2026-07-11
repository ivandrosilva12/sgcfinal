-- Bounded Context: shared (Shared Kernel)
-- Migration forward-only. Tabela Outbox para publicação assíncrona de eventos
-- de domínio inter-bounded-context (padrão Outbox). O relay é implementado em
-- marco futuro (internal/adapters/outbox).

CREATE SCHEMA IF NOT EXISTS shared;

CREATE TABLE IF NOT EXISTS shared.outbox (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    agregado     text        NOT NULL,
    tipo_evento  text        NOT NULL,
    payload      jsonb       NOT NULL,
    ocorrido_em  timestamptz NOT NULL DEFAULT now(),
    publicado_em timestamptz
);

COMMENT ON TABLE shared.outbox IS
    'Outbox de eventos de domínio (comunicação assíncrona inter-bounded-context).';

-- Índice parcial para o poller: apenas eventos ainda não publicados.
CREATE INDEX IF NOT EXISTS idx_outbox_pendentes
    ON shared.outbox (ocorrido_em)
    WHERE publicado_em IS NULL;
