# ADR-041 — Selagem canónica: enquadramento injectivo e âmbito do selo

- **Estado:** Aceite
- **Data:** 2026-07-19
- **Marco/Sprint:** M4 — Financeiro (Sprint 16)
- **Fontes:** desenho em
  `docs/superpowers/specs/2026-07-19-adr-041-selagem-canonica-design.md`; plano em
  `docs/superpowers/plans/2026-07-19-adr-041-selagem-canonica.md`; REG-001 §3.2;
  ADR-040 (emissão da Factura — R1, R2); ADR-039 (agregado `Factura`).

## Contexto

A ADR-040 fixou o formato canónico do hash da `Factura` e registou duas decisões
conscientes sobre o seu conteúdo — R1 (separadores não escapados no digest das
linhas) e R2 (`OperacaoID` e `ItemFactura.ID` fora do digest) — ambas com o mesmo
prazo duro: **só são revisíveis antes da primeira emissão em produção**, porque
alterar a canonicalização depois disso quebra retroactivamente o elo de todas as
facturas já emitidas.

Esta fatia exerce esse prazo. Ao instrui-la, verificou-se que o R1 **não é o risco
teórico que a ADR-040 descreve**, e que existe um segundo defeito que nem o R1 nem
o R2 previram.

### A colisão é real, não teórica

A ADR-040 justifica o adiamento do R1 com a frase *"o risco prático é baixo porque
as descrições vêm de catálogo e não de texto livre do utilizador"*. **Essa
justificação é factualmente falsa.** `ItemFactura.Descricao` chega no corpo do
pedido HTTP (`internal/adapters/http/financeiro_handler.go`, campo `descricao` do
JSON) e o domínio valida-a apenas com `strings.TrimSpace` e não-vazio
(`internal/domain/financeiro/factura.go`). É texto livre sob controlo de quem
chama, e `|` e `\n` entravam intactos no digest.

A colisão foi **produzida contra o código real**, não deduzida. Antes desta fatia,
uma factura de duas linhas e outra de uma linha produziam o **mesmo hash e o mesmo
total**:

```
A: 2 linhas, total 5000 | hash cac4fec4b7e103a6f232cb2eacb7dd15c82700b4e1205f93eb20377664bf1bd4
B: 1 linha,  total 5000 | hash cac4fec4b7e103a6f232cb2eacb7dd15c82700b4e1205f93eb20377664bf1bd4
```

- **A** = duas linhas: `("Z", CONSULTA, 1, 0, ISENTO)` e `("W", CONSULTA, 1, 5000, ISENTO)`
- **B** = uma linha: `("Z|CONSULTA|1|0|ISENTO\n1|W", CONSULTA, 1, 5000, ISENTO)`

Duas facturas materialmente diferentes partilhavam selo e total. A cadeia não as
distinguia; um auditor que recalculasse o hash aceitaria qualquer uma como o
conteúdo selado.

**Porque é que os totais não bastavam.** O canónico inclui subtotal, IVA e total, o
que restringe a família de colisões: o preço da linha forjada tem de igualar o da
última linha imitada, obrigando as restantes a somar zero. Mas a CHECK
`preco_unitario_centimos >= 0` (`migrations/financeiro/0001_facturas.sql`)
**admite linhas a preço zero**, que fornecem exactamente a folga necessária. A
defesa existente era **incidental, não desenhada** — e uma defesa incidental não é
uma defesa que se possa registar numa ADR como mitigação.

**Alcance real.** Os montantes continuavam presos: isto não permitia forjar um
total. O que permitia era alterar **o que o documento diz** mantendo o selo
válido — acrescentar ou remover uma linha descritiva a preço zero, ou re-partir
texto entre linhas. Num documento fiscal retido dez anos para inspecção da AGT,
isso é uma falha de evidência de adulteração.

### Segundo defeito: a identidade do cliente estava parcialmente fora do selo

Não previsto no R1 nem no R2. `ClienteSnapshot` tem `Nome` (obrigatório), `NIF`
(opcional) e `Morada`; o canónico da ADR-040 selava **apenas o NIF**.

Numa factura a *consumidor final* — o doente que não fornece NIF, que é o caso
comum numa clínica — a identidade selada era a **string vazia**, e o `Nome` era a
única identificação que o documento carregava. Nessas facturas, **o destinatário do
documento era livremente alterável com o selo intacto**. "Emitida a quem" é
conteúdo fiscal, não metadado interno.

## Decisão

### 1. Regra de enquadramento (normativa)

**Todo o campo de texto leva prefixo de comprimento em bytes; os inteiros vão
nus.**

