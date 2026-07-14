-- Bounded Context: laboratorio
-- Migration forward-only. Requisição de análises e resultados.
--
-- episodio_id/doente_id/medico_requisitante_id são referências textuais a outros
-- bounded contexts: SEM foreign key (regra de arquitectura). A existência e o estado
-- do episódio são validados pela ACL na camada de aplicação.

CREATE TABLE IF NOT EXISTS laboratorio.requisicoes (
    id                     uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    episodio_id            uuid        NOT NULL,
    doente_id              uuid        NOT NULL,
    medico_requisitante_id uuid        NOT NULL,
    prioridade             text        NOT NULL CHECK (prioridade IN ('ROTINA','URGENTE')),
    estado                 text        NOT NULL CHECK (estado IN ('EMITIDA','CANCELADA')),
    criado_em              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_requisicoes_episodio
    ON laboratorio.requisicoes (episodio_id, criado_em DESC);

CREATE TABLE IF NOT EXISTS laboratorio.itens_requisicao (
    requisicao_id  uuid NOT NULL REFERENCES laboratorio.requisicoes(id) ON DELETE CASCADE,
    codigo_analise text NOT NULL,
    observacoes    text,
    PRIMARY KEY (requisicao_id, codigo_analise)
);

-- Resultado: uma linha por item da requisição, criada em PENDENTE na emissão.
-- As CHECK impõem a coerência estado↔timestamps↔autores (lição do Sprint 11): a base
-- de dados não aceita uma PROCESSADA sem submissor nem valor, nem uma RECUSADA sem
-- motivo. Os estados VALIDADA/CONCLUIDA e a CHECK de segregação já existem aqui,
-- embora a transição só seja implementada no Sprint 13.
CREATE TABLE IF NOT EXISTS laboratorio.resultados (
    id                       uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    requisicao_id            uuid        NOT NULL REFERENCES laboratorio.requisicoes(id),
    codigo_analise           text        NOT NULL,
    valor                    text,
    unidade                  text        NOT NULL,
    observacoes              text,
    motivo_recusa            text,
    estado                   text        NOT NULL CHECK (estado IN
                               ('PENDENTE','COLHIDA','PROCESSADA','VALIDADA','CONCLUIDA','RECUSADA')),
    tecnico_colheita_id      uuid,
    tecnico_submissor_id     uuid,
    patologista_validador_id uuid,
    colhida_em               timestamptz,
    submetida_em             timestamptz,
    validada_em              timestamptz,
    valor_critico            boolean     NOT NULL DEFAULT false,
    criado_em                timestamptz NOT NULL DEFAULT now(),
    CHECK (estado <> 'COLHIDA' OR (colhida_em IS NOT NULL AND tecnico_colheita_id IS NOT NULL)),
    CHECK (estado <> 'PROCESSADA' OR (submetida_em IS NOT NULL AND tecnico_submissor_id IS NOT NULL AND valor IS NOT NULL)),
    CHECK (estado <> 'RECUSADA' OR motivo_recusa IS NOT NULL),
    CHECK (estado NOT IN ('VALIDADA','CONCLUIDA') OR (validada_em IS NOT NULL AND patologista_validador_id IS NOT NULL)),
    -- Segregação de funções (DDM): quem valida nunca é quem submeteu. Defesa em
    -- profundidade — a invariante vive no agregado (Sprint 13), mas a BD também a nega.
    CHECK (patologista_validador_id IS NULL OR patologista_validador_id <> tecnico_submissor_id)
);
CREATE INDEX IF NOT EXISTS idx_resultados_fila       ON laboratorio.resultados (estado, criado_em);
CREATE INDEX IF NOT EXISTS idx_resultados_requisicao ON laboratorio.resultados (requisicao_id);
