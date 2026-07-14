package clinico_test

import (
	"testing"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestAnestesia_RequerAnestesista(t *testing.T) {
	if dominio.AnestesiaNenhuma.RequerAnestesista() {
		t.Fatalf("NENHUMA não devia exigir anestesista")
	}
	for _, a := range []dominio.Anestesia{dominio.AnestesiaLocal, dominio.AnestesiaSedacaoLigeira, dominio.AnestesiaLocoRegional} {
		if !a.RequerAnestesista() {
			t.Fatalf("%s devia exigir anestesista", a)
		}
	}
}

func TestParseAnestesia(t *testing.T) {
	if _, err := dominio.ParseAnestesia("sedacao_ligeira"); err != nil {
		t.Fatalf("sedacao_ligeira devia ser válida: %v", err)
	}
	if _, err := dominio.ParseAnestesia("GERAL"); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("GERAL devia falhar com Validacao, veio %v", err)
	}
}
