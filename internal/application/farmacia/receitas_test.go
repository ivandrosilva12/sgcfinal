package farmacia_test

import (
	"context"
	"strconv"
	"testing"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeRepoReceitas é um repositório de receitas em memória (usado só nesta task).
type fakeRepoReceitas struct {
	porID      map[string]*farmacia.Receita
	seq        int
	pagina     farmacia.PaginaReceitas
	ultimoFilt farmacia.FiltroReceitas
}

func novoFakeRepoReceitas() *fakeRepoReceitas {
	return &fakeRepoReceitas{porID: map[string]*farmacia.Receita{}}
}
func (f *fakeRepoReceitas) Guardar(_ context.Context, r *farmacia.Receita) (string, error) {
	snap := r.Snapshot()
	id := snap.ID
	if id == "" {
		f.seq++
		id = "rec-" + strconv.Itoa(f.seq)
		snap.ID = id
	}
	f.porID[id] = farmacia.ReconstruirReceita(snap)
	return id, nil
}
func (f *fakeRepoReceitas) ObterPorID(_ context.Context, id string) (*farmacia.Receita, error) {
	r, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "receita não encontrada")
	}
	return r, nil
}
func (f *fakeRepoReceitas) ListarPorDoente(_ context.Context, filt farmacia.FiltroReceitas) (farmacia.PaginaReceitas, error) {
	f.ultimoFilt = filt
	return f.pagina, nil
}

// fakeLeitorClinico simula a porta anti-corrupção do BC Clínico.
type fakeLeitorClinico struct {
	activo    bool
	alergias  []appfarmacia.AlergiaClinica
	episodios map[string]string // episodioID -> doenteID
	err       error
}

func (f *fakeLeitorClinico) ObterContextoDoente(_ context.Context, _ string) (bool, []appfarmacia.AlergiaClinica, error) {
	return f.activo, f.alergias, f.err
}
func (f *fakeLeitorClinico) EpisodioDoDoente(_ context.Context, episodioID, doenteID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.episodios[episodioID] == doenteID, nil
}

func prepararEmissao(t *testing.T) (*fakeRepoReceitas, *fakeRepoMed, *fakeLeitorClinico, string) {
	t.Helper()
	repoMed := novoFakeRepoMed()
	medID, _ := repoMed.Guardar(context.Background(), medicamentoParaRepo(t)) // Amoxicilina, activo
	leitor := &fakeLeitorClinico{activo: true, episodios: map[string]string{"ep-1": "doente-1"}}
	return novoFakeRepoReceitas(), repoMed, leitor, medID
}

func dadosReceita(medID string) appfarmacia.DadosNovaReceita {
	return appfarmacia.DadosNovaReceita{
		EpisodioID: "ep-1", DoenteID: "doente-1",
		Itens: []appfarmacia.DadosItemReceita{{MedicamentoID: medID, Posologia: "1 comp 8/8h", QuantidadePrescrita: 20}},
	}
}

func TestEmitirReceita_SemAlergia(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	aud := &fakeAuditor{}
	caso := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, aud)
	out, err := caso.Executar(context.Background(), "medico-1", dadosReceita(medID))
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if out.ID == "" || out.MedicoID != "medico-1" || out.Estado != "EMITIDA" {
		t.Fatalf("saída inesperada: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.receita.emitida" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestEmitirReceita_AlergiaBloqueia(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	leitor.alergias = []appfarmacia.AlergiaClinica{{Substancia: "Amoxicilina", Severidade: "ANAFILACTICA"}}
	caso := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), "medico-1", dadosReceita(medID))
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava RegraNegocio (422), obtive %v", err)
	}
}

func TestEmitirReceita_OverrideSemJustificacao(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	leitor.alergias = []appfarmacia.AlergiaClinica{{Substancia: "Amoxicilina", Severidade: "GRAVE"}}
	dados := dadosReceita(medID)
	dados.IgnorarAlertaAlergia = true // sem justificação
	caso := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "medico-1", dados); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação (falta justificação), obtive %v", err)
	}
}

func TestEmitirReceita_OverrideComJustificacao(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	leitor.alergias = []appfarmacia.AlergiaClinica{{Substancia: "Amoxicilina", Severidade: "GRAVE"}}
	dados := dadosReceita(medID)
	dados.IgnorarAlertaAlergia = true
	dados.JustificacaoAlerta = "Benefício supera o risco; doente monitorizado."
	aud := &fakeAuditor{}
	out, err := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, aud).Executar(context.Background(), "medico-1", dados)
	if err != nil {
		t.Fatalf("emitir com override: %v", err)
	}
	if out.ID == "" {
		t.Fatal("esperava receita emitida com override")
	}
	if len(aud.registos) != 1 || aud.registos[0].Detalhe == "" {
		t.Fatalf("esperava auditoria com detalhe do override: %+v", aud.registos)
	}
}

func TestEmitirReceita_DoenteInactivo(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	leitor.activo = false
	caso := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "medico-1", dadosReceita(medID)); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito (doente inactivo), obtive %v", err)
	}
}

func TestEmitirReceita_EpisodioDeOutroDoente(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	leitor.episodios = map[string]string{"ep-1": "outro-doente"}
	caso := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "medico-1", dadosReceita(medID)); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação (episódio de outro doente), obtive %v", err)
	}
}

func TestAnularReceita(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	emitida, _ := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{}).Executar(context.Background(), "medico-1", dadosReceita(medID))
	aud := &fakeAuditor{}
	out, err := appfarmacia.NovoCasoAnularReceita(repoRec, aud).Executar(context.Background(), "medico-1", emitida.ID, "erro de prescrição")
	if err != nil {
		t.Fatalf("anular: %v", err)
	}
	if out.Estado != "ANULADA" {
		t.Fatalf("estado=%q, esperava ANULADA", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.receita.anulada" || aud.registos[0].Detalhe == "" {
		t.Fatalf("auditoria em falta ou sem motivo: %+v", aud.registos)
	}
}

func TestObterReceita_Audita(t *testing.T) {
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	emitida, _ := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{}).Executar(context.Background(), "medico-1", dadosReceita(medID))
	aud := &fakeAuditor{}
	if _, err := appfarmacia.NovoCasoObterReceita(repoRec, aud).Executar(context.Background(), "medico-1", emitida.ID); err != nil {
		t.Fatalf("obter: %v", err)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.receita.consultada" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}
