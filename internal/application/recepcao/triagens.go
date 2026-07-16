// internal/application/recepcao/triagens.go
package recepcao

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoObterTriagem lê a triagem de uma chegada.
type CasoObterTriagem struct {
	triagens dominio.RepositorioTriagens
}

// NovoCasoObterTriagem constrói o caso de uso.
func NovoCasoObterTriagem(t dominio.RepositorioTriagens) *CasoObterTriagem {
	return &CasoObterTriagem{triagens: t}
}

// Executar devolve o detalhe da triagem de uma chegada.
func (uc *CasoObterTriagem) Executar(ctx context.Context, chegadaID string) (DetalheTriagem, error) {
	t, err := uc.triagens.ObterPorChegada(ctx, chegadaID)
	if err != nil {
		return DetalheTriagem{}, err
	}
	return paraDetalheTriagem(t), nil
}

// CasoListarFilaClinica lê a fila clínica (chegadas TRIADO por prioridade).
type CasoListarFilaClinica struct {
	triagens dominio.RepositorioTriagens
}

// NovoCasoListarFilaClinica constrói o caso de uso.
func NovoCasoListarFilaClinica(t dominio.RepositorioTriagens) *CasoListarFilaClinica {
	return &CasoListarFilaClinica{triagens: t}
}

// Executar devolve a fila clínica; médico vazio = todos.
func (uc *CasoListarFilaClinica) Executar(ctx context.Context, medicoID string) ([]ResumoFilaClinica, error) {
	return uc.triagens.ListarFilaClinica(ctx, medicoID)
}

// CasoRegistarTriagem regista a triagem de uma chegada chamada: valida os sinais vitais
// e a prioridade, transita a chegada CHAMADO→TRIADO (atribuindo o médico ao walk-in) e
// cria a triagem, atomicamente.
type CasoRegistarTriagem struct {
	triagens dominio.RepositorioTriagens
	chegadas dominio.RepositorioChegadas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoRegistarTriagem constrói o caso de uso.
func NovoCasoRegistarTriagem(t dominio.RepositorioTriagens, c dominio.RepositorioChegadas, a Auditor) *CasoRegistarTriagem {
	return &CasoRegistarTriagem{triagens: t, chegadas: c, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoRegistarTriagem) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar valida os dados clínicos, transita a chegada e regista a triagem numa
// transacção coordenada. O actor é o enfermeiro triador. Uma triagem sobre uma chegada
// que já não está CHAMADO (segunda triagem) falha com Conflito.
func (uc *CasoRegistarTriagem) Executar(ctx context.Context, actor, chegadaID string, dados DadosTriagem) (DetalheTriagem, error) {
	chegada, err := uc.chegadas.ObterPorID(ctx, chegadaID)
	if err != nil {
		return DetalheTriagem{}, err
	}
	// valida os dados clínicos ANTES de tocar no estado da chegada
	sinais, err := dominio.NovosSinaisVitais(dominio.SinaisVitais{
		TensaoSistolica: dados.TensaoSistolica, TensaoDiastolica: dados.TensaoDiastolica,
		FrequenciaCardiaca: dados.FrequenciaCardiaca, Temperatura: dados.Temperatura,
		FrequenciaRespiratoria: dados.FrequenciaRespiratoria, SaturacaoO2: dados.SaturacaoO2,
		Dor: dados.Dor, Glicemia: dados.Glicemia, Peso: dados.Peso,
	})
	if err != nil {
		return DetalheTriagem{}, err
	}
	triagem, err := dominio.NovaTriagem(chegadaID, actor, dominio.PrioridadeManchester(dados.Prioridade), sinais, dados.Observacoes, uc.agora())
	if err != nil {
		return DetalheTriagem{}, err
	}
	if err := chegada.RegistarTriada(dados.MedicoID, uc.agora()); err != nil {
		return DetalheTriagem{}, err
	}
	id, err := uc.triagens.RegistarTriagem(ctx, triagem, chegada)
	if err != nil {
		return DetalheTriagem{}, err
	}
	triagem = dominio.ReconstruirTriagem(comIDTriagem(triagem.Snapshot(), id))
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.triagem.registada",
		Entidade: "triagem", EntidadeID: id, OcorridoEm: uc.agora(),
		Detalhe: "chegada: " + chegadaID + " prioridade: " + dados.Prioridade,
	}); err != nil {
		return DetalheTriagem{}, err
	}
	return paraDetalheTriagem(triagem), nil
}

// comIDTriagem devolve uma cópia do snapshot com o id preenchido.
func comIDTriagem(s dominio.SnapshotTriagem, id string) dominio.SnapshotTriagem {
	s.ID = id
	return s
}
