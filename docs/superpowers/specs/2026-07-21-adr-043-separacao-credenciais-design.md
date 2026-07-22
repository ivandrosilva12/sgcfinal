# Desenho — ADR-043: separação da credencial de migração da de runtime (R7)

- **Data**: 2026-07-21
- **Marco**: M4 — Financeiro (fatia de segurança transversal)
- **Fecha**: R7 da ADR-040, transportado intacto pela ADR-041 (§R4) e deixado
  explicitamente fora de âmbito pela ADR-042 (§5)
- **ADR a registar**: ADR-043
- **Estado**: desenho aprovado, por implementar

---

## 1. Contexto e problema medido

A ADR-042 fechou o R6 (a factura tem de nascer em `RASCUNHO`) com um trigger
`BEFORE INSERT` na migração `financeiro/0004`, e registou com honestidade na própria
migração que a garantia era condicional:

> «enquanto o R7 estiver aberto — o papel da aplicação é dono desta tabela e pode
> correr `ALTER TABLE ... DISABLE TRIGGER` — este trigger é defesa contra erro e
> contra SQL directo de terceiros, NÃO contra a aplicação comprometida.»

A medição feita para este desenho confirma o R7 e mostra que é **mais largo do que a
ADR-042 §5 descreve**.

### 1.1 A credencial única

Uma só credencial, `sgc`, em três sítios do repositório:

| Ficheiro | Linha | Valor |
|---|---|---|
| `.env.example` | 13 | `DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable` |
| `docker-compose.yml` | 115 | `DATABASE_URL: postgres://sgc:sgc@postgres:5432/sgc?sslmode=disable` |
| `.github/workflows/ci.yml` | 94 | `DATABASE_URL: postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable` |

Consumida indistintamente por runtime e migração:

- `ExecutarServidor` — `internal/platform/app.go:45` → `db.LigarPool(ctx, cfg.URLBaseDados)`
- `ExecutarMigracoes` — `internal/platform/app.go:326` → `db.LigarPool(ctx, cfg.URLBaseDados)`
  e `app.go:332` → `db.AplicarMigracoes(...)`

### 1.2 O papel é superuser, não apenas dono

`docker-compose.yml:10` define `POSTGRES_USER: sgc`. A imagem oficial `postgres:16` cria
o `POSTGRES_USER` como **`SUPERUSER`**. Não é, portanto, apenas o dono das tabelas: é
superuser do cluster. Três consequências, todas alcançáveis a partir do processo
servidor — isto é, a partir de qualquer via de execução de SQL que uma aplicação
comprometida obtenha:

| Ataque | Porque funciona hoje |
|---|---|
| `ALTER TABLE financeiro.facturas DISABLE TRIGGER ALL` | é dono da tabela |
| `SET session_replication_role = 'replica'` | é superuser — desliga *todos* os triggers da sessão, em toda a base de dados |
| `TRUNCATE auditoria.auditoria_eventos` | é dono; e o trigger é `BEFORE UPDATE OR DELETE`, e `TRUNCATE` não é `DELETE` |

A terceira não estava registada em risco nenhum e é a mais grave: o audit log — Princípio
Não-Negociável #3 do CLAUDE.md, retenção mínima de 10 anos por LPDP / Lei 22/11 — é
apagável com uma instrução, **sem sequer tocar em triggers**. A tabela é
`auditoria.auditoria_eventos` (`migrations/auditoria/0001_auditoria_eventos.sql`), e o
seu trigger `trg_auditoria_imutavel` cobre `UPDATE OR DELETE`, não `TRUNCATE`.

### 1.3 O que joga a favor

Três factos, verificados, tornam esta fatia mais barata do que a ADR-042 §5 antecipava:

1. **O servidor não aplica migrações.** `AplicarMigracoes` é invocado exclusivamente por
   `ExecutarMigracoes` (subcomando `api migrate`) e pelos testes de integração. O
   arranque do servidor nunca faz DDL.
