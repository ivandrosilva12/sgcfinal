# Emissão da Factura (ADR-040) — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tornar a `Factura` um documento fiscal: transição `RASCUNHO → EMITIDA` com numeração sequencial por série sem buracos, cadeia hash SHA-256 verificável e imutabilidade imposta pela BD.

**Architecture:** O hash é invariante do agregado — calculado dentro de `Factura.Emitir`, nunca num serviço. A serialização (atribuição de sequencial e do elo anterior) vive no adaptador de persistência, numa transacção que bloqueia a linha da série com `FOR UPDATE`. A imutabilidade é defesa em profundidade: guarda no domínio mais trigger na BD.

**Tech Stack:** Go 1.22+, pgx v5 (SQL puro, sem ORM), PostgreSQL 16, Gin, `crypto/sha256` da stdlib.

**Spec:** `docs/superpowers/specs/2026-07-18-adr-040-emissao-factura-design.md`

## Global Constraints

- **Idioma:** PT-PT angolano em código, comentários, mensagens de erro, commits. Nunca PT-BR, nunca EN em texto visível.
- **Linguagem ubíqua:** `Factura`, `ItemFactura`, `Cliente`, `Serie`, `Numero`. Nunca `Invoice`/`Bill`.
- **Camadas:** `internal/domain/financeiro/` não importa `pgx`, `gin` nem `net/http`. A serialização é da camada 3.
- **Migrações:** forward-only, sem `.down.sql`. Nunca editar `0001_facturas.sql`.
- **Erros:** nunca `panic()`. Sempre `erros.Novo(erros.Categoria…, "mensagem")`.
- **Cobertura:** domínio ≥85%, aplicação ≥75%, adapters ≥60%.
- **Canonicalização do hash (exacta, não negociável):** `dataEmissao` em UTC truncada ao segundo com `time.RFC3339` (nunca `RFC3339Nano`); montantes em cêntimos `int64`; `clienteNIF` ausente é `""`, nunca `null` nem `<nil>`; `hashAnterior` da primeira factura da série é `""`.
- **Hash em hexadecimal minúsculo** (`hex.EncodeToString`).

---

## File Structure

| Ficheiro | Responsabilidade |
|---|---|
| `internal/domain/financeiro/numero.go` (criar) | VO `NumeroFactura`: formatação, parsing, `SerieDe` |
| `internal/domain/financeiro/numero_test.go` (criar) | Testes do VO |
| `internal/domain/financeiro/factura.go` (modificar) | Campos de emissão, `Emitir`, `TotaisDe`, hash canónico |
| `internal/domain/financeiro/cadeia.go` (criar) | `VerificarCadeia` — função pura |
| `internal/domain/financeiro/cadeia_test.go` (criar) | Testes da verificação |
| `migrations/financeiro/0002_emissao_facturas.sql` (criar) | Colunas, UNIQUE, CHECK, tabela `series`, trigger |
| `internal/adapters/pgrepo/facturas_repo.go` (modificar) | Versão no `Guardar`; `Emitir` serializado; `ListarSnapshotsPorSerie` |
| `internal/application/financeiro/facturas.go` (modificar) | `CasoEmitirFactura`, `CasoVerificarCadeia` |
| `internal/application/financeiro/ports.go` (modificar) | Campos de emissão em `DetalheFactura`; `ResultadoVerificacao` |
| `internal/adapters/http/financeiro_handler.go` (modificar) | Rotas `POST …/emitir` e `GET …/cadeia/verificacao` |
| `internal/domain/identidade/papel.go` (modificar) | `PapelTesoureiro` em `papeisSensiveis` |
| `docs/ERRATA-002-papel-tesoureiro.md` (modificar) | Bloco de revisão aditivo |
| `adrs/ADR-040-emissao-factura.md` (criar) | ADR |

---

## Task 1: VO `NumeroFactura`

**Files:**
- Create: `internal/domain/financeiro/numero.go`
- Test: `internal/domain/financeiro/numero_test.go`

**Interfaces:**
- Consumes: nada.
- Produces: `type NumeroFactura string`; `NovoNumeroFactura(serie string, sequencial int) (NumeroFactura, error)`; `ParseNumeroFactura(s string) (string, int, error)`; `(NumeroFactura) String() string`; `SerieDe(momento time.Time) string`.

- [ ] **Step 1: Escrever o teste que falha**

```go
package financeiro_test

import (
	"testing"
	"time"

	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
)

func TestNumeroFactura_FormatoLegal(t *testing.T) {
	n, err := fin.NovoNumeroFactura("2026", 12345)
	if err != nil {
		t.Fatalf("NovoNumeroFactura: %v", err)
	}
	if got, quer := n.String(), "FAC 2026/00012345"; got != quer {
		t.Errorf("número = %q, queria %q", got, quer)
	}
}

func TestNumeroFactura_IdaEVolta(t *testing.T) {
	n, _ := fin.NovoNumeroFactura("2026", 7)
	serie, seq, err := fin.ParseNumeroFactura(n.String())
	if err != nil {
		t.Fatalf("ParseNumeroFactura: %v", err)
	}
	if serie != "2026" || seq != 7 {
		t.Errorf("parse = (%q,%d), queria (\"2026\",7)", serie, seq)
	}
}

func TestNumeroFactura_Invalidos(t *testing.T) {
	if _, err := fin.NovoNumeroFactura("", 1); err == nil {
		t.Error("série vazia devia falhar")
	}
	if _, err := fin.NovoNumeroFactura("2026", 0); err == nil {
		t.Error("sequencial zero devia falhar")
	}
	for _, s := range []string{"", "FAC2026/00000001", "REC 2026/00000001", "FAC 2026/abc"} {
		if _, _, err := fin.ParseNumeroFactura(s); err == nil {
			t.Errorf("ParseNumeroFactura(%q) devia falhar", s)
		}
	}
}

func TestSerieDe_EhOAno(t *testing.T) {
	m := time.Date(2026, 7, 18, 23, 30, 0, 0, time.FixedZone("WAT", 1*60*60))
	if got := fin.SerieDe(m); got != "2026" {
		t.Errorf("SerieDe = %q, queria \"2026\" (ano em UTC)", got)
	}
}
```

- [ ] **Step 2: Correr o teste e confirmar que falha**

Run: `go test ./internal/domain/financeiro/ -run 'NumeroFactura|SerieDe' -v`
Expected: FAIL — `undefined: fin.NovoNumeroFactura`

- [ ] **Step 3: Implementar**

```go
package financeiro

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// NumeroFactura é o número legal AGT de uma factura: "FAC 2026/00012345"
// (DDM-001 v2.0 §5.2.1). Prefixo fixo, série, barra, sequencial a 8 dígitos.
type NumeroFactura string

const prefixoNumero = "FAC"

// NovoNumeroFactura compõe o número legal a partir da série e do sequencial.
func NovoNumeroFactura(serie string, sequencial int) (NumeroFactura, error) {
	serie = strings.TrimSpace(serie)
	if serie == "" {
		return "", erros.Novo(erros.CategoriaValidacao, "série da factura em falta")
	}
	if sequencial <= 0 {
		return "", erros.Novo(erros.CategoriaValidacao, "sequencial da factura tem de ser positivo")
	}
	return NumeroFactura(fmt.Sprintf("%s %s/%08d", prefixoNumero, serie, sequencial)), nil
}

// String devolve a representação legal do número.
func (n NumeroFactura) String() string { return string(n) }

// ParseNumeroFactura decompõe um número legal na série e no sequencial.
func ParseNumeroFactura(s string) (string, int, error) {
	invalido := erros.Novo(erros.CategoriaValidacao, "número de factura inválido")
	partes := strings.SplitN(strings.TrimSpace(s), " ", 2)
	if len(partes) != 2 || partes[0] != prefixoNumero {
		return "", 0, invalido
	}
	corpo := strings.SplitN(partes[1], "/", 2)
	if len(corpo) != 2 || corpo[0] == "" {
		return "", 0, invalido
	}
	seq, err := strconv.Atoi(corpo[1])
	if err != nil || seq <= 0 {
		return "", 0, invalido
	}
	return corpo[0], seq, nil
}

// SerieDe devolve a série a que um instante pertence. A série é o ano civil em
// UTC — a mesma normalização usada no hash, para que a factura emitida à
// meia-noite não caia em séries diferentes conforme o fuso do servidor.
func SerieDe(momento time.Time) string {
	return strconv.Itoa(momento.UTC().Year())
}
```

- [ ] **Step 4: Correr o teste e confirmar que passa**

Run: `go test ./internal/domain/financeiro/ -run 'NumeroFactura|SerieDe' -v`
Expected: PASS (4 testes)

- [ ] **Step 5: Commit**

```bash
git add internal/domain/financeiro/numero.go internal/domain/financeiro/numero_test.go
git commit -m "feat(financeiro): VO NumeroFactura e série por ano (ADR-040)"
```

---

## Task 2: Emissão no agregado, com hash como invariante

**Files:**
- Modify: `internal/domain/financeiro/factura.go`
- Test: `internal/domain/financeiro/factura_test.go`

