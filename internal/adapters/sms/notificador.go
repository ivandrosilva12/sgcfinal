// Package sms implementa o NotificadorCritico (application/laboratorio) por HTTP
// contra um gateway SMS. Camada 3 — Adaptadores. A integração com um gateway real de
// Angola fica para um marco posterior; em dev usa-se o notificador no-op.
package sms

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
)

// EnviarFunc é o transporte do SMS; extraído para substituição em testes.
type EnviarFunc func(ctx context.Context, telefone, mensagem string) error

// NotificadorSMS envia alertas por SMS através de um gateway HTTP.
type NotificadorSMS struct {
	endpoint  string
	remetente string
	// Enviar é o transporte; default enviarHTTP. Público para testes.
	Enviar EnviarFunc
}

// NovoNotificadorSMS constrói o adaptador apontado ao endpoint indicado.
func NovoNotificadorSMS(endpoint, remetente string) *NotificadorSMS {
	n := &NotificadorSMS{endpoint: endpoint, remetente: remetente}
	n.Enviar = n.enviarHTTP
	return n
}

// NotificarValorCritico compõe e envia a mensagem de valor crítico.
func (n *NotificadorSMS) NotificarValorCritico(ctx context.Context, telefone, codigoAnalise, valor string) error {
	msg := fmt.Sprintf("SGC: valor crítico na análise %s: %s. Contacte o laboratório.", codigoAnalise, valor)
	return n.Enviar(ctx, telefone, msg)
}

func (n *NotificadorSMS) enviarHTTP(ctx context.Context, telefone, mensagem string) error {
	form := url.Values{"from": {n.remetente}, "to": {telefone}, "text": {mensagem}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("preparar pedido SMS: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("enviar SMS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gateway SMS respondeu %d", resp.StatusCode)
	}
	return nil
}

var _ applaboratorio.NotificadorCritico = (*NotificadorSMS)(nil)
