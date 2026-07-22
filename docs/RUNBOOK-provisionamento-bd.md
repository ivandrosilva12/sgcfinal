# RUNBOOK — Provisionamento da base de dados em produção (ADR-043)

- **Aplica-se a**: instalação nova de uma clínica (on-premise) e a qualquer
  reinstalação da base de dados do SGC Angola.
- **Fecha**: R7 da ADR-040 — a aplicação deixa de ser dona das tabelas de valor
  legal e deixa de poder desligar os triggers que as protegem.
- **Fonte de verdade das verificações**: `internal/platform/db/privilegios.go`
  (`VerificarPapelRuntime`, executada em cada arranque do servidor). As consultas
  da §5 são a transcrição dessas, adaptadas para `psql`. **Se aquele ficheiro
  mudar, este runbook muda com ele.**

Todas as saídas registadas neste documento foram **medidas** contra
`postgres:16`, num cluster limpo provisionado exactamente por estes passos
(2026-07-22). Nenhuma consulta aqui foi escrita sem ser executada.

---

## 1. Modelo — duas credenciais, dois papéis

| Papel | Variável de ambiente | Quem a usa | Poder |
|---|---|---|---|
| `sgc` | `DATABASE_MIGRATION_URL` | `api migrate`, e só ele | dono dos schemas e das tabelas; aplica DDL |
| `sgc_app` | `DATABASE_URL` | o processo servidor | DML apenas; sem posse, sem DDL |

Regra operacional que dá sentido a tudo o resto: **`DATABASE_MIGRATION_URL`
nunca entra no ambiente do processo servidor.** Um servidor comprometido não
pode usar uma credencial que não tem. `config.Carregar()` trata-a como opcional
precisamente para isto; `ExecutarMigracoes` recusa-se a correr sem ela.

### 1.1 O papel de migração é IMUTÁVEL durante a vida da instalação

A migração `shared/0003` declara `ALTER DEFAULT PRIVILEGES FOR ROLE
CURRENT_USER`, e o PostgreSQL amarra esses defaults ao **papel** que correu a
migração — aqui, `sgc`. Se um dia as migrações passarem a ser corridas por outro
papel, as tabelas novas nascem **sem privilégios para `sgc_app`** e **a falha é
silenciosa**: a migração corre com sucesso, o servidor arranca, e só o primeiro
pedido que toque na tabela nova é que falha com erro de permissão. Nenhum teste
do projecto a pode apanhar — a suite corre sempre com migrador `sgc`.

Medido em transacção revertida, na instalação provisionada por este runbook:

| Quem criou `clinico.zz_futura` | `relacl` da tabela | `has_table_privilege('sgc_app', …, 'SELECT')` |
|---|---|---|
| `sgc` (o mesmo papel de sempre) | `{sgc=arwdDxt/sgc,sgc_app=arwd/sgc}` | `t` |
| `sgc2` (papel diferente) | *(vazia)* | `f` |

**Se o papel de migração tiver mesmo de mudar** (por exemplo, uma migração de
cluster), o novo papel tem de re-declarar os defaults, uma vez, nos oito
schemas, **antes** de aplicar qualquer migração que crie tabelas:

```sql
-- ligado como o NOVO papel de migração, na base de dados sgc
DO $$
DECLARE s text;
BEGIN
    FOREACH s IN ARRAY ARRAY['clinico','farmacia','financeiro','identidade',
                             'laboratorio','recepcao','shared']
    LOOP
        EXECUTE format('ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA %I '
                       'GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO sgc_app', s);
        EXECUTE format('ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA %I '
                       'GRANT USAGE, SELECT ON SEQUENCES TO sgc_app', s);
    END LOOP;
END
$$;
ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER IN SCHEMA auditoria
    GRANT SELECT, INSERT ON TABLES TO sgc_app;
```

Note-se a assimetria deliberada: o schema `auditoria` recebe `SELECT, INSERT` e
**não** recebe defaults de sequências (ver §5.4 e a ADR-043 §6, nota N1). E confirme-se
o resultado com o passo §5.4, que é o único que detecta este erro.

---

## 2. Passo 1 — Criar os papéis e a base de dados

Ligado ao cluster como o **superutilizador de bootstrap** (tipicamente
`postgres`), uma única vez:

