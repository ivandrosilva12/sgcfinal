package laboratorio_test

import (
	"context"
	"testing"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// cenarioValidacao prepara um resultado PROCESSADA na BD em memória, com o catálogo e
// a requisição correspondentes, e devolve os fakes e o id do resultado.
func cenarioValidacao(t *testing.T, criticos []dominio.ValorCritico, valor string) (
	*fakeResultados, *fakeRequisicoes, *fakeAnalises, *fakeResolvedorContacto, *fakeNotificadorCritico, *fakeAuditor, string,
) {
	t.Helper()
	analises := novoFakeAnalises()
	hb, err := dominio.NovaAnalise("HB", "Hemoglobina", "g/dL", nil, criticos)
	if err != nil {
		t.Fatalf("análise: %v", err)
	}
	_ = analises.Guardar(context.Background(), hb)

	resultados := novoFakeResultados()
	requisicoes := novoFakeRequisicoes(resultados)
	req, _ := dominio.NovaRequisicao(dominio.DadosNovaRequisicao{
		EpisodioID: "ep-1", DoenteID: "doe-1", MedicoRequisitanteID: "med-1",
		Prioridade: dominio.PrioridadeRotina, Itens: []dominio.ItemRequisicao{{CodigoAnalise: "HB"}},
	})
	res, _ := dominio.NovoResultado("por-atribuir", "HB", "g/dL")
	reqID, _ := requisicoes.Emitir(context.Background(), req, []*dominio.Resultado{res})

	// Levar o único resultado até PROCESSADA (colher + submeter) via os fakes.
	var resID string
	fila, _ := resultados.ListarFila(context.Background(), []dominio.EstadoResultado{dominio.ResPendente})
	for _, r := range fila {
		if r.RequisicaoID == reqID {
			resID = r.ID
		}
	}
	r0, _ := resultados.ObterPorID(context.Background(), resID)
	_ = r0.ColherAmostra("tec-1", agoraFixo())
	_ = resultados.Transitar(context.Background(), r0)
	r1, _ := resultados.ObterPorID(context.Background(), resID)
	_ = r1.SubmeterPreliminar("tec-1", valor, "", agoraFixo())
	_ = resultados.Transitar(context.Background(), r1)

	return resultados, requisicoes, analises,
		&fakeResolvedorContacto{telefone: "+244923000000", ok: true},
		&fakeNotificadorCritico{}, &fakeAuditor{}, resID
}

func TestValidarResultado_CriticoNotificaEAudita(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t,
		[]dominio.ValorCritico{{Operador: dominio.CriticoMenor, Limite: 3.0, Descricao: "anemia grave"}}, "2.5")

	uc := applaboratorio.NovoCasoValidarResultado(res, req, an, contactos, notif, aud)
	out, err := uc.Executar(context.Background(), "pat-9", resID)
	if err != nil {
		t.Fatalf("validar: %v", err)
	}
	if out.Estado != string(dominio.ResValidada) || !out.ValorCritico {
		t.Fatalf("esperava VALIDADA crítica, veio %+v", out)
	}
	if len(notif.enviados) != 1 || notif.enviados[0] != "+244923000000" {
		t.Fatalf("esperava 1 SMS ao médico, veio %+v", notif.enviados)
	}
	if !aud.tem("laboratorio.resultado.validado") || !aud.tem("laboratorio.valor_critico.notificado") {
		t.Fatalf("faltam registos de auditoria: %+v", aud.registos)
	}
}

func TestValidarResultado_NaoCritico_NaoNotifica(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t,
		[]dominio.ValorCritico{{Operador: dominio.CriticoMenor, Limite: 3.0, Descricao: "x"}}, "12.5")

	uc := applaboratorio.NovoCasoValidarResultado(res, req, an, contactos, notif, aud)
	if _, err := uc.Executar(context.Background(), "pat-9", resID); err != nil {
		t.Fatalf("validar: %v", err)
	}
	if len(notif.enviados) != 0 {
		t.Fatalf("um valor normal não devia disparar SMS, veio %+v", notif.enviados)
	}
	if aud.tem("laboratorio.valor_critico.notificado") {
		t.Fatal("um valor normal não devia auditar notificação de crítico")
	}
}

func TestValidarResultado_Segregacao_Bloqueia(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t, nil, "12.5")
	uc := applaboratorio.NovoCasoValidarResultado(res, req, an, contactos, notif, aud)
	// O submissor foi "tec-1" — validar como "tec-1" viola a segregação.
	_, err := uc.Executar(context.Background(), "tec-1", resID)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("auto-validação devia dar RegraNegocio, veio %v", err)
	}
}

func TestValidarResultado_SMSFalhado_NaoFalhaValidacao(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t,
		[]dominio.ValorCritico{{Operador: dominio.CriticoMenor, Limite: 3.0, Descricao: "x"}}, "2.5")
	notif.err = erros.Novo(erros.CategoriaInterno, "gateway em baixo")

	uc := applaboratorio.NovoCasoValidarResultado(res, req, an, contactos, notif, aud)
	out, err := uc.Executar(context.Background(), "pat-9", resID)
	if err != nil {
		t.Fatalf("uma falha de SMS não deve falhar a validação: %v", err)
	}
	if out.Estado != string(dominio.ResValidada) {
		t.Fatalf("o resultado devia ficar VALIDADA na mesma, veio %s", out.Estado)
	}
	// Mesmo com falha de envio, a tentativa é auditada.
	if !aud.tem("laboratorio.valor_critico.notificado") {
		t.Fatal("a tentativa de notificação devia ser auditada mesmo em falha")
	}
}
