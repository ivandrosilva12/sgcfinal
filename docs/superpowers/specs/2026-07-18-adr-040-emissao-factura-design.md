# Desenho — ADR-040: Emissão da Factura (cadeia hash, numeração e imutabilidade)

> Sprint 15 · Marco M4 — Financeiro
> Precede o plano de implementação e a ADR-040.
> Fonte normativa: REG-001 §3.2/§3.4, DDM-001 v2.0 §5.2, `marcos/m4-financeiro.md`.

---

## 1. Contexto

A ADR-039 entregou o agregado `Factura` em `RASCUNHO`: linhas com snapshot e id
lógico da operação de origem, IVA por linha com arredondamento meia-acima, total
autoritário no domínio, persistência por upsert transaccional e RBAC. Ficou
deliberadamente de fora a emissão — a peça de risco regulatório.

Uma factura em `RASCUNHO` não tem valor legal: é puramente interna. Esta fatia é o
que a torna um documento fiscal. O enum `EstadoFactura` e a `CHECK` da BD já
antecipam `EMITIDA` e `ANULADA`, mas hoje só `RASCUNHO` é alcançável — não existe
método `Emitir()`, e uma busca por `hash`, `sha256`, `numero`, `serie` ou `versao`
em todo o `domain`, `application` e `migrations` do financeiro devolve uma única
ocorrência: um comentário a dizer que a emissão fica para esta ADR.

### Obrigações normativas (REG-001 §3.2)

| Obrigação | Como é satisfeita nesta fatia |
|---|---|
| Numeração sequencial única, contínua, **por série** | Tabela `series` com `FOR UPDATE`; sem buracos por construção |
| Cadeia hash SHA-256, **quebra detectável** | `hashAnterior` + `hash` calculados no agregado; `VerificarCadeia` |
| Imutabilidade — emitidas não alteráveis | Guarda de domínio **e** trigger na BD (defesa em profundidade) |
| Retenção 10 anos | Já satisfeita: nada apaga facturas; o trigger impede `DELETE` |

O REG-001 **não** especifica o conteúdo canónico do hash — exige apenas "hash
SHA-256; quebra detectável". O formato é, portanto, decisão desta ADR, e fica
documentado aqui para poder ser reproduzido por um auditor da AGT.

### Decisões de brainstorming

1. **Assinatura digital diferida.** O DDM-001 §5.2.1 modela um campo `assinatura`
   (chave privada da clínica), mas o REG-001 §3.2 não a lista nas obrigações e o
   blueprint não a põe na Sprint 15. Traz gestão de chaves — geração,
   armazenamento, rotação, backup — multiplicada por instalação, dado o on-premise
   por clínica. É um sub-projecto de segurança por direito próprio. Mantém-se a
   fatia fina, como na ADR-039. A coluna **não** é criada vazia: coluna morta dá
   ilusão de conformidade a quem a leia sem contexto.
2. **Tesoureiro passa a papel sensível.** A emissão é irreversível e com efeito
   fiscal; a ERRATA-002 marcou explicitamente a decisão como provisória, a rever
   aqui. Reutiliza-se o mecanismo existente (`EhSensivel` + middleware
   `MFAObrigatoria` + realm Keycloak) sem código novo. O custo de UX é baixo — o
   MFA estabelece-se no login, uma vez por sessão, não por operação.
3. **Serialização por tabela de séries com `FOR UPDATE`.** Rejeitadas: `SEQUENCE`
   do Postgres (não reverte com a transacção, logo deixa buracos permanentes —
   viola o REG-001 §3.2 e o blueprint lista corrigir buracos como antipadrão) e
   advisory lock com `MAX(sequencial)` (funciona, mas deixa a serialização
   invisível no esquema e não materializa a cabeça da cadeia).
4. **Hash sobre cabeçalho + digest das linhas.** Rejeitadas: fórmula mínima ao
   estilo do SAF-T PT (não sela conteúdo de linha — permite reescrever descrições
   ou compensar quantidade e preço mantendo o total) e JSON canónico completo
   (qualquer evolução do modelo torna facturas antigas irreverificáveis, dando
   falsos positivos de quebra ao longo dos 10 anos de retenção).

---

