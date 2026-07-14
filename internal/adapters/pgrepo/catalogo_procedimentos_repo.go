package pgrepo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioCatalogoProcedimentos implementa a leitura do catálogo com pgx.
type RepositorioCatalogoProcedimentos struct {
	pool *pgxpool.Pool
}

// NovoRepositorioCatalogoProcedimentos constrói o repositório sobre o pool pgx.
func NovoRepositorioCatalogoProcedimentos(pool *pgxpool.Pool) *RepositorioCatalogoProcedimentos {
	return &RepositorioCatalogoProcedimentos{pool: pool}
}

// ObterPorCodigo devolve a entrada do catálogo. NaoEncontrado se não existir.
func (r *RepositorioCatalogoProcedimentos) ObterPorCodigo(ctx context.Context, codigo string) (*dominio.CatalogoProcedimento, error) {
	const q = `
SELECT codigo, descricao, COALESCE(especialidade,''), COALESCE(duracao_estimada_min,0),
       requer_anestesista, activo
FROM clinico.catalogo_procedimentos WHERE codigo=$1`
	var c dominio.CatalogoProcedimento
	err := r.pool.QueryRow(ctx, q, strings.ToUpper(strings.TrimSpace(codigo))).Scan(
		&c.Codigo, &c.Descricao, &c.Especialidade, &c.DuracaoEstimadaMin, &c.RequerAnestesista, &c.Activo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "procedimento do catálogo não encontrado")
		}
		return nil, fmt.Errorf("obter procedimento do catálogo: %w", err)
	}
	return &c, nil
}
