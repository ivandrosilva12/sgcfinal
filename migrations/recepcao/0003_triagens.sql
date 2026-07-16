-- migrations/recepcao/0003_triagens.sql
-- Bounded Context: recepcao
-- Migration forward-only. Triagem: prioridade de Manchester e sinais vitais.

-- Estende o enum de estado da chegada com TRIADO. A CHECK inline de 0002 tem o nome
-- auto-gerado determinístico chegadas_estado_check (só referencia a coluna estado).
ALTER TABLE recepcao.chegadas DROP CONSTRAINT chegadas_estado_check;
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_estado_check
    CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU','TRIADO'));

-- Triagem: registo clínico imutável, 1:1 com uma chegada. chegada_id é FK interna ao
-- schema; enfermeiro_id/medico_id são referências textuais sem FK. Os sinais vitais são
-- NULL quando não medidos; as CHECK só se aplicam a valores presentes.
CREATE TABLE IF NOT EXISTS recepcao.triagens (
    id                       uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    chegada_id               uuid         NOT NULL REFERENCES recepcao.chegadas(id),
    enfermeiro_id            uuid         NOT NULL,
    prioridade               text         NOT NULL CHECK (prioridade IN
                               ('VERMELHO','LARANJA','AMARELO','VERDE','AZUL')),
    tensao_sistolica         int          CHECK (tensao_sistolica        BETWEEN 50 AND 300),
    tensao_diastolica        int          CHECK (tensao_diastolica       BETWEEN 30 AND 200),
    frequencia_cardiaca      int          CHECK (frequencia_cardiaca     BETWEEN 20 AND 300),
    temperatura              numeric(4,1) CHECK (temperatura             BETWEEN 30 AND 45),
    frequencia_respiratoria  int          CHECK (frequencia_respiratoria BETWEEN 5 AND 80),
    saturacao_o2             int          CHECK (saturacao_o2            BETWEEN 50 AND 100),
    dor                      int          CHECK (dor                     BETWEEN 0 AND 10),
    glicemia                 int          CHECK (glicemia                BETWEEN 20 AND 600),
    peso                     numeric(5,1) CHECK (peso                    BETWEEN 0.5 AND 400),
    observacoes              text,
    triada_em                timestamptz  NOT NULL,
    criado_em                timestamptz  NOT NULL DEFAULT now(),
    -- Uma triagem por chegada (o duplicado é negado também pela guarda CAS do domínio; a
    -- BD fecha a corrida concorrente).
    UNIQUE (chegada_id)
);
CREATE INDEX IF NOT EXISTS idx_triagens_chegada ON recepcao.triagens (chegada_id);
