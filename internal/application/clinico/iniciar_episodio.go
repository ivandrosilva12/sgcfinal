package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoIniciarEpisodio inicia um episódio clínico para um doente activo e audita.
type CasoIniciarEpisodio struct {
	episodios dominio.RepositorioEpisodios
	doentes   dominio.RepositorioDoentes
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoIniciarEpisodio constrói o caso de uso.
func NovoCasoIniciarEpisodio(ep dominio.RepositorioEpisodios, doentes dominio.RepositorioDoentes, aud Auditor) *CasoIniciarEpisodio {
	return &CasoIniciarEpisodio{episodios: ep, doentes: doentes, auditor: aud, agora: time.Now}
}

// Executar valida o doente (existe e activo), cria o episódio, persiste e audita.
func (c *CasoIniciarEpisodio) Executar(ctx context.Context, actor string, dados DadosNovoEpisodio) (DetalheEpisodio, error) {
	doente, err := c.doentes.ObterPorID(ctx, dados.DoenteID)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if doente.Estado() != dominio.EstadoActivo {
		return DetalheEpisodio{}, erros.Novo(erros.CategoriaConflito, "não é possível abrir um episódio a um doente que não está activo")
	}
	tipo, err := dominio.ParseTipoEpisodio(dados.Tipo)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	inicio := c.agora()
	if dados.Inicio != nil {
		inicio = *dados.Inicio
	}
	episodio, err := dominio.NovoEpisodio(dados.DoenteID, tipo, dados.EspecialidadeID, dados.MedicoID, inicio)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	id, err := c.episodios.Guardar(ctx, episodio)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.aberto",
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
