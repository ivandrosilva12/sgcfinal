package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoObterEHR monta a projecção de leitura EHR (doente + alergias/antecedentes +
// episódios paginados) e audita o acesso.
type CasoObterEHR struct {
	doentes   dominio.RepositorioDoentes
	episodios dominio.RepositorioEpisodios
	triagem   LeitorTriagem
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoObterEHR constrói o caso de uso.
func NovoCasoObterEHR(doentes dominio.RepositorioDoentes, ep dominio.RepositorioEpisodios, triagem LeitorTriagem, aud Auditor) *CasoObterEHR {
	return &CasoObterEHR{doentes: doentes, episodios: ep, triagem: triagem, auditor: aud, agora: time.Now}
}

// Executar carrega o doente e os seus episódios paginados, preenche a
// prioridade de triagem em lote quando autorizado (ADR-034/ADR-037), audita e
// devolve o EHR.
func (c *CasoObterEHR) Executar(ctx context.Context, actor string, papeis []string, doenteID string, filtroEpisodios FiltroEpisodios) (EHR, error) {
	doente, err := c.doentes.ObterPorID(ctx, doenteID)
	if err != nil {
		return EHR{}, err
	}
	filtroEpisodios.DoenteID = doenteID
	if filtroEpisodios.Limite <= 0 {
		filtroEpisodios.Limite = limiteDefault
	}
	if filtroEpisodios.Limite > limiteMaximo {
		filtroEpisodios.Limite = limiteMaximo
	}
	if filtroEpisodios.Deslocamento < 0 {
		filtroEpisodios.Deslocamento = 0
	}
	pagina, err := c.episodios.ListarPorDoente(ctx, filtroEpisodios)
	if err != nil {
		return EHR{}, err
	}
	// Enriquecimento antes da auditoria (deliberado): falha aqui devolve erro sem registar o acesso.
	if err := preencherPrioridadesTriagem(ctx, c.triagem, papeis, pagina.Itens); err != nil {
		return EHR{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.ehr.consultado",
		Entidade: "doente", EntidadeID: doenteID, OcorridoEm: c.agora(),
	}); err != nil {
		return EHR{}, err
	}
	return EHR{Doente: paraDetalhe(doente), Episodios: pagina}, nil
}
