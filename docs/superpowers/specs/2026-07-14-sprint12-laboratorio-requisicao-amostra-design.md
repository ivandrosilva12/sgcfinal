# Sprint 12 — BC Laboratório: Catálogo + Requisição + Amostra — Desenho

- **Marco:** M3 — Laboratório (Sprints 12–13)
- **Data:** 2026-07-14
- **Fontes:** CCD-M3 (blueprint, Sprint 9 do marco "Farmácia + Laboratório"), DDM-001,
  ADR-026/027 (BC Clínico), ADR-028/029 (BC Farmácia — precedente da ACL e da receita).
- **ADR a registar:** ADR-031.

## 1. Contexto e âmbito do marco

O BC Laboratório é o último bounded context clínico por abrir
(`internal/domain/laboratorio/` tem apenas um `doc.go`). O M3 entrega-o em duas
fatias verticais:

- **Sprint 12 (esta spec):** catálogo de análises, requisição (via ACL sobre o
  Clínico), colheita/recusa de amostra e submissão de **resultado preliminar** pelo
  técnico. O preliminar não é visível ao médico.
- **Sprint 13:** validação pelo patologista com a invariante de **segregação de
  funções**, resultado validado imutável, correcção como novo resultado, detecção de
  valores críticos com notificação, e demo do marco.

### 1.1 Duas divergências face ao blueprint, registadas conscientemente

1. **Numeração de marcos.** O blueprint (CCD-M3) define o M3 como "Farmácia +
   Laboratório". Neste repositório a Farmácia foi entregue dentro do M2 (Clínico
   Core, Sprints 9–10), pelo que o M3 aqui é **apenas o Laboratório**. Os marcos
   seguintes do blueprint mantêm-se como referência de âmbito (M4 Financeiro, M5
   Frontend), não de numeração.
2. **O "biólogo validador" é o papel `Patologista`.** Os 11 papéis do DDM-001
   (ver `docs/ERRATA-001-papeis.md`) não incluem `Biologo`. O validador do
   laboratório é o `Patologista`; o submissor é o `TecnicoLab`.

### 1.2 Fora de âmbito (dívida registada, não deste marco)

- **Override de alergia com dupla aprovação na Farmácia.** O CCD-M3 exige que o
  override de uma alergia GRAVE seja aprovado por médico prescritor **e** Director
  Clínico. O que existe (`internal/application/farmacia/dispensa.go`) é um override de
  actor único com justificação auditada. Fica registado como dívida; não se resolve
  dentro de um sprint de Laboratório.
- Frontend (filas de técnico e de patologista) — o M5 do blueprint.
- Integração real de SMS (stub no Sprint 13; integração real num marco posterior).

## 2. Domínio (`internal/domain/laboratorio/`)

Três agregados raiz e as respectivas portas de repositório. Domínio puro: só stdlib
e Shared Kernel (`erros`), sem pgx/gin.

### 2.1 `Analise` — catálogo

Dados de referência com regras próprias, logo agregado (e não read model puro), pelo
precedente do `Medicamento` na Farmácia.

- Campos: `codigo` (único), `nome`, `unidade` (mg/dL, g/dL, mmol/L…),
  `intervalosReferencia []IntervaloReferencia`, `valoresCriticos []ValorCritico`,
  `activo`.
- VO `IntervaloReferencia`: `perfil` (ADULTO/PEDIATRICO/GERIATRICO), `sexo`
  (M/F/AMBOS), `minimo`, `maximo`. Invariante: `minimo <= maximo`.
- VO `ValorCritico`: `operador` (`<`, `>`, `<=`, `>=`), `limite`, `descricao`.
  **Registados no Sprint 12, avaliados no Sprint 13.**
- Porta: `RepositorioAnalises` (`Guardar`, `ObterPorCodigo`, `Listar`).

### 2.2 `RequisicaoLab`

- Campos: `id`, `episodioID`, `doenteID`, `medicoRequisitanteID`, `prioridade`
  (ROTINA/URGENTE), `itens []ItemRequisicao` (código de análise + observações),
  `estado` (EMITIDA/CANCELADA), `criadoEm`.
- Sem FK cross-context: `episodioID`/`doenteID` são referências textuais validadas
  pela ACL (secção 3.2), como a `Receita` da Farmácia.
- Invariantes: pelo menos um item; sem itens duplicados (mesmo código de análise);
  médico requisitante obrigatório.
- Porta: `RepositorioRequisicoes` (`Guardar`, `ObterPorID`, `ListarPorEpisodio`).

### 2.3 `Resultado` — o agregado com a state machine