```sql
-- O papel de migração NÃO é superuser em produção (ao contrário de
-- desenvolvimento, onde a imagem postgres:16 cria o POSTGRES_USER como
-- SUPERUSER por construção — ADR-043 §6, R1).
CREATE ROLE sgc     LOGIN PASSWORD '<SENHA-MIGRADOR>' NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE sgc_app LOGIN PASSWORD '<SENHA-RUNTIME>'  NOSUPERUSER NOCREATEDB NOCREATEROLE;

CREATE DATABASE sgc OWNER sgc;

REVOKE ALL ON DATABASE sgc FROM PUBLIC;
GRANT CONNECT ON DATABASE sgc TO sgc_app;
```

E, já ligado à base de dados `sgc`:

```sql
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
```

Os **oito schemas** de bounded context (`auditoria`, `clinico`, `farmacia`,
`financeiro`, `identidade`, `laboratorio`, `recepcao`, `shared`) **não** se
criam à mão: cada um nasce na primeira migração do seu bounded context
(`CREATE SCHEMA IF NOT EXISTS`), e o migrador cria-os por ser dono da base de
dados. O `docker/postgres/init.sql` é infra-estrutura de **desenvolvimento** e
não deve ser montado em produção — dá ao `sgc_app` uma password conhecida e
versionada em git.

---

## 3. Passo 2 — Gerar e guardar as senhas

As duas senhas são geradas **no local** e nunca vêm de git, do binário, nem de
uma imagem de container:

```bash
# 32 bytes aleatórios, base64 sem caracteres problemáticos em DSN
openssl rand -base64 32 | tr -d '/+=' | cut -c1-32
```

Onde ficam:

| Segredo | Onde vive | Quem lhe acede |
|---|---|---|
| `<SENHA-RUNTIME>` (`sgc_app`) | gestor de segredos do deploy (Vault / Sealed Secrets — ADR-020), injectada como `DATABASE_URL` no serviço da API | o processo servidor |
| `<SENHA-MIGRADOR>` (`sgc`) | cofre da operação, **fora** do ambiente da API; injectada só no passo de migração do deploy, como `DATABASE_MIGRATION_URL` | o operador e o passo `api migrate` |

Se a senha de `sgc_app` for rodada, basta `ALTER ROLE sgc_app PASSWORD '…'` e
actualizar o segredo: os privilégios vivem no papel, não na credencial.

---

## 4. Passo 3 — Aplicar as migrações e arrancar

```bash
# 1) migração — só aqui é que a credencial de migração existe no ambiente
export DATABASE_MIGRATION_URL='postgres://sgc:<SENHA-MIGRADOR>@<host>:5432/sgc?sslmode=verify-full'
api migrate

# 2) arranque do servidor — sem DATABASE_MIGRATION_URL no ambiente
unset DATABASE_MIGRATION_URL
export DATABASE_URL='postgres://sgc_app:<SENHA-RUNTIME>@<host>:5432/sgc?sslmode=verify-full'
api
```

`api migrate` carrega a configuração completa, pelo que o passo de migração
precisa também das variáveis obrigatórias que `config.Carregar()` exige
(`REDIS_URL`, `KEYCLOAK_ISSUER`, `KEYCLOAK_ADMIN_CLIENT_ID`,
`KEYCLOAK_ADMIN_CLIENT_SECRET`). Medido: sem elas o comando termina com
`configuração inválida: variáveis em falta ou inválidas: …`.

### 4.1 O migrador nunca precisa de ser `SUPERUSER`

Não há promoção temporária, nem janela de privilégio, nem passo de despromoção a
esquecer. O migrador é criado `NOSUPERUSER` na §2 e **assim fica** — **desde que a
§2 tenha sido cumprida**, isto é, desde que `sgc_app` já exista quando o `api
migrate` corre. A dependência de ordem é real e vale a pena dizê-la por inteiro:
a `shared/0003` **cria** `sgc_app` se ele faltar, e criar um papel exige
`CREATEROLE` (ou superuser). Medido em dois clusters limpos, com `sgc_app`
inexistente:

| Migrador | `api migrate` |
|---|---|
| `NOSUPERUSER NOCREATEROLE` | pára em `shared/0003 … ERROR: permission denied to create role (SQLSTATE 42501)` |
| `NOSUPERUSER CREATEROLE` | aplica as **32** migrações; `sgc_app` nasce `NOSUPERUSER NOCREATEDB NOCREATEROLE NOLOGIN` |

