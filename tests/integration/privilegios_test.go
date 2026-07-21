//go:build integration

// Provas da separação de credenciais (ADR-043 / R7 da ADR-040). Correm ligadas
// como sgc_app — o papel de runtime — e verificam que ele NÃO consegue subverter
// as garantias que a base de dados impõe por trigger, e que CONTINUA a conseguir
// fazer o trabalho legítimo da aplicação.
package integration_test

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// osOitoSchemas são os schemas de negócio da base de dados: os sete a que o
// GRANT em massa da migração 0003 dá acesso, mais auditoria (que recebe um
// GRANT mais restrito, sem UPDATE/DELETE/TRUNCATE). É, ela própria, uma lista
// mantida à mão — por isso TestRuntime_AcedeATodasAsTabelasDosOitoSchemas não
// se limita a usá-la: primeiro deriva os schemas de negócio directamente da
// base (pg_namespace) e assere que os dois conjuntos coincidem exactamente,
// antes de fazer qualquer varrimento de tabelas. Um bounded context futuro com
// um schema novo faz essa asserção falhar — em vez de deixar o varrimento
// passar em silêncio sobre um schema que ninguém enumerou.
var osOitoSchemas = []string{
	"auditoria", "clinico", "farmacia", "financeiro",
	"identidade", "laboratorio", "recepcao", "shared",
}

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

// TestRuntime_AcedeATodasAsTabelasDosOitoSchemas é a cobertura positiva que
// faltava: até aqui, TestRuntime_ContinuaAFazerOTrabalhoLegitimo só exercitava
// duas tabelas (auditoria_eventos, financeiro.series) como sgc_app — o resto
// das 31 tabelas dos oito schemas nunca fora tocado por essa credencial. A
// afirmação "nenhuma prova pré-existente falhou por permissões" não media nada:
// os 25 ficheiros de teste de integração pré-existentes ligam-se todos como
// sgc (migração), nunca como sgc_app.
//
// Este teste começa por derivar, de forma independente, os schemas de negócio
// que existem de facto na base (pg_namespace, com a credencial de MIGRAÇÃO —
// enumerar com a credencial sob teste seria circular: um schema sem USAGE
// para sgc_app não deixaria de ser real só por isso, foi bem feito da
// primeira vez) e assere que esse conjunto coincide exactamente com
// osOitoSchemas, reportando quais schemas estão a mais e quais a menos se não
// coincidir — a mesma disciplina da guarda AST da ADR-042: conjunto nomeado,
// derivação independente, asserção de que concordam. Só depois enumera as
// tabelas a partir de information_schema.tables (também com a credencial de
// migração, que tem de ver tudo) e tenta um SELECT em cada uma como sgc_app.
// As duas verificações juntas apanham, de uma vez, um bounded context novo com
// schema próprio — a asserção de igualdade fica vermelha antes de qualquer
// varrimento, em vez de passar em silêncio — USAGE em falta num schema já
// conhecido, e SELECT em falta numa tabela.
func TestRuntime_AcedeATodasAsTabelasDosOitoSchemas(t *testing.T) {
	migrarTudo(t)
	poolMigracao, ctxMigracao := ligar(t)
	poolApp, ctxApp := ligarApp(t)

	schemasRows, err := poolMigracao.Query(ctxMigracao, `
		SELECT nspname FROM pg_namespace
		WHERE nspname NOT LIKE 'pg\_%' AND nspname NOT IN ('information_schema', 'public')
		ORDER BY nspname`)
	if err != nil {
		t.Fatalf("derivar os schemas de negócio de pg_namespace: %v", err)
	}
	var schemasDaBase []string
	for schemasRows.Next() {
		var s string
		if err := schemasRows.Scan(&s); err != nil {
			t.Fatalf("ler linha de pg_namespace: %v", err)
		}
		schemasDaBase = append(schemasDaBase, s)
	}
	if err := schemasRows.Err(); err != nil {
		t.Fatalf("percorrer pg_namespace: %v", err)
	}
	schemasRows.Close()

	aMais := diferencaDeConjuntos(schemasDaBase, osOitoSchemas)
	aMenos := diferencaDeConjuntos(osOitoSchemas, schemasDaBase)
	if len(aMais) > 0 || len(aMenos) > 0 {
		t.Fatalf("osOitoSchemas diverge dos schemas de negócio reais da base — "+
			"a mais na base (schemas que existem mas não estão em osOitoSchemas, "+
			"por isso NUNCA seriam enumerados abaixo) = %v; a menos na base "+
			"(estão em osOitoSchemas mas não existem de facto) = %v — um bounded "+
			"context novo precisa de ser tratado deliberadamente (privilégios na "+
			"migração + entrada em osOitoSchemas) antes de este teste voltar a verde",
			aMais, aMenos)
	}

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
