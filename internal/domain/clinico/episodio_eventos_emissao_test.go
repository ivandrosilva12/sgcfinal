package clinico_test

import (
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func episodioAbertoCompleto(t *testing.T) *dominio.EpisodioClinico {
	t.Helper()
	e, err := dominio.NovoEpisodio("11111111-1111-4111-8111-111111111111",
		dominio.EpisodioConsulta, "22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333", time.Now())
	if err != nil {
		t.Fatalf("novo episódio: %v", err)
	}
	nota := dominio.NovaNotaClinica("queixa", "história", "exame", "diagnóstico", "plano")
	if err := e.ActualizarNota(nota); err != nil {
		t.Fatalf("actualizar nota: %v", err)
	}
	cid, err := dominio.NovoDiagnosticoCID("J06", true) // (cid string, principal bool)
	if err != nil {
		t.Fatalf("cid: %v", err)
	}
	if err := e.DefinirDiagnosticosCID([]dominio.DiagnosticoCID{cid}); err != nil {
		t.Fatalf("definir cid: %v", err)
	}
	return e
}

func TestFechar_EmiteEpisodioFechado(t *testing.T) {
	e := episodioAbertoCompleto(t)
	if len(e.EventosPendentes()) != 0 {
		t.Fatalf("episódio aberto não devia ter eventos pendentes")
	}
	if err := e.Fechar("33333333-3333-4333-8333-333333333333", time.Now()); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	pend := e.EventosPendentes()
	if len(pend) != 1 {
		t.Fatalf("esperava 1 evento, obtive %d", len(pend))
	}
	ev, ok := pend[0].(dominio.EpisodioFechado)
	if !ok {
		t.Fatalf("esperava EpisodioFechado, obtive %T", pend[0])
	}
	if ev.DoenteID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("doente do evento errado: %q", ev.DoenteID)
	}
	if ev.NomeEvento() != "clinico.episodio.fechado" {
		t.Fatalf("nome do evento errado: %q", ev.NomeEvento())
	}
}

func TestFechar_Invalido_NaoEmite(t *testing.T) {
	e := episodioAbertoCompleto(t)
	// fecha uma vez (válido)
	_ = e.Fechar("33333333-3333-4333-8333-333333333333", time.Now())
	// segundo fecho é inválido (já fechado) e não deve acrescentar evento
	if err := e.Fechar("x", time.Now()); err == nil {
		t.Fatalf("segundo fecho devia falhar")
	}
	if len(e.EventosPendentes()) != 1 {
		t.Fatalf("um fecho inválido não devia emitir; esperava 1, obtive %d", len(e.EventosPendentes()))
	}
}
