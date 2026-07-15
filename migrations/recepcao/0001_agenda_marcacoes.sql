-- migrations/recepcao/0001_agenda_marcacoes.sql
-- Bounded Context: recepcao
-- Migration forward-only. Marcação: janelas de disponibilidade e marcações.
--
-- doente_id/medico_id/especialidade_id são referências textuais a outros bounded
-- contexts: SEM foreign key (regra de arquitectura). A existência/estado do doente é
-- validada pela ACL na camada de aplicação.

CREATE SCHEMA IF NOT EXISTS recepcao;

-- btree_gist é preciso para a restrição EXCLUDE que combina uma igualdade (medico_id)
-- com uma sobreposição de intervalo (tstzrange) no mesmo índice.
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS recepcao.janelas (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    medico_id        uuid        NOT NULL,
    especialidade_id uuid        NOT NULL,
    inicio           timestamptz NOT NULL,
    fim              timestamptz NOT NULL,
    criado_em        timestamptz NOT NULL DEFAULT now(),
    CHECK (fim > inicio)
);
CREATE INDEX IF NOT EXISTS idx_janelas_medico
    ON recepcao.janelas (medico_id, inicio);

-- Marcação: uma consulta agendada. A CHECK impõe a coerência estado↔motivo (uma
-- CANCELADA sem motivo é recusada pela base de dados). A EXCLUDE é defesa em
-- profundidade: a invariante de não-sobreposição vive no agregado
-- (VerificarDisponibilidade), mas a base de dados também nega marcações MARCADA
-- sobrepostas do mesmo médico — o único guarda à prova de corridas concorrentes.
CREATE TABLE IF NOT EXISTS recepcao.marcacoes (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id        uuid        NOT NULL,
    medico_id        uuid        NOT NULL,
    especialidade_id uuid        NOT NULL,
    inicio           timestamptz NOT NULL,
    fim              timestamptz NOT NULL,
    estado           text        NOT NULL CHECK (estado IN
                       ('MARCADA','CANCELADA','REMARCADA','FALTOU')),
    motivo           text,
    remarca_de       uuid        REFERENCES recepcao.marcacoes(id),
    criado_em        timestamptz NOT NULL DEFAULT now(),
    actualizado_em   timestamptz NOT NULL DEFAULT now(),
    CHECK (fim > inicio),
    CHECK (estado <> 'CANCELADA' OR motivo IS NOT NULL),
    EXCLUDE USING gist (
        medico_id WITH =,
        tstzrange(inicio, fim) WITH &&
    ) WHERE (estado = 'MARCADA')
);
CREATE INDEX IF NOT EXISTS idx_marcacoes_doente ON recepcao.marcacoes (doente_id);
CREATE INDEX IF NOT EXISTS idx_marcacoes_medico ON recepcao.marcacoes (medico_id, inicio);
