# ADR-043 — Separação da credencial de migração da de runtime

- **Estado:** Aceite
- **Data:** 2026-07-22
- **Marco/Sprint:** M4 — Financeiro (Sprint 18, fatia de segurança transversal)
- **Fontes:** desenho em
  `docs/superpowers/specs/2026-07-21-adr-043-separacao-credenciais-design.md`;
  ADR-040 (emissão da Factura — R7); ADR-041 (selagem canónica — §R4, que
  transportou o R7); ADR-042 (§2.6 e §5, que o deixaram explicitamente para
  fatia própria); CLAUDE.md §5 (princípios 3 e 4); REG-001 §3.4.
- **Runbook operacional:** `docs/RUNBOOK-provisionamento-bd.md`.

## Contexto

A ADR-040 registou no seu §R7 que o papel da aplicação era dono das tabelas
fiscais e podia correr `ALTER TABLE … DISABLE TRIGGER`. A ADR-041 transportou-o
intacto; a ADR-042 fechou o R6 com um trigger `BEFORE INSERT` e escreveu, na
própria migração, que a garantia era condicional — «defesa contra erro e contra
SQL directo de terceiros, **não** contra a aplicação comprometida».

Esta fatia fecha o R7. A medição feita para a executar mostrou que ele era
**mais largo do que qualquer das três ADR descrevia**.

### 1.1 Uma credencial só, e superuser

`docker-compose.yml:11` define `POSTGRES_USER: sgc`, e a imagem oficial
`postgres:16` cria o `POSTGRES_USER` como **`SUPERUSER`** — não apenas como dono
das tabelas. Essa mesma credencial estava em `DATABASE_URL` em três sítios
(`.env.example`, `docker-compose.yml`, `.github/workflows/ci.yml`) e era
consumida indistintamente pelo servidor (`ExecutarServidor`) e pelas migrações
(`ExecutarMigracoes`). Três consequências, todas alcançáveis a partir de
qualquer via de execução de SQL que uma aplicação comprometida obtenha:

| Ataque | Porque funcionava |
|---|---|
| `ALTER TABLE financeiro.facturas DISABLE TRIGGER ALL` | era dono da tabela |
| `SET session_replication_role = 'replica'` | era superuser — desliga *todos* os triggers da sessão, em toda a base de dados |
| `TRUNCATE auditoria.auditoria_eventos` | era dono; e o trigger é `BEFORE UPDATE OR DELETE`, e `TRUNCATE` não é `DELETE` |

**A terceira não estava registada em risco nenhum, em ADR nenhuma.** O audit log
— Princípio Não-Negociável #3 do CLAUDE.md, retenção mínima de 10 anos por LPDP
/ Lei 22/11 — era apagável com uma instrução, **sem sequer tocar em triggers**.
Foi executada contra a base viva antes de se escrever uma linha de código.

### 1.2 O desvio por pertença é real, não teórico

Com `GRANT sgc TO sgc_app`, um `SET ROLE sgc; ALTER TABLE financeiro.facturas
DISABLE TRIGGER ALL` corrido **como `sgc_app`** teve sucesso: `pg_trigger.tgenabled`
passou de `'O'` para `'D'`. Uma verificação por comparação de nomes
(`pg_get_userbyid(relowner) = current_user`) devolve `f` e deixa passar. É por
isso que todas as interrogações desta fatia usam `pg_has_role`.

### 1.3 A medição que mudou o desenho: `USAGE` não, `MEMBER`

É o facto mais reutilizável da fatia, e vale para qualquer código que pergunte
"este papel é privilegiado?".

`pg_has_role(papel, alvo, 'USAGE')` responde *"os privilégios do alvo
**herdam-se** automaticamente para este papel?"*. `pg_has_role(…, 'MEMBER')`
responde *"este papel pode fazer `SET ROLE` para o alvo?"*. **O vector de ataque
é o `SET ROLE`, não a herança.** Medido, com um papel de teste dado a `sgc_app`
com `GRANT … WITH INHERIT FALSE`:

| Interrogação | Resultado |
|---|---|
| `pg_has_role(current_user, alvo, 'USAGE')` | **`false`** |
| `pg_has_role(current_user, alvo, 'MEMBER')` | **`true`** |
| `SET ROLE alvo; ALTER TABLE … DISABLE TRIGGER` | **funcionou** (`tgenabled` `'O'` → `'D'`) |

Uma pertença `NOINHERIT` passava incólume por uma verificação escrita com
`'USAGE'`. A mesma assimetria aparece uma segunda vez, noutra forma: **atributos
de papel não se herdam por pertença, mas poderes assumem-se por `SET ROLE`** —
`SELECT rolsuper OR rolcreaterole OR rolcreatedb … WHERE rolname = current_user`
devolve `f` para um `sgc_app` que seja membro de um papel superuser, e esse
membro desliga triggers à vontade.

E há um terceiro corolário, medido: **os 14 papéis predefinidos do PostgreSQL 16
têm `rolsuper`, `rolcreaterole` e `rolcreatedb` todos `f`**. `pg_write_all_data`,
`pg_write_server_files` e `pg_execute_server_program` são invisíveis a qualquer
verificação por atributos.

