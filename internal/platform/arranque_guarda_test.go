package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// pacoteDBImportPath é o caminho de import real do pacote que expõe
// VerificarPapelRuntime. A guarda resolve os aliases de app.go contra este
// caminho, não contra o identificador textual "db" — ver o comentário de
// resolverAliasesDeImport para o porquê.
const pacoteDBImportPath = "github.com/ivandrosilva12/sgcfinal/internal/platform/db"

// TestArranque_VerificaOPapelDeRuntime é a guarda que a ADR-042 ensinou a
// escrever: sem ela, apagar (ou neutralizar) a chamada a
// db.VerificarPapelRuntime em app.go deixaria as provas de tests/integration a
// passar na mesma, porque essas chamam a função directamente e não pelo
// arranque.
//
// A guarda original (anterior a esta correcção) só verificava que a SelectorExpr
// `db.VerificarPapelRuntime(...)` aparecia algures dentro de ExecutarServidor —
// via ast.Inspect sem restrição de posição ou de tratamento do erro. Isso
// aceitava, além da remoção pura, seis variantes que não protegem nada:
//
//  1. `_ = db.VerificarPapelRuntime(ctx, pool)` — chamada feita, erro descartado.
//  2. erro capturado mas só passado a `logger.Warn`, sem `return`.
//  3. chamada dentro de `if false { ... }` — nunca executa.
//  4. chamada dentro de uma closure atribuída a uma variável nunca invocada.
//  5. `db` redefinido por um import com esse alias apontando para outro
//     pacote, enquanto o pacote real (com o mesmo nome de função) fica sob um
//     alias diferente — a guarda por nome via "db.VerificarPapelRuntime" e
//     achava-se satisfeita, sem que a função real alguma vez fosse chamada.
//
// Esta versão exige DUAS coisas, cada uma insuficiente sem a outra:
//
//  1. Resolução por caminho de import, não por identificador — o pacote do
//     lado esquerdo do selector é resolvido contra os imports do próprio
//     ficheiro (resolverAliasesDeImport) e comparado a pacoteDBImportPath.
//     Isto apanha a variante 5 (o nome "db" ligado a outro pacote) e, ao
//     mesmo tempo, não quebra se o import real for renomeado para outro
//     alias (ex.: `dbreal "…/internal/platform/db"`), porque o que importa é
//     o caminho, não o nome local.
//  2. A chamada tem de ser tratada como fatal — Init de um `if` cujo Cond
//     compara a variável atribuída com `nil` e cujo corpo contém, em algum
//     ponto do seu próprio bloco (sem descer a closures aninhadas), um
//     `return`. Isto apanha as variantes 1 (erro descartado, não há `if`
//     nenhum a examinar), 2 (corpo sem `return`) e 3 (o `if false` que
//     envolve o padrão é reconhecido como ramo morto e podado da procura,
//     antes de sequer se olhar para o que lá dentro). A variante 4 (closure
//     nunca invocada) fica de fora porque só se desce para dentro de um
//     `*ast.FuncLit` quando ele é invocado imediatamente (`func(){...}()`) —
//     uma atribuição a uma variável não conta, invocada depois ou não.
//
// O que esta guarda NÃO garante, deliberadamente: a POSIÇÃO da chamada dentro
// de ExecutarServidor. Uma chamada colocada depois de `return srv.Iniciar(ctx)`
// (ou, de forma mais realista, depois de o servidor já ter começado a aceitar
// ligações) passaria por esta guarda na mesma — é a mesma limitação que a
// versão anterior já documentava, e fica deliberadamente por corrigir aqui: o
// pedido desta tarefa era endurecer contra ERRO DESCARTADO e ALIAS TROCADO,
// não contra ORDEM. Uma guarda de posição exigiria comparar índices de
// statement dentro do corpo da função, o que a tornaria frágil a qualquer
// refactor inócuo de ExecutarServidor; não vale o custo para o que é, no
// limite, um erro de code review humano mais do que um ataque automatizável.
func TestArranque_VerificaOPapelDeRuntime(t *testing.T) {
	fset := token.NewFileSet()
	ficheiro, err := parser.ParseFile(fset, "app.go", nil, 0)
	if err != nil {
		t.Fatalf("analisar app.go: %v", err)
	}

	aliases := resolverAliasesDeImport(ficheiro)

	var encontrada bool
	for _, decl := range ficheiro.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "ExecutarServidor" || fn.Body == nil {
			continue
		}
		if contemChamadaTratadaComoFatal(fn.Body, aliases) {
			encontrada = true
		}
	}

	if !encontrada {
		t.Fatal("ExecutarServidor tem de tratar o erro de db.VerificarPapelRuntime (resolvido " +
			"pelo caminho de import real, não só pelo nome do identificador) como fatal — a " +
			"chamada como Init de um `if` cujo corpo devolve. Sem isso, a separação de " +
			"credenciais da ADR-043 volta a ser uma suposição sobre o deployment")
	}
}

