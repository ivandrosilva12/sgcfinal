# ADR-043 — Separação da credencial de migração da de runtime (R7) — Plano de Implementação

> **Para executores agênticos:** SUB-SKILL OBRIGATÓRIA: usar `superpowers:subagent-driven-development`
> (recomendado) ou `superpowers:executing-plans` para implementar tarefa a tarefa.
> Os passos usam sintaxe de checkbox (`- [ ]`) para acompanhamento.

**Objectivo:** Retirar ao papel PostgreSQL da aplicação a posse das tabelas de valor legal
e os privilégios de administração, de modo que os triggers que protegem as facturas e o
audit log deixem de ser desligáveis pela própria aplicação comprometida — fechando o R7 da
ADR-040.

**Arquitectura:** Nasce um papel `sgc_app` (`NOSUPERUSER`, não-dono) com que o servidor
passa a ligar-se; `sgc` mantém-se dono e migrador. Os privilégios são fixados por uma
migração forward-only (`shared/0003`), que corre em último lugar porque `AplicarMigracoes`
ordena os bounded contexts alfabeticamente. A credencial (LOGIN + password) é acto de
provisionamento, nunca de migração. O arranque do servidor interroga a base de dados sobre
o seu próprio papel e recusa arrancar se estiver privilegiado.

**Stack:** Go 1.22+, pgx v5, PostgreSQL 16, Docker Compose, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-07-21-adr-043-separacao-credenciais-design.md`

## Restrições Globais

- **Idioma:** PT-PT angolano em tudo — código, comentários, commits, mensagens de erro.
  Nunca PT-BR, nunca EN em texto visível.
- **Migrations forward-only.** Nunca editar uma migração já aplicada: o executor salta
  versões registadas em `public.schema_migrations`. Corrigir sempre por migração nova.
- **Nada de `panic()`** fora de inicialização — sempre `error`.
- **Sem infra em `internal/domain/`.** Todo o código desta fatia vive na Camada 4
  (`internal/platform/`), em `migrations/`, em `docker/`, em `.github/` e em `tests/`.
- **Gates de cobertura:** domínio ≥85%, aplicação ≥75%, adaptadores ≥60%
  (`bash scripts/cobertura.sh`). `go-arch-lint check` sem violações.
- **Nenhuma password de produção em git nem embebida no binário.**
- **Nome do papel de runtime:** `sgc_app`. **Password de dev e de CI:** `sgc_app`.
- **Variáveis de ambiente:** `DATABASE_URL` = runtime (`sgc_app`);
  `DATABASE_MIGRATION_URL` = migrador (`sgc`). Este significado é o mesmo em dev, CI e
  produção, sem excepções.
- **Os 8 schemas por bounded context:** `auditoria`, `clinico`, `farmacia`, `financeiro`,
  `identidade`, `laboratorio`, `recepcao`, `shared`. Atenção: `docker/postgres/init.sql`
  só cria 7 — `recepcao` nasce em `migrations/recepcao/0001_agenda_marcacoes.sql:9`.
- **As 3 tabelas de valor legal:** `financeiro.facturas`, `financeiro.itens_factura`,
  `auditoria.auditoria_eventos`.

## Nota de verificação prévia

Todo o SQL deste plano foi executado contra um `postgres:16` descartável antes de ser
escrito. Factos confirmados empiricamente, não deduzidos:

1. `sgc` (o `POSTGRES_USER` da imagem oficial) tem `rolsuper = t`. É superuser, não apenas
   dono.
2. `ALTER DEFAULT PRIVILEGES FOR ROLE CURRENT_USER` é sintaxe válida e a migração inteira
   corre dentro de uma transacção.
3. Como `sgc_app`, as sete operações negativas da Tarefa 2 falham com as mensagens exactas
   registadas nessa tarefa.
4. Como `sgc_app`, o ciclo de factura e o `INSERT` em auditoria continuam a funcionar, e o
   trigger `trg_facturas_nascem_rascunho` continua a morder.
5. **O desvio por pertença é real.** Com `GRANT sgc TO sgc_app`, o comando
   `SET ROLE sgc; ALTER TABLE financeiro.facturas DISABLE TRIGGER ALL` **teve sucesso** —
   `pg_trigger.tgenabled` passou de `O` para `D`. A verificação com `pg_has_role` apanha-o
   (`t`); a variante ingénua que compara `pg_get_userbyid(relowner) = current_user` **não**
   apanha (`f`). É por isso que a Tarefa 3 usa `pg_has_role` e não comparação de nomes.

---

### Tarefa 1: Separar as duas variáveis de configuração

Nesta tarefa **ainda não existe** `sgc_app`. `DATABASE_URL` e `DATABASE_MIGRATION_URL`
apontam ambas para `sgc`. O objectivo é isolar a mudança de configuração da mudança de
privilégios, para que um revisor possa aceitar uma e rejeitar a outra.

**Ficheiros:**
- Modificar: `internal/platform/config/config.go:19` (campo) e `:56` (leitura)
- Modificar: `internal/platform/app.go:318-336` (`ExecutarMigracoes`)
- Modificar: `tests/integration/migracoes_test.go:21-34` (`ligar`)
- Modificar: `.env.example:12-13`
- Modificar: `.github/workflows/ci.yml:92-95`
- Modificar: `Makefile` (alvo `migrate`)
- Teste: `internal/platform/config/config_test.go`

**Interfaces:**
- Produz: `config.Config.URLMigracaoBaseDados string` — DSN do migrador, **opcional**.
- Produz: `ligar(t *testing.T) (*pgxpool.Pool, context.Context)` — passa a ligar com a
  credencial de **migração**. Assinatura inalterada; todos os 29 ficheiros de integração
  continuam a chamá-la sem alteração.
- Produz: `ligarApp(t *testing.T) (*pgxpool.Pool, context.Context)` — liga com a
  credencial de **runtime**. Consumida pelas Tarefas 2, 3 e 4.

- [ ] **Passo 1: Escrever o teste que falha (configuração)**

Acrescentar ao fim de `internal/platform/config/config_test.go`:

```go
func TestCarregar_MigracaoOpcionalEAusenteNaoImpedeArranque(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/sgc")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("KEYCLOAK_ISSUER", "http://localhost:8081/realms/sgc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "sgc-admin")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "segredo")
	t.Setenv("DATABASE_MIGRATION_URL", "")

	cfg, err := Carregar()
	if err != nil {
		t.Fatalf("DATABASE_MIGRATION_URL é opcional: o servidor tem de arrancar sem ela; obtive %v", err)
	}
	if cfg.URLMigracaoBaseDados != "" {
		t.Fatalf("esperava DSN de migração vazio, obtive %q", cfg.URLMigracaoBaseDados)
	}
}

