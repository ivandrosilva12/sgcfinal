# CLAUDE.md — Contexto Mestre do Projecto SGC Angola

> Lido automaticamente por Claude Code no início de cada sessão. Conciso e curado.
> Adaptado das convenções do blueprint `Software Gestão Clínicas` (v2.0, Maio 2026)
> para o repositório de implementação `Software Clinicas Final`.

---

## 1. Identidade e Voz

- **Projecto**: SGC Angola — Sistema de Gestão de Clínicas privadas em Angola.
- **Idioma**: **PT-PT angolano** em TODA a saída — código, comentários, commits, PRs,
  docs, mensagens de erro, UI. Nunca PT-BR. Nunca EN nas mensagens visíveis.
- **Linguagem ubíqua**: termos clínicos/negócio em português (Doente, Episódio, Receita,
  Factura, Lote, Dispensa, Utilizador, Papel, Sessão, MovimentoStock, Composição). Nunca
  misturar com EN (não usar Patient/Invoice/Prescription).
- **Tom**: profissional, preciso, sem floreados.

## 2. Stack Técnico (não negociável)

| Camada | Tecnologia | Versão | Notas |
|--------|------------|--------|-------|
| Backend | Go | 1.22+ | sem alternativas |
| Web framework | Gin | latest | escolhido sobre chi (ADR-002) |
| Driver PG | pgx | v5 | **sem ORM — SQL puro** (ADR-003) |
| BD | PostgreSQL | 16 | schema por bounded context (ADR-004) |
| Auth | Keycloak | 25 | OIDC; não rolar próprio (ADR-008) |
| Cache/sessões/rate-limit | Redis | 7+ | |
| Object storage (documentos) | MinIO (S3) | latest | presigned URLs |
| Observabilidade | Prometheus + Grafana | latest | on-premise; slog→JSON→journald |
| Container | Docker + Compose | latest | dev/staging/prod |
| PACS (M3, ADR-016) | Orthanc DICOMweb | 1.12+ | referência apenas; DICOM nunca na BD |

**Nunca propor alternativas sem uma ADR formal.** Em dúvida, parar e pedir validação.

## 3. Arquitectura

- **Estilo**: Monólito modular com **DDD táctico + Clean Architecture**.
- **5 Bounded Contexts**: `clinico`, `farmacia`, `laboratorio`, `financeiro`,
  `identidade` + **Shared Kernel**.
- **4 Camadas Clean** (dependência aponta para dentro):
  1. **Domínio** — entidades, VOs, eventos, interfaces de repositório. Zero imports de infra.
  2. **Aplicação** — casos de uso. Importa apenas Domínio.
  3. **Adaptadores** — HTTP (Gin), repositórios PG (pgx), integrações. Importam Domínio+Aplicação.
  4. **Plataforma** — composition root, config, observabilidade. Importa tudo.
- **Inter-context**: eventos via **Outbox** (assíncrono); interfaces explícitas; **ACL**.
- **Sem FK cross-context**. Snapshots onde necessário.
- **Migrations**: forward-only, sem `.down.sql`. Reversão por restore.
- **Linter arquitectural**: `go-arch-lint` em CI bloqueia violações da regra de dependência.

## 4. Layout do Repositório

```
cmd/api/main.go              # entrypoint (graceful shutdown)
internal/
├── domain/                  # Camada 1 — DDD (shared, clinico, farmacia, laboratorio,
│                            #   financeiro, identidade)
├── application/             # Camada 2 — casos de uso
├── adapters/                # Camada 3 — http, pgrepo, keycloak, redis, minio, outbox, pacs
└── platform/                # Camada 4 — config, log, observ, server, db, app.go
migrations/                  # por BC, numeradas (forward-only)
seeds/  tests/  docs/  adrs/  docker/
```

## 5. Princípios Não-Negociáveis

