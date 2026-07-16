-- Bounded Context: laboratorio
-- Migration forward-only. Defesa em profundidade da correcção (dívida da ADR-035).
--
-- Dois invariantes que até aqui só o compare-and-set da camada de aplicação
-- garantia passam a ser negados também pelo Postgres, mesmo a escritas que
-- contornem o repositório (bug a montante, migração de dados):
--   1. Um só resultado VIGENTE (VALIDADA) por (requisição, análise) — o
--      arquivado (CONCLUIDA) e os estados anteriores ficam fora do índice.
--   2. Um resultado só é corrigido uma vez: um único sucessor por original.

CREATE UNIQUE INDEX IF NOT EXISTS idx_resultados_um_vigente
    ON laboratorio.resultados (requisicao_id, codigo_analise)
    WHERE estado = 'VALIDADA';

CREATE UNIQUE INDEX IF NOT EXISTS idx_resultados_correccao_unica
    ON laboratorio.resultados (corrige_resultado_id)
    WHERE corrige_resultado_id IS NOT NULL;
