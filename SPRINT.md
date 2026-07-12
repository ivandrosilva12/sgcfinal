# SPRINT ACTUAL

- **Marco**: M1 — Fundações
- **Sprint**: 5 (BC Identidade — ciclo de vida do utilizador) — **entregue**
- **Objectivo**: completar o ciclo de vida do utilizador — reset de password/OTP
  (admin), edição de perfil self-service (telefone/BI), revogação de sessões na
  desactivação e compensação da criação não-atómica.

## Sprint 5 — entregue

- [x] Reset de password (admin): nova senha temporária devolvida 1x; reset de OTP
      (remove credenciais + `CONFIGURE_TOTP`). Auditados.
- [x] Edição de perfil self-service (`PATCH /perfil`): telefone/BI validados pelos VOs
      Angola; omitido preserva, vazio limpa.
- [x] Revogação de sessões na desactivação; compensação da criação não-atómica
      (ADR-023 fechada).
- [x] ADR-024.

## Sprint 4 — entregue

- [x] Caminho positivo de MFA validado e2e: `director.teste` com OTP → autenticação
      forte reconhecida (acesso concedido); config de MFA fixada no realm.
- [x] Criação administrativa de utilizadores: `POST /api/v1/identidade/utilizadores`
      (RBAC Admin), senha temporária gerada (devolvida 1x), `CONFIGURE_TOTP` em papéis
      sensíveis, 409 em duplicados. Auditoria `identidade.utilizador.criado`.
- [x] ADR-023.

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

- [x] Identidade Keycloak operacional (login, 11 papéis, MFA para papéis sensíveis — positivo e negativo). — Sprint 2/3/4
- [x] BC Identidade testado (domínio 98% ≥85%). — Sprint 2 (fatia vertical completa)
- [x] Audit log append-only funcional (retenção 10 anos). — trigger imutável + teste de integração
- [ ] CI/CD: build + test + deploy staging < 15 min. — build+test ok; deploy staging por configurar
- [x] `go-arch-lint` sem violações.
- [x] Estrutura de pacotes alinhada com a arquitectura.
- [x] Migrations forward-only funcionais.
- [x] Observabilidade base: slog→JSON, healthchecks, métricas Prometheus.
- [x] README + docs de setup validados.
