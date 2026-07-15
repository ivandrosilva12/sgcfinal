package recepcao_test

import (
	"context"
	"testing"

	adrecepcao "github.com/ivandrosilva12/sgcfinal/internal/adapters/recepcao"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeDoentes é um duplo mínimo de clinico.RepositorioDoentes; só ObterPorID é usado.
type fakeDoentes struct {
	doente *clinico.Doente
	erro   error
}

func (f fakeDoentes) Guardar(context.Context, *clinico.Doente) (string, error) { return "", nil }
func (f fakeDoentes) ObterPorID(context.Context, string) (*clinico.Doente, error) {
	return f.doente, f.erro
}
func (f fakeDoentes) ObterPorNumProcesso(context.Context, string) (*clinico.Doente, error) {
	return nil, nil
}
func (f fakeDoentes) Pesquisar(context.Context, clinico.FiltroDoentes) (clinico.PaginaDoentes, error) {
	return clinico.PaginaDoentes{}, nil
}
func (f fakeDoentes) ProximoNumeroProcesso(context.Context, int) (string, error) { return "", nil }

func TestDoenteActivo_Inexistente_FalseSemErro(t *testing.T) {
	l := adrecepcao.NovoLeitorDoente(fakeDoentes{erro: erros.Novo(erros.CategoriaNaoEncontrado, "não existe")})
	ok, err := l.DoenteActivo(context.Background(), "doe-x")
	if err != nil || ok {
		t.Fatalf("doente inexistente devia dar (false, nil), veio (%v, %v)", ok, err)
	}
}
