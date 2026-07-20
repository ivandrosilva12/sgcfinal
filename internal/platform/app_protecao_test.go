package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// A exposição que a ADR-042 corrige (R3 da ADR-040) existiu porque cada local de
// chamada escolhia os seus middlewares: três grupos receberam o mfaMW e onze não,
// e nada tornava isso visível. O pacote `protecao` torna a omissão um desvio
// deliberado — esta guarda torna-a detectável.
//
// O teste lê o código-fonte em vez de exercitar comportamento. É invulgar, e a
// razão está registada na ADR-042 §4.1: o que falhou aqui foi a LIGAÇÃO, não o
// comportamento, e prová-la por comportamento exigiria montar os catorze
// handlers com todos os seus fakes só para verificar uma propriedade de wiring.
//
// A guarda analisa a árvore sintáctica (go/parser + go/ast), não o texto-fonte,
// e garante DUAS coisas, cada uma insuficiente sem a outra:
//
//  1. Ligação — dentro do corpo literal de `registarRotas`, cada um dos catorze
//     grupos nomeados em `gruposEsperados` aparece exactamente uma vez, como
//     `adhttp.Registar<algo>(..., protecao...)`. A comparação é por CONJUNTO
//     NOMEADO, não por contagem: um grupo em falta é nomeado como "em falta" e
//     um grupo extra é nomeado como "inesperado" — nunca apenas um número. Isto
//     apanha remoção, extracção da chamada para um helper invocado a partir de
//     `registarRotas` (o helper deixa de aparecer como `adhttp.Registar*` no
//     corpo inspeccionado) e troca do alias de import `adhttp` por outro nome
//     (a mesma razão: o selector deixa de bater com o pacote reconhecido).
//  2. Conteúdo — a variável `protecao` é atribuída exactamente uma vez na
//     função que a declara, e o valor é sempre o composite literal
//     `[]gin.HandlerFunc{limiteMW, authMW, mfaMW}`, nesta ordem. Sem isto, a
//     guarda anterior via o NOME `protecao...` chegar a cada chamada mas era
//     cega ao que esse nome significava — uma reatribuição no mesmo escopo
//     (`protecao = []gin.HandlerFunc{limiteMW, authMW}`) compilava, passava no
//     `gofmt` e passava na guarda, desligando o `mfaMW` nos catorze grupos de
//     uma só vez.
//
// O que a guarda NÃO garante (lacuna conhecida, deixada explícita para não
// prometer mais do que cumpre): não verifica o COMPORTAMENTO de `limiteMW`,
// `authMW` ou `mfaMW` — apenas que esses três identificadores, por este nome
// exacto e nesta ordem, chegam a `protecao` e daí aos catorze grupos. Se
// `adhttp.MFAObrigatoria()` deixasse de impor MFA por dentro, ou se `mfaMW`
// fosse redefinido para um no-op mais acima no ficheiro, esta guarda continua
// verde — essa é uma propriedade de comportamento do middleware, testada em
// `internal/adapters/http`, não de wiring, que é o que esta guarda cobre.
//
// RegistarHealth está isento por desenho: healthchecks e o scrape do Prometheus
// são não-autenticados. Na prática nem chega a ser chamado dentro de
// registarRotas — é registado directamente em
// internal/platform/server/server.go (junto de /metrics), fora do bloco que
// esta guarda inspecciona. A isenção aqui é puramente defensiva (para o caso de
// um dia passar a ser chamado a partir daqui); não é ela que hoje o protege ou
// desprotege — mexer nesta guarda não muda o estado do healthcheck.
func TestRegistarRotas_TodasAsRotasDeNegocioUsamOPacoteProteccao(t *testing.T) {
	fset := token.NewFileSet()
	ficheiro, err := parser.ParseFile(fset, "app.go", nil, 0)
	if err != nil {
		t.Fatalf("analisar app.go: %v", err)
	}

	funcRegistarRotas, corpoExterior := localizarFuncRegistarRotas(ficheiro)
	if funcRegistarRotas == nil {
		t.Fatal("não encontrei a atribuição de registarRotas em app.go — o teste precisa de ser actualizado")
	}

	verificarConteudoDeProteccao(t, corpoExterior)

	var chamadas []*ast.CallExpr
	ast.Inspect(funcRegistarRotas.Body, func(n ast.Node) bool {
		chamada, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if nomeChamadaAdHTTPRegistar(chamada) != "" {
			chamadas = append(chamadas, chamada)
		}
		return true
	})
	if len(chamadas) == 0 {
		t.Fatal("não encontrei chamadas adhttp.Registar* dentro de registarRotas")
	}

	var semProteccao []string
	encontrados := map[string]bool{}
	for _, chamada := range chamadas {
		nome := nomeChamadaAdHTTPRegistar(chamada)
		encontrados[nome] = true
		if nome == "RegistarHealth" {
			continue // isento por desenho — ver comentário do pacote: é registado
			// em server.go, nunca dentro de registarRotas.
		}
		if !terminaEmProteccaoVariadica(chamada) {
			semProteccao = append(semProteccao, nome)
		}
	}
	if len(semProteccao) > 0 {
		t.Errorf("grupos de rotas sem o pacote `protecao`: %v\n"+
			"Todo o grupo de negócio tem de terminar em `protecao...`. Se um grupo "+
			"tiver mesmo de ficar sem MFA, isso exige uma ADR — não uma excepção "+
			"silenciosa aqui.", semProteccao)
	}

	// Comparação por conjunto nomeado, não por contagem (`len(chamadas) < 14`):
	// um piso numérico não distingue "um grupo foi removido" de "um grupo
	// escapou à inspecção" (helper extraído, alias de import diferente), e é um
	// limite inferior que se degrada com o crescimento normal — um 15.º grupo
	// mal ligado passa por baixo de qualquer piso fixo. Nomear os catorze
	// grupos esperados obriga a que acrescentar ou remover um seja um acto
	// deliberado (a tese do ADR-042), e produz sempre a mensagem certa: o nome
	// exacto do grupo em falta ou inesperado.
	var emFalta []string
	for _, esperado := range gruposEsperados {
		if !encontrados[esperado] {
			emFalta = append(emFalta, esperado)
		}
	}
	sort.Strings(emFalta)
	if len(emFalta) > 0 {
		t.Errorf("grupos de rotas esperados mas ausentes de registarRotas: %v\n"+
			"Um grupo desaparece desta lista por ter sido removido, extraído para um "+
			"helper invocado a partir de registarRotas (o helper não é visto por esta "+
			"guarda), ou chamado através de um alias de import diferente de `adhttp` — "+
			"em qualquer dos casos o resultado é o mesmo: o grupo deixou de ser "+
			"inspeccionado, o que hoje normalmente também quer dizer que ficou sem MFA. "+
			"Corrige a chamada ou, se a remoção for deliberada, actualiza "+
			"`gruposEsperados` conscientemente.", emFalta)
	}

	esperadoSet := map[string]bool{}
	for _, esperado := range gruposEsperados {
		esperadoSet[esperado] = true
	}
	var inesperados []string
	for nome := range encontrados {
		if nome == "RegistarHealth" || esperadoSet[nome] {
			continue
		}
		inesperados = append(inesperados, nome)
	}
	sort.Strings(inesperados)
	if len(inesperados) > 0 {
		t.Errorf("grupos de rotas inesperados em registarRotas: %v\n"+
			"Se for um grupo novo e deliberado, acrescenta-o a `gruposEsperados` "+
			"acima — não deixes o crescimento do catálogo passar sem essa decisão "+
			"explícita.", inesperados)
	}
}

