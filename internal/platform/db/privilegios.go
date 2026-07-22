package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schemasBC são os oito schemas por bounded context. O papel de runtime tem
// USAGE em todos e CREATE em nenhum. Atenção: `recepcao` não é criado pelo
// init.sql — nasce na sua própria migração.
var schemasBC = []string{
	"auditoria", "clinico", "farmacia", "financeiro",
	"identidade", "laboratorio", "recepcao", "shared",
}

// tabelaDeValorLegal junta o nome da tabela ao conjunto de privilégios que o
// papel de runtime não pode ter sobre ela. Os dois campos vivem na MESMA
// estrutura de propósito: era a separação entre uma lista de tabelas e uma
// consulta com UMA tabela fixa no texto que deixava duas das três
// desprotegidas contra TRUNCATE (ADR-043, correcção 7 da revisão da Tarefa 3).
type tabelaDeValorLegal struct {
	Nome      string
	Proibidos []string
}

// tabelasDeValorLegal são as tabelas cuja posse daria ao runtime o poder de
// desligar os triggers que as protegem (ADR-040, ADR-042 §2.6).
//
// O conjunto proibido NÃO é o mesmo nas quatro, e colapsá-lo partiria a
// aplicação: a factura em RASCUNHO é mutável (ADR-039) e sgc_app tem hoje
// UPDATE e DELETE nas duas tabelas do Financeiro — é trabalho legítimo. O que
// não pode ter em nenhuma das quatro é TRUNCATE: os três triggers de
// imutabilidade são FOR EACH ROW (verificado em pg_get_triggerdef para
// trg_facturas_imutaveis, trg_itens_factura_imutaveis e trg_auditoria_imutavel)
// e TRUNCATE não dispara triggers de linha. Em auditoria_eventos, append-only
// significa também sem UPDATE nem DELETE.
//
// Medido contra sgc-postgres-1, com GRANT DIRECTO e sem sequer precisar de
// SET ROLE: `GRANT TRUNCATE ON financeiro.facturas, financeiro.itens_factura TO
// sgc_app` deixava as quatro interrogações limpas — o servidor arrancava — e
// `TRUNCATE financeiro.itens_factura, financeiro.facturas` executou. É a
// destruição integral da cadeia de hash, da numeração sem buracos e da base do
// SAF-T-AO/AGT (ADR-040/041, CLAUDE.md §5.4).
//
// financeiro.series é a QUARTA entrada e a única sem trigger nenhum: guarda
// ultimo_sequencial e ultimo_hash — a cabeça da numeração sem buracos e da
// cadeia hash da ADR-040 — e é serializada por SELECT ... FOR UPDATE, não por
// trigger. Apagar a linha perde o ultimo_hash e o elo seguinte nasce partido:
// dano não reparável. Faltava aqui, e a falta era invisível por construção —
// TestTabelasDeValorLegal_CobreAsTabelasProtegidasPorTrigger deriva de
// pg_trigger e nunca a veria (ADR-043, Important I1 da revisão final).
// Reproduzido contra sgc-postgres-1 em transacção revertida: com
// `GRANT TRUNCATE, DELETE ON financeiro.series TO sgc_app` o servidor
// arrancava, e como sgc_app o TRUNCATE levou 32 linhas a 0.
//
// Só DELETE e TRUNCATE são proibidos: SELECT, INSERT e UPDATE são o coração da
// emissão (o FOR UPDATE da numeração e o UPDATE do ultimo_hash) e proibi-los
// partiria a ADR-040.
var tabelasDeValorLegal = []tabelaDeValorLegal{
	{Nome: "financeiro.facturas", Proibidos: []string{"TRUNCATE"}},
	{Nome: "financeiro.itens_factura", Proibidos: []string{"TRUNCATE"}},
	{Nome: "financeiro.series", Proibidos: []string{"DELETE", "TRUNCATE"}},
	{Nome: "auditoria.auditoria_eventos", Proibidos: []string{"DELETE", "TRUNCATE", "UPDATE"}},
}

