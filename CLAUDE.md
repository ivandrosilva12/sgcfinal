# CLAUDE.md — Contexto Mestre do Projecto SGC Angola

> Lido automaticamente por Claude Code no início de cada sessão. Conciso e curado.
> Adaptado das convenções do blueprint `Software Gestão Clínicas` (v2.0, Maio 2026)
> para o repositório de implementação `Software Clinicas Final`.

---

## 1. Identidade e Voz

- **Projecto**: SGC Angola — Sistema de Gestão de Clínicas privadas em Angola.
- **Idioma**: **PT-PT angolano** em TODA a saída — código, comentários, commits, PRs,
  docs, mensagens de erro, UI. Nunca PT-BR. Nunca EN nas mensagens visíveis.
- **Linguagem ubíqua**: termos clínicos/negócio em português (Doente, Episódio, Receita,
  Factura, Lote, Dispensa, Utilizador, Papel, Sessão, MovimentoStock, Composição). Nunca
  misturar com EN (não usar Patient/Invoice/Prescription).
- **Tom**: profissional, preciso, sem floreados.

## 2. Stack Técnico (não negociável)

| Camada | Tecnologia | Versão | Notas |
|--------|------------|--------|-------|
| Backend | Go | 1.22+ | sem alternativas |
| Web framework | Gin | latest | escolhido sobre chi (ADR-002) |
| Driver PG | pgx | v5 | **sem ORM — SQL puro** (ADR-003) |
| BD | PostgreSQL | 16 | schema por bounded context (ADR-004) |
| Auth | Keycloak | 25 | OIDC; não rolar próprio (ADR-008) |
| Cache/sessões/rate-limit | Redis | 7+ | |
| Object storage (documentos) | MinIO (S3) | latest | presigned URLs |
| Observabilidade | Prometheus + Grafana | latest | on-premise; slog→JSON→journald |
| Container | Docker + Compose | latest | dev/staging/prod |
| PACS (M3, ADR-016) | Orthanc DICOMweb | 1.12+ | referência apenas; DICOM nunca na BD |

**Nunca propor alternativas sem uma ADR formal.** Em dúvida, parar e pedir validação.

## 3. Arquitectura

- **Estilo**: Monólito modular com **DDD táctico + Clean Architecture**.
- **5 Bounded Contexts**: `clinico`, `farmacia`, `laboratorio`, `financeiro`,
  `identidade` + **Shared Kernel**.
- **4 Camadas Clean** (dependência aponta para dentro):
  1. **Domínio** — entidades, VOs, eventos, interfaces de repositório. Zero imports de infra.
  2. **Aplicação** — casos de uso. Importa apenas Domínio.
  3. **Adaptadores** — HTTP (Gin), repositórios PG (pgx), integrações. Importam Domínio+Aplicação.
  4. **Plataforma** — composition root, config, observabilidade. Importa tudo.
- **Inter-context**: eventos via **Outbox** (assíncrono); interfaces explícitas; **ACL**.
- **Sem FK cross-context**. Snapshots onde necessário.
- **Migrations**: forward-only, sem `.down.sql`. Reversão por restore.
- **Linter arquitectural**: `go-arch-lint` em CI bloqueia violações da regra de dependência.

## 4. Layout do Repositório

```
cmd/api/main.go              # entrypoint (graceful shutdown)
internal/
├── domain/                  # Camada 1 — DDD (shared, clinico, farmacia, laboratorio,
│                            #   financeiro, identidade)
├── application/             # Camada 2 — casos de uso
├── adapters/                # Camada 3 — http, pgrepo, keycloak, redis, minio, outbox, pacs
└── platform/                # Camada 4 — config, log, observ, server, db, app.go
migrations/                  # por BC, numeradas (forward-only)
seeds/  tests/  docs/  adrs/  docker/
```

## 5. Princípios Não-Negociáveis

1. **On-premise por clínica** — dados clínicos não saem do território (Lei 22/11).
2. **LPDP mínimo** — encryption at rest/in transit, audit log append-only, RBAC granular.
3. **Audit log imutável** — trigger PG bloqueia UPDATE/DELETE. **Retenção 10 anos.**
4. **Cadeia hash de facturas** — SHA-256, imutáveis. Anulação por nova factura.
5. **Domínio rico, não anémico** — regras nas entidades.
6. **Fakes > Mocks** em testes de aplicação.
7. **Forward-only migrations**. Excepção única e registada: a edição do `ALTER ROLE` da
   `shared/0003` (ver **ADR-043 §6 R1**), justificada por igualdade de resultado. Não é
   precedente — qualquer outra correcção continua a ser por migração nova.