// gruposEsperados é o conjunto nomeado e explícito dos catorze grupos de rotas
// de negócio que registarRotas tem de ligar, cada um terminando em
// `protecao...`. Acrescentar ou remover um grupo em app.go exige acrescentar
// ou remover o nome correspondente aqui — deliberadamente, nunca por acidente
// de um piso numérico.
var gruposEsperados = []string{
	"RegistarIdentidade",
	"RegistarAdministracao",
	"RegistarDoentes",
	"RegistarEpisodios",
	"RegistarConsentimentos",
	"RegistarCirurgia",
	"RegistarFarmacia",
	"RegistarFarmaciaStock",
	"RegistarLaboratorio",
	"RegistarFinanceiro",
	"RegistarRecepcao",
	"RegistarRecepcaoChegadas",
	"RegistarRecepcaoTriagem",
	"RegistarClinicoConsulta",
}

// localizarFuncRegistarRotas procura, em todo o ficheiro, a atribuição
// `registarRotas := func(...) { ... }` e devolve o literal de função
// atribuído, junto com o corpo da função de topo que a contém (a função onde
// `protecao` é declarada — hoje ExecutarServidor). Devolve (nil, nil) se não
// encontrar. O corpo exterior é necessário para verificarConteudoDeProteccao:
// a reatribuição maliciosa de `protecao` vive no mesmo escopo da declaração,
// não dentro do literal de registarRotas.
func localizarFuncRegistarRotas(ficheiro *ast.File) (*ast.FuncLit, *ast.BlockStmt) {
	for _, decl := range ficheiro.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			continue
		}
		var encontrada *ast.FuncLit
		ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
			if encontrada != nil {
				return false
			}
			atribuicao, ok := n.(*ast.AssignStmt)
			if !ok || len(atribuicao.Lhs) != 1 || len(atribuicao.Rhs) != 1 {
				return true
			}
			alvo, ok := atribuicao.Lhs[0].(*ast.Ident)
			if !ok || alvo.Name != "registarRotas" {
				return true
			}
			literal, ok := atribuicao.Rhs[0].(*ast.FuncLit)
			if !ok {
				return true
			}
			encontrada = literal
			return false
		})
		if encontrada != nil {
			return encontrada, funcDecl.Body
		}
	}
	return nil, nil
}

