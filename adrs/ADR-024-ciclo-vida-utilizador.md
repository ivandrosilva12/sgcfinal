# ADR-024 — Ciclo de vida do utilizador

- **Estado**: Aceite
- **Data**: 2026-07-12
- **Marco/Sprint**: M1 / Sprint 5
- **Contexto BC**: Identidade

## Contexto

Completar a gestão de utilizadores dos Sprints 3-4 com as operações de ciclo de vida
em falta.

## Decisão

1. **Reset de password (admin)**: gera nova senha temporária (`temporary:true`),
   devolvida uma única vez. Coerente com a criação (Sprint 4).
2. **Reset de OTP (admin)**: remove as credenciais OTP e exige `CONFIGURE_TOTP`
   (re-inscrição no próximo login).
3. **Edição de perfil (self-service)**: `PATCH /api/v1/identidade/perfil` — o próprio
   actualiza telefone/BI (campos locais, validados pelos VOs Angola). Campos omitidos
   preservam-se; vazios limpam.
4. **Revogação de sessões na desactivação**: ao desactivar, revogam-se as sessões
   Keycloak do utilizador (deixa de poder renovar tokens).
5. **Compensação da criação não-atómica**: se a atribuição de papel falhar após a
   criação, o utilizador criado é apagado (best-effort), fechando a limitação da
   ADR-023.

## Consequências

- A gestão de utilizadores fica completa para o M1; auditoria em todas as operações.
- Reset de password/OTP e revogação de sessões usam a Admin REST API (Keycloak fonte
  de verdade); o perfil telefone/BI vive na BD local.
- Alternativas descartadas: reset por email (sem SMTP em dev); edição administrativa
  do perfil de outros (mantém-se self-service — fica para Sprint 6+).
