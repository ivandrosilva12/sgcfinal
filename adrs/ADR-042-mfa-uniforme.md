# ADR-042 — Imposição uniforme de MFA e factura nascida em RASCUNHO

- **Estado:** Aceite
- **Data:** 2026-07-20
- **Marco/Sprint:** M4 — Financeiro (Sprint 17)
- **Fontes:** desenho em
  `docs/superpowers/specs/2026-07-19-adr-042-mfa-uniforme-design.md`; ADR-040
  (emissão da Factura — R3, R6, R7); ADR-041 (selagem canónica — §R4, que carregou
  o R3 da ADR-040); DDM-001 v2.0 §5.2.1 (5 papéis sensíveis em 12); REG-001 §3.2.

## Contexto

A ADR-040 registou, no seu §R3, uma dívida sistémica de segurança que nem ela nem a
ADR-041 fecharam: o segundo factor (`MFAObrigatoria()`) era imposto em três dos
catorze grupos de rotas de negócio e faltava nos outros onze. A ADR-041 tocou apenas
o domínio (selagem canónica do hash) e transportou a dívida intacta no seu §R4.

Esta fatia fecha dois dos riscos herdados — o **R3** (MFA por impor) e o **R6** (a
factura pode nascer `EMITIDA`) — e deixa o **R7** para fatia própria, pelas razões
da §5.

### 1.1 R3 — a exposição, medida contra o código real

Em `internal/platform/app.go`, cada local de chamada `adhttp.Registar*` escolhia os
seus middlewares. O `mfaMW` chegava a `RegistarIdentidade`, `RegistarAdministracao`
e — desde a ADR-040 — `RegistarFinanceiro`. Os restantes **onze** grupos não o
recebiam.

Cruzando o RBAC de cada família de handlers com o catálogo de papéis sensíveis
(`Director`, `Admin`, `DPO`, `Auditor`, `Tesoureiro`), a exposição foi **medida**:

| Grupo sem `mfaMW` | Papéis sensíveis no RBAC | Onde |
|---|---|---|
| Doentes | `Director`, `DPO`, `Auditor` | leitura clínica |
| Episódios | `Director`, `DPO`, `Auditor` | leitura clínica |
| Consentimentos | `Director`, `DPO`, `Auditor` | leitura clínica (LPDP) |
| Cirurgia | `Director`, `DPO`, `Auditor` | leitura clínica |
| Farmácia | `Director`, `DPO`, `Auditor` | leitura |
| Farmácia-Stock | `Director`, `DPO`, `Auditor` | leitura |
| Laboratório | `Admin`, `Director` | leitura clínica e catálogo |
| Recepção — marcação | `Admin`, `Director` | leitura de agenda **e** escrita |
| Recepção — chegadas | `Admin`, `Director` | **só escrita** (`soAdministrativo`) |
| Recepção — triagem | `Director` | leitura clínica |
| Clínico-Consulta | nenhum | `RBAC(PapelMedico)` |

**Dez dos onze grupos desprotegidos expunham pelo menos um papel sensível.**
`Director`, `DPO` e `Auditor` figuram na leitura clínica de praticamente todo o
sistema — doentes, episódios, cirurgia, farmácia e farmácia-stock; e `Director`,
sozinho ou com `Admin`, no laboratório, na recepção e na triagem. Alcançavam esses
dados **sem segundo factor**. Só `Clínico-Consulta` (`RBAC(PapelMedico)`) não expõe
nenhum.

Uma nuance material, para não sobredimensionar a leitura: em **Recepção — chegadas**
o papel sensível aparece **só numa rota de escrita** (o conjunto `soAdministrativo`,
aplicado aos `POST` de chegada, walk-in e desistência), e não numa rota de leitura;
a leitura da fila (`filaLeitura`) e a chamada (`chamada`) não expõem papel sensível
nenhum. A exposição é real, mas não é de leitura clínica.

### 1.2 Nota de rigor: uma medição errada, e porque a ADR-040 tinha razão

