# ADR-040 — Emissão de Factura: numeração por série, cadeia hash e imutabilidade

- **Estado:** Aceite
- **Data:** 2026-07-18
- **Marco/Sprint:** M4 — Financeiro (Sprint 15)
- **Fontes:** design em
  `docs/superpowers/specs/2026-07-18-adr-040-emissao-factura-design.md`; plano em
  `docs/superpowers/plans/2026-07-18-adr-040-emissao-factura.md`; REG-001 §3.2/§3.4;
  DDM-001 v2.0 §5.2.1; ADR-039 (agregado `Factura` em RASCUNHO); ERRATA-002 (papel
  Tesoureiro).

## Contexto

O ADR-039 entregou o agregado `Factura` em `RASCUNHO` e deixou explicitamente para
esta fatia a jóia da coroa regulatória: a **emissão**. Uma factura em rascunho não
tem valor legal nenhum — é um documento interno. É a emissão que lhe fixa um número
legal, uma data e um elo numa cadeia de integridade, e que a torna imutável.

O REG-001 §3.2 impõe três obrigações sobre documentos fiscais em Angola: numeração
**sequencial e sem buracos** por série, **cadeia hash** que torne qualquer adulteração
detectável, e **imutabilidade** do documento emitido (a correcção faz-se por novo
documento, nunca por alteração). O §3.4 exige ainda que a cadeia seja validada
periodicamente.

O REG-001 **não** especifica o conteúdo canónico do hash — exige apenas que exista e
que a quebra seja detectável. A escolha desse conteúdo é, portanto, uma decisão
arquitectural desta ADR, e é uma decisão praticamente irreversível: o hash de cada
factura sela o hash da anterior, pelo que alterar a canonicalização depois da primeira
emissão em produção invalidaria retroactivamente toda a cadeia já construída.

O ADR-039 deixou ainda uma dívida técnica explícita — a ausência de bloqueio optimista
no `Guardar` — com a condição de ser paga **antes** de a factura ganhar valor legal.
Esta fatia paga-a.

## Decisão

### 1. VO `NumeroFactura` e série como ano civil UTC

`internal/domain/financeiro/numero.go`. O número legal é
`FAC <série>/<sequencial a 8 dígitos>` (DDM-001 v2.0 §5.2.1) — por exemplo
`FAC 2026/00012345`. `NovoNumeroFactura` rejeita série vazia, sequencial não positivo
e **sequencial acima de 99999999**: a largura do campo é fixada pelo formato legal, e
sem essa guarda o `%08d` alargaria o número em silêncio ao esgotar a série. O
esgotamento é um erro de regra de negócio explícito ("abrir uma nova série").
`ParseNumeroFactura` faz o caminho inverso.

`SerieDe(momento)` devolve o **ano civil em UTC**. A normalização para UTC é a mesma
usada no hash: sem ela, uma factura emitida perto da meia-noite de 31 de Dezembro
poderia cair em séries diferentes conforme o fuso do servidor.

### 2. O hash é invariante do agregado, não de um serviço

`Factura.Emitir(serie, sequencial, hashAnterior, momento)` transita `RASCUNHO →
EMITIDA`, recusando factura já emitida e factura **sem linhas**, e calcula o hash
**dentro do agregado**. Nenhum serviço de aplicação, adaptador ou trigger de BD
calcula hashes. O repositório entrega ao agregado apenas o sequencial e o elo
anterior; a integridade do documento é responsabilidade de quem detém as suas
invariantes.

### 3. Formato canónico do hash (normativo)

Esta é a secção que uma auditoria ou uma certificação AGT tem de poder ler. O formato
é **fixo e não deriva do esquema da BD**, para continuar reproduzível ao longo dos 10
anos de retenção legal, mesmo que as colunas mudem de nome ou de tipo.

O hash de uma factura é o SHA-256, em hexadecimal minúsculo, da concatenação dos nove
campos seguintes, separados pelo caractere `|` (barra vertical), nesta ordem exacta:

