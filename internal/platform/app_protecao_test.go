package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
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
// A guarda analisa a árvore sintáctica (go/parser + go/ast), não o texto-fonte.
// Uma versão anterior usava uma expressão regular e podia ser enganada de duas
// formas (achado da revisão à Task 1, corrigido aqui): um comentário à direita
// com parênteses — `RegistarX(r, h, protecao...) // nota (ver protecao...)` —
// era engolido para dentro da captura dos argumentos porque a regex era gulosa
// até ao último `)` da linha; e uma chamada partida em várias linhas ficava
// invisível porque `[^\n]*` não atravessa quebras de linha. Um analisador
// sintáctico vê a estrutura real da chamada — argumentos, reticências — e não
// é afectado por comentários, formatação ou quebras de linha.
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

	funcRegistarRotas := localizarFuncRegistarRotas(ficheiro)
	if funcRegistarRotas == nil {
		t.Fatal("não encontrei a atribuição de registarRotas em app.go — o teste precisa de ser actualizado")
	}

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
	for _, chamada := range chamadas {
		nome := nomeChamadaAdHTTPRegistar(chamada)
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
	if len(chamadas) < 14 {
		t.Errorf("esperava pelo menos 14 grupos de rotas, encontrei %d — "+
			"se um grupo foi removido, actualiza este número deliberadamente", len(chamadas))
	}
}

// localizarFuncRegistarRotas procura, em todo o ficheiro, a atribuição
// `registarRotas := func(...) { ... }` e devolve o literal de função
// atribuído. Devolve nil se não encontrar.
func localizarFuncRegistarRotas(ficheiro *ast.File) *ast.FuncLit {
	var encontrada *ast.FuncLit
	ast.Inspect(ficheiro, func(n ast.Node) bool {
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
	return encontrada
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
