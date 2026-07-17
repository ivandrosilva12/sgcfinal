package outbox_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
	domclinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func TestCodificar_EpisodioFechado(t *testing.T) {
	ev := domclinico.EpisodioFechado{
		EpisodioID: "ep-1", DoenteID: "do-1",
		Em: time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC),
	}
	agregado, payload, err := outbox.Codificar(ev)
	if err != nil {
		t.Fatalf("codificar: %v", err)
	}
	if agregado != "episodio" {
		t.Fatalf("agregado errado: %q", agregado)
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("payload não é JSON: %v", err)
	}
	if m["EpisodioID"] != "ep-1" || m["DoenteID"] != "do-1" {
		t.Fatalf("payload sem os campos esperados: %s", payload)
	}
}

type eventoDesconhecido struct{}

func (eventoDesconhecido) NomeEvento() string    { return "x.y.z" }
func (eventoDesconhecido) OcorridoEm() time.Time { return time.Time{} }

func TestCodificar_TipoNaoMapeado_Erro(t *testing.T) {
	if _, _, err := outbox.Codificar(eventoDesconhecido{}); err == nil {
		t.Fatalf("evento não mapeado devia devolver erro")
	}
}
