// internal/application/recepcao/consultas.go
package recepcao

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
)

// CasoListarAgenda lê a agenda de um médico num intervalo: janelas de disponibilidade
// e marcações (de todos os estados). Leitura pura — sem auditoria.
type CasoListarAgenda struct {
	janelas   dominio.RepositorioJanelas
	marcacoes dominio.RepositorioMarcacoes
}

// NovoCasoListarAgenda constrói o caso de uso.
func NovoCasoListarAgenda(j dominio.RepositorioJanelas, m dominio.RepositorioMarcacoes) *CasoListarAgenda {
	return &CasoListarAgenda{janelas: j, marcacoes: m}
}

// Executar devolve a agenda do médico entre `de` e `ate`.
func (uc *CasoListarAgenda) Executar(ctx context.Context, medicoID string, de, ate time.Time) (Agenda, error) {
	js, err := uc.janelas.ListarPorMedicoIntervalo(ctx, medicoID, de, ate)
	if err != nil {
		return Agenda{}, err
	}
	ms, err := uc.marcacoes.ListarPorMedicoIntervalo(ctx, medicoID, de, ate)
	if err != nil {
		return Agenda{}, err
	}
	detalhes := make([]DetalheJanela, 0, len(js))
	for i := range js {
		detalhes = append(detalhes, paraDetalheJanela(&js[i]))
	}
	return Agenda{Janelas: detalhes, Marcacoes: ms}, nil
}

// CasoListarMarcacoesDoente lê as marcações de um doente. Leitura pura — sem auditoria.
type CasoListarMarcacoesDoente struct {
	marcacoes dominio.RepositorioMarcacoes
}

// NovoCasoListarMarcacoesDoente constrói o caso de uso.
func NovoCasoListarMarcacoesDoente(m dominio.RepositorioMarcacoes) *CasoListarMarcacoesDoente {
	return &CasoListarMarcacoesDoente{marcacoes: m}
}

// Executar devolve as marcações do doente.
func (uc *CasoListarMarcacoesDoente) Executar(ctx context.Context, doenteID string) ([]ResumoMarcacao, error) {
	return uc.marcacoes.ListarPorDoente(ctx, doenteID)
}
