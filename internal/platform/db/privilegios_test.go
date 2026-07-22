//go:build integration

// Provas de caixa branca da verificação de arranque dos privilégios (ADR-043).
// Vivem no próprio pacote db (e não em tests/integration) porque precisam de
// manipular as variáveis não-exportadas schemasBC e tabelasDeValorLegal
// directamente, sem tocar em nenhum schema nem tabela real da base — a
// alternativa (apagar um schema migrado a sério) arriscaria corromper a base
// de desenvolvimento partilhada por outras suites concorrentes.
//
// Atenção: têm a tag `integration`, pelo que o passo `go test -race ./...` do
// job de qualidade as salta. Correm porque o passo de integração da CI nomeia
// explicitamente ./internal/platform/db/... (.github/workflows/ci.yml).
package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ligarRuntimeParaProva liga com a credencial de runtime (DATABASE_URL). O
// catálogo do PostgreSQL (pg_namespace, pg_trigger, pg_class) é legível por
// qualquer papel, pelo que derivar dele com esta credencial não é circular: um
// schema sem USAGE para sgc_app continua a aparecer em pg_namespace.
func ligarRuntimeParaProva(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
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
	return pool, ctx
}

// diferencaDeConjuntos devolve os elementos de a que não estão em b, para
// nomear na mensagem de falha o que está a mais e o que está a menos em vez de
// um "não coincide" sem detalhe.
func diferencaDeConjuntos(a, b []string) []string {
	presente := make(map[string]bool, len(b))
	for _, x := range b {
		presente[x] = true
	}
	var d []string
	for _, x := range a {
		if !presente[x] {
			d = append(d, x)
		}
	}
	return d
}

// TestRecusarCriacaoDeObjectos_NomeiaSchemaAusente é a prova da correcção 4 da
// revisão da Tarefa 3: recusarCriacaoDeObjectos filtrava `WHERE ns IS NOT
// NULL`, o que faz um schema ausente desaparecer da verificação em silêncio —
// hoje coberto porque recusarDono corre primeiro e exige as tabelas migradas,
// mas um 9.º schema acrescentado a schemasBC antes da migração respectiva
// passaria calado.
func TestRecusarCriacaoDeObjectos_NomeiaSchemaAusente(t *testing.T) {
	pool, ctx := ligarRuntimeParaProva(t)

	original := schemasBC
	t.Cleanup(func() { schemasBC = original })
	schemasBC = append(append([]string{}, original...), "zz_schema_inexistente_teste")

	err := recusarCriacaoDeObjectos(ctx, pool, "sgc_app")
	if err == nil {
		t.Fatal("um schema inexistente em schemasBC tinha de ser nomeado como ausente, " +
			"não saltado em silêncio")
	}
	if !strings.Contains(err.Error(), "zz_schema_inexistente_teste") {
		t.Fatalf("a mensagem de erro tem de nomear o schema ausente; obtive: %v", err)
	}
}

