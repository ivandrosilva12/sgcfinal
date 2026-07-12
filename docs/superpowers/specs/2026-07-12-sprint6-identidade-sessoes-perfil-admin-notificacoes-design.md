# Spec — M1 / Sprint 6: Sessões activas, edição admin de perfil e notificações

- **Data**: 2026-07-12
- **Marco**: M1 — Fundações (encerramento dos loose-ends do BC Identidade)
- **Bounded Context**: Identidade
- **ADR associada**: ADR-025 (a criar)
- **Precede**: plano de implementação (skill writing-plans)

## Contexto

Os Sprints 1-5 entregaram a fundação, autenticação OIDC, MFA (positivo e negativo), RBAC,
gestão administrativa, criação e o ciclo de vida do utilizador (reset de credenciais, edição
de perfil self-service, revogação de sessões na desactivação, compensação da criação
não-atómica). Ficaram fora de âmbito, como seguimento, três loose-ends registados nos "fora
de âmbito" dos sprints anteriores. O Sprint 6 fecha-os.

Backend/API apenas. PT-PT angolano em todo o output. Keycloak é a fonte de verdade dos
utilizadores/papéis/credenciais/sessões; a BD local guarda o perfil operacional (telefone/BI).

## Decisões fixadas (com o utilizador)

- **Foco**: fechar loose-ends do BC Identidade — gestão de sessões activas, edição
  administrativa do perfil de outros utilizadores e notificações por email/SMTP.
- **Entrega da senha temporária**: **email + resposta HTTP** (ambos). A senha continua a ser
  devolvida uma vez na resposta 201/200 (retrocompatível) **e** é enviada por email. O envio é
  **best-effort** — falha de SMTP regista aviso e não falha a operação.
- **Adaptador de email**: abordagem **A** — `net/smtp` da stdlib (zero dependências novas) +
  fallback `NotificadorNulo` seleccionado por config. MailHog no compose para dev.

## Componentes

Três capacidades, todas no BC Identidade.

### A) Gestão de sessões activas

- `GET /api/v1/identidade/utilizadores/:id/sessoes` — **leitura** (RBAC Admin/Auditor/DPO).
  Lista as sessões activas do utilizador via Admin API
  (`GET /admin/realms/{realm}/users/{id}/sessions`).
- `DELETE /api/v1/identidade/sessoes/:sessionId` — **escrita** (RBAC Admin). Revoga *uma*
  sessão específica (`DELETE /admin/realms/{realm}/sessions/{session}`). Audita
  `identidade.sessao.revogada` (com o sessionId no detalhe).
- DTO de leitura `SessaoActiva{ID, IP, Inicio, UltimoAcesso, Clientes []string}` — read-model,
  não confundir com o VO `Sessao` de autenticação.
- A rota `DELETE /sessoes/:sessionId` vive num grupo próprio (não sob `/utilizadores/:id`),
  porque a Admin API revoga por sessionId sem precisar do userID.

### B) Edição administrativa de perfil

- `PATCH /api/v1/identidade/utilizadores/:id/perfil` — **escrita** (RBAC Admin). Corpo
  `{ "telefone": "...", "bi": "..." }` (ambos opcionais; mesma forma do self-service do
  Sprint 5).
- Caso de uso `CasoEditarPerfilAdmin`: garante a linha local do alvo — se `ObterPorID` devolver
  `NaoEncontrado`, hidrata a partir de `admin.ObterUtilizador` + `GuardarComPapeis` (o alvo pode
  nunca ter feito login localmente); aplica o domínio `AtualizarContacto`, persiste via
  `repo.AtualizarContacto`, audita `identidade.perfil.actualizado` (actor=admin, alvo=utilizador),
  devolve o `Perfil` actualizado.
- Reutiliza integralmente a validação dos VOs Angola (`identity.NovoTelefone`/`NovoBI`) e a
  persistência do Sprint 5 — muda apenas quem despoleta (admin sobre `:id`), o RBAC e a
  hidratação a partir do Keycloak.

