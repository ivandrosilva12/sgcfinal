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
	triagem   LeitorTriagem
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoObterEpisodio constrói o caso de uso.
func NovoCasoObterEpisodio(ep dominio.RepositorioEpisodios, triagem LeitorTriagem, aud Auditor) *CasoObterEpisodio {
	return &CasoObterEpisodio{episodios: ep, triagem: triagem, auditor: aud, agora: time.Now}
}

// Executar carrega o episódio, audita a consulta e devolve o detalhe. A
// triagem de origem só é lida e anexada se algum papel do actor autorizar
// vê-la (minimização LPDP, ADR-034/ADR-037).
func (c *CasoObterEpisodio) Executar(ctx context.Context, actor string, papeis []string, id string) (DetalheEpisodio, error) {
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
	det := paraDetalheEpisodio(episodio)
	if temPapelLeituraTriagem(papeis) {
		tr, ok, err := c.triagem.TriagemDoEpisodio(ctx, id)
		if err != nil {
			return DetalheEpisodio{}, err
		}
		if ok {
			det.Triagem = &tr
		}
	}
	return det, nil
}
