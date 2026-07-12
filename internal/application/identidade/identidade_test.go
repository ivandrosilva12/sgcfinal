package identidade_test

import (
	"context"
	"errors"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// --- Fakes das portas (preferidos a mocks, per CLAUDE.md §5.7) ---

type fakeVerificador struct {
	sessao dominio.Sessao
	err    error
}

func (f fakeVerificador) Verificar(context.Context, string) (dominio.Sessao, error) {
	return f.sessao, f.err
}

type fakeRepo struct {
	guardado        *dominio.Utilizador
	guardarErr      error
	obterErr        error
	chamadasGuardar int
	atualizarErr    error
}

func (f *fakeRepo) ObterPorID(context.Context, string) (*dominio.Utilizador, error) {
	if f.obterErr != nil {
		return nil, f.obterErr
	}
	return f.guardado, nil
}

func (f *fakeRepo) GuardarComPapeis(_ context.Context, u *dominio.Utilizador) error {
	f.chamadasGuardar++
	if f.guardarErr != nil {
		return f.guardarErr
	}
	// Upsert: preserve existing local fields (telefone, BI) if user already exists
	if f.guardado != nil && f.guardado.KeycloakID == u.KeycloakID {
		u.Telefone = f.guardado.Telefone
		u.BI = f.guardado.BI
	}
	f.guardado = u
	return nil
}

func (f *fakeRepo) AtualizarContacto(_ context.Context, _, telefone, bi string) error {
	if f.atualizarErr != nil {
		return f.atualizarErr
	}
	if f.guardado != nil {
		f.guardado.Telefone = telefone
		f.guardado.BI = bi
	}
	return nil
}

type fakeAuditor struct {
	registos []auditoria.Registo
	err      error
}

func (f *fakeAuditor) Registar(_ context.Context, r auditoria.Registo) error {
	if f.err != nil {
		return f.err
	}
	f.registos = append(f.registos, r)
	return nil
}

// --- CasoAutenticar ---

func TestAutenticar_Sucesso(t *testing.T) {
	esperada := dominio.Sessao{Sujeito: "uuid-1", Papeis: []dominio.Papel{dominio.PapelMedico}}
	caso := appident.NovoCasoAutenticar(fakeVerificador{sessao: esperada})

	got, err := caso.Executar(context.Background(), "token-valido")
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if got.Sujeito != "uuid-1" || !got.TemPapel(dominio.PapelMedico) {
		t.Fatalf("sessão inesperada: %+v", got)
	}
}

func TestAutenticar_PropagaErro(t *testing.T) {
	falha := errors.New("token inválido")
	caso := appident.NovoCasoAutenticar(fakeVerificador{err: falha})

	if _, err := caso.Executar(context.Background(), "mau"); !errors.Is(err, falha) {
		t.Fatalf("esperava propagação do erro do verificador, obtive %v", err)
	}
}

// --- CasoObterPerfil ---

func novaSessao() dominio.Sessao {
	return dominio.Sessao{
		Sujeito: "uuid-1",
		Nome:    "Ana Silva",
		Email:   "ana@sgc.ao",
		Papeis:  []dominio.Papel{dominio.PapelMedico},
	}
}

func TestObterPerfil_JITeAuditoria(t *testing.T) {
	repo := &fakeRepo{}
	auditor := &fakeAuditor{}
	caso := appident.NovoCasoObterPerfil(repo, auditor)

	perfil, err := caso.Executar(context.Background(), novaSessao())
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}

	// JIT provisioning: upsert foi chamado.
	if repo.chamadasGuardar != 1 {
		t.Fatalf("esperava 1 upsert (JIT), obtive %d", repo.chamadasGuardar)
	}
	if perfil.KeycloakID != "uuid-1" || perfil.Nome != "Ana Silva" {
		t.Fatalf("perfil inesperado: %+v", perfil)
	}
	if len(perfil.Papeis) != 1 || perfil.Papeis[0] != "Medico" {
		t.Fatalf("papéis do perfil inesperados: %v", perfil.Papeis)
	}

	// Auditoria de acesso registada.
	if len(auditor.registos) != 1 {
		t.Fatalf("esperava 1 registo de auditoria, obtive %d", len(auditor.registos))
	}
	reg := auditor.registos[0]
	if reg.Accao != "identidade.perfil.consultado" || reg.Actor != "uuid-1" {
		t.Fatalf("registo de auditoria inesperado: %+v", reg)
	}
}

func TestObterPerfil_ErroRepo(t *testing.T) {
	caso := appident.NovoCasoObterPerfil(&fakeRepo{guardarErr: errors.New("bd em baixo")}, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), novaSessao()); err == nil {
		t.Fatal("esperava erro do repositório")
	}
}

func TestObterPerfil_ErroAuditoria(t *testing.T) {
	caso := appident.NovoCasoObterPerfil(&fakeRepo{}, &fakeAuditor{err: errors.New("auditoria falhou")})
	if _, err := caso.Executar(context.Background(), novaSessao()); err == nil {
		t.Fatal("esperava erro da auditoria")
	}
}

func TestObterPerfil_SessaoInvalida(t *testing.T) {
	s := novaSessao()
	s.Email = "não-é-email"
	caso := appident.NovoCasoObterPerfil(&fakeRepo{}, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), s); err == nil {
		t.Fatal("esperava erro de validação por email inválido")
	}
}
