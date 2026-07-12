// Package smtp implementa o Notificador (application/identidade) por email, via
// SMTP (net/smtp da stdlib). Adequado a dev com MailHog (sem autenticação).
// Camada 3 — Adaptadores.
package smtp

import (
	"context"
	"fmt"
	"net"
	nsmtp "net/smtp"
	"strings"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
)

// EnviarFunc é a assinatura de net/smtp.SendMail; extraída para permitir
// substituição em testes.
type EnviarFunc func(addr string, a nsmtp.Auth, from string, to []string, msg []byte) error

// NotificadorSMTP envia notificações por email através de um servidor SMTP.
type NotificadorSMTP struct {
	host      string
	porta     string
	remetente string
	// Enviar é o transporte SMTP; default net/smtp.SendMail. Público para testes.
	Enviar EnviarFunc
}

// NovoNotificadorSMTP constrói o adaptador apontado a host:porta, com o
// remetente indicado.
func NovoNotificadorSMTP(host, porta, remetente string) *NotificadorSMTP {
	return &NotificadorSMTP{host: host, porta: porta, remetente: remetente, Enviar: nsmtp.SendMail}
}

// NotificarCriacao envia o email de conta criada com a senha temporária.
func (n *NotificadorSMTP) NotificarCriacao(_ context.Context, email, nome, senha string) error {
	assunto := "Conta SGC criada"
	corpo := fmt.Sprintf(
		"Olá %s,\r\n\r\nFoi criada uma conta no Sistema de Gestão de Clínicas (SGC).\r\n"+
			"Senha temporária: %s\r\n\r\nSerá pedido para a alterar no primeiro acesso.\r\n",
		nome, senha)
	return n.enviarMensagem(email, assunto, corpo)
}

// NotificarResetPassword envia o email de senha reposta com a nova senha temporária.
func (n *NotificadorSMTP) NotificarResetPassword(_ context.Context, email, nome, senha string) error {
	assunto := "Senha SGC reposta"
	corpo := fmt.Sprintf(
		"Olá %s,\r\n\r\nA sua senha no SGC foi reposta por um administrador.\r\n"+
			"Senha temporária: %s\r\n\r\nSerá pedido para a alterar no próximo acesso.\r\n",
		nome, senha)
	return n.enviarMensagem(email, assunto, corpo)
}

func (n *NotificadorSMTP) enviarMensagem(para, assunto, corpo string) error {
	msg := montarMensagem(n.remetente, para, assunto, corpo)
	return n.Enviar(net.JoinHostPort(n.host, n.porta), nil, n.remetente, []string{para}, msg)
}

// montarMensagem compõe uma mensagem RFC 5322 de texto simples (UTF-8).
func montarMensagem(de, para, assunto, corpo string) []byte {
	var b strings.Builder
	b.WriteString("From: " + limparCabecalho(de) + "\r\n")
	b.WriteString("To: " + limparCabecalho(para) + "\r\n")
	b.WriteString("Subject: " + limparCabecalho(assunto) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(corpo)
	return []byte(b.String())
}

// limparCabecalho remove CR/LF de um valor de cabeçalho, como defesa em
// profundidade contra injecção de cabeçalhos SMTP. O email já é validado por
// net/mail a montante e o transporte (net/smtp) rejeita CR/LF em endereços;
// esta é uma salvaguarda adicional para os cabeçalhos escritos no corpo DATA.
func limparCabecalho(v string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(v)
}

// Garantia de conformidade com a porta.
var _ appident.Notificador = (*NotificadorSMTP)(nil)
