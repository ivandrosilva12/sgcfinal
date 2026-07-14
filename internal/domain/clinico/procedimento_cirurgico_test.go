package clinico_test

import (
	"strings"
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

func TestProcedimento_Cancelar_MotivoObrigatorio(t *testing.T) {
	p, _ := dominio.NovoProcedimento(dadosProc(), consentimentoCirurgiaValido(t))
	inicio := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	_ = p.Iniciar(inicio)
	// Motivo vazio → Validacao.
	if err := p.Cancelar(inicio.Add(time.Minute), ""); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("cancelar sem motivo devia falhar com Validacao, veio %v", err)
	}
	// Motivo só com espaços → Validacao.
	if err := p.Cancelar(inicio.Add(time.Minute), "   "); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("cancelar com motivo só de espaços devia falhar com Validacao, veio %v", err)
	}
	// O procedimento continua EM_CURSO — a tentativa falhada não muda o estado.
	if p.Estado() != dominio.ProcEmCurso {
		t.Fatalf("esperado EM_CURSO após tentativas falhadas, veio %s", p.Estado())
	}
}

// dadosProcComObservacoes devolve os dados de agendamento com uma nota
// pré-operatória (registo clínico que não pode perder-se).
func dadosProcComObservacoes() dominio.DadosNovoProcedimento {
	d := dadosProc()
	d.Observacoes = "doente anticoagulado — varfarina suspensa a 5/7"
	return d
}

// TestProcedimento_Cancelar_PreservaObservacoesPreOperatorias prova que o motivo
// do cancelamento é ANEXADO às observações e não as sobrepõe: antes da correcção,
// a nota pré-operatória desaparecia definitivamente da linha (não há versionamento).
func TestProcedimento_Cancelar_PreservaObservacoesPreOperatorias(t *testing.T) {
	p, err := dominio.NovoProcedimento(dadosProcComObservacoes(), consentimentoCirurgiaValido(t))
	if err != nil {
		t.Fatalf("novo procedimento: %v", err)
	}
	inicio := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	if err := p.Iniciar(inicio); err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if err := p.Cancelar(inicio.Add(20*time.Minute), "instabilidade hemodinâmica"); err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	obs := p.Snapshot().Observacoes
	if !strings.Contains(obs, "varfarina suspensa a 5/7") {
		t.Fatalf("a nota pré-operatória devia manter-se, veio %q", obs)
	}
	if !strings.Contains(obs, "instabilidade hemodinâmica") {
		t.Fatalf("o motivo do cancelamento devia ficar registado, veio %q", obs)
	}
}

// TestProcedimento_Concluir_ObservacoesAnexadas cobre os três casos do anexar:
// (a) sem texto anterior fica só o novo; (b) com texto anterior e novo, ficam os
// dois; (c) novo vazio mantém o anterior intacto.
func TestProcedimento_Concluir_ObservacoesAnexadas(t *testing.T) {
	inicio := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	fim := inicio.Add(time.Hour)

	// (a) sem observações prévias → fica só a nova.
	p, _ := dominio.NovoProcedimento(dadosProc(), consentimentoCirurgiaValido(t))
	_ = p.Iniciar(inicio)
	if err := p.Concluir(fim, "", "sem intercorrências"); err != nil {
		t.Fatalf("concluir: %v", err)
	}
	if obs := p.Snapshot().Observacoes; obs != "sem intercorrências" {
		t.Fatalf("esperado apenas a nova observação, veio %q", obs)
	}

	// (b) com observações prévias → ficam as duas.
	p2, _ := dominio.NovoProcedimento(dadosProcComObservacoes(), consentimentoCirurgiaValido(t))
	_ = p2.Iniciar(inicio)
	if err := p2.Concluir(fim, "", "sem intercorrências"); err != nil {
		t.Fatalf("concluir: %v", err)
	}
	obs := p2.Snapshot().Observacoes
	if !strings.Contains(obs, "varfarina suspensa a 5/7") || !strings.Contains(obs, "sem intercorrências") {
		t.Fatalf("esperadas as duas observações, veio %q", obs)
	}

	// (c) nova observação vazia (conclusão sem corpo) → mantém a anterior intacta.
	p3, _ := dominio.NovoProcedimento(dadosProcComObservacoes(), consentimentoCirurgiaValido(t))
	_ = p3.Iniciar(inicio)
	if err := p3.Concluir(fim, "nenhuma", "   "); err != nil {
		t.Fatalf("concluir: %v", err)
	}
	if obs := p3.Snapshot().Observacoes; obs != "doente anticoagulado — varfarina suspensa a 5/7" {
		t.Fatalf("esperada a observação pré-operatória intacta, veio %q", obs)
	}
}

// TestProcedimento_Snapshot_EstadoAnterior prova a base da guarda compare-and-set
// do repositório: um agregado novo não tem estado anterior; um agregado
// rehidratado expõe o estado com que foi lido, mesmo depois de transitar.
func TestProcedimento_Snapshot_EstadoAnterior(t *testing.T) {
	novo, _ := dominio.NovoProcedimento(dadosProc(), consentimentoCirurgiaValido(t))
	if ea := novo.Snapshot().EstadoAnterior; ea != "" {
		t.Fatalf("um procedimento novo não devia ter estado anterior, veio %q", ea)
	}

	inicio := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	lido := dominio.ReconstruirProcedimento(dominio.SnapshotProcedimento{
		ID: "proc-1", EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura",
		CirurgiaoID: "cir-1", Anestesia: dominio.AnestesiaLocal, AnestesistaID: "an-1",
		ConsentimentoID: "cons-1", Estado: dominio.ProcEmCurso, Inicio: &inicio,
	})
	if err := lido.Concluir(inicio.Add(time.Hour), "", ""); err != nil {
		t.Fatalf("concluir: %v", err)
	}
	s := lido.Snapshot()
	if s.Estado != dominio.ProcConcluido || s.EstadoAnterior != dominio.ProcEmCurso {
		t.Fatalf("esperado estado CONCLUIDO com estado anterior EM_CURSO, veio %s/%s", s.Estado, s.EstadoAnterior)
	}
}
