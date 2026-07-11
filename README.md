# SGC Angola — Sistema de Gestão de Clínicas

Sistema de gestão de clínicas privadas em Angola. Backend em **Go/Gin** com **DDD táctico +
Clean Architecture** (monólito modular), **PostgreSQL 16** (SQL puro via pgx v5),
**Keycloak** (OIDC), **Redis**, **MinIO** (documentos) e observabilidade
**Prometheus + Grafana**.

> Convenções completas em [`CLAUDE.md`](./CLAUDE.md). Marco actual e âmbito em
> [`docs/PLANO-M1-Fundacao-Identidade.md`](./docs/PLANO-M1-Fundacao-Identidade.md).

## Estado

**M1 — Fundações** (em arranque). Esta fase entrega:

- Esqueleto arquitectural (5 bounded contexts + Shared Kernel, 4 camadas Clean).
- Infra local via Docker Compose (PostgreSQL, Keycloak, Redis, MinIO, Prometheus, Grafana).
- Camada Plataforma: config validada, logging estruturado (slog), pool pgx, runner de
  migrations forward-only, servidor HTTP com shutdown gracioso, métricas Prometheus.
- Fatia vertical do BC **Identidade**: Keycloak OIDC + middleware JWT RS256 + RBAC (8 papéis).
- Audit log append-only (trigger PG imutável, retenção 10 anos).

## Arquitectura (resumo)

```
cmd/api            → entrypoint
internal/domain    → DDD puro (sem infra)
internal/application → casos de uso
internal/adapters  → http, pgrepo, keycloak, redis, minio, outbox
internal/platform  → config, log, observ, server, db, composition root
migrations/        → SQL forward-only por bounded context
```

Regra de dependência (Domínio → Aplicação → Adaptadores → Plataforma) validada por
`go-arch-lint` em CI.

## Arranque local (após implementação de M1)

```bash
cp .env.example .env          # ajustar segredos locais
docker compose up -d          # PG, Keycloak, Redis, MinIO, Prometheus, Grafana
make migrate                  # aplica migrations forward-only
make run                      # arranca a API
```

Verificação rápida: `GET /healthz` (200), `GET /metrics` (Prometheus), login via Keycloak
e `GET /api/v1/identidade/perfil` com token `Bearer`.

## Qualidade

```bash
make lint    # golangci-lint + go-arch-lint
make test    # unit + aplicação (+ integração via Testcontainers se Docker disponível)
make cover   # relatório de cobertura (domínio ≥85%, aplicação ≥75%, adapters ≥60%)
```

## Licença / Conformidade

Conformidade LPDP (Lei 22/11), on-premise por clínica, audit log 10 anos, AGT/SAF-T-AO.
Ver `docs/` e `adrs/`.
