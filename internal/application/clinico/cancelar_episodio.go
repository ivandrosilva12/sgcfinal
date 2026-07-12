package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoCancelarEpisodio cancela um episódio aberto e audita (motivo em Detalhe).
type CasoCancelarEpisodio struct {
	episodios dominio.RepositorioEpisodios
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoCancelarEpisodio constrói o caso de uso.
func NovoCasoCancelarEpisodio(ep dominio.RepositorioEpisodios, aud Auditor) *CasoCancelarEpisodio {
	return &CasoCancelarEpisodio{episodios: ep, auditor: aud, agora: time.Now}
}

// Executar cancela o episódio identificado.
func (c *CasoCancelarEpisodio) Executar(ctx context.Context, actor, id, motivo string) (DetalheEpisodio, error) {
	episodio, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if err := episodio.Cancelar(c.agora()); err != nil {
		return DetalheEpisodio{}, err
	}
	if _, err := c.episodios.Guardar(ctx, episodio); err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.cancelado",
		Entidade: "episodio", EntidadeID: id, Detalhe: motivo, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheEpisodio{}, err
	}
	final, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	return paraDetalheEpisodio(final), nil
}