**Interfaces:**
- Consumes: `NovoNumeroFactura`, `SerieDe` (Task 1).
- Produces: `(*Factura).Emitir(serie string, sequencial int, hashAnterior string, momento time.Time) error`; acessores `Numero() NumeroFactura`, `Serie() string`, `Sequencial() int`, `DataEmissao() time.Time`, `Hash() string`, `HashAnterior() string`, `Versao() int`; `TotaisDe(itens []ItemFactura) Totais`; `HashDe(s SnapshotFactura) string`; campos novos em `SnapshotFactura`.

- [ ] **Step 1: Escrever os testes que falham**

Acrescentar a `internal/domain/financeiro/factura_test.go`:

```go
func TestEmitir_FixaNumeroDataEHash(t *testing.T) {
	f := novaFacturaValida(t)
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1,
		moeda.DeCentimos(50000), fin.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem: %v", err)
	}
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	if err := f.Emitir("2026", 1, "", m); err != nil {
		t.Fatalf("Emitir: %v", err)
	}
	if f.Estado() != fin.FactEmitida {
		t.Errorf("estado = %q, queria EMITIDA", f.Estado())
	}
	if got := f.Numero().String(); got != "FAC 2026/00000001" {
		t.Errorf("número = %q", got)
	}
	if f.Hash() == "" {
		t.Error("hash não podia ficar vazio")
	}
	if len(f.Hash()) != 64 {
		t.Errorf("hash tem %d caracteres, queria 64 (SHA-256 hex)", len(f.Hash()))
	}
}

func TestEmitir_SoDeRascunho(t *testing.T) {
	f := novaFacturaValida(t)
	_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeCentimos(1000), fin.RegimeIsento)
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	if err := f.Emitir("2026", 1, "", m); err != nil {
		t.Fatalf("primeira emissão: %v", err)
	}
	err := f.Emitir("2026", 2, f.Hash(), m)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Errorf("segunda emissão devia dar Conflito, deu %v", err)
	}
}

func TestEmitir_RecusaFacturaSemLinhas(t *testing.T) {
	f := novaFacturaValida(t)
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	err := f.Emitir("2026", 1, "", m)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Errorf("factura sem linhas devia dar RegraNegocio, deu %v", err)
	}
}

func TestHash_DeterministicoEEstavel(t *testing.T) {
	cria := func() *fin.Factura {
		f := novaFacturaValida(t)
		_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeCentimos(50000), fin.RegimeIsento)
		return f
	}
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	a, b := cria(), cria()
	_ = a.Emitir("2026", 1, "", m)
	_ = b.Emitir("2026", 1, "", m)
	if a.Hash() != b.Hash() {
		t.Error("mesma entrada devia dar o mesmo hash")
	}
}

func TestHash_IgnoraSubSegundo(t *testing.T) {
	cria := func() *fin.Factura {
		f := novaFacturaValida(t)
		_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeCentimos(50000), fin.RegimeIsento)
		return f
	}
	base := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	a, b := cria(), cria()
	_ = a.Emitir("2026", 1, "", base)
	_ = b.Emitir("2026", 1, "", base.Add(999*time.Millisecond))
	if a.Hash() != b.Hash() {
		t.Error("sub-segundo não podia entrar no hash: o valor relido da BD tem outra precisão")
	}
}

func TestHash_SelaConteudoDaLinha(t *testing.T) {
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	comDescricao := func(d string) string {
		f := novaFacturaValida(t)
		_ = f.AdicionarItem(d, fin.LinhaConsulta, "", 1, moeda.DeCentimos(50000), fin.RegimeIsento)
		_ = f.Emitir("2026", 1, "", m)
		return f.Hash()
	}
	if comDescricao("Consulta") == comDescricao("Cirurgia") {
		t.Error("alterar a descrição da linha tinha de mudar o hash (total igual não chega)")
	}
}

func TestHash_EncadeiaComAnterior(t *testing.T) {
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	comAnterior := func(ha string) string {
		f := novaFacturaValida(t)
		_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeCentimos(50000), fin.RegimeIsento)
		_ = f.Emitir("2026", 2, ha, m)
		return f.Hash()
	}
	if comAnterior("") == comAnterior("aaaa") {
		t.Error("o hash anterior tinha de entrar no cálculo")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/financeiro/ -run 'Emitir|Hash' -v`
Expected: FAIL — `f.Emitir undefined`

- [ ] **Step 3: Implementar em `factura.go`**

Acrescentar aos imports: `"crypto/sha256"`, `"encoding/hex"`, `"strconv"`.

Acrescentar os campos à struct `Factura` (depois de `actualizadoEm`):

```go
	numero        NumeroFactura
	serie         string
	sequencial    int
	dataEmissao   time.Time
	hash          string
	hashAnterior  string
	versao        int
```

Extrair o cálculo de totais para uma função reutilizável e reescrever o método:

```go
// TotaisDe soma, por linha, subtotais e IVA. Partilhada pelo agregado e pela
// verificação da cadeia, que trabalha sobre snapshots.
func TotaisDe(itens []ItemFactura) Totais {
	sub := moeda.DeCentimos(0)
	iva := moeda.DeCentimos(0)
	for _, it := range itens {
		sub = sub.Somar(it.Subtotal())
		iva = iva.Somar(it.ValorIVA())
	}
	return Totais{Subtotal: sub, TotalIVA: iva, Total: sub.Somar(iva)}
}

// Totais calcula os totais da factura.
func (f *Factura) Totais() Totais { return TotaisDe(f.itens) }
```

Acrescentar a emissão, o hash e os acessores:

```go
// Emitir transita a factura de RASCUNHO para EMITIDA, fixando número, data e o
// seu elo na cadeia de integridade. O hash é calculado aqui — é invariante do
// agregado, nunca de um serviço (antipadrão M4).
func (f *Factura) Emitir(serie string, sequencial int, hashAnterior string, momento time.Time) error {
	if f.estado != FactRascunho {
		return erros.Novo(erros.CategoriaConflito, "só é possível emitir uma factura em rascunho")
	}
	if len(f.itens) == 0 {
		return erros.Novo(erros.CategoriaRegraNegocio, "não é possível emitir uma factura sem linhas")
	}
	numero, err := NovoNumeroFactura(serie, sequencial)
	if err != nil {
		return err
	}
	f.numero = numero
	f.serie = strings.TrimSpace(serie)
	f.sequencial = sequencial
	f.dataEmissao = momento.UTC().Truncate(time.Second)
	f.hashAnterior = hashAnterior
	f.estado = FactEmitida
	f.hash = HashDe(f.Snapshot())
	return nil
}

// digestLinhas resume as linhas por ordem, selando descrição, tipo, quantidade,
// preço e regime — não só o total. Sem isto, trocar "Consulta" por "Cirurgia"
// mantendo o valor passaria despercebido.
func digestLinhas(itens []ItemFactura) string {
	h := sha256.New()
	for ordem, it := range itens {
		fmt.Fprintf(h, "%d|%s|%s|%d|%d|%s\n", ordem, it.Descricao, it.Tipo,
			it.Quantidade, it.PrecoUnitario.Centimos(), it.RegimeIVA)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// HashDe calcula o SHA-256 canónico de uma factura a partir do seu snapshot.
// O formato está documentado na ADR-040 e é fixo: não deriva do esquema da BD,
// para continuar reproduzível ao longo dos 10 anos de retenção legal.
//
//	serie|sequencial|dataEmissaoRFC3339UTC|clienteNIF|subtotal|iva|total|digestLinhas|hashAnterior
func HashDe(s SnapshotFactura) string {
	t := TotaisDe(s.Itens)
	canonico := strings.Join([]string{
		s.Serie,
		strconv.Itoa(s.Sequencial),
		s.DataEmissao.UTC().Truncate(time.Second).Format(time.RFC3339),
		s.Cliente.NIF,
		strconv.FormatInt(t.Subtotal.Centimos(), 10),
		strconv.FormatInt(t.TotalIVA.Centimos(), 10),
		strconv.FormatInt(t.Total.Centimos(), 10),
		digestLinhas(s.Itens),
		s.HashAnterior,
	}, "|")
	soma := sha256.Sum256([]byte(canonico))
	return hex.EncodeToString(soma[:])
}

// Numero devolve o número legal (vazio enquanto RASCUNHO).
func (f *Factura) Numero() NumeroFactura { return f.numero }

// Serie devolve a série da factura.
func (f *Factura) Serie() string { return f.serie }

// Sequencial devolve o sequencial dentro da série.
func (f *Factura) Sequencial() int { return f.sequencial }

// DataEmissao devolve o instante da emissão (zero enquanto RASCUNHO).
func (f *Factura) DataEmissao() time.Time { return f.dataEmissao }

// Hash devolve o elo desta factura na cadeia de integridade.
func (f *Factura) Hash() string { return f.hash }

// HashAnterior devolve o elo da factura imediatamente anterior na série.
func (f *Factura) HashAnterior() string { return f.hashAnterior }

// Versao devolve a versão para bloqueio optimista.
func (f *Factura) Versao() int { return f.versao }
```

Acrescentar os campos a `SnapshotFactura` e propagá-los em `Snapshot()` e `ReconstruirFactura`:

```go
type SnapshotFactura struct {
	ID            string
	Estado        EstadoFactura
	Cliente       ClienteSnapshot
	EpisodioID    string
	Itens         []ItemFactura
	CriadoEm      time.Time
	ActualizadoEm time.Time
	Numero        NumeroFactura
	Serie         string
	Sequencial    int
	DataEmissao   time.Time
	Hash          string
	HashAnterior  string
	Versao        int
}
```

