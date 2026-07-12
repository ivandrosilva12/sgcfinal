package clinico_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func episodioAberto(t *testing.T) *clinico.EpisodioClinico {
	t.Helper()
	e, err := clinico.NovoEpisodio("doente-1", clinico.EpisodioConsulta, "esp-1", "medico-1", time.Now())
	if err != nil {
		t.Fatalf("NovoEpisodio: %v", err)
	}
	return e
}

func TestNovoEpisodio_EstadoInicialAberto(t *testing.T) {
	e := episodioAberto(t)
	if e.Estado() != clinico.EstadoEpisodioAberto {
		t.Fatalf("estado inicial=%q, esperava ABERTO", e.Estado())
	}
	if e.DoenteID() != "doente-1" {
		t.Fatalf("doente=%q", e.DoenteID())
	}
}

func TestNovoEpisodio_CamposObrigatorios(t *testing.T) {
	if _, err := clinico.NovoEpisodio("", clinico.EpisodioConsulta, "esp-1", "medico-1", time.Now()); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para doente vazio")
	}
	if _, err := clinico.NovoEpisodio("d1", clinico.EpisodioConsulta, "esp-1", "medico-1", time.Time{}); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para início zero")
	}
}

func TestFechar_ExigeNotaCompletaEDiagnostico(t *testing.T) {
	e := episodioAberto(t)
	// Sem nota completa → erro de validação.
	if erros.CategoriaDe(e.Fechar("medico-1", time.Now())) != erros.CategoriaValidacao {
		t.Fatal("esperava validação: nota incompleta")
	}
	_ = e.ActualizarNota(clinico.NovaNotaClinica("Febre", "", "Temp 39", "Gripe", "Repouso"))
	// Nota completa mas sem CID → erro de validação.
	if erros.CategoriaDe(e.Fechar("medico-1", time.Now())) != erros.CategoriaValidacao {
		t.Fatal("esperava validação: sem diagnóstico CID")
	}
	cid, _ := clinico.NovoDiagnosticoCID("J11", true)
	_ = e.DefinirDiagnosticosCID([]clinico.DiagnosticoCID{cid})
	if err := e.Fechar("medico-1", time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if e.Estado() != clinico.EstadoEpisodioFechado {
		t.Fatalf("estado=%q, esperava FECHADO", e.Estado())
	}
	if e.Snapshot().FechadoPor != "medico-1" || e.Snapshot().Fim == nil {
		t.Fatalf("campos de fecho não preenchidos: %+v", e.Snapshot())
	}
}

func TestActualizarNota_ProibidaSeNaoAberto(t *testing.T) {
	e := episodioAberto(t)
	_ = e.ActualizarNota(clinico.NovaNotaClinica("Q", "", "E", "D", "P"))
	cid, _ := clinico.NovoDiagnosticoCID("J11", false)
	_ = e.DefinirDiagnosticosCID([]clinico.DiagnosticoCID{cid})
	_ = e.Fechar("medico-1", time.Now())
	if erros.CategoriaDe(e.ActualizarNota(clinico.NovaNotaClinica("X", "", "Y", "Z", "W"))) != erros.CategoriaConflito {
		t.Fatal("esperava conflito ao alterar nota de episódio fechado")
	}
}

func TestDefinirDiagnosticos_MaximoUmPrincipal(t *testing.T) {
	e := episodioAberto(t)
	c1, _ := clinico.NovoDiagnosticoCID("J11", true)
	c2, _ := clinico.NovoDiagnosticoCID("J12", true)
	if erros.CategoriaDe(e.DefinirDiagnosticosCID([]clinico.DiagnosticoCID{c1, c2})) != erros.CategoriaValidacao {
		t.Fatal("esperava validação: dois diagnósticos principais")
	}
}

func TestCancelar(t *testing.T) {
	e := episodioAberto(t)
	if err := e.Cancelar(time.Now()); err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	if e.Estado() != clinico.EstadoEpisodioCancelado {
		t.Fatalf("estado=%q, esperava CANCELADO", e.Estado())
	}
	// Cancelar de novo (já cancelado) → conflito.
	if erros.CategoriaDe(e.Cancelar(time.Now())) != erros.CategoriaConflito {
		t.Fatal("esperava conflito ao cancelar um episódio não aberto")
	}
}

func TestReconstruirEpisodio_PreservaEstado(t *testing.T) {
	orig := episodioAberto(t)
	_ = orig.Cancelar(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	snap := orig.Snapshot()
	snap.ID = "ep-1"
	rec := clinico.ReconstruirEpisodio(snap)
	if rec.ID() != "ep-1" || rec.Estado() != clinico.EstadoEpisodioCancelado {
		t.Fatalf("rehidratação perdeu estado: id=%q estado=%q", rec.ID(), rec.Estado())
	}
}
