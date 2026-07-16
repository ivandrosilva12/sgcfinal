// internal/domain/recepcao/triagem_test.go
package recepcao_test

import (
	"testing"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovaTriagem_Valida(t *testing.T) {
	sv, _ := recepcao.NovosSinaisVitais(recepcao.SinaisVitais{Temperatura: fptr(37.0)})
	tr, err := recepcao.NovaTriagem("cheg-1", "enf-1", recepcao.ManAmarelo, sv, "cefaleia", inst("09:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if tr.ChegadaID() != "cheg-1" || tr.EnfermeiroID() != "enf-1" || tr.Prioridade() != recepcao.ManAmarelo {
		t.Fatalf("campos mal preenchidos: %+v", tr.Snapshot())
	}
	if tr.SinaisVitais().Temperatura == nil || *tr.SinaisVitais().Temperatura != 37.0 {
		t.Fatal("sinais vitais mal preenchidos")
	}
}

func TestNovaTriagem_CamposObrigatorios(t *testing.T) {
	sv := recepcao.SinaisVitais{}
	if _, err := recepcao.NovaTriagem("", "enf-1", recepcao.ManVerde, sv, "", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("sem chegada: esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
	if _, err := recepcao.NovaTriagem("cheg-1", "  ", recepcao.ManVerde, sv, "", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("sem enfermeiro: esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestNovaTriagem_PrioridadeInvalida(t *testing.T) {
	if _, err := recepcao.NovaTriagem("cheg-1", "enf-1", recepcao.PrioridadeManchester("ROXO"), recepcao.SinaisVitais{}, "", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestTriagem_RoundTrip(t *testing.T) {
	sv, _ := recepcao.NovosSinaisVitais(recepcao.SinaisVitais{Dor: iptr(5)})
	s := recepcao.SnapshotTriagem{
		ID: "tri-1", ChegadaID: "cheg-1", EnfermeiroID: "enf-1",
		Prioridade: recepcao.ManLaranja, SinaisVitais: sv, Observacoes: "x", TriadaEm: inst("09:00"),
	}
	tr := recepcao.ReconstruirTriagem(s)
	got := tr.Snapshot()
	if got.ID != "tri-1" || got.Prioridade != recepcao.ManLaranja || got.SinaisVitais.Dor == nil || *got.SinaisVitais.Dor != 5 {
		t.Fatalf("round-trip não preserva: %+v", got)
	}
}
