# Spec — M1 / Sprint 3: MFA + Gestão administrativa de utilizadores/papéis

- **Data**: 2026-07-11
- **Marco**: M1 — Fundações
- **Bounded Context**: Identidade
- **ADR associada**: ADR-022 (a criar)
- **Precede**: plano de implementação (skill writing-plans)

## Contexto

O Sprint 2 entregou a fatia vertical base do BC Identidade: autenticação Keycloak
(JWT RS256 via go-oidc), RBAC pelos 11 papéis (DDM-001), auditoria de acesso, JIT
provisioning e `GET /api/v1/identidade/perfil`. Os papéis vêm do claim
`realm_access.roles` do token e são espelhados na BD local (`utilizadores_papeis`)
via JIT no login/consulta. O Keycloak é a fonte de verdade da autenticação e da
autorização.

O Sprint 3 completa os critérios de saída M1 relativos a autenticação/RBAC:

1. **Imposição de MFA** para papéis sensíveis (Director, Admin, DPO, Auditor).
2. **Gestão administrativa** de utilizadores e papéis (listar, ver, atribuir/revogar
   papel, activar/desactivar).
3. **Endpoints protegidos por papel** (aplicação efetiva do middleware `RBAC`).
4. **Smoke tests e2e de login**.

Backend/API apenas. PT-PT angolano em todo o output.

## Decisões fixadas (com o utilizador)

- **Fonte de verdade dos papéis**: **Keycloak (Admin REST API)**. Os endpoints de
  gestão escrevem no Keycloak; a BD local mantém-se espelho JIT (atualiza no próximo
  login/consulta do visado). Não se escreve diretamente em `utilizadores_papeis` a
  partir da gestão administrativa — evita divergência da fonte única.
- **Imposição de MFA**: **dupla camada**. O realm exige OTP para papéis sensíveis
  (fluxo condicional), E a API rejeita (403) qualquer sessão com papel sensível cujo
  token não comprove segundo fator (claims `acr`/`amr`). A parte imposta e testável
  no código é a verificação na API.
- **Operações de gestão**: listar, ver, atribuir/revogar papel, activar/desactivar.
- **Acesso à gestão**: **Admin** faz escrita; **Auditor** e **DPO** têm leitura
  (listar/ver) para auditoria/conformidade. Restantes papéis não acedem.
- **Cliente Admin do Keycloak**: HTTP puro (`net/http` + `encoding/json`), sem nova
  dependência (coerente com o ethos "deps mínimas / SQL puro").

## Arquitetura e fluxo

Duas capacidades, ambas respeitando a regra de dependência (Domínio → Aplicação →
Adaptadores → Plataforma).

### A) Imposição de MFA (transversal)

- A `Sessao` passa a carregar `AutenticacaoForte bool`, derivada dos claims `acr`/`amr`
  do token pela camada de adaptadores (keycloak).
- Middleware `MFAObrigatoria()` aplicado ao grupo protegido logo a seguir ao `Auth`:
  rejeita com **403 (tipo `mfa-obrigatorio`)** qualquer sessão que tenha papel
  sensível mas cujo token não comprove segundo fator.
- As regras "que papéis exigem MFA" e "que valores contam como autenticação forte"
  vivem no domínio (funções puras) — testáveis sem infra.

### B) Gestão administrativa (fatia vertical nova)

- Endpoints sob `/api/v1/identidade/utilizadores` que falam com a **Admin REST API do
  Keycloak** (novo adaptador `keycloak/admin.go`, cliente confidencial via
  `client_credentials`, com cache/refresh do token de serviço).
- Cada operação de escrita é **auditada** (reutiliza `auditoria.auditoria_eventos`).
- O espelho local reflete no próximo login/consulta do visado; a gestão não escreve
  em `utilizadores_papeis`.

### Fluxo exemplo — atribuir papel

