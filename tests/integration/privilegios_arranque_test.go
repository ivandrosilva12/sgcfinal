//go:build integration

// Prova da verificação de arranque da ADR-043: o servidor recusa arrancar com um
// papel privilegiado. Sem esta verificação, a separação de credenciais seria uma
// suposição sobre o deployment em vez de uma invariante do arranque — e é no
// deployment que o R7 vive.
package integration_test

import (
	"strings"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
)

func TestVerificarPapelRuntime_AceitaOPapelDeRuntime(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligarApp(t)

	if err := db.VerificarPapelRuntime(ctx, pool); err != nil {
		t.Fatalf("sgc_app tem de ser aceite como papel de runtime: %v", err)
	}
}

func TestVerificarPapelRuntime_RecusaOMigrador(t *testing.T) {
	migrarTudo(t)
	pool, ctx := ligar(t) // credencial de migração: superuser e dona das tabelas

	err := db.VerificarPapelRuntime(ctx, pool)
	if err == nil {
		t.Fatal("a credencial de migração tinha de ser recusada como papel de runtime")
	}
	if !strings.Contains(err.Error(), "ADR-043") {
		t.Fatalf("a mensagem tem de encaminhar para a ADR-043; obtive: %v", err)
	}
}

// TestVerificarPapelRuntime_ApanhaODesvioPorPertenca cobre o caso que uma
// comparação de nomes não apanharia: sgc_app não é dono das tabelas, mas se for
// membro de sgc pode assumi-lo com SET ROLE e desligar os triggers na mesma.
// Verificado contra postgres:16 — tgenabled passa de 'O' para 'D'.
func TestVerificarPapelRuntime_ApanhaODesvioPorPertenca(t *testing.T) {
	migrarTudo(t)
	admin, ctxAdmin := ligar(t)

	if _, err := admin.Exec(ctxAdmin, `GRANT sgc TO sgc_app`); err != nil {
		t.Fatalf("preparar o desvio: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctxAdmin, `REVOKE sgc FROM sgc_app`); err != nil {
			t.Fatalf("repor a pertença: %v — a base FICOU com uma pertença privilegiada "+
				"residual (sgc_app membro de sgc); repor manualmente com "+
				"`REVOKE sgc FROM sgc_app;` antes de correr qualquer outro teste", err)
		}
	})

	pool, ctx := ligarApp(t)
	if err := db.VerificarPapelRuntime(ctx, pool); err == nil {
		t.Fatal("um runtime que pode assumir o papel do dono tinha de ser recusado")
	}
}

// TestVerificarPapelRuntime_ApanhaODesvioPorPertencaNoInherit cobre o caso que
// TestVerificarPapelRuntime_ApanhaODesvioPorPertenca (acima, com GRANT normal —
// INHERIT por omissão) não distingue: 'USAGE' diz se os privilégios se herdam
// automaticamente, 'MEMBER' diz se current_user pode fazer SET ROLE. O vector
// de ataque é o SET ROLE, não a herança automática. Medido contra
// sgc-postgres-1: com `GRANT sgc TO sgc_app WITH INHERIT FALSE`,
// pg_has_role(current_user, 'sgc', 'USAGE') devolve false — uma verificação
// baseada em 'USAGE' deixaria passar este caso — mas 'MEMBER' devolve true, e
// SET ROLE sgc seguido de DISABLE TRIGGER desligou o trigger na mesma
// (tgenabled 'O' → 'D'). Trava a regressão para 'USAGE' em recusarDono.
func TestVerificarPapelRuntime_ApanhaODesvioPorPertencaNoInherit(t *testing.T) {
	migrarTudo(t)
	admin, ctxAdmin := ligar(t)

	if _, err := admin.Exec(ctxAdmin, `GRANT sgc TO sgc_app WITH INHERIT FALSE`); err != nil {
		t.Fatalf("preparar o desvio: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctxAdmin, `REVOKE sgc FROM sgc_app`); err != nil {
			t.Fatalf("repor a pertença: %v — a base FICOU com uma pertença privilegiada "+
				"residual (sgc_app membro NOINHERIT de sgc); repor manualmente com "+
				"`REVOKE sgc FROM sgc_app;` antes de correr qualquer outro teste", err)
		}
	})

	pool, ctx := ligarApp(t)
	if err := db.VerificarPapelRuntime(ctx, pool); err == nil {
		t.Fatal("um runtime que pode assumir (SET ROLE) o dono por pertença NOINHERIT " +
			"tinha de ser recusado; uma verificação baseada em pg_has_role(..., 'USAGE') " +
			"não apanharia este caso")
	}
}