Em `Snapshot()`, acrescentar ao literal devolvido:

```go
		Numero: f.numero, Serie: f.serie, Sequencial: f.sequencial,
		DataEmissao: f.dataEmissao, Hash: f.hash, HashAnterior: f.hashAnterior,
		Versao: f.versao,
```

Em `ReconstruirFactura`, acrescentar ao literal:

```go
		numero: s.Numero, serie: s.Serie, sequencial: s.Sequencial,
		dataEmissao: s.DataEmissao, hash: s.Hash, hashAnterior: s.HashAnterior,
		versao: s.Versao,
```

Actualizar o comentário de cabeçalho do pacote (linhas 2-4) para reflectir que a emissão passou a existir.

- [ ] **Step 4: Correr os testes**

Run: `go test ./internal/domain/financeiro/ -v`
Expected: PASS — todos, incluindo os da ADR-039 que já existiam

- [ ] **Step 5: Commit**

```bash
git add internal/domain/financeiro/factura.go internal/domain/financeiro/factura_test.go
git commit -m "feat(financeiro): emissão da Factura com hash canónico no agregado (ADR-040)"
```

---

## Task 3: Verificação da cadeia

**Files:**
- Create: `internal/domain/financeiro/cadeia.go`
- Test: `internal/domain/financeiro/cadeia_test.go`

**Interfaces:**
- Consumes: `HashDe`, `SnapshotFactura` (Task 2).
- Produces: `VerificarCadeia(facturas []SnapshotFactura) error`.

- [ ] **Step 1: Escrever o teste que falha**

```go
package financeiro_test

import (
	"strings"
	"testing"
	"time"

	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
)

// cadeiaValida devolve n snapshots correctamente encadeados na série 2026.
func cadeiaValida(t *testing.T, n int) []fin.SnapshotFactura {
	t.Helper()
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	out := make([]fin.SnapshotFactura, 0, n)
	anterior := ""
	for i := 1; i <= n; i++ {
		cliente, err := fin.NovoClienteSnapshot("Cliente", "", "")
		if err != nil {
			t.Fatalf("NovoClienteSnapshot: %v", err)
		}
		f, err := fin.NovaFactura(cliente, "6f1e7a8c-0b2d-4c3e-9f10-1a2b3c4d5e6f")
		if err != nil {
			t.Fatalf("NovaFactura: %v", err)
		}
		if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1,
			moeda.DeCentimos(int64(1000*i)), fin.RegimeIsento); err != nil {
			t.Fatalf("AdicionarItem: %v", err)
		}
		if err := f.Emitir("2026", i, anterior, m); err != nil {
			t.Fatalf("Emitir: %v", err)
		}
		anterior = f.Hash()
		out = append(out, f.Snapshot())
	}
	return out
}

func TestVerificarCadeia_Intacta(t *testing.T) {
	if err := fin.VerificarCadeia(cadeiaValida(t, 5)); err != nil {
		t.Errorf("cadeia válida devia verificar, deu: %v", err)
	}
}

func TestVerificarCadeia_VaziaEIntacta(t *testing.T) {
	if err := fin.VerificarCadeia(nil); err != nil {
		t.Errorf("cadeia vazia é trivialmente íntegra, deu: %v", err)
	}
}

func TestVerificarCadeia_DetectaHashAdulterado(t *testing.T) {
	c := cadeiaValida(t, 5)
	c[2].Itens[0].Descricao = "Adulterada"
	err := fin.VerificarCadeia(c)
	if err == nil {
		t.Fatal("adulteração de linha devia quebrar a cadeia")
	}
	if !strings.Contains(err.Error(), "00000003") {
		t.Errorf("erro devia apontar a 3.ª factura, deu: %v", err)
	}
}

func TestVerificarCadeia_DetectaEncadeamentoErrado(t *testing.T) {
	c := cadeiaValida(t, 5)
	c[3].HashAnterior = c[0].Hash
	c[3].Hash = fin.HashDe(c[3]) // recalculado: o hash bate, o elo é que não
	if err := fin.VerificarCadeia(c); err == nil {
		t.Fatal("elo apontado à factura errada devia quebrar a cadeia")
	}
}

func TestVerificarCadeia_DetectaBuraco(t *testing.T) {
	c := cadeiaValida(t, 5)
	semTerceira := append(append([]fin.SnapshotFactura{}, c[:2]...), c[3:]...)
	err := fin.VerificarCadeia(semTerceira)
	if err == nil || !strings.Contains(err.Error(), "buraco") {
		t.Errorf("buraco na série devia ser detectado, deu: %v", err)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/financeiro/ -run VerificarCadeia -v`
Expected: FAIL — `undefined: fin.VerificarCadeia`

- [ ] **Step 3: Implementar**

```go
package financeiro

import (
	"sort"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// VerificarCadeia confirma a integridade de uma série de facturas emitidas.
// Função pura: recebe os snapshots, ordena-os por sequencial e verifica três
// propriedades, devolvendo o PRIMEIRO problema encontrado —
//
//  1. a numeração é contígua desde 1 (sem buracos, REG-001 §3.2);
//  2. o hash de cada factura corresponde ao recálculo do seu conteúdo;
//  3. o hashAnterior de cada uma é o hash da que a precede.
//
// É esta função que torna a "quebra detectável" do REG-001 §3.2 verificável, e
// é sobre ela que assentará o cron diário do §3.4.
func VerificarCadeia(facturas []SnapshotFactura) error {
	if len(facturas) == 0 {
		return nil
	}
	ordenadas := make([]SnapshotFactura, len(facturas))
	copy(ordenadas, facturas)
	sort.Slice(ordenadas, func(i, j int) bool {
		return ordenadas[i].Sequencial < ordenadas[j].Sequencial
	})

	anterior := ""
	for i, f := range ordenadas {
		esperado := i + 1
		if f.Sequencial != esperado {
			return erros.Novo(erros.CategoriaRegraNegocio, fmt.Sprintf(
				"buraco na série %s: esperava o sequencial %08d e encontrou %08d",
				f.Serie, esperado, f.Sequencial))
		}
		if f.HashAnterior != anterior {
			return erros.Novo(erros.CategoriaRegraNegocio,
				"cadeia quebrada na factura "+f.Numero.String()+
					": o elo anterior não corresponde à factura que a precede")
		}
		if recalculado := HashDe(f); recalculado != f.Hash {
			return erros.Novo(erros.CategoriaRegraNegocio,
				"cadeia quebrada na factura "+f.Numero.String()+
					": o conteúdo não corresponde ao hash registado")
		}
		anterior = f.Hash
	}
	return nil
}
```

Imports de `cadeia.go`: `"fmt"`, `"sort"` e o pacote `erros`.

O `%08d` na mensagem do buraco não é cosmético — é o que faz
`TestVerificarCadeia_DetectaHashAdulterado` encontrar `"00000003"` e confirmar que
o erro aponta a factura certa, e não apenas que houve *algum* erro.

- [ ] **Step 4: Correr os testes**

Run: `go test ./internal/domain/financeiro/ -run VerificarCadeia -v`
Expected: PASS (5 testes)

- [ ] **Step 5: Verificar a cobertura do domínio**

Run: `go test ./internal/domain/financeiro/ -cover`
Expected: coverage ≥ 85.0%

- [ ] **Step 6: Commit**

```bash
git add internal/domain/financeiro/cadeia.go internal/domain/financeiro/cadeia_test.go
git commit -m "feat(financeiro): VerificarCadeia detecta adulteração, elo errado e buraco (ADR-040)"
```

---

## Task 4: Migração — colunas, série e trigger de imutabilidade

**Files:**
- Create: `migrations/financeiro/0002_emissao_facturas.sql`
- Test: `tests/integration/facturas_test.go` (acrescentar)

**Interfaces:**
- Consumes: schema de `0001_facturas.sql`.
- Produces: colunas `numero`, `serie`, `sequencial`, `data_emissao`, `hash`, `hash_anterior`, `versao` em `financeiro.facturas`; tabela `financeiro.series`; trigger `trg_facturas_imutaveis`.

- [ ] **Step 1: Escrever a migração**

