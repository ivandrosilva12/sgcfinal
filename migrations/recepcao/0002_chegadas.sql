-- migrations/recepcao/0002_chegadas.sql
-- Bounded Context: recepcao
-- Migration forward-only. Check-in: chegada do doente e fila de espera.

-- Estende o enum de estado da marcação com COMPARECEU (desfecho do check-in). A CHECK
-- inline de 0001 tem o nome auto-gerado determinístico marcacoes_estado_check (só
-- referencia a coluna estado).
ALTER TABLE recepcao.marcacoes DROP CONSTRAINT marcacoes_estado_check;
ALTER TABLE recepcao.marcacoes ADD CONSTRAINT marcacoes_estado_check
    CHECK (estado IN ('MARCADA','CANCELADA','REMARCADA','FALTOU','COMPARECEU'));

-- Chegada: o doente presente na clínica. marcacao_id é FK interna ao schema (como
-- remarca_de em 0001); doente_id/medico_id/especialidade_id são referências textuais a
-- outros bounded contexts, SEM foreign key.
CREATE TABLE IF NOT EXISTS recepcao.chegadas (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id        uuid        NOT NULL,
    marcacao_id      uuid        REFERENCES recepcao.marcacoes(id),
    especialidade_id uuid        NOT NULL,
    medico_id        uuid,
    hora_chegada     timestamptz NOT NULL,
    estado           text        NOT NULL CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU')),
    criado_em        timestamptz NOT NULL DEFAULT now(),
    actualizado_em   timestamptz NOT NULL DEFAULT now(),
    -- Coerência: uma chegada com marcação tem sempre médico (herdado da marcação); o
    -- walk-in não tem marcação nem médico.
    CHECK (marcacao_id IS NULL OR medico_id IS NOT NULL)
);
-- Defesa em profundidade: uma chegada por marcação (o check-in duplo é negado também
-- pela guarda CAS do domínio; a BD fecha a corrida concorrente).
CREATE UNIQUE INDEX IF NOT EXISTS idx_chegadas_marcacao
    ON recepcao.chegadas (marcacao_id) WHERE marcacao_id IS NOT NULL;
-- Índice da fila: AGUARDA por especialidade, ordem FIFO por chegada.
CREATE INDEX IF NOT EXISTS idx_chegadas_fila
    ON recepcao.chegadas (estado, especialidade_id, hora_chegada);
