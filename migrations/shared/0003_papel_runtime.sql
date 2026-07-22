-- Bounded Context: shared
-- Migration forward-only. Fecha o R7 da ADR-040 (ADR-043): separa a credencial
-- de migração da de runtime.
--
-- Problema: `sgc` é o POSTGRES_USER da imagem oficial postgres:16 e, por
-- construção dessa imagem, é SUPERUSER. Não é apenas dono das tabelas. Isso
-- deixava a aplicação capaz de:
--   ALTER TABLE financeiro.facturas DISABLE TRIGGER ALL   (é dono)
--   SET session_replication_role = 'replica'              (é superuser)
--   TRUNCATE auditoria.auditoria_eventos                  (é dono; e TRUNCATE
--                                                          não é DELETE, pelo
--                                                          que o trigger de
--                                                          imutabilidade não o
--                                                          via sequer)
--
-- Correcção: nasce `sgc_app`, sem privilégios de administração e sem posse de
-- nada. O servidor liga-se com ele; `sgc` fica como dono e migrador.
--
-- Porque `shared/0003` e não outro sítio: AplicarMigracoes ordena os bounded
-- contexts alfabeticamente, pelo que `shared/` corre em último lugar. Numa base
-- criada do zero, esta é a última migração de todas e todos os schemas e tabelas
-- já existem quando ela corre. Note-se que o schema `recepcao` NÃO é criado pelo
-- init.sql (que só cria 7) — nasce em recepcao/0001_agenda_marcacoes.sql.
--
-- São precisas as duas metades, por razões diferentes:
--   GRANT ... ON ALL TABLES     cobre o que já existe agora;
--   ALTER DEFAULT PRIVILEGES    cobre o que vier em migrações futuras.
-- Nenhuma substitui a outra.
--
-- A CREDENCIAL não vive aqui. O papel nasce NOLOGIN e sem password; quem lhe dá
-- LOGIN e password é o provisionamento (docker/postgres/init.sql em dev, um
-- passo do CI, e docs/RUNBOOK-provisionamento-bd.md em produção). Uma password
-- de produção não pode estar embebida no binário nem versionada em git.
--
-- Idempotente: o papel é criado só se faltar, e GRANT/REVOKE são convergentes.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'sgc_app') THEN
        CREATE ROLE sgc_app NOSUPERUSER NOCREATEDB NOCREATEROLE NOLOGIN;
    END IF;
END
$$;

-- Reafirmar os atributos mesmo que o papel já existisse. Dar-lhe LOGIN e
-- password é legítimo (é o que o provisionamento faz); torná-lo administrador
-- não é, e este bloco desfaz isso.
--
-- Porquê condicional e não um ALTER ROLE directo: o PostgreSQL exige
-- SUPERUSER para tocar no atributo SUPERUSER de um papel — MESMO para o repor
-- no valor que ele já tem. Medido contra postgres:16 nas três configurações
-- possíveis de migrador (NOSUPERUSER NOCREATEROLE; NOSUPERUSER CREATEROLE;
-- NOSUPERUSER CREATEROLE com GRANT sgc_app TO ... WITH ADMIN OPTION): as três
-- falham com `permission denied to alter role`. Com a instrução incondicional,
-- `api migrate` numa instalação de produção — onde o migrador é NOSUPERUSER,
-- que é precisamente o que o runbook prescreve — aplicava 30 migrações e
-- parava aqui com SQLSTATE 42501. Em desenvolvimento e em CI o defeito era
-- invisível, porque a imagem postgres:16 cria o POSTGRES_USER como SUPERUSER.
--
-- A guarda NÃO fica vazia: quando sgc_app existe com algum dos três atributos
-- (provisionamento descuidado, ou alguém a promovê-lo depois), o bloco dispara
-- e retira-os — e nesse caso o migrador TEM de ser superuser, porque a
-- correcção exige mesmo esse poder. O que deixa de acontecer é pagar o preço
-- no caso normal, em que não há nada para corrigir.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles
                WHERE rolname = 'sgc_app'
                  AND (rolsuper OR rolcreatedb OR rolcreaterole)) THEN
        ALTER ROLE sgc_app NOSUPERUSER NOCREATEDB NOCREATEROLE;
    END IF;
END
$$;

-- Os sete schemas de negócio: DML completo, zero DDL.
DO $$
DECLARE s text;
BEGIN
    FOREACH s IN ARRAY ARRAY['clinico','farmacia','financeiro','identidade','laboratorio','recepcao','shared']
    LOOP
        EXECUTE format('GRANT USAGE ON SCHEMA %I TO sgc_app', s);
        EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM sgc_app', s);
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %I TO sgc_app', s);
        EXECUTE format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA %I TO sgc_app', s);
        EXECUTE format('ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA %I GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO sgc_app', s);
        EXECUTE format('ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA %I GRANT USAGE, SELECT ON SEQUENCES TO sgc_app', s);
    END LOOP;
END
$$;

-- O schema auditoria é append-only também ao nível do privilégio, e não apenas
-- por trigger. Sem UPDATE/DELETE concedidos, a imutabilidade do audit log deixa
-- de depender exclusivamente de um trigger que pudesse ser contornado; e como
-- TRUNCATE nunca é concedido a quem não é dono, o buraco do TRUNCATE fecha-se
-- na mesma passagem.
GRANT USAGE ON SCHEMA auditoria TO sgc_app;
REVOKE CREATE ON SCHEMA auditoria FROM sgc_app;
REVOKE ALL ON ALL TABLES IN SCHEMA auditoria FROM sgc_app;
GRANT SELECT, INSERT ON ALL TABLES IN SCHEMA auditoria TO sgc_app;
ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA auditoria GRANT SELECT, INSERT ON TABLES TO sgc_app;

-- Nada em public: sgc_app não vê public.schema_migrations.
