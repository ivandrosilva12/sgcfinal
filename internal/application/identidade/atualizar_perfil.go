package identidade

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoAtualizarPerfil permite ao próprio utilizador actualizar o seu perfil local
// (telefone/BI). Garante a linha local (JIT a partir da sessão), aplica a
// validação de domínio, persiste, audita e devolve o perfil actualizado.
type CasoAtualizarPerfil struct {
	utilizadores dominio.RepositorioUtilizadores
	auditor      Auditor
	agora        func() time.Time
}

// NovoCasoAtualizarPerfil constrói o caso de uso.
func NovoCasoAtualizarPerfil(r dominio.RepositorioUtilizadores, a Auditor) *CasoAtualizarPerfil {
	return &CasoAtualizarPerfil{utilizadores: r, auditor: a, agora: time.Now}
}

// Executar actualiza telefone/BI do próprio. `telefone`/`bi` nil preservam o valor
// actual; string vazia limpa; valor presente é validado. Devolve o perfil final.
func (c *CasoAtualizarPerfil) Executar(ctx context.Context, s dominio.Sessao, telefone, bi *string) (Perfil, error) {
	// JIT: garantir a linha local a partir da sessão (fonte de verdade Keycloak).
	base, err := dominio.NovoUtilizador(s.Sujeito, s.Nome, s.Email, "", "", s.Papeis)
	if err != nil {
		return Perfil{}, err
	}
	if err := c.utilizadores.GuardarComPapeis(ctx, base); err != nil {
		return Perfil{}, err
	}

	persistido, err := c.utilizadores.ObterPorID(ctx, s.Sujeito)
	if err != nil {
		return Perfil{}, err
	}

	tel := persistido.Telefone
	if telefone != nil {
		tel = *telefone
	}
	doc := persistido.BI
	if bi != nil {
		doc = *bi
	}
	if err := persistido.AtualizarContacto(tel, doc); err != nil {
		return Perfil{}, err
	}
	if err := c.utilizadores.AtualizarContacto(ctx, s.Sujeito, persistido.Telefone, persistido.BI); err != nil {
		return Perfil{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      s.Sujeito,
		Accao:      "identidade.perfil.actualizado",
		Entidade:   "utilizador",
		EntidadeID: s.Sujeito,
		OcorridoEm: c.agora(),
	}); err != nil {
		return Perfil{}, err
	}

	final, err := c.utilizadores.ObterPorID(ctx, s.Sujeito)
	if err != nil {
		return Perfil{}, err
	}
	return paraPerfil(final), nil
}
