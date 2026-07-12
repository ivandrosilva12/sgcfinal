# Sprint 7 — BC Clínico: agregado Doente — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar a primeira fatia vertical do BC Clínico — o agregado Doente (+ Alergia + AntecedenteClinico) — do domínio ao HTTP, com registo, pesquisa, actualização, alergias, antecedentes e ciclo de vida (desactivação/óbito).

**Architecture:** DDD táctico + Clean Architecture. Novo BC `clinico` em `internal/{domain,application,adapters}/clinico`, sobre o schema PostgreSQL `clinico`. Domínio rico e puro (IDs `string`, gerados pela BD via `gen_random_uuid()` + `RETURNING`), reutilizando o Shared Kernel (validadores Angola, erros, auditoria) e a infra transversal do M1 (Auth/RBAC/LimiteTaxa, RFC 7807, repositório de auditoria).

**Tech Stack:** Go 1.22+, Gin, pgx v5 (SQL puro), PostgreSQL 16 (pg_trgm), testes `go test` com fakes; integração com tag `integration`.

## Global Constraints

- **Linguagem ubíqua PT-PT angolano** em TODO o output: código, comentários, mensagens de erro, JSON, commits. Nunca inglês nem PT-BR.
- **Domínio puro:** `internal/domain/**` importa apenas stdlib + Shared Kernel. Zero `pgx`/`gin`/`net/http`/`google/uuid`. `google/uuid` também proibido na aplicação (só permitido em adapters pelo arch-lint).
- **Sem `panic()`** fora de inicialização.
- **Erros de domínio** via `erros.Novo(categoria, mensagem)` com mensagem PT-PT **literal** (o padrão do M1 — ver `internal/domain/identidade/utilizador.go` — usa literais no domínio, não `i18n.T`; seguir esse padrão). Categorias: `CategoriaValidacao`, `CategoriaNaoEncontrado`, `CategoriaConflito`.
- **Erros HTTP** via `responderErro(c, err)` (RFC 7807, `application/problem+json`, PT-PT) — já existe em `internal/adapters/http/problem.go`.
- **Modelo de dados** extraído verbatim do DDM-001 v2.0 (não inventado) — ver a migration na Task 5.
- **IDs** do domínio são `string`, gerados pela BD (`gen_random_uuid()` DEFAULT + `RETURNING id`). O domínio nunca gera IDs.
- **Auditoria:** toda a escrita e a consulta individual de doentes regista um `auditoria.Registo` via a porta `Auditor`. Acções: `clinico.doente.registado`, `clinico.doente.consultado`, `clinico.doente.actualizado`, `clinico.doente.desactivado`, `clinico.doente.falecido`, `clinico.alergia.registada`, `clinico.antecedente.registado`.
- **Nunca registar em log** dados de saúde nem identificadores (BI/NIF/telefone/nome).
- **Cobertura** (`bash scripts/cobertura.sh`, agregado por camada): domínio ≥85%, aplicação ≥75%, adaptadores ≥60%. O gate corre **sem** a tag `integration`.
- **Commits** Conventional Commits em PT-PT, a terminar com o trailer:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- **Branch de trabalho:** `m2-sprint7-clinico-doente` (já criado; a spec já lá está commitada).

### Convenções de código a seguir (do M1)

- Value Objects imutáveis com factory `NovoX(...) (X, error)` que valida e normaliza (ver `internal/domain/shared/identity/bi.go`).
- Enums como `type X string` + constantes + `ParseX(string) (X, error)` (novo neste sprint, mesmo espírito de `Papel`).
- Casos de uso: struct com dependências por construtor `NovoCaso...`, relógio injectado `agora func() time.Time` (default `time.Now`), método `Executar(ctx, ...)`. Ver `internal/application/identidade/sessoes.go` e `editar_perfil_admin.go`.
- Repositórios pgx: struct com `pool *pgxpool.Pool`, construtor `NovoRepositorioX(pool)`, SQL puro, `errors.Is(err, pgx.ErrNoRows)` → `CategoriaNaoEncontrado`. Ver `internal/adapters/pgrepo/identidade_repo.go`.
- Handlers: struct + interfaces de serviço + `RegistarX(r gin.IRouter, h, protecao ...gin.HandlerFunc)`, RBAC por rota via `RBAC(dominio.PapelX, ...)`, actor via `SessaoDe(c)`. Ver `internal/adapters/http/admin_handler.go`.
- Testes de domínio/aplicação em package externo `clinico_test` quando possível.

---

### Task 1: Validador de NIF angolano (Shared Kernel)

**Files:**
- Create: `internal/domain/shared/identity/nif.go`
- Test: `internal/domain/shared/identity/nif_test.go`

**Interfaces:**
- Consumes: nada (folha do Shared Kernel).
- Produces: `identity.NovoNIF(entrada string) (NIF, error)`; tipo `NIF` com métodos `String() string` e `Valido() bool`; erro sentinela `identity.ErrNIFInvalido`.

**Contexto:** O Shared Kernel `identity` já tem `NovoBI` e `NovoTelefone`. O NIF angolano tem 10 caracteres: pessoa singular = 9 dígitos + 1 letra final; pessoa colectiva = 10 dígitos. Seguir exactamente o estilo de `bi.go`.

- [ ] **Step 1: Escrever o teste que falha**

```go
package identity

import "testing"

func TestNovoNIF(t *testing.T) {
	casos := []struct {
		nome    string
		entrada string
		querErr bool
		querStr string
	}{
		{"colectiva 10 dígitos", "5417000001", false, "5417000001"},
		{"singular 9 dígitos + letra", "004567890A", false, "004567890A"},
		{"minúsculas normalizadas", "004567890a", false, "004567890A"},
		{"com espaços", " 5417 000 001 ", false, "5417000001"},
		{"curto demais", "12345", true, ""},
		{"longo demais", "12345678901", true, ""},
		{"letra no meio", "5417A00001", true, ""},
		{"vazio", "", true, ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			nif, err := NovoNIF(c.entrada)
			if c.querErr {
				if err == nil {
					t.Fatalf("esperava erro para %q", c.entrada)
				}
				return
			}
			if err != nil {
				t.Fatalf("inesperado: %v", err)
			}
			if nif.String() != c.querStr {
				t.Fatalf("String()=%q, esperava %q", nif.String(), c.querStr)
			}
			if !nif.Valido() {
				t.Fatal("esperava Valido()=true")
			}
		})
	}
}
```

- [ ] **Step 2: Correr o teste e confirmar que falha**

Run: `go test ./internal/domain/shared/identity/ -run TestNovoNIF -v`
Expected: FAIL — `undefined: NovoNIF`.

- [ ] **Step 3: Implementar `nif.go`**

```go
package identity

import (
	"errors"
	"regexp"
	"strings"
)

// ErrNIFInvalido é devolvido quando um Número de Identificação Fiscal não
// respeita o formato angolano.
var ErrNIFInvalido = errors.New("nif inválido")

// formatoNIF valida o NIF angolano de 10 caracteres: ou 10 dígitos (pessoa
// colectiva), ou 9 dígitos seguidos de 1 letra (pessoa singular).
// Exemplos: "5417000001", "004567890A".
var formatoNIF = regexp.MustCompile(`^([0-9]{10}|[0-9]{9}[A-Z])$`)

// NIF representa um Número de Identificação Fiscal angolano validado. Value
// Object imutável — a sua existência garante que o valor é bem-formado.
type NIF struct {
	valor string
}

// NovoNIF valida e constrói um NIF. A entrada é normalizada (espaços removidos e
// letras em maiúsculas) antes da validação. Devolve ErrNIFInvalido se o formato
// não corresponder a 10 dígitos ou 9 dígitos + 1 letra.
func NovoNIF(entrada string) (NIF, error) {
	normalizado := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(entrada), " ", ""))
	if !formatoNIF.MatchString(normalizado) {
		return NIF{}, ErrNIFInvalido
	}
	return NIF{valor: normalizado}, nil
}

// String devolve a representação canónica do NIF.
func (n NIF) String() string {
	return n.valor
}

// Valido indica se um NIF foi construído com sucesso (valor não vazio).
func (n NIF) Valido() bool {
	return n.valor != ""
}
```

- [ ] **Step 4: Correr o teste e confirmar que passa**

Run: `go test ./internal/domain/shared/identity/ -run TestNovoNIF -v`
Expected: PASS (todos os subtestes).

- [ ] **Step 5: Commit**

```bash
git add internal/domain/shared/identity/nif.go internal/domain/shared/identity/nif_test.go
git commit -m "feat(shared): validador de NIF angolano no Shared Kernel

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Value Objects de identificação e contactos

**Files:**
- Create: `internal/domain/clinico/identificacao.go`
- Create: `internal/domain/clinico/contactos.go`
- Test: `internal/domain/clinico/identificacao_test.go`
- Test: `internal/domain/clinico/contactos_test.go`

**Interfaces:**
- Consumes: `identity.NovoBI`, `identity.NovoNIF` (Task 1), `identity.NovoTelefone`; `erros.Novo`.
- Produces:
  - `type Sexo string`; consts `SexoMasculino="M"`, `SexoFeminino="F"`, `SexoOutro="O"`; `ParseSexo(string) (Sexo, error)`.
  - `type Identificacao struct { NomeCompleto string; DataNascimento time.Time; Sexo Sexo; BI, NIF, Passaporte *string }`.
  - `NovaIdentificacao(nome string, dataNasc time.Time, sexo Sexo, bi, nif, passaporte *string) (Identificacao, error)`.
  - `type Morada struct { Provincia, Municipio, Comuna, Bairro, Rua string; Casa, Referencia *string }`.
  - `type Contactos struct { Telefone string; Email *string; Morada *Morada }`.
  - `NovosContactos(telefone string, email *string, morada *Morada) (Contactos, error)`.
  - Helper `normalizarOpcional(*string) *string` (usado também nas Tasks 3-4).

**Invariantes (DDM-001):** nome não-vazio; data de nascimento não-futura (comparar com `time.Now()`); sexo ∈ {M,F,O}; **BI ou Passaporte obrigatório** (`doc_identificacao`); BI/NIF validados e normalizados se presentes; telefone obrigatório e normalizado; email validado se presente.

- [ ] **Step 1: Escrever os testes que falham**

`identificacao_test.go`:
```go
package clinico_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func ptr(s string) *string { return &s }

func TestNovaIdentificacao_ValidaComBI(t *testing.T) {
	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	id, err := clinico.NovaIdentificacao("Ana Domingos", nasc, clinico.SexoFeminino, ptr("00123456la042"), nil, nil)
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if id.BI == nil || *id.BI != "00123456LA042" {
		t.Fatalf("BI não normalizado: %v", id.BI)
	}
}

