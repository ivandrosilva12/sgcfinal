package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoEmitirRequisicao emite uma requisição de análises para um episódio aberto e
// cria um resultado PENDENTE por análise pedida (é o que povoa a fila do laboratório).
type CasoEmitirRequisicao struct {
	requisicoes dominio.RepositorioRequisicoes
	analises    dominio.RepositorioAnalises
	leitor      LeitorClinico
	auditor     Auditor
	agora       func() time.Time
}

// NovoCasoEmitirRequisicao constrói o caso de uso.
func NovoCasoEmitirRequisicao(
	r dominio.RepositorioRequisicoes, a dominio.RepositorioAnalises,
	l LeitorClinico, aud Auditor,
) *CasoEmitirRequisicao {
	return &CasoEmitirRequisicao{requisicoes: r, analises: a, leitor: l, auditor: aud, agora: time.Now}
}

// Executar valida o doente e o episódio (ACL), valida cada código contra o catálogo,
// e grava requisição + resultados numa só transacção. O médico requisitante é o
// actor autenticado — nunca um campo do corpo do pedido.
func (uc *CasoEmitirRequisicao) Executar(ctx context.Context, actor string, dados DadosEmitirRequisicao) (DetalheRequisicao, error) {
	activo, err := uc.leitor.DoenteActivo(ctx, dados.DoenteID)
	if err != nil {
		return DetalheRequisicao{}, err
	}
	if !activo {
		return DetalheRequisicao{}, erros.Novo(erros.CategoriaRegraNegocio,
			"não é possível requisitar análises para um doente inexistente ou inactivo")
	}
	aberto, err := uc.leitor.EpisodioAbertoDoDoente(ctx, dados.EpisodioID, dados.DoenteID)
	if err != nil {
		return DetalheRequisicao{}, err
	}
	if !aberto {
		return DetalheRequisicao{}, erros.Novo(erros.CategoriaConflito,
			"só é possível requisitar análises num episódio aberto do doente")
	}
	prioridade, err := dominio.ParsePrioridade(dados.Prioridade)
	if err != nil {
		return DetalheRequisicao{}, err
	}

	itens := make([]dominio.ItemRequisicao, 0, len(dados.Itens))
	for _, i := range dados.Itens {
		itens = append(itens, dominio.ItemRequisicao{
			CodigoAnalise: i.CodigoAnalise, Observacoes: i.Observacoes,
		})
	}
	req, err := dominio.NovaRequisicao(dominio.DadosNovaRequisicao{
		EpisodioID: dados.EpisodioID, DoenteID: dados.DoenteID,
		MedicoRequisitanteID: actor, Prioridade: prioridade, Itens: itens,
	})
	if err != nil {
		return DetalheRequisicao{}, err
	}

	// Um resultado PENDENTE por item. A unidade é a do catálogo (fonte de verdade):
	// o resultado guarda-a para que a leitura clínica não dependa de o catálogo ser
	// alterado mais tarde. Códigos inexistentes ou inactivos são rejeitados aqui —
	// já normalizados pelo agregado, pelo que a pesquisa é pelo código canónico.
	resultados := make([]*dominio.Resultado, 0, len(req.Itens()))
	for _, item := range req.Itens() {
		analise, err := uc.analises.ObterPorCodigo(ctx, item.CodigoAnalise)
		if err != nil {
			return DetalheRequisicao{}, err
		}
		if !analise.Activo() {
			return DetalheRequisicao{}, erros.Novo(erros.CategoriaValidacao,
				"análise inactiva no catálogo: "+item.CodigoAnalise)
		}
		res, err := dominio.NovoResultado("pendente-de-id", item.CodigoAnalise, analise.Unidade())
		if err != nil {
			return DetalheRequisicao{}, err
		}
		resultados = append(resultados, res)
	}

	id, err := uc.requisicoes.Emitir(ctx, req, resultados)
	if err != nil {
		return DetalheRequisicao{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.requisicao.emitida",
		Entidade: "requisicao", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheRequisicao{}, err
	}
	final, err := uc.requisicoes.ObterPorID(ctx, id)
	if err != nil {
		return DetalheRequisicao{}, err
	}
	return paraDetalheRequisicao(final), nil
}
