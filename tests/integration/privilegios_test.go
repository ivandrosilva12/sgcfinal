//go:build integration

// Provas da separação de credenciais (ADR-043 / R7 da ADR-040). Correm ligadas
// como sgc_app — o papel de runtime — e verificam que ele NÃO consegue subverter
// as garantias que a base de dados impõe por trigger, e que CONTINUA a conseguir
// fazer o trabalho legítimo da aplicação.
package integration_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// osOitoSchemas são os schemas de negócio da base de dados: os sete a que o
// GRANT em massa da migração 0003 dá acesso, mais auditoria (que recebe um
// GRANT mais restrito, sem UPDATE/DELETE/TRUNCATE). É, ela própria, uma lista
// mantida à mão — por isso nenhuma prova desta suite se limita a usá-la:
// exigirSchemasDeNegocioCoincidentes deriva os schemas de negócio
// directamente da base (pg_namespace) e assere que os dois conjuntos
// coincidem exactamente, antes de qualquer varrimento. Um bounded context
// futuro com um schema novo faz essa asserção falhar — em vez de deixar o
// varrimento passar em silêncio sobre um schema que ninguém enumerou.
var osOitoSchemas = []string{
	"auditoria", "clinico", "farmacia", "financeiro",
	"identidade", "laboratorio", "recepcao", "shared",
}

// schemasDeExtensao é a lista de exclusão declarada para a SEGUNDA causa de um
// schema aparecer na base sem estar em osOitoSchemas: um schema criado por uma
// EXTENSÃO do PostgreSQL, e não por um bounded context.
//
// As duas causas existem em separado porque o remédio é OPOSTO. Para um
// bounded context novo, o remédio é conceder USAGE + DML a sgc_app na migração
// do BC e acrescentá-lo a osOitoSchemas. Para um schema de extensão, conceder
// DML seria alargar a superfície do runtime exactamente onde esta guarda
// existe para a estreitar — o remédio certo é declará-lo aqui, sem GRANT
// nenhum. Colapsar as duas causas numa só mensagem faria a guarda ditar, com
// autoridade, o remédio inseguro (ADR-043, Minor A da revisão da Tarefa 3).
//
// Hoje está vazia, e é o estado correcto: as três extensões instaladas
// (plpgsql, btree_gist, pg_trgm — medido em pg_extension) vivem em pg_catalog
// e public, ambos já excluídos pelo filtro. Extensões que instalam schema
// próprio — pg_cron→cron, postgis→postgis, timescaledb→_timescaledb_catalog,
// pg_repack→repack — passariam o filtro e cairiam aqui.
//
// Nota: a derivação exclui já, automaticamente, os schemas que o PostgreSQL
// ATRIBUI a uma extensão em pg_depend (deptype 'e'), que é o caso de quem faz
// `CREATE EXTENSION` sem schema pré-criado. Esta lista cobre o resto: quem
// cria o schema à mão e só depois lá instala a extensão, caso em que o
// PostgreSQL não regista dependência nenhuma e o schema fica indistinguível
// de um schema de negócio.
var schemasDeExtensao = []string{}

// migrarTudo aplica as migrations com a credencial de MIGRAÇÃO. As provas de
// privilégio precisam do esquema montado antes de se ligarem como sgc_app.
func migrarTudo(t *testing.T) {
	t.Helper()
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("aplicar migrations: %v", err)
	}
}

