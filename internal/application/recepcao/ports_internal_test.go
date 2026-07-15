// internal/application/recepcao/ports_internal_test.go
package recepcao

import (
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
)

// TestParaDetalheMarcacao_Projecta cobre o mapeador paraDetalheMarcacao, consumido
// pelas Tasks 5–6 (casos de marcação/remarcação) mas já introduzido nesta task como
// parte das ports partilhadas.
func TestParaDetalheMarcacao_Projecta(t *testing.T) {
	inicio := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	fim := time.Date(2026, 7, 20, 8, 30, 0, 0, time.UTC)
	m, err := dominio.NovaMarcacao("doe-1", "med-1", "esp-1", inicio, fim)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}

	d := paraDetalheMarcacao(m)

	if d.DoenteID != "doe-1" || d.MedicoID != "med-1" || d.EspecialidadeID != "esp-1" {
		t.Fatalf("detalhe mal projectado: %+v", d)
	}
	if d.Estado != string(dominio.MarcMarcada) {
		t.Fatalf("estado mal projectado: %q", d.Estado)
	}
	if !d.Inicio.Equal(inicio) || !d.Fim.Equal(fim) {
		t.Fatalf("intervalo mal projectado: %+v", d)
	}
	if d.RemarcaDe != "" || d.Motivo != "" {
		t.Fatalf("campos opcionais deviam estar vazios: %+v", d)
	}
}