// nomesDasTabelasDeValorLegal é o que as consultas de existência e de posse
// consomem — derivado da declaração única acima, para que acrescentar uma
// tabela não possa deixar metade das verificações para trás.
func nomesDasTabelasDeValorLegal() []string {
	nomes := make([]string, 0, len(tabelasDeValorLegal))
	for _, tabela := range tabelasDeValorLegal {
		nomes = append(nomes, tabela.Nome)
	}
	return nomes
}

// VerificarPapelRuntime confirma que o papel com que a aplicação está ligada não
// consegue subverter as garantias que a base de dados impõe por trigger: não é
// administrador, não é — nem pode assumir — o dono das tabelas de valor legal,
// não cria objectos (nem nos schemas de negócio nem na base de dados, que
// permitiria schemas novos) e não pode destruir nenhuma das quatro tabelas de
// valor legal por uma via que os triggers de linha não vêem.
//
// Devolve erro, nunca panic; o chamador trata-o como fatal. O servidor não
// arranca com um papel privilegiado, em ambiente nenhum: falhar fechado é
// preferível a correr inseguro (ADR-043 §2.6).
func VerificarPapelRuntime(ctx context.Context, pool *pgxpool.Pool) error {
	var papel string
	if err := pool.QueryRow(ctx, `SELECT current_user`).Scan(&papel); err != nil {
		return fmt.Errorf("determinar o papel de runtime: %w", err)
	}
	if err := recusarAdministrador(ctx, pool, papel); err != nil {
		return err
	}
	if err := recusarDono(ctx, pool, papel); err != nil {
		return err
	}
	if err := recusarCriacaoDeObjectos(ctx, pool, papel); err != nil {
		return err
	}
	return recusarMutacaoDoValorLegal(ctx, pool, papel)
}

func recusarAdministrador(ctx context.Context, pool *pgxpool.Pool, papel string) error {
	// pg_has_role(..., 'MEMBER'), não os atributos do próprio papel: atributos
	// (rolsuper/rolcreaterole/rolcreatedb) não se herdam por pertença, mas o
	// poder de os assumir com SET ROLE sim. Medido contra sgc-postgres-1
	// (ADR-043, correcção da revisão da Tarefa 3):
	//
	//   CREATE ROLE zz_super_teste SUPERUSER NOLOGIN;
	//   GRANT zz_super_teste TO sgc_app;
	//
	// deixa os atributos do próprio sgc_app todos falsos (a consulta anterior
	// devolvia "não é administrador"), mas
	//
	//   SET ROLE zz_super_teste;
	//   ALTER TABLE auditoria.auditoria_eventos DISABLE TRIGGER trg_auditoria_imutavel;
	//
	// funcionou (tgenabled passou de 'O' a 'D'). É a mesma assimetria que
	// recusarDono já tratava do lado da posse; faltava aqui.
	//
	// 'MEMBER' e não 'USAGE': 'USAGE' diz se os privilégios do papel se herdam
	// automaticamente; 'MEMBER' diz se current_user pode fazer SET ROLE — e é o
	// SET ROLE que dá o poder, com ou sem herança automática. Medido: com
	//
	//   GRANT zz_noinherit_teste TO sgc_app WITH INHERIT FALSE
	//
	// pg_has_role(current_user, 'zz_noinherit_teste', 'USAGE') devolveu false
	// mas pg_has_role(..., 'MEMBER') devolveu true, e o SET ROLE seguinte
	// desligou o mesmo trigger na mesma. Uma pertença NOINHERIT passaria pela
	// versão com 'USAGE' sem ser apanhada.
	//
	// Os papéis predefinidos pg_write_server_files, pg_execute_server_program e
	// pg_read_server_files entram deliberadamente na lista: não têm rolsuper,
	// rolcreaterole nem rolcreatedb (medido: os três `f`), mas dão escrita de
	// ficheiros e execução de programas no servidor — uma via indirecta para o
	// mesmo poder (reescrever os ficheiros da base ou o próprio serviço, fora
	// do alcance de qualquer trigger). Incluir mais é seguro; excluir exigiria
	// justificar por que um poder equivalente ficaria de fora — não há essa
	// justificação aqui.
	const q = `SELECT coalesce(string_agg(r.rolname, ', '), '')
	             FROM pg_roles r
	            WHERE (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb
	                    OR r.rolname IN ('pg_write_server_files',
	                                     'pg_execute_server_program',
	                                     'pg_read_server_files'))
	              AND pg_has_role(current_user, r.oid, 'MEMBER')`
	var papeisAdmin string
	if err := pool.QueryRow(ctx, q).Scan(&papeisAdmin); err != nil {
		return fmt.Errorf("verificar a pertença a papéis administrativos: %w", err)
	}
	if papeisAdmin != "" {
		return fmt.Errorf("o papel de runtime %q é, ou pode assumir por SET ROLE, o papel "+
			"administrativo %s: pode desligar triggers e apagar o audit log. Use a credencial "+
			"de runtime em DATABASE_URL, não a de migração (ADR-043)", papel, papeisAdmin)
	}
	return nil
}

