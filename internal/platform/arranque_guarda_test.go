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
// A guarda exige uma FORMA CANÓNICA, e recusa tudo o resto (falha fechada). No
// corpo de ExecutarServidor tem de existir um statement DIRECTO — elemento de
// Body.List, não aninhado em coisa nenhuma — que seja um `if` com:
//
//  1. Init = atribuição de uma só variável ao resultado de uma chamada a
//     VerificarPapelRuntime, num pacote resolvido pelo CAMINHO DE IMPORT
//     (pacoteDBImportPath), não pelo identificador "db";
//  2. segundo argumento dessa chamada = o mesmo identificador que recebeu o
//     pool de db.LigarPool (também resolvido por caminho);
//  3. Cond = `<mesma variável> != nil`;
//  4. Body.List com um `return` DIRECTO;
//  5. índice desse `if` estritamente menor que o índice do statement que
//     contém a chamada a `.Iniciar` do servidor — verifica-se ANTES de servir.
//
// Duas escolhas de desenho fazem o trabalho pesado, e são o motivo de esta
// versão ser mais curta do que a anterior em vez de mais longa:
//
//   - Exigir que o `if` seja elemento DIRECTO de Body.List dispensa reconhecer
//     ramos mortos. A versão anterior descia recursivamente e tentava podar o
//     caso `if false { … }` com um teste ao literal `false` — o que deixava
//     passar `if 1 == 2 { … }` (medido: guarda VERDE) e qualquer outra
//     constante falsa. Aninhar a verificação em seja o que for passa agora a
//     ser recusado sem a guarda ter de avaliar constantes nem provar que o
//     ramo é morto.
//   - Percorrer Body.List por índice dá de graça a garantia de ORDEM, que a
//     versão anterior declarava como limitação por ser cara. Deixou de ser.
//
// Variantes medidas por mutação real de app.go (gofmt + go build + esta guarda,
// com `git checkout --` entre cada uma). Contra a versão anterior desta guarda,
// (g), (h), (i) e (k) ficavam VERDES:
//
//	a  `_ = db.VerificarPapelRuntime(...)`                    → VERMELHO
//	b  erro só em logger.Warn, sem return                     → VERMELHO
//	c  dentro de `if false { … }`                             → VERMELHO
//	d  dentro de closure nunca invocada                       → VERMELHO
//	e  `db` como alias de OUTRO pacote (chamariz)             → VERMELHO
//	f  alias legítimo do pacote certo (`dbreal "…/db"`)       → VERDE
//	g  chamada depois de `srv.Iniciar`                        → VERMELHO
//	h  dentro de `if 1 == 2 { … }`                            → VERMELHO
//	i  isenção por ambiente (`if cfg.EmProducao()` aninhado)  → VERMELHO
//	j  código real, intocado                                  → VERDE
//	k  chamada com outro pool (`db.Verificar…(ctx, outro)`)   → VERMELHO
//
// A variante (i) é a neutralização mais provável na vida real: lê-se como
// engenharia razoável num code review e é exactamente o que o comentário de
// app.go proíbe (sem isenção por ambiente). Fica fechada pelo requisito 4 — o
// `return` tem de estar DIRECTO no corpo do `if`, não dentro de um `if`
// aninhado.
//
// O que esta guarda NÃO garante, deliberadamente: que a função verificada faça
// o que promete (isso é a suite de tests/integration) e que não exista uma
// segunda chamada, mais abaixo, que desfaça o efeito da primeira — não há forma
// de o fazer sem interpretar o programa.
func TestArranque_VerificaOPapelDeRuntime(t *testing.T) {
	fset := token.NewFileSet()
	ficheiro, err := parser.ParseFile(fset, "app.go", nil, 0)
	if err != nil {
		t.Fatalf("analisar app.go: %v", err)
	}

	aliases := resolverAliasesDeImport(ficheiro)

	var corpo *ast.BlockStmt
	for _, decl := range ficheiro.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "ExecutarServidor" && fn.Body != nil {
			corpo = fn.Body
			break
		}
	}
	if corpo == nil {
		t.Fatal("não encontrei ExecutarServidor com corpo em app.go: a guarda falha fechada — " +
			"se a função foi renomeada, actualize também esta guarda (ADR-043)")
	}

	nomePool := identificadorDoPool(corpo, aliases)
	if nomePool == "" {
		t.Fatal("não encontrei, como statement directo de ExecutarServidor, a atribuição do " +
			"pool a partir de LigarPool no pacote " + pacoteDBImportPath + ": a guarda falha " +
			"fechada porque sem saber qual é o pool não consegue verificar que é ESSE que vai " +
			"à verificação de privilégios (ADR-043)")
	}

	indiceVerificacao := -1
	for i, stmt := range corpo.List {
		ifStmt, ok := stmt.(*ast.IfStmt)
		if ok && verificacaoCanonica(ifStmt, aliases, nomePool) {
			indiceVerificacao = i
			break
		}
	}
	if indiceVerificacao < 0 {
		t.Fatal("ExecutarServidor tem de conter, como statement DIRECTO do seu corpo, " +
			"`if err := db.VerificarPapelRuntime(ctx, " + nomePool + "); err != nil { return … }` " +
			"— com o pacote resolvido pelo caminho de import real, o mesmo pool de LigarPool e " +
			"um `return` directo no corpo do `if`. Aninhado em `if`, closure ou qualquer outra " +
			"coisa não conta, e o erro não pode ser descartado nem só registado. Sem isto, a " +
			"separação de credenciais da ADR-043 volta a ser uma suposição sobre o deployment")
	}

	indiceArranque := indiceDoArranque(corpo)
	if indiceArranque < 0 {
		t.Fatal("não encontrei, como statement directo de ExecutarServidor, a chamada a " +
			"`.Iniciar` do servidor: a guarda falha fechada porque sem esse ponto de referência " +
			"não consegue provar que a verificação de privilégios corre ANTES de o servidor " +
			"começar a servir (ADR-043)")
	}
	if indiceVerificacao >= indiceArranque {
		t.Fatalf("a verificação do papel de runtime está no statement %d e o arranque do "+
			"servidor no %d: verificar depois de servir não protege nada — a janela entre "+
			"aceitar ligações e recusar arrancar é exactamente o que a ADR-043 fecha",
			indiceVerificacao, indiceArranque)
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

// chamadaAoPacoteDB confirma que a expressão é uma chamada
// `<alias>.<funcao>(...)` em que <alias>, resolvido pelos imports do ficheiro,
// aponta mesmo para pacoteDBImportPath — não apenas que se chama "db".
func chamadaAoPacoteDB(expr ast.Expr, aliases map[string]string, funcao string) (*ast.CallExpr, bool) {
	chamada, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	sel, ok := chamada.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != funcao {
		return nil, false
	}
	pacote, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, false
	}
	return chamada, aliases[pacote.Name] == pacoteDBImportPath
}

