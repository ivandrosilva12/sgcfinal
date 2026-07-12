package farmacia

import (
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
)

func TestParaDetalheReceita_Mapeia(t *testing.T) {
	agora := time.Now()
	itens := []dominio.ItemReceita{
		{MedicamentoID: "med-1", Posologia: "1 comp. 8/8h", QuantidadePrescrita: 21},
	}
	r := dominio.ReconstruirReceita(dominio.SnapshotReceita{
		ID: "rec-1", EpisodioID: "epi-1", DoenteID: "doe-1", MedicoID: "med-1",
		EmitidaEm: agora.Add(-24 * time.Hour), Estado: dominio.ReceitaEmitida,
		ExpiraEm: agora.Add(48 * time.Hour), Itens: itens,
	})

	det := paraDetalheReceita(r, agora)

	if det.ID != "rec-1" || det.EpisodioID != "epi-1" || det.DoenteID != "doe-1" || det.MedicoID != "med-1" {
		t.Fatalf("identificadores mal mapeados: %+v", det)
	}
	if det.Estado != string(dominio.ReceitaEmitida) {
		t.Fatalf("estado efectivo=%q, esperava %q", det.Estado, dominio.ReceitaEmitida)
	}
	if len(det.Itens) != 1 || det.Itens[0].MedicamentoID != "med-1" || det.Itens[0].QuantidadePrescrita != 21 {
		t.Fatalf("itens mal mapeados: %+v", det.Itens)
	}
}
