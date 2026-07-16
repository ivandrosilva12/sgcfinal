# Sprint 13 — BC Laboratório: Validação + Valores Críticos + Correcção — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fechar o marco M3 — o patologista valida o resultado preliminar com segregação de funções, os valores críticos são detectados na validação e notificados por SMS auditado, e a correcção cria um novo resultado preservando o original.

**Architecture:** Continua o BC Laboratório do Sprint 12 (DDD táctico + Clean). As novas transições `Validar` e `Corrigir` vivem no agregado `Resultado` (domínio puro); a avaliação de valor crítico é um método de `Analise`; a aplicação orquestra (carrega catálogo, avalia crítico, notifica); dois adaptadores novos (SMS e resolvedor de contacto via ACL sobre Identidade) e duas rotas HTTP fecham a fatia.

**Tech Stack:** Go 1.22+, Gin, pgx v5 (SQL puro), PostgreSQL 16. Testes com a stdlib `testing` e fakes.

## Global Constraints

- **Idioma:** PT-PT angolano em TODO o código, comentários, mensagens e commits. Nunca EN/BR.
- **Linguagem ubíqua:** `Resultado`, `Analise`, `RequisicaoLab`, `Patologista`, `TecnicoLab`, `ValorCritico`. Nunca Patient/Result/Invoice.
- **Módulo Go:** `github.com/ivandrosilva12/sgcfinal`.
- **Domínio sem infra:** `internal/domain/**` importa apenas stdlib e Shared Kernel (`erros`, `evento`). Zero `pgx`/`gin`/`net/http`. O `go-arch-lint` bloqueia violações.
- **Erros:** `erros.Novo(erros.CategoriaX, "mensagem pt-PT")`; categorias `CategoriaValidacao` (400), `CategoriaConflito` (409), `CategoriaRegraNegocio` (422), `CategoriaNaoEncontrado` (404).
- **Sem `panic()`** fora de inicialização — sempre `error`.
- **Actor = sujeito autenticado**, nunca campo do corpo (regra ADR-031). O validador/corrector vem de `SessaoDe(c).Sujeito`.
- **Migrations forward-only**, sem `.down.sql`.
- **Gates de cobertura:** domínio ≥85%, aplicação ≥75%, adaptadores ≥60% (pgrepo coberto por integração).
- **Estado da máquina (final):** `PENDENTE → COLHIDA → PROCESSADA → VALIDADA`; `VALIDADA →(correcção)→ CONCLUIDA` (original arquivado) + novo `VALIDADA`; `RECUSADA` a partir de PENDENTE/COLHIDA.

---

### Task 1: `Analise.AvaliarCritico` — avaliação de valor crítico no domínio

**Files:**
- Modify: `internal/domain/laboratorio/analise.go`
- Test: `internal/domain/laboratorio/analise_test.go`

**Interfaces:**
- Consumes: `Analise` (tem `criticos []ValorCritico`), `ValorCritico{Operador OperadorCritico, Limite float64}`.
- Produces: `func (a *Analise) AvaliarCritico(valorTexto string) bool`.

- [ ] **Step 1: Escrever o teste que falha**

Acrescentar a `internal/domain/laboratorio/analise_test.go`:

```go
func TestAnalise_AvaliarCritico(t *testing.T) {
	criticos := []dominio.ValorCritico{
		{Operador: dominio.CriticoMenor, Limite: 3.0, Descricao: "anemia grave"},
		{Operador: dominio.CriticoMaior, Limite: 18.0, Descricao: "policitemia"},
	}
	a, err := dominio.NovaAnalise("HB", "Hemoglobina", "g/dL", nil, criticos)
	if err != nil {
		t.Fatalf("análise base inválida: %v", err)
	}
	casos := []struct {
		valor    string
		esperado bool
	}{
		{"2.9", true},   // < 3.0
		{"3.0", false},  // fronteira: não é < 3.0 nem > 18.0
		{"12.5", false}, // normal
		{"18.1", true},  // > 18.0
		{"Positivo", false}, // não numérico → nunca crítico
		{"", false},         // vazio → nunca crítico
	}
	for _, c := range casos {
		t.Run(c.valor, func(t *testing.T) {
			if got := a.AvaliarCritico(c.valor); got != c.esperado {
				t.Fatalf("AvaliarCritico(%q) = %v, esperava %v", c.valor, got, c.esperado)
			}
		})
	}
}

func TestAnalise_AvaliarCritico_OperadoresIgual(t *testing.T) {
	criticos := []dominio.ValorCritico{
		{Operador: dominio.CriticoMenorIgual, Limite: 2.0, Descricao: "x"},
		{Operador: dominio.CriticoMaiorIgual, Limite: 40.0, Descricao: "y"},
	}
	a, _ := dominio.NovaAnalise("K", "Potássio", "mmol/L", nil, criticos)
	if !a.AvaliarCritico("2.0") {
		t.Fatal("2.0 devia ser crítico com <= 2.0")
	}
	if !a.AvaliarCritico("40.0") {
		t.Fatal("40.0 devia ser crítico com >= 40.0")
	}
	if a.AvaliarCritico("10") {
		t.Fatal("10 não devia ser crítico")
	}
}

func TestAnalise_AvaliarCritico_SemRegras(t *testing.T) {
	a, _ := dominio.NovaAnalise("XPT", "Sem críticos", "un", nil, nil)
	if a.AvaliarCritico("999") {
		t.Fatal("sem valores críticos configurados, nada é crítico")
	}
}
```

- [ ] **Step 2: Correr o teste para confirmar que falha**

Run: `go test ./internal/domain/laboratorio/ -run TestAnalise_AvaliarCritico -v`
Expected: FAIL — `a.AvaliarCritico undefined`.

- [ ] **Step 3: Implementar o método**

Em `internal/domain/laboratorio/analise.go`, acrescentar `"strconv"` aos imports e o método (a seguir a `NovaAnalise`):

```go
// AvaliarCritico indica se o valor textual satisfaz alguma das condições de valor
// crítico do catálogo. Valores não numéricos (ex.: "Positivo") nunca são críticos:
// os limiares configurados são numéricos. Avaliado na validação (Sprint 13).
func (a *Analise) AvaliarCritico(valorTexto string) bool {
	v, err := strconv.ParseFloat(strings.TrimSpace(valorTexto), 64)
	if err != nil {
		return false
	}
	for _, c := range a.criticos {
		switch c.Operador {
		case CriticoMenor:
			if v < c.Limite {
				return true
			}
		case CriticoMaior:
			if v > c.Limite {
				return true
			}
		case CriticoMenorIgual:
			if v <= c.Limite {
				return true
			}
		case CriticoMaiorIgual:
			if v >= c.Limite {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 4: Correr o teste para confirmar que passa**

Run: `go test ./internal/domain/laboratorio/ -run TestAnalise_AvaliarCritico -v`
Expected: PASS (todos os subtestes).

- [ ] **Step 5: Commit**

```bash
git add internal/domain/laboratorio/analise.go internal/domain/laboratorio/analise_test.go
git commit -m "feat(laboratorio): avaliação de valor crítico no catálogo (Analise.AvaliarCritico)"
```

---

### Task 2: `Resultado.Validar` + getter `Valor()` — transição com segregação

**Files:**
- Modify: `internal/domain/laboratorio/resultado.go`
- Test: `internal/domain/laboratorio/resultado_test.go`

**Interfaces:**
- Consumes: `Resultado` (campos `estado`, `tecnicoSubmissorID`, `patologistaValidadorID`, `validadaEm`, `valorCritico`).
- Produces:
  - `func (r *Resultado) Validar(patologistaID string, critico bool, em time.Time) error`
  - `func (r *Resultado) Valor() string`

- [ ] **Step 1: Escrever o teste que falha**

Acrescentar a `internal/domain/laboratorio/resultado_test.go`:

```go
// processadaPor devolve um resultado rehidratado em PROCESSADA submetido por `submissor`.
func processadaPor(t *testing.T, submissor string) *dominio.Resultado {
	t.Helper()
	colhida := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	submetida := colhida.Add(time.Hour)
	return dominio.ReconstruirResultado(dominio.SnapshotResultado{
		ID: "res-1", RequisicaoID: "req-1", CodigoAnalise: "HB", Valor: "2.5", Unidade: "g/dL",
		Estado: dominio.ResProcessada, TecnicoColheitaID: submissor, TecnicoSubmissorID: submissor,
		ColhidaEm: &colhida, SubmetidaEm: &submetida,
	})
}

func TestResultado_Validar_FluxoFeliz(t *testing.T) {
	quando := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	r := processadaPor(t, "tec-1")
	if err := r.Validar("pat-9", true, quando); err != nil {
		t.Fatalf("validar devia funcionar: %v", err)
	}
	if r.Estado() != dominio.ResValidada {
		t.Fatalf("esperava VALIDADA, veio %s", r.Estado())
	}
	s := r.Snapshot()
	if s.PatologistaValidadorID != "pat-9" || s.ValidadaEm == nil || !s.ValorCritico {
		t.Fatalf("validação não gravou validador/data/crítico: %+v", s)
	}
}

func TestResultado_Validar_Segregacao(t *testing.T) {
	quando := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	r := processadaPor(t, "tec-1")
	// O próprio submissor não pode validar.
	err := r.Validar("tec-1", false, quando)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("auto-validação devia falhar com RegraNegocio, veio %v", err)
	}
}

func TestResultado_Validar_ForaDeProcessada_Conflito(t *testing.T) {
	quando := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	for _, estado := range []dominio.EstadoResultado{
		dominio.ResPendente, dominio.ResColhida, dominio.ResValidada,
		dominio.ResConcluida, dominio.ResRecusada,
	} {
		r := resultadoEmEstado(t, estado)
		err := r.Validar("pat-1", false, quando)
		if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
			t.Fatalf("validar desde %s devia dar Conflito, veio %v", estado, err)
		}
	}
}