func TestCarregar_MigracaoQuandoDefinidaEhLida(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://sgc_app:p@localhost:5432/sgc")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("KEYCLOAK_ISSUER", "http://localhost:8081/realms/sgc")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "sgc-admin")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "segredo")
	t.Setenv("DATABASE_MIGRATION_URL", "postgres://sgc:p@localhost:5432/sgc")

	cfg, err := Carregar()
	if err != nil {
		t.Fatalf("Carregar: %v", err)
	}
	if cfg.URLMigracaoBaseDados != "postgres://sgc:p@localhost:5432/sgc" {
		t.Fatalf("DSN de migração não foi lido: %q", cfg.URLMigracaoBaseDados)
	}
	if cfg.URLBaseDados == cfg.URLMigracaoBaseDados {
		t.Fatal("os dois DSN têm de ser independentes")
	}
}
```

- [ ] **Passo 2: Correr o teste e confirmar que falha**

```
go test ./internal/platform/config/ -run TestCarregar_Migracao -v
```

Esperado: FAIL na compilação — `cfg.URLMigracaoBaseDados undefined (type Config has no field or method URLMigracaoBaseDados)`.

- [ ] **Passo 3: Acrescentar o campo à configuração**

Em `internal/platform/config/config.go`, imediatamente a seguir à linha 19
(`URLBaseDados string // DSN PostgreSQL (pgx)`):

```go
	URLMigracaoBaseDados      string        // DSN do migrador (ADR-043); opcional — o servidor corre sem ela
```

E em `Carregar()`, imediatamente a seguir à linha 56 (`URLBaseDados: os.Getenv("DATABASE_URL"),`):

```go
		URLMigracaoBaseDados:      os.Getenv("DATABASE_MIGRATION_URL"),
```

**Não acrescentar validação em `Carregar()`.** A variável é deliberadamente opcional: é
isso que permite ao processo servidor correr sem sequer ter a credencial de migração no
ambiente. Um servidor comprometido não pode usar o que não tem.

- [ ] **Passo 4: Correr o teste e confirmar que passa**

```
go test ./internal/platform/config/ -run TestCarregar_Migracao -v
```

Esperado: PASS nos dois testes.

- [ ] **Passo 5: `ExecutarMigracoes` passa a exigir a credencial de migração**

Em `internal/platform/app.go`, substituir o corpo de `ExecutarMigracoes` (linhas 318-336)
por:

```go
// ExecutarMigracoes carrega a configuração e aplica as migrations forward-only
// embebidas, saindo no fim. Usado por `make migrate` (subcomando "migrate").
//
// Usa DATABASE_MIGRATION_URL e nunca DATABASE_URL: desde a ADR-043 a credencial
// de runtime não tem privilégios de DDL, e correr migrations com ela falharia na
// primeira instrução com um erro de permissão obscuro. Falhar aqui, com mensagem
// explícita, é mais honesto.
func ExecutarMigracoes(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}

	if cfg.URLMigracaoBaseDados == "" {
		return fmt.Errorf("DATABASE_MIGRATION_URL não definida: as migrations exigem a " +
			"credencial de migração, distinta da de runtime (ADR-043)")
	}

	pool, err := db.LigarPool(ctx, cfg.URLMigracaoBaseDados)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		return fmt.Errorf("aplicar migrations: %w", err)
	}
	return nil
}
```

- [ ] **Passo 6: `ligar()` passa a migrador e nasce `ligarApp()`**

Em `tests/integration/migracoes_test.go`, substituir a função `ligar` (linhas 21-34) por:

```go
// ligar liga com a credencial de MIGRAÇÃO (sgc): tem DDL, é o que os testes de
// integração precisam para aplicar migrations e montar cenários.
func ligar(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	return ligarCom(t, "DATABASE_MIGRATION_URL")
}

// ligarApp liga com a credencial de RUNTIME (sgc_app): sem DDL, sem posse das
// tabelas. É com esta que se provam as garantias da ADR-043.
func ligarApp(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	return ligarCom(t, "DATABASE_URL")
}

func ligarCom(t *testing.T, chave string) (*pgxpool.Pool, context.Context) {
	t.Helper()
	runtime := os.Getenv("DATABASE_URL")
	migracao := os.Getenv("DATABASE_MIGRATION_URL")

	// Nada configurado: a suite salta, como sempre saltou.
	if runtime == "" && migracao == "" {
		t.Skip("DATABASE_URL/DATABASE_MIGRATION_URL não definidos; a saltar testes de integração")
	}
	// Configuração pela metade é erro, não motivo para saltar. Uma suite que se
	// cala quando devia correr é exactamente o modo de falha que a ADR-042
	// apanhou: provas a passar a verde pela razão errada.
	if runtime == "" || migracao == "" {
		t.Fatal("configuração pela metade: DATABASE_URL e DATABASE_MIGRATION_URL têm de estar " +
			"ambas definidas (ADR-043)")
	}

	ctx := context.Background()
	pool, err := db.LigarPool(ctx, os.Getenv(chave))
	if err != nil {
		t.Fatalf("ligar ao PostgreSQL com %s: %v", chave, err)
	}
	t.Cleanup(pool.Close)
	return pool, ctx
}
```

Actualizar também o comentário de cabeçalho do ficheiro (linhas 3-5):

```go
// Testes de integração da fundação: runner de migrations forward-only,
// imutabilidade do audit log e seed dos papéis. Exigem um PostgreSQL acessível
// via DATABASE_MIGRATION_URL (migrador) e DATABASE_URL (runtime) — ver ADR-043.
// Corridos com: go test -tags=integration ./tests/integration/...
```

**Nota:** `ligarApp` fica sem chamadores até à Tarefa 2. Em Go isso não é erro de
compilação para funções de pacote de teste, mas o `golangci-lint` pode assinalar `unused`.
Se assinalar, avançar para a Tarefa 2 sem commit intermédio de lint — a Tarefa 2 dá-lhe
chamadores. Em alternativa, mover a criação de `ligarApp` para a Tarefa 2.

- [ ] **Passo 7: Actualizar `.env.example`**

Substituir as linhas 12-13 por:

```
# PostgreSQL (pgx). No compose, host = "postgres".
#
# DUAS credenciais, desde a ADR-043 (R7):
#   DATABASE_URL           — runtime (sgc_app). Sem DDL, não é dono de nada. É a
#                            única que o processo servidor precisa de ter.
#   DATABASE_MIGRATION_URL — migrador (sgc). Só para `make migrate` e testes de
#                            integração. NÃO a coloque no ambiente do servidor.
DATABASE_URL=postgres://sgc_app:sgc_app@localhost:5432/sgc?sslmode=disable
DATABASE_MIGRATION_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable
```

**Atenção:** o `.env.example` já aponta `DATABASE_URL` para `sgc_app`, que só passa a
existir na Tarefa 2. É documentação do estado final e não é lida por nenhum teste; deixar
assim evita um segundo toque no ficheiro.

- [ ] **Passo 8: Actualizar o job de integração do CI**

Em `.github/workflows/ci.yml`, substituir o passo das linhas 92-95 por:

```yaml
      - name: Testes de integração
        env:
          DATABASE_URL: postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable
          DATABASE_MIGRATION_URL: postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable
        run: go test -tags=integration ./tests/integration/...
```

