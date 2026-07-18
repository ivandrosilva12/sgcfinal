-- Bounded Context: financeiro
-- Migration forward-only. Estende a imutabilidade das linhas de factura ao
-- INSERT (ADR-040, §6).
--
-- A migração 0002 estabeleceu trg_itens_factura_imutaveis como
-- BEFORE UPDATE OR DELETE. Faltava o INSERT: era possível ACRESCENTAR linhas a
-- uma factura já EMITIDA por escrita directa em SQL. A adulteração não escapa ao
-- VerificarCadeia (o digestLinhas muda e a cadeia acusa-a), mas contradizia a
-- promessa da fatia de que a própria base de dados, por si só, já rejeita a
-- escrita.
--
-- A correcção vem por migração nova, e NÃO por edição da 0002, porque a 0002 já
-- está registada em public.schema_migrations nas bases de dados existentes. O
-- executor salta as versões já aplicadas, pelo que editá-la no lugar corrigiria
-- apenas as bases de dados criadas de raiz e faria o ficheiro mentir sobre o
-- estado das restantes — a divergência só apareceria mais tarde, num ambiente
-- antigo ou em produção. Com esta migração, toda a base de dados (nova ou
-- existente) percorre o mesmo caminho e converge para o mesmo estado.
--
-- Em INSERT não existe OLD (é NULL), pelo que o id da factura-mãe passa a
-- obter-se por COALESCE(NEW.factura_id, OLD.factura_id) — válido nos três casos:
-- INSERT (só NEW), UPDATE (ambos) e DELETE (só OLD).
--
-- estado_pai NULL (factura-mãe já não encontrada) NÃO bloqueia: é o que acontece
-- quando este trigger dispara pelo ON DELETE CASCADE de uma factura RASCUNHO —
-- nesse instante a linha da factura-mãe já foi removida dentro da mesma
-- instrução, e a sua própria imutabilidade já foi garantida por
-- trg_facturas_imutaveis (que teria abortado a instrução inteira se a factura
-- fosse EMITIDA/ANULADA, antes de a cascata sequer começar).
--
-- Idempotente: CREATE OR REPLACE sobre a função e DROP TRIGGER IF EXISTS antes
-- de recriar o trigger — reexecutar com ON_ERROR_STOP=1 não altera nada.

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