func TestRuntime_NaoConsegueSubverterAsGarantias(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligarApp(t)

	casos := []struct {
		nome string
		sql  string
	}{
		{"desligar triggers das facturas", `ALTER TABLE financeiro.facturas DISABLE TRIGGER ALL`},
		{"apagar o trigger de nascer rascunho", `DROP TRIGGER trg_facturas_nascem_rascunho ON financeiro.facturas`},
		{"desligar triggers na sessão", `SET session_replication_role = 'replica'`},
		{"truncar o audit log", `TRUNCATE auditoria.auditoria_eventos`},
		{"actualizar o audit log", `UPDATE auditoria.auditoria_eventos SET accao = 'adulterado'`},
		{"apagar do audit log", `DELETE FROM auditoria.auditoria_eventos`},
		{"criar objectos no financeiro", `CREATE TABLE financeiro.intruso (id int)`},
		// financeiro.facturas e financeiro.itens_factura são as outras duas
		// tabelas de valor legal (cadeia hash da ADR-040). Os triggers de
		// imutabilidade são FOR EACH ROW sobre UPDATE/DELETE — o TRUNCATE
		// passa-lhes ao lado, exactamente como acontece com o audit log. Hoje
		// falha por falta de privilégio (dono/GRANT), não por trigger; sem esta
		// prova, um GRANT TRUNCATE futuro ou uma mudança de posse reabririam o
		// buraco com a suite verde.
		{"truncar as facturas", `TRUNCATE financeiro.facturas`},
		{"truncar os itens de factura", `TRUNCATE financeiro.itens_factura`},
		// A migração 0003 afirma no comentário (linha 77): "Nada em public:
		// sgc_app não vê public.schema_migrations." Sem prova, é só uma
		// afirmação — exactamente o que esta fatia inteira existe para não
		// deixar passar (ADR-043, achado N3 da re-revisão). Medido
		// empiricamente contra o Postgres real antes de escrever a asserção:
		// ambas devolvem 42501, tal como as restantes provas negativas desta
		// tabela — falta de GRANT em public para sgc_app, não posse.
		{"ler o schema_migrations em public", `SELECT 1 FROM public.schema_migrations LIMIT 1`},
		{"criar tabela em public", `CREATE TABLE public.intruso (id int)`},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := pool.Exec(ctx, c.sql)
			if err == nil {
				t.Fatalf("o papel de runtime conseguiu %q — o R7 continua aberto", c.nome)
			}
			// Não basta que haja ALGUM erro: um erro de sintaxe ou de tabela
			// inexistente também satisfaria `err != nil` sem provar nada sobre
			// privilégios. O SQLSTATE das sete operações foi medido empiricamente
			// contra o Postgres real (ADR-043, passo de endurecimento): todas
			// devolvem 42501 (insufficient_privilege) — quer a mensagem seja
			// "must be owner of..." (DDL sobre objecto que não possui) quer
			// "permission denied for..." (falta de GRANT), o PostgreSQL classifica
			// ambas na mesma classe 42 (syntax_error_or_access_rule_violation),
			// código 42501.
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
				t.Fatalf("esperava SQLSTATE 42501 (insufficient_privilege) em %q, obtive: %v", c.nome, err)
			}
		})
	}
}

func TestRuntime_ContinuaAFazerOTrabalhoLegitimo(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligarApp(t)

	if _, err := pool.Exec(ctx,
		`INSERT INTO auditoria.auditoria_eventos (actor, accao) VALUES ($1, $2)`,
		"tester", "adr043.prova"); err != nil {
		t.Fatalf("INSERT no audit log tem de continuar a funcionar: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM auditoria.auditoria_eventos`).Scan(&n); err != nil {
		t.Fatalf("SELECT no audit log tem de continuar a funcionar: %v", err)
	}
	if n == 0 {
		t.Fatal("esperava ler o evento que acabei de inserir")
	}

	// A série é o ponto de serialização da numeração (ADR-040): o runtime tem de
	// poder bloqueá-la com FOR UPDATE.
	if _, err := pool.Exec(ctx,
		`SELECT 1 FROM financeiro.series WHERE false FOR UPDATE`); err != nil {
		t.Fatalf("SELECT ... FOR UPDATE em financeiro.series tem de funcionar: %v", err)
	}
}

func TestRuntime_TriggerDeRascunhoContinuaAMorder(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligarApp(t)

	// Não basta que sgc_app não possa desligar o trigger: o trigger tem de
	// continuar a disparar para ele. Sem esta prova, um GRANT errado poderia
	// deixar passar facturas fabricadas sem ninguém dar por isso.
	//
	// As colunas são as NOT NULL sem default de financeiro/0001_facturas.sql:
	// cliente_nome e episodio_id (id tem DEFAULT gen_random_uuid()).
	_, err := pool.Exec(ctx,
		`INSERT INTO financeiro.facturas (estado, cliente_nome, episodio_id)
		 VALUES ('EMITIDA', 'Prova ADR-043', gen_random_uuid())`)
	if err == nil {
		t.Fatal("uma factura EMITIDA à nascença tinha de ser rejeitada pelo trigger")
	}
	// Tem de falhar PELO TRIGGER, não por violação de NOT NULL ou de CHECK: uma
	// prova que passa a verde pela razão errada não prova nada. A mensagem por
	// si só não chega — um CHECK constraint com "RASCUNHO" no texto passaria
	// pela mesma verificação sem ser o trigger. O SQLSTATE foi medido
	// empiricamente contra o Postgres real: financeiro.impedir_factura_nascer_
	// emitida() levanta ERRCODE = 'restrict_violation', classe 23 (integrity_
	// constraint_violation), código 23001 — diferente do 42501 das provas
	// negativas de privilégio acima, porque aqui o trigger é que dispara, não
	// uma falta de GRANT.
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23001" {
		t.Fatalf("esperava SQLSTATE 23001 (restrict_violation) do trigger de nascer rascunho, obtive: %v", err)
	}
	if !strings.Contains(err.Error(), "RASCUNHO") {
		t.Fatalf("esperava a rejeição do trigger de nascer rascunho, obtive: %v", err)
	}
}