```
serie|sequencial|dataEmissao|clienteNIF|subtotal|totalIVA|total|digestLinhas|hashAnterior
```

onde:

| Campo | Conteúdo |
|---|---|
| `serie` | a série, tal como registada (ex.: `2026`) |
| `sequencial` | o sequencial em decimal **sem zeros à esquerda** (ex.: `12345`) |
| `dataEmissao` | o instante da emissão em UTC, truncado ao segundo, em RFC3339 (ex.: `2026-07-18T09:30:00Z`) |
| `clienteNIF` | o NIF do cliente; **string vazia** quando não há NIF |
| `subtotal` | soma dos subtotais das linhas, em cêntimos inteiros |
| `totalIVA` | soma do IVA das linhas já arredondado por linha, em cêntimos inteiros |
| `total` | `subtotal + totalIVA`, em cêntimos inteiros |
| `digestLinhas` | ver abaixo |
| `hashAnterior` | o hash da factura anterior da mesma série; **string vazia** na primeira factura da série |

`digestLinhas` é, por sua vez, o SHA-256 em hexadecimal minúsculo da concatenação,
para cada linha por ordem de apresentação, de:

```
ordem|descricao|tipo|quantidade|precoUnitarioCentimos|regimeIVA\n
```

com `ordem` a começar em `0`. Resumir as linhas — e não apenas o total — é
deliberado: sem isso, trocar "Consulta" por "Cirurgia" mantendo o mesmo valor passaria
despercebido.

**As três regras de canonicalização** (é aqui que as implementações divergem, e é por
isso que ficam escritas):

1. **Tempo** — todo o instante é convertido a UTC, truncado ao segundo e formatado em
   RFC3339. Nunca hora local, nunca fracções de segundo.
2. **Dinheiro** — todo o valor monetário é um inteiro de cêntimos em decimal, sem
   separador de milhares, sem separador decimal e sem símbolo de moeda. Nunca vírgula
   flutuante.
3. **Ordem** — a ordem das linhas é significativa e fica selada pelo índice `ordem`
   prefixado a cada linha, e não pela ordem em que a BD as devolve.

Um **vector dourado** (`TestHash_VectorDourado`, hash
`8caeeee0017219380ffbca9560b2d24894b07a45ba1fdb63a6cc4710293cc169`) fixa esta
canonicalização em teste: qualquer alteração acidental ao formato parte a suite antes
de chegar a produção.

### 4. `VerificarCadeia` é uma função pura do domínio

`internal/domain/financeiro/cadeia.go`. Recebe os snapshots de uma série, ordena-os
por sequencial e verifica três propriedades, devolvendo o **primeiro** problema
encontrado:

1. a numeração é contígua desde 1 (sem buracos);
2. o `hashAnterior` de cada factura é o hash da que a precede;
3. o hash registado corresponde ao recálculo do conteúdo.

Uma série vazia é válida. Não toca em BD, relógio nem rede — é sobre esta função que
assentará o cron do REG-001 §3.4.

### 5. Serialização por `SELECT ... FOR UPDATE` na linha da série

`migrations/financeiro/0002_emissao_facturas.sql` cria `financeiro.series` (chave
`serie`, `ultimo_sequencial`, `ultimo_hash`), que é a **cabeça** da numeração e da
cadeia. `RepositorioFacturas.Emitir` numa única transacção:

1. `INSERT ... ON CONFLICT DO NOTHING` na série (resolve a viragem de ano: criar a
   linha fora do bloqueio seria ela própria uma corrida);
2. `SELECT ultimo_sequencial, ultimo_hash ... FOR UPDATE` — **o ponto de serialização**;
3. lê o agregado já sob o bloqueio e chama `Factura.Emitir`;
4. `UPDATE` da factura guardado por `estado='RASCUNHO' AND versao=$8`;
5. `UPDATE` da série com o novo sequencial e o novo elo.