## 2. Âmbito e fronteiras

**Dentro:**

- Transição `RASCUNHO → EMITIDA` com número legal, data de emissão e elo da cadeia.
- Numeração sequencial por série, sem buracos, sob concorrência.
- Cadeia hash SHA-256 encadeada, com função de verificação.
- Imutabilidade da factura emitida: guarda no domínio e trigger na BD.
- Bloqueio optimista no rascunho (dívida declarada da ADR-039).
- `PapelTesoureiro` passa a sensível.

**Fora:**

| Tema | Fica para |
|---|---|
| Assinatura digital (chave privada da clínica) | ADR futura de gestão de chaves |
| Anulação por nota de crédito | ADR-041 |
| Pagamentos (parcial, múltiplos métodos, EMIS) | ADR-041 |
| SAF-T-AO (XML, XSD, submissão) | ADR-042 |
| Agendamento do cron diário de verificação | Infraestrutura, após esta fatia |
| Campos do DDM ainda não modelados (`tipo`, `desconto`, `dataVencimento`, `moeda`, seguradora) | Fatias seguintes do M4 |

**Nota sobre a verificação da cadeia.** O REG-001 §3.4 exige cadeia "validada
continuamente (cron diário)". Entrega-se agora a *função de verificação* — pura,
no domínio — e o caso de uso de leitura que a expõe. O agendamento fica para
depois. A razão é que sem verificador a "quebra detectável" do §3.2 fica por
provar; com ele, o cron passa a ser apenas infraestrutura sobre lógica já testada.

---

## 3. Domínio (`internal/domain/financeiro/`)

### 3.1 Value Object `NumeroFactura`

Formato legal AGT, conforme DDM-001 §5.2.1: `"FAC 2026/00012345"` — prefixo fixo
`FAC`, série, barra, sequencial com 8 dígitos à esquerda. Formatação e parsing,
com o parsing a rejeitar formatos malformados (`CategoriaValidacao`).

### 3.2 Campos novos em `Factura`

```go
numero        NumeroFactura  // vazio enquanto RASCUNHO
serie         string         // ex: "2026"
sequencial    int            // ex: 12345
dataEmissao   time.Time      // zero enquanto RASCUNHO
hash          string         // SHA-256 hex desta factura
hashAnterior  string         // hash da factura imediatamente anterior; "" na primeira
versao        int            // bloqueio optimista do rascunho
```

### 3.3 Comportamento

```go
func (f *Factura) Emitir(serie string, sequencial int, hashAnterior string, momento time.Time) error
```

Guardas, por ordem:

1. `estado != RASCUNHO` → `CategoriaConflito` (409). Emitir duas vezes é conflito.
2. Sem linhas → `CategoriaRegraNegocio` (422). Não se emite factura vazia.
3. `serie` vazia ou `sequencial <= 0` → `CategoriaValidacao` (400).

Em sucesso: fixa `serie`, `sequencial`, `numero`, `dataEmissao = momento`,
`hashAnterior`, calcula `hash` e transita para `EMITIDA`.

O hash é calculado **pelo agregado**, nunca por um serviço — antipadrão explícito
do M4 ("calcular hash em service quando devia ser invariante do agregado"). O
`AdicionarItem` e o `RemoverItem` já guardam `estado == RASCUNHO`, pelo que a
imutabilidade das linhas vem de graça.

### 3.4 Conteúdo canónico do hash

```
digestLinhas = SHA256( join("\n", para cada linha por ordem:
    "{ordem}|{descricao}|{tipo}|{quantidade}|{precoUnitarioCentimos}|{regimeIVA}" ) )

hash = SHA256(
    "{serie}|{sequencial}|{dataEmissaoRFC3339UTC}|{clienteNIF}|"
    "{subtotalCentimos}|{ivaCentimos}|{totalCentimos}|{digestLinhas}|{hashAnterior}" )
```

Ambos em hexadecimal minúsculo. Três regras de canonicalização, sem as quais o
hash deixa de ser reproduzível daqui a dez anos:

