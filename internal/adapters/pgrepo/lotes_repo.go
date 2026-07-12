package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioLotes implementa dominio.RepositorioLotes com pgx.
type RepositorioLotes struct {
	pool *pgxpool.Pool
}

// NovoRepositorioLotes constrói o repositório sobre o pool pgx.
func NovoRepositorioLotes(pool *pgxpool.Pool) *RepositorioLotes {
	return &RepositorioLotes{pool: pool}
}

// RegistarEntrada insere o lote e o movimento ENTRADA numa só transacção.
func (r *RepositorioLotes) RegistarEntrada(ctx context.Context, l *dominio.Lote, realizadoPor string) (string, error) {
	s := l.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const qLote = `
INSERT INTO farmacia.lotes (medicamento_id, numero_lote, validade, quantidade_inicial, quantidade_actual, preco_unit_custo, fornecedor_id, notas)
VALUES ($1,$2,$3,$4,$5,$6::numeric,$7,NULLIF($8,'')) RETURNING id::text`
	var id string
	if err := tx.QueryRow(ctx, qLote,
		s.MedicamentoID, s.NumeroLote, s.Validade, s.QuantidadeInicial, s.QuantidadeActual,
		s.PrecoUnitarioCusto, s.FornecedorID, s.Notas,
	).Scan(&id); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", erros.Novo(erros.CategoriaConflito, "já existe um lote com este número para o medicamento e fornecedor")
		}
		return "", fmt.Errorf("inserir lote: %w", err)
	}
	const qMov = `
INSERT INTO farmacia.movimentos_stock (tipo, medicamento_id, lote_id, quantidade, realizado_por)
VALUES ($1,$2,$3,$4,$5)`
	if _, err := tx.Exec(ctx, qMov, string(dominio.MovimentoEntrada), s.MedicamentoID, id, s.QuantidadeInicial, realizadoPor); err != nil {
		return "", fmt.Errorf("registar movimento de entrada: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar transacção: %w", err)
	}
	return id, nil
}

// ObterPorID devolve o lote reconstruído a partir do snapshot lido da BD
// (sempre um agregado fresco, nunca partilhado entre chamadas).
func (r *RepositorioLotes) ObterPorID(ctx context.Context, id string) (*dominio.Lote, error) {
	const q = `
SELECT id::text, medicamento_id::text, numero_lote, validade, quantidade_inicial, quantidade_actual,
       preco_unit_custo::text, fornecedor_id::text, entrada_em, COALESCE(notas,'')
FROM farmacia.lotes WHERE id=$1`
	var s dominio.SnapshotLote
	if err := r.pool.QueryRow(ctx, q, id).Scan(
		&s.ID, &s.MedicamentoID, &s.NumeroLote, &s.Validade, &s.QuantidadeInicial, &s.QuantidadeActual,
		&s.PrecoUnitarioCusto, &s.FornecedorID, &s.EntradaEm, &s.Notas,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "lote não encontrado")
		}
		return nil, fmt.Errorf("obter lote: %w", err)
	}
	return dominio.ReconstruirLote(s), nil
}

// ListarPorMedicamento devolve os lotes de um medicamento, ordenados por
// validade ascendente (ordem FEFO). Se apenasDisponiveis, filtra lotes com
// stock e ainda válidos.
func (r *RepositorioLotes) ListarPorMedicamento(ctx context.Context, medicamentoID string, apenasDisponiveis bool) ([]dominio.ResumoLote, error) {
	q := `SELECT id::text, numero_lote, validade, quantidade_actual, fornecedor_id::text
	      FROM farmacia.lotes WHERE medicamento_id=$1 AND ($2 = false OR (quantidade_actual > 0 AND validade > CURRENT_DATE))
	      ORDER BY validade ASC`
	linhas, err := r.pool.Query(ctx, q, medicamentoID, apenasDisponiveis)
	if err != nil {
		return nil, fmt.Errorf("listar lotes: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoLote{}
	for linhas.Next() {
		var it dominio.ResumoLote
		if err := linhas.Scan(&it.ID, &it.NumeroLote, &it.Validade, &it.QuantidadeActual, &it.FornecedorID); err != nil {
			return nil, fmt.Errorf("ler lote: %w", err)
		}
		out = append(out, it)
	}
	return out, linhas.Err()
}

// StockDisponivel soma a quantidade actual dos lotes válidos e com stock.
func (r *RepositorioLotes) StockDisponivel(ctx context.Context, medicamentoID string) (int, error) {
	const q = `SELECT COALESCE(SUM(quantidade_actual),0) FROM farmacia.lotes
	           WHERE medicamento_id=$1 AND quantidade_actual > 0 AND validade > CURRENT_DATE`
	var total int
	if err := r.pool.QueryRow(ctx, q, medicamentoID).Scan(&total); err != nil {
		return 0, fmt.Errorf("consultar stock: %w", err)
	}
	return total, nil
}