Este ponto é ele próprio matéria de registo, e não uma nota de rodapé. Uma versão
anterior do desenho desta fatia apresentou uma tabela de **quatro** famílias
(Doentes/Admin, Consentimentos/Admin, Laboratório/Admin+Director,
Recepção/Admin+Director) e afirmou que a medição herdada — "11 dos 14 grupos, 10 a
expor papel sensível", escrita na ADR-040 §R3 e carregada pela ADR-041 §R4 — era
"menos informativa do que parece". **Estava ao contrário: a ADR-040 tinha razão, e a
tabela de quatro estava errada. Retira-se.**

O erro teve duas causas, ambas **de instrumento** e ambas a **subestimar** a
exposição:

1. O padrão de busca `Papel(Director|Admin|DPO|Auditor|Tesoureiro)` casava o
   **prefixo** de `PapelAdministrativo` — o papel do pessoal administrativo, que
   **não é sensível** — e contava-o como `PapelAdmin`. Daí "Doentes: Admin" e
   "Consentimentos: Admin", ambos falsos: o que lá está é `PapelAdministrativo`.
2. Extrair primeiro `RBAC\([^)]*\)` perdia todas as chamadas `RBAC(...)` que se
   estendem por **várias linhas** — que são precisamente as listas de leitura
   clínica onde `Director`, `DPO` e `Auditor` aparecem (ver `doente_handler.go`,
   `episodio_handler.go`, `cirurgia_handler.go`, `farmacia_handler.go`). Daí
   "Episódios, Cirurgia, Farmácia: nenhum papel sensível", também falso.

