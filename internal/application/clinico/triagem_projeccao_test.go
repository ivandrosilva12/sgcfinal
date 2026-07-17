package clinico_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeLeitorTriagem serve triagens por episódio e conta invocações — permite
// assertar que papéis não autorizados nem sequer invocam a porta.
type fakeLeitorTriagem struct {
	porEpisodio map[string]appclinico.TriagemDoEpisodio
	err         error
	chamadas    int
}

func (f *fakeLeitorTriagem) TriagemDoEpisodio(_ context.Context, id string) (appclinico.TriagemDoEpisodio, bool, error) {
	f.chamadas++
	if f.err != nil {
		return appclinico.TriagemDoEpisodio{}, false, f.err
	}
	tr, ok := f.porEpisodio[id]
	return tr, ok, nil
}

func (f *fakeLeitorTriagem) PrioridadesDosEpisodios(_ context.Context, ids []string) (map[string]string, error) {
	f.chamadas++
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]string{}
	for _, id := range ids {
		if tr, ok := f.porEpisodio[id]; ok {
			out[id] = tr.Prioridade
		}
	}
	return out, nil
}

func episodioNoRepo(t *testing.T, repo *fakeRepoEpisodios, doenteID string) string {
	t.Helper()
	ep, err := clinico.NovoEpisodio(doenteID, clinico.EpisodioConsulta, "esp-1", "medico-1", time.Now())
	if err != nil {
		t.Fatalf("construir episódio: %v", err)
	}
	id, err := repo.Guardar(context.Background(), ep)
	if err != nil {
		t.Fatalf("guardar episódio: %v", err)
	}
	return id
}

func TestObterEpisodio_MedicoVeTriagem(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	epID := episodioNoRepo(t, repoEp, "doe-1")
	leitor := &fakeLeitorTriagem{porEpisodio: map[string]appclinico.TriagemDoEpisodio{
		epID: {Prioridade: "AMARELO", EnfermeiroID: "enf-1"},
	}}
	caso := appclinico.NovoCasoObterEpisodio(repoEp, leitor, &fakeAuditor{})

	out, err := caso.Executar(context.Background(), "medico-1", []string{"Medico"}, epID)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if out.Triagem == nil || out.Triagem.Prioridade != "AMARELO" {
		t.Fatalf("triagem devia vir preenchida: %+v", out.Triagem)
	}
}

func TestObterEpisodio_FarmaceuticoNaoVeTriagem_LeitorNaoInvocado(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	epID := episodioNoRepo(t, repoEp, "doe-1")
	leitor := &fakeLeitorTriagem{porEpisodio: map[string]appclinico.TriagemDoEpisodio{
		epID: {Prioridade: "AMARELO"},
	}}
	caso := appclinico.NovoCasoObterEpisodio(repoEp, leitor, &fakeAuditor{})

	out, err := caso.Executar(context.Background(), "farm-1", []string{"Farmaceutico"}, epID)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if out.Triagem != nil {
		t.Fatalf("farmacêutico não devia ver a triagem: %+v", out.Triagem)
	}
	if leitor.chamadas != 0 {
		t.Fatal("o leitor de triagem não devia ser invocado sem papel autorizado")
	}
}

func TestObterEpisodio_SemTriagem_OmitidoSemErro(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	epID := episodioNoRepo(t, repoEp, "doe-1")
	caso := appclinico.NovoCasoObterEpisodio(repoEp, &fakeLeitorTriagem{}, &fakeAuditor{})

	out, err := caso.Executar(context.Background(), "medico-1", []string{"Medico"}, epID)
	if err != nil || out.Triagem != nil {
		t.Fatalf("episódio sem triagem devia vir sem bloco e sem erro: %v (%+v)", err, out.Triagem)
	}
}

func TestObterEpisodio_FalhaDoLeitor_Propaga(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	epID := episodioNoRepo(t, repoEp, "doe-1")
	falha := erros.Novo(erros.CategoriaInterno, "recepção indisponível")
	caso := appclinico.NovoCasoObterEpisodio(repoEp, &fakeLeitorTriagem{err: falha}, &fakeAuditor{})

	if _, err := caso.Executar(context.Background(), "medico-1", []string{"Medico"}, epID); !errors.Is(err, falha) {
		t.Fatalf("a falha do leitor devia propagar, veio %v", err)
	}
}

