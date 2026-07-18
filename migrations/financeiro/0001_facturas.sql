-- Bounded Context: financeiro
-- Migration forward-only. Arranque do BC Financeiro (ADR-039): factura em RASCUNHO.
-- Sem FK cross-context: episodio_id e operacao_id são uuid lógicos (snapshot + id),
-- nunca FK para outros BCs. A FK itens_factura → facturas é intra-BC (permitida).
-- EMITIDA/ANULADA já figuram na CHECK para o ADR-040/041; nesta fatia só se cria RASCUNHO.

CREATE SCHEMA IF NOT EXISTS financeiro;

CREATE TABLE IF NOT EXISTS financeiro.facturas (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    estado         text        NOT NULL DEFAULT 'RASCUNHO'
                               CHECK (estado IN ('RASCUNHO','EMITIDA','ANULADA')),
    cliente_nome   text        NOT NULL,
    cliente_nif    text,
    cliente_morada text,
    episodio_id    uuid        NOT NULL,
    criado_em      timestamptz NOT NULL DEFAULT now(),
    actualizado_em timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_facturas_episodio ON financeiro.facturas (episodio_id);

CREATE TABLE IF NOT EXISTS financeiro.itens_factura (
    id                      uuid    PRIMARY KEY DEFAULT gen_random_uuid(),
    factura_id              uuid    NOT NULL REFERENCES financeiro.facturas (id) ON DELETE CASCADE,
    descricao               text    NOT NULL,
    tipo                    text    NOT NULL
                                    CHECK (tipo IN ('CONSULTA','DISPENSA','EXAME_ANALISE','ESTUDO_IMAGEM','PROCEDIMENTO_CIRURGICO')),
    operacao_id             uuid,
    quantidade              integer NOT NULL CHECK (quantidade > 0),
    preco_unitario_centimos bigint  NOT NULL CHECK (preco_unitario_centimos >= 0),
    regime_iva              text    NOT NULL CHECK (regime_iva IN ('ISENTO','STANDARD')),
    ordem                   integer NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_itens_factura_factura ON financeiro.itens_factura (factura_id);