Emitir uma requisição **cria um `Resultado` em PENDENTE por item**. É isto que
povoa a fila do laboratório.

```
PENDENTE ──ColherAmostra──► COLHIDA ──SubmeterPreliminar──► PROCESSADA ─(S13)─► VALIDADA ──► CONCLUIDA
    │                          │
    └──RecusarAmostra──────────┴──► RECUSADA
```

- Campos: `id`, `requisicaoID`, `codigoAnalise`, `valor` (texto — acomoda numérico e
  qualitativo), `unidade`, `observacoes`, `motivoRecusa`, `estado`,
  `tecnicoSubmissorID`, `patologistaValidadorID`, `colhidaEm`, `submetidaEm`,
  `validadaEm`, `valorCritico bool`, `criadoEm`.
- Métodos do Sprint 12:
  - `ColherAmostra(tecnicoID string, em time.Time) error` — só de PENDENTE
    (senão Conflito/409).
  - `RecusarAmostra(motivo string, em time.Time) error` — de PENDENTE ou COLHIDA;
    motivo obrigatório (senão Validação/400).
  - `SubmeterPreliminar(tecnicoID, valor, observacoes string, em time.Time) error` —
    só de COLHIDA; valor obrigatório; grava `tecnicoSubmissorID`.
- **`Validar` fica para o Sprint 13.** Os estados VALIDADA/CONCLUIDA existem já no
  enum e nas CHECK da base de dados — só a transição é que não está implementada. A
  invariante de segregação (submissor ≠ validador) é o valor central do Sprint 13 e
  merece o seu próprio ciclo, como a invariante-estrela mereceu no Sprint 11.
- `Snapshot()`/`ReconstruirResultado()` para persistência, como nos agregados anteriores.
- Porta: `RepositorioResultados` (`GuardarVarios` para a emissão, `ObterPorID`,
  `Transitar` com compare-and-set, `ListarFila`, `ListarPorRequisicao`,
  `ListarPorEpisodio`).

### 2.4 Eventos (scaffolding)

`AmostraColhida`, `AmostraRecusada`, `ResultadoPreliminarSubmetido` — definidos e
testados, não emitidos nesta fatia (coerente com Clínico e Farmácia, onde os eventos
são scaffolding até o Outbox ser ligado).

## 3. Aplicação (`internal/application/laboratorio/`)

### 3.1 O `tecnicoSubmissorID` vem do sujeito autenticado

**Decisão central para a correcção do BC.** O identificador de quem colhe e de quem
submete é o sujeito autenticado (extraído da sessão pelo handler), **nunca um campo
do corpo do pedido**. Se viesse do corpo, a segregação técnico ≠ patologista do
Sprint 13 seria decorativa: um cliente poderia mentir sobre quem submeteu e validar
o seu próprio resultado. A mesma regra valerá para o `patologistaValidadorID`.

### 3.2 ACL Laboratório → Clínico

Espelha a da Farmácia (`internal/adapters/farmacia/leitor_clinico.go`): a **porta**
vive na aplicação, o **adaptador** em `internal/adapters/laboratorio/`.

```go
type LeitorClinico interface {
    // Devolve se o doente existe e está activo.
    DoenteActivo(ctx context.Context, doenteID string) (bool, error)
    // Devolve se o episódio existe, pertence ao doente e está ABERTO.
    EpisodioAbertoDoDoente(ctx context.Context, episodioID, doenteID string) (bool, error)
}
```

O Laboratório nunca importa tipos do domínio Clínico.

### 3.3 Casos de uso

| Caso de uso | Regra central |
|---|---|
| `RegistarAnalise` / `ListarAnalises` | catálogo; código único (Conflito/409 em duplicado) |
| `EmitirRequisicao` | valida doente activo + episódio aberto do doente (ACL) **e cada código de análise contra o catálogo** (inexistente ou inactivo → Validação/400); cria N `Resultado` em PENDENTE; auditado |
| `ObterRequisicao` / `ListarRequisicoesDoEpisodio` | leitura |
| `ColherAmostra` | PENDENTE → COLHIDA; técnico = sujeito autenticado; auditado |
| `RecusarAmostra` | → RECUSADA; motivo obrigatório; auditado |
| `SubmeterPreliminar` | COLHIDA → PROCESSADA; técnico = sujeito autenticado; auditado |
| `ListarFilaLaboratorio` | fila do técnico/patologista; **vê todos os estados** |
| `ListarResultadosDoEpisodio` | leitura clínica; **só devolve VALIDADA/CONCLUIDA** |

### 3.4 A regra de visibilidade

O resultado preliminar não é visível ao médico. A regra é imposta **na aplicação**
(não só no RBAC de rota, que não chegaria — o médico tem de poder ver os validados):

