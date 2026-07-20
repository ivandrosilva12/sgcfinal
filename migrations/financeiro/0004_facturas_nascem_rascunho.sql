-- Bounded Context: financeiro
-- Migration forward-only. Fecha o R6 da ADR-040 (ADR-042): toda a factura tem
-- de nascer RASCUNHO.
--
-- ADR-042 (R6 da ADR-040): trg_facturas_imutaveis é BEFORE UPDATE OR DELETE e não
-- cobre o INSERT, pelo que uma factura podia nascer já EMITIDA. Isso é fabricação,
-- não mutação: nada do que está selado muda, mas cria-se um documento que nunca
-- passou pelo caminho de emissão e que fica órfão da cadeia (a emissão legítima
-- seguinte lê series.ultimo_hash, não a linha fabricada).
--
-- Nota honesta de âmbito: enquanto o R7 estiver aberto — o papel da aplicação é
-- dono desta tabela e pode correr ALTER TABLE ... DISABLE TRIGGER — este trigger
-- é defesa contra erro e contra SQL directo de terceiros, NÃO contra a aplicação
-- comprometida.
--
-- A correcção vem por migração nova (0004), e NÃO por edição da 0002, pela mesma
-- razão da 0003: a 0002 já está registada em public.schema_migrations nas bases
-- de dados existentes, e o executor salta versões já aplicadas.
--
-- Idempotente: CREATE OR REPLACE sobre a função e DROP TRIGGER IF EXISTS antes de
-- recriar o trigger — reexecutar com ON_ERROR_STOP=1 não altera nada.
CREATE OR REPLACE FUNCTION financeiro.impedir_factura_nascer_emitida() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'a factura tem de nascer em RASCUNHO: estado % não permitido no INSERT', NEW.estado
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_facturas_nascem_rascunho ON financeiro.facturas;
CREATE TRIGGER trg_facturas_nascem_rascunho
    BEFORE INSERT ON financeiro.facturas
    FOR EACH ROW
    WHEN (NEW.estado <> 'RASCUNHO')
    EXECUTE FUNCTION financeiro.impedir_factura_nascer_emitida();
