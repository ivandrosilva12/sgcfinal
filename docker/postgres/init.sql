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

-- Papel de runtime (ADR-043 / R7). Os PRIVILÉGIOS são dados pela migração
-- shared/0003_papel_runtime.sql; aqui dá-se apenas a CREDENCIAL de
-- desenvolvimento. Em produção, ver docs/RUNBOOK-provisionamento-bd.md: a
-- password é gerada pelo operador e NUNCA vem de git.
--
-- Este ficheiro só corre na primeira criação do volume. Numa base de dados de
-- desenvolvimento já existente, correr à mão:
--   CREATE ROLE sgc_app NOSUPERUSER NOCREATEDB NOCREATEROLE LOGIN PASSWORD 'sgc_app';
CREATE ROLE sgc_app NOSUPERUSER NOCREATEDB NOCREATEROLE LOGIN PASSWORD 'sgc_app';
