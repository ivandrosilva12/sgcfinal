# ADR-021 — Identidade: verificação OIDC, RBAC e provisionamento JIT

- **Estado**: Aceite
- **Data**: 2026-07-11
- **Marco**: M1 — Fundações (Sprint 2 — fatia vertical do BC Identidade)
- **Contexto ADRs**: sucede a ADR-020.

## Contexto

O Sprint 2 implementa a primeira fatia vertical completa (domínio → HTTP): autenticação
Keycloak, autorização RBAC pelos 11 papéis, auditoria de acesso e o endpoint
`GET /api/v1/identidade/perfil`. As decisões de implementação abaixo não estavam fixadas
ao nível de código pelos ADRs do blueprint.

## Decisões

### 1. Biblioteca de verificação OIDC: go-oidc (coreos)

- A validação de JWT RS256 (assinatura, issuer, expiração) e o cache de JWKS usam
  **`github.com/coreos/go-oidc/v3`** (via OIDC discovery no `KEYCLOAK_ISSUER`).
- **Alternativas rejeitadas**: `golang-jwt` + JWKS manual (mais código e superfície de erro
  na rotação de chaves); `lestrrat-go/jwx` (API mais verbosa, dependência maior).
- Registada como vendor `oidc` em `.go-arch-lint.yml` e autorizada apenas na camada de
  adaptadores.

### 2. Validação de audience por `aud` OU `azp`

- O client `sgc-api` é **público** (direct access grants); nos tokens do Keycloak o client
  surge em `azp` e o `aud` costuma ser `account`. Em vez de exigir um *audience mapper* no
  realm, o verificador aceita o token se a audiência esperada constar de `aud` **ou** de
  `azp`. Confirmado em teste de integração com token real.
- **Alternativa**: adicionar um protocol mapper de audience ao realm (mais configuração; não
  necessária).

### 3. Provisionamento Just-In-Time (JIT)

- No primeiro acesso, o perfil local (`identidade.utilizadores` + `utilizadores_papeis`) é
  criado/actualizado a partir dos claims do token (upsert). O **Keycloak é a fonte de
  verdade** de nome/email/papéis; o upsert **preserva** `telefone`/`bi` definidos localmente.
- Os 11 papéis são semeados por migration (`identidade/0004_seed_papeis.sql`) — dados de
  referência necessários à FK de `utilizadores_papeis`.
- **Alternativa rejeitada**: exigir pré-provisionamento (403 para utilizador desconhecido) —
  friccional e sem fluxo administrativo nesta fase.

### 4. Auditoria no acesso ao recurso, não por pedido

- A verificação do token corre por pedido (middleware, sem I/O de BD). O **provisionamento
  JIT e o registo de auditoria** (`identidade.perfil.consultado`) ocorrem no caso de uso de
  negócio (`ObterPerfil`), evitando inundar o audit log com um registo por pedido HTTP.

### 5. `/perfil` é só-autenticado; RBAC guarda endpoints por papel

- O perfil do próprio utilizador não é restringido por papel (qualquer utilizador
  autenticado o consulta). O middleware `RBAC` existe e é testado (401/403/200) e guardará
  endpoints específicos por papel nos próximos sprints.

### 6. Rate limiting por janela fixa em Redis; fail-open

- `LimitadorTaxa` usa `INCR`+`EXPIRE` (sem novas dependências). Em falha do Redis, o
  middleware **deixa passar** (fail-open) para não derrubar o serviço por indisponibilidade
  da cache. Excedido → 429 + `Retry-After`.

### 7. `KEYCLOAK_ISSUER` obrigatório

- Passa a ser configuração obrigatória (validada no arranque). Nota operacional: em Docker o
  issuer é `http://keycloak:8080/realms/sgc`; tokens têm de ser emitidos pelo mesmo issuer
  que a API valida (mismatch localhost:8081 vs keycloak:8080 se cruzados).

## Consequências

- API autenticada e auditável, com RBAC pronto a aplicar por endpoint.
- Cobertura: domínio 98%, aplicação 95%, adaptadores 61% (repos cobertos por integração).
- Erros uniformes em `application/problem+json` (RFC 7807) e mensagens pt-AO.
- O endpoint `/readyz` passa a verificar também o Keycloak (JWKS/discovery).
