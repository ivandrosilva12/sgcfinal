# SPRINT ACTUAL

- **Marco**: M4 — Financeiro — **em curso**
- **Sprint**: 16 (Financeiro — selagem canónica: enquadramento injectivo e âmbito do
  selo) — **entregue**
- **Objectivo**: tornar o canónico do hash da `Factura` **injectivo** e alargar o
  âmbito do selo, pagando as dívidas R1 e R2 da ADR-040 dentro do prazo que ela
  própria fixou — revisíveis apenas **antes da primeira emissão em produção**. A
  anulação, o SAF-T-AO e a certificação AGT ficam fora e **não estão feitas**.

## Sprint 16 — entregue

- [x] **Enquadramento injectivo** (`enquadrar` em
      `internal/domain/financeiro/factura.go`): `{len em bytes}:{s}` em **todo** o
      campo de texto do canónico e do digest das linhas, sem excepções, inteiros nus.
      A regra é deliberadamente cega — o defeito nasceu de se ter julgado quais os
      campos eram seguros e de esse juízo estar errado.
- [x] **Colisão do R1 confirmada contra o código real** e convertida em teste de
      regressão permanente (`TestHash_DescricaoNaoImitaFronteiraDeLinha`): antes da
      fatia, uma factura de 2 linhas e outra de 1 linha partilhavam hash
      (`cac4fec4…`) e total. Os totais não bastavam porque a CHECK
      `preco_unitario_centimos >= 0` admite linhas a preço zero.
- [x] **Selo alargado**: `clienteNome`, `clienteMorada` (identidade do destinatário —
      numa factura a consumidor final sem NIF, era a string vazia que ia selada),
      `operacaoID` (proveniência da linha) e `episodioID` (proveniência da factura).
      Um teste por campo recém-selado, cada um a falhar antes e a passar depois.
- [x] **`ItemFactura.ID` deliberadamente fora do selo** — chave substituta sem
      significado fiscal; selá-la ataria o documento fiscal a um detalhe de
      implementação da BD. Registado como decisão, não como omissão.
- [x] **Canónico com 12 campos** e **vector dourado novo**
      (`7c99e3dbc895f04e3e40d4114dea8f5129e10297de33a25222d9dcc401c796da`), calculado
      contra a implementação real e conferido por **reimplementação independente em
      Python derivada da regra normativa escrita**, não do código Go.
- [x] **Injectividade confirmada por enumeração**: força bruta sobre os 4 campos de
      texto adjacentes (12⁴ = 20 736 facturas logicamente distintas) → 20 736
      canónicos distintos, **zero colisões**.
- [x] **Série por corrida** no teste de integração da cadeia
      (`tests/integration/facturas_test.go`): deixa de listar a série `2999`, que
      acumula facturas EMITIDA irremovíveis com elos em formato antigo. Sem isto a
      mudança de formato deixaria a suite vermelha.
- [x] **Fatia puramente de domínio: zero migrações** — `cliente_nome`,
      `cliente_morada`, `operacao_id` e `episodio_id` já eram colunas persistidas.
- [x] ADR-041, e R1/R2 da ADR-040 marcados como resolvidos **aditivamente** (o texto
      original fica: a avaliação de risco lá escrita ter estado errada é matéria de
      auditoria).

## Sprint 15 — entregue

- [x] VO `NumeroFactura` (`internal/domain/financeiro/numero.go`): formato legal
      `FAC <série>/<8 dígitos>` (DDM-001 v2.0 §5.2.1), `SerieDe` = ano civil UTC,
      rejeição explícita de sequencial acima dos 8 dígitos (série esgotada).
- [x] `Factura.Emitir(serie, sequencial, hashAnterior, momento)` com o **hash SHA-256
      canónico calculado no agregado** — invariante do domínio, nunca de um serviço —
      e vector dourado (`TestHash_VectorDourado`) a travar a canonicalização.
