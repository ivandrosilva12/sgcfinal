// internal/adapters/pgrepo/janelas_repo.go
package pgrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioJanelas implementa dominio.RepositorioJanelas com pgx.
type RepositorioJanelas struct {
	pool *pgxpool.Pool
}

// NovoRepositorioJanelas constrói o repositório sobre o pool pgx.
func NovoRepositorioJanelas(pool *pgxpool.Pool) *RepositorioJanelas {
	return &RepositorioJanelas{pool: pool}
}

// Guardar insere a janela e devolve o id gerado.
func (r *RepositorioJanelas) Guardar(ctx context.Context, j *dominio.JanelaDisponibilidade) (string, error) {
	s := j.Snapshot()
	const q = `
INSERT INTO recepcao.janelas (medico_id, especialidade_id, inicio, fim)
VALUES ($1::uuid, $2::uuid, $3, $4)
RETURNING id::text`
	var id string
	if err := r.pool.QueryRow(ctx, q, s.MedicoID, s.EspecialidadeID, s.Inicio, s.Fim).Scan(&id); err != nil {
		return "", fmt.Errorf("guardar janela: %w", err)
	}
	return id, nil
}

// ObterPorID reconstrói a janela. NaoEncontrado se não existir.
func (r *RepositorioJanelas) ObterPorID(ctx context.Context, id string) (*dominio.JanelaDisponibilidade, error) {
	const q = `
SELECT id::text, medico_id::text, especialidade_id::text, inicio, fim, criado_em
FROM recepcao.janelas WHERE id=$1`
	var s dominio.SnapshotJanela
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.MedicoID, &s.EspecialidadeID, &s.Inicio, &s.Fim, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "janela não encontrada")
		}
		return nil, fmt.Errorf("obter janela: %w", err)
	}
	return dominio.ReconstruirJanela(s), nil
}

// ListarPorMedicoIntervalo devolve as janelas do médico que se sobrepõem a [de,ate].
func (r *RepositorioJanelas) ListarPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]dominio.JanelaDisponibilidade, error) {
	const q = `
SELECT id::text, medico_id::text, especialidade_id::text, inicio, fim, criado_em
FROM recepcao.janelas
WHERE medico_id=$1::uuid AND inicio < $3 AND $2 < fim
ORDER BY inicio`
	linhas, err := r.pool.Query(ctx, q, medicoID, de, ate)
	if err != nil {
		return nil, fmt.Errorf("listar janelas: %w", err)
	}
	defer linhas.Close()
	var out []dominio.JanelaDisponibilidade
	for linhas.Next() {
		var s dominio.SnapshotJanela
		if err := linhas.Scan(&s.ID, &s.MedicoID, &s.EspecialidadeID, &s.Inicio, &s.Fim, &s.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler janela: %w", err)
		}
		out = append(out, *dominio.ReconstruirJanela(s))
	}
	return out, linhas.Err()
}

// Remover apaga a janela. NaoEncontrado se não existir.
func (r *RepositorioJanelas) Remover(ctx context.Context, id string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM recepcao.janelas WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("remover janela: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return erros.Novo(erros.CategoriaNaoEncontrado, "janela não encontrada")
	}
	return nil
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioJanelas = (*RepositorioJanelas)(nil)
