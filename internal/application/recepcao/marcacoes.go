// internal/application/recepcao/marcacoes.go
package recepcao

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoMarcar cria uma marcação dentro de uma janela livre.
type CasoMarcar struct {
	marcacoes dominio.RepositorioMarcacoes
	janelas   dominio.RepositorioJanelas
	doentes   LeitorDoente
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoMarcar constrói o caso de uso.
func NovoCasoMarcar(m dominio.RepositorioMarcacoes, j dominio.RepositorioJanelas, d LeitorDoente, a Auditor) *CasoMarcar {
	return &CasoMarcar{marcacoes: m, janelas: j, doentes: d, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste (usado só em testes).
func (uc *CasoMarcar) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar valida o doente (ACL), verifica a disponibilidade e persiste a marcação. O
// actor (quem marca) é o sujeito autenticado e vai no registo de auditoria — não é
// guardado na marcação.
func (uc *CasoMarcar) Executar(ctx context.Context, actor string, dados DadosMarcar) (DetalheMarcacao, error) {
	activo, err := uc.doentes.DoenteActivo(ctx, dados.DoenteID)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	if !activo {
		return DetalheMarcacao{}, erros.Novo(erros.CategoriaRegraNegocio,
			"o doente não existe ou não está activo")
	}
	m, err := dominio.NovaMarcacao(dados.DoenteID, dados.MedicoID, dados.EspecialidadeID, dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	janelas, err := uc.janelas.ListarPorMedicoIntervalo(ctx, dados.MedicoID, dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	activas, err := uc.marcacoes.ListarActivasPorMedicoIntervalo(ctx, dados.MedicoID, dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	if err := dominio.VerificarDisponibilidade(janelas, activas, dados.EspecialidadeID, dados.Inicio, dados.Fim, uc.agora()); err != nil {
		return DetalheMarcacao{}, err
	}
	id, err := uc.marcacoes.Guardar(ctx, m)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	m = dominio.ReconstruirMarcacao(comIDMarcacao(m.Snapshot(), id))
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.marcacao.criada",
		Entidade: "marcacao", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheMarcacao{}, err
	}
	return paraDetalheMarcacao(m), nil
}

// CasoRemarcar remarca uma marcação para um novo intervalo, preservando a original.
type CasoRemarcar struct {
	marcacoes dominio.RepositorioMarcacoes
	janelas   dominio.RepositorioJanelas
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoRemarcar constrói o caso de uso.
func NovoCasoRemarcar(m dominio.RepositorioMarcacoes, j dominio.RepositorioJanelas, a Auditor) *CasoRemarcar {
	return &CasoRemarcar{marcacoes: m, janelas: j, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoRemarcar) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar transita a original para REMARCADA e cria uma nova MARCADA no novo horário,
// numa única transacção. A disponibilidade do novo horário é verificada excluindo a
// própria original (senão a marcação colidiria consigo mesma).
func (uc *CasoRemarcar) Executar(ctx context.Context, actor, marcacaoID string, dados DadosRemarcar) (DetalheMarcacao, error) {
	original, err := uc.marcacoes.ObterPorID(ctx, marcacaoID)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	nova, err := original.Remarcar(dados.Inicio, dados.Fim, uc.agora())
	if err != nil {
		return DetalheMarcacao{}, err
	}
	janelas, err := uc.janelas.ListarPorMedicoIntervalo(ctx, nova.MedicoID(), dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	activas, err := uc.marcacoes.ListarActivasPorMedicoIntervalo(ctx, nova.MedicoID(), dados.Inicio, dados.Fim)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	activas = semAMarcacao(activas, marcacaoID)
	if err := dominio.VerificarDisponibilidade(janelas, activas, nova.EspecialidadeID(), dados.Inicio, dados.Fim, uc.agora()); err != nil {
		return DetalheMarcacao{}, err
	}
	novoID, err := uc.marcacoes.Remarcar(ctx, original, nova)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	nova = dominio.ReconstruirMarcacao(comIDMarcacao(nova.Snapshot(), novoID))
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.marcacao.remarcada",
		Entidade: "marcacao", EntidadeID: novoID, OcorridoEm: uc.agora(),
		Detalhe: "remarca_de: " + marcacaoID,
	}); err != nil {
		return DetalheMarcacao{}, err
	}
	return paraDetalheMarcacao(nova), nil
}

// CasoCancelar cancela uma marcação com motivo.
type CasoCancelar struct {
	marcacoes dominio.RepositorioMarcacoes
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoCancelar constrói o caso de uso.
func NovoCasoCancelar(m dominio.RepositorioMarcacoes, a Auditor) *CasoCancelar {
	return &CasoCancelar{marcacoes: m, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoCancelar) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar cancela a marcação e audita (o motivo vai no detalhe: um cancelamento sem
// razão registada não é auditável).
func (uc *CasoCancelar) Executar(ctx context.Context, actor, marcacaoID, motivo string) (DetalheMarcacao, error) {
	m, err := uc.marcacoes.ObterPorID(ctx, marcacaoID)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	if err := m.Cancelar(motivo, uc.agora()); err != nil {
		return DetalheMarcacao{}, err
	}
	if err := uc.marcacoes.Transitar(ctx, m); err != nil {
		return DetalheMarcacao{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.marcacao.cancelada",
		Entidade: "marcacao", EntidadeID: marcacaoID, OcorridoEm: uc.agora(),
		Detalhe: "motivo: " + motivo,
	}); err != nil {
		return DetalheMarcacao{}, err
	}
	return paraDetalheMarcacao(m), nil
}

// CasoRegistarFalta regista a falta (no-show) de uma marcação.
type CasoRegistarFalta struct {
	marcacoes dominio.RepositorioMarcacoes
	auditor   Auditor
	agora     func() time.Time
}

// NovoCasoRegistarFalta constrói o caso de uso.
func NovoCasoRegistarFalta(m dominio.RepositorioMarcacoes, a Auditor) *CasoRegistarFalta {
	return &CasoRegistarFalta{marcacoes: m, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoRegistarFalta) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar regista a falta (só depois da hora, invariante do agregado) e audita.
func (uc *CasoRegistarFalta) Executar(ctx context.Context, actor, marcacaoID string) (DetalheMarcacao, error) {
	m, err := uc.marcacoes.ObterPorID(ctx, marcacaoID)
	if err != nil {
		return DetalheMarcacao{}, err
	}
	if err := m.RegistarFalta(uc.agora()); err != nil {
		return DetalheMarcacao{}, err
	}
	if err := uc.marcacoes.Transitar(ctx, m); err != nil {
		return DetalheMarcacao{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.marcacao.faltou",
		Entidade: "marcacao", EntidadeID: marcacaoID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheMarcacao{}, err
	}
	return paraDetalheMarcacao(m), nil
}

// comIDMarcacao devolve uma cópia do snapshot com o id preenchido.
func comIDMarcacao(s dominio.SnapshotMarcacao, id string) dominio.SnapshotMarcacao {
	s.ID = id
	return s
}

// semAMarcacao devolve a lista sem a marcação de id dado (usada na remarcação para não
// contar a própria original como sobreposição).
func semAMarcacao(activas []dominio.Marcacao, id string) []dominio.Marcacao {
	out := activas[:0]
	for i := range activas {
		if activas[i].ID() != id {
			out = append(out, activas[i])
		}
	}
	return out
}