Se a transacção reverter, o contador não avança. A ausência de buracos é **propriedade
da estrutura**, não uma verificação a posteriori.

A migração acrescenta ainda os índices únicos parciais `uq_facturas_numero` e
`uq_facturas_serie_sequencial` (sobre facturas com número/série não nulos) e a CHECK
`facturas_coerencia_emissao`, que impõe a coerência estado↔campos de emissão: em
`RASCUNHO` os campos de emissão são todos NULL; fora dele são todos NOT NULL.

### 6. Imutabilidade por trigger, em defesa em profundidade

Dois triggers `BEFORE UPDATE OR DELETE` rejeitam qualquer escrita sobre factura já
emitida — `trg_facturas_imutaveis` (condição `WHEN (OLD.estado <> 'RASCUNHO')`) e
`trg_itens_factura_imutaveis`, que consulta o estado da factura-mãe no corpo da função
porque o PostgreSQL não admite subconsulta na cláusula `WHEN`. O domínio já recusa
alterar uma factura emitida; o trigger garante que nem um `UPDATE` directo em SQL o
consegue. Espelha `auditoria.impedir_mutacao`.

Dois detalhes ficaram registados no próprio SQL por serem armadilhas reais: a função
dos itens devolve `NEW` (nunca `OLD`) num `BEFORE UPDATE`, sob pena de o `UPDATE`
reportar sucesso sem alterar nada — perda silenciosa de dados; e estado da mãe NULL
não bloqueia, porque é o que acontece na cascata de apagar um rascunho.

### 7. Bloqueio optimista — a dívida do ADR-039 fica paga

