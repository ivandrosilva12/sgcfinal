package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schemasBC são os oito schemas por bounded context. O papel de runtime tem
// USAGE em todos e CREATE em nenhum. Atenção: `recepcao` não é criado pelo
// init.sql — nasce na sua própria migração.
var schemasBC = []string{
	"auditoria", "clinico", "farmacia", "financeiro",
	"identidade", "laboratorio", "recepcao", "shared",
}

// tabelasDeValorLegal são as tabelas cuja posse daria ao runtime o poder de
// desligar os triggers que as protegem (ADR-040, ADR-042 §2.6).
var tabelasDeValorLegal = []string{
	"financeiro.facturas",
	"financeiro.itens_factura",
	"auditoria.auditoria_eventos",
}

// VerificarPapelRuntime confirma que o papel com que a aplicação está ligada não
// consegue subverter as garantias que a base de dados impõe por trigger: não é
// administrador, não é — nem pode assumir — o dono das tabelas de valor legal,
// não cria objectos nos schemas de negócio e não muta o audit log.
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
	return recusarMutacaoDaAuditoria(ctx, pool, papel)
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
	if err := pool.QueryRow(ctx, qFaltam, tabelasDeValorLegal).Scan(&faltam); err != nil {
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
	// (sgc é o dono real das três tabelas de valor legal neste ambiente),
	// pg_has_role(current_user, 'sgc', 'USAGE') devolveu false — a versão
	// anterior desta consulta não apanhava o caso — mas 'MEMBER' devolveu true,
	// e SET ROLE sgc seguido de DISABLE TRIGGER desligou o mesmo trigger.
	const q = `SELECT coalesce(bool_or(pg_has_role(current_user, c.relowner, 'MEMBER')), false)
	             FROM unnest($1::text[]) AS t(nome)
	             JOIN pg_class c ON c.oid = to_regclass(t.nome)`
	var dono bool
	if err := pool.QueryRow(ctx, q, tabelasDeValorLegal).Scan(&dono); err != nil {
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

	const q = `SELECT coalesce(string_agg(nome, ', '), '')
	             FROM (SELECT s AS nome, to_regnamespace(s) AS ns
	                     FROM unnest($1::text[]) AS s) x
	            WHERE has_schema_privilege(current_user, ns::oid, 'CREATE')`
	var comCreate string
	if err := pool.QueryRow(ctx, q, schemasBC).Scan(&comCreate); err != nil {
		return fmt.Errorf("verificar o privilégio CREATE nos schemas: %w", err)
	}
	if comCreate != "" {
		return fmt.Errorf("o papel de runtime %q tem CREATE nos schemas %s: pode criar objectos "+
			"fora das migrations forward-only (ADR-043)", papel, comCreate)
	}
	return nil
}

func recusarMutacaoDaAuditoria(ctx context.Context, pool *pgxpool.Pool, papel string) error {
	// UPDATE, DELETE e TRUNCATE: o único trigger em auditoria_eventos
	// (trg_auditoria_imutavel) é `BEFORE DELETE OR UPDATE ... FOR EACH ROW` —
	// não existe trigger de TRUNCATE em nenhuma das três tabelas de valor
	// legal, porque TRUNCATE não dispara triggers de linha. Hoje o privilégio
	// está ausente (não há GRANT TRUNCATE para sgc_app), logo não há buraco
	// vivo — mas faltava aqui verificar o que a garantia de append-only
	// promete: sem esta linha, um GRANT TRUNCATE futuro apagaria o audit log
	// inteiro contornando o trigger, e esta função continuaria a devolver nil
	// (ADR-043, correcção da revisão da Tarefa 3).
	const q = `SELECT has_table_privilege(current_user, 'auditoria.auditoria_eventos', 'UPDATE')
	              OR has_table_privilege(current_user, 'auditoria.auditoria_eventos', 'DELETE')
	              OR has_table_privilege(current_user, 'auditoria.auditoria_eventos', 'TRUNCATE')`
	var muta bool
	if err := pool.QueryRow(ctx, q).Scan(&muta); err != nil {
		return fmt.Errorf("verificar os privilégios sobre o audit log: %w", err)
	}
	if muta {
		return fmt.Errorf("o papel de runtime %q tem UPDATE, DELETE ou TRUNCATE em "+
			"auditoria.auditoria_eventos: o audit log é append-only e a retenção de 10 anos "+
			"depende disso (LPDP / Lei 22/11, ADR-043)", papel)
	}
	return nil
}