Notação: `p(s)` = comprimento de `s` em **bytes**, seguido de `:`, seguido de `s`
literal. Exemplo: `p("Z|CONSULTA")` = `10:Z|CONSULTA`.

A implementação é uma única função no domínio
(`internal/domain/financeiro/factura.go`):

```go
func enquadrar(s string) string { return strconv.Itoa(len(s)) + ":" + s }
```

**A regra é deliberadamente cega.** Não distingue campos "arriscados" de campos
"seguros" — e é essa a questão. O defeito acima existe precisamente porque alguém
julgou que as descrições eram seguras e escreveu esse juízo numa ADR como se fosse
facto. Uma regra sem excepções não pode ser mal julgada por quem vier a seguir, e
especifica-se numa frase a quem tenha de a reproduzir noutra linguagem: *conta os
bytes, depois lê esse número de bytes.*

**Campos ausentes canonicalizam-se como `0:`** — nunca `null`, nunca `<nil>`,
mantendo a disciplina que a ADR-040 já fixara para o NIF. Aplica-se a `clienteNIF`,
`clienteMorada`, `operacaoID`, `episodioID` e ao `hashAnterior` da primeira factura
da série.

### 2. O formato canónico (normativo, substitui o da ADR-040 §3)

O hash de uma factura é o SHA-256, em hexadecimal minúsculo, da concatenação dos
**doze** campos seguintes, separados por `|`, nesta ordem exacta:

```
{p(serie)}|{sequencial}|{p(dataEmissaoRFC3339UTC)}|{p(clienteNome)}|{p(clienteNIF)}|{p(clienteMorada)}|{p(episodioID)}|{subtotal}|{iva}|{total}|{p(digestLinhas)}|{p(hashAnterior)}
```

`digestLinhas` é o SHA-256 em hexadecimal minúsculo da concatenação, para cada
linha por ordem de apresentação (`ordem` a começar em `0`), de:

```
{ordem}|{p(descricao)}|{p(tipo)}|{quantidade}|{precoUnitarioCentimos}|{p(regimeIVA)}|{p(operacaoID)}\n
```

As **três regras de canonicalização** da ADR-040 §3 mantêm-se inalteradas e
continuam normativas: tempo em UTC truncado ao segundo em RFC3339; dinheiro em
cêntimos inteiros decimais sem separadores nem símbolo; ordem das linhas selada
pelo índice `ordem` e não pela ordem de devolução da BD.

### 3. Âmbito do selo

**Entram, face à ADR-040:** `clienteNome`, `clienteMorada`, `operacaoID`
(proveniência cross-context de cada linha) e `episodioID` (proveniência
cross-context da factura inteira).

O `episodioID` entrou por **coerência com o `operacaoID`**: um é a proveniência da
factura, o outro a da linha; ambos eram reapontáveis com o selo intacto, e a mesma
razão que manda selar um manda selar o outro. Fica no canónico a seguir à morada,
fechando o bloco de identidade/proveniência antes dos montantes.

**Não entra: `ItemFactura.ID`.** É chave substituta sem significado fiscal; selá-la
ataria o documento fiscal a um detalhe de implementação da base de dados, sem
acrescentar nada que um auditor reconheça. Fica registado como **decisão
consciente, não como omissão**.

### 4. Vector dourado

O vector dourado mudou de valor, e **isso é o comportamento correcto** — ele existe
para gritar quando a canonicalização muda. O valor novo, fixado em
`TestHash_VectorDourado` (`internal/domain/financeiro/factura_test.go`):

```
7c99e3dbc895f04e3e40d4114dea8f5129e10297de33a25222d9dcc401c796da
```

Fixtura que o produz (a mesma da ADR-040, para a comparação ser directa):

```go
c, _ := fin.NovoClienteSnapshot("Sol", "", "")
f, _ := fin.NovaFactura(c, "11111111-1111-1111-1111-111111111111")
f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeCentimos(50000), fin.RegimeIsento)
f.AdicionarItem("Paracetamol", fin.LinhaDispensa, "22222222-2222-2222-2222-222222222222", 2,
    moeda.DeCentimos(1000), fin.RegimeStandard)
m := time.Date(2026, 7, 18, 10, 0, 0, 123456789, time.UTC)
f.Emitir("2026", 7, "abc", m)
```

Totais: subtotal 52000, IVA 280 (`(2000*14+50)/100`, meia-acima por linha), total
52280.

### 5. Verificação independente do formato

O formato foi conferido por **reimplementação independente**: uma implementação em
Python derivada da **regra normativa escrita acima** — não do código Go — reproduz
o vector dourado exactamente. Isto é a propriedade que interessa a uma auditoria:
o canónico é reproduzível a partir da especificação, sem acesso ao nosso stack.