func TestResultado_Validar_CamposEmFalta(t *testing.T) {
	r := processadaPor(t, "tec-1")
	if err := r.Validar("  ", false, time.Now()); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("validar sem patologista devia dar Validacao, veio %v", err)
	}
	r2 := processadaPor(t, "tec-1")
	if err := r2.Validar("pat-1", false, time.Time{}); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("validar sem data devia dar Validacao, veio %v", err)
	}
}
```

- [ ] **Step 2: Correr o teste para confirmar que falha**

Run: `go test ./internal/domain/laboratorio/ -run TestResultado_Validar -v`
Expected: FAIL — `r.Validar undefined`.

- [ ] **Step 3: Implementar o método e o getter**

Em `internal/domain/laboratorio/resultado.go`, a seguir a `SubmeterPreliminar`:

```go
// Validar transita PROCESSADA → VALIDADA. O validador é o sujeito autenticado. A
// invariante de segregação de funções é o coração do Sprint 13: quem submeteu o
// preliminar NUNCA o valida. `critico` é avaliado pela aplicação contra o catálogo
// (o agregado não conhece a Analise) e gravado aqui.
func (r *Resultado) Validar(patologistaID string, critico bool, em time.Time) error {
	if r.estado != ResProcessada {
		return erros.Novo(erros.CategoriaConflito, "só é possível validar um resultado processado")
	}
	patologistaID = strings.TrimSpace(patologistaID)
	if patologistaID == "" {
		return erros.Novo(erros.CategoriaValidacao, "patologista validador em falta")
	}
	if patologistaID == r.tecnicoSubmissorID {
		return erros.Novo(erros.CategoriaRegraNegocio,
			"segregação de funções: quem submeteu o resultado não o pode validar")
	}
	if em.IsZero() {
		return erros.Novo(erros.CategoriaValidacao, "data da validação em falta")
	}
	r.estado = ResValidada
	r.patologistaValidadorID = patologistaID
	r.validadaEm = &em
	r.valorCritico = critico
	return nil
}
```

E, junto dos outros getters (a seguir a `TecnicoSubmissorID`):

```go
// Valor devolve o valor submetido (vazio antes da submissão).
func (r *Resultado) Valor() string { return r.valor }
```

- [ ] **Step 4: Correr o teste para confirmar que passa**

Run: `go test ./internal/domain/laboratorio/ -run TestResultado_Validar -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/laboratorio/resultado.go internal/domain/laboratorio/resultado_test.go
git commit -m "feat(laboratorio): transição Validar com segregação de funções (submissor ≠ validador)"
```

---

### Task 3: `Resultado.Corrigir` + campo `corrigeResultadoID`

**Files:**
- Modify: `internal/domain/laboratorio/resultado.go`
- Test: `internal/domain/laboratorio/resultado_test.go`

**Interfaces:**
- Consumes: `Resultado` no estado `VALIDADA`.
- Produces:
  - novo campo `corrigeResultadoID string` no `Resultado`, no `SnapshotResultado` (`CorrigeResultadoID string`), mapeado em `Snapshot()` e `ReconstruirResultado()`.
  - `func (r *Resultado) Corrigir(patologistaID, novoValor, observacoes string, critico bool, em time.Time) (*Resultado, error)`
  - `func (r *Resultado) CorrigeResultadoID() string`

- [ ] **Step 1: Escrever o teste que falha**

Acrescentar a `internal/domain/laboratorio/resultado_test.go`:

```go
// validadaPor devolve um resultado rehidratado em VALIDADA (submetido por `submissor`,
// validado por `validador`).
func validadaPor(t *testing.T, submissor, validador string) *dominio.Resultado {
	t.Helper()
	colhida := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	submetida := colhida.Add(time.Hour)
	validada := submetida.Add(time.Hour)
	return dominio.ReconstruirResultado(dominio.SnapshotResultado{
		ID: "res-1", RequisicaoID: "req-1", CodigoAnalise: "HB", Valor: "2.5", Unidade: "g/dL",
		Estado: dominio.ResValidada, TecnicoColheitaID: submissor, TecnicoSubmissorID: submissor,
		PatologistaValidadorID: validador, ColhidaEm: &colhida, SubmetidaEm: &submetida, ValidadaEm: &validada,
	})
}

func TestResultado_Corrigir_ArquivaOriginalECriaNovo(t *testing.T) {
	quando := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	original := validadaPor(t, "tec-1", "pat-2")
	novo, err := original.Corrigir("pat-3", "12.5", "releitura", false, quando)
	if err != nil {
		t.Fatalf("corrigir devia funcionar: %v", err)
	}
	// Original arquivado.
	if original.Estado() != dominio.ResConcluida {
		t.Fatalf("original devia ficar CONCLUIDA, veio %s", original.Estado())
	}
	// Novo é VALIDADA, vigente, e aponta para o original.
	sn := novo.Snapshot()
	if novo.Estado() != dominio.ResValidada {
		t.Fatalf("novo devia ser VALIDADA, veio %s", novo.Estado())
	}
	if sn.Valor != "12.5" || sn.PatologistaValidadorID != "pat-3" {
		t.Fatalf("novo não gravou valor/validador: %+v", sn)
	}
	if novo.CorrigeResultadoID() != "res-1" {
		t.Fatalf("novo devia apontar para o original res-1, veio %q", novo.CorrigeResultadoID())
	}
	// Proveniência: o técnico submissor original é preservado no novo.
	if sn.TecnicoSubmissorID != "tec-1" {
		t.Fatalf("novo devia preservar o submissor original tec-1, veio %q", sn.TecnicoSubmissorID)
	}
	// Novo é um agregado por inserir: sem estado anterior.
	if sn.EstadoAnterior != "" {
		t.Fatalf("novo (por inserir) não devia ter estado anterior, veio %q", sn.EstadoAnterior)
	}
}

func TestResultado_Corrigir_Segregacao(t *testing.T) {
	quando := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	original := validadaPor(t, "tec-1", "pat-2")
	// O corrector não pode ser o técnico submissor original.
	_, err := original.Corrigir("tec-1", "12.5", "", false, quando)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("corrigir pelo submissor original devia dar RegraNegocio, veio %v", err)
	}
}

func TestResultado_Corrigir_ForaDeValidada_Conflito(t *testing.T) {
	quando := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for _, estado := range []dominio.EstadoResultado{
		dominio.ResPendente, dominio.ResColhida, dominio.ResProcessada,
		dominio.ResConcluida, dominio.ResRecusada,
	} {
		r := resultadoEmEstado(t, estado)
		if _, err := r.Corrigir("pat-1", "12.5", "", false, quando); err == nil ||
			erros.CategoriaDe(err) != erros.CategoriaConflito {
			t.Fatalf("corrigir desde %s devia dar Conflito, veio %v", estado, err)
		}
	}
}

func TestResultado_Corrigir_ValorEmFalta(t *testing.T) {
	original := validadaPor(t, "tec-1", "pat-2")
	if _, err := original.Corrigir("pat-3", "  ", "", false, time.Now()); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("corrigir sem valor devia dar Validacao, veio %v", err)
	}
}
```

Actualizar também `TestResultado_ReconstruirRoundTrip` (já existente): acrescentar ao `original` o campo `CorrigeResultadoID: "res-0"` para garantir que o round-trip o preserva. Localizar o literal `dominio.SnapshotResultado{` desse teste e acrescentar a linha `CorrigeResultadoID: "res-0",` antes de `ValorCritico: true,`.

- [ ] **Step 2: Correr o teste para confirmar que falha**

Run: `go test ./internal/domain/laboratorio/ -run 'TestResultado_Corrigir|TestResultado_ReconstruirRoundTrip' -v`
Expected: FAIL — `original.Corrigir undefined` e campo `CorrigeResultadoID` inexistente.

- [ ] **Step 3: Implementar o campo, o mapeamento e o método**

Em `internal/domain/laboratorio/resultado.go`:

1. No struct `Resultado`, acrescentar após `valorCritico bool`:
```go
	corrigeResultadoID     string
```

2. No `SnapshotResultado`, acrescentar após `ValorCritico bool`:
```go
	CorrigeResultadoID     string
```

3. Em `Snapshot()`, acrescentar o campo ao literal devolvido (junto de `ValorCritico`):
```go
		ValorCritico: r.valorCritico, CorrigeResultadoID: r.corrigeResultadoID, CriadoEm: r.criadoEm,
```

4. Em `ReconstruirResultado()`, acrescentar (junto de `valorCritico`):
```go
		valorCritico: s.ValorCritico, corrigeResultadoID: s.CorrigeResultadoID, criadoEm: s.CriadoEm,
```

5. O getter e o método (a seguir a `Validar`):
```go
// CorrigeResultadoID devolve o id do resultado que este corrige (vazio se não é
// uma correcção).
func (r *Resultado) CorrigeResultadoID() string { return r.corrigeResultadoID }

// Corrigir arquiva o resultado validado (→ CONCLUIDA) e devolve um NOVO Resultado
// VALIDADA que o substitui, apontando-lhe via corrigeResultadoID. Preserva o técnico
// submissor original (proveniência) — pelo que a segregação continua a valer: o
// corrector nunca é o técnico que submeteu o preliminar original. O novo agregado
// nasce por inserir (sem estado anterior).
func (r *Resultado) Corrigir(patologistaID, novoValor, observacoes string, critico bool, em time.Time) (*Resultado, error) {
	if r.estado != ResValidada {
		return nil, erros.Novo(erros.CategoriaConflito, "só é possível corrigir um resultado validado")
	}
	patologistaID = strings.TrimSpace(patologistaID)
	if patologistaID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "patologista corrector em falta")
	}
	if patologistaID == r.tecnicoSubmissorID {
		return nil, erros.Novo(erros.CategoriaRegraNegocio,
			"segregação de funções: quem submeteu o resultado não o pode corrigir")
	}
	novoValor = strings.TrimSpace(novoValor)
	if novoValor == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "valor corrigido em falta")
	}
	if em.IsZero() {
		return nil, erros.Novo(erros.CategoriaValidacao, "data da correcção em falta")
	}
	novo := &Resultado{
		requisicaoID:           r.requisicaoID,
		codigoAnalise:          r.codigoAnalise,
		valor:                  novoValor,
		unidade:                r.unidade,
		observacoes:            strings.TrimSpace(observacoes),
		estado:                 ResValidada,
		tecnicoColheitaID:      r.tecnicoColheitaID,
		tecnicoSubmissorID:     r.tecnicoSubmissorID,
		patologistaValidadorID: patologistaID,
		colhidaEm:              r.colhidaEm,
		submetidaEm:            r.submetidaEm,
		validadaEm:             &em,
		valorCritico:           critico,
		corrigeResultadoID:     r.id,
	}
	r.estado = ResConcluida
	return novo, nil
}
```

- [ ] **Step 4: Correr o teste para confirmar que passa**

Run: `go test ./internal/domain/laboratorio/ -run 'TestResultado_Corrigir|TestResultado_ReconstruirRoundTrip' -v`
Expected: PASS.

- [ ] **Step 5: Correr toda a suite do domínio (regressão)**

Run: `go test ./internal/domain/laboratorio/... -v`
Expected: PASS (todos).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/laboratorio/resultado.go internal/domain/laboratorio/resultado_test.go
git commit -m "feat(laboratorio): correcção cria novo resultado VALIDADA e arquiva o original em CONCLUIDA"
```

---

### Task 4: Eventos de domínio (scaffolding)

**Files:**
- Modify: `internal/domain/laboratorio/eventos.go`
- Test: `internal/domain/laboratorio/eventos_test.go`

**Interfaces:**
- Produces: `ResultadoValidado`, `ValorCriticoDetectado`, `ResultadoCorrigido` — cada um implementa `evento.EventoDominio` (`NomeEvento() string`, `OcorridoEm() time.Time`).

- [ ] **Step 1: Escrever o teste que falha**

Acrescentar a `internal/domain/laboratorio/eventos_test.go` (seguir o padrão dos testes já lá para `AmostraColhida` etc. — verificar `NomeEvento()` e `OcorridoEm()`):

```go
func TestEventosValidacao_NomesEData(t *testing.T) {
	em := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	casos := []struct {
		evt  interface{ NomeEvento() string }
		nome string
	}{
		{dominio.ResultadoValidado{Em: em}, "laboratorio.resultado.validado"},
		{dominio.ValorCriticoDetectado{Em: em}, "laboratorio.valor_critico.detectado"},
		{dominio.ResultadoCorrigido{Em: em}, "laboratorio.resultado.corrigido"},
	}
	for _, c := range casos {
		if c.evt.NomeEvento() != c.nome {
			t.Fatalf("esperava %q, veio %q", c.nome, c.evt.NomeEvento())
		}
	}
	if dominio.ResultadoValidado{Em: em}.OcorridoEm() != em {
		t.Fatal("OcorridoEm devia devolver o instante do evento")
	}
}
```

> Nota: se o `eventos_test.go` ainda não importa `dominio`/`time`, alinhar os imports pelo topo do ficheiro existente.

- [ ] **Step 2: Correr o teste para confirmar que falha**

Run: `go test ./internal/domain/laboratorio/ -run TestEventosValidacao -v`
Expected: FAIL — tipos indefinidos.

- [ ] **Step 3: Implementar os eventos**

Acrescentar a `internal/domain/laboratorio/eventos.go` (antes do bloco `var (...)` de asserções):