Ambas apontam para `sgc` nesta tarefa: `sgc_app` ainda não existe. A Tarefa 2 muda a
primeira.

- [ ] **Passo 9: Documentar a variável no `Makefile`**

Substituir o alvo `migrate`:

```makefile
.PHONY: migrate
migrate: ## Aplica as migrations forward-only (exige DATABASE_MIGRATION_URL — ADR-043)
	go run ./cmd/api migrate
```

- [ ] **Passo 10: Correr a suite completa**

```
go build ./... && go test ./internal/platform/... && go vet ./...
```

Esperado: tudo PASS.

E com a base de dados de desenvolvimento a correr:

```
DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
DATABASE_MIGRATION_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
go test -tags=integration ./tests/integration/...
```

Esperado: PASS (a suite de integração inteira, com `ligar()` já a usar a nova variável).

- [ ] **Passo 11: Provar a guarda de meia-configuração**

```
DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
go test -tags=integration ./tests/integration/... -run TestMigracoes 2>&1 | head -5
```

Esperado: FAIL com `configuração pela metade: DATABASE_URL e DATABASE_MIGRATION_URL têm de
estar ambas definidas (ADR-043)`. **Não** SKIP, **não** PASS.

- [ ] **Passo 12: Commit**

```bash
git add internal/platform/config/config.go internal/platform/config/config_test.go \
        internal/platform/app.go tests/integration/migracoes_test.go \
        .env.example .github/workflows/ci.yml Makefile
git commit -m "feat(plataforma): DATABASE_URL e DATABASE_MIGRATION_URL passam a credenciais distintas (ADR-043)

DATABASE_MIGRATION_URL e opcional em config.Carregar e obrigatoria em
ExecutarMigracoes: e isso que permite ao processo servidor correr sem ter a
credencial de migracao no ambiente. Nos testes de integracao, ligar() passa a
usar a credencial de migracao e nasce ligarApp() para a de runtime; configuracao
pela metade FALHA em vez de saltar."
```

---

### Tarefa 2: Papel `sgc_app`, privilégios e provas de comportamento

**Ficheiros:**
- Criar: `migrations/shared/0003_papel_runtime.sql`
- Criar: `tests/integration/privilegios_test.go`
- Modificar: `docker/postgres/init.sql`
- Modificar: `docker-compose.yml:115`
- Modificar: `.github/workflows/ci.yml` (novo passo + `DATABASE_URL`)

**Interfaces:**
- Consome: `ligarApp(t)` e `ligar(t)` da Tarefa 1.
- Produz: papel `sgc_app` com os privilégios descritos; nenhuma interface Go.

**`migrations/embed.go` não precisa de ser tocado** — verificado. A directiva é
`//go:embed auditoria clinico farmacia financeiro identidade laboratorio recepcao shared`,
que embebe os directórios inteiros: o ficheiro novo em `shared/` é apanhado
automaticamente.

- [ ] **Passo 1: Escrever as provas que falham**

Criar `tests/integration/privilegios_test.go`:

```go
//go:build integration

// Provas da separação de credenciais (ADR-043 / R7 da ADR-040). Correm ligadas
// como sgc_app — o papel de runtime — e verificam que ele NÃO consegue subverter
// as garantias que a base de dados impõe por trigger, e que CONTINUA a conseguir
// fazer o trabalho legítimo da aplicação.
package integration_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// migrarTudo aplica as migrations com a credencial de MIGRAÇÃO. As provas de
// privilégio precisam do esquema montado antes de se ligarem como sgc_app.
func migrarTudo(t *testing.T) {
	t.Helper()
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("aplicar migrations: %v", err)
	}
}

func TestRuntime_NaoConsegueSubverterAsGarantias(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligarApp(t)

	casos := []struct {
		nome string
		sql  string
	}{
		{"desligar triggers das facturas", `ALTER TABLE financeiro.facturas DISABLE TRIGGER ALL`},
		{"apagar o trigger de nascer rascunho", `DROP TRIGGER trg_facturas_nascem_rascunho ON financeiro.facturas`},
		{"desligar triggers na sessão", `SET session_replication_role = 'replica'`},
		{"truncar o audit log", `TRUNCATE auditoria.auditoria_eventos`},
		{"actualizar o audit log", `UPDATE auditoria.auditoria_eventos SET accao = 'adulterado'`},
		{"apagar do audit log", `DELETE FROM auditoria.auditoria_eventos`},
		{"criar objectos no financeiro", `CREATE TABLE financeiro.intruso (id int)`},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := pool.Exec(ctx, c.sql); err == nil {
				t.Fatalf("o papel de runtime conseguiu %q — o R7 continua aberto", c.nome)
			}
		})
	}
}

func TestRuntime_ContinuaAFazerOTrabalhoLegitimo(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligarApp(t)

	if _, err := pool.Exec(ctx,
		`INSERT INTO auditoria.auditoria_eventos (actor, accao) VALUES ($1, $2)`,
		"tester", "adr043.prova"); err != nil {
		t.Fatalf("INSERT no audit log tem de continuar a funcionar: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM auditoria.auditoria_eventos`).Scan(&n); err != nil {
		t.Fatalf("SELECT no audit log tem de continuar a funcionar: %v", err)
	}
	if n == 0 {
		t.Fatal("esperava ler o evento que acabei de inserir")
	}

	// A série é o ponto de serialização da numeração (ADR-040): o runtime tem de
	// poder bloqueá-la com FOR UPDATE.
	if _, err := pool.Exec(ctx,
		`SELECT 1 FROM financeiro.series WHERE false FOR UPDATE`); err != nil {
		t.Fatalf("SELECT ... FOR UPDATE em financeiro.series tem de funcionar: %v", err)
	}
}

func TestRuntime_TriggerDeRascunhoContinuaAMorder(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligarApp(t)

	// Não basta que sgc_app não possa desligar o trigger: o trigger tem de
	// continuar a disparar para ele. Sem esta prova, um GRANT errado poderia
	// deixar passar facturas fabricadas sem ninguém dar por isso.
	//
	// As colunas são as NOT NULL sem default de financeiro/0001_facturas.sql:
	// cliente_nome e episodio_id (id tem DEFAULT gen_random_uuid()).
	_, err := pool.Exec(ctx,
		`INSERT INTO financeiro.facturas (estado, cliente_nome, episodio_id)
		 VALUES ('EMITIDA', 'Prova ADR-043', gen_random_uuid())`)
	if err == nil {
		t.Fatal("uma factura EMITIDA à nascença tinha de ser rejeitada pelo trigger")
	}
	// Tem de falhar PELO TRIGGER, não por violação de NOT NULL ou de CHECK: uma
	// prova que passa a verde pela razão errada não prova nada.
	if !strings.Contains(err.Error(), "RASCUNHO") {
		t.Fatalf("esperava a rejeição do trigger de nascer rascunho, obtive: %v", err)
	}
}
```

Este teste importa `strings`; acrescentar ao bloco de imports do ficheiro.

- [ ] **Passo 2: Correr as provas e confirmar que falham**

```
DATABASE_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
DATABASE_MIGRATION_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
go test -tags=integration ./tests/integration/ -run TestRuntime -v
```

Esperado: `TestRuntime_NaoConsegueSubverterAsGarantias` FALHA nos 7 subtestes — porque
`DATABASE_URL` ainda aponta para `sgc`, que consegue fazer tudo. É exactamente esta a
demonstração de que o R7 é real. Registar a saída.

- [ ] **Passo 3: Escrever a migração**

Criar `migrations/shared/0003_papel_runtime.sql`:

```sql
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
-- não é, e esta linha desfaz isso.
ALTER ROLE sgc_app NOSUPERUSER NOCREATEDB NOCREATEROLE;

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
```

- [ ] **Passo 4: Credencial de desenvolvimento**

Acrescentar ao fim de `docker/postgres/init.sql`:

```sql

