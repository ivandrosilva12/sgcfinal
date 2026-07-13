package clinico_test

import (
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func consentimentoCirurgiaValido(t *testing.T) *dominio.Consentimento {
	t.Helper()
	c, err := dominio.NovoConsentimento("doente-1", dominio.FinalidadeCirurgia, true, "s3://c.pdf",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("consentimento base inválido: %v", err)
	}
	return c
}

func dadosProc() dominio.DadosNovoProcedimento {
	return dominio.DadosNovoProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura",
		CirurgiaoID: "cir-1", Anestesia: dominio.AnestesiaLocal, AnestesistaID: "an-1",
	}
}

func TestNovoProcedimento_ConsentimentoInvalido(t *testing.T) {
	// Consentimento de tratamento (não cirurgia) → RegraNegocio.
	cons, _ := dominio.NovoConsentimento("doente-1", dominio.FinalidadeTratamento, true, "",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	if _, err := dominio.NovoProcedimento(dadosProc(), cons); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("consentimento não-cirurgia devia falhar com RegraNegocio, veio %v", err)
	}
	// Consentimento nil → RegraNegocio.
	if _, err := dominio.NovoProcedimento(dadosProc(), nil); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("consentimento nil devia falhar com RegraNegocio, veio %v", err)
	}
}

func TestNovoProcedimento_AnestesistaObrigatorio(t *testing.T) {
	d := dadosProc()
	d.AnestesistaID = ""
	if _, err := dominio.NovoProcedimento(d, consentimentoCirurgiaValido(t)); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("anestesia≠NENHUMA sem anestesista devia falhar com Validacao, veio %v", err)
	}
	// Com NENHUMA não é preciso anestesista.
	d.Anestesia = dominio.AnestesiaNenhuma
	if _, err := dominio.NovoProcedimento(d, consentimentoCirurgiaValido(t)); err != nil {
		t.Fatalf("NENHUMA sem anestesista devia ser válido: %v", err)
	}
}

func TestProcedimento_StateMachine(t *testing.T) {
	p, err := dominio.NovoProcedimento(dadosProc(), consentimentoCirurgiaValido(t))
	if err != nil {
		t.Fatalf("construção válida falhou: %v", err)
	}
	if p.Estado() != dominio.ProcAgendado {
		t.Fatalf("esperado AGENDADO, veio %s", p.Estado())
	}
	// Concluir sem iniciar → Conflito.
	if err := p.Concluir(time.Now(), "", ""); err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("concluir sem iniciar devia falhar com Conflito, veio %v", err)
	}
	inicio := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	if err := p.Iniciar(inicio); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}
	if p.Estado() != dominio.ProcEmCurso {
		t.Fatalf("esperado EM_CURSO, veio %s", p.Estado())
	}
	// Iniciar de novo → Conflito.
	if err := p.Iniciar(inicio); err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("iniciar de novo devia falhar com Conflito, veio %v", err)
	}
	// Concluir com fim antes do início → Validacao.
	if err := p.Concluir(inicio.Add(-time.Hour), "", ""); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("fim antes do início devia falhar com Validacao, veio %v", err)
	}
	if err := p.Concluir(inicio.Add(time.Hour), "sem complicações", ""); err != nil {
		t.Fatalf("concluir devia funcionar: %v", err)
	}
	if p.Estado() != dominio.ProcConcluido {
		t.Fatalf("esperado CONCLUIDO, veio %s", p.Estado())
	}
}

func TestProcedimento_Cancelar_SoEmCurso(t *testing.T) {
	p, _ := dominio.NovoProcedimento(dadosProc(), consentimentoCirurgiaValido(t))
	// Cancelar em AGENDADO → Conflito (DDM estrito).
	if err := p.Cancelar(time.Now(), "desmarcado"); err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("cancelar AGENDADO devia falhar com Conflito, veio %v", err)
	}
	inicio := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	_ = p.Iniciar(inicio)
	if err := p.Cancelar(inicio.Add(time.Minute), "complicação intra-op"); err != nil {
		t.Fatalf("cancelar EM_CURSO devia funcionar: %v", err)
	}
	if p.Estado() != dominio.ProcCancelado {
		t.Fatalf("esperado CANCELADO, veio %s", p.Estado())
	}
}

func procedimentoCorrompidoEmCursoSemInicio() *dominio.ProcedimentoCirurgico {
	// Simula uma rehidratação a partir de um snapshot corrompido (ex.: bug de
	// mapeamento no pgrepo que não faz Scan da coluna "inicio"): estado
	// EM_CURSO mas sem início registado.
	return dominio.ReconstruirProcedimento(dominio.SnapshotProcedimento{
		ID: "proc-1", EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura",
		CirurgiaoID: "cir-1", Anestesia: dominio.AnestesiaLocal, AnestesistaID: "an-1",
		Estado: dominio.ProcEmCurso, Inicio: nil,
	})
}

func TestProcedimento_Concluir_EmCursoSemInicio_NaoEntraEmPanic(t *testing.T) {
	p := procedimentoCorrompidoEmCursoSemInicio()
	err := p.Concluir(time.Now(), "", "")
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("concluir EM_CURSO sem início devia falhar com Conflito, veio %v", err)
	}
}

func TestProcedimento_Cancelar_EmCursoSemInicio_NaoEntraEmPanic(t *testing.T) {
	p := procedimentoCorrompidoEmCursoSemInicio()
	err := p.Cancelar(time.Now(), "motivo")
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("cancelar EM_CURSO sem início devia falhar com Conflito, veio %v", err)
	}
}
