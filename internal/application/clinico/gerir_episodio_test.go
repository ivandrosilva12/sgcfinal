package clinico_test

import (
	"context"
	"errors"
	"testing"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// iniciarNoRepo cria um episódio ABERTO no fake e devolve o seu id.
func iniciarNoRepo(t *testing.T, repoEp *fakeRepoEpisodios, repoDoentes *fakeRepo) string {
	t.Helper()
	doenteID := registarNoRepo(t, repoDoentes)
	caso := appclinico.NovoCasoIniciarEpisodio(repoEp, repoDoentes, &fakeAuditor{})
	out, err := caso.Executar(context.Background(), "medico-1", dadosEpisodioBase(doenteID))
	if err != nil {
		t.Fatalf("preparar episódio: %v", err)
	}
	return out.ID
}

func TestActualizarEpisodio_NotaEDiagnosticos(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoActualizarEpisodio(repoEp, aud)

	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre", ExameObjectivo: "Temp 39", Diagnostico: "Gripe", Plano: "Repouso"}
	cids := &[]appclinico.DadosDiagnosticoCID{{CID: "J11", Principal: true}}
	out, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: nota, DiagnosticosCID: cids})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.Nota.Diagnostico != "Gripe" || len(out.DiagnosticosCID) != 1 {
		t.Fatalf("actualização não reflectida: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.episodio.actualizado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestActualizarEpisodio_NaoEncontrado(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	caso := appclinico.NovoCasoActualizarEpisodio(repoEp, &fakeAuditor{})
	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre"}
	_, err := caso.Executar(context.Background(), "medico-1", "inexistente", appclinico.DadosActualizarEpisodio{Nota: nota})
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestActualizarEpisodio_DiagnosticoComDoisPrincipais(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	caso := appclinico.NovoCasoActualizarEpisodio(repoEp, &fakeAuditor{})
	cids := &[]appclinico.DadosDiagnosticoCID{{CID: "J11", Principal: true}, {CID: "J12", Principal: true}}
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{DiagnosticosCID: cids})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("dois diagnósticos principais devia falhar com Validacao, obtive %v", err)
	}
}

func TestActualizarEpisodio_EpisodioFechado(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	repoDoentes := novoFakeRepo()
	id := iniciarNoRepo(t, repoEp, repoDoentes)
	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre", ExameObjectivo: "Temp 39", Diagnostico: "Gripe", Plano: "Repouso"}
	cids := &[]appclinico.DadosDiagnosticoCID{{CID: "J11", Principal: true}}
	if _, err := appclinico.NovoCasoActualizarEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: nota, DiagnosticosCID: cids}); err != nil {
		t.Fatalf("preparar nota/diagnóstico: %v", err)
	}
	if _, err := appclinico.NovoCasoFecharEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", id); err != nil {
		t.Fatalf("fechar: %v", err)
	}

	caso := appclinico.NovoCasoActualizarEpisodio(repoEp, &fakeAuditor{})
	novaNota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Tosse"}
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: novaNota})
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("actualizar episódio fechado devia falhar com Conflito, obtive %v", err)
	}
}

func TestActualizarEpisodio_GuardarFalha(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	repoEp.guardarErr = errSimulado
	caso := appclinico.NovoCasoActualizarEpisodio(repoEp, &fakeAuditor{})
	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre"}
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: nota})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestActualizarEpisodio_AuditorFalha(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	aud := &fakeAuditor{err: errSimulado}
	caso := appclinico.NovoCasoActualizarEpisodio(repoEp, aud)
	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre"}
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: nota})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestActualizarEpisodio_ReleituraFinalFalha(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	repoEp.obterChamadas = 0 // reinicia a contagem: iniciarNoRepo já fez uma leitura.
	repoEp.obterErr = errSimulado
	repoEp.obterErrNaChamada = 2
	caso := appclinico.NovoCasoActualizarEpisodio(repoEp, &fakeAuditor{})
	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre"}
	_, err := caso.Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: nota})
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}

func TestFecharEpisodio_SemNota_Erro(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	caso := appclinico.NovoCasoFecharEpisodio(repoEp, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", id)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação (nota incompleta), obtive %v", err)
	}
}

