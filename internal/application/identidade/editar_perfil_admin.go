package identidade

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoEditarPerfilAdmin permite a um administrador actualizar o perfil local
// (telefone/BI) de outro utilizador. Garante a linha local — hidratando-a a
// partir do Keycloak se ainda não existir — aplica a validação de domínio,
// persiste, audita e devolve o perfil actualizado.
type CasoEditarPerfilAdmin struct {
	admin        AdminIdentidade
	utilizadores dominio.RepositorioUtilizadores
	auditor      Auditor
	agora        func() time.Time
}

// NovoCasoEditarPerfilAdmin constrói o caso de uso.
func NovoCasoEditarPerfilAdmin(a AdminIdentidade, r dominio.RepositorioUtilizadores, aud Auditor) *CasoEditarPerfilAdmin {
	return &CasoEditarPerfilAdmin{admin: a, utilizadores: r, auditor: aud, agora: time.Now}
}

// Executar actualiza telefone/BI do utilizador `id`. `telefone`/`bi` a nil
// preservam o valor actual; string vazia limpa; valor presente é validado.
func (c *CasoEditarPerfilAdmin) Executar(ctx context.Context, actor, id string, telefone, bi *string) (Perfil, error) {
	persistido, err := c.garantirLinha(ctx, id)
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
	if err := c.utilizadores.AtualizarContacto(ctx, id, persistido.Telefone, persistido.BI); err != nil {
		return Perfil{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.perfil.actualizado",
		Entidade:   "utilizador",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	}); err != nil {
		return Perfil{}, err
	}

	final, err := c.utilizadores.ObterPorID(ctx, id)
	if err != nil {
		return Perfil{}, err
	}
	return paraPerfil(final), nil
}

// garantirLinha devolve a linha local do utilizador; se não existir, hidrata-a a
// partir do Keycloak (fonte de verdade de nome/email/papéis).
func (c *CasoEditarPerfilAdmin) garantirLinha(ctx context.Context, id string) (*dominio.Utilizador, error) {
	persistido, err := c.utilizadores.ObterPorID(ctx, id)
	if err == nil {
		return persistido, nil
	}
	if erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		return nil, err
	}
	det, err := c.admin.ObterUtilizador(ctx, id)
	if err != nil {
		return nil, err
	}
	base, err := dominio.NovoUtilizador(det.ID, det.Nome, det.Email, "", "", dominio.PapeisDe(det.Papeis))
	if err != nil {
		return nil, err
	}
	if err := c.utilizadores.GuardarComPapeis(ctx, base); err != nil {
		return nil, err
	}
	return c.utilizadores.ObterPorID(ctx, id)
}
