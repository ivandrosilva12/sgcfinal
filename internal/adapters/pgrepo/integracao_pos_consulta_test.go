package pgrepo

// Teste unitário puro (sem BD) dos ramos de erro de HandlerEpisodioFechado na
// desserialização do payload — ambos devolvem erro antes de tocar no pool, por
// isso um *IntegracaoPosConsulta com pool nil é seguro aqui.

import (
	"context"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
)

func TestHandlerEpisodioFechado_PayloadInvalido(t *testing.T) {
	a := &IntegracaoPosConsulta{}

	casos := []struct {
		nome    string
		payload []byte
	}{
		{nome: "JSON malformado", payload: []byte("{")},
		{nome: "EpisodioID vazio", payload: []byte(`{"EpisodioID":""}`)},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			ev := outbox.EventoEntregue{
				TipoEvento: "clinico.episodio.fechado",
				Payload:    c.payload,
			}
			if err := a.HandlerEpisodioFechado(context.Background(), ev); err == nil {
				t.Fatal("esperava erro, obteve nil")
			}
		})
	}
}