-- Papel de runtime (ADR-043 / R7). Os PRIVILÉGIOS são dados pela migração
-- shared/0003_papel_runtime.sql; aqui dá-se apenas a CREDENCIAL de
-- desenvolvimento. Em produção, ver docs/RUNBOOK-provisionamento-bd.md: a
-- password é gerada pelo operador e NUNCA vem de git.
--
-- Este ficheiro só corre na primeira criação do volume. Numa base de dados de
-- desenvolvimento já existente, correr à mão:
--   CREATE ROLE sgc_app NOSUPERUSER NOCREATEDB NOCREATEROLE LOGIN PASSWORD 'sgc_app';
CREATE ROLE sgc_app NOSUPERUSER NOCREATEDB NOCREATEROLE LOGIN PASSWORD 'sgc_app';
```

- [ ] **Passo 5: Credencial no CI**

Em `.github/workflows/ci.yml`, no job `integracao`, inserir um passo **antes** de
"Testes de integração":

```yaml
      - name: Credencial de runtime (sgc_app — ADR-043)
        env:
          PGPASSWORD: sgc
        run: |
          psql -h localhost -U sgc -d sgc -c "CREATE ROLE sgc_app NOSUPERUSER NOCREATEDB NOCREATEROLE" || true
          psql -h localhost -U sgc -d sgc -v ON_ERROR_STOP=1 -c "ALTER ROLE sgc_app LOGIN PASSWORD 'sgc_app'"
```

O `|| true` no primeiro comando torna o passo idempotente em reexecuções; numa base de CI
criada de fresco o papel nunca existe. Os privilégios não são dados aqui — vêm da migração,
que os testes aplicam.

E alterar o `DATABASE_URL` do passo "Testes de integração" para apontar ao novo papel:

```yaml
      - name: Testes de integração
        env:
          DATABASE_URL: postgres://sgc_app:sgc_app@localhost:5432/sgc?sslmode=disable
          DATABASE_MIGRATION_URL: postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable
        run: go test -tags=integration ./tests/integration/...
```

- [ ] **Passo 6: O servidor em compose passa a usar `sgc_app`**

Em `docker-compose.yml`, no serviço `api`, substituir a linha 115:

```yaml
      DATABASE_URL: postgres://sgc_app:sgc_app@postgres:5432/sgc?sslmode=disable
```

**Não** acrescentar `DATABASE_MIGRATION_URL` ao serviço `api`. É deliberado: o container do
servidor não tem a credencial de migração no ambiente.

- [ ] **Passo 7: Correr as provas e confirmar que passam**

Recriar a base de dados de desenvolvimento do zero, para que o `init.sql` corra:

```
docker compose down -v && docker compose up -d postgres
```

Depois:

```
DATABASE_URL=postgres://sgc_app:sgc_app@localhost:5432/sgc?sslmode=disable \
DATABASE_MIGRATION_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
go test -tags=integration ./tests/integration/ -run TestRuntime -v
```

Esperado: PASS nos três testes e nos 7 subtestes. As mensagens de erro do PostgreSQL, se
inspeccionadas, são: `must be owner of table facturas`, `must be owner of relation
facturas`, `permission denied to set parameter "session_replication_role"`, e `permission
denied for table auditoria_eventos` (três vezes), `permission denied for schema financeiro`.

- [ ] **Passo 8: Correr a suite de integração completa**

```
DATABASE_URL=postgres://sgc_app:sgc_app@localhost:5432/sgc?sslmode=disable \
DATABASE_MIGRATION_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
go test -tags=integration ./tests/integration/...
```

Esperado: PASS. Se algum teste pré-existente falhar por permissões, isso é um privilégio em
falta na migração — **não** relaxar a migração sem perceber qual é a operação legítima que
ficou de fora, e registá-la no commit.

- [ ] **Passo 9: Mutação — provar que as provas provam alguma coisa**

Comentar o bloco `DO $$ ... FOREACH ...` da migração, recriar a base de dados do zero
(`docker compose down -v && docker compose up -d postgres`, `make migrate`) e correr
`-run TestRuntime`.

Esperado: `TestRuntime_ContinuaAFazerOTrabalhoLegitimo` falha por falta de privilégios.
Repor o bloco.

Depois, comentar apenas as linhas do bloco `auditoria` (`REVOKE ALL` + `GRANT SELECT,
INSERT`) e repetir.

Esperado: os subtestes `truncar o audit log`, `actualizar o audit log` e `apagar do audit
log` falham — porque sem os grants restritivos o `sgc_app` herda `SELECT, INSERT, UPDATE,
DELETE`... **e o trigger continuaria a rejeitar UPDATE/DELETE**. Portanto: confirmar que
pelo menos `truncar o audit log` falha, que é o caso que só o privilégio cobre. Registar o
resultado exacto no commit — se os três continuarem verdes, a prova de TRUNCATE está a
medir a coisa errada e tem de ser corrigida antes de avançar.

Repor tudo e reconfirmar verde.

- [ ] **Passo 10: Commit**

```bash
git add migrations/shared/0003_papel_runtime.sql tests/integration/privilegios_test.go \
        docker/postgres/init.sql docker-compose.yml .github/workflows/ci.yml
git commit -m "feat(seguranca): papel de runtime sgc_app sem posse nem DDL (ADR-043)

Fecha o R7 da ADR-040. O papel sgc e SUPERUSER por construcao da imagem
postgres:16, alem de dono das tabelas: podia desligar triggers, correr
SET session_replication_role e truncar o audit log (TRUNCATE nao e DELETE, pelo
que o trigger de imutabilidade nem o via).

Nasce sgc_app: NOSUPERUSER, nao-dono, DML apenas. auditoria fica com SELECT e
INSERT so, pelo que a imutabilidade do audit log deixa de depender apenas do
trigger. Privilegios pela migracao (convergem em dev/CI/prod pelo mesmo
caminho); credencial pelo provisionamento (nunca em git).