// TestSchemasBC_CoincideComOsSchemasAlcancaveisPeloRuntime amarra schemasBC à
// base real. Até aqui, os oito schemas existiam em DUAS cópias escritas à mão —
// schemasBC (aqui) e osOitoSchemas (tests/integration/privilegios_test.go) — e
// só a segunda tinha guarda. schemasBC não estava amarrado a nada: um bounded
// context novo com schema próprio deixava-o desactualizado em silêncio, e
// recusarCriacaoDeObjectos passava a NÃO verificar o privilégio CREATE nesse
// schema — a aplicação podia criar objectos lá, fora das migrations
// forward-only, com toda a suite verde (ADR-043, achado A1 da revisão da
// Tarefa 4).
//
// A derivação é sobre os schemas que o runtime ALCANÇA (USAGE), e não sobre
// "todos os schemas que não são do sistema", por duas razões: é exactamente a
// pergunta a que schemasBC serve de resposta — onde é que faz sentido perguntar
// se o runtime pode criar objectos — e dispensa uma segunda lista de exclusão
// escrita à mão para schemas de extensão, porque um schema de extensão a que o
// runtime não chega fica de fora sozinho.
//
// Como no resto do ficheiro, a pergunta é sobre a UNIÃO dos papéis que
// current_user pode assumir por SET ROLE e não sobre current_user: uma pertença
// NOINHERIT dá USAGE por SET ROLE sem o herdar.
func TestSchemasBC_CoincideComOsSchemasAlcancaveisPeloRuntime(t *testing.T) {
	pool, ctx := ligarRuntimeParaProva(t)

	rows, err := pool.Query(ctx, `
		SELECT n.nspname
		  FROM pg_namespace n
		 WHERE n.nspname NOT LIKE 'pg\_%'
		   AND n.nspname NOT IN ('information_schema', 'public')
		   AND EXISTS (SELECT 1 FROM pg_roles r
		                WHERE pg_has_role(current_user, r.oid, 'MEMBER')
		                  AND has_schema_privilege(r.oid, n.oid, 'USAGE'))
		 ORDER BY n.nspname`)
	if err != nil {
		t.Fatalf("derivar os schemas alcançáveis pelo runtime: %v", err)
	}
	defer rows.Close()

	var alcancaveis []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("ler linha de pg_namespace: %v", err)
		}
		alcancaveis = append(alcancaveis, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("percorrer pg_namespace: %v", err)
	}
	if len(alcancaveis) == 0 {
		t.Fatal("o runtime não alcança schema de negócio nenhum — a base não está migrada " +
			"ou o provisionamento de sgc_app não correu")
	}

	aMais := diferencaDeConjuntos(alcancaveis, schemasBC)
	aMenos := diferencaDeConjuntos(schemasBC, alcancaveis)
	if len(aMais) == 0 && len(aMenos) == 0 {
		return
	}
	t.Fatalf("schemasBC diverge dos schemas que o runtime alcança — a mais na base "+
		"(o runtime chega lá mas recusarCriacaoDeObjectos NUNCA lhe verifica o CREATE) = %v; "+
		"a menos (estão em schemasBC mas o runtime não os alcança) = %v.\n"+
		"Um schema a mais tem duas causas:\n"+
		"  (1) bounded context novo — acrescente-o a schemasBC (e a osOitoSchemas em "+
		"tests/integration/privilegios_test.go), com os GRANT mínimos na migração do BC;\n"+
		"  (2) schema de uma EXTENSÃO do PostgreSQL a que o runtime ganhou USAGE — se ele "+
		"não tem lá trabalho nenhum, o remédio é REVOKE USAGE, não alargar schemasBC; se "+
		"tiver, acrescentá-lo a schemasBC apenas ACRESCENTA a verificação de CREATE, e "+
		"nunca deve vir acompanhado de DML (ADR-043)", aMais, aMenos)
}