- `ListarFilaLaboratorio` — RBAC `TecnicoLab`/`Patologista`/`Director`; todos os estados.
- `ListarResultadosDoEpisodio` — RBAC clínico; filtra e só devolve VALIDADA/CONCLUIDA.

**Consequência assumida:** no fim do Sprint 12 este endpoint devolve sempre vazio,
porque nada chega ainda a VALIDADA. Isso *é* o critério de saída do blueprint ("o
médico tenta ver o preliminar e não vê"), não um defeito. Fica coberto por um teste
que o afirma explicitamente.

## 4. Adaptadores e persistência

### 4.1 Migrações (schema `laboratorio`, forward-only)

- `migrations/laboratorio/0001_catalogo_analises.sql` — `CREATE SCHEMA IF NOT EXISTS
  laboratorio`; tabela `analises` com `intervalos_referencia`/`valores_criticos` em
  `jsonb`; seed de análises comuns (hemoglobina, hemograma, glicemia, creatinina,
  ureia).
- `migrations/laboratorio/0002_requisicoes_resultados.sql` — `requisicoes`,
  `itens_requisicao`, `resultados`.
- **Acrescentar `laboratorio` à directiva `//go:embed` em `migrations/embed.go`** —
  os BCs estão listados explicitamente; sem isto as migrações não entram no binário.
  O runner ordena os BCs alfabeticamente, pelo que `laboratorio` corre depois de
  `farmacia` e antes de `shared`, sem dependências entre eles.

Duas lições do Sprint 11 levadas para o schema:

1. **CHECK de coerência estado↔timestamps** — não há PROCESSADA sem `submetida_em`
   nem submissor; não há COLHIDA sem `colhida_em`; RECUSADA exige `motivo_recusa`.
2. **Guarda compare-and-set nos `UPDATE` de transição** — `WHERE id = $1 AND estado =
   $2`; zero linhas afectadas → Conflito/409. Duas colheitas concorrentes da mesma
   amostra não se atropelam.

### 4.2 Repositórios pgx e HTTP

- `internal/adapters/pgrepo/`: `analises_repo.go`, `requisicoes_repo.go`,
  `resultados_repo.go` (SQL puro, sem ORM).
- `internal/adapters/laboratorio/leitor_clinico.go`: a ACL da secção 3.2.
- `internal/adapters/http/laboratorio_handler.go`: erros por `responderErro`
  (RFC 7807), RBAC por rota:

| Rota | Papéis |
|---|---|
| `POST /analises` | `Admin`, `Director` |
| `GET /analises` | `Medico`, `Enfermeiro`, `TecnicoLab`, `Patologista`, `Director`, `Admin` |
| `POST /episodios/:id/requisicoes` | `Medico` |
| `GET /laboratorio/fila` | `TecnicoLab`, `Patologista`, `Director` |
| `POST /resultados/:id/colheita`, `/recusa`, `/preliminar` | `TecnicoLab` |
| `GET /episodios/:id/resultados` | `Medico`, `Enfermeiro`, `Director` (só validados) |

- `internal/platform/app.go`: wiring dos repositórios, casos de uso e rotas.

## 5. Testes

- **Domínio (≥85%):** tabela de transições inválidas (colher o que já foi colhido,
  submeter sem colher, recusar o que já foi processado), obrigatoriedade de motivo na
  recusa e de valor na submissão, invariantes do catálogo e da requisição.
- **Aplicação (≥75%, fakes):** ACL a recusar episódio fechado ou doente inactivo;
  emissão a criar um resultado PENDENTE por item; **teste explícito de que
  `ListarResultadosDoEpisodio` não devolve PROCESSADA**; auditoria das escritas.
- **Adaptadores (≥60%):** integração contra Postgres real (tag `integration`, SKIP sem
  `DATABASE_URL`) — ciclo requisição → colheita → preliminar, e a guarda
  compare-and-set a rejeitar a transição concorrente.

## 6. Critérios de saída do Sprint 12

- [ ] Médico requisita análises para um episódio aberto; requisição inválida
      (episódio fechado, doente inactivo, análise inexistente) é rejeitada.
- [ ] Técnico vê a requisição na fila e regista a colheita; pode recusar a amostra
      com motivo.
- [ ] Técnico submete resultado preliminar; o submissor gravado é o utilizador
      autenticado.
- [ ] O resultado preliminar **não** é visível ao médico.
- [ ] Gates de cobertura verdes; integração contra Postgres a passar.
- [ ] ADR-031 registada (incluindo as divergências da secção 1.1 e a dívida da 1.2).