// diferencaDeConjuntos devolve os elementos de a que não estão em b. Usada
// pela guarda de schemas de TestRuntime_AcedeATodasAsTabelasDosOitoSchemas
// para nomear, na mensagem de falha, exactamente quais schemas estão a mais e
// quais a menos — em vez de um "não coincide" sem detalhe.
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

// exigirSchemasDeNegocioCoincidentes deriva, de forma independente, os
// schemas de negócio que existem de facto na base e assere que esse conjunto
// coincide exactamente com osOitoSchemas — reportando quais estão a mais e
// quais a menos, e nomeando as DUAS causas possíveis com o remédio de cada uma
// (ver schemasDeExtensao). É a mesma disciplina da guarda AST da ADR-042:
// conjunto nomeado, derivação independente, asserção de que concordam.
//
// Vive num helper e não dentro de um teste porque é o pré-requisito de tudo o
// que varre "os oito schemas": sem ela, um nono schema não enumerado deixaria
// os varrimentos a passar em silêncio precisamente sobre o schema novo.
// Escrever a lista à mão uma segunda vez dentro de outro teste reintroduziria
// esse buraco (ADR-043, correcção 1 da revisão da Tarefa 4).
//
// Recebe a ligação de MIGRAÇÃO: enumerar com a credencial sob teste seria
// circular — um schema sem USAGE para sgc_app não deixa de ser real por isso.
func exigirSchemasDeNegocioCoincidentes(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	// deptype 'e': o objecto pertence a uma extensão. Cobre o schema que a
	// própria `CREATE EXTENSION` cria; o resto vive em schemasDeExtensao.
	rows, err := pool.Query(ctx, `
		SELECT n.nspname
		  FROM pg_namespace n
		 WHERE n.nspname NOT LIKE 'pg\_%'
		   AND n.nspname NOT IN ('information_schema', 'public')
		   AND NOT (n.nspname = ANY($1::text[]))
		   AND NOT EXISTS (SELECT 1 FROM pg_depend d
		                    WHERE d.classid = 'pg_namespace'::regclass
		                      AND d.objid = n.oid
		                      AND d.deptype = 'e')
		 ORDER BY n.nspname`, schemasDeExtensao)
	if err != nil {
		t.Fatalf("derivar os schemas de negócio de pg_namespace: %v", err)
	}
	defer rows.Close()

	var schemasDaBase []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("ler linha de pg_namespace: %v", err)
		}
		schemasDaBase = append(schemasDaBase, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("percorrer pg_namespace: %v", err)
	}

	aMais := diferencaDeConjuntos(schemasDaBase, osOitoSchemas)
	aMenos := diferencaDeConjuntos(osOitoSchemas, schemasDaBase)
	if len(aMais) == 0 && len(aMenos) == 0 {
		return
	}
	t.Fatalf("osOitoSchemas diverge dos schemas de negócio reais da base — "+
		"a mais na base (existem mas não estão em osOitoSchemas, por isso NUNCA "+
		"seriam enumerados pelas verificações de privilégio) = %v; a menos na base "+
		"(estão em osOitoSchemas mas não existem de facto) = %v.\n"+
		"Um schema a mais tem DUAS causas possíveis, com remédios OPOSTOS:\n"+
		"  (1) bounded context novo — trate-o deliberadamente: GRANT USAGE e o DML "+
		"mínimo a sgc_app na migração do BC, mais a entrada em osOitoSchemas;\n"+
		"  (2) schema de uma EXTENSÃO do PostgreSQL instalada fora de pg_catalog/public "+
		"(pg_cron→cron, postgis→postgis, timescaledb→_timescaledb_catalog, "+
		"pg_repack→repack) — NÃO lhe conceda DML nenhum: declare-o em schemasDeExtensao. "+
		"Aplicar o remédio (1) a um schema de extensão alarga a superfície do runtime "+
		"exactamente onde esta guarda existe para a estreitar (ADR-043)",
		aMais, aMenos)
}