## 2. Decisão

### 2.1 Duas credenciais, com significado fixo em todos os ambientes

| Variável | Papel PostgreSQL | Quem a lê |
|---|---|---|
| `DATABASE_URL` | `sgc_app` — runtime, DML apenas | `ExecutarServidor`; serviço `api` do compose |
| `DATABASE_MIGRATION_URL` | `sgc` — dono e migrador | `ExecutarMigracoes` (`api migrate`); testes de integração |

`DATABASE_MIGRATION_URL` é **opcional** em `config.Carregar()` e **obrigatória**
dentro de `ExecutarMigracoes`. Não é arrumação: é o que permite ao processo
servidor correr sem sequer ter a credencial de migração no ambiente. **Um
servidor comprometido não pode usar o que não tem.** Torná-la obrigatória em
`Carregar()` obrigaria o servidor a transportá-la e anularia metade da fatia.

Manteve-se `sgc` como migrador, em vez do padrão canónico de três papéis
(`sgc_dono` NOLOGIN + `sgc_migrador` + `sgc_app`): o raio fica mínimo — CI,
testes de integração e `make migrate` inalterados, só o DSN do servidor muda — e
o essencial fica feito, que é o papel atrás da superfície HTTP deixar de poder
mexer em triggers.

### 2.2 O papel e os privilégios (`migrations/shared/0003`, `0004`)

`sgc_app` nasce `NOSUPERUSER NOCREATEDB NOCREATEROLE NOLOGIN` e **sem password**:
a migração dá **privilégios**, o provisionamento dá **credencial**. Uma password
de produção não pode estar embebida no binário nem versionada em git.

- `USAGE` — e **não** `CREATE` — nos oito schemas de bounded context.
- `SELECT, INSERT, UPDATE, DELETE` nas tabelas dos sete schemas de negócio;
  `USAGE, SELECT` nas sequências.
- `auditoria`: apenas `SELECT, INSERT`. Sem `UPDATE`/`DELETE` concedidos, a
  imutabilidade do audit log deixa de depender exclusivamente de um trigger; e
  como `TRUNCATE` nunca é concedido a quem não é dono, o buraco do `TRUNCATE`
  fecha-se na mesma passagem.
- `ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER` em cada schema, para as
  tabelas que vierem em migrações futuras. As duas metades são precisas por
  razões diferentes: o `GRANT … ON ALL TABLES` cobre o que existe agora, os
  defaults cobrem o que vier. Nenhuma substitui a outra.
- **Nada em `public`** — `sgc_app` não vê `public.schema_migrations`.
- `financeiro.series` perde o `DELETE` (`shared/0004`): é a cabeça da cadeia
  hash e da numeração sem buracos da ADR-040 (`ultimo_sequencial`,
  `ultimo_hash`) e, ao contrário das facturas, **não tem trigger nenhum**.
  Apagar a linha perde o `ultimo_hash` e o elo seguinte nasce partido — dano não
  reparável. Foi encontrado por medição: `sgc_app` conseguia `DELETE FROM
  financeiro.series` (devolveu `DELETE 3`).

`financeiro.facturas` e `financeiro.itens_factura` **continuam a precisar de
`UPDATE` e `DELETE`**: o rascunho é mutável por desenho (ADR-039) e o upsert
transaccional reescreve as linhas. Aí a defesa **é** o trigger — o que o R7 muda
não é o trigger, é que `sgc_app` deixa de o poder desligar.

### 2.3 Fail-fast no arranque (`db.VerificarPapelRuntime`)

Chamada em `ExecutarServidor` imediatamente a seguir a `LigarPool`, antes de
qualquer outra dependência. Erro devolvido é fatal. **Sem isenção por ambiente**
— o desenvolvimento liga-se como a produção, que é também o que torna as provas
reais em vez de cerimoniais. Falha fechado: uma clínica mal provisionada fica
com a API em baixo em vez de a correr insegura.

Quatro famílias de interrogação, todas avaliadas sobre a **união dos papéis que
`current_user` pode assumir por `SET ROLE`** — nunca sobre os atributos do
próprio papel nem sobre o privilégio herdado:

1. **Administrador** — pertença a qualquer papel com `rolsuper`/`rolcreaterole`/
   `rolcreatedb`, mais os três papéis predefinidos de escrita e execução no
   servidor (`pg_write_server_files`, `pg_execute_server_program`,
   `pg_read_server_files`), que a via genérica por atributos nunca apanharia.
2. **Posse** das **quatro** tabelas de valor legal (`pg_has_role(…, relowner,
   'MEMBER')`), precedida de uma verificação de existência por `to_regclass` que
   distingue "base por migrar" de "papel privilegiado".
3. **Criação de objectos** — `CREATE` nos oito schemas **e** `CREATE` na base de
   dados, que é uma via distinta e igualmente fatal para a restrição
   forward-only: não cria objectos nos schemas conhecidos, cria schemas novos.
