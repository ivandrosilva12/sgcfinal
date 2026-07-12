package farmacia

import (
	"context"
	"fmt"
	"strings"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoDispensarReceita dispensa (parcial ou totalmente) uma receita: valida
// não-exceder e alergias, e consome stock por FEFO via o MotorDispensa (atómico).
type CasoDispensarReceita struct {
	receitas     dominio.RepositorioReceitas
	medicamentos dominio.RepositorioMedicamentos
	leitor       LeitorClinico
	motor        MotorDispensa
	auditor      Auditor
	agora        func() time.Time
}

func NovoCasoDispensarReceita(receitas dominio.RepositorioReceitas, medicamentos dominio.RepositorioMedicamentos, leitor LeitorClinico, motor MotorDispensa, aud Auditor) *CasoDispensarReceita {
	return &CasoDispensarReceita{receitas: receitas, medicamentos: medicamentos, leitor: leitor, motor: motor, auditor: aud, agora: time.Now}
}

func (c *CasoDispensarReceita) Executar(ctx context.Context, actor, receitaID string, dados DadosDispensa) (DetalheReceita, error) {
	receita, err := c.receitas.ObterPorID(ctx, receitaID)
	if err != nil {
		return DetalheReceita{}, err
	}
	efectivo := receita.EstadoEfectivo(c.agora())
	if efectivo != dominio.ReceitaEmitida && efectivo != dominio.ReceitaParcial {
		return DetalheReceita{}, erros.Novo(erros.CategoriaConflito, "esta receita não pode ser dispensada (expirada, anulada ou já dispensada)")
	}
	if len(dados.Itens) == 0 {
		return DetalheReceita{}, erros.Novo(erros.CategoriaValidacao, "indique pelo menos um item a dispensar")
	}

	for _, it := range dados.Itens {
		if err := receita.RegistarDispensa(it.MedicamentoID, it.Quantidade); err != nil {
			return DetalheReceita{}, err
		}
	}

	_, alergiasGraves, err := c.leitor.ObterContextoDoente(ctx, receita.DoenteID())
	if err != nil {
		return DetalheReceita{}, err
	}
	var alertas []string
	itensMotor := make([]ItemDispensa, 0, len(dados.Itens))
	for _, it := range dados.Itens {
		med, err := c.medicamentos.ObterPorID(ctx, it.MedicamentoID)
		if err != nil {
			return DetalheReceita{}, err
		}
		itensMotor = append(itensMotor, ItemDispensa(it))
		for _, a := range alergiasGraves {
			if med.CorrespondeSubstancia(a.Substancia) {
				alertas = append(alertas, fmt.Sprintf("%s (alergia %s a %s)", med.CodigoInterno(), a.Severidade, a.Substancia))
			}
		}
	}
	if len(alertas) > 0 {
		if !dados.IgnorarAlertaAlergia {
			return DetalheReceita{}, erros.Novo(erros.CategoriaRegraNegocio, "a dispensa colide com alergias graves do doente: "+strings.Join(alertas, "; "))
		}
		if strings.TrimSpace(dados.JustificacaoAlerta) == "" {
			return DetalheReceita{}, erros.Novo(erros.CategoriaValidacao, "é obrigatória uma justificação para ignorar o alerta de alergia")
		}
	}

	if _, err := c.motor.Dispensar(ctx, receita.Snapshot(), itensMotor, actor); err != nil {
		return DetalheReceita{}, err
	}

	detalheAud := ""
	if len(alertas) > 0 {
		detalheAud = "override alergia: " + dados.JustificacaoAlerta + " | alertas: " + strings.Join(alertas, "; ")
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.receita.dispensada", Entidade: "receita", EntidadeID: receitaID, Detalhe: detalheAud, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheReceita{}, err
	}
	final, err := c.receitas.ObterPorID(ctx, receitaID)
	if err != nil {
		return DetalheReceita{}, err
	}
	return paraDetalheReceita(final, c.agora()), nil
}
