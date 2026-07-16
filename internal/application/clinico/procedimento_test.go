package clinico_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// episodioCirurgico devolve um episódio ABERTO de cirurgia ambulatória para os fakes.
func episodioCirurgico(doenteID string) *clinico.EpisodioClinico {
	return clinico.ReconstruirEpisodio(clinico.SnapshotEpisodio{
		ID: "ep-1", DoenteID: doenteID, Tipo: clinico.EpisodioCirurgiaAmbulatoria,
		EspecialidadeID: "esp-1", MedicoID: "med-1", Inicio: nowUTC(),
		Estado: clinico.EstadoEpisodioAberto,
	})
}

func consentimentoGuardado(t *testing.T, repo *fakeConsentimentos, doenteID string) string {
	t.Helper()
	c, err := clinico.NovoConsentimento(doenteID, clinico.FinalidadeCirurgia, true, "s3://c.pdf", nowUTC())
	if err != nil {
		t.Fatalf("consentimento inválido: %v", err)
	}
	id, _ := repo.Guardar(context.Background(), c)
	return id
}

func TestAgendarProcedimento_EpisodioNaoCirurgico(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-2"] = clinico.ReconstruirEpisodio(clinico.SnapshotEpisodio{
		ID: "ep-2", DoenteID: "doente-1", Tipo: clinico.EpisodioConsulta,
		EspecialidadeID: "esp-1", MedicoID: "med-1", Inicio: nowUTC(), Estado: clinico.EstadoEpisodioAberto,
	})
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-2", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("episódio não-cirúrgico devia falhar com Conflito, veio %v", err)
	}
}

func TestAgendarProcedimento_ConsentimentoDeOutroDoente(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-OUTRO")
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("consentimento de outro doente devia falhar com Validacao, veio %v", err)
	}
}

func TestAgendarProcedimento_CatalogoInactivo(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoCat := novoFakeCatalogo()
	repoCat.porCodigo["PRC999"] = &clinico.CatalogoProcedimento{Codigo: "PRC999", Descricao: "Inactivo", Activo: false}
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, repoCat, &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC999", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("procedimento inactivo no catálogo devia falhar com Validacao, veio %v", err)
	}
}

func TestAgendarProcedimento_EpisodioNaoEncontrado(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "inexistente", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestAgendarProcedimento_EpisodioNaoAberto(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	fim := nowUTC().Add(time.Hour)
	repoE.porID["ep-1"] = clinico.ReconstruirEpisodio(clinico.SnapshotEpisodio{
		ID: "ep-1", DoenteID: "doente-1", Tipo: clinico.EpisodioCirurgiaAmbulatoria,
		EspecialidadeID: "esp-1", MedicoID: "med-1", Inicio: nowUTC(), Fim: &fim,
		Estado: clinico.EstadoEpisodioFechado,
	})
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("agendar num episódio fechado devia falhar com Conflito, veio %v", err)
	}
}

func TestAgendarProcedimento_CatalogoNaoEncontrado(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "INEXISTENTE", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestAgendarProcedimento_ConsentimentoNaoEncontrado(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: "inexistente",
	})
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestAgendarProcedimento_AnestesiaInvalida(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "DESCONHECIDA", ConsentimentoID: consID,
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("anestesia inválida devia falhar com Validacao, obtive %v", err)
	}
}

func TestAgendarProcedimento_GuardarFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	repoP.guardarErr = errSimulado
	uc := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestAgendarProcedimento_AuditorFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	aud := &fakeAuditor{err: errSimulado}
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, novoFakeCatalogo(), aud)

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestAgendarProcedimento_ReleituraFinalFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	repoP.obterErr = errSimulado
	repoP.obterErrNaChamada = 1 // o agendamento não lê procedimentos antes de guardar: a única ObterPorID é a releitura final.
	uc := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}