Coluna `versao`, incrementada em cada escrita e usada como guarda no `UPDATE` do
`Guardar` e no `UPDATE` do `Emitir`. Duas edições concorrentes do mesmo rascunho já não
perdem uma actualização: a segunda recebe um conflito ("a factura foi alterada
entretanto — recarregue e tente de novo"), distinguido por leitura do estado actual do
caso em que a factura simplesmente já não está em rascunho. `Emitir` devolve o agregado
com a versão **efectivamente gravada**, sem o que a escrita seguinte falharia com um
conflito falso.

### 8. Casos de uso: a cadeia quebrada é um resultado, não um erro

`CasoEmitirFactura` emite e audita `financeiro.factura.emitida` com o número legal e o
hash no detalhe. `CasoVerificarCadeia` devolve `ResultadoVerificacao{Serie,
TotalFacturas, Integra, Detalhe}`: uma cadeia partida responde **200 com
`integra:false`**, não um erro HTTP. É uma decisão de desenho e não uma comodidade —
se a quebra viesse como 500, um auditor não conseguiria distinguir "cadeia adulterada"
de "serviço em baixo", que é exactamente a distinção que ele precisa de fazer.

Rotas: `POST /api/v1/financeiro/facturas/:fid/emitir` (escrita, Tesoureiro) e
`GET /api/v1/financeiro/facturas/cadeia/verificacao?serie=` (leitura; sem `serie`
assume a série corrente).

### 9. O Tesoureiro passa a papel sensível — e o MFA passa a ser imposto

O ADR-039 registou o Tesoureiro como não-sensível, com a reavaliação explicitamente
marcada para esta ADR. Com a emissão, o Tesoureiro pratica um acto **irreversível com
efeito fiscal**: passa a sensível, ao lado de Director, Admin, DPO e Auditor (5 papéis
sensíveis em 12). Impacto: `identidade.papeisSensiveis`, `seeds/papeis.sql`, realm
Keycloak (com `tesoureiro.teste` e credencial OTP) e a migração forward-only
`identidade/0006_papel_tesoureiro_sensivel.sql` — a `0005`, que o semeou como
não-sensível, **não é editada**: o intervalo em que o papel esteve não-sensível é
matéria de auditoria e não se apaga.

A ERRATA-002 foi alterada **aditivamente**, por blocos de revisão, pela mesma razão.

Marcar o papel como sensível não bastava: `RegistarFinanceiro` não recebia
`MFAObrigatoria()`, pelo que a marcação não teria efeito prático nenhum. Passa a
recebê-la em `internal/platform/app.go`. **Isto corrige um buraco mais antigo do que o
Tesoureiro**: o Director e o Auditor já eram sensíveis desde o Sprint 3 e já tinham
leitura no Financeiro — ambos passavam sem segundo factor nessas rotas.

## Alternativas rejeitadas

1. **Assinatura digital (chave privada da clínica) em vez de, ou além de, cadeia
   hash.** Rejeitada **para esta fatia**. O REG-001 §3.2 não a lista nas obrigações, e
   traz consigo gestão de chaves, rotação e custódia — um problema de segurança inteiro
   que não deve viajar à boleia da fatia que fixa a numeração. A cadeia hash satisfaz a
   obrigação de detecção de adulteração; a assinatura é um reforço, não um requisito.
   Fica em aberto para quando a certificação AGT a exigir.

2. **`SEQUENCE` do PostgreSQL para o sequencial.** Rejeitada. Uma sequência **não é
   transaccional**: `nextval` não reverte com o `ROLLBACK`. Cada emissão falhada
   deixaria um buraco permanente na numeração — exactamente aquilo que o REG-001 §3.2
   proíbe. E "corrigir buracos a posteriori" é um antipadrão listado no blueprint. A
   linha de série bloqueada com `FOR UPDATE` avança **dentro** da transacção e reverte
   com ela.

3. **`pg_advisory_xact_lock` para serializar a emissão.** Rejeitada. Funcionaria, mas
   a serialização ficaria **invisível no esquema**: nada em `\d financeiro.facturas`
   revelaria que existe um ponto de bloqueio, e o próximo programador — ou o auditor —
   teria de o descobrir lendo código Go. A tabela `financeiro.series` torna o mecanismo
   auto-documentado e, de caminho, dá um lugar natural ao `ultimo_hash`.

4. **JSON canónico (ou serialização do esquema da BD) como conteúdo do hash.**
   Rejeitada. Amarraria o hash a uma biblioteca de serialização e ao esquema da tabela:
   uma actualização de dependência que mudasse a ordem das chaves, o escape de
   caracteres ou a representação de números tornaria **irreverificáveis todas as
   facturas antigas**. Com 10 anos de retenção legal, isso é inaceitável. A
   concatenação explícita de campos, com as três regras de canonicalização escritas
   nesta ADR, é verbosa de propósito — é legível por um humano e reimplementável em
   qualquer linguagem sem depender do nosso stack.

## Consequências

**Positivas**

- As três obrigações do REG-001 §3.2 estão satisfeitas e provadas: numeração sem
  buracos (12 emissões concorrentes na mesma série produzem sequenciais 1..12 sem
  repetição nem falha), cadeia detectável (adulteração, elo errado e buraco cada um com
  o seu teste) e imutabilidade (testada contra a BD real, não só contra o domínio).
- O conteúdo do hash está fixado por escrito e por vector dourado, e é reimplementável
  fora do nosso stack — condição prática para uma auditoria AGT.
- A dívida técnica que o ADR-039 assumiu (lost-update no rascunho) fica paga **antes**
  de a factura ganhar valor legal, que era exactamente a condição posta.
- O MFA passa a ser efectivamente imposto no BC Financeiro, fechando um bypass que
  precedia esta sprint e afectava Director e Auditor.

**Negativas**

- A emissão é um ponto de serialização por série: emissões concorrentes na mesma série
  põem-se em fila. É o preço da ausência de buracos e, ao ritmo de facturação de uma
  clínica, não é um estrangulamento — mas é uma propriedade a conhecer antes de
  qualquer teste de carga.
- O formato do hash fica **congelado** a partir da primeira emissão em produção. Ver
  os riscos abaixo: as duas decisões conscientes sobre o conteúdo do digest só são
  revisíveis antes desse momento.
- `ListarSnapshotsPorSerie` carrega a série inteira em memória. Aceitável hoje
  (verificação manual, séries curtas), insuficiente para o cron do §3.4 numa série de
  fim de ano — ver Riscos.

## Riscos e dívida registada

### R1 — Separadores não escapados no digest das linhas (a fixar **antes** da primeira emissão em produção)

`digestLinhas` **não escapa** `|` nem `\n` na descrição da linha. Em teoria, uma
descrição que contenha esses caracteres pode compor-se de forma a colidir com outra
combinação de campos. O índice `ordem` prefixado a cada linha mitiga o problema, e o
risco prático é baixo porque as descrições vêm de catálogo e não de texto livre do
utilizador.

Fica registado como **decisão consciente, e não como omissão** — mas com um prazo
duro: se alguma vez for corrigido, tem de o ser **antes da primeira emissão em
produção**. Alterar a canonicalização depois disso quebra retroactivamente o hash de
todas as facturas já emitidas, que é precisamente o cenário que a alternativa 4
rejeitou.

### R2 — `OperacaoID` e `ItemFactura.ID` ficam fora do digest (decisão consciente)

O digest sela `ordem`, `descrição`, `tipo`, `quantidade`, `preço unitário` e `regime
de IVA`. **Não** sela o `OperacaoID` — o elo cross-context de cada linha para a
dispensa, requisição ou procedimento que lhe deu origem — nem o `ID` da própria linha.
Consequência: esse elo poderia ser reapontado para outra operação sem invalidar o hash
da factura.

Aceite conscientemente porque o **valor fiscal está protegido**: o que a AGT audita —
descrição, quantidade, preço, regime, totais — está todo selado. O `OperacaoID` é
rastreabilidade interna, e a sua adulteração é detectável pelo audit log (append-only,
10 anos), não pela cadeia. Pela mesma razão que R1, esta decisão só é revisível antes
da primeira emissão em produção.

Nota de âmbito, pela mesma lógica: do cliente, o hash sela apenas o **NIF** — o
elemento fiscalmente relevante — e não o nome nem a morada.

### R3 — Dívida sistémica: o mesmo bypass de MFA persiste em 11 dos 14 grupos de rotas

Ao impor `MFAObrigatoria()` no Financeiro, tornou-se visível que o problema é mais
vasto do que o BC Financeiro. Medição sobre `internal/platform/app.go` (linhas
275–288):

- São registados **14 grupos de rotas**. Recebem `mfaMW` **três**: `RegistarIdentidade`,
  `RegistarAdministracao` e — desde esta fatia — `RegistarFinanceiro`.
- Os restantes **11** não o recebem: `RegistarDoentes`, `RegistarEpisodios`,
  `RegistarConsentimentos`, `RegistarCirurgia`, `RegistarFarmacia`,
  `RegistarFarmaciaStock`, `RegistarLaboratorio`, `RegistarRecepcao`,
  `RegistarRecepcaoChegadas`, `RegistarRecepcaoTriagem` e `RegistarClinicoConsulta`.
- Desses 11, **10** expõem pelo menos um papel sensível no seu RBAC. Concretamente:
  Doentes, Episódios, Consentimentos, Cirurgia, Farmácia e Farmácia/Stock incluem
  `Director`, `DPO` e `Auditor` na leitura; Laboratório inclui `Director` e `Admin`;
  Recepção e Recepção/Chegadas incluem `Director` e `Admin`; Recepção/Triagem inclui
  `Director`. O único sem papel sensível é `RegistarClinicoConsulta` (`RBAC(PapelMedico)`).

Ou seja: **o mesmo bypass de MFA que esta fatia fechou no Financeiro persiste em 10
grupos de rotas clínicas, de farmácia, de laboratório e de recepção.** Um Director ou
Auditor comprometido continua a poder ler dados clínicos sem segundo factor.

Isto é **pré-existente e não uma regressão desta sprint** — só ficou visível porque
fomos verificar. Fica registado aqui, por decisão explícita, para ser resolvido em
**fatia própria**: a correcção toca 10 grupos de rotas e o RBAC de quatro bounded
contexts, e enfiá-la nesta sprint misturaria uma correcção de segurança transversal
com a entrega regulatória da emissão.

Nota lateral sobre o realm de desenvolvimento: `admin.teste` (papel `Admin`, sensível)
**não tem credencial OTP**, e não existem utilizadores de teste para `DPO` nem para
`Auditor`. Só `director.teste` e `tesoureiro.teste` têm OTP configurado. O realm de
desenvolvimento, portanto, não exercita o caminho de MFA para os papéis `Admin`, `DPO`
e `Auditor` — a fatia que pagar esta dívida terá de o corrigir também, sob pena de
"passar nos testes" sem provar nada.

### R4 — `ListarSnapshotsPorSerie` faz N+1 e não pagina

O método lê as facturas da série numa consulta e depois lê as linhas de cada uma numa
consulta por factura (o cursor da primeira consulta ocupa a ligação do pool enquanto
está aberto, pelo que as linhas só se podem ler depois de o fechar). Não há paginação:
a série inteira vem para memória.

Aceitável agora — o único chamador é a verificação manual, invocada por um humano. Mas
o cron diário do REG-001 §3.4 vai correr isto sobre uma série real de fim de ano, e
nessa altura será preciso um `JOIN` único ou verificação incremental (guardar o último
sequencial verificado e continuar dali).

### R5 — `ListarSnapshotsPorSerie` inclui `ANULADA` por desenho — restrição para o ADR-041

O filtro é `estado <> 'RASCUNHO'`, e **não** `estado = 'EMITIDA'`. É deliberado: uma
factura anulada continua a ocupar o seu sequencial e o seu elo na cadeia. Omiti-la da
verificação **fabricaria um buraco** e faria `VerificarCadeia` acusar uma quebra que
não existe.

Consequência vinculativa para o ADR-041: **a anulação não pode apagar nem renumerar**.
Tem de ser uma transição de estado que preserve número, sequencial, hash e
`hashAnterior` intactos — a anulação faz-se por nova factura, e a factura anulada
permanece na cadeia.

## Fora do âmbito desta fatia

Registado explicitamente para não haver leitura optimista desta ADR:

- **A anulação de facturas não existe.** O estado `ANULADA` figura no enum e na CHECK
  desde o ADR-039, mas **nenhuma transição o alcança**. Fica para o ADR-041.
- **A submissão AGT/SAF-T-AO não está feita** — nem a geração do XML, nem a validação
  XSD, nem a submissão. Fica para o ADR-042.
- **A certificação de software junto da AGT não está obtida.** Esta ADR estabelece as
  condições técnicas que uma certificação examinaria; não a substitui nem a antecipa.
- **Pagamentos** (parcial, múltiplos métodos) e integração EMIS Multicaixa.

## Diferido

- **Agendamento do cron diário de verificação da cadeia (REG-001 §3.4).** A função de
  domínio, o caso de uso e o endpoint existem e estão provados; falta o agendador que
  os invoca e o alarme que dispara quando `integra=false`. Depende de R4.
- **ADR-041**: anulação por nova factura (respeitando R5) e pagamentos.
- **ADR-042**: SAF-T-AO — geração XML, validação XSD, submissão em sandbox.
- **ADR-043**: integração EMIS Multicaixa.
- **Fatia própria de segurança**: impor `MFAObrigatoria()` nos 10 grupos de rotas com
  papéis sensíveis identificados em R3, e completar as credenciais OTP do realm de
  desenvolvimento para `Admin`, `DPO` e `Auditor`.
- Auto-população de linhas via ACL e validação de `episodio_id` cross-BC (herdados do
  ADR-039).