// resolverAliasesDeImport devolve, para cada import de app.go, o mapa
// identificador-local → caminho-de-import. Um import sem alias explícito usa
// como identificador local o último segmento do caminho (a resolução real do
// Go é mais subtil — lê o nome do pacote do próprio ficheiro-fonte — mas todos
// os pacotes deste repositório seguem essa convenção, e é a mesma que já vale
// para "db", "adhttp", etc. nos outros ficheiros de internal/platform).
//
// Isto é o que permite à guarda distinguir "o identificador chama-se db" de
// "o identificador resolve para o pacote internal/platform/db" — sem isto,
// um import como `db "internal/platform/config"` faria a guarda anterior
// (que só olhava para o nome "db") aceitar uma chamada a uma função de
// mentira com o mesmo nome, num pacote completamente diferente.
func resolverAliasesDeImport(ficheiro *ast.File) map[string]string {
	aliases := make(map[string]string, len(ficheiro.Imports))
	for _, imp := range ficheiro.Imports {
		caminho, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		nome := caminho
		if i := strings.LastIndex(caminho, "/"); i >= 0 {
			nome = caminho[i+1:]
		}
		if imp.Name != nil {
			nome = imp.Name.Name
		}
		aliases[nome] = caminho
	}
	return aliases
}

// ehChamadaAoPacoteDB confirma que a chamada é `<alias>.VerificarPapelRuntime(...)`
// e que <alias>, resolvido pelos imports do ficheiro, aponta mesmo para
// pacoteDBImportPath — não apenas que se chama "db".
func ehChamadaAoPacoteDB(chamada *ast.CallExpr, aliases map[string]string) bool {
	sel, ok := chamada.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "VerificarPapelRuntime" {
		return false
	}
	pacote, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return aliases[pacote.Name] == pacoteDBImportPath
}

// contemChamadaTratadaComoFatal percorre um nó a partir do corpo de
// ExecutarServidor à procura do padrão `if err := db.VerificarPapelRuntime(...);
// err != nil { … return … }`. Não é um ast.Inspect genérico: a descida é
// explícita por tipo de nó, precisamente para poder recusar-se a descer em
// dois casos —
//
//   - um `if` cuja condição é o literal `false` (ramo morto, variante 3);
//   - um `*ast.FuncLit` que não é o `Fun` de uma chamada imediata (closure não
//     invocada, variante 4).
//
// Qualquer nó de outro tipo não descrito abaixo devolve false sem recursão —
// isto é deliberado: o padrão-alvo só existe dentro de BlockStmt/IfStmt/
// ExprStmt/CallExpr(FuncLit imediato), e alargar a recursão a mais tipos de
// nó (for, switch, etc.) não é preciso para os casos desta tarefa e alargaria
// a superfície sem prova de que fecha algo real.
func contemChamadaTratadaComoFatal(node ast.Node, aliases map[string]string) bool {
	switch n := node.(type) {
	case *ast.BlockStmt:
		for _, stmt := range n.List {
			if contemChamadaTratadaComoFatal(stmt, aliases) {
				return true
			}
		}
		return false
	case *ast.IfStmt:
		if ehLiteralFalse(n.Cond) {
			return false // ramo morto: nunca executa, não conta como protecção real
		}
		if chamadaTratadaComoFatalNesteIf(n, aliases) {
			return true
		}
		if contemChamadaTratadaComoFatal(n.Body, aliases) {
			return true
		}
		if n.Else != nil {
			return contemChamadaTratadaComoFatal(n.Else, aliases)
		}
		return false
	case *ast.ExprStmt:
		return contemChamadaTratadaComoFatal(n.X, aliases)
	case *ast.CallExpr:
		if funcLit, ok := n.Fun.(*ast.FuncLit); ok {
			// IIFE — func(){...}() — invocada imediatamente: entra.
			return contemChamadaTratadaComoFatal(funcLit.Body, aliases)
		}
		return false
	default:
		return false
	}
}

