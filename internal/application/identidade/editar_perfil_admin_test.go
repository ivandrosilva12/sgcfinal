package identidade_test

import (
	"context"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// fakeRepoPerfil modela a presença (ou ausência) da linha local, para exercitar
// a hidratação JIT a partir do Keycloak.
type fakeRepoPerfil struct {
	existe   bool
	u        *dominio.Utilizador
	guardou  bool
	telefone string
	bi       string
}

func (f *fakeRepoPerfil) ObterPorID(_ context.Context, _ string) (*dominio.Utilizador, error) {
	if !f.existe {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, i18n.T(i18n.MsgUtilizadorNaoEncontrado))
	}
	return f.u, nil
}
func (f *fakeRepoPerfil) GuardarComPapeis(_ context.Context, u *dominio.Utilizador) error {
	f.existe = true
	f.u = u
	f.guardou = true
	return nil
}
func (f *fakeRepoPerfil) AtualizarContacto(_ context.Context, _, telefone, bi string) error {
	f.telefone, f.bi = telefone, bi
	if f.u != nil {
		f.u.Telefone, f.u.BI = telefone, bi
	}
	return nil
}

func TestEditarPerfilAdmin_LinhaExistente(t *testing.T) {
	base, _ := dominio.NovoUtilizador("u1", "Ana Silva", "ana@sgc.ao", "", "", []dominio.Papel{dominio.PapelMedico})
	repo := &fakeRepoPerfil{existe: true, u: base}
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoEditarPerfilAdmin(admin, repo, aud)

	// O agregado normaliza para o formato de apresentação "+244 9XX XXX XXX".
	tel := "+244 923 456 789"
	perfil, err := caso.Executar(context.Background(), "admin-1", "u1", &tel, nil)
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if perfil.Telefone != "+244 923 456 789" {
		t.Fatalf("telefone não persistido: %+v", perfil)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.perfil.actualizado" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
	if aud.registos[0].Actor != "admin-1" || aud.registos[0].EntidadeID != "u1" {
		t.Fatalf("auditoria com dados errados: %+v", aud.registos[0])
	}
}

func TestEditarPerfilAdmin_HidrataDoKeycloak(t *testing.T) {
	repo := &fakeRepoPerfil{existe: false} // sem linha local
	admin := &fakeAdmin{detalhe: appident.DetalheUtilizador{
		ID: "u2", Nome: "Rui Mendes", Email: "rui@sgc.ao", Papeis: []string{"Enfermeiro"},
	}}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoEditarPerfilAdmin(admin, repo, aud)

	tel := "+244 912 000 000"
	perfil, err := caso.Executar(context.Background(), "admin-1", "u2", &tel, nil)
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if !repo.guardou {
		t.Fatal("esperava hidratação (GuardarComPapeis) da linha local a partir do Keycloak")
	}
	if perfil.Nome != "Rui Mendes" || perfil.Telefone != "+244 912 000 000" {
		t.Fatalf("perfil inesperado: %+v", perfil)
	}
}

func TestEditarPerfilAdmin_TelefoneInvalido(t *testing.T) {
	base, _ := dominio.NovoUtilizador("u1", "Ana", "ana@sgc.ao", "", "", nil)
	repo := &fakeRepoPerfil{existe: true, u: base}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoEditarPerfilAdmin(&fakeAdmin{}, repo, aud)

	mau := "não-é-telefone"
	_, err := caso.Executar(context.Background(), "admin-1", "u1", &mau, nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
	// Entrada inválida: não deve persistir nem auditar.
	if repo.telefone != "" || repo.bi != "" {
		t.Fatalf("não devia persistir com telefone inválido: telefone=%q bi=%q", repo.telefone, repo.bi)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("não devia auditar com telefone inválido: %v", aud.registos)
	}
}
