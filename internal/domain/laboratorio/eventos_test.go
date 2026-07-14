package laboratorio_test

import (
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
)

func TestEventos_NomeEOcorridoEm(t *testing.T) {
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	colhida := dominio.AmostraColhida{ResultadoID: "res-1", RequisicaoID: "req-1", CodigoAnalise: "HB", Em: quando}
	if colhida.NomeEvento() != "laboratorio.amostra.colhida" {
		t.Fatalf("nome de evento inesperado: %q", colhida.NomeEvento())
	}
	if !colhida.OcorridoEm().Equal(quando) {
		t.Fatalf("data de ocorrência inesperada: %v", colhida.OcorridoEm())
	}

	recusada := dominio.AmostraRecusada{ResultadoID: "res-1", RequisicaoID: "req-1", Motivo: "amostra coagulada", Em: quando}
	if recusada.NomeEvento() != "laboratorio.amostra.recusada" {
		t.Fatalf("nome de evento inesperado: %q", recusada.NomeEvento())
	}
	if !recusada.OcorridoEm().Equal(quando) {
		t.Fatalf("data de ocorrência inesperada: %v", recusada.OcorridoEm())
	}

	preliminar := dominio.ResultadoPreliminarSubmetido{ResultadoID: "res-1", RequisicaoID: "req-1", CodigoAnalise: "HB", Em: quando}
	if preliminar.NomeEvento() != "laboratorio.resultado.preliminar_submetido" {
		t.Fatalf("nome de evento inesperado: %q", preliminar.NomeEvento())
	}
	if !preliminar.OcorridoEm().Equal(quando) {
		t.Fatalf("data de ocorrência inesperada: %v", preliminar.OcorridoEm())
	}
}