func TestAgendarProcedimento_RequerAnestesistaSemAnestesista(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoCat := novoFakeCatalogo()
	repoCat.porCodigo["PRC777"] = &clinico.CatalogoProcedimento{Codigo: "PRC777", Descricao: "Exige anestesista", Activo: true, RequerAnestesista: true}
	uc := app.NovoCasoAgendarProcedimento(novoFakeProcedimentos(), repoE, repoC, repoCat, &fakeAuditor{})

	_, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC777", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "GERAL", ConsentimentoID: consID,
	})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("procedimento que exige anestesista sem anestesista devia falhar com Validacao, veio %v", err)
	}
}

func TestProcedimento_CicloCompleto(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}

	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	if det.Estado != "AGENDADO" {
		t.Fatalf("esperado AGENDADO, veio %s", det.Estado)
	}

	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}

	concluir := app.NovoCasoConcluirProcedimento(repoP, aud)
	fim, err := concluir.Executar(context.Background(), "actor-1", det.ID, app.DadosConcluirProcedimento{Complicacoes: "nenhuma"})
	if err != nil {
		t.Fatalf("concluir devia funcionar: %v", err)
	}
	if fim.Estado != "CONCLUIDO" {
		t.Fatalf("esperado CONCLUIDO, veio %s", fim.Estado)
	}
	esperadas := []string{"clinico.procedimento.agendado", "clinico.procedimento.iniciado", "clinico.procedimento.concluido"}
	if len(aud.registos) != len(esperadas) {
		t.Fatalf("auditoria esperada %v, veio %v", esperadas, aud.registos)
	}
	for i, a := range esperadas {
		if aud.registos[i].Accao != a {
			t.Fatalf("auditoria esperada %v, veio %v", esperadas, aud.registos)
		}
	}
}

func TestIniciarProcedimento_ProcedimentoNaoEncontrado(t *testing.T) {
	uc := app.NovoCasoIniciarProcedimento(novoFakeProcedimentos(), novoFakeRepoEpisodios(), novoFakeConsentimentos(), &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "actor-1", "inexistente")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestIniciarProcedimento_ConsentimentoNaoEncontrado(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	// O consentimento desaparece do repositório (ex.: apagado por engano) antes do início.
	delete(repoC.porID, consID)

	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	_, err = iniciar.Executar(context.Background(), "actor-1", det.ID)
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestIniciarProcedimento_EpisodioNaoEncontrado(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	delete(repoE.porID, "ep-1")

	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	_, err = iniciar.Executar(context.Background(), "actor-1", det.ID)
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestIniciarProcedimento_JaEmCurso(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}
	_, err = iniciar.Executar(context.Background(), "actor-1", det.ID)
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("iniciar duas vezes devia falhar com Conflito, obtive %v", err)
	}
}

func TestIniciarProcedimento_GuardarFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	repoP.guardarErr = errSimulado
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	_, err = iniciar.Executar(context.Background(), "actor-1", det.ID)
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestIniciarProcedimento_AuditorFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	aud := &fakeAuditor{err: errSimulado}
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	_, err = iniciar.Executar(context.Background(), "actor-1", det.ID)
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestIniciarProcedimento_ReleituraFinalFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	repoP.obterChamadas = 0 // reinicia a contagem: o agendamento já fez uma releitura final.
	repoP.obterErr = errSimulado
	repoP.obterErrNaChamada = 2
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, &fakeAuditor{})
	_, err = iniciar.Executar(context.Background(), "actor-1", det.ID)
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}

func TestConcluirProcedimento_NaoEncontrado(t *testing.T) {
	uc := app.NovoCasoConcluirProcedimento(novoFakeProcedimentos(), &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "actor-1", "inexistente", app.DadosConcluirProcedimento{})
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestConcluirProcedimento_SemIniciar(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	// Ainda AGENDADO — nunca foi iniciado.
	concluir := app.NovoCasoConcluirProcedimento(repoP, aud)
	_, err = concluir.Executar(context.Background(), "actor-1", det.ID, app.DadosConcluirProcedimento{})
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("concluir um procedimento agendado (nunca iniciado) devia falhar com Conflito, obtive %v", err)
	}
}

