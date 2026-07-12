package identidade_test

import (
	"context"
	"errors"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeCriador estende o comportamento de criação sobre um fakeAdmin mínimo.
type fakeCriador struct {
	recebido appident.DadosNovoUtilizador
	id       string
	err      error
}

func (f *fakeCriador) ListarUtilizadores(context.Context, appident.FiltroUtilizadores) ([]appident.ResumoUtilizador, error) {
	return nil, nil
}
func (f *fakeCriador) ObterUtilizador(context.Context, string) (appident.DetalheUtilizador, error) {
	return appident.DetalheUtilizador{}, nil
}
func (f *fakeCriador) AtribuirPapel(context.Context, string, dominio.Papel) error { return nil }
func (f *fakeCriador) RevogarPapel(context.Context, string, dominio.Papel) error  { return nil }
func (f *fakeCriador) DefinirActivo(context.Context, string, bool) error          { return nil }
func (f *fakeCriador) CriarUtilizador(_ context.Context, d appident.DadosNovoUtilizador) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.recebido = d
	return f.id, nil
}
func (f *fakeCriador) DefinirPasswordTemporaria(context.Context, string, string) error { return nil }
func (f *fakeCriador) ResetOTP(context.Context, string) error                          { return nil }
func (f *fakeCriador) RevogarSessoes(context.Context, string) error                    { return nil }
func (f *fakeCriador) ApagarUtilizador(context.Context, string) error                  { return nil }
func (f *fakeCriador) ListarSessoes(context.Context, string) ([]appident.SessaoActiva, error) {
	return nil, nil
}
func (f *fakeCriador) RevogarSessao(context.Context, string) error { return nil }

func TestCriarUtilizador_PapelComum(t *testing.T) {
	admin := &fakeCriador{id: "novo-id"}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoCriarUtilizador(admin, aud, &fakeNotificador{})

	out, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "ana.silva", Nome: "Ana Silva", Email: "ana@sgc.ao",
		Papeis: []dominio.Papel{dominio.PapelMedico},
	})
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if out.ID != "novo-id" || out.SenhaTemporaria == "" {
		t.Fatalf("saída inesperada: %+v", out)
	}
	if admin.recebido.ConfigurarOTP {
		t.Fatal("papel comum não deve exigir OTP")
	}
	if admin.recebido.SenhaTemporaria != out.SenhaTemporaria {
		t.Fatal("a senha passada ao adaptador deve ser a devolvida")
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.utilizador.criado" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
}

func TestCriarUtilizador_PapelSensivel_ExigeOTP(t *testing.T) {
	admin := &fakeCriador{id: "novo-id"}
	caso := appident.NovoCasoCriarUtilizador(admin, &fakeAuditor{}, &fakeNotificador{})
	if _, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "chefe", Nome: "Chefe Geral", Email: "chefe@sgc.ao",
		Papeis: []dominio.Papel{dominio.PapelAdmin},
	}); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if !admin.recebido.ConfigurarOTP {
		t.Fatal("papel sensível deve exigir CONFIGURE_TOTP")
	}
}

func TestCriarUtilizador_EmailInvalido(t *testing.T) {
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{}, &fakeAuditor{}, &fakeNotificador{})
	_, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "x", Nome: "X", Email: "não-é-email", Papeis: nil,
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestCriarUtilizador_PapelInvalido(t *testing.T) {
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{}, &fakeAuditor{}, &fakeNotificador{})
	_, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "x", Nome: "X", Email: "x@sgc.ao", Papeis: []dominio.Papel{"Inexistente"},
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação por papel inválido, obtive %v", err)
	}
}

func TestCriarUtilizador_UsernameVazio(t *testing.T) {
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{}, &fakeAuditor{}, &fakeNotificador{})
	_, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "", Nome: "X", Email: "x@sgc.ao",
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação por username vazio, obtive %v", err)
	}
}

func TestCriarUtilizador_PropagaConflito(t *testing.T) {
	conflito := erros.Novo(erros.CategoriaConflito, "já existe")
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{err: conflito}, &fakeAuditor{}, &fakeNotificador{})
	_, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "dup", Nome: "Dup", Email: "dup@sgc.ao",
	})
	if !errors.Is(err, conflito) {
		t.Fatalf("esperava propagação do conflito, obtive %v", err)
	}
}

func TestCriarUtilizador_NotificaPorEmail(t *testing.T) {
	notif := &fakeNotificador{}
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{id: "novo-id"}, &fakeAuditor{}, notif)
	if _, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "ana.silva", Nome: "Ana Silva", Email: "ana@sgc.ao",
		Papeis: []dominio.Papel{dominio.PapelMedico},
	}); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if notif.criacoes != 1 {
		t.Fatalf("esperava 1 notificação de criação, obtive %d", notif.criacoes)
	}
}

func TestCriarUtilizador_FalhaEmailNaoFalhaCriacao(t *testing.T) {
	notif := &fakeNotificador{err: errors.New("smtp em baixo")}
	caso := appident.NovoCasoCriarUtilizador(&fakeCriador{id: "novo-id"}, &fakeAuditor{}, notif)
	out, err := caso.Executar(context.Background(), "actor-1", appident.CriacaoUtilizador{
		Username: "ana.silva", Nome: "Ana Silva", Email: "ana@sgc.ao",
		Papeis: []dominio.Papel{dominio.PapelMedico},
	})
	if err != nil {
		t.Fatalf("falha de email não deve falhar a criação, obtive %v", err)
	}
	if out.ID != "novo-id" || out.SenhaTemporaria == "" {
		t.Fatalf("criação devia ter tido sucesso: %+v", out)
	}
}
