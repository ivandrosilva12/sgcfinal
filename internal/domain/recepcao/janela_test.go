// internal/domain/recepcao/janela_test.go
package recepcao_test

import (
	"testing"
	"time"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func inst(hhmm string) time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-07-20T"+hhmm+":00Z")
	return t
}

func TestNovaJanela_Valida(t *testing.T) {
	j, err := recepcao.NovaJanela("med-1", "esp-1", inst("08:00"), inst("13:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if j.MedicoID() != "med-1" || j.EspecialidadeID() != "esp-1" {
		t.Fatalf("campos mal preenchidos: %+v", j)
	}
	if !j.Inicio().Equal(inst("08:00")) || !j.Fim().Equal(inst("13:00")) {
		t.Fatalf("intervalo mal preenchido")
	}
}

func TestNovaJanela_FimNaoPosteriorAoInicio_Erro(t *testing.T) {
	_, err := recepcao.NovaJanela("med-1", "esp-1", inst("13:00"), inst("08:00"))
	if err == nil {
		t.Fatal("esperava erro quando fim <= início")
	}
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestNovaJanela_MedicoEmFalta_Erro(t *testing.T) {
	if _, err := recepcao.NovaJanela("  ", "esp-1", inst("08:00"), inst("13:00")); err == nil {
		t.Fatal("esperava erro com médico em falta")
	}
}

func TestJanela_SnapshotEReconstrucao(t *testing.T) {
	s := recepcao.SnapshotJanela{
		ID: "jan-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("08:00"), Fim: inst("13:00"), CriadoEm: inst("07:00"),
	}
	j := recepcao.ReconstruirJanela(s)
	if j.ID() != "jan-1" || j.Snapshot().MedicoID != "med-1" {
		t.Fatalf("snapshot não redondo: %+v", j.Snapshot())
	}
}