A medição correcta usa fronteira de palavra e varre o ficheiro inteiro, e foi
confirmada lendo as chamadas `RBAC` completas de cada handler. O erro fica registado
porque a lição é geral e cara: **as duas falhas apontaram no mesmo sentido e
produziram uma tabela que parecia precisa.** Uma medição apresentada com aparência
de rigor merece a mesma desconfiança que uma afirmação sem medição nenhuma — foi
esta a classe de defeito que a ADR-041 já tinha apanhado no R1 da ADR-040 ("as
descrições vêm de catálogo", também falso, também apresentado como facto).

### 1.3 A causa do R3 não foram onze esquecimentos

As catorze funções `Registar*` de negócio **já eram variádicas e já chamavam ao
parâmetro** que aplica os middlewares — treze das catorze com o nome `protecao`, e a
décima quarta (`RegistarIdentidade`) com o nome `middlewares`. O conceito de "pacote
de protecção passado a cada grupo" existia; o que falhou foi a **aplicação uniforme
nos locais de chamada**. Cada local escolhia os seus middlewares, e nada tornava
visível quem ficara de fora.

É a mesma classe de defeito que a ADR-041 corrigiu no hash: um **juízo caso a caso
que corre mal em silêncio**. A correcção durável é remover o juízo, não corrigir os
casos um a um.

### 1.4 A lacuna de detecção: nada verificava a ligação em `app.go`

Ao levantar o raio de impacto descobriu-se **porque é que a exposição sobreviveu**:
os routers dos testes de negócio construíam as suas próprias cadeias de middlewares.
`routerDoentes` passava apenas `adhttp.Auth(...)`; só `routerFin` passava também
`MFAObrigatoria()`, porque a ADR-040 o alterara. Duas consequências:

1. Alterar `app.go` sozinho **não partia teste nenhum** — cada router de teste
   redeclarava a protecção por sua conta. (Uma versão anterior do desenho chegou a
   afirmar que ~12 ficheiros de teste passariam a devolver 403 só por se mexer no
   `app.go`; **era falso**, e é a mesma classe de afirmação-sem-medição que estas
   ADR têm vindo a apanhar.)
2. Portanto **nada verificava a ligação real em `app.go`**. Retirar o `mfaMW` da
   produção não fazia falhar nenhum teste. A exposição do R3 existiu exactamente por
   isto: não havia como a detectar.

### 1.5 R6 — a factura podia nascer `EMITIDA`

`trg_facturas_imutaveis` (ADR-040 §6) é `BEFORE UPDATE OR DELETE` e não cobre o
`INSERT`. Um `INSERT` directo com `estado = 'EMITIDA'` era aceite. Como a ADR-040
§R6 já regista, isto difere em espécie do buraco fechado em `itens_factura`: aquele
era **mutação** de um documento selado; este é **fabricação**. A fabricação já é
detectável — os índices únicos impedem reutilizar número ou `(série, sequencial)`, a
CHECK obriga ao conjunto completo de campos de emissão, e a emissão legítima
seguinte lê `series.ultimo_hash` e não a linha fabricada, deixando-a órfã da cadeia.
Mas o invariante *"toda a factura nasce RASCUNHO"* é genuíno e barato.

## 2. Decisão

### 2.1 Um único pacote de protecção (R3)

Em `internal/platform/app.go`, um pacote único:

```go
protecao := []gin.HandlerFunc{limiteMW, authMW, mfaMW}
```

passado aos **14** grupos de negócio dentro de `registarRotas`. O parâmetro de
`RegistarIdentidade` foi uniformizado (`middlewares` → `protecao`), alinhando-o com
os outros treze.

**Custo zero onde não há papéis sensíveis.** `MFAObrigatoria()` delega em
`dominio.VerificarAutenticacaoForte`, que só rejeita sessões de papel sensível.
Aplicá-lo a um grupo sem papéis sensíveis (`Clínico-Consulta`) é um no-op — pelo que
a regra uniforme não exige julgar quem "precisa". **Acrescentar um grupo
desprotegido passa a exigir desvio deliberado e visível**, em vez de esquecimento.

### 2.2 A guarda sobre o `app.go`

Fechar o R3 uniformizando o `app.go` não bastava: "exige desvio deliberado" não é o
mesmo que "é detectado". A fatia acrescenta, por isso, uma guarda —
`TestRegistarRotas_TodasAsRotasDeNegocioUsamOPacoteProteccao` em
`internal/platform/app_protecao_test.go` — que **lê o código-fonte** de `app.go`,
analisando-o com `go/parser` + `go/ast`, e garante duas propriedades, cada uma
insuficiente sem a outra:

1. **Ligação** — dentro do corpo de `registarRotas`, cada um dos catorze grupos
   nomeados em `gruposEsperados` aparece exactamente uma vez como
   `adhttp.Registar<algo>(..., protecao...)`. A comparação é por **conjunto
   nomeado**, não por contagem: um grupo em falta é nomeado como "em falta", um
   extra como "inesperado".
2. **Conteúdo** — `protecao` é atribuída **exactamente uma vez** na função que a
   declara, e o valor é sempre o composite literal
   `[]gin.HandlerFunc{limiteMW, authMW, mfaMW}`, nesta ordem, terminando em `mfaMW`.

É um teste invulgar — inspecciona código-fonte em vez de comportamento — e a razão
de o preferir mesmo assim fica registada: **o que falhou aqui foi a ligação, não o
comportamento**, e prová-la por comportamento exigiria montar os catorze handlers
com todos os seus fakes só para verificar uma propriedade de wiring. A alternativa
aceitável, se um dia se julgar a guarda frágil, é extrair `registarRotas` para uma
função testável que receba os handlers e devolva a lista de registos.

**As quatro rondas de ataque que a guarda passou a fechar** ficam registadas, porque
documentam porque não se deve voltar a uma verificação por texto:

- **regex gulosa a engolir um comentário à direita** da chamada;
- **chamada multi-linha invisível** a uma extracção por linha;
- **extracção da chamada para um helper**, ou troca do alias de import `adhttp`, a
  fazer o grupo desaparecer da inspecção sem baixar contagem nenhuma;
- o mais grave: **reatribuir `protecao` sem o `mfaMW`** (`protecao =
  []gin.HandlerFunc{limiteMW, authMW}`), que compila, passa no `gofmt` e passava na
  guarda anterior — desligando o segundo factor nos catorze grupos de uma só vez.
  É esta a razão da propriedade 2 (conteúdo), e a razão de a guarda ter passado de
  regex para `go/ast`.

**Lacuna conhecida, deixada explícita:** um **subgrupo esquecido dentro de um
handler** — uma rota registada num sub-router que não termine em `protecao` — é
invisível a esta guarda, que só vê os catorze registos de topo. É por isso que a
fatia **não** se apoia só na guarda: cada uma das dez famílias expostas ganhou
**prova comportamental** (§4).

### 2.3 `RegistarHealth` fica deliberadamente de fora

Não recebe protecção e **não pode receber**: os healthchecks e o *scrape* do
Prometheus são não-autenticados por desenho. Na prática nem sequer é chamado dentro
de `registarRotas` — é registado directamente em `internal/platform/server/server.go`
(junto de `/metrics`), fora do bloco que a guarda inspecciona; a isenção explícita na
guarda é puramente defensiva. Fica registado aqui para que ninguém o "corrija" por
simetria: um "aplicar a todos" ingénuo partiria a observabilidade e o healthcheck do
container.

### 2.4 Credenciais OTP no realm de desenvolvimento

A ADR-040 §R3 registou que `admin.teste` (papel `Admin`, sensível) **não tinha OTP**
e que não existiam utilizadores `DPO` nem `Auditor` — pelo que o caminho positivo do
MFA nunca era exercitado para esses papéis, e os testes passariam sem provar nada.
Esta fatia corrige-o em `docker/keycloak/realm-sgc.json`: OTP acrescentado ao
`admin.teste`, e `dpo.teste` e `auditor.teste` novos, espelhando `director.teste`.
Os **cinco** papéis sensíveis passam a ter utilizador de teste com OTP configurado,
validado por import real num Keycloak 25.

### 2.5 A factura nasce RASCUNHO (R6)

Migração nova `migrations/financeiro/0004_facturas_nascem_rascunho.sql`,
forward-only: trigger `BEFORE INSERT ON financeiro.facturas` com
`WHEN (NEW.estado <> 'RASCUNHO')` que levanta excepção (`restrict_violation`). **Não
se edita a `0001`, a `0002` nem a `0003`** — editar uma migração já aplicada foi o
incidente registado na ADR-040 §6 (a `0002` no lugar da `0003`). A migração é
idempotente (`CREATE OR REPLACE` na função, `DROP TRIGGER IF EXISTS` antes de
recriar).

### 2.6 O que esta fatia NÃO garante (frase obrigatória de âmbito)

**Enquanto o R7 estiver aberto, o trigger do R6 é contornável pela própria
aplicação.** O papel da aplicação é dono das tabelas fiscais e pode correr
`ALTER TABLE … DISABLE TRIGGER` — foi demonstrado na revisão final da ADR-040. O
trigger é **defesa em profundidade contra erro e contra SQL directo de terceiros,
não contra a aplicação comprometida**. A imutabilidade da BD, por si só, não é
absoluta enquanto o mesmo papel puder desligar os triggers.

Esta frase é obrigatória na ADR. Omiti-la repetiria o defeito que a ADR-041 corrigiu
na ADR-040: prometer no documento uma garantia que o código não dá.

## 3. Âmbito da alteração

| Ficheiro | Alteração |
|---|---|
| `internal/platform/app.go` | pacote `protecao` nos 14 grupos; `RegistarIdentidade` uniformizado |
| `internal/platform/app_protecao_test.go` | guarda AST (ligação + conteúdo) — novo |
| `internal/adapters/http/*_test.go` | routers de teste passam a espelhar a produção (`MFAObrigatoria()`); sessões de papel sensível ganham `AutenticacaoForte: true`; prova `mfa-obrigatorio` nas dez famílias |
| `docker/keycloak/realm-sgc.json` | OTP no `admin.teste`; `dpo.teste` e `auditor.teste` novos |
| `migrations/financeiro/0004_facturas_nascem_rascunho.sql` | trigger de INSERT — novo |
| `tests/integration/facturas_test.go` | 3 fixturas passam a INSERT RASCUNHO → UPDATE |
| `adrs/ADR-042-mfa-uniforme.md` | esta ADR — novo |
| `adrs/ADR-040-emissao-factura.md` | R3 e R6 marcados resolvidos (aditivo) |
| `CLAUDE.md`, `SPRINT.md` | marco e sprint |

**Não muda:** o domínio (`internal/domain/`), a canonicalização do hash, o vector
dourado, as rotas, nem o RBAC de cada rota.

## 4. Testes

- **Prova negativa por família exposta** — as dez: papel sensível **sem** segundo
  factor → **403**, com `type: mfa-obrigatorio` asserido no corpo. Verificar só o
  código 403 não distinguiria o 403 do MFA do 403 do RBAC — ambiguidade já
  assinalada na Sprint 15. As dez famílias têm a asserção do corpo.
- **Routers de teste a espelhar a produção:** cada router aplica agora
  `MFAObrigatoria()` como o `routerFin` já fazia, e só então as sessões de papel
  sensível precisam de `AutenticacaoForte: true` — só então os testes exercitam a
  cadeia real em vez de uma cadeia própria e diferente.
- **A guarda AST sobre o `app.go`** fecha o outro lado: a ligação que nada verificava
  passa a ser verificada, e sobreviveu a quatro rondas de ataque (§2.2).
- **R6:** `INSERT` directo de factura `EMITIDA` → rejeitado; `INSERT` de `RASCUNHO`
  → continua a passar (fechar o caminho normal seria pior do que o buraco). As três
  fixturas reescritas continuam a provar a imutabilidade das emitidas, agora pelo
  caminho de produção (INSERT RASCUNHO → UPDATE).
- **Integração** contra a BD de desenvolvimento **e** contra uma criada de raiz, duas
  corridas.

**Risco de teste a vigiar:** esta é uma alteração larga e rasa; o modo de falha
próprio é um teste que passa a verde pela razão errada. A defesa são as provas
negativas por família (dez); a prova de que provam algo é a mutação — retirar o
`mfaMW` do pacote e confirmar que falham.

## 5. Fora do âmbito

- **R7** — retirar ao papel da aplicação a propriedade das tabelas fiscais. É o
  controlo que fecha de facto a classe da fabricação, e fica em **fatia própria**
  porque exige separar a credencial de migração da de runtime: a aplicação aplica as
  suas próprias migrações com o mesmo `DATABASE_URL` (`cfg.URLBaseDados` em
  `ExecutarServidor` e em `ExecutarMigracoes` → `AplicarMigracoes`,
  `internal/platform/app.go:332`), o que toca config, `docker-compose`, CI e o
  arranque. Enquanto estiver aberto, vale a §2.6.
- **Anulação** de factura — o estado `ANULADA` figura no enum e na CHECK desde a
  ADR-039, mas **nenhuma transição o alcança**. Vinculada pela ADR-040 §R5: não pode
  apagar nem renumerar.
- **Pagamentos** (parcial, múltiplos métodos) e integração EMIS Multicaixa.
- **SAF-T-AO** — geração XML, validação XSD, submissão em sandbox — e **certificação
  de software junto da AGT**: **não estão feitas**. Esta ADR não as substitui nem as
  antecipa.

## 6. Riscos e dívida registada

### R1 — A guarda AST não vê subgrupos dentro de um handler

A guarda garante que os catorze registos de topo terminam em `protecao...` e que
`protecao` contém `mfaMW`. **Não** vê um sub-router criado dentro de um handler que
registe uma rota sem a cadeia. Mitigado pelas provas comportamentais das dez
famílias, mas registado para não se ler a guarda como cobertura total.

### R2 — A guarda não verifica o comportamento dos middlewares

Verifica que `limiteMW`, `authMW` e `mfaMW`, por este nome e ordem, chegam aos
catorze grupos. Se `MFAObrigatoria()` deixasse de impor MFA por dentro, ou `mfaMW`
fosse redefinido para um no-op mais acima, a guarda continuaria verde — isso é
comportamento do middleware, testado em `internal/adapters/http`, não wiring.

### R3 — O R7 continua aberto (herdado da ADR-040)

Ver §5 e §2.6. A "imutabilidade imposta pela BD" continua contornável pela própria
aplicação enquanto o seu papel for dono das tabelas fiscais. Esta fatia **não** o
fecha e **não** deve ser lida como se o fechasse.

## 7. Estado dos riscos da ADR-040 após esta fatia

| Risco (ADR-040) | Estado |
|---|---|
| R3 — MFA por impor em 11 dos 14 grupos | **Resolvido** por esta ADR (§2.1, §2.2) |
| R4 — `ListarSnapshotsPorSerie` N+1 e sem paginação | Aberto |
| R5 — verificação inclui `ANULADA` por desenho | Aberto (restrição, não defeito) |
| R6 — factura pode nascer `EMITIDA` | **Resolvido** por esta ADR (§2.5) **— mas** a garantia continua condicionada pelo R7 (§2.6) |
| R7 — papel da aplicação é dono das tabelas fiscais | Aberto — fatia própria (§5) |