2. **Zero DDL em runtime.** O único `CREATE TABLE` fora de `migrations/` é
   `public.schema_migrations`, dentro do próprio runner (`internal/platform/db/migrate.go:68`).
   Nenhum adaptador emite DDL.
3. **Um só ponto de leitura do DSN nos testes.** `ligar(t)` em
   `tests/integration/migracoes_test.go:21` é a única função de toda a suite de
   integração que lê `os.Getenv("DATABASE_URL")`. Os 29 ficheiros de teste dependem dela.

Não existe hoje um único `GRANT`, `REVOKE` ou `OWNER TO` em `migrations/` ou `docker/`.

---

## 2. Decisão

Nasce um papel PostgreSQL de runtime, `sgc_app`, sem privilégios de DDL e sem posse de
objectos. O servidor passa a ligar-se com ele. `sgc` mantém-se dono e migrador.

### 2.1 Forma da separação — porquê manter `sgc` como migrador

Alternativas ponderadas:

- **Três papéis** (`sgc_dono` NOLOGIN + `sgc_migrador` + `sgc_app`) — padrão canónico e o
  mais defensável numa auditoria AGT, mas obriga a mexer em CI, testes e `make migrate`,
  e a `REASSIGN OWNED` nas bases existentes.
- **Inverter** (`sgc` despromovido a runtime, novo `sgc_migrador` dono) — o nome já
  espalhado por `.env`/compose/CI ficaria com o papel menos privilegiado, o que é seguro
  por omissão; mas `sgc` é superuser por construção da imagem e rebaixá-lo é operação
  delicada em bases já existentes.
- **Escolhida: manter `sgc`, nascer `sgc_app`.** Raio mínimo — CI, testes de integração e
  `make migrate` ficam inalterados; só o DSN do servidor muda. E o essencial fica feito:
  o papel que está atrás da superfície HTTP deixa de poder mexer em triggers.

Custo aceite: a credencial de migração continua superuser em desenvolvimento. Vive no
deploy, nunca no processo servidor. Registado em §6 como risco aberto, com prescrição
para produção.

### 2.2 O papel

```
sgc_app: NOSUPERUSER, NOCREATEDB, NOCREATEROLE, NOLOGIN (sem password)
```

Nasce **`NOLOGIN` e sem password**. A credencial é acto de provisionamento, nunca de
migração — ver §2.5.

### 2.3 Privilégios

Concedidos numa migração forward-only nova, `migrations/shared/0003_papel_runtime.sql`:

- `USAGE` — **e não `CREATE`** — nos 8 schemas: `auditoria`, `clinico`, `farmacia`,
  `financeiro`, `identidade`, `laboratorio`, `recepcao`, `shared`.
- `SELECT, INSERT, UPDATE, DELETE` em todas as tabelas desses schemas.
- `USAGE, SELECT` em `farmacia.seq_codigo_medicamento` — única sequência explícita do
  repositório. As colunas `GENERATED ALWAYS AS IDENTITY` (ex.: `auditoria_eventos.id`)
  não requerem grant separado: a sequência é interna à tabela e `INSERT` basta.
- `ALTER DEFAULT PRIVILEGES FOR ROLE sgc IN SCHEMA <cada um dos 8>` — tudo o que `sgc`
  criar daqui em diante nasce já concedido a `sgc_app`.
- **Nada em `public`.** `sgc_app` não vê `public.schema_migrations`.

**Ordem — porque `shared/0003` e não outro sítio.** `AplicarMigracoes` ordena por bounded
context alfabeticamente e depois por número de ficheiro, pelo que `shared/` corre em
último lugar. Numa base criada do zero, `shared/0003` é a última migração de todas: os 8
schemas e todas as tabelas já existem quando ela corre. Isto importa porque o schema
`recepcao` **não** é criado pelo `init.sql` (que só cria 7) — nasce na sua própria
migração, `migrations/recepcao/0001_agenda_marcacoes.sql:9`.

São necessárias as duas metades, e por razões diferentes:

- `GRANT ... ON ALL TABLES IN SCHEMA` cobre o que já existe no momento em que a migração
  corre;
- `ALTER DEFAULT PRIVILEGES` cobre o que vier depois, em migrações futuras.

Nenhuma das duas substitui a outra.

Duas restrições explícitas, que são o coração da fatia:

- **`auditoria.auditoria_eventos` recebe apenas `SELECT, INSERT`.** Sem `UPDATE`/`DELETE`
  concedidos, a imutabilidade do audit log deixa de depender exclusivamente do trigger.
  E como `TRUNCATE` nunca é concedido a não-donos, o buraco do `TRUNCATE` fecha-se na
  mesma passagem.
- `financeiro.facturas` e `financeiro.itens_factura` **continuam a precisar de `UPDATE`**
  — ver §2.4.

### 2.4 Porque as facturas continuam a precisar de `UPDATE`

O agregado `Factura` em `RASCUNHO` é persistido por upsert transaccional (ADR-039) e a
coluna `versao` é o bloqueio optimista introduzido pela ADR-040. O runtime tem
legitimamente de poder actualizar estas linhas. Aí a defesa **é** o trigger
`trg_facturas_imutaveis`, que continua a ser a peça que distingue rascunho de emitido.

O que o R7 muda não é o trigger: é que `sgc_app` deixa de o poder desligar. É exactamente
esta a diferença entre a nota de âmbito da migração `financeiro/0004` — «defesa contra
erro, não contra a aplicação comprometida» — e o que fica verdadeiro depois desta fatia.

### 2.5 Onde vive a credencial

A migração dá **privilégios**; o provisionamento dá **credencial**. A separação é
deliberada:

- A migração é embebida no binário e versionada em git. Uma password de produção não
  pode estar em nenhum dos dois.
- Os privilégios, esses, têm de convergir por dev, CI e produção pelo mesmo caminho — e
  o único caminho que corre nos três é a migração. O `docker/postgres/init.sql` só corre
  na primeira criação do volume e o Postgres de CI (`services:` do GitHub Actions) nem
  sequer o monta.

Logo:

| Ambiente | Quem faz `ALTER ROLE sgc_app LOGIN PASSWORD ...` |
|---|---|
| dev | `docker/postgres/init.sql` |
| CI | passo `psql` no job `integracao`, antes dos testes |
| produção | runbook de provisionamento em `docs/` — password gerada pelo operador |

A migração cria o papel num bloco `DO` idempotente se ele ainda não existir, pelo que a
ordem entre provisionamento e migração é indiferente em qualquer ambiente.

### 2.6 Fail-fast no arranque

`db.VerificarPapelRuntime(ctx, pool)`, num ficheiro novo
`internal/platform/db/privilegios.go`, chamada em `ExecutarServidor` imediatamente a
seguir a `LigarPool` (`app.go:45`), antes de qualquer outra dependência. Erro devolvido
é fatal — o servidor não arranca.

Quatro interrogações sobre o próprio papel; qualquer resposta afirmativa é falha:

1. `rolsuper OR rolcreaterole OR rolcreatedb` em `pg_roles` para `current_user`.
2. `pg_has_role(current_user, relowner, 'USAGE')` sobre `financeiro.facturas`,
   `financeiro.itens_factura` e `auditoria.auditoria_eventos`. Usar `pg_has_role` em vez
   de comparar nomes cobre, na mesma pergunta, a posse directa **e** o caso em que
   alguém torne `sgc_app` membro de `sgc` — que reintroduziria o problema inteiro por uma
   porta lateral.
3. `has_schema_privilege(current_user, <schema>, 'CREATE')` nos 8 schemas.
4. `has_table_privilege(current_user, 'auditoria.auditoria_eventos', 'UPDATE')` e o mesmo
   para `'DELETE'`.

**Sem isenção por `APP_ENV`.** O desenvolvimento passa a ligar-se como `sgc_app`, como a
produção — que é também o que torna as provas reais em vez de cerimoniais. Falha fechado:
uma clínica mal provisionada fica com a API em baixo em vez de a correr insegura.