Sete provas negativas e tres positivas, ligadas como sgc_app."
```

---

### Tarefa 3: Fail-fast no arranque

**Ficheiros:**
- Criar: `internal/platform/db/privilegios.go`
- Criar: `tests/integration/privilegios_arranque_test.go`
- Modificar: `internal/platform/app.go:45-49`

**Interfaces:**
- Consome: `ligar(t)` e `ligarApp(t)` da Tarefa 1; o papel `sgc_app` da Tarefa 2.
- Produz: `db.VerificarPapelRuntime(ctx context.Context, pool *pgxpool.Pool) error` — nil
  se o papel é adequado a runtime, erro descritivo caso contrário.

- [ ] **Passo 1: Escrever o teste que falha**

Criar `tests/integration/privilegios_arranque_test.go`:

```go
//go:build integration

// Prova da verificação de arranque da ADR-043: o servidor recusa arrancar com um
// papel privilegiado. Sem esta verificação, a separação de credenciais seria uma
// suposição sobre o deployment em vez de uma invariante do arranque — e é no
// deployment que o R7 vive.
package integration_test

import (
	"strings"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
)

func TestVerificarPapelRuntime_AceitaOPapelDeRuntime(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligarApp(t)

	if err := db.VerificarPapelRuntime(ctx, pool); err != nil {
		t.Fatalf("sgc_app tem de ser aceite como papel de runtime: %v", err)
	}
}

func TestVerificarPapelRuntime_RecusaOMigrador(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligar(t) // credencial de migração: superuser e dona das tabelas

	err := db.VerificarPapelRuntime(ctx, pool)
	if err == nil {
		t.Fatal("a credencial de migração tinha de ser recusada como papel de runtime")
	}
	if !strings.Contains(err.Error(), "ADR-043") {
		t.Fatalf("a mensagem tem de encaminhar para a ADR-043; obtive: %v", err)
	}
}

// TestVerificarPapelRuntime_ApanhaODesvioPorPertenca cobre o caso que uma
// comparação de nomes não apanharia: sgc_app não é dono das tabelas, mas se for
// membro de sgc pode assumi-lo com SET ROLE e desligar os triggers na mesma.
// Verificado contra postgres:16 — tgenabled passa de 'O' para 'D'.
func TestVerificarPapelRuntime_ApanhaODesvioPorPertenca(t *testing.T) {
	migrarTudo(t)
	admin, ctxAdmin := ligar(t)

	if _, err := admin.Exec(ctxAdmin, `GRANT sgc TO sgc_app`); err != nil {
		t.Fatalf("preparar o desvio: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctxAdmin, `REVOKE sgc FROM sgc_app`); err != nil {
			t.Fatalf("repor a pertença: %v", err)
		}
	})

	pool, ctx := ligarApp(t)
	if err := db.VerificarPapelRuntime(ctx, pool); err == nil {
		t.Fatal("um runtime que pode assumir o papel do dono tinha de ser recusado")
	}
}
```

**Nota:** o nome do papel migrador está escrito à mão como `sgc` no `GRANT`/`REVOKE`. Se o
ambiente de CI ou de desenvolvimento usar outro nome, derivá-lo com
`SELECT current_user` sobre `admin` em vez de o fixar.

- [ ] **Passo 2: Correr o teste e confirmar que falha**

```
DATABASE_URL=postgres://sgc_app:sgc_app@localhost:5432/sgc?sslmode=disable \
DATABASE_MIGRATION_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
go test -tags=integration ./tests/integration/ -run TestVerificarPapelRuntime -v
```

Esperado: FAIL na compilação — `undefined: db.VerificarPapelRuntime`.

- [ ] **Passo 3: Implementar a verificação**

Criar `internal/platform/db/privilegios.go`:

```go
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schemasBC são os oito schemas por bounded context. O papel de runtime tem
// USAGE em todos e CREATE em nenhum. Atenção: `recepcao` não é criado pelo
// init.sql — nasce na sua própria migração.
var schemasBC = []string{
	"auditoria", "clinico", "farmacia", "financeiro",
	"identidade", "laboratorio", "recepcao", "shared",
}

// tabelasDeValorLegal são as tabelas cuja posse daria ao runtime o poder de
// desligar os triggers que as protegem (ADR-040, ADR-042 §2.6).
var tabelasDeValorLegal = []string{
	"financeiro.facturas",
	"financeiro.itens_factura",
	"auditoria.auditoria_eventos",
}

// VerificarPapelRuntime confirma que o papel com que a aplicação está ligada não
// consegue subverter as garantias que a base de dados impõe por trigger: não é
// administrador, não é — nem pode assumir — o dono das tabelas de valor legal,
// não cria objectos nos schemas de negócio e não muta o audit log.
//
// Devolve erro, nunca panic; o chamador trata-o como fatal. O servidor não
// arranca com um papel privilegiado, em ambiente nenhum: falhar fechado é
// preferível a correr inseguro (ADR-043 §2.6).
func VerificarPapelRuntime(ctx context.Context, pool *pgxpool.Pool) error {
	var papel string
	if err := pool.QueryRow(ctx, `SELECT current_user`).Scan(&papel); err != nil {
		return fmt.Errorf("determinar o papel de runtime: %w", err)
	}
	if err := recusarAdministrador(ctx, pool, papel); err != nil {
		return err
	}
	if err := recusarDono(ctx, pool, papel); err != nil {
		return err
	}
	if err := recusarCriacaoDeObjectos(ctx, pool, papel); err != nil {
		return err
	}
	return recusarMutacaoDaAuditoria(ctx, pool, papel)
}

func recusarAdministrador(ctx context.Context, pool *pgxpool.Pool, papel string) error {
	const q = `SELECT rolsuper OR rolcreaterole OR rolcreatedb
	             FROM pg_roles WHERE rolname = current_user`
	var admin bool
	if err := pool.QueryRow(ctx, q).Scan(&admin); err != nil {
		return fmt.Errorf("verificar os atributos do papel %q: %w", papel, err)
	}
	if admin {
		return fmt.Errorf("o papel de runtime %q é administrador (SUPERUSER, CREATEROLE ou "+
			"CREATEDB): pode desligar triggers e apagar o audit log. Use a credencial de "+
			"runtime em DATABASE_URL, não a de migração (ADR-043)", papel)
	}
	return nil
}

