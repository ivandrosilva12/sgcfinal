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
			t.Fatalf("repor a pertença: %v", err)
		}
	})

	pool, ctx := ligarApp(t)
	if err := db.VerificarPapelRuntime(ctx, pool); err == nil {
		t.Fatal("um runtime que pode assumir o papel do dono tinha de ser recusado")
	}
}