func recusarDono(ctx context.Context, pool *pgxpool.Pool, papel string) error {
	// to_regclass devolve NULL em vez de erro quando a tabela não existe, o que
	// permite distinguir "base por migrar" de "papel privilegiado".
	const qFaltam = `SELECT coalesce(string_agg(t.nome, ', '), '')
	                   FROM unnest($1::text[]) AS t(nome)
	                  WHERE to_regclass(t.nome) IS NULL`
	var faltam string
	if err := pool.QueryRow(ctx, qFaltam, nomesDasTabelasDeValorLegal()).Scan(&faltam); err != nil {
		return fmt.Errorf("localizar as tabelas de valor legal: %w", err)
	}
	if faltam != "" {
		return fmt.Errorf("tabelas de valor legal ausentes (%s): aplique as migrations com a "+
			"credencial de migração antes de arrancar (ADR-043)", faltam)
	}

	// pg_has_role e não comparação de nomes: um papel pode não ser o dono e ainda
	// assim assumi-lo por pertença (SET ROLE), o que dá exactamente o mesmo poder.
	// Verificado: com GRANT sgc TO sgc_app, o DISABLE TRIGGER passa a funcionar.
	//
	// 'MEMBER' e não 'USAGE' (correcção da revisão da Tarefa 3, mesma medição
	// que em recusarAdministrador): com
	//
	//   GRANT sgc TO sgc_app WITH INHERIT FALSE
	//
	// (sgc é o dono real das tabelas de valor legal neste ambiente),
	// pg_has_role(current_user, 'sgc', 'USAGE') devolveu false — a versão
	// anterior desta consulta não apanhava o caso — mas 'MEMBER' devolveu true,
	// e SET ROLE sgc seguido de DISABLE TRIGGER desligou o mesmo trigger.
	const q = `SELECT coalesce(bool_or(pg_has_role(current_user, c.relowner, 'MEMBER')), false)
	             FROM unnest($1::text[]) AS t(nome)
	             JOIN pg_class c ON c.oid = to_regclass(t.nome)`
	var dono bool
	if err := pool.QueryRow(ctx, q, nomesDasTabelasDeValorLegal()).Scan(&dono); err != nil {
		return fmt.Errorf("verificar a posse das tabelas de valor legal: %w", err)
	}
	if dono {
		return fmt.Errorf("o papel de runtime %q é dono das tabelas de valor legal, ou membro "+
			"do papel que as detém: pode correr ALTER TABLE ... DISABLE TRIGGER e anular a "+
			"imutabilidade das facturas e do audit log (ADR-043)", papel)
	}
	return nil
}