func recusarDono(ctx context.Context, pool *pgxpool.Pool, papel string) error {
	// to_regclass devolve NULL em vez de erro quando a tabela não existe, o que
	// permite distinguir "base por migrar" de "papel privilegiado".
	const qFaltam = `SELECT coalesce(string_agg(t.nome, ', '), '')
	                   FROM unnest($1::text[]) AS t(nome)
	                  WHERE to_regclass(t.nome) IS NULL`
	var faltam string
	if err := pool.QueryRow(ctx, qFaltam, tabelasDeValorLegal).Scan(&faltam); err != nil {
		return fmt.Errorf("localizar as tabelas de valor legal: %w", err)
	}
	if faltam != "" {
		return fmt.Errorf("tabelas de valor legal ausentes (%s): aplique as migrations com a "+
			"credencial de migração antes de arrancar (ADR-043)", faltam)
	}

	// pg_has_role e não comparação de nomes: um papel pode não ser o dono e ainda
	// assim assumi-lo por pertença (SET ROLE), o que dá exactamente o mesmo poder.
	// Verificado: com GRANT sgc TO sgc_app, o DISABLE TRIGGER passa a funcionar.
	const q = `SELECT coalesce(bool_or(pg_has_role(current_user, c.relowner, 'USAGE')), false)
	             FROM unnest($1::text[]) AS t(nome)
	             JOIN pg_class c ON c.oid = to_regclass(t.nome)`
	var dono bool
	if err := pool.QueryRow(ctx, q, tabelasDeValorLegal).Scan(&dono); err != nil {
		return fmt.Errorf("verificar a posse das tabelas de valor legal: %w", err)
	}
	if dono {
		return fmt.Errorf("o papel de runtime %q é dono das tabelas de valor legal, ou membro "+
			"do papel que as detém: pode correr ALTER TABLE ... DISABLE TRIGGER e anular a "+
			"imutabilidade das facturas e do audit log (ADR-043)", papel)
	}
	return nil
}

func recusarCriacaoDeObjectos(ctx context.Context, pool *pgxpool.Pool, papel string) error {
	const q = `SELECT coalesce(string_agg(nome, ', '), '')
	             FROM (SELECT s AS nome, to_regnamespace(s) AS ns
	                     FROM unnest($1::text[]) AS s) x
	            WHERE ns IS NOT NULL
	              AND has_schema_privilege(current_user, ns::oid, 'CREATE')`
	var comCreate string
	if err := pool.QueryRow(ctx, q, schemasBC).Scan(&comCreate); err != nil {
		return fmt.Errorf("verificar o privilégio CREATE nos schemas: %w", err)
	}
	if comCreate != "" {
		return fmt.Errorf("o papel de runtime %q tem CREATE nos schemas %s: pode criar objectos "+
			"fora das migrations forward-only (ADR-043)", papel, comCreate)
	}
	return nil
}

func recusarMutacaoDaAuditoria(ctx context.Context, pool *pgxpool.Pool, papel string) error {
	const q = `SELECT has_table_privilege(current_user, 'auditoria.auditoria_eventos', 'UPDATE')
	              OR has_table_privilege(current_user, 'auditoria.auditoria_eventos', 'DELETE')`
	var muta bool
	if err := pool.QueryRow(ctx, q).Scan(&muta); err != nil {
		return fmt.Errorf("verificar os privilégios sobre o audit log: %w", err)
	}
	if muta {
		return fmt.Errorf("o papel de runtime %q tem UPDATE ou DELETE em "+
			"auditoria.auditoria_eventos: o audit log é append-only e a retenção de 10 anos "+
			"depende disso (LPDP / Lei 22/11, ADR-043)", papel)
	}
	return nil
}
```

- [ ] **Passo 4: Correr o teste e confirmar que passa**

```
DATABASE_URL=postgres://sgc_app:sgc_app@localhost:5432/sgc?sslmode=disable \
DATABASE_MIGRATION_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
go test -tags=integration ./tests/integration/ -run TestVerificarPapelRuntime -v
```

Esperado: PASS nos três testes.

- [ ] **Passo 5: Ligar a verificação ao arranque**

Em `internal/platform/app.go`, substituir as linhas 45-49 por:

```go
	pool, err := db.LigarPool(ctx, cfg.URLBaseDados)
	if err != nil {
		return err
	}
	defer pool.Close()

	// ADR-043 (R7): o servidor recusa arrancar com um papel que possa desligar os
	// triggers que protegem as facturas e o audit log. Sem isenção por ambiente —
	// o desenvolvimento liga-se como a produção, que é também o que torna as
	// provas de integração reais em vez de cerimoniais.
	if err := db.VerificarPapelRuntime(ctx, pool); err != nil {
		return fmt.Errorf("papel de runtime inadequado: %w", err)
	}
```

- [ ] **Passo 6: Confirmar que compila e que a suite continua verde**

```
go build ./... && go vet ./... && go test ./internal/...
```

Esperado: tudo PASS.

- [ ] **Passo 7: Mutação — provar que a verificação está mesmo ligada**

Comentar a chamada a `db.VerificarPapelRuntime` em `app.go` e correr:

```
go test -tags=integration ./tests/integration/ -run TestVerificarPapelRuntime -v
```

Esperado: os testes **continuam a passar**, porque testam a função directamente. Isso
mostra que falta uma prova de que a função está **ligada ao arranque**. Repor a chamada e
acrescentar a `internal/platform/` um teste que verifique a ligação — a forma mais barata,
seguindo o precedente da guarda AST da ADR-042, é uma guarda sintáctica:

```go
package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestArranque_VerificaOPapelDeRuntime é a guarda que a ADR-042 ensinou a
// escrever: sem ela, apagar uma linha de app.go deixaria as provas de
// tests/integration a passar na mesma, porque essas chamam a função directamente
// e não pelo arranque.
func TestArranque_VerificaOPapelDeRuntime(t *testing.T) {
	fset := token.NewFileSet()
	ficheiro, err := parser.ParseFile(fset, "app.go", nil, 0)
	if err != nil {
		t.Fatalf("analisar app.go: %v", err)
	}

	var encontrada bool
	ast.Inspect(ficheiro, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "ExecutarServidor" {
			return true
		}
		ast.Inspect(fn, func(m ast.Node) bool {
			chamada, ok := m.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := chamada.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pacote, ok := sel.X.(*ast.Ident)
			if ok && pacote.Name == "db" && sel.Sel.Name == "VerificarPapelRuntime" {
				encontrada = true
				return false
			}
			return true
		})
		return false
	})

	if !encontrada {
		t.Fatal("ExecutarServidor tem de chamar db.VerificarPapelRuntime: sem isso, a separação " +
			"de credenciais da ADR-043 volta a ser uma suposição sobre o deployment")
	}
}
```

Guardar como `internal/platform/arranque_guarda_test.go`. Voltar a comentar a chamada em
`app.go` e confirmar que **este** teste fica vermelho. Repor.

**O que esta guarda não cobre**, e que fica dito no ADR: ela vê a chamada, não a sua
posição. Uma chamada colocada depois do servidor arrancar passaria à mesma.

- [ ] **Passo 8: Commit**

```bash
git add internal/platform/db/privilegios.go internal/platform/app.go \
        internal/platform/arranque_guarda_test.go \
        tests/integration/privilegios_arranque_test.go
git commit -m "feat(plataforma): o servidor recusa arrancar com papel privilegiado (ADR-043)

VerificarPapelRuntime faz quatro perguntas a base de dados sobre o proprio
papel: e administrador? e dono (ou membro do dono) das tabelas de valor legal?
tem CREATE nos schemas? pode mutar o audit log? Qualquer sim e fatal, sem
isencao por APP_ENV.

A posse verifica-se com pg_has_role e nao por comparacao de nomes: com
GRANT sgc TO sgc_app, um papel que nao e dono assume-o com SET ROLE e desliga os
triggers na mesma — verificado contra postgres:16, tgenabled passa de O para D.
A variante ingenua nao apanha esse caso.

