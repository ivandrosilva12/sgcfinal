# ADR-023 — MFA (caminho positivo) e criação de utilizadores

- **Estado**: Aceite
- **Data**: 2026-07-11
- **Marco/Sprint**: M1 / Sprint 4
- **Contexto BC**: Identidade

## Contexto

O Sprint 3 deixou o caminho positivo de MFA por validar e a gestão administrativa
sem criação de utilizadores. Este sprint fecha ambos.

## Decisão

1. **Sinal de MFA**: após spike, adoptou-se a variante **Variante A (acr via LoA)**. O
   realm emite `acr="2"` num login com OTP, que a API reconhece como autenticação
   forte (`KEYCLOAK_ACR_FORTE` / `amrFortes`). O realm inclui `director.teste` com OTP
   de segredo conhecido para validação e2e positiva.
2. **Criação de utilizadores**: `POST /api/v1/identidade/utilizadores` (RBAC Admin)
   cria no Keycloak (fonte de verdade) com senha temporária gerada (devolvida uma
   vez, `temporary:true`). Se algum papel inicial for sensível, adiciona a required
   action `CONFIGURE_TOTP` (coerente com o MFA fail-closed). Duplicados → 409.

## Consequências

- O critério de saída M1 de MFA fica totalmente fechado (positivo + negativo).
- A senha temporária é devolvida uma única vez; o admin comunica-a por canal seguro.
- Alternativas descartadas: envio por email (sem SMTP em dev); gestão de credenciais
  na BD local (Keycloak é a fonte de verdade).