func recusarCriacaoDeObjectos(ctx context.Context, pool *pgxpool.Pool, papel string) error {
	// to_regnamespace devolve NULL para um schema que não existe. Nomear o que
	// falta em vez de o filtrar em silêncio (`WHERE ns IS NOT NULL`, como a
	// versão anterior fazia): hoje está coberto porque recusarDono já corre
	// antes e exige as tabelas migradas, mas um 9.º schema acrescentado a
	// schemasBC antes da migração respectiva passaria por aqui sem sinal nenhum
	// (ADR-043, correcção da revisão da Tarefa 3). Mesmo padrão que a
	// verificação de tabelas ausentes em recusarDono.
	const qFaltam = `SELECT coalesce(string_agg(s, ', '), '')
	                    FROM unnest($1::text[]) AS s
	                   WHERE to_regnamespace(s) IS NULL`
	var faltam string
	if err := pool.QueryRow(ctx, qFaltam, schemasBC).Scan(&faltam); err != nil {
		return fmt.Errorf("localizar os schemas de negócio: %w", err)
	}
	if faltam != "" {
		return fmt.Errorf("schemas de negócio ausentes (%s): aplique as migrations com a "+
			"credencial de migração antes de arrancar (ADR-043)", faltam)
	}

	// A pergunta é sobre a UNIÃO dos papéis que current_user pode assumir por
	// SET ROLE, não sobre current_user: has_schema_privilege(current_user, ...)
	// responde "o que este papel tem POR HERANÇA", que é a mesma semântica
	// errada que a correcção de 'USAGE'→'MEMBER' já tinha identificado acima e
	// que esta consulta ignorava (ADR-043, correcção 6 da revisão da Tarefa 3).
	// Exploração reproduzida contra sgc-postgres-1:
	//
	//   CREATE ROLE zz_create_teste NOLOGIN;
	//   GRANT CREATE, USAGE ON SCHEMA financeiro TO zz_create_teste;
	//   GRANT zz_create_teste TO sgc_app WITH INHERIT FALSE;
	//
	// deixava as quatro interrogações todas limpas — o servidor arrancava — e a
	// seguir, como sgc_app, `SET ROLE zz_create_teste; CREATE TABLE
	// financeiro.zz_ddl_teste(x int);` executou: DDL fora das migrations
	// forward-only.
	//
	// pg_has_role(current_user, r.oid, 'MEMBER') é verdadeiro para o próprio
	// current_user, pelo que esta consulta CONTÉM a anterior — medido, não
	// deduzido: com a base no estado de provisionamento a consulta antiga e a
	// nova devolvem ambas vazio, e um GRANT CREATE directo a sgc_app é apanhado
	// pelas duas.
	//
	// Nomear o papel pela via do qual o poder chega ("financeiro (via
	// zz_create_teste)") e não só o schema: quem lê o erro em produção precisa
	// de saber o que revogar.
	const q = `SELECT coalesce(string_agg(x.nome || ' (via ' || r.rolname || ')', ', '
	                                      ORDER BY x.nome, r.rolname), '')
	             FROM (SELECT s AS nome, to_regnamespace(s) AS ns
	                     FROM unnest($1::text[]) AS s) x
	             JOIN pg_roles r ON pg_has_role(current_user, r.oid, 'MEMBER')
	            WHERE has_schema_privilege(r.oid, x.ns::oid, 'CREATE')`
	var comCreate string
	if err := pool.QueryRow(ctx, q, schemasBC).Scan(&comCreate); err != nil {
		return fmt.Errorf("verificar o privilégio CREATE nos schemas: %w", err)
	}
	if comCreate != "" {
		return fmt.Errorf("o papel de runtime %q tem — por si ou por um papel que pode assumir "+
			"com SET ROLE — CREATE nos schemas %s: pode criar objectos fora das migrations "+
			"forward-only (ADR-043)", papel, comCreate)
	}

	// CREATE na BASE DE DADOS é uma via distinta e igualmente fatal para a
	// restrição forward-only: não cria objectos nos schemas conhecidos, cria
	// SCHEMAS NOVOS — e objectos lá dentro, à vontade. Medido contra
	// sgc-postgres-1: `GRANT CREATE ON DATABASE sgc TO sgc_app` deixava as
	// quatro interrogações limpas e `CREATE SCHEMA zz_novo_schema; CREATE TABLE
	// zz_novo_schema.t(x int);` executou (ADR-043, correcção 7 da revisão da
	// Tarefa 3). Mesma união de papéis assumíveis por SET ROLE, pela mesma razão.
	const qBase = `SELECT current_database(),
	                      coalesce(string_agg(r.rolname, ', ' ORDER BY r.rolname), '')
	                 FROM pg_roles r
	                WHERE pg_has_role(current_user, r.oid, 'MEMBER')
	                  AND has_database_privilege(r.oid, current_database(), 'CREATE')`
	var base, viasBase string
	if err := pool.QueryRow(ctx, qBase).Scan(&base, &viasBase); err != nil {
		return fmt.Errorf("verificar o privilégio CREATE na base de dados: %w", err)
	}
	if viasBase != "" {
		return fmt.Errorf("o papel de runtime %q tem — por si ou por um papel que pode assumir "+
			"com SET ROLE — CREATE na base de dados %q (via %s): pode criar schemas novos e "+
			"objectos lá dentro, fora das migrations forward-only (ADR-043)",
			papel, base, viasBase)
	}
	return nil
}

