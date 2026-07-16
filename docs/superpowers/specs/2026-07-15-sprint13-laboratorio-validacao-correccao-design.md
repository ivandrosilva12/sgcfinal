# Sprint 13 — BC Laboratório: Validação + Valores Críticos + Correcção — Desenho

- **Marco:** M3 — Laboratório (Sprints 12–13) — **fecho do marco**.
- **Data:** 2026-07-15
- **Fontes:** CCD-M3 (blueprint), DDM-001, ADR-031 (fatia 1 do BC Laboratório),
  ADR-025 (padrão do Notificador/fallback no-op do Sprint 6).
- **ADR a registar:** ADR-032.

## 1. Contexto e âmbito

O Sprint 12 (ADR-031) abriu o BC Laboratório até ao **resultado preliminar**
(`PROCESSADA`), submetido pelo técnico e invisível ao médico. Ficaram preparados no
código, mas por implementar: os estados `VALIDADA`/`CONCLUIDA` no enum e nas CHECK da
base de dados, a CHECK de segregação (`patologista_validador_id <> tecnico_submissor_id`),
o campo `valorCritico bool` no agregado e as regras `[]ValorCritico` no catálogo.

Esta fatia fecha o marco M3 com as três entregas em falta (`SPRINT.md:161-163`):

1. **Validação pelo patologista** com segregação de funções (submissor ≠ validador).
2. **Valores críticos** detectados na validação e notificados por SMS auditado.
3. **Correcção** que cria um novo resultado preservando o original.

### 1.1 Fora de âmbito (dívida registada)

- **Integração real de SMS.** O adaptador SMS espelha o padrão do email (Sprint 6):
  interface + implementação concreta configurável + fallback no-op auditado. A
  integração com um gateway SMS real de Angola fica para um marco posterior; em dev o
  no-op é o suficiente para exercer a auditoria.
- **Override de alergia com dupla aprovação na Farmácia** (dívida herdada da ADR-031).
- **Auditoria fora da transacção** das escritas (como em todos os sprints anteriores).
- **Emissão de eventos de domínio** (scaffolding à espera do Outbox).

## 2. Máquina de estados (final)

```
PENDENTE ──Colher──► COLHIDA ──Submeter──► PROCESSADA ──Validar──► VALIDADA
    │                   │                                             │
    └──Recusar──────────┴──► RECUSADA                        Corrigir │
                                                                      ▼
        original: VALIDADA ─────────────────────────────► CONCLUIDA (arquivado)
        novo:                                              VALIDADA  (vigente, corrige_resultado_id→original)
```

- **`PROCESSADA → VALIDADA`** (`Validar`): o patologista valida o preliminar. A partir
  daqui o resultado é visível ao médico.
- **`VALIDADA → CONCLUIDA`** só acontece por **correcção**: o original é arquivado
  (`CONCLUIDA`) e nasce um novo `Resultado` `VALIDADA` que o substitui. Não há outro
  caminho para `CONCLUIDA` — um resultado validado sem correcção permanece `VALIDADA`.

### 2.1 Decisão central: `CONCLUIDA` = arquivado/substituído

`CONCLUIDA` **não** é o fecho normal de um resultado bom; é o estado do original depois
de ter sido corrigido. Um resultado validado que nunca é corrigido fica `VALIDADA`
para sempre. Esta é a semântica que dá corpo ao critério "a correcção cria novo
resultado preservando o original": o original não é apagado nem editado — muda de
estado e passa a ser apontado pelo novo (`corrige_resultado_id`).

## 3. Domínio (`internal/domain/laboratorio/`)

Domínio puro: só stdlib e Shared Kernel (`erros`). Sem pgx/gin.

### 3.1 `resultado.go` — novos métodos no agregado

**`Validar(patologistaID string, critico bool, em time.Time) error`**

- Exige `estado == PROCESSADA` (senão `CategoriaConflito`/409).
- `patologistaID` não vazio (senão `CategoriaValidacao`/400).
- **Invariante de segregação:** `patologistaID != tecnicoSubmissorID` — senão
  `CategoriaRegraNegocio`/422. É o valor central do sprint: quem submeteu nunca valida
  o seu próprio resultado.
- `em` não zero.
- Efeito: `estado = VALIDADA`, grava `patologistaValidadorID`, `validadaEm`,
  `valorCritico = critico`.

