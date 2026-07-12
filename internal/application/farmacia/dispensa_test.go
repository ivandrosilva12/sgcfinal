package farmacia_test

import (
	"context"
	"testing"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeMotorDispensa simula a persistência transaccional: se tiver um repo,
// guarda nele a receita recebida (como o motor real faria), para que o re-ler do
// caso de uso reflicta o novo estado. Devolve um erro configurável.
type fakeMotorDispensa struct {
	err  error
	repo *fakeRepoReceitas
}

func (m *fakeMotorDispensa) Dispensar(_ context.Context, receita dominio.SnapshotReceita, _ []appfarmacia.ItemDispensa, _ string) ([]dominio.AlocacaoFEFO, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.repo != nil {
		m.repo.porID[receita.ID] = dominio.ReconstruirReceita(receita)
	}
	return []dominio.AlocacaoFEFO{}, nil
}

// prepararDispensa emite uma receita (via o fluxo do Sprint 9) e devolve o que é
// preciso para dispensar. Reutiliza prepararEmissao/dadosReceita de receitas_test.go.
func prepararDispensa(t *testing.T) (*fakeRepoReceitas, *fakeRepoMed, *fakeLeitorClinico, string, string) {
	t.Helper()
	repoRec, repoMed, leitor, medID := prepararEmissao(t)
	emitida, err := appfarmacia.NovoCasoEmitirReceita(repoRec, repoMed, leitor, &fakeAuditor{}).Executar(context.Background(), "medico-1", dadosReceita(medID))
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	return repoRec, repoMed, leitor, emitida.ID, medID
}

func dispensa(medID string, qtd int) appfarmacia.DadosDispensa {
	return appfarmacia.DadosDispensa{Itens: []appfarmacia.ItemDispensaDTO{{MedicamentoID: medID, Quantidade: qtd}}}
}

func TestDispensar_Parcial(t *testing.T) {
	repoRec, repoMed, leitor, recID, medID := prepararDispensa(t)
	aud := &fakeAuditor{}
	// O motor persiste o snapshot no repo, para o re-ler reflectir PARCIAL.
	caso := appfarmacia.NovoCasoDispensarReceita(repoRec, repoMed, leitor, &fakeMotorDispensa{repo: repoRec}, aud)
	out, err := caso.Executar(context.Background(), "farm-1", recID, dispensa(medID, 5)) // prescrita=20
	if err != nil {
		t.Fatalf("dispensar: %v", err)
	}
	if out.Estado != "PARCIAL" {
		t.Fatalf("estado=%q, esperava PARCIAL", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.receita.dispensada" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestDispensar_Excede(t *testing.T) {
	repoRec, repoMed, leitor, recID, medID := prepararDispensa(t)
	caso := appfarmacia.NovoCasoDispensarReceita(repoRec, repoMed, leitor, &fakeMotorDispensa{}, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "farm-1", recID, dispensa(medID, 25)); erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava RegraNegocio (excede prescrito), obtive %v", err)
	}
}

func TestDispensar_AlergiaBloqueia(t *testing.T) {
	repoRec, repoMed, leitor, recID, medID := prepararDispensa(t)
	leitor.alergias = []appfarmacia.AlergiaClinica{{Substancia: "Amoxicilina", Severidade: "ANAFILACTICA"}}
	caso := appfarmacia.NovoCasoDispensarReceita(repoRec, repoMed, leitor, &fakeMotorDispensa{}, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "farm-1", recID, dispensa(medID, 5)); erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava RegraNegocio (alergia), obtive %v", err)
	}
}

func TestDispensar_OverrideComJustificacao(t *testing.T) {
	repoRec, repoMed, leitor, recID, medID := prepararDispensa(t)
	leitor.alergias = []appfarmacia.AlergiaClinica{{Substancia: "Amoxicilina", Severidade: "GRAVE"}}
	d := dispensa(medID, 5)
	d.IgnorarAlertaAlergia = true
	d.JustificacaoAlerta = "Doente monitorizado."
	aud := &fakeAuditor{}
	if _, err := appfarmacia.NovoCasoDispensarReceita(repoRec, repoMed, leitor, &fakeMotorDispensa{}, aud).Executar(context.Background(), "farm-1", recID, d); err != nil {
		t.Fatalf("dispensar com override: %v", err)
	}
	if len(aud.registos) != 1 || aud.registos[0].Detalhe == "" {
		t.Fatalf("esperava auditoria com detalhe do override: %+v", aud.registos)
	}
}

func TestDispensar_StockInsuficiente(t *testing.T) {
	repoRec, repoMed, leitor, recID, medID := prepararDispensa(t)
	motor := &fakeMotorDispensa{err: erros.Novo(erros.CategoriaRegraNegocio, "stock insuficiente")}
	caso := appfarmacia.NovoCasoDispensarReceita(repoRec, repoMed, leitor, motor, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "farm-1", recID, dispensa(medID, 5)); erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava RegraNegocio (stock), obtive %v", err)
	}
}