Distinção vs. Sprint 5: o self-service (`PATCH /perfil`) opera sobre a própria sessão; este
opera sobre `:id` com RBAC Admin.

### C) Notificações email/SMTP

- **Porta** (aplicação) `Notificador`:
  - `NotificarCriacao(ctx, email, nome, senhaTemporaria string) error`
  - `NotificarResetPassword(ctx, email, nome, senhaTemporaria string) error`
- **Adaptador** `internal/adapters/smtp/notificador.go` — `net/smtp` da stdlib (zero deps
  novas). Templates PT-PT como constantes Go (assunto + corpo texto simples). Envia via
  `SMTP_HOST:SMTP_PORT`, remetente `SMTP_FROM`. MailHog não exige auth (envio plain).
- **Fallback no-op** `NotificadorNulo` — regista via slog e devolve `nil`. O composition root
  escolhe: `SMTP_HOST` vazio → `NotificadorNulo`; caso contrário → adaptador SMTP. Testes e
  infra sem SMTP nunca quebram.
- **Ligação aos casos de uso (best-effort):**
  - `CasoCriarUtilizador` — após criar com sucesso, chama `NotificarCriacao`. Falha de envio →
    `slog.Warn` + continua (a senha mantém-se na resposta 201).
  - `CasoResetPassword` — após definir a nova senha, obtém o email do alvo
    (`admin.ObterUtilizador`) e chama `NotificarResetPassword`. Mesma política; senha mantém-se
    na resposta 200.
  - Reset-OTP e edição de perfil **não** enviam email (não carregam segredo — YAGNI).
- O envio nunca vaza a senha em logs (só regista sucesso/falha, sem corpo).

## Portas e repositório

- **`AdminIdentidade`** (2 métodos novos) — todos os implementadores (o adaptador real
  `AdminCliente` e os fakes de teste) recebem-nos:
  - `ListarSessoes(ctx, userID string) ([]SessaoActiva, error)`
  - `RevogarSessao(ctx, sessionID string) error`
- **`Notificador`** (porta nova) — Secção C.
- **`RepositorioUtilizadores`** — **sem alterações** (a edição admin reusa `AtualizarContacto`
  do Sprint 5).
- `SessaoActiva` é DTO da camada de aplicação.

## Componentes (ficheiros)

- **Domínio** `internal/domain/identidade/`:
  - `eventos.go` — evento `SessaoRevogada{Actor, SessionID, Em}` (nome
    `identidade.sessao.revogada`) + linha de conformidade. A edição admin reutiliza o evento
    `PerfilActualizado` existente.
- **Aplicação** `internal/application/identidade/`:
  - `ports.go` — 2 métodos na porta `AdminIdentidade`, porta `Notificador`, DTO `SessaoActiva`.
  - `sessoes.go` (novo) — `CasoListarSessoes`, `CasoRevogarSessao` (auditoria).
  - `editar_perfil_admin.go` (novo) — `CasoEditarPerfilAdmin`.
  - `criar_utilizador.go` — chamada best-effort ao `Notificador` após criar.
  - `reset_credenciais.go` — `CasoResetPassword` chama o `Notificador` após repor.
- **Adaptadores**:
  - `keycloak/admin.go` — `ListarSessoes`, `RevogarSessao`.
  - `smtp/notificador.go` (novo) — adaptador SMTP + `NotificadorNulo`.
  - `http/admin_handler.go` — handlers de sessões (listar/revogar) e edição admin de perfil +
    rotas + serviços novos no handler.
- **Plataforma** `internal/platform/`:
  - `config/config.go` — `SMTP_HOST` (default vazio), `SMTP_PORT` (default 1025), `SMTP_FROM`
    (default `nao-responder@sgc.ao`). Todos opcionais.
  - `app.go` — construir o `Notificador` (SMTP ou nulo conforme `SMTP_HOST`), fiar os novos
    casos de uso e ligar o notificador à criação e ao reset de password.
