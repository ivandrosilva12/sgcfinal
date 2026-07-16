package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// alertarValorCritico resolve o telefone do médico requisitante e envia o SMS de
// valor crítico, auditando sempre o resultado da tentativa. Best-effort: qualquer
// falha (resolver contacto, enviar, obter requisição) é engolida — a validação ou a
// correcção já estão persistidas e não devem falhar por causa de um alerta. É isto
// que cumpre "SMS auditado": a prova é o registo de auditoria, não a entrega.
func alertarValorCritico(
	ctx context.Context,
	requisicoes dominio.RepositorioRequisicoes,
	contactos ResolvedorContacto,
	notificador NotificadorCritico,
	auditor Auditor,
	agora func() time.Time,
	actor, resultadoID, requisicaoID, codigoAnalise, valor string,
) {
	detalhe := ""
	req, err := requisicoes.ObterPorID(ctx, requisicaoID)
	switch {
	case err != nil:
		detalhe = "falha: requisição " + requisicaoID + " não encontrada"
	default:
		medicoID := req.Snapshot().MedicoRequisitanteID
		telefone, ok, cErr := contactos.ContactoClinico(ctx, medicoID)
		switch {
		case cErr != nil:
			detalhe = "falha ao resolver contacto do médico " + medicoID
		case !ok || telefone == "":
			detalhe = "médico " + medicoID + " sem telefone registado; alerta não enviado"
		default:
			if nErr := notificador.NotificarValorCritico(ctx, telefone, codigoAnalise, valor); nErr != nil {
				detalhe = "falha no envio do SMS ao médico " + medicoID
			} else {
				detalhe = "SMS enviado ao médico " + medicoID
			}
		}
	}
	_ = auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.valor_critico.notificado",
		Entidade: "resultado", EntidadeID: resultadoID, OcorridoEm: agora(),
		Detalhe: detalhe,
	})
}