// nomeChamadaAdHTTPRegistar devolve o nome da função (ex.: "RegistarDoentes")
// quando a chamada é da forma adhttp.Registar<algo>(...); devolve "" para
// qualquer outra chamada.
func nomeChamadaAdHTTPRegistar(chamada *ast.CallExpr) string {
	seletor, ok := chamada.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pacote, ok := seletor.X.(*ast.Ident)
	if !ok || pacote.Name != "adhttp" {
		return ""
	}
	if !strings.HasPrefix(seletor.Sel.Name, "Registar") {
		return ""
	}
	return seletor.Sel.Name
}

// terminaEmProteccaoVariadica confirma que o último argumento da chamada é,
// de facto, `protecao...` — um identificador chamado `protecao` expandido com
// reticências (Ellipsis), e não apenas um argumento com esse nome passado sem
// elas, o que mudaria silenciosamente o número de middlewares aplicados.
func terminaEmProteccaoVariadica(chamada *ast.CallExpr) bool {
	if !chamada.Ellipsis.IsValid() {
		return false
	}
	if len(chamada.Args) == 0 {
		return false
	}
	ultimo, ok := chamada.Args[len(chamada.Args)-1].(*ast.Ident)
	if !ok {
		return false
	}
	return ultimo.Name == "protecao"
}