Guarda AST sobre app.go garante que a chamada nao desaparece em silencio."
```

---

### Tarefa 4: Guarda de deriva — inventário de privilégios

Impede que uma migração futura crie uma tabela sem grants. O `ALTER DEFAULT PRIVILEGES` da
Tarefa 2 cobre tabelas novas em schemas existentes; **não** cobre um schema novo, que é o
que um bounded context novo traz.

**Ficheiros:**
- Modificar: `tests/integration/privilegios_test.go`

**Interfaces:**
- Consome: `ligar(t)`, `migrarTudo(t)` da Tarefa 2.

- [ ] **Passo 1: Escrever o teste**

Acrescentar a `tests/integration/privilegios_test.go`:

```go
func TestPrivilegios_NenhumaTabelaOuSequenciaFicaSemGrant(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligar(t)

	const q = `
		SELECT coalesce(string_agg(format('%s.%s', schemaname, tablename), ', '), '')
		  FROM pg_tables
		 WHERE schemaname = ANY($1::text[])
		   AND NOT has_table_privilege('sgc_app', schemaname || '.' || tablename, 'SELECT')`
	schemas := []string{"auditoria", "clinico", "farmacia", "financeiro",
		"identidade", "laboratorio", "recepcao", "shared"}

	var semGrant string
	if err := pool.QueryRow(ctx, q, schemas).Scan(&semGrant); err != nil {
		t.Fatalf("inventariar privilégios: %v", err)
	}
	if semGrant != "" {
		t.Fatalf("tabelas sem privilégio para sgc_app: %s — acrescente os GRANT à migração "+
			"que as criou, ou o schema à lista da shared/0003 se for um bounded context novo", semGrant)
	}
}

func TestPrivilegios_AuditoriaTemExactamenteSelectEInsert(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligar(t)

	const q = `SELECT has_table_privilege('sgc_app','auditoria.auditoria_eventos','SELECT'),
	                  has_table_privilege('sgc_app','auditoria.auditoria_eventos','INSERT'),
	                  has_table_privilege('sgc_app','auditoria.auditoria_eventos','UPDATE'),
	                  has_table_privilege('sgc_app','auditoria.auditoria_eventos','DELETE'),
	                  has_table_privilege('sgc_app','auditoria.auditoria_eventos','TRUNCATE')`
	var sel, ins, upd, del, trunc bool
	if err := pool.QueryRow(ctx, q).Scan(&sel, &ins, &upd, &del, &trunc); err != nil {
		t.Fatalf("ler privilégios do audit log: %v", err)
	}
	if !sel || !ins {
		t.Fatalf("o audit log tem de aceitar SELECT e INSERT (SELECT=%v INSERT=%v)", sel, ins)
	}
	if upd || del || trunc {
		t.Fatalf("o audit log é append-only: UPDATE=%v DELETE=%v TRUNCATE=%v têm de ser todos falsos "+
			"(LPDP / Lei 22/11, ADR-043)", upd, del, trunc)
	}
}
```

- [ ] **Passo 2: Correr e confirmar que passa**

```
DATABASE_URL=postgres://sgc_app:sgc_app@localhost:5432/sgc?sslmode=disable \
DATABASE_MIGRATION_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
go test -tags=integration ./tests/integration/ -run TestPrivilegios -v
```

Esperado: PASS.

- [ ] **Passo 3: Mutação — provar que o inventário apanha uma tabela nova sem grant**

Com a credencial de migração:

```
docker compose exec postgres psql -U sgc -d sgc -c "CREATE TABLE clinico.orfa_temporaria (id int)"
```

Correr `-run TestPrivilegios_NenhumaTabela`.

Esperado: **PASS**, porque o `ALTER DEFAULT PRIVILEGES` cobre tabelas novas criadas por
`sgc` em `clinico`. Isto confirma que os defaults funcionam.

Depois, o caso que os defaults **não** cobrem:

```
docker compose exec postgres psql -U sgc -d sgc -c "CREATE SCHEMA clinico2; CREATE TABLE clinico2.orfa (id int)"
```

Acrescentar temporariamente `"clinico2"` à lista `schemas` do teste e correr de novo.

Esperado: **FAIL** com `tabelas sem privilégio para sgc_app: clinico2.orfa`. É exactamente
esta a lacuna que este teste existe para apanhar.

Limpar:

```
docker compose exec postgres psql -U sgc -d sgc -c "DROP TABLE clinico.orfa_temporaria; DROP SCHEMA clinico2 CASCADE"
```

Retirar `"clinico2"` da lista e reconfirmar verde.

- [ ] **Passo 4: Commit**

```bash
git add tests/integration/privilegios_test.go
git commit -m "test(seguranca): guarda de deriva sobre os privilegios de sgc_app (ADR-043)

O ALTER DEFAULT PRIVILEGES cobre tabelas novas em schemas existentes, mas nao um
schema novo — que e o que um bounded context novo traz. O inventario sobre
pg_tables apanha esse caso e falha em CI com o nome da tabela orfa. Mutacao
registada: tabela nova em schema existente passa (defaults funcionam); tabela em
schema novo falha (e para isso que a guarda serve).

O audit log e verificado em exactidao: SELECT e INSERT sim; UPDATE, DELETE e
TRUNCATE nao."
```

---

### Tarefa 5: Runbook, ADR-043 e actualização do contexto mestre

**Ficheiros:**
- Criar: `docs/RUNBOOK-provisionamento-bd.md`
- Criar: `adrs/ADR-043-separacao-credenciais.md`
- Modificar: `CLAUDE.md` (§6 e índice de ADRs no rodapé)
- Modificar: `SPRINT.md` (secção de critérios de saída)

- [ ] **Passo 1: Escrever o runbook**

Criar `docs/RUNBOOK-provisionamento-bd.md` cobrindo, com comandos exactos:

1. Criar o papel dono/migrador **`NOSUPERUSER`** em produção (ao contrário de
   desenvolvimento, onde a imagem impõe superuser) e o papel `sgc_app`.
2. Gerar a password de `sgc_app` no local (nunca de git) e onde a guardar.
3. Correr `api migrate` com `DATABASE_MIGRATION_URL`, e só depois arrancar o servidor com
   `DATABASE_URL`.
4. **Verificação pós-provisionamento** — as quatro consultas que o arranque faz, para o
   operador poder confirmar à mão antes de arrancar:

```sql
SELECT rolsuper OR rolcreaterole OR rolcreatedb FROM pg_roles WHERE rolname = 'sgc_app';
-- esperado: f

SELECT bool_or(pg_has_role('sgc_app', c.relowner, 'USAGE'))
  FROM unnest(ARRAY['financeiro.facturas','financeiro.itens_factura','auditoria.auditoria_eventos']) AS t(nome)
  JOIN pg_class c ON c.oid = to_regclass(t.nome);
-- esperado: f