- **`dataEmissaoRFC3339UTC`**: convertida a UTC e **truncada ao segundo**, formatada
  com `time.RFC3339` (nunca `RFC3339Nano`). A precisão de sub-segundo do relógio e
  do `timestamptz` do Postgres não coincide; deixá-la entrar tornaria o hash
  irreproduzível a partir da linha relida da BD.
- **Montantes**: sempre em cêntimos inteiros (`int64`), nunca em vírgula flutuante
  nem em texto decimal formatado.
- **`clienteNIF` ausente**: string vazia, nunca a palavra `null` nem o literal
  `<nil>`.

Na primeira factura da cadeia, `hashAnterior = ""` (string vazia, não `NULL`).

Esta lista é fixa e documentada: não deriva do esquema da BD, pelo que acrescentar
colunas em fatias futuras não altera hashes já calculados.

### 3.5 Verificação

```go
func VerificarCadeia(facturas []SnapshotFactura) error
```

Função pura. Recebe os snapshots por ordem de sequencial dentro da série,
recalcula cada hash e confirma que o `hashAnterior` de cada um é o `hash` do
anterior. Devolve erro identificando o **primeiro** elo quebrado (número da
factura e natureza da quebra: hash recalculado diverge, ou encadeamento errado).
Detecta ainda buracos na sequência.

---

## 4. Persistência

### 4.1 Migração `migrations/financeiro/0002_emissao_facturas.sql` (forward-only)

Em `financeiro.facturas`:

- `numero text`, `serie text`, `sequencial integer`, `data_emissao timestamptz`,
  `hash text`, `hash_anterior text` — todas nullable (o rascunho não as tem).
- `versao integer NOT NULL DEFAULT 0`.
- `UNIQUE (numero)` e `UNIQUE (serie, sequencial)`.
- `CHECK` de coerência estado↔campos: em `EMITIDA`, `numero`/`serie`/`sequencial`/
  `data_emissao`/`hash`/`hash_anterior` têm de ser não-nulos; em `RASCUNHO`, têm de
  ser nulos. Note-se que `hash_anterior` é não-nulo mas **pode ser string vazia** —
  é esse o valor na primeira factura de cada série, e distingue-se de `NULL`
  (rascunho, ainda sem posição na cadeia).

Tabela nova:

```sql
CREATE TABLE financeiro.series (
    serie             text        PRIMARY KEY,
    ultimo_sequencial integer     NOT NULL DEFAULT 0,
    ultimo_hash       text        NOT NULL DEFAULT '',
    actualizado_em    timestamptz NOT NULL DEFAULT now()
);
```

Trigger de imutabilidade, espelhando `auditoria.impedir_mutacao`:

```sql
BEFORE UPDATE OR DELETE ON financeiro.facturas
FOR EACH ROW WHEN (OLD.estado <> 'RASCUNHO')
EXECUTE FUNCTION financeiro.impedir_mutacao_factura();
-- RAISE EXCEPTION ... USING ERRCODE = 'restrict_violation'
```

A condição incide sobre `OLD.estado`: a emissão parte de um rascunho e passa;
qualquer escrita sobre uma factura já emitida é rejeitada pela BD, aconteça o que
acontecer na aplicação.

### 4.2 `Guardar` — bloqueio optimista

O `UPDATE` do rascunho passa a `WHERE id=$1 AND estado='RASCUNHO' AND versao=$v`,
com `SET versao = versao + 1`. Zero linhas afectadas → `CategoriaConflito` (409),
distinguindo na mensagem "já não está em rascunho" de "foi alterada entretanto".
Isto fecha o lost-update declarado como dívida na ADR-039 §Riscos.

### 4.3 `Emitir` — alocação serializada

Uma transacção única no repositório:

```sql
INSERT INTO financeiro.series (serie) VALUES ($1) ON CONFLICT DO NOTHING;
SELECT ultimo_sequencial, ultimo_hash FROM financeiro.series
  WHERE serie = $1 FOR UPDATE;
-- domínio: f.Emitir(serie, ultimo_sequencial+1, ultimo_hash, agora)
UPDATE financeiro.facturas
   SET estado='EMITIDA', numero=$n, serie=$s, sequencial=$q, data_emissao=$d,
       hash=$h, hash_anterior=$ha, versao = versao + 1
 WHERE id = $1 AND estado = 'RASCUNHO' AND versao = $v;
UPDATE financeiro.series SET ultimo_sequencial = $n, ultimo_hash = $h,
       actualizado_em = now() WHERE serie = $1;
```