1. **On-premise por clínica** — dados clínicos não saem do território (Lei 22/11).
2. **LPDP mínimo** — encryption at rest/in transit, audit log append-only, RBAC granular.
3. **Audit log imutável** — trigger PG bloqueia UPDATE/DELETE. **Retenção 10 anos.**
4. **Cadeia hash de facturas** — SHA-256, imutáveis. Anulação por nova factura.
5. **Domínio rico, não anémico** — regras nas entidades.
6. **Fakes > Mocks** em testes de aplicação.
7. **Forward-only migrations**.
8. **Nada de `panic()`** fora de inicialização — sempre `error`.
9. Cobertura **desde Sprint 1**: domínio ≥85%, aplicação ≥75%, adapters ≥60%.

## 6. Marco Actual

**M3 — Laboratório** (Sprints 12–13; ver `SPRINT.md`). Entrega o BC Laboratório:
catálogo de análises, requisição (via ACL sobre o Clínico), amostra e resultado com
state machine; a validação com segregação de funções (técnico ≠ patologista) e os
valores críticos fecham o marco no Sprint 13.

**Marco Percurso Ambulatório** (em curso, a par do M3): abre o percurso do doente antes
da consulta. Sub-projecto entregue: **Marcação** (BC `recepcao` — agenda por
disponibilidade, ciclo de vida da marcação, ACL sobre o Clínico; ver ADR-032). Pendentes:
Recepção/check-in e Triagem.

**M2 — Clínico Core** (entregue; Sprints 7–11): BC Clínico (doente, episódio + EHR,
cirurgia ambulatória com consentimento LPDP) e BC Farmácia (catálogo, receita, stock
e dispensa FEFO).

**M1 — Fundações** (entregue; ver `docs/PLANO-M1-Fundacao-Identidade.md`): esqueleto
arquitectural + infra (Docker Compose) + fatia vertical do BC Identidade (Keycloak
OIDC + JWT RS256 + RBAC 11 papéis — DDM-001, ver `docs/ERRATA-001-papeis.md`) +
audit log + observabilidade. Pendente: deploy de staging em CI/CD.

## 7. Antipadrões a Rejeitar

- ❌ Domínio anémico. ❌ Infra (`pgx`/`gin`/`http`) em `internal/domain/`.
- ❌ "God service". ❌ Modelo único partilhado entre BCs. ❌ Repositório CRUD genérico.
- ❌ Linguagem misturada (PT/EN/BR). ❌ Coverage theatre. ❌ ORM "para acelerar".
- ❌ Armazenar DICOM na BD (ADR-016). ❌ MovimentoStock como UPDATE (ADR-017).

## 8. Regras Operacionais Angola

- **BI**: 8 dígitos + 2 letras + 3 dígitos. Validador em `internal/domain/shared/identity/`.
- **Telefone**: `+244 9XX XXX XXX`.
- **Moeda**: AOA (Kwanza). Display: `1.234,50 Kz`.
- **IVA**: 14% standard; saúde geralmente isenta; configurável por item.
- **AGT/SAF-T-AO**: cadeia hash SHA-256, numeração sequencial, submissão mensal até dia 25.

## 9. Em Caso de Dúvida

Consultar `docs/`, procurar ADR existente em `adrs/`; se persistir, **parar e pedir
confirmação humana**. Nunca improvisar decisão arquitectural ou de conformidade.

---

**Convenções-fonte**: `..\Software Gestão Clínicas\Prompts\CLAUDE.md` (blueprint completo,
80 documentos + 19 ADRs). ADRs registadas: `adrs/ADR-020-fundacao-m1.md`,
`adrs/ADR-021-identidade-oidc-rbac.md`, `adrs/ADR-022-mfa-gestao-admin.md`,
`adrs/ADR-023-mfa-positivo-criar-utilizadores.md`, `adrs/ADR-024-ciclo-vida-utilizador.md`,
`adrs/ADR-025-sessoes-perfil-admin-notificacoes.md`, `adrs/ADR-026-bc-clinico-doente.md`,
`adrs/ADR-027-bc-clinico-episodio.md`, `adrs/ADR-028-bc-farmacia-receita.md`,
`adrs/ADR-029-farmacia-stock-dispensa.md`,
`adrs/ADR-030-cirurgia-ambulatoria-consentimento.md`,
`adrs/ADR-031-bc-laboratorio.md`,
`adrs/ADR-032-bc-recepcao-marcacao.md`.
Próximo ADR: **ADR-033**.