```go
// ResultadoValidado é emitido quando o patologista valida o preliminar. A partir
// daqui o resultado é visível ao médico.
type ResultadoValidado struct {
	ResultadoID   string
	RequisicaoID  string
	CodigoAnalise string
	ValorCritico  bool
	Em            time.Time
}

func (e ResultadoValidado) NomeEvento() string    { return "laboratorio.resultado.validado" }
func (e ResultadoValidado) OcorridoEm() time.Time { return e.Em }

// ValorCriticoDetectado é emitido quando a validação detecta um valor crítico.
type ValorCriticoDetectado struct {
	ResultadoID   string
	RequisicaoID  string
	CodigoAnalise string
	Valor         string
	Em            time.Time
}

func (e ValorCriticoDetectado) NomeEvento() string    { return "laboratorio.valor_critico.detectado" }
func (e ValorCriticoDetectado) OcorridoEm() time.Time { return e.Em }

// ResultadoCorrigido é emitido quando um resultado validado é corrigido: o original
// é arquivado e nasce um novo resultado.
type ResultadoCorrigido struct {
	ResultadoIDOriginal string
	ResultadoIDNovo     string
	RequisicaoID        string
	CodigoAnalise       string
	Em                  time.Time
}

func (e ResultadoCorrigido) NomeEvento() string    { return "laboratorio.resultado.corrigido" }
func (e ResultadoCorrigido) OcorridoEm() time.Time { return e.Em }
```

E acrescentar ao bloco `var (...)` de asserções de conformidade:

```go
	_ evento.EventoDominio = ResultadoValidado{}
	_ evento.EventoDominio = ValorCriticoDetectado{}
	_ evento.EventoDominio = ResultadoCorrigido{}
```

- [ ] **Step 4: Correr o teste para confirmar que passa**

Run: `go test ./internal/domain/laboratorio/ -run TestEventosValidacao -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/laboratorio/eventos.go internal/domain/laboratorio/eventos_test.go
git commit -m "feat(laboratorio): eventos de validação, valor crítico e correcção (scaffolding)"
```

---

### Task 5: Portas novas + `RepositorioResultados.Corrigir` + visibilidade + fakes

**Files:**
- Modify: `internal/domain/laboratorio/resultado.go` (interface `RepositorioResultados`)
- Modify: `internal/application/laboratorio/ports.go` (portas + `EstadosVisiveisAoMedico` + DTO)
- Modify: `internal/application/laboratorio/fakes_test.go` (implementar novos métodos/fakes)
- Test: `internal/application/laboratorio/resultados_test.go` (novo ficheiro — teste de visibilidade)

**Interfaces:**
- Produces (domínio):
  - `RepositorioResultados.Corrigir(ctx context.Context, novo *Resultado, original *Resultado) (string, error)`
- Produces (aplicação):
  - `type ResolvedorContacto interface { ContactoClinico(ctx context.Context, userID string) (telefone string, ok bool, err error) }`
  - `type NotificadorCritico interface { NotificarValorCritico(ctx context.Context, telefone, codigoAnalise, valor string) error }`
  - `type DadosCorrigirResultado struct { Valor string; Observacoes string }`
  - `EstadosVisiveisAoMedico` passa a `{dominio.ResValidada}`.

- [ ] **Step 1: Acrescentar `Corrigir` à interface do domínio**

Em `internal/domain/laboratorio/resultado.go`, na interface `RepositorioResultados`, acrescentar após `Transitar`:

```go
	// Corrigir persiste uma correcção numa única transacção: INSERT do novo Resultado
	// (VALIDADA, corrige_resultado_id→original) e UPDATE compare-and-set do original
	// (VALIDADA→CONCLUIDA). Qualquer falha faz rollback de ambos. Devolve o id do novo.
	Corrigir(ctx context.Context, novo *Resultado, original *Resultado) (string, error)
```

- [ ] **Step 2: Acrescentar portas, DTO e mudar a visibilidade na aplicação**

Em `internal/application/laboratorio/ports.go`:

1. A seguir à interface `LeitorClinico`, acrescentar:

```go
// ResolvedorContacto resolve o telefone de um utilizador do BC Identidade para a
// notificação de valor crítico. É uma extensão da ACL: o domínio/aplicação do Lab
// continua sem importar Identidade — só o adaptador conhece o outro contexto.
type ResolvedorContacto interface {
	ContactoClinico(ctx context.Context, userID string) (telefone string, ok bool, err error)
}

// NotificadorCritico envia o alerta de valor crítico. Best-effort: uma falha de
// envio não reverte a validação — só é auditada.
type NotificadorCritico interface {
	NotificarValorCritico(ctx context.Context, telefone, codigoAnalise, valor string) error
}
```

2. A seguir a `DadosSubmeterPreliminar`, acrescentar:

```go
// DadosCorrigirResultado é a entrada da correcção de um resultado validado.
type DadosCorrigirResultado struct {
	Valor       string `json:"valor"`
	Observacoes string `json:"observacoes"`
}
```

3. Substituir a definição de `EstadosVisiveisAoMedico` por:

```go
// EstadosVisiveisAoMedico são os únicos estados que a leitura clínica devolve: o
// preliminar (PROCESSADA) não é visível, e o resultado arquivado por uma correcção
// (CONCLUIDA) sai da vista clínica normal — o médico vê apenas o resultado vigente
// (VALIDADA). O CONCLUIDA fica preservado e auditável pela cadeia corrige_resultado_id.
var EstadosVisiveisAoMedico = []dominio.EstadoResultado{dominio.ResValidada}
```

- [ ] **Step 3: Implementar `Corrigir` no fake e acrescentar os novos fakes**

Em `internal/application/laboratorio/fakes_test.go`:

1. A seguir ao método `Transitar` do `fakeResultados`, acrescentar:

```go
func (f *fakeResultados) Corrigir(_ context.Context, novo, original *laboratorio.Resultado) (string, error) {
	so := original.Snapshot()
	actual, ok := f.porID[so.ID]
	if !ok {
		return "", erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
	}
	// Compare-and-set, como o repositório real (VALIDADA → CONCLUIDA).
	if actual.Estado != so.EstadoAnterior {
		return "", erros.Novo(erros.CategoriaConflito, "o estado do resultado mudou entretanto")
	}
	f.porID[so.ID] = so // original arquivado em CONCLUIDA
	sn := novo.Snapshot()
	// O novo herda o episódio do original (para ListarPorEpisodio).
	f.episodioDe[sn.RequisicaoID] = f.episodioDe[so.RequisicaoID]
	return f.inserir(sn), nil
}
```

2. No fim do ficheiro, acrescentar os fakes de contacto e notificação:

```go
// fakeResolvedorContacto devolve um telefone fixo.
type fakeResolvedorContacto struct {
	telefone string
	ok       bool
	err      error
}

func (f *fakeResolvedorContacto) ContactoClinico(_ context.Context, _ string) (string, bool, error) {
	return f.telefone, f.ok, f.err
}

// fakeNotificadorCritico recolhe os telefones notificados.
type fakeNotificadorCritico struct {
	enviados []string
	err      error
}

func (f *fakeNotificadorCritico) NotificarValorCritico(_ context.Context, telefone, _, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.enviados = append(f.enviados, telefone)
	return nil
}
```

- [ ] **Step 4: Escrever o teste de visibilidade e correr**

Criar `internal/application/laboratorio/resultados_test.go`:

```go
package laboratorio_test

import (
	"testing"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
)

// TestEstadosVisiveisAoMedico_SoValidada fixa a regra de visibilidade do Sprint 13:
// o médico vê o resultado vigente (VALIDADA) e não o preliminar (PROCESSADA) nem o
// arquivado por correcção (CONCLUIDA).
func TestEstadosVisiveisAoMedico_SoValidada(t *testing.T) {
	vis := applaboratorio.EstadosVisiveisAoMedico
	if len(vis) != 1 || vis[0] != dominio.ResValidada {
		t.Fatalf("a leitura clínica deve mostrar só VALIDADA, veio %+v", vis)
	}
}
```

Run: `go test ./internal/application/laboratorio/ -run TestEstadosVisiveisAoMedico -v`
Expected: PASS. (E o pacote de testes compila com os novos fakes.)

- [ ] **Step 5: Confirmar que todo o pacote de aplicação ainda compila e passa**

Run: `go test ./internal/application/laboratorio/... -v`
Expected: PASS (os testes existentes continuam verdes; nenhum dependia de `ResConcluida` estar visível).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/laboratorio/resultado.go internal/application/laboratorio/ports.go internal/application/laboratorio/fakes_test.go internal/application/laboratorio/resultados_test.go
git commit -m "feat(laboratorio): portas de correcção/contacto/SMS e visibilidade clínica só do resultado vigente"
```

---

### Task 6: `CasoValidarResultado` (aplicação) + alerta de valor crítico

**Files:**
- Create: `internal/application/laboratorio/validacao.go`
- Create: `internal/application/laboratorio/notificacao.go`
- Test: `internal/application/laboratorio/validacao_test.go`

**Interfaces:**
- Consumes: `dominio.RepositorioResultados`, `dominio.RepositorioRequisicoes`, `dominio.RepositorioAnalises`, `ResolvedorContacto`, `NotificadorCritico`, `Auditor`; `Resultado.Validar/Valor/RequisicaoID/Snapshot`, `Analise.AvaliarCritico`, `RequisicaoLab.Snapshot().MedicoRequisitanteID`.
- Produces:
  - `func NovoCasoValidarResultado(res dominio.RepositorioResultados, req dominio.RepositorioRequisicoes, an dominio.RepositorioAnalises, c ResolvedorContacto, n NotificadorCritico, a Auditor) *CasoValidarResultado`
  - `func (uc *CasoValidarResultado) Executar(ctx context.Context, actor, resultadoID string) (DetalheResultado, error)`
  - helper `alertarValorCritico(...)` (usado também pela Task 7).

- [ ] **Step 1: Escrever os testes que falham**

Criar `internal/application/laboratorio/validacao_test.go`:

```go
package laboratorio_test

import (
	"context"
	"testing"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// cenarioValidacao prepara um resultado PROCESSADA na BD em memória, com o catálogo e
// a requisição correspondentes, e devolve os fakes e o id do resultado.
func cenarioValidacao(t *testing.T, criticos []dominio.ValorCritico, valor string) (
	*fakeResultados, *fakeRequisicoes, *fakeAnalises, *fakeResolvedorContacto, *fakeNotificadorCritico, *fakeAuditor, string,
) {
	t.Helper()
	analises := novoFakeAnalises()
	hb, err := dominio.NovaAnalise("HB", "Hemoglobina", "g/dL", nil, criticos)
	if err != nil {
		t.Fatalf("análise: %v", err)
	}
	_ = analises.Guardar(context.Background(), hb)

	resultados := novoFakeResultados()
	requisicoes := novoFakeRequisicoes(resultados)
	req, _ := dominio.NovaRequisicao(dominio.DadosNovaRequisicao{
		EpisodioID: "ep-1", DoenteID: "doe-1", MedicoRequisitanteID: "med-1",
		Prioridade: dominio.PrioridadeRotina, Itens: []dominio.ItemRequisicao{{CodigoAnalise: "HB"}},
	})
	res, _ := dominio.NovoResultado("por-atribuir", "HB", "g/dL")
	reqID, _ := requisicoes.Emitir(context.Background(), req, []*dominio.Resultado{res})

	// Levar o único resultado até PROCESSADA (colher + submeter) via os fakes.
	var resID string
	fila, _ := resultados.ListarFila(context.Background(), []dominio.EstadoResultado{dominio.ResPendente})
	for _, r := range fila {
		if r.RequisicaoID == reqID {
			resID = r.ID
		}
	}
	r0, _ := resultados.ObterPorID(context.Background(), resID)
	_ = r0.ColherAmostra("tec-1", agoraFixo())
	_ = resultados.Transitar(context.Background(), r0)
	r1, _ := resultados.ObterPorID(context.Background(), resID)
	_ = r1.SubmeterPreliminar("tec-1", valor, "", agoraFixo())
	_ = resultados.Transitar(context.Background(), r1)

	return resultados, requisicoes, analises,
		&fakeResolvedorContacto{telefone: "+244923000000", ok: true},
		&fakeNotificadorCritico{}, &fakeAuditor{}, resID
}

func TestValidarResultado_CriticoNotificaEAudita(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t,
		[]dominio.ValorCritico{{Operador: dominio.CriticoMenor, Limite: 3.0, Descricao: "anemia grave"}}, "2.5")

	uc := applaboratorio.NovoCasoValidarResultado(res, req, an, contactos, notif, aud)
	out, err := uc.Executar(context.Background(), "pat-9", resID)
	if err != nil {
		t.Fatalf("validar: %v", err)
	}
	if out.Estado != string(dominio.ResValidada) || !out.ValorCritico {
		t.Fatalf("esperava VALIDADA crítica, veio %+v", out)
	}
	if len(notif.enviados) != 1 || notif.enviados[0] != "+244923000000" {
		t.Fatalf("esperava 1 SMS ao médico, veio %+v", notif.enviados)
	}
	if !aud.tem("laboratorio.resultado.validado") || !aud.tem("laboratorio.valor_critico.notificado") {
		t.Fatalf("faltam registos de auditoria: %+v", aud.registos)
	}
}