// TestRuntime_AcedeATodasAsTabelasDosOitoSchemas é a cobertura positiva que
// faltava: até aqui, TestRuntime_ContinuaAFazerOTrabalhoLegitimo só exercitava
// duas tabelas (auditoria_eventos, financeiro.series) como sgc_app — o resto
// das 31 tabelas dos oito schemas nunca fora tocado por essa credencial. A
// afirmação "nenhuma prova pré-existente falhou por permissões" não media nada:
// os 25 ficheiros de teste de integração pré-existentes ligam-se todos como
// sgc (migração), nunca como sgc_app.
//
// Este teste começa por exigir que osOitoSchemas coincida com os schemas de
// negócio reais da base (exigirSchemasDeNegocioCoincidentes). Só depois enumera
// as tabelas a partir de information_schema.tables (com a credencial de
// migração, que tem de ver tudo) e tenta um SELECT em cada uma como sgc_app.
// As duas verificações juntas apanham, de uma vez, um bounded context novo com
// schema próprio — a asserção de igualdade fica vermelha antes de qualquer
// varrimento, em vez de passar em silêncio — USAGE em falta num schema já
// conhecido, e SELECT em falta numa tabela.
func TestRuntime_AcedeATodasAsTabelasDosOitoSchemas(t *testing.T) {
	migrarTudo(t)
	poolMigracao, ctxMigracao := ligar(t)
	poolApp, ctxApp := ligarApp(t)

	exigirSchemasDeNegocioCoincidentes(t, ctxMigracao, poolMigracao)

	rows, err := poolMigracao.Query(ctxMigracao,
		`SELECT table_schema, table_name FROM information_schema.tables
		 WHERE table_schema = ANY($1) AND table_type = 'BASE TABLE'
		 ORDER BY table_schema, table_name`, osOitoSchemas)
	if err != nil {
		t.Fatalf("enumerar tabelas dos oito schemas: %v", err)
	}
	defer rows.Close()

	type tabela struct{ schema, nome string }
	var tabelas []tabela
	for rows.Next() {
		var tb tabela
		if err := rows.Scan(&tb.schema, &tb.nome); err != nil {
			t.Fatalf("ler linha de information_schema.tables: %v", err)
		}
		tabelas = append(tabelas, tb)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("percorrer information_schema.tables: %v", err)
	}
	if len(tabelas) == 0 {
		t.Fatal("esperava encontrar tabelas nos oito schemas — a consulta ou a lista de schemas está errada")
	}

	for _, tb := range tabelas {
		t.Run(tb.schema+"."+tb.nome, func(t *testing.T) {
			ident := pgx.Identifier{tb.schema, tb.nome}.Sanitize()
			if _, err := poolApp.Exec(ctxApp, fmt.Sprintf(`SELECT 1 FROM %s LIMIT 1`, ident)); err != nil {
				t.Fatalf("sgc_app não conseguiu SELECT em %s.%s: %v", tb.schema, tb.nome, err)
			}
		})
	}
}

