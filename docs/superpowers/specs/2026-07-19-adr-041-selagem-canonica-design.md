# ADR-041 — Selagem canónica: enquadramento injectivo e âmbito do selo

> Desenho validado. Sprint 16, a seguir ao ADR-040 (emissão da Factura), entregue e
> publicado em `origin/main` em `8b5bf4d`.

## 1. Porque existe esta fatia

A ADR-040 diferiu duas decisões sobre o hash da `Factura` — R1 (separadores não
escapados no digest) e R2 (`OperacaoID`/`ItemFactura.ID` fora do selo) — com a nota de
que **só são revisíveis antes da primeira emissão em produção**, porque alterá-las
depois quebra retroactivamente a cadeia de todas as facturas emitidas.

Ao instruir esta fatia verificou-se que o R1 não é o risco teórico que a ADR-040
descreve. São dois defeitos concretos de evidência de adulteração.

### 1.1 Colisão confirmada (R1)

A ADR-040 justifica o adiamento com *"o risco prático é baixo (as descrições vêm de
catálogo)"*. **É factualmente falso.** `ItemFactura.Descricao` chega no corpo do pedido
HTTP (`financeiro_handler.go`, campo `descricao`) e o domínio valida-a apenas com
`strings.TrimSpace` e não-vazio (`factura.go:172-173`). É texto livre sob controlo de
quem chama, e `|` e `\n` entram intactos no digest.

A colisão foi **produzida contra o código real**, não deduzida:

```
A: 2 linhas, total 5000 | hash cac4fec4b7e103a6f232cb2eacb7dd15c82700b4e1205f93eb20377664bf1bd4
B: 1 linha,  total 5000 | hash cac4fec4b7e103a6f232cb2eacb7dd15c82700b4e1205f93eb20377664bf1bd4
```

- **A** = duas linhas: `("Z", CONSULTA, 1, 0, ISENTO)` e `("W", CONSULTA, 1, 5000, ISENTO)`
- **B** = uma linha: `("Z|CONSULTA|1|0|ISENTO\n1|W", CONSULTA, 1, 5000, ISENTO)`

Duas facturas materialmente diferentes partilham selo e total. A cadeia não as
distingue; um auditor que recalcule o hash aceita qualquer uma como o conteúdo selado.

**Porque é que os totais não bastam.** O canónico inclui subtotal/IVA/total, o que
restringe a família de colisões: o preço da linha forjada tem de igualar o da última
linha imitada, obrigando as restantes a somar zero. Mas
`preco_unitario_centimos >= 0` (migração `0001`) **admite linhas a preço zero**, que
fornecem exactamente isso. A defesa existente é **incidental**, não desenhada.

**Alcance real.** Os montantes continuam presos — isto não permite forjar um total. O
que permite é alterar **o que o documento diz** mantendo o selo válido: acrescentar ou
remover uma linha descritiva a preço zero, ou re-partir texto entre linhas. Num
documento fiscal retido dez anos para inspecção da AGT, isso é uma falha de evidência
de adulteração.

### 1.2 Identidade do cliente parcialmente fora do selo (novo, não previsto na ADR-040)

`ClienteSnapshot` tem `Nome` (obrigatório), `NIF` (opcional) e `Morada`. O canónico sela
**apenas o NIF**.

Num *consumidor final* — doente que não fornece NIF, o caso comum numa clínica — a
identidade selada é a string vazia, e o `Nome` é a única identificação que o documento
carrega. Nessas facturas, **o destinatário do documento é livremente alterável com o
selo intacto**.

Isto não consta do R1 nem do R2. Foi encontrado ao inventariar o que mais o selo omite,
e é mais material do que uma chave substituta: "emitida a quem" é conteúdo fiscal.

## 2. Decisão

### 2.1 Regra de enquadramento (normativa)

**Todo o campo de texto leva prefixo de comprimento em bytes; os inteiros vão nus.**

Notação: `p(s)` = `len(s)` em bytes, `:`, e a seguir `s` literal. Exemplo:
`p("Z|CONSULTA")` = `10:Z|CONSULTA`.

**Digest das linhas** — uma linha por item, na ordem do agregado:

```
{ordem}|{p(descricao)}|{p(tipo)}|{quantidade}|{precoUnitarioCentimos}|{p(regimeIVA)}|{p(operacaoID)}\n
```

**Canónico da factura:**

```
{p(serie)}|{sequencial}|{p(dataEmissaoRFC3339UTC)}|{p(clienteNome)}|{p(clienteNIF)}|{p(clienteMorada)}|{subtotalCentimos}|{ivaCentimos}|{totalCentimos}|{p(digestLinhas)}|{p(hashAnterior)}
```

