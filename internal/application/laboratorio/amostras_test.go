package laboratorio_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// primeiroPendente emite uma requisição e devolve o id de um resultado PENDENTE.
func primeiroPendente(t *testing.T, c *cenario) string {
	t.Helper()
	if _, err := c.emitir().Executar(context.Background(), "med-1", dadosEmitir()); err != nil {
		t.Fatalf("emitir requisição: %v", err)
	}
	fila, err := c.resultados.ListarFila(context.Background(), []dominio.EstadoResultado{dominio.ResPendente})
	if err != nil || len(fila) == 0 {
		t.Fatalf("esperava resultados na fila, veio (%+v, %v)", fila, err)
	}
	return fila[0].ID
}

func TestColherESubmeter_GravaOSujeitoAutenticado(t *testing.T) {
	c := novoCenario(t)
	id := primeiroPendente(t, c)

	if _, err := app.NovoCasoColherAmostra(c.resultados, c.auditor).
		Executar(context.Background(), "tec-1", id); err != nil {
		t.Fatalf("colher amostra: %v", err)
	}
	out, err := app.NovoCasoSubmeterPreliminar(c.resultados, c.auditor).
		Executar(context.Background(), "tec-1", id, app.DadosSubmeterPreliminar{Valor: "12.5"})
	if err != nil {
		t.Fatalf("submeter preliminar: %v", err)
	}
	if out.Estado != string(dominio.ResProcessada) {
		t.Fatalf("esperava PROCESSADA, veio %s", out.Estado)
	}
	// O submissor é o actor, não um campo do pedido — é contra ele que o Sprint 13
	// comparará o patologista para impor a segregação de funções.
	if out.TecnicoSubmissorID != "tec-1" {
		t.Fatalf("esperava submissor tec-1, veio %q", out.TecnicoSubmissorID)
	}
	if !c.auditor.tem("laboratorio.amostra.colhida") || !c.auditor.tem("laboratorio.resultado.preliminar_submetido") {
		t.Fatalf("esperava auditoria da colheita e da submissão: %+v", c.auditor.registos)
	}
}

func TestSubmeterPreliminar_SemColher(t *testing.T) {
	c := novoCenario(t)
	id := primeiroPendente(t, c)
	_, err := app.NovoCasoSubmeterPreliminar(c.resultados, c.auditor).
		Executar(context.Background(), "tec-1", id, app.DadosSubmeterPreliminar{Valor: "12.5"})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("submeter sem colher devia falhar com Conflito, veio %v", err)
	}
}

func TestRecusarAmostra_ExigeMotivo(t *testing.T) {
	c := novoCenario(t)
	id := primeiroPendente(t, c)
	uc := app.NovoCasoRecusarAmostra(c.resultados, c.auditor)

	if _, err := uc.Executar(context.Background(), "tec-1", id, "  "); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("recusar sem motivo devia falhar com Validacao, veio %v", err)
	}
	out, err := uc.Executar(context.Background(), "tec-1", id, "amostra coagulada")
	if err != nil {
		t.Fatalf("recusar amostra: %v", err)
	}
	if out.Estado != string(dominio.ResRecusada) || out.MotivoRecusa != "amostra coagulada" {
		t.Fatalf("recusa não registada: %+v", out)
	}
	if !c.auditor.tem("laboratorio.amostra.recusada") {
		t.Fatalf("esperava auditoria da recusa: %+v", c.auditor.registos)
	}
}

// TestPreliminarNaoEVisivelAoMedico é o critério de saída do marco: o resultado
// submetido pelo técnico não aparece na leitura clínica; a fila do laboratório vê-o.
func TestPreliminarNaoEVisivelAoMedico(t *testing.T) {
	c := novoCenario(t)
	id := primeiroPendente(t, c)
	if _, err := app.NovoCasoColherAmostra(c.resultados, c.auditor).
		Executar(context.Background(), "tec-1", id); err != nil {
		t.Fatalf("colher: %v", err)
	}
	if _, err := app.NovoCasoSubmeterPreliminar(c.resultados, c.auditor).
		Executar(context.Background(), "tec-1", id, app.DadosSubmeterPreliminar{Valor: "12.5"}); err != nil {
		t.Fatalf("submeter: %v", err)
	}

	// Visão clínica: nada — nenhum resultado está VALIDADA/CONCLUIDA.
	clinica, err := app.NovoCasoListarResultadosDoEpisodio(c.resultados).
		Executar(context.Background(), "ep-1")
	if err != nil {
		t.Fatalf("listar resultados do episódio: %v", err)
	}
	if len(clinica) != 0 {
		t.Fatalf("o preliminar NÃO pode ser visível ao médico, veio %+v", clinica)
	}

	// Fila do laboratório: vê o PROCESSADA.
	fila, err := app.NovoCasoListarFila(c.resultados).
		Executar(context.Background(), []dominio.EstadoResultado{dominio.ResProcessada})
	if err != nil {
		t.Fatalf("listar fila: %v", err)
	}
	if len(fila) != 1 || fila[0].Estado != string(dominio.ResProcessada) {
		t.Fatalf("a fila do laboratório devia ver o preliminar, veio %+v", fila)
	}
}
