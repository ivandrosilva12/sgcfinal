package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoCancelarProcedimento transita um procedimento EM_CURSO para CANCELADO.
type CasoCancelarProcedimento struct {
	procedimentos dominio.RepositorioProcedimentos
	auditor       Auditor
	agora         func() time.Time
}

// NovoCasoCancelarProcedimento constrói o caso de uso.
func NovoCasoCancelarProcedimento(p dominio.RepositorioProcedimentos, a Auditor) *CasoCancelarProcedimento {
	return &CasoCancelarProcedimento{procedimentos: p, auditor: a, agora: time.Now}
}

// Executar carrega, cancela (com motivo), persiste e audita.
func (uc *CasoCancelarProcedimento) Executar(ctx context.Context, actor, id, motivo string) (DetalheProcedimento, error) {
	p, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if err := p.Cancelar(uc.agora(), motivo); err != nil {
		return DetalheProcedimento{}, err
	}
	if _, err := uc.procedimentos.Guardar(ctx, p); err != nil {
		return DetalheProcedimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.procedimento.cancelado",
		Entidade: "procedimento", EntidadeID: id, OcorridoEm: uc.agora(), Detalhe: motivo,
	}); err != nil {
		return DetalheProcedimento{}, err
	}
	final, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	return paraDetalheProcedimento(final), nil
}
