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

const validadeReceitaDias = 30

// CasoEmitirReceita emite uma receita de um episódio, validando as alergias do
// doente (bloqueio com override auditado).
type CasoEmitirReceita struct {
	receitas     dominio.RepositorioReceitas
	medicamentos dominio.RepositorioMedicamentos
	leitor       LeitorClinico
	auditor      Auditor
	agora        func() time.Time
}

func NovoCasoEmitirReceita(receitas dominio.RepositorioReceitas, medicamentos dominio.RepositorioMedicamentos, leitor LeitorClinico, aud Auditor) *CasoEmitirReceita {
	return &CasoEmitirReceita{receitas: receitas, medicamentos: medicamentos, leitor: leitor, auditor: aud, agora: time.Now}
}

func (c *CasoEmitirReceita) Executar(ctx context.Context, actor string, dados DadosNovaReceita) (DetalheReceita, error) {
	activo, alergiasGraves, err := c.leitor.ObterContextoDoente(ctx, dados.DoenteID)
	if err != nil {
		return DetalheReceita{}, err
	}
	if !activo {
		return DetalheReceita{}, erros.Novo(erros.CategoriaConflito, "não é possível emitir uma receita a um doente que não está activo")
	}
	pertence, err := c.leitor.EpisodioDoDoente(ctx, dados.EpisodioID, dados.DoenteID)
	if err != nil {
		return DetalheReceita{}, err
	}
	if !pertence {
		return DetalheReceita{}, erros.Novo(erros.CategoriaValidacao, "o episódio indicado não pertence ao doente")
	}

	itens := make([]dominio.ItemReceita, 0, len(dados.Itens))
	var alertas []string
	for _, di := range dados.Itens {
		med, err := c.medicamentos.ObterPorID(ctx, di.MedicamentoID)
		if err != nil {
			return DetalheReceita{}, err
		}
		if !med.Activo() {
			return DetalheReceita{}, erros.Novo(erros.CategoriaConflito, "o medicamento "+med.CodigoInterno()+" está inactivo")
		}
		item, err := dominio.NovoItemReceita(di.MedicamentoID, di.Posologia, di.DuracaoDias, di.QuantidadePrescrita, di.Notas)
		if err != nil {
			return DetalheReceita{}, err
		}
		itens = append(itens, item)
		for _, a := range alergiasGraves {
			if med.CorrespondeSubstancia(a.Substancia) {
				alertas = append(alertas, fmt.Sprintf("%s (alergia %s a %s)", med.CodigoInterno(), a.Severidade, a.Substancia))
			}
		}
	}

	if len(alertas) > 0 {
		if !dados.IgnorarAlertaAlergia {
			return DetalheReceita{}, erros.Novo(erros.CategoriaRegraNegocio, "a prescrição colide com alergias graves do doente: "+strings.Join(alertas, "; "))
		}
		if strings.TrimSpace(dados.JustificacaoAlerta) == "" {
			return DetalheReceita{}, erros.Novo(erros.CategoriaValidacao, "é obrigatória uma justificação para ignorar o alerta de alergia")
		}
	}

	agora := c.agora()
	expira := agora.AddDate(0, 0, validadeReceitaDias)
	receita, err := dominio.NovaReceita(dados.EpisodioID, dados.DoenteID, actor, itens, dados.Notas, agora, expira)
	if err != nil {
		return DetalheReceita{}, err
	}
	id, err := c.receitas.Guardar(ctx, receita)
	if err != nil {
		return DetalheReceita{}, err
	}
	detalheAud := ""
	if len(alertas) > 0 {
		detalheAud = "override alergia: " + dados.JustificacaoAlerta + " | alertas: " + strings.Join(alertas, "; ")
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.receita.emitida", Entidade: "receita", EntidadeID: id, Detalhe: detalheAud, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheReceita{}, err
	}
	final, err := c.receitas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheReceita{}, err
	}
	return paraDetalheReceita(final, c.agora()), nil
}

// CasoAnularReceita anula uma receita e audita (motivo em Detalhe).
type CasoAnularReceita struct {
	receitas dominio.RepositorioReceitas
	auditor  Auditor
	agora    func() time.Time
}

func NovoCasoAnularReceita(receitas dominio.RepositorioReceitas, aud Auditor) *CasoAnularReceita {
	return &CasoAnularReceita{receitas: receitas, auditor: aud, agora: time.Now}
}
func (c *CasoAnularReceita) Executar(ctx context.Context, actor, id, motivo string) (DetalheReceita, error) {
	receita, err := c.receitas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheReceita{}, err
	}
	if err := receita.Anular(); err != nil {
		return DetalheReceita{}, err
	}
	if _, err := c.receitas.Guardar(ctx, receita); err != nil {
		return DetalheReceita{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.receita.anulada", Entidade: "receita", EntidadeID: id, Detalhe: motivo, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheReceita{}, err
	}
	final, err := c.receitas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheReceita{}, err
	}
	return paraDetalheReceita(final, c.agora()), nil
}

// CasoObterReceita devolve o detalhe de uma receita (com estado efectivo) e audita.
type CasoObterReceita struct {
	receitas dominio.RepositorioReceitas
	auditor  Auditor
	agora    func() time.Time
}

func NovoCasoObterReceita(receitas dominio.RepositorioReceitas, aud Auditor) *CasoObterReceita {
	return &CasoObterReceita{receitas: receitas, auditor: aud, agora: time.Now}
}
func (c *CasoObterReceita) Executar(ctx context.Context, actor, id string) (DetalheReceita, error) {
	receita, err := c.receitas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheReceita{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.receita.consultada", Entidade: "receita", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheReceita{}, err
	}
	return paraDetalheReceita(receita, c.agora()), nil
}

// CasoListarReceitas lista as receitas de um doente (não audita).
type CasoListarReceitas struct {
	receitas dominio.RepositorioReceitas
}

func NovoCasoListarReceitas(receitas dominio.RepositorioReceitas) *CasoListarReceitas {
	return &CasoListarReceitas{receitas: receitas}
}
func (c *CasoListarReceitas) Executar(ctx context.Context, filtro FiltroReceitas) (PaginaReceitas, error) {
	if filtro.Limite <= 0 {
		filtro.Limite = limiteDefault
	}
	if filtro.Limite > limiteMaximo {
		filtro.Limite = limiteMaximo
	}
	if filtro.Deslocamento < 0 {
		filtro.Deslocamento = 0
	}
	return c.receitas.ListarPorDoente(ctx, filtro)
}