8. **Nada de `panic()`** fora de inicialização — sempre `error`.
9. Cobertura **desde Sprint 1**: domínio ≥85%, aplicação ≥75%, adapters ≥60%.

## 6. Marco Actual

**M4 — Financeiro** (em curso; Sprints 14–18; ver `SPRINT.md`). Arranque do último dos
5 bounded contexts, precedido da fundação RBAC do 12.º papel **Tesoureiro**
(ERRATA-002, `docs/ERRATA-002-papel-tesoureiro.md`; não-sensível na fatia do ADR-039 e
**sensível, com MFA obrigatória**, desde a emissão — ver a revisão de 2026-07-18 na
ERRATA). A Sprint 14 entregou o agregado `Factura` em estado **RASCUNHO** (ADR-039,
Opção A): `ItemFactura` com snapshot de linha (descrição/preço fornecidos no pedido) e
id lógico da operação de origem (sem FK cross-context), IVA por item (ISENTO/STANDARD
14%, arredondamento meia-acima por linha) com total autoritário no domínio,
persistência por upsert transaccional e RBAC (escrita Tesoureiro; leitura
Tesoureiro/Director/Auditor).

A Sprint 15 entregou a **emissão** (ADR-040): VO `NumeroFactura` (`FAC <série>/<8
dígitos>`, série = ano civil UTC), `Factura.Emitir` com o **hash SHA-256 canónico como
invariante do agregado** (formato normativo e três regras de canonicalização fixados na
ADR-040 e travados por vector dourado), `VerificarCadeia` como função pura (detecta
buraco, elo errado e adulteração), **numeração sequencial por série sem buracos**
serializada por `SELECT ... FOR UPDATE` na tabela `financeiro.series`,
**imutabilidade** por triggers em `facturas`/`itens_factura` e **bloqueio optimista**
(coluna `versao`), que fecha a dívida técnica assumida no ADR-039. O papel
**Tesoureiro passa a sensível** (12 papéis, 5 sensíveis) e a `MFAObrigatoria()` passa a
ser imposta nas rotas do Financeiro. Ficam fora desta fatia — e **não estão feitas** —
a **anulação** de facturas, a submissão **AGT/SAF-T-AO** e o agendamento do cron diário
de verificação da cadeia (REG-001 §3.4). A ADR-040 regista ainda uma **dívida sistémica
de segurança**: 10 grupos de rotas fora do Financeiro expõem papéis sensíveis sem
`MFAObrigatoria()` (R3), a resolver em fatia própria.

A Sprint 16 entregou a **selagem canónica** (ADR-041), pagando as duas dívidas que a
ADR-040 deixou com prazo — R1 e R2, revisíveis apenas **antes da primeira emissão em
produção**. A colisão do R1 revelou-se **real e não teórica**: reproduzida contra o
código, duas facturas materialmente diferentes partilhavam hash e total (a `Descricao`
chega no corpo do pedido HTTP, validada só por `TrimSpace` e não-vazio, e a CHECK
`preco_unitario_centimos >= 0` admite linhas a preço zero, que dão a folga para os
totais coincidirem). Correcção: **enquadramento injectivo** — `{len em bytes}:{s}` em
todo o campo de texto, sem excepções — e **alargamento do selo** a `clienteNome`,
`clienteMorada`, `operacaoID` e `episodioID`; o `ItemFactura.ID` fica deliberadamente
fora (chave substituta sem significado fiscal). O canónico passa a ter **12 campos** e
o vector dourado a
`7c99e3dbc895f04e3e40d4114dea8f5129e10297de33a25222d9dcc401c796da`. Fatia **puramente
de domínio: zero migrações**. Continuam por fazer a **anulação** (que herda a restrição
vinculativa da ADR-040 §R5: não apagar nem renumerar), o **SAF-T-AO** e a
**certificação AGT**.

