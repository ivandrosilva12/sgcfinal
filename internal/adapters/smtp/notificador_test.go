package smtp

import (
	"context"
	"errors"
	nsmtp "net/smtp"
	"strings"
	"testing"
)

func TestNotificadorSMTP_NotificarCriacao_ComponMensagem(t *testing.T) {
	var (
		addr string
		to   []string
		msg  []byte
	)
	n := NovoNotificadorSMTP("mailhog", "1025", "nao-responder@sgc.ao")
	n.Enviar = func(a string, _ nsmtp.Auth, _ string, dest []string, corpo []byte) error {
		addr, to, msg = a, dest, corpo
		return nil
	}

	if err := n.NotificarCriacao(context.Background(), "ana@sgc.ao", "Ana", "senha-secreta-1"); err != nil {
		t.Fatalf("NotificarCriacao: %v", err)
	}
	if addr != "mailhog:1025" {
		t.Fatalf("addr = %q; quer mailhog:1025", addr)
	}
	if len(to) != 1 || to[0] != "ana@sgc.ao" {
		t.Fatalf("to = %v; quer [ana@sgc.ao]", to)
	}
	s := string(msg)
	if !strings.Contains(s, "To: ana@sgc.ao") {
		t.Fatalf("cabeçalho To em falta: %s", s)
	}
	if !strings.Contains(s, "senha-secreta-1") {
		t.Fatalf("senha temporária em falta no corpo: %s", s)
	}
}

func TestNotificadorSMTP_PropagaErro(t *testing.T) {
	n := NovoNotificadorSMTP("mailhog", "1025", "x@sgc.ao")
	n.Enviar = func(string, nsmtp.Auth, string, []string, []byte) error {
		return errors.New("smtp em baixo")
	}
	if err := n.NotificarResetPassword(context.Background(), "a@sgc.ao", "A", "s"); err == nil {
		t.Fatal("esperava erro propagado do envio")
	}
}

func TestNotificadorNulo_DevolveNil(t *testing.T) {
	n := NovoNotificadorNulo(nil)
	if err := n.NotificarCriacao(context.Background(), "a@sgc.ao", "A", "s"); err != nil {
		t.Fatalf("NotificarCriacao nulo devia devolver nil, obtive %v", err)
	}
	if err := n.NotificarResetPassword(context.Background(), "a@sgc.ao", "A", "s"); err != nil {
		t.Fatalf("NotificarResetPassword nulo devia devolver nil, obtive %v", err)
	}
}