```
POST /api/v1/identidade/utilizadores/:id/papeis
  → LimiteTaxa → Auth → MFAObrigatoria → RBAC(Admin)
  → admin_handler.atribuirPapel
  → CasoAtribuirPapel (valida PapelValido; porta AdminIdentidade.AtribuirPapel;
                        Auditor.Registar "identidade.papel.atribuido")
  → 204 No Content
```

## Componentes (ficheiros)

### Domínio — `internal/domain/identidade/`
- `sessao.go` (modificar): campo `AutenticacaoForte bool`.
- `mfa.go` (novo): `ExigeAutenticacaoForte(papeis []Papel) bool` (true se algum
  `EhSensivel`); `VerificarAutenticacaoForte(s Sessao) error` →
  `erros.Novo(CategoriaMFAObrigatorio, …)` se exige mas a sessão não a tem.
- `eventos.go` (modificar): `PapelAtribuido`, `PapelRevogado`, `UtilizadorActivado`,
  `UtilizadorDesactivado` (implementam `shared/evento.EventoDominio`).

### Shared — `internal/domain/shared/erros/erros.go`
- Nova categoria `CategoriaMFAObrigatorio` (mapeada a 403, com `type` RFC 7807
  distinto de `CategoriaProibido`).

### Aplicação — `internal/application/identidade/`
- `ports.go` (modificar): nova porta de saída
  `AdminIdentidade { ListarUtilizadores(ctx, filtro) ([]ResumoUtilizador, error);
   ObterUtilizador(ctx, id) (DetalheUtilizador, error);
   AtribuirPapel(ctx, id, Papel) error; RevogarPapel(ctx, id, Papel) error;
   DefinirActivo(ctx, id, bool) error }`.
- `gerir_utilizadores.go` (novo): casos de uso `CasoListarUtilizadores`,
  `CasoObterUtilizador`, `CasoAtribuirPapel`, `CasoRevogarPapel`, `CasoDefinirActivo`.
  As escritas validam papel (`PapelValido`) e auditam via `Auditor`.
- DTOs `ResumoUtilizador` (id, nome, email, activo, papéis) e `DetalheUtilizador`.

### Adaptadores
- `keycloak/admin.go` (novo): `AdminCliente` implementa `AdminIdentidade` via Admin
  REST API. Token `client_credentials` com cache e refresh preemptivo; chamadas
  `GET /admin/realms/{realm}/users` (+ query), role-mappings realm
  (`GET`/`POST`/`DELETE /admin/realms/{realm}/users/{id}/role-mappings/realm`),
  `PUT /admin/realms/{realm}/users/{id}` (enabled). HTTP puro.
- `http/admin_handler.go` (novo): `RegistarAdministracao(r, h, mws...)`; rotas
  `GET /utilizadores`, `GET /utilizadores/:id`, `POST /utilizadores/:id/papeis`,
  `DELETE /utilizadores/:id/papeis/:papel`, `PATCH /utilizadores/:id`. Escrita:
  `RBAC(Admin)`; leitura: `RBAC(Admin, Auditor, DPO)`.
- `http/middleware_auth.go` (modificar): `MFAObrigatoria()`.
- `http/problem.go` (modificar): mapear `CategoriaMFAObrigatorio` → 403 + `type`
  próprio (`/erros/mfa-obrigatorio`).

### Plataforma — `internal/platform/`
- `config/config.go`: `KEYCLOAK_ADMIN_CLIENT_ID` e `KEYCLOAK_ADMIN_CLIENT_SECRET`
  (obrigatórios); base do Keycloak e realm derivados de `KEYCLOAK_ISSUER`;
  `KEYCLOAK_ACR_FORTE` (lista de valores `acr` considerados fortes, com default).
- `app.go`: fiar `AdminCliente`, casos de uso de gestão, `admin_handler` e o
  middleware `MFAObrigatoria` no grupo protegido.
- i18n: novas mensagens (`MsgMFAObrigatoria`, `MsgPapelInvalido`,
  `MsgUtilizadorNaoEncontrado`).

