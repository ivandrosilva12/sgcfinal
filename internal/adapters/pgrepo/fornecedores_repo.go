package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioFornecedores implementa dominio.RepositorioFornecedores com pgx.
type RepositorioFornecedores struct {
	pool *pgxpool.Pool
}

// NovoRepositorioFornecedores constrói o repositório sobre o pool pgx.
func NovoRepositorioFornecedores(pool *pgxpool.Pool) *RepositorioFornecedores {
	return &RepositorioFornecedores{pool: pool}
}

// Guardar persiste o fornecedor (INSERT se id vazio, senão UPDATE).
func (r *RepositorioFornecedores) Guardar(ctx context.Context, f *dominio.Fornecedor) (string, error) {
	s := f.Snapshot()
	if s.ID == "" {
		const q = `INSERT INTO farmacia.fornecedores (nome, nif, contacto, activo)
		           VALUES ($1, NULLIF($2,''), NULLIF($3,''), $4) RETURNING id::text`
		var id string
		if err := r.pool.QueryRow(ctx, q, s.Nome, deref(s.NIF), deref(s.Contacto), s.Activo).Scan(&id); err != nil {
			return "", fmt.Errorf("inserir fornecedor: %w", err)
		}
		return id, nil
	}
	const q = `UPDATE farmacia.fornecedores SET nome=$2, nif=NULLIF($3,''), contacto=NULLIF($4,''), activo=$5 WHERE id=$1`
	ct, err := r.pool.Exec(ctx, q, s.ID, s.Nome, deref(s.NIF), deref(s.Contacto), s.Activo)
	if err != nil {
		return "", fmt.Errorf("actualizar fornecedor: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", erros.Novo(erros.CategoriaNaoEncontrado, "fornecedor não encontrado")
	}
	return s.ID, nil
}

// ObterPorID devolve o fornecedor reconstruído a partir do snapshot lido da
// BD (sempre um agregado fresco, nunca partilhado entre chamadas).
func (r *RepositorioFornecedores) ObterPorID(ctx context.Context, id string) (*dominio.Fornecedor, error) {
	const q = `SELECT id::text, nome, nif, contacto, activo, criado_em FROM farmacia.fornecedores WHERE id=$1`
	var s dominio.SnapshotFornecedor
	if err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.Nome, &s.NIF, &s.Contacto, &s.Activo, &s.CriadoEm); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "fornecedor não encontrado")
		}
		return nil, fmt.Errorf("obter fornecedor: %w", err)
	}
	return dominio.ReconstruirFornecedor(s), nil
}

// Listar devolve uma página de fornecedores filtrada por termo e estado.
func (r *RepositorioFornecedores) Listar(ctx context.Context, f dominio.FiltroFornecedores) (dominio.PaginaFornecedores, error) {
	base := `FROM farmacia.fornecedores WHERE ($1='' OR nome ILIKE '%'||$1||'%') AND ($2 = false OR activo)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+base, f.Termo, f.ApenasActivos).Scan(&total); err != nil {
		return dominio.PaginaFornecedores{}, fmt.Errorf("contar fornecedores: %w", err)
	}
	q := `SELECT id::text, nome, nif, activo ` + base + ` ORDER BY nome LIMIT $3 OFFSET $4`
	linhas, err := r.pool.Query(ctx, q, f.Termo, f.ApenasActivos, f.Limite, f.Deslocamento)
	if err != nil {
		return dominio.PaginaFornecedores{}, fmt.Errorf("listar fornecedores: %w", err)
	}
	defer linhas.Close()
	pagina := dominio.PaginaFornecedores{Total: total, Limite: f.Limite, Deslocamento: f.Deslocamento, Itens: []dominio.ResumoFornecedor{}}
	for linhas.Next() {
		var it dominio.ResumoFornecedor
		if err := linhas.Scan(&it.ID, &it.Nome, &it.NIF, &it.Activo); err != nil {
			return dominio.PaginaFornecedores{}, fmt.Errorf("ler fornecedor: %w", err)
		}
		pagina.Itens = append(pagina.Itens, it)
	}
	return pagina, linhas.Err()
}
