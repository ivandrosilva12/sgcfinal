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

// RepositorioMedicamentos implementa dominio.RepositorioMedicamentos com pgx.
type RepositorioMedicamentos struct {
	pool *pgxpool.Pool
}

// NovoRepositorioMedicamentos constrói o repositório sobre o pool pgx.
func NovoRepositorioMedicamentos(pool *pgxpool.Pool) *RepositorioMedicamentos {
	return &RepositorioMedicamentos{pool: pool}
}

// ProximoCodigo reserva atomicamente o próximo código do catálogo (MED-NNNNN).
func (r *RepositorioMedicamentos) ProximoCodigo(ctx context.Context) (string, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT nextval('farmacia.seq_codigo_medicamento')`).Scan(&n); err != nil {
		return "", fmt.Errorf("reservar código de medicamento: %w", err)
	}
	return fmt.Sprintf("MED-%05d", n), nil
}

// Guardar persiste o medicamento (INSERT se id vazio, senão UPDATE). Código
// interno duplicado → CategoriaConflito.
func (r *RepositorioMedicamentos) Guardar(ctx context.Context, m *dominio.Medicamento) (string, error) {
	s := m.Snapshot()
	if s.ID == "" {
		return r.inserir(ctx, s)
	}
	return s.ID, r.actualizar(ctx, s)
}

func (r *RepositorioMedicamentos) inserir(ctx context.Context, s dominio.SnapshotMedicamento) (string, error) {
	const q = `
INSERT INTO farmacia.medicamentos (
    codigo_interno, nome_comercial, nome_generico, forma_farmaceutica, dosagem,
    via_administracao, fabricante, requer_receita, psicotropico, classe_atc, stock_minimo, activo
) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,$12) RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q,
		s.CodigoInterno, s.NomeComercial, s.NomeGenerico, s.FormaFarmaceutica, s.Dosagem,
		s.ViaAdministracao, s.Fabricante, s.RequerReceita, s.Psicotropico, s.ClasseATC, s.StockMinimo, s.Activo,
	).Scan(&id)
	if err != nil {
		return "", traduzUnicidadeMedicamento(err)
	}
	return id, nil
}

func (r *RepositorioMedicamentos) actualizar(ctx context.Context, s dominio.SnapshotMedicamento) error {
	const q = `
UPDATE farmacia.medicamentos SET
    nome_comercial=$2, nome_generico=$3, forma_farmaceutica=$4, dosagem=$5,
    via_administracao=$6, fabricante=NULLIF($7,''), requer_receita=$8, psicotropico=$9,
    classe_atc=$10, stock_minimo=$11, activo=$12, actualizado_em=now()
WHERE id=$1`
	ct, err := r.pool.Exec(ctx, q, s.ID,
		s.NomeComercial, s.NomeGenerico, s.FormaFarmaceutica, s.Dosagem,
		s.ViaAdministracao, s.Fabricante, s.RequerReceita, s.Psicotropico, s.ClasseATC, s.StockMinimo, s.Activo,
	)
	if err != nil {
		return traduzUnicidadeMedicamento(err)
	}
	if ct.RowsAffected() == 0 {
		return erros.Novo(erros.CategoriaNaoEncontrado, "medicamento não encontrado")
	}
	return nil
}

// ObterPorID devolve o medicamento. NaoEncontrado se não existir.
func (r *RepositorioMedicamentos) ObterPorID(ctx context.Context, id string) (*dominio.Medicamento, error) {
	const q = `
SELECT id::text, codigo_interno, nome_comercial, nome_generico, forma_farmaceutica, dosagem,
       via_administracao, COALESCE(fabricante,''), requer_receita, psicotropico, classe_atc,
       stock_minimo, activo, criado_em, actualizado_em
FROM farmacia.medicamentos WHERE id=$1`
	var s dominio.SnapshotMedicamento
	if err := r.pool.QueryRow(ctx, q, id).Scan(
		&s.ID, &s.CodigoInterno, &s.NomeComercial, &s.NomeGenerico, &s.FormaFarmaceutica, &s.Dosagem,
		&s.ViaAdministracao, &s.Fabricante, &s.RequerReceita, &s.Psicotropico, &s.ClasseATC,
		&s.StockMinimo, &s.Activo, &s.CriadoEm, &s.ActualizadoEm,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "medicamento não encontrado")
		}
		return nil, fmt.Errorf("obter medicamento: %w", err)
	}
	return dominio.ReconstruirMedicamento(s), nil
}

// Pesquisar devolve uma página do catálogo (nome via trigram na expressão indexada).
func (r *RepositorioMedicamentos) Pesquisar(ctx context.Context, f dominio.FiltroMedicamentos) (dominio.PaginaMedicamentos, error) {
	base := `FROM farmacia.medicamentos WHERE ($1='' OR (nome_comercial || ' ' || nome_generico) ILIKE '%'||$1||'%') AND ($2 = false OR activo)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+base, f.Termo, f.ApenasActivos).Scan(&total); err != nil {
		return dominio.PaginaMedicamentos{}, fmt.Errorf("contar medicamentos: %w", err)
	}
	q := `SELECT id::text, codigo_interno, nome_comercial, nome_generico, forma_farmaceutica, dosagem, activo ` +
		base + ` ORDER BY nome_comercial LIMIT $3 OFFSET $4`
	linhas, err := r.pool.Query(ctx, q, f.Termo, f.ApenasActivos, f.Limite, f.Deslocamento)
	if err != nil {
		return dominio.PaginaMedicamentos{}, fmt.Errorf("pesquisar medicamentos: %w", err)
	}
	defer linhas.Close()
	pagina := dominio.PaginaMedicamentos{Total: total, Limite: f.Limite, Deslocamento: f.Deslocamento, Itens: []dominio.ResumoMedicamento{}}
	for linhas.Next() {
		var it dominio.ResumoMedicamento
		if err := linhas.Scan(&it.ID, &it.CodigoInterno, &it.NomeComercial, &it.NomeGenerico, &it.FormaFarmaceutica, &it.Dosagem, &it.Activo); err != nil {
			return dominio.PaginaMedicamentos{}, fmt.Errorf("ler medicamento: %w", err)
		}
		pagina.Itens = append(pagina.Itens, it)
	}
	return pagina, linhas.Err()
}

// traduzUnicidadeMedicamento mapeia 23505 (código interno duplicado) para conflito.
func traduzUnicidadeMedicamento(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return erros.Novo(erros.CategoriaConflito, "já existe um medicamento com este código interno")
	}
	return fmt.Errorf("guardar medicamento: %w", err)
}