O caminho deste runbook não passa por aí — a §2 cria os dois papéis antes de
qualquer migração —, pelo que o migrador pode e deve ficar sem `CREATEROLE`. Mas
saltar a criação de `sgc_app` na §2 troca uma decisão de segurança por um erro de
arranque, e a alternativa (dar `CREATEROLE` ao migrador) é dar-lhe o poder de
criar e alterar papéis para o resto da vida da instalação.

Nem sempre foi assim, e a razão fica registada porque explica a forma actual da
migração. A `shared/0003_papel_runtime.sql` reafirma os atributos de `sgc_app`, e
o PostgreSQL **só deixa alterar o atributo `SUPERUSER` a quem é superuser** —
mesmo para o repor no valor que já tem. Medido nas três configurações possíveis
de migrador, contra `postgres:16`:

| Papel que corre a migração | Resultado de `ALTER ROLE sgc_app NOSUPERUSER NOCREATEDB NOCREATEROLE` |
|---|---|
| `NOSUPERUSER NOCREATEROLE` | `ERROR: permission denied to alter role` — *Only roles with the SUPERUSER attribute may change the SUPERUSER attribute.* |
| `NOSUPERUSER CREATEROLE` (sem `ADMIN OPTION`) | o mesmo erro |
| `NOSUPERUSER CREATEROLE` **com** `GRANT sgc_app TO … WITH ADMIN OPTION` | o mesmo erro; e mesmo sem `NOSUPERUSER` na instrução, `ERROR: … Only roles with the CREATEDB attribute may change the CREATEDB attribute.` |

Com a instrução incondicional, `api migrate` num cluster de produção aplicava as
30 primeiras migrações e parava em `shared/0003 … permission denied to alter role
(SQLSTATE 42501)`. A migração passou por isso a executar o `ALTER ROLE` **só
quando há algo para corrigir** — ou seja, quando `sgc_app` tem de facto um dos
três atributos (ADR-043 §6, R1).

Medido contra um cluster limpo, com o migrador `NOSUPERUSER` do princípio ao fim:
`api migrate` aplicou as **32** migrações e `sgc_app` ficou

```
 rolname | rolsuper | rolcreatedb | rolcreaterole
---------+----------+-------------+---------------
 sgc     | f        | f           | f
 sgc_app | f        | f           | f
```

**A salvaguarda continua viva.** Se `sgc_app` chegar às migrações com um dos
atributos — provisionamento descuidado, ou alguém a promovê-lo depois —, o bloco
dispara e retira-o. Medido: `sgc_app` criado com `CREATEDB` antes das migrações
(`rolcreatedb = t`) ficou, depois do `api migrate`, com `rolcreatedb = f`. **Nesse
caso, e só nesse, o migrador tem de ser superuser** — corrigir um papel promovido
exige mesmo esse poder. Se acontecer em produção, a leitura certa não é "promover
o migrador": é descobrir quem promoveu `sgc_app` e corrigir isso primeiro, com o
superutilizador do cluster.

Depois de migrar, `sgc` continua a poder aplicar migrações normais — é dono dos
schemas e das tabelas. Medido, já `NOSUPERUSER`: `CREATE TABLE
clinico.zz_futura (id int)` executou e a tabela nasceu com `sgc_app=arwd/sgc`
pelos default privileges.

---

## 5. Passo 4 — Verificação pós-provisionamento

Correr **antes** de abrir o serviço aos utilizadores. As verificações da §5.1 a
§5.3 são as mesmas que `VerificarPapelRuntime` faz em cada arranque; a da §5.4 e
a da §5.5 cobrem o que só a suite de integração cobre hoje — em produção não há
quem as corra entre deploys.

> **Ligar como `sgc_app`, não como `sgc`.** As consultas da §5.1–§5.3 perguntam
> pelo que `current_user` **pode assumir por `SET ROLE`**; reescrevê-las com
> `'sgc_app'` literal mudaria a semântica. Corra-as com:
> `psql "postgres://sgc_app:<SENHA-RUNTIME>@<host>:5432/sgc"`.
>
> ```sql
> SELECT current_user, current_database();
> --  current_user | current_database
> -- --------------+------------------
> --  sgc_app      | sgc
> ```

### 5.1 O papel de runtime não é nem pode tornar-se administrador