A **injectividade foi confirmada por enumeração**, e não apenas por argumento: a
força bruta sobre os quatro campos de texto adjacentes do canónico
(`clienteNome`, `clienteNIF`, `clienteMorada`, `episodioID`), com 12 valores cada
— incluindo `""`, `"|"`, `"\n"`, `"A|B"`, `"A\nB"` e sequências que imitam o
próprio prefixo de comprimento, como `"12:X"` e `"0:"` — produziu **12⁴ = 20 736
facturas logicamente distintas → 20 736 canónicos distintos, zero colisões**.

Registe-se, por honestidade de percurso, que o desenho desta fatia foi acompanhado
de um protótipo em Go cujo valor dourado consta do documento de desenho
(`6f5a535c…`); esse valor **precede a extensão que selou o `episodioID`** e não é o
valor em vigor. O valor autoritário é o do §4, fixado contra a implementação real e
conferido pela reimplementação em Python.

### 6. Testes

- `TestHash_DescricaoNaoImitaFronteiraDeLinha` — a colisão acima como **teste de
  regressão permanente**, agora a exigir hashes diferentes. É o teste de maior valor
  desta fatia: falha se alguém voltar a remover o enquadramento. Tem uma
  pré-condição explícita que aborta se os totais de A e B deixarem de coincidir, sem
  o que o teste deixaria de exercitar o digest e passaria por razão errada.
- `TestHash_SelaNomeDoCliente`, `TestHash_SelaMoradaDoCliente`,
  `TestHash_SelaOperacaoIDDaLinha`, `TestHash_SelaEpisodioIDDaFactura` — um por
  campo recém-selado. Cada um falhava antes da implementação e passa depois (TDD
  genuíno, não tautológico).
- `TestHash_VectorDourado` — a canonicalização inteira travada num valor.

## Alternativas rejeitadas

1. **`%q` (aspas e escape do Go).** Rejeitada. É legível e idiomática, mas **ata o
   formato canónico fiscal às regras de `strconv.Quote`**, incluindo o tratamento de
   não-ASCII e de UTF-8 inválido. Este artefacto tem de ser reproduzível por
   terceiros — possivelmente pelas ferramentas da própria AGT, numa certificação,
   daqui a dez anos. "Replicar o quoting do Go" convida exactamente à divergência
   subtil que o vector dourado existe para apanhar. O prefixo de comprimento
   especifica-se numa frase; o quoting do Go especifica-se num capítulo.

2. **SHA-256 por campo, concatenado.** Rejeitada. É injectivo e independente de
   linguagem, mas **o canónico deixa de ser inspeccionável como texto**, o que
   dificulta a auditoria manual — a capacidade de um humano abrir o canónico e ler o
   que lá está — sem ganho nenhum sobre o prefixo de comprimento em matéria de
   injectividade.

3. **Escapar `|` e `\n` na descrição.** Rejeitada implicitamente pela regra cega do
   §1: escapar exige decidir *quais* os caracteres perigosos e *quais* os campos que
   os podem conter, que é a mesma classe de juízo que produziu o defeito. O
   enquadramento não faz juízo nenhum.

## Consequências

**Positivas**

- O canónico é **injectivo por construção**: nenhum conteúdo de campo consegue
  imitar um separador, porque quem lê consome exactamente os bytes anunciados.
  Confirmado por enumeração (§5), não só por argumento.
- A identidade fiscal do destinatário passa a estar selada mesmo **sem NIF**, que é
  o caso comum numa clínica.
- A proveniência cross-context — `operacaoID` por linha, `episodioID` por factura —
  deixa de ser reapontável com o selo intacto.
- A regra é **especificável numa frase** e foi demonstrada reproduzível fora do
  nosso stack, que é a condição prática para uma auditoria AGT.
- **Alteração puramente de domínio: zero migrações.** `cliente_nome`,
  `cliente_morada`, `operacao_id` e `episodio_id` já eram colunas persistidas, pelo
  que o hash continua reproduzível a partir do que está gravado.

**Negativas**

- O canónico ficou **mais verboso** e menos agradável de ler à vista desarmada: cada
  campo de texto passa a carregar o seu comprimento. É o preço da injectividade, e
  foi pago de propósito.
- O vector dourado mudou. Qualquer cadeia construída com o formato da ADR-040
  **deixa de verificar** — ver Riscos.
- O formato volta a ficar **congelado** a partir da primeira emissão em produção,
  agora sem prazo de revisão em aberto: o R1 e o R2 estavam explicitamente
  pendentes, e deixam de estar.