- **Infra/config**:
  - `docker-compose.yml` — serviço MailHog (SMTP :1025, UI :8025) com healthcheck.
  - `.env.example` — documenta `SMTP_HOST`/`SMTP_PORT`/`SMTP_FROM`.
  - `.go-arch-lint.yml` — novo componente `adapters.smtp` (só stdlib `net/smtp`; sem novos
    vendors).

## Tratamento de erros (RFC 7807, PT-PT)

| Situação | Categoria | HTTP |
|---|---|---|
| Sessões/perfil sobre utilizador inexistente | `CategoriaNaoEncontrado` | 404 |
| Revogar sessionId inexistente | `CategoriaNaoEncontrado` | 404 |
| Telefone/BI inválido (edição admin) | `CategoriaValidacao` | 400 |
| Sessão sem papel exigido | `CategoriaProibido` | 403 |
| Keycloak indisponível | `CategoriaInterno` | 500 |

Falha de **email** nunca vira erro HTTP (best-effort → só `slog.Warn`). Mensagens em PT-PT;
nunca vazar detalhes internos.

## Testes (gates: domínio ≥85%, aplicação ≥75%, adaptadores ≥60%)

- **Domínio** — evento `SessaoRevogada`.
- **Aplicação (fakes)** — `CasoEditarPerfilAdmin` (hidratação JIT a partir do Keycloak +
  validação + persistência + auditoria); `CasoListarSessoes`/`CasoRevogarSessao` (auditoria da
  revogação); `CasoCriarUtilizador`/`CasoResetPassword` chamam o `Notificador` e a **falha de
  envio não falha o caso de uso** (fake notificador que devolve erro → operação na mesma OK).
- **Adaptador keycloak** (httptest) — `ListarSessoes` (mapeia JSON→DTO), `RevogarSessao`.
- **Adaptador smtp** — servidor SMTP falso (`net.Listen`) a validar assunto/destinatário;
  `NotificadorNulo` devolve nil.
- **HTTP** — `GET /:id/sessoes` (200 + RBAC leitura), `DELETE /sessoes/:sid` (204 + RBAC Admin),
  `PATCH /:id/perfil` (200 + validação 400 + RBAC Admin).
- **Integração e2e** (`//go:build integration`, SKIP sem infra) — criar utilizador → listar
  sessões (após login) → revogar uma sessão → confirmar via Admin API; edição admin de perfil
  via BD; email capturado no MailHog (`GET /api/v2/messages`) na criação.

## Documentação e config

- **ADR-025** (novo): gestão de sessões activas, edição administrativa de perfil, notificações
  por email best-effort (com fallback no-op).
- `SPRINT.md`: Sprint 6 → entregue; nota de encerramento dos loose-ends do BC Identidade.
- `CLAUDE.md`: registar ADR-025; próximo ADR → 026.

## Verificação (fim a fim)

1. `go build ./...`, `go vet`, `gofmt` limpos; `go test ./...` verde; cobertura 85/75/60.
2. Com o compose a correr: `GET /utilizadores/:id/sessoes` como Admin → 200 + sessões activas;
   `DELETE /sessoes/:sid` → 204 e a sessão deixa de aparecer na Admin API.
3. `PATCH /utilizadores/:id/perfil` como Admin com telefone/BI válidos → 200 + perfil
   actualizado; inválidos → 400; alvo sem linha local → hidratado do Keycloak.
4. Criar utilizador → 201 com senha na resposta **e** email no MailHog. Reset de password →
   200 com senha na resposta **e** email no MailHog.
5. Com `SMTP_HOST` vazio → `NotificadorNulo`: operações OK, sem email, sem erro.
6. Auditoria registada para revogação de sessão e edição admin de perfil. Não-Admin nas
   escritas → 403.

## Fora de âmbito (Sprint 7+ / M2)

- Notificações assíncronas via outbox (retry, desacoplamento) — o caminho fica aberto.
- Auto-registo/self-service de novos utilizadores.
- Refresh-token flows e rotação de tokens.
- Início do BC Clínico (M2): agregado Doente, Alergia, Antecedente.