### Infra — `docker/keycloak/realm-sgc.json`
- Cliente confidencial `sgc-admin` com service account e roles de
  `realm-management`: `view-users`, `manage-users`, `query-users`.
- Fluxo condicional de OTP para papéis sensíveis + mapeamento `acr`→LoA para o token
  refletir a força de autenticação.

Sem novas migrations (reutiliza `identidade.utilizadores` e
`auditoria.auditoria_eventos`).

## Tratamento de erros (RFC 7807, PT-PT)

| Situação | Categoria | HTTP | type |
|---|---|---|---|
| Papel inexistente ao atribuir | `CategoriaValidacao` | 400 | /erros/validacao |
| Papel sensível sem 2º fator | `CategoriaMFAObrigatorio` | 403 | /erros/mfa-obrigatorio |
| Sessão sem papel Admin (escrita) | `CategoriaProibido` | 403 | /erros/proibido |
| Utilizador inexistente no Keycloak | `CategoriaNaoEncontrado` | 404 | /erros/nao-encontrado |
| Admin API do Keycloak indisponível | `CategoriaInterno` | 500 | /erros/interno |

Mensagens em PT-PT; nunca vazar detalhes internos ao cliente.

## Testes (gates: domínio ≥85%, aplicação ≥75%, adaptadores ≥60%)

- **Domínio** `mfa_test.go`: `ExigeAutenticacaoForte` (sensível vs não sensível);
  `VerificarAutenticacaoForte` (permite/nega). Eventos.
- **Aplicação** `gerir_utilizadores_test.go`: fakes de `AdminIdentidade` e `Auditor`;
  caminho feliz de cada caso de uso, validação de papel e asserção do evento auditado.
- **Adaptadores HTTP** `admin_handler_test.go` (httptest + fakes): matriz RBAC (Admin
  200; Auditor lê / não escreve 403; papel comum 403); `MFAObrigatoria` (papel
  sensível sem MFA → 403; com MFA → segue; papel comum → segue).
- **Keycloak admin** teste interno: construção de URLs e parsing de respostas (sem
  rede).
- **Integração e2e** (`tests/integration`, tag `integration`) — **smoke tests de
  login**: utilizador comum → 200; Admin sem OTP → 403 (MFA); sem token → 401; papel
  errado → 403. Fluxo admin: atribuir papel via Admin API e confirmar via
  `GET /utilizadores/:id`.

## Documentação e config

- **ADR-022** (novo): gestão via Admin REST API (client_credentials), imposição de MFA
  por `acr`/`amr`, mapeamento de erros.
- `SPRINT.md`: Sprint 3 → entregue; próximas sprints atualizadas.
- `CLAUDE.md`: registar ADR-022; próximo ADR → 023.
- `.env.example`: `KEYCLOAK_ADMIN_CLIENT_ID`, `KEYCLOAK_ADMIN_CLIENT_SECRET`,
  `KEYCLOAK_ACR_FORTE`.
- `.go-arch-lint.yml`: sem vendor novo (HTTP puro no adaptador admin).

## Verificação (fim a fim)

1. `go build ./...`, `go vet`, `gofmt` limpos.
2. `make test` (unit+aplicação, `-race`) verde; cobertura cumpre 85/75/60.
3. Com o compose a correr: token de utilizador comum → `GET /perfil` 200; token de
   Admin sem OTP → qualquer rota protegida devolve 403 MFA.
4. `POST /utilizadores/:id/papeis` como Admin → 204 + linha em `auditoria_eventos`
   (`identidade.papel.atribuido`); `GET /utilizadores/:id` confirma o papel.
5. Auditor: `GET /utilizadores` 200; `POST .../papeis` 403.
6. `PATCH /utilizadores/:id` (desactivar) → conta `enabled=false` no Keycloak.

## Fora de âmbito (Sprint 4+)

- Auto-provisionamento/registo de novos utilizadores pela API (criação de conta no
  Keycloak).
- Fluxos de reset de OTP/password geridos pela API.
- Refresh-token flows e gestão de sessões ativas.
