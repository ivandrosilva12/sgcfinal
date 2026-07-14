# SPRINT ACTUAL

- **Marco**: M3 — Laboratório
- **Sprint**: 12 (BC Laboratório — catálogo, requisição, amostra, preliminar) — **entregue**
- **Objectivo**: abrir o BC Laboratório até ao resultado preliminar submetido pelo
  técnico — que não é visível ao médico. A validação com segregação de funções e os
  valores críticos são o Sprint 13.

## Sprint 12 — entregue

- [x] Catálogo de análises (intervalos de referência, valores críticos) com seed.
- [x] Requisição no BC Laboratório via ACL sobre o Clínico (doente activo, episódio
      aberto); um resultado PENDENTE por análise, em transacção única.
- [x] Colheita, recusa (motivo obrigatório) e submissão do preliminar; o técnico
      gravado é sempre o sujeito autenticado.
- [x] O preliminar **não** é visível ao médico: a leitura clínica filtra por
      VALIDADA/CONCLUIDA na aplicação.
- [x] Guarda compare-and-set nas transições; CHECK de coerência estado↔timestamps.
- [x] ADR-031.

## Sprint 11 — entregue

- [x] Agregado `Consentimento` (LPDP): 5 finalidades, registar/revogar/listar/obter,
      escritas auditadas. Migration `clinico/0003_consentimentos.sql`.
- [x] Invariante-estrela: um procedimento exige consentimento de finalidade CIRURGIA,
      concedido e com anexo — verificada no agendamento e **revalidada no início**
      (o doente pode revogar e o episódio pode fechar entretanto).
- [x] Agregado `ProcedimentoCirurgico` com state machine AGENDADO → EM_CURSO →
      CONCLUIDO/CANCELADO (cancelamento DDM-estrito, só intra-operatório), VO
      `Anestesia` e tipo de episódio `CIRURGIA_AMBULATORIA`.
- [x] Catálogo de procedimentos (read model, seed PRC001–PRC007), repositórios pgx,
      handlers HTTP com RBAC e testes de integração contra Postgres.
- [x] Guardas de concorrência: compare-and-set nos UPDATE de transição e de revogação.
- [x] ADR-030.

## Sprint 10 — entregue

- [x] BC Farmácia — stock: agregados `Fornecedor` e `Lote`, movimentos de stock
      append-only (ADR-017), migration própria.
- [x] Motor de dispensa transaccional com alocação FEFO, validação de alergias e
      serialização da dispensa da mesma receita (não-exceder revalidado na transacção).
- [x] Repositórios pgx, handler HTTP com RBAC e testes de integração.
- [x] ADR-029.

## Sprint 9 — entregue

- [x] BC Farmácia — receita: agregados `Medicamento` (catálogo) e `Receita` (itens,
      estado, eventos), casos de uso com validação de alergias e override justificado.
- [x] Adaptador `LeitorClinico` (camada anti-corrupção sobre o BC Clínico).
- [x] Categoria de erro `RegraNegocio` no Shared Kernel, mapeada para 422.
- [x] ADR-028.

## Sprint 8 — entregue

- [x] BC Clínico — agregado `EpisodioClinico`: estados, diagnósticos CID, notas,
      iniciar/actualizar/fechar/cancelar, e projecção EHR do doente.
- [x] Repositório pgx, handler HTTP com RBAC clínico, integração.
- [x] ADR-027.

## Sprint 7 — entregue

- [x] BC Clínico — agregado `Doente`: VOs de identificação e contactos, entidades-filho
      (alergia, antecedente), estados e eventos.
- [x] Casos de uso de registo, pesquisa, obtenção auditada, actualização e gestão de
      estado; validador de NIF angolano no Shared Kernel.
- [x] Repositório pgx, handler HTTP com RBAC clínico/administrativo, integração.
- [x] ADR-026.

## Sprint 6 — entregue

- [x] Gestão de sessões activas: listar por utilizador (Admin/Auditor/DPO),
      revogar sessão granular por sessionId (Admin). Auditado.
- [x] Edição administrativa de perfil (`PATCH /utilizadores/:id/perfil`, Admin)
      com hidratação JIT a partir do Keycloak.
- [x] Notificações por email (criação e reset de password) best-effort, com
      fallback no-op quando SMTP não configurado. MailHog no compose.
- [x] ADR-025.

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

## Critérios de saída M2 — Clínico Core

- [x] BC Clínico: doente, episódio clínico e EHR. — Sprints 7/8
- [x] BC Farmácia: catálogo, receita, stock e dispensa (FEFO). — Sprints 9/10
- [x] Cirurgia ambulatória: tipo de episódio, procedimento cirúrgico e consentimento
      com anexo obrigatório. — Sprint 11
- [x] Gates de cobertura verdes em todas as fatias (domínio ≥85%, aplicação ≥75%,
      adaptadores ≥60%; pgrepo coberto por integração).
- [x] ADRs 026–030 registadas.

## Critérios de saída M3 — Laboratório

- [x] Médico requisita análises para um episódio aberto. — Sprint 12
- [x] Técnico colhe/recusa amostra e submete resultado preliminar. — Sprint 12
- [x] O preliminar não é visível ao médico. — Sprint 12
- [ ] Validação pelo patologista com segregação (submissor ≠ validador). — Sprint 13
- [ ] Valores críticos detectados e notificados (SMS auditado). — Sprint 13
- [ ] Correcção cria novo resultado preservando o original. — Sprint 13

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
