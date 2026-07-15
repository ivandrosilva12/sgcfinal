// internal/adapters/pgrepo/marcacoes_repo.go
package pgrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// codigoExclusaoPG é o SQLSTATE de violação de restrição EXCLUDE (sobreposição).
const codigoExclusaoPG = "23P01"

// RepositorioMarcacoes implementa dominio.RepositorioMarcacoes com pgx.
type RepositorioMarcacoes struct {
	pool *pgxpool.Pool
}

// NovoRepositorioMarcacoes constrói o repositório sobre o pool pgx.
func NovoRepositorioMarcacoes(pool *pgxpool.Pool) *RepositorioMarcacoes {
	return &RepositorioMarcacoes{pool: pool}
}

// colunasMarcacao é a lista SELECT reutilizada para reconstruir agregados.
const colunasMarcacao = `id::text, doente_id::text, medico_id::text, especialidade_id::text,
       inicio, fim, estado, COALESCE(motivo,''), COALESCE(remarca_de::text,''),
       criado_em, actualizado_em`

// Guardar insere a marcação e devolve o id gerado. Uma sobreposição negada pela
// EXCLUDE devolve Conflito.
func (r *RepositorioMarcacoes) Guardar(ctx context.Context, m *dominio.Marcacao) (string, error) {
	return r.inserir(ctx, r.pool, m)
}

// inserir insere uma marcação numa dada querier (pool ou tx).
func (r *RepositorioMarcacoes) inserir(ctx context.Context, q querier, m *dominio.Marcacao) (string, error) {
	s := m.Snapshot()
	const sql = `
INSERT INTO recepcao.marcacoes
    (doente_id, medico_id, especialidade_id, inicio, fim, estado, motivo, remarca_de)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, NULLIF($7,''), NULLIF($8,'')::uuid)
RETURNING id::text`
	var id string
	err := q.QueryRow(ctx, sql, s.DoenteID, s.MedicoID, s.EspecialidadeID, s.Inicio, s.Fim,
		string(s.Estado), s.Motivo, s.RemarcaDe).Scan(&id)
	if err != nil {
		if ehExclusao(err) {
			return "", erros.Novo(erros.CategoriaConflito, "o horário sobrepõe outra marcação do médico")
		}
		return "", fmt.Errorf("guardar marcação: %w", err)
	}
	return id, nil
}

// ObterPorID reconstrói a marcação. NaoEncontrado se não existir.
func (r *RepositorioMarcacoes) ObterPorID(ctx context.Context, id string) (*dominio.Marcacao, error) {
	q := `SELECT ` + colunasMarcacao + ` FROM recepcao.marcacoes WHERE id=$1`
	m, err := r.scanUma(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
		}
		return nil, err
	}
	return m, nil
}