A Sprint 17 entregou a **imposição uniforme de MFA** e a **factura nascida em RASCUNHO**
(ADR-042), fechando dois riscos herdados da ADR-040. **R3:** o `mfaMW` chegava a três
dos catorze grupos de rotas; a medição da ADR-040 §R3 (11 grupos sem `mfaMW`, 10 a
expor papel sensível — `Director`/`DPO`/`Auditor` em toda a leitura clínica,
`Admin`/`Director` em laboratório e recepção) **estava certa**, e uma contra-medição
posterior que a dava por exagerada é que estava errada (dois defeitos de instrumento a
subestimar). Correcção: pacote único `protecao := []gin.HandlerFunc{limiteMW, authMW,
mfaMW}` nos 14 grupos + **guarda AST** sobre o `app.go` (analisa `go/ast`, exige
conjunto nomeado e conteúdo de `protecao`; resistiu a 4 rondas de ataque). A lacuna que
deixou a exposição passar — routers de teste com cadeias próprias, nada a verificar o
`app.go` — ficou fechada dos dois lados: **prova comportamental** (`mfa-obrigatorio`)
nas 10 famílias e routers de teste a espelhar a produção. OTP completo no realm
(`admin.teste`, `dpo.teste`, `auditor.teste`). **R6:** a factura passa a **nascer
RASCUNHO** (trigger `BEFORE INSERT ... WHEN (NEW.estado <> 'RASCUNHO')`, migração
`financeiro/0004`) — **mas** a garantia ficou condicionada pelo **R7**, fechado depois
pela ADR-043. `RegistarHealth` fica isento por desenho. **Não existem** ainda anulação,
pagamentos, SAF-T-AO nem certificação AGT.

A Sprint 18 entregou a **separação da credencial de migração da de runtime** (ADR-043),
fechando o **R7** — o último risco estrutural herdado da ADR-040. A medição mostrou que o
R7 era mais largo do que as três ADR anteriores descreviam: o papel único (`sgc`) era
**superuser** por construção da imagem `postgres:16`, não apenas dono, e **`TRUNCATE
auditoria.auditoria_eventos` apagava o audit log de retenção obrigatória de 10 anos sem
sequer tocar em triggers** — não estava registado em risco nenhum. Nasce `sgc_app`
(`DATABASE_URL`; DML apenas, sem posse e sem DDL), `sgc` fica migrador
(`DATABASE_MIGRATION_URL`, **opcional** na config precisamente para não ter de viver no
ambiente do processo servidor), e `db.VerificarPapelRuntime` faz o servidor **recusar
arrancar** com papel privilegiado, sem isenção por ambiente. As quatro famílias de
interrogação avaliam o poder sobre a **união dos papéis assumíveis por `SET ROLE`** e não
os atributos do papel: a lição transferível é que **`pg_has_role` se pergunta com
`MEMBER`, não com `USAGE`** — medido, um membro `NOINHERIT` dá `USAGE=false`,
`MEMBER=true` e o `SET ROLE` funciona na mesma. Acrescem uma guarda AST sobre o `app.go`
(17 mutações medidas) e uma guarda de deriva do **inventário exacto** de privilégios (31
tabelas + 3 sequências). A fatia custou **dois Critical**, ambos com exploração
reproduzida e ambos da mesma classe — o âmbito real da verificação era mais estreito do
que o nome dela prometia. O **R2 (DBA malicioso, `pg_dump`/`pg_restore`) fica declarado
como limite, não como omissão**: o R7 defende contra aplicação comprometida, não contra
acesso directo ao cluster. Provisionamento de produção em
`docs/RUNBOOK-provisionamento-bd.md`, cujo ensaio literal contra um cluster limpo apanhou
o último defeito da fatia: o `ALTER ROLE` incondicional da `shared/0003` **não corria**
com o migrador `NOSUPERUSER` que o próprio runbook prescreve — e em dev/CI nunca falhava,
porque lá o migrador é superuser por construção da imagem. **O ambiente que a fatia
endurece era o único onde o defeito aparecia.** Continuam por fazer **anulação**,
**pagamentos**, **SAF-T-AO** e **certificação AGT**.

**M3 — Laboratório** (entregue; Sprints 12–13; ver `SPRINT.md`). Entrega o BC
Laboratório completo: catálogo de análises, requisição (via ACL sobre o Clínico),
amostra e resultado com state machine até ao preliminar (Sprint 12); validação com
segregação de funções (técnico ≠ patologista), detecção e notificação de valores
críticos (SMS best-effort auditado) e correcção de resultado preservando o original
(Sprint 13, ADR-035).

