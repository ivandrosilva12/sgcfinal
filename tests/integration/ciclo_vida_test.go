//go:build integration

package integration_test

import (
	"context"
	"testing"

	"log/slog"
	"os"

	"github.com/google/uuid"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/keycloak"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// TestResetPasswordEOTP_ViaKeycloak cria um utilizador, repõe a password e o OTP,
// e limpa. Exercita DefinirPasswordTemporaria/ResetOTP contra o Keycloak real.
func TestResetPasswordEOTP_ViaKeycloak(t *testing.T) {
	issuer := issuerTeste()
	admin, err := keycloak.NovoAdmin(issuer, "sgc-admin", "segredo-admin")
	if err != nil {
		t.Fatalf("NovoAdmin: %v", err)
	}
	ctx := context.Background()

	id, err := admin.CriarUtilizador(ctx, appident.DadosNovoUtilizador{
		Username: "reset.teste.sprint5", Nome: "Reset Teste", Email: "reset.teste.sprint5@sgc.ao",
		SenhaTemporaria: "Temp-1234", Papeis: []dominio.Papel{dominio.PapelMedico}, ConfigurarOTP: false,
	})
	if err != nil {
		t.Skipf("Admin API indisponível ou já existe: %v", err)
	}
	defer apagarUtilizador(t, issuer, id)

	if err := admin.DefinirPasswordTemporaria(ctx, id, "Nova-Senha-9"); err != nil {
		t.Fatalf("DefinirPasswordTemporaria: %v", err)
	}
	if err := admin.ResetOTP(ctx, id); err != nil {
		t.Fatalf("ResetOTP: %v", err)
	}
	if err := admin.RevogarSessoes(ctx, id); err != nil {
		t.Fatalf("RevogarSessoes: %v", err)
	}
}

// TestAtualizarPerfil_ViaBD exercita o CasoAtualizarPerfil contra a BD real:
// garante a linha (JIT) e persiste telefone/BI.
func TestAtualizarPerfil_ViaBD(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repo := pgrepo.NovoRepositorioUtilizadores(pool)
	repoAud := pgrepo.NovoRepositorioAuditoria(pool)
	caso := appident.NovoCasoAtualizarPerfil(repo, repoAud)

	// keycloak_id é uuid na BD real (migrations/identidade/0001_utilizadores.sql);
	// o exemplo do brief usava um sujeito não-UUID, o que falha contra o esquema real.
	sessao := dominio.Sessao{Sujeito: uuid.NewString(), Nome: "Perfil Teste", Email: "perfil.teste@sgc.ao", Papeis: []dominio.Papel{dominio.PapelMedico}}
	tel := "+244 923 456 789"
	perfil, err := caso.Executar(ctx, sessao, &tel, nil)
	if err != nil {
		t.Fatalf("actualizar perfil: %v", err)
	}
	if perfil.Telefone != "+244 923 456 789" {
		t.Fatalf("telefone não persistido: %q", perfil.Telefone)
	}

	// Limpeza da linha local criada.
	_, _ = pool.Exec(ctx, `DELETE FROM identidade.utilizadores WHERE keycloak_id = $1`, sessao.Sujeito)
}
