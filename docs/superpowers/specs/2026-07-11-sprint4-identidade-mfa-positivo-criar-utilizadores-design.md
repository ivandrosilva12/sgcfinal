# Spec — M1 / Sprint 4: MFA (caminho positivo) + Criação de utilizadores

- **Data**: 2026-07-11
- **Marco**: M1 — Fundações
- **Bounded Context**: Identidade
- **ADR associada**: ADR-023 (a criar)
- **Precede**: plano de implementação (skill writing-plans)

## Contexto

O Sprint 3 entregou a imposição de MFA (fail-closed) e a gestão administrativa
(listar/ver/atribuir/revogar papel, activar/desactivar) via Admin REST API do Keycloak.
A revisão final apontou um risco funcional em aberto: o **caminho positivo de MFA** não
está validado — o realm de desenvolvimento não emite ainda um `acr`/`amr` que a API
reconheça como 2º factor, pelo que um utilizador com papel sensível autenticado com OTP
poderia continuar sem acesso. Além disso, a gestão administrativa não permite **criar**
novos utilizadores.

O Sprint 4 fecha ambos, em duas partes coesas, ambas no BC Identidade.

Backend/API apenas. PT-PT angolano em todo o output.

## Decisões fixadas (com o utilizador)

- **Foco do Sprint 4**: fechar o caminho positivo de MFA **e** criação administrativa de
  utilizadores.
- **Sinal de MFA (caminho positivo)**: **decidir num spike** — testar `acr` (via
  `acr.loa.map` + OTP condicional) vs `amr` (via script mapper) contra o *direct grant*,
  e adoptar o que emitir de forma fiável um sinal em `KEYCLOAK_ACR_FORTE`/`amr`.
- **Credencial inicial**: a API gera uma **senha temporária** (`temporary:true`, muda no
  1º login) e devolve-a **uma única vez** na resposta 201.
- **OTP na criação**: se algum papel inicial for sensível (Director/Admin/DPO/Auditor), a
  API adiciona automaticamente a required action `CONFIGURE_TOTP` (coerente com o MFA
  fail-closed — evita criar um utilizador sensível que ficaria logo sem acesso).

## Parte A — Fechar o caminho positivo de MFA

Sem código novo de domínio/aplicação: a lógica `ehAutenticacaoForte` (Sprint 3) já
reconhece 2º factor por `acr` **ou** `amr`. Esta parte é **configuração + validação**.

### Passo 1 — Spike (time-boxed)

Um teste de integração exploratório (`//go:build integration`, fora do gate) que configura
no realm duas variantes e, via *direct grant* com OTP, inspecciona os claims do access
token:

- **Variante acr-LoA**: `acr.loa.map` no realm (ex.: `{"otp":"2"}`), OTP condicional no
  fluxo *Direct Grant*; verificar se o token traz `acr` num valor de `KEYCLOAK_ACR_FORTE`.
- **Variante amr-script**: feature `scripts` do Keycloak ligada + script protocol mapper
  que emite `amr:["otp"]` quando a credencial OTP é usada.

Regista qual emite fiavelmente o sinal. **Decisão fixada no spike**, documentada na
ADR-023. Se a escolha for amr-script, o compose do Keycloak passa a incluir
`--features=scripts`.

### Passo 2 — Fixar config no realm

Aplica a variante vencedora em `docker/keycloak/realm-sgc.json`: OTP condicional para
papéis sensíveis + o mapeamento/mapper que produz o claim forte. Inscreve `director.teste`
(papel Director) com password `teste` e uma credencial **OTP de segredo TOTP conhecido**
(semeado no realm import), para permitir a validação positiva automatizada.

### Passo 3 — Teste e2e positivo

Em `tests/integration`: computa o código TOTP corrente a partir do segredo conhecido
(algoritmo TOTP em Go puro — `crypto/hmac` + `crypto/sha1`, sem dependência nova), faz
*direct grant* `username=director.teste&password=teste&totp=<código>`, e verifica:
`verificador.Verificar(token).AutenticacaoForte == true` e
`VerificarAutenticacaoForte(sessao) == nil` (acesso concedido). Complementa o caso negativo
(`admin.teste` sem OTP) já existente no Sprint 3.

Se o spike revelar que `amrFortes`/`acrFortes` precisa de um valor específico, isso é uma
pequena alteração ao adaptador `keycloak` e/ou a `KEYCLOAK_ACR_FORTE`.

## Parte B — Criação administrativa de utilizadores

### Contrato

`POST /api/v1/identidade/utilizadores` — `RBAC(Admin)`, sob os mesmos middlewares do grupo
protegido (rate limit + Auth + MFAObrigatoria).

Pedido (JSON):

```json
{ "username": "ana.silva", "nome": "Ana Silva", "email": "ana@sgc.ao", "papeis": ["Medico"] }
```

- `username`, `nome`, `email` obrigatórios; `email` validado (`net/mail`); `papeis`
  opcional, cada um validado por `PapelValido` (desconhecido → 400).
- `nome` é dividido em firstName (1º termo) + lastName (resto) para o Keycloak.
- `telefone`/`BI` ficam **fora** da criação (campos de perfil local, preenchidos depois).