O `INSERT ... ON CONFLICT DO NOTHING` antes do `FOR UPDATE` resolve a viragem de
ano: na primeira emissão de uma série nova a linha ainda não existe, e criá-la
fora do lock seria ela própria uma corrida.

Se a transacção reverte por qualquer motivo, o contador não avança — a ausência de
buracos é uma propriedade da estrutura, não uma verificação a posteriori.

---

## 5. Aplicação + HTTP

### 5.1 Casos de uso (`internal/application/financeiro/`)

| Caso | Papel | Auditoria |
|---|---|---|
| `CasoEmitirFactura` | Tesoureiro | `financeiro.factura.emitida` (número, hash) |
| `CasoVerificarCadeia` | Tesoureiro, Director, Auditor | leitura, não auditada |

A alocação de número e elo vive no adaptador (é serialização, camada 3); o cálculo
do hash vive no domínio (é invariante, camada 1). O caso de uso orquestra sem
conhecer nenhum dos dois mecanismos.

### 5.2 HTTP (Gin, RFC 7807 pt-AO)

```
POST /facturas/:fid/emitir          escrita   200 · 400 · 404 · 409 · 422
GET  /facturas/cadeia/verificacao   leitura   200 · 403
```

`409` cobre tanto "já emitida" como conflito de versão; `422` cobre factura sem
linhas. A resposta da emissão devolve `numero`, `dataEmissao` e `hash`.

## 6. Identidade

`EhSensivel()` passa a incluir `PapelTesoureiro`. O `TestPapelTesoureiroNaoSensivel`
inverte-se e passa a afirmar o contrário, mantendo a cobertura explícita da
decisão. Seed e realm Keycloak acompanham. A ERRATA-002 é actualizada com a
resolução da sua própria nota provisória.

---

## 7. Testes e gates de cobertura

**Domínio (≥85%)** — hash determinístico e estável para a mesma entrada; hashes
diferentes ao alterar qualquer campo selado, incluindo descrição de linha e ordem
das linhas; `Emitir` só a partir de `RASCUNHO`; `Emitir` recusa factura sem linhas;
`VerificarCadeia` detecta hash adulterado, encadeamento errado e buraco na
sequência, identificando sempre o primeiro elo quebrado; `NumeroFactura` formata e
faz parsing ida-e-volta.

**Aplicação (≥75%)** — fakes, RBAC (só Tesoureiro emite; Auditor e Director lêem a
verificação mas não emitem), evento de auditoria emitido com número e hash.

**Integração real contra Postgres (≥60% adapters)** — três provas que só valem
contra a BD verdadeira:

1. **Numeração sob concorrência.** N emissões simultâneas na mesma série;
   afirmar sequenciais exactamente `1..N`, sem lacunas nem repetições, e cadeia
   íntegra ponta a ponta. É o teste central desta fatia.
2. **Imutabilidade imposta pela BD.** `UPDATE` directo numa factura emitida tem de
   falhar com `restrict_violation`, provando o trigger e não apenas a guarda de
   domínio.
3. **Lost-update prevenido.** Duas escritas concorrentes sobre o mesmo rascunho:
   uma passa, a outra recebe 409.

Segue o padrão da Sprint 13, em que o `CHECK` de segregação foi provado por
integração real (SQLSTATE 23514) e não por unitário.

---

## 8. Antipadrões a evitar (do M4)

- Calcular o hash num serviço quando é invariante do agregado.
- Desbloquear a série para "corrigir" um buraco.
- `UPDATE facturas SET ...` numa factura emitida.
- Editar factura errada em vez de anular e reemitir (ADR-041).

## 9. Sequência no M4

| Fatia | ADR |
|---|---|
| Agregado `Factura` em RASCUNHO | ADR-039 (entregue) |
| **Emissão, cadeia hash, numeração, imutabilidade** | **ADR-040 (esta)** |
| Anulação por nota de crédito, pagamentos | ADR-041 |
| SAF-T-AO (XML, XSD, submissão) | ADR-042 |
