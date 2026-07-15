// internal/application/recepcao/janelas.go
package recepcao

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoDefinirJanela cria uma janela de disponibilidade de um médico.
type CasoDefinirJanela struct {
	janelas dominio.RepositorioJanelas
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoDefinirJanela constrói o caso de uso.
func NovoCasoDefinirJanela(j dominio.RepositorioJanelas, a Auditor) *CasoDefinirJanela {
	return &CasoDefinirJanela{janelas: j, auditor: a, agora: time.Now}
}

// Executar valida e persiste a janela, e audita. O actor (quem define) é o sujeito
// autenticado.
func (uc *CasoDefinirJanela) Executar(ctx context.Context, actor string, dados DadosDefinirJanela) (DetalheJanela, error) {
	j, err := dominio.NovaJanela(dados.MedicoID, dados.EspecialidadeID, dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheJanela{}, err
	}
	id, err := uc.janelas.Guardar(ctx, j)
	if err != nil {
		return DetalheJanela{}, err
	}
	j = dominio.ReconstruirJanela(comIDJanela(j.Snapshot(), id))
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.janela.definida",
		Entidade: "janela", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheJanela{}, err
	}
	return paraDetalheJanela(j), nil
}

// CasoRemoverJanela remove uma janela, desde que não tenha marcações activas dentro.
type CasoRemoverJanela struct {
	janelas   dominio.RepositorioJanelas
	marcacoes dominio.RepositorioMarcacoes
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoRemoverJanela constrói o caso de uso.
func NovoCasoRemoverJanela(j dominio.RepositorioJanelas, m dominio.RepositorioMarcacoes, a Auditor) *CasoRemoverJanela {
	return &CasoRemoverJanela{janelas: j, marcacoes: m, auditor: a, agora: time.Now}
}

// Executar remove a janela se não houver marcações MARCADA no seu intervalo. Remover
// uma janela com marcações activas deixaria consultas sem cobertura de agenda.
func (uc *CasoRemoverJanela) Executar(ctx context.Context, actor, janelaID string) error {
	j, err := uc.janelas.ObterPorID(ctx, janelaID)
	if err != nil {
		return err
	}
	activas, err := uc.marcacoes.ListarActivasPorMedicoIntervalo(ctx, j.MedicoID(), j.Inicio(), j.Fim())
	if err != nil {
		return err
	}
	if len(activas) > 0 {
		return erros.Novo(erros.CategoriaConflito,
			"a janela tem marcações activas e não pode ser removida")
	}
	if err := uc.janelas.Remover(ctx, janelaID); err != nil {
		return err
	}
	return uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.janela.removida",
		Entidade: "janela", EntidadeID: janelaID, OcorridoEm: uc.agora(),
	})
}

// comIDJanela devolve uma cópia do snapshot com o id preenchido.
func comIDJanela(s dominio.SnapshotJanela, id string) dominio.SnapshotJanela {
	s.ID = id
	return s
}
