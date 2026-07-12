package identidade

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// VerificadorToken valida um token OIDC (JWT RS256: assinatura, issuer, audience,
// expiração) e devolve a Sessao derivada dos claims. É implementado pela camada
// de adaptadores (keycloak). Em caso de token inválido/ausente deve devolver um
// erros.ErroDominio de categoria NaoAutorizado (→ 401).
type VerificadorToken interface {
	Verificar(ctx context.Context, tokenBruto string) (dominio.Sessao, error)
}

// Auditor persiste registos de auditoria de forma append-only. Implementado por
// pgrepo (INSERT em auditoria.auditoria_eventos).
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}

// Notificador envia notificações ao utilizador (ex.: email). O envio é
// best-effort na perspectiva do chamador: um erro devolvido aqui é registado
// mas não falha a operação de negócio. Implementado por adapters/smtp.
type Notificador interface {
	NotificarCriacao(ctx context.Context, email, nome, senhaTemporaria string) error
	NotificarResetPassword(ctx context.Context, email, nome, senhaTemporaria string) error
}

// FiltroUtilizadores parametriza a listagem de utilizadores.
type FiltroUtilizadores struct {
	Termo        string // pesquisa por nome/email/username (opcional)
	Limite       int    // máximo de resultados (0 = default do adaptador)
	Deslocamento int    // paginação
}

// ResumoUtilizador é o DTO de um utilizador na gestão administrativa.
type ResumoUtilizador struct {
	ID     string   `json:"id"`
	Nome   string   `json:"nome"`
	Email  string   `json:"email"`
	Activo bool     `json:"activo"`
	Papeis []string `json:"papeis"`
}

// DetalheUtilizador é o detalhe de um utilizador (mesma forma que o resumo).
type DetalheUtilizador = ResumoUtilizador

// CriacaoUtilizador é a entrada do caso de uso de criação (dados do pedido).
type CriacaoUtilizador struct {
	Username string
	Nome     string
	Email    string
	Papeis   []dominio.Papel
}

// DadosNovoUtilizador são os dados enviados ao adaptador para criar o utilizador
// no Keycloak (já enriquecidos com a senha temporária e a política de OTP).
type DadosNovoUtilizador struct {
	Username        string
	Nome            string
	Email           string
	SenhaTemporaria string
	Papeis          []dominio.Papel
	ConfigurarOTP   bool
}

// UtilizadorCriado é a saída do caso de uso: id do Keycloak e senha temporária
// (devolvida uma única vez).
type UtilizadorCriado struct {
	ID              string `json:"id"`
	SenhaTemporaria string `json:"senha_temporaria"`
}

// SessaoActiva é o read-model de uma sessão Keycloak activa de um utilizador.
type SessaoActiva struct {
	ID           string    `json:"id"`
	IP           string    `json:"ip"`
	Inicio       time.Time `json:"inicio"`
	UltimoAcesso time.Time `json:"ultimo_acesso"`
	Clientes     []string  `json:"clientes"`
}

// AdminIdentidade é a porta de saída para a gestão de utilizadores/papéis no
// Keycloak (fonte de verdade). Implementada por adapters/keycloak.AdminCliente.
type AdminIdentidade interface {
	ListarUtilizadores(ctx context.Context, filtro FiltroUtilizadores) ([]ResumoUtilizador, error)
	ObterUtilizador(ctx context.Context, id string) (DetalheUtilizador, error)
	AtribuirPapel(ctx context.Context, id string, papel dominio.Papel) error
	RevogarPapel(ctx context.Context, id string, papel dominio.Papel) error
	DefinirActivo(ctx context.Context, id string, activo bool) error
	CriarUtilizador(ctx context.Context, dados DadosNovoUtilizador) (id string, err error)
	DefinirPasswordTemporaria(ctx context.Context, id, senha string) error
	ResetOTP(ctx context.Context, id string) error
	RevogarSessoes(ctx context.Context, id string) error
	ApagarUtilizador(ctx context.Context, id string) error
	ListarSessoes(ctx context.Context, userID string) ([]SessaoActiva, error)
	RevogarSessao(ctx context.Context, sessionID string) error
}

// CredencialReposta é a saída de um reset de password: a nova senha temporária,
// devolvida uma única vez.
type CredencialReposta struct {
	SenhaTemporaria string `json:"senha_temporaria"`
}
