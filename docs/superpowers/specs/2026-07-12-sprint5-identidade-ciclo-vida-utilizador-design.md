# Spec — M1 / Sprint 5: Ciclo de vida do utilizador

- **Data**: 2026-07-12
- **Marco**: M1 — Fundações (encerramento da gestão de utilizadores)
- **Bounded Context**: Identidade
- **ADR associada**: ADR-024 (a criar)
- **Precede**: plano de implementação (skill writing-plans)

## Contexto

Os Sprints 3-4 entregaram autenticação, MFA (positivo e negativo), RBAC, gestão
administrativa e criação de utilizadores. Ficaram fora de âmbito, como seguimento, as
operações que completam o ciclo de vida do utilizador: reposição de credenciais, edição
de perfil, revogação de sessões e a compensação da criação não-atómica (registada como
limitação na ADR-023). O Sprint 5 fecha essa história.

Backend/API apenas. PT-PT angolano em todo o output. Keycloak é a fonte de verdade dos
utilizadores/papéis/credenciais; a BD local guarda o perfil operacional (telefone/BI).

## Decisões fixadas (com o utilizador)

- **Foco**: completar o ciclo de vida do utilizador — reset de password/OTP, edição de
  perfil, revogação de sessões na desactivação, e compensação da criação não-atómica.
- **Reset de password**: a API gera uma **nova senha temporária** (`temporary:true`,
  muda no próximo login) e devolve-a **uma única vez**. Coerente com a criação (Sprint 4);
  testável sem SMTP.
- **Edição de perfil**: **self-service** — o utilizador autenticado edita o seu próprio
  telefone/BI via `PATCH /api/v1/identidade/perfil`. Não envolve o Keycloak (campos locais).

## Componentes

Cinco capacidades, todas no BC Identidade.

### A) Reset de password (admin)

`POST /api/v1/identidade/utilizadores/:id/reset-password` — `RBAC(Admin)`. Gera nova
senha temporária, define-a no Keycloak (`temporary:true`), devolve `{senha_temporaria}`
uma vez, audita `identidade.password.reposta`.

### B) Reset de OTP (admin)

`POST /api/v1/identidade/utilizadores/:id/reset-otp` — `RBAC(Admin)`. Remove as
credenciais do tipo `otp` do utilizador e adiciona a required action `CONFIGURE_TOTP`
(re-inscrição no próximo login). Audita `identidade.otp.reposto`. Essencial para papéis
sensíveis que perderam o dispositivo de 2º factor.

### C) Edição de perfil self-service

`PATCH /api/v1/identidade/perfil` — só autenticado (sem RBAC de papel). Corpo:
`{ "telefone": "+244 9XX XXX XXX", "bi": "..." }` (ambos opcionais). O próprio actualiza
telefone/BI (campos locais), validados pelos VOs Angola (`identity.NovoTelefone` /
`identity.NovoBI`). Garante a linha local via JIT (a partir da sessão), persiste, audita
`identidade.perfil.actualizado`, e devolve o `Perfil` actualizado.

### D) Revogação de sessões na desactivação

O `CasoDefinirActivo` existente passa a, quando `activo=false`, revogar as sessões
Keycloak do utilizador (`POST /admin/realms/{realm}/users/:id/logout`) após aplicar o
estado — um utilizador desactivado deixa de poder renovar tokens. Audita a revogação.

### E) Compensação da criação não-atómica

No `AdminCliente.CriarUtilizador`, se uma atribuição de papel falhar após a criação do
utilizador, faz best-effort **apagar** o utilizador criado (`DELETE /users/:id`) antes de
devolver o erro — assim uma nova tentativa com o mesmo username não bate em 409. Fecha a
limitação registada na ADR-023.

### Portas e repositório

- **`AdminIdentidade`** (nova operações): `DefinirPasswordTemporaria(ctx, id, senha string) error`,
  `ResetOTP(ctx, id string) error`, `RevogarSessoes(ctx, id string) error`,
  `ApagarUtilizador(ctx, id string) error`.
- **`RepositorioUtilizadores`** (nova operação): `AtualizarContacto(ctx, keycloakID,
  telefone, bi string) error` (UPDATE dos campos locais; 0 linhas → `NaoEncontrado`).

> A porta `AdminIdentidade` cresce 4 métodos — todos os implementadores (o adaptador real
> `AdminCliente` e os fakes de teste) recebem os novos métodos/stubs.

## Componentes (ficheiros)