func TestNovaIdentificacao_PassaporteSemBI(t *testing.T) {
	nasc := time.Date(1985, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := clinico.NovaIdentificacao("João Paulo", nasc, clinico.SexoMasculino, nil, nil, ptr("N1234567")); err != nil {
		t.Fatalf("passaporte devia bastar: %v", err)
	}
}

func TestNovaIdentificacao_SemBINemPassaporte(t *testing.T) {
	nasc := time.Date(1985, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := clinico.NovaIdentificacao("João", nasc, clinico.SexoMasculino, nil, nil, nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestNovaIdentificacao_DataFutura(t *testing.T) {
	futuro := time.Now().AddDate(1, 0, 0)
	_, err := clinico.NovaIdentificacao("Ana", futuro, clinico.SexoFeminino, ptr("00123456LA042"), nil, nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestNovaIdentificacao_NIFInvalido(t *testing.T) {
	nasc := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := clinico.NovaIdentificacao("Ana", nasc, clinico.SexoFeminino, ptr("00123456LA042"), ptr("XYZ"), nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação para NIF inválido, obtive %v", err)
	}
}

func TestParseSexo(t *testing.T) {
	if s, err := clinico.ParseSexo("m"); err != nil || s != clinico.SexoMasculino {
		t.Fatalf("ParseSexo(m)=%v,%v", s, err)
	}
	if _, err := clinico.ParseSexo("X"); err == nil {
		t.Fatal("esperava erro para sexo inválido")
	}
}
```

`contactos_test.go`:
```go
package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovosContactos_NormalizaTelefone(t *testing.T) {
	ct, err := clinico.NovosContactos("+244923456789", nil, nil)
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if ct.Telefone != "+244 923 456 789" {
		t.Fatalf("telefone não normalizado: %q", ct.Telefone)
	}
}

func TestNovosContactos_TelefoneObrigatorio(t *testing.T) {
	_, err := clinico.NovosContactos("", nil, nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestNovosContactos_EmailInvalido(t *testing.T) {
	mau := "nao-e-email"
	_, err := clinico.NovosContactos("+244923456789", &mau, nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestNovosContactos_ComMorada(t *testing.T) {
	m := &clinico.Morada{Provincia: "Luanda", Municipio: "Belas", Comuna: "Benfica", Bairro: "Morro Bento", Rua: "Rua 1"}
	ct, err := clinico.NovosContactos("+244923456789", nil, m)
	if err != nil || ct.Morada == nil || ct.Morada.Provincia != "Luanda" {
		t.Fatalf("morada não preservada: %+v, %v", ct.Morada, err)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falham**

Run: `go test ./internal/domain/clinico/ -v`
Expected: FAIL — pacote `clinico` inexistente / símbolos indefinidos.

- [ ] **Step 3: Implementar `identificacao.go`**

```go
// Package clinico é o domínio do Bounded Context Clínico do SGC Angola. Contém o
// agregado Doente e os seus Value Objects e entidades-filho. Camada 1 (Domínio):
// importa apenas a biblioteca-padrão e o Shared Kernel — sem infra.
package clinico

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/identity"
)

// Sexo é o sexo biológico registado do doente (DDM-001: CHAR(1) M|F|O).
type Sexo string

const (
	SexoMasculino Sexo = "M"
	SexoFeminino  Sexo = "F"
	SexoOutro     Sexo = "O"
)

// ParseSexo valida e normaliza um código de sexo (aceita minúsculas).
func ParseSexo(codigo string) (Sexo, error) {
	switch Sexo(strings.ToUpper(strings.TrimSpace(codigo))) {
	case SexoMasculino:
		return SexoMasculino, nil
	case SexoFeminino:
		return SexoFeminino, nil
	case SexoOutro:
		return SexoOutro, nil
	default:
		return "", erros.Novo(erros.CategoriaValidacao, "sexo inválido (esperado M, F ou O)")
	}
}

// Identificacao é o Value Object de identificação civil do doente. Invariante do
// DDM-001: pelo menos um de BI ou Passaporte tem de estar presente.
type Identificacao struct {
	NomeCompleto   string
	DataNascimento time.Time
	Sexo           Sexo
	BI             *string
	NIF            *string
	Passaporte     *string
}

// NovaIdentificacao valida e normaliza a identificação. Nome obrigatório; data de
// nascimento não pode ser futura; sexo válido; BI ou Passaporte obrigatório; BI e
// NIF, quando presentes, são validados/normalizados pelo Shared Kernel.
func NovaIdentificacao(nome string, dataNasc time.Time, sexo Sexo, bi, nif, passaporte *string) (Identificacao, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return Identificacao{}, erros.Novo(erros.CategoriaValidacao, "nome completo em falta")
	}
	if dataNasc.After(time.Now()) {
		return Identificacao{}, erros.Novo(erros.CategoriaValidacao, "data de nascimento não pode ser futura")
	}
	if _, err := ParseSexo(string(sexo)); err != nil {
		return Identificacao{}, err
	}

	biNorm := normalizarOpcional(bi)
	passNorm := normalizarOpcional(passaporte)
	if biNorm == nil && passNorm == nil {
		return Identificacao{}, erros.Novo(erros.CategoriaValidacao, "é obrigatório indicar o Bilhete de Identidade ou o Passaporte")
	}

	if biNorm != nil {
		b, err := identity.NovoBI(*biNorm)
		if err != nil {
			return Identificacao{}, erros.Novo(erros.CategoriaValidacao, "bilhete de identidade inválido")
		}
		v := b.String()
		biNorm = &v
	}

	nifNorm := normalizarOpcional(nif)
	if nifNorm != nil {
		n, err := identity.NovoNIF(*nifNorm)
		if err != nil {
			return Identificacao{}, erros.Novo(erros.CategoriaValidacao, "nif inválido")
		}
		v := n.String()
		nifNorm = &v
	}

	return Identificacao{
		NomeCompleto:   nome,
		DataNascimento: dataNasc,
		Sexo:           sexo,
		BI:             biNorm,
		NIF:            nifNorm,
		Passaporte:     passNorm,
	}, nil
}

// normalizarOpcional apara espaços e devolve nil se o resultado for vazio.
func normalizarOpcional(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}
```

- [ ] **Step 4: Implementar `contactos.go`**

```go
package clinico

import (
	"net/mail"
	"strings"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/identity"
)

// Morada é o Value Object de morada do doente (campos morada_* do DDM-001).
type Morada struct {
	Provincia  string
	Municipio  string
	Comuna     string
	Bairro     string
	Rua        string
	Casa       *string
	Referencia *string
}

// Contactos é o Value Object de contactos do doente. Telefone é obrigatório
// (telemóvel angolano); email e morada são opcionais.
type Contactos struct {
	Telefone string
	Email    *string
	Morada   *Morada
}

// NovosContactos valida e normaliza os contactos. Telefone obrigatório e
// normalizado para "+244 9XX XXX XXX"; email validado se presente.
func NovosContactos(telefone string, email *string, morada *Morada) (Contactos, error) {
	tel, err := identity.NovoTelefone(strings.TrimSpace(telefone))
	if err != nil {
		return Contactos{}, erros.Novo(erros.CategoriaValidacao, "telefone inválido")
	}

	emailNorm := normalizarOpcional(email)
	if emailNorm != nil {
		if _, err := mail.ParseAddress(*emailNorm); err != nil {
			return Contactos{}, erros.Novo(erros.CategoriaValidacao, "email inválido")
		}
	}

	return Contactos{
		Telefone: tel.String(),
		Email:    emailNorm,
		Morada:   morada,
	}, nil
}
```

- [ ] **Step 5: Correr os testes e confirmar que passam**

Run: `go test ./internal/domain/clinico/ -v`
Expected: PASS. Corra também `gofmt -l internal/domain/clinico/` (deve vir vazio).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/clinico/identificacao.go internal/domain/clinico/contactos.go internal/domain/clinico/identificacao_test.go internal/domain/clinico/contactos_test.go
git commit -m "feat(clinico): value objects de identificação e contactos do doente

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Entidades-filho e enums clínicos

**Files:**
- Create: `internal/domain/clinico/alergia.go`
- Create: `internal/domain/clinico/antecedente.go`
- Create: `internal/domain/clinico/grupo_sanguineo.go`
- Test: `internal/domain/clinico/alergia_test.go`
- Test: `internal/domain/clinico/antecedente_test.go`
- Test: `internal/domain/clinico/grupo_sanguineo_test.go`

**Interfaces:**
- Consumes: `erros.Novo`, `normalizarOpcional` (Task 2).
- Produces:
  - `type Severidade string`; consts `SeveridadeLeve="LEVE"`, `SeveridadeModerada="MODERADA"`, `SeveridadeGrave="GRAVE"`, `SeveridadeAnafilactica="ANAFILACTICA"`; `ParseSeveridade(string) (Severidade, error)`.
  - `type Alergia struct { Substancia string; Severidade Severidade; ReaccaoTipica string; ConfirmadaEm *time.Time; Notas string }`; `NovaAlergia(substancia string, sev Severidade, reaccao string, confirmadaEm *time.Time, notas string) (Alergia, error)`.
  - `type TipoAntecedente string`; consts `AntecedentePessoal="PESSOAL"`, `AntecedenteFamiliar="FAMILIAR"`, `AntecedenteCirurgico="CIRURGICO"`, `AntecedenteObstetrico="OBSTETRICO"`; `ParseTipoAntecedente(string) (TipoAntecedente, error)`.
  - `type AntecedenteClinico struct { Tipo TipoAntecedente; Descricao string; CID string; DataInicio *time.Time; Activo bool; Notas string }`; `NovoAntecedente(tipo TipoAntecedente, descricao, cid string, dataInicio *time.Time, activo bool, notas string) (AntecedenteClinico, error)`.
  - `type GrupoSanguineo string`; consts para os 8 valores; `ParseGrupoSanguineo(string) (GrupoSanguineo, error)`; método `String() string`.

- [ ] **Step 1: Escrever os testes que falham**

`grupo_sanguineo_test.go`:
```go
package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func TestParseGrupoSanguineo(t *testing.T) {
	g, err := clinico.ParseGrupoSanguineo("o+")
	if err != nil || g != clinico.GrupoOPositivo {
		t.Fatalf("ParseGrupoSanguineo(o+)=%v,%v", g, err)
	}
	if g.String() != "O+" {
		t.Fatalf("String()=%q", g.String())
	}
	if _, err := clinico.ParseGrupoSanguineo("Z+"); err == nil {
		t.Fatal("esperava erro para grupo inválido")
	}
}
```

`alergia_test.go`:
```go
package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovaAlergia(t *testing.T) {
	a, err := clinico.NovaAlergia("Penicilina", clinico.SeveridadeGrave, "Urticária", nil, "")
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if a.Substancia != "Penicilina" || a.Severidade != clinico.SeveridadeGrave {
		t.Fatalf("alergia inesperada: %+v", a)
	}
}

func TestNovaAlergia_SubstanciaObrigatoria(t *testing.T) {
	_, err := clinico.NovaAlergia("  ", clinico.SeveridadeLeve, "", nil, "")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestNovaAlergia_SeveridadeInvalida(t *testing.T) {
	_, err := clinico.NovaAlergia("Penicilina", clinico.Severidade("EXTREMA"), "", nil, "")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}
```

`antecedente_test.go`:
```go
package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovoAntecedente(t *testing.T) {
	a, err := clinico.NovoAntecedente(clinico.AntecedentePessoal, "Hipertensão", "I10", nil, true, "")
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if a.Tipo != clinico.AntecedentePessoal || !a.Activo {
		t.Fatalf("antecedente inesperado: %+v", a)
	}
}

func TestNovoAntecedente_DescricaoObrigatoria(t *testing.T) {
	_, err := clinico.NovoAntecedente(clinico.AntecedenteFamiliar, "", "", nil, true, "")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestParseTipoAntecedente(t *testing.T) {
	if _, err := clinico.ParseTipoAntecedente("cirurgico"); err != nil {
		t.Fatalf("cirurgico devia ser válido: %v", err)
	}
	if _, err := clinico.ParseTipoAntecedente("GENETICO"); err == nil {
		t.Fatal("esperava erro para tipo inválido")
	}
}
```

- [ ] **Step 2: Correr e confirmar que falham**

Run: `go test ./internal/domain/clinico/ -run 'GrupoSanguineo|Alergia|Antecedente' -v`
Expected: FAIL — símbolos indefinidos.

- [ ] **Step 3: Implementar `grupo_sanguineo.go`**

```go
package clinico

import (
	"strings"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// GrupoSanguineo é o grupo sanguíneo ABO/Rh do doente (DDM-001: 8 valores).
type GrupoSanguineo string

const (
	GrupoAPositivo  GrupoSanguineo = "A+"
	GrupoANegativo  GrupoSanguineo = "A-"
	GrupoBPositivo  GrupoSanguineo = "B+"
	GrupoBNegativo  GrupoSanguineo = "B-"
	GrupoABPositivo GrupoSanguineo = "AB+"
	GrupoABNegativo GrupoSanguineo = "AB-"
	GrupoOPositivo  GrupoSanguineo = "O+"
	GrupoONegativo  GrupoSanguineo = "O-"
)

var gruposValidos = map[GrupoSanguineo]bool{
	GrupoAPositivo: true, GrupoANegativo: true,
	GrupoBPositivo: true, GrupoBNegativo: true,
	GrupoABPositivo: true, GrupoABNegativo: true,
	GrupoOPositivo: true, GrupoONegativo: true,
}

// ParseGrupoSanguineo valida e normaliza um grupo sanguíneo (aceita minúsculas).
func ParseGrupoSanguineo(codigo string) (GrupoSanguineo, error) {
	g := GrupoSanguineo(strings.ToUpper(strings.TrimSpace(codigo)))
	if !gruposValidos[g] {
		return "", erros.Novo(erros.CategoriaValidacao, "grupo sanguíneo inválido")
	}
	return g, nil
}

// String devolve a representação canónica do grupo sanguíneo.
func (g GrupoSanguineo) String() string { return string(g) }
```

- [ ] **Step 4: Implementar `alergia.go`**

```go
package clinico

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Severidade classifica a gravidade de uma alergia (DDM-001).
type Severidade string

const (
	SeveridadeLeve         Severidade = "LEVE"
	SeveridadeModerada     Severidade = "MODERADA"
	SeveridadeGrave        Severidade = "GRAVE"
	SeveridadeAnafilactica Severidade = "ANAFILACTICA"
)

var severidadesValidas = map[Severidade]bool{
	SeveridadeLeve: true, SeveridadeModerada: true,
	SeveridadeGrave: true, SeveridadeAnafilactica: true,
}

// ParseSeveridade valida e normaliza uma severidade (aceita minúsculas).
func ParseSeveridade(codigo string) (Severidade, error) {
	s := Severidade(strings.ToUpper(strings.TrimSpace(codigo)))
	if !severidadesValidas[s] {
		return "", erros.Novo(erros.CategoriaValidacao, "severidade inválida (esperado LEVE, MODERADA, GRAVE ou ANAFILACTICA)")
	}
	return s, nil
}

// Alergia é uma entidade-filho do agregado Doente: uma alergia conhecida.
type Alergia struct {
	Substancia    string
	Severidade    Severidade
	ReaccaoTipica string
	ConfirmadaEm  *time.Time
	Notas         string
}

// NovaAlergia valida e constrói uma Alergia. Substância obrigatória; severidade
// válida.
func NovaAlergia(substancia string, sev Severidade, reaccao string, confirmadaEm *time.Time, notas string) (Alergia, error) {
	substancia = strings.TrimSpace(substancia)
	if substancia == "" {
		return Alergia{}, erros.Novo(erros.CategoriaValidacao, "substância da alergia em falta")
	}
	if _, err := ParseSeveridade(string(sev)); err != nil {
		return Alergia{}, err
	}
	return Alergia{
		Substancia:    substancia,
		Severidade:    sev,
		ReaccaoTipica: strings.TrimSpace(reaccao),
		ConfirmadaEm:  confirmadaEm,
		Notas:         strings.TrimSpace(notas),
	}, nil
}
```

- [ ] **Step 5: Implementar `antecedente.go`**

```go
package clinico

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// TipoAntecedente classifica um antecedente clínico (DDM-001).
type TipoAntecedente string

const (
	AntecedentePessoal    TipoAntecedente = "PESSOAL"
	AntecedenteFamiliar   TipoAntecedente = "FAMILIAR"
	AntecedenteCirurgico  TipoAntecedente = "CIRURGICO"
	AntecedenteObstetrico TipoAntecedente = "OBSTETRICO"
)

var tiposAntecedenteValidos = map[TipoAntecedente]bool{
	AntecedentePessoal: true, AntecedenteFamiliar: true,
	AntecedenteCirurgico: true, AntecedenteObstetrico: true,
}

// ParseTipoAntecedente valida e normaliza um tipo de antecedente.
func ParseTipoAntecedente(codigo string) (TipoAntecedente, error) {
	t := TipoAntecedente(strings.ToUpper(strings.TrimSpace(codigo)))
	if !tiposAntecedenteValidos[t] {
		return "", erros.Novo(erros.CategoriaValidacao, "tipo de antecedente inválido (esperado PESSOAL, FAMILIAR, CIRURGICO ou OBSTETRICO)")
	}
	return t, nil
}

// AntecedenteClinico é uma entidade-filho do agregado Doente: um antecedente
// clínico (pessoal, familiar, cirúrgico ou obstétrico).
type AntecedenteClinico struct {
	Tipo       TipoAntecedente
	Descricao  string
	CID        string
	DataInicio *time.Time
	Activo     bool
	Notas      string
}

// NovoAntecedente valida e constrói um AntecedenteClinico. Descrição obrigatória;
// tipo válido.
func NovoAntecedente(tipo TipoAntecedente, descricao, cid string, dataInicio *time.Time, activo bool, notas string) (AntecedenteClinico, error) {
	if _, err := ParseTipoAntecedente(string(tipo)); err != nil {
		return AntecedenteClinico{}, err
	}
	descricao = strings.TrimSpace(descricao)
	if descricao == "" {
		return AntecedenteClinico{}, erros.Novo(erros.CategoriaValidacao, "descrição do antecedente em falta")
	}
	return AntecedenteClinico{
		Tipo:       tipo,
		Descricao:  descricao,
		CID:        strings.TrimSpace(cid),
		DataInicio: dataInicio,
		Activo:     activo,
		Notas:      strings.TrimSpace(notas),
	}, nil
}
```

- [ ] **Step 6: Correr os testes e confirmar que passam**

Run: `go test ./internal/domain/clinico/ -v`
Expected: PASS (todos, incluindo os da Task 2).

- [ ] **Step 7: Commit**

```bash
git add internal/domain/clinico/alergia.go internal/domain/clinico/antecedente.go internal/domain/clinico/grupo_sanguineo.go internal/domain/clinico/alergia_test.go internal/domain/clinico/antecedente_test.go internal/domain/clinico/grupo_sanguineo_test.go
git commit -m "feat(clinico): entidades-filho (alergia, antecedente) e enums clínicos

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Agregado Doente, estados, eventos e porta de repositório

**Files:**
- Create: `internal/domain/clinico/estado.go`
- Create: `internal/domain/clinico/doente.go`
- Create: `internal/domain/clinico/eventos.go`
- Create: `internal/domain/clinico/repositorio.go`
- Test: `internal/domain/clinico/doente_test.go`

**Interfaces:**
- Consumes: `Identificacao`, `Contactos`, `Alergia`, `AntecedenteClinico`, `GrupoSanguineo` (Tasks 2-3); `erros.Novo`; `evento.EventoDominio`.
- Produces:
  - `type EstadoDoente string`; consts `EstadoActivo="ACTIVO"`, `EstadoInactivo="INACTIVO"`, `EstadoFalecido="FALECIDO"`, `EstadoApagado="APAGADO"`.
  - `type Doente struct { ... }` (campos privados) + getters `ID()`, `NumProcesso()`, `Estado()`.
  - `type SnapshotDoente struct { ... }` (todos os campos exportados) — usado para rehidratação e persistência.
  - `NovoDoente(numProcesso string, ident Identificacao, contactos Contactos, nacionalidade string) (*Doente, error)`.
  - `ReconstruirDoente(s SnapshotDoente) *Doente`.
  - Método `(*Doente) Snapshot() SnapshotDoente`.
  - Métodos de negócio: `AdicionarAlergia(Alergia) error`, `AdicionarAntecedente(AntecedenteClinico) error`, `Desactivar(motivo string, em time.Time) error`, `Reactivar() error`, `DeclararFalecido(data time.Time, causaCID string) error`, `AtualizarIdentificacao(Identificacao) error`, `AtualizarContactos(Contactos) error`, `DefinirGrupoSanguineo(*GrupoSanguineo)`.
  - `type RepositorioDoentes interface { ... }`; tipos `FiltroDoentes`, `ResumoDoente`, `PaginaDoentes`.
  - Eventos: `DoenteRegistado`, `DoenteDesactivado`, `DoenteFalecido`, `AlergiaRegistada`.

**Regras de transição:** `Desactivar` só a partir de `ACTIVO`/`INACTIVO` (proíbe `FALECIDO`/`APAGADO`), exige motivo não-vazio, põe `INACTIVO` + `desactivadoEm`/`desactivadoMotivo`. `Reactivar` só a partir de `INACTIVO` → `ACTIVO`, limpa os campos de desactivação. `DeclararFalecido` proíbe se `APAGADO`, exige data não-futura, põe `FALECIDO` + `falecidoEm` (DATE) + `causaMorteCID`. `AdicionarAlergia`/`AdicionarAntecedente` proíbem se `APAGADO`.

- [ ] **Step 1: Escrever o teste que falha (`doente_test.go`)**

```go
package clinico_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func doenteValido(t *testing.T) *clinico.Doente {
	t.Helper()
	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	ident, err := clinico.NovaIdentificacao("Ana Domingos", nasc, clinico.SexoFeminino, ptr("00123456LA042"), nil, nil)
	if err != nil {
		t.Fatalf("identificação: %v", err)
	}
	ct, err := clinico.NovosContactos("+244923456789", nil, nil)
	if err != nil {
		t.Fatalf("contactos: %v", err)
	}
	d, err := clinico.NovoDoente("P-2026-000001", ident, ct, "AO")
	if err != nil {
		t.Fatalf("NovoDoente: %v", err)
	}
	return d
}

func TestNovoDoente_EstadoInicialActivo(t *testing.T) {
	d := doenteValido(t)
	if d.Estado() != clinico.EstadoActivo {
		t.Fatalf("estado inicial=%q, esperava ACTIVO", d.Estado())
	}
	if d.NumProcesso() != "P-2026-000001" {
		t.Fatalf("num processo=%q", d.NumProcesso())
	}
}

func TestNovoDoente_NacionalidadeDefault(t *testing.T) {
	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	ident, _ := clinico.NovaIdentificacao("Ana", nasc, clinico.SexoFeminino, ptr("00123456LA042"), nil, nil)
	ct, _ := clinico.NovosContactos("+244923456789", nil, nil)
	d, err := clinico.NovoDoente("P-2026-000002", ident, ct, "")
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if d.Snapshot().Nacionalidade != "AO" {
		t.Fatalf("nacionalidade default=%q, esperava AO", d.Snapshot().Nacionalidade)
	}
}

func TestDoente_Desactivar(t *testing.T) {
	d := doenteValido(t)
	em := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	if err := d.Desactivar("dados duplicados", em); err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	if d.Estado() != clinico.EstadoInactivo {
		t.Fatalf("estado=%q, esperava INACTIVO", d.Estado())
	}
	if d.Snapshot().DesactivadoMotivo != "dados duplicados" || d.Snapshot().DesactivadoEm == nil {
		t.Fatalf("campos de desactivação não preenchidos: %+v", d.Snapshot())
	}
}

func TestDoente_DesactivarSemMotivo(t *testing.T) {
	d := doenteValido(t)
	if erros.CategoriaDe(d.Desactivar("  ", time.Now())) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para motivo vazio")
	}
}

func TestDoente_DeclararFalecido(t *testing.T) {
	d := doenteValido(t)
	data := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := d.DeclararFalecido(data, "I21"); err != nil {
		t.Fatalf("declarar falecido: %v", err)
	}
	if d.Estado() != clinico.EstadoFalecido {
		t.Fatalf("estado=%q, esperava FALECIDO", d.Estado())
	}
	// Um doente falecido não pode ser desactivado.
	if d.Desactivar("x", time.Now()) == nil {
		t.Fatal("esperava erro ao desactivar um falecido")
	}
}

func TestDoente_DeclararFalecidoDataFutura(t *testing.T) {
	d := doenteValido(t)
	if erros.CategoriaDe(d.DeclararFalecido(time.Now().AddDate(0, 1, 0), "")) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para data de óbito futura")
	}
}

func TestDoente_Reactivar(t *testing.T) {
	d := doenteValido(t)
	_ = d.Desactivar("engano", time.Now())
	if err := d.Reactivar(); err != nil {
		t.Fatalf("reactivar: %v", err)
	}
	if d.Estado() != clinico.EstadoActivo || d.Snapshot().DesactivadoEm != nil {
		t.Fatalf("reactivação incompleta: %+v", d.Snapshot())
	}
}

func TestDoente_AdicionarAlergia(t *testing.T) {
	d := doenteValido(t)
	a, _ := clinico.NovaAlergia("Penicilina", clinico.SeveridadeGrave, "", nil, "")
	if err := d.AdicionarAlergia(a); err != nil {
		t.Fatalf("adicionar alergia: %v", err)
	}
	if len(d.Snapshot().Alergias) != 1 {
		t.Fatalf("esperava 1 alergia, obtive %d", len(d.Snapshot().Alergias))
	}
}

func TestReconstruirDoente_PreservaEstado(t *testing.T) {
	orig := doenteValido(t)
	_ = orig.Desactivar("motivo", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	snap := orig.Snapshot()
	snap.ID = "id-1"
	rec := clinico.ReconstruirDoente(snap)
	if rec.ID() != "id-1" || rec.Estado() != clinico.EstadoInactivo {
		t.Fatalf("rehidratação perdeu estado: id=%q estado=%q", rec.ID(), rec.Estado())
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/domain/clinico/ -run Doente -v`
Expected: FAIL — símbolos indefinidos.

- [ ] **Step 3: Implementar `estado.go`**

```go
package clinico

// EstadoDoente é o estado do ciclo de vida de um doente (DDM-001).
type EstadoDoente string

const (
	EstadoActivo   EstadoDoente = "ACTIVO"
	EstadoInactivo EstadoDoente = "INACTIVO"
	EstadoFalecido EstadoDoente = "FALECIDO"
	EstadoApagado  EstadoDoente = "APAGADO"
)
```

- [ ] **Step 4: Implementar `doente.go`**

```go
package clinico

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Doente é o agregado raiz do BC Clínico. Encapsula a identificação, os contactos,
// o estado do ciclo de vida e as entidades-filho (alergias e antecedentes). Os
// campos são privados: a construção (NovoDoente) e as transições garantem os
// invariantes. O id é gerado pela base de dados (vazio até persistir).
type Doente struct {
	id                string
	numProcesso       string
	identificacao     Identificacao
	contactos         Contactos
	nacionalidade     string
	grupoSanguineo    *GrupoSanguineo
	estado            EstadoDoente
	alergias          []Alergia
	antecedentes      []AntecedenteClinico
	criadoEm          time.Time
	actualizadoEm     time.Time
	desactivadoEm     *time.Time
	desactivadoMotivo string
	falecidoEm        *time.Time
	causaMorteCID     string
}

// NovoDoente valida e constrói um novo Doente no estado ACTIVO. O número de
// processo já vem resolvido (automático ou manual) da camada de aplicação.
// nacionalidade vazia assume "AO".
func NovoDoente(numProcesso string, ident Identificacao, contactos Contactos, nacionalidade string) (*Doente, error) {
	numProcesso = strings.TrimSpace(numProcesso)
	if numProcesso == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "número de processo em falta")
	}
	// Revalida os VOs recebidos (defesa em profundidade: garante que não foram
	// construídos por atribuição directa a partir de dados não validados).
	if _, err := NovaIdentificacao(ident.NomeCompleto, ident.DataNascimento, ident.Sexo, ident.BI, ident.NIF, ident.Passaporte); err != nil {
		return nil, err
	}
	if contactos.Telefone == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "contactos inválidos")
	}
	nac := strings.TrimSpace(nacionalidade)
	if nac == "" {
		nac = "AO"
	}
	return &Doente{
		numProcesso:   numProcesso,
		identificacao: ident,
		contactos:     contactos,
		nacionalidade: nac,
		estado:        EstadoActivo,
	}, nil
}

// ID devolve o identificador atribuído pela base de dados (vazio se ainda não
// persistido).
func (d *Doente) ID() string { return d.id }

// NumProcesso devolve o número de processo do doente.
func (d *Doente) NumProcesso() string { return d.numProcesso }

// Estado devolve o estado actual do ciclo de vida.
func (d *Doente) Estado() EstadoDoente { return d.estado }

// AdicionarAlergia acrescenta uma alergia ao doente. Proibido se o doente estiver
// apagado.
func (d *Doente) AdicionarAlergia(a Alergia) error {
	if d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível alterar um doente apagado")
	}
	if _, err := NovaAlergia(a.Substancia, a.Severidade, a.ReaccaoTipica, a.ConfirmadaEm, a.Notas); err != nil {
		return err
	}
	d.alergias = append(d.alergias, a)
	return nil
}

// AdicionarAntecedente acrescenta um antecedente clínico ao doente. Proibido se o
// doente estiver apagado.
func (d *Doente) AdicionarAntecedente(a AntecedenteClinico) error {
	if d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível alterar um doente apagado")
	}
	if _, err := NovoAntecedente(a.Tipo, a.Descricao, a.CID, a.DataInicio, a.Activo, a.Notas); err != nil {
		return err
	}
	d.antecedentes = append(d.antecedentes, a)
	return nil
}

// Desactivar coloca o doente em INACTIVO com um motivo. Proíbe se já estiver
// falecido ou apagado.
func (d *Doente) Desactivar(motivo string, em time.Time) error {
	if d.estado == EstadoFalecido || d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível desactivar um doente falecido ou apagado")
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return erros.Novo(erros.CategoriaValidacao, "motivo de desactivação em falta")
	}
	d.estado = EstadoInactivo
	d.desactivadoEm = &em
	d.desactivadoMotivo = motivo
	return nil
}

// Reactivar repõe um doente inactivo em ACTIVO, limpando os campos de
// desactivação. Só válido a partir de INACTIVO.
func (d *Doente) Reactivar() error {
	if d.estado != EstadoInactivo {
		return erros.Novo(erros.CategoriaConflito, "apenas um doente inactivo pode ser reactivado")
	}
	d.estado = EstadoActivo
	d.desactivadoEm = nil
	d.desactivadoMotivo = ""
	return nil
}

// DeclararFalecido coloca o doente em FALECIDO com a data de óbito e a causa
// (código CID opcional). Proíbe se apagado; a data não pode ser futura.
func (d *Doente) DeclararFalecido(data time.Time, causaCID string) error {
	if d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível declarar falecido um doente apagado")
	}
	if data.After(time.Now()) {
		return erros.Novo(erros.CategoriaValidacao, "data de óbito não pode ser futura")
	}
	d.estado = EstadoFalecido
	d.falecidoEm = &data
	d.causaMorteCID = strings.TrimSpace(causaCID)
	return nil
}

// AtualizarIdentificacao substitui a identificação (já validada como VO). Proíbe
// se apagado.
func (d *Doente) AtualizarIdentificacao(ident Identificacao) error {
	if d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível alterar um doente apagado")
	}
	if _, err := NovaIdentificacao(ident.NomeCompleto, ident.DataNascimento, ident.Sexo, ident.BI, ident.NIF, ident.Passaporte); err != nil {
		return err
	}
	d.identificacao = ident
	return nil
}

// AtualizarContactos substitui os contactos (já validados como VO). Proíbe se
// apagado.
func (d *Doente) AtualizarContactos(ct Contactos) error {
	if d.estado == EstadoApagado {
		return erros.Novo(erros.CategoriaConflito, "não é possível alterar um doente apagado")
	}
	if ct.Telefone == "" {
		return erros.Novo(erros.CategoriaValidacao, "contactos inválidos")
	}
	d.contactos = ct
	return nil
}

// DefinirGrupoSanguineo define (ou limpa, com nil) o grupo sanguíneo.
func (d *Doente) DefinirGrupoSanguineo(g *GrupoSanguineo) {
	d.grupoSanguineo = g
}

// SnapshotDoente carrega o estado completo de um Doente para persistência ou
// rehidratação. Não valida invariantes — os dados vêm de fonte confiável
// (agregado em memória ou base de dados).
type SnapshotDoente struct {
	ID                string
	NumProcesso       string
	Identificacao     Identificacao
	Contactos         Contactos
	Nacionalidade     string
	GrupoSanguineo    *GrupoSanguineo
	Estado            EstadoDoente
	Alergias          []Alergia
	Antecedentes      []AntecedenteClinico
	CriadoEm          time.Time
	ActualizadoEm     time.Time
	DesactivadoEm     *time.Time
	DesactivadoMotivo string
	FalecidoEm        *time.Time
	CausaMorteCID     string
}

// Snapshot devolve o estado completo do agregado (para mapear DTOs ou persistir).
func (d *Doente) Snapshot() SnapshotDoente {
	return SnapshotDoente{
		ID:                d.id,
		NumProcesso:       d.numProcesso,
		Identificacao:     d.identificacao,
		Contactos:         d.contactos,
		Nacionalidade:     d.nacionalidade,
		GrupoSanguineo:    d.grupoSanguineo,
		Estado:            d.estado,
		Alergias:          d.alergias,
		Antecedentes:      d.antecedentes,
		CriadoEm:          d.criadoEm,
		ActualizadoEm:     d.actualizadoEm,
		DesactivadoEm:     d.desactivadoEm,
		DesactivadoMotivo: d.desactivadoMotivo,
		FalecidoEm:        d.falecidoEm,
		CausaMorteCID:     d.causaMorteCID,
	}
}

// ReconstruirDoente reconstrói um agregado a partir de um snapshot persistido
// (usado pelo repositório na leitura). Não revalida invariantes.
func ReconstruirDoente(s SnapshotDoente) *Doente {
	return &Doente{
		id:                s.ID,
		numProcesso:       s.NumProcesso,
		identificacao:     s.Identificacao,
		contactos:         s.Contactos,
		nacionalidade:     s.Nacionalidade,
		grupoSanguineo:    s.GrupoSanguineo,
		estado:            s.Estado,
		alergias:          s.Alergias,
		antecedentes:      s.Antecedentes,
		criadoEm:          s.CriadoEm,
		actualizadoEm:     s.ActualizadoEm,
		desactivadoEm:     s.DesactivadoEm,
		desactivadoMotivo: s.DesactivadoMotivo,
		falecidoEm:        s.FalecidoEm,
		causaMorteCID:     s.CausaMorteCID,
	}
}
```

- [ ] **Step 5: Implementar `repositorio.go`**

```go
package clinico

import (
	"context"
	"time"
)

// FiltroDoentes parametriza a pesquisa de doentes.
type FiltroDoentes struct {
	Termo        string // nome (fuzzy), BI, num de processo ou telefone (exacto)
	Estado       string // filtro opcional por estado
	Limite       int    // máximo de resultados
	Deslocamento int    // paginação (offset)
}

// ResumoDoente é o read-model de um doente numa listagem/pesquisa.
type ResumoDoente struct {
	ID             string    `json:"id"`
	NumProcesso    string    `json:"num_processo"`
	NomeCompleto   string    `json:"nome_completo"`
	DataNascimento time.Time `json:"data_nascimento"`
	Sexo           string    `json:"sexo"`
	Telefone       string    `json:"telefone"`
	Estado         string    `json:"estado"`
}

// PaginaDoentes é uma página de resultados de pesquisa.
type PaginaDoentes struct {
	Itens        []ResumoDoente `json:"itens"`
	Total        int            `json:"total"`
	Limite       int            `json:"limite"`
	Deslocamento int            `json:"deslocamento"`
}

// RepositorioDoentes é a porta de saída para persistência do agregado Doente. A
// implementação vive em adapters/pgrepo.
type RepositorioDoentes interface {
	// Guardar persiste o doente (INSERT se id vazio, senão UPDATE) e devolve o id.
	// Conflitos de unicidade (num de processo ou BI) devolvem CategoriaConflito.
	Guardar(ctx context.Context, d *Doente) (string, error)
	// ObterPorID devolve o doente e as suas entidades-filho. NaoEncontrado se não existir.
	ObterPorID(ctx context.Context, id string) (*Doente, error)
	// ObterPorNumProcesso devolve o doente pelo número de processo. NaoEncontrado se não existir.
	ObterPorNumProcesso(ctx context.Context, num string) (*Doente, error)
	// Pesquisar devolve uma página de doentes segundo o filtro.
	Pesquisar(ctx context.Context, f FiltroDoentes) (PaginaDoentes, error)
	// ProximoNumeroProcesso reserva e devolve o próximo número automático do ano
	// indicado, no formato "P-{ano}-{sequencial:06d}".
	ProximoNumeroProcesso(ctx context.Context, ano int) (string, error)
}
```

- [ ] **Step 6: Implementar `eventos.go`**

```go
package clinico

import (
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

// DoenteRegistado é emitido quando um doente é registado.
type DoenteRegistado struct {
	DoenteID string
	Em       time.Time
}

func (e DoenteRegistado) NomeEvento() string    { return "clinico.doente.registado" }
func (e DoenteRegistado) OcorridoEm() time.Time { return e.Em }

// DoenteDesactivado é emitido quando um doente é desactivado.
type DoenteDesactivado struct {
	DoenteID string
	Em       time.Time
}

func (e DoenteDesactivado) NomeEvento() string    { return "clinico.doente.desactivado" }
func (e DoenteDesactivado) OcorridoEm() time.Time { return e.Em }

// DoenteFalecido é emitido quando um doente é declarado falecido.
type DoenteFalecido struct {
	DoenteID string
	Em       time.Time
}

func (e DoenteFalecido) NomeEvento() string    { return "clinico.doente.falecido" }
func (e DoenteFalecido) OcorridoEm() time.Time { return e.Em }

// AlergiaRegistada é emitido quando uma alergia é registada num doente.
type AlergiaRegistada struct {
	DoenteID string
	Em       time.Time
}

func (e AlergiaRegistada) NomeEvento() string    { return "clinico.alergia.registada" }
func (e AlergiaRegistada) OcorridoEm() time.Time { return e.Em }

// Garantias de conformidade com a interface de evento de domínio.
var (
	_ evento.EventoDominio = DoenteRegistado{}
	_ evento.EventoDominio = DoenteDesactivado{}
	_ evento.EventoDominio = DoenteFalecido{}
	_ evento.EventoDominio = AlergiaRegistada{}
)
```

- [ ] **Step 7: Correr os testes e a cobertura do domínio**

Run: `go test ./internal/domain/clinico/ -v`
Expected: PASS.
Run: `bash scripts/cobertura.sh` (secção domínio) — Expected: domínio ≥85%.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/clinico/estado.go internal/domain/clinico/doente.go internal/domain/clinico/eventos.go internal/domain/clinico/repositorio.go internal/domain/clinico/doente_test.go
git commit -m "feat(clinico): agregado Doente com estados, eventos e porta de repositório

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Migration do schema clínico e registo no embed

**Files:**
- Create: `migrations/clinico/0001_doentes.sql`
- Modify: `migrations/embed.go` (linha do `//go:embed`)

**Interfaces:**
- Consumes: runner de migrations existente (`internal/platform/db/migrate.go`, aplica por subdirectório alfabético).
- Produces: schema `clinico` com tabelas `doentes`, `alergias`, `antecedentes_clinicos`, `processo_sequencia`, extensão `pg_trgm` e índices.

**Contexto:** O runner descobre bounded contexts pelos subdirectórios de `migrations/`. O `embed.go` tem `//go:embed auditoria identidade shared` — é preciso acrescentar `clinico`. O schema `clinico` já é criado por `docker/postgres/init.sql`, mas a migration usa `CREATE SCHEMA IF NOT EXISTS` para ser auto-suficiente em ambientes de teste.

- [ ] **Step 1: Criar `migrations/clinico/0001_doentes.sql`**

```sql
-- Bounded Context: clinico
-- Migration forward-only. Esquema extraído verbatim do DDM-001 v2.0.
--
-- Agregado Doente e entidades-filho (alergias, antecedentes clínicos). As
-- tabelas consentimentos e episodios_clinicos do DDM ficam para fatias futuras.

CREATE SCHEMA IF NOT EXISTS clinico;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS clinico.doentes (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    num_processo        text        NOT NULL UNIQUE,
    nome_completo       text        NOT NULL,
    data_nascimento     date        NOT NULL,
    sexo                char(1)     NOT NULL CHECK (sexo IN ('M','F','O')),
    bi                  text,
    nif                 text,
    passaporte          text,
    nacionalidade       text        NOT NULL DEFAULT 'AO',
    telefone            text        NOT NULL,
    email               text,
    morada_provincia    text,
    morada_municipio    text,
    morada_comuna       text,
    morada_bairro       text,
    morada_rua          text,
    morada_casa         text,
    morada_referencia   text,
    grupo_sanguineo     text        CHECK (grupo_sanguineo IN ('A+','A-','B+','B-','AB+','AB-','O+','O-')),
    estado              text        NOT NULL DEFAULT 'ACTIVO'
                        CHECK (estado IN ('ACTIVO','INACTIVO','FALECIDO','APAGADO')),
    falecido_em         date,
    causa_morte_cid     text,
    criado_em           timestamptz NOT NULL DEFAULT now(),
    actualizado_em      timestamptz NOT NULL DEFAULT now(),
    desactivado_em      timestamptz,
    desactivado_motivo  text,
    apagado_em          timestamptz,
    CONSTRAINT doc_identificacao CHECK (bi IS NOT NULL OR passaporte IS NOT NULL)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_doentes_bi
    ON clinico.doentes (bi) WHERE bi IS NOT NULL AND apagado_em IS NULL;
CREATE INDEX IF NOT EXISTS idx_doentes_nome
    ON clinico.doentes USING gin (nome_completo gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_doentes_telefone ON clinico.doentes (telefone);
CREATE INDEX IF NOT EXISTS idx_doentes_estado
    ON clinico.doentes (estado) WHERE desactivado_em IS NULL;

COMMENT ON TABLE clinico.doentes IS
    'Doente (agregado raiz do BC Clínico). Esquema extraído do DDM-001 v2.0.';
COMMENT ON COLUMN clinico.doentes.num_processo IS
    'Número de processo: automático "P-{ano}-{sequencial}" ou manual (migração).';

CREATE TABLE IF NOT EXISTS clinico.alergias (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id      uuid        NOT NULL REFERENCES clinico.doentes(id) ON DELETE CASCADE,
    substancia     text        NOT NULL,
    severidade     text        NOT NULL CHECK (severidade IN ('LEVE','MODERADA','GRAVE','ANAFILACTICA')),
    reaccao_tipica text,
    confirmada_em  date,
    notas          text,
    criada_em      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_alergias_doente ON clinico.alergias (doente_id);

CREATE TABLE IF NOT EXISTS clinico.antecedentes_clinicos (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    doente_id   uuid        NOT NULL REFERENCES clinico.doentes(id) ON DELETE CASCADE,
    tipo        text        NOT NULL CHECK (tipo IN ('PESSOAL','FAMILIAR','CIRURGICO','OBSTETRICO')),
    descricao   text        NOT NULL,
    cid         text,
    data_inicio date,
    activo      boolean     NOT NULL DEFAULT true,
    notas       text,
    criado_em   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_antecedentes_doente ON clinico.antecedentes_clinicos (doente_id);

-- Contador por ano para o número de processo automático.
CREATE TABLE IF NOT EXISTS clinico.processo_sequencia (
    ano    int PRIMARY KEY,
    ultimo int NOT NULL DEFAULT 0
);
```

- [ ] **Step 2: Actualizar `migrations/embed.go`**

Alterar a directiva de embed (linha 10) para incluir `clinico` (mantendo a ordem alfabética):

```go
//go:embed auditoria clinico identidade shared
var FS embed.FS
```

- [ ] **Step 3: Confirmar que o embed compila e o teste do embed passa**

Run: `go test ./migrations/ -v`
Expected: PASS (o `embed_test.go` valida que as migrations são legíveis).
Run: `go build ./...`
Expected: sem erros.

- [ ] **Step 4: Commit**

```bash
git add migrations/clinico/0001_doentes.sql migrations/embed.go
git commit -m "feat(clinico): migration do schema clínico (doentes, alergias, antecedentes)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Portas/DTOs da aplicação e caso de uso Registar Doente

**Files:**
- Create: `internal/application/clinico/ports.go`
- Create: `internal/application/clinico/mapa.go`
- Create: `internal/application/clinico/registar_doente.go`
- Test: `internal/application/clinico/registar_doente_test.go`
- Test: `internal/application/clinico/fakes_test.go`

**Interfaces:**
- Consumes: domínio `clinico` (Tasks 2-4); `auditoria.Registo`.
- Produces:
  - Porta `Auditor interface { Registar(ctx, auditoria.Registo) error }`.
  - Reexports: `FiltroDoentes = clinico.FiltroDoentes`, `PaginaDoentes = clinico.PaginaDoentes`, `ResumoDoente = clinico.ResumoDoente`.
  - DTOs: `DadosMorada`, `DadosIdentificacao`, `DadosContactos`, `DadosNovoDoente`, `DadosActualizarDoente`, `DadosAlergia`, `DadosAntecedente`, `DetalheDoente` (+ `MoradaDTO`, `AlergiaDTO`, `AntecedenteDTO`).
  - `paraDetalhe(d *clinico.Doente) DetalheDoente`, `construirIdentificacao(DadosIdentificacao) (clinico.Identificacao, error)`, `construirContactos(DadosContactos) (clinico.Contactos, error)`.
  - `CasoRegistarDoente` com `NovoCasoRegistarDoente(repo clinico.RepositorioDoentes, aud Auditor) *CasoRegistarDoente` e `Executar(ctx, actor string, dados DadosNovoDoente) (DetalheDoente, error)`.

**Regras Registar:** se `dados.NumProcesso` vazio → `repo.ProximoNumeroProcesso(ctx, agora().Year())`; constrói identificação/contactos/grupo; `NovoDoente`; `repo.Guardar`; audita `clinico.doente.registado` (EntidadeID = id devolvido); re-lê via `repo.ObterPorID` e devolve `DetalheDoente`.

- [ ] **Step 1: Escrever os fakes e o teste que falha**

`fakes_test.go`:
```go
package clinico_test

import (
	"context"
	"strconv"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeRepo é um repositório de doentes em memória para os testes de aplicação.
type fakeRepo struct {
	porID      map[string]*clinico.Doente
	seq        int
	proxErr    error
	guardarErr error
	pagina     clinico.PaginaDoentes
	ultimoFilt clinico.FiltroDoentes
}

func novoFakeRepo() *fakeRepo { return &fakeRepo{porID: map[string]*clinico.Doente{}} }

func (f *fakeRepo) Guardar(_ context.Context, d *clinico.Doente) (string, error) {
	if f.guardarErr != nil {
		return "", f.guardarErr
	}
	snap := d.Snapshot()
	id := snap.ID
	if id == "" {
		f.seq++
		id = "id-" + strconv.Itoa(f.seq)
		snap.ID = id
	}
	f.porID[id] = clinico.ReconstruirDoente(snap)
	return id, nil
}

func (f *fakeRepo) ObterPorID(_ context.Context, id string) (*clinico.Doente, error) {
	d, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
	}
	return d, nil
}

func (f *fakeRepo) ObterPorNumProcesso(_ context.Context, num string) (*clinico.Doente, error) {
	for _, d := range f.porID {
		if d.NumProcesso() == num {
			return d, nil
		}
	}
	return nil, erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
}

func (f *fakeRepo) Pesquisar(_ context.Context, filt clinico.FiltroDoentes) (clinico.PaginaDoentes, error) {
	f.ultimoFilt = filt
	return f.pagina, nil
}

func (f *fakeRepo) ProximoNumeroProcesso(_ context.Context, ano int) (string, error) {
	if f.proxErr != nil {
		return "", f.proxErr
	}
	f.seq++
	return "P-" + strconv.Itoa(ano) + "-" + leftPad(f.seq), nil
}

func leftPad(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 6 {
		s = "0" + s
	}
	return s
}

// fakeAuditor recolhe os registos de auditoria.
type fakeAuditor struct{ registos []auditoria.Registo }

func (a *fakeAuditor) Registar(_ context.Context, r auditoria.Registo) error {
	a.registos = append(a.registos, r)
	return nil
}
```

`registar_doente_test.go`:
```go
package clinico_test

import (
	"context"
	"testing"
	"time"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func dadosBase() appclinico.DadosNovoDoente {
	return appclinico.DadosNovoDoente{
		Identificacao: appclinico.DadosIdentificacao{
			NomeCompleto:   "Ana Domingos",
			DataNascimento: time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC),
			Sexo:           "F",
			BI:             ptrS("00123456LA042"),
		},
		Contactos: appclinico.DadosContactos{Telefone: "+244923456789"},
	}
}

func ptrS(s string) *string { return &s }

func TestRegistarDoente_NumeroAutomatico(t *testing.T) {
	repo := novoFakeRepo()
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoRegistarDoente(repo, aud)

	out, err := caso.Executar(context.Background(), "actor-1", dadosBase())
	if err != nil {
		t.Fatalf("registar: %v", err)
	}
	if out.NumProcesso == "" || out.ID == "" {
		t.Fatalf("saída incompleta: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.registado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
	if aud.registos[0].Actor != "actor-1" || aud.registos[0].EntidadeID != out.ID {
		t.Fatalf("auditoria com dados errados: %+v", aud.registos[0])
	}
}

func TestRegistarDoente_NumeroManual(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoRegistarDoente(repo, &fakeAuditor{})
	dados := dadosBase()
	dados.NumProcesso = "PROC-LEGADO-42"

	out, err := caso.Executar(context.Background(), "actor-1", dados)
	if err != nil {
		t.Fatalf("registar: %v", err)
	}
	if out.NumProcesso != "PROC-LEGADO-42" {
		t.Fatalf("num de processo manual não respeitado: %q", out.NumProcesso)
	}
}

func TestRegistarDoente_IdentificacaoInvalida(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoRegistarDoente(repo, &fakeAuditor{})
	dados := dadosBase()
	dados.Identificacao.BI = nil
	dados.Identificacao.Passaporte = nil // sem BI nem passaporte

	_, err := caso.Executar(context.Background(), "actor-1", dados)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/clinico/ -v`
Expected: FAIL — pacote/símbolos inexistentes.

- [ ] **Step 3: Implementar `ports.go`**

```go
// Package clinico contém os casos de uso do BC Clínico (Camada 2 — Aplicação).
// Orquestra o agregado Doente sobre portas de saída (repositório, auditoria),
// sem qualquer dependência de infra.
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// Auditor persiste registos de auditoria de forma append-only. Implementado por
// pgrepo.RepositorioAuditoria (partilhado com o BC Identidade).
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}

// Reexports dos read-models do domínio, para os handlers não importarem o domínio.
type (
	FiltroDoentes = dominio.FiltroDoentes
	PaginaDoentes = dominio.PaginaDoentes
	ResumoDoente  = dominio.ResumoDoente
)

// DadosMorada é a morada num pedido.
type DadosMorada struct {
	Provincia  string  `json:"provincia"`
	Municipio  string  `json:"municipio"`
	Comuna     string  `json:"comuna"`
	Bairro     string  `json:"bairro"`
	Rua        string  `json:"rua"`
	Casa       *string `json:"casa,omitempty"`
	Referencia *string `json:"referencia,omitempty"`
}

// DadosIdentificacao é a identificação num pedido.
type DadosIdentificacao struct {
	NomeCompleto   string    `json:"nome_completo"`
	DataNascimento time.Time `json:"-"`
	Sexo           string    `json:"sexo"`
	BI             *string   `json:"bi,omitempty"`
	NIF            *string   `json:"nif,omitempty"`
	Passaporte     *string   `json:"passaporte,omitempty"`
}

// DadosContactos são os contactos num pedido.
type DadosContactos struct {
	Telefone string       `json:"telefone"`
	Email    *string      `json:"email,omitempty"`
	Morada   *DadosMorada `json:"morada,omitempty"`
}

// DadosNovoDoente é a entrada do caso de uso de registo.
type DadosNovoDoente struct {
	NumProcesso    string
	Identificacao  DadosIdentificacao
	Contactos      DadosContactos
	Nacionalidade  string
	GrupoSanguineo *string
}

// DadosActualizarDoente é a entrada da actualização (campos a nil são ignorados).
type DadosActualizarDoente struct {
	Identificacao  *DadosIdentificacao
	Contactos      *DadosContactos
	GrupoSanguineo *string // "" limpa; presente redefine
}

// DadosAlergia é a entrada do registo de alergia.
type DadosAlergia struct {
	Substancia    string     `json:"substancia"`
	Severidade    string     `json:"severidade"`
	ReaccaoTipica string     `json:"reaccao_tipica,omitempty"`
	ConfirmadaEm  *time.Time `json:"-"`
	Notas         string     `json:"notas,omitempty"`
}

// DadosAntecedente é a entrada do registo de antecedente clínico.
type DadosAntecedente struct {
	Tipo       string     `json:"tipo"`
	Descricao  string     `json:"descricao"`
	CID        string     `json:"cid,omitempty"`
	DataInicio *time.Time `json:"-"`
	Activo     bool       `json:"activo"`
	Notas      string     `json:"notas,omitempty"`
}

// MoradaDTO é a morada numa resposta.
type MoradaDTO struct {
	Provincia  string  `json:"provincia"`
	Municipio  string  `json:"municipio"`
	Comuna     string  `json:"comuna"`
	Bairro     string  `json:"bairro"`
	Rua        string  `json:"rua"`
	Casa       *string `json:"casa,omitempty"`
	Referencia *string `json:"referencia,omitempty"`
}

// AlergiaDTO é uma alergia numa resposta.
type AlergiaDTO struct {
	Substancia    string     `json:"substancia"`
	Severidade    string     `json:"severidade"`
	ReaccaoTipica string     `json:"reaccao_tipica,omitempty"`
	ConfirmadaEm  *time.Time `json:"confirmada_em,omitempty"`
	Notas         string     `json:"notas,omitempty"`
}

// AntecedenteDTO é um antecedente numa resposta.
type AntecedenteDTO struct {
	Tipo       string     `json:"tipo"`
	Descricao  string     `json:"descricao"`
	CID        string     `json:"cid,omitempty"`
	DataInicio *time.Time `json:"data_inicio,omitempty"`
	Activo     bool       `json:"activo"`
	Notas      string     `json:"notas,omitempty"`
}

// DetalheDoente é o detalhe completo de um doente numa resposta.
type DetalheDoente struct {
	ID             string           `json:"id"`
	NumProcesso    string           `json:"num_processo"`
	NomeCompleto   string           `json:"nome_completo"`
	DataNascimento time.Time        `json:"data_nascimento"`
	Sexo           string           `json:"sexo"`
	BI             *string          `json:"bi,omitempty"`
	NIF            *string          `json:"nif,omitempty"`
	Passaporte     *string          `json:"passaporte,omitempty"`
	Nacionalidade  string           `json:"nacionalidade"`
	Telefone       string           `json:"telefone"`
	Email          *string          `json:"email,omitempty"`
	Morada         *MoradaDTO       `json:"morada,omitempty"`
	GrupoSanguineo *string          `json:"grupo_sanguineo,omitempty"`
	Estado         string           `json:"estado"`
	Alergias       []AlergiaDTO     `json:"alergias"`
	Antecedentes   []AntecedenteDTO `json:"antecedentes"`
	CriadoEm       time.Time        `json:"criado_em"`
	ActualizadoEm  time.Time        `json:"actualizado_em"`
}
```

- [ ] **Step 4: Implementar `mapa.go` (conversões DTO ⇄ domínio)**

```go
package clinico

import dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"

// construirIdentificacao converte o DTO de identificação num VO validado.
func construirIdentificacao(d DadosIdentificacao) (dominio.Identificacao, error) {
	sexo, err := dominio.ParseSexo(d.Sexo)
	if err != nil {
		return dominio.Identificacao{}, err
	}
	return dominio.NovaIdentificacao(d.NomeCompleto, d.DataNascimento, sexo, d.BI, d.NIF, d.Passaporte)
}

// construirContactos converte o DTO de contactos num VO validado.
func construirContactos(d DadosContactos) (dominio.Contactos, error) {
	var morada *dominio.Morada
	if d.Morada != nil {
		morada = &dominio.Morada{
			Provincia: d.Morada.Provincia, Municipio: d.Morada.Municipio,
			Comuna: d.Morada.Comuna, Bairro: d.Morada.Bairro, Rua: d.Morada.Rua,
			Casa: d.Morada.Casa, Referencia: d.Morada.Referencia,
		}
	}
	return dominio.NovosContactos(d.Telefone, d.Email, morada)
}

// paraDetalhe mapeia um agregado Doente para o DTO de detalhe.
func paraDetalhe(d *dominio.Doente) DetalheDoente {
	s := d.Snapshot()
	det := DetalheDoente{
		ID:             s.ID,
		NumProcesso:    s.NumProcesso,
		NomeCompleto:   s.Identificacao.NomeCompleto,
		DataNascimento: s.Identificacao.DataNascimento,
		Sexo:           string(s.Identificacao.Sexo),
		BI:             s.Identificacao.BI,
		NIF:            s.Identificacao.NIF,
		Passaporte:     s.Identificacao.Passaporte,
		Nacionalidade:  s.Nacionalidade,
		Telefone:       s.Contactos.Telefone,
		Email:          s.Contactos.Email,
		Estado:         string(s.Estado),
		CriadoEm:       s.CriadoEm,
		ActualizadoEm:  s.ActualizadoEm,
		Alergias:       []AlergiaDTO{},
		Antecedentes:   []AntecedenteDTO{},
	}
	if s.Contactos.Morada != nil {
		m := s.Contactos.Morada
		det.Morada = &MoradaDTO{
			Provincia: m.Provincia, Municipio: m.Municipio, Comuna: m.Comuna,
			Bairro: m.Bairro, Rua: m.Rua, Casa: m.Casa, Referencia: m.Referencia,
		}
	}
	if s.GrupoSanguineo != nil {
		g := s.GrupoSanguineo.String()
		det.GrupoSanguineo = &g
	}
	for _, a := range s.Alergias {
		det.Alergias = append(det.Alergias, AlergiaDTO{
			Substancia: a.Substancia, Severidade: string(a.Severidade),
			ReaccaoTipica: a.ReaccaoTipica, ConfirmadaEm: a.ConfirmadaEm, Notas: a.Notas,
		})
	}
	for _, a := range s.Antecedentes {
		det.Antecedentes = append(det.Antecedentes, AntecedenteDTO{
			Tipo: string(a.Tipo), Descricao: a.Descricao, CID: a.CID,
			DataInicio: a.DataInicio, Activo: a.Activo, Notas: a.Notas,
		})
	}
	return det
}
```

- [ ] **Step 5: Implementar `registar_doente.go`**

```go
package clinico

import (
	"context"
	"strings"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarDoente regista um novo doente (com número de processo automático
// ou manual) e audita a operação.
type CasoRegistarDoente struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoRegistarDoente constrói o caso de uso.
func NovoCasoRegistarDoente(repo dominio.RepositorioDoentes, aud Auditor) *CasoRegistarDoente {
	return &CasoRegistarDoente{repo: repo, auditor: aud, agora: time.Now}
}

// Executar valida os dados, resolve o número de processo, persiste o doente,
// audita e devolve o detalhe.
func (c *CasoRegistarDoente) Executar(ctx context.Context, actor string, dados DadosNovoDoente) (DetalheDoente, error) {
	ident, err := construirIdentificacao(dados.Identificacao)
	if err != nil {
		return DetalheDoente{}, err
	}
	contactos, err := construirContactos(dados.Contactos)
	if err != nil {
		return DetalheDoente{}, err
	}

	numProcesso := strings.TrimSpace(dados.NumProcesso)
	if numProcesso == "" {
		gerado, err := c.repo.ProximoNumeroProcesso(ctx, c.agora().Year())
		if err != nil {
			return DetalheDoente{}, err
		}
		numProcesso = gerado
	}

	doente, err := dominio.NovoDoente(numProcesso, ident, contactos, dados.Nacionalidade)
	if err != nil {
		return DetalheDoente{}, err
	}
	if dados.GrupoSanguineo != nil {
		if g, err := grupoOpcional(*dados.GrupoSanguineo); err != nil {
			return DetalheDoente{}, err
		} else {
			doente.DefinirGrupoSanguineo(g)
		}
	}

	id, err := c.repo.Guardar(ctx, doente)
	if err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "clinico.doente.registado",
		Entidade:   "doente",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheDoente{}, err
	}

	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}
	return paraDetalhe(final), nil
}

// grupoOpcional converte uma string em *GrupoSanguineo; string vazia → nil.
func grupoOpcional(valor string) (*dominio.GrupoSanguineo, error) {
	if strings.TrimSpace(valor) == "" {
		return nil, nil
	}
	g, err := dominio.ParseGrupoSanguineo(valor)
	if err != nil {
		return nil, err
	}
	return &g, nil
}
```

- [ ] **Step 6: Correr os testes e confirmar que passam**

Run: `go test ./internal/application/clinico/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/application/clinico/
git commit -m "feat(clinico): portas, DTOs e caso de uso de registo de doente

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Casos de uso Obter e Pesquisar Doente

**Files:**
- Create: `internal/application/clinico/obter_doente.go`
- Create: `internal/application/clinico/pesquisar_doentes.go`
- Test: `internal/application/clinico/obter_pesquisar_test.go`

**Interfaces:**
- Consumes: `RepositorioDoentes`, `Auditor`, `paraDetalhe`, `FiltroDoentes`, `PaginaDoentes` (Task 6).
- Produces:
  - `CasoObterDoente` com `NovoCasoObterDoente(repo, aud) *CasoObterDoente` e `Executar(ctx, actor, id string) (DetalheDoente, error)`.
  - `CasoPesquisarDoentes` com `NovoCasoPesquisarDoentes(repo) *CasoPesquisarDoentes` e `Executar(ctx, filtro FiltroDoentes) (PaginaDoentes, error)`.
  - Constantes `limiteDefault = 20`, `limiteMaximo = 100`.

**Regras:** Obter audita `clinico.doente.consultado` (acesso a dados de saúde). Pesquisar aplica limite por omissão (20) e máximo (100); não audita (evita ruído em listagens).

- [ ] **Step 1: Escrever o teste que falha (`obter_pesquisar_test.go`)**

```go
package clinico_test

import (
	"context"
	"testing"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func registarNoRepo(t *testing.T, repo *fakeRepo) string {
	t.Helper()
	caso := appclinico.NovoCasoRegistarDoente(repo, &fakeAuditor{})
	out, err := caso.Executar(context.Background(), "sys", dadosBase())
	if err != nil {
		t.Fatalf("preparar doente: %v", err)
	}
	return out.ID
}

func TestObterDoente_Audita(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoObterDoente(repo, aud)

	out, err := caso.Executar(context.Background(), "actor-1", id)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if out.ID != id {
		t.Fatalf("id inesperado: %q", out.ID)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.consultado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestObterDoente_NaoEncontrado(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoObterDoente(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "actor-1", "inexistente")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestPesquisarDoentes_AplicaLimiteDefault(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoPesquisarDoentes(repo)
	if _, err := caso.Executar(context.Background(), appclinico.FiltroDoentes{Termo: "ana"}); err != nil {
		t.Fatalf("pesquisar: %v", err)
	}
	if repo.ultimoFilt.Limite != 20 {
		t.Fatalf("limite default=%d, esperava 20", repo.ultimoFilt.Limite)
	}
}

func TestPesquisarDoentes_LimiteMaximo(t *testing.T) {
	repo := novoFakeRepo()
	caso := appclinico.NovoCasoPesquisarDoentes(repo)
	if _, err := caso.Executar(context.Background(), appclinico.FiltroDoentes{Limite: 5000}); err != nil {
		t.Fatalf("pesquisar: %v", err)
	}
	if repo.ultimoFilt.Limite != 100 {
		t.Fatalf("limite máximo=%d, esperava 100", repo.ultimoFilt.Limite)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/clinico/ -run 'Obter|Pesquisar' -v`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Implementar `obter_doente.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoObterDoente devolve o detalhe de um doente e audita o acesso (dados de
// saúde são de acesso auditável).
type CasoObterDoente struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoObterDoente constrói o caso de uso.
func NovoCasoObterDoente(repo dominio.RepositorioDoentes, aud Auditor) *CasoObterDoente {
	return &CasoObterDoente{repo: repo, auditor: aud, agora: time.Now}
}

// Executar carrega o doente por id, audita a consulta e devolve o detalhe.
func (c *CasoObterDoente) Executar(ctx context.Context, actor, id string) (DetalheDoente, error) {
	doente, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "clinico.doente.consultado",
		Entidade:   "doente",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheDoente{}, err
	}
	return paraDetalhe(doente), nil
}
```

- [ ] **Step 4: Implementar `pesquisar_doentes.go`**

```go
package clinico

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

const (
	limiteDefault = 20
	limiteMaximo  = 100
)

// CasoPesquisarDoentes pesquisa doentes por nome (fuzzy), BI, número de processo
// ou telefone, com paginação.
type CasoPesquisarDoentes struct {
	repo dominio.RepositorioDoentes
}

// NovoCasoPesquisarDoentes constrói o caso de uso.
func NovoCasoPesquisarDoentes(repo dominio.RepositorioDoentes) *CasoPesquisarDoentes {
	return &CasoPesquisarDoentes{repo: repo}
}

// Executar normaliza os limites e delega a pesquisa ao repositório.
func (c *CasoPesquisarDoentes) Executar(ctx context.Context, filtro FiltroDoentes) (PaginaDoentes, error) {
	if filtro.Limite <= 0 {
		filtro.Limite = limiteDefault
	}
	if filtro.Limite > limiteMaximo {
		filtro.Limite = limiteMaximo
	}
	if filtro.Deslocamento < 0 {
		filtro.Deslocamento = 0
	}
	return c.repo.Pesquisar(ctx, filtro)
}
```

- [ ] **Step 5: Correr os testes e confirmar que passam**

Run: `go test ./internal/application/clinico/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/application/clinico/obter_doente.go internal/application/clinico/pesquisar_doentes.go internal/application/clinico/obter_pesquisar_test.go
git commit -m "feat(clinico): casos de uso obter (auditado) e pesquisar doentes

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Casos de uso Actualizar e Gerir Estado

**Files:**
- Create: `internal/application/clinico/actualizar_doente.go`
- Create: `internal/application/clinico/gerir_estado_doente.go`
- Test: `internal/application/clinico/actualizar_estado_test.go`

**Interfaces:**
- Consumes: `RepositorioDoentes`, `Auditor`, `construirIdentificacao`, `construirContactos`, `grupoOpcional`, `paraDetalhe` (Tasks 6).
- Produces:
  - `CasoActualizarDoente` com `NovoCasoActualizarDoente(repo, aud)` e `Executar(ctx, actor, id string, dados DadosActualizarDoente) (DetalheDoente, error)`.
  - `CasoGerirEstadoDoente` com `NovoCasoGerirEstadoDoente(repo, aud)`, `Desactivar(ctx, actor, id, motivo string) (DetalheDoente, error)` e `DeclararFalecido(ctx, actor, id string, data time.Time, causaCID string) (DetalheDoente, error)`.

**Regras:** cada método hidrata (`ObterPorID`), aplica a transição de domínio, `Guardar`, audita (`clinico.doente.actualizado` / `.desactivado` / `.falecido`), re-lê e devolve o detalhe. Actualizar: `Identificacao`/`Contactos`/`GrupoSanguineo` a nil ficam inalterados; `GrupoSanguineo` presente com "" limpa.

- [ ] **Step 1: Escrever o teste que falha (`actualizar_estado_test.go`)**

```go
package clinico_test

import (
	"context"
	"testing"
	"time"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestActualizarDoente_Contactos(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoActualizarDoente(repo, aud)

	novoTel := "+244912000000"
	out, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{
		Contactos: &appclinico.DadosContactos{Telefone: novoTel},
	})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.Telefone != "+244 912 000 000" {
		t.Fatalf("telefone não actualizado: %q", out.Telefone)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.actualizado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestActualizarDoente_GrupoSanguineo(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoActualizarDoente(repo, &fakeAuditor{})
	g := "O+"
	out, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{GrupoSanguineo: &g})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.GrupoSanguineo == nil || *out.GrupoSanguineo != "O+" {
		t.Fatalf("grupo sanguíneo não definido: %v", out.GrupoSanguineo)
	}
}

func TestGerirEstado_Desactivar(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, aud)

	out, err := caso.Desactivar(context.Background(), "actor-1", id, "dados duplicados")
	if err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	if out.Estado != "INACTIVO" {
		t.Fatalf("estado=%q, esperava INACTIVO", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.desactivado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestGerirEstado_DesactivarSemMotivo(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, &fakeAuditor{})
	_, err := caso.Desactivar(context.Background(), "actor-1", id, "  ")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestGerirEstado_DeclararFalecido(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, aud)

	data := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	out, err := caso.DeclararFalecido(context.Background(), "actor-1", id, data, "I21")
	if err != nil {
		t.Fatalf("declarar falecido: %v", err)
	}
	if out.Estado != "FALECIDO" {
		t.Fatalf("estado=%q, esperava FALECIDO", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.falecido" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/clinico/ -run 'Actualizar|GerirEstado' -v`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Implementar `actualizar_doente.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoActualizarDoente actualiza identificação, contactos e/ou grupo sanguíneo de
// um doente e audita a operação.
type CasoActualizarDoente struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoActualizarDoente constrói o caso de uso.
func NovoCasoActualizarDoente(repo dominio.RepositorioDoentes, aud Auditor) *CasoActualizarDoente {
	return &CasoActualizarDoente{repo: repo, auditor: aud, agora: time.Now}
}

// Executar aplica as alterações fornecidas (campos a nil ficam inalterados).
func (c *CasoActualizarDoente) Executar(ctx context.Context, actor, id string, dados DadosActualizarDoente) (DetalheDoente, error) {
	doente, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}

	if dados.Identificacao != nil {
		ident, err := construirIdentificacao(*dados.Identificacao)
		if err != nil {
			return DetalheDoente{}, err
		}
		if err := doente.AtualizarIdentificacao(ident); err != nil {
			return DetalheDoente{}, err
		}
	}
	if dados.Contactos != nil {
		contactos, err := construirContactos(*dados.Contactos)
		if err != nil {
			return DetalheDoente{}, err
		}
		if err := doente.AtualizarContactos(contactos); err != nil {
			return DetalheDoente{}, err
		}
	}
	if dados.GrupoSanguineo != nil {
		g, err := grupoOpcional(*dados.GrupoSanguineo)
		if err != nil {
			return DetalheDoente{}, err
		}
		doente.DefinirGrupoSanguineo(g)
	}

	if _, err := c.repo.Guardar(ctx, doente); err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.doente.actualizado",
		Entidade: "doente", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheDoente{}, err
	}

	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}
	return paraDetalhe(final), nil
}
```

- [ ] **Step 4: Implementar `gerir_estado_doente.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoGerirEstadoDoente aplica transições de ciclo de vida (desactivação, óbito)
// e audita cada operação.
type CasoGerirEstadoDoente struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoGerirEstadoDoente constrói o caso de uso.
func NovoCasoGerirEstadoDoente(repo dominio.RepositorioDoentes, aud Auditor) *CasoGerirEstadoDoente {
	return &CasoGerirEstadoDoente{repo: repo, auditor: aud, agora: time.Now}
}

// Desactivar coloca o doente em INACTIVO com um motivo.
func (c *CasoGerirEstadoDoente) Desactivar(ctx context.Context, actor, id, motivo string) (DetalheDoente, error) {
	return c.transicionar(ctx, actor, id, "clinico.doente.desactivado", func(d *dominio.Doente) error {
		return d.Desactivar(motivo, c.agora())
	})
}

// DeclararFalecido coloca o doente em FALECIDO com a data de óbito e a causa.
func (c *CasoGerirEstadoDoente) DeclararFalecido(ctx context.Context, actor, id string, data time.Time, causaCID string) (DetalheDoente, error) {
	return c.transicionar(ctx, actor, id, "clinico.doente.falecido", func(d *dominio.Doente) error {
		return d.DeclararFalecido(data, causaCID)
	})
}

// transicionar hidrata o doente, aplica a transição, persiste, audita e devolve o
// detalhe actualizado.
func (c *CasoGerirEstadoDoente) transicionar(ctx context.Context, actor, id, accao string, aplicar func(*dominio.Doente) error) (DetalheDoente, error) {
	doente, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}
	if err := aplicar(doente); err != nil {
		return DetalheDoente{}, err
	}
	if _, err := c.repo.Guardar(ctx, doente); err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: accao, Entidade: "doente", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheDoente{}, err
	}
	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}
	return paraDetalhe(final), nil
}
```

- [ ] **Step 5: Correr os testes e confirmar que passam**

Run: `go test ./internal/application/clinico/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/application/clinico/actualizar_doente.go internal/application/clinico/gerir_estado_doente.go internal/application/clinico/actualizar_estado_test.go
git commit -m "feat(clinico): casos de uso de actualização e gestão de estado do doente

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Casos de uso Registar Alergia e Registar Antecedente

**Files:**
- Create: `internal/application/clinico/registar_alergia.go`
- Create: `internal/application/clinico/registar_antecedente.go`
- Test: `internal/application/clinico/alergia_antecedente_test.go`

**Interfaces:**
- Consumes: `RepositorioDoentes`, `Auditor`, `paraDetalhe` (Task 6); domínio `NovaAlergia`, `NovoAntecedente`, `ParseSeveridade`, `ParseTipoAntecedente`.
- Produces:
  - `CasoRegistarAlergia` com `NovoCasoRegistarAlergia(repo, aud)` e `Executar(ctx, actor, doenteID string, dados DadosAlergia) (DetalheDoente, error)`.
  - `CasoRegistarAntecedente` com `NovoCasoRegistarAntecedente(repo, aud)` e `Executar(ctx, actor, doenteID string, dados DadosAntecedente) (DetalheDoente, error)`.

**Regras:** hidrata, constrói o VO (validando severidade/tipo), `AdicionarAlergia`/`AdicionarAntecedente`, `Guardar`, audita (`clinico.alergia.registada` / `clinico.antecedente.registado`), re-lê e devolve o detalhe.

- [ ] **Step 1: Escrever o teste que falha (`alergia_antecedente_test.go`)**

```go
package clinico_test

import (
	"context"
	"testing"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRegistarAlergia(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoRegistarAlergia(repo, aud)

	out, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAlergia{
		Substancia: "Penicilina", Severidade: "GRAVE",
	})
	if err != nil {
		t.Fatalf("registar alergia: %v", err)
	}
	if len(out.Alergias) != 1 || out.Alergias[0].Substancia != "Penicilina" {
		t.Fatalf("alergia não registada: %+v", out.Alergias)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.alergia.registada" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestRegistarAlergia_SeveridadeInvalida(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoRegistarAlergia(repo, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAlergia{
		Substancia: "X", Severidade: "EXTREMA",
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestRegistarAntecedente(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoRegistarAntecedente(repo, aud)

	out, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosAntecedente{
		Tipo: "PESSOAL", Descricao: "Hipertensão", CID: "I10", Activo: true,
	})
	if err != nil {
		t.Fatalf("registar antecedente: %v", err)
	}
	if len(out.Antecedentes) != 1 || out.Antecedentes[0].Descricao != "Hipertensão" {
		t.Fatalf("antecedente não registado: %+v", out.Antecedentes)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.antecedente.registado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}
```

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/application/clinico/ -run 'RegistarAlergia|RegistarAntecedente' -v`
Expected: FAIL — símbolos inexistentes.

- [ ] **Step 3: Implementar `registar_alergia.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarAlergia acrescenta uma alergia a um doente e audita a operação.
type CasoRegistarAlergia struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoRegistarAlergia constrói o caso de uso.
func NovoCasoRegistarAlergia(repo dominio.RepositorioDoentes, aud Auditor) *CasoRegistarAlergia {
	return &CasoRegistarAlergia{repo: repo, auditor: aud, agora: time.Now}
}

// Executar valida e regista a alergia no doente indicado.
func (c *CasoRegistarAlergia) Executar(ctx context.Context, actor, doenteID string, dados DadosAlergia) (DetalheDoente, error) {
	doente, err := c.repo.ObterPorID(ctx, doenteID)
	if err != nil {
		return DetalheDoente{}, err
	}
	sev, err := dominio.ParseSeveridade(dados.Severidade)
	if err != nil {
		return DetalheDoente{}, err
	}
	alergia, err := dominio.NovaAlergia(dados.Substancia, sev, dados.ReaccaoTipica, dados.ConfirmadaEm, dados.Notas)
	if err != nil {
		return DetalheDoente{}, err
	}
	if err := doente.AdicionarAlergia(alergia); err != nil {
		return DetalheDoente{}, err
	}
	if _, err := c.repo.Guardar(ctx, doente); err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.alergia.registada",
		Entidade: "doente", EntidadeID: doenteID, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheDoente{}, err
	}
	final, err := c.repo.ObterPorID(ctx, doenteID)
	if err != nil {
		return DetalheDoente{}, err
	}
	return paraDetalhe(final), nil
}
```

- [ ] **Step 4: Implementar `registar_antecedente.go`**

```go
package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarAntecedente acrescenta um antecedente clínico a um doente e audita.
type CasoRegistarAntecedente struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoRegistarAntecedente constrói o caso de uso.
func NovoCasoRegistarAntecedente(repo dominio.RepositorioDoentes, aud Auditor) *CasoRegistarAntecedente {
	return &CasoRegistarAntecedente{repo: repo, auditor: aud, agora: time.Now}
}

// Executar valida e regista o antecedente no doente indicado.
func (c *CasoRegistarAntecedente) Executar(ctx context.Context, actor, doenteID string, dados DadosAntecedente) (DetalheDoente, error) {
	doente, err := c.repo.ObterPorID(ctx, doenteID)
	if err != nil {
		return DetalheDoente{}, err
	}
	tipo, err := dominio.ParseTipoAntecedente(dados.Tipo)
	if err != nil {
		return DetalheDoente{}, err
	}
	antecedente, err := dominio.NovoAntecedente(tipo, dados.Descricao, dados.CID, dados.DataInicio, dados.Activo, dados.Notas)
	if err != nil {
		return DetalheDoente{}, err
	}
	if err := doente.AdicionarAntecedente(antecedente); err != nil {
		return DetalheDoente{}, err
	}
	if _, err := c.repo.Guardar(ctx, doente); err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.antecedente.registado",
		Entidade: "doente", EntidadeID: doenteID, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheDoente{}, err
	}
	final, err := c.repo.ObterPorID(ctx, doenteID)
	if err != nil {
		return DetalheDoente{}, err
	}
	return paraDetalhe(final), nil
}
```

- [ ] **Step 5: Correr os testes e a cobertura da aplicação**

Run: `go test ./internal/application/clinico/ -v`
Expected: PASS.
Run: `bash scripts/cobertura.sh` (secção aplicação) — Expected: aplicação ≥75%.

- [ ] **Step 6: Commit**

```bash
git add internal/application/clinico/registar_alergia.go internal/application/clinico/registar_antecedente.go internal/application/clinico/alergia_antecedente_test.go
git commit -m "feat(clinico): casos de uso de registo de alergia e antecedente clínico

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Repositório PostgreSQL de doentes e teste de integração

**Files:**
- Create: `internal/adapters/pgrepo/doentes_repo.go`
- Test: `tests/integration/doentes_test.go` (tag `integration`)

**Interfaces:**
- Consumes: domínio `clinico` (agregado, `SnapshotDoente`, `ReconstruirDoente`, `FiltroDoentes`, `PaginaDoentes`, `ResumoDoente`, VOs); `erros`; `pgx`/`pgxpool`.
- Produces: `RepositorioDoentes` com `NovoRepositorioDoentes(pool *pgxpool.Pool) *RepositorioDoentes`, implementando `clinico.RepositorioDoentes`.

**Contexto/padrão:** seguir `internal/adapters/pgrepo/identidade_repo.go` (transacção com defer rollback, `errors.Is(err, pgx.ErrNoRows)` → `CategoriaNaoEncontrado`). A persistência dos filhos faz-se por delete-and-reinsert dentro da transacção do `Guardar`. Conflito de unicidade (`23505`) → `CategoriaConflito`. O gate de cobertura corre sem a tag `integration`, por isso este repo fica coberto **apenas** pelo teste de integração (como `identidade_repo`).

- [ ] **Step 1: Implementar `doentes_repo.go`**

```go
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioDoentes implementa dominio.RepositorioDoentes com pgx.
type RepositorioDoentes struct {
	pool *pgxpool.Pool
}

// NovoRepositorioDoentes constrói o repositório sobre o pool pgx.
func NovoRepositorioDoentes(pool *pgxpool.Pool) *RepositorioDoentes {
	return &RepositorioDoentes{pool: pool}
}

// ProximoNumeroProcesso reserva atomicamente o próximo sequencial do ano e
// formata "P-{ano}-{sequencial:06d}".
func (r *RepositorioDoentes) ProximoNumeroProcesso(ctx context.Context, ano int) (string, error) {
	const q = `
INSERT INTO clinico.processo_sequencia (ano, ultimo) VALUES ($1, 1)
ON CONFLICT (ano) DO UPDATE SET ultimo = clinico.processo_sequencia.ultimo + 1
RETURNING ultimo`
	var ultimo int
	if err := r.pool.QueryRow(ctx, q, ano).Scan(&ultimo); err != nil {
		return "", fmt.Errorf("reservar número de processo: %w", err)
	}
	return fmt.Sprintf("P-%d-%06d", ano, ultimo), nil
}

// Guardar persiste o doente (INSERT se id vazio, senão UPDATE) e os seus filhos,
// numa única transacção. Conflitos de unicidade → CategoriaConflito.
func (r *RepositorioDoentes) Guardar(ctx context.Context, d *dominio.Doente) (string, error) {
	s := d.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := s.ID
	if id == "" {
		id, err = r.inserir(ctx, tx, s)
	} else {
		err = r.actualizar(ctx, tx, s)
	}
	if err != nil {
		return "", traduzErroUnicidade(err)
	}

	if err := r.guardarFilhos(ctx, tx, id, s); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar transacção: %w", err)
	}
	return id, nil
}

func (r *RepositorioDoentes) inserir(ctx context.Context, tx pgx.Tx, s dominio.SnapshotDoente) (string, error) {
	const q = `
INSERT INTO clinico.doentes (
    num_processo, nome_completo, data_nascimento, sexo, bi, nif, passaporte,
    nacionalidade, telefone, email,
    morada_provincia, morada_municipio, morada_comuna, morada_bairro, morada_rua, morada_casa, morada_referencia,
    grupo_sanguineo, estado, falecido_em, causa_morte_cid, desactivado_em, desactivado_motivo
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,NULLIF($21,''),$22,NULLIF($23,'')
) RETURNING id::text`
	mp, mm, mc, mb, mr, mca, mref := desmontarMorada(s)
	var id string
	err := tx.QueryRow(ctx, q,
		s.NumProcesso, s.Identificacao.NomeCompleto, s.Identificacao.DataNascimento, string(s.Identificacao.Sexo),
		s.Identificacao.BI, s.Identificacao.NIF, s.Identificacao.Passaporte,
		s.Nacionalidade, s.Contactos.Telefone, s.Contactos.Email,
		mp, mm, mc, mb, mr, mca, mref,
		grupoTexto(s), string(s.Estado), s.FalecidoEm, s.CausaMorteCID, s.DesactivadoEm, s.DesactivadoMotivo,
	).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (r *RepositorioDoentes) actualizar(ctx context.Context, tx pgx.Tx, s dominio.SnapshotDoente) error {
	const q = `
UPDATE clinico.doentes SET
    num_processo=$2, nome_completo=$3, data_nascimento=$4, sexo=$5, bi=$6, nif=$7, passaporte=$8,
    nacionalidade=$9, telefone=$10, email=$11,
    morada_provincia=$12, morada_municipio=$13, morada_comuna=$14, morada_bairro=$15, morada_rua=$16, morada_casa=$17, morada_referencia=$18,
    grupo_sanguineo=NULLIF($19,''), estado=$20, falecido_em=$21, causa_morte_cid=NULLIF($22,''),
    desactivado_em=$23, desactivado_motivo=NULLIF($24,''), actualizado_em=now()
WHERE id=$1`
	mp, mm, mc, mb, mr, mca, mref := desmontarMorada(s)
	ct, err := tx.Exec(ctx, q, s.ID,
		s.NumProcesso, s.Identificacao.NomeCompleto, s.Identificacao.DataNascimento, string(s.Identificacao.Sexo),
		s.Identificacao.BI, s.Identificacao.NIF, s.Identificacao.Passaporte,
		s.Nacionalidade, s.Contactos.Telefone, s.Contactos.Email,
		mp, mm, mc, mb, mr, mca, mref,
		grupoTexto(s), string(s.Estado), s.FalecidoEm, s.CausaMorteCID, s.DesactivadoEm, s.DesactivadoMotivo,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
	}
	return nil
}

// guardarFilhos substitui alergias e antecedentes por delete-and-reinsert.
func (r *RepositorioDoentes) guardarFilhos(ctx context.Context, tx pgx.Tx, id string, s dominio.SnapshotDoente) error {
	if _, err := tx.Exec(ctx, `DELETE FROM clinico.alergias WHERE doente_id=$1`, id); err != nil {
		return fmt.Errorf("limpar alergias: %w", err)
	}
	for _, a := range s.Alergias {
		if _, err := tx.Exec(ctx,
			`INSERT INTO clinico.alergias (doente_id, substancia, severidade, reaccao_tipica, confirmada_em, notas)
			 VALUES ($1,$2,$3,NULLIF($4,''),$5,NULLIF($6,''))`,
			id, a.Substancia, string(a.Severidade), a.ReaccaoTipica, a.ConfirmadaEm, a.Notas); err != nil {
			return fmt.Errorf("inserir alergia: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM clinico.antecedentes_clinicos WHERE doente_id=$1`, id); err != nil {
		return fmt.Errorf("limpar antecedentes: %w", err)
	}
	for _, a := range s.Antecedentes {
		if _, err := tx.Exec(ctx,
			`INSERT INTO clinico.antecedentes_clinicos (doente_id, tipo, descricao, cid, data_inicio, activo, notas)
			 VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,NULLIF($7,''))`,
			id, string(a.Tipo), a.Descricao, a.CID, a.DataInicio, a.Activo, a.Notas); err != nil {
			return fmt.Errorf("inserir antecedente: %w", err)
		}
	}
	return nil
}

// ObterPorID devolve o doente com os filhos. NaoEncontrado se não existir.
func (r *RepositorioDoentes) ObterPorID(ctx context.Context, id string) (*dominio.Doente, error) {
	return r.obter(ctx, `id=$1`, id)
}

// ObterPorNumProcesso devolve o doente pelo número de processo.
func (r *RepositorioDoentes) ObterPorNumProcesso(ctx context.Context, num string) (*dominio.Doente, error) {
	return r.obter(ctx, `num_processo=$1`, num)
}

func (r *RepositorioDoentes) obter(ctx context.Context, cond string, arg any) (*dominio.Doente, error) {
	q := `
SELECT id::text, num_processo, nome_completo, data_nascimento, sexo, bi, nif, passaporte,
       nacionalidade, telefone, email,
       morada_provincia, morada_municipio, morada_comuna, morada_bairro, morada_rua, morada_casa, morada_referencia,
       grupo_sanguineo, estado, falecido_em, causa_morte_cid, criado_em, actualizado_em,
       desactivado_em, desactivado_motivo
FROM clinico.doentes WHERE ` + cond
	var s dominio.SnapshotDoente
	var sexo, estado string
	var grupo, motivo *string
	if err := r.pool.QueryRow(ctx, q, arg).Scan(
		&s.ID, &s.NumProcesso, &s.Identificacao.NomeCompleto, &s.Identificacao.DataNascimento, &sexo,
		&s.Identificacao.BI, &s.Identificacao.NIF, &s.Identificacao.Passaporte,
		&s.Nacionalidade, &s.Contactos.Telefone, &s.Contactos.Email,
		&monta.p, &monta.m, &monta.c, &monta.b, &monta.r, &monta.casa, &monta.ref,
		&grupo, &estado, &s.FalecidoEm, &s.CausaMorteCID, &s.CriadoEm, &s.ActualizadoEm,
		&s.DesactivadoEm, &motivo,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
		}
		return nil, fmt.Errorf("obter doente: %w", err)
	}
	// NOTA AO IMPLEMENTADOR: `monta` acima é um placeholder para os 7 destinos da
	// morada — declare antes do Scan sete variáveis `*string` (mp, mm, mc, mb, mr,
	// mca, mref) e passe os seus endereços; depois monte `s.Contactos.Morada` só se
	// pelo menos `mp` (província) não for nil. Substitua `&monta.*` por `&mp` etc.
	s.Identificacao.Sexo = dominio.Sexo(sexo)
	s.Estado = dominio.EstadoDoente(estado)
	if grupo != nil {
		g := dominio.GrupoSanguineo(*grupo)
		s.GrupoSanguineo = &g
	}
	if motivo != nil {
		s.DesactivadoMotivo = *motivo
	}
	// montar morada (ver nota) → s.Contactos.Morada
	filhos, err := r.carregarFilhos(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	s.Alergias, s.Antecedentes = filhos.alergias, filhos.antecedentes
	return dominio.ReconstruirDoente(s), nil
}
```

> **Nota importante ao implementador (leitura da morada):** o bloco de `Scan` acima
> usa um pseudo-`monta.*` só para encurtar o plano. Na implementação real:
> 1. Declare, antes do `Scan`, sete `var mp, mm, mc, mb, mr, mca, mref *string`.
> 2. Passe `&mp, &mm, &mc, &mb, &mr, &mca, &mref` nas posições da morada.
> 3. Depois do `Scan`, se `mp != nil` (há morada), construa
>    `s.Contactos.Morada = &dominio.Morada{Provincia: deref(mp), Municipio: deref(mm), Comuna: deref(mc), Bairro: deref(mb), Rua: deref(mr), Casa: mca, Referencia: mref}`
>    com um helper `deref(*string) string` (nil → "").
> A morada é opcional; todos os campos podem ser NULL na BD.

- [ ] **Step 2: Implementar os helpers e a pesquisa (no mesmo ficheiro)**

```go
type destinoFilhos struct {
	alergias     []dominio.Alergia
	antecedentes []dominio.AntecedenteClinico
}

func (r *RepositorioDoentes) carregarFilhos(ctx context.Context, id string) (destinoFilhos, error) {
	var out destinoFilhos
	linhasA, err := r.pool.Query(ctx,
		`SELECT substancia, severidade, COALESCE(reaccao_tipica,''), confirmada_em, COALESCE(notas,'')
		 FROM clinico.alergias WHERE doente_id=$1 ORDER BY criada_em`, id)
	if err != nil {
		return out, fmt.Errorf("carregar alergias: %w", err)
	}
	defer linhasA.Close()
	for linhasA.Next() {
		var a dominio.Alergia
		var sev string
		if err := linhasA.Scan(&a.Substancia, &sev, &a.ReaccaoTipica, &a.ConfirmadaEm, &a.Notas); err != nil {
			return out, fmt.Errorf("ler alergia: %w", err)
		}
		a.Severidade = dominio.Severidade(sev)
		out.alergias = append(out.alergias, a)
	}
	if err := linhasA.Err(); err != nil {
		return out, err
	}
	linhasAnt, err := r.pool.Query(ctx,
		`SELECT tipo, descricao, COALESCE(cid,''), data_inicio, activo, COALESCE(notas,'')
		 FROM clinico.antecedentes_clinicos WHERE doente_id=$1 ORDER BY criado_em`, id)
	if err != nil {
		return out, fmt.Errorf("carregar antecedentes: %w", err)
	}
	defer linhasAnt.Close()
	for linhasAnt.Next() {
		var a dominio.AntecedenteClinico
		var tipo string
		if err := linhasAnt.Scan(&tipo, &a.Descricao, &a.CID, &a.DataInicio, &a.Activo, &a.Notas); err != nil {
			return out, fmt.Errorf("ler antecedente: %w", err)
		}
		a.Tipo = dominio.TipoAntecedente(tipo)
		out.antecedentes = append(out.antecedentes, a)
	}
	return out, linhasAnt.Err()
}

// Pesquisar devolve uma página de doentes. Nome via trigram (ILIKE); BI, número
// de processo e telefone por igualdade. Filtro de estado opcional.
func (r *RepositorioDoentes) Pesquisar(ctx context.Context, f dominio.FiltroDoentes) (dominio.PaginaDoentes, error) {
	base := `FROM clinico.doentes WHERE ($1='' OR nome_completo ILIKE '%'||$1||'%' OR bi=$1 OR num_processo=$1 OR telefone=$1) AND ($2='' OR estado=$2)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+base, f.Termo, f.Estado).Scan(&total); err != nil {
		return dominio.PaginaDoentes{}, fmt.Errorf("contar doentes: %w", err)
	}
	q := `SELECT id::text, num_processo, nome_completo, data_nascimento, sexo, telefone, estado ` +
		base + ` ORDER BY nome_completo LIMIT $3 OFFSET $4`
	linhas, err := r.pool.Query(ctx, q, f.Termo, f.Estado, f.Limite, f.Deslocamento)
	if err != nil {
		return dominio.PaginaDoentes{}, fmt.Errorf("pesquisar doentes: %w", err)
	}
	defer linhas.Close()
	pagina := dominio.PaginaDoentes{Total: total, Limite: f.Limite, Deslocamento: f.Deslocamento, Itens: []dominio.ResumoDoente{}}
	for linhas.Next() {
		var it dominio.ResumoDoente
		if err := linhas.Scan(&it.ID, &it.NumProcesso, &it.NomeCompleto, &it.DataNascimento, &it.Sexo, &it.Telefone, &it.Estado); err != nil {
			return dominio.PaginaDoentes{}, fmt.Errorf("ler resumo: %w", err)
		}
		pagina.Itens = append(pagina.Itens, it)
	}
	return pagina, linhas.Err()
}

// desmontarMorada devolve os sete campos da morada (nil se ausente).
func desmontarMorada(s dominio.SnapshotDoente) (mp, mm, mc, mb, mr, mca, mref *string) {
	if s.Contactos.Morada == nil {
		return nil, nil, nil, nil, nil, nil, nil
	}
	m := s.Contactos.Morada
	return &m.Provincia, &m.Municipio, &m.Comuna, &m.Bairro, &m.Rua, m.Casa, m.Referencia
}

// grupoTexto devolve o grupo sanguíneo como texto ("" se ausente → NULLIF na SQL).
func grupoTexto(s dominio.SnapshotDoente) string {
	if s.GrupoSanguineo == nil {
		return ""
	}
	return s.GrupoSanguineo.String()
}

// traduzErroUnicidade mapeia a violação de unicidade do Postgres (23505) para um
// erro de domínio de conflito; os restantes erros passam inalterados.
func traduzErroUnicidade(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return erros.Novo(erros.CategoriaConflito, "já existe um doente com este número de processo ou Bilhete de Identidade")
	}
	return fmt.Errorf("guardar doente: %w", err)
}
```

> **Nota ao implementador:** `traduzErroUnicidade` importa `github.com/jackc/pgx/v5/pgconn`. Acrescente-o ao bloco de imports. Se a província for `""` na morada (campo obrigatório do VO Morada quando presente), a leitura considera a morada presente — garanta que o handler só envia morada com província preenchida, ou ajuste a nota de leitura para tratar todos-nil como ausência de morada.

- [ ] **Step 3: Escrever o teste de integração `tests/integration/doentes_test.go`**

```go
//go:build integration

// Testes de integração do BC Clínico (agregado Doente) contra a BD real. Seguem o
// padrão de ciclo_vida_test.go: SKIP (nunca FAIL) quando DATABASE_URL não está
// definido.
package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRepositorioDoentes_CicloCompleto(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	aplicarMigracoesTeste(t, pool, ctx)
	repo := pgrepo.NovoRepositorioDoentes(pool)

	// Número automático.
	num, err := repo.ProximoNumeroProcesso(ctx, 2026)
	if err != nil {
		t.Fatalf("próximo número: %v", err)
	}
	if num[:2] != "P-" {
		t.Fatalf("formato inesperado: %q", num)
	}

	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	bi := "00123456LA042"
	ident, _ := dominio.NovaIdentificacao("Ana Integração", nasc, dominio.SexoFeminino, &bi, nil, nil)
	ct, _ := dominio.NovosContactos("+244923456789", nil, nil)
	doente, _ := dominio.NovoDoente(num, ident, ct, "AO")

	id, err := repo.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, id) })

	// Pesquisa por parte do nome (trigram).
	pag, err := repo.Pesquisar(ctx, dominio.FiltroDoentes{Termo: "Integr", Limite: 10})
	if err != nil {
		t.Fatalf("pesquisar: %v", err)
	}
	if pag.Total < 1 {
		t.Fatalf("esperava >=1 resultado, obtive %d", pag.Total)
	}

	// Adicionar alergia e persistir.
	alergia, _ := dominio.NovaAlergia("Penicilina", dominio.SeveridadeGrave, "", nil, "")
	_ = doente.AdicionarAlergia(alergia)
	if _, err := repo.Guardar(ctx, dominio.ReconstruirDoente(comID(doente, id))); err != nil {
		t.Fatalf("guardar com alergia: %v", err)
	}
	lido, err := repo.ObterPorID(ctx, id)
	if err != nil || len(lido.Snapshot().Alergias) != 1 {
		t.Fatalf("alergia não persistida: %v (alergias=%d)", err, len(lido.Snapshot().Alergias))
	}

	// Unicidade do número de processo.
	dup, _ := dominio.NovoDoente(num, ident, ct, "AO")
	if _, err := repo.Guardar(ctx, dup); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito de num de processo, obtive %v", err)
	}
}

// comID devolve um snapshot do doente com o id atribuído (o agregado em memória
// não conhece o id gerado pela BD).
func comID(d *dominio.Doente, id string) dominio.SnapshotDoente {
	s := d.Snapshot()
	s.ID = id
	return s
}
```

> **Nota ao implementador:** este teste usa dois helpers do pacote de integração:
> `ligar(t) (*pgxpool.Pool, context.Context)` (já existe — ver `sessoes_perfil_admin_test.go`) e um `aplicarMigracoesTeste(t, pool, ctx)`. Se este último ainda não existir como helper partilhado, extraia-o do padrão já usado em `TestEditarPerfilAdmin_ViaBD` (chama `db.AplicarMigracoes(ctx, pool, migrations.FS, logger)`), ou inline essas três linhas no início do teste. Não dupliques um `ligar` novo — reutiliza o existente.

- [ ] **Step 4: Compilar tudo e correr o teste de integração (com SKIP se sem BD)**

Run: `go build ./...`
Expected: sem erros.
Run: `go vet ./internal/adapters/pgrepo/`
Expected: limpo.
Run: `go test -tags integration ./tests/integration/ -run TestRepositorioDoentes -v`
Expected: PASS (com BD) ou SKIP (sem `DATABASE_URL`).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/pgrepo/doentes_repo.go tests/integration/doentes_test.go
git commit -m "feat(clinico): repositório pgx de doentes e teste de integração

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Handler HTTP de doentes com RBAC e testes unitários

**Files:**
- Create: `internal/adapters/http/doente_handler.go`
- Test: `internal/adapters/http/doente_test.go`

**Interfaces:**
- Consumes: casos de uso da aplicação `clinico` (Tasks 6-9) via interfaces de serviço locais; `SessaoDe`, `RBAC`, `responderErro`, `i18n`, `erros`; `dominio "internal/domain/identidade"` (para os papéis e `Sessao`).
- Produces: `DoentesHandler`, `NovoDoentesHandler(...)`, `RegistarDoentes(r gin.IRouter, h *DoentesHandler, protecao ...gin.HandlerFunc)`.

**Rotas e RBAC** (grupo `/api/v1/doentes`, protegido por `protecao...` = LimiteTaxa + Auth):

| Rota | Método | Papéis |
|---|---|---|
| `/api/v1/doentes` | POST | Administrativo, Médico, Enfermeiro |
| `/api/v1/doentes` | GET | leitura ampla* |
| `/api/v1/doentes/:id` | GET | leitura ampla* |
| `/api/v1/doentes/:id` | PATCH | Administrativo, Médico, Enfermeiro |
| `/api/v1/doentes/:id/estado` | POST | Administrativo, Médico, Enfermeiro |
| `/api/v1/doentes/:id/alergias` | POST | Médico, Enfermeiro |
| `/api/v1/doentes/:id/antecedentes` | POST | Médico, Enfermeiro |

\* leitura ampla = Médico, Enfermeiro, Administrativo, Farmacêutico, TecnicoLab, Director, DPO, Auditor.

**Datas:** os corpos JSON transportam datas como `"2006-01-02"` (string); o handler converte para `time.Time` com `time.Parse("2006-01-02", ...)` antes de chamar a aplicação. `data_nascimento` é obrigatória no registo.

- [ ] **Step 1: Escrever o teste que falha (`doente_test.go`)**

```go
package http_test

import (
	nethttp "net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes dos serviços de doentes ---

type fakeRegistarDoente struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeRegistarDoente) Executar(_ ctxT, _ string, _ appclinico.DadosNovoDoente) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

type fakeObterDoente struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeObterDoente) Executar(_ ctxT, _, _ string) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

type fakePesquisarDoentes struct {
	out appclinico.PaginaDoentes
	err error
}

func (f fakePesquisarDoentes) Executar(_ ctxT, _ appclinico.FiltroDoentes) (appclinico.PaginaDoentes, error) {
	return f.out, f.err
}

type fakeActualizarDoente struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeActualizarDoente) Executar(_ ctxT, _, _ string, _ appclinico.DadosActualizarDoente) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

type fakeGerirEstado struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeGerirEstado) Desactivar(_ ctxT, _, _, _ string) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}
func (f fakeGerirEstado) DeclararFalecido(_ ctxT, _, _ string, _ timeT, _ string) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

type fakeRegistarAlergia struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeRegistarAlergia) Executar(_ ctxT, _, _ string, _ appclinico.DadosAlergia) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

type fakeRegistarAntecedente struct {
	out appclinico.DetalheDoente
	err error
}

func (f fakeRegistarAntecedente) Executar(_ ctxT, _, _ string, _ appclinico.DadosAntecedente) (appclinico.DetalheDoente, error) {
	return f.out, f.err
}

func routerDoentes(sessao dominio.Sessao) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoDoentesHandler(
		fakeRegistarDoente{out: appclinico.DetalheDoente{ID: "id-1", NumProcesso: "P-2026-000001"}},
		fakeObterDoente{out: appclinico.DetalheDoente{ID: "id-1"}},
		fakePesquisarDoentes{out: appclinico.PaginaDoentes{Total: 0, Itens: nil}},
		fakeActualizarDoente{out: appclinico.DetalheDoente{ID: "id-1"}},
		fakeGerirEstado{out: appclinico.DetalheDoente{ID: "id-1", Estado: "INACTIVO"}},
		fakeRegistarAlergia{out: appclinico.DetalheDoente{ID: "id-1"}},
		fakeRegistarAntecedente{out: appclinico.DetalheDoente{ID: "id-1"}},
	)
	adhttp.RegistarDoentes(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestDoentes_Registar_AdministrativoPermitido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Sujeito: "a1", Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	corpo := `{"nome_completo":"Ana","data_nascimento":"1990-05-20","sexo":"F","bi":"00123456LA042","telefone":"+244923456789"}`
	w := pedidoCorpo(r, "POST", "/api/v1/doentes", corpo)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Registar_FarmaceuticoProibido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}})
	corpo := `{"nome_completo":"Ana","data_nascimento":"1990-05-20","sexo":"F","bi":"00123456LA042","telefone":"+244923456789"}`
	w := pedidoCorpo(r, "POST", "/api/v1/doentes", corpo)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestDoentes_Registar_DataInvalida_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	corpo := `{"nome_completo":"Ana","data_nascimento":"20-05-1990","sexo":"F","bi":"00123456LA042","telefone":"+244923456789"}`
	w := pedidoCorpo(r, "POST", "/api/v1/doentes", corpo)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Pesquisar_LeituraAmpla(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelFarmaceutico}})
	w := pedido(r, "GET", "/api/v1/doentes?termo=ana", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestDoentes_Obter_LeituraAmpla(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAuditor}})
	w := pedido(r, "GET", "/api/v1/doentes/id-1", "Bearer xyz")
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d", w.Code)
	}
}

func TestDoentes_Alergia_MedicoPermitido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/alergias", `{"substancia":"Penicilina","severidade":"GRAVE"}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
}

func TestDoentes_Alergia_AdministrativoProibido(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdministrativo}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/alergias", `{"substancia":"Penicilina","severidade":"GRAVE"}`)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("esperava 403, obtive %d", w.Code)
	}
}

