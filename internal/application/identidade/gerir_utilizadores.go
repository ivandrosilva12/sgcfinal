package identidade

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// CasoListarUtilizadores lista utilizadores (leitura; sem auditoria).
type CasoListarUtilizadores struct{ admin AdminIdentidade }

// NovoCasoListarUtilizadores constrói o caso de uso.
func NovoCasoListarUtilizadores(a AdminIdentidade) *CasoListarUtilizadores {
	return &CasoListarUtilizadores{admin: a}
}

// Executar devolve a lista de utilizadores segundo o filtro.
func (c *CasoListarUtilizadores) Executar(ctx context.Context, f FiltroUtilizadores) ([]ResumoUtilizador, error) {
	return c.admin.ListarUtilizadores(ctx, f)
}

// CasoObterUtilizador devolve o detalhe de um utilizador (leitura).
type CasoObterUtilizador struct{ admin AdminIdentidade }

// NovoCasoObterUtilizador constrói o caso de uso.
func NovoCasoObterUtilizador(a AdminIdentidade) *CasoObterUtilizador {
	return &CasoObterUtilizador{admin: a}
}

// Executar devolve o detalhe do utilizador com o id indicado.
func (c *CasoObterUtilizador) Executar(ctx context.Context, id string) (DetalheUtilizador, error) {
	return c.admin.ObterUtilizador(ctx, id)
}

// CasoAtribuirPapel atribui um papel a um utilizador e audita a acção.
type CasoAtribuirPapel struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoAtribuirPapel constrói o caso de uso.
func NovoCasoAtribuirPapel(a AdminIdentidade, aud Auditor) *CasoAtribuirPapel {
	return &CasoAtribuirPapel{admin: a, auditor: aud, agora: time.Now}
}

// Executar valida o papel, delega no Keycloak e regista a auditoria.
func (c *CasoAtribuirPapel) Executar(ctx context.Context, actor, id string, papel dominio.Papel) error {
	if !dominio.PapelValido(string(papel)) {
		return erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPapelInvalido))
	}
	if err := c.admin.AtribuirPapel(ctx, id, papel); err != nil {
		return err
	}
	return c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.papel.atribuido",
		Entidade:   "utilizador",
		EntidadeID: id,
		Detalhe:    string(papel),
		OcorridoEm: c.agora(),
	})
}

// CasoRevogarPapel revoga um papel de um utilizador e audita a acção.
type CasoRevogarPapel struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoRevogarPapel constrói o caso de uso.
func NovoCasoRevogarPapel(a AdminIdentidade, aud Auditor) *CasoRevogarPapel {
	return &CasoRevogarPapel{admin: a, auditor: aud, agora: time.Now}
}

// Executar valida o papel, delega no Keycloak e regista a auditoria.
func (c *CasoRevogarPapel) Executar(ctx context.Context, actor, id string, papel dominio.Papel) error {
	if !dominio.PapelValido(string(papel)) {
		return erros.Novo(erros.CategoriaValidacao, i18n.T(i18n.MsgPapelInvalido))
	}
	if err := c.admin.RevogarPapel(ctx, id, papel); err != nil {
		return err
	}
	return c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "identidade.papel.revogado",
		Entidade:   "utilizador",
		EntidadeID: id,
		Detalhe:    string(papel),
		OcorridoEm: c.agora(),
	})
}

// CasoDefinirActivo activa/desactiva um utilizador e audita a acção.
type CasoDefinirActivo struct {
	admin   AdminIdentidade
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoDefinirActivo constrói o caso de uso.
func NovoCasoDefinirActivo(a AdminIdentidade, aud Auditor) *CasoDefinirActivo {
	return &CasoDefinirActivo{admin: a, auditor: aud, agora: time.Now}
}

// Executar aplica o estado no Keycloak e regista a auditoria. Ao desactivar,
// revoga também as sessões activas do utilizador (deixa de poder renovar tokens).
func (c *CasoDefinirActivo) Executar(ctx context.Context, actor, id string, activo bool) error {
	if err := c.admin.DefinirActivo(ctx, id, activo); err != nil {
		return err
	}
	accao := "identidade.utilizador.desactivado"
	if activo {
		accao = "identidade.utilizador.activado"
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      accao,
		Entidade:   "utilizador",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	}); err != nil {
		return err
	}
	if !activo {
		if err := c.admin.RevogarSessoes(ctx, id); err != nil {
			return err
		}
		return c.auditor.Registar(ctx, auditoria.Registo{
			Actor:      actor,
			Accao:      "identidade.sessoes.revogadas",
			Entidade:   "utilizador",
			EntidadeID: id,
			OcorridoEm: c.agora(),
		})
	}
	return nil
}
