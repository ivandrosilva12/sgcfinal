# ADR-042 — Imposição uniforme de MFA e factura nascida em RASCUNHO

> Desenho validado. Sprint 17, a seguir ao ADR-041 (selagem canónica), entregue e
> publicado em `origin/main` em `9169c16`.

## 1. Porque existe esta fatia

A ADR-040 registou três riscos de segurança que a ADR-041 não tocou. Esta fatia fecha
dois deles — o **R3** (MFA por impor) e o **R6** (factura pode nascer `EMITIDA`) — e
deixa o **R7** para fatia própria, pelas razões da §5.

### 1.1 R3 — papéis sensíveis alcançam dados clínicos sem segundo factor

Dos 14 grupos de rotas de negócio registados em `internal/platform/app.go`, apenas três
recebem o `mfaMW`: Identidade, Administração e Financeiro. Os restantes onze não.

A exposição foi **medida**, não presumida. Cruzando o RBAC de cada família de handlers
com o catálogo de papéis sensíveis (`Director`, `Admin`, `DPO`, `Auditor`, `Tesoureiro`):

| Grupo sem `mfaMW` | Papéis sensíveis no RBAC |
|---|---|
| Doentes | `Director`, `DPO`, `Auditor` |
| Consentimentos | `Director`, `DPO`, `Auditor` |
| Episódios | `Director`, `DPO`, `Auditor` |
| Cirurgia | `Director`, `DPO`, `Auditor` |
| Farmácia | `Director`, `DPO`, `Auditor` |
| Farmácia-Stock | `Director`, `DPO`, `Auditor` |
| Laboratório | `Admin`, `Director` |
| Recepção — marcação | `Admin`, `Director` |
| Recepção — chegadas | `Admin`, `Director` |
| Recepção — triagem | `Director` |
| Clínico-Consulta | nenhum |

**Dez dos onze grupos desprotegidos expõem papéis sensíveis.** `Director`, `DPO` e
`Auditor` figuram na leitura clínica de praticamente todo o sistema — doentes,
episódios, cirurgia, farmácia, laboratório, consentimentos LPDP e triagem — e
alcançavam-na **sem segundo factor**. Só `Clínico-Consulta` não expõe nenhum.

### Nota de rigor sobre esta medição — e uma correcção a retirar

Uma versão anterior deste desenho apresentou uma tabela de **quatro** famílias e afirmou
que a ADR-041 §R3, ao falar de "10 dos 14 grupos", era "menos informativa do que parece".
**Essa afirmação estava errada e retira-se: a ADR-041 estava certa.**

A medição errada teve duas causas, ambas de instrumento e não de leitura:

1. O padrão `Papel(Director|Admin|DPO|Auditor|Tesoureiro)` casa o **prefixo** de
   `PapelAdministrativo` — um papel **não sensível**, o do pessoal administrativo — e
   contou-o como `PapelAdmin`. Daí "Doentes: Admin" e "Consentimentos: Admin", ambos falsos.
2. Extrair primeiro `RBAC\([^)]*\)` perde as chamadas `RBAC(...)` que se estendem por
   **várias linhas** — que são precisamente as listas de leitura clínica onde `Director`,
   `DPO` e `Auditor` aparecem. Daí "Episódios, Cirurgia, Farmácia: nenhum", também falso.

A medição correcta usa fronteira de palavra (`\b`) e varre o ficheiro inteiro, e foi
confirmada lendo as chamadas `RBAC` completas. Fica registada aqui porque o erro é
instrutivo: **as duas falhas apontaram no mesmo sentido — subestimar a exposição — e
produziram uma tabela que parecia precisa.** Uma medição apresentada com aparência de
rigor merece a mesma desconfiança que uma afirmação sem medição nenhuma.

### 1.2 A causa não são onze esquecimentos

As catorze funções `Registar*` de negócio **já são variádicas e já chamam ao parâmetro
`protecao`** — treze das catorze com esse nome exacto. O conceito existia; falhou a
aplicação uniforme nos locais de chamada. Cada local escolhe os seus middlewares, e
nada torna visível quem ficou de fora.

É a mesma classe de defeito que a ADR-041 corrigiu no hash: um juízo caso a caso que
correu mal em silêncio. A correcção durável é remover o juízo, não corrigir os casos.

### 1.3 R6 — a factura pode nascer `EMITIDA`

`trg_facturas_imutaveis` é `BEFORE UPDATE OR DELETE`, e não cobre `INSERT`. Um `INSERT`
directo com `estado = 'EMITIDA'` é aceite.