`ExecutarMigracoes` não recebe verificação simétrica: correr migrações com a credencial
de runtime falha naturalmente, com erro de permissão, na primeira instrução DDL.

---

## 3. Configuração

Duas variáveis, com significado fixo em todos os ambientes:

| Variável | Papel | Quem a lê |
|---|---|---|
| `DATABASE_URL` | runtime (`sgc_app`) | `ExecutarServidor`; serviço `api` do compose |
| `DATABASE_MIGRATION_URL` | migrador (`sgc`) | `ExecutarMigracoes`; testes de integração |

`DATABASE_MIGRATION_URL` é **opcional** em `config.Carregar()` e **obrigatória** dentro de
`ExecutarMigracoes`, que falha com mensagem explícita se estiver vazia.

Não é um detalhe de arrumação: é o que permite ao processo servidor correr sem sequer ter
a credencial de migração no ambiente. Um servidor comprometido não pode usar o que não
tem. Torná-la obrigatória em `Carregar()` obrigaria o servidor a transportá-la, e
anularia metade da fatia.

---

## 4. Provas

### 4.1 Integração — `tests/integration/privilegios_test.go` (novo), ligado como `sgc_app`

Negativas; cada uma tem de falhar com erro de permissão do PostgreSQL:

- `ALTER TABLE financeiro.facturas DISABLE TRIGGER ALL`
- `DROP TRIGGER trg_facturas_nascem_rascunho ON financeiro.facturas`
- `SET session_replication_role = 'replica'`
- `TRUNCATE auditoria.auditoria_eventos`
- `UPDATE auditoria.auditoria_eventos SET accao = ...`
- `DELETE FROM auditoria.auditoria_eventos`
- `CREATE TABLE financeiro.intruso (...)`

Positivas — sem elas, a fatia partiria a aplicação sem ninguém dar por isso:

- `INSERT` em `auditoria.auditoria_eventos` continua a passar.
- O ciclo completo de factura (criar rascunho → adicionar item → emitir, com o
  `SELECT ... FOR UPDATE` sobre `financeiro.series`) continua a passar como `sgc_app`.

### 4.2 Deriva — teste de inventário

Percorre `pg_class` nos 8 schemas e falha se existir tabela ou sequência sem grant a
`sgc_app`. Apanha o caso que o `ALTER DEFAULT PRIVILEGES` não cobre: um bounded context
novo traz um schema novo, e os defaults são por schema.

Verifica também que `auditoria.auditoria_eventos` tem **exactamente** `SELECT, INSERT` —
nem menos, nem mais.

### 4.3 Guarda contra falso-verde

`ligar(t)` mantém o `t.Skip` quando nada está configurado, mas passa a **falhar** se
`DATABASE_URL` estiver definida e `DATABASE_MIGRATION_URL` não. Uma suite meio-configurada
não se pode calar: seria a repetição exacta do modo de falha que a ADR-042 apanhou —
provas que passam a verde pela razão errada.

### 4.4 Mutação — obrigatória antes do merge

1. Retirar os `GRANT`/`REVOKE` da migração → as provas de §4.1 e §4.2 têm de ficar
   vermelhas.
2. Retirar a chamada a `VerificarPapelRuntime` de `ExecutarServidor` → a prova de arranque
   tem de ficar vermelha.

Correr também contra base de dados criada do zero, não só contra a base de dados de
desenvolvimento já migrada.

---

## 5. Ficheiros

