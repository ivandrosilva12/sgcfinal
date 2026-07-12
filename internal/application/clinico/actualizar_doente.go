package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoActualizarDoente actualiza identificação, contactos e/ou grupo sanguíneo de
// um doente e audita a operação.
type CasoActualizarDoente struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoActualizarDoente constrói o caso de uso.
func NovoCasoActualizarDoente(repo dominio.RepositorioDoentes, aud Auditor) *CasoActualizarDoente {
	return &CasoActualizarDoente{repo: repo, auditor: aud, agora: time.Now}
}

// Executar aplica as alterações fornecidas (campos a nil ficam inalterados).
func (c *CasoActualizarDoente) Executar(ctx context.Context, actor, id string, dados DadosActualizarDoente) (DetalheDoente, error) {
	doente, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}

	if dados.Identificacao != nil {
		ident, err := construirIdentificacao(*dados.Identificacao)
		if err != nil {
			return DetalheDoente{}, err
		}
		if err := doente.AtualizarIdentificacao(ident); err != nil {
			return DetalheDoente{}, err
		}
	}
	if dados.Contactos != nil {
		contactos, err := construirContactos(*dados.Contactos)
		if err != nil {
			return DetalheDoente{}, err
		}
		if err := doente.AtualizarContactos(contactos); err != nil {
			return DetalheDoente{}, err
		}
	}
	if dados.GrupoSanguineo != nil {
		g, err := grupoOpcional(*dados.GrupoSanguineo)
		if err != nil {
			return DetalheDoente{}, err
		}
		doente.DefinirGrupoSanguineo(g)
	}

	if _, err := c.repo.Guardar(ctx, doente); err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.doente.actualizado",
		Entidade: "doente", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheDoente{}, err
	}

	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}
	return paraDetalhe(final), nil
}