- [x] `VerificarCadeia` (`internal/domain/financeiro/cadeia.go`) como função pura:
      detecta buraco na numeração, elo anterior errado e conteúdo adulterado,
      devolvendo o primeiro problema encontrado.
- [x] Migração `financeiro/0002_emissao_facturas.sql`: colunas de emissão, índices
      únicos parciais de `numero` e de `(serie, sequencial)`, CHECK de coerência
      estado↔emissão, tabela `financeiro.series` e **triggers de imutabilidade** em
      `facturas` e `itens_factura`.
- [x] **Bloqueio optimista** (coluna `versao`) no `Guardar` e no `Emitir` — fecha a
      dívida técnica que o ADR-039 assumiu, provada por teste de lost-update.
- [x] `RepositorioFacturas.Emitir` com alocação **serializada** por
      `SELECT ... FOR UPDATE` na linha da série; numeração sem buracos provada com 12
      emissões concorrentes na mesma série.
- [x] Casos de uso `CasoEmitirFactura` (auditado com número legal e hash no detalhe) e
      `CasoVerificarCadeia` — cadeia quebrada é **resultado** (`integra:false`, 200),
      não erro de execução.
- [x] Rotas `POST /api/v1/financeiro/facturas/:fid/emitir` e
      `GET /api/v1/financeiro/facturas/cadeia/verificacao` + composition root.
- [x] `PapelTesoureiro` passa a **sensível** (5 sensíveis em 12 papéis) e
      `MFAObrigatoria()` passa a ser imposta nas rotas do Financeiro — o que corrige
      também o bypass anterior de Director e Auditor. Migração
      `identidade/0006_papel_tesoureiro_sensivel.sql`, seed, realm Keycloak
      (`tesoureiro.teste` com OTP) e ERRATA-002 alterada **aditivamente**.
- [x] ADR-040, com a dívida sistémica de MFA (R3), as duas decisões conscientes sobre
      o conteúdo do digest (R1, R2) e as restrições impostas ao ADR-041 (R5).

## Sprint 14 — entregue

- [x] 12.º papel RBAC `Tesoureiro` (ERRATA-002): não-sensível nesta fatia (sem
      MFA — decisão a rever no ADR-040), seed idempotente e realm role Keycloak.
- [x] Agregado `Factura` (RASCUNHO) + `ItemFactura` + `ClienteSnapshot`: tipos de
      linha (CONSULTA/DISPENSA/EXAME_ANALISE/ESTUDO_IMAGEM/PROCEDIMENTO_CIRURGICO)
      com snapshot de descrição/preço e id lógico da operação de origem, sem FK
      cross-context.
- [x] IVA por item (ISENTO/STANDARD 14%), arredondamento meia-acima por linha,
      total autoritário calculado no domínio (nunca em SQL).
- [x] Persistência `financeiro/0001_facturas.sql` (schema + `facturas`/
      `itens_factura`, FK intra-BC permitida) e `RepositorioFacturas` pgx com
      upsert transaccional (reescrita de linhas por substituição).
- [x] Casos de uso auditados (`financeiro.factura.*`) e HTTP+RBAC (escrita
      Tesoureiro; leitura Tesoureiro/Director/Auditor).
- [x] ADR-039.

## Sprint 13 — entregue

- [x] Transição `Validar` (PROCESSADA → VALIDADA) com segregação de funções
      (`actor != tecnicoSubmissorID`, 422 `RegraNegocio`) no domínio
      (`internal/domain/laboratorio/resultado.go`).
- [x] Avaliação de valor crítico no domínio (`Analise.AvaliarCritico`,
      `internal/domain/laboratorio/analise.go`), chamada na validação e reavaliada na
      correcção; valor não numérico nunca é crítico.
- [x] Notificação SMS ao médico requisitante best-effort e sempre auditada
      (`laboratorio.valor_critico.notificado`), via `ResolvedorContacto` (ACL sobre a
      Identidade) e `NotificadorCritico` (gateway HTTP ou `NotificadorNulo`); falha de
      envio não reverte a validação nem a correcção.