func TestValidarResultado_NaoCritico_NaoNotifica(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t,
		[]dominio.ValorCritico{{Operador: dominio.CriticoMenor, Limite: 3.0, Descricao: "x"}}, "12.5")

	uc := applaboratorio.NovoCasoValidarResultado(res, req, an, contactos, notif, aud)
	if _, err := uc.Executar(context.Background(), "pat-9", resID); err != nil {
		t.Fatalf("validar: %v", err)
	}
	if len(notif.enviados) != 0 {
		t.Fatalf("um valor normal não devia disparar SMS, veio %+v", notif.enviados)
	}
	if aud.tem("laboratorio.valor_critico.notificado") {
		t.Fatal("um valor normal não devia auditar notificação de crítico")
	}
}

func TestValidarResultado_Segregacao_Bloqueia(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t, nil, "12.5")
	uc := applaboratorio.NovoCasoValidarResultado(res, req, an, contactos, notif, aud)
	// O submissor foi "tec-1" — validar como "tec-1" viola a segregação.
	_, err := uc.Executar(context.Background(), "tec-1", resID)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("auto-validação devia dar RegraNegocio, veio %v", err)
	}
}

func TestValidarResultado_SMSFalhado_NaoFalhaValidacao(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t,
		[]dominio.ValorCritico{{Operador: dominio.CriticoMenor, Limite: 3.0, Descricao: "x"}}, "2.5")
	notif.err = erros.Novo(erros.CategoriaInterno, "gateway em baixo")

	uc := applaboratorio.NovoCasoValidarResultado(res, req, an, contactos, notif, aud)
	out, err := uc.Executar(context.Background(), "pat-9", resID)
	if err != nil {
		t.Fatalf("uma falha de SMS não deve falhar a validação: %v", err)
	}
	if out.Estado != string(dominio.ResValidada) {
		t.Fatalf("o resultado devia ficar VALIDADA na mesma, veio %s", out.Estado)
	}
	// Mesmo com falha de envio, a tentativa é auditada.
	if !aud.tem("laboratorio.valor_critico.notificado") {
		t.Fatal("a tentativa de notificação devia ser auditada mesmo em falha")
	}
}
```

> Este ficheiro usa dois helpers de teste do pacote — `agoraFixo()`. Se ainda não existir no pacote de testes, criá-lo em `fakes_test.go`:
> ```go
> func agoraFixo() time.Time { return time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC) }
> ```
> (acrescentar `"time"` aos imports de `fakes_test.go`).

- [ ] **Step 2: Correr para confirmar que falha**

Run: `go test ./internal/application/laboratorio/ -run TestValidarResultado -v`
Expected: FAIL — `NovoCasoValidarResultado undefined`.

- [ ] **Step 3: Implementar o helper de notificação**

Criar `internal/application/laboratorio/notificacao.go`:

```go
package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// alertarValorCritico resolve o telefone do médico requisitante e envia o SMS de
// valor crítico, auditando sempre o resultado da tentativa. Best-effort: qualquer
// falha (resolver contacto, enviar, obter requisição) é engolida — a validação ou a
// correcção já estão persistidas e não devem falhar por causa de um alerta. É isto
// que cumpre "SMS auditado": a prova é o registo de auditoria, não a entrega.
func alertarValorCritico(
	ctx context.Context,
	requisicoes dominio.RepositorioRequisicoes,
	contactos ResolvedorContacto,
	notificador NotificadorCritico,
	auditor Auditor,
	agora func() time.Time,
	actor, resultadoID, requisicaoID, codigoAnalise, valor string,
) {
	detalhe := ""
	req, err := requisicoes.ObterPorID(ctx, requisicaoID)
	switch {
	case err != nil:
		detalhe = "falha: requisição " + requisicaoID + " não encontrada"
	default:
		medicoID := req.Snapshot().MedicoRequisitanteID
		telefone, ok, cErr := contactos.ContactoClinico(ctx, medicoID)
		switch {
		case cErr != nil:
			detalhe = "falha ao resolver contacto do médico " + medicoID
		case !ok || telefone == "":
			detalhe = "médico " + medicoID + " sem telefone registado; alerta não enviado"
		default:
			if nErr := notificador.NotificarValorCritico(ctx, telefone, codigoAnalise, valor); nErr != nil {
				detalhe = "falha no envio do SMS ao médico " + medicoID
			} else {
				detalhe = "SMS enviado ao médico " + medicoID
			}
		}
	}
	_ = auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.valor_critico.notificado",
		Entidade: "resultado", EntidadeID: resultadoID, OcorridoEm: agora(),
		Detalhe: detalhe,
	})
}
```

- [ ] **Step 4: Implementar o caso de uso**

Criar `internal/application/laboratorio/validacao.go`:

```go
package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoValidarResultado valida o preliminar (PROCESSADA → VALIDADA) com segregação de
// funções, avalia o valor crítico contra o catálogo e, se crítico, notifica o médico
// requisitante por SMS (best-effort, auditado).
type CasoValidarResultado struct {
	resultados  dominio.RepositorioResultados
	requisicoes dominio.RepositorioRequisicoes
	analises    dominio.RepositorioAnalises
	contactos   ResolvedorContacto
	notificador NotificadorCritico
	auditor     Auditor
	agora       func() time.Time
}

// NovoCasoValidarResultado constrói o caso de uso.
func NovoCasoValidarResultado(
	res dominio.RepositorioResultados, req dominio.RepositorioRequisicoes,
	an dominio.RepositorioAnalises, c ResolvedorContacto, n NotificadorCritico, a Auditor,
) *CasoValidarResultado {
	return &CasoValidarResultado{
		resultados: res, requisicoes: req, analises: an,
		contactos: c, notificador: n, auditor: a, agora: time.Now,
	}
}

// Executar valida o resultado. O validador é o sujeito autenticado (nunca do corpo).
func (uc *CasoValidarResultado) Executar(ctx context.Context, actor, resultadoID string) (DetalheResultado, error) {
	res, err := uc.resultados.ObterPorID(ctx, resultadoID)
	if err != nil {
		return DetalheResultado{}, err
	}
	an, err := uc.analises.ObterPorCodigo(ctx, res.Snapshot().CodigoAnalise)
	if err != nil {
		return DetalheResultado{}, err
	}
	critico := an.AvaliarCritico(res.Valor())
	if err := res.Validar(actor, critico, uc.agora()); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.resultados.Transitar(ctx, res); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.resultado.validado",
		Entidade: "resultado", EntidadeID: resultadoID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheResultado{}, err
	}
	if critico {
		s := res.Snapshot()
		alertarValorCritico(ctx, uc.requisicoes, uc.contactos, uc.notificador, uc.auditor,
			uc.agora, actor, s.ID, s.RequisicaoID, s.CodigoAnalise, s.Valor)
	}
	return paraDetalheResultado(res), nil
}
```

- [ ] **Step 5: Correr os testes**

Run: `go test ./internal/application/laboratorio/ -run TestValidarResultado -v`
Expected: PASS (os quatro).

- [ ] **Step 6: Commit**

```bash
git add internal/application/laboratorio/validacao.go internal/application/laboratorio/notificacao.go internal/application/laboratorio/validacao_test.go internal/application/laboratorio/fakes_test.go
git commit -m "feat(laboratorio): caso de uso de validação com detecção de valor crítico e SMS auditado"
```

---

### Task 7: `CasoCorrigirResultado` (aplicação)

**Files:**
- Create: `internal/application/laboratorio/correccao.go`
- Test: `internal/application/laboratorio/correccao_test.go`

**Interfaces:**
- Consumes: os mesmos repositórios/portas da Task 6; `Resultado.Corrigir`, `RepositorioResultados.Corrigir`, `alertarValorCritico`.
- Produces:
  - `func NovoCasoCorrigirResultado(res dominio.RepositorioResultados, req dominio.RepositorioRequisicoes, an dominio.RepositorioAnalises, c ResolvedorContacto, n NotificadorCritico, a Auditor) *CasoCorrigirResultado`
  - `func (uc *CasoCorrigirResultado) Executar(ctx context.Context, actor, resultadoID string, dados DadosCorrigirResultado) (DetalheResultado, error)`

- [ ] **Step 1: Escrever os testes que falham**

Criar `internal/application/laboratorio/correccao_test.go`:

```go
package laboratorio_test

import (
	"context"
	"testing"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// validarNoFake leva o resultado de PROCESSADA a VALIDADA usando o caso de uso real.
func validarNoFake(t *testing.T, res *fakeResultados, req *fakeRequisicoes, an *fakeAnalises, resID string) {
	t.Helper()
	uc := applaboratorio.NovoCasoValidarResultado(res, req, an,
		&fakeResolvedorContacto{ok: false}, &fakeNotificadorCritico{}, &fakeAuditor{})
	if _, err := uc.Executar(context.Background(), "pat-2", resID); err != nil {
		t.Fatalf("preparar VALIDADA: %v", err)
	}
}

func TestCorrigirResultado_CriaNovoEArquivaOriginal(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t, nil, "2.5")
	validarNoFake(t, res, req, an, resID)

	uc := applaboratorio.NovoCasoCorrigirResultado(res, req, an, contactos, notif, aud)
	out, err := uc.Executar(context.Background(), "pat-3", resID,
		applaboratorio.DadosCorrigirResultado{Valor: "12.5", Observacoes: "releitura"})
	if err != nil {
		t.Fatalf("corrigir: %v", err)
	}
	// A resposta é o novo resultado vigente.
	if out.Estado != string(dominio.ResValidada) || out.Valor != "12.5" || out.ID == resID {
		t.Fatalf("esperava um novo VALIDADA com valor 12.5, veio %+v", out)
	}
	// O original ficou CONCLUIDA.
	orig, _ := res.ObterPorID(context.Background(), resID)
	if orig.Estado() != dominio.ResConcluida {
		t.Fatalf("o original devia ficar CONCLUIDA, veio %s", orig.Estado())
	}
	if !aud.tem("laboratorio.resultado.corrigido") {
		t.Fatalf("faltou auditar a correcção: %+v", aud.registos)
	}
}

func TestCorrigirResultado_ReavaliaCriticoENotifica(t *testing.T) {
	// O original não era crítico (12.5); a correcção mete um valor crítico (2.5).
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t,
		[]dominio.ValorCritico{{Operador: dominio.CriticoMenor, Limite: 3.0, Descricao: "anemia grave"}}, "12.5")
	contactos.telefone, contactos.ok = "+244923111111", true
	validarNoFake(t, res, req, an, resID)

	uc := applaboratorio.NovoCasoCorrigirResultado(res, req, an, contactos, notif, aud)
	out, err := uc.Executar(context.Background(), "pat-3", resID,
		applaboratorio.DadosCorrigirResultado{Valor: "2.5"})
	if err != nil {
		t.Fatalf("corrigir: %v", err)
	}
	if !out.ValorCritico {
		t.Fatal("a correcção com valor crítico devia marcar o novo resultado como crítico")
	}
	if len(notif.enviados) != 1 {
		t.Fatalf("a correcção crítica devia notificar por SMS, veio %+v", notif.enviados)
	}
}

