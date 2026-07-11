package identidade

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// Perfil é o DTO de saída do perfil do utilizador autenticado.
type Perfil struct {
	KeycloakID string   `json:"keycloak_id"`
	Nome       string   `json:"nome"`
	Email      string   `json:"email"`
	Telefone   string   `json:"telefone,omitempty"`
	BI         string   `json:"bi,omitempty"`
	Activo     bool     `json:"activo"`
	Papeis     []string `json:"papeis"`
}

// CasoObterPerfil faz o JIT provisioning do utilizador a partir da sessão,
// audita o acesso e devolve o perfil persistido.
type CasoObterPerfil struct {
	utilizadores dominio.RepositorioUtilizadores
	auditor      Auditor
	agora        func() time.Time
}

// NovoCasoObterPerfil constrói o caso de uso com as suas dependências.
func NovoCasoObterPerfil(r dominio.RepositorioUtilizadores, a Auditor) *CasoObterPerfil {
	return &CasoObterPerfil{utilizadores: r, auditor: a, agora: time.Now}
}

// Executar provisiona/actualiza o perfil (upsert a partir da sessão do Keycloak,
// que é a fonte de verdade de nome/email/papéis), regista o acesso na auditoria
// e devolve o perfil tal como persistido (reflectindo telefone/BI eventualmente
// já preenchidos localmente).
func (c *CasoObterPerfil) Executar(ctx context.Context, s dominio.Sessao) (Perfil, error) {
	u, err := dominio.NovoUtilizador(s.Sujeito, s.Nome, s.Email, "", "", s.Papeis)
	if err != nil {
		return Perfil{}, err
	}
	if err := c.utilizadores.GuardarComPapeis(ctx, u); err != nil {
		return Perfil{}, err
	}

	persistido, err := c.utilizadores.ObterPorID(ctx, s.Sujeito)
	if err != nil {
		return Perfil{}, err
	}

	reg := auditoria.Registo{
		Actor:      s.Sujeito,
		Accao:      "identidade.perfil.consultado",
		Entidade:   "utilizador",
		EntidadeID: s.Sujeito,
		OcorridoEm: c.agora(),
	}
	if err := c.auditor.Registar(ctx, reg); err != nil {
		return Perfil{}, err
	}

	return paraPerfil(persistido), nil
}

// paraPerfil converte o agregado de domínio no DTO de saída.
func paraPerfil(u *dominio.Utilizador) Perfil {
	papeis := make([]string, 0, len(u.Papeis))
	for _, p := range u.Papeis {
		papeis = append(papeis, string(p))
	}
	return Perfil{
		KeycloakID: u.KeycloakID,
		Nome:       u.Nome,
		Email:      u.Email,
		Telefone:   u.Telefone,
		BI:         u.BI,
		Activo:     u.Activo,
		Papeis:     papeis,
	}
}
