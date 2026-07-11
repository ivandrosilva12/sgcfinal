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
