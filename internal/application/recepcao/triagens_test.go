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
	// duas chegadas TRIADO do mesmo médico, prioridades diferentes.
	// o VERDE chega mais cedo (08:00) e o VERMELHO chega mais tarde (09:00):
	// se a fila fosse ordenada por hora de chegada ascendente, o VERDE (c1)
	// viria primeiro. A asserção abaixo só passa se for a severidade Manchester
	// a decidir a ordem — prova que não é um falso positivo por hora ascendente.
	c1 := semearChegadaTriada(t, chegadas, "doe-1", "med-1", "esp-1", "08:00")
	c2 := semearChegadaTriada(t, chegadas, "doe-2", "med-1", "esp-1", "09:00")
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
	// o VERMELHO (c2) tem de vir primeiro por severidade, apesar de ter chegado depois do VERDE (c1)
	if fila[0].ChegadaID != c2 {
		t.Fatalf("o VERMELHO devia vir primeiro na fila por severidade, veio %+v", fila)
	}
}

func TestObterTriagem_Inexistente_NaoEncontrado(t *testing.T) {
	triagens := novoFakeTriagens(novoFakeChegadas(novoFakeMarcacoes()))
	uc := app.NovoCasoObterTriagem(triagens)
	if _, err := uc.Executar(context.Background(), "cheg-x"); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava CategoriaNaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

// fptr é um atalho para obter um ponteiro para um float64 literal nos testes.
func fptr(v float64) *float64 { return &v }

func TestRegistarTriagem_WalkIn_TransitaAtribuiMedicoECria(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	// walk-in chamado (sem médico)
	c, _ := dominio.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	_ = c.Chamar(inst("09:05"))
	cid, _ := chegadas.Guardar(context.Background(), c)

	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarTriagem(triagens, chegadas, aud)
	uc.DefinirRelogio(agoraFixo("09:10"))
	out, err := uc.Executar(context.Background(), "enf-1", cid, app.DadosTriagem{
		Prioridade: "AMARELO", Temperatura: fptr(37.5), MedicoID: "med-9",
	})
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.EnfermeiroID != "enf-1" || out.Prioridade != "AMARELO" {
		t.Fatalf("detalhe mal preenchido: %+v", out)
	}
	// a chegada ficou TRIADO com o médico atribuído
	ch, _ := chegadas.ObterPorID(context.Background(), cid)
	if ch.Estado() != dominio.ChegTriado || ch.MedicoID() != "med-9" {
		t.Fatalf("chegada mal transitada: estado=%s medico=%s", ch.Estado(), ch.MedicoID())
	}
	if !aud.tem("recepcao.triagem.registada") {
		t.Fatal("esperava auditoria recepcao.triagem.registada")
	}
}

func TestRegistarTriagem_SinaisForaDeIntervalo_Validacao(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	c, _ := dominio.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	_ = c.Chamar(inst("09:05"))
	cid, _ := chegadas.Guardar(context.Background(), c)

	uc := app.NovoCasoRegistarTriagem(triagens, chegadas, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("09:10"))
	_, err := uc.Executar(context.Background(), "enf-1", cid, app.DadosTriagem{
		Prioridade: "VERDE", Temperatura: fptr(60), MedicoID: "med-9", // temperatura absurda
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
	// nada foi transitado
	ch, _ := chegadas.ObterPorID(context.Background(), cid)
	if ch.Estado() != dominio.ChegChamado {
		t.Fatalf("a chegada não devia ter transitado, veio %s", ch.Estado())
	}
}

func TestRegistarTriagem_ChegadaNaoChamada_Conflito(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	c, _ := dominio.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00")) // AGUARDA, não chamada
	cid, _ := chegadas.Guardar(context.Background(), c)

	uc := app.NovoCasoRegistarTriagem(triagens, chegadas, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("09:10"))
	_, err := uc.Executar(context.Background(), "enf-1", cid, app.DadosTriagem{Prioridade: "VERDE", MedicoID: "med-9"})
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestRegistarTriagem_Duplicada_Conflito(t *testing.T) {
	chegadas := novoFakeChegadas(novoFakeMarcacoes())
	triagens := novoFakeTriagens(chegadas)
	c, _ := dominio.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	_ = c.Chamar(inst("09:05"))
	cid, _ := chegadas.Guardar(context.Background(), c)

	uc := app.NovoCasoRegistarTriagem(triagens, chegadas, &fakeAuditor{})
	uc.DefinirRelogio(agoraFixo("09:10"))
	if _, err := uc.Executar(context.Background(), "enf-1", cid, app.DadosTriagem{Prioridade: "VERDE", MedicoID: "med-9"}); err != nil {
		t.Fatalf("primeira triagem não devia falhar: %v", err)
	}
	// segunda: a chegada já não está CHAMADO → Conflito
	_, err := uc.Executar(context.Background(), "enf-1", cid, app.DadosTriagem{Prioridade: "VERDE"})
	if erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("triagem duplicada devia dar CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}
