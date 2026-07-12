package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarAntecedente acrescenta um antecedente clínico a um doente e audita.
type CasoRegistarAntecedente struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoRegistarAntecedente constrói o caso de uso.
func NovoCasoRegistarAntecedente(repo dominio.RepositorioDoentes, aud Auditor) *CasoRegistarAntecedente {
	return &CasoRegistarAntecedente{repo: repo, auditor: aud, agora: time.Now}
}

// Executar valida e regista o antecedente no doente indicado.
func (c *CasoRegistarAntecedente) Executar(ctx context.Context, actor, doenteID string, dados DadosAntecedente) (DetalheDoente, error) {
	doente, err := c.repo.ObterPorID(ctx, doenteID)
	if err != nil {
		return DetalheDoente{}, err
	}
	tipo, err := dominio.ParseTipoAntecedente(dados.Tipo)
	if err != nil {
		return DetalheDoente{}, err
	}
	antecedente, err := dominio.NovoAntecedente(tipo, dados.Descricao, dados.CID, dados.DataInicio, dados.Activo, dados.Notas)
	if err != nil {
		return DetalheDoente{}, err
	}
	if err := doente.AdicionarAntecedente(antecedente); err != nil {
		return DetalheDoente{}, err
	}
	if _, err := c.repo.Guardar(ctx, doente); err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.antecedente.registado",
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
