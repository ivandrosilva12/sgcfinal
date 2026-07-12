-- Bounded Context: clinico
-- Migration forward-only. Esquema extraído verbatim do DDM-001 v2.0.
--
-- Episódio clínico (agregado raiz independente) e diagnósticos CID associados.

CREATE TABLE IF NOT EXISTS clinico.episodios_clinicos (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id        uuid        NOT NULL REFERENCES clinico.doentes(id),
    tipo             text        NOT NULL CHECK (tipo IN ('CONSULTA','URGENCIA','INTERNAMENTO')),
    especialidade_id uuid        NOT NULL,
    medico_id        uuid        NOT NULL,
    inicio           timestamptz NOT NULL,
    fim              timestamptz,
    queixa_principal text,
    historia_doenca  text,
    exame_objectivo  text,
    diagnostico      text,
    plano            text,
    estado           text        NOT NULL DEFAULT 'ABERTO'
                     CHECK (estado IN ('ABERTO','FECHADO','CANCELADO')),
    criado_em        timestamptz NOT NULL DEFAULT now(),
    actualizado_em   timestamptz NOT NULL DEFAULT now(),
    fechado_em       timestamptz,
    fechado_por      uuid
);

CREATE INDEX IF NOT EXISTS idx_episodios_doente ON clinico.episodios_clinicos (doente_id, inicio DESC);
CREATE INDEX IF NOT EXISTS idx_episodios_medico ON clinico.episodios_clinicos (medico_id, inicio DESC);
CREATE INDEX IF NOT EXISTS idx_episodios_estado ON clinico.episodios_clinicos (estado) WHERE estado = 'ABERTO';

COMMENT ON TABLE clinico.episodios_clinicos IS
    'Episódio clínico (agregado raiz). FK doente_id sem cascade — os episódios sobrevivem à pseudonimização do doente.';

CREATE TABLE IF NOT EXISTS clinico.diagnosticos_cid (
    episodio_id uuid    NOT NULL REFERENCES clinico.episodios_clinicos(id) ON DELETE CASCADE,
    cid         text    NOT NULL,
    principal   boolean NOT NULL DEFAULT false,
    PRIMARY KEY (episodio_id, cid)
);