// ehLiteralFalse reconhece exactamente o literal `false` como condição de um
// `if`. Não tenta avaliação de constantes em geral (ex.: `1 == 2`) — o caso
// pedido por esta tarefa é `if false { … }`, e ir além disso não tem prova de
// necessidade.
func ehLiteralFalse(cond ast.Expr) bool {
	ident, ok := cond.(*ast.Ident)
	return ok && ident.Name == "false"
}

// chamadaTratadaComoFatalNesteIf confirma que ESTE `if`, especificamente, é o
// padrão `if err := db.VerificarPapelRuntime(...); err != nil { … return … }`:
// o Init é uma atribuição de uma única variável ao resultado da chamada ao
// pacote db, o Cond compara essa MESMA variável com `nil`, e o Body contém um
// `return` nalgum ponto do seu próprio bloco.
func chamadaTratadaComoFatalNesteIf(ifStmt *ast.IfStmt, aliases map[string]string) bool {
	atribuicao, ok := ifStmt.Init.(*ast.AssignStmt)
	if !ok || len(atribuicao.Lhs) != 1 || len(atribuicao.Rhs) != 1 {
		return false
	}
	varErro, ok := atribuicao.Lhs[0].(*ast.Ident)
	if !ok {
		return false
	}
	chamada, ok := atribuicao.Rhs[0].(*ast.CallExpr)
	if !ok || !ehChamadaAoPacoteDB(chamada, aliases) {
		return false
	}
	if !condicaoComparaComNil(ifStmt.Cond, varErro.Name) {
		return false
	}
	return contemReturn(ifStmt.Body)
}

// condicaoComparaComNil confirma que cond é `nomeVar != nil` ou `nil !=
// nomeVar` — a variável tem de ser a mesma que recebeu o resultado da
// chamada no Init do mesmo `if`, não qualquer variável chamada "err".
func condicaoComparaComNil(cond ast.Expr, nomeVar string) bool {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return false
	}
	esquerda, okE := bin.X.(*ast.Ident)
	direita, okD := bin.Y.(*ast.Ident)
	if !okE || !okD {
		return false
	}
	return (esquerda.Name == nomeVar && direita.Name == "nil") ||
		(direita.Name == nomeVar && esquerda.Name == "nil")
}

// contemReturn procura um *ast.ReturnStmt dentro do bloco, sem descer para
// dentro de closures aninhadas (um `return` dentro de uma FuncLit dentro do
// corpo do `if` pertence a essa closure, não ao fluxo de ExecutarServidor, e
// não trata o erro como fatal para o arranque).
func contemReturn(bloco *ast.BlockStmt) bool {
	encontrado := false
	ast.Inspect(bloco, func(n ast.Node) bool {
		if encontrado {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if _, ok := n.(*ast.ReturnStmt); ok {
			encontrado = true
			return false
		}
		return true
	})
	return encontrado
}