// TestVerificarPapelRuntime_ApanhaPertencaAPapelAdministrativo é a prova
// directa da correcção 1 da revisão da Tarefa 3: recusarAdministrador lia os
// atributos do PRÓPRIO papel (pg_roles.rolsuper/rolcreaterole/rolcreatedb de
// current_user), o que não apanha o poder ganho por pertença. Reproduzido
// contra sgc-postgres-1: com
//
//	CREATE ROLE zz_super_teste SUPERUSER NOLOGIN; GRANT zz_super_teste TO sgc_app;
//
// os atributos do próprio sgc_app continuavam todos falsos — a versão anterior
// de VerificarPapelRuntime devolvia nil — mas SET ROLE zz_super_teste seguido
// de DISABLE TRIGGER desligou trg_auditoria_imutavel (tgenabled 'O' → 'D'). O
// runtime não precisa de possuir a tabela nem os atributos: basta poder
// assumir, por SET ROLE, um papel que os tenha.
func TestVerificarPapelRuntime_ApanhaPertencaAPapelAdministrativo(t *testing.T) {
	migrarTudo(t)
	admin, ctxAdmin := ligar(t)

	if _, err := admin.Exec(ctxAdmin, `CREATE ROLE zz_super_teste SUPERUSER NOLOGIN`); err != nil {
		t.Fatalf("preparar o papel administrativo de teste: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctxAdmin, `DROP ROLE zz_super_teste`); err != nil {
			t.Fatalf("apagar o papel administrativo de teste: %v — a base FICOU com o papel "+
				"privilegiado residual `zz_super_teste` (e possivelmente ainda membro de "+
				"sgc_app); repor manualmente com `REVOKE zz_super_teste FROM sgc_app; "+
				"DROP ROLE zz_super_teste;` antes de correr qualquer outro teste", err)
		}
	})
	if _, err := admin.Exec(ctxAdmin, `GRANT zz_super_teste TO sgc_app`); err != nil {
		t.Fatalf("preparar o desvio: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctxAdmin, `REVOKE zz_super_teste FROM sgc_app`); err != nil {
			t.Fatalf("repor a pertença: %v — a base FICOU com uma pertença administrativa "+
				"residual (sgc_app membro de zz_super_teste); repor manualmente com "+
				"`REVOKE zz_super_teste FROM sgc_app;` antes de correr qualquer outro teste", err)
		}
	})

	pool, ctx := ligarApp(t)
	if err := db.VerificarPapelRuntime(ctx, pool); err == nil {
		t.Fatal("um runtime que pode assumir (SET ROLE) um papel SUPERUSER por pertença " +
			"tinha de ser recusado; uma verificação que só lê os atributos do próprio papel " +
			"não apanha este caso")
	}
}

// papelDescartavelNoInherit cria um papel de teste sem login, corre os comandos
// de preparação dados (tipicamente GRANTs sobre esse papel) e concede-o a
// sgc_app com `WITH INHERIT FALSE` — a forma que NÃO se herda automaticamente
// mas que continua a permitir `SET ROLE`. Regista a limpeza (DROP OWNED BY +
// DROP ROLE) antes de qualquer asserção, para que a base não fique envenenada
// quando o teste falha a meio.
func papelDescartavelNoInherit(t *testing.T, nome string, preparar ...string) {
	t.Helper()
	admin, ctxAdmin := ligar(t)

	if _, err := admin.Exec(ctxAdmin, `CREATE ROLE `+nome+` NOLOGIN`); err != nil {
		t.Fatalf("criar o papel de teste %s: %v", nome, err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctxAdmin, `DROP OWNED BY `+nome); err != nil {
			t.Fatalf("largar os privilégios de %s: %v — a base FICOU com o papel de teste e os "+
				"privilégios dele; repor manualmente com `DROP OWNED BY %s; DROP ROLE %s;` antes "+
				"de correr qualquer outro teste", nome, err, nome, nome)
		}
		if _, err := admin.Exec(ctxAdmin, `DROP ROLE `+nome); err != nil {
			t.Fatalf("apagar o papel de teste %s: %v — a base FICOU com o papel residual (e "+
				"possivelmente ainda concedido a sgc_app); repor manualmente com `REVOKE %s FROM "+
				"sgc_app; DROP ROLE %s;` antes de correr qualquer outro teste", nome, err, nome, nome)
		}
	})

	for _, sql := range preparar {
		if _, err := admin.Exec(ctxAdmin, sql); err != nil {
			t.Fatalf("preparar %q: %v", sql, err)
		}
	}
	if _, err := admin.Exec(ctxAdmin, `GRANT `+nome+` TO sgc_app WITH INHERIT FALSE`); err != nil {
		t.Fatalf("conceder %s a sgc_app: %v", nome, err)
	}
}