```sql
SELECT coalesce(string_agg(r.rolname, ', '), '') AS papeis_administrativos
  FROM pg_roles r
 WHERE (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb
         OR r.rolname IN ('pg_write_server_files',
                          'pg_execute_server_program',
                          'pg_read_server_files'))
   AND pg_has_role(current_user, r.oid, 'MEMBER');
```
**Esperado: vazio.** Medido: vazio.

`pg_has_role(…, 'MEMBER')` e **não** `rolsuper` lido directamente do próprio
papel: os atributos não se herdam por pertença, mas o poder de os assumir com
`SET ROLE` sim. Uma verificação que leia só os atributos de `sgc_app` devolve
"está tudo bem" numa instalação em que `sgc_app` seja membro de um papel
superuser — e esse membro consegue `ALTER TABLE … DISABLE TRIGGER`
(reproduzido: `pg_trigger.tgenabled` de `O` para `D`). `'MEMBER'` e não
`'USAGE'` pela mesma razão: `GRANT … WITH INHERIT FALSE` dá `USAGE = false`,
`MEMBER = true`, e o `SET ROLE` funciona na mesma.

### 5.2 Não é dono das tabelas de valor legal, e elas existem

```sql
SELECT coalesce(string_agg(t.nome, ', '), '') AS em_falta
  FROM unnest(ARRAY['financeiro.facturas','financeiro.itens_factura',
                    'auditoria.auditoria_eventos']) AS t(nome)
 WHERE to_regclass(t.nome) IS NULL;
```
**Esperado: vazio** (uma tabela em falta significa base por migrar). Medido: vazio.

```sql
SELECT coalesce(bool_or(pg_has_role(current_user, c.relowner, 'MEMBER')), false) AS e_dono
  FROM unnest(ARRAY['financeiro.facturas','financeiro.itens_factura',
                    'auditoria.auditoria_eventos']) AS t(nome)
  JOIN pg_class c ON c.oid = to_regclass(t.nome);
```
**Esperado: `f`.** Medido: `f`.

### 5.3 Não cria objectos, e não destrói valor legal por vias sem trigger

Schemas presentes:

```sql
SELECT coalesce(string_agg(s, ', '), '') AS em_falta
  FROM unnest(ARRAY['auditoria','clinico','farmacia','financeiro',
                    'identidade','laboratorio','recepcao','shared']) AS s
 WHERE to_regnamespace(s) IS NULL;
```
**Esperado: vazio.** Medido: vazio.

`CREATE` nos schemas de negócio — avaliado sobre a **união dos papéis assumíveis
por `SET ROLE`**, e não sobre o privilégio herdado por `sgc_app`:

```sql
SELECT coalesce(string_agg(x.nome || ' (via ' || r.rolname || ')', ', '
                           ORDER BY x.nome, r.rolname), '') AS com_create
  FROM (SELECT s AS nome, to_regnamespace(s) AS ns
          FROM unnest(ARRAY['auditoria','clinico','farmacia','financeiro',
                            'identidade','laboratorio','recepcao','shared']) AS s) x
  JOIN pg_roles r ON pg_has_role(current_user, r.oid, 'MEMBER')
 WHERE has_schema_privilege(r.oid, x.ns::oid, 'CREATE');
```
**Esperado: vazio.** Medido: vazio.

`CREATE` na própria base de dados — via distinta, que permitiria criar schemas
novos fora das migrações forward-only:

```sql
SELECT current_database() AS base,
       coalesce(string_agg(r.rolname, ', ' ORDER BY r.rolname), '') AS vias
  FROM pg_roles r
 WHERE pg_has_role(current_user, r.oid, 'MEMBER')
   AND has_database_privilege(r.oid, current_database(), 'CREATE');
```
**Esperado: `vias` vazio.** Medido: `sgc | ` (base `sgc`, sem vias).

Privilégios que destroem valor legal por vias que os triggers **de linha** não
vêem — `TRUNCATE` nas três tabelas, mais `UPDATE`/`DELETE` no audit log
(`UPDATE`/`DELETE` continuam legítimos nas facturas, porque o rascunho é
mutável):

```sql
SELECT coalesce(string_agg(t.tabela || ' ' || t.priv || ' (via ' || r.rolname || ')',
                           ', ' ORDER BY t.tabela, t.priv, r.rolname), '') AS vias
  FROM (VALUES ('financeiro.facturas','TRUNCATE'),
               ('financeiro.itens_factura','TRUNCATE'),
               ('auditoria.auditoria_eventos','DELETE'),
               ('auditoria.auditoria_eventos','TRUNCATE'),
               ('auditoria.auditoria_eventos','UPDATE')) AS t(tabela, priv)
  JOIN pg_roles r ON pg_has_role(current_user, r.oid, 'MEMBER')
 WHERE has_table_privilege(r.oid, t.tabela, t.priv);
```
**Esperado: vazio.** Medido: vazio.