O flag `critico` é calculado **fora** do agregado (na aplicação, secção 4.2), porque a
avaliação precisa da `Analise` e o `Resultado` não a conhece. O agregado recebe o
booleano já decidido — mantém-se rico quanto às suas invariantes de transição, sem
importar o catálogo.

**`Corrigir(patologistaID, novoValor, observacoes string, critico bool, em time.Time) (*Resultado, error)`**

Método sobre o `Resultado` `VALIDADA` vigente. Devolve o **novo** `Resultado` e muda o
receptor para `CONCLUIDA`:

- Exige `estado == VALIDADA` (senão `CategoriaConflito`/409).
- `patologistaID` não vazio; `novoValor` não vazio.
- **Segregação preservada:** o novo resultado herda o `tecnicoSubmissorID` do original
  (proveniência), e `patologistaID != tecnicoSubmissorID` continua a valer — o
  corrector nunca é o técnico que submeteu o preliminar original.
- Efeito no receptor: `estado = CONCLUIDA` (arquivado).
- Novo `Resultado` devolvido: cópia com novo `valor`/`observacoes`, `estado = VALIDADA`,
  `patologistaValidadorID = patologistaID`, `validadaEm = em`, `valorCritico = critico`,
  `corrigeResultadoID = <id do original>`, `tecnicoSubmissorID` herdado.

> **Nota de segregação na correcção:** o corrector *pode* ser o mesmo patologista que
> validou o original (auto-emenda de uma releitura/erro de transcrição é clínica
> normal). O que a invariante proíbe é validar/corrigir contra o **técnico submissor**
> original — nunca contra o validador anterior.

**Novo campo e getter:** `corrigeResultadoID string` no struct, no `SnapshotResultado`,
no `Snapshot()` e no `ReconstruirResultado()`. Getter `CorrigeResultadoID() string`.

### 3.2 `analise.go` — avaliação de valor crítico no domínio

**`AvaliarCritico(valorTexto string) bool`** (método de `*Analise`):

- Faz `strconv.ParseFloat` do valor. Se **não** for numérico (ex.: "Positivo",
  "Negativo") devolve `false` — os valores críticos configurados são limiares
  numéricos; um valor qualitativo não dispara alarme numérico.
- Se numérico, testa contra cada `ValorCritico` (`operador` ∈ {`<`,`>`,`<=`,`>=`},
  `limite`). Devolve `true` se **alguma** condição for satisfeita.

Zero dependências de infra; testável isoladamente.

### 3.3 `resultado.go` — porta de repositório

Acrescentar à `RepositorioResultados`:

```go
// Corrigir persiste a correcção numa única transacção: INSERT do novo Resultado
// (VALIDADA, corrige_resultado_id→original) e UPDATE compare-and-set do original
// (VALIDADA→CONCLUIDA). Qualquer falha faz rollback de ambos. Devolve o id do novo.
Corrigir(ctx context.Context, novo *Resultado, original *Resultado) (string, error)
```

`Validar` reutiliza o `Transitar` compare-and-set já existente (`PROCESSADA → VALIDADA`).

### 3.4 `eventos.go` — scaffolding

Acrescentar `ResultadoValidado`, `ValorCriticoDetectado` e `ResultadoCorrigido`
(definidos e testados, não emitidos — coerente com os sprints anteriores até o Outbox
ser ligado).

## 4. Aplicação (`internal/application/laboratorio/`)

### 4.1 Novas portas (`ports.go`)

```go
// ContactoClinico resolve o telefone de um utilizador do BC Identidade para a
// notificação de valor crítico. É uma extensão da ACL: o domínio/aplicação do Lab
// continua sem importar Identidade — só o adaptador conhece o outro contexto.
type ResolvedorContacto interface {
    ContactoClinico(ctx context.Context, userID string) (telefone string, ok bool, err error)
}

// NotificadorCritico envia o alerta de valor crítico. Best-effort: uma falha de
// envio não reverte a validação (secção 4.3).
type NotificadorCritico interface {
    NotificarValorCritico(ctx context.Context, telefone, codigoAnalise, valor string) error
}
```

**Mudança de visibilidade:** `EstadosVisiveisAoMedico` passa de `{ResValidada,
ResConcluida}` para **`{ResValidada}`**. O `CONCLUIDA` (arquivado/substituído) sai da
leitura clínica normal — evita mostrar ao médico um valor crítico obsoleto ao lado do
corrigido. O original permanece auditável e acessível pela cadeia `corrige_resultado_id`.

