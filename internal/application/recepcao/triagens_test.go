// internal/application/recepcao/triagens_test.go
package recepcao_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// triagemAgregada cria uma triagem de domínio para semear os fakes nos testes de leitura.
func triagemAgregada(t *testing.T, chegadaID, enfermeiro string, p dominio.PrioridadeManchester) *dominio.Triagem {
	t.Helper()
	tr, err := dominio.NovaTriagem(chegadaID, enfermeiro, p, dominio.SinaisVitais{}, "", inst("09:10"))
	if err != nil {
		t.Fatalf("triagem inválida no teste: %v", err)
	}
	return tr
}

func TestObterTriagem_DevolveDetalhe(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	// semeia uma chegada TRIADO e a sua triagem
	cid, _ := chegadas.Guardar(context.Background(), chegadaTriadaSemear(t, chegadas, "doe-1", "med-1", "esp-1"))
	_, _ = triagens.RegistarTriagem(context.Background(), triagemAgregada(t, cid, "enf-1", dominio.ManAmarelo), reconstruirTriada(t, chegadas, cid))

	uc := app.NovoCasoObterTriagem(triagens)
	out, err := uc.Executar(context.Background(), cid)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.ChegadaID != cid || out.Prioridade != string(dominio.ManAmarelo) {
		t.Fatalf("detalhe mal preenchido: %+v", out)
	}
}

func TestListarFilaClinica_OrdenadaPorPrioridade(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	// duas chegadas TRIADO do mesmo médico, prioridades diferentes
	c1 := semearChegadaTriada(t, chegadas, "doe-1", "med-1", "esp-1", "09:00")
	c2 := semearChegadaTriada(t, chegadas, "doe-2", "med-1", "esp-1", "08:00")
	_, _ = triagens.RegistarTriagem(context.Background(), triagemAgregada(t, c1, "enf-1", dominio.ManVerde), reconstruirTriada(t, chegadas, c1))
	_, _ = triagens.RegistarTriagem(context.Background(), triagemAgregada(t, c2, "enf-1", dominio.ManVermelho), reconstruirTriada(t, chegadas, c2))

	uc := app.NovoCasoListarFilaClinica(triagens)
	fila, err := uc.Executar(context.Background(), "med-1")
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if len(fila) != 2 {
		t.Fatalf("esperava 2 na fila, veio %d", len(fila))
	}
	// o VERMELHO (c2) tem de vir primeiro, apesar de ter chegado antes
	if fila[0].ChegadaID != c2 {
		t.Fatalf("o VERMELHO devia vir primeiro na fila, veio %+v", fila)
	}
}

func TestObterTriagem_Inexistente_NaoEncontrado(t *testing.T) {
	triagens := novoFakeTriagens(novoFakeChegadas(novoFakeMarcacoes()))
	uc := app.NovoCasoObterTriagem(triagens)
	if _, err := uc.Executar(context.Background(), "cheg-x"); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava CategoriaNaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}
