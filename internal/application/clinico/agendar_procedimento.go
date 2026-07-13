package clinico

import (
	"context"
	"strings"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoAgendarProcedimento agenda um procedimento cirúrgico num episódio de
// cirurgia ambulatória aberto, validando catálogo e consentimento.
type CasoAgendarProcedimento struct {
	procedimentos  dominio.RepositorioProcedimentos
	episodios      dominio.RepositorioEpisodios
	consentimentos dominio.RepositorioConsentimentos
	catalogo       dominio.RepositorioCatalogoProcedimentos
	auditor        Auditor
	agora          func() time.Time
}

// NovoCasoAgendarProcedimento constrói o caso de uso.
func NovoCasoAgendarProcedimento(
	p dominio.RepositorioProcedimentos, e dominio.RepositorioEpisodios,
	c dominio.RepositorioConsentimentos, cat dominio.RepositorioCatalogoProcedimentos, a Auditor,
) *CasoAgendarProcedimento {
	return &CasoAgendarProcedimento{procedimentos: p, episodios: e, consentimentos: c, catalogo: cat, auditor: a, agora: time.Now}
}

// Executar valida episódio/catálogo/consentimento, cria o procedimento e audita.
func (uc *CasoAgendarProcedimento) Executar(ctx context.Context, actor string, dados DadosAgendarProcedimento) (DetalheProcedimento, error) {
	ep, err := uc.episodios.ObterPorID(ctx, dados.EpisodioID)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if ep.Tipo() != dominio.EpisodioCirurgiaAmbulatoria {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaConflito, "o episódio não é de cirurgia ambulatória")
	}
	if ep.Estado() != dominio.EstadoEpisodioAberto {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaConflito, "só é possível agendar procedimentos num episódio aberto")
	}
	cat, err := uc.catalogo.ObterPorCodigo(ctx, dados.Codigo)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if !cat.Activo {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaValidacao, "procedimento do catálogo inactivo")
	}
	cons, err := uc.consentimentos.ObterPorID(ctx, dados.ConsentimentoID)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if cons.DoenteID() != ep.DoenteID() {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaValidacao, "o consentimento não pertence ao doente do episódio")
	}
	anestesia, err := dominio.ParseAnestesia(dados.Anestesia)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if cat.RequerAnestesista && strings.TrimSpace(dados.AnestesistaID) == "" {
		return DetalheProcedimento{}, erros.Novo(erros.CategoriaValidacao, "este procedimento exige anestesista")
	}
	proc, err := dominio.NovoProcedimento(dominio.DadosNovoProcedimento{
		EpisodioID: dados.EpisodioID, Codigo: dados.Codigo, Descricao: dados.Descricao,
		Sala: dados.Sala, CirurgiaoID: dados.CirurgiaoID, AuxiliarID: dados.AuxiliarID,
		Anestesia: anestesia, AnestesistaID: dados.AnestesistaID, Observacoes: dados.Observacoes,
	}, cons)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	id, err := uc.procedimentos.Guardar(ctx, proc)
	if err != nil {
		return DetalheProcedimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.procedimento.agendado",
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