### 4.2 `CasoValidarResultado`

1. Carrega o `Resultado` (`ObterPorID`).
2. Carrega a `Analise` do resultado (`RepositorioAnalises.ObterPorCodigo`).
3. `critico := analise.AvaliarCritico(res.Valor())` — precisa de getter `Valor()`.
4. `res.Validar(actor, critico, agora())`.
5. `Transitar` (compare-and-set).
6. Audita `laboratorio.resultado.validado`.
7. **Se `critico`:** resolve o telefone do médico requisitante e notifica (secção 4.4).

O médico requisitante vem da requisição: `res.RequisicaoID()` →
`RepositorioRequisicoes.ObterPorID` → `medicoRequisitanteID`.

### 4.3 `CasoCorrigirResultado`

1. Carrega o `Resultado` `VALIDADA` + a `Analise`.
2. `critico := analise.AvaliarCritico(novoValor)` — reavalia com o valor corrigido.
3. `novo, err := original.Corrigir(actor, novoValor, observacoes, critico, agora())`.
4. `repo.Corrigir(ctx, novo, original)` — transacção única (secção 3.3).
5. Audita `laboratorio.resultado.corrigido` (detalhe: id do original e do novo).
6. **Se `critico`:** resolve contacto e notifica (o valor corrigido pode ter tornado
   crítico um resultado que não era, ou vice-versa).

### 4.4 Notificação de valor crítico (best-effort, auditada)

Sequência partilhada por 4.2 e 4.3 quando `critico == true`:

1. `medicoID` da requisição → `ResolvedorContacto.ContactoClinico(medicoID)`.
2. Se `ok`, `NotificadorCritico.NotificarValorCritico(telefone, codigo, valor)`.
3. **Audita sempre** `laboratorio.valor_critico.notificado` — com o resultado do envio
   (enviado / sem contacto / falha). É isto que cumpre o critério "SMS auditado": o
   registo de auditoria é a prova, não o envio em si.

**Best-effort:** um erro do resolvedor ou do notificador **não** falha a validação/
correcção (que já está persistida). É registado na auditoria e em log; a validação
devolve 200. Falhar a validação por causa de um SMS seria pior do que o alerta não sair.

### 4.5 Resumo dos casos de uso novos

| Caso de uso | Regra central |
|---|---|
| `ValidarResultado` | PROCESSADA → VALIDADA; segregação `actor ≠ submissor`; avalia crítico; notifica se crítico; auditado |
| `CorrigirResultado` | VALIDADA → CONCLUIDA + novo VALIDADA; herda submissor; reavalia crítico; notifica; auditado |

## 5. Adaptadores e persistência

### 5.1 Migração — `migrations/laboratorio/0003_correccao_resultados.sql` (forward-only)

```sql
ALTER TABLE laboratorio.resultados
    ADD COLUMN corrige_resultado_id uuid NULL REFERENCES laboratorio.resultados(id);
CREATE INDEX IF NOT EXISTS idx_resultados_corrige
    ON laboratorio.resultados (corrige_resultado_id);
```

As CHECK de coerência estado↔timestamps↔autores e a CHECK de segregação
(`patologista_validador_id <> tecnico_submissor_id`) **já existem** desde o Sprint 12
(`0002_requisicoes_resultados.sql`) — cobrem `VALIDADA`/`CONCLUIDA` sem alteração.

### 5.2 `pgrepo/resultados_repo.go`

- `Corrigir(ctx, novo, original)`: `BEGIN`; `INSERT` do novo (com `corrige_resultado_id`);
  `UPDATE ... SET estado='CONCLUIDA' WHERE id=$original AND estado='VALIDADA'`
  (compare-and-set); se `RowsAffected()==0` → `erroTransicaoFalhada` (404/409);
  `COMMIT`. Rollback de ambos em qualquer falha.
- `Validar` usa o `Transitar` existente.

### 5.3 `adapters/laboratorio/leitor_clinico.go` — resolvedor de contacto

Implementa `ResolvedorContacto.ContactoClinico` lendo o telefone via
`identidade.RepositorioUtilizadores` (o adaptador de ACL pode conhecer Identidade — é o
seu trabalho traduzir). Devolve `ok=false` quando o utilizador não tem telefone.

### 5.4 `adapters/sms/` (novo, espelha `adapters/smtp/`)

