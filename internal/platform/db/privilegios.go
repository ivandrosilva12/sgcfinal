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
	const q = `SELECT rolsuper OR rolcreaterole OR rolcreatedb
	             FROM pg_roles WHERE rolname = current_user`
	var admin bool
	if err := pool.QueryRow(ctx, q).Scan(&admin); err != nil {
		return fmt.Errorf("verificar os atributos do papel %q: %w", papel, err)
	}
	if admin {
		return fmt.Errorf("o papel de runtime %q é administrador (SUPERUSER, CREATEROLE ou "+
			"CREATEDB): pode desligar triggers e apagar o audit log. Use a credencial de "+
			"runtime em DATABASE_URL, não a de migração (ADR-043)", papel)
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
	const q = `SELECT coalesce(bool_or(pg_has_role(current_user, c.relowner, 'USAGE')), false)
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
	const q = `SELECT coalesce(string_agg(nome, ', '), '')
	             FROM (SELECT s AS nome, to_regnamespace(s) AS ns
	                     FROM unnest($1::text[]) AS s) x
	            WHERE ns IS NOT NULL
	              AND has_schema_privilege(current_user, ns::oid, 'CREATE')`
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
	const q = `SELECT has_table_privilege(current_user, 'auditoria.auditoria_eventos', 'UPDATE')
	              OR has_table_privilege(current_user, 'auditoria.auditoria_eventos', 'DELETE')`
	var muta bool
	if err := pool.QueryRow(ctx, q).Scan(&muta); err != nil {
		return fmt.Errorf("verificar os privilégios sobre o audit log: %w", err)
	}
	if muta {
		return fmt.Errorf("o papel de runtime %q tem UPDATE ou DELETE em "+
			"auditoria.auditoria_eventos: o audit log é append-only e a retenção de 10 anos "+
			"depende disso (LPDP / Lei 22/11, ADR-043)", papel)
	}
	return nil
}