4. **Mutação do valor legal** — `TRUNCATE` nas **quatro** tabelas, mais
   `UPDATE`/`DELETE` no audit log e `DELETE` em `financeiro.series`. O conjunto
   proibido **não é o mesmo nas quatro**, e colapsá-lo partiria a aplicação (ver
   §2.2): a factura em RASCUNHO é mutável, e `SELECT`/`INSERT`/`UPDATE` na série
   são a própria emissão da ADR-040. Esta via genérica apanha também
   `pg_write_all_data` sem que ele esteja em lista nenhuma — porque tem `UPDATE`
   e `DELETE` em `auditoria_eventos`.

   As **quatro** são `financeiro.facturas`, `financeiro.itens_factura`,
   `financeiro.series` e `auditoria.auditoria_eventos`. A série entrou na
   re-revisão (ver R9) e é a única sem trigger: é protegida por `SELECT … FOR
   UPDATE`, e por isso a guarda derivada de `pg_trigger` nunca a veria.

`ExecutarMigracoes` não recebe verificação simétrica: correr migrações com a
credencial de runtime falha naturalmente, na primeira instrução DDL.

### 2.4 A guarda AST sobre o `app.go`

No molde da ADR-042 §2.2: sem ela, apagar ou neutralizar a chamada em `app.go`
deixaria as provas de integração verdes, porque essas chamam a função
directamente. A guarda exige uma **forma canónica** — um `if` que seja elemento
**directo** do corpo de `ExecutarServidor`, com a chamada resolvida pelo
**caminho de import** (não pelo identificador `db`), o segundo argumento igual ao
identificador que recebeu o pool de `LigarPool`, `Cond` a comparar essa mesma
variável com `nil`, um `return` **directo** no corpo, e o índice desse `if`
**estritamente menor** que o do statement que chama `.Iniciar`.

Foram medidas 17 mutações reais de `app.go` (gofmt + build + guarda, com
`git checkout --` entre cada). As variantes fechadas incluem o erro descartado
(`_ =`), o erro só registado em `logger.Warn`, `if false`, `if 1 == 2`, closure
nunca invocada, alias `db` apontado a um pacote-chamariz, pool trocado, chamada
depois do arranque, e — a mais provável na vida real — a **isenção por ambiente**
(`if err != nil { if cfg.EmProducao() { return … }; logger.Warn(…) }`), que
compila, passa `vet` e `gofmt`, e se lê como engenharia razoável num code review.
Código legítimo idiomático (incluindo um alias legítimo do pacote certo)
continua verde.

### 2.5 A guarda de deriva — inventário exacto de privilégios

`TestPrivilegios_InventarioExactoDeTabelasESequencias` assere o conjunto
**exacto** de privilégios de `sgc_app`, relação a relação, sobre as 31 tabelas e
3 sequências dos oito schemas. Verificar só a **presença** de `SELECT` apanharia
a deriva num sentido e seria cega ao outro, que é o perigoso: um `GRANT
TRUNCATE` colado por engano.

Três decisões de desenho merecem registo:

- **Ler a ACL (`aclexplode`) e não `has_table_privilege` por privilégio
  nomeado.** A lista dos privilégios que existem é um facto da **versão** do
  PostgreSQL, não deste projecto: o PG17 acrescenta `MAINTAIN`, que o PG16 nem
  reconhece (medido: `unrecognized privilege type`). Uma lista fixa ficaria
  atrás da próxima versão em silêncio.
- **O `grantee` inclui o pseudo-papel `PUBLIC` (oid 0)** e os papéis assumíveis
  por `SET ROLE`: um `GRANT … TO PUBLIC` chega a `sgc_app` na mesma.
- **Segunda passagem sobre `pg_attribute.attacl`**, porque os grants de coluna
  vivem noutro sítio do catálogo. Medido: `GRANT UPDATE (id) ON
  auditoria.auditoria_eventos` deixa `has_table_privilege(…,'UPDATE')` a `f` e
  `has_any_column_privilege` a `t` — escapa ao inventário da `relacl` **e** ao
  `VerificarPapelRuntime`, ficando só o trigger a segurar o audit log.

### 2.6 O que esta fatia NÃO garante

O R7 defende contra **aplicação comprometida**, não contra acesso directo ao
cluster. Um administrador de base de dados malicioso continua a poder tudo, e
`pg_dump`/`pg_restore` repõem dados sem passar por trigger nenhum. Num modelo
on-premise por clínica, em que o administrador de sistemas é do cliente, este é
um limite real e não um detalhe teórico; fechá-lo exigiria armazenamento WORM ou
notarização externa. **Esta ADR não afirma que o fecha.**

## 3. Âmbito da alteração

