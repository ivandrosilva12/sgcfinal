package laboratorio_test

import (
	"context"
	"testing"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// validarNoFake leva o resultado de PROCESSADA a VALIDADA usando o caso de uso real.
func validarNoFake(t *testing.T, res *fakeResultados, req *fakeRequisicoes, an *fakeAnalises, resID string) {
	t.Helper()
	uc := applaboratorio.NovoCasoValidarResultado(res, req, an,
		&fakeResolvedorContacto{ok: false}, &fakeNotificadorCritico{}, &fakeAuditor{})
	if _, err := uc.Executar(context.Background(), "pat-2", resID); err != nil {
		t.Fatalf("preparar VALIDADA: %v", err)
	}
}

func TestCorrigirResultado_CriaNovoEArquivaOriginal(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t, nil, "2.5")
	validarNoFake(t, res, req, an, resID)

	uc := applaboratorio.NovoCasoCorrigirResultado(res, req, an, contactos, notif, aud)
	out, err := uc.Executar(context.Background(), "pat-3", resID,
		applaboratorio.DadosCorrigirResultado{Valor: "12.5", Observacoes: "releitura"})
	if err != nil {
		t.Fatalf("corrigir: %v", err)
	}
	// A resposta é o novo resultado vigente.
	if out.Estado != string(dominio.ResValidada) || out.Valor != "12.5" || out.ID == resID {
		t.Fatalf("esperava um novo VALIDADA com valor 12.5, veio %+v", out)
	}
	// O original ficou CONCLUIDA.
	orig, _ := res.ObterPorID(context.Background(), resID)
	if orig.Estado() != dominio.ResConcluida {
		t.Fatalf("o original devia ficar CONCLUIDA, veio %s", orig.Estado())
	}
	if !aud.tem("laboratorio.resultado.corrigido") {
		t.Fatalf("faltou auditar a correcção: %+v", aud.registos)
	}
}

func TestCorrigirResultado_ReavaliaCriticoENotifica(t *testing.T) {
	// O original não era crítico (12.5); a correcção mete um valor crítico (2.5).
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t,
		[]dominio.ValorCritico{{Operador: dominio.CriticoMenor, Limite: 3.0, Descricao: "anemia grave"}}, "12.5")
	contactos.telefone, contactos.ok = "+244923111111", true
	validarNoFake(t, res, req, an, resID)

	uc := applaboratorio.NovoCasoCorrigirResultado(res, req, an, contactos, notif, aud)
	out, err := uc.Executar(context.Background(), "pat-3", resID,
		applaboratorio.DadosCorrigirResultado{Valor: "2.5"})
	if err != nil {
		t.Fatalf("corrigir: %v", err)
	}
	if !out.ValorCritico {
		t.Fatal("a correcção com valor crítico devia marcar o novo resultado como crítico")
	}
	if len(notif.enviados) != 1 {
		t.Fatalf("a correcção crítica devia notificar por SMS, veio %+v", notif.enviados)
	}
}

func TestCorrigirResultado_Segregacao(t *testing.T) {
	res, req, an, contactos, notif, aud, resID := cenarioValidacao(t, nil, "2.5")
	validarNoFake(t, res, req, an, resID)

	uc := applaboratorio.NovoCasoCorrigirResultado(res, req, an, contactos, notif, aud)
	// "tec-1" foi o submissor original — não pode corrigir.
	_, err := uc.Executar(context.Background(), "tec-1", resID,
		applaboratorio.DadosCorrigirResultado{Valor: "12.5"})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("correcção pelo submissor original devia dar RegraNegocio, veio %v", err)
	}
}