func TestListarEpisodios_LotePreenchePrioridades(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	repoEp.pagina = clinico.PaginaEpisodios{Itens: []clinico.ResumoEpisodio{
		{ID: "ep-1"}, {ID: "ep-2"},
	}, Total: 2}
	leitor := &fakeLeitorTriagem{porEpisodio: map[string]appclinico.TriagemDoEpisodio{
		"ep-1": {Prioridade: "VERMELHO"},
	}}
	caso := appclinico.NovoCasoListarEpisodios(repoEp, leitor)

	pagina, err := caso.Executar(context.Background(), "doe-1", []string{"Enfermeiro"}, appclinico.FiltroEpisodios{})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if pagina.Itens[0].PrioridadeTriagem != "VERMELHO" || pagina.Itens[1].PrioridadeTriagem != "" {
		t.Fatalf("prioridades mal preenchidas: %+v", pagina.Itens)
	}
}

func TestListarEpisodios_SemPapel_LeitorNaoInvocado(t *testing.T) {
	repoEp := novoFakeRepoEpisodios()
	repoEp.pagina = clinico.PaginaEpisodios{Itens: []clinico.ResumoEpisodio{{ID: "ep-1"}}, Total: 1}
	leitor := &fakeLeitorTriagem{porEpisodio: map[string]appclinico.TriagemDoEpisodio{
		"ep-1": {Prioridade: "VERMELHO"},
	}}
	caso := appclinico.NovoCasoListarEpisodios(repoEp, leitor)

	pagina, err := caso.Executar(context.Background(), "doe-1", []string{"TecnicoLab"}, appclinico.FiltroEpisodios{})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if pagina.Itens[0].PrioridadeTriagem != "" || leitor.chamadas != 0 {
		t.Fatalf("sem papel: prioridade devia ficar vazia e o leitor por invocar (%+v, chamadas=%d)", pagina.Itens, leitor.chamadas)
	}
}

func TestObterEHR_LotePreenchePrioridades(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	repoEp.pagina = clinico.PaginaEpisodios{Itens: []clinico.ResumoEpisodio{
		{ID: "ep-1"}, {ID: "ep-2"},
	}, Total: 2}
	leitor := &fakeLeitorTriagem{porEpisodio: map[string]appclinico.TriagemDoEpisodio{
		"ep-2": {Prioridade: "LARANJA"},
	}}
	caso := appclinico.NovoCasoObterEHR(repoDoentes, repoEp, leitor, &fakeAuditor{})

	ehr, err := caso.Executar(context.Background(), "medico-1", []string{"Director"}, doenteID, appclinico.FiltroEpisodios{})
	if err != nil {
		t.Fatalf("ehr: %v", err)
	}
	if ehr.Episodios.Itens[0].PrioridadeTriagem != "" || ehr.Episodios.Itens[1].PrioridadeTriagem != "LARANJA" {
		t.Fatalf("prioridades do EHR mal preenchidas: %+v", ehr.Episodios.Itens)
	}
}

func TestObterEHR_SemPapel_LeitorNaoInvocado(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	repoEp.pagina = clinico.PaginaEpisodios{Itens: []clinico.ResumoEpisodio{
		{ID: "ep-1"}, {ID: "ep-2"},
	}, Total: 2}
	leitor := &fakeLeitorTriagem{porEpisodio: map[string]appclinico.TriagemDoEpisodio{
		"ep-2": {Prioridade: "LARANJA"},
	}}
	caso := appclinico.NovoCasoObterEHR(repoDoentes, repoEp, leitor, &fakeAuditor{})

	ehr, err := caso.Executar(context.Background(), "farm-1", []string{"Farmaceutico"}, doenteID, appclinico.FiltroEpisodios{})
	if err != nil {
		t.Fatalf("ehr: %v", err)
	}
	for _, item := range ehr.Episodios.Itens {
		if item.PrioridadeTriagem != "" {
			t.Fatalf("sem papel autorizado: prioridade devia ficar vazia: %+v", ehr.Episodios.Itens)
		}
	}
	if leitor.chamadas != 0 {
		t.Fatalf("sem papel autorizado: o leitor de triagem não devia ser invocado no EHR, chamadas=%d", leitor.chamadas)
	}
}

func TestObterEHR_FalhaDoLote_Propaga(t *testing.T) {
	repoDoentes := novoFakeRepo()
	doenteID := registarNoRepo(t, repoDoentes)
	repoEp := novoFakeRepoEpisodios()
	repoEp.pagina = clinico.PaginaEpisodios{Itens: []clinico.ResumoEpisodio{{ID: "ep-1"}}, Total: 1}
	falha := erros.Novo(erros.CategoriaInterno, "recepção indisponível")
	caso := appclinico.NovoCasoObterEHR(repoDoentes, repoEp, &fakeLeitorTriagem{err: falha}, &fakeAuditor{})

	if _, err := caso.Executar(context.Background(), "medico-1", []string{"Medico"}, doenteID, appclinico.FiltroEpisodios{}); !errors.Is(err, falha) {
		t.Fatalf("a falha do lote devia propagar, veio %v", err)
	}
}