| Ficheiro | Alteração |
|---|---|
| `internal/platform/config/config.go` | campo `URLMigracaoBaseDados`, **opcional** |
| `internal/platform/app.go` | `ExecutarServidor` verifica o papel; `ExecutarMigracoes` usa a nova variável e recusa correr sem ela |
| `internal/platform/db/privilegios.go` | novo — `VerificarPapelRuntime` e as quatro famílias de interrogação |
| `internal/platform/db/privilegios_test.go` | novo — provas de caixa branca (tag `integration`) |
| `internal/platform/arranque_guarda_test.go` | novo — guarda AST sobre o `app.go` |
| `migrations/shared/0003_papel_runtime.sql` | novo — papel, grants, revokes, default privileges |
| `migrations/shared/0004_series_sem_delete.sql` | novo — revoga `DELETE` em `financeiro.series` |
| `docker/postgres/init.sql` | credencial de **desenvolvimento** para `sgc_app` |
| `docker-compose.yml`, `.env.example`, `Makefile` | as duas variáveis, com o significado documentado |
| `.github/workflows/ci.yml` | passo `psql` de credencial; as duas variáveis; `internal/platform/db` no passo de integração; `-count=1` |
| `tests/integration/migracoes_test.go` | `ligar()` passa a migrador; novo `ligarApp()`; guarda de meia-configuração |
| `tests/integration/privilegios_test.go` | novo — provas de comportamento e inventário de deriva |
| `tests/integration/privilegios_arranque_test.go` | novo — provas de arranque, incluindo as quatro tabelas de valor legal |
| `internal/platform/db/migrate.go` | advisory lock a serializar `AplicarMigracoes` |
| `docs/RUNBOOK-provisionamento-bd.md` | novo — provisionamento de produção; §5.3 cobre as **quatro** tabelas |
| `README.md` | comando de integração corrigido para as duas credenciais e os dois pacotes |
| `adrs/ADR-043-separacao-credenciais.md` | esta ADR — novo |
| `adrs/ADR-040-emissao-factura.md` | R7 marcado resolvido (aditivo) |
| `CLAUDE.md`, `SPRINT.md` | marco e sprint |

**Não muda:** o domínio, a canonicalização do hash, o vector dourado, as rotas
nem o RBAC. O `internal/platform/db/migrate.go` **passou a mudar** na revisão
final (advisory lock, ver R7): a ordenação e a idempotência das migrações ficam
como estavam, só deixa de haver corrida entre migradores concorrentes.

## 4. Provas

- **Nove provas negativas como `sgc_app`**, cada uma com o **SQLSTATE
  verificado** (`errors.As` sobre `*pgconn.PgError`), e não apenas `err != nil`
  — um nome de tabela mal escrito satisfaria a versão fraca. `42501` medido nas
  que dependem de privilégio; `23001` no trigger de rascunho.
- **Cobertura positiva real:** varrimento das 31 tabelas dos oito schemas como
  `sgc_app`, mais o bloqueio optimista (`versao`), a reescrita de linhas de
  rascunho, o `nextval` e o ciclo de emissão com `SELECT … FOR UPDATE`. Sem
  isto, a fatia partiria a aplicação sem ninguém dar por isso.
- **Os três inventários à mão amarrados à base**, cada um pela derivação que
  corresponde ao que ele decide (ver §6, nota N5).
- **Mutação, sempre:** as guardas foram vistas a **morder** — 21 mutações
  independentes contra a guarda de deriva (incluindo `GRANT` via `PUBLIC` e via
  papel assumível `WITH INHERIT FALSE`), 17 contra a guarda AST, e a fase
  vermelha provada por `git stash` em cada correcção.
- **Duas convergências:** suite de integração verde contra a base de
  desenvolvimento já migrada **e** contra uma base criada do zero.

## 5. Fora do âmbito

- **Anulação** de factura — `ANULADA` figura no enum e na CHECK desde a ADR-039,
  **nenhuma transição o alcança**. Vinculada pela ADR-040 §R5: não pode apagar
  nem renumerar.
- **Pagamentos** (parcial, múltiplos métodos) e integração EMIS Multicaixa.
- **SAF-T-AO** — geração XML, validação XSD, submissão — e **certificação de
  software junto da AGT**: **não estão feitas**. Esta ADR não as substitui nem
  as antecipa.
- **Agendamento do cron diário de verificação da cadeia** (REG-001 §3.4).

## 6. Riscos e dívida registada

As notas **N1–N5** não são riscos abertos: são factos medidos que a próxima
pessoa a mexer neste código precisa de ter à mão, e que de outro modo ficariam
em relatórios não versionados.

### R1 — O migrador é `NOSUPERUSER` em produção, e é-o em desenvolvimento que não

Em produção o migrador é criado `NOSUPERUSER` e **assim fica, sem excepção de
provisionamento**. Em desenvolvimento e em CI continua superuser por construção
da imagem `postgres:16`, que cria o `POSTGRES_USER` assim. Consequência honesta:
em dev, quem tiver a credencial de migração pode tudo. Não é o vector que esta
fatia fecha — o vector é a aplicação comprometida.

**Mas o facto a registar é outro, e é o mais instrutivo desta fatia:** foi
precisamente essa diferença entre dev e produção que escondeu um defeito real
até ao ensaio literal do runbook contra um cluster limpo. A `shared/0003`
reafirmava os atributos de `sgc_app` com um `ALTER ROLE sgc_app NOSUPERUSER
NOCREATEDB NOCREATEROLE` **incondicional**, e o PostgreSQL só deixa alterar o
atributo `SUPERUSER` a quem é superuser — mesmo para o repor no valor que já
tem, e mesmo com `CREATEROLE` mais `ADMIN OPTION` sobre o papel (medido nas três
configurações). Resultado medido contra um cluster limpo: `api migrate` com
migrador `NOSUPERUSER` aplicava as 30 primeiras migrações e parava em
`shared/0003 … permission denied to alter role (SQLSTATE 42501)`. Em dev e em CI
nunca falhou uma vez, porque lá o migrador é superuser — **o ambiente que a
fatia existe para endurecer era o único onde o defeito aparecia.**