- [x] Correcção (`Resultado.Corrigir`) arquiva o original em CONCLUIDA e cria um novo
      resultado VALIDADA (`corrige_resultado_id → original`), preservando a
      proveniência; transacção única no `pgrepo` (CAS + INSERT); segregação de
      funções também na correcção.
- [x] Visibilidade clínica reduzida a `EstadosVisiveisAoMedico = {VALIDADA}` — o
      arquivado sai da leitura normal do episódio; a fila de trabalho mantém-se
      fail-open para o histórico de correcção.
- [x] CHECK de segregação da BD (`resultados_check4`) provada por teste de
      integração real (SQLSTATE 23514), defesa em profundidade da regra de domínio.
- [x] ADR-035.

## Sprint 12 — entregue

- [x] Catálogo de análises (intervalos de referência, valores críticos), com os 5
      registos de referência (HB, HEMOG, GLIC, CREAT, UREIA) semeados na própria
      migração (`migrations/laboratorio/0001_catalogo_analises.sql`).
- [x] Requisição no BC Laboratório via ACL sobre o Clínico (doente activo, episódio
      aberto); um resultado PENDENTE por análise, em transacção única.
- [x] Colheita, recusa (motivo obrigatório) e submissão do preliminar; o técnico
      gravado é sempre o sujeito autenticado.
- [x] O preliminar **não** é visível ao médico: a leitura clínica filtra por
      VALIDADA/CONCLUIDA na aplicação.
- [x] Guarda compare-and-set nas transições; CHECK de coerência estado↔timestamps.
- [x] ADR-031.

## Sprint 11 — entregue

- [x] Agregado `Consentimento` (LPDP): 5 finalidades, registar/revogar/listar/obter,
      escritas auditadas. Migration `clinico/0003_consentimentos.sql`.
- [x] Invariante-estrela: um procedimento exige consentimento de finalidade CIRURGIA,
      concedido e com anexo — verificada no agendamento e **revalidada no início**
      (o doente pode revogar e o episódio pode fechar entretanto).
- [x] Agregado `ProcedimentoCirurgico` com state machine AGENDADO → EM_CURSO →
      CONCLUIDO/CANCELADO (cancelamento DDM-estrito, só intra-operatório), VO
      `Anestesia` e tipo de episódio `CIRURGIA_AMBULATORIA`.
- [x] Catálogo de procedimentos (read model, seed PRC001–PRC007), repositórios pgx,
      handlers HTTP com RBAC e testes de integração contra Postgres.
- [x] Guardas de concorrência: compare-and-set nos UPDATE de transição e de revogação.
- [x] ADR-030.

## Sprint 10 — entregue

- [x] BC Farmácia — stock: agregados `Fornecedor` e `Lote`, movimentos de stock
      append-only (ADR-017), migration própria.
- [x] Motor de dispensa transaccional com alocação FEFO, validação de alergias e
      serialização da dispensa da mesma receita (não-exceder revalidado na transacção).
- [x] Repositórios pgx, handler HTTP com RBAC e testes de integração.
- [x] ADR-029.

## Sprint 9 — entregue

- [x] BC Farmácia — receita: agregados `Medicamento` (catálogo) e `Receita` (itens,
      estado, eventos), casos de uso com validação de alergias e override justificado.
- [x] Adaptador `LeitorClinico` (camada anti-corrupção sobre o BC Clínico).
- [x] Categoria de erro `RegraNegocio` no Shared Kernel, mapeada para 422.
- [x] ADR-028.

## Sprint 8 — entregue

- [x] BC Clínico — agregado `EpisodioClinico`: estados, diagnósticos CID, notas,
      iniciar/actualizar/fechar/cancelar, e projecção EHR do doente.
- [x] Repositório pgx, handler HTTP com RBAC clínico, integração.
- [x] ADR-027.

## Sprint 7 — entregue

- [x] BC Clínico — agregado `Doente`: VOs de identificação e contactos, entidades-filho
      (alergia, antecedente), estados e eventos.
