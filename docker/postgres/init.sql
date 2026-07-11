-- Inicialização do PostgreSQL (dev). Executado uma vez pelo entrypoint do
-- container na primeira criação do volume (docker-entrypoint-initdb.d).
--
-- Cria os schemas por bounded context. O esquema das tabelas é da
-- responsabilidade das migrations forward-only (make migrate), não deste ficheiro.

CREATE SCHEMA IF NOT EXISTS identidade;
CREATE SCHEMA IF NOT EXISTS auditoria;
CREATE SCHEMA IF NOT EXISTS shared;

-- Schemas dos restantes bounded contexts (estrutura preparada para marcos futuros).
CREATE SCHEMA IF NOT EXISTS clinico;
CREATE SCHEMA IF NOT EXISTS farmacia;
CREATE SCHEMA IF NOT EXISTS laboratorio;
CREATE SCHEMA IF NOT EXISTS financeiro;