func TestDoentes_Estado_Desactivar(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Sujeito: "a1", Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/estado", `{"accao":"desactivar","motivo":"engano"}`)
	if w.Code != nethttp.StatusOK {
		t.Fatalf("esperava 200, obtive %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "INACTIVO") {
		t.Fatalf("esperava estado INACTIVO no corpo: %s", w.Body.String())
	}
}

func TestDoentes_Estado_AccaoInvalida_400(t *testing.T) {
	r := routerDoentes(dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}})
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/estado", `{"accao":"teletransportar"}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}

func TestDoentes_Antecedente_ErroAplicacaoMapeado(t *testing.T) {
	// Um erro de validação da aplicação deve mapear para 400 (RFC 7807).
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoDoentesHandler(
		fakeRegistarDoente{}, fakeObterDoente{}, fakePesquisarDoentes{}, fakeActualizarDoente{},
		fakeGerirEstado{}, fakeRegistarAlergia{},
		fakeRegistarAntecedente{err: erros.Novo(erros.CategoriaValidacao, "tipo inválido")},
	)
	adhttp.RegistarDoentes(r, h, adhttp.Auth(fakeAuth{sessao: dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}}}))
	w := pedidoCorpo(r, "POST", "/api/v1/doentes/id-1/antecedentes", `{"tipo":"X","descricao":"y"}`)
	if w.Code != nethttp.StatusBadRequest {
		t.Fatalf("esperava 400, obtive %d", w.Code)
	}
}
```

> **Nota ao implementador:** os aliases `ctxT` e `timeT` acima são só para encurtar as assinaturas dos fakes no plano. No ficheiro real, importe `"context"` e `"time"` e use `context.Context` e `time.Time` directamente (remova `ctxT`/`timeT`).

- [ ] **Step 2: Correr e confirmar que falha**

Run: `go test ./internal/adapters/http/ -run Doentes -v`
Expected: FAIL — `NovoDoentesHandler`/`RegistarDoentes` indefinidos.

- [ ] **Step 3: Implementar `doente_handler.go`**

```go
package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Interfaces dos casos de uso do BC Clínico (application/clinico).
type (
	// ServicoRegistarDoente regista um novo doente.
	ServicoRegistarDoente interface {
		Executar(ctx context.Context, actor string, dados appclinico.DadosNovoDoente) (appclinico.DetalheDoente, error)
	}
	// ServicoObterDoente devolve o detalhe de um doente.
	ServicoObterDoente interface {
		Executar(ctx context.Context, actor, id string) (appclinico.DetalheDoente, error)
	}
	// ServicoPesquisarDoentes pesquisa doentes.
	ServicoPesquisarDoentes interface {
		Executar(ctx context.Context, filtro appclinico.FiltroDoentes) (appclinico.PaginaDoentes, error)
	}
	// ServicoActualizarDoente actualiza identificação/contactos/grupo.
	ServicoActualizarDoente interface {
		Executar(ctx context.Context, actor, id string, dados appclinico.DadosActualizarDoente) (appclinico.DetalheDoente, error)
	}
	// ServicoGerirEstadoDoente aplica transições de estado.
	ServicoGerirEstadoDoente interface {
		Desactivar(ctx context.Context, actor, id, motivo string) (appclinico.DetalheDoente, error)
		DeclararFalecido(ctx context.Context, actor, id string, data time.Time, causaCID string) (appclinico.DetalheDoente, error)
	}
	// ServicoRegistarAlergia regista uma alergia.
	ServicoRegistarAlergia interface {
		Executar(ctx context.Context, actor, doenteID string, dados appclinico.DadosAlergia) (appclinico.DetalheDoente, error)
	}
	// ServicoRegistarAntecedente regista um antecedente clínico.
	ServicoRegistarAntecedente interface {
		Executar(ctx context.Context, actor, doenteID string, dados appclinico.DadosAntecedente) (appclinico.DetalheDoente, error)
	}
)

