# Desenho — ADR-039: Arranque do BC Financeiro (agregado Factura em RASCUNHO)

> Data: 2026-07-18
> Marco: M4 — Financeiro (Sprints 14-17). Primeira fatia vertical.
> Estado: aprovado (brainstorming) — pronto para plano de implementação.

## 1. Contexto

O BC Financeiro é o último dos 5 bounded contexts por implementar e o de maior
risco regulatório (AGT, SAF-T-AO, cadeia hash SHA-256, integração EMIS). Segue a
convenção do projecto: cada novo BC arranca por uma fatia vertical fina com ADR
próprio (ADR-026 abriu o Clínico, ADR-031 abriu o Laboratório).

O blueprint separa Sprint 14 (domínio Factura) de Sprint 15 (cadeia hash +
numeração + imutabilidade). Esta fatia (ADR-039) entrega **apenas o domínio em
estado RASCUNHO**; a emissão e a jóia da coroa regulatória ficam para o ADR-040.

### Decisões de brainstorming

1. **Alvo:** arranque do BC Financeiro (M4), não dívida pendente (deploy staging,
   gateway SMS).
2. **Corte da fatia:** Opção A — domínio RASCUNHO; cadeia hash/numeração/emissão
   deferidas para o ADR-040.
3. **RBAC:** introduzir `PapelTesoureiro` (12.º papel) via ERRATA-002. Confirmado
   pelo utilizador: Tesoureiro **não-sensível** (sem MFA) nesta fatia; a exigência
   de MFA é decisão explícita a rever no ADR-040.
4. **Snapshot de linha:** descrição + preço são **fornecidos no pedido** por quem
   factura. Auto-população via ACL (puxar preço/descrição das operações clínicas)
   fica deferida.

## 2. Âmbito e fronteiras

**Entra nesta fatia:**

- Agregado `Factura` rico em estado RASCUNHO: construção, linhas com tipo e
  snapshot, cálculo de IVA e totais, persistência transaccional, HTTP + RBAC.
- Fundação RBAC: papel Tesoureiro (ERRATA-002).

**Fica deferido (fatias seguintes do M4):**

| Deferido | Fatia |
|----------|-------|
| Emissão, cadeia hash SHA-256, numeração sequencial por série, imutabilidade | ADR-040 |
| Anulação por nova factura, pagamentos (parcial, múltiplos métodos) | ADR-041 |
| SAF-T-AO (geração XML + validação XSD + submissão sandbox) | ADR-042 |
| Integração EMIS Multicaixa (sandbox, webhook, estado pendente) | ADR-043 |
| Auto-população de linhas via ACL (Clínico/Farmácia/Lab); validação de episódio via ACL | posterior |
| MFA para o papel Tesoureiro (sensível) | a rever no ADR-040 |

Nesta fatia, `episodio_id` é um uuid lógico sem validação cross-BC, e o snapshot
de cada linha (descrição + preço) vem no pedido.

## 3. Fundação RBAC — Tesoureiro (ERRATA-002)

Os 11 papéis canónicos (DDM-001 v2.0, ERRATA-001) não incluem Tesoureiro, mas o
M4 pressupõe-o ("Tesoureiro + Director + Auditor assinam UAT"). Introduz-se o
12.º papel:

- `PapelTesoureiro` no enum `internal/domain/identidade/papel.go`; `papeisValidos`
  passa a 12 entradas.
- `seeds/papeis.sql` ganha o papel (não-sensível: `sensivel=false`, sem MFA).
- Realm Keycloak (`docker/…`) ganha o realm role `Tesoureiro`.
- `docs/ERRATA-002-papel-tesoureiro.md` regista a decisão e a rastreabilidade ao
  DDM-001 / M4.
- Actualizar os pontos que afirmam "11 papéis": testes de identidade, comentários
  de código, `CLAUDE.md` (§6 nota DDM / índice) e docs que o refiram.

**Sequência no plano:** a ERRATA-002 vem primeiro (o RBAC das rotas financeiras
depende dela).

## 4. Domínio (`internal/domain/financeiro/`)

Segue o padrão dos agregados existentes: campos privados, construtor `Nova…`,
métodos que devolvem `error` via `erros.Novo(erros.Categoria…)`, par
`Snapshot`/`Reconstruir` para persistência, montantes em `moeda.AOA` (cêntimos).

