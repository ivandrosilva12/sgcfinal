package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarAnalise regista uma análise no catálogo.
type CasoRegistarAnalise struct {
	analises dominio.RepositorioAnalises
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoRegistarAnalise constrói o caso de uso.
func NovoCasoRegistarAnalise(a dominio.RepositorioAnalises, aud Auditor) *CasoRegistarAnalise {
	return &CasoRegistarAnalise{analises: a, auditor: aud, agora: time.Now}
}

// Executar valida, persiste e audita o registo da análise.
func (uc *CasoRegistarAnalise) Executar(ctx context.Context, actor string, dados DadosNovaAnalise) (DetalheAnalise, error) {
	a, err := dominio.NovaAnalise(dados.Codigo, dados.Nome, dados.Unidade, dados.Intervalos, dados.ValoresCriticos)
	if err != nil {
		return DetalheAnalise{}, err
	}
	if err := uc.analises.Guardar(ctx, a); err != nil {
		return DetalheAnalise{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.analise.registada",
		Entidade: "analise", EntidadeID: a.Codigo(), OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheAnalise{}, err
	}
	return paraDetalheAnalise(a), nil
}

// CasoListarAnalises lista o catálogo.
type CasoListarAnalises struct {
	analises dominio.RepositorioAnalises
}

// NovoCasoListarAnalises constrói o caso de uso.
func NovoCasoListarAnalises(a dominio.RepositorioAnalises) *CasoListarAnalises {
	return &CasoListarAnalises{analises: a}
}

// Executar devolve o catálogo de análises.
func (uc *CasoListarAnalises) Executar(ctx context.Context) ([]ResumoAnalise, error) {
	return uc.analises.Listar(ctx)
}