// DoentesHandler expõe os endpoints HTTP do agregado Doente.
type DoentesHandler struct {
	registar     ServicoRegistarDoente
	obter        ServicoObterDoente
	pesquisar    ServicoPesquisarDoentes
	actualizar   ServicoActualizarDoente
	estado       ServicoGerirEstadoDoente
	alergia      ServicoRegistarAlergia
	antecedente  ServicoRegistarAntecedente
}

// NovoDoentesHandler constrói o handler com os casos de uso.
func NovoDoentesHandler(
	registar ServicoRegistarDoente,
	obter ServicoObterDoente,
	pesquisar ServicoPesquisarDoentes,
	actualizar ServicoActualizarDoente,
	estado ServicoGerirEstadoDoente,
	alergia ServicoRegistarAlergia,
	antecedente ServicoRegistarAntecedente,
) *DoentesHandler {
	return &DoentesHandler{
		registar: registar, obter: obter, pesquisar: pesquisar, actualizar: actualizar,
		estado: estado, alergia: alergia, antecedente: antecedente,
	}
}

// RegistarDoentes regista as rotas sob /api/v1/doentes, aplicando `protecao` ao
// grupo (rate limit + Auth) e o RBAC por rota.
func RegistarDoentes(r gin.IRouter, h *DoentesHandler, protecao ...gin.HandlerFunc) {
	g := r.Group("/api/v1/doentes")
	g.Use(protecao...)

	leitura := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro, dominio.PapelAdministrativo,
		dominio.PapelFarmaceutico, dominio.PapelTecnicoLab, dominio.PapelDirector,
		dominio.PapelDPO, dominio.PapelAuditor)
	demografia := RBAC(dominio.PapelAdministrativo, dominio.PapelMedico, dominio.PapelEnfermeiro)
	clinicos := RBAC(dominio.PapelMedico, dominio.PapelEnfermeiro)

	g.POST("", demografia, h.registarDoente)
	g.GET("", leitura, h.pesquisarDoentes)
	g.GET("/:id", leitura, h.obterDoente)
	g.PATCH("/:id", demografia, h.actualizarDoente)
	g.POST("/:id/estado", demografia, h.gerirEstado)
	g.POST("/:id/alergias", clinicos, h.registarAlergia)
	g.POST("/:id/antecedentes", clinicos, h.registarAntecedente)
}