- [x] Casos de uso de registo, pesquisa, obtenção auditada, actualização e gestão de
      estado; validador de NIF angolano no Shared Kernel.
- [x] Repositório pgx, handler HTTP com RBAC clínico/administrativo, integração.
- [x] ADR-026.

## Sprint 6 — entregue

- [x] Gestão de sessões activas: listar por utilizador (Admin/Auditor/DPO),
      revogar sessão granular por sessionId (Admin). Auditado.
- [x] Edição administrativa de perfil (`PATCH /utilizadores/:id/perfil`, Admin)
      com hidratação JIT a partir do Keycloak.
- [x] Notificações por email (criação e reset de password) best-effort, com
      fallback no-op quando SMTP não configurado. MailHog no compose.
- [x] ADR-025.

## Sprint 5 — entregue

- [x] Reset de password (admin): nova senha temporária devolvida 1x; reset de OTP
      (remove credenciais + `CONFIGURE_TOTP`). Auditados.
- [x] Edição de perfil self-service (`PATCH /perfil`): telefone/BI validados pelos VOs
      Angola; omitido preserva, vazio limpa.
- [x] Revogação de sessões na desactivação; compensação da criação não-atómica
      (ADR-023 fechada).
- [x] ADR-024.

## Sprint 4 — entregue

- [x] Caminho positivo de MFA validado e2e: `director.teste` com OTP → autenticação
      forte reconhecida (acesso concedido); config de MFA fixada no realm.
- [x] Criação administrativa de utilizadores: `POST /api/v1/identidade/utilizadores`
      (RBAC Admin), senha temporária gerada (devolvida 1x), `CONFIGURE_TOTP` em papéis
      sensíveis, 409 em duplicados. Auditoria `identidade.utilizador.criado`.
- [x] ADR-023.

## Sprint 3 — entregue

- [x] Imposição de MFA para papéis sensíveis (Director, Admin, DPO, Auditor):
      `Sessao.AutenticacaoForte` derivada de `acr`/`amr`, middleware
      `MFAObrigatoria` → 403 (`type: /erros/mfa-obrigatorio`).
- [x] Gestão administrativa via Admin REST API do Keycloak: listar, ver,
      atribuir/revogar papel, activar/desactivar (adaptador HTTP puro).
- [x] RBAC por rota: escrita só Admin; leitura Admin/Auditor/DPO. Auditoria de
      todas as escritas.
- [x] Realm: client `sgc-admin` (service account) + utilizador `admin.teste`.
- [x] Smoke tests e2e (MFA negativo + fluxo de atribuição via Keycloak).
- [x] ADR-022.

## Sprint 2 — entregue

- [x] Domínio Identidade: agregado `Utilizador`, VO `Sessao`, enum `Papel` (11), regras
      RBAC puras, eventos e interface de repositório (cobertura 98%).
- [x] Aplicação: casos de uso `Autenticar` e `ObterPerfil` (JIT provisioning + auditoria),
      com portas `VerificadorToken`/`Auditor` (cobertura 95%).
- [x] Adaptadores: cliente Keycloak OIDC (go-oidc, JWKS/RS256, validação `aud`/`azp`),
      middleware `Auth`/`RBAC`/`SegurancaHTTP`/`LimiteTaxa`, respostas RFC 7807 (pt-AO),
      repos pgx (`utilizadores`/`utilizadores_papeis`/auditoria).
- [x] `/readyz` passa a verificar também o Keycloak (JWKS/discovery).
- [x] Migration `identidade/0004_seed_papeis.sql` (catálogo de 11 papéis para o JIT).
- [x] Testes: unit+aplicação (`-race`), gate 85/75/60 OK; **integração end-to-end** com
      Keycloak+PG reais (token real → JIT → auditoria).
- [x] ADR-021 (verificação OIDC, RBAC, JIT). go-oidc registado no `go-arch-lint`.

## Sprint 1 — entregue