A `shared/0003` tem uma **segunda** dependência do mesmo tipo, que o caminho do
runbook evita mas que fica registada: ela **cria** `sgc_app` se ele faltar, e
criar um papel exige `CREATEROLE`. Medido em clusters limpos com `sgc_app`
inexistente: migrador `NOSUPERUSER NOCREATEROLE` pára com `permission denied to
create role (SQLSTATE 42501)`; migrador `NOSUPERUSER CREATEROLE` aplica as 32 e
`sgc_app` nasce correcto (`f`/`f`/`f`, `NOLOGIN`). O runbook §2 cria os dois
papéis antes de migrar, precisamente para o migrador poder ficar **sem**
`CREATEROLE` — dar-lho seria dar-lhe poder sobre papéis para o resto da vida da
instalação.

**Correcção:** o `ALTER ROLE` passa a correr dentro de um `DO` condicionado a
`sgc_app` ter de facto um dos três atributos. No caso normal não há nada para
corrigir e a instrução não é emitida; quando há, é emitida — e nesse caso o
migrador tem mesmo de ser superuser, porque corrigir um papel promovido exige
esse poder. A guarda não fica vazia, e isso foi medido por mutação: `sgc_app`
criado com `CREATEDB` antes das migrações (`rolcreatedb = t`) ficou, depois do
`api migrate`, com `rolcreatedb = f`.

#### Porque foi legítimo editar uma migração já aplicada

A regra do projecto é forward-only, e a razão da regra é concreta: o executor
salta as versões registadas em `public.schema_migrations`, pelo que uma edição
**não propaga** e o ficheiro passa a divergir do estado real da base — foi o
incidente registado na ADR-040 §6.

Aqui a divergência é **semanticamente vazia**, e é isso — e só isso — que
justifica a excepção. A forma nova difere da antiga apenas na **ausência de uma
instrução que, no estado normal, é um no-op**: quem já aplicou a `0003` tem
`sgc_app` exactamente no estado que a versão nova produziria, porque a versão
antiga executou um `ALTER ROLE` cujo efeito era nenhum. Não há estado alcançável
pela versão antiga que a nova não produza. Provado nos três cenários que
importam: cluster novo com migrador `NOSUPERUSER` (32 migrações, `f`/`f`/`f`),
cluster novo com migrador superuser (idem, suite verde) e base já migrada, com a
`0003` registada, onde o executor salta a versão (`aplicadas_agora: 0`), nada
muda e a suite continua verde.

**Não é licença geral.** A excepção vale por igualdade de resultado demonstrada,
não por a alteração "parecer inofensiva" — que é exactamente o raciocínio que a
regra existe para bloquear. Uma edição que mude o estado produzido continua a
exigir migração nova, como a `shared/0004` fez.

### R2 — Um administrador do cluster continua a poder tudo

Ver §2.6. Inclui `pg_dump`/`pg_restore`, que contornam triggers.

### R3 — O papel de migração é imutável, e trocá-lo falha em silêncio

`ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER` amarra os defaults ao **nome**
do papel que correu a migração. Se um dia as migrações forem corridas por outro
papel, as tabelas novas nascem sem privilégios para `sgc_app`, a migração
termina com sucesso e o servidor arranca — só o primeiro pedido que toque na
tabela nova é que falha. **Nenhum teste do projecto o pode apanhar**, porque a
suite corre sempre com migrador `sgc`. Medido em transacção revertida: tabela
criada por `sgc` nasce `sgc_app=arwd/sgc` (`has_table_privilege` = `t`); criada
por outro papel nasce com `relacl` vazia (`f`). O remédio é operacional e vive
no runbook (§1.1 e §5.4): o papel não muda; se mudar, re-declara os defaults; e
há um passo de verificação sobre `pg_default_acl`.

### R4 — `pg_read_all_data` está fora de âmbito, por decisão medida

Nenhuma das quatro famílias de interrogação recusa `pg_read_all_data`, e é
deliberado: medido, ele dá `SELECT` e nada mais — não há daí caminho para
desligar um trigger, apagar o audit log ou destruir a cadeia de hash. O R7 é
sobre **integridade**, não sobre confidencialidade; a minimização de leitura é
território da LPDP e da ADR-037. Fica escrito porque quem lê o nome
`VerificarPapelRuntime` presume o contrário.

### R5 — As limitações reais da guarda AST

Duas, ambas medidas, e nenhuma delas é a que o desenho previa (a posição da
chamada **é** verificada desde a correcção 6 — o índice do `if` tem de ser menor
que o do `srv.Iniciar`, e a guarda falha fechada se não encontrar o arranque):

