package clinico

import (
	"testing"
)

func TestConstruirNota_AparaEspacos(t *testing.T) {
	n := construirNota(DadosNotaClinica{QueixaPrincipal: "  Febre  ", ExameObjectivo: " Temp ", Diagnostico: "Gripe", Plano: "Repouso"})
	if n.QueixaPrincipal != "Febre" || n.ExameObjectivo != "Temp" {
		t.Fatalf("nota não aparada: %+v", n)
	}
}

func TestConstruirDiagnosticos_Mapeia(t *testing.T) {
	out := construirDiagnosticos([]DadosDiagnosticoCID{{CID: "J11", Principal: true}, {CID: "J12"}})
	if len(out) != 2 || out[0].CID != "J11" || !out[0].Principal || out[1].CID != "J12" || out[1].Principal {
		t.Fatalf("diagnósticos mal mapeados: %+v", out)
	}
}
