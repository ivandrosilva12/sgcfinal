//go:build integration

// Provas da separação de credenciais (ADR-043 / R7 da ADR-040). Correm ligadas
// como sgc_app — o papel de runtime — e verificam que ele NÃO consegue subverter
// as garantias que a base de dados impõe por trigger, e que CONTINUA a conseguir
// fazer o trabalho legítimo da aplicação.
package integration_test

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

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
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := pool.Exec(ctx, c.sql); err == nil {
				t.Fatalf("o papel de runtime conseguiu %q — o R7 continua aberto", c.nome)
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
	// prova que passa a verde pela razão errada não prova nada.
	if !strings.Contains(err.Error(), "RASCUNHO") {
		t.Fatalf("esperava a rejeição do trigger de nascer rascunho, obtive: %v", err)
	}
}