1. **Quatro acoplamentos sintácticos** que um refactor legítimo obriga a rever:
   os nomes `ExecutarServidor`, `LigarPool` e `.Iniciar`, e a **aridade** de dois
   argumentos de `VerificarPapelRuntime`. Todos falham **fechados**, com
   mensagem accionável — e essa é a escolha certa: uma guarda que se declara
   satisfeita por não ter encontrado a referência é pior do que uma guarda que
   chumba.
2. **A âncora do pool é o *primeiro* `LigarPool` do corpo, não *o* pool que a
   aplicação usa.** Duas construções escapam, ambas medidas verdes: um
   pool-chamariz criado e verificado primeiro, com o pool real criado a seguir; e
   `pool = poolPriv` reatribuído depois do `if`. Fechá-las obrigaria a seguir o
   fluxo de dados — interpretar o programa, não inspeccionar a sua forma. Ambas
   exigem reestruturação visível em code review. **É defesa em profundidade, não
   garantia.**

Também por desenho, a guarda não verifica que a função faça o que promete (isso
é a suite de integração) nem que não exista uma segunda chamada mais abaixo a
desfazer o efeito da primeira.

### R6 — A guarda de deriva é de INTEGRAÇÃO, não de arranque

O inventário exacto protege CI e desenvolvimento; **não impede deriva numa base
de produção entre deploys**. Promovê-lo ao arranque mudaria o contrato — o
servidor recusaria arrancar por um `GRANT` a mais numa tabela sem valor legal —
e é decisão arquitectural para fatia própria. O **subconjunto perigoso** já é
imposto ao arranque (`TRUNCATE` nas **quatro** tabelas de valor legal, `UPDATE`/
`DELETE` no audit log, `DELETE` na série, `CREATE` nos schemas e na base). Para produção, o runbook
§5.4 transcreve as mesmas consultas para o operador correr à mão.

### R7 — Dois migradores concorrentes corriam à mesma migração (fechado)

Fechado nesta fatia, mas fica registado porque a causa é estrutural e a
descoberta foi tardia. `AplicarMigracoes` lê `jaAplicada` e só depois aplica:
entre as duas há uma janela em que dois migradores concorrentes vêem ambos a
migração por aplicar. Ficou visível quando o passo de integração da CI passou a
correr, no mesmo `go test`, **dois pacotes que migram** — e `go test` corre
pacotes em paralelo. Medido contra base virgem, deterministicamente nas duas
tentativas:

```
aplicar migration auditoria/0001_auditoria_eventos:
  duplicate key value violates unique constraint "pg_namespace_nspname_index" (SQLSTATE 23505)
aplicar migration farmacia/0002_stock:
  duplicate key value violates unique constraint "pg_type_typname_nsp_index" (SQLSTATE 23505)
```

A corrida **já existia antes desta fatia**, dentro do próprio
`tests/integration`, e nunca fora observada porque a base de desenvolvimento já
estava migrada. É a mesma lição do C1: o ambiente de medição escondia o defeito.
Fechado com um advisory lock de sessão, tomado e largado na mesma ligação
adquirida do pool.

Os cenários de concorrência **reais** são dois: os pacotes de teste, e um
pipeline que invoque `api migrate` mais do que uma vez em simultâneo. **Não**
inclui "duas réplicas da API a arrancar ao mesmo tempo" — verificado:
`AplicarMigracoes` é chamada apenas de `ExecutarMigracoes` (`app.go`) e dos
testes; `ExecutarServidor` **não migra**, e o runbook §4 faz da migração um passo
separado. A primeira redacção desta nota reivindicava um risco que não existe, o
que é a mesma classe de defeito desta fatia no sentido contrário (N2 da
re-revisão).

Dois cuidados que o lock trouxe e que estão fechados: `AplicarMigracoes` **exige
`MaxConns ≥ 2`** (segura uma ligação e o corpo continua a usar o pool; com uma
só, bloqueava para sempre — medido com `pool_max_conns=1`, e agora recusado à
cabeça com mensagem própria); e tenta primeiro `pg_try_advisory_lock`, para poder
**registar em log** que vai esperar em vez de deixar o `api migrate` parado e
mudo (N3 e N4 da re-revisão).

### R8 — Nada impede a próxima edição de uma migração já aplicada

`public.schema_migrations` regista `bounded_context`, `versao` e `aplicada_em`
— **não** um checksum. O executor salta versões já registadas, pelo que editar
uma migração aplicada é silencioso: as bases existentes divergem do ficheiro e
nada o detecta. Esta fatia abriu o precedente (R1, o `ALTER ROLE` da
`shared/0003`, com edição justificada por igualdade de resultado) e **não** o
fechou — é a única invariante de prosa desta fatia que não foi convertida em
guarda, num projecto que converteu todas as outras (AST sobre o `app.go`,
derivação dos três inventários).

Fica para **fatia própria**. Candidata: coluna `sha256` em
`public.schema_migrations`, preenchida na aplicação e verificada no arranque,
com **excepção declarada** para a `shared/0003` (que já divergiu por R1) — a
excepção tem de ser nomeada e justificada em código, não um caso especial mudo.
Não implementar de passagem: mexe no controlo de migrações de todas as bases
existentes.

