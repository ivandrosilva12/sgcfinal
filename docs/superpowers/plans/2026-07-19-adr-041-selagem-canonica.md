# Selagem canónica (ADR-041) — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tornar a composição canónica do hash da `Factura` injectiva e alargar o selo à identidade completa do cliente e à proveniência de cada linha, fechando o R1 e o R2 da ADR-040 antes da primeira emissão em produção.

**Architecture:** Alteração puramente de domínio, num único ficheiro de produção. Introduz-se um auxiliar `enquadrar` que prefixa cada campo de texto com o seu comprimento em bytes; `digestLinhas` e `HashDe` passam a usá-lo em todos os campos de texto, e o canónico ganha `clienteNome`, `clienteMorada` e `operacaoID`. Não há migração: as três colunas já existem e estão preenchidas.

**Tech Stack:** Go 1.22+, `crypto/sha256` e `strconv` da stdlib. Sem dependências novas.

**Spec:** `docs/superpowers/specs/2026-07-19-adr-041-selagem-canonica-design.md`

## Global Constraints

- **Idioma:** PT-PT angolano **com diacríticos** em código, comentários, mensagens de erro e commits. Nunca PT-BR, nunca inglês em texto visível. Três mensagens de commit sem acentos foram devolvidas na sprint anterior.
- **Linguagem ubíqua:** `Factura`, `ItemFactura`, `Cliente`, `Série`, `Número`. Nunca `Invoice`/`Bill`.
- **Camadas:** `internal/domain/financeiro/` não importa `pgx`, `gin` nem `net/http`.
- **Erros:** nunca `panic()`. Sempre `erros.Novo(erros.Categoria…, "mensagem")`.
- **Migrações:** forward-only. **Esta fatia não cria nem edita nenhuma migração.** Se concluíres que precisas de uma, **pára e reporta** — editar uma migração já aplicada causou um incidente na sprint anterior.
- **Regra de enquadramento (normativa):** todo o campo de **texto** leva prefixo de comprimento em bytes; os **inteiros vão nus**. Sem excepções.
- **Campos ausentes** canonicalizam-se como `0:` — nunca `null` nem `<nil>`.
- **Cobertura:** domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
- **`go-arch-lint check` tem de sair com código 0 e zero notices.** Verificar o **código de saída**, não só a saída em texto.

---

## File Structure

| Ficheiro | Responsabilidade |
|---|---|
| `internal/domain/financeiro/factura.go` (modificar) | `enquadrar`, `digestLinhas`, `HashDe` — o único sítio onde a canonicalização vive |
| `internal/domain/financeiro/factura_test.go` (modificar) | Vector dourado novo; regressão da colisão; um teste por campo recém-selado |
| `tests/integration/facturas_test.go` (modificar) | Série por corrida no teste da cadeia |
| `adrs/ADR-041-selagem-canonica.md` (criar) | ADR |
| `adrs/ADR-040-emissao-factura.md` (modificar) | R1 e R2 marcados como resolvidos |
| `CLAUDE.md`, `SPRINT.md` (modificar) | Marco e sprint |

**Ordem obrigatória:** a Task 1 (série por corrida) vem **antes** da Task 2 (canonicalização). Se a ordem se inverter, a suite de integração fica vermelha entre commits, porque a série `2999` acumula facturas com hashes no formato antigo e o `VerificarCadeia` recalculá-las-ia com a função nova.

---

## Task 1: Série por corrida no teste da cadeia

**Files:**
- Modify: `tests/integration/facturas_test.go` (função `TestEmitirFacturas_NumeracaoSemBuracosSobConcorrencia`, cabeçalho ~linha 775-790)

**Interfaces:**
- Consumes: `gerarUUIDv4(t *testing.T) string` (já existe no ficheiro); `pgrepo.NovoRepositorioFacturas(pool)`; `repo.Emitir(ctx, id, momento) (*fin.Factura, error)`.
- Produces: nada para tarefas seguintes. É uma correcção de robustez do teste.

