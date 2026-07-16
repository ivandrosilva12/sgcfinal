package sms_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/sms"
)

func TestNotificadorSMS_UsaOTransporteEComoeAMensagem(t *testing.T) {
	n := sms.NovoNotificadorSMS("http://gateway.local", "SGC")
	var telefone, mensagem string
	n.Enviar = func(_ context.Context, tel, msg string) error {
		telefone, mensagem = tel, msg
		return nil
	}
	if err := n.NotificarValorCritico(context.Background(), "+244923000000", "HB", "2.5"); err != nil {
		t.Fatalf("notificar: %v", err)
	}
	if telefone != "+244923000000" {
		t.Fatalf("telefone errado: %q", telefone)
	}
	if mensagem == "" || mensagem[:4] != "SGC:" {
		t.Fatalf("mensagem inesperada: %q", mensagem)
	}
}

// TestNotificadorSMS_EnviarHTTP_Sucesso exercita o transporte HTTP real
// (enviarHTTP, o default de NovoNotificadorSMS), sem substituir Enviar, contra
// um gateway falso local.
func TestNotificadorSMS_EnviarHTTP_Sucesso(t *testing.T) {
	var corpo, contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		corpo = r.PostForm.Encode()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := sms.NovoNotificadorSMS(srv.URL, "SGC")
	if err := n.NotificarValorCritico(context.Background(), "+244923000000", "HB", "2.5"); err != nil {
		t.Fatalf("notificar: %v", err)
	}
	if contentType != "application/x-www-form-urlencoded" {
		t.Fatalf("content-type inesperado: %q", contentType)
	}
	if corpo == "" {
		t.Fatal("corpo do pedido vazio")
	}
}

// TestNotificadorSMS_EnviarHTTP_ErroServidor confirma que uma resposta de erro do
// gateway se propaga como erro — o alerta de valor crítico é best-effort, mas o
// chamador tem de saber que falhou (para auditar).
func TestNotificadorSMS_EnviarHTTP_ErroServidor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := sms.NovoNotificadorSMS(srv.URL, "SGC")
	if err := n.NotificarValorCritico(context.Background(), "+244923000000", "HB", "2.5"); err == nil {
		t.Fatal("esperava erro do gateway SMS")
	}
}

// TestNotificadorSMS_EnviarHTTP_EndpointInvalido confirma que um endpoint mal
// formado falha ao preparar o pedido, sem panic.
func TestNotificadorSMS_EnviarHTTP_EndpointInvalido(t *testing.T) {
	n := sms.NovoNotificadorSMS("://endpoint-invalido", "SGC")
	if err := n.NotificarValorCritico(context.Background(), "+244923000000", "HB", "2.5"); err == nil {
		t.Fatal("esperava erro ao preparar o pedido")
	}
}
