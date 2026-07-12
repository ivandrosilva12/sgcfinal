# ADR-025 — Sessões activas, edição administrativa de perfil e notificações

- **Estado**: Aceite
- **Data**: 2026-07-12
- **Marco/Sprint**: M1 / Sprint 6
- **Contexto BC**: Identidade

## Contexto

Os Sprints 3-5 deixaram três loose-ends por fechar no BC Identidade: (1) a Admin
REST API do Keycloak expõe sessões activas, mas o SGC não as lista nem permite
revogar uma sessão isolada (só a revogação em massa, na desactivação — ADR-024);
(2) a edição de perfil (telefone/BI) é apenas self-service (ADR-024), sem via
administrativa para um Admin corrigir o perfil de outro utilizador; (3) a senha
temporária (criação e reset — ADR-023/ADR-024) só é devolvida na resposta HTTP,
sem notificação por email, alternativa descartada em ambos por "sem SMTP em dev".
Este sprint fecha os três.

## Decisão

1. **Gestão de sessões activas.**
   `AdminIdentidade` ganha `ListarSessoes(ctx, userID)` e `RevogarSessao(ctx,
   sessionID)`, mapeados para `GET /admin/realms/{realm}/users/{id}/sessions` e
   `DELETE /admin/realms/{realm}/sessions/{sessionId}` da Admin API. Expostos como
   `GET /utilizadores/:id/sessoes` (leitura: Admin/Auditor/DPO, coerente com o RBAC
   de leitura já existente — ADR-022) e `DELETE /sessoes/:sid` (escrita: só Admin).
   A revogação é auditada (`identidade.sessao.revogada`). Mantém-se
   `RevogarSessoes` (plural, em massa) para a desactivação (ADR-024) — este ADR
   acrescenta a variante granular, não a substitui.

2. **Edição administrativa de perfil.**
   `CasoEditarPerfilAdmin` (`PATCH /utilizadores/:id/perfil`, só Admin) permite
   corrigir telefone/BI de **outro** utilizador, reutilizando os mesmos VOs Angola
   do self-service (ADR-024). Garante a linha local antes de escrever: se ainda não
   existir (utilizador nunca autenticado no SGC, só no Keycloak), hidrata-a
   just-in-time a partir do `AdminIdentidade.ObterUtilizador` (fonte de verdade de
   nome/email/papéis) antes de aplicar a alteração — o mesmo padrão JIT do
   `CasoObterPerfil` (ADR-021), aplicado agora também ao caminho administrativo.
   Campos omitidos preservam o valor actual; string vazia limpa; auditado
   (`identidade.perfil.actualizado`).

3. **Notificações por email best-effort.**
   `internal/adapters/smtp.NotificadorSMTP` implementa a porta `Notificador`
   (`application/identidade`) com `net/smtp` da stdlib — sem dependências novas.
   Envia a senha temporária por email na criação e no reset de password,
   complementando (não substituindo) a devolução na resposta HTTP: a senha
   continua a vir no corpo 1x (ADR-023/024) **e** é enviada por email, para que o
   admin não seja o único canal de distribuição. Quando `SMTP_HOST` está vazio, o
   composition root usa `NotificadorNulo` (fallback no-op): a operação prossegue
   sem erro e sem email — a notificação é sempre best-effort, nunca bloqueia a
   criação/reset. Em dev, o `docker-compose.yml` inclui o serviço `mailhog`
   (SMTP em `1025`, UI/API em `8025`) como destino de teste. A senha nunca é
   registada em logs — os pontos de log em torno do envio (sucesso/falha) omitem
   o corpo da mensagem.

## Consequências

- Fecha os loose-ends dos Sprints 3-5 no BC Identidade: sessões (visibilidade e
  revogação granular), perfil administrativo, e o canal de distribuição de senha
  temporária deixa de depender só da resposta HTTP.
- `AdminIdentidade` cresce para 12 métodos; todos os implementadores (adaptador
  real `keycloak.AdminCliente` e os fakes de teste) foram actualizados no mesmo
  commit em que a interface mudou.
- Falha de email nunca é falha de negócio: `NotificadorNulo` garante que
  ambientes sem SMTP configurado (ex.: alguns testes, ou uma clínica sem SMTP
  próprio) continuam a criar/repor utilizadores normalmente.
- Alternativas descartadas: tornar o email obrigatório (rejeitado — quebraria o
  fluxo em ambientes sem SMTP, contra o princípio best-effort); mover a senha
  temporária só para o email (rejeitado — perderia o canal síncrono já existente
  e criaria dependência rígida de SMTP disponível no momento da criação).