Como a ADR-040 §R6 já regista, isto difere em espécie do buraco fechado em
`itens_factura`: aquele era **mutação** de um documento selado; este é **fabricação**. A
fabricação já é detectável — os índices únicos impedem reutilizar número ou
`(série, sequencial)`, a CHECK obriga ao conjunto completo de campos de emissão, e a
emissão legítima seguinte lê `series.ultimo_hash` e não a linha fabricada, deixando-a
órfã da cadeia. Mas o invariante *"toda a factura nasce RASCUNHO"* é genuíno e barato.

## 2. Decisão

### 2.1 Um único pacote de protecção (R3)

```go
protecao := []gin.HandlerFunc{limiteMW, authMW, mfaMW}
```

passado aos **14** grupos de negócio. Uniformizar o nome do parâmetro em
`RegistarIdentidade` (`middlewares` → `protecao`), alinhando-o com os outros treze.

**Custo zero onde não há papéis sensíveis.** `MFAObrigatoria` delega em
`dominio.VerificarAutenticacaoForte`, que só rejeita sessões de papel sensível. Aplicá-lo
a um grupo sem papéis sensíveis é um no-op — pelo que a regra uniforme não exige julgar
quem "precisa".

**Acrescentar um grupo desprotegido passa a exigir desvio deliberado e visível**, em vez
de esquecimento.

### 2.2 `RegistarHealth` fica deliberadamente de fora

Não recebe protecção e **não pode receber**: os healthchecks e o *scrape* do Prometheus
são não-autenticados por desenho. Fica registado aqui para que ninguém o "corrija" mais
tarde por simetria — um "aplicar a todos" ingénuo partiria a observabilidade e os
healthchecks do Docker.

### 2.3 Credenciais OTP no realm de desenvolvimento

Estado actual de `docker/keycloak/realm-sgc.json`:

| Utilizador | Papéis | Credenciais |
|---|---|---|
| `medico.teste` | `Medico` | password |
| `admin.teste` | `Admin` | password |
| `director.teste` | `Director` | password, **otp** |
| `tesoureiro.teste` | `Tesoureiro` | password, **otp** |

`Admin` é sensível e **não tem OTP**; não existem utilizadores `DPO` nem `Auditor`. Sem
corrigir isto, o percurso positivo do MFA nunca é exercitado para esses papéis — os
testes passariam sem provar nada, que é o modo de falha que esta fatia existe para evitar.

Acrescentar OTP ao `admin.teste`, e criar `dpo.teste` e `auditor.teste` espelhando
`director.teste`.

### 2.4 A factura nasce RASCUNHO (R6)

Migração nova `migrations/financeiro/0004_facturas_nascem_rascunho.sql`, forward-only:
trigger `BEFORE INSERT ON financeiro.facturas` com `WHEN (NEW.estado <> 'RASCUNHO')` que
levanta excepção. **Não se edita `0001`, `0002` nem `0003`** — editar uma migração já
aplicada causou um incidente na Sprint 15.

### 2.5 O que esta fatia NÃO garante

**Enquanto o R7 estiver aberto, o R6 é contornável pela própria aplicação.** O papel da
aplicação é dono das tabelas fiscais e pode correr `ALTER TABLE … DISABLE TRIGGER` — foi
demonstrado na revisão final da ADR-040. O trigger é defesa em profundidade contra erro e
contra SQL directo de terceiros, **não** contra a aplicação comprometida.

Esta frase é obrigatória na ADR. Omiti-la repetiria o defeito que a ADR-041 corrigiu na
ADR-040: prometer no documento uma garantia que o código não dá.

## 3. Âmbito da alteração

| Ficheiro | Alteração |
|---|---|
| `internal/platform/app.go` | pacote `protecao` nos 14 grupos |
| `internal/adapters/http/identidade_handler.go` | `middlewares` → `protecao` |
| `internal/adapters/http/*_test.go` | routers de teste passam a espelhar a produção (`MFAObrigatoria()`); sessões de papel sensível ganham `AutenticacaoForte: true` |
| `docker/keycloak/realm-sgc.json` | OTP no `admin.teste`; `dpo.teste` e `auditor.teste` novos |
| `migrations/financeiro/0004_facturas_nascem_rascunho.sql` (criar) | trigger de INSERT |
| `tests/integration/facturas_test.go` | 3 fixturas passam a INSERT RASCUNHO → UPDATE |
| `adrs/ADR-042-mfa-uniforme.md` (criar) | ADR |
| `adrs/ADR-040-emissao-factura.md` | R3 e R6 marcados resolvidos (aditivo) |
| `CLAUDE.md`, `SPRINT.md` | marco e sprint |

