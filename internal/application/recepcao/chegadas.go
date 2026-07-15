// internal/application/recepcao/chegadas.go
package recepcao

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoRegistarWalkIn regista a chegada de um doente sem marcação.
type CasoRegistarWalkIn struct {
	chegadas dominio.RepositorioChegadas
	doentes  LeitorDoente
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoRegistarWalkIn constrói o caso de uso.
func NovoCasoRegistarWalkIn(c dominio.RepositorioChegadas, d LeitorDoente, a Auditor) *CasoRegistarWalkIn {
	return &CasoRegistarWalkIn{chegadas: c, doentes: d, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoRegistarWalkIn) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar valida o doente (ACL) e regista a chegada walk-in. O actor vai na auditoria.
func (uc *CasoRegistarWalkIn) Executar(ctx context.Context, actor string, dados DadosWalkIn) (DetalheChegada, error) {
	activo, err := uc.doentes.DoenteActivo(ctx, dados.DoenteID)
	if err != nil {
		return DetalheChegada{}, err
	}
	if !activo {
		return DetalheChegada{}, erros.Novo(erros.CategoriaRegraNegocio, "o doente não existe ou não está activo")
	}
	c, err := dominio.NovaChegadaWalkIn(dados.DoenteID, dados.EspecialidadeID, uc.agora())
	if err != nil {
		return DetalheChegada{}, err
	}
	id, err := uc.chegadas.Guardar(ctx, c)
	if err != nil {
		return DetalheChegada{}, err
	}
	c = dominio.ReconstruirChegada(comIDChegada(c.Snapshot(), id))
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "recepcao.chegada.walkin",
		Entidade: "chegada", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheChegada{}, err
	}
	return paraDetalheChegada(c), nil
}

// CasoChamar chama uma chegada da fila (AGUARDA → CHAMADO).
type CasoChamar struct {
	chegadas dominio.RepositorioChegadas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoChamar constrói o caso de uso.
func NovoCasoChamar(c dominio.RepositorioChegadas, a Auditor) *CasoChamar {
	return &CasoChamar{chegadas: c, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoChamar) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar chama a chegada e audita.
func (uc *CasoChamar) Executar(ctx context.Context, actor, chegadaID string) (DetalheChegada, error) {
	return transitarChegada(ctx, uc.chegadas, uc.auditor, chegadaID, actor, "recepcao.chegada.chamada", uc.agora(),
		func(c *dominio.Chegada) error { return c.Chamar(uc.agora()) })
}

// CasoRegistarDesistencia regista a desistência de uma chegada (AGUARDA → DESISTIU).
type CasoRegistarDesistencia struct {
	chegadas dominio.RepositorioChegadas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoRegistarDesistencia constrói o caso de uso.
func NovoCasoRegistarDesistencia(c dominio.RepositorioChegadas, a Auditor) *CasoRegistarDesistencia {
	return &CasoRegistarDesistencia{chegadas: c, auditor: a, agora: time.Now}
}

// DefinirRelogio injecta um relógio de teste.
func (uc *CasoRegistarDesistencia) DefinirRelogio(f func() time.Time) { uc.agora = f }

// Executar regista a desistência e audita.
func (uc *CasoRegistarDesistencia) Executar(ctx context.Context, actor, chegadaID string) (DetalheChegada, error) {
	return transitarChegada(ctx, uc.chegadas, uc.auditor, chegadaID, actor, "recepcao.chegada.desistiu", uc.agora(),
		func(c *dominio.Chegada) error { return c.RegistarDesistencia(uc.agora()) })
}

// CasoListarFila lê a fila de espera (chegadas em AGUARDA) por especialidade.
type CasoListarFila struct {
	chegadas dominio.RepositorioChegadas
}

// NovoCasoListarFila constrói o caso de uso.
func NovoCasoListarFila(c dominio.RepositorioChegadas) *CasoListarFila {
	return &CasoListarFila{chegadas: c}
}

// Executar devolve a fila; especialidade vazia = todas.
func (uc *CasoListarFila) Executar(ctx context.Context, especialidadeID string) ([]ResumoChegada, error) {
	return uc.chegadas.ListarFila(ctx, especialidadeID)
}

// transitarChegada é o padrão comum de Chamar/Desistir: obter → transição de domínio →
// Transitar (CAS) → auditar.
func transitarChegada(ctx context.Context, chegadas dominio.RepositorioChegadas, auditor Auditor,
	chegadaID, actor, accao string, em time.Time, transicao func(*dominio.Chegada) error) (DetalheChegada, error) {
	c, err := chegadas.ObterPorID(ctx, chegadaID)
	if err != nil {
		return DetalheChegada{}, err
	}
	if err := transicao(c); err != nil {
		return DetalheChegada{}, err
	}
	if err := chegadas.Transitar(ctx, c); err != nil {
		return DetalheChegada{}, err
	}
	if err := auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: accao,
		Entidade: "chegada", EntidadeID: chegadaID, OcorridoEm: em,
	}); err != nil {
		return DetalheChegada{}, err
	}
	return paraDetalheChegada(c), nil
}

// comIDChegada devolve uma cópia do snapshot com o id preenchido.
func comIDChegada(s dominio.SnapshotChegada, id string) dominio.SnapshotChegada {
	s.ID = id
	return s
}