func TestCorrigirResultado_Segregacao(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t, nil, "2.5")
	validarNoFake(t, res, req, an, resID)

	uc := applaboratorio.NovoCasoCorrigirResultado(res, req, an, contactos, notif, aud)
	// "tec-1" foi o submissor original — não pode corrigir.
	_, err := uc.Executar(context.Background(), "tec-1", resID,
		applaboratorio.DadosCorrigirResultado{Valor: "12.5"})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("correcção pelo submissor original devia dar RegraNegocio, veio %v", err)
	}
}
```

- [ ] **Step 2: Correr para confirmar que falha**

Run: `go test ./internal/application/laboratorio/ -run TestCorrigirResultado -v`
Expected: FAIL — `NovoCasoCorrigirResultado undefined`.

- [ ] **Step 3: Implementar o caso de uso**

Criar `internal/application/laboratorio/correccao.go`:

```go
package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoCorrigirResultado corrige um resultado validado: arquiva o original em
// CONCLUIDA e cria um novo VALIDADA que o substitui, reavaliando o valor crítico
// contra o catálogo e notificando o médico se o valor corrigido for crítico.
type CasoCorrigirResultado struct {
	resultados  dominio.RepositorioResultados
	requisicoes dominio.RepositorioRequisicoes
	analises    dominio.RepositorioAnalises
	contactos   ResolvedorContacto
	notificador NotificadorCritico
	auditor     Auditor
	agora       func() time.Time
}

// NovoCasoCorrigirResultado constrói o caso de uso.
func NovoCasoCorrigirResultado(
	res dominio.RepositorioResultados, req dominio.RepositorioRequisicoes,
	an dominio.RepositorioAnalises, c ResolvedorContacto, n NotificadorCritico, a Auditor,
) *CasoCorrigirResultado {
	return &CasoCorrigirResultado{
		resultados: res, requisicoes: req, analises: an,
		contactos: c, notificador: n, auditor: a, agora: time.Now,
	}
}

// Executar corrige o resultado. O corrector é o sujeito autenticado (nunca do corpo).
func (uc *CasoCorrigirResultado) Executar(ctx context.Context, actor, resultadoID string, dados DadosCorrigirResultado) (DetalheResultado, error) {
	original, err := uc.resultados.ObterPorID(ctx, resultadoID)
	if err != nil {
		return DetalheResultado{}, err
	}
	an, err := uc.analises.ObterPorCodigo(ctx, original.Snapshot().CodigoAnalise)
	if err != nil {
		return DetalheResultado{}, err
	}
	critico := an.AvaliarCritico(dados.Valor)
	novo, err := original.Corrigir(actor, dados.Valor, dados.Observacoes, critico, uc.agora())
	if err != nil {
		return DetalheResultado{}, err
	}
	novoID, err := uc.resultados.Corrigir(ctx, novo, original)
	if err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.resultado.corrigido",
		Entidade: "resultado", EntidadeID: novoID, OcorridoEm: uc.agora(),
		Detalhe: "corrige o resultado " + resultadoID,
	}); err != nil {
		return DetalheResultado{}, err
	}
	sn := novo.Snapshot()
	sn.ID = novoID
	if critico {
		alertarValorCritico(ctx, uc.requisicoes, uc.contactos, uc.notificador, uc.auditor,
			uc.agora, actor, novoID, sn.RequisicaoID, sn.CodigoAnalise, sn.Valor)
	}
	return paraDetalheResultado(dominio.ReconstruirResultado(sn)), nil
}
```

- [ ] **Step 4: Correr os testes**

Run: `go test ./internal/application/laboratorio/ -run TestCorrigirResultado -v`
Expected: PASS (os três).

- [ ] **Step 5: Correr toda a suite de aplicação + gate de cobertura**

Run: `go test ./internal/application/laboratorio/... -cover`
Expected: PASS, cobertura ≥75%.

- [ ] **Step 6: Commit**

```bash
git add internal/application/laboratorio/correccao.go internal/application/laboratorio/correccao_test.go
git commit -m "feat(laboratorio): caso de uso de correcção (novo resultado, reavaliação de crítico)"
```

---

### Task 8: Migração 0003 + pgrepo (`Transitar` estendido + `Corrigir`) + integração

**Files:**
- Create: `migrations/laboratorio/0003_correccao_resultados.sql`
- Modify: `internal/adapters/pgrepo/resultados_repo.go`
- Test: `tests/integration/laboratorio_test.go` (acrescentar casos)

**Interfaces:**
- Consumes: `dominio.RepositorioResultados` (agora com `Corrigir`), `SnapshotResultado.CorrigeResultadoID/PatologistaValidadorID/ValidadaEm/ValorCritico`.
- Produces: implementação pgx de `Corrigir`; `Transitar` passa a persistir `patologista_validador_id`, `validada_em`, `valor_critico`.

- [ ] **Step 1: Escrever a migração**

Criar `migrations/laboratorio/0003_correccao_resultados.sql`:

```sql
-- Bounded Context: laboratorio
-- Migration forward-only. Correcção de resultados (Sprint 13).
--
-- corrige_resultado_id liga o resultado corrigido (VALIDADA vigente) ao original que
-- substitui (arquivado em CONCLUIDA). É uma referência DENTRO do mesmo bounded
-- context, logo com FK. As CHECK de coerência estado↔timestamps↔autores e a CHECK de
-- segregação de funções já existem desde a migração 0002 e cobrem VALIDADA/CONCLUIDA.

ALTER TABLE laboratorio.resultados
    ADD COLUMN IF NOT EXISTS corrige_resultado_id uuid NULL REFERENCES laboratorio.resultados(id);

CREATE INDEX IF NOT EXISTS idx_resultados_corrige
    ON laboratorio.resultados (corrige_resultado_id);
```

- [ ] **Step 2: Estender `Transitar` e implementar `Corrigir` no repositório**

Em `internal/adapters/pgrepo/resultados_repo.go`:

1. Substituir a query e o `Exec` de `Transitar` por (acrescenta as três colunas de validação):

```go
	const q = `
UPDATE laboratorio.resultados SET
    estado=$2, valor=NULLIF($3,''), observacoes=NULLIF($4,''), motivo_recusa=NULLIF($5,''),
    tecnico_colheita_id=NULLIF($6,'')::uuid, tecnico_submissor_id=NULLIF($7,'')::uuid,
    colhida_em=$8, submetida_em=$9,
    patologista_validador_id=NULLIF($11,'')::uuid, validada_em=$12, valor_critico=$13
WHERE id=$1 AND estado=$10`
	ct, err := r.pool.Exec(ctx, q, s.ID, string(s.Estado), s.Valor, s.Observacoes, s.MotivoRecusa,
		s.TecnicoColheitaID, s.TecnicoSubmissorID, s.ColhidaEm, s.SubmetidaEm,
		string(s.EstadoAnterior), s.PatologistaValidadorID, s.ValidadaEm, s.ValorCritico)