```sql
-- Bounded Context: financeiro
-- Migration forward-only. Emissão da factura (ADR-040): numeração sequencial por
-- série, cadeia hash SHA-256 e imutabilidade.
--
-- A imutabilidade é defesa em profundidade: o domínio já recusa alterar uma
-- factura emitida, e o trigger abaixo garante que nem um UPDATE directo em SQL o
-- consegue. Espelha auditoria.impedir_mutacao (migrations/auditoria/0001).

ALTER TABLE financeiro.facturas
    ADD COLUMN IF NOT EXISTS numero        text,
    ADD COLUMN IF NOT EXISTS serie         text,
    ADD COLUMN IF NOT EXISTS sequencial    integer,
    ADD COLUMN IF NOT EXISTS data_emissao  timestamptz,
    ADD COLUMN IF NOT EXISTS hash          text,
    ADD COLUMN IF NOT EXISTS hash_anterior text,
    ADD COLUMN IF NOT EXISTS versao        integer NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS uq_facturas_numero
    ON financeiro.facturas (numero) WHERE numero IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_facturas_serie_sequencial
    ON financeiro.facturas (serie, sequencial) WHERE serie IS NOT NULL;

-- Coerência estado↔campos de emissão. hash_anterior é NOT NULL quando emitida mas
-- pode ser string vazia: é esse o valor na primeira factura de cada série.
ALTER TABLE financeiro.facturas
    DROP CONSTRAINT IF EXISTS facturas_coerencia_emissao;
ALTER TABLE financeiro.facturas
    ADD CONSTRAINT facturas_coerencia_emissao CHECK (
        (estado = 'RASCUNHO' AND numero IS NULL AND serie IS NULL
         AND sequencial IS NULL AND data_emissao IS NULL
         AND hash IS NULL AND hash_anterior IS NULL)
        OR
        (estado <> 'RASCUNHO' AND numero IS NOT NULL AND serie IS NOT NULL
         AND sequencial IS NOT NULL AND data_emissao IS NOT NULL
         AND hash IS NOT NULL AND hash_anterior IS NOT NULL)
    );

-- Cabeça de cada série: último sequencial atribuído e último elo da cadeia.
-- É a linha bloqueada com FOR UPDATE na emissão — o ponto de serialização.
CREATE TABLE IF NOT EXISTS financeiro.series (
    serie             text        PRIMARY KEY,
    ultimo_sequencial integer     NOT NULL DEFAULT 0 CHECK (ultimo_sequencial >= 0),
    ultimo_hash       text        NOT NULL DEFAULT '',
    actualizado_em    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE financeiro.series IS
    'Cabeça da numeração e da cadeia hash por série (AGT). Bloqueada com FOR UPDATE na emissão.';

-- Imutabilidade da factura emitida. A condição incide sobre OLD.estado: a própria
-- emissão parte de um RASCUNHO e passa; qualquer escrita sobre uma factura já
-- emitida é rejeitada, aconteça o que acontecer na aplicação.
CREATE OR REPLACE FUNCTION financeiro.impedir_mutacao_factura() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'factura emitida é imutável: operação % não permitida', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_facturas_imutaveis ON financeiro.facturas;
CREATE TRIGGER trg_facturas_imutaveis
    BEFORE UPDATE OR DELETE ON financeiro.facturas
    FOR EACH ROW WHEN (OLD.estado <> 'RASCUNHO')
    EXECUTE FUNCTION financeiro.impedir_mutacao_factura();

-- As linhas de uma factura emitida seguem a mesma regra.
DROP TRIGGER IF EXISTS trg_itens_factura_imutaveis ON financeiro.itens_factura;
CREATE TRIGGER trg_itens_factura_imutaveis
    BEFORE UPDATE OR DELETE ON financeiro.itens_factura
    FOR EACH ROW WHEN (
        (SELECT estado FROM financeiro.facturas WHERE id = OLD.factura_id) <> 'RASCUNHO'
    )
    EXECUTE FUNCTION financeiro.impedir_mutacao_factura();
```

- [ ] **Step 2: Escrever o teste de integração que prova o trigger**

Acrescentar a `tests/integration/facturas_test.go`:

```go
func TestFacturaEmitida_ImutavelNaBD(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)

	var id string
	err := pool.QueryRow(ctx, `
INSERT INTO financeiro.facturas
    (estado, cliente_nome, episodio_id, numero, serie, sequencial,
     data_emissao, hash, hash_anterior)
VALUES ('EMITIDA','Cliente',gen_random_uuid(),'FAC 2026/09999999','2026',9999999,
        now(),'abc','')
RETURNING id::text`).Scan(&id)
	if err != nil {
		t.Fatalf("inserir factura emitida: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id=$1 AND estado='RASCUNHO'`, id)
	})

	_, err = pool.Exec(ctx, `UPDATE financeiro.facturas SET cliente_nome='Outro' WHERE id=$1`, id)
	if err == nil {
		t.Fatal("UPDATE numa factura emitida tinha de falhar")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "2F004" {
		t.Errorf("esperava SQLSTATE 2F004 (restrict_violation), deu: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id=$1`, id); err == nil {
		t.Error("DELETE numa factura emitida tinha de falhar (retenção 10 anos)")
	}
}
```

Acrescentar aos imports do ficheiro: `"errors"` e `"github.com/jackc/pgx/v5/pgconn"`.

> Nota: a factura de teste é inserida directamente em SQL, com um sequencial fora do
> alcance da aplicação (9999999), porque o objectivo é exercitar o trigger e não o
> repositório. O `Cleanup` não a consegue apagar — é essa exactamente a propriedade
> em teste. Deixá-la na BD de testes é inofensivo e intencional.

- [ ] **Step 3: Correr o teste de integração**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run 'Imutavel' -count=1 -v`
Expected: PASS

- [ ] **Step 4: Confirmar que as migrações continuam idempotentes**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run 'Migracoes|Factura' -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add migrations/financeiro/0002_emissao_facturas.sql tests/integration/facturas_test.go
git commit -m "feat(financeiro): migração de emissão com série e trigger de imutabilidade (ADR-040)"
```

---

## Task 5: Bloqueio optimista no `Guardar`

**Files:**
- Modify: `internal/adapters/pgrepo/facturas_repo.go:44-56`
- Test: `tests/integration/facturas_test.go` (acrescentar)

**Interfaces:**
- Consumes: coluna `versao` (Task 4); `(*Factura).Versao()` (Task 2).
- Produces: `Guardar` passa a rejeitar escrita sobre versão desactualizada com `erros.CategoriaConflito`.

- [ ] **Step 1: Escrever o teste que falha**

```go
func TestGuardarFactura_BloqueioOptimista(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	cliente, err := fin.NovoClienteSnapshot("Cliente", "", "")
	if err != nil {
		t.Fatalf("NovoClienteSnapshot: %v", err)
	}
	f, err := fin.NovaFactura(cliente, uuid.NewString())
	if err != nil {
		t.Fatalf("NovaFactura: %v", err)
	}
	id, err := repo.Guardar(ctx, f)
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	limparFactura(t, pool, ctx, id)

	// Dois leitores carregam a MESMA versão.
	a, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("ObterPorID a: %v", err)
	}
	b, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("ObterPorID b: %v", err)
	}
	if err := a.AdicionarItem("Consulta A", fin.LinhaConsulta, "", 1,
		moeda.DeCentimos(1000), fin.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem a: %v", err)
	}
	if err := b.AdicionarItem("Consulta B", fin.LinhaConsulta, "", 1,
		moeda.DeCentimos(2000), fin.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem b: %v", err)
	}

	if _, err := repo.Guardar(ctx, a); err != nil {
		t.Fatalf("a primeira escrita devia passar: %v", err)
	}
	_, err = repo.Guardar(ctx, b)
	if err == nil {
		t.Fatal("a segunda escrita sobre versão velha tinha de dar conflito (lost update)")
	}
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Errorf("esperava Conflito, deu %v", err)
	}

	// A linha de 'a' sobreviveu — nada se perdeu.
	final, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("ObterPorID final: %v", err)
	}
	if len(final.Itens()) != 1 || final.Itens()[0].Descricao != "Consulta A" {
		t.Errorf("esperava só a linha de A, tem %d linhas", len(final.Itens()))
	}
}
```

Garantir que os imports do ficheiro incluem `"github.com/google/uuid"` e `"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"`.

- [ ] **Step 2: Correr e confirmar que falha**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run BloqueioOptimista -count=1 -v`
Expected: FAIL — a segunda escrita passa e a linha de A desaparece

- [ ] **Step 3: Implementar a guarda de versão**

Em `facturas_repo.go`, substituir o ramo `else` do `Guardar` (linhas 44-56):

```go
	} else {
		const qUpd = `
UPDATE financeiro.facturas
SET cliente_nome=$2, cliente_nif=NULLIF($3,''), cliente_morada=NULLIF($4,''),
    versao=versao+1, actualizado_em=now()
WHERE id=$1 AND estado='RASCUNHO' AND versao=$5`
		ct, err := tx.Exec(ctx, qUpd, id, s.Cliente.Nome, s.Cliente.NIF, s.Cliente.Morada, s.Versao)
		if err != nil {
			return "", fmt.Errorf("actualizar factura: %w", err)
		}
		if ct.RowsAffected() != 1 {
			// Distingue-se pela leitura do estado actual: se continua em rascunho, o
			// que falhou foi a versão (outra escrita passou à frente).
			var estado string
			if e := tx.QueryRow(ctx, `SELECT estado FROM financeiro.facturas WHERE id=$1`, id).Scan(&estado); e == nil && estado == "RASCUNHO" {
				return "", erros.Novo(erros.CategoriaConflito,
					"a factura foi alterada entretanto — recarregue e tente de novo")
			}
			return "", erros.Novo(erros.CategoriaConflito, "a factura já não está em rascunho ou não existe")
		}
	}
```

Garantir que `ObterPorID` selecciona e preenche `versao` no snapshot (acrescentar `versao` à lista de colunas do `SELECT` e ao `Scan`, mapeando para `SnapshotFactura.Versao`).

- [ ] **Step 4: Correr o teste**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run BloqueioOptimista -count=1 -v`
Expected: PASS

- [ ] **Step 5: Correr toda a suite de integração do financeiro**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run 'Factura' -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/pgrepo/facturas_repo.go tests/integration/facturas_test.go
git commit -m "fix(financeiro): bloqueio optimista fecha o lost-update do rascunho (ADR-040)"
```

