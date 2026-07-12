package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoGerirEstadoDoente aplica transições de ciclo de vida (desactivação, óbito)
// e audita cada operação.
type CasoGerirEstadoDoente struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoGerirEstadoDoente constrói o caso de uso.
func NovoCasoGerirEstadoDoente(repo dominio.RepositorioDoentes, aud Auditor) *CasoGerirEstadoDoente {
	return &CasoGerirEstadoDoente{repo: repo, auditor: aud, agora: time.Now}
}

// Desactivar coloca o doente em INACTIVO com um motivo.
func (c *CasoGerirEstadoDoente) Desactivar(ctx context.Context, actor, id, motivo string) (DetalheDoente, error) {
	return c.transicionar(ctx, actor, id, "clinico.doente.desactivado", func(d *dominio.Doente) error {
		return d.Desactivar(motivo, c.agora())
	})
}

// DeclararFalecido coloca o doente em FALECIDO com a data de óbito e a causa.
func (c *CasoGerirEstadoDoente) DeclararFalecido(ctx context.Context, actor, id string, data time.Time, causaCID string) (DetalheDoente, error) {
	return c.transicionar(ctx, actor, id, "clinico.doente.falecido", func(d *dominio.Doente) error {
		return d.DeclararFalecido(data, causaCID)
	})
}

// transicionar hidrata o doente, aplica a transição, persiste, audita e devolve o
// detalhe actualizado.
func (c *CasoGerirEstadoDoente) transicionar(ctx context.Context, actor, id, accao string, aplicar func(*dominio.Doente) error) (DetalheDoente, error) {
	doente, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}
	if err := aplicar(doente); err != nil {
		return DetalheDoente{}, err
	}
	if _, err := c.repo.Guardar(ctx, doente); err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: accao, Entidade: "doente", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheDoente{}, err
	}
	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}
	return paraDetalhe(final), nil
}