| Ficheiro | Acção |
|---|---|
| `migrations/shared/0003_papel_runtime.sql` | novo — papel, grants, revokes, default privileges |
| `internal/platform/db/privilegios.go` | novo — `VerificarPapelRuntime` |
| `internal/platform/app.go` | `ExecutarServidor` chama a verificação; `ExecutarMigracoes` usa a nova variável |
| `internal/platform/config/config.go` | campo `URLMigracaoBaseDados`, opcional |
| `docker/postgres/init.sql` | credencial de dev para `sgc_app` |
| `docker-compose.yml` | serviço `api` passa a usar `sgc_app` |
| `.env.example` | as duas variáveis, documentadas |
| `.github/workflows/ci.yml` | passo `psql` de credencial; as duas variáveis no job `integracao` |
| `tests/integration/migracoes_test.go` | `ligar()` passa a migrador; novo `ligarApp()`; guarda de meia-configuração |
| `tests/integration/privilegios_test.go` | novo |
| `Makefile` | alvo `migrate` documenta a variável que usa |
| `docs/RUNBOOK-provisionamento-bd.md` | novo — provisionamento de produção |
| `adrs/ADR-043-separacao-credenciais.md` | novo |
| `CLAUDE.md` | §6 e índice de ADRs |
| `SPRINT.md` | critérios de saída da fatia |

`internal/platform/db/migrate.go` fica **inalterado**.

---

## 6. Riscos e dívida a registar na ADR-043

### R1 — O migrador continua `SUPERUSER` em desenvolvimento

Por construção da imagem `postgres:16`, `POSTGRES_USER` é superuser. O runbook de
produção prescreve um dono `NOSUPERUSER`; em desenvolvimento a cirurgia não compensa.
Consequência honesta: em dev, quem tiver a credencial de migração continua a poder tudo.
Não é o vector que esta fatia fecha — o vector é a aplicação comprometida.

### R2 — Um DBA malicioso continua a poder tudo

O R7 defende contra **aplicação comprometida**, não contra acesso directo ao cluster.
Num modelo on-premise por clínica, onde o administrador de sistemas é do cliente, este é
um limite real e não um detalhe teórico. Fechá-lo exigiria notarização externa ou
armazenamento WORM. Não é esta fatia, e a ADR-043 não vai fingir que é.

### R3 — `pg_dump` / `pg_restore` contornam triggers

Um restore repõe dados sem passar por trigger nenhum. Fora de âmbito; registado para não
se perder.

### Herdados, inalterados por esta fatia

- **Anulação** de factura — `ANULADA` existe no enum e na CHECK desde a ADR-039, nenhuma
  transição a alcança. Vinculada pela ADR-040 §R5: não pode apagar nem renumerar.
- **Pagamentos** (parcial, múltiplos métodos) e integração EMIS Multicaixa.
- **SAF-T-AO** — geração XML, validação XSD, submissão — e **certificação AGT**.

---

## 7. Critérios de saída

- [ ] `sgc_app` existe, é `NOSUPERUSER`/`NOCREATEDB`/`NOCREATEROLE` e não é dono de
      nenhum objecto nem membro de `sgc`.
- [ ] O servidor liga-se como `sgc_app` em dev, CI e no compose; a credencial de migração
      não está no ambiente do processo servidor.
- [ ] `ExecutarServidor` recusa arrancar com papel privilegiado, sem isenção por ambiente.
- [ ] As sete provas negativas de §4.1 falham com erro de permissão como `sgc_app`.
- [ ] As provas positivas de §4.1 passam como `sgc_app` — a aplicação não regride.
- [ ] O teste de inventário cobre os 8 schemas e fixa `SELECT, INSERT` em
      `auditoria.auditoria_eventos`.
- [ ] `ligar()` falha, em vez de saltar, com configuração pela metade.
- [ ] Mutação feita e registada: sem grants → vermelho; sem verificação de arranque →
      vermelho. Também contra base de dados criada do zero.
- [ ] Migração `shared/0003` aplicada e embebida; forward-only, sem editar migrações já
      aplicadas.
- [ ] Gates de cobertura verdes (domínio ≥85%, aplicação ≥75%, adaptadores ≥60%);
      `go-arch-lint` sem violações.
- [ ] ADR-043 registada; `CLAUDE.md` §6 e índice de ADRs actualizados; `SPRINT.md`
      actualizado.