**Marco Percurso Ambulatório** (entregue, a par do M3): o percurso do doente antes da
consulta. Sub-projectos: **Marcação** (ADR-032), **Check-in** (ADR-033) e **Triagem** (BC
`recepcao` — prioridade Manchester, sinais vitais, fila clínica; ver ADR-034). O início da
consulta (Chegada TRIADO → Episódio no BC Clínico) foi entregue como **Integração
Recepção→Clínico** (ADR-036): transacção única no adaptador de integração, estado
EM_CONSULTA, só o médico atribuído. A triagem ficou visível no EHR (ADR-037):
leitura ACL pela ponte episodio_id, filtrada por papel (minimização LPDP). O
mecanismo Outbox (ADR-038) ficou construído — relay in-process com `SKIP LOCKED`,
escrita transaccional no repositório dono da tx, entrega at-least-once — com o
primeiro consumidor real: `clinico.episodio.fechado` transita a chegada
`EM_CONSULTA → ATENDIDO`, fechando o desfecho pós-consulta do percurso.

**M2 — Clínico Core** (entregue; Sprints 7–11): BC Clínico (doente, episódio + EHR,
cirurgia ambulatória com consentimento LPDP) e BC Farmácia (catálogo, receita, stock
e dispensa FEFO).

**M1 — Fundações** (entregue; ver `docs/PLANO-M1-Fundacao-Identidade.md`): esqueleto
arquitectural + infra (Docker Compose) + fatia vertical do BC Identidade (Keycloak
OIDC + JWT RS256 + RBAC 11 papéis — DDM-001, ver `docs/ERRATA-001-papeis.md`) +
audit log + observabilidade. Pendente: deploy de staging em CI/CD. **Nota**: o
catálogo RBAC do DDM-001 v2.0 passou a **12 papéis** com o arranque do M4 —
Tesoureiro, ver `docs/ERRATA-002-papel-tesoureiro.md`.

## 7. Antipadrões a Rejeitar

- ❌ Domínio anémico. ❌ Infra (`pgx`/`gin`/`http`) em `internal/domain/`.
- ❌ "God service". ❌ Modelo único partilhado entre BCs. ❌ Repositório CRUD genérico.
- ❌ Linguagem misturada (PT/EN/BR). ❌ Coverage theatre. ❌ ORM "para acelerar".
- ❌ Armazenar DICOM na BD (ADR-016). ❌ MovimentoStock como UPDATE (ADR-017).

## 8. Regras Operacionais Angola

- **BI**: 8 dígitos + 2 letras + 3 dígitos. Validador em `internal/domain/shared/identity/`.
- **Telefone**: `+244 9XX XXX XXX`.
- **Moeda**: AOA (Kwanza). Display: `1.234,50 Kz`.
- **IVA**: 14% standard; saúde geralmente isenta; configurável por item.
- **AGT/SAF-T-AO**: cadeia hash SHA-256, numeração sequencial, submissão mensal até dia 25.

## 9. Em Caso de Dúvida

Consultar `docs/`, procurar ADR existente em `adrs/`; se persistir, **parar e pedir
confirmação humana**. Nunca improvisar decisão arquitectural ou de conformidade.

---

**Convenções-fonte**: `..\Software Gestão Clínicas\Prompts\CLAUDE.md` (blueprint completo,
80 documentos + 19 ADRs). ADRs registadas: `adrs/ADR-020-fundacao-m1.md`,
`adrs/ADR-021-identidade-oidc-rbac.md`, `adrs/ADR-022-mfa-gestao-admin.md`,
`adrs/ADR-023-mfa-positivo-criar-utilizadores.md`, `adrs/ADR-024-ciclo-vida-utilizador.md`,
`adrs/ADR-025-sessoes-perfil-admin-notificacoes.md`, `adrs/ADR-026-bc-clinico-doente.md`,
`adrs/ADR-027-bc-clinico-episodio.md`, `adrs/ADR-028-bc-farmacia-receita.md`,
`adrs/ADR-029-farmacia-stock-dispensa.md`,
`adrs/ADR-030-cirurgia-ambulatoria-consentimento.md`,
`adrs/ADR-031-bc-laboratorio.md`,
`adrs/ADR-032-bc-recepcao-marcacao.md`,
`adrs/ADR-033-bc-recepcao-checkin.md`,
`adrs/ADR-034-bc-recepcao-triagem.md`,
`adrs/ADR-035-laboratorio-validacao-correccao.md`,
`adrs/ADR-036-integracao-inicio-consulta.md`,
`adrs/ADR-037-ehr-triagem.md`,
`adrs/ADR-038-outbox.md`,
`adrs/ADR-039-bc-financeiro-factura.md`,
`adrs/ADR-040-emissao-factura.md`,
`adrs/ADR-041-selagem-canonica.md`,
`adrs/ADR-042-mfa-uniforme.md`,
`adrs/ADR-043-separacao-credenciais.md`.
Próximo ADR: **ADR-044**.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
