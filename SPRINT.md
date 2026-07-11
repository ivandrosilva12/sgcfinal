# SPRINT ACTUAL

- **Marco**: M1 — Fundações
- **Sprint**: 1 (Setup + esqueleto + infra) — **entregue**
- **Objectivo**: repositório, CI/CD, layout de pacotes (5 BCs + Shared Kernel + 4 camadas
  Clean), Docker Compose (PG16, Keycloak, Redis, MinIO, Prometheus, Grafana), camada
  Plataforma e migrations forward-only funcionais.

## Sprint 1 — entregue

- [x] Layout de pacotes (5 BCs + Shared Kernel + 4 camadas Clean) com `go-arch-lint` a
      impor a regra de dependência (Domínio sem infra).
- [x] Camada Plataforma funcional: `config` validada, `log` (slog JSON), `observ`
      (Prometheus + `/metrics`), `db` (pgxpool + runner de migrations), `server` (Gin +
      shutdown gracioso), Shared Kernel `i18n` (pt-AO).
- [x] Migrations forward-only + `schema_migrations` (BC `auditoria`/`identidade`/`shared`);
      audit log append-only com trigger imutável; seed dos 11 papéis (DDM-001).
- [x] Endpoints `/healthz`, `/readyz` (PG+Redis) e `/metrics`.
- [x] Docker Compose (PG16, Keycloak 25, Redis 7, MinIO, Prometheus, Grafana) com
      healthchecks; realm `sgc` preparado (11 papéis, client, utilizador de teste).
- [x] CI/CD: build, `golangci-lint`, `go-arch-lint`, testes `-race`, gate de cobertura
      (85/75/60), integração (migrations/audit/seed com PG), `govulncheck`, `gosec`,
      `hadolint`, `Trivy`. Spec OpenAPI base (`api/openapi/`).
- [x] Validadores Angola (BI, telefone, AOA) no Shared Kernel, testados (domínio ≥85%).
- [x] Errata-001 resolvida (11 papéis) e docs reconciliados.

## Próximas sprints M1

- **Sprint 2**: BC Identidade (domínio + casos de uso de login), integração Keycloak OIDC,
  middleware de validação JWT, audit log (tabela + trigger imutável + retenção 10 anos).
- **Sprint 3**: RBAC 11 papéis (Medico, Enfermeiro, Administrativo, Farmaceutico,
  FarmaceuticoSenior, TecnicoLab, Patologista, Director, Admin, DPO, Auditor — alinhado
  pelo DDM-001, ver `docs/ERRATA-001-papeis.md`), MFA para papéis sensíveis, smoke tests
  e2e de login.

## Critérios de saída M1

- [ ] Identidade Keycloak operacional (login, 11 papéis per DDM-001, MFA). — Sprint 2/3
- [ ] BC Identidade testado (domínio ≥85%). — Sprint 2 (fatia vertical de Identidade)
- [x] Audit log append-only funcional (retenção 10 anos). — trigger imutável + teste de integração
- [ ] CI/CD: build + test + deploy staging < 15 min. — build+test ok; deploy staging por configurar
- [x] `go-arch-lint` sem violações.
- [x] Estrutura de pacotes alinhada com a arquitectura.
- [x] Migrations forward-only funcionais.
- [x] Observabilidade base: slog→JSON, healthchecks, métricas Prometheus.
- [x] README + docs de setup validados.