// TestRuntime_OperacoesLegitimasAdicionais cobre três operações legítimas da
// aplicação que a suite, até aqui, nunca tinha exercitado como sgc_app: o
// UPDATE de versao em financeiro.facturas (bloqueio optimista, ADR-040), o
// INSERT/DELETE em financeiro.itens_factura (reescrita de rascunho por
// upsert) e o nextval da única sequência explícita da base — sem isto, o
// GRANT USAGE, SELECT ON ALL SEQUENCES da migração 0003 ficava por exercitar.
// Cada asserção limpa o que insere, para não deixar resíduo na base de
// desenvolvimento.
func TestRuntime_OperacoesLegitimasAdicionais(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligarApp(t)

	// UPDATE em financeiro.facturas: insere em RASCUNHO (as colunas NOT NULL
	// sem default são cliente_nome e episodio_id) e incrementa a versao — é
	// exactamente o que o bloqueio optimista da ADR-040 faz a cada escrita.
	var facturaID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO financeiro.facturas (estado, cliente_nome, episodio_id)
		 VALUES ('RASCUNHO', 'Prova ADR-043 sweep', gen_random_uuid())
		 RETURNING id`).Scan(&facturaID); err != nil {
		t.Fatalf("INSERT de factura em RASCUNHO tem de funcionar: %v", err)
	}
	t.Cleanup(func() {
		// A factura fica em RASCUNHO: o trigger de imutabilidade só morde
		// OLD.estado <> 'RASCUNHO', pelo que sgc_app consegue apagar a sua
		// própria prova sem precisar da credencial de migração.
		if _, err := pool.Exec(ctx, `DELETE FROM financeiro.facturas WHERE id = $1`, facturaID); err != nil {
			t.Logf("limpeza da factura de prova falhou: %v", err)
		}
	})

	if _, err := pool.Exec(ctx,
		`UPDATE financeiro.facturas SET versao = versao + 1 WHERE id = $1`, facturaID); err != nil {
		t.Fatalf("UPDATE da versao em financeiro.facturas (bloqueio optimista) tem de funcionar: %v", err)
	}

	// INSERT e DELETE em financeiro.itens_factura, ainda em RASCUNHO.
	var itemID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO financeiro.itens_factura
			(factura_id, descricao, tipo, quantidade, preco_unitario_centimos, regime_iva)
		 VALUES ($1, 'Prova ADR-043', 'CONSULTA', 1, 100, 'STANDARD')
		 RETURNING id`, facturaID).Scan(&itemID); err != nil {
		t.Fatalf("INSERT de item de factura em RASCUNHO tem de funcionar: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM financeiro.itens_factura WHERE id = $1`, itemID); err != nil {
		t.Fatalf("DELETE de item de factura em RASCUNHO (reescrita por upsert) tem de funcionar: %v", err)
	}

	// nextval da única sequência explícita da base. Não há linha a limpar: o
	// nextval não insere dados, só avança o contador da sequência (efeito
	// inócuo e não revertível de forma segura — não vale a pena um setval
	// especulativo que poderia colidir com outro teste concorrente).
	var codigo int64
	if err := pool.QueryRow(ctx,
		`SELECT nextval('farmacia.seq_codigo_medicamento')`).Scan(&codigo); err != nil {
		t.Fatalf("nextval em farmacia.seq_codigo_medicamento tem de funcionar: %v", err)
	}
}