### 4.1 Agregado `Factura` (raiz)

| Campo | Tipo | Notas |
|-------|------|-------|
| `id` | string | uuid atribuído pela BD |
| `estado` | `EstadoFactura` | RASCUNHO nesta fatia |
| `cliente` | `ClienteSnapshot` (VO) | snapshot imutável, sem FK ao Doente |
| `episodioID` | string | uuid lógico, sem FK cross-context |
| `itens` | `[]*ItemFactura` | entidades-filho |
| `criadoEm` / `actualizadoEm` | `time.Time` | |

`EstadoFactura` é um enum que **antecipa** EMITIDA e ANULADA (à imagem do padrão
VALIDADA/CONCLUIDA já presente no enum do Laboratório antes das suas transições),
mas nesta fatia só RASCUNHO é alcançável. As transições para EMITIDA/ANULADA são
do ADR-040/041.

### 4.2 Entidade-filho `ItemFactura`

| Campo | Tipo | Notas |
|-------|------|-------|
| `id` | string | |
| `descricao` | string | snapshot do nome da operação |
| `tipo` | `TipoLinha` | ver enum |
| `operacaoID` | string | uuid lógico da operação de origem; sem FK |
| `quantidade` | int | > 0 |
| `precoUnitario` | `moeda.AOA` | snapshot do preço no momento |
| `regimeIVA` | `RegimeIVA` | configurável por item |

### 4.3 Value Objects

- **`ClienteSnapshot`**: `nome` (obrigatório), `nif` (opcional; se presente,
  validado pelo validador de NIF do Shared Kernel), `morada` (opcional).
  Imutável, sem FK ao `Doente` (snapshot).
- **`TipoLinha`** (enum): `CONSULTA`, `DISPENSA`, `EXAME_ANALISE`,
  `ESTUDO_IMAGEM`, `PROCEDIMENTO_CIRURGICO`.
- **`RegimeIVA`** (enum): `ISENTO` (saúde) | `STANDARD` (14%). Configurável por
  item (CLAUDE.md §8). A taxa é derivada do regime.
- **`Totais`**: `subtotal`, `totalIVA`, `total` (todos `moeda.AOA`).

### 4.4 Comportamento

- `NovaFactura(cliente ClienteSnapshot, episodioID string) (*Factura, error)` —
  cria RASCUNHO sem itens. Valida cliente (nome obrigatório; NIF se presente) e
  episodioID.
- `AdicionarItem(descricao string, tipo TipoLinha, operacaoID string, quantidade int, precoUnitario moeda.AOA, regime RegimeIVA) error` —
  só em RASCUNHO. Invariantes: descrição obrigatória; `tipo` válido; `quantidade > 0`;
  preço ≥ 0; `operacaoID` obrigatório para DISPENSA/EXAME_ANALISE/ESTUDO_IMAGEM/
  PROCEDIMENTO_CIRURGICO (CONSULTA liga-se ao `episodioID`).
- `RemoverItem(itemID string) error` — só em RASCUNHO. Uma factura sem itens é
  válida em rascunho.
- `Totais() Totais` — cálculo derivado (ver 4.5).
- `Snapshot() SnapshotFactura` / `ReconstruirFactura(SnapshotFactura) *Factura`.

### 4.5 Cálculo de IVA e totais

- Aritmética **inteira em cêntimos** (nunca vírgula flutuante).
- Por linha: `subtotalLinha = quantidade × precoUnitario`.
- IVA por linha: `ISENTO → 0`; `STANDARD → arredondamento meia-acima de 14%`:
  `ivaCentimos = (subtotalCentimos × 14 + 50) / 100`.
- Totais da factura = soma **por linha** dos subtotais e dos IVA (arredondar por
  linha e somar, não somar e arredondar — prática fiscal). `total = subtotal + totalIVA`.

## 5. Persistência

### 5.1 Migração `migrations/financeiro/0001_facturas.sql` (forward-only)

- Schema `financeiro`.
- Tabela `facturas`: `id uuid pk`, `estado`, `cliente_nome`, `cliente_nif` (null),
  `cliente_morada` (null), `episodio_id uuid`, `criado_em`, `actualizado_em`.
- Tabela `itens_factura`: `id uuid pk`, `factura_id uuid` (FK → `facturas`, **FK
  intra-BC permitida**), `descricao`, `tipo`, `operacao_id uuid` (null para
  CONSULTA), `quantidade int`, `preco_unitario_centimos bigint`, `regime_iva`,
  `ordem int` (preserva a ordem das linhas).
