package identidade_test

import (
	"context"
	"errors"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- Fakes ---
// NOTA: reutiliza o `fakeAuditor` já definido em identidade_test.go (mesmo
// pacote de teste `identidade_test`) — NÃO o redefinir aqui (colidiria).

type fakeAdmin struct {
	lista     []appident.ResumoUtilizador
	detalhe   appident.DetalheUtilizador
	err       error
	atribuido []string // "alvo:papel"
	revogado  []string
	activo    map[string]bool

	passwordDefinida map[string]string
	otpReposto       map[string]bool
	sessoesRevogadas []string
	apagados         []string
}

func (f *fakeAdmin) ListarUtilizadores(context.Context, appident.FiltroUtilizadores) ([]appident.ResumoUtilizador, error) {
	return f.lista, f.err
}
func (f *fakeAdmin) ObterUtilizador(context.Context, string) (appident.DetalheUtilizador, error) {
	return f.detalhe, f.err
}
func (f *fakeAdmin) AtribuirPapel(_ context.Context, id string, p dominio.Papel) error {
	if f.err != nil {
		return f.err
	}
	f.atribuido = append(f.atribuido, id+":"+string(p))
	return nil
}
func (f *fakeAdmin) RevogarPapel(_ context.Context, id string, p dominio.Papel) error {
	if f.err != nil {
		return f.err
	}
	f.revogado = append(f.revogado, id+":"+string(p))
	return nil
}
func (f *fakeAdmin) DefinirActivo(_ context.Context, id string, activo bool) error {
	if f.err != nil {
		return f.err
	}
	if f.activo == nil {
		f.activo = map[string]bool{}
	}
	f.activo[id] = activo
	return nil
}
func (f *fakeAdmin) CriarUtilizador(context.Context, appident.DadosNovoUtilizador) (string, error) {
	return "", f.err
}
func (f *fakeAdmin) DefinirPasswordTemporaria(_ context.Context, id, senha string) error {
	if f.err != nil {
		return f.err
	}
	if f.passwordDefinida == nil {
		f.passwordDefinida = map[string]string{}
	}
	f.passwordDefinida[id] = senha
	return nil
}
func (f *fakeAdmin) ResetOTP(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	if f.otpReposto == nil {
		f.otpReposto = map[string]bool{}
	}
	f.otpReposto[id] = true
	return nil
}
func (f *fakeAdmin) RevogarSessoes(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.sessoesRevogadas = append(f.sessoesRevogadas, id)
	return nil
}
func (f *fakeAdmin) ApagarUtilizador(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.apagados = append(f.apagados, id)
	return nil
}

// --- Testes ---

func TestAtribuirPapel_ValidaEAudita(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoAtribuirPapel(admin, aud)

	if err := caso.Executar(context.Background(), "actor-1", "alvo-1", dominio.PapelMedico); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(admin.atribuido) != 1 || admin.atribuido[0] != "alvo-1:Medico" {
		t.Fatalf("atribuição não delegada: %v", admin.atribuido)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.papel.atribuido" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
	if aud.registos[0].Actor != "actor-1" || aud.registos[0].EntidadeID != "alvo-1" {
		t.Fatalf("auditoria com dados errados: %+v", aud.registos[0])
	}
}

func TestAtribuirPapel_PapelInvalido(t *testing.T) {
	admin := &fakeAdmin{}
	caso := appident.NovoCasoAtribuirPapel(admin, &fakeAuditor{})
	err := caso.Executar(context.Background(), "actor-1", "alvo-1", dominio.Papel("Inexistente"))
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
	if len(admin.atribuido) != 0 {
		t.Fatal("não devia ter delegado com papel inválido")
	}
}

func TestRevogarPapel_Audita(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoRevogarPapel(admin, aud)
	if err := caso.Executar(context.Background(), "actor-1", "alvo-1", dominio.PapelAdmin); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.papel.revogado" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
}

func TestDefinirActivo_AuditaAccaoCorrecta(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoDefinirActivo(admin, aud)

	if err := caso.Executar(context.Background(), "actor-1", "alvo-1", false); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if admin.activo["alvo-1"] != false {
		t.Fatal("estado activo não aplicado")
	}
	if aud.registos[0].Accao != "identidade.utilizador.desactivado" {
		t.Fatalf("acção errada: %q", aud.registos[0].Accao)
	}
}

func TestListarUtilizadores_Delega(t *testing.T) {
	admin := &fakeAdmin{lista: []appident.ResumoUtilizador{{ID: "u1", Nome: "Ana"}}}
	caso := appident.NovoCasoListarUtilizadores(admin)
	out, err := caso.Executar(context.Background(), appident.FiltroUtilizadores{})
	if err != nil || len(out) != 1 || out[0].ID != "u1" {
		t.Fatalf("listagem inesperada: %v, %v", out, err)
	}
}

func TestObterUtilizador_PropagaErro(t *testing.T) {
	admin := &fakeAdmin{err: errors.New("falha")}
	caso := appident.NovoCasoObterUtilizador(admin)
	if _, err := caso.Executar(context.Background(), "u1"); err == nil {
		t.Fatal("esperava erro propagado")
	}
}

func TestDefinirActivo_DesactivarRevogaSessoes(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoDefinirActivo(admin, aud)
	if err := caso.Executar(context.Background(), "actor-1", "alvo-1", false); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(admin.sessoesRevogadas) != 1 || admin.sessoesRevogadas[0] != "alvo-1" {
		t.Fatalf("esperava revogação de sessões ao desactivar: %v", admin.sessoesRevogadas)
	}
}

func TestDefinirActivo_ActivarNaoRevoga(t *testing.T) {
	admin := &fakeAdmin{}
	caso := appident.NovoCasoDefinirActivo(admin, &fakeAuditor{})
	if err := caso.Executar(context.Background(), "actor-1", "alvo-1", true); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(admin.sessoesRevogadas) != 0 {
		t.Fatalf("activar não deve revogar sessões: %v", admin.sessoesRevogadas)
	}
}
