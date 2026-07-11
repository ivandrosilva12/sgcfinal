# Plano — SGC Angola: Fundação (M1) + Bounded Context Identidade

## Contexto

A pasta `Software Clinicas Final` está a receber a **implementação real** do sistema **SGC
Angola**, cujo blueprint completo (≈80 documentos, 19 ADRs, modelo de dados, convenções)
existe na pasta irmã `Software Gestão Clínicas`. **Não existia código** — só especificação.

Stack pedida = stack não-negociável já definida: Go 1.22+, Gin, pgx v5 (SQL puro),
PostgreSQL 16 (schema por bounded context), Keycloak 25 (OIDC), Redis 7, Prometheus +
Grafana, MinIO (documentos).

**Módulo Go**: `github.com/ivandrosilva12/sgcfinal`.

**Âmbito desta construção**: Marco **M1 — Fundações** + fatia vertical completa do BC
**Identidade**, **apenas backend/API**. Entrega esqueleto arquitectural correcto, runnable,
autenticado e observável.

Fonte de verdade das convenções: `Software Gestão Clínicas\Prompts\CLAUDE.md` (adaptado →
`CLAUDE.md` do repo). O esquema das tabelas de Identidade/Auditoria foi **extraído do
`DDM-001 v2.0.docx`** (não inventado) — ver secção "Modelo de dados".

## Princípios herdados (não-negociáveis)

- **Linguagem ubíqua PT-PT angolano** em TODO o output.
- **DDD táctico + Clean Architecture**, monólito modular. Dependência aponta para dentro:
  Domínio → Aplicação → Adaptadores → Plataforma.
- **Domínio rico**; zero infra (`pgx`/`gin`/`http`) em `domain/`.
- **Migrations forward-only**, sem `.down.sql`.
- **Audit log append-only** (trigger PG bloqueia UPDATE/DELETE), retenção 10 anos.
- **Sem `panic()`** fora de inicialização.
- Cobertura desde Sprint 1: domínio ≥85%, aplicação ≥75%, adapters ≥60% — **imposta em CI**.

---

## Modelo de dados (extraído do DDM-001 v2.0)

Schemas por bounded context. Para M1, os schemas `identidade` e `auditoria`:

- **`identidade.utilizadores`** — perfil do utilizador; `keycloak_id` é a referência à
  identidade no Keycloak (Keycloak é a fonte de verdade da autenticação; a BD guarda
  perfil + papéis + dados operacionais).
- **`identidade.utilizadores_papeis`** — junção `(utilizador_id → utilizadores(keycloak_id),
  papel_codigo)`. Modelo RBAC.
- **`auditoria.auditoria_eventos`** (audit log) — append-only, trigger imutável, retenção
  10 anos.

### Papéis (enum `Papel`) — alinhado pelos 11 do DDM-001

`PapelMedico`, `PapelEnfermeiro`, `PapelAdministrativo`, `PapelFarmaceutico`,
`PapelFarmaceuticoSenior`, `PapelTecnicoLab`, `PapelPatologista`, `PapelDirector`,
`PapelAdmin`, `PapelDPO`, `PapelAuditor`.

> **Errata registada** (`docs/ERRATA-001-papeis.md`): o `m1-fundacoes.md`/`CLAUDE.md`
> referem "8 papéis"; o `DDM-001` (modelo de dados, mais recente) define 11. **Fonte
> adoptada: DDM-001 (11)**. A validar formalmente pelo Tech Lead antes de fixar o enum.

---

## Layout a criar (raiz = `Software Clinicas Final`)

```
go.mod (github.com/ivandrosilva12/sgcfinal)  go.sum
Makefile  .golangci.yml  .go-arch-lint.yml  .editorconfig
.gitignore  .env.example  .dockerignore
README.md  CLAUDE.md  SPRINT.md  Dockerfile  docker-compose.yml
.github/workflows/ci.yml     # ← NOVO (melhoria 1)
docker/
  prometheus/prometheus.yml
  grafana/provisioning/{datasources,dashboards}/*  + dashboard base JSON
  keycloak/realm-sgc.json    # realm: 11 papéis, clients, utilizador de teste
  postgres/init.sql          # cria BDs/schemas por BC
cmd/api/main.go              # entrypoint (graceful shutdown)
internal/
  domain/
    shared/                  # Shared Kernel
      identity/bi.go telefone.go     # validadores Angola
      moeda/aoa.go  evento/evento.go  erros/erros.go
      auditoria/registo.go
    identidade/              # ← BC REAL
      utilizador.go          # agregado raiz (rico)
      papel.go               # enum 11 papéis + regras RBAC (funções puras)
      sessao.go  eventos.go  regras.go  repositorio.go
    clinico/ farmacia/ laboratorio/ financeiro/   # placeholders (estrutura preparada)
  application/
    identidade/
      autenticar.go  obter_perfil.go  ports.go
  adapters/
    http/
      router.go
      middleware/            # auth JWT RS256 (JWKS), rbac, requestid, logging,
                             #   recover, ratelimit (Redis), seguranca (CORS/HSTS/CSP),
                             #   problem (RFC 7807)
      identidade_handler.go  # GET /api/v1/identidade/perfil
      health_handler.go      # /healthz (liveness) + /readyz (readiness)  ← melhoria 4
    pgrepo/
      identidade_repo.go  auditoria_repo.go   # impl pgx (o pool vive em platform/db)
    keycloak/cliente.go      # OIDC discovery + cache JWKS + validação RS256
    redis/cliente.go  ratelimit.go
    minio/cliente.go         # wrapper S3 (documentos, presigned URLs) — skeleton
    outbox/relay.go          # poller do outbox (skeleton p/ eventos inter-BC)
  platform/
    config/config.go         # env com validação explícita (falha no arranque)
    log/log.go               # slog JSON estruturado
    observ/metrics.go        # registry Prometheus + middleware + /metrics
    i18n/                    # ← NOVO (melhoria 8) mensagens pt-AO extraídas
    server/server.go         # http.Server + graceful shutdown (signal.NotifyContext)
    db/pool.go  db/migrate.go# pgxpool + runner forward-only (embed.FS)
    app.go                   # composition root
migrations/                  # ← por bounded context (melhoria 5)
  auditoria/0001_auditoria_eventos.sql   # tabela + trigger imutável + retenção 10 anos
  identidade/0001_utilizadores.sql
  identidade/0002_utilizadores_papeis.sql
  shared/0001_outbox.sql
seeds/papeis.sql             # 11 papéis
tests/
  unit/identidade/  application/identidade/  integration/  fakes/  factories/
docs/  adrs/ADR-020-fundacao-m1.md
api/openapi/                 # ← NOVO (melhoria 7) spec gerada por swag
```