**Não muda:** o domínio (`internal/domain/`), a canonicalização do hash, o vector dourado,
as rotas, o RBAC de cada rota.

## 4. Testes

- **Um teste negativo por família que expõe papéis sensíveis** — Doentes, Consentimentos,
  Laboratório, Recepção: papel sensível **sem** segundo factor → **403**, com o
  `type: mfa-obrigatorio` asserido no corpo. Verificar só o código 403 não distingue o do
  MFA do do RBAC, e essa ambiguidade já foi assinalada na Sprint 15.
- **Um teste positivo por família:** com segundo factor, prossegue.
- **R6:** `INSERT` directo de factura `EMITIDA` → rejeitado; `INSERT` de `RASCUNHO` →
  continua a passar. Fechar o caminho normal seria pior do que o buraco.
- As três fixturas reescritas continuam a provar o que provavam (imutabilidade das
  emitidas), agora pelo caminho de produção.
- Integração contra a BD de desenvolvimento **e** contra uma criada de raiz, duas corridas.

**Risco de teste a vigiar:** esta é uma alteração larga e rasa. O modo de falha próprio é
um teste que passa a verde pela razão errada — por exemplo, um grupo que ficou sem
`mfaMW` e cujo teste nunca usou papel sensível, ou uma sessão que ganhou
`AutenticacaoForte: true` sem que o teste exercite o caminho. Os testes negativos por
família são a defesa; a mutação (retirar o `mfaMW` do pacote e confirmar que falham) é a
prova.

### 4.1 A lacuna que só apareceu ao medir: os testes não verificam o `app.go`

Ao levantar o raio de impacto descobriu-se que **os routers de teste constroem as suas
próprias cadeias de middlewares**. `routerDoentes` passa apenas `adhttp.Auth(...)`;
`routerFin` passa `Auth` **e** `MFAObrigatoria()`, porque a ADR-041 o alterou.

Duas consequências, e a segunda é a que importa:

1. Alterar o `app.go` sozinho **não parte teste nenhum**. Uma versão anterior deste
   desenho afirmava que ~12 ficheiros passariam a receber 403; **era falso**, e fica
   registado por ser exactamente a classe de erro que estas ADR têm vindo a apanhar —
   uma afirmação plausível que ninguém tinha medido.
2. **Nada, hoje, verifica a ligação em `app.go`.** Cada router de teste redeclara a
   protecção por sua conta, pelo que retirar o `mfaMW` da produção não faria falhar
   nenhum teste. A exposição do R3 existiu por isso: não houve como a detectar.

Portanto a fatia tem de fazer as duas coisas:

- **Os routers de teste passam a espelhar a produção**, aplicando `MFAObrigatoria()` como
  o `routerFin` já faz. Só então as sessões de papel sensível precisam de
  `AutenticacaoForte: true`, e só então os testes exercitam a cadeia real.
- **Uma guarda sobre o próprio `app.go`.** Com um pacote `protecao` único, a divergência
  exige desvio deliberado — mas "exige desvio deliberado" não é o mesmo que "é detectado".
  A guarda proposta é um teste que lê o código-fonte de `app.go` e exige que toda a
  chamada `adhttp.Registar*` dentro de `registarRotas` termine em `protecao...`, com
  `RegistarHealth` explicitamente isento.

  É um teste invulgar — inspecciona código-fonte em vez de comportamento — e regista-se
  aqui a razão de se preferir mesmo assim: a alternativa comportamental exigiria montar
  os catorze handlers com todos os seus fakes só para provar uma propriedade de ligação,
  e o que falhou aqui foi precisamente a ligação, não o comportamento. Se a revisão o
  julgar frágil (por exemplo por depender de formatação), a alternativa aceitável é
  extrair `registarRotas` para uma função testável que receba os handlers e devolva a
  lista de registos.

## 5. Fora do âmbito

- **R7** — retirar ao papel da aplicação a propriedade das tabelas fiscais. Exige separar
  a credencial de migração da de runtime (a aplicação aplica as suas próprias migrações no
  arranque, `app.go:322`, com o mesmo `DATABASE_URL`), o que toca config,
  `docker-compose`, CI e o arranque. Fatia própria, com desenho próprio.
- **Anulação** de factura (vinculada pelo R5 da ADR-040: não pode apagar nem renumerar).
- **Pagamentos** e EMIS Multicaixa.
- **SAF-T-AO** e certificação junto da AGT.