**Contexto que precisas de saber:** `repo.Emitir` deriva a série do **ano** do `momento`, via `fin.SerieDe(momento)`. Não se passa a série como argumento — muda-se o ano do `momento`. Hoje o teste fixa `const serie = "2999"` e `momento` no ano 2999, pelo que todas as corridas partilham a mesma cadeia, que é imortal (as facturas EMITIDA não são removíveis, por desenho do trigger de imutabilidade).

- [ ] **Step 1: Escrever o teste que falha**

Não há teste novo a escrever — o que muda é a fixtura do teste existente. Substituir o bloco da série fixa (as linhas com `const serie = "2999"` e a atribuição de `momento` logo a seguir) por:

```go
	// Série própria desta corrida. A série é derivada do ANO do momento por
	// fin.SerieDe, logo escolhe-se um ano ao acaso numa banda reservada.
	//
	// Porquê ao acaso, e não um ano fixo: as facturas EMITIDA são imortais (o
	// trigger de imutabilidade impede apagá-las), pelo que uma série fixa acumula
	// os elos de todas as corridas anteriores. Basta a canonicalização do hash
	// mudar uma vez para que os elos antigos deixem de fechar, e o teste passa a
	// acusar uma quebra que não é da cadeia mas da história. Uma série por corrida
	// torna o teste auto-contido e imune a futuras mudanças de formato.
	//
	// A banda evita o 2999, que já tem elos em formato anterior ao ADR-041.
	ano := 2100 + mathrand.Intn(800)
	serie := strconv.Itoa(ano)
	momento := time.Date(ano, 1, 15, 9, 0, 0, 0, time.UTC)
```

Acrescentar aos imports do ficheiro:

```go
	mathrand "math/rand"
	"strconv"
```

**Nota sobre `math/rand`:** é deliberado e adequado — serve para dispersar séries de teste, não para nada criptográfico. O `crypto/rand` já está importado no ficheiro como `rand` (usado por `gerarUUIDv4`), daí o alias `mathrand` para não colidir.

**Nota sobre colisões de ano:** se duas corridas calharem no mesmo ano, a segunda apenas acrescenta elos a uma cadeia que já está no formato corrente — continua a fechar. A colisão é inofensiva; o que a série por corrida elimina é a transição de formato dentro da mesma cadeia.

- [ ] **Step 2: Correr e confirmar que passa (a série muda, o comportamento não)**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run NumeracaoSemBuracos -count=1 -v`
Expected: PASS. O teste continua a provar o mesmo (12 emissões concorrentes, sequenciais 1..12 contíguos, cadeia íntegra), agora numa série virgem.

- [ ] **Step 3: Correr duas vezes seguidas para provar a repetibilidade**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run NumeracaoSemBuracos -count=2`
Expected: PASS. Cada corrida usa (quase sempre) uma série diferente, e `base` é 0 numa série virgem.