### Componentes-chave

**Plataforma**: `config` (validação explícita), `log` (slog JSON), `observ` (Prometheus,
alvo P95 CRUD <500ms), `db` (pgxpool + runner de migrations por BC via `embed.FS` +
`schema_migrations`), `server` (shutdown gracioso), `i18n` (pt-AO), `app.go` (composition root).

**BC Identidade**: `Utilizador` (agregado rico) + `Papel` (enum 11) + regras RBAC puras +
interfaces de repositório (zero infra). Aplicação: `Autenticar`/`ObterPerfil` sobre portas
(`KeycloakPort`, `AuditoriaPort`, `RepositorioUtilizadores`). Adaptadores: cliente Keycloak
(JWKS/RS256), middleware auth+RBAC, repos pgx, auditoria append-only, erros RFC 7807 (PT-PT).

**Segurança transversal**: middleware auth JWT RS256, RBAC por endpoint, rate limit Redis
(100/min IP · 1000/min utilizador · 10/min endpoints sensíveis), cabeçalhos de segurança,
CORS por ambiente, request-id, recover.

**docker-compose.yml** (dev): `postgres:16`, `keycloak:25` (import `realm-sgc.json`),
`redis:7`, `minio` (+ init bucket `documentos`), `prometheus`, `grafana` (datasource +
dashboard provisionados), `orthanc` (preparado p/ M3), e o serviço `api`. Healthchecks em todos.

---

## Melhorias incorporadas (todas as 4 escolhidas)

1. **CI + gates + segurança** — `.github/workflows/ci.yml`: build, `golangci-lint`,
   `go-arch-lint`, testes, **gate de cobertura que falha <85/75/60**, e scans
   `gosec` + `govulncheck` + `Trivy` (imagem) + `hadolint` (Dockerfile).
2. **Readiness vs liveness** — `/healthz` (processo vivo) separado de `/readyz` (verifica
   PG + Redis + JWKS do Keycloak); usado pelos healthchecks do compose.
3. **Migrations por bounded context** — `migrations/<bc>/NNNN_*.sql`, forward-only.
4. **OpenAPI (swag)** — anotações nos handlers de Identidade → spec em `api/openapi/`.
5. **i18n pt-AO desde Sprint 1** — mensagens extraídas em `internal/platform/i18n/`.
6. **Segredos** — `.env` em dev; nota do caminho de produção (Vault/Sealed Secrets),
   registada em ADR-020. Nenhum segredo em env em produção.
7. **Esquema real do DDM-001** — `utilizadores`, `utilizadores_papeis`, `auditoria_eventos`
   (não inventados); enum `Papel` com 11 valores + errata da divergência 8 vs 11.

---

## Verificação (fim a fim)

1. **Infra**: `docker compose up -d` → todos `healthy`.
2. **Migrations**: `make migrate` aplica por BC; `schema_migrations` populado.
3. **Audit log imutável**: INSERT ok; UPDATE/DELETE em `auditoria_eventos` → `RAISE EXCEPTION`.
4. **API**: `GET /healthz` = 200; `GET /readyz` = 200 só com dependências prontas;
   `GET /metrics` expõe Prometheus.
5. **Auth Keycloak**: token do utilizador de teste (realm `sgc`); `GET
   /api/v1/identidade/perfil` com `Bearer` válido → 200 + evento de auditoria; sem token →
   401; papel sem permissão → 403 (RFC 7807).
6. **Rate limit**: exceder endpoint sensível → 429 + `Retry-After`.
7. **Observabilidade**: alvo `api` `UP` no Prometheus; dashboard base no Grafana.
8. **Qualidade/CI**: `make lint` (golangci + go-arch-lint) sem violações; `make test`
   passa; **cobertura ≥85% no domínio Identidade** (gate); scans de segurança sem findings
   críticos.

## Decisões registadas / abertas

- **ADR-020** documenta: runner de migrations embebido, layout M1, caminho de segredos.
- **Errata-001**: divergência 8 vs 11 papéis — fonte adoptada `DDM-001`; validação humana pendente.
- MFA: configurável; obrigatório para papéis sensíveis fica para M1 Sprint 3.
