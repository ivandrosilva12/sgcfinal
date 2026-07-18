package financeiro_test

import (
	"context"
	"testing"
	"time"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/financeiro"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestCriarFacturaAuditada(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	uc := app.NovoCasoCriarFactura(repo, aud)
	out, err := uc.Executar(context.Background(), "u-1", app.DadosNovaFactura{
		EpisodioID: "11111111-1111-1111-1111-111111111111", ClienteNome: "Clínica Sol",
	})
	if err != nil {
		t.Fatalf("criar: %v", err)
	}
	if out.ID == "" || out.Estado != "RASCUNHO" {
		t.Errorf("factura mal criada: %+v", out)
	}
	if !aud.tem("financeiro.factura.criada") {
		t.Error("criação devia ser auditada")
	}
}

func TestAdicionarERemoverLinha(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	criar := app.NovoCasoCriarFactura(repo, aud)
	f, _ := criar.Executar(context.Background(), "u-1", app.DadosNovaFactura{
		EpisodioID: "11111111-1111-1111-1111-111111111111", ClienteNome: "Sol",
	})

	adicionar := app.NovoCasoAdicionarItem(repo, aud)
	det, err := adicionar.Executar(context.Background(), "u-1", app.DadosNovoItem{
		FacturaID: f.ID, Descricao: "Medicamento", Tipo: "DISPENSA",
		OperacaoID: "22222222-2222-2222-2222-222222222222", Quantidade: 2,
		PrecoUnitarioCentimos: 100000, RegimeIVA: "STANDARD",
	})
	if err != nil {
		t.Fatalf("adicionar: %v", err)
	}
	if len(det.Itens) != 1 || det.TotalCentimos != 228000 {
		t.Errorf("linha/total errados: %+v", det)
	}
	if !aud.tem("financeiro.factura.item.adicionado") {
		t.Error("adição de linha devia ser auditada")
	}

	remover := app.NovoCasoRemoverItem(repo, aud)
	det2, err := remover.Executar(context.Background(), "u-1", f.ID, det.Itens[0].ID)
	if err != nil {
		t.Fatalf("remover: %v", err)
	}
	if len(det2.Itens) != 0 {
		t.Error("linha devia ter sido removida")
	}
	if !aud.tem("financeiro.factura.item.removido") {
		t.Error("remoção de linha devia ser auditada")
	}
}

func TestListarPorEpisodio(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	criar := app.NovoCasoCriarFactura(repo, aud)
	_, _ = criar.Executar(context.Background(), "u-1", app.DadosNovaFactura{
		EpisodioID: "ep-x", ClienteNome: "Sol",
	})
	listar := app.NovoCasoListarFacturasPorEpisodio(repo)
	res, err := listar.Executar(context.Background(), "ep-x")
	if err != nil || len(res) != 1 {
		t.Fatalf("listar: err=%v n=%d", err, len(res))
	}
}

func TestObterFactura(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	criar := app.NovoCasoCriarFactura(repo, aud)
	f, err := criar.Executar(context.Background(), "u-1", app.DadosNovaFactura{
		EpisodioID: "11111111-1111-1111-1111-111111111111", ClienteNome: "Sol",
	})
	if err != nil {
		t.Fatalf("criar: %v", err)
	}

	obter := app.NovoCasoObterFactura(repo)
	det, err := obter.Executar(context.Background(), f.ID)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if det.ID != f.ID || det.Estado != "RASCUNHO" {
		t.Errorf("detalhe errado: %+v", det)
	}

	if _, err := obter.Executar(context.Background(), "inexistente"); err == nil {
		t.Error("obter factura inexistente devia falhar")
	}
}

func TestCriarFactura_ClienteInvalidoNaoAudita(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	uc := app.NovoCasoCriarFactura(repo, aud)
	if _, err := uc.Executar(context.Background(), "u-1", app.DadosNovaFactura{
		EpisodioID: "11111111-1111-1111-1111-111111111111", ClienteNome: "",
	}); err == nil {
		t.Error("cliente sem nome devia falhar")
	}
	if aud.tem("financeiro.factura.criada") {
		t.Error("falha de validação não devia ser auditada")
	}
}

func TestAdicionarItem_LinhaInvalidaNaoAudita(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	criar := app.NovoCasoCriarFactura(repo, aud)
	f, _ := criar.Executar(context.Background(), "u-1", app.DadosNovaFactura{
		EpisodioID: "11111111-1111-1111-1111-111111111111", ClienteNome: "Sol",
	})

	adicionar := app.NovoCasoAdicionarItem(repo, aud)
	if _, err := adicionar.Executar(context.Background(), "u-1", app.DadosNovoItem{
		FacturaID: f.ID, Descricao: "Medicamento", Tipo: "TIPO_INVALIDO",
		OperacaoID: "22222222-2222-2222-2222-222222222222", Quantidade: 2,
		PrecoUnitarioCentimos: 100000, RegimeIVA: "STANDARD",
	}); err == nil {
		t.Error("tipo de linha inválido devia falhar")
	}
	if aud.tem("financeiro.factura.item.adicionado") {
		t.Error("falha de validação não devia ser auditada")
	}
}