- `NotificadorSMS` — cliente concreto configurável (endpoint/credenciais por config),
  implementa `NotificadorCritico`.
- `NotificadorNulo` — fallback no-op que regista em nível debug quando o SMS não está
  configurado, tal como `smtp/nulo.go`. Garante que a validação nunca falha por
  ausência de infra de SMS.
- `platform/app.go` escolhe SMS real ou nulo consoante a config (padrão do email).

### 5.5 `adapters/http/laboratorio_handler.go` — rotas novas

`soPatologista := RBAC(dominio.PapelPatologista)`:

| Rota | Papéis | Corpo |
|---|---|---|
| `POST /api/v1/resultados/:rid/validacao` | `Patologista` | — |
| `POST /api/v1/resultados/:rid/correccao` | `Patologista` | `valor`, `observacoes` |

O actor é sempre `SessaoDe(c).Sujeito` — nunca um campo do corpo (regra da ADR-031). O
corpo da correcção é obrigatório: um corpo malformado → 400 (lição do Sprint 11 —
nunca 200 a confirmar uma escrita que não aconteceu).

### 5.6 `platform/app.go` — wiring

Ligar `CasoValidarResultado` e `CasoCorrigirResultado` (com `repoResultados`,
`repoRequisicoes`, `repoAnalises`, resolvedor de contacto, notificador SMS e auditoria),
e as duas rotas novas no handler.

## 6. Testes (gates 85/75/60)

- **Domínio (≥85%):**
  - `Validar`: feliz; não-PROCESSADA → conflito; `actor == submissor` → regra de
    negócio; actor vazio → validação; grava `valorCritico`.
  - `Corrigir`: novo `VALIDADA` + original `CONCLUIDA`; herança do submissor;
    segregação contra o submissor original; `corrigeResultadoID` correcto; não-VALIDADA
    → conflito.
  - `AvaliarCritico`: cada operador (`<`,`>`,`<=`,`>=`), valor não-numérico → `false`,
    catálogo sem valores críticos → `false`, fronteira do limite.
- **Aplicação (≥75%, fakes):**
  - Validação com crítico dispara SMS; sem crítico não dispara.
  - SMS falhado / sem contacto **não** falha a validação (best-effort) mas é auditado.
  - Correcção reavalia crítico com o novo valor e reenvia.
  - Segregação bloqueia a auto-validação antes de tocar o repositório.
  - `ListarResultadosDoEpisodio` passa a mostrar o `VALIDADA` vigente e **esconde** o
    `CONCLUIDA` arquivado após correcção.
- **Integração (≥60%, Postgres real, tag `integration`, SKIP sem `DATABASE_URL`):**
  - Ciclo completo requisição → colheita → preliminar → validação → correcção.
  - A CHECK de segregação da BD nega uma linha `validador == submissor` (defesa em
    profundidade).
  - `Corrigir` numa transacção: novo + original arquivado, atómico.

## 7. ADR-032 e SPRINT.md

- **ADR-032** regista as decisões deste sprint:
  1. `CONCLUIDA` = arquivado/substituído; a correcção substitui e preserva (secção 2.1).
  2. Avaliação de valor crítico **na validação**, no domínio (`Analise.AvaliarCritico`).
  3. SMS ao médico requisitante via extensão da ACL (`ResolvedorContacto`); best-effort
     e auditado.
  4. `EstadosVisiveisAoMedico` reduzido a `{VALIDADA}`.
  5. Segregação: corrector ≠ técnico submissor original (não ≠ validador anterior).
- **SPRINT.md:** marcar os três critérios de saída M3 como entregues e acrescentar a
  secção "Sprint 13 — entregue".

## 8. Critérios de saída do Sprint 13 (fecho do M3)

- [ ] Patologista valida o preliminar (PROCESSADA → VALIDADA); a auto-validação
      (validador == submissor) é rejeitada na aplicação **e** na base de dados.
- [ ] O resultado validado passa a ser visível ao médico na leitura clínica.
- [ ] Valor crítico é detectado na validação e notificado por SMS ao médico
      requisitante; a notificação é auditada mesmo sem infra de SMS.
- [ ] Correcção cria um novo resultado `VALIDADA` e arquiva o original em `CONCLUIDA`,
      preservando-o e ligando os dois; o médico vê só o vigente.
- [ ] Gates de cobertura verdes; integração contra Postgres a passar.
- [ ] ADR-032 registada.
