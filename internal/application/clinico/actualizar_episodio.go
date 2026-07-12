package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoActualizarEpisodio actualiza a nota clínica e/ou os diagnósticos CID de um
// episódio aberto e audita.
type CasoActualizarEpisodio struct {
	episodios dominio.RepositorioEpisodios
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoActualizarEpisodio constrói o caso de uso.
func NovoCasoActualizarEpisodio(ep dominio.RepositorioEpisodios, aud Auditor) *CasoActualizarEpisodio {
	return &CasoActualizarEpisodio{episodios: ep, auditor: aud, agora: time.Now}
}

// Executar aplica as alterações fornecidas (campos a nil ficam inalterados).
func (c *CasoActualizarEpisodio) Executar(ctx context.Context, actor, id string, dados DadosActualizarEpisodio) (DetalheEpisodio, error) {
	episodio, err := c.episodios.ObterPorID(ctx, id)
	if err != nil {
		return DetalheEpisodio{}, err
	}
	if dados.Nota != nil {
		if err := episodio.ActualizarNota(construirNota(*dados.Nota)); err != nil {
			return DetalheEpisodio{}, err
		}
	}
	if dados.DiagnosticosCID != nil {
		if err := episodio.DefinirDiagnosticosCID(construirDiagnosticos(*dados.DiagnosticosCID)); err != nil {
			return DetalheEpisodio{}, err
		}
	}
	if _, err := c.episodios.Guardar(ctx, episodio); err != nil {
		return DetalheEpisodio{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.episodio.actualizado",
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
