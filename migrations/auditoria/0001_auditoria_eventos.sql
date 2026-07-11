-- Bounded Context: auditoria
-- Migration forward-only. NÃO editar após aplicada; corrigir com nova migration.
--
-- Audit log append-only e imutável (LPDP / Lei 22/11). Retenção mínima 10 anos.
-- A imutabilidade é imposta por trigger que bloqueia UPDATE e DELETE.

CREATE SCHEMA IF NOT EXISTS auditoria;

CREATE TABLE IF NOT EXISTS auditoria.auditoria_eventos (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor        text        NOT NULL,
    accao        text        NOT NULL,
    entidade     text,
    entidade_id  text,
    detalhe      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ocorrido_em  timestamptz NOT NULL DEFAULT now(),
    registado_em timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE auditoria.auditoria_eventos IS
    'Audit log append-only e imutável. Retenção mínima: 10 anos (LPDP / Lei 22/11).';

CREATE INDEX IF NOT EXISTS idx_auditoria_eventos_ocorrido_em
    ON auditoria.auditoria_eventos (ocorrido_em);
CREATE INDEX IF NOT EXISTS idx_auditoria_eventos_actor
    ON auditoria.auditoria_eventos (actor);

-- Trigger de imutabilidade: qualquer UPDATE/DELETE é rejeitado.
CREATE OR REPLACE FUNCTION auditoria.impedir_mutacao() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'auditoria_eventos é append-only: operação % não permitida', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_auditoria_imutavel ON auditoria.auditoria_eventos;
CREATE TRIGGER trg_auditoria_imutavel
    BEFORE UPDATE OR DELETE ON auditoria.auditoria_eventos
    FOR EACH ROW EXECUTE FUNCTION auditoria.impedir_mutacao();