// identificadorDoPool devolve o nome da variável que recebe o pool de
// db.LigarPool como statement DIRECTO do corpo — `pool, err := db.LigarPool(…)`.
// Devolve "" se não existir: é o que permite à guarda exigir que seja ESSE pool
// a ir à verificação, fechando a variante em que a chamada existe mas recebe
// outro pool qualquer (por exemplo o da credencial de migração, que passaria
// a verificação enquanto o pool real, privilegiado, ficava por verificar).
func identificadorDoPool(corpo *ast.BlockStmt, aliases map[string]string) string {
	for _, stmt := range corpo.List {
		atribuicao, ok := stmt.(*ast.AssignStmt)
		if !ok || len(atribuicao.Rhs) != 1 || len(atribuicao.Lhs) == 0 {
			continue
		}
		if _, ok := chamadaAoPacoteDB(atribuicao.Rhs[0], aliases, "LigarPool"); !ok {
			continue
		}
		if ident, ok := atribuicao.Lhs[0].(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

// verificacaoCanonica confirma que ESTE `if`, especificamente, é a forma
// canónica: `if <err> := <pacote db>.VerificarPapelRuntime(<ctx>, <pool>);
// <err> != nil { … return … }`, com o `return` directo no corpo.
func verificacaoCanonica(ifStmt *ast.IfStmt, aliases map[string]string, nomePool string) bool {
	atribuicao, ok := ifStmt.Init.(*ast.AssignStmt)
	if !ok || len(atribuicao.Lhs) != 1 || len(atribuicao.Rhs) != 1 {
		return false
	}
	varErro, ok := atribuicao.Lhs[0].(*ast.Ident)
	if !ok {
		return false
	}
	chamada, ok := chamadaAoPacoteDB(atribuicao.Rhs[0], aliases, "VerificarPapelRuntime")
	if !ok {
		return false
	}
	if len(chamada.Args) != 2 {
		return false
	}
	argPool, ok := chamada.Args[1].(*ast.Ident)
	if !ok || argPool.Name != nomePool {
		return false
	}
	if !condicaoComparaComNil(ifStmt.Cond, varErro.Name) {
		return false
	}
	return contemReturnDirecto(ifStmt.Body)
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

// contemReturnDirecto procura um *ast.ReturnStmt entre os statements DIRECTOS
// do bloco. Não usa ast.Inspect: um `return` aninhado (dentro de um `if
// cfg.EmProducao()`, de um `switch` ou de uma closure) não devolve de
// ExecutarServidor em todos os caminhos, e era exactamente assim que a isenção
// por ambiente da variante (i) passava despercebida.
func contemReturnDirecto(bloco *ast.BlockStmt) bool {
	for _, stmt := range bloco.List {
		if _, ok := stmt.(*ast.ReturnStmt); ok {
			return true
		}
	}
	return false
}

// indiceDoArranque devolve o índice do PRIMEIRO statement directo do corpo que
// contenha uma chamada `<algo>.Iniciar(…)` — o ponto a partir do qual o
// servidor aceita ligações. Devolve -1 se não existir, e a guarda falha fechada
// nesse caso: é preferível uma guarda que chumba depois de um refactor do
// arranque (obrigando a revê-la) a uma que se declara satisfeita por não ter
// encontrado a referência.
//
// Aqui, ao contrário do resto da guarda, a procura desce por ast.Inspect: o
// arranque pode estar embrulhado (`return srv.Iniciar(ctx)`, uma atribuição,
// um `defer`), e o que interessa é o statement de topo em que aparece.
func indiceDoArranque(corpo *ast.BlockStmt) int {
	for i, stmt := range corpo.List {
		encontrado := false
		ast.Inspect(stmt, func(n ast.Node) bool {
			if encontrado {
				return false
			}
			chamada, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := chamada.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Iniciar" {
				return true
			}
			if _, ok := sel.X.(*ast.Ident); ok {
				encontrado = true
				return false
			}
			return true
		})
		if encontrado {
			return i
		}
	}
	return -1
}