const formatoData = "2006-01-02"

type corpoMorada struct {
	Provincia  string  `json:"provincia"`
	Municipio  string  `json:"municipio"`
	Comuna     string  `json:"comuna"`
	Bairro     string  `json:"bairro"`
	Rua        string  `json:"rua"`
	Casa       *string `json:"casa"`
	Referencia *string `json:"referencia"`
}

func (m *corpoMorada) paraDTO() *appclinico.DadosMorada {
	if m == nil {
		return nil
	}
	return &appclinico.DadosMorada{
		Provincia: m.Provincia, Municipio: m.Municipio, Comuna: m.Comuna,
		Bairro: m.Bairro, Rua: m.Rua, Casa: m.Casa, Referencia: m.Referencia,
	}
}

type corpoRegistarDoente struct {
	NumProcesso    string       `json:"num_processo"`
	NomeCompleto   string       `json:"nome_completo"`
	DataNascimento string       `json:"data_nascimento"`
	Sexo           string       `json:"sexo"`
	BI             *string      `json:"bi"`
	NIF            *string      `json:"nif"`
	Passaporte     *string      `json:"passaporte"`
	Nacionalidade  string       `json:"nacionalidade"`
	Telefone       string       `json:"telefone"`
	Email          *string      `json:"email"`
	Morada         *corpoMorada `json:"morada"`
	GrupoSanguineo *string      `json:"grupo_sanguineo"`
}

