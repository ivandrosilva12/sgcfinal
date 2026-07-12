package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoFecharEpisodio fecha um episódio (exige nota completa + ≥1 CID no domínio)
// e audita.
type CasoFecharEpisodio struct {
	episodios dominio.RepositorioEpisodios
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoFecharEpisodio constrói o caso de uso.
func NovoCasoFecharEpisodio(ep dominio.RepositorioEpisodios, aud Auditor) *CasoFecharEpisodio {
	return &CasoFecharEpisodio{episodios: ep, auditor: aud, agora: time.Now}
}

// Executar fecha o episódio identificado, registando o actor como fechado_por.
func (c *CasoFecharEpisodio) Executar(ctx context.Context, actor, id string) (DetalheEpisodio, error) {
	episodio, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if err := episodio.Fechar(actor, c.agora()); err != nil {
		return DetalheEpisodio{}, err
	}
	if _, err := c.episodios.Guardar(ctx, episodio); err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.fechado",
		Entidade: "episodio", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheEpisodio{}, err
	}
	final, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	return paraDetalheEpisodio(final), nil
}