// TestVerificarPapelRuntime_ApanhaTruncatePorPertencaNoInherit é a prova da
// correcção 6 (CRITICAL) do lado do audit log: recusarMutacaoDaAuditoria
// perguntava `has_table_privilege(current_user, ...)`, que responde "o que este
// papel tem POR HERANÇA". Uma pertença NOINHERIT não herda — mas faz SET ROLE, e
// aí usa tudo o que o outro papel tem. Reproduzido contra sgc-postgres-1: com
//
//	CREATE ROLE zz_trunc_teste NOLOGIN;
//	GRANT TRUNCATE ON auditoria.auditoria_eventos TO zz_trunc_teste;
//	GRANT zz_trunc_teste TO sgc_app WITH INHERIT FALSE;
//
// as quatro interrogações devolviam tudo limpo e o servidor arrancava; a seguir,
// como sgc_app, `SET ROLE zz_trunc_teste; TRUNCATE auditoria.auditoria_eventos;`
// executou — apagamento integral de um log append-only com retenção obrigatória
// de 10 anos (LPDP / Lei 22/11), contornando o trigger, que não vê TRUNCATE.
func TestVerificarPapelRuntime_ApanhaTruncatePorPertencaNoInherit(t *testing.T) {
	migrarTudo(t)
	papelDescartavelNoInherit(t, "zz_trunc_teste",
		`GRANT USAGE ON SCHEMA auditoria TO zz_trunc_teste`,
		`GRANT TRUNCATE ON auditoria.auditoria_eventos TO zz_trunc_teste`)

	pool, ctx := ligarApp(t)
	err := db.VerificarPapelRuntime(ctx, pool)
	if err == nil {
		t.Fatal("um runtime que pode assumir (SET ROLE) um papel com TRUNCATE em " +
			"auditoria.auditoria_eventos tinha de ser recusado; uma verificação sobre " +
			"has_table_privilege(current_user, ...) só vê o privilégio herdado e não apanha isto")
	}
	if !strings.Contains(err.Error(), "zz_trunc_teste") {
		t.Fatalf("a mensagem tem de nomear o papel pela via do qual o poder chega, senão quem "+
			"lê o erro em produção não sabe o que revogar; obtive: %v", err)
	}
	if !strings.Contains(err.Error(), "auditoria.auditoria_eventos") {
		t.Fatalf("a mensagem tem de continuar a nomear a tabela; obtive: %v", err)
	}
}

// TestVerificarPapelRuntime_ApanhaCreateNoSchemaPorPertencaNoInherit é a mesma
// correcção 6 do lado do DDL: recusarCriacaoDeObjectos perguntava
// `has_schema_privilege(current_user, ...)`. Reproduzido: com CREATE no schema
// financeiro concedido a um papel assumível por SET ROLE, `CREATE TABLE
// financeiro.zz_ddl_teste(x int)` executou como sgc_app — objectos fora das
// migrations forward-only.
func TestVerificarPapelRuntime_ApanhaCreateNoSchemaPorPertencaNoInherit(t *testing.T) {
	migrarTudo(t)
	papelDescartavelNoInherit(t, "zz_create_teste",
		`GRANT USAGE ON SCHEMA financeiro TO zz_create_teste`,
		`GRANT CREATE ON SCHEMA financeiro TO zz_create_teste`)

	pool, ctx := ligarApp(t)
	err := db.VerificarPapelRuntime(ctx, pool)
	if err == nil {
		t.Fatal("um runtime que pode assumir (SET ROLE) um papel com CREATE num schema de " +
			"negócio tinha de ser recusado")
	}
	if !strings.Contains(err.Error(), "zz_create_teste") {
		t.Fatalf("a mensagem tem de nomear o papel pela via do qual o poder chega; obtive: %v", err)
	}
	if !strings.Contains(err.Error(), "financeiro") {
		t.Fatalf("a mensagem tem de continuar a nomear o schema; obtive: %v", err)
	}
}