Resposta 201:

```json
{ "id": "uuid-keycloak", "senha_temporaria": "<gerada>" }
```

A senha temporária é devolvida **uma única vez**.

### Componentes

- **Domínio** `internal/domain/identidade/` — evento `UtilizadorCriado`; reutiliza
  `ExigeAutenticacaoForte(papeis)` para decidir o `CONFIGURE_TOTP`.
- **Aplicação** `internal/application/identidade/` — nova porta
  `AdminIdentidade.CriarUtilizador(ctx, dados CriacaoUtilizador) (id string, err error)`;
  DTOs `CriacaoUtilizador` (username, nome, email, papéis, configurarOTP, senhaTemporaria)
  e `UtilizadorCriado` (id, senhaTemporaria). Caso de uso `CasoCriarUtilizador`: valida
  entrada, **gera senha temporária** (`crypto/rand`, sem dependência nova), calcula
  `configurarOTP := dominio.ExigeAutenticacaoForte(papeis)`, chama o adaptador, **audita**
  `identidade.utilizador.criado`, devolve `{id, senhaTemporaria}`.
- **Adaptador** `internal/adapters/keycloak/admin.go` — `CriarUtilizador`: `POST
  /admin/realms/{realm}/users` com a representação (username, firstName, lastName, email,
  `enabled:true`, `credentials:[{type:"password", value:<temp>, temporary:true}]`, e
  `requiredActions:["CONFIGURE_TOTP"]` quando sensível); lê o `id` do cabeçalho `Location`;
  atribui os papéis (reutiliza a lógica de role-mappings). `409` do Keycloak → mapeado a
  `erros.CategoriaConflito` (novo tratamento no helper `pedir` ou no método).
- **HTTP** `internal/adapters/http/admin_handler.go` — handler `criarUtilizador` +
  registo da rota `POST ""` no grupo, com `RBAC(Admin)`.

## Tratamento de erros (RFC 7807, PT-PT)

| Situação | Categoria | HTTP |
|---|---|---|
| Entrada inválida (campos em falta, email malformado, papel desconhecido) | `CategoriaValidacao` | 400 |
| Username/email já existe | `CategoriaConflito` | 409 |
| Sessão sem papel Admin | `CategoriaProibido` | 403 |
| Keycloak indisponível | `CategoriaInterno` | 500 |

Mensagens em PT-PT; nunca vazar detalhes internos.

## Testes (gates: domínio ≥85%, aplicação ≥75%, adaptadores ≥60%)

- **Aplicação** — `CasoCriarUtilizador` com fakes: validação de entrada; senha temporária
  gerada não-trivial; `configurarOTP` correcto (true p/ papel sensível, false p/ comum);
  auditoria `identidade.utilizador.criado`; propagação de 409.
- **Adaptador keycloak** — httptest (estende `admin_httptest_test.go`): `POST /users` →
  201 + `Location` → id extraído; atribuição de papéis a seguir; `409` → `CategoriaConflito`;
  `requiredActions` presente quando sensível.
- **HTTP** — handler: 201 com `{id, senha_temporaria}`; RBAC (não-Admin → 403); corpo
  inválido → 400.
- **Integração e2e** — Parte A: `director.teste` + TOTP computado → `AutenticacaoForte==true`
  (caminho positivo). Parte B: criar utilizador (username único), confirmar via
  `ObterUtilizador`, e **limpar** (DELETE via Admin API no fim, para o realm ficar como
  estava).

## Documentação e config

- **ADR-023** (novo): decisão do sinal de MFA (spike) + decisões da criação (senha devolvida
  1x, `CONFIGURE_TOTP` em papéis sensíveis).
- `SPRINT.md`: Sprint 4 → entregue; critérios de saída M1 do MFA totalmente fechados.
- `CLAUDE.md`: registar ADR-023; próximo ADR → 024.
- `.env.example`: eventuais notas (ex.: feature `scripts` se a variante amr for escolhida).
- `realm-sgc.json`: config de MFA + `director.teste` com OTP conhecido.
- `docker-compose.yml`: `--features=scripts` no Keycloak apenas se a variante amr vencer.

## Verificação (fim a fim)

1. `go build ./...`, `go vet`, `gofmt` limpos; `go test ./...` verde; cobertura 85/75/60.
2. Spike concluído e decisão registada na ADR-023.
3. Com o compose a correr: `director.teste` + TOTP → rota protegida **200** (2º factor
   reconhecido); `admin.teste` sem OTP → **403** (negativo, regressão).
4. `POST /utilizadores` como Admin com papel sensível → 201 + senha temporária + o
   utilizador criado tem `CONFIGURE_TOTP`; papel comum → 201 sem `CONFIGURE_TOTP`.
5. Criar com username existente → 409. Criar como não-Admin → 403.
6. Auditoria `identidade.utilizador.criado` registada.

## Fora de âmbito (Sprint 5+)

- Reset de OTP/password pela API (admin despoleta reset).
- Edição de perfil (telefone/BI) e desactivação com revogação de sessões.
- Gestão de sessões activas e refresh-token flows.
- Auto-registo/self-service pelo próprio utilizador.
