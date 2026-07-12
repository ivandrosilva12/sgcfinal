package identidade

import (
	"testing"
	"time"
)

func TestSessaoRevogada_Evento(t *testing.T) {
	em := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	e := SessaoRevogada{Actor: "admin-1", SessionID: "sess-9", Em: em}

	if e.NomeEvento() != "identidade.sessao.revogada" {
		t.Fatalf("NomeEvento = %q; quer identidade.sessao.revogada", e.NomeEvento())
	}
	if !e.OcorridoEm().Equal(em) {
		t.Fatalf("OcorridoEm = %v; quer %v", e.OcorridoEm(), em)
	}
}