Os três triggers de imutabilidade (`trg_facturas_imutaveis`,
`trg_itens_factura_imutaveis`, `trg_auditoria_imutavel`) são `FOR EACH ROW`:
`TRUNCATE` não dispara nenhum. Verificar só o audit log, e só `UPDATE`/`DELETE`,
deixaria as duas tabelas da cadeia de hash sem protecção nenhuma contra
`TRUNCATE`.

### 5.4 Inventário de privilégios e default privileges

**Ligar como `sgc` (migrador)** — estas leem o catálogo e precisam de ver tudo:
`psql "postgres://sgc:<SENHA-MIGRADOR>@<host>:5432/sgc"`.

Em desenvolvimento e em CI, esta verificação é feita pela suite de integração
(`TestPrivilegios_InventarioExactoDeTabelasESequencias`). Em produção não corre
ninguém entre deploys — por isso está aqui.

Divergências do inventário exacto, relação a relação:

```sql
SELECT n.nspname || '.' || c.relname AS relacao,
       coalesce(string_agg(DISTINCT a.priv, ',' ORDER BY a.priv), '') AS tem,
       CASE WHEN c.relkind = 'S' AND n.nspname = 'auditoria'      THEN ''
            WHEN c.relkind = 'S'                                  THEN 'SELECT,USAGE'
            WHEN n.nspname = 'auditoria'                          THEN 'INSERT,SELECT'
            WHEN n.nspname = 'financeiro' AND c.relname='series'  THEN 'INSERT,SELECT,UPDATE'
            ELSE 'DELETE,INSERT,SELECT,UPDATE' END AS devia_ter
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
  LEFT JOIN LATERAL (
        SELECT x.privilege_type ||
               CASE WHEN x.is_grantable THEN ' WITH GRANT OPTION' ELSE '' END AS priv
          FROM aclexplode(coalesce(c.relacl,
                 acldefault((CASE c.relkind WHEN 'S' THEN 's' ELSE 'r' END)::"char",
                            c.relowner))) x
         WHERE x.grantee = 0 OR pg_has_role('sgc_app', x.grantee, 'MEMBER')) a ON TRUE
 WHERE c.relkind IN ('r','p','v','m','f','S')
   AND n.nspname = ANY(ARRAY['auditoria','clinico','farmacia','financeiro',
                             'identidade','laboratorio','recepcao','shared'])
 GROUP BY n.nspname, c.relname, c.relkind, c.oid
HAVING coalesce(string_agg(DISTINCT a.priv, ',' ORDER BY a.priv), '') <>
       CASE WHEN c.relkind = 'S' AND n.nspname = 'auditoria'      THEN ''
            WHEN c.relkind = 'S'                                  THEN 'SELECT,USAGE'
            WHEN n.nspname = 'auditoria'                          THEN 'INSERT,SELECT'
            WHEN n.nspname = 'financeiro' AND c.relname='series'  THEN 'INSERT,SELECT,UPDATE'
            ELSE 'DELETE,INSERT,SELECT,UPDATE' END
 ORDER BY 1;
```
**Esperado: 0 linhas.** Medido: 0 linhas (a instalação tem 31 tabelas e 3
sequências nos oito schemas).

Um privilégio **a mais** é o caso perigoso — `TRUNCATE` acima de todos — e
revoga-se por migração nova. Um privilégio **a menos** parte a aplicação, e
resolve-se acrescentando o `GRANT` à migração que criou a relação.

Grants ao nível da **coluna**, que a ACL da relação não mostra (um `GRANT UPDATE
(coluna)` no audit log dá `UPDATE` efectivo sobre essa coluna sem que
`has_table_privilege` o revele):