---

## Task 6: `Emitir` no repositório — alocação serializada

**Files:**
- Modify: `internal/adapters/pgrepo/facturas_repo.go`, `internal/domain/financeiro/factura.go` (interface do repositório)
- Test: `tests/integration/facturas_test.go` (acrescentar)

**Interfaces:**
- Consumes: `financeiro.series` (Task 4); `(*Factura).Emitir` (Task 2); `SerieDe` (Task 1).
- Produces: `RepositorioFacturas.Emitir(ctx context.Context, facturaID string, momento time.Time) (*Factura, error)`; `RepositorioFacturas.ListarSnapshotsPorSerie(ctx context.Context, serie string) ([]SnapshotFactura, error)`.

- [ ] **Step 1: Estender a porta no domínio**

Em `factura.go`, substituir a interface:

```go
// RepositorioFacturas é a porta de saída de persistência de facturas.
//
// Guardar é um upsert transaccional guardado por estado e versão (bloqueio
// optimista). Emitir aloca sequencial e elo da cadeia sob serialização e transita
// a factura para EMITIDA numa única transacção.
type RepositorioFacturas interface {
	Guardar(ctx context.Context, f *Factura) (string, error)
	ObterPorID(ctx context.Context, id string) (*Factura, error)
	ListarPorEpisodio(ctx context.Context, episodioID string) ([]ResumoFactura, error)
	Emitir(ctx context.Context, facturaID string, momento time.Time) (*Factura, error)
	ListarSnapshotsPorSerie(ctx context.Context, serie string) ([]SnapshotFactura, error)
}
```

- [ ] **Step 2: Escrever o teste de concorrência que falha**

```go
func TestEmitirFacturas_NumeracaoSemBuracosSobConcorrencia(t *testing.T) {
	pool, ctx := ligar(t)
	migrarFinanceiro(t, pool, ctx)
	repo := pgrepo.NovoRepositorioFacturas(pool)

	// Série própria deste teste, para não colidir com outros.
	const serie = "2999"
	momento := time.Date(2999, 1, 15, 9, 0, 0, 0, time.UTC)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM financeiro.series WHERE serie=$1`, serie)
	})

	const n = 12
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		cliente, err := fin.NovoClienteSnapshot("Cliente", "", "")
		if err != nil {
			t.Fatalf("NovoClienteSnapshot: %v", err)
		}
		f, err := fin.NovaFactura(cliente, uuid.NewString())
		if err != nil {
			t.Fatalf("NovaFactura: %v", err)
		}
		if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1,
			moeda.DeCentimos(int64(1000+i)), fin.RegimeIsento); err != nil {
			t.Fatalf("AdicionarItem: %v", err)
		}
		id, err := repo.Guardar(ctx, f)
		if err != nil {
			t.Fatalf("Guardar: %v", err)
		}
		ids = append(ids, id)
	}

	// Emitir todas em simultâneo.
	var wg sync.WaitGroup
	erradas := make([]error, n)
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			_, erradas[i] = repo.Emitir(ctx, id, momento)
		}(i, id)
	}
	wg.Wait()
	for i, err := range erradas {
		if err != nil {
			t.Fatalf("emissão %d falhou: %v", i, err)
		}
	}

	snaps, err := repo.ListarSnapshotsPorSerie(ctx, serie)
	if err != nil {
		t.Fatalf("ListarSnapshotsPorSerie: %v", err)
	}
	if len(snaps) != n {
		t.Fatalf("esperava %d facturas na série, tem %d", n, len(snaps))
	}
	vistos := map[int]bool{}
	for _, s := range snaps {
		if vistos[s.Sequencial] {
			t.Errorf("sequencial %d repetido", s.Sequencial)
		}
		vistos[s.Sequencial] = true
	}
	for i := 1; i <= n; i++ {
		if !vistos[i] {
			t.Errorf("buraco na série: falta o sequencial %d", i)
		}
	}
	if err := fin.VerificarCadeia(snaps); err != nil {
		t.Errorf("cadeia devia estar íntegra após emissões concorrentes: %v", err)
	}
}
```

Acrescentar `"sync"` e `"time"` aos imports do ficheiro de teste.

- [ ] **Step 3: Correr e confirmar que falha**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run NumeracaoSemBuracos -count=1 -v`
Expected: FAIL — `repo.Emitir undefined`

- [ ] **Step 4: Implementar `Emitir` e `ListarSnapshotsPorSerie`**

Acrescentar a `facturas_repo.go`:

```go
// Emitir aloca o sequencial e o elo da cadeia sob serialização e transita a
// factura para EMITIDA, tudo numa transacção.
//
// O ponto de serialização é o SELECT ... FOR UPDATE sobre a linha da série: duas
// emissões simultâneas na mesma série põem-se em fila. Se a transacção reverter,
// o contador não avança — a ausência de buracos é propriedade da estrutura, não
// uma verificação a posteriori.
func (r *RepositorioFacturas) Emitir(ctx context.Context, facturaID string, momento time.Time) (*fin.Factura, error) {
	serie := fin.SerieDe(momento)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("iniciar transacção de emissão: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Criar a série antes de a bloquear resolve a viragem de ano: na primeira
	// emissão de uma série nova a linha ainda não existe, e criá-la fora do lock
	// seria ela própria uma corrida.
	if _, err := tx.Exec(ctx,
		`INSERT INTO financeiro.series (serie) VALUES ($1) ON CONFLICT DO NOTHING`, serie); err != nil {
		return nil, fmt.Errorf("garantir a série: %w", err)
	}

	var ultimoSeq int
	var ultimoHash string
	if err := tx.QueryRow(ctx, `
SELECT ultimo_sequencial, ultimo_hash FROM financeiro.series
 WHERE serie=$1 FOR UPDATE`, serie).Scan(&ultimoSeq, &ultimoHash); err != nil {
		return nil, fmt.Errorf("bloquear a série: %w", err)
	}

	f, err := r.obterComTx(ctx, tx, facturaID)
	if err != nil {
		return nil, err
	}
	if err := f.Emitir(serie, ultimoSeq+1, ultimoHash, momento); err != nil {
		return nil, err
	}
	s := f.Snapshot()

	ct, err := tx.Exec(ctx, `
UPDATE financeiro.facturas
   SET estado='EMITIDA', numero=$2, serie=$3, sequencial=$4, data_emissao=$5,
       hash=$6, hash_anterior=$7, versao=versao+1, actualizado_em=now()
 WHERE id=$1 AND estado='RASCUNHO' AND versao=$8`,
		facturaID, s.Numero.String(), s.Serie, s.Sequencial, s.DataEmissao,
		s.Hash, s.HashAnterior, s.Versao)
	if err != nil {
		return nil, fmt.Errorf("emitir factura: %w", err)
	}
	if ct.RowsAffected() != 1 {
		return nil, erros.Novo(erros.CategoriaConflito,
			"a factura já não está em rascunho ou foi alterada entretanto")
	}

	if _, err := tx.Exec(ctx, `
UPDATE financeiro.series SET ultimo_sequencial=$2, ultimo_hash=$3, actualizado_em=now()
 WHERE serie=$1`, serie, s.Sequencial, s.Hash); err != nil {
		return nil, fmt.Errorf("avançar a série: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("confirmar a emissão: %w", err)
	}
	return f, nil
}

// ListarSnapshotsPorSerie devolve os snapshots das facturas emitidas de uma
// série, ordenados por sequencial — a entrada de VerificarCadeia.
func (r *RepositorioFacturas) ListarSnapshotsPorSerie(ctx context.Context, serie string) ([]fin.SnapshotFactura, error) {
	linhas, err := r.pool.Query(ctx, `
SELECT id::text, estado, cliente_nome, COALESCE(cliente_nif,''), COALESCE(cliente_morada,''),
       episodio_id::text, criado_em, actualizado_em, COALESCE(numero,''), COALESCE(serie,''),
       COALESCE(sequencial,0), COALESCE(data_emissao, to_timestamp(0)),
       COALESCE(hash,''), COALESCE(hash_anterior,''), versao
  FROM financeiro.facturas
 WHERE serie=$1 AND estado <> 'RASCUNHO'
 ORDER BY sequencial`, serie)
	if err != nil {
		return nil, fmt.Errorf("listar facturas da série: %w", err)
	}
	defer linhas.Close()

	var out []fin.SnapshotFactura
	for linhas.Next() {
		var s fin.SnapshotFactura
		var numero string
		if err := linhas.Scan(&s.ID, &s.Estado, &s.Cliente.Nome, &s.Cliente.NIF, &s.Cliente.Morada,
			&s.EpisodioID, &s.CriadoEm, &s.ActualizadoEm, &numero, &s.Serie,
			&s.Sequencial, &s.DataEmissao, &s.Hash, &s.HashAnterior, &s.Versao); err != nil {
			return nil, fmt.Errorf("ler factura da série: %w", err)
		}
		s.Numero = fin.NumeroFactura(numero)
		itens, err := r.itensDe(ctx, s.ID)
		if err != nil {
			return nil, err
		}
		s.Itens = itens
		out = append(out, s)
	}
	return out, linhas.Err()
}
```

Extrair dois helpers a partir do `ObterPorID` existente, para não duplicar SQL:

