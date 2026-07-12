package clinico_test

import (
	"context"
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