- **Domínio** `internal/domain/identidade/`:
  - `utilizador.go` — método `AtualizarContacto(telefone, bi string) error`.
  - `eventos.go` — `PasswordReposta`, `OtpReposto`, `PerfilActualizado`, `SessoesRevogadas`.
  - `repositorio.go` — `AtualizarContacto` na interface.
- **Aplicação** `internal/application/identidade/`:
  - `ports.go` — 4 métodos na porta `AdminIdentidade` + DTO `CredencialReposta`.
  - `reset_credenciais.go` (novo) — `CasoResetPassword`, `CasoResetOTP`.
  - `atualizar_perfil.go` (novo) — `CasoAtualizarPerfil`.
  - `gerir_utilizadores.go` — `CasoDefinirActivo` revoga sessões quando `activo=false`.
- **Adaptadores**:
  - `keycloak/admin.go` — `DefinirPasswordTemporaria`, `ResetOTP`, `RevogarSessoes`,
    `ApagarUtilizador` + compensação no `CriarUtilizador`.
  - `http/admin_handler.go` — handlers `resetPassword`/`resetOtp` + rotas (RBAC Admin);
    2 serviços novos no handler.
  - `http/identidade_handler.go` — handler `atualizarPerfil` + rota `PATCH /perfil`.
  - `pgrepo/identidade_repo.go` — implementar `AtualizarContacto`.
- **Plataforma** `internal/platform/app.go` — fiar os novos casos de uso.
- **i18n** — mensagens novas se necessário (ex.: `MsgContactoInvalido`).

Sem alterações de realm/infra (usa a Admin API já configurada).

## Tratamento de erros (RFC 7807, PT-PT)

| Situação | Categoria | HTTP |
|---|---|---|
| Reset/edição sobre utilizador inexistente | `CategoriaNaoEncontrado` | 404 |
| Telefone/BI inválido (edição de perfil) | `CategoriaValidacao` | 400 |
| Sessão sem papel Admin (reset) | `CategoriaProibido` | 403 |
| Keycloak indisponível | `CategoriaInterno` | 500 |

## Testes (gates: domínio ≥85%, aplicação ≥75%, adaptadores ≥60%)

- **Domínio** — `AtualizarContacto` (telefone/BI válidos e inválidos); eventos.
- **Aplicação** (fakes) — `CasoResetPassword` (senha gerada não-trivial + auditoria),
  `CasoResetOTP` (auditoria), `CasoAtualizarPerfil` (JIT + persistência + validação +
  auditoria), `CasoDefinirActivo` (revoga sessões **só** quando `activo=false`).
- **Adaptador keycloak** — httptest: `DefinirPasswordTemporaria`, `ResetOTP`,
  `RevogarSessoes`, `ApagarUtilizador`, e a compensação do `CriarUtilizador` (falha de
  atribuição → `ApagarUtilizador` chamado, erro propagado).
- **HTTP** — reset-password 200 `{senha_temporaria}`, reset-otp 204, RBAC Admin;
  `PATCH /perfil` 200 + validação 400.
- **Integração e2e** — reset password/OTP via Keycloak real; edição de perfil via BD real;
  desactivação revoga sessões (confirmar via Admin API que não há sessões activas).

## Documentação e config

- **ADR-024** (novo): reset de credenciais, edição self-service de perfil, revogação de
  sessões na desactivação, compensação da criação não-atómica.
- `SPRINT.md`: Sprint 5 → entregue; nota de encerramento da gestão de utilizadores M1.
- `CLAUDE.md`: registar ADR-024; próximo ADR → 025.

## Verificação (fim a fim)

1. `go build ./...`, `go vet`, `gofmt` limpos; `go test ./...` verde; cobertura 85/75/60.
2. Com o compose a correr: reset-password como Admin → 200 + nova senha; o utilizador
   autentica com a nova senha (forçado a mudar).
3. reset-otp como Admin → 204; o utilizador tem `CONFIGURE_TOTP` pendente.
4. `PATCH /perfil` com telefone/BI válidos → 200 + perfil actualizado; inválidos → 400.
5. Desactivar um utilizador → sessões revogadas (sem sessões activas na Admin API).
6. Auditoria registada para cada operação. Não-Admin nos resets → 403.

## Fora de âmbito (Sprint 6+ / M2)

- Gestão de sessões activas (listagem) e refresh-token flows.
- Auto-registo/self-service de novos utilizadores.
- Edição administrativa do perfil local de outros utilizadores.
- Notificações por email (requer SMTP).