```

2. A seguir a `Transitar` (antes de `erroTransicaoFalhada`), acrescentar:

```go
// Corrigir persiste a correcção de um resultado validado numa única transacção: o
// original transita VALIDADA → CONCLUIDA (compare-and-set) e o novo resultado é
// inserido em VALIDADA a apontar-lhe via corrige_resultado_id. Qualquer falha faz
// rollback de ambos; devolve o id do novo resultado.
func (r *RepositorioResultados) Corrigir(ctx context.Context, novo, original *dominio.Resultado) (string, error) {
	so := original.Snapshot()
	sn := novo.Snapshot()
	if so.ID == "" {
		return "", erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção de correcção: %w", err)
	}
	defer tx.Rollback(ctx)

	const arquivar = `UPDATE laboratorio.resultados SET estado=$2 WHERE id=$1 AND estado=$3`
	ct, err := tx.Exec(ctx, arquivar, so.ID, string(dominio.ResConcluida), string(so.EstadoAnterior))
	if err != nil {
		return "", fmt.Errorf("arquivar resultado original: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", r.erroTransicaoFalhada(ctx, so.ID)
	}

	const inserir = `
INSERT INTO laboratorio.resultados
    (requisicao_id, codigo_analise, valor, unidade, observacoes, estado,
     tecnico_colheita_id, tecnico_submissor_id, patologista_validador_id,
     colhida_em, submetida_em, validada_em, valor_critico, corrige_resultado_id)
VALUES ($1,$2,NULLIF($3,''),$4,NULLIF($5,''),$6,
        NULLIF($7,'')::uuid, NULLIF($8,'')::uuid, NULLIF($9,'')::uuid,
        $10,$11,$12,$13,$14::uuid)
RETURNING id::text`
	var novoID string
	if err := tx.QueryRow(ctx, inserir,
		sn.RequisicaoID, sn.CodigoAnalise, sn.Valor, sn.Unidade, sn.Observacoes, string(sn.Estado),
		sn.TecnicoColheitaID, sn.TecnicoSubmissorID, sn.PatologistaValidadorID,
		sn.ColhidaEm, sn.SubmetidaEm, sn.ValidadaEm, sn.ValorCritico, sn.CorrigeResultadoID,
	).Scan(&novoID); err != nil {
		return "", fmt.Errorf("inserir resultado corrigido: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar correcção: %w", err)
	}
	return novoID, nil
}
```

3. Confirmar que `ObterPorID` continua a devolver `corrige_resultado_id`? Não é necessário para os fluxos — o `SnapshotResultado.CorrigeResultadoID` fica vazio na leitura (não usado a jusante). Deixar o `SELECT` de `ObterPorID` como está.

- [ ] **Step 3: Verificar a compilação**

Run: `go build ./...`
Expected: sem erros (o `var _ dominio.RepositorioResultados = (*RepositorioResultados)(nil)` no fim do ficheiro passa a exigir `Corrigir`, agora presente).

- [ ] **Step 4: Escrever o teste de integração**

Acrescentar a `tests/integration/laboratorio_test.go` uma constante de patologista e um teste. Junto das constantes de topo (`medicoLabID`, ...), acrescentar:

```go
const patologistaLabID = "00000000-0000-4000-8000-0000000000f4"
```

E o teste (usa os repositórios reais e prova validação, segregação na BD e correcção atómica):

```go
// TestLaboratorio_ValidacaoECorreccao fecha o ciclo do Sprint 13 contra Postgres:
// PROCESSADA → VALIDADA (com as colunas de validação persistidas), a CHECK de
// segregação da BD a negar validador == submissor, e a correcção a arquivar o
// original (CONCLUIDA) e criar um novo VALIDADA na mesma transacção.
func TestLaboratorio_ValidacaoECorreccao(t *testing.T) {
	pool, ctx := ligar(t)
	migrarLaboratorio(t, pool, ctx)

	repoAnalises := pgrepo.NovoRepositorioAnalises(pool)
	repoRequisicoes := pgrepo.NovoRepositorioRequisicoes(pool)
	repoResultados := pgrepo.NovoRepositorioResultados(pool)

	hb, err := repoAnalises.ObterPorCodigo(ctx, "HB")
	if err != nil {
		t.Fatalf("obter HB: %v", err)
	}
	doenteID, episodioID := fixturaLaboratorio(t, pool, ctx, "12345678LA130", "Elsa Laboratório")
	req, _ := dominio.NovaRequisicao(dominio.DadosNovaRequisicao{
		EpisodioID: episodioID, DoenteID: doenteID, MedicoRequisitanteID: medicoLabID,
		Prioridade: dominio.PrioridadeRotina, Itens: []dominio.ItemRequisicao{{CodigoAnalise: "HB"}},
	})
	res0, _ := dominio.NovoResultado("por-atribuir", "HB", hb.Unidade())
	reqID, err := repoRequisicoes.Emitir(ctx, req, []*dominio.Resultado{res0})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	fila, _ := repoResultados.ListarFila(ctx, []dominio.EstadoResultado{dominio.ResPendente})
	var id string
	for _, r := range fila {
		if r.RequisicaoID == reqID {
			id = r.ID
		}
	}
	if id == "" {
		t.Fatal("não encontrei o resultado PENDENTE emitido")
	}

	// Colher e submeter (submissor = tecnicoLabID).
	res, _ := repoResultados.ObterPorID(ctx, id)
	_ = res.ColherAmostra(tecnicoLabID, time.Now())
	_ = repoResultados.Transitar(ctx, res)
	res, _ = repoResultados.ObterPorID(ctx, id)
	_ = res.SubmeterPreliminar(tecnicoLabID, "2.5", "", time.Now())
	if err := repoResultados.Transitar(ctx, res); err != nil {
		t.Fatalf("submeter: %v", err)
	}

	// Segregação na BD: validar como o próprio submissor viola a CHECK. Constrói-se
	// um agregado rehidratado em PROCESSADA e força-se validador == submissor por
	// reconstrução (o domínio recusá-lo-ia, por isso vai-se pela reconstrução).
	validada := time.Now()
	autoValidado := dominio.ReconstruirResultado(dominio.SnapshotResultado{
		ID: id, RequisicaoID: reqID, CodigoAnalise: "HB", Valor: "2.5", Unidade: hb.Unidade(),
		Estado: dominio.ResValidada, TecnicoSubmissorID: tecnicoLabID,
		PatologistaValidadorID: tecnicoLabID, ValidadaEm: &validada,
	})
	// EstadoAnterior de um agregado rehidratado é o próprio Estado; forçamos a
	// partida em PROCESSADA reconstruindo de novo com o estado de leitura.
	autoValidado = dominio.ReconstruirResultado(dominio.SnapshotResultado{
		ID: id, RequisicaoID: reqID, CodigoAnalise: "HB", Valor: "2.5", Unidade: hb.Unidade(),
		Estado: dominio.ResProcessada, TecnicoSubmissorID: tecnicoLabID,
	})
	_ = autoValidado.Validar(patologistaLabID, false, validada) // válido no domínio (pat != tec)
	// mas agora sobrepomos o validador para == submissor, só para testar a CHECK:
	sobreposto := autoValidado.Snapshot()
	sobreposto.PatologistaValidadorID = tecnicoLabID
	if err := repoResultados.Transitar(ctx, dominio.ReconstruirResultado(sobreposto)); err == nil {
		t.Fatal("a CHECK de segregação da BD devia negar validador == submissor")
	}

	// Validação legítima (patologista distinto).
	res, _ = repoResultados.ObterPorID(ctx, id)
	if err := res.Validar(patologistaLabID, true, time.Now()); err != nil {
		t.Fatalf("validar no domínio: %v", err)
	}
	if err := repoResultados.Transitar(ctx, res); err != nil {
		t.Fatalf("persistir validação: %v", err)
	}
	relido, _ := repoResultados.ObterPorID(ctx, id)
	if relido.Estado() != dominio.ResValidada {
		t.Fatalf("esperava VALIDADA na BD, veio %s", relido.Estado())
	}

	// Correcção: novo VALIDADA + original CONCLUIDA, atómico.
	novo, err := relido.Corrigir(patologistaLabID, "12.5", "releitura", false, time.Now())
	if err != nil {
		t.Fatalf("corrigir no domínio: %v", err)
	}
	novoID, err := repoResultados.Corrigir(ctx, novo, relido)
	if err != nil {
		t.Fatalf("persistir correcção: %v", err)
	}
	origFinal, _ := repoResultados.ObterPorID(ctx, id)
	novoFinal, _ := repoResultados.ObterPorID(ctx, novoID)
	if origFinal.Estado() != dominio.ResConcluida {
		t.Fatalf("o original devia ficar CONCLUIDA, veio %s", origFinal.Estado())
	}
	if novoFinal.Estado() != dominio.ResValidada || novoFinal.Valor() != "12.5" {
		t.Fatalf("o novo devia ser VALIDADA com 12.5, veio %s/%s", novoFinal.Estado(), novoFinal.Valor())
	}
}
```

- [ ] **Step 5: Correr a integração (precisa de Postgres)**

Run: `DATABASE_URL="$DATABASE_URL" go test -tags integration ./tests/integration/ -run TestLaboratorio_ValidacaoECorreccao -v`
Expected: PASS se `DATABASE_URL` estiver definido; SKIP caso contrário. Se não houver Postgres à mão, correr ao menos `go vet -tags integration ./tests/integration/` para garantir que compila.

- [ ] **Step 6: Correr a suite de integração do laboratório completa (regressão)**

Run: `DATABASE_URL="$DATABASE_URL" go test -tags integration ./tests/integration/ -run TestLaboratorio -v`
Expected: PASS/SKIP — os testes do Sprint 12 continuam verdes (o `Transitar` estendido é retro-compatível: para transições pré-validação escreve NULL/false nas novas colunas).

- [ ] **Step 7: Commit**

```bash
git add migrations/laboratorio/0003_correccao_resultados.sql internal/adapters/pgrepo/resultados_repo.go tests/integration/laboratorio_test.go
git commit -m "feat(laboratorio): migração 0003 + pgrepo da validação e correcção (Transitar estendido, Corrigir transaccional)"
```

---

### Task 9: Adaptadores de saída — SMS + resolvedor de contacto + config

**Files:**
- Create: `internal/adapters/sms/notificador.go`
- Create: `internal/adapters/sms/nulo.go`
- Create: `internal/adapters/laboratorio/resolvedor_contacto.go`
- Test: `internal/adapters/laboratorio/resolvedor_contacto_test.go`
- Modify: `internal/platform/config/config.go`

**Interfaces:**
- Consumes: `applaboratorio.NotificadorCritico`, `applaboratorio.ResolvedorContacto`, `identidade.RepositorioUtilizadores` (`ObterPorID(ctx, keycloakID) (*Utilizador, error)`; `Utilizador.Telefone string`).
- Produces:
  - `sms.NovoNotificadorSMS(endpoint, remetente string) *NotificadorSMS`
  - `sms.NovoNotificadorNulo(log *slog.Logger) NotificadorNulo`
  - `laboratorio.NovoResolvedorContacto(u identidade.RepositorioUtilizadores) *ResolvedorContacto`
  - `config.Config.SMSEndpoint string`, `config.Config.SMSRemetente string`.

- [ ] **Step 1: Acrescentar os campos de config**

Em `internal/platform/config/config.go`:

1. No struct `Config`, após `SMTPRemetente string ...`:
```go
	SMSEndpoint               string        // endpoint HTTP do gateway SMS (vazio → notificador no-op)
	SMSRemetente              string        // remetente (sender id) das mensagens SMS
```

2. Em `Carregar` (ou equivalente), após a linha `SMTPRemetente: valorOu("SMTP_FROM", ...),`:
```go
		SMSEndpoint:               os.Getenv("SMS_ENDPOINT"),
		SMSRemetente:              valorOu("SMS_FROM", "SGC"),
```

- [ ] **Step 2: Escrever o teste do resolvedor de contacto**

Criar `internal/adapters/laboratorio/resolvedor_contacto_test.go`:

```go
package laboratorio_test

import (
	"context"
	"testing"

	adlaboratorio "github.com/ivandrosilva12/sgcfinal/internal/adapters/laboratorio"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeUtilizadores implementa identidade.RepositorioUtilizadores para o teste.
type fakeUtilizadores struct {
	u   *identidade.Utilizador
	err error
}

func (f fakeUtilizadores) ObterPorID(_ context.Context, _ string) (*identidade.Utilizador, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.u, nil
}
func (f fakeUtilizadores) GuardarComPapeis(_ context.Context, _ *identidade.Utilizador) error { return nil }
func (f fakeUtilizadores) AtualizarContacto(_ context.Context, _, _, _ string) error         { return nil }

func TestResolvedorContacto_ComTelefone(t *testing.T) {
	r := adlaboratorio.NovoResolvedorContacto(fakeUtilizadores{u: &identidade.Utilizador{Telefone: "+244 923 000 000"}})
	tel, ok, err := r.ContactoClinico(context.Background(), "kc-1")
	if err != nil || !ok || tel != "+244 923 000 000" {
		t.Fatalf("esperava o telefone, veio tel=%q ok=%v err=%v", tel, ok, err)
	}
}

func TestResolvedorContacto_SemTelefone(t *testing.T) {
	r := adlaboratorio.NovoResolvedorContacto(fakeUtilizadores{u: &identidade.Utilizador{Telefone: ""}})
	_, ok, err := r.ContactoClinico(context.Background(), "kc-1")
	if err != nil || ok {
		t.Fatalf("sem telefone devia dar ok=false sem erro, veio ok=%v err=%v", ok, err)
	}
}

func TestResolvedorContacto_Inexistente(t *testing.T) {
	r := adlaboratorio.NovoResolvedorContacto(fakeUtilizadores{err: erros.Novo(erros.CategoriaNaoEncontrado, "não existe")})
	_, ok, err := r.ContactoClinico(context.Background(), "kc-1")
	if err != nil || ok {
		t.Fatalf("utilizador inexistente devia dar ok=false sem erro, veio ok=%v err=%v", ok, err)
	}
}
```

- [ ] **Step 3: Correr para confirmar que falha**

Run: `go test ./internal/adapters/laboratorio/ -run TestResolvedorContacto -v`
Expected: FAIL — `NovoResolvedorContacto undefined`.

- [ ] **Step 4: Implementar o resolvedor de contacto**

Criar `internal/adapters/laboratorio/resolvedor_contacto.go`:

```go
package laboratorio

import (
	"context"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// ResolvedorContacto implementa applaboratorio.ResolvedorContacto lendo o telefone de
// um utilizador no BC Identidade. É trabalho de ACL: o adaptador pode conhecer
// Identidade — o domínio/aplicação do Laboratório não.
type ResolvedorContacto struct {
	utilizadores identidade.RepositorioUtilizadores
}

// NovoResolvedorContacto constrói o adaptador sobre o repositório de utilizadores.
func NovoResolvedorContacto(u identidade.RepositorioUtilizadores) *ResolvedorContacto {
	return &ResolvedorContacto{utilizadores: u}
}

// ContactoClinico devolve o telefone do utilizador. Um utilizador inexistente ou sem
// telefone devolve ok=false sem erro — para o alerta, "não sei o número" e "não há
// número" são a mesma resposta, e nunca fazem falhar a validação.
func (r *ResolvedorContacto) ContactoClinico(ctx context.Context, userID string) (string, bool, error) {
	u, err := r.utilizadores.ObterPorID(ctx, userID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return "", false, nil
		}
		return "", false, err
	}
	if u.Telefone == "" {
		return "", false, nil
	}
	return u.Telefone, true, nil
}

var _ applaboratorio.ResolvedorContacto = (*ResolvedorContacto)(nil)
```

- [ ] **Step 5: Implementar o notificador SMS e o no-op**

Criar `internal/adapters/sms/notificador.go`:

```go
// Package sms implementa o NotificadorCritico (application/laboratorio) por HTTP
// contra um gateway SMS. Camada 3 — Adaptadores. A integração com um gateway real de
// Angola fica para um marco posterior; em dev usa-se o notificador no-op.
package sms

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
)

// EnviarFunc é o transporte do SMS; extraído para substituição em testes.
type EnviarFunc func(ctx context.Context, telefone, mensagem string) error

// NotificadorSMS envia alertas por SMS através de um gateway HTTP.
type NotificadorSMS struct {
	endpoint  string
	remetente string
	// Enviar é o transporte; default enviarHTTP. Público para testes.
	Enviar EnviarFunc
}

// NovoNotificadorSMS constrói o adaptador apontado ao endpoint indicado.
func NovoNotificadorSMS(endpoint, remetente string) *NotificadorSMS {
	n := &NotificadorSMS{endpoint: endpoint, remetente: remetente}
	n.Enviar = n.enviarHTTP
	return n
}

// NotificarValorCritico compõe e envia a mensagem de valor crítico.
func (n *NotificadorSMS) NotificarValorCritico(ctx context.Context, telefone, codigoAnalise, valor string) error {
	msg := fmt.Sprintf("SGC: valor crítico na análise %s: %s. Contacte o laboratório.", codigoAnalise, valor)
	return n.Enviar(ctx, telefone, msg)
}

func (n *NotificadorSMS) enviarHTTP(ctx context.Context, telefone, mensagem string) error {
	form := url.Values{"from": {n.remetente}, "to": {telefone}, "text": {mensagem}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("preparar pedido SMS: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("enviar SMS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gateway SMS respondeu %d", resp.StatusCode)
	}
	return nil
}

var _ applaboratorio.NotificadorCritico = (*NotificadorSMS)(nil)
```

Criar `internal/adapters/sms/nulo.go`:

```go
package sms

import (
	"context"
	"log/slog"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
)

// NotificadorNulo é usado quando o SMS não está configurado: não envia nada, apenas
// regista em debug. Garante que a validação nunca falha por ausência de gateway SMS.
type NotificadorNulo struct{ log *slog.Logger }

// NovoNotificadorNulo constrói o notificador no-op (log opcional).
func NovoNotificadorNulo(log *slog.Logger) NotificadorNulo {
	return NotificadorNulo{log: log}
}

// NotificarValorCritico não envia; regista em debug.
func (n NotificadorNulo) NotificarValorCritico(_ context.Context, telefone, codigoAnalise, valor string) error {
	if n.log != nil {
		n.log.Debug("alerta de valor crítico suprimido (SMS não configurado)",
			"telefone", telefone, "analise", codigoAnalise, "valor", valor)
	}
	return nil
}

var _ applaboratorio.NotificadorCritico = NotificadorNulo{}
```

- [ ] **Step 6: Escrever um teste rápido do notificador SMS (seam de envio)**

Criar `internal/adapters/sms/notificador_test.go`:

```go
package sms_test

import (
	"context"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/sms"
)

func TestNotificadorSMS_UsaOTransporteEComoeAMensagem(t *testing.T) {
	n := sms.NovoNotificadorSMS("http://gateway.local", "SGC")
	var telefone, mensagem string
	n.Enviar = func(_ context.Context, tel, msg string) error {
		telefone, mensagem = tel, msg
		return nil
	}
	if err := n.NotificarValorCritico(context.Background(), "+244923000000", "HB", "2.5"); err != nil {
		t.Fatalf("notificar: %v", err)
	}
	if telefone != "+244923000000" {
		t.Fatalf("telefone errado: %q", telefone)
	}
	if mensagem == "" || mensagem[:4] != "SGC:" {
		t.Fatalf("mensagem inesperada: %q", mensagem)
	}
}
```

- [ ] **Step 7: Correr os testes dos adaptadores**

Run: `go test ./internal/adapters/laboratorio/ ./internal/adapters/sms/ -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/sms/ internal/adapters/laboratorio/resolvedor_contacto.go internal/adapters/laboratorio/resolvedor_contacto_test.go internal/platform/config/config.go
git commit -m "feat(laboratorio): adaptadores de SMS (gateway HTTP + no-op) e resolvedor de contacto via ACL"
```

---

### Task 10: Rotas HTTP de validação e correcção

**Files:**
- Modify: `internal/adapters/http/laboratorio_handler.go`
- Test: `internal/adapters/http/laboratorio_test.go`

**Interfaces:**
- Consumes: `applaboratorio.DetalheResultado`, `applaboratorio.DadosCorrigirResultado`, `dominio.PapelPatologista`, `SessaoDe`, `RBAC`, `responderErro`.
- Produces:
  - interfaces `ServicoValidarResultado`, `ServicoCorrigirResultado`
  - `NovoLaboratorioHandler(...)` passa a receber mais dois serviços (validar, corrigir) — **muda a assinatura**.
  - rotas `POST /api/v1/resultados/:rid/validacao` e `POST /api/v1/resultados/:rid/correccao` (RBAC `Patologista`).

- [ ] **Step 1: Escrever os testes que falham**

Em `internal/adapters/http/laboratorio_test.go`:

1. Acrescentar os duplos (a seguir a `duploSubmeter`):

```go
type duploValidar struct {
	actorRecebido string
}

func (d *duploValidar) Executar(_ context.Context, actor, id string) (applaboratorio.DetalheResultado, error) {
	d.actorRecebido = actor
	return applaboratorio.DetalheResultado{ID: id, Estado: string(dominio.ResValidada)}, nil
}

type duploCorrigir struct {
	actorRecebido string
	valorRecebido string
}

func (d *duploCorrigir) Executar(_ context.Context, actor, id string, dados applaboratorio.DadosCorrigirResultado) (applaboratorio.DetalheResultado, error) {
	d.actorRecebido = actor
	d.valorRecebido = dados.Valor
	return applaboratorio.DetalheResultado{ID: "res-novo", Estado: string(dominio.ResValidada), Valor: dados.Valor}, nil
}
```

2. Actualizar o helper `routerLab` para aceitar e injectar os dois novos serviços. Substituir a assinatura e a construção do handler:

```go
func routerLab(t *testing.T, emitir *duploEmitir, submeter *duploSubmeter,
	validar *duploValidar, corrigir *duploCorrigir, sessao identidade.Sessao) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := adhttp.NovoLaboratorioHandler(
		duploRegistarAnalise{}, duploListarAnalises{},
		emitir, duploObterRequisicao{}, duploListarRequisicoes{},
		duploColher{}, duploRecusar{}, submeter,
		duploListarFila{}, duploListarResultadosEpisodio{},
		validar, corrigir,
	)
	adhttp.RegistarLaboratorio(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}
```

3. **Actualizar todas as chamadas existentes a `routerLab`** neste ficheiro: cada `routerLab(t, X, Y, sessao)` passa a `routerLab(t, X, Y, &duploValidar{}, &duploCorrigir{}, sessao)`. (São ~25 chamadas; um find/replace de `, sessaoLabDe(` não serve — inserir `&duploValidar{}, &duploCorrigir{}, ` imediatamente antes de `sessaoLabDe(` em cada chamada a `routerLab`.)

4. Acrescentar os testes novos:

```go
func TestValidarResultado_SoPatologista(t *testing.T) {
	// Um médico não valida: 403.
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, &duploValidar{}, &duploCorrigir{},
		sessaoLabDe("med-1", identidade.PapelMedico))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/validacao", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("esperava 403 para o Medico na validação, veio %d", w.Code)
	}

	// O patologista valida, e o validador é o sujeito da sessão.
	validar := &duploValidar{}
	rp := routerLab(t, &duploEmitir{}, &duploSubmeter{}, validar, &duploCorrigir{},
		sessaoLabDe("pat-7", identidade.PapelPatologista))
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/v1/resultados/res-1/validacao", nil)
	rp.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("esperava 200 para o Patologista, veio %d (%s)", w2.Code, w2.Body.String())
	}
	if validar.actorRecebido != "pat-7" {
		t.Fatalf("o validador devia vir da sessão (pat-7), veio %q", validar.actorRecebido)
	}
}

func TestValidarResultado_TecnicoLab_Proibido(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, &duploValidar{}, &duploCorrigir{},
		sessaoLabDe("tec-1", identidade.PapelTecnicoLab))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/validacao", nil)
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("o técnico não valida (segregação): esperava 403, veio %d", w.Code)
	}
}