// recusarMutacaoDoValorLegal percorre as QUATRO tabelas de valor legal, cada uma
// com o seu conjunto proibido — não uma tabela fixa no texto da consulta, que é
// o que deixava `financeiro.facturas` e `financeiro.itens_factura` sem qualquer
// protecção contra TRUNCATE enquanto a função dizia proteger o valor legal
// (ADR-043, correcção 7 da revisão da Tarefa 3). Chamava-se
// recusarMutacaoDaAuditoria: o nome dizia a verdade sobre o que fazia e mentia
// sobre o que prometia.
func recusarMutacaoDoValorLegal(ctx context.Context, pool *pgxpool.Pool, papel string) error {
	// UPDATE, DELETE e TRUNCATE no audit log; só TRUNCATE nas duas tabelas de
	// facturas — a factura em RASCUNHO é mutável (ADR-039) e sgc_app tem hoje
	// UPDATE/DELETE lá, que é trabalho legítimo; DELETE e TRUNCATE em
	// financeiro.series, onde SELECT/INSERT/UPDATE são a própria emissão. Ver o
	// comentário de tabelasDeValorLegal para o porquê de os conjuntos diferirem.
	//
	// Como em recusarCriacaoDeObjectos, a pergunta é sobre a UNIÃO dos papéis
	// assumíveis por SET ROLE e não sobre current_user (ADR-043, correcção 6 da
	// revisão da Tarefa 3). has_table_privilege(current_user, ...) responde "o
	// que este papel tem POR HERANÇA"; uma pertença NOINHERIT não herda nada e
	// mesmo assim faz SET ROLE. Exploração reproduzida contra sgc-postgres-1:
	//
	//   CREATE ROLE zz_trunc_teste NOLOGIN;
	//   GRANT TRUNCATE ON auditoria.auditoria_eventos TO zz_trunc_teste;
	//   GRANT zz_trunc_teste TO sgc_app WITH INHERIT FALSE;
	//
	// deixava tudo limpo — o servidor arrancava — e a seguir, como sgc_app,
	// `SET ROLE zz_trunc_teste; TRUNCATE auditoria.auditoria_eventos;` executou:
	// apagamento integral do audit log, com retenção obrigatória de 10 anos,
	// contornando o trigger (que é FOR EACH ROW e não vê TRUNCATE).
	//
	// Esta via genérica cobre também os papéis predefinidos que a lista fixa de
	// recusarAdministrador não enumera: dos 14 do PG16 nenhum tem rolsuper,
	// rolcreaterole ou rolcreatedb (medido: todos `f`), pelo que
	// `GRANT pg_write_all_data TO sgc_app WITH INHERIT FALSE` era invisível —
	// e passa a ser apanhado aqui, porque pg_write_all_data tem UPDATE e DELETE
	// em auditoria_eventos. Preferir a via genérica a alargar listas fixas: a
	// lista fixa fica sempre atrás da próxima versão do PostgreSQL.
	//
	// Os pares (tabela, privilégio) chegam achatados em dois arrays paralelos,
	// derivados de tabelasDeValorLegal. Uma tabela declarada sem conjunto
	// proibido é erro e não silêncio: falhar fechado é o que impede a próxima
	// tabela de valor legal de entrar na lista e ficar por verificar — que foi,
	// exactamente, este defeito.
	tabelas, privilegios := make([]string, 0), make([]string, 0)
	for _, tabela := range tabelasDeValorLegal {
		if len(tabela.Proibidos) == 0 {
			return fmt.Errorf("a tabela de valor legal %q está declarada sem privilégios "+
				"proibidos: a verificação de arranque recusa-se a passar por cima dela "+
				"(ADR-043)", tabela.Nome)
		}
		for _, privilegio := range tabela.Proibidos {
			tabelas = append(tabelas, tabela.Nome)
			privilegios = append(privilegios, privilegio)
		}
	}

	// O JOIN é por to_regclass e não pelo nome directamente em
	// has_table_privilege(oid, text, text), que REBENTA se a tabela não
	// existir. Antes isto estava só documentado como "aceitável porque
	// recusarDono já correu"; passando por to_regclass, que devolve NULL em vez
	// de erro, a ordem das verificações deixa de importar — e uma dependência
	// de ordem que não existe é melhor do que uma documentada (ADR-043, A2 da
	// revisão da Tarefa 3). Uma tabela ausente é caso de recusarDono, que tem
	// mensagem própria para ela; aqui apenas não produz linha.
	const q = `SELECT coalesce(string_agg(t.tabela || ' ' || t.priv || ' (via ' || r.rolname || ')',
	                                      ', ' ORDER BY t.tabela, t.priv, r.rolname), ''),
	                  coalesce(bool_or(t.tabela = 'auditoria.auditoria_eventos'), false),
	                  coalesce(bool_or(t.tabela IN ('financeiro.facturas',
	                                                'financeiro.itens_factura')), false),
	                  coalesce(bool_or(t.tabela = 'financeiro.series'), false)
	             FROM unnest($1::text[], $2::text[]) AS t(tabela, priv)
	             JOIN pg_class c ON c.oid = to_regclass(t.tabela)
	             JOIN pg_roles r ON pg_has_role(current_user, r.oid, 'MEMBER')
	            WHERE has_table_privilege(r.oid, c.oid, t.priv)`
	var vias string
	var tocaAuditoria, tocaFacturas, tocaSerie bool
	if err := pool.QueryRow(ctx, q, tabelas, privilegios).Scan(
		&vias, &tocaAuditoria, &tocaFacturas, &tocaSerie); err != nil {
		return fmt.Errorf("verificar os privilégios sobre as tabelas de valor legal: %w", err)
	}
	if vias != "" {
		return fmt.Errorf("o papel de runtime %q tem — por si ou por um papel que pode assumir "+
			"com SET ROLE — privilégios que destroem valor legal por uma via que os triggers de "+
			"linha não vêem: %s.%s (ADR-040/041, ADR-043)", papel, vias,
			consequenciaDaViolacao(tocaAuditoria, tocaFacturas, tocaSerie))
	}
	return nil
}

