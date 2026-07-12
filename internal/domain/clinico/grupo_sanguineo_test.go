package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func TestParseGrupoSanguineo(t *testing.T) {
	g, err := clinico.ParseGrupoSanguineo("o+")
	if err != nil || g != clinico.GrupoOPositivo {
		t.Fatalf("ParseGrupoSanguineo(o+)=%v,%v", g, err)
	}
	if g.String() != "O+" {
		t.Fatalf("String()=%q", g.String())
	}
	if _, err := clinico.ParseGrupoSanguineo("Z+"); err == nil {
		t.Fatal("esperava erro para grupo inválido")
	}
}
