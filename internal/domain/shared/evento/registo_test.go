package evento_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

type eventoFalso struct{ nome string }

func (e eventoFalso) NomeEvento() string    { return e.nome }
func (e eventoFalso) OcorridoEm() time.Time { return time.Time{} }

func TestRegistoEventos_AcumulaEDevolveNaOrdem(t *testing.T) {
	var r evento.RegistoEventos
	if len(r.EventosPendentes()) != 0 {
		t.Fatalf("registo novo devia estar vazio")
	}
	r.RegistarEvento(eventoFalso{nome: "a"})
	r.RegistarEvento(eventoFalso{nome: "b"})
	pend := r.EventosPendentes()
	if len(pend) != 2 || pend[0].NomeEvento() != "a" || pend[1].NomeEvento() != "b" {
		t.Fatalf("esperava [a b], obtive %v", pend)
	}
}

func TestRegistoEventos_LimparEsvazia(t *testing.T) {
	var r evento.RegistoEventos
	r.RegistarEvento(eventoFalso{nome: "a"})
	r.LimparEventos()
	if len(r.EventosPendentes()) != 0 {
		t.Fatalf("após limpar devia estar vazio, obtive %d", len(r.EventosPendentes()))
	}
}