- `obterComTx(ctx, tx pgx.Tx, id string) (*fin.Factura, error)` — o mesmo `SELECT` do `ObterPorID` mas sobre a transacção, incluindo já as colunas de emissão e `versao`.
- `itensDe(ctx, facturaID string) ([]fin.ItemFactura, error)` — a leitura das linhas por `ordem`, hoje embutida no `ObterPorID`.

`ObterPorID` passa a usar `itensDe`.

- [ ] **Step 5: Correr o teste de concorrência**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run NumeracaoSemBuracos -count=1 -v`
Expected: PASS — 12 sequenciais distintos de 1 a 12, cadeia íntegra

- [ ] **Step 6: Correr com detector de corridas**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration -race ./tests/integration/ -run NumeracaoSemBuracos -count=1`
Expected: PASS, sem `DATA RACE`

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/pgrepo/facturas_repo.go internal/domain/financeiro/factura.go tests/integration/facturas_test.go
git commit -m "feat(financeiro): emissão serializada por série, sem buracos (ADR-040)"
```

---

## Task 7: Casos de uso

**Files:**
- Modify: `internal/application/financeiro/facturas.go`, `internal/application/financeiro/ports.go`, `internal/application/financeiro/mapa.go`, `internal/application/financeiro/fakes_test.go`
- Test: `internal/application/financeiro/facturas_test.go`

**Interfaces:**
- Consumes: `RepositorioFacturas.Emitir`, `.ListarSnapshotsPorSerie` (Task 6); `VerificarCadeia` (Task 3).
- Produces: `NovoCasoEmitirFactura(f dominio.RepositorioFacturas, aud Auditor) *CasoEmitirFactura` com `Executar(ctx, actor, facturaID string) (DetalheFactura, error)`; `NovoCasoVerificarCadeia(f dominio.RepositorioFacturas) *CasoVerificarCadeia` com `Executar(ctx, serie string) (ResultadoVerificacao, error)`; tipo `ResultadoVerificacao`.

- [ ] **Step 1: Escrever os testes que falham**

O fake existente chama-se `fakeFacturas`, construído por `novoFakeFacturas()`, e o
auditor é `fakeAuditor` com o campo `registos` e o helper `tem(accao string) bool`
(ver `internal/application/financeiro/fakes_test.go`). Estender o fake com os dois
métodos novos da porta:

```go
// Emitir replica em memória a alocação serializada do repositório real: contador
// e último elo por série, com o cálculo do hash a acontecer no agregado.
func (f *fakeFacturas) Emitir(_ context.Context, id string, momento time.Time) (*dominio.Factura, error) {
	fa, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "factura não encontrada")
	}
	serie := dominio.SerieDe(momento)
	if f.ultimoSeq == nil {
		f.ultimoSeq = map[string]int{}
		f.ultimoHash = map[string]string{}
	}
	seq := f.ultimoSeq[serie] + 1
	if err := fa.Emitir(serie, seq, f.ultimoHash[serie], momento); err != nil {
		return nil, err
	}
	f.ultimoSeq[serie] = seq
	f.ultimoHash[serie] = fa.Hash()
	return fa, nil
}

// ListarSnapshotsPorSerie devolve os snapshots emitidos da série, por sequencial.
func (f *fakeFacturas) ListarSnapshotsPorSerie(_ context.Context, serie string) ([]dominio.SnapshotFactura, error) {
	var out []dominio.SnapshotFactura
	for _, fa := range f.porID {
		s := fa.Snapshot()
		if s.Serie == serie && s.Estado != dominio.FactRascunho {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequencial < out[j].Sequencial })
	return out, nil
}
```

Acrescentar os campos `ultimoSeq map[string]int` e `ultimoHash map[string]string` à
struct `fakeFacturas`, e `"sort"`/`"time"` aos imports. Ajustar os nomes de campo do
mapa interno (`porID`) ao que o fake já usa.

Helpers de sementeira, no mesmo ficheiro:

```go
// rascunhoComLinha semeia um rascunho com uma linha e devolve o seu id.
func rascunhoComLinha(t *testing.T, f *fakeFacturas) string {
	t.Helper()
	cliente, err := dominio.NovoClienteSnapshot("Cliente", "", "")
	if err != nil {
		t.Fatalf("NovoClienteSnapshot: %v", err)
	}
	fa, err := dominio.NovaFactura(cliente, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("NovaFactura: %v", err)
	}
	if err := fa.AdicionarItem("Consulta", dominio.LinhaConsulta, "", 1,
		moeda.DeCentimos(50000), dominio.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem: %v", err)
	}
	id, err := f.Guardar(context.Background(), fa)
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	return id
}

// rascunhoSemLinhas semeia um rascunho vazio e devolve o seu id.
func rascunhoSemLinhas(t *testing.T, f *fakeFacturas) string {
	t.Helper()
	cliente, _ := dominio.NovoClienteSnapshot("Cliente", "", "")
	fa, _ := dominio.NovaFactura(cliente, "11111111-1111-1111-1111-111111111111")
	id, err := f.Guardar(context.Background(), fa)
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	return id
}
```

Testes:

```go
func TestEmitirFactura_AuditaComNumeroEHash(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	id := rascunhoComLinha(t, repo)

	uc := financeiro.NovoCasoEmitirFactura(repo, aud)
	out, err := uc.Executar(context.Background(), "tesoureiro-1", id)
	if err != nil {
		t.Fatalf("Executar: %v", err)
	}
	if out.Estado != "EMITIDA" {
		t.Errorf("estado = %q, queria EMITIDA", out.Estado)
	}
	if out.Numero == "" || out.Hash == "" {
		t.Errorf("número e hash tinham de vir preenchidos: %+v", out)
	}
	if !aud.tem("financeiro.factura.emitida") {
		t.Error("a emissão tinha de ser auditada")
	}
}

func TestEmitirFactura_PropagaErroDoDominio(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	id := rascunhoSemLinhas(t, repo)

	uc := financeiro.NovoCasoEmitirFactura(repo, aud)
	_, err := uc.Executar(context.Background(), "tesoureiro-1", id)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Errorf("factura sem linhas devia dar RegraNegocio, deu %v", err)
	}
	if len(aud.registos) != 0 {
		t.Error("emissão falhada não podia ser auditada como sucesso")
	}
}

func TestVerificarCadeia_DevolveIntegra(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	emissor := financeiro.NovoCasoEmitirFactura(repo, aud)
	for i := 0; i < 3; i++ {
		if _, err := emissor.Executar(context.Background(), "tes-1", rascunhoComLinha(t, repo)); err != nil {
			t.Fatalf("emitir %d: %v", i, err)
		}
	}
	serie := dominio.SerieDe(time.Now())

	uc := financeiro.NovoCasoVerificarCadeia(repo)
	out, err := uc.Executar(context.Background(), serie)
	if err != nil {
		t.Fatalf("Executar: %v", err)
	}
	if !out.Integra || out.TotalFacturas != 3 {
		t.Errorf("esperava íntegra com 3 facturas, deu %+v", out)
	}
}

func TestVerificarCadeia_QuebraEhResultadoNaoErro(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	emissor := financeiro.NovoCasoEmitirFactura(repo, aud)
	for i := 0; i < 3; i++ {
		if _, err := emissor.Executar(context.Background(), "tes-1", rascunhoComLinha(t, repo)); err != nil {
			t.Fatalf("emitir %d: %v", i, err)
		}
	}
	serie := dominio.SerieDe(time.Now())
	repo.adulterarPrimeiraLinha(serie, 2, "Adulterada")

	uc := financeiro.NovoCasoVerificarCadeia(repo)
	out, err := uc.Executar(context.Background(), serie)
	if err != nil {
		t.Fatalf("uma cadeia quebrada é um resultado, não um erro de execução: %v", err)
	}
	if out.Integra || out.Detalhe == "" {
		t.Errorf("esperava quebra reportada com detalhe, deu %+v", out)
	}
}
```

O helper `adulterarPrimeiraLinha(serie string, sequencial int, descricao string)` no
fake localiza a factura pelo par série/sequencial e reescreve a descrição da
primeira linha **sem** recalcular o hash — simulando adulteração directa na BD.
Como o agregado não expõe mutação de linhas depois de emitido, o fake guarda o
snapshot e reconstrói via `dominio.ReconstruirFactura`.

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/financeiro/ -run 'EmitirFactura|VerificarCadeia' -v`
Expected: FAIL — `undefined: financeiro.NovoCasoEmitirFactura`

- [ ] **Step 3: Implementar os casos de uso**

Acrescentar a `facturas.go`:

