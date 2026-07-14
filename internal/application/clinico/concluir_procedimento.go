package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoConcluirProcedimento transita um procedimento EM_CURSO para CONCLUIDO.
type CasoConcluirProcedimento struct {
	procedimentos dominio.RepositorioProcedimentos
	auditor       Auditor
	agora         func() time.Time
}

// NovoCasoConcluirProcedimento constrói o caso de uso.
func NovoCasoConcluirProcedimento(p dominio.RepositorioProcedimentos, a Auditor) *CasoConcluirProcedimento {
	return &CasoConcluirProcedimento{procedimentos: p, auditor: a, agora: time.Now}
}

// Executar carrega, conclui, persiste e audita.
func (uc *CasoConcluirProcedimento) Executar(ctx context.Context, actor, id string, dados DadosConcluirProcedimento) (DetalheProcedimento, error) {
	p, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if err := p.Concluir(uc.agora(), dados.Complicacoes, dados.Observacoes); err != nil {
		return DetalheProcedimento{}, err
	}
	if _, err := uc.procedimentos.Guardar(ctx, p); err != nil {
		return DetalheProcedimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.procedimento.concluido",
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
