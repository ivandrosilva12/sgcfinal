package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoValidarResultado valida o preliminar (PROCESSADA → VALIDADA) com segregação de
// funções, avalia o valor crítico contra o catálogo e, se crítico, notifica o médico
// requisitante por SMS (best-effort, auditado).
type CasoValidarResultado struct {
	resultados  dominio.RepositorioResultados
	requisicoes dominio.RepositorioRequisicoes
	analises    dominio.RepositorioAnalises
	contactos   ResolvedorContacto
	notificador NotificadorCritico
	auditor     Auditor
	agora       func() time.Time
}

// NovoCasoValidarResultado constrói o caso de uso.
func NovoCasoValidarResultado(
	res dominio.RepositorioResultados, req dominio.RepositorioRequisicoes,
	an dominio.RepositorioAnalises, c ResolvedorContacto, n NotificadorCritico, a Auditor,
) *CasoValidarResultado {
	return &CasoValidarResultado{
		resultados: res, requisicoes: req, analises: an,
		contactos: c, notificador: n, auditor: a, agora: time.Now,
	}
}

// Executar valida o resultado. O validador é o sujeito autenticado (nunca do corpo).
func (uc *CasoValidarResultado) Executar(ctx context.Context, actor, resultadoID string) (DetalheResultado, error) {
	res, err := uc.resultados.ObterPorID(ctx, resultadoID)
	if err != nil {
		return DetalheResultado{}, err
	}
	an, err := uc.analises.ObterPorCodigo(ctx, res.Snapshot().CodigoAnalise)
	if err != nil {
		return DetalheResultado{}, err
	}
	critico := an.AvaliarCritico(res.Valor())
	if err := res.Validar(actor, critico, uc.agora()); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.resultados.Transitar(ctx, res); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.resultado.validado",
		Entidade: "resultado", EntidadeID: resultadoID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheResultado{}, err
	}
	if critico {
		s := res.Snapshot()
		alertarValorCritico(ctx, uc.requisicoes, uc.contactos, uc.notificador, uc.auditor,
			uc.agora, actor, s.ID, s.RequisicaoID, s.CodigoAnalise, s.Valor)
	}
	return paraDetalheResultado(res), nil
}
