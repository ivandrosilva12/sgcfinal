package laboratorio_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// cenario monta os fakes com o catálogo já povoado (HB, GLIC) e a ACL a aceitar.
type cenario struct {
	analises    *fakeAnalises
	requisicoes *fakeRequisicoes
	resultados  *fakeResultados
	leitor      *fakeLeitorClinico
	auditor     *fakeAuditor
}

func novoCenario(t *testing.T) *cenario {
	t.Helper()
	an := novoFakeAnalises()
	for _, a := range []struct{ codigo, nome, unidade string }{
		{"HB", "Hemoglobina", "g/dL"},
		{"GLIC", "Glicemia", "mg/dL"},
	} {
		x, err := dominio.NovaAnalise(a.codigo, a.nome, a.unidade, nil, nil)
		if err != nil {
			t.Fatalf("análise base inválida: %v", err)
		}
		if err := an.Guardar(context.Background(), x); err != nil {
			t.Fatalf("guardar análise base: %v", err)
		}
	}
	res := novoFakeResultados()
	return &cenario{
		analises: an, resultados: res, requisicoes: novoFakeRequisicoes(res),
		leitor:  &fakeLeitorClinico{doenteActivo: true, episodioAberto: true},
		auditor: &fakeAuditor{},
	}
}

func (c *cenario) emitir() *app.CasoEmitirRequisicao {
	return app.NovoCasoEmitirRequisicao(c.requisicoes, c.analises, c.leitor, c.auditor)
}

func dadosEmitir() app.DadosEmitirRequisicao {
	return app.DadosEmitirRequisicao{
		EpisodioID: "ep-1", DoenteID: "doente-1", Prioridade: "ROTINA",
		Itens: []app.ItemPedido{{CodigoAnalise: "HB"}, {CodigoAnalise: "GLIC"}},
	}
}

func TestEmitirRequisicao_CriaUmResultadoPendentePorItem(t *testing.T) {
	c := novoCenario(t)
	out, err := c.emitir().Executar(context.Background(), "med-1", dadosEmitir())
	if err != nil {
		t.Fatalf("emitir requisição: %v", err)
	}
	if out.MedicoRequisitanteID != "med-1" {
		t.Fatalf("o requisitante tem de ser o sujeito autenticado, veio %q", out.MedicoRequisitanteID)
	}
	fila, err := c.resultados.ListarFila(context.Background(), []dominio.EstadoResultado{dominio.ResPendente})
	if err != nil {
		t.Fatalf("listar fila: %v", err)
	}
	if len(fila) != 2 {
		t.Fatalf("esperava 2 resultados PENDENTE (um por item), veio %d", len(fila))
	}
	if !c.auditor.tem("laboratorio.requisicao.emitida") {
		t.Fatalf("esperava auditoria da emissão: %+v", c.auditor.registos)
	}
}

func TestEmitirRequisicao_EpisodioFechado(t *testing.T) {
	c := novoCenario(t)
	c.leitor.episodioAberto = false
	_, err := c.emitir().Executar(context.Background(), "med-1", dadosEmitir())
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("episódio fechado devia falhar com Conflito, veio %v", err)
	}
}

func TestEmitirRequisicao_DoenteInactivo(t *testing.T) {
	c := novoCenario(t)
	c.leitor.doenteActivo = false
	_, err := c.emitir().Executar(context.Background(), "med-1", dadosEmitir())
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("doente inactivo devia falhar com RegraNegocio, veio %v", err)
	}
}

func TestEmitirRequisicao_AnaliseInexistente(t *testing.T) {
	c := novoCenario(t)
	d := dadosEmitir()
	d.Itens = []app.ItemPedido{{CodigoAnalise: "NAOEXISTE"}}
	_, err := c.emitir().Executar(context.Background(), "med-1", d)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("análise inexistente devia falhar com NaoEncontrado, veio %v", err)
	}
}

func TestObterEListarRequisicoes(t *testing.T) {
	c := novoCenario(t)
	criada, err := c.emitir().Executar(context.Background(), "med-1", dadosEmitir())
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	obtida, err := app.NovoCasoObterRequisicao(c.requisicoes).Executar(context.Background(), criada.ID)
	if err != nil {
		t.Fatalf("obter requisição: %v", err)
	}
	if obtida.ID != criada.ID || len(obtida.Itens) != 2 {
		t.Fatalf("requisição obtida não bate certo: %+v", obtida)
	}
	lista, err := app.NovoCasoListarRequisicoesDoEpisodio(c.requisicoes).Executar(context.Background(), "ep-1")
	if err != nil {
		t.Fatalf("listar requisições: %v", err)
	}
	if len(lista) != 1 || lista[0].NumAnalises != 2 {
		t.Fatalf("esperava 1 requisição com 2 análises, veio %+v", lista)
	}
}
