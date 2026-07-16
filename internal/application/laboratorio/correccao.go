package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoCorrigirResultado corrige um resultado validado: arquiva o original em
// CONCLUIDA e cria um novo VALIDADA que o substitui, reavaliando o valor crítico
// contra o catálogo e notificando o médico se o valor corrigido for crítico.
type CasoCorrigirResultado struct {
	resultados  dominio.RepositorioResultados
	requisicoes dominio.RepositorioRequisicoes
	analises    dominio.RepositorioAnalises
	contactos   ResolvedorContacto
	notificador NotificadorCritico
	auditor     Auditor
	agora       func() time.Time
}

// NovoCasoCorrigirResultado constrói o caso de uso.
func NovoCasoCorrigirResultado(
	res dominio.RepositorioResultados, req dominio.RepositorioRequisicoes,
	an dominio.RepositorioAnalises, c ResolvedorContacto, n NotificadorCritico, a Auditor,
) *CasoCorrigirResultado {
	return &CasoCorrigirResultado{
		resultados: res, requisicoes: req, analises: an,
		contactos: c, notificador: n, auditor: a, agora: time.Now,
	}
}

// Executar corrige o resultado. O corrector é o sujeito autenticado (nunca do corpo).
func (uc *CasoCorrigirResultado) Executar(ctx context.Context, actor, resultadoID string, dados DadosCorrigirResultado) (DetalheResultado, error) {
	original, err := uc.resultados.ObterPorID(ctx, resultadoID)
	if err != nil {
		return DetalheResultado{}, err
	}
	an, err := uc.analises.ObterPorCodigo(ctx, original.Snapshot().CodigoAnalise)
	if err != nil {
		return DetalheResultado{}, err
	}
	critico := an.AvaliarCritico(dados.Valor)
	novo, err := original.Corrigir(actor, dados.Valor, dados.Observacoes, critico, uc.agora())
	if err != nil {
		return DetalheResultado{}, err
	}
	novoID, err := uc.resultados.Corrigir(ctx, novo, original)
	if err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.resultado.corrigido",
		Entidade: "resultado", EntidadeID: novoID, OcorridoEm: uc.agora(),
		Detalhe: "corrige o resultado " + resultadoID,
	}); err != nil {
		return DetalheResultado{}, err
	}
	sn := novo.Snapshot()
	sn.ID = novoID
	if critico {
		alertarValorCritico(ctx, uc.requisicoes, uc.contactos, uc.notificador, uc.auditor,
			uc.agora, actor, novoID, sn.RequisicaoID, sn.CodigoAnalise, sn.Valor)
	}
	return paraDetalheResultado(dominio.ReconstruirResultado(sn)), nil
}