// verificarConteudoDeProteccao assere que, no corpo da função que declara
// `protecao` (ExecutarServidor), existe EXACTAMENTE UMA atribuição a essa
// variável, e que o valor atribuído é sempre o composite literal
// `[]gin.HandlerFunc{limiteMW, authMW, mfaMW}`, nesta ordem. A guarda de
// ligação (terminaEmProteccaoVariadica) só sabe olhar para o NOME `protecao`
// chegar a cada chamada; esta função olha para o que esse nome significa. Sem
// ela, `protecao = []gin.HandlerFunc{limiteMW, authMW}` no mesmo escopo
// compila, passa no gofmt e desliga o mfaMW nos catorze grupos de uma só vez,
// sem que a guarda de ligação veja diferença nenhuma.
func verificarConteudoDeProteccao(t *testing.T, corpoExterior *ast.BlockStmt) {
	t.Helper()

	var atribuicoes []*ast.AssignStmt
	ast.Inspect(corpoExterior, func(n ast.Node) bool {
		atribuicao, ok := n.(*ast.AssignStmt)
		if !ok || len(atribuicao.Lhs) != 1 || len(atribuicao.Rhs) != 1 {
			return true
		}
		alvo, ok := atribuicao.Lhs[0].(*ast.Ident)
		if !ok || alvo.Name != "protecao" {
			return true
		}
		atribuicoes = append(atribuicoes, atribuicao)
		return true
	})

	if len(atribuicoes) != 1 {
		t.Errorf("esperava exactamente uma atribuição a `protecao` na função que a "+
			"declara, encontrei %d. Zero atribuições quer dizer que `protecao` não "+
			"está definida onde a guarda espera; mais do que uma — por exemplo uma "+
			"reatribuição mais abaixo no mesmo escopo — permite que qualquer uma "+
			"delas mude silenciosamente o que os catorze grupos recebem, e a guarda "+
			"de ligação não distingue qual das duas está de facto em vigor no fim.",
			len(atribuicoes))
		return
	}

	valor := atribuicoes[0].Rhs[0]
	composto, ok := valor.(*ast.CompositeLit)
	if !ok || !ehListaDeHandlerFuncGin(composto.Type) {
		t.Errorf("a atribuição a `protecao` tem de ser um composite literal "+
			"`[]gin.HandlerFunc{...}`; encontrei %T", valor)
		return
	}

	nomesEsperados := []string{"limiteMW", "authMW", "mfaMW"}
	if len(composto.Elts) != len(nomesEsperados) {
		t.Errorf("`protecao` tem de ter exactamente %d elementos (%s), encontrei %d — "+
			"um `protecao = []gin.HandlerFunc{limiteMW, authMW}` compila e passa no "+
			"gofmt, mas desliga o mfaMW nos catorze grupos de uma só vez",
			len(nomesEsperados), strings.Join(nomesEsperados, ", "), len(composto.Elts))
		return
	}

	var encontrados []string
	for _, elt := range composto.Elts {
		ident, ok := elt.(*ast.Ident)
		if !ok {
			encontrados = append(encontrados, "<não-identificador>")
			continue
		}
		encontrados = append(encontrados, ident.Name)
	}
	for i, esperado := range nomesEsperados {
		if encontrados[i] != esperado {
			t.Errorf("`protecao` tem de ser `[]gin.HandlerFunc{%s}`, nesta ordem; "+
				"encontrei `[]gin.HandlerFunc{%s}` — sem `mfaMW` como último elemento, "+
				"nenhum dos catorze grupos exige segundo factor",
				strings.Join(nomesEsperados, ", "), strings.Join(encontrados, ", "))
			return
		}
	}
}

// ehListaDeHandlerFuncGin confirma que um tipo de composite literal é, de
// facto, o slice `[]gin.HandlerFunc` — não um array de tamanho fixo (que
// exigiria número de elementos diferente) nem um slice de outro tipo.
func ehListaDeHandlerFuncGin(tipo ast.Expr) bool {
	arrayType, ok := tipo.(*ast.ArrayType)
	if !ok || arrayType.Len != nil {
		return false
	}
	seletor, ok := arrayType.Elt.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pacote, ok := seletor.X.(*ast.Ident)
	return ok && pacote.Name == "gin" && seletor.Sel.Name == "HandlerFunc"
}