// TestVerificarPapelRuntime_ApanhaPgWriteAllData mede o agravante registado na
// revisão: dos 14 papéis predefinidos do PG16, nenhum tem rolsuper/rolcreaterole
// /rolcreatedb (medido: todos `f`) e só três estão na lista fixa de
// recusarAdministrador. `pg_write_all_data` dá INSERT/UPDATE/DELETE em todas as
// tabelas — incluindo o audit log — e por pertença NOINHERIT ficava invisível às
// quatro interrogações. Depois da correcção 6 é apanhado pela via genérica (o
// privilégio de UPDATE/DELETE sobre auditoria_eventos passa a ser avaliado sobre
// a união dos papéis assumíveis por SET ROLE), sem precisar de o acrescentar a
// nenhuma lista fixa — que é precisamente a razão para preferir a via genérica.
func TestVerificarPapelRuntime_ApanhaPgWriteAllData(t *testing.T) {
	migrarTudo(t)
	admin, ctxAdmin := ligar(t)

	if _, err := admin.Exec(ctxAdmin,
		`GRANT pg_write_all_data TO sgc_app WITH INHERIT FALSE`); err != nil {
		t.Fatalf("preparar o desvio: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctxAdmin, `REVOKE pg_write_all_data FROM sgc_app`); err != nil {
			t.Fatalf("repor a pertença: %v — a base FICOU com sgc_app membro de "+
				"pg_write_all_data, o que lhe dá escrita em TODAS as tabelas incluindo o audit "+
				"log; repor manualmente com `REVOKE pg_write_all_data FROM sgc_app;` antes de "+
				"correr qualquer outro teste", err)
		}
	})

	pool, ctx := ligarApp(t)
	err := db.VerificarPapelRuntime(ctx, pool)
	if err == nil {
		t.Fatal("um runtime que pode assumir (SET ROLE) pg_write_all_data tinha de ser " +
			"recusado: esse papel dá UPDATE e DELETE em auditoria.auditoria_eventos")
	}
	if !strings.Contains(err.Error(), "pg_write_all_data") {
		t.Fatalf("a mensagem tem de nomear o papel predefinido pela via do qual o poder "+
			"chega; obtive: %v", err)
	}
}

// TestVerificarPapelRuntime_ApanhaTruncateNasFacturas é a prova da correcção 7
// (CRITICAL): a interrogação da mutação fixava 'auditoria.auditoria_eventos' no
// texto da consulta, quando tabelasDeValorLegal declara TRÊS tabelas e os três
// triggers de imutabilidade são FOR EACH ROW (verificado em pg_get_triggerdef),
// pelo que nenhum vê TRUNCATE. Reproduzido com GRANT DIRECTO, sem sequer
// precisar de SET ROLE:
//
//	GRANT TRUNCATE ON financeiro.facturas, financeiro.itens_factura TO sgc_app;
//	-- as quatro interrogações devolviam tudo limpo: o servidor arrancava
//	BEGIN; TRUNCATE financeiro.itens_factura, financeiro.facturas; -- executou
//
// O dano é a destruição integral da cadeia de hash das facturas, da numeração
// sem buracos e da base do SAF-T-AO/AGT (ADR-040/041, CLAUDE.md §5.4), com a
// verificação de arranque a declarar-se satisfeita. sgc_app não tem TRUNCATE
// hoje — isto é uma guarda de deriva, que é a função da verificação.
func TestVerificarPapelRuntime_ApanhaTruncateNasFacturas(t *testing.T) {
	migrarTudo(t)
	concederTemporariamente(t, "TRUNCATE", "financeiro.facturas")

	pool, ctx := ligarApp(t)
	err := db.VerificarPapelRuntime(ctx, pool)
	if err == nil {
		t.Fatal("um runtime com TRUNCATE em financeiro.facturas tinha de ser recusado: " +
			"TRUNCATE não dispara o trigger de imutabilidade (FOR EACH ROW) e apaga a cadeia " +
			"de hash inteira")
	}
	if !strings.Contains(err.Error(), "financeiro.facturas") {
		t.Fatalf("a mensagem tem de nomear a tabela; obtive: %v", err)
	}
}

