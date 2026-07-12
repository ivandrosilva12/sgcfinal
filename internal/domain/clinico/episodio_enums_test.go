package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func TestParseTipoEpisodio(t *testing.T) {
	if tp, err := clinico.ParseTipoEpisodio("consulta"); err != nil || tp != clinico.EpisodioConsulta {
		t.Fatalf("ParseTipoEpisodio(consulta)=%v,%v", tp, err)
	}
	if _, err := clinico.ParseTipoEpisodio("CIRURGIA"); err == nil {
		t.Fatal("esperava erro para tipo inválido")
	}
}
