// internal/adapters/pgrepo/integracao_inicio_consulta.go
//
// Adaptador de integração Recepção→Clínico (ADR-036): o único componente que
// conhece os dois contextos. Implementa as portas appclinico.LeitorRecepcao e
// appclinico.ConsumidorChegadas — a primeira escrita cross-BC do sistema, numa
// única transacção PG (INSERT do episódio + CAS da chegada). Camada 3: um
// adaptador pode importar ambos os domínios; a regra de dependência proíbe
// infra no domínio, não adaptadores multi-contexto.
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	domclinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	domrecepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// IntegracaoInicioConsulta implementa as portas de integração do início da consulta.
type IntegracaoInicioConsulta struct {
	pool      *pgxpool.Pool
	episodios *RepositorioEpisodios
}

// NovaIntegracaoInicioConsulta constrói o adaptador sobre o pool pgx.
func NovaIntegracaoInicioConsulta(pool *pgxpool.Pool) *IntegracaoInicioConsulta {
	return &IntegracaoInicioConsulta{pool: pool, episodios: NovoRepositorioEpisodios(pool)}
}

// ChegadaTriada devolve o retrato mínimo de uma chegada TRIADO. NaoEncontrado se
// a chegada não existir ou não estiver TRIADO (para o Clínico é a mesma resposta:
// não há nada na fila para consumir com este id).
func (a *IntegracaoInicioConsulta) ChegadaTriada(ctx context.Context, chegadaID string) (appclinico.ChegadaTriada, error) {
	const q = `SELECT doente_id::text, COALESCE(medico_id::text,''), especialidade_id::text
FROM recepcao.chegadas WHERE id=$1 AND estado='TRIADO'`
	var ct appclinico.ChegadaTriada
	err := a.pool.QueryRow(ctx, q, chegadaID).Scan(&ct.DoenteID, &ct.MedicoID, &ct.EspecialidadeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appclinico.ChegadaTriada{}, erros.Novo(erros.CategoriaNaoEncontrado, "chegada triada não encontrada")
		}
		return appclinico.ChegadaTriada{}, fmt.Errorf("obter chegada triada: %w", err)
	}
	return ct, nil
}

// ConsumirEIniciar insere o episódio e transita a chegada TRIADO→EM_CONSULTA numa
// única transacção. As regras correm no domínio da Recepção (estado + médico, com
// a categoria certa: 409/403); a guarda CAS do UPDATE fecha a corrida entre a
// leitura em transacção e a escrita.
func (a *IntegracaoInicioConsulta) ConsumirEIniciar(ctx context.Context, chegadaID, medicoID string, episodio *domclinico.EpisodioClinico) (string, error) {
	se := episodio.Snapshot()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção de início de consulta: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	// lê a chegada dentro da transacção e aplica as regras no domínio
	q := `SELECT ` + colunasChegada + ` FROM recepcao.chegadas WHERE id=$1`
	var sc domrecepcao.SnapshotChegada
	var estado string
	err = tx.QueryRow(ctx, q, chegadaID).Scan(&sc.ID, &sc.DoenteID, &sc.MarcacaoID,
		&sc.EspecialidadeID, &sc.MedicoID, &sc.EpisodioID, &sc.HoraChegada, &estado,
		&sc.CriadoEm, &sc.ActualizadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", erros.Novo(erros.CategoriaNaoEncontrado, "chegada não encontrada")
		}
		return "", fmt.Errorf("obter chegada: %w", err)
	}
	sc.Estado = domrecepcao.EstadoChegada(estado)
	chegada := domrecepcao.ReconstruirChegada(sc)
	if err := chegada.IniciarConsulta(medicoID, se.Inicio); err != nil {
		return "", err
	}

	epID, err := a.episodios.inserirEpisodio(ctx, tx, se)
	if err != nil {
		return "", err
	}

	scDepois := chegada.Snapshot()
	const upd = `UPDATE recepcao.chegadas
SET estado=$2, episodio_id=$3::uuid, actualizado_em=$4
WHERE id=$1 AND estado=$5 AND medico_id=$6::uuid`
	ct, err := tx.Exec(ctx, upd, scDepois.ID, string(scDepois.Estado), epID,
		scDepois.ActualizadoEm, string(scDepois.EstadoAnterior), medicoID)
	if err != nil {
		return "", fmt.Errorf("consumir chegada: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", erros.Novo(erros.CategoriaConflito,
			"o estado da chegada mudou entretanto; recarregue e repita a operação")
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar início de consulta: %w", err)
	}
	return epID, nil
}

// Garantias de conformidade com as portas.
var (
	_ appclinico.LeitorRecepcao     = (*IntegracaoInicioConsulta)(nil)
	_ appclinico.ConsumidorChegadas = (*IntegracaoInicioConsulta)(nil)
)
