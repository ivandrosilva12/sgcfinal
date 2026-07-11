# SPRINT ACTUAL

- **Marco**: M1 — Fundações
- **Sprint**: 1 (Setup + esqueleto + infra)
- **Objectivo**: repositório, CI/CD, layout de pacotes (5 BCs + Shared Kernel + 4 camadas
  Clean), Docker Compose (PG16, Keycloak, Redis, MinIO, Prometheus, Grafana), camada
  Plataforma e migrations forward-only funcionais.

## Próximas sprints M1

- **Sprint 2**: BC Identidade (domínio + casos de uso de login), integração Keycloak OIDC,
  middleware de validação JWT, audit log (tabela + trigger imutável + retenção 10 anos).
- **Sprint 3**: RBAC 11 papéis (Medico, Enfermeiro, Administrativo, Farmaceutico,
  FarmaceuticoSenior, TecnicoLab, Patologista, Director, Admin, DPO, Auditor — alinhado
  pelo DDM-001, ver `docs/ERRATA-001-papeis.md`), MFA para papéis sensíveis, smoke tests
  e2e de login.

## Critérios de saída M1

- [ ] Identidade Keycloak operacional (login, 11 papéis per DDM-001, MFA).
- [ ] BC Identidade testado (domínio ≥85%).
- [ ] Audit log append-only funcional (retenção 10 anos).
- [ ] CI/CD: build + test + deploy staging < 15 min.
- [ ] `go-arch-lint` sem violações.
- [ ] Estrutura de pacotes alinhada com a arquitectura.
- [ ] Migrations forward-only funcionais.
- [ ] Observabilidade base: slog→JSON, healthchecks, métricas Prometheus.
- [ ] README + docs de setup validados.