**Cuidado que não pode ser descoberto no próprio dia:** o `sha256` terá de ser
**semeado** a partir do conteúdo actual dos ficheiros para as migrações já
registadas. Sem esse passo de sementeira, todas as migrações aplicadas até hoje
nascem com checksum vazio ou divergente e a guarda acusa deriva em toda a linha,
logo no primeiro arranque de todas as instalações.

### R9 — Uma guarda derivada é cega ao que a sua fonte não vê

`TestTabelasDeValorLegal_CobreAsTabelasProtegidasPorTrigger` amarra
`tabelasDeValorLegal` derivando de `pg_trigger`. Garante que uma tabela **com**
trigger não fica de fora; não pode garantir nada sobre uma tabela de valor legal
**sem** trigger — e era exactamente essa, `financeiro.series`, que faltava na
lista (achado I1 da revisão final: com `GRANT TRUNCATE, DELETE` o servidor
arrancava e o `TRUNCATE` levou a tabela de 32 linhas a 0). Corrigido:
`financeiro.series` é a quarta entrada, com `DELETE` e `TRUNCATE` proibidos e
`SELECT`/`INSERT`/`UPDATE` preservados, porque são a própria emissão da ADR-040.

O risco que fica é o geral: **uma guarda derivada não substitui a decisão sobre
o que pertence ao conjunto**, só impede que o conjunto encolha em silêncio
naquilo que a fonte vê. Ao acrescentar uma tabela de valor legal futura,
verificar sempre pelas duas vias — a derivação e a leitura da ADR — porque a
derivação, sozinha, ficará calada se a protecção não for por trigger.

### N1 — A assimetria dos default privileges de sequências em `auditoria`

O schema `auditoria` recebe defaults para tabelas e **não** para sequências; os
outros sete recebem ambos. É deliberado: `auditoria_eventos.id` é `GENERATED
ALWAYS AS IDENTITY`, que não cria sequência com nome próprio e não consome
`USAGE`/`SELECT` — conceder ali privilégio seria dar algo que nada consome.
Medido: `auditoria.auditoria_eventos_id_seq` tem ACL vazia para `sgc_app`,
enquanto `farmacia.seq_codigo_medicamento` e `shared.outbox_id_seq` têm
`SELECT`+`USAGE`.

**Isto tem de ser revisto no dia em que uma tabela do schema `auditoria` for
declarada com `bigserial`** — que, ao contrário de `IDENTITY`, cria uma
sequência nomeada e depende de privilégio nela. Nesse dia, uma migração nova do
BC `auditoria` acrescenta `ALTER DEFAULT PRIVILEGES … GRANT USAGE, SELECT ON
SEQUENCES TO sgc_app`; não se altera a `0003` nem a `0004` para o fazer
preventivamente. A nota está no cabeçalho da `shared/0004`, cujo assunto é
`financeiro.series` — quem criar a tal tabela nunca a lê. **Por isso está
também aqui.**

### N2 — A expectativa é permissiva por herança para espécies que o projecto não usa

A regra do inventário foi escrita para armazenamento real (`relkind` `'r'`,
`'p'`): tudo o que não é sequência num schema de negócio herda a expectativa
`DELETE,INSERT,SELECT,UPDATE`. Uma **vista** ou **vista materializada** nascida
num schema de negócio herdaria essa expectativa sem que ninguém tivesse decidido
que o runtime precisa de escrever nela. Hoje não há nenhuma relação dessas
espécies (medido: `r = 31`, `S = 3`, e nada em `'p'`/`'v'`/`'m'`/`'f'`), pelo que
o problema é futuro e não presente.

Remédio proporcionado, **para o dia em que a primeira aparecer**: manter a
herança para `'r'`/`'p'` e exigir declaração explícita para `'v'`/`'m'`/`'f'`.
Fica registado como item, deliberadamente não implementado — implementá-lo hoje
seria escrever um ramo que nenhuma relação exercita.

### N3 — Duas assimetrias residuais da guarda de deriva

1. A exclusão **automática** de schemas atribuídos a uma extensão (`pg_depend`,
   `deptype = 'e'`) ainda **salta** em vez de asserir zero. Mitigado porque a
   guarda irmã de caixa branca morde no `USAGE`, e sem `USAGE` os grants de
   tabela são inertes (medido) — mas é o mesmo padrão que a válvula
   `schemasDeExtensao` foi corrigida para não ter.
2. A expectativa **zero** dos schemas de extensão não tem escape declarado: um
   `GRANT SELECT … TO PUBLIC`, que extensões reais fazem nos seus próprios
   catálogos, fá-la nascer vermelha sem deriva nenhuma. Quando alguém declarar
   um schema de extensão a sério, o remédio é **excepção declarada**, não
   relaxar a asserção.

### N4 — `acldefault` e `relkind` colidem em duas letras

