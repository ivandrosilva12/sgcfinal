-- Bounded Context: farmacia
-- Migration forward-only. Esquema extraído verbatim do DDM-001 v2.0.
--
-- Catálogo de medicamentos e receitas/prescrições. As tabelas de stock
-- (lotes, fornecedores, movimentos_stock) ficam para a fatia de stock/dispensa.

CREATE SCHEMA IF NOT EXISTS farmacia;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE SEQUENCE IF NOT EXISTS farmacia.seq_codigo_medicamento;

CREATE TABLE IF NOT EXISTS farmacia.medicamentos (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo_interno     text        NOT NULL UNIQUE,
    nome_comercial     text        NOT NULL,
    nome_generico      text        NOT NULL,
    forma_farmaceutica text        NOT NULL,
    dosagem            text        NOT NULL,
    via_administracao  text        NOT NULL,
    fabricante         text,
    requer_receita     boolean     NOT NULL DEFAULT true,
    psicotropico       boolean     NOT NULL DEFAULT false,
    classe_atc         text,
    stock_minimo       integer     NOT NULL DEFAULT 10 CHECK (stock_minimo >= 0),
    activo             boolean     NOT NULL DEFAULT true,
    criado_em          timestamptz NOT NULL DEFAULT now(),
    actualizado_em     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_medicamentos_nome
    ON farmacia.medicamentos USING gin ((nome_comercial || ' ' || nome_generico) gin_trgm_ops);

COMMENT ON TABLE farmacia.medicamentos IS
    'Catálogo de medicamentos. codigo_interno gerado por SEQUENCE (MED-NNNNN).';

CREATE TABLE IF NOT EXISTS farmacia.receitas (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    episodio_id  uuid        NOT NULL,
    doente_id    uuid        NOT NULL,
    medico_id    uuid        NOT NULL,
    emitida_em   timestamptz NOT NULL DEFAULT now(),
    estado       text        NOT NULL DEFAULT 'EMITIDA'
                 CHECK (estado IN ('EMITIDA','PARCIAL','DISPENSADA','EXPIRADA','ANULADA')),
    notas        text,
    expira_em    date        NOT NULL DEFAULT (CURRENT_DATE + INTERVAL '30 days')
);
CREATE INDEX IF NOT EXISTS idx_receitas_doente ON farmacia.receitas (doente_id, emitida_em DESC);
CREATE INDEX IF NOT EXISTS idx_receitas_episodio ON farmacia.receitas (episodio_id);

COMMENT ON TABLE farmacia.receitas IS
    'Receita/prescrição. episodio_id/doente_id referenciam o BC Clínico por id (sem FK cross-schema).';

CREATE TABLE IF NOT EXISTS farmacia.itens_receita (
    id                    uuid    PRIMARY KEY DEFAULT gen_random_uuid(),
    receita_id            uuid    NOT NULL REFERENCES farmacia.receitas(id) ON DELETE CASCADE,
    medicamento_id        uuid    NOT NULL REFERENCES farmacia.medicamentos(id),
    posologia             text    NOT NULL,
    duracao_dias          integer,
    quantidade_prescrita  integer NOT NULL CHECK (quantidade_prescrita > 0),
    quantidade_dispensada integer NOT NULL DEFAULT 0,
    notas                 text
);
CREATE INDEX IF NOT EXISTS idx_itens_receita_receita ON farmacia.itens_receita (receita_id);
