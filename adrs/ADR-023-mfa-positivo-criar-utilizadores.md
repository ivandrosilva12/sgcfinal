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
- `docker/keycloak/realm-sgc.json` é o realm de **desenvolvimento** e contém,
  deliberadamente, utilizadores de teste com credenciais conhecidas
  (`director.teste` com segredo TOTP conhecido, passwords `teste`). NÃO deve ser
  importado em produção — o pipeline de produção usa um realm separado sem estes
  fixtures.

## Limitações conhecidas (seguimento Sprint 5)

1. **Criação não-atómica.** `CriarUtilizador` cria o utilizador no Keycloak e depois
   atribui os papéis num ciclo; se uma atribuição de papel (ou o registo de
   auditoria subsequente) falhar a meio, o utilizador JÁ existe no Keycloak mas o
   chamador recebe erro, a senha temporária não é devolvida e uma nova tentativa
   com o mesmo username falha com 409. Para o M1 (volume baixo, o admin tem acesso
   directo ao Keycloak) é aceite; fica como seguimento no Sprint 5 uma compensação
   best-effort (limpeza em caso de falha parcial) e/ou devolver o id para a conta
   criada ser endereçável.
   **RESOLVIDO no Sprint 5 (ver ADR-024):** `CriarUtilizador` apaga best-effort o
   utilizador Keycloak se a atribuição de papéis falhar a meio, evitando o 409 em
   novas tentativas.
2. **Corpo da resposta contém `senha_temporaria`.** É intencional (devolvida uma
   única vez), mas qualquer middleware futuro que serialize/registe corpos de
   resposta não pode fazê-lo para esta rota. Nota para quem adicionar logging de
   respostas.