// TestVerificarPapelRuntime_ApanhaTruncateNosItensDaFactura: a mesma correcção
// para a segunda tabela do Financeiro. Truncar só os itens deixa as facturas de
// pé com o total selado a apontar para linhas que já não existem — o hash
// canónico da ADR-041 deixa de ser reproduzível.
func TestVerificarPapelRuntime_ApanhaTruncateNosItensDaFactura(t *testing.T) {
	migrarTudo(t)
	concederTemporariamente(t, "TRUNCATE", "financeiro.itens_factura")

	pool, ctx := ligarApp(t)
	err := db.VerificarPapelRuntime(ctx, pool)
	if err == nil {
		t.Fatal("um runtime com TRUNCATE em financeiro.itens_factura tinha de ser recusado")
	}
	if !strings.Contains(err.Error(), "financeiro.itens_factura") {
		t.Fatalf("a mensagem tem de nomear a tabela; obtive: %v", err)
	}
}

// TestVerificarPapelRuntime_ApanhaTruncateNasFacturasPorPertencaNoInherit junta
// os dois vectores: a tabela que faltava (correcção 7) e a via que a correcção 6
// fechou (pertença NOINHERIT, poder por SET ROLE).
func TestVerificarPapelRuntime_ApanhaTruncateNasFacturasPorPertencaNoInherit(t *testing.T) {
	migrarTudo(t)
	papelDescartavelNoInherit(t, "zz_trunc_fac_teste",
		`GRANT USAGE ON SCHEMA financeiro TO zz_trunc_fac_teste`,
		`GRANT TRUNCATE ON financeiro.facturas TO zz_trunc_fac_teste`)

	pool, ctx := ligarApp(t)
	err := db.VerificarPapelRuntime(ctx, pool)
	if err == nil {
		t.Fatal("um runtime que pode assumir (SET ROLE) um papel com TRUNCATE em " +
			"financeiro.facturas tinha de ser recusado")
	}
	if !strings.Contains(err.Error(), "zz_trunc_fac_teste") {
		t.Fatalf("a mensagem tem de nomear o papel pela via do qual o poder chega; obtive: %v", err)
	}
	if !strings.Contains(err.Error(), "financeiro.facturas") {
		t.Fatalf("a mensagem tem de nomear a tabela; obtive: %v", err)
	}
}

// TestVerificarPapelRuntime_NaoRecusaEscritaNoRascunho é a não-regressão que
// impede a correcção 7 de ir longe demais: o conjunto de privilégios recusado
// NÃO é o mesmo nas três tabelas. A factura em RASCUNHO é mutável (ADR-039) e
// sgc_app tem hoje UPDATE e DELETE em facturas e itens_factura — recusá-los
// partiria a reescrita de rascunhos. Só TRUNCATE é proibido nas duas tabelas do
// Financeiro; em auditoria_eventos são proibidos os três (append-only).
//
// O teste começa por MEDIR a precondição contra a base viva em vez de a assumir:
// se um dia o provisionamento deixar de conceder UPDATE/DELETE, este teste diz
// que a premissa mudou em vez de passar a verde por vacuidade.
func TestVerificarPapelRuntime_NaoRecusaEscritaNoRascunho(t *testing.T) {
	migrarTudo(t)
	admin, ctxAdmin := ligar(t)

	const qPrecondicao = `SELECT count(*)
	                        FROM information_schema.table_privileges
	                       WHERE grantee = 'sgc_app'
	                         AND table_schema = 'financeiro'
	                         AND table_name IN ('facturas', 'itens_factura')
	                         AND privilege_type IN ('UPDATE', 'DELETE')`
	var concedidos int
	if err := admin.QueryRow(ctxAdmin, qPrecondicao).Scan(&concedidos); err != nil {
		t.Fatalf("medir a precondição: %v", err)
	}
	if concedidos != 4 {
		t.Fatalf("precondição falhada: esperava UPDATE e DELETE de sgc_app nas duas tabelas do "+
			"Financeiro (4 privilégios), encontrei %d — a premissa da factura em RASCUNHO "+
			"mutável (ADR-039) mudou e este teste deixou de provar o que promete", concedidos)
	}

	pool, ctx := ligarApp(t)
	if err := db.VerificarPapelRuntime(ctx, pool); err != nil {
		t.Fatalf("UPDATE e DELETE em facturas/itens_factura são trabalho legítimo sobre o "+
			"RASCUNHO e não podem impedir o arranque: %v", err)
	}
}

