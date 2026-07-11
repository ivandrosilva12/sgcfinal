package i18n_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

func TestT_ChaveConhecida(t *testing.T) {
	if got := i18n.T(i18n.MsgServicoOperacional); got != "Serviço operacional." {
		t.Fatalf("mensagem inesperada: %q", got)
	}
	if got := i18n.T(i18n.MsgNaoAutenticado); got == "" {
		t.Fatal("mensagem conhecida não deve ser vazia")
	}
}

func TestT_ChaveDesconhecida(t *testing.T) {
	// Falha visível: devolve a própria chave, não vazio.
	desconhecida := i18n.Chave("inexistente.xyz")
	if got := i18n.T(desconhecida); got != "inexistente.xyz" {
		t.Fatalf("esperava devolver a própria chave, obtive %q", got)
	}
}
