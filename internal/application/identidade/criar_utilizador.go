package identidade

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// CasoCriarUtilizador cria um utilizador no Keycloak (fonte de verdade), com uma
// senha temporária gerada e, se algum papel for sensível, exigência de OTP.
type CasoCriarUtilizador struct {
	admin   AdminIdentidade
	auditor Auditor
	notif   Notificador
	agora   func() time.Time
}

// NovoCasoCriarUtilizador constrói o caso de uso.
func NovoCasoCriarUtilizador(a AdminIdentidade, aud Auditor, notif Notificador) *CasoCriarUtilizador {
	return &CasoCriarUtilizador{admin: a, auditor: aud, notif: notif, agora: time.Now}
}

// Executar valida a entrada, gera a senha temporária, delega a criação no Keycloak,
// audita e devolve o id + a senha (uma única vez).
func (c *CasoCriarUtilizador) Executar(ctx context.Context, actor string, entrada CriacaoUtilizador) (UtilizadorCriado, error) {
	if strings.TrimSpace(entrada.Username) == "" || strings.TrimSpace(entrada.Nome) == "" {
		return UtilizadorCriado{}, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgCriacaoInvalida))
	}
	if _, err := mail.ParseAddress(entrada.Email); err != nil {
		return UtilizadorCriado{}, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgCriacaoInvalida))
	}
	for _, p := range entrada.Papeis {
		if !dominio.PapelValido(string(p)) {
			return UtilizadorCriado{}, erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPapelInvalido))
		}
	}

	senha, err := gerarSenhaTemporaria()
	if err != nil {
		return UtilizadorCriado{}, err
	}

	dados := DadosNovoUtilizador{
		Username:        entrada.Username,
		Nome:            entrada.Nome,
		Email:           entrada.Email,
		SenhaTemporaria: senha,
		Papeis:          entrada.Papeis,
		ConfigurarOTP:   dominio.ExigeAutenticacaoForte(entrada.Papeis),
	}
	id, err := c.admin.CriarUtilizador(ctx, dados)
	if err != nil {
		return UtilizadorCriado{}, err
	}

	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.utilizador.criado",
		Entidade:   "utilizador",
		EntidadeID: id,
		Detalhe:    entrada.Username,
		OcorridoEm: c.agora(),
	}); err != nil {
		return UtilizadorCriado{}, err
	}

	// Notificação best-effort: falha de email não falha a criação nem vaza a senha.
	if err := c.notif.NotificarCriacao(ctx, entrada.Email, entrada.Nome, senha); err != nil {
		slog.Warn("falha ao notificar criação por email", "utilizador", id, "erro", err)
	}

	return UtilizadorCriado{ID: id, SenhaTemporaria: senha}, nil
}

// gerarSenhaTemporaria devolve uma senha aleatória segura (base64 url-safe de 18
// bytes ≈ 24 caracteres), adequada a uma credencial temporária.
func gerarSenhaTemporaria() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", erros.Novo(erros.CategoriaInterno, "falha ao gerar senha temporária")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
