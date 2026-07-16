-- Bounded Context: laboratorio
-- Migration forward-only. Correcção de resultados (Sprint 13).
--
-- corrige_resultado_id liga o resultado corrigido (VALIDADA vigente) ao original que
-- substitui (arquivado em CONCLUIDA). É uma referência DENTRO do mesmo bounded
-- context, logo com FK. As CHECK de coerência estado↔timestamps↔autores e a CHECK de
-- segregação de funções já existem desde a migração 0002 e cobrem VALIDADA/CONCLUIDA.

ALTER TABLE laboratorio.resultados
    ADD COLUMN IF NOT EXISTS corrige_resultado_id uuid NULL REFERENCES laboratorio.resultados(id);

CREATE INDEX IF NOT EXISTS idx_resultados_corrige
    ON laboratorio.resultados (corrige_resultado_id);
