package laboratorio_test

import (
	"context"
	"testing"

	adlaboratorio "github.com/ivandrosilva12/sgcfinal/internal/adapters/laboratorio"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeUtilizadores implementa identidade.RepositorioUtilizadores para o teste.
type fakeUtilizadores struct {
	u   *identidade.Utilizador
	err error
}

func (f fakeUtilizadores) ObterPorID(_ context.Context, _ string) (*identidade.Utilizador, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.u, nil
}
func (f fakeUtilizadores) GuardarComPapeis(_ context.Context, _ *identidade.Utilizador) error { return nil }
func (f fakeUtilizadores) AtualizarContacto(_ context.Context, _, _, _ string) error         { return nil }

func TestResolvedorContacto_ComTelefone(t *testing.T) {
	r := adlaboratorio.NovoResolvedorContacto(fakeUtilizadores{u: &identidade.Utilizador{Telefone: "+244 923 000 000"}})
	tel, ok, err := r.ContactoClinico(context.Background(), "kc-1")
	if err != nil || !ok || tel != "+244 923 000 000" {
		t.Fatalf("esperava o telefone, veio tel=%q ok=%v err=%v", tel, ok, err)
	}
}

func TestResolvedorContacto_SemTelefone(t *testing.T) {
	r := adlaboratorio.NovoResolvedorContacto(fakeUtilizadores{u: &identidade.Utilizador{Telefone: ""}})
	_, ok, err := r.ContactoClinico(context.Background(), "kc-1")
	if err != nil || ok {
		t.Fatalf("sem telefone devia dar ok=false sem erro, veio ok=%v err=%v", ok, err)
	}
}

func TestResolvedorContacto_Inexistente(t *testing.T) {
	r := adlaboratorio.NovoResolvedorContacto(fakeUtilizadores{err: erros.Novo(erros.CategoriaNaoEncontrado, "não existe")})
	_, ok, err := r.ContactoClinico(context.Background(), "kc-1")
	if err != nil || ok {
		t.Fatalf("utilizador inexistente devia dar ok=false sem erro, veio ok=%v err=%v", ok, err)
	}
}
