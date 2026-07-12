package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarAlergia acrescenta uma alergia a um doente e audita a operação.
type CasoRegistarAlergia struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoRegistarAlergia constrói o caso de uso.
func NovoCasoRegistarAlergia(repo dominio.RepositorioDoentes, aud Auditor) *CasoRegistarAlergia {
	return &CasoRegistarAlergia{repo: repo, auditor: aud, agora: time.Now}
}

// Executar valida e regista a alergia no doente indicado.
func (c *CasoRegistarAlergia) Executar(ctx context.Context, actor, doenteID string, dados DadosAlergia) (DetalheDoente, error) {
	doente, err := c.repo.ObterPorID(ctx, doenteID)
	if err != nil {
		return DetalheDoente{}, err
	}
	sev, err := dominio.ParseSeveridade(dados.Severidade)
	if err != nil {
		return DetalheDoente{}, err
	}
	alergia, err := dominio.NovaAlergia(dados.Substancia, sev, dados.ReaccaoTipica, dados.ConfirmadaEm, dados.Notas)
	if err != nil {
		return DetalheDoente{}, err
	}
	if err := doente.AdicionarAlergia(alergia); err != nil {
		return DetalheDoente{}, err
	}
	if _, err := c.repo.Guardar(ctx, doente); err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.alergia.registada",
		Entidade: "doente", EntidadeID: doenteID, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheDoente{}, err
	}
	final, err := c.repo.ObterPorID(ctx, doenteID)
	if err != nil {
		return DetalheDoente{}, err
	}
	return paraDetalhe(final), nil
}