Armadilha que qualquer pessoa a mexer neste código volta a encontrar. O
inventário usa `acldefault((CASE relkind WHEN 'S' THEN 's' ELSE 'r' END), …)` e
está certo **precisamente por não passar o `relkind` adiante**: `acldefault('f')`
é **FUNÇÃO** (concede `EXECUTE` a `PUBLIC`) e `acldefault('p')` é **PARÂMETRO**,
enquanto em `relkind` são tabela externa e tabela particionada. Um mapeamento
"natural" `relkind → objtype` compararia uma tabela externa órfã contra o default
de uma função que concede `EXECUTE` a toda a gente — **e o inventário ficaria
verde sobre lixo**.

### N5 — O que as duas listas de schemas garantem (e o que não garantem)

`db.schemasBC` (arranque) e `osOitoSchemas` (integração) coincidem hoje, e é
tentador escrever que "não podem divergir uma da outra". **Não é verdade**, e a
formulação importa: um bounded context cujo schema o runtime não alcance teria
de estar em `osOitoSchemas` — porque existe na base — e **não** em `schemasBC` —
porque não é território do runtime. A garantia real é outra, e é mais forte:
**cada lista está amarrada à base pela derivação que corresponde ao que ela
decide** — `osOitoSchemas` contra `pg_namespace` (o que existe), `schemasBC` por
**alcançabilidade** (`USAGE` pela união dos papéis assumíveis), e
`tabelasDeValorLegal` contra `pg_trigger`. Nenhuma pode crescer ou encolher em
silêncio.

### N6 — Risco declarado e não fechado: o `init.sql` cria `sgc_app` com password conhecida

`docker/postgres/init.sql:25` dá a `sgc_app` a password de desenvolvimento
`sgc_app`, **versionada em git**. É deliberado — é o que torna dev e CI
utilizáveis sem passo manual — e o runbook proíbe montar o ficheiro em produção
(§6.3). Mas a proibição é **operacional**: nada no código a impõe, e um
`docker-compose` de produção copiado a partir do de desenvolvimento traria a
montagem atrás. `VerificarPapelRuntime` não sabe distinguir uma password fraca
de uma forte — verifica poder, não credencial. Fica registado como risco
declarado, não fechado.

## 7. O que esta fatia custou, e a lição que fica

Vale mais para o projecto do que qualquer decisão técnica acima.

**Dois Critical, ambos com exploração reproduzida contra a base, ambos da mesma
classe: o âmbito real da verificação era mais estreito do que o nome dela
prometia.**

1. `recusarAdministrador` lia os **atributos do próprio papel** em vez do poder
   **assumível por `SET ROLE`**. Reproduzido: `CREATE ROLE zz SUPERUSER; GRANT zz
   TO sgc_app` → as quatro interrogações limpas, `VerificarPapelRuntime` devolve
   `nil`, o servidor arranca a achar-se seguro, e `SET ROLE zz; ALTER TABLE
   auditoria.auditoria_eventos DISABLE TRIGGER …` desligou o trigger.
2. `recusarMutacaoDaAuditoria` verificava **uma** das três tabelas de valor
   legal, com as três declaradas na variável ao lado. Reproduzido com `GRANT`
   directo, sem sequer precisar de `SET ROLE`: `GRANT TRUNCATE ON
   financeiro.facturas, financeiro.itens_factura TO sgc_app` → tudo limpo, e
   `TRUNCATE financeiro.itens_factura, financeiro.facturas` executou. Os três
   triggers de imutabilidade são `FOR EACH ROW` e `TRUNCATE` não dispara nenhum.

Um terceiro, da mesma classe, apareceu na guarda de deriva: o filtro
`relkind IN ('r','S')` via **metade** das espécies de relação — escapavam
particionadas, vistas, materializadas e externas. Particionar
`auditoria.auditoria_eventos`, que é a candidata natural com retenção de 10
anos, fá-la-ia sair do inventário em silêncio; e medido, o `TRUNCATE` no pai
esvazia as partições **mesmo com `TRUNCATE` revogado nelas**.

A lição operacional é directa e transferível: **verificar se o que a função
percorre é o que o seu nome promete.** As três falhas passaram por revisão de
diff sem serem vistas; foram apanhadas por quem foi medir contra a base.

Corolário, já registado na ADR-042 §1.2 e reconfirmado aqui: uma medição
apresentada com aparência de rigor merece a mesma desconfiança que uma afirmação
sem medição nenhuma. É por isso que este documento diz "medido" onde mediu, e
diz o que **não** garante onde não garante.

## 8. Estado dos riscos da ADR-040 após esta fatia

| Risco (ADR-040) | Estado |
|---|---|
| R3 — MFA por impor em 11 dos 14 grupos | **Resolvido** pela ADR-042 |
| R4 — `ListarSnapshotsPorSerie` N+1 e sem paginação | Aberto |
| R5 — verificação inclui `ANULADA` por desenho | Aberto (restrição, não defeito) |
| R6 — factura pode nascer `EMITIDA` | **Resolvido** pela ADR-042 — e a condição que a ADR-042 lhe pôs (§2.6) cai com esta ADR |
| R7 — papel da aplicação é dono das tabelas fiscais | **Resolvido** por esta ADR (§2.1–§2.3), com os limites da §2.6 e os riscos R1–R3 da §6 |