func (h *DoentesHandler) registarDoente(c *gin.Context) {
	var corpo corpoRegistarDoente
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	nasc, err := time.Parse(formatoData, corpo.DataNascimento)
	if err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "data de nascimento inválida (formato esperado AAAA-MM-DD)"))
		return
	}
	actor, _ := SessaoDe(c)
	dados := appclinico.DadosNovoDoente{
		NumProcesso: corpo.NumProcesso,
		Identificacao: appclinico.DadosIdentificacao{
			NomeCompleto: corpo.NomeCompleto, DataNascimento: nasc, Sexo: corpo.Sexo,
			BI: corpo.BI, NIF: corpo.NIF, Passaporte: corpo.Passaporte,
		},
		Contactos:      appclinico.DadosContactos{Telefone: corpo.Telefone, Email: corpo.Email, Morada: corpo.Morada.paraDTO()},
		Nacionalidade:  corpo.Nacionalidade,
		GrupoSanguineo: corpo.GrupoSanguineo,
	}
	out, err := h.registar.Executar(c.Request.Context(), actor.Sujeito, dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, out)
}

func (h *DoentesHandler) pesquisarDoentes(c *gin.Context) {
	filtro := appclinico.FiltroDoentes{
		Termo:        c.Query("termo"),
		Estado:       c.Query("estado"),
		Limite:       inteiroQuery(c, "limite"),
		Deslocamento: inteiroQuery(c, "deslocamento"),
	}
	out, err := h.pesquisar.Executar(c.Request.Context(), filtro)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *DoentesHandler) obterDoente(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.obter.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoActualizarDoente struct {
	Identificacao *struct {
		NomeCompleto   string  `json:"nome_completo"`
		DataNascimento string  `json:"data_nascimento"`
		Sexo           string  `json:"sexo"`
		BI             *string `json:"bi"`
		NIF            *string `json:"nif"`
		Passaporte     *string `json:"passaporte"`
	} `json:"identificacao"`
	Contactos *struct {
		Telefone string       `json:"telefone"`
		Email    *string      `json:"email"`
		Morada   *corpoMorada `json:"morada"`
	} `json:"contactos"`
	GrupoSanguineo *string `json:"grupo_sanguineo"`
}

func (h *DoentesHandler) actualizarDoente(c *gin.Context) {
	var corpo corpoActualizarDoente
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	var dados appclinico.DadosActualizarDoente
	if corpo.Identificacao != nil {
		nasc, err := time.Parse(formatoData, corpo.Identificacao.DataNascimento)
		if err != nil {
			responderErro(c, erros.Novo(erros.CategoriaValidacao, "data de nascimento inválida (formato esperado AAAA-MM-DD)"))
			return
		}
		dados.Identificacao = &appclinico.DadosIdentificacao{
			NomeCompleto: corpo.Identificacao.NomeCompleto, DataNascimento: nasc, Sexo: corpo.Identificacao.Sexo,
			BI: corpo.Identificacao.BI, NIF: corpo.Identificacao.NIF, Passaporte: corpo.Identificacao.Passaporte,
		}
	}
	if corpo.Contactos != nil {
		dados.Contactos = &appclinico.DadosContactos{
			Telefone: corpo.Contactos.Telefone, Email: corpo.Contactos.Email, Morada: corpo.Contactos.Morada.paraDTO(),
		}
	}
	dados.GrupoSanguineo = corpo.GrupoSanguineo

	actor, _ := SessaoDe(c)
	out, err := h.actualizar.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), dados)
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoEstado struct {
	Accao     string `json:"accao"`      // "desactivar" | "falecido"
	Motivo    string `json:"motivo"`     // desactivar
	DataObito string `json:"data_obito"` // falecido (AAAA-MM-DD)
	CausaCID  string `json:"causa_cid"`  // falecido
}

