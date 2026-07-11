# ADR-022 — MFA para papéis sensíveis e gestão administrativa via Keycloak

- **Estado**: Aceite
- **Data**: 2026-07-11
- **Marco/Sprint**: M1 / Sprint 3
- **Contexto BC**: Identidade

## Contexto

O Sprint 2 estabeleceu autenticação OIDC, RBAC e auditoria. Faltavam, para os
critérios de saída M1: imposição de MFA para papéis sensíveis e gestão
administrativa de utilizadores/papéis.

## Decisão

1. **Fonte de verdade dos papéis: Keycloak (Admin REST API).** Os endpoints de
   gestão escrevem no Keycloak via um client confidencial (`sgc-admin`,
   `client_credentials`). A BD local mantém-se espelho JIT.
2. **Imposição de MFA em dupla camada.** O realm exige OTP para papéis sensíveis;
   a API rejeita (403, `type: /erros/mfa-obrigatorio`) qualquer sessão com papel
   sensível cujo token não comprove segundo factor. A força de autenticação é
   derivada dos claims `amr` (métodos: otp/totp/mfa/hwk/sms/swk) ou `acr` (lista
   configurável `KEYCLOAK_ACR_FORTE`).
3. **Autorização dos endpoints de gestão.** Escrita: apenas `Admin`. Leitura:
   `Admin`, `Auditor`, `DPO` (auditoria/conformidade, menor privilégio).
4. **Adaptador HTTP puro** (`net/http`), sem nova dependência.

## Consequências

- Coerência com o Sprint 2 (Keycloak como fonte única); a atribuição de papel só
  reflecte no espelho local no próximo login/consulta do visado.
- O `admin.teste` sem OTP serve de caso negativo verificável nos smoke tests.
- Alternativas descartadas: gestão de papéis na BD local (divergiria do token);
  MFA só por configuração de realm (não testável na API); biblioteca gocloak
  (dependência desnecessária).

## Requisito operacional — configuração de MFA no Keycloak

- A imposição de MFA é **fail-closed**: um utilizador com papel sensível
  (Director, Admin, DPO, Auditor) cujo token não comprove segundo factor
  recebe 403 (`/erros/mfa-obrigatorio`).
- Para que um utilizador sensível autenticado com OTP seja reconhecido como
  autenticação forte, o realm Keycloak TEM de emitir no access token um claim
  `acr` cujo valor conste de `KEYCLOAK_ACR_FORTE` (por omissão `mfa,gold,2`),
  OU um claim `amr` com um método forte (otp/totp/mfa/hwk/sms/swk).
- Por omissão o Keycloak não emite `amr` e emite `acr="1"` para password
  simples. É por isso necessário configurar no realm: (1) um fluxo condicional
  de OTP exigido aos papéis sensíveis; e (2) um mapeamento ACR→LoA (atributo
  `acr.loa.map` do realm) que associe o nível autenticado com OTP a um `acr`
  presente em `KEYCLOAK_ACR_FORTE` (ex.: LoA 2 → "2"), OU um protocol mapper
  que emita `amr`.
- **Consequência**: enquanto esta configuração não existir, os papéis
  sensíveis ficam deliberadamente sem acesso (por segurança). O realm de
  desenvolvimento inclui `admin.teste` SEM OTP como caso negativo verificado.
  A validação do caminho positivo (utilizador com TOTP inscrito → acesso
  concedido) exige um utilizador com OTP inscrito e fica como passo de
  validação operacional, a completar quando o fluxo de OTP for provisionado
  (relacionado com o Sprint 4 — MFA para papéis sensíveis).
