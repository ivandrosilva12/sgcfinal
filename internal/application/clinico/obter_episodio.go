package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoObterEpisodio devolve o detalhe de um episódio e audita o acesso.
type CasoObterEpisodio struct {
	episodios dominio.RepositorioEpisodios
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoObterEpisodio constrói o caso de uso.
func NovoCasoObterEpisodio(ep dominio.RepositorioEpisodios, aud Auditor) *CasoObterEpisodio {
	return &CasoObterEpisodio{episodios: ep, auditor: aud, agora: time.Now}
}

// Executar carrega o episódio, audita a consulta e devolve o detalhe.
func (c *CasoObterEpisodio) Executar(ctx context.Context, actor, id string) (DetalheEpisodio, error) {
	episodio, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.consultado",
		Entidade: "episodio", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheEpisodio{}, err
	}
	return paraDetalheEpisodio(episodio), nil
}
