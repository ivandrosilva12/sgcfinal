-- Bounded Context: financeiro
-- Migration forward-only. Emissão da factura (ADR-040): numeração sequencial por
-- série, cadeia hash SHA-256 e imutabilidade.
--
-- A imutabilidade é defesa em profundidade: o domínio já recusa alterar uma
-- factura emitida, e os triggers abaixo garantem que nem uma escrita directa em
-- SQL o consegue — UPDATE, DELETE ou INSERT de novas linhas numa factura já
-- emitida. Espelha auditoria.impedir_mutacao (migrations/auditoria/0001).

ALTER TABLE financeiro.facturas
    ADD COLUMN IF NOT EXISTS numero        text,
    ADD COLUMN IF NOT EXISTS serie         text,
    ADD COLUMN IF NOT EXISTS sequencial    integer,
    ADD COLUMN IF NOT EXISTS data_emissao  timestamptz,
    ADD COLUMN IF NOT EXISTS hash          text,
    ADD COLUMN IF NOT EXISTS hash_anterior text,
    ADD COLUMN IF NOT EXISTS versao        integer NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS uq_facturas_numero
    ON financeiro.facturas (numero) WHERE numero IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_facturas_serie_sequencial
    ON financeiro.facturas (serie, sequencial) WHERE serie IS NOT NULL;

-- Coerência estado↔campos de emissão. hash_anterior é NOT NULL quando emitida mas
-- pode ser string vazia: é esse o valor na primeira factura de cada série.
ALTER TABLE financeiro.facturas
    DROP CONSTRAINT IF EXISTS facturas_coerencia_emissao;
ALTER TABLE financeiro.facturas
    ADD CONSTRAINT facturas_coerencia_emissao CHECK (
        (estado = 'RASCUNHO' AND numero IS NULL AND serie IS NULL
         AND sequencial IS NULL AND data_emissao IS NULL
         AND hash IS NULL AND hash_anterior IS NULL)
        OR
        (estado <> 'RASCUNHO' AND numero IS NOT NULL AND serie IS NOT NULL
         AND sequencial IS NOT NULL AND data_emissao IS NOT NULL
         AND hash IS NOT NULL AND hash_anterior IS NOT NULL)
    );

-- Cabeça de cada série: último sequencial atribuído e último elo da cadeia.
-- É a linha bloqueada com FOR UPDATE na emissão — o ponto de serialização.
CREATE TABLE IF NOT EXISTS financeiro.series (
    serie             text        PRIMARY KEY,
    ultimo_sequencial integer     NOT NULL DEFAULT 0 CHECK (ultimo_sequencial >= 0),
    ultimo_hash       text        NOT NULL DEFAULT '',
    actualizado_em    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE financeiro.series IS
    'Cabeça da numeração e da cadeia hash por série (AGT). Bloqueada com FOR UPDATE na emissão.';

-- Imutabilidade da factura emitida. A condição incide sobre OLD.estado: a própria
-- emissão parte de um RASCUNHO e passa; qualquer escrita sobre uma factura já
-- emitida é rejeitada, aconteça o que acontecer na aplicação.
CREATE OR REPLACE FUNCTION financeiro.impedir_mutacao_factura() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'factura emitida é imutável: operação % não permitida', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_facturas_imutaveis ON financeiro.facturas;
CREATE TRIGGER trg_facturas_imutaveis
    BEFORE UPDATE OR DELETE ON financeiro.facturas
    FOR EACH ROW WHEN (OLD.estado <> 'RASCUNHO')
    EXECUTE FUNCTION financeiro.impedir_mutacao_factura();

-- As linhas de uma factura emitida seguem a mesma regra — incluindo o INSERT:
-- sem ele seria possível ACRESCENTAR linhas a uma factura já emitida, o que
-- não escapa ao VerificarCadeia (o digestLinhas muda e a cadeia acusa a
-- adulteração), mas contradiria a promessa desta fatia de que a BD, por si só,
-- já rejeita a escrita. O PostgreSQL não admite subconsulta na condição WHEN de
-- um trigger, pelo que a verificação ao estado da factura-mãe fica no corpo da
-- função (em vez de WHEN, como em trg_facturas_imutaveis, que testa OLD.estado
-- directamente). Em INSERT, OLD é NULL — por isso COALESCE(NEW.factura_id,
-- OLD.factura_id) para obter o id da factura-mãe em qualquer dos três casos.
--
-- estado_pai NULL (factura-mãe já não encontrada) NÃO bloqueia: é o que acontece
-- quando este trigger dispara pelo ON DELETE CASCADE de uma factura RASCUNHO —
-- nesse instante a linha da factura-mãe já foi removida dentro da mesma
-- instrução, e a sua própria imutabilidade já foi garantida por
-- trg_facturas_imutaveis (que teria abortado a instrução inteira se a factura
-- fosse EMITIDA/ANULADA, antes de a cascata sequer começar).
CREATE OR REPLACE FUNCTION financeiro.impedir_mutacao_item_factura() RETURNS trigger AS $$
DECLARE
    estado_pai text;
BEGIN
    SELECT estado INTO estado_pai FROM financeiro.facturas
        WHERE id = COALESCE(NEW.factura_id, OLD.factura_id);
    IF estado_pai IS NOT NULL AND estado_pai <> 'RASCUNHO' THEN
        RAISE EXCEPTION 'linha de factura emitida é imutável: operação % não permitida', TG_OP
            USING ERRCODE = 'restrict_violation';
    END IF;
    -- Devolver sempre OLD faria o PostgreSQL gravar os valores antigos num
    -- BEFORE UPDATE: o UPDATE reportaria sucesso (UPDATE 1) sem alterar nada —
    -- perda silenciosa de dados. Em DELETE não há NEW; em INSERT/UPDATE tem de
    -- ser NEW — em INSERT é o próprio valor a inserir, sem o que a linha nunca
    -- entraria na tabela.
    RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_itens_factura_imutaveis ON financeiro.itens_factura;
CREATE TRIGGER trg_itens_factura_imutaveis
    BEFORE INSERT OR UPDATE OR DELETE ON financeiro.itens_factura
    FOR EACH ROW EXECUTE FUNCTION financeiro.impedir_mutacao_item_factura();
