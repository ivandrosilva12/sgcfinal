-- Procedimento cirúrgico ambulatório (DDM-001 v2.1 adenda §4.2). State machine e
-- consistência estado↔timestamps impostas por CHECK. Consentimento obrigatório.
CREATE TABLE IF NOT EXISTS clinico.procedimentos_cirurgicos (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    episodio_id         uuid        NOT NULL REFERENCES clinico.episodios_clinicos(id),
    codigo_procedimento text        NOT NULL,
    descricao           text        NOT NULL,
    sala                text,
    cirurgiao_id        uuid        NOT NULL,
    auxiliar_id         uuid,
    anestesia           text        NOT NULL CHECK (anestesia IN
                          ('NENHUMA','LOCAL','SEDACAO_LIGEIRA','LOCO_REGIONAL')),
    anestesista_id      uuid,
    inicio              timestamptz,
    fim                 timestamptz,
    consentimento_id    uuid        NOT NULL REFERENCES clinico.consentimentos(id),
    complicacoes        text,
    observacoes         text,
    estado              text        NOT NULL CHECK (estado IN
                          ('AGENDADO','EM_CURSO','CONCLUIDO','CANCELADO')),
    criado_em           timestamptz NOT NULL DEFAULT now(),
    CHECK (fim IS NULL OR fim >= inicio),
    CHECK (
        (estado = 'AGENDADO'  AND inicio IS NULL     AND fim IS NULL) OR
        (estado = 'EM_CURSO'  AND inicio IS NOT NULL AND fim IS NULL) OR
        (estado IN ('CONCLUIDO','CANCELADO') AND inicio IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_procedimentos_episodio  ON clinico.procedimentos_cirurgicos (episodio_id);
CREATE INDEX IF NOT EXISTS idx_procedimentos_cirurgiao ON clinico.procedimentos_cirurgicos (cirurgiao_id, criado_em DESC);
CREATE INDEX IF NOT EXISTS idx_procedimentos_codigo    ON clinico.procedimentos_cirurgicos (codigo_procedimento);
