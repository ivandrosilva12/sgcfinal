package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoIniciarConsulta consome uma chegada TRIADO da fila clínica (BC Recepção) e
// abre o episódio CONSULTA correspondente, atomicamente (ADR-036). Só o médico
// atribuído à chegada pode iniciar; a guarda corre aqui (erro claro) e novamente
// no CAS da transacção (fecha a corrida entre a leitura e a escrita).
type CasoIniciarConsulta struct {
	recepcao   LeitorRecepcao
	consumidor ConsumidorChegadas
	doentes    dominio.RepositorioDoentes
	episodios  dominio.RepositorioEpisodios
	auditor    Auditor
	agora      func() time.Time
}

// NovoCasoIniciarConsulta constrói o caso de uso.
func NovoCasoIniciarConsulta(recepcao LeitorRecepcao, consumidor ConsumidorChegadas,
	doentes dominio.RepositorioDoentes, episodios dominio.RepositorioEpisodios, aud Auditor) *CasoIniciarConsulta {
	return &CasoIniciarConsulta{recepcao: recepcao, consumidor: consumidor,
		doentes: doentes, episodios: episodios, auditor: aud, agora: time.Now}
}

// Executar valida a chegada (TRIADO, do médico actor) e o doente (activo), cria o
// episódio CONSULTA e consome a chegada na mesma transacção, audita nos dois
// contextos e devolve o episódio aberto.
func (c *CasoIniciarConsulta) Executar(ctx context.Context, actor, chegadaID string) (DetalheEpisodio, error) {
	ch, err := c.recepcao.ChegadaTriada(ctx, chegadaID)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if actor != ch.MedicoID {
		return DetalheEpisodio{}, erros.Novo(erros.CategoriaProibido, "só o médico atribuído pode iniciar a consulta")
	}
	doente, err := c.doentes.ObterPorID(ctx, ch.DoenteID)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if doente.Estado() != dominio.EstadoActivo {
		return DetalheEpisodio{}, erros.Novo(erros.CategoriaConflito, "não é possível abrir um episódio a um doente que não está activo")
	}
	episodio, err := dominio.NovoEpisodio(ch.DoenteID, dominio.EpisodioConsulta, ch.EspecialidadeID, actor, c.agora())
	if err != nil {
		return DetalheEpisodio{}, err
	}
	id, err := c.consumidor.ConsumirEIniciar(ctx, chegadaID, actor, episodio)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.aberto",
		Entidade: "episodio", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.chegada.consulta_iniciada",
		Entidade: "chegada", EntidadeID: chegadaID, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheEpisodio{}, err
	}
	final, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	return paraDetalheEpisodio(final), nil
}