func TestConcluirProcedimento_GuardarFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}
	repoP.guardarErr = errSimulado
	concluir := app.NovoCasoConcluirProcedimento(repoP, aud)
	_, err = concluir.Executar(context.Background(), "actor-1", det.ID, app.DadosConcluirProcedimento{})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestConcluirProcedimento_AuditorFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, &fakeAuditor{})
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}
	aud := &fakeAuditor{err: errSimulado}
	concluir := app.NovoCasoConcluirProcedimento(repoP, aud)
	_, err = concluir.Executar(context.Background(), "actor-1", det.ID, app.DadosConcluirProcedimento{})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestConcluirProcedimento_ReleituraFinalFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, &fakeAuditor{})
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}
	repoP.obterChamadas = 0 // reinicia a contagem: agendar+iniciar já fizeram releituras.
	repoP.obterErr = errSimulado
	repoP.obterErrNaChamada = 2
	concluir := app.NovoCasoConcluirProcedimento(repoP, &fakeAuditor{})
	_, err = concluir.Executar(context.Background(), "actor-1", det.ID, app.DadosConcluirProcedimento{})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}

func TestCancelarProcedimento_NaoEncontrado(t *testing.T) {
	uc := app.NovoCasoCancelarProcedimento(novoFakeProcedimentos(), &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "actor-1", "inexistente", "motivo")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestCancelarProcedimento_AindaAgendado(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	cancelar := app.NovoCasoCancelarProcedimento(repoP, aud)
	_, err = cancelar.Executar(context.Background(), "actor-1", det.ID, "motivo")
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("cancelar um procedimento agendado (nunca iniciado) devia falhar com Conflito, obtive %v", err)
	}
}

func TestCancelarProcedimento_GuardarFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}
	repoP.guardarErr = errSimulado
	cancelar := app.NovoCasoCancelarProcedimento(repoP, aud)
	_, err = cancelar.Executar(context.Background(), "actor-1", det.ID, "motivo")
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestCancelarProcedimento_AuditorFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, &fakeAuditor{})
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}
	aud := &fakeAuditor{err: errSimulado}
	cancelar := app.NovoCasoCancelarProcedimento(repoP, aud)
	_, err = cancelar.Executar(context.Background(), "actor-1", det.ID, "motivo")
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestCancelarProcedimento_ReleituraFinalFalha(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, &fakeAuditor{})
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}
	repoP.obterChamadas = 0 // reinicia a contagem: agendar+iniciar já fizeram releituras.
	repoP.obterErr = errSimulado
	repoP.obterErrNaChamada = 2
	cancelar := app.NovoCasoCancelarProcedimento(repoP, &fakeAuditor{})
	_, err = cancelar.Executar(context.Background(), "actor-1", det.ID, "motivo")
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}

func TestCancelarProcedimento_LevaMotivoNoDetalhe(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}

	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}

	cancelar := app.NovoCasoCancelarProcedimento(repoP, aud)
	out, err := cancelar.Executar(context.Background(), "actor-1", det.ID, "hemorragia inesperada")
	if err != nil {
		t.Fatalf("cancelar devia funcionar: %v", err)
	}
	if out.Estado != "CANCELADO" {
		t.Fatalf("esperado CANCELADO, veio %s", out.Estado)
	}
	ultimo := aud.registos[len(aud.registos)-1]
	if ultimo.Accao != "clinico.procedimento.cancelado" || ultimo.Detalhe != "hemorragia inesperada" {
		t.Fatalf("esperado registo de cancelamento com motivo no Detalhe, veio %+v", ultimo)
	}
}