`hash = SHA256(canónico)`, em hexadecimal minúsculo.

**A regra é deliberadamente cega.** Não distingue campos "arriscados" de "seguros" — e é
essa a questão. O defeito de 1.1 existe precisamente porque alguém julgou que as
descrições eram seguras e escreveu esse juízo na ADR como facto. Uma regra sem excepções
não pode ser mal julgada por quem vier a seguir, e especifica-se numa frase a quem tenha
de a reproduzir noutra linguagem.

**Campos ausentes canonicalizam-se como `0:`** — nunca `null` nem `<nil>`, mantendo a
disciplina que a ADR-040 já fixou para o NIF. Aplica-se a `clienteNIF`, `clienteMorada`,
`operacaoID` e ao `hashAnterior` da primeira factura da série.

### 2.2 Âmbito do selo

**Entram:** `clienteNome`, `clienteMorada` e `operacaoID` (proveniência cross-context da
linha), além de tudo o que a ADR-040 já selava.

**Não entra: `ItemFactura.ID`.** É chave substituta sem significado fiscal; selá-la
ataria o documento fiscal a um detalhe de implementação da base de dados, sem acrescentar
nada que um auditor reconheça. Fica registado como decisão consciente, não como omissão.

### 2.3 Enquadramento escolhido, e as alternativas rejeitadas

Rejeitou-se **`%q` (aspas e escape do Go)**: é legível e idiomático, mas ata o formato
canónico fiscal às regras de `strconv.Quote`, incluindo o tratamento de não-ASCII e de
UTF-8 inválido. Este artefacto tem de ser reproduzível por terceiros — possivelmente
pelas ferramentas da própria AGT, numa certificação, daqui a dez anos. "Replicar o
quoting do Go" convida exactamente à divergência subtil que o vector dourado existe para
apanhar.

Rejeitou-se **SHA-256 por campo, concatenado**: é injectivo e independente de linguagem,
mas o canónico deixa de ser inspeccionável como texto, o que dificulta a auditoria manual
sem ganho sobre o prefixo de comprimento.

O prefixo de comprimento é injectivo por construção, independente de linguagem, e
especifica-se numa frase: *conta os bytes, depois lê esse número de bytes*.

## 3. Âmbito da alteração

**Alteração puramente de domínio. Sem migração.** `cliente_nome`, `cliente_morada` e
`operacao_id` já são colunas persistidas (verificado contra o schema real), pelo que o
hash continua reproduzível a partir do que está gravado.

| Ficheiro | Alteração |
|---|---|
| `internal/domain/financeiro/factura.go` | `digestLinhas` e `HashDe` — o único sítio onde a canonicalização vive |
| `internal/domain/financeiro/factura_test.go` | Vector dourado novo; testes por campo recém-selado; teste de regressão da colisão |
| `tests/integration/facturas_test.go` | Série por corrida no teste da cadeia (ver §5) |
| `adrs/ADR-041-selagem-canonica.md` | ADR nova |
| `adrs/ADR-040-emissao-factura.md` | R1 e R2 marcados como resolvidos, a apontar para a ADR-041 |

**Não muda:** `VerificarCadeia` (a estrutura da verificação é a mesma; só a função de
hash que ela invoca muda), a serialização por `FOR UPDATE`, os triggers de imutabilidade,
o bloqueio optimista, as rotas, o RBAC, o schema.

## 4. O vector dourado

O vector dourado **vai mudar de valor, e isso é o comportamento correcto** — ele existe
para gritar quando a canonicalização muda. Vai falhar, e actualiza-se deliberadamente,
registando o novo valor na ADR-041.

**Instrução vinculativa para o plano e para quem implementar:** o valor novo tem de ser
**calculado a partir da implementação real** e conferido contra a fixtura abaixo. Um
protótipo desta fatia produziu `6f5a535c960536cc759cd10d589599d916b58f23087c5faa2cc5c80ff5f59ff9`
para esta fixtura. **Esse valor é uma conferência, não uma autoridade**: se a
implementação real produzir outro, isso é sinal para investigar a divergência — não para
ajustar a constante ao que o código emite.

A razão é histórica e específica. Na ADR-040, um valor dourado foi fornecido a partir de
uma fixtura descrita em prosa e sub-especificada; custou 736 tentativas falhadas e uma
tarefa bloqueada. O implementador parou em vez de ajustar a constante, e fez bem —
ajustá-la teria produzido um teste que afirma a implementação em vez da especificação,
que é pior do que não ter teste.

