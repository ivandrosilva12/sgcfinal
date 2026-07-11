# ADR-020 — Fundação M1 (layout, runner de migrations, segredos, papéis)

- **Estado**: Aceite
- **Data**: 2026-07-11
- **Marco**: M1 — Fundações (Sprint 1)
- **Contexto ADRs**: sucede os 19 ADRs do blueprint (`Software Gestão Clínicas`).

## Contexto

O Sprint 1 do M1 materializa, pela primeira vez em código, a fundação do SGC Angola: um
esqueleto arquitectural runnable, observável e com base de dados versionada. Foram
necessárias decisões concretas que os ADRs do blueprint não fixavam ao nível de
implementação. Este ADR regista-as.

## Decisões

### 1. Runner de migrations embebido e forward-only

- As migrations SQL são **embebidas no binário** (`embed.FS` no pacote `migrations`) e
  aplicadas por um runner próprio em `internal/platform/db/migrate.go`.
- **Forward-only**: sem ficheiros `.down.sql`. A reversão faz-se por restore (ADR do
  blueprint). O controlo é a tabela `public.schema_migrations (bounded_context, versao)`.
- Organização **por bounded context** (`migrations/<bc>/NNNN_*.sql`); ordem determinística
  (BC alfabético, ficheiro numérico); cada ficheiro aplica numa transacção e regista-se.
  Reexecução é idempotente.
- **Alternativas rejeitadas**: golang-migrate/goose (dependência externa e ficheiros de
  reversão que contrariam a política forward-only); DDL manual (não versionável/auditável).

### 2. Layout M1 (Clean Architecture)

- Quatro camadas: `domain` → `application` → `adapters` → `platform`, com a dependência a
  apontar para dentro. Cinco bounded contexts + Shared Kernel.
- A regra é **imposta em CI** por `go-arch-lint` (`.go-arch-lint.yml`): o domínio não pode
  importar vendors de infra (pgx, gin, http). Teste negativo confirmado.

### 3. i18n no Shared Kernel (desvio ao plano inicial)

- As mensagens pt-AO (`i18n`) foram colocadas em **`internal/domain/shared/i18n`** e não em
  `internal/platform/i18n` como o rascunho previa.
- **Motivo**: os adaptadores HTTP precisam das mensagens; se `i18n` vivesse na Plataforma,
  um adaptador (Camada 3) importaria a Plataforma (Camada 4), violando a regra de
  dependência. Como `i18n` é uma folha sem dependências e a linguagem ubíqua PT-PT é um
  princípio de domínio (CLAUDE.md §1), o Shared Kernel é o lugar correcto — qualquer camada
  o pode importar sem excepções ao linter.

### 4. Gestão de segredos

- **Dev**: ficheiro `.env` (a partir de `.env.example`), nunca versionado.
- **Produção**: segredos **não** vêm de `.env`; virão de **Vault ou Sealed Secrets**
  (on-premise). Nenhum segredo em variável de ambiente em claro em produção.

### 5. Papéis RBAC — 11 (DDM-001)

- Fixados os **11 papéis do DDM-001** (Errata-001 resolvida): Medico, Enfermeiro,
  Administrativo, Farmaceutico, FarmaceuticoSenior, TecnicoLab, Patologista, Director,
  Admin, DPO, Auditor. `Financeiro`/`Recepção` da lista antiga → `Administrativo`.
- Materializado em `seeds/papeis.sql`, `migrations/identidade/0003_papeis.sql` e
  `docker/keycloak/realm-sgc.json`.

### 6. Versão de Go e dependências

- `go 1.25` no `go.mod`: o Gin mais recente (mandato "latest" do CLAUDE.md) e
  `golang.org/x/net` exigem-no. Satisfaz o mínimo "Go 1.22+" do stack.

## Consequências

- Base de dados versionada e auditável desde o primeiro dia, sem dependências externas de
  migração.
- A regra de dependência é verificável mecanicamente (CI), evitando erosão arquitectural.
- Segurança de segredos com caminho claro dev→prod.
- O desvio do `i18n` deve ser tido em conta ao adicionar outras utilidades transversais:
  preferir o Shared Kernel a `platform` quando forem folhas sem infra usadas por adaptadores.
