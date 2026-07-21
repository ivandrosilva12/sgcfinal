package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestArranque_VerificaOPapelDeRuntime é a guarda que a ADR-042 ensinou a
// escrever: sem ela, apagar uma linha de app.go deixaria as provas de
// tests/integration a passar na mesma, porque essas chamam a função directamente
// e não pelo arranque.
func TestArranque_VerificaOPapelDeRuntime(t *testing.T) {
	fset := token.NewFileSet()
	ficheiro, err := parser.ParseFile(fset, "app.go", nil, 0)
	if err != nil {
		t.Fatalf("analisar app.go: %v", err)
	}

	var encontrada bool
	ast.Inspect(ficheiro, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "ExecutarServidor" {
			return true
		}
		ast.Inspect(fn, func(m ast.Node) bool {
			chamada, ok := m.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := chamada.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pacote, ok := sel.X.(*ast.Ident)
			if ok && pacote.Name == "db" && sel.Sel.Name == "VerificarPapelRuntime" {
				encontrada = true
				return false
			}
			return true
		})
		return false
	})

	if !encontrada {
		t.Fatal("ExecutarServidor tem de chamar db.VerificarPapelRuntime: sem isso, a separação " +
			"de credenciais da ADR-043 volta a ser uma suposição sobre o deployment")
	}
}
