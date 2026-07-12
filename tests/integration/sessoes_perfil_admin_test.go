//go:build integration

// Testes de integração dos loose-ends do Sprint 6 (BC Identidade): gestão de
// sessões activas via Admin API, edição administrativa de perfil contra a BD
// real, e notificações por email via MailHog. Seguem o padrão de
// ciclo_vida_test.go: SKIP (nunca FAIL) quando a respectiva infra está em
// baixo.
package integration_test

import (
	"context"
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/keycloak"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/smtp"
	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// TestListarERevogarSessoes_ViaKeycloak cria um utilizador de teste, lista as
// suas sessões activas via Admin API e, se existir alguma, revoga-a e
// reconfirma. Sem login prévio a lista pode vir vazia — nesse caso confirma-se
// apenas que a chamada não erra e devolve um slice. SKIP se a Admin API do
// Keycloak não estiver disponível.
func TestListarERevogarSessoes_ViaKeycloak(t *testing.T) {
	issuer := issuerTeste()
	admin, err := keycloak.NovoAdmin(issuer, "sgc-admin", "segredo-admin")
	if err != nil {
		t.Fatalf("NovoAdmin: %v", err)
	}
	ctx := context.Background()

	const username = "sessoes.teste.sprint6"
	// Remove resíduo de uma corrida anterior abortada, para que uma falha de
	// criação signifique de facto infra indisponível (e não um 409 mascarado).
	limparResiduoUtilizador(ctx, admin, username)

	id, err := admin.CriarUtilizador(ctx, appident.DadosNovoUtilizador{
		Username: username, Nome: "Sessoes Teste", Email: "sessoes.teste.sprint6@sgc.ao",
		SenhaTemporaria: "Temp-1234", Papeis: []dominio.Papel{dominio.PapelMedico}, ConfigurarOTP: false,
	})
	if err != nil {
		t.Skipf("Admin API indisponível: %v", err)
	}
	defer apagarUtilizador(t, issuer, id)

	sessoes, err := admin.ListarSessoes(ctx, id)
	if err != nil {
		t.Fatalf("ListarSessoes: %v", err)
	}
	if sessoes == nil {
		t.Fatal("esperava slice (mesmo que vazio), obtive nil")
	}
	if len(sessoes) == 0 {
		// Sem login prévio não há sessões a revogar — a chamada não errou, é o
		// suficiente para validar o caminho de leitura.
		return
	}

	alvo := sessoes[0]
	if err := admin.RevogarSessao(ctx, alvo.ID); err != nil {
		t.Fatalf("RevogarSessao: %v", err)
	}
	depois, err := admin.ListarSessoes(ctx, id)
	if err != nil {
		t.Fatalf("ListarSessoes (depois de revogar): %v", err)
	}
	for _, s := range depois {
		if s.ID == alvo.ID {
			t.Fatalf("sessão %q ainda presente após revogação", alvo.ID)
		}
	}
}

// limparResiduoUtilizador apaga (best-effort) qualquer utilizador cujo termo de
// pesquisa corresponda ao username indicado, para limpar resíduos de corridas
// anteriores. Silencioso se a Admin API estiver em baixo — nesse caso o próprio
// CriarUtilizador tratará do SKIP.
func limparResiduoUtilizador(ctx context.Context, admin *keycloak.AdminCliente, username string) {
	lista, err := admin.ListarUtilizadores(ctx, appident.FiltroUtilizadores{Termo: username})
	if err != nil {
		return
	}
	for _, u := range lista {
		_ = admin.ApagarUtilizador(ctx, u.ID)
	}
}

// TestEditarPerfilAdmin_ViaBD exercita o CasoEditarPerfilAdmin contra a BD
// real: garante a linha local (JIT via admin fake) e persiste telefone/BI.
// SKIP se DATABASE_URL não estiver definido.
func TestEditarPerfilAdmin_ViaBD(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repo := pgrepo.NovoRepositorioUtilizadores(pool)
	repoAud := pgrepo.NovoRepositorioAuditoria(pool)

	// keycloak_id é uuid na BD real (migrations/identidade/0001_utilizadores.sql).
	const alvo = "00000000-0000-4000-8000-0000000000d1"
	admin := &adminHidratacaoFixa{
		detalhe: appident.DetalheUtilizador{
			ID: alvo, Nome: "Perfil Admin Teste", Email: "perfil.admin.teste@sgc.ao",
			Papeis: []string{"Medico"},
		},
	}
	caso := appident.NovoCasoEditarPerfilAdmin(admin, repo, repoAud)

	tel := "+244 923 456 789"
	bi := "00123456LA042"
	perfil, err := caso.Executar(ctx, "admin.teste.sprint6", alvo, &tel, &bi)
	if err != nil {
		t.Fatalf("editar perfil admin: %v", err)
	}
	if perfil.Telefone != "+244 923 456 789" {
		t.Fatalf("telefone não persistido: %q", perfil.Telefone)
	}
	if perfil.BI != "00123456LA042" {
		t.Fatalf("BI não persistido: %q", perfil.BI)
	}

	// Limpeza da linha local criada.
	_, _ = pool.Exec(ctx, `DELETE FROM identidade.utilizadores WHERE keycloak_id = $1`, alvo)
}

// adminHidratacaoFixa implementa apenas ObterUtilizador (o necessário à
// hidratação JIT do CasoEditarPerfilAdmin); os restantes métodos da porta
// AdminIdentidade não são exercitados por este teste.
type adminHidratacaoFixa struct {
	detalhe appident.DetalheUtilizador
}

func (a *adminHidratacaoFixa) ObterUtilizador(_ context.Context, _ string) (appident.DetalheUtilizador, error) {
	return a.detalhe, nil
}
func (a *adminHidratacaoFixa) ListarUtilizadores(context.Context, appident.FiltroUtilizadores) ([]appident.ResumoUtilizador, error) {
	return nil, nil
}
func (a *adminHidratacaoFixa) AtribuirPapel(context.Context, string, dominio.Papel) error { return nil }
func (a *adminHidratacaoFixa) RevogarPapel(context.Context, string, dominio.Papel) error  { return nil }
func (a *adminHidratacaoFixa) DefinirActivo(context.Context, string, bool) error          { return nil }
func (a *adminHidratacaoFixa) CriarUtilizador(context.Context, appident.DadosNovoUtilizador) (string, error) {
	return "", nil
}
func (a *adminHidratacaoFixa) DefinirPasswordTemporaria(context.Context, string, string) error {
	return nil
}
func (a *adminHidratacaoFixa) ResetOTP(context.Context, string) error         { return nil }
func (a *adminHidratacaoFixa) RevogarSessoes(context.Context, string) error   { return nil }
func (a *adminHidratacaoFixa) ApagarUtilizador(context.Context, string) error { return nil }
func (a *adminHidratacaoFixa) ListarSessoes(context.Context, string) ([]appident.SessaoActiva, error) {
	return nil, nil
}
func (a *adminHidratacaoFixa) RevogarSessao(context.Context, string) error { return nil }

// Garantia de conformidade com a porta.
var _ appident.AdminIdentidade = (*adminHidratacaoFixa)(nil)

// mailhogMensagens é a forma mínima da resposta de GET /api/v2/messages do
// MailHog, suficiente para confirmar o destinatário de uma mensagem.
type mailhogMensagens struct {
	Items []struct {
		To []struct {
			Mailbox string `json:"Mailbox"`
			Domain  string `json:"Domain"`
		} `json:"To"`
	} `json:"items"`
}

// TestNotificacaoCriacao_ViaMailHog envia uma notificação de criação de conta
// via SMTP (apontado ao MailHog local) e confirma, através da API HTTP do
// MailHog, que a mensagem chegou ao destinatário esperado. SKIP se o MailHog
// não responder (dial ou GET falha).
func TestNotificacaoCriacao_ViaMailHog(t *testing.T) {
	notificador := smtp.NovoNotificadorSMTP("localhost", "1025", "nao-responder@sgc.ao")
	ctx := context.Background()

	const destinatario = "mailhog.teste.sprint6@sgc.ao"
	if err := notificador.NotificarCriacao(ctx, destinatario, "Teste", "senha-x"); err != nil {
		t.Skipf("MailHog (SMTP) indisponível: %v", err)
	}
	// Limpa a caixa do MailHog no fim, para não acumular mensagens entre corridas.
	t.Cleanup(func() {
		req, err := nethttp.NewRequest(nethttp.MethodDelete, "http://localhost:8025/api/v1/messages", nil)
		if err != nil {
			return
		}
		if resp, err := (&nethttp.Client{Timeout: 5 * time.Second}).Do(req); err == nil {
			_ = resp.Body.Close()
		}
	})

	cliente := nethttp.Client{Timeout: 5 * time.Second}
	resp, err := cliente.Get("http://localhost:8025/api/v2/messages")
	if err != nil {
		t.Skipf("MailHog (API HTTP) indisponível: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != nethttp.StatusOK {
		t.Skipf("MailHog API devolveu %d", resp.StatusCode)
	}

	var corpo mailhogMensagens
	if err := json.NewDecoder(resp.Body).Decode(&corpo); err != nil {
		t.Fatalf("descodificar resposta do MailHog: %v", err)
	}

	encontrada := false
	for _, item := range corpo.Items {
		for _, to := range item.To {
			if strings.EqualFold(to.Mailbox+"@"+to.Domain, destinatario) {
				encontrada = true
			}
		}
	}
	if !encontrada {
		t.Fatalf("nenhuma mensagem no MailHog para %q", destinatario)
	}
}
