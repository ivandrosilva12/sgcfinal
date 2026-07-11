-- Bounded Context: identidade
-- Migration forward-only. Modelo RBAC (DDM-001 v2.0, 11 papéis — ver ERRATA-001).
--
-- Junção utilizador ↔ papel. A lista canónica de papéis é semeada em
-- seeds/papeis.sql. FK dentro do mesmo bounded context (permitido).

CREATE TABLE IF NOT EXISTS identidade.utilizadores_papeis (
    utilizador_id uuid        NOT NULL
        REFERENCES identidade.utilizadores (keycloak_id) ON DELETE CASCADE,
    papel_codigo  text        NOT NULL,
    atribuido_em  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (utilizador_id, papel_codigo)
);

COMMENT ON TABLE identidade.utilizadores_papeis IS
    'Atribuição de papéis RBAC a utilizadores. papel_codigo alinhado pelos 11 papéis do DDM-001.';

CREATE INDEX IF NOT EXISTS idx_utilizadores_papeis_papel
    ON identidade.utilizadores_papeis (papel_codigo);