```go
// CasoEmitirFactura transita uma factura de rascunho para emitida, fixando-a na
// cadeia de integridade. Acto irreversível com efeito fiscal — sempre auditado.
type CasoEmitirFactura struct {
	facturas dominio.RepositorioFacturas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoEmitirFactura constrói o caso de uso.
func NovoCasoEmitirFactura(f dominio.RepositorioFacturas, aud Auditor) *CasoEmitirFactura {
	return &CasoEmitirFactura{facturas: f, auditor: aud, agora: time.Now}
}

// Executar emite a factura e audita o acto com o número legal e o hash.
//
// A alocação do sequencial e do elo da cadeia acontece dentro do repositório, sob
// serialização; o cálculo do hash acontece dentro do agregado. Este caso de uso
// não conhece nenhum dos dois mecanismos.
func (uc *CasoEmitirFactura) Executar(ctx context.Context, actor, facturaID string) (DetalheFactura, error) {
	f, err := uc.facturas.Emitir(ctx, facturaID, uc.agora())
	if err != nil {
		return DetalheFactura{}, err
	}
	detalhe, err := json.Marshal(map[string]string{
		"numero": f.Numero().String(), "hash": f.Hash(),
	})
	if err != nil {
		return DetalheFactura{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "financeiro.factura.emitida",
		Entidade: "factura", EntidadeID: facturaID, OcorridoEm: uc.agora(),
		Detalhe: string(detalhe),
	}); err != nil {
		return DetalheFactura{}, err
	}
	return paraDetalheFactura(f), nil
}

// CasoVerificarCadeia verifica a integridade da cadeia hash de uma série.
type CasoVerificarCadeia struct {
	facturas dominio.RepositorioFacturas
}

// NovoCasoVerificarCadeia constrói o caso de uso.
func NovoCasoVerificarCadeia(f dominio.RepositorioFacturas) *CasoVerificarCadeia {
	return &CasoVerificarCadeia{facturas: f}
}

// Executar verifica a série. Uma cadeia quebrada é um RESULTADO (Integra=false),
// não um erro de execução: o endpoint tem de responder 200 com o diagnóstico,
// senão um auditor não distingue "cadeia partida" de "serviço em baixo".
func (uc *CasoVerificarCadeia) Executar(ctx context.Context, serie string) (ResultadoVerificacao, error) {
	snaps, err := uc.facturas.ListarSnapshotsPorSerie(ctx, serie)
	if err != nil {
		return ResultadoVerificacao{}, err
	}
	res := ResultadoVerificacao{Serie: serie, TotalFacturas: len(snaps), Integra: true}
	if err := dominio.VerificarCadeia(snaps); err != nil {
		res.Integra = false
		res.Detalhe = err.Error()
	}
	return res, nil
}
```

Acrescentar `"encoding/json"` aos imports de `facturas.go`. O campo
`auditoria.Registo.Detalhe` é uma `string` com JSON serializado (ver
`internal/domain/shared/auditoria/registo.go`), não um mapa — daí o `json.Marshal`.

Em `ports.go`, acrescentar os campos de emissão a `DetalheFactura` e o novo tipo:

```go
	Numero      string    `json:"numero,omitempty"`
	Serie       string    `json:"serie,omitempty"`
	Sequencial  int       `json:"sequencial,omitempty"`
	DataEmissao time.Time `json:"data_emissao,omitempty"`
	Hash        string    `json:"hash,omitempty"`

// ResultadoVerificacao é o diagnóstico da cadeia hash de uma série.
type ResultadoVerificacao struct {
	Serie         string `json:"serie"`
	TotalFacturas int    `json:"total_facturas"`
	Integra       bool   `json:"integra"`
	Detalhe       string `json:"detalhe,omitempty"`
}
```

Em `mapa.go`, preencher os campos novos em `paraDetalheFactura`.

- [ ] **Step 4: Correr os testes e a cobertura**

Run: `go test ./internal/application/financeiro/ -cover -v`
Expected: PASS, coverage ≥ 75.0%

- [ ] **Step 5: Commit**

```bash
git add internal/application/financeiro/
git commit -m "feat(financeiro): casos de uso de emissão e verificação da cadeia (ADR-040)"
```

---

## Task 8: HTTP e composition root

**Files:**
- Modify: `internal/adapters/http/financeiro_handler.go`, `internal/platform/app.go:207-215,282`
- Test: `internal/adapters/http/financeiro_test.go`

**Interfaces:**
- Consumes: `CasoEmitirFactura`, `CasoVerificarCadeia` (Task 7).
- Produces: rotas `POST /api/v1/financeiro/facturas/:fid/emitir` e `GET /api/v1/financeiro/facturas/cadeia/verificacao`.

- [ ] **Step 1: Escrever os testes que falham**

O helper existente é `routerFin(t, criar, adicionar, sessao)` e a sessão vem de
`sessaoLabDe(sujeito, papel)` — ver `financeiro_test.go:72-79`. Como
`NovoFinanceiroHandler` ganha dois parâmetros, **`routerFin` tem de ser actualizado**:

```go
// duploEmitirFactura devolve uma factura emitida; err força um erro de domínio.
type duploEmitirFactura struct{ err error }

func (d *duploEmitirFactura) Executar(_ context.Context, _, facturaID string) (appfinanceiro.DetalheFactura, error) {
	if d.err != nil {
		return appfinanceiro.DetalheFactura{}, d.err
	}
	return appfinanceiro.DetalheFactura{
		ID: facturaID, Estado: "EMITIDA",
		Numero: "FAC 2026/00000001", Serie: "2026", Sequencial: 1,
		Hash: "0000000000000000000000000000000000000000000000000000000000000000",
	}, nil
}

type duploVerificarCadeia struct{}

func (duploVerificarCadeia) Executar(_ context.Context, serie string) (appfinanceiro.ResultadoVerificacao, error) {
	return appfinanceiro.ResultadoVerificacao{Serie: serie, TotalFacturas: 3, Integra: true}, nil
}

// routerFin monta o router com os duplos e uma sessão fixa.
func routerFin(t *testing.T, criar *duploCriarFactura, adicionar *duploAdicionarItemFin,
	emitir *duploEmitirFactura, sessao identidade.Sessao) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if emitir == nil {
		emitir = &duploEmitirFactura{}
	}
	h := adhttp.NovoFinanceiroHandler(criar, adicionar, duploRemoverItemFin{},
		duploObterFactura{}, duploListarFacturas{}, emitir, duploVerificarCadeia{})
	adhttp.RegistarFinanceiro(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}
```

As chamadas existentes a `routerFin` passam a levar `nil` no parâmetro novo — por
exemplo `routerFin(t, criar, &duploAdicionarItemFin{}, nil, sessaoLabDe(...))`.

Testes novos:

```go
const facturaIDTeste = "22222222-2222-2222-2222-222222222222"

func TestFinanceiro_Emitir_Tesoureiro_200(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil,
		sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas/"+facturaIDTeste+"/emitir", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("esperava 200, veio %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "FAC 2026/00000001") {
		t.Errorf("resposta devia trazer o número legal: %s", w.Body.String())
	}
}

func TestFinanceiro_Emitir_Medico_Proibido(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil,
		sessaoLabDe("med-1", identidade.PapelMedico))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas/"+facturaIDTeste+"/emitir", nil)
	r.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Fatalf("esperava 403, veio %d", w.Code)
	}
}

func TestFinanceiro_Emitir_SemLinhas_422(t *testing.T) {
	emitir := &duploEmitirFactura{
		err: erros.Novo(erros.CategoriaRegraNegocio, "não é possível emitir uma factura sem linhas"),
	}
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, emitir,
		sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas/"+facturaIDTeste+"/emitir", nil)
	r.ServeHTTP(w, req)

	if w.Code != 422 {
		t.Fatalf("esperava 422, veio %d (%s)", w.Code, w.Body.String())
	}
}

func TestFinanceiro_Emitir_JaEmitida_409(t *testing.T) {
	emitir := &duploEmitirFactura{
		err: erros.Novo(erros.CategoriaConflito, "só é possível emitir uma factura em rascunho"),
	}
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, emitir,
		sessaoLabDe("tes-1", identidade.PapelTesoureiro))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/financeiro/facturas/"+facturaIDTeste+"/emitir", nil)
	r.ServeHTTP(w, req)

	if w.Code != 409 {
		t.Fatalf("esperava 409, veio %d", w.Code)
	}
}

func TestFinanceiro_VerificarCadeia_Auditor_200(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil,
		sessaoLabDe("aud-1", identidade.PapelAuditor))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/financeiro/facturas/cadeia/verificacao?serie=2026", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("esperava 200, veio %d (%s)", w.Code, w.Body.String())
	}
}

// A rota /cadeia/verificacao é registada antes de /:fid; este teste falha se
// alguém trocar a ordem e "cadeia" passar a ser capturado como id de factura.
func TestFinanceiro_CadeiaNaoEhCapturadaComoID(t *testing.T) {
	r := routerFin(t, &duploCriarFactura{}, &duploAdicionarItemFin{}, nil,
		sessaoLabDe("aud-1", identidade.PapelAuditor))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/financeiro/facturas/cadeia/verificacao", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("a rota da cadeia foi capturada por /:fid: veio %d", w.Code)
	}
}
```

Acrescentar `"strings"` aos imports do ficheiro de teste, se ainda não estiver.

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/http/ -run 'Financeiro_(Emitir|VerificarCadeia)' -v`
Expected: FAIL — rota 404

- [ ] **Step 3: Implementar**

Acrescentar às interfaces de serviço em `financeiro_handler.go`:

```go
	// ServicoEmitirFactura emite uma factura em rascunho.
	ServicoEmitirFactura interface {
		Executar(ctx context.Context, actor, facturaID string) (appfinanceiro.DetalheFactura, error)
	}
	// ServicoVerificarCadeia verifica a integridade da cadeia de uma série.
	ServicoVerificarCadeia interface {
		Executar(ctx context.Context, serie string) (appfinanceiro.ResultadoVerificacao, error)
	}
