-- Bounded Context: farmacia
-- Migration forward-only. Esquema extraído verbatim do DDM-001 v2.0.
--
-- Gestão de stock: fornecedores, lotes (com FEFO) e movimentos de stock.

CREATE TABLE IF NOT EXISTS farmacia.fornecedores (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    nome        text        NOT NULL,
    nif         text,
    contacto    text,
    activo      boolean     NOT NULL DEFAULT true,
    criado_em   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS farmacia.lotes (
    id                 uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    medicamento_id     uuid          NOT NULL REFERENCES farmacia.medicamentos(id),
    numero_lote        text          NOT NULL,
    validade           date          NOT NULL,
    quantidade_inicial integer       NOT NULL CHECK (quantidade_inicial > 0),
    quantidade_actual  integer       NOT NULL CHECK (quantidade_actual >= 0),
    preco_unit_custo   numeric(14,4) NOT NULL CHECK (preco_unit_custo >= 0),
    fornecedor_id      uuid          REFERENCES farmacia.fornecedores(id),
    entrada_em         timestamptz   NOT NULL DEFAULT now(),
    notas              text,
    -- NULLS NOT DISTINCT: dois lotes com o mesmo medicamento e número sem
    -- fornecedor associado (fornecedor_id NULL) também contam como duplicados.
    UNIQUE NULLS NOT DISTINCT (medicamento_id, numero_lote, fornecedor_id)
);
CREATE INDEX IF NOT EXISTS idx_lotes_fefo
    ON farmacia.lotes (medicamento_id, validade ASC) WHERE quantidade_actual > 0;
-- Nota: o predicado não pode depender de CURRENT_DATE (função STABLE) porque
-- índices parciais exigem predicados IMMUTABLE; o filtro dos 90 dias
-- aplica-se em tempo de consulta, não no índice.
CREATE INDEX IF NOT EXISTS idx_lotes_validade_proxima
    ON farmacia.lotes (validade) WHERE quantidade_actual > 0;

COMMENT ON TABLE farmacia.lotes IS
    'Lotes de stock por medicamento. FEFO: consumir primeiro a validade mais próxima (mas válida).';

CREATE TABLE IF NOT EXISTS farmacia.movimentos_stock (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tipo           text        NOT NULL CHECK (tipo IN ('ENTRADA','SAIDA_DISPENSA','SAIDA_VENDA','AJUSTE','EXPIRADO','TRANSFERENCIA')),
    medicamento_id uuid        NOT NULL REFERENCES farmacia.medicamentos(id),
    lote_id        uuid        NOT NULL REFERENCES farmacia.lotes(id),
    quantidade     integer     NOT NULL CHECK (quantidade != 0),
    motivo         text,
    receita_id     uuid,
    factura_id     uuid,
    ajuste_justif  text,
    realizado_por  uuid        NOT NULL,
    realizado_em   timestamptz NOT NULL DEFAULT now(),
    CHECK (tipo != 'AJUSTE' OR ajuste_justif IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS idx_movimentos_lote ON farmacia.movimentos_stock (lote_id, realizado_em DESC);
CREATE INDEX IF NOT EXISTS idx_movimentos_medicamento_data ON farmacia.movimentos_stock (medicamento_id, realizado_em DESC);
