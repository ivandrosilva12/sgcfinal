# SPRINT ACTUAL

- **Marco**: M1 — Fundações
- **Sprint**: 3 (BC Identidade — MFA + gestão administrativa) — **entregue**
- **Objectivo**: fatia vertical do BC Identidade — autenticação Keycloak (JWT RS256),
  RBAC pelos 11 papéis, auditoria de acesso e `GET /api/v1/identidade/perfil`.

## Sprint 3 — entregue

- [x] Imposição de MFA para papéis sensíveis (Director, Admin, DPO, Auditor):
      `Sessao.AutenticacaoForte` derivada de `acr`/`amr`, middleware
      `MFAObrigatoria` → 403 (`type: /erros/mfa-obrigatorio`).
- [x] Gestão administrativa via Admin REST API do Keycloak: listar, ver,
      atribuir/revogar papel, activar/desactivar (adaptador HTTP puro).
- [x] RBAC por rota: escrita só Admin; leitura Admin/Auditor/DPO. Auditoria de
      todas as escritas.
- [x] Realm: client `sgc-admin` (service account) + utilizador `admin.teste`.
- [x] Smoke tests e2e (MFA negativo + fluxo de atribuição via Keycloak).
- [x] ADR-022.

## Sprint 2 — entregue

- [x] Domínio Identidade: agregado `Utilizador`, VO `Sessao`, enum `Papel` (11), regras
      RBAC puras, eventos e interface de repositório (cobertura 98%).
- [x] Aplicação: casos de uso `Autenticar` e `ObterPerfil` (JIT provisioning + auditoria),
      com portas `VerificadorToken`/`Auditor` (cobertura 95%).
- [x] Adaptadores: cliente Keycloak OIDC (go-oidc, JWKS/RS256, validação `aud`/`azp`),
      middleware `Auth`/`RBAC`/`SegurancaHTTP`/`LimiteTaxa`, respostas RFC 7807 (pt-AO),
      repos pgx (`utilizadores`/`utilizadores_papeis`/auditoria).
- [x] `/readyz` passa a verificar também o Keycloak (JWKS/discovery).
- [x] Migration `identidade/0004_seed_papeis.sql` (catálogo de 11 papéis para o JIT).
- [x] Testes: unit+aplicação (`-race`), gate 85/75/60 OK; **integração end-to-end** com
      Keycloak+PG reais (token real → JIT → auditoria).
- [x] ADR-021 (verificação OIDC, RBAC, JIT). go-oidc registado no `go-arch-lint`.

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

- **Sprint 3**: MFA para papéis sensíveis (Director, Admin, DPO, Auditor), gestão
  administrativa de utilizadores/papéis, endpoints protegidos por papel (aplicar `RBAC`),
  smoke tests e2e de login. Ver `docs/ERRATA-001-papeis.md`.

## Critérios de saída M1

- [x] Identidade Keycloak operacional (login, 11 papéis, MFA para papéis sensíveis). — Sprint 2/3
- [x] BC Identidade testado (domínio 98% ≥85%). — Sprint 2 (fatia vertical completa)
- [x] Audit log append-only funcional (retenção 10 anos). — trigger imutável + teste de integração
- [ ] CI/CD: build + test + deploy staging < 15 min. — build+test ok; deploy staging por configurar
- [x] `go-arch-lint` sem violações.
- [x] Estrutura de pacotes alinhada com a arquitectura.
- [x] Migrations forward-only funcionais.
- [x] Observabilidade base: slog→JSON, healthchecks, métricas Prometheus.
- [x] README + docs de setup validados.
