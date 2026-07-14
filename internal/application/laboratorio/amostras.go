package laboratorio

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoColherAmostra regista a colheita da amostra (PENDENTE → COLHIDA).
type CasoColherAmostra struct {
	resultados dominio.RepositorioResultados
	auditor    Auditor
	agora      func() time.Time
}

// NovoCasoColherAmostra constrói o caso de uso.
func NovoCasoColherAmostra(r dominio.RepositorioResultados, a Auditor) *CasoColherAmostra {
	return &CasoColherAmostra{resultados: r, auditor: a, agora: time.Now}
}

// Executar transita o resultado e audita. O técnico é o sujeito autenticado.
func (uc *CasoColherAmostra) Executar(ctx context.Context, actor, resultadoID string) (DetalheResultado, error) {
	res, err := uc.resultados.ObterPorID(ctx, resultadoID)
	if err != nil {
		return DetalheResultado{}, err
	}
	if err := res.ColherAmostra(actor, uc.agora()); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.resultados.Transitar(ctx, res); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.amostra.colhida",
		Entidade: "resultado", EntidadeID: resultadoID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheResultado{}, err
	}
	return paraDetalheResultado(res), nil
}

// CasoRecusarAmostra recusa a amostra por inviabilidade (→ RECUSADA).
type CasoRecusarAmostra struct {
	resultados dominio.RepositorioResultados
	auditor    Auditor
	agora      func() time.Time
}

// NovoCasoRecusarAmostra constrói o caso de uso.
func NovoCasoRecusarAmostra(r dominio.RepositorioResultados, a Auditor) *CasoRecusarAmostra {
	return &CasoRecusarAmostra{resultados: r, auditor: a, agora: time.Now}
}

// Executar recusa a amostra com motivo e audita (o motivo vai no detalhe do registo:
// uma recusa sem razão registada não é auditável nem repetível).
func (uc *CasoRecusarAmostra) Executar(ctx context.Context, actor, resultadoID, motivo string) (DetalheResultado, error) {
	res, err := uc.resultados.ObterPorID(ctx, resultadoID)
	if err != nil {
		return DetalheResultado{}, err
	}
	if err := res.RecusarAmostra(motivo, uc.agora()); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.resultados.Transitar(ctx, res); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.amostra.recusada",
		Entidade: "resultado", EntidadeID: resultadoID, OcorridoEm: uc.agora(),
		Detalhe: "motivo: " + motivo,
	}); err != nil {
		return DetalheResultado{}, err
	}
	return paraDetalheResultado(res), nil
}

// CasoSubmeterPreliminar submete o resultado preliminar (COLHIDA → PROCESSADA).
type CasoSubmeterPreliminar struct {
	resultados dominio.RepositorioResultados
	auditor    Auditor
	agora      func() time.Time
}

// NovoCasoSubmeterPreliminar constrói o caso de uso.
func NovoCasoSubmeterPreliminar(r dominio.RepositorioResultados, a Auditor) *CasoSubmeterPreliminar {
	return &CasoSubmeterPreliminar{resultados: r, auditor: a, agora: time.Now}
}

// Executar submete o preliminar e audita. O submissor gravado é o actor autenticado
// — nunca um campo do corpo: é contra ele que a validação (Sprint 13) compara o
// patologista para impor a segregação de funções.
func (uc *CasoSubmeterPreliminar) Executar(ctx context.Context, actor, resultadoID string, dados DadosSubmeterPreliminar) (DetalheResultado, error) {
	res, err := uc.resultados.ObterPorID(ctx, resultadoID)
	if err != nil {
		return DetalheResultado{}, err
	}
	if err := res.SubmeterPreliminar(actor, dados.Valor, dados.Observacoes, uc.agora()); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.resultados.Transitar(ctx, res); err != nil {
		return DetalheResultado{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "laboratorio.resultado.preliminar_submetido",
		Entidade: "resultado", EntidadeID: resultadoID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheResultado{}, err
	}
	return paraDetalheResultado(res), nil
}