func TestCancelarProcedimento_MotivoVazio_PropagaValidacao(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}

	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}
	registosAntes := len(aud.registos)

	cancelar := app.NovoCasoCancelarProcedimento(repoP, aud)
	_, err = cancelar.Executar(context.Background(), "actor-1", det.ID, "   ")
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("cancelar com motivo vazio devia propagar Validacao do domínio, veio %v", err)
	}
	// O caso de uso não deve auditar nem persistir uma tentativa falhada.
	if len(aud.registos) != registosAntes {
		t.Fatalf("cancelamento falhado não devia auditar, veio %v", aud.registos)
	}
}

// TestAgendarProcedimento_GravaCodigoCanonicoDoCatalogo prova que o código
// gravado no agregado é o canónico do catálogo (PRC001) e não o que o cliente
// enviou (prc001): o catálogo normaliza a pesquisa, logo o pedido em minúsculas
// é aceite — e sem esta correcção a linha ficaria persistida em minúsculas, sem
// FK que a apanhasse.
func TestAgendarProcedimento_GravaCodigoCanonicoDoCatalogo(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	uc := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), &fakeAuditor{})

	det, err := uc.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "  prc001 ", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar com código em minúsculas devia funcionar: %v", err)
	}
	if det.Codigo != "PRC001" {
		t.Fatalf("esperado o código canónico do catálogo (PRC001) no detalhe, veio %q", det.Codigo)
	}
	persistido, err := repoP.ObterPorID(context.Background(), det.ID)
	if err != nil {
		t.Fatalf("obter procedimento persistido: %v", err)
	}
	if c := persistido.Snapshot().Codigo; c != "PRC001" {
		t.Fatalf("esperado PRC001 persistido, veio %q", c)
	}
}

// TestIniciarProcedimento_ConsentimentoRevogado prova a revalidação da
// invariante-estrela no início: o doente revoga o consentimento (direito LPDP,
// que nunca é bloqueado) depois de a cirurgia estar agendada, e o início passa a
// ser recusado com RegraNegocio (422). Antes da correcção, iniciava-se e
// concluía-se uma cirurgia sobre um consentimento revogado.
func TestIniciarProcedimento_ConsentimentoRevogado(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}

	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	// O doente exerce o direito de revogar — a revogação não é (nem deve ser) bloqueada.
	revogar := app.NovoCasoRevogarConsentimento(repoC, aud)
	if _, err := revogar.Executar(context.Background(), "actor-1", consID); err != nil {
		t.Fatalf("revogar devia funcionar (direito LPDP): %v", err)
	}
	registosAntes := len(aud.registos)

	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	_, err = iniciar.Executar(context.Background(), "actor-1", det.ID)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("iniciar com consentimento revogado devia falhar com RegraNegocio, veio %v", err)
	}
	if len(aud.registos) != registosAntes {
		t.Fatalf("um início recusado não devia auditar, veio %v", aud.registos)
	}
	depois, err := repoP.ObterPorID(context.Background(), det.ID)
	if err != nil {
		t.Fatalf("obter procedimento: %v", err)
	}
	if depois.Estado() != clinico.ProcAgendado {
		t.Fatalf("o procedimento devia continuar AGENDADO, veio %s", depois.Estado())
	}
}

// TestIniciarProcedimento_EpisodioFechado prova que não se inicia uma cirurgia
// num episódio já fechado (Conflito/409) — antes da correcção, o procedimento
// iniciava-se depois do `fechado_em` do episódio.
func TestIniciarProcedimento_EpisodioFechado(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}

	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	// O episódio é fechado depois do agendamento.
	fim := nowUTC().Add(time.Hour)
	repoE.porID["ep-1"] = clinico.ReconstruirEpisodio(clinico.SnapshotEpisodio{
		ID: "ep-1", DoenteID: "doente-1", Tipo: clinico.EpisodioCirurgiaAmbulatoria,
		EspecialidadeID: "esp-1", MedicoID: "med-1", Inicio: nowUTC(), Fim: &fim,
		Estado: clinico.EstadoEpisodioFechado,
	})

	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	_, err = iniciar.Executar(context.Background(), "actor-1", det.ID)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("iniciar num episódio fechado devia falhar com Conflito, veio %v", err)
	}
}

