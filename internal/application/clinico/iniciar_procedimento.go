package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoIniciarProcedimento transita um procedimento AGENDADO para EM_CURSO.
type CasoIniciarProcedimento struct {
	procedimentos dominio.RepositorioProcedimentos
	auditor       Auditor
	agora         func() time.Time
}

// NovoCasoIniciarProcedimento constrói o caso de uso.
func NovoCasoIniciarProcedimento(p dominio.RepositorioProcedimentos, a Auditor) *CasoIniciarProcedimento {
	return &CasoIniciarProcedimento{procedimentos: p, auditor: a, agora: time.Now}
}

// Executar carrega, inicia, persiste e audita.
func (uc *CasoIniciarProcedimento) Executar(ctx context.Context, actor, id string) (DetalheProcedimento, error) {
	p, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if err := p.Iniciar(uc.agora()); err != nil {
		return DetalheProcedimento{}, err
	}
	if _, err := uc.procedimentos.Guardar(ctx, p); err != nil {
		return DetalheProcedimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.procedimento.iniciado",
		Entidade: "procedimento", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheProcedimento{}, err
	}
	final, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	return paraDetalheProcedimento(final), nil
}