func TestCorrigirResultado_SoPatologista_UsaSessaoEValorDoCorpo(t *testing.T) {
	corrigir := &duploCorrigir{}
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, &duploValidar{}, corrigir,
		sessaoLabDe("pat-3", identidade.PapelPatologista))
	corpo, _ := json.Marshal(map[string]string{"valor": "12.5", "observacoes": "releitura"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/correccao", bytes.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperava 200 para o Patologista na correcção, veio %d (%s)", w.Code, w.Body.String())
	}
	if corrigir.actorRecebido != "pat-3" || corrigir.valorRecebido != "12.5" {
		t.Fatalf("correcção não usou sessão/corpo esperados: actor=%q valor=%q",
			corrigir.actorRecebido, corrigir.valorRecebido)
	}
}

func TestCorrigirResultado_CorpoMalformado_400(t *testing.T) {
	r := routerLab(t, &duploEmitir{}, &duploSubmeter{}, &duploValidar{}, &duploCorrigir{},
		sessaoLabDe("pat-1", identidade.PapelPatologista))
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/resultados/res-1/correccao", bytes.NewReader([]byte("{nao-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corpo malformado devia dar 400, veio %d", w.Code)
	}
}
```

- [ ] **Step 2: Correr para confirmar que falha**

Run: `go test ./internal/adapters/http/ -run 'TestValidarResultado|TestCorrigirResultado' -v`
Expected: FAIL — assinatura de `NovoLaboratorioHandler` não bate certo / rotas inexistentes.

- [ ] **Step 3: Estender o handler**

Em `internal/adapters/http/laboratorio_handler.go`:

1. No bloco de interfaces (após `ServicoListarResultadosDoEpisodio`), acrescentar:

```go
	// ServicoValidarResultado valida o resultado preliminar.
	ServicoValidarResultado interface {
		Executar(ctx context.Context, actor, resultadoID string) (applaboratorio.DetalheResultado, error)
	}
	// ServicoCorrigirResultado corrige um resultado validado.
	ServicoCorrigirResultado interface {
		Executar(ctx context.Context, actor, resultadoID string, dados applaboratorio.DadosCorrigirResultado) (applaboratorio.DetalheResultado, error)
	}
```

2. No struct `LaboratorioHandler`, acrescentar após `resultadosEpisodio`:

```go
	validar  ServicoValidarResultado
	corrigir ServicoCorrigirResultado
```

3. Na assinatura e no corpo de `NovoLaboratorioHandler`, acrescentar os dois parâmetros no fim e no literal:

```go
func NovoLaboratorioHandler(
	registarAnalise ServicoRegistarAnalise, listarAnalises ServicoListarAnalises,
	emitir ServicoEmitirRequisicao, obterRequisicao ServicoObterRequisicao,
	listarRequisicoes ServicoListarRequisicoes, colher ServicoColherAmostra,
	recusar ServicoRecusarAmostra, submeter ServicoSubmeterPreliminar,
	listarFila ServicoListarFila, resultadosEpisodio ServicoListarResultadosDoEpisodio,
	validar ServicoValidarResultado, corrigir ServicoCorrigirResultado,
) *LaboratorioHandler {
	return &LaboratorioHandler{
		registarAnalise: registarAnalise, listarAnalises: listarAnalises,
		emitir: emitir, obterRequisicao: obterRequisicao, listarRequisicoes: listarRequisicoes,
		colher: colher, recusar: recusar, submeter: submeter,
		listarFila: listarFila, resultadosEpisodio: resultadosEpisodio,
		validar: validar, corrigir: corrigir,
	}
}
```

4. Em `RegistarLaboratorio`, definir o RBAC do patologista e as rotas. Após `soTecnico := RBAC(dominio.PapelTecnicoLab)`:

```go
	soPatologista := RBAC(dominio.PapelPatologista)