// TestConcluirProcedimento_ConsentimentoRevogadoNaoBloqueia fixa a decisão
// deliberada da ADR-030: revogar o consentimento depois de a cirurgia ter
// começado não impede a conclusão — o acto já ocorreu e o registo clínico do que
// se fez tem de poder ser completado. O bloqueio é no início, não na conclusão.
func TestConcluirProcedimento_ConsentimentoRevogadoNaoBloqueia(t *testing.T) {
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	repoP := novoFakeProcedimentos()
	aud := &fakeAuditor{}

	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	iniciar := app.NovoCasoIniciarProcedimento(repoP, repoE, repoC, aud)
	if _, err := iniciar.Executar(context.Background(), "actor-1", det.ID); err != nil {
		t.Fatalf("iniciar devia funcionar: %v", err)
	}
	revogar := app.NovoCasoRevogarConsentimento(repoC, aud)
	if _, err := revogar.Executar(context.Background(), "actor-1", consID); err != nil {
		t.Fatalf("revogar devia funcionar: %v", err)
	}

	concluir := app.NovoCasoConcluirProcedimento(repoP, aud)
	out, err := concluir.Executar(context.Background(), "actor-1", det.ID,
		app.DadosConcluirProcedimento{Complicacoes: "nenhuma"})
	if err != nil {
		t.Fatalf("concluir depois de revogação não devia ser bloqueado: %v", err)
	}
	if out.Estado != "CONCLUIDO" {
		t.Fatalf("esperado CONCLUIDO, veio %s", out.Estado)
	}
}

func TestObterProcedimento_NaoAudita(t *testing.T) {
	repoP := novoFakeProcedimentos()
	repoE := novoFakeRepoEpisodios()
	repoE.porID["ep-1"] = episodioCirurgico("doente-1")
	repoC := novoFakeConsentimentos()
	consID := consentimentoGuardado(t, repoC, "doente-1")
	aud := &fakeAuditor{}
	agendar := app.NovoCasoAgendarProcedimento(repoP, repoE, repoC, novoFakeCatalogo(), aud)
	det, err := agendar.Executar(context.Background(), "actor-1", app.DadosAgendarProcedimento{
		EpisodioID: "ep-1", Codigo: "PRC001", Descricao: "Sutura", CirurgiaoID: "cir-1",
		Anestesia: "NENHUMA", ConsentimentoID: consID,
	})
	if err != nil {
		t.Fatalf("agendar devia funcionar: %v", err)
	}
	registosAntes := len(aud.registos)

	obter := app.NovoCasoObterProcedimento(repoP)
	out, err := obter.Executar(context.Background(), det.ID)
	if err != nil {
		t.Fatalf("obter devia funcionar: %v", err)
	}
	if out.ID != det.ID {
		t.Fatalf("esperado detalhe com id %s, veio %+v", det.ID, out)
	}
	if len(aud.registos) != registosAntes {
		t.Fatalf("obter não devia auditar, veio %v", aud.registos)
	}
}

func TestListarProcedimentos_NaoAudita(t *testing.T) {
	repoP := novoFakeProcedimentos()
	repoP.lista = []clinico.ResumoProcedimento{{ID: "proc-1", EpisodioID: "ep-1"}}
	uc := app.NovoCasoListarProcedimentos(repoP)

	out, err := uc.Executar(context.Background(), "ep-1")
	if err != nil {
		t.Fatalf("listar devia funcionar: %v", err)
	}
	if len(out) != 1 || out[0].ID != "proc-1" {
		t.Fatalf("esperada lista com o resumo preparado, veio %+v", out)
	}
}