- CHECKs: `estado IN ('RASCUNHO','EMITIDA','ANULADA')`; `quantidade > 0`;
  `preco_unitario_centimos >= 0`; `tipo IN (...)`; `regime_iva IN ('ISENTO','STANDARD')`.
- **Sem FK cross-context** (`episodio_id`, `operacao_id` são uuid nus).
- Registo em `schema_migrations` (padrão existente).

### 5.2 Porta `RepositorioFacturas`

```
Guardar(ctx, *Factura) error          // upsert transaccional factura + itens,
                                       // guarda estado = RASCUNHO; substitui itens
                                       // (delete + insert) numa única tx
ObterPorID(ctx, id string) (*Factura, error)
ListarPorEpisodio(ctx, episodioID string) ([]ResumoFactura, error)
```

Implementação pgx no adaptador `pgrepo`.

## 6. Aplicação + HTTP

### 6.1 Casos de uso (`internal/application/financeiro/`)

Fakes nos testes (não mocks). Todas as escritas **auditadas** (`financeiro.factura.*`,
retenção 10 anos):

- `CriarFactura(cliente, episodioID)` → RASCUNHO. Audita `financeiro.factura.criada`.
- `AdicionarItemFactura(facturaID, linha)` → audita `financeiro.factura.item.adicionado`.
- `RemoverItemFactura(facturaID, itemID)` → audita `financeiro.factura.item.removido`.
- `ObterFactura(facturaID)`.
- `ListarFacturasPorEpisodio(episodioID)`.

Portas consumidas: `RepositorioFacturas`, `Auditor` (existente), relógio.

### 6.2 HTTP (`internal/adapters/http/…`, Gin, RFC 7807 pt-AO)

| Método | Rota | RBAC |
|--------|------|------|
| POST | `/api/v1/financeiro/facturas` | Tesoureiro |
| POST | `/api/v1/financeiro/facturas/:id/itens` | Tesoureiro |
| DELETE | `/api/v1/financeiro/facturas/:id/itens/:itemID` | Tesoureiro |
| GET | `/api/v1/financeiro/facturas/:id` | Tesoureiro / Director / Auditor |
| GET | `/api/v1/financeiro/facturas?episodio_id=` | Tesoureiro / Director / Auditor |

Mapeamento de erros: `Validacao → 400`, `RegraNegocio → 422`, `Conflito → 409`,
`NaoEncontrado → 404` (padrão existente).

## 7. Testes e gates de cobertura

- **Domínio ≥85%**: construção, `AdicionarItem`/`RemoverItem`, invariantes
  (quantidade, preço, operacaoID por tipo, estado), cálculo de IVA (ISENTO,
  STANDARD, arredondamento por linha), `Snapshot`/`Reconstruir`.
- **Aplicação ≥75%**: casos de uso com fakes de repositório e auditor.
- **Adaptadores ≥60%**: `pgrepo` por integração real contra Postgres (prova o
  upsert transaccional e os CHECKs — ex. 23514); handlers HTTP.
- `go-arch-lint` verde (domínio sem imports de infra).

## 8. Decomposição M4 (recapitulação)

- **ADR-039** (esta fatia) — Domínio Financeiro: Factura + ItemFactura + Cliente
  snapshot, tipos de linha, IVA e totais, RASCUNHO + papel Tesoureiro.
- **ADR-040** — Emissão: cadeia hash SHA-256 + numeração sequencial por série +
  imutabilidade (+ decisão MFA Tesoureiro).
- **ADR-041** — Anulação por nova factura + pagamentos.
- **ADR-042** — SAF-T-AO (XML + XSD).
- **ADR-043** — Integração EMIS Multicaixa (sandbox, webhook).

## 9. Antipadrões a evitar (do M4)

- ❌ `UPDATE facturas SET …` para "corrigir" (imutabilidade absoluta — anular +
  reemitir, do ADR-041).
- ❌ Calcular hash em service em vez de invariante do agregado (ADR-040).
- ❌ FK cross-BC para `episodio_id`/`operacao_id`/`dispensa_id` — snapshot + ID lógico.
- ❌ Audit log financeiro com retenção < 10 anos.
- ❌ Linguagem misturada (PT/EN/BR).