- [ ] **Step 4: Correr a suite de integração completa**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/... -count=1`
Expected: PASS

- [ ] **Step 5: Confirmar formatação e vet**

Run: `gofmt -l . && go vet ./...`
Expected: sem saída em ambos.

- [ ] **Step 6: Commit**

```bash
git add tests/integration/facturas_test.go
git commit -m "test(financeiro): série por corrida no teste da cadeia (ADR-041)"
```

---

## Task 2: Nova canonicalização — enquadramento injectivo e âmbito do selo

**Files:**
- Modify: `internal/domain/financeiro/factura.go` (funções `digestLinhas` e `HashDe`; acrescentar `enquadrar`)
- Test: `internal/domain/financeiro/factura_test.go`

**Interfaces:**
- Consumes: `SnapshotFactura{Serie, Sequencial, DataEmissao, Cliente{Nome,NIF,Morada}, Itens, HashAnterior}`; `ItemFactura{Descricao, Tipo, Quantidade, PrecoUnitario, RegimeIVA, OperacaoID}`; `TotaisDe(itens) Totais`.
- Produces: `HashDe(s SnapshotFactura) string` — mesma assinatura, novo formato. Nenhum chamador muda.

**Porque é uma só tarefa:** o enquadramento e o alargamento do selo são a mesma alteração ao formato canónico. Separá-los obrigaria a calcular e registar um vector dourado intermédio que ficaria obsoleto no commit seguinte.

- [ ] **Step 1: Escrever os testes que falham**

Acrescentar a `internal/domain/financeiro/factura_test.go`. Auxiliar primeiro, para os testes não repetirem a construção:

```go
// facturaDe constrói e emite uma factura com os parâmetros dados, para os testes
// de canonicalização poderem variar um campo de cada vez.
func facturaDe(t *testing.T, nome, nif, morada string, itens func(*fin.Factura) error) fin.SnapshotFactura {
	t.Helper()
	c, err := fin.NovoClienteSnapshot(nome, nif, morada)
	if err != nil {
		t.Fatalf("cliente: %v", err)
	}
	f, err := fin.NovaFactura(c, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("factura: %v", err)
	}
	if err := itens(f); err != nil {
		t.Fatalf("itens: %v", err)
	}
	m := time.Date(2026, 7, 18, 10, 0, 0, 123456789, time.UTC)
	if err := f.Emitir("2026", 7, "abc", m); err != nil {
		t.Fatalf("Emitir: %v", err)
	}
	return f.Snapshot()
}

// Regressão do defeito que motivou a ADR-041. Antes do enquadramento, uma
// descrição com '|' e '\n' conseguia imitar a fronteira entre linhas: a factura
// A (duas linhas, a primeira a preço zero) e a factura B (uma linha cuja
// descrição embebe os separadores) produziam o MESMO hash e o MESMO total.
// Verificado contra o código real antes desta fatia:
//
//	A: 2 linhas, total 5000 | hash cac4fec4b7e103a6f232cb2eacb7dd15c8...
//	B: 1 linha,  total 5000 | hash cac4fec4b7e103a6f232cb2eacb7dd15c8...
//
// Os totais no canónico não bastavam: a CHECK admite preço zero, que fornece
// exactamente a folga necessária para os totais coincidirem.
func TestHash_DescricaoNaoImitaFronteiraDeLinha(t *testing.T) {
	a := facturaDe(t, "Sol", "", "", func(f *fin.Factura) error {
		if err := f.AdicionarItem("Z", fin.LinhaConsulta, "", 1,
			moeda.DeCentimos(0), fin.RegimeIsento); err != nil {
			return err
		}
		return f.AdicionarItem("W", fin.LinhaConsulta, "", 1,
			moeda.DeCentimos(5000), fin.RegimeIsento)
	})
	b := facturaDe(t, "Sol", "", "", func(f *fin.Factura) error {
		return f.AdicionarItem("Z|CONSULTA|1|0|ISENTO\n1|W", fin.LinhaConsulta, "", 1,
			moeda.DeCentimos(5000), fin.RegimeIsento)
	})

	// Pré-condição do teste: os totais TÊM de coincidir, senão a colisão seria
	// impedida pelos totais e o teste não estaria a exercitar o digest.
	if ta, tb := fin.TotaisDe(a.Itens), fin.TotaisDe(b.Itens); ta.Total != tb.Total {
		t.Fatalf("fixtura inválida: totais diferem (%d vs %d) — o teste deixaria de exercitar o digest",
			ta.Total.Centimos(), tb.Total.Centimos())
	}
	if fin.HashDe(a) == fin.HashDe(b) {
		t.Errorf("colisão: %d linhas e %d linhas partilham o hash %s",
			len(a.Itens), len(b.Itens), fin.HashDe(a))
	}
}