## Riscos e dívida registada

### R1 — Cadeias construídas com o formato da ADR-040 deixam de verificar

Esta fatia **muda o formato do hash**. Toda a factura EMITIDA cujo elo tenha sido
calculado com o canónico da ADR-040 falha o recálculo em `VerificarCadeia` — e
falha com razão, porque o conteúdo selado deixou de corresponder à regra em vigor.

Isto é aceitável **exactamente porque ainda não houve emissão em produção**, que era
a condição que a ADR-040 pôs. É também a razão pela qual esta fatia teve de ser
feita agora e não mais tarde.

Consequência prática, já paga: o teste de integração da cadeia
(`tests/integration/facturas_test.go`) passou a usar uma **série própria por
corrida**, verificando apenas a cadeia que ele próprio cria. Antes listava toda a
série `2999`, que acumula facturas EMITIDA de corridas anteriores — irremovíveis
por desenho, pelo trigger de imutabilidade — com elos em formato antigo. Sem essa
correcção, a mudança de formato deixaria a suite vermelha e a base de dados de
desenvolvimento teria de ser recriada. Com ela, as facturas antigas tornam-se
inertes em vez de venenosas.

### R2 — `ItemFactura.ID` continua fora do selo (decisão consciente, agora definitiva)

Ver §3. A linha de uma factura emitida é imutável por trigger e a sua ordem está
selada pelo índice `ordem`; a chave substituta não acrescenta evidência fiscal.
Fica registado para que a ausência não seja lida no futuro como esquecimento.

### R3 — A injectividade está provada por enumeração parcial, não por prova formal

O §5 enumera quatro campos de texto adjacentes. Não é uma prova formal de
injectividade sobre todo o espaço de facturas — é evidência forte sobre a região
onde a colisão real ocorreu, mais o argumento estrutural (um prefixo de
comprimento é injectivo por construção). Registado para não haver leitura
optimista de "zero colisões" como "provado impossível".

### R4 — Riscos herdados da ADR-040 que esta fatia **não** toca

Continuam abertos e por resolver, sem alteração: **R3** (dívida sistémica de MFA em
10 grupos de rotas com papéis sensíveis), **R4** (`ListarSnapshotsPorSerie` faz N+1
e não pagina), **R5** (a verificação inclui `ANULADA` por desenho), **R6**
(`trg_facturas_imutaveis` não cobre `INSERT`) e **R7** (o papel da aplicação é dono
das tabelas fiscais e pode desactivar os triggers). Esta fatia é de domínio puro e
não lhes mexe.

## Fora do âmbito desta fatia

Registado explicitamente para não haver leitura optimista desta ADR:

- **A anulação de facturas continua a não existir.** O estado `ANULADA` figura no
  enum e na CHECK desde a ADR-039, mas **nenhuma transição o alcança**. A ADR-040
  diferira-a para a ADR-041; esta fatia foi reordenada à frente dela por causa do
  prazo do R1/R2 (formato revisível apenas antes da primeira emissão em produção).
  A restrição vinculativa que a ADR-040 §R5 impôs — a anulação **não pode apagar
  nem renumerar**, tem de preservar número, sequencial, hash e `hashAnterior` —
  mantém-se integralmente e transfere-se para a ADR que vier a implementá-la.
- **A submissão AGT/SAF-T-AO não está feita** — nem a geração do XML, nem a
  validação XSD, nem a submissão.
- **A certificação de software junto da AGT não está obtida.** Esta ADR estabelece
  condições técnicas que uma certificação examinaria; não a substitui nem a
  antecipa.
- **Pagamentos** (parcial, múltiplos métodos) e integração EMIS Multicaixa.
- **Assinatura digital** com chave privada da clínica (alternativa 1 da ADR-040).

## Diferido

- **Anulação por nova factura**, respeitando a ADR-040 §R5, e **pagamentos**.
- **SAF-T-AO**: geração XML, validação XSD, submissão em sandbox.
- **Integração EMIS Multicaixa.**
- **Fatia própria de segurança**: impor `MFAObrigatoria()` nos 10 grupos de rotas
  com papéis sensíveis (ADR-040 §R3), completar as credenciais OTP do realm de
  desenvolvimento para `Admin`, `DPO` e `Auditor`, retirar a propriedade das tabelas
  fiscais ao papel da aplicação (§R7) e, se se decidir fechá-lo, o §R6.
- **Agendamento do cron diário de verificação da cadeia** (REG-001 §3.4), dependente
  do §R4 da ADR-040.
- Auto-população de linhas via ACL e validação de `episodio_id` cross-BC (herdados
  da ADR-039).
