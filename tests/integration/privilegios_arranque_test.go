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
