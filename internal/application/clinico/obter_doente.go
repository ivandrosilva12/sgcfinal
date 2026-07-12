package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoObterDoente devolve o detalhe de um doente e audita o acesso (dados de
// saúde são de acesso auditável).
type CasoObterDoente struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoObterDoente constrói o caso de uso.
func NovoCasoObterDoente(repo dominio.RepositorioDoentes, aud Auditor) *CasoObterDoente {
	return &CasoObterDoente{repo: repo, auditor: aud, agora: time.Now}
}

// Executar carrega o doente por id, audita a consulta e devolve o detalhe.
func (c *CasoObterDoente) Executar(ctx context.Context, actor, id string) (DetalheDoente, error) {
	doente, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "clinico.doente.consultado",
		Entidade:   "doente",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheDoente{}, err
	}
	return paraDetalhe(doente), nil
}