// TestRuntime_SeriesSemDelete prova a correcção 3 da revisão da Tarefa 2
// (ADR-043): financeiro.series guarda ultimo_sequencial e ultimo_hash — a
// cabeça da cadeia hash de facturas da ADR-040 — e, ao contrário de
// facturas/itens_factura, não tem trigger nenhum a proteger UPDATE/DELETE.
// Verificado contra a base viva antes desta migração: sgc_app conseguia
// DELETE ali. A re-emissão continua travada pelos índices únicos
// uq_facturas_serie_sequencial/uq_facturas_numero — não há forja de
// facturas — mas apagar a linha perde o ultimo_hash e o elo seguinte da
// cadeia nasce partido, numa tabela imutável por desenho: dano não
// reparável. SELECT, INSERT e UPDATE continuam a ser trabalho legítimo do
// runtime (é a linha bloqueada com FOR UPDATE na emissão).
func TestRuntime_SeriesSemDelete(t *testing.T) {
	migrarTudo(t)
	poolApp, ctxApp := ligarApp(t)
	poolMigracao, ctxMigracao := ligar(t)

	const serieProva = "PROVA-ADR043-SERIES"
	t.Cleanup(func() {
		// sgc_app já não consegue DELETE em financeiro.series — é precisamente
		// o que este teste prova — pelo que a limpeza usa a credencial de
		// migração.
		if _, err := poolMigracao.Exec(ctxMigracao,
			`DELETE FROM financeiro.series WHERE serie = $1`, serieProva); err != nil {
			t.Logf("limpeza da série de prova falhou: %v", err)
		}
	})

	if _, err := poolApp.Exec(ctxApp,
		`SELECT 1 FROM financeiro.series WHERE false FOR UPDATE`); err != nil {
		t.Fatalf("SELECT ... FOR UPDATE em financeiro.series tem de continuar a funcionar: %v", err)
	}

	if _, err := poolApp.Exec(ctxApp,
		`INSERT INTO financeiro.series (serie) VALUES ($1)`, serieProva); err != nil {
		t.Fatalf("INSERT em financeiro.series tem de continuar a funcionar: %v", err)
	}

	if _, err := poolApp.Exec(ctxApp,
		`UPDATE financeiro.series SET ultimo_sequencial = ultimo_sequencial + 1 WHERE serie = $1`,
		serieProva); err != nil {
		t.Fatalf("UPDATE em financeiro.series tem de continuar a funcionar: %v", err)
	}

	_, err := poolApp.Exec(ctxApp, `DELETE FROM financeiro.series WHERE serie = $1`, serieProva)
	if err == nil {
		t.Fatal("sgc_app conseguiu DELETE em financeiro.series — a migração 0004_series_sem_delete não fechou o privilégio")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("esperava SQLSTATE 42501 (insufficient_privilege) no DELETE de financeiro.series, obtive: %v", err)
	}
}

// privilegiosEsperados devolve o conjunto EXACTO de privilégios que sgc_app
// tem de ter sobre uma relação dos oito schemas, por ordem alfabética (a mesma
// do string_agg da consulta). É a transcrição fiel do que as migrações
// concedem, e não uma lista independente: por isso a regra é por SCHEMA, com
// as excepções por tabela declaradas à parte — uma tabela nova no schema
// auditoria herda automaticamente a expectativa certa, em vez de nascer fora
// do inventário.
//
//   - migrations/shared/0003_papel_runtime.sql concede, nos sete schemas de
//     negócio, SELECT/INSERT/UPDATE/DELETE em tabelas e USAGE/SELECT em
//     sequências; e, no schema auditoria, apenas SELECT/INSERT em tabelas.
//   - migrations/shared/0004_series_sem_delete.sql revoga o DELETE em
//     financeiro.series.
func privilegiosEsperados(schema, nome string, ehSequencia bool) []string {
	if ehSequencia {
		// Assimetria conhecida e deliberada (documentada em
		// migrations/shared/0004): o schema auditoria NÃO recebe privilégios de
		// sequência, nem por GRANT em massa nem por default privileges. A única
		// sequência que lá existe é auditoria_eventos_id_seq, criada
		// implicitamente pela coluna GENERATED ALWAYS AS IDENTITY de
		// migrations/auditoria/0001 — e uma coluna IDENTITY não consome
		// USAGE/SELECT na sequência (o INSERT no audit log funciona sem eles, o
		// que TestRuntime_ContinuaAFazerOTrabalhoLegitimo prova). Conceder ali
		// privilégio seria dar algo que nada consome. Medido contra a base:
		// auditoria.auditoria_eventos_id_seq tem ACL vazia para sgc_app,
		// enquanto farmacia.seq_codigo_medicamento e shared.outbox_id_seq têm
		// SELECT+USAGE. O teste reflecte a assimetria em vez de a esconder.
		//
		// UPDATE não é concedido em sequência nenhuma: só `setval` o consome, e
		// reescrever o contador não é trabalho legítimo do runtime — o nextval
		// basta-se com USAGE.
		if schema == "auditoria" {
			return nil
		}
		return []string{"SELECT", "USAGE"}
	}
	// Append-only: retenção obrigatória de 10 anos (LPDP / Lei 22/11). Sem
	// UPDATE nem DELETE ao nível do privilégio, a imutabilidade do audit log
	// deixa de depender exclusivamente do trigger trg_auditoria_imutavel.
	if schema == "auditoria" {
		return []string{"INSERT", "SELECT"}
	}
	// financeiro.series é a cabeça da cadeia hash e da numeração sem buracos da
	// ADR-040 (ultimo_sequencial, ultimo_hash) e não tem trigger nenhum a
	// protegê-la — é serializada por SELECT ... FOR UPDATE. Apagar a linha perde
	// o ultimo_hash e o elo seguinte nasce partido, sem reparação possível
	// (migrations/shared/0004). SELECT, INSERT e UPDATE continuam a ser trabalho
	// legítimo do runtime.
	if schema == "financeiro" && nome == "series" {
		return []string{"INSERT", "SELECT", "UPDATE"}
	}
	return []string{"DELETE", "INSERT", "SELECT", "UPDATE"}
}