SELECT bool_or(has_schema_privilege('sgc_app', s, 'CREATE'))
  FROM unnest(ARRAY['auditoria','clinico','farmacia','financeiro','identidade','laboratorio','recepcao','shared']) AS s;
-- esperado: f

SELECT has_table_privilege('sgc_app','auditoria.auditoria_eventos','UPDATE')
    OR has_table_privilege('sgc_app','auditoria.auditoria_eventos','DELETE');
-- esperado: f
```

5. O que **não** fazer: nunca `GRANT <dono> TO sgc_app` — verificado que isso repõe o poder
   de desligar triggers por `SET ROLE`.

- [ ] **Passo 2: Escrever a ADR-043**

Criar `adrs/ADR-043-separacao-credenciais.md` seguindo a estrutura das ADR-040/041/042:
contexto e medição, decisão, consequências, âmbito excluído, riscos.

Conteúdo obrigatório:

- A medição do §1 da spec, com os factos verificados: `sgc` é superuser; as três operações
  que isso permitia; e que o `TRUNCATE` do audit log **não estava registado em risco
  nenhum** antes desta fatia.
- A verificação do desvio por pertença, com o resultado concreto (`tgenabled` de `O` para
  `D`) e a razão de se usar `pg_has_role` em vez de comparação de nomes.
- **R1** — o migrador continua `SUPERUSER` em desenvolvimento, por construção da imagem;
  o runbook prescreve `NOSUPERUSER` em produção.
- **R2** — um DBA malicioso continua a poder tudo. O R7 defende contra aplicação
  comprometida, não contra acesso directo ao cluster. Num modelo on-premise por clínica é
  um limite real; fechá-lo exigiria WORM ou notarização externa. **A ADR não deve afirmar
  que o fecha.**
- **R3** — `pg_dump`/`pg_restore` contornam triggers.
- **R4** — a guarda AST vê a chamada a `VerificarPapelRuntime`, não a sua posição.
- Tabela final de estado dos riscos herdados: R3 e R6 da ADR-040 resolvidos (ADR-042);
  **R7 resolvido por esta ADR**; R5 (a verificação inclui `ANULADA` por desenho) continua
  aberto como restrição.
- Fora de âmbito, sem os antecipar: **anulação**, **pagamentos**, **SAF-T-AO**,
  **certificação AGT**.

- [ ] **Passo 3: Actualizar o `CLAUDE.md`**

Na §6, acrescentar um parágrafo sobre a Sprint 18 / ADR-043 no mesmo registo dos
anteriores: o que foi entregue, o que a medição revelou, e que o R7 fica **resolvido** —
mas com R2 (DBA malicioso) declarado como limite, não como omissão.

No rodapé, acrescentar `adrs/ADR-043-separacao-credenciais.md` à lista e mudar
`Próximo ADR: **ADR-043**` para `**ADR-044**`.

- [ ] **Passo 4: Actualizar o `SPRINT.md`**

Acrescentar a secção de critérios de saída, transcrevendo a §7 da spec com as caixas
marcadas.

- [ ] **Passo 5: Verificação final antes do merge**

```
go build ./... && go vet ./... && go test -race ./... && bash scripts/cobertura.sh && go-arch-lint check
```

E, contra base de dados **criada do zero** (`docker compose down -v && docker compose up -d postgres`):

```
DATABASE_MIGRATION_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable go run ./cmd/api migrate
DATABASE_URL=postgres://sgc_app:sgc_app@localhost:5432/sgc?sslmode=disable \
DATABASE_MIGRATION_URL=postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable \
go test -tags=integration ./tests/integration/...
```

Esperado: tudo PASS. Correr **também** contra a base de dados de desenvolvimento já
existente (sem `down -v`), para confirmar que a migração converge numa base já migrada —
nesse caso o papel `sgc_app` não existe ainda e tem de ser criado à mão com a linha
indicada no comentário do `init.sql`.

- [ ] **Passo 6: Commit**

```bash
git add docs/RUNBOOK-provisionamento-bd.md adrs/ADR-043-separacao-credenciais.md \
        CLAUDE.md SPRINT.md
git commit -m "docs(seguranca): ADR-043 e runbook de provisionamento da base de dados (ADR-043)

Regista o fecho do R7 e o que ele nao fecha: o migrador continua superuser em
dev por construcao da imagem, um DBA malicioso continua a poder tudo, e
pg_dump/restore contornam triggers. O R7 defende contra aplicacao comprometida,
nao contra acesso directo ao cluster — num modelo on-premise por clinica isso e
um limite real e a ADR nao finge o contrario.

Proximo ADR: 044."
```

---

## Auto-revisão do plano

**Cobertura da spec:**

| Secção da spec | Tarefa |
|---|---|
| §2.1 forma da separação (`sgc_app`) | 2 |
| §2.2 atributos do papel | 2 |
| §2.3 privilégios, ordem `shared/0003`, defaults | 2 |
| §2.4 facturas continuam com UPDATE | 2 (prova positiva) + 4 (inventário) |
| §2.5 credencial por provisionamento | 2 (dev/CI) + 5 (produção) |
| §2.6 fail-fast no arranque | 3 |
| §3 configuração | 1 |
| §4.1 provas negativas e positivas | 2 |
| §4.2 deriva | 4 |
| §4.3 guarda contra falso-verde | 1 (passo 11) |
| §4.4 mutação | 2 (passo 9), 3 (passo 7), 4 (passo 3) |
| §5 ficheiros | todas |
| §6 riscos | 5 |
| §7 critérios de saída | 5 (passo 4) |

**Consistência de tipos:** `URLMigracaoBaseDados` (Tarefa 1) é usado tal e qual na Tarefa 1
passo 5. `ligar`/`ligarApp`/`ligarCom` (Tarefa 1) são consumidos com as mesmas assinaturas
nas Tarefas 2, 3 e 4. `migrarTudo` é definido na Tarefa 2 e consumido nas Tarefas 3 e 4.
`db.VerificarPapelRuntime(ctx, pool) error` é definido na Tarefa 3 e consumido em `app.go`
na mesma tarefa. Os nomes dos schemas e das tabelas de valor legal são os mesmos em
`privilegios.go`, na migração e nos testes.

**Verificações feitas ao escrever o plano, para não sobrarem incógnitas na execução:**

1. `migrations/embed.go` embebe directórios inteiros
   (`//go:embed auditoria clinico ... shared`) — o ficheiro novo é apanhado sem tocar no
   `embed.go`.
2. `financeiro.series` existe (`migrations/financeiro/0002_emissao_facturas.sql:40`), pelo
   que a prova positiva do `FOR UPDATE` tem alvo real.
3. As únicas colunas `NOT NULL` sem default em `financeiro.facturas` são `cliente_nome` e
   `episodio_id` — o `INSERT` da prova do trigger já as inclui, e a prova exige que a
   mensagem de erro contenha `RASCUNHO` para não passar a verde por violação de `NOT NULL`.
4. Todo o SQL do plano foi corrido contra um `postgres:16` descartável, incluindo o desvio
   por pertença (`tgenabled` de `O` para `D`).