```sql
SELECT n.nspname || '.' || c.relname || '.' || a.attname || ' ' || x.privilege_type AS grant_de_coluna
  FROM pg_attribute a
  JOIN pg_class c ON c.oid = a.attrelid
  JOIN pg_namespace n ON n.oid = c.relnamespace,
       LATERAL aclexplode(a.attacl) x
 WHERE n.nspname = ANY(ARRAY['auditoria','clinico','farmacia','financeiro',
                             'identidade','laboratorio','recepcao','shared'])
   AND a.attacl IS NOT NULL
   AND (x.grantee = 0 OR pg_has_role('sgc_app', x.grantee, 'MEMBER'))
 ORDER BY 1;
```
**Esperado: 0 linhas.** Medido: 0 linhas. As migrações não concedem privilégio
de coluna nenhum, a papel nenhum — qualquer linha aqui é deriva.

`TRUNCATE` **efectivo**, que é o que o papel pode de facto (a ACL diz o que foi
concedido; as duas divergem se o papel for superuser):

```sql
SELECT n.nspname || '.' || c.relname AS com_truncate
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE c.relkind IN ('r','p','v','m','f')
   AND n.nspname = ANY(ARRAY['auditoria','clinico','farmacia','financeiro',
                             'identidade','laboratorio','recepcao','shared'])
   AND has_table_privilege('sgc_app', c.oid, 'TRUNCATE')
 ORDER BY 1;
```
**Esperado: 0 linhas.** Medido: 0 linhas.

Default privileges — o passo que detecta a troca silenciosa do papel de migração
(§1.1). Um só papel, e é o migrador:

```sql
SELECT DISTINCT pg_get_userbyid(d.defaclrole) AS papel_migrador FROM pg_default_acl d;
```
**Esperado: exactamente uma linha, `sgc`.** Medido: `sgc`.

```sql
SELECT s AS schema_sem_default
  FROM unnest(ARRAY['auditoria','clinico','farmacia','financeiro',
                    'identidade','laboratorio','recepcao','shared']) AS s
 WHERE NOT EXISTS (
   SELECT 1 FROM pg_default_acl d JOIN pg_namespace n ON n.oid = d.defaclnamespace
    WHERE n.nspname = s AND d.defaclobjtype = 'r'
      AND array_to_string(d.defaclacl,' ') LIKE '%sgc_app=%');
```
**Esperado: 0 linhas.** Medido: 0 linhas.

O quadro completo, para conferência visual (`\dp` não o mostra):

```sql
SELECT n.nspname AS schema, d.defaclobjtype AS tipo, array_to_string(d.defaclacl,' ') AS defaults
  FROM pg_default_acl d JOIN pg_namespace n ON n.oid = d.defaclnamespace ORDER BY 1,2;
```

Medido: **15 linhas** — oito para tabelas (`r`) e **sete** para sequências
(`S`). `auditoria` tem `sgc_app=ar/sgc` em tabelas e **não** tem linha de
sequências: é deliberado (ADR-043 §6, nota N1), porque `auditoria_eventos.id` é
`GENERATED ALWAYS AS IDENTITY` e não consome privilégio de sequência. Os
restantes sete schemas têm `sgc_app=arwd/sgc` em tabelas e `sgc_app=rU/sgc` em
sequências.

### 5.5 Triggers ligados

```sql
SELECT c.relname, t.tgname, t.tgenabled
  FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
 WHERE NOT t.tgisinternal ORDER BY 1, 2;
```

**Esperado: quatro triggers, todos com `tgenabled = 'O'`** —
`auditoria_eventos/trg_auditoria_imutavel`, `facturas/trg_facturas_imutaveis`,
`facturas/trg_facturas_nascem_rascunho`, `itens_factura/trg_itens_factura_imutaveis`.
Medido: os quatro a `O`. Um `D` aqui significa que alguém já desligou um trigger
de imutabilidade — é incidente, não configuração.

---

## 6. O que NUNCA fazer

### 6.1 Nunca `GRANT <papel privilegiado> TO sgc_app`

Vale para o dono/migrador (`GRANT sgc TO sgc_app`) e para **qualquer** papel
privilegiado, incluindo os papéis **predefinidos** do PostgreSQL —
`pg_write_all_data`, `pg_write_server_files`, `pg_execute_server_program`,
`pg_read_server_files`. Reproduzido: com `GRANT sgc TO sgc_app`, o `SET ROLE
sgc; ALTER TABLE … DISABLE TRIGGER` executou e `pg_trigger.tgenabled` passou de
`O` para `D`. Todo o R7 desaparece com esta única instrução.

