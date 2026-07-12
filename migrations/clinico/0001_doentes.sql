-- Bounded Context: clinico
-- Migration forward-only. Esquema extraído verbatim do DDM-001 v2.0.
--
-- Agregado Doente e entidades-filho (alergias, antecedentes clínicos). As
-- tabelas consentimentos e episodios_clinicos do DDM ficam para fatias futuras.

CREATE SCHEMA IF NOT EXISTS clinico;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS clinico.doentes (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    num_processo        text        NOT NULL UNIQUE,
    nome_completo       text        NOT NULL,
    data_nascimento     date        NOT NULL,
    sexo                char(1)     NOT NULL CHECK (sexo IN ('M','F','O')),
    bi                  text,
    nif                 text,
    passaporte          text,
    nacionalidade       text        NOT NULL DEFAULT 'AO',
    telefone            text        NOT NULL,
    email               text,
    morada_provincia    text,
    morada_municipio    text,
    morada_comuna       text,
    morada_bairro       text,
    morada_rua          text,
    morada_casa         text,
    morada_referencia   text,
    grupo_sanguineo     text        CHECK (grupo_sanguineo IN ('A+','A-','B+','B-','AB+','AB-','O+','O-')),
    estado              text        NOT NULL DEFAULT 'ACTIVO'
                        CHECK (estado IN ('ACTIVO','INACTIVO','FALECIDO','APAGADO')),
    falecido_em         date,
    causa_morte_cid     text,
    criado_em           timestamptz NOT NULL DEFAULT now(),
    actualizado_em      timestamptz NOT NULL DEFAULT now(),
    desactivado_em      timestamptz,
    desactivado_motivo  text,
    apagado_em          timestamptz,
    CONSTRAINT doc_identificacao CHECK (bi IS NOT NULL OR passaporte IS NOT NULL)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_doentes_bi
    ON clinico.doentes (bi) WHERE bi IS NOT NULL AND apagado_em IS NULL;
CREATE INDEX IF NOT EXISTS idx_doentes_nome
    ON clinico.doentes USING gin (nome_completo gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_doentes_telefone ON clinico.doentes (telefone);
CREATE INDEX IF NOT EXISTS idx_doentes_estado
    ON clinico.doentes (estado) WHERE desactivado_em IS NULL;

COMMENT ON TABLE clinico.doentes IS
    'Doente (agregado raiz do BC Clínico). Esquema extraído do DDM-001 v2.0.';
COMMENT ON COLUMN clinico.doentes.num_processo IS
    'Número de processo: automático "P-{ano}-{sequencial}" ou manual (migração).';

CREATE TABLE IF NOT EXISTS clinico.alergias (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id      uuid        NOT NULL REFERENCES clinico.doentes(id) ON DELETE CASCADE,
    substancia     text        NOT NULL,
    severidade     text        NOT NULL CHECK (severidade IN ('LEVE','MODERADA','GRAVE','ANAFILACTICA')),
    reaccao_tipica text,
    confirmada_em  date,
    notas          text,
    criada_em      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_alergias_doente ON clinico.alergias (doente_id);

CREATE TABLE IF NOT EXISTS clinico.antecedentes_clinicos (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id   uuid        NOT NULL REFERENCES clinico.doentes(id) ON DELETE CASCADE,
    tipo        text        NOT NULL CHECK (tipo IN ('PESSOAL','FAMILIAR','CIRURGICO','OBSTETRICO')),
    descricao   text        NOT NULL,
    cid         text,
    data_inicio date,
    activo      boolean     NOT NULL DEFAULT true,
    notas       text,
    criado_em   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_antecedentes_doente ON clinico.antecedentes_clinicos (doente_id);

-- Contador por ano para o número de processo automático.
CREATE TABLE IF NOT EXISTS clinico.processo_sequencia (
    ano    int PRIMARY KEY,
    ultimo int NOT NULL DEFAULT 0
);
