package farmacia_test

import (
	"context"
	"testing"
	"time"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRegistarFornecedor(t *testing.T) {
	repo := novoFakeRepoFornecedores()
	aud := &fakeAuditor{}
	out, err := appfarmacia.NovoCasoRegistarFornecedor(repo, aud).Executar(context.Background(), "farm-1", appfarmacia.DadosNovoFornecedor{Nome: "Farmédica"})
	if err != nil {
		t.Fatalf("registar: %v", err)
	}
	if out.ID == "" || out.Nome != "Farmédica" {
		t.Fatalf("saída inesperada: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.fornecedor.registado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func dadosEntrada() appfarmacia.DadosEntradaStock {
	return appfarmacia.DadosEntradaStock{
		MedicamentoID: "med-1", NumeroLote: "L001", Validade: time.Now().AddDate(1, 0, 0),
		Quantidade: 100, PrecoUnitarioCusto: "12.5000",
	}
}

func TestRegistarEntradaStock(t *testing.T) {
	repoLotes := novoFakeRepoLotes()
	repoMed := novoFakeRepoMed()
	medID, _ := repoMed.Guardar(context.Background(), medicamentoParaRepo(t))
	repoForn := novoFakeRepoFornecedores()
	aud := &fakeAuditor{}
	caso := appfarmacia.NovoCasoRegistarEntradaStock(repoLotes, repoMed, repoForn, aud)

	dados := dadosEntrada()
	dados.MedicamentoID = medID
	out, err := caso.Executar(context.Background(), "farm-1", dados)
	if err != nil {
		t.Fatalf("entrada: %v", err)
	}
	if out.ID == "" || out.QuantidadeActual != 100 {
		t.Fatalf("saída inesperada: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.stock.entrada" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestRegistarEntradaStock_MedicamentoInexistente(t *testing.T) {
	caso := appfarmacia.NovoCasoRegistarEntradaStock(novoFakeRepoLotes(), novoFakeRepoMed(), novoFakeRepoFornecedores(), &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "farm-1", dadosEntrada()); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, obtive %v", err)
	}
}

func TestConsultarStock(t *testing.T) {
	repoLotes := novoFakeRepoLotes()
	repoLotes.stock = 250
	out, err := appfarmacia.NovoCasoConsultarStock(repoLotes).Executar(context.Background(), "med-1")
	if err != nil || out.Disponivel != 250 {
		t.Fatalf("stock inesperado: %+v, %v", out, err)
	}
}
