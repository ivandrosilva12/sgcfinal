//go:build integration

// Prova de caixa branca da correcção 4 da revisão da Tarefa 3 (ADR-043):
// recusarCriacaoDeObjectos filtrava `WHERE ns IS NOT NULL`, o que faz um
// schema ausente desaparecer da verificação em silêncio — hoje coberto porque
// recusarDono corre primeiro e exige as tabelas migradas, mas um 9.º schema
// acrescentado a schemasBC antes da migração respectiva passaria calado.
//
// Este teste vive no próprio pacote db (não em tests/integration) porque
// precisa de manipular a variável não-exportada schemasBC directamente, sem
// tocar em nenhum schema real da base — a alternativa (apagar um schema
// migrado a sério) arriscaria corromper a base de desenvolvimento partilhada
// por outras suites concorrentes.
package db

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestRecusarCriacaoDeObjectos_NomeiaSchemaAusente(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL não definida; a saltar")
	}
	ctx := context.Background()
	pool, err := LigarPool(ctx, url)
	if err != nil {
		t.Fatalf("ligar como runtime: %v", err)
	}
	t.Cleanup(pool.Close)

	original := schemasBC
	t.Cleanup(func() { schemasBC = original })
	schemasBC = append(append([]string{}, original...), "zz_schema_inexistente_teste")

	err = recusarCriacaoDeObjectos(ctx, pool, "sgc_app")
	if err == nil {
		t.Fatal("um schema inexistente em schemasBC tinha de ser nomeado como ausente, " +
			"não saltado em silêncio")
	}
	if !strings.Contains(err.Error(), "zz_schema_inexistente_teste") {
		t.Fatalf("a mensagem de erro tem de nomear o schema ausente; obtive: %v", err)
	}
}