// O nome do cliente passa a ser selado (ADR-041). Antes não era: numa factura a
// consumidor final (sem NIF), o nome é a única identificação do documento, e
// era alterável com o selo intacto.
func TestHash_SelaNomeDoCliente(t *testing.T) {
	comItem := func(f *fin.Factura) error {
		return f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1,
			moeda.DeCentimos(50000), fin.RegimeIsento)
	}
	sol := facturaDe(t, "Sol", "", "", comItem)
	lua := facturaDe(t, "Lua", "", "", comItem)
	if fin.HashDe(sol) == fin.HashDe(lua) {
		t.Error("mudar o nome do cliente tinha de mudar o hash")
	}
}

// A morada do cliente passa a ser selada (ADR-041).
func TestHash_SelaMoradaDoCliente(t *testing.T) {
	comItem := func(f *fin.Factura) error {
		return f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1,
			moeda.DeCentimos(50000), fin.RegimeIsento)
	}
	sem := facturaDe(t, "Sol", "", "", comItem)
	com := facturaDe(t, "Sol", "", "Rua Amílcar Cabral, Luanda", comItem)
	if fin.HashDe(sem) == fin.HashDe(com) {
		t.Error("mudar a morada do cliente tinha de mudar o hash")
	}
}

// O operacaoID passa a ser selado (ADR-041): é a proveniência cross-context da
// linha (que dispensa ou requisição a originou). Sem selo, podia ser reapontado
// sem invalidar a factura.
func TestHash_SelaOperacaoIDDaLinha(t *testing.T) {
	comOp := func(op string) func(*fin.Factura) error {
		return func(f *fin.Factura) error {
			return f.AdicionarItem("Paracetamol", fin.LinhaDispensa, op, 2,
				moeda.DeCentimos(1000), fin.RegimeStandard)
		}
	}
	a := facturaDe(t, "Sol", "", "", comOp("22222222-2222-2222-2222-222222222222"))
	b := facturaDe(t, "Sol", "", "", comOp("33333333-3333-3333-3333-333333333333"))
	if fin.HashDe(a) == fin.HashDe(b) {
		t.Error("mudar o operacaoID da linha tinha de mudar o hash")
	}
}
```

**Um teste que NÃO se escreve, e porquê:** o spec menciona "ausente vs vazio" para a morada. Em Go, `Morada` é `string`, logo ausente **é** `""` — os dois casos são o mesmo valor e o teste seria tautológico. Fica registado aqui para que ninguém o acrescente mais tarde a pensar que falta.

- [ ] **Step 2: Correr e confirmar que falham**

Run: `go test ./internal/domain/financeiro/ -run 'TestHash_(DescricaoNaoImita|SelaNome|SelaMorada|SelaOperacaoID)' -v`
Expected: FAIL nos quatro:
- `TestHash_DescricaoNaoImitaFronteiraDeLinha` → `colisão: 2 linhas e 1 linhas partilham o hash cac4fec4…`
- `TestHash_SelaNomeDoCliente` → `mudar o nome do cliente tinha de mudar o hash`
- `TestHash_SelaMoradaDoCliente` → `mudar a morada do cliente tinha de mudar o hash`
- `TestHash_SelaOperacaoIDDaLinha` → `mudar o operacaoID da linha tinha de mudar o hash`

Se algum **passar** nesta fase, pára e reporta: significa que a premissa da ADR-041 não corresponde ao código.

- [ ] **Step 3: Implementar o enquadramento**

Em `internal/domain/financeiro/factura.go`, acrescentar `enquadrar` imediatamente antes de `digestLinhas`:

```go
// enquadrar prefixa um campo de texto com o seu comprimento em bytes, tornando a
// composição canónica injectiva: nenhum conteúdo consegue imitar um separador,
// porque quem lê consome exactamente os bytes anunciados.
//
// A regra é cega de propósito — aplica-se a todo o campo de texto, sem excepções.
// O defeito que a ADR-041 corrige nasceu precisamente de se ter julgado quais os
// campos eram seguros ("as descrições vêm de catálogo") e de esse juízo estar
// errado. Uma regra sem excepções não pode ser mal julgada por quem vier a seguir.
func enquadrar(s string) string { return strconv.Itoa(len(s)) + ":" + s }
```

Substituir o corpo de `digestLinhas` por:

```go
func digestLinhas(itens []ItemFactura) string {
	h := sha256.New()
	for ordem, it := range itens {
		// hash.Hash.Write nunca devolve erro (contrato do pacote hash), pelo que o
		// retorno se descarta explicitamente — errcheck exige-o e um panic aqui
		// seria pior: partiria a emissão por uma condição que não pode ocorrer.
		_, _ = fmt.Fprintf(h, "%d|%s|%s|%d|%d|%s|%s\n", ordem,
			enquadrar(it.Descricao), enquadrar(string(it.Tipo)),
			it.Quantidade, it.PrecoUnitario.Centimos(),
			enquadrar(string(it.RegimeIVA)), enquadrar(it.OperacaoID))
	}
	return hex.EncodeToString(h.Sum(nil))
}
```

Substituir o corpo de `HashDe` por:

```go
func HashDe(s SnapshotFactura) string {
	t := TotaisDe(s.Itens)
	canonico := strings.Join([]string{
		enquadrar(s.Serie),
		strconv.Itoa(s.Sequencial),
		enquadrar(s.DataEmissao.UTC().Truncate(time.Second).Format(time.RFC3339)),
		enquadrar(s.Cliente.Nome),
		enquadrar(s.Cliente.NIF),
		enquadrar(s.Cliente.Morada),
		strconv.FormatInt(t.Subtotal.Centimos(), 10),
		strconv.FormatInt(t.TotalIVA.Centimos(), 10),
		strconv.FormatInt(t.Total.Centimos(), 10),
		enquadrar(digestLinhas(s.Itens)),
		enquadrar(s.HashAnterior),
	}, "|")
	soma := sha256.Sum256([]byte(canonico))
	return hex.EncodeToString(soma[:])
}
```

Actualizar o comentário de cabeçalho do pacote (linhas 1-5 de `factura.go`) para referir a ADR-041:

```go
// Package financeiro é o Bounded Context Financeiro (Camada 1 — Domínio).
// O agregado Factura nasce em RASCUNHO (ADR-039): linhas com tipo e snapshot,
// cálculo de IVA e totais. A emissão (ADR-040) fixa número, data e o hash
// SHA-256 canónico — invariante do agregado, calculado aqui, nunca num serviço.
// A composição canónica é injectiva por enquadramento (ADR-041): todo o campo de
// texto leva prefixo de comprimento, e o selo cobre a identidade completa do
// cliente e a proveniência de cada linha.
package financeiro
```

- [ ] **Step 4: Correr e confirmar que os quatro passam**

Run: `go test ./internal/domain/financeiro/ -run 'TestHash_(DescricaoNaoImita|SelaNome|SelaMorada|SelaOperacaoID)' -v`
Expected: PASS nos quatro.

- [ ] **Step 5: Recalcular e registar o vector dourado**

O `TestHash_VectorDourado` vai agora **falhar**, e isso é o comportamento correcto: ele existe para gritar quando a canonicalização muda.

Run: `go test ./internal/domain/financeiro/ -run VectorDourado -v`
Expected: FAIL, com a mensagem `hash = "<novo>", queria "8caeeee0017219380ffbca9560b2d24894b07a45ba1fdb63a6cc4710293cc169"`.

Actualizar a constante em `factura_test.go:418` com o valor **novo, lido dessa saída**, e o comentário que a acompanha:

```go
// Vector dourado: fixa o hash canónico de uma factura conhecida. Se a
// canonicalização mudar (ordem dos campos, enquadramento, âmbito do selo,
// formato da data), este teste falha — é a única salvaguarda contra tornar
// irreproduzível a cadeia das facturas já emitidas (retenção AGT/SAF-T-AO,
// 10 anos). Valor recalculado no ADR-041, que tornou o canónico injectivo e
// acrescentou nome, morada e operacaoID ao selo.
const hashDourado = "<valor lido da saída do teste>"
```

**Conferência, não autoridade.** Um protótipo desta fatia produziu
`6f5a535c960536cc759cd10d589599d916b58f23087c5faa2cc5c80ff5f59ff9` para esta mesma fixtura. Se o valor que obtiveres **coincidir**, é boa confirmação. Se **divergir**, **pára e reporta a divergência** — não ajustes nada nem investigues sozinho. O protótipo pode estar errado (não usa `TotaisDe` real), mas a divergência também pode denunciar um erro na implementação, e distinguir os dois casos exige comparar o canónico byte a byte.

**Nunca ajustes a constante a um valor que não venha da implementação real.** Na ADR-040, um vector dourado fornecido a partir de uma fixtura descrita em prosa custou 736 tentativas falhadas e uma tarefa bloqueada; o implementador parou em vez de o forçar, e fez bem — forçá-lo teria produzido um teste que afirma a implementação em vez da especificação.

- [ ] **Step 6: Correr o pacote inteiro**

Run: `go test ./internal/domain/financeiro/ -race -cover -count=1`
Expected: PASS, cobertura ≥85%.

Nota: os testes que reconstroem uma factura emitida e recalculam o elo (`ReconstruirFactura` → `HashDe`) continuam a passar sem alteração — o formato mudou dos dois lados.

- [ ] **Step 7: Correr a suite completa e a integração**

Run: `go test ./... -race -count=1`
Expected: PASS

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/... -count=1`
Expected: PASS. A série por corrida da Task 1 é o que torna isto verdade — sem ela, o `VerificarCadeia` recalcularia os elos antigos da série `2999` com a função nova e acusaria uma quebra.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/financeiro/factura.go internal/domain/financeiro/factura_test.go
git commit -m "feat(financeiro): canónico injectivo e selo alargado ao cliente e à proveniência (ADR-041)"
```

---

## Task 3: ADR-041, resolução do R1/R2 e actualização de marco

**Files:**
- Create: `adrs/ADR-041-selagem-canonica.md`
- Modify: `adrs/ADR-040-emissao-factura.md` (secções R1 e R2)
- Modify: `CLAUDE.md` (§6 marco actual, índice de ADRs, `Próximo ADR`)
- Modify: `SPRINT.md` (Sprint 16)

**Interfaces:**
- Consumes: o valor do vector dourado fixado na Task 2.
- Produces: nada em código.

- [ ] **Step 1: Escrever a ADR-041**

Criar `adrs/ADR-041-selagem-canonica.md`, seguindo o formato de `adrs/ADR-040-emissao-factura.md` (secções: Contexto, Decisão, Alternativas rejeitadas, Consequências, Riscos e dívida registada, Fora do âmbito, Diferido).

Conteúdo obrigatório, e **cada afirmação tem de ser verdadeira face ao código** — verifica antes de escrever:

1. **O formato canónico normativo**, tal como implementado, incluindo a regra de enquadramento e o `0:` para campos ausentes.
2. **O vector dourado novo**, com o valor real fixado na Task 2 e a fixtura que o produz.
3. **A colisão que motivou a fatia**, com os dois hashes iguais de antes (`cac4fec4…`) e a explicação de porque é que os totais não bastavam (a CHECK admite preço zero).
4. **A correcção do registo da ADR-040**: a justificação lá escrita — *"o risco prático é baixo (as descrições vêm de catálogo)"* — era factualmente falsa; `Descricao` chega no corpo do pedido e é validada só por `TrimSpace` e não-vazio.
5. **O segundo defeito**, não previsto no R1 nem no R2: nome e morada fora do selo, e o que isso significa numa factura a consumidor final sem NIF.
6. **A decisão consciente de deixar `ItemFactura.ID` fora do selo** — chave substituta sem significado fiscal.
7. **As alternativas rejeitadas** e porquê: `%q` (ata o formato fiscal às regras de quoting do Go, que terceiros teriam de replicar) e SHA-256 por campo (canónico deixa de ser inspeccionável).

- [ ] **Step 2: Marcar o R1 e o R2 da ADR-040 como resolvidos**

Em `adrs/ADR-040-emissao-factura.md`, nas secções `### R1` e `### R2`, **acrescentar** (sem apagar o texto original — é registo histórico do que se sabia na altura) uma nota de resolução:

```markdown
> **Resolvido pela ADR-041 (2026-07-19).** O enquadramento por prefixo de
> comprimento tornou o canónico injectivo. Registe-se que a avaliação de risco
> acima estava errada: as descrições **não** vêm de catálogo — chegam no corpo do
> pedido, validadas apenas por `TrimSpace` e não-vazio — e a colisão foi produzida
> contra o código real, não deduzida.
```

Para o R2, a nota deve dizer que `operacaoID` **passou** a ser selado e que `ItemFactura.ID` ficou deliberadamente fora.

- [ ] **Step 3: Actualizar CLAUDE.md**

- §6: acrescentar o parágrafo do marco (Sprint 16, ADR-041 entregue).
- Índice de ADRs registadas: acrescentar `adrs/ADR-041-selagem-canonica.md`.
- `Próximo ADR:` passa a **ADR-042**.

- [ ] **Step 4: Actualizar SPRINT.md**

Acrescentar a secção da Sprint 16 seguindo o formato das anteriores, com os critérios cumpridos.

- [ ] **Step 5: Correr todos os gates**

```bash
go build ./...
go vet ./...
gofmt -l .
go test ./... -race -count=1
DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/... -count=1
golangci-lint run
go-arch-lint check; echo "arch-lint exit=$?"
```