// TestVerificarPapelRuntime_ApanhaCreateNaBaseDeDados fecha a correcção 7 (N3):
// as quatro interrogações olhavam para CREATE nos schemas conhecidos, mas
// CREATE na BASE DE DADOS permite criar schemas NOVOS — e objectos lá dentro —
// fora das migrations forward-only. Medido: com
// `GRANT CREATE ON DATABASE sgc TO sgc_app`, tudo ficava limpo e
// `CREATE SCHEMA zz_novo_schema; CREATE TABLE zz_novo_schema.t(x int)` executou.
func TestVerificarPapelRuntime_ApanhaCreateNaBaseDeDados(t *testing.T) {
	migrarTudo(t)
	admin, ctxAdmin := ligar(t)

	var base string
	if err := admin.QueryRow(ctxAdmin, `SELECT current_database()`).Scan(&base); err != nil {
		t.Fatalf("determinar a base de dados: %v", err)
	}
	if _, err := admin.Exec(ctxAdmin, `GRANT CREATE ON DATABASE `+base+` TO sgc_app`); err != nil {
		t.Fatalf("preparar o desvio: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctxAdmin,
			`REVOKE CREATE ON DATABASE `+base+` FROM sgc_app`); err != nil {
			t.Fatalf("repor o privilégio: %v — a base FICOU com CREATE ON DATABASE concedido a "+
				"sgc_app, o que lhe permite criar schemas fora das migrations; repor manualmente "+
				"com `REVOKE CREATE ON DATABASE %s FROM sgc_app;` antes de correr qualquer outro "+
				"teste", err, base)
		}
	})

	pool, ctx := ligarApp(t)
	err := db.VerificarPapelRuntime(ctx, pool)
	if err == nil {
		t.Fatal("um runtime com CREATE na base de dados tinha de ser recusado: pode criar " +
			"schemas novos e objectos lá dentro, fora das migrations forward-only")
	}
	if !strings.Contains(err.Error(), base) {
		t.Fatalf("a mensagem tem de nomear a base de dados; obtive: %v", err)
	}
}

// concederTemporariamente concede um privilégio a sgc_app sobre uma tabela e
// regista a revogação antes de qualquer asserção.
func concederTemporariamente(t *testing.T, privilegio, tabela string) {
	t.Helper()
	admin, ctxAdmin := ligar(t)

	if _, err := admin.Exec(ctxAdmin,
		`GRANT `+privilegio+` ON `+tabela+` TO sgc_app`); err != nil {
		t.Fatalf("preparar o desvio (%s em %s): %v", privilegio, tabela, err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctxAdmin,
			`REVOKE `+privilegio+` ON `+tabela+` FROM sgc_app`); err != nil {
			t.Fatalf("repor o privilégio: %v — a base FICOU com %s concedido a sgc_app em %s, "+
				"contornando a imutabilidade; repor manualmente com `REVOKE %s ON %s FROM "+
				"sgc_app;` antes de correr qualquer outro teste",
				err, privilegio, tabela, privilegio, tabela)
		}
	})
}

// TestVerificarPapelRuntime_ApanhaTruncateNaAuditoria trava a regressão da
// correcção 3 da revisão da Tarefa 3: recusarMutacaoDaAuditoria verificava
// UPDATE e DELETE mas não TRUNCATE. O único trigger em auditoria_eventos é
// BEFORE DELETE OR UPDATE ... FOR EACH ROW — TRUNCATE não dispara triggers de
// linha, pelo que um GRANT TRUNCATE apagaria o audit log inteiro contornando o
// trigger. Hoje não há esse GRANT (a suite de trabalho legítimo prova que
// sgc_app não consegue TRUNCATE); este teste concede-o temporariamente para
// confirmar que a verificação o apanha, e repõe o estado a seguir.
func TestVerificarPapelRuntime_ApanhaTruncateNaAuditoria(t *testing.T) {
	migrarTudo(t)
	admin, ctxAdmin := ligar(t)

	if _, err := admin.Exec(ctxAdmin,
		`GRANT TRUNCATE ON auditoria.auditoria_eventos TO sgc_app`); err != nil {
		t.Fatalf("preparar o desvio: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctxAdmin,
			`REVOKE TRUNCATE ON auditoria.auditoria_eventos FROM sgc_app`); err != nil {
			t.Fatalf("repor o privilégio: %v — a base FICOU com TRUNCATE concedido a sgc_app "+
				"em auditoria.auditoria_eventos, contornando a garantia de append-only; repor "+
				"manualmente com `REVOKE TRUNCATE ON auditoria.auditoria_eventos FROM sgc_app;` "+
				"antes de correr qualquer outro teste", err)
		}
	})

	pool, ctx := ligarApp(t)
	if err := db.VerificarPapelRuntime(ctx, pool); err == nil {
		t.Fatal("um runtime com TRUNCATE em auditoria.auditoria_eventos tinha de ser recusado")
	}
}