// listaDePrivilegios parte a lista separada por vírgulas que a consulta
// devolve, tratando a lista vazia como conjunto vazio: strings.Split("", ",")
// devolveria [""], que envenenaria as diferenças de conjuntos com um elemento
// fantasma.
func listaDePrivilegios(csv string) []string {
	if csv == "" {
		return nil
	}
	return strings.Split(csv, ",")
}

// TestPrivilegios_InventarioExactoDeTabelasESequencias é a guarda de deriva do
// inventário de privilégios (ADR-043, Tarefa 4). Existe por duas razões
// distintas:
//
//  1. O `ALTER DEFAULT PRIVILEGES` da migração 0003 cobre tabelas novas em
//     schemas existentes, mas NÃO cobre um schema novo — que é o que um
//     bounded context novo traz. Uma tabela órfã sem GRANT nenhum aparece aqui
//     como privilégios "a menos".
//
//  2. Verificar apenas a PRESENÇA de SELECT apanharia a deriva num sentido e
//     seria cega ao outro, que é o perigoso: um `GRANT TRUNCATE` colado a uma
//     tabela por engano passaria despercebido, e TRUNCATE não dispara triggers
//     FOR EACH ROW — é assim que se destrói a cadeia de hash das facturas e o
//     audit log (medido na Tarefa 3: com TRUNCATE concedido, as quatro
//     interrogações de VerificarPapelRuntime ficavam limpas e o `TRUNCATE
//     financeiro.itens_factura, financeiro.facturas` executou). Por isso a
//     asserção é sobre o conjunto EXACTO, relação a relação, e não sobre a
//     presença de um privilégio.
//
// Cobre tabelas E sequências. Cobrir só tabelas seria a mesma classe de
// defeito que esta fatia já pagou duas vezes: âmbito real mais estreito do que
// o nome promete.
//
// O inventário é lido de pg_class/aclexplode com a credencial de MIGRAÇÃO, e
// não de has_table_privilege por privilégio nomeado, deliberadamente: a lista
// dos privilégios que existem é um facto da VERSÃO do PostgreSQL, não deste
// projecto, e uma lista fixa ficaria atrás da próxima versão em silêncio (o
// PG17 acrescenta MAINTAIN, que o PG16 nem reconhece — medido: `unrecognized
// privilege type`). Ler a ACL devolve qualquer privilégio que o servidor
// conheça. O filtro sobre grantee inclui o pseudo-papel PUBLIC (oid 0) e os
// papéis que sgc_app pode assumir por SET ROLE — um `GRANT TRUNCATE ... TO
// PUBLIC` chega a sgc_app na mesma.
//
// Torna redundante uma verificação separada só sobre
// auditoria.auditoria_eventos: a expectativa append-only dessa tabela é
// asserida aqui, no mesmo sítio que todas as outras, para que não possam
// existir duas verificações a dizer coisas diferentes sobre a mesma tabela.
func TestPrivilegios_InventarioExactoDeTabelasESequencias(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligar(t)

	// Sem isto, um nono schema não enumerado deixava o varrimento abaixo a
	// passar em silêncio precisamente sobre o schema novo.
	exigirSchemasDeNegocioCoincidentes(t, ctx, pool)

	const q = `
		SELECT n.nspname, c.relname, c.relkind = 'S',
		       coalesce(string_agg(DISTINCT a.privilege_type, ',' ORDER BY a.privilege_type), ''),
		       CASE WHEN c.relkind = 'r'
		            THEN has_table_privilege('sgc_app', c.oid, 'TRUNCATE') END
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  LEFT JOIN LATERAL (
		        SELECT x.privilege_type
		          FROM aclexplode(coalesce(c.relacl,
		                 acldefault((CASE c.relkind WHEN 'S' THEN 's' ELSE 'r' END)::"char",
		                            c.relowner))) x
		         WHERE x.grantee = 0 OR pg_has_role('sgc_app', x.grantee, 'MEMBER')) a ON TRUE
		 WHERE c.relkind IN ('r', 'S') AND n.nspname = ANY($1::text[])
		 GROUP BY n.nspname, c.relname, c.relkind, c.oid
		 ORDER BY n.nspname, c.relname`

	rows, err := pool.Query(ctx, q, osOitoSchemas)
	if err != nil {
		t.Fatalf("inventariar os privilégios de sgc_app: %v", err)
	}
	defer rows.Close()

	type relacao struct {
		schema, nome  string
		ehSequencia   bool
		privilegios   string
		truncEfectivo *bool
	}
	var relacoes []relacao
	for rows.Next() {
		var r relacao
		if err := rows.Scan(&r.schema, &r.nome, &r.ehSequencia, &r.privilegios, &r.truncEfectivo); err != nil {
			t.Fatalf("ler linha do inventário de privilégios: %v", err)
		}
		relacoes = append(relacoes, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("percorrer o inventário de privilégios: %v", err)
	}
	if len(relacoes) == 0 {
		t.Fatal("esperava encontrar tabelas e sequências nos oito schemas — " +
			"a consulta do inventário ou a lista de schemas está errada")
	}

	for _, r := range relacoes {
		t.Run(r.schema+"."+r.nome, func(t *testing.T) {
			especie := "tabela"
			if r.ehSequencia {
				especie = "sequência"
			}
			esperados := privilegiosEsperados(r.schema, r.nome, r.ehSequencia)
			esperado := strings.Join(esperados, ",")
			if r.privilegios != esperado {
				tem := listaDePrivilegios(r.privilegios)
				aMais := diferencaDeConjuntos(tem, esperados)
				aMenos := diferencaDeConjuntos(esperados, tem)
				t.Fatalf("os privilégios de sgc_app sobre a %s %s.%s divergem do inventário "+
					"declarado — tem [%s], devia ter [%s]; a mais = %v; a menos = %v.\n"+
					"A MAIS é o caso perigoso: um privilégio concedido por engano (TRUNCATE "+
					"acima de todos, que não dispara triggers FOR EACH ROW) destrói valor legal "+
					"sem deixar rasto — revogue-o por migração nova. A MENOS parte a aplicação: "+
					"acrescente o GRANT à migração que criou a relação, ou trate o caso em "+
					"privilegiosEsperados se a regra mudou deliberadamente (ADR-043)",
					especie, r.schema, r.nome, r.privilegios, esperado, aMais, aMenos)
			}
			// A ACL diz o que foi CONCEDIDO; has_table_privilege diz o que o papel
			// EFECTIVAMENTE pode. As duas divergem se sgc_app for superuser — que
			// não precisa de ACL nenhuma e pode tudo. VerificarPapelRuntime já
			// recusa arrancar nesse caso, mas esta prova não depende dela: no
			// privilégio que destrói a cadeia de hash, as duas respostas têm de
			// concordar.
			if r.truncEfectivo != nil && *r.truncEfectivo {
				t.Fatalf("sgc_app tem TRUNCATE efectivo em %s.%s apesar de a ACL não o conceder — "+
					"sinal de que o papel é superuser, ou de que o privilégio lhe chega por uma "+
					"via que a ACL da relação não mostra (ADR-043)", r.schema, r.nome)
			}
		})
	}
}