// consequenciaDaViolacao devolve a cauda da mensagem de erro conforme o que foi
// DE FACTO violado. A cauda era fixa e falava sempre do audit log append-only:
// com financeiro.series no conjunto, uma violação da cadeia de hash vinha
// acompanhada de uma frase sobre retenção de 10 anos que não lhe dizia respeito
// (ADR-043, A4 da revisão da Tarefa 3).
//
// A distinção é entre as TRÊS consequências e não entre dois schemas: agrupar
// as facturas com a série imprimia "apagar a linha da série perde o
// ultimo_hash" numa violação só de financeiro.facturas, sobre uma tabela que
// não fora violada — o mesmo defeito uma camada abaixo (ADR-043, N5 da
// re-revisão). A violação em si já vem nomeada antes; isto só evita que o ruído
// a contradiga.
func consequenciaDaViolacao(auditoria, facturas, serie bool) string {
	var partes []string
	if auditoria {
		partes = append(partes,
			"o audit log é append-only com retenção de 10 anos (LPDP / Lei 22/11)")
	}
	if facturas {
		partes = append(partes,
			"a cadeia de hash das facturas não sobrevive a um TRUNCATE")
	}
	if serie {
		partes = append(partes,
			"apagar a linha da série perde o ultimo_hash e o elo seguinte nasce partido, "+
				"sem reparação possível")
	}
	switch len(partes) {
	case 0:
		return ""
	case 1:
		return " Nota: " + partes[0] + "."
	default:
		return " Nota: " + strings.Join(partes[:len(partes)-1], "; ") + "; e " +
			partes[len(partes)-1] + "."
	}
}