Fixtura (a mesma da ADR-040, para a comparação ser directa):

```go
c, _ := fin.NovoClienteSnapshot("Sol", "", "")          // nome "Sol", sem NIF, sem morada
f, _ := fin.NovaFactura(c, "11111111-1111-1111-1111-111111111111")
f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeCentimos(50000), fin.RegimeIsento)
f.AdicionarItem("Paracetamol", fin.LinhaDispensa, "22222222-2222-2222-2222-222222222222", 2,
    moeda.DeCentimos(1000), fin.RegimeStandard)
m := time.Date(2026, 7, 18, 10, 0, 0, 123456789, time.UTC)
f.Emitir("2026", 7, "abc", m)
```

Totais esperados: subtotal 52000, IVA 280 (`(2000*14+50)/100`, meia-acima), total 52280.

## 5. Consequência operacional na base de dados de desenvolvimento

`TestEmitirFactura_Concorrente_SemBuracos` (`tests/integration/facturas_test.go:854-875`)
lista **toda** a série `2999` e corre `VerificarCadeia` sobre ela. Essa série acumula
facturas de corridas anteriores, com hashes no formato antigo, e as facturas EMITIDA são
irremovíveis por desenho (trigger de imutabilidade). Depois da mudança de formato, o
`VerificarCadeia` recalcularia esses elos antigos com a função nova e acusaria — com
razão — uma quebra.

**Correcção: o teste passa a usar uma série própria por corrida**, verificando apenas a
cadeia que ele próprio criou. Fica auto-contido e deixa de herdar história de formato
anterior. É a mesma classe de correcção que a Task 6 da ADR-040 já aplicou ao passar a
contar relativamente à linha de base da série.

Com isso, **não é preciso recriar a base de dados de desenvolvimento**: as facturas
antigas da série `2999` deixam de ser verificadas por qualquer teste e tornam-se inertes
em vez de venenosas. As 3 facturas permanentes da série `2026`
(`FAC 2026/09999997`, `09999998`, `09999999`) têm `hash = 'abc'` literal — são fixturas
de imutabilidade, nunca passam por `VerificarCadeia`, e não são afectadas.

## 6. Testes

- **A colisão de §1.1 torna-se teste de regressão permanente**, agora a exigir hashes
  **diferentes** para A e B. É o teste de maior valor desta fatia: falha se alguém voltar
  a remover o enquadramento.
- **Vector dourado novo**, com a fixtura de §4.
- **Um teste por campo recém-selado** — `clienteNome`, `clienteMorada`, `operacaoID`:
  alterar o campo tem de alterar o hash. Hoje nenhum destes altera, pelo que cada teste
  falha antes da implementação e passa depois (TDD genuíno, não tautológico).
- **Ausente vs vazio**: uma factura sem morada e uma com morada `""` produzem o mesmo
  hash; o `0:` não introduz ambiguidade.
- **Reconstrução** de factura emitida continua a fechar a cadeia (`ReconstruirFactura` →
  `HashDe` devolve o mesmo elo).
- **Integração**: a suite passa contra a BD de desenvolvimento e contra uma criada de
  raiz, duas corridas seguidas.

## 7. Gates

Domínio ≥85%, aplicação ≥75%, adaptadores ≥60%. `go build`, `go vet`, `gofmt -l`,
`go test ./... -race`, integração, `golangci-lint run` (0 issues) e `go-arch-lint check`
(**exit 0**, zero notices).

A nota sobre o `go-arch-lint` é deliberada: na sprint anterior um `notice` foi descrito
como pré-existente por vários revisores — era-o relativamente a cada tarefa, mas não
relativamente ao `main` — e teria partido o CI no push, porque o comando sai com código 1.
Verificar o **código de saída**, e contra o `merge-base`, não contra o ramo.

## 8. Fora do âmbito

- **Anulação de factura** (transição para `ANULADA`, nota de crédito) — fatia própria,
  vinculada pelo R5 da ADR-040: não pode apagar nem renumerar.
- **Pagamentos** (parciais, métodos, saldo) e EMIS Multicaixa — fatia própria.
- **R3** (bypass de MFA em 11 dos 14 grupos de rotas), **R6** (factura pode nascer
  EMITIDA por INSERT directo) e **R7** (papel da aplicação é dono das tabelas fiscais) —
  continuam registados na ADR-040 e pertencem à fatia de segurança já prevista.
- **Submissão SAF-T-AO** e certificação junto da AGT.