- [x] Layout de pacotes (5 BCs + Shared Kernel + 4 camadas Clean) com `go-arch-lint` a
      impor a regra de dependência (Domínio sem infra).
- [x] Camada Plataforma funcional: `config` validada, `log` (slog JSON), `observ`
      (Prometheus + `/metrics`), `db` (pgxpool + runner de migrations), `server` (Gin +
      shutdown gracioso), Shared Kernel `i18n` (pt-AO).
- [x] Migrations forward-only + `schema_migrations` (BC `auditoria`/`identidade`/`shared`);
      audit log append-only com trigger imutável; seed dos 11 papéis (DDM-001).
- [x] Endpoints `/healthz`, `/readyz` (PG+Redis) e `/metrics`.
- [x] Docker Compose (PG16, Keycloak 25, Redis 7, MinIO, Prometheus, Grafana) com
      healthchecks; realm `sgc` preparado (11 papéis, client, utilizador de teste).
- [x] CI/CD: build, `golangci-lint`, `go-arch-lint`, testes `-race`, gate de cobertura
      (85/75/60), integração (migrations/audit/seed com PG), `govulncheck`, `gosec`,
      `hadolint`, `Trivy`. Spec OpenAPI base (`api/openapi/`).
- [x] Validadores Angola (BI, telefone, AOA) no Shared Kernel, testados (domínio ≥85%).
- [x] Errata-001 resolvida (11 papéis) e docs reconciliados.

## Critérios de saída M2 — Clínico Core

- [x] BC Clínico: doente, episódio clínico e EHR. — Sprints 7/8
- [x] BC Farmácia: catálogo, receita, stock e dispensa (FEFO). — Sprints 9/10
- [x] Cirurgia ambulatória: tipo de episódio, procedimento cirúrgico e consentimento
      com anexo obrigatório. — Sprint 11
- [x] Gates de cobertura verdes em todas as fatias (domínio ≥85%, aplicação ≥75%,
      adaptadores ≥60%; pgrepo coberto por integração).
- [x] ADRs 026–030 registadas.

## Critérios de saída M3 — Laboratório

- [x] Médico requisita análises para um episódio aberto. — Sprint 12
- [x] Técnico colhe/recusa amostra e submete resultado preliminar. — Sprint 12
- [x] O preliminar não é visível ao médico. — Sprint 12
- [x] Validação pelo patologista com segregação (submissor ≠ validador). — Sprint 13
- [x] Valores críticos detectados e notificados (SMS auditado). — Sprint 13
- [x] Correcção cria novo resultado preservando o original. — Sprint 13

## Critérios de saída — Integração Início da Consulta (ADR-036)

- [x] O médico atribuído inicia a consulta a partir da fila e recebe o episódio
      ABERTO (tipo CONSULTA) na resposta (201).
- [x] A chegada transita TRIADO→EM_CONSULTA, sai da fila clínica e regista o
      episodio_id que a consumiu (uuid sem FK cross-context).
- [x] Transição + criação atómicas (transacção única no adaptador de integração):
      nunca existe episódio sem chegada consumida nem chegada consumida sem episódio.
- [x] Só o médico atribuído pode iniciar (403 no domínio e no CAS); duplo
      início/corrida → 409; zero colunas novas no BC Clínico.
- [x] 1:1 chegada↔episódio garantido por UNIQUE parcial + CHECK (migração
      recepcao/0004), provado em integração (23505/23514).
- [x] Comando auditado nos dois contextos; cobertura nos limiares.

## Critérios de saída — Triagem no EHR (ADR-037)

- [x] Médico/Enfermeiro/Director veem no detalhe do episódio o bloco triagem
      (prioridade, sinais vitais, observações, enfermeiro, instante) quando o
      episódio nasceu da fila clínica.
- [x] Os resumos de episódio (EHR e listagem) mostram a cor de Manchester a esses
      papéis (leitura em lote).
- [x] Farmacêutico/Técnico de Lab/DPO/Auditor recebem as respostas de sempre — e o
      leitor de triagem nem é invocado (minimização LPDP, ADR-034).
