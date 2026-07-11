-- Bounded Context: identidade
-- Migration forward-only. Esquema extraído do DDM-001 v2.0.
--
-- Perfil do utilizador. O Keycloak é a fonte de verdade da autenticação;
-- keycloak_id liga o perfil local à identidade no Keycloak.

CREATE SCHEMA IF NOT EXISTS identidade;

CREATE TABLE IF NOT EXISTS identidade.utilizadores (
    keycloak_id    uuid        PRIMARY KEY,
    nome           text        NOT NULL,
    email          text        NOT NULL UNIQUE,
    telefone       text,
    bi             text,
    activo         boolean     NOT NULL DEFAULT true,
    criado_em      timestamptz NOT NULL DEFAULT now(),
    actualizado_em timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE identidade.utilizadores IS
    'Perfil do utilizador. keycloak_id referencia a identidade no Keycloak (fonte de verdade da autenticação).';
COMMENT ON COLUMN identidade.utilizadores.bi IS
    'Bilhete de Identidade angolano (8 dígitos + 2 letras + 3 dígitos).';
COMMENT ON COLUMN identidade.utilizadores.telefone IS
    'Telemóvel no formato +244 9XX XXX XXX.';