func TestFecharEpisodio_Completo(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	repoDoentes := novoFakeRepo()
	id := iniciarNoRepo(t, repoEp, repoDoentes)
	// Preenche nota + CID.
	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre", ExameObjectivo: "Temp 39", Diagnostico: "Gripe", Plano: "Repouso"}
	cids := &[]appclinico.DadosDiagnosticoCID{{CID: "J11", Principal: true}}
	_, _ = appclinico.NovoCasoActualizarEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: nota, DiagnosticosCID: cids})

	aud := &fakeAuditor{}
	out, err := appclinico.NovoCasoFecharEpisodio(repoEp, aud).Executar(context.Background(), "medico-1", id)
	if err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if out.Estado != "FECHADO" || out.FechadoPor != "medico-1" {
		t.Fatalf("fecho inesperado: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.episodio.fechado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestCancelarEpisodio(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	aud := &fakeAuditor{}
	out, err := appclinico.NovoCasoCancelarEpisodio(repoEp, aud).Executar(context.Background(), "medico-1", id, "duplicado")
	if err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	if out.Estado != "CANCELADO" {
		t.Fatalf("estado=%q, esperava CANCELADO", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.episodio.cancelado" || aud.registos[0].Detalhe == "" {
		t.Fatalf("auditoria em falta ou sem motivo: %+v", aud.registos)
	}
}

func TestFecharEpisodio_NaoEncontrado(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	_, err := appclinico.NovoCasoFecharEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", "inexistente")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestFecharEpisodio_GuardarFalha(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	repoDoentes := novoFakeRepo()
	id := iniciarNoRepo(t, repoEp, repoDoentes)
	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre", ExameObjectivo: "Temp 39", Diagnostico: "Gripe", Plano: "Repouso"}
	cids := &[]appclinico.DadosDiagnosticoCID{{CID: "J11", Principal: true}}
	if _, err := appclinico.NovoCasoActualizarEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: nota, DiagnosticosCID: cids}); err != nil {
		t.Fatalf("preparar nota/diagnóstico: %v", err)
	}
	repoEp.guardarErr = errSimulado
	_, err := appclinico.NovoCasoFecharEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", id)
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestFecharEpisodio_AuditorFalha(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	repoDoentes := novoFakeRepo()
	id := iniciarNoRepo(t, repoEp, repoDoentes)
	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre", ExameObjectivo: "Temp 39", Diagnostico: "Gripe", Plano: "Repouso"}
	cids := &[]appclinico.DadosDiagnosticoCID{{CID: "J11", Principal: true}}
	if _, err := appclinico.NovoCasoActualizarEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: nota, DiagnosticosCID: cids}); err != nil {
		t.Fatalf("preparar nota/diagnóstico: %v", err)
	}
	aud := &fakeAuditor{err: errSimulado}
	_, err := appclinico.NovoCasoFecharEpisodio(repoEp, aud).Executar(context.Background(), "medico-1", id)
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestFecharEpisodio_ReleituraFinalFalha(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	repoDoentes := novoFakeRepo()
	id := iniciarNoRepo(t, repoEp, repoDoentes)
	nota := &appclinico.DadosNotaClinica{QueixaPrincipal: "Febre", ExameObjectivo: "Temp 39", Diagnostico: "Gripe", Plano: "Repouso"}
	cids := &[]appclinico.DadosDiagnosticoCID{{CID: "J11", Principal: true}}
	if _, err := appclinico.NovoCasoActualizarEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", id, appclinico.DadosActualizarEpisodio{Nota: nota, DiagnosticosCID: cids}); err != nil {
		t.Fatalf("preparar nota/diagnóstico: %v", err)
	}
	repoEp.obterChamadas = 0 // reinicia a contagem: as chamadas de preparação já leram/escreveram.
	repoEp.obterErr = errSimulado
	repoEp.obterErrNaChamada = 2
	_, err := appclinico.NovoCasoFecharEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", id)
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}

func TestCancelarEpisodio_NaoEncontrado(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	_, err := appclinico.NovoCasoCancelarEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", "inexistente", "motivo")
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestCancelarEpisodio_JaCancelado(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	aud := &fakeAuditor{}
	if _, err := appclinico.NovoCasoCancelarEpisodio(repoEp, aud).Executar(context.Background(), "medico-1", id, "primeiro cancelamento"); err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	_, err := appclinico.NovoCasoCancelarEpisodio(repoEp, aud).Executar(context.Background(), "medico-1", id, "segundo cancelamento")
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("cancelar um episódio já cancelado devia falhar com Conflito, obtive %v", err)
	}
}

func TestCancelarEpisodio_GuardarFalha(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	repoEp.guardarErr = errSimulado
	_, err := appclinico.NovoCasoCancelarEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", id, "motivo")
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro de Guardar, obtive %v", err)
	}
}

func TestCancelarEpisodio_AuditorFalha(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	aud := &fakeAuditor{err: errSimulado}
	_, err := appclinico.NovoCasoCancelarEpisodio(repoEp, aud).Executar(context.Background(), "medico-1", id, "motivo")
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro do auditor, obtive %v", err)
	}
	if len(aud.registos) != 0 {
		t.Fatalf("auditor falhado não devia ter registos: %+v", aud.registos)
	}
}

func TestCancelarEpisodio_ReleituraFinalFalha(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	id := iniciarNoRepo(t, repoEp, novoFakeRepo())
	repoEp.obterChamadas = 0 // reinicia a contagem: iniciarNoRepo já fez uma leitura.
	repoEp.obterErr = errSimulado
	repoEp.obterErrNaChamada = 2
	_, err := appclinico.NovoCasoCancelarEpisodio(repoEp, &fakeAuditor{}).Executar(context.Background(), "medico-1", id, "motivo")
	if !errors.Is(err, errSimulado) {
		t.Fatalf("esperava a propagação do erro da releitura final, obtive %v", err)
	}
}