func TestRemoverItem_NaoEncontradoNaoAudita(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	criar := app.NovoCasoCriarFactura(repo, aud)
	f, _ := criar.Executar(context.Background(), "u-1", app.DadosNovaFactura{
		EpisodioID: "11111111-1111-1111-1111-111111111111", ClienteNome: "Sol",
	})

	remover := app.NovoCasoRemoverItem(repo, aud)
	if _, err := remover.Executar(context.Background(), "u-1", f.ID, "item-inexistente"); err == nil {
		t.Error("remoção de linha inexistente devia falhar")
	}
	if aud.tem("financeiro.factura.item.removido") {
		t.Error("falha de validação não devia ser auditada")
	}
}

func TestEmitirFactura_AuditaComNumeroEHash(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	id := rascunhoComLinha(t, repo)

	uc := app.NovoCasoEmitirFactura(repo, aud)
	out, err := uc.Executar(context.Background(), "tesoureiro-1", id)
	if err != nil {
		t.Fatalf("Executar: %v", err)
	}
	if out.Estado != "EMITIDA" {
		t.Errorf("estado = %q, queria EMITIDA", out.Estado)
	}
	if out.Numero == "" || out.Hash == "" {
		t.Errorf("número e hash tinham de vir preenchidos: %+v", out)
	}
	if !aud.tem("financeiro.factura.emitida") {
		t.Error("a emissão tinha de ser auditada")
	}
}

func TestEmitirFactura_PropagaErroDoDominio(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	id := rascunhoSemLinhas(t, repo)

	uc := app.NovoCasoEmitirFactura(repo, aud)
	_, err := uc.Executar(context.Background(), "tesoureiro-1", id)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Errorf("factura sem linhas devia dar RegraNegocio, deu %v", err)
	}
	if len(aud.registos) != 0 {
		t.Error("emissão falhada não podia ser auditada como sucesso")
	}
}

func TestEmitirFactura_FacturaInexistente(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}

	uc := app.NovoCasoEmitirFactura(repo, aud)
	_, err := uc.Executar(context.Background(), "tesoureiro-1", "inexistente")
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Errorf("factura inexistente devia dar NaoEncontrado, deu %v", err)
	}
	if len(aud.registos) != 0 {
		t.Error("emissão falhada não podia ser auditada como sucesso")
	}
}

func TestEmitirFactura_JaEmitidaNaoPodeSerReemitida(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	id := rascunhoComLinha(t, repo)

	uc := app.NovoCasoEmitirFactura(repo, aud)
	if _, err := uc.Executar(context.Background(), "tesoureiro-1", id); err != nil {
		t.Fatalf("primeira emissão: %v", err)
	}

	_, err := uc.Executar(context.Background(), "tesoureiro-1", id)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Errorf("reemissão devia dar Conflito, deu %v", err)
	}
}

func TestVerificarCadeia_DevolveIntegra(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	emissor := app.NovoCasoEmitirFactura(repo, aud)
	for i := 0; i < 3; i++ {
		if _, err := emissor.Executar(context.Background(), "tes-1", rascunhoComLinha(t, repo)); err != nil {
			t.Fatalf("emitir %d: %v", i, err)
		}
	}
	serie := dominio.SerieDe(time.Now())

	uc := app.NovoCasoVerificarCadeia(repo)
	out, err := uc.Executar(context.Background(), serie)
	if err != nil {
		t.Fatalf("Executar: %v", err)
	}
	if !out.Integra || out.TotalFacturas != 3 {
		t.Errorf("esperava íntegra com 3 facturas, deu %+v", out)
	}
}

func TestVerificarCadeia_QuebraEhResultadoNaoErro(t *testing.T) {
	repo := novoFakeFacturas()
	aud := &fakeAuditor{}
	emissor := app.NovoCasoEmitirFactura(repo, aud)
	for i := 0; i < 3; i++ {
		if _, err := emissor.Executar(context.Background(), "tes-1", rascunhoComLinha(t, repo)); err != nil {
			t.Fatalf("emitir %d: %v", i, err)
		}
	}
	serie := dominio.SerieDe(time.Now())
	repo.adulterarPrimeiraLinha(serie, 2, "Adulterada")

	uc := app.NovoCasoVerificarCadeia(repo)
	out, err := uc.Executar(context.Background(), serie)
	if err != nil {
		t.Fatalf("uma cadeia quebrada é um resultado, não um erro de execução: %v", err)
	}
	if out.Integra || out.Detalhe == "" {
		t.Errorf("esperava quebra reportada com detalhe, deu %+v", out)
	}
}
