// internal/application/recepcao/consultas_test.go
package recepcao_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
)

func TestListarAgenda_DevolveJanelasEMarcacoes(t *testing.T) {
	janelas := novoFakeJanelas()
	marc := novoFakeMarcacoes()
	_, _ = janelas.Guardar(context.Background(), janelaAgregada(t, "med-1", "esp-1", "08:00", "13:00"))
	_, _ = marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-1", "med-1", "esp-1", "09:00", "09:30"))

	uc := app.NovoCasoListarAgenda(janelas, marc)
	ag, err := uc.Executar(context.Background(), "med-1", inst("00:00"), inst("23:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if len(ag.Janelas) != 1 || len(ag.Marcacoes) != 1 {
		t.Fatalf("esperava 1 janela e 1 marcação, veio %d/%d", len(ag.Janelas), len(ag.Marcacoes))
	}
}

func TestListarMarcacoesDoente(t *testing.T) {
	marc := novoFakeMarcacoes()
	_, _ = marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-1", "med-1", "esp-1", "09:00", "09:30"))
	_, _ = marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-2", "med-1", "esp-1", "10:00", "10:30"))

	uc := app.NovoCasoListarMarcacoesDoente(marc)
	out, err := uc.Executar(context.Background(), "doe-1")
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if len(out) != 1 || out[0].DoenteID != "doe-1" {
		t.Fatalf("esperava só as marcações do doe-1, veio %+v", out)
	}
}