func (h *DoentesHandler) gerirEstado(c *gin.Context) {
	var corpo corpoEstado
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	id := c.Param("id")
	var (
		out appclinico.DetalheDoente
		err error
	)
	switch corpo.Accao {
	case "desactivar":
		out, err = h.estado.Desactivar(c.Request.Context(), actor.Sujeito, id, corpo.Motivo)
	case "falecido":
		data, perr := time.Parse(formatoData, corpo.DataObito)
		if perr != nil {
			responderErro(c, erros.Novo(erros.CategoriaValidacao, "data de óbito inválida (formato esperado AAAA-MM-DD)"))
			return
		}
		out, err = h.estado.DeclararFalecido(c.Request.Context(), actor.Sujeito, id, data, corpo.CausaCID)
	default:
		responderErro(c, erros.Novo(erros.CategoriaValidacao, "acção de estado inválida (esperado 'desactivar' ou 'falecido')"))
		return
	}
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoAlergia struct {
	Substancia    string  `json:"substancia"`
	Severidade    string  `json:"severidade"`
	ReaccaoTipica string  `json:"reaccao_tipica"`
	ConfirmadaEm  *string `json:"confirmada_em"`
	Notas         string  `json:"notas"`
}

func (h *DoentesHandler) registarAlergia(c *gin.Context) {
	var corpo corpoAlergia
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	confirmada, err := dataOpcional(corpo.ConfirmadaEm)
	if err != nil {
		responderErro(c, err)
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.alergia.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), appclinico.DadosAlergia{
		Substancia: corpo.Substancia, Severidade: corpo.Severidade, ReaccaoTipica: corpo.ReaccaoTipica,
		ConfirmadaEm: confirmada, Notas: corpo.Notas,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

type corpoAntecedente struct {
	Tipo       string  `json:"tipo"`
	Descricao  string  `json:"descricao"`
	CID        string  `json:"cid"`
	DataInicio *string `json:"data_inicio"`
	Activo     bool    `json:"activo"`
	Notas      string  `json:"notas"`
}

func (h *DoentesHandler) registarAntecedente(c *gin.Context) {
	var corpo corpoAntecedente
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	inicio, err := dataOpcional(corpo.DataInicio)
	if err != nil {
		responderErro(c, err)
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.antecedente.Executar(c.Request.Context(), actor.Sujeito, c.Param("id"), appclinico.DadosAntecedente{
		Tipo: corpo.Tipo, Descricao: corpo.Descricao, CID: corpo.CID,
		DataInicio: inicio, Activo: corpo.Activo, Notas: corpo.Notas,
	})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

// dataOpcional converte uma data "AAAA-MM-DD" opcional (ponteiro) num *time.Time.
func dataOpcional(v *string) (*time.Time, error) {
	if v == nil || *v == "" {
		return nil, nil
	}
	t, err := time.Parse(formatoData, *v)
	if err != nil {
		return nil, erros.Novo(erros.CategoriaValidacao, "data inválida (formato esperado AAAA-MM-DD)")
	}
	return &t, nil
}

// inteiroQuery lê um parâmetro de query como inteiro; 0 se ausente ou inválido.
func inteiroQuery(c *gin.Context, chave string) int {
	v := c.Query(chave)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}
```

> **Nota ao implementador:** acrescente `"strconv"` ao bloco de imports (usado por `inteiroQuery`). Confirme `gofmt`.

- [ ] **Step 4: Correr os testes e a cobertura dos adaptadores**

Run: `go test ./internal/adapters/http/ -v`
Expected: PASS (todos, incluindo os pré-existentes).
Run: `bash scripts/cobertura.sh` (secção adaptadores) — Expected: adaptadores ≥60%. Se ficar abaixo, acrescente casos ao `doente_test.go` (ex.: PATCH 200, antecedente 200, estado falecido 200, pesquisa com erro 500) até recuperar.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/doente_handler.go internal/adapters/http/doente_test.go
git commit -m "feat(clinico): handler HTTP de doentes com RBAC clínico e administrativo

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 12: Wiring do BC Clínico no composition root

**Files:**
- Modify: `internal/platform/app.go`

**Interfaces:**
- Consumes: `pgrepo.NovoRepositorioDoentes` (Task 10), casos de uso `application/clinico` (Tasks 6-9), `adhttp.NovoDoentesHandler`/`RegistarDoentes` (Task 11); middlewares já construídos (`limiteMW`, `authMW`).
- Produces: rotas `/api/v1/doentes` activas no servidor.

**Contexto:** `ExecutarServidor` já constrói `pool`, `repoAuditoria`, `limiteMW`, `authMW`, `mfaMW` e o closure `registarRotas`. Acrescentar o BC Clínico e registar as suas rotas com **`limiteMW` + `authMW`** (sem `mfaMW` — o MFA é imposto no login dos papéis sensíveis; os endpoints clínicos servem também papéis não-sensíveis).

- [ ] **Step 1: Acrescentar a construção do BC Clínico**

Em `internal/platform/app.go`, importar o pacote da aplicação clínica (junto aos outros imports):

```go
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
```

Depois do bloco que constrói o handler de administração (a seguir a `handlerAdmin := adhttp.NovoAdministracaoHandler(...)`), acrescentar:

```go
	// BC Clínico: repositório, casos de uso e handler do agregado Doente.
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	handlerDoentes := adhttp.NovoDoentesHandler(
		appclinico.NovoCasoRegistarDoente(repoDoentes, repoAuditoria),
		appclinico.NovoCasoObterDoente(repoDoentes, repoAuditoria),
		appclinico.NovoCasoPesquisarDoentes(repoDoentes),
		appclinico.NovoCasoActualizarDoente(repoDoentes, repoAuditoria),
		appclinico.NovoCasoGerirEstadoDoente(repoDoentes, repoAuditoria),
		appclinico.NovoCasoRegistarAlergia(repoDoentes, repoAuditoria),
		appclinico.NovoCasoRegistarAntecedente(repoDoentes, repoAuditoria),
	)
```

- [ ] **Step 2: Registar as rotas no closure `registarRotas`**

Alterar o closure para incluir o registo das rotas de doentes (protecção = rate limit + Auth):

```go
	registarRotas := func(r gin.IRouter) {
		adhttp.RegistarIdentidade(r, handlerIdentidade, limiteMW, authMW, mfaMW)
		adhttp.RegistarAdministracao(r, handlerAdmin, limiteMW, authMW, mfaMW)
		adhttp.RegistarDoentes(r, handlerDoentes, limiteMW, authMW)
	}
```

- [ ] **Step 3: Compilar e correr o lint de arquitectura**

Run: `go build ./...`
Expected: sem erros.
Run: `go run github.com/fe3dback/go-arch-lint@latest check` (ou o alvo `make lint` do projecto)
Expected: sem violações — o componente `domain` (que inclui `internal/domain/clinico/**`) não importa `pgx`/`gin`/`uuid`; `application` não importa infra.

> **Nota:** o `.go-arch-lint.yml` usa componentes por caminho (`internal/domain/**`, `internal/application/**`, `internal/adapters/**`), pelo que o novo BC `clinico` já é abrangido — **não é preciso** alterar o arch-lint.

- [ ] **Step 4: Correr a suite completa e os gates de cobertura**

Run: `go test ./...`
Expected: PASS.
Run: `bash scripts/cobertura.sh`
Expected: domínio ≥85%, aplicação ≥75%, adaptadores ≥60% — todos OK.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/app.go
git commit -m "feat(clinico): liga o BC Clínico (doentes) ao composition root

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 13: ADR-026 — decisões do BC Clínico (Doente)

**Files:**
- Create: `adrs/ADR-026-bc-clinico-doente.md`

**Interfaces:** documentação; sem código.

**Contexto:** seguir o formato dos ADRs existentes (`adrs/ADR-025-*.md`). Registar as decisões estruturais desta fatia.

- [ ] **Step 1: Escrever `adrs/ADR-026-bc-clinico-doente.md`**

```markdown
# ADR-026 — BC Clínico: agregado Doente

- **Estado:** Aceite
- **Data:** 2026-07-12
- **Marco:** M2 — Clínico Core (Sprint 7)
- **Contexto de spec:** docs/superpowers/specs/2026-07-12-sprint7-clinico-doente-design.md

## Contexto

O M2 abre o Bounded Context Clínico. A primeira fatia vertical é o agregado
Doente (com Alergia e AntecedenteClinico), do domínio ao HTTP. O modelo de dados
foi extraído verbatim do DDM-001 v2.0.

## Decisões

1. **Identidade gerada pela base de dados.** O `id` dos doentes (e filhos) é
   `uuid` com `DEFAULT gen_random_uuid()`, obtido por `RETURNING`. O domínio usa
   `string` e nunca gera IDs — mantém-se puro (sem `google/uuid` no domínio nem na
   aplicação, conforme o arch-lint).

2. **Número de processo híbrido.** Se o pedido trouxer um número, é usado (unicidade
   garantida por `UNIQUE`; colisão → 409). Caso contrário, é gerado
   `P-{ano}-{sequencial:06d}` a partir de um contador por ano
   (`clinico.processo_sequencia`), incrementado atomicamente por
   `INSERT ... ON CONFLICT (ano) DO UPDATE ... RETURNING`.

3. **RBAC clínico vs administrativo.** Escrita de demografia/contactos/estado:
   Administrativo, Médico, Enfermeiro. Escrita de dados clínicos
   (alergias/antecedentes): apenas Médico e Enfermeiro. Leitura: ampla (Médico,
   Enfermeiro, Administrativo, Farmacêutico, TecnicoLab, Director, DPO, Auditor).

4. **Auditoria de acesso a dados de saúde.** Além da escrita, a consulta individual
   de um doente é auditada (`clinico.doente.consultado`). A pesquisa (listagem) não
   é auditada, para evitar ruído.

5. **Rehidratação por snapshot.** O agregado Doente tem campos privados e expõe
   `Snapshot()`/`ReconstruirDoente(SnapshotDoente)`; a construção validante
   (`NovoDoente`) fica separada da rehidratação a partir da BD (dados confiáveis).

6. **Validador de NIF no Shared Kernel.** Acrescentado `identity.NovoNIF` (10
   caracteres: 10 dígitos ou 9 dígitos + 1 letra), a par de `NovoBI`/`NovoTelefone`.

7. **Pesquisa por trigram.** Índice `gin (nome_completo gin_trgm_ops)` sustenta o
   ILIKE fuzzy por nome; BI, número de processo e telefone são pesquisados por
   igualdade. Paginação por `limite` (default 20, máximo 100) e `deslocamento`.

## Diferimentos

- **LPDP:** o estado `APAGADO` e a pseudonimização (apagamento com retenção legal)
  ficam para uma fatia dedicada. A coluna `apagado_em` e o estado já existem no
  esquema, mas o fluxo de apagamento não é implementado neste sprint.
- **Consentimentos** e **episódios clínicos** (tabelas do DDM) ficam fora de âmbito
  (Sprint 8+).
- **Telefone fixo:** o validador cobre apenas o telemóvel angolano (+244 9XX).

## Consequências

- Base sólida e auditável para os episódios clínicos (Sprint 8), que referenciarão
  `clinico.doentes(id)`.
- O contador por ano exige atenção em cenários multi-instância (o
  `ON CONFLICT ... RETURNING` é atómico, pelo que é seguro sob concorrência).
```

- [ ] **Step 2: Commit**

```bash
git add adrs/ADR-026-bc-clinico-doente.md
git commit -m "docs(clinico): ADR-026 com as decisões do BC Clínico (Doente)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Verificação final (fim a fim)

Após a Task 13, antes da revisão de ramo completa:

1. `go build ./...` — sem erros.
2. `go test ./...` — PASS (unidade + aplicação + adaptadores).
3. `bash scripts/cobertura.sh` — domínio ≥85%, aplicação ≥75%, adaptadores ≥60%.
4. `make lint` (golangci-lint + go-arch-lint) — sem violações; o domínio `clinico`
   não importa `pgx`/`gin`/`uuid`.
5. `gofmt -l internal/ migrations/ tests/` — vazio.
6. Com o compose a correr: `make migrate` aplica `clinico/0001`; `schema_migrations`
   regista `clinico/0001_doentes`.
7. `go test -tags integration ./tests/integration/ -run TestRepositorioDoentes` —
   PASS (com BD) ou SKIP (sem `DATABASE_URL`).
8. Fluxo HTTP (com token de um Médico via password grant, ver Sprint 2):
   - `POST /api/v1/doentes` sem `num_processo` → 201 + `P-2026-000001`.
   - `POST /api/v1/doentes` com `num_processo` repetido → 409 (RFC 7807 PT-PT).
   - `GET /api/v1/doentes?termo=<parte-do-nome>` → 200 + página.
   - `POST /api/v1/doentes/:id/alergias` como Médico → 200; como Administrativo → 403.
   - `POST /api/v1/doentes/:id/estado` `{"accao":"desactivar","motivo":"..."}` → 200 + `INACTIVO`.
   - Confirmar eventos em `auditoria.auditoria_eventos` (`clinico.doente.*`).

## Fora de âmbito (fatias futuras)

- Apagamento LPDP (`APAGADO` + pseudonimização) e tabela `consentimentos`.
- `episodios_clinicos` (Sprint 8).
- Endpoint de consulta por número de processo (o repo já suporta
  `ObterPorNumProcesso`; falta a rota, reservada para quando for necessária).