Expected: build/vet/gofmt sem saída; testes PASS; `golangci-lint` `0 issues.`; **`go-arch-lint` com `exit=0` e zero notices**.

Verificar o **código de saída** do `go-arch-lint`, não só o texto: na sprint anterior um `notice` foi tratado como benigno por vários revisores e o comando saía com 1, o que teria partido o CI no push.

- [ ] **Step 6: Correr a integração contra uma base de dados criada de raiz**

A BD de desenvolvimento tem história; o CI cria a BD do zero. Nesta sprint já houve dois testes verdes em desenvolvimento e vermelhos em CI, e é exactamente esta fatia que muda o formato do hash — o sítio onde história antiga e código novo se encontram.

```bash
docker exec sgc-postgres-1 psql -U sgc -d postgres -c "CREATE DATABASE sgc_adr041;"
DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc_adr041?sslmode=disable' \
  go test -tags=integration ./tests/integration/... -count=1
DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc_adr041?sslmode=disable' \
  go test -tags=integration ./tests/integration/... -count=1
docker exec sgc-postgres-1 psql -U sgc -d postgres -c "DROP DATABASE sgc_adr041;"
```

Expected: PASS nas duas corridas (as migrações aplicam-se de raiz na primeira). Apagar a base no fim.

**Nota:** a BD de desenvolvimento tem 3 facturas EMITIDA permanentes (`FAC 2026/09999997`, `09999998`, `09999999`) com `hash = 'abc'` literal — são fixturas de imutabilidade, nunca passam por `VerificarCadeia`, e não são afectadas por esta fatia. Não as tentes apagar: o trigger impede-o, por desenho.

- [ ] **Step 7: Confirmar a cobertura**

Run: `go test ./internal/domain/financeiro/ ./internal/application/financeiro/ -cover -count=1`
Expected: domínio ≥85%, aplicação ≥75%.

- [ ] **Step 8: Commit**

```bash
git add adrs/ CLAUDE.md SPRINT.md
git commit -m "docs(financeiro): ADR-041 e resolução do R1/R2 da ADR-040 (ADR-041)"
```

---

## Verificação final

- [ ] `graphify update .` para manter o grafo de conhecimento actual.
- [ ] Confirmar que **nenhuma migração foi criada nem alterada** nesta fatia: `git diff main --name-only -- migrations/` tem de vir vazio.
- [ ] Confirmar que o único ficheiro de produção alterado é `internal/domain/financeiro/factura.go`: `git diff main --name-only -- internal/` .