// TestTabelasDeValorLegal_CobreAsTabelasProtegidasPorTrigger amarra o terceiro
// inventário escrito à mão desta fatia (ADR-043, achado A1 da revisão da Tarefa
// 4). tabelasDeValorLegal decide o que recusarDono e recusarMutacaoDoValorLegal
// verificam: uma tabela protegida por trigger que ninguém acrescente à lista
// fica fora das duas verificações de arranque, em silêncio.
//
// A derivação é sobre pg_trigger, e é DERIVAR-E-ASSERIR, nunca derivar-e-usar,
// por duas razões que a revisão registou:
//
//   - "protegida por trigger" NÃO é o mesmo que "de valor legal":
//     financeiro.series é a cabeça da cadeia hash e não tem trigger nenhum (é
//     serializada por SELECT ... FOR UPDATE), pelo que nunca apareceria nesta
//     derivação. Por isso a asserção é unidireccional: uma tabela com trigger
//     TEM de estar declarada; uma tabela declarada sem trigger é legítima e só
//     é registada no log.
//   - o conjunto Proibidos de cada tabela não é derivável de pg_trigger e
//     continua declarado à mão. O que se deriva é a PERTENÇA ao conjunto — e,
//     para TRUNCATE, o facto que a torna obrigatória: um trigger que não é de
//     TRUNCATE não vê um TRUNCATE.
func TestTabelasDeValorLegal_CobreAsTabelasProtegidasPorTrigger(t *testing.T) {
	pool, ctx := ligarRuntimeParaProva(t)

	// tgisinternal exclui os triggers que o PostgreSQL cria sozinho para
	// suportar chaves estrangeiras e constraints diferidas — não são protecção
	// de valor legal e enchiam a derivação de ruído. (tgtype & 32) é o bit de
	// TRUNCATE: um trigger FOR EACH ROW sobre UPDATE/DELETE não o tem, e é
	// precisamente por isso que TRUNCATE tem de estar em Proibidos.
	rows, err := pool.Query(ctx, `
		SELECT n.nspname || '.' || c.relname,
		       bool_or((t.tgtype & 32) <> 0)
		  FROM pg_trigger t
		  JOIN pg_class c ON c.oid = t.tgrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE NOT t.tgisinternal
		   AND n.nspname = ANY($1::text[])
		 GROUP BY 1
		 ORDER BY 1`, schemasBC)
	if err != nil {
		t.Fatalf("derivar as tabelas protegidas por trigger: %v", err)
	}
	defer rows.Close()

	protegidas := map[string]bool{} // nome -> tem trigger de TRUNCATE
	var nomesProtegidos []string
	for rows.Next() {
		var nome string
		var temTriggerDeTruncate bool
		if err := rows.Scan(&nome, &temTriggerDeTruncate); err != nil {
			t.Fatalf("ler linha de pg_trigger: %v", err)
		}
		protegidas[nome] = temTriggerDeTruncate
		nomesProtegidos = append(nomesProtegidos, nome)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("percorrer pg_trigger: %v", err)
	}
	if len(nomesProtegidos) == 0 {
		t.Fatal("não há trigger nenhum nos schemas de negócio — a base não está migrada, " +
			"ou os triggers de imutabilidade das facturas e do audit log foram apagados")
	}

	declaradas := make([]string, 0, len(tabelasDeValorLegal))
	proibidosDe := make(map[string][]string, len(tabelasDeValorLegal))
	for _, tabela := range tabelasDeValorLegal {
		declaradas = append(declaradas, tabela.Nome)
		proibidosDe[tabela.Nome] = tabela.Proibidos
	}

	if emFalta := diferencaDeConjuntos(nomesProtegidos, declaradas); len(emFalta) > 0 {
		t.Fatalf("há tabelas protegidas por trigger que não estão em tabelasDeValorLegal: %v — "+
			"recusarDono e recusarMutacaoDoValorLegal não as verificam, pelo que o runtime pode "+
			"passar a ser dono delas (e desligar-lhes o trigger) sem que a verificação de "+
			"arranque diga nada. Declare-as, com o conjunto Proibidos que lhes cabe (ADR-043)",
			emFalta)
	}

	// O sentido contrário NÃO é falha: uma tabela pode ser de valor legal sem
	// trigger — financeiro.series é o exemplo vivo. Fica registado para quem lê
	// a saída perceber que a diferença é conhecida e não um esquecimento.
	if semTrigger := diferencaDeConjuntos(declaradas, nomesProtegidos); len(semTrigger) > 0 {
		t.Logf("tabelas de valor legal declaradas sem trigger (legítimo — a protecção pode ser "+
			"outra, como o SELECT ... FOR UPDATE de financeiro.series): %v", semTrigger)
	}

	for _, nome := range nomesProtegidos {
		if protegidas[nome] {
			continue // tem trigger de TRUNCATE: o TRUNCATE é visto
		}
		if !contem(proibidosDe[nome], "TRUNCATE") {
			t.Errorf("%s é protegida só por triggers que não são de TRUNCATE, e TRUNCATE não "+
				"está no seu conjunto Proibidos (%v): um TRUNCATE passa ao lado do trigger e "+
				"destrói a tabela inteira sem o disparar (ADR-043)", nome, proibidosDe[nome])
		}
	}
}

func contem(lista []string, valor string) bool {
	for _, x := range lista {
		if x == valor {
			return true
		}
	}
	return false
}

// TestRecusarMutacaoDoValorLegal_RecusaTabelaSemProibidos cobre o ramo
// defensivo de recusarMutacaoDoValorLegal: uma tabela de valor legal declarada
// com o conjunto Proibidos vazio faz a verificação de arranque falhar fechado,
// em vez de a saltar em silêncio. Era a única linha de comportamento nova da
// Tarefa 3 sem cobertura (ADR-043, achado A3 da revisão da Tarefa 4).
func TestRecusarMutacaoDoValorLegal_RecusaTabelaSemProibidos(t *testing.T) {
	pool, ctx := ligarRuntimeParaProva(t)

	original := tabelasDeValorLegal
	t.Cleanup(func() { tabelasDeValorLegal = original })
	tabelasDeValorLegal = []tabelaDeValorLegal{{Nome: "financeiro.facturas"}}

	err := recusarMutacaoDoValorLegal(ctx, pool, "sgc_app")
	if err == nil {
		t.Fatal("uma tabela de valor legal sem privilégios proibidos tinha de fazer a " +
			"verificação falhar fechado, não passar por cima dela")
	}
	if !strings.Contains(err.Error(), "financeiro.facturas") {
		t.Fatalf("a mensagem de erro tem de nomear a tabela mal declarada; obtive: %v", err)
	}
}
