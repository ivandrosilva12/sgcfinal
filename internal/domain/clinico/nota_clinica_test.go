package clinico_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func TestNotaClinica_Completa(t *testing.T) {
	completa := clinico.NovaNotaClinica("Febre", "", "Temp 39", "Gripe", "Repouso")
	if !completa.Completa() {
		t.Fatal("esperava nota completa (queixa+exame+diagnóstico+plano)")
	}
	semExame := clinico.NovaNotaClinica("Febre", "", "", "Gripe", "Repouso")
	if semExame.Completa() {
		t.Fatal("nota sem exame não devia ser completa")
	}
}

func TestNovaNotaClinica_AparaEspacos(t *testing.T) {
	n := clinico.NovaNotaClinica("  Febre  ", " ", " Temp ", " Gripe ", " Repouso ")
	if n.QueixaPrincipal != "Febre" || n.ExameObjectivo != "Temp" {
		t.Fatalf("não aparou espaços: %+v", n)
	}
}
