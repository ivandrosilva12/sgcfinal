-- Bounded Context: identidade
-- Migration forward-only. Tabela de referência dos papéis RBAC (DDM-001, 11
-- papéis — ver docs/ERRATA-001-papeis.md). Os valores são semeados em
-- seeds/papeis.sql. A coluna sensivel marca papéis que exigirão MFA (Sprint 3).

CREATE TABLE IF NOT EXISTS identidade.papeis (
    codigo    text    PRIMARY KEY,
    descricao text    NOT NULL,
    sensivel  boolean NOT NULL DEFAULT false
);

COMMENT ON TABLE identidade.papeis IS
    'Catálogo canónico de papéis RBAC (11 papéis do DDM-001 v2.0).';

-- Integridade referencial: um papel atribuído tem de existir no catálogo.
-- utilizadores_papeis está vazia nesta fase, pelo que a constraint aplica sem
-- necessidade de dados prévios.
ALTER TABLE identidade.utilizadores_papeis
    DROP CONSTRAINT IF EXISTS fk_utilizadores_papeis_papel;
ALTER TABLE identidade.utilizadores_papeis
    ADD CONSTRAINT fk_utilizadores_papeis_papel
    FOREIGN KEY (papel_codigo) REFERENCES identidade.papeis (codigo);
