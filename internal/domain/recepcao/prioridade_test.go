package recepcao_test

import (
	"testing"
	"time"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestParsePrioridade_ValidaENormaliza(t *testing.T) {
	casos := map[string]recepcao.PrioridadeManchester{
		"VERMELHO":  recepcao.ManVermelho,
		"laranja":   recepcao.ManLaranja,
		" Amarelo ": recepcao.ManAmarelo,
		"VERDE":     recepcao.ManVerde,
		"azul":      recepcao.ManAzul,
	}
	for entrada, esperado := range casos {
		got, err := recepcao.ParsePrioridade(entrada)
		if err != nil {
			t.Fatalf("ParsePrioridade(%q): erro inesperado %v", entrada, err)
		}
		if got != esperado {
			t.Fatalf("ParsePrioridade(%q) = %q; esperava %q", entrada, got, esperado)
		}
	}
}

func TestParsePrioridade_Invalida(t *testing.T) {
	if _, err := recepcao.ParsePrioridade("ROXO"); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestSeveridade_OrdemDeUrgencia(t *testing.T) {
	// menor severidade = mais urgente
	ordem := []recepcao.PrioridadeManchester{
		recepcao.ManVermelho, recepcao.ManLaranja, recepcao.ManAmarelo, recepcao.ManVerde, recepcao.ManAzul,
	}
	for i := 1; i < len(ordem); i++ {
		if ordem[i-1].Severidade() >= ordem[i].Severidade() {
			t.Fatalf("%s (%d) devia ser mais urgente que %s (%d)",
				ordem[i-1], ordem[i-1].Severidade(), ordem[i], ordem[i].Severidade())
		}
	}
	if recepcao.ManVermelho.Severidade() != 1 || recepcao.ManAzul.Severidade() != 5 {
		t.Fatal("VERMELHO devia ser 1 e AZUL 5")
	}
}

func TestTempoAlvo(t *testing.T) {
	casos := map[recepcao.PrioridadeManchester]time.Duration{
		recepcao.ManVermelho: 0,
		recepcao.ManLaranja:  10 * time.Minute,
		recepcao.ManAmarelo:  60 * time.Minute,
		recepcao.ManVerde:    120 * time.Minute,
		recepcao.ManAzul:     240 * time.Minute,
	}
	for p, esperado := range casos {
		if p.TempoAlvo() != esperado {
			t.Fatalf("%s.TempoAlvo() = %v; esperava %v", p, p.TempoAlvo(), esperado)
		}
	}
}