// Transitar aplica a transição de estado com guarda compare-and-set: o UPDATE só se
// aplica se a linha ainda estiver no estado com que o agregado foi lido.
func (r *RepositorioMarcacoes) Transitar(ctx context.Context, m *dominio.Marcacao) error {
	s := m.Snapshot()
	if s.ID == "" {
		return erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
	}
	const q = `
UPDATE recepcao.marcacoes
SET estado=$2, motivo=NULLIF($3,''), actualizado_em=$4
WHERE id=$1 AND estado=$5`
	ct, err := r.pool.Exec(ctx, q, s.ID, string(s.Estado), s.Motivo, s.ActualizadoEm, string(s.EstadoAnterior))
	if err != nil {
		return fmt.Errorf("actualizar marcação: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return r.erroTransicaoFalhada(ctx, s.ID)
	}
	return nil
}

// Remarcar grava, numa única transacção, a original a passar a REMARCADA (compare-and-set)
// e a nova MARCADA. Devolve o id da nova.
func (r *RepositorioMarcacoes) Remarcar(ctx context.Context, original, nova *dominio.Marcacao) (string, error) {
	so := original.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção de remarcação: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	const upd = `UPDATE recepcao.marcacoes SET estado=$2, actualizado_em=$3 WHERE id=$1 AND estado=$4`
	ct, err := tx.Exec(ctx, upd, so.ID, string(so.Estado), so.ActualizadoEm, string(so.EstadoAnterior))
	if err != nil {
		return "", fmt.Errorf("marcar original como remarcada: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", r.erroTransicaoFalhada(ctx, so.ID)
	}
	novoID, err := r.inserir(ctx, tx, nova)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar remarcação: %w", err)
	}
	return novoID, nil
}

// ListarActivasPorMedicoIntervalo devolve os agregados das marcações MARCADA do médico
// que se sobrepõem a [de,ate].
func (r *RepositorioMarcacoes) ListarActivasPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]dominio.Marcacao, error) {
	q := `SELECT ` + colunasMarcacao + `
FROM recepcao.marcacoes
WHERE medico_id=$1::uuid AND estado='MARCADA' AND inicio < $3 AND $2 < fim
ORDER BY inicio`
	linhas, err := r.pool.Query(ctx, q, medicoID, de, ate)
	if err != nil {
		return nil, fmt.Errorf("listar marcações activas: %w", err)
	}
	defer linhas.Close()
	var out []dominio.Marcacao
	for linhas.Next() {
		m, err := r.scanUma(linhas)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, linhas.Err()
}

// ListarPorMedicoIntervalo devolve read-models de TODAS as marcações do médico que se
// sobrepõem a [de,ate].
func (r *RepositorioMarcacoes) ListarPorMedicoIntervalo(ctx context.Context, medicoID string, de, ate time.Time) ([]dominio.ResumoMarcacao, error) {
	const q = `
SELECT id::text, doente_id::text, medico_id::text, especialidade_id::text, estado,
       COALESCE(motivo,''), inicio, fim, criado_em
FROM recepcao.marcacoes
WHERE medico_id=$1::uuid AND inicio < $3 AND $2 < fim
ORDER BY inicio`
	return r.consultarResumos(ctx, q, medicoID, de, ate)
}

// ListarPorDoente devolve read-models das marcações de um doente.
func (r *RepositorioMarcacoes) ListarPorDoente(ctx context.Context, doenteID string) ([]dominio.ResumoMarcacao, error) {
	const q = `
SELECT id::text, doente_id::text, medico_id::text, especialidade_id::text, estado,
       COALESCE(motivo,''), inicio, fim, criado_em
FROM recepcao.marcacoes
WHERE doente_id=$1::uuid
ORDER BY inicio DESC`
	return r.consultarResumos(ctx, q, doenteID)
}

func (r *RepositorioMarcacoes) consultarResumos(ctx context.Context, q string, args ...any) ([]dominio.ResumoMarcacao, error) {
	linhas, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listar marcações: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoMarcacao{}
	for linhas.Next() {
		var rm dominio.ResumoMarcacao
		if err := linhas.Scan(&rm.ID, &rm.DoenteID, &rm.MedicoID, &rm.EspecialidadeID, &rm.Estado,
			&rm.Motivo, &rm.Inicio, &rm.Fim, &rm.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler marcação: %w", err)
		}
		out = append(out, rm)
	}
	return out, linhas.Err()
}

// erroTransicaoFalhada distingue 404 (linha inexistente) de 409 (estado mudou).
func (r *RepositorioMarcacoes) erroTransicaoFalhada(ctx context.Context, id string) error {
	var existe bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM recepcao.marcacoes WHERE id=$1)`, id).Scan(&existe); err != nil {
		return fmt.Errorf("verificar marcação: %w", err)
	}
	if !existe {
		return erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
	}
	return erros.Novo(erros.CategoriaConflito,
		"o estado da marcação mudou entretanto; recarregue a marcação e repita a operação")
}

// scanUma reconstrói uma Marcacao a partir de uma linha (QueryRow ou Rows).
func (r *RepositorioMarcacoes) scanUma(linha pgx.Row) (*dominio.Marcacao, error) {
	var s dominio.SnapshotMarcacao
	var estado string
	if err := linha.Scan(&s.ID, &s.DoenteID, &s.MedicoID, &s.EspecialidadeID,
		&s.Inicio, &s.Fim, &estado, &s.Motivo, &s.RemarcaDe, &s.CriadoEm, &s.ActualizadoEm); err != nil {
		return nil, err
	}
	s.Estado = dominio.EstadoMarcacao(estado)
	return dominio.ReconstruirMarcacao(s), nil
}

// ehExclusao indica se o erro é uma violação da restrição EXCLUDE (sobreposição).
func ehExclusao(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoExclusaoPG
}

// querier abstrai o que pool e tx têm em comum (QueryRow), para reutilizar `inserir`.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioMarcacoes = (*RepositorioMarcacoes)(nil)
