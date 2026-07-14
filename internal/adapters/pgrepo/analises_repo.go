package pgrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioAnalises implementa dominio.RepositorioAnalises com pgx.
type RepositorioAnalises struct {
	pool *pgxpool.Pool
}

// NovoRepositorioAnalises constrói o repositório sobre o pool pgx.
func NovoRepositorioAnalises(pool *pgxpool.Pool) *RepositorioAnalises {
	return &RepositorioAnalises{pool: pool}
}

// Guardar insere a análise. Código duplicado → Conflito (violação de PK).
func (r *RepositorioAnalises) Guardar(ctx context.Context, a *dominio.Analise) error {
	s := a.Snapshot()
	intervalos, err := json.Marshal(naoNil(s.Intervalos))
	if err != nil {
		return fmt.Errorf("serializar intervalos de referência: %w", err)
	}
	criticos, err := json.Marshal(naoNilCriticos(s.ValoresCriticos))
	if err != nil {
		return fmt.Errorf("serializar valores críticos: %w", err)
	}
	const q = `
INSERT INTO laboratorio.analises (codigo, nome, unidade, intervalos_referencia, valores_criticos, activo)
VALUES ($1,$2,$3,$4,$5,$6)`
	if _, err := r.pool.Exec(ctx, q, s.Codigo, s.Nome, s.Unidade, intervalos, criticos, s.Activo); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return erros.Novo(erros.CategoriaConflito, "já existe uma análise com este código")
		}
		return fmt.Errorf("inserir análise: %w", err)
	}
	return nil
}

// naoNil garante `[]` em vez de `null` no jsonb — não por imposição do NOT NULL da
// coluna (json.Marshal de um slice nil dá o JSON `null`, que é um jsonb válido e não
// viola NOT NULL nenhum), mas por coerência com o DEFAULT '[]'::jsonb da coluna e com
// qualquer jsonb_array_length() que venha a ser usado sobre ela.
func naoNil(v []dominio.IntervaloReferencia) []dominio.IntervaloReferencia {
	if v == nil {
		return []dominio.IntervaloReferencia{}
	}
	return v
}

func naoNilCriticos(v []dominio.ValorCritico) []dominio.ValorCritico {
	if v == nil {
		return []dominio.ValorCritico{}
	}
	return v
}

// ObterPorCodigo reconstrói a análise. NaoEncontrado se não existir.
func (r *RepositorioAnalises) ObterPorCodigo(ctx context.Context, codigo string) (*dominio.Analise, error) {
	const q = `
SELECT codigo, nome, unidade, intervalos_referencia, valores_criticos, activo, criado_em
FROM laboratorio.analises WHERE codigo = upper($1)`
	var s dominio.SnapshotAnalise
	var intervalos, criticos []byte
	err := r.pool.QueryRow(ctx, q, codigo).Scan(&s.Codigo, &s.Nome, &s.Unidade,
		&intervalos, &criticos, &s.Activo, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "análise não encontrada: "+codigo)
		}
		return nil, fmt.Errorf("obter análise: %w", err)
	}
	if err := json.Unmarshal(intervalos, &s.Intervalos); err != nil {
		return nil, fmt.Errorf("ler intervalos de referência: %w", err)
	}
	if err := json.Unmarshal(criticos, &s.ValoresCriticos); err != nil {
		return nil, fmt.Errorf("ler valores críticos: %w", err)
	}
	return dominio.ReconstruirAnalise(s), nil
}

// Listar devolve o catálogo por ordem de código.
func (r *RepositorioAnalises) Listar(ctx context.Context) ([]dominio.ResumoAnalise, error) {
	const q = `SELECT codigo, nome, unidade, activo FROM laboratorio.analises ORDER BY codigo`
	linhas, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("listar análises: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoAnalise{}
	for linhas.Next() {
		var a dominio.ResumoAnalise
		if err := linhas.Scan(&a.Codigo, &a.Nome, &a.Unidade, &a.Activo); err != nil {
			return nil, fmt.Errorf("ler análise: %w", err)
		}
		out = append(out, a)
	}
	return out, linhas.Err()
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioAnalises = (*RepositorioAnalises)(nil)