- [x] Episódios sem chegada associada ficam exactamente como antes (sem bloco, sem
      erro); falha do leitor propaga (nunca degrada em silêncio).
- [x] Zero migrações; zero alterações ao BC Recepção; fonte única de verdade.
- [x] Cobertura nos limiares; integração prova a junção real por episodio_id.

## Critérios de saída — Outbox (ADR-038)

- [x] `EpisodioClinico.Fechar` emite `EpisodioFechado`; `Guardar` persiste episódio
      + evento na mesma tx (provado por rollback).
- [x] Relay in-process arranca na Plataforma, processa em lote com `SKIP LOCKED`,
      marca publicados e regista falhas em `tentativas`/`ultimo_erro`.
- [x] Fecho de consulta transita a chegada `EM_CONSULTA → ATENDIDO`
      assincronamente; idempotente; episódio sem chegada é no-op.
- [x] Migrações `shared/0002` e `recepcao/0005` forward-only aplicadas.
- [x] Métricas de outbox expostas; shutdown gracioso: o loop pára com o ctx e um lote
  em curso é abortado em segurança (rollback), retomado no arranque seguinte.
- [x] Gates de cobertura verdes (85/75/60); `go-arch-lint` sem violações.
- [x] ADR-038 registada; CLAUDE.md §6 e o índice de ADRs actualizados.

## Critérios de saída — Arranque do BC Financeiro (ADR-039)

- [x] Agregado `Factura` nasce em RASCUNHO (`NovaFactura`); `AdicionarItem`/
      `RemoverItem` só permitidos em RASCUNHO; `EstadoFactura` antecipa
      EMITIDA/ANULADA no enum e na CHECK, inalcançáveis nesta fatia.
- [x] Papel `Tesoureiro` (12.º, ERRATA-002) válido no enum `identidade.Papel`,
      não-sensível (sem MFA), semeado (`identidade/0005`) e no realm Keycloak.
- [x] IVA por item (ISENTO/STANDARD 14%) com arredondamento meia-acima por linha;
      `Totais()` soma por linha (arredondar e somar, não somar e arredondar) —
      total autoritário no domínio, provado em testes de unidade.
- [x] RBAC nas rotas financeiras: escrita (`POST`/`DELETE`) só Tesoureiro; leitura
      (`GET`) Tesoureiro/Director/Auditor.
- [x] Migração `financeiro/0001_facturas.sql` aplicada (schema `financeiro`,
      `facturas`+`itens_factura`, CHECKs de estado/tipo/regime/quantidade/preço,
      FK intra-BC `itens_factura → facturas`; sem FK cross-context em
      `episodio_id`/`operacao_id`), embebida em `migrations/embed.go`.
- [x] `RepositorioFacturas` pgx: upsert transaccional (INSERT/UPDATE guardado por
      `estado='RASCUNHO'`) com reescrita de linhas, provado por integração real
      contra Postgres.
- [x] Gates de cobertura verdes (domínio ≥85%, aplicação ≥75%, adaptadores ≥60%);
      `go-arch-lint` sem violações.
- [x] ADR-039 registada; CLAUDE.md §6 e o índice de ADRs actualizados.

## Critérios de saída M1

- [x] Identidade Keycloak operacional (login, 11 papéis, MFA para papéis sensíveis — positivo e negativo). — Sprint 2/3/4
- [x] BC Identidade testado (domínio 98% ≥85%). — Sprint 2 (fatia vertical completa)
- [x] Audit log append-only funcional (retenção 10 anos). — trigger imutável + teste de integração
- [ ] CI/CD: build + test + deploy staging < 15 min. — build+test ok; deploy staging por configurar
- [x] `go-arch-lint` sem violações.
- [x] Estrutura de pacotes alinhada com a arquitectura.
- [x] Migrations forward-only funcionais.
- [x] Observabilidade base: slog→JSON, healthchecks, métricas Prometheus.
- [x] README + docs de setup validados.