Os papéis predefinidos merecem menção própria porque **escapam à intuição**: os
14 do PostgreSQL 16 têm `rolsuper`, `rolcreaterole` e `rolcreatedb` todos `f`
(medido), pelo que uma verificação por atributos os declararia inofensivos.

**`WITH INHERIT FALSE` não é atenuante.** Medido: um membro `NOINHERIT` não
herda os privilégios automaticamente — `pg_has_role(…, 'USAGE')` devolve `false`
— mas `pg_has_role(…, 'MEMBER')` devolve `true` e o `SET ROLE` seguinte
funciona na mesma. As verificações da §5 usam `MEMBER` exactamente por isto.

Se precisar de dar a um humano poder de administração, dê-o a um papel **de
pessoa**, com credencial própria e registo próprio — nunca ao papel com que a
API se liga.

### 6.2 Nunca conceder DDL a `sgc_app`

Sem `CREATE` em schema nenhum e sem `CREATE` na base de dados, o runtime não
pode criar objectos fora das migrações forward-only. Um schema de bounded
context novo trata-se na migração desse BC (`GRANT USAGE` + o DML mínimo), nunca
com um `GRANT CREATE` de conveniência.

### 6.3 Nunca montar `docker/postgres/init.sql` em produção

Dá a `sgc_app` a password de desenvolvimento, **que está em git**
(`docker/postgres/init.sql:25`). É uma proibição operacional e **nada no código a
impõe**: o ficheiro é montado pelo `docker-compose.yml` de desenvolvimento, e um
compose de produção copiado a partir dele traria a montagem atrás. Risco
declarado, não fechado — a verificação de arranque não sabe distinguir uma
password fraca de uma forte. Mitigação imediata se houver dúvida: rodar a
password (§3) e confirmar pela §5.

---

## 7. Limites conhecidos — o que este provisionamento NÃO protege

Está aqui para que ninguém leia a verificação da §5 como garantia total.

- **`pg_dump` / `pg_restore` contornam os triggers.** Um restore repõe linhas
  sem passar por trigger nenhum: pode repor facturas com hash inconsistente ou
  um audit log truncado. Quem opera os backups tem de ser tratado como quem
  opera o cluster (§7, ponto seguinte).
- **Um administrador do cluster continua a poder tudo.** A separação de
  credenciais defende contra **aplicação comprometida**, não contra acesso
  directo à base de dados. Num modelo on-premise por clínica, em que o
  administrador de sistemas é do cliente, este é um limite real; fechá-lo
  exigiria armazenamento WORM ou notarização externa, que não fazem parte desta
  fatia.
- **Privilégios de coluna e grants a `PUBLIC`** não aparecem num `\dp` distraído.
  São cobertos pelas consultas da §5.4 — corra-as, não presuma.

---

## 8. Diagnóstico rápido

| Sintoma | Causa provável | Acção |
|---|---|---|
| `papel de runtime inadequado: … é, ou pode assumir por SET ROLE, o papel administrativo <x>` | `GRANT <x> TO sgc_app` | `REVOKE <x> FROM sgc_app` (§6.1) |
| `papel de runtime inadequado: … é dono das tabelas de valor legal` | `DATABASE_URL` aponta para o migrador | corrigir a variável (§1) |
| `tabelas de valor legal ausentes (…)` / `schemas de negócio ausentes (…)` | base por migrar | correr `api migrate` (§4) |
| **(create)** `aplicar migration shared/0003_papel_runtime: ERROR: permission denied to create role` | `sgc_app` **não existe** — a §2 foi saltada | criar `sgc_app` com o superutilizador do cluster, como na §2, e repetir o `api migrate`. **Não** dar `CREATEROLE` ao migrador (§4.1) |
| **(alter)** `aplicar migration shared/0003_papel_runtime: ERROR: permission denied to alter role` | `sgc_app` **existe** mas tem um atributo de administração, e o migrador não é superuser | descobrir quem promoveu `sgc_app`; corrigir com o superutilizador do cluster (§4.1, §6.1) |
| erro de permissão numa tabela **nova**, com tudo o resto a funcionar | a migração que a criou correu com outro papel | §1.1 |
| `DATABASE_MIGRATION_URL não definida` no `api migrate` | variável ausente no passo de migração | §4 |

As duas linhas da `shared/0003` diferem numa palavra — `create` e `alter` — e
pedem acções **opostas**: uma cria o papel que falta, a outra despromove um papel
que existe a mais. Leia a palavra antes de agir.