```

E, no grupo `gres` (`/api/v1/resultados`), após as três rotas de técnico:

```go
	gres.POST("/:rid/validacao", soPatologista, h.validarResultadoHTTP)
	gres.POST("/:rid/correccao", soPatologista, h.corrigirResultadoHTTP)
```

5. Acrescentar o struct de corpo (junto de `corpoPreliminar`):

```go
type corpoCorreccao struct {
	Valor       string `json:"valor"`
	Observacoes string `json:"observacoes"`
}
```

6. Acrescentar os handlers (no fim do ficheiro):

```go
func (h *LaboratorioHandler) validarResultadoHTTP(c *gin.Context) {
	actor, _ := SessaoDe(c)
	out, err := h.validar.Executar(c.Request.Context(), actor.Sujeito, c.Param("rid"))
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}

func (h *LaboratorioHandler) corrigirResultadoHTTP(c *gin.Context) {
	var corpo corpoCorreccao
	// O corpo é obrigatório: sem valor não há correcção. Um corpo malformado tem de
	// falhar com 400 — nunca 200 a confirmar uma escrita que não aconteceu.
	if err := c.ShouldBindJSON(&corpo); err != nil {
		responderErro(c, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPedidoInvalido)))
		return
	}
	actor, _ := SessaoDe(c)
	out, err := h.corrigir.Executar(c.Request.Context(), actor.Sujeito, c.Param("rid"),
		applaboratorio.DadosCorrigirResultado{Valor: corpo.Valor, Observacoes: corpo.Observacoes})
	if err != nil {
		responderErro(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, out)
}
```

- [ ] **Step 4: Correr os testes de HTTP do laboratório (todos)**

Run: `go test ./internal/adapters/http/ -run 'Lab|Fila|Requisicao|Resultado|Analise|Amostra|Emitir|Submeter|Colher|Recusar|Validar|Corrigir' -v`
Expected: PASS — os novos e os já existentes (que agora passam pelo `routerLab` actualizado).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/laboratorio_handler.go internal/adapters/http/laboratorio_test.go
git commit -m "feat(laboratorio): rotas HTTP de validação e correcção (RBAC Patologista)"
```

---

### Task 11: Wiring no composition root

**Files:**
- Modify: `internal/platform/app.go`

**Interfaces:**
- Consumes: tudo o que foi construído acima. `repoUtilizadores` (já existe em `app.go:72`), `repoAnalises`, `repoRequisicoes`, `repoResultados`, `repoAuditoria`, `logger`, `cfg`.
- Produces: aplicação a arrancar com as rotas de validação/correcção ligadas.

- [ ] **Step 1: Acrescentar os imports**

Em `internal/platform/app.go`, no bloco de imports, acrescentar (alinhado com os aliases já usados, ex. `adsmtp`):

```go
	adsms "github.com/ivandrosilva12/sgcfinal/internal/adapters/sms"
```

(`adlaboratorio` e `applaboratorio` já estão importados.)

- [ ] **Step 2: Construir o notificador SMS e o resolvedor de contacto**

Imediatamente antes da construção de `handlerLaboratorio` (a seguir a `leitorClinicoLab := adlaboratorio.NovoLeitorClinico(...)`), acrescentar:

```go
	// Alertas de valor crítico por SMS: gateway real se configurado, senão no-op.
	var notificadorCritico applaboratorio.NotificadorCritico
	if cfg.SMSEndpoint == "" {
		notificadorCritico = adsms.NovoNotificadorNulo(logger)
		logger.Info("alertas por SMS desactivados (SMS_ENDPOINT vazio)")
	} else {
		notificadorCritico = adsms.NovoNotificadorSMS(cfg.SMSEndpoint, cfg.SMSRemetente)
		logger.Info("alertas por SMS activados", "endpoint", cfg.SMSEndpoint)
	}
	// ACL de contacto: o telefone do médico vive no BC Identidade.
	resolvedorContacto := adlaboratorio.NovoResolvedorContacto(repoUtilizadores)
```

- [ ] **Step 3: Passar os dois novos casos de uso ao handler**

Na chamada a `adhttp.NovoLaboratorioHandler(...)`, acrescentar os dois argumentos finais após `applaboratorio.NovoCasoListarResultadosDoEpisodio(repoResultados),`:

```go
		applaboratorio.NovoCasoValidarResultado(repoResultados, repoRequisicoes, repoAnalises, resolvedorContacto, notificadorCritico, repoAuditoria),
		applaboratorio.NovoCasoCorrigirResultado(repoResultados, repoRequisicoes, repoAnalises, resolvedorContacto, notificadorCritico, repoAuditoria),
```

- [ ] **Step 4: Compilar e correr a suite completa**

Run: `go build ./... && go test ./... `
Expected: build OK; testes unitários/aplicação PASS (integração faz SKIP sem `DATABASE_URL`).

- [ ] **Step 5: Verificar a arquitectura e o lint**

Run: `go-arch-lint check` (ou o comando usado no CI — ver `Makefile`/`.github`) e `golangci-lint run ./internal/adapters/sms/... ./internal/adapters/laboratorio/... ./internal/application/laboratorio/... ./internal/domain/laboratorio/...`
Expected: sem violações. Em particular, confirmar que `internal/adapters/sms` e `internal/adapters/laboratorio` estão autorizados a importar `application/laboratorio` e `domain/identidade` no `.go-arch-lint.yml`; se o componente `sms` for novo, acrescentá-lo à configuração espelhando o componente `smtp`.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/app.go .go-arch-lint.yml
git commit -m "feat(laboratorio): liga validação, correcção e alertas SMS no composition root"
```

---

### Task 12: ADR-032, SPRINT.md e CLAUDE.md — fecho do M3

**Files:**
- Create: `adrs/ADR-032-laboratorio-validacao-correccao.md`
- Modify: `SPRINT.md`
- Modify: `CLAUDE.md`

**Interfaces:** documentação; sem código.

- [ ] **Step 1: Escrever a ADR-032**

Criar `adrs/ADR-032-laboratorio-validacao-correccao.md` com, no mínimo, as secções: Estado (Aceite), Data (2026-07-15), Marco/Sprint (M3/Sprint 13), Contexto, e as decisões:

1. `CONCLUIDA` = arquivado/substituído; a correcção cria um novo `VALIDADA` (`corrige_resultado_id → original`) preservando o original e o técnico submissor (proveniência).
2. Valor crítico avaliado **na validação**, no domínio (`Analise.AvaliarCritico`); não numérico → nunca crítico.
3. Segregação de funções: `Validar` e `Corrigir` exigem `actor != tecnicoSubmissorID` (422); defesa em profundidade com a CHECK da BD (migração 0002).
4. SMS ao médico requisitante via extensão da ACL (`ResolvedorContacto` sobre Identidade); **best-effort e auditado** — falha de envio não reverte a validação; a prova de "SMS auditado" é o registo `laboratorio.valor_critico.notificado`.
5. `EstadosVisiveisAoMedico` reduzido a `{VALIDADA}` — o arquivado sai da vista clínica normal.
6. Consequências: fecha o marco M3. Dívida mantida: integração real de SMS, eventos por emitir (Outbox), auditoria fora da transacção.

- [ ] **Step 2: Actualizar o SPRINT.md**

Em `SPRINT.md`:

1. No cabeçalho, mudar o Sprint para 13 e marcar como entregue (espelhar o formato do Sprint 12).
2. Nos "Critérios de saída M3 — Laboratório", marcar os três últimos como `[x]`:
```
- [x] Validação pelo patologista com segregação (submissor ≠ validador). — Sprint 13
- [x] Valores críticos detectados e notificados (SMS auditado). — Sprint 13
- [x] Correcção cria novo resultado preservando o original. — Sprint 13
```
3. Acrescentar uma secção "## Sprint 13 — entregue" com a lista de deliverables (validação com segregação; avaliação de crítico no domínio; SMS best-effort auditado; correcção substitui/arquiva; visibilidade só do vigente; ADR-032).

- [ ] **Step 3: Actualizar o CLAUDE.md**

Em `CLAUDE.md`, secção 6 (Marco Actual): mover o M3 para entregue (ou actualizar a descrição do Sprint 13 como concluído) e acrescentar `adrs/ADR-032-laboratorio-validacao-correccao.md` à lista de ADRs registadas; actualizar "Próximo ADR" para **ADR-033**.

- [ ] **Step 4: Commit**

```bash
git add adrs/ADR-032-laboratorio-validacao-correccao.md SPRINT.md CLAUDE.md
git commit -m "docs(laboratorio): ADR-032 e fecho do marco M3 no SPRINT.md e CLAUDE.md"
```

---

### Task 13: Verificação final e gates de cobertura

**Files:** nenhum (verificação).

- [ ] **Step 1: Suite completa com `-race`**

Run: `go test -race ./...`
Expected: PASS (integração faz SKIP sem `DATABASE_URL`).

- [ ] **Step 2: Integração contra Postgres (se disponível)**

Run: `DATABASE_URL="$DATABASE_URL" go test -tags integration ./tests/integration/ -run TestLaboratorio -v`
Expected: PASS (o novo `TestLaboratorio_ValidacaoECorreccao` e os do Sprint 12).

- [ ] **Step 3: Gates de cobertura por camada**

Run:
```bash
go test -cover ./internal/domain/laboratorio/...
go test -cover ./internal/application/laboratorio/...
go test -cover ./internal/adapters/laboratorio/... ./internal/adapters/sms/... ./internal/adapters/http/...
```
Expected: domínio ≥85%, aplicação ≥75%, adaptadores ≥60%. Se algum adaptador ficar abaixo, acrescentar um teste dirigido ao ramo em falta (ex.: `NotificadorNulo`, ramo de erro do `enviarHTTP`).

- [ ] **Step 4: Lint e arquitectura**

Run: `golangci-lint run ./... && go-arch-lint check`
Expected: sem erros nem violações da regra de dependência.

- [ ] **Step 5: Confirmar a árvore limpa e o histórico**

Run: `git status --short && git log --oneline -13`
Expected: árvore limpa; os 12 commits das Tasks 1–12 presentes na branch `sprint-13-laboratorio-validacao`.

---

## Self-Review (preenchido pelo autor do plano)

**Cobertura do spec:**
- §2 máquina de estados → Tasks 2, 3, 8. §3.1 Validar/Corrigir/Valor() → Tasks 2, 3. §3.2 AvaliarCritico → Task 1. §3.3 porta Corrigir → Tasks 5, 8. §3.4 eventos → Task 4. §4.1 portas + visibilidade → Task 5. §4.2 CasoValidar → Task 6. §4.3 CasoCorrigir → Task 7. §4.4 notificação best-effort auditada → Tasks 6 (helper), 9 (adaptadores). §5.1 migração → Task 8. §5.2 pgrepo → Task 8. §5.3 resolvedor → Task 9. §5.4 SMS → Task 9. §5.5 rotas → Task 10. §5.6 wiring → Task 11. §6 testes → distribuídos + Task 13. §7 ADR/SPRINT → Task 12. §8 critérios de saída → Task 12/13.
- Sem lacunas identificadas.

**Consistência de tipos:** `Corrigir(ctx, novo, original) (string, error)` idêntico na interface (Task 5), no fake (Task 5) e no pgrepo (Task 8). `AvaliarCritico(string) bool`, `Validar(string, bool, time.Time) error`, `Corrigir(string,string,string,bool,time.Time) (*Resultado, error)` consistentes entre domínio (Tasks 1–3) e aplicação (Tasks 6–7). `ResolvedorContacto`/`NotificadorCritico` idênticos entre porta (Task 5), fakes (Task 5), adaptadores (Task 9) e wiring (Task 11). Assinatura de `NovoLaboratorioHandler` estendida coerente entre Task 10 (handler + teste) e Task 11 (wiring).
