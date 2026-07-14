package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoIniciarProcedimento transita um procedimento AGENDADO para EM_CURSO.
//
// O início é o ponto de não-retorno do acto cirúrgico: é aqui — e não na
// conclusão nem no cancelamento — que a invariante-estrela é revalidada. Entre o
// agendamento e o início pode passar tempo, e o mundo muda: o doente pode revogar
// o consentimento (direito LPDP, nunca bloqueável) e o episódio pode ser fechado.
type CasoIniciarProcedimento struct {
	procedimentos  dominio.RepositorioProcedimentos
	episodios      dominio.RepositorioEpisodios
	consentimentos dominio.RepositorioConsentimentos
	auditor        Auditor
	agora          func() time.Time
}

// NovoCasoIniciarProcedimento constrói o caso de uso.
func NovoCasoIniciarProcedimento(
	p dominio.RepositorioProcedimentos, e dominio.RepositorioEpisodios,
	c dominio.RepositorioConsentimentos, a Auditor,
) *CasoIniciarProcedimento {
	return &CasoIniciarProcedimento{procedimentos: p, episodios: e, consentimentos: c, auditor: a, agora: time.Now}
}

// Executar carrega, revalida o consentimento e o episódio, inicia, persiste e audita.
func (uc *CasoIniciarProcedimento) Executar(ctx context.Context, actor, id string) (DetalheProcedimento, error) {
	p, err := uc.procedimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	cons, err := uc.consentimentos.ObterPorID(ctx, p.ConsentimentoID())
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if cons.Finalidade() != dominio.FinalidadeCirurgia || !cons.TemAnexo() || !cons.EstaVigente() {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaRegraNegocio,
			"o consentimento cirúrgico deixou de ser válido (exige finalidade CIRURGIA, anexo e estar vigente); não é possível iniciar o procedimento")
	}
	ep, err := uc.episodios.ObterPorID(ctx, p.EpisodioID())
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if ep.Estado() != dominio.EstadoEpisodioAberto {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaConflito,
			"só é possível iniciar procedimentos num episódio aberto")
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
