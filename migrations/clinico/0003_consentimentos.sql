-- Consentimentos LPDP do doente (DDM-001 v2.0). Finalidade CIRURGIA acrescentada
-- pela adenda v2.1; o anexo obrigatório para CIRURGIA é imposto no domínio.
CREATE TABLE IF NOT EXISTS clinico.consentimentos (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id     uuid        NOT NULL REFERENCES clinico.doentes(id),
    finalidade    text        NOT NULL CHECK (finalidade IN
                    ('TRATAMENTO','COMUNICACAO','PARTILHA_SEGURADORA','INVESTIGACAO','CIRURGIA')),
    concedido     boolean     NOT NULL,
    documento_url text,
    concedido_em  date        NOT NULL,
    revogado_em   date,
    criado_em     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_consentimentos_doente
    ON clinico.consentimentos (doente_id, concedido_em DESC);

COMMENT ON TABLE clinico.consentimentos IS
    'Consentimentos LPDP do doente; anexo (documento_url) obrigatório para finalidade CIRURGIA (invariante de domínio).';