```

Acrescentar os campos ao `FinanceiroHandler` e aos parâmetros de `NovoFinanceiroHandler` (`emitir ServicoEmitirFactura, verificar ServicoVerificarCadeia`).

Registar as rotas — a de verificação **antes** da rota `/:fid`, para que `cadeia` não seja capturado como id:

```go
	g.GET("/cadeia/verificacao", leitura, h.verificarCadeiaHTTP)
	g.POST("/:fid/emitir", escrita, h.emitirFacturaHTTP)
```

Handlers:

```go
func (h *FinanceiroHandler) emitirFacturaHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.emitir.Executar(c.Request.Context(), actor.Sujeito, c.Param("fid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *FinanceiroHandler) verificarCadeiaHTTP(c *gin.Context) {
	serie := c.Query("serie")
	if serie == "" {
		serie = dominiofin.SerieDe(time.Now())
	}
	out, err := h.verificar.Executar(c.Request.Context(), serie)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
```

Acrescentar aos imports: `"time"` e `dominiofin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"`.

Em `app.go`, acrescentar os dois casos de uso à construção do handler (linhas 209-215):

```go
		appfinanceiro.NovoCasoEmitirFactura(repoFacturas, repoAuditoria),
		appfinanceiro.NovoCasoVerificarCadeia(repoFacturas),
```

- [ ] **Step 4: Correr os testes e o linter arquitectural**

Run: `go test ./internal/adapters/http/ -run Financeiro -v && go build ./... && go-arch-lint check`
Expected: PASS, build OK, sem violações de dependência

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/ internal/platform/app.go
git commit -m "feat(financeiro): rotas de emissão e verificação da cadeia (ADR-040)"
```

---

## Task 9: Tesoureiro passa a papel sensível

**Files:**
- Modify: `internal/domain/identidade/papel.go:39-46`, `internal/domain/identidade/identidade_test.go:149-156`, `seeds/papeis.sql:19`, `docker/keycloak/realm-sgc.json`, `docs/ERRATA-002-papel-tesoureiro.md`
- Test: `internal/domain/identidade/identidade_test.go`

**Interfaces:**
- Consumes: nada.
- Produces: `EhSensivel(PapelTesoureiro) == true`.

- [ ] **Step 1: Inverter o teste existente**

Substituir `TestPapelTesoureiroNaoSensivel` (linhas 149-156) por:

```go
func TestPapelTesoureiroSensivel(t *testing.T) {
	if !ident.PapelValido(string(ident.PapelTesoureiro)) {
		t.Fatal("PapelTesoureiro devia ser um papel válido")
	}
	// ADR-040: com a emissão, o Tesoureiro pratica um acto irreversível com
	// efeito fiscal. A ERRATA-002 marcou esta reavaliação como pendente.
	if !ident.EhSensivel(ident.PapelTesoureiro) {
		t.Error("PapelTesoureiro passou a sensível com a emissão de facturas (ADR-040)")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/identidade/ -run TesoureiroSensivel -v`
Expected: FAIL — "PapelTesoureiro passou a sensível…"

- [ ] **Step 3: Implementar**

Em `papel.go`, actualizar o mapa e o comentário:

```go
// papeisSensiveis exigem MFA. Alinhado com sensivel=true em seeds/papeis.sql:
// Director, Admin, DPO, Auditor e — desde o ADR-040 — Tesoureiro, que com a
// emissão passou a praticar um acto irreversível com efeito fiscal (ERRATA-002).
var papeisSensiveis = map[Papel]bool{
	PapelDirector:   true,
	PapelAdmin:      true,
	PapelDPO:        true,
	PapelAuditor:    true,
	PapelTesoureiro: true,
}
```

Em `seeds/papeis.sql`, linha 19, mudar `false` para `true`:

```sql
    ('Tesoureiro',         'Tesoureiro (facturação)',      true)
```

Em `docker/keycloak/realm-sgc.json`, alinhar a configuração do role `Tesoureiro` com a dos restantes papéis sensíveis (mesma marcação usada por `Director`/`Admin`/`DPO`/`Auditor`).

- [ ] **Step 4: Correr os testes**

Run: `go test ./internal/domain/identidade/ ./internal/adapters/http/ -v`
Expected: PASS. Se algum teste HTTP assumia que o Tesoureiro passava sem MFA, actualizá-lo — a mudança de comportamento é intencional.

- [ ] **Step 5: Acrescentar o bloco de revisão à ERRATA-002**

Acrescentar ao fim de `docs/ERRATA-002-papel-tesoureiro.md`, **sem alterar a linha `Decisão` original**:

```markdown
- **Revisão (2026-07-18, ADR-040)**: com a entrega da emissão, o Tesoureiro passa
  a praticar um acto irreversível com efeito fiscal. `Tesoureiro` passa a **papel
  sensível**, exigindo MFA, ao lado de Director, Admin, DPO e Auditor. Fecha-se a
  reavaliação prevista na Decisão original, que se mantém acima como registo do
  estado anterior — o intervalo em que o papel esteve não-sensível é ele próprio
  matéria de auditoria e não deve ser apagado.
- **Impacto da revisão**: `identidade.papeisSensiveis`, `TestPapelTesoureiroSensivel`,
  `seeds/papeis.sql`, `docker/keycloak/realm-sgc.json`, CLAUDE.md §6.
```

- [ ] **Step 6: Correr o teste de integração da contagem de papéis**

Run: `DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -run 'Seed|Papeis' -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/domain/identidade/ seeds/papeis.sql docker/keycloak/realm-sgc.json docs/ERRATA-002-papel-tesoureiro.md
git commit -m "feat(identidade): Tesoureiro passa a papel sensível com a emissão (ADR-040)"
```

---

## Task 10: ADR-040 e actualização de marco

**Files:**
- Create: `adrs/ADR-040-emissao-factura.md`
- Modify: `CLAUDE.md` (§6 e índice de ADRs), `SPRINT.md`

**Interfaces:**
- Consumes: tudo o que foi entregue nas Tasks 1-9.
- Produces: registo arquitectural.

- [ ] **Step 1: Escrever a ADR**

Seguir a estrutura de `adrs/ADR-039-bc-financeiro-factura.md`: Contexto, Decisão, Alternativas rejeitadas, Consequências, Riscos, Diferido. Incluir obrigatoriamente:

- O **formato canónico do hash**, verbatim, com as três regras de canonicalização. É a secção que a certificação AGT vai ler.
- As quatro alternativas rejeitadas com o porquê: assinatura digital diferida; `SEQUENCE` rejeitada por deixar buracos; advisory lock rejeitado por deixar a serialização invisível no esquema; JSON canónico rejeitado por tornar facturas antigas irreverificáveis.
- A resolução da nota provisória da ERRATA-002.
- Diferido: agendamento do cron diário (REG-001 §3.4); ADR-041 (anulação e pagamentos); ADR-042 (SAF-T-AO).

- [ ] **Step 2: Actualizar CLAUDE.md**

- §6 — reescrever o parágrafo do M4: a Sprint 15 entregou a emissão; mencionar a cadeia hash, a numeração por série e o Tesoureiro agora sensível (12 papéis, 5 sensíveis).
- Índice de ADRs no fim — acrescentar `adrs/ADR-040-emissao-factura.md` e mudar "Próximo ADR" para **ADR-041**.

- [ ] **Step 3: Actualizar SPRINT.md**

Acrescentar a secção `## Sprint 15 — entregue` no topo (acima da Sprint 14), com um `- [x]` por entregável, no formato das sprints anteriores. Actualizar o cabeçalho para Sprint 15.

- [ ] **Step 4: Verificação final completa**

```bash
gofmt -l . && go vet ./... && go build ./...
go test ./... -cover
go-arch-lint check
DATABASE_URL='postgres://sgc:sgc@localhost:5432/sgc?sslmode=disable' go test -tags=integration ./tests/integration/ -count=1
```

Expected: sem ficheiros por formatar; vet limpo; domínio ≥85%, aplicação ≥75%, adapters ≥60%; sem violações arquitecturais; integração toda verde.

- [ ] **Step 5: Commit**

```bash
git add adrs/ADR-040-emissao-factura.md CLAUDE.md SPRINT.md
git commit -m "docs(financeiro): ADR-040 e actualização de marco (ADR-040)"
```

---

## Verificação de cobertura do spec

| Requisito do spec | Task |
|---|---|
| VO `NumeroFactura` (§3.1) | 1 |
| Campos de emissão (§3.2) | 2 |
| `Emitir` com guardas e hash como invariante (§3.3) | 2 |
| Conteúdo canónico do hash e canonicalização (§3.4) | 2 |
| `VerificarCadeia` (§3.5) | 3 |
| Migração: colunas, UNIQUE, CHECK, `series`, trigger (§4.1) | 4 |
| Bloqueio optimista no `Guardar` (§4.2) | 5 |
| `Emitir` serializado com `ON CONFLICT` + `FOR UPDATE` (§4.3) | 6 |
| Casos de uso e auditoria (§5.1) | 7 |
| Rotas HTTP e códigos (§5.2) | 8 |
| Tesoureiro sensível (§6) | 9 |
| Alteração aditiva à ERRATA-002 (§6.1) | 9 |
| Teste de numeração sob concorrência (§7) | 6 |
| Teste do trigger de imutabilidade (§7) | 4 |
| Teste de lost-update (§7) | 5 |
| Gates de cobertura (§7) | 3, 7, 10 |
