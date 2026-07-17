package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioEpisodios implementa dominio.RepositorioEpisodios com pgx.
type RepositorioEpisodios struct {
	pool *pgxpool.Pool
}

// NovoRepositorioEpisodios constrói o repositório sobre o pool pgx.
func NovoRepositorioEpisodios(pool *pgxpool.Pool) *RepositorioEpisodios {
	return &RepositorioEpisodios{pool: pool}
}

// Guardar persiste o episódio (INSERT se id vazio, senão UPDATE) e os seus
// diagnósticos CID, numa única transacção.
func (r *RepositorioEpisodios) Guardar(ctx context.Context, e *dominio.EpisodioClinico) (string, error) {
	s := e.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := s.ID
	if id == "" {
		id, err = r.inserirEpisodio(ctx, tx, s)
	} else {
		err = r.actualizarEpisodio(ctx, tx, s)
	}
	if err != nil {
		return "", err
	}
	if err := r.guardarDiagnosticos(ctx, tx, id, s); err != nil {
		return "", err
	}
	if err := inserirEventos(ctx, tx, e.EventosPendentes()); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar transacção: %w", err)
	}
	return id, nil
}

func (r *RepositorioEpisodios) inserirEpisodio(ctx context.Context, tx pgx.Tx, s dominio.SnapshotEpisodio) (string, error) {
	const q = `
INSERT INTO clinico.episodios_clinicos (
    doente_id, tipo, especialidade_id, medico_id, inicio, fim,
    queixa_principal, historia_doenca, exame_objectivo, diagnostico, plano,
    estado, fechado_em, fechado_por
) VALUES (
    $1,$2,$3,$4,$5,$6,
    NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),
    $12,$13,NULLIF($14,'')::uuid
) RETURNING id::text`
	var id string
	err := tx.QueryRow(ctx, q,
		s.DoenteID, string(s.Tipo), s.EspecialidadeID, s.MedicoID, s.Inicio, s.Fim,
		s.Nota.QueixaPrincipal, s.Nota.HistoriaDoenca, s.Nota.ExameObjectivo, s.Nota.Diagnostico, s.Nota.Plano,
		string(s.Estado), s.FechadoEm, s.FechadoPor,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserir episódio: %w", err)
	}
	return id, nil
}

func (r *RepositorioEpisodios) actualizarEpisodio(ctx context.Context, tx pgx.Tx, s dominio.SnapshotEpisodio) error {
	const q = `
UPDATE clinico.episodios_clinicos SET
    tipo=$2, especialidade_id=$3, medico_id=$4, inicio=$5, fim=$6,
    queixa_principal=NULLIF($7,''), historia_doenca=NULLIF($8,''), exame_objectivo=NULLIF($9,''),
    diagnostico=NULLIF($10,''), plano=NULLIF($11,''),
    estado=$12, fechado_em=$13, fechado_por=NULLIF($14,'')::uuid, actualizado_em=now()
WHERE id=$1`
	ct, err := tx.Exec(ctx, q, s.ID,
		string(s.Tipo), s.EspecialidadeID, s.MedicoID, s.Inicio, s.Fim,
		s.Nota.QueixaPrincipal, s.Nota.HistoriaDoenca, s.Nota.ExameObjectivo, s.Nota.Diagnostico, s.Nota.Plano,
		string(s.Estado), s.FechadoEm, s.FechadoPor,
	)
	if err != nil {
		return fmt.Errorf("actualizar episódio: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return erros.Novo(erros.CategoriaNaoEncontrado, "episódio não encontrado")
	}
	return nil
}

func (r *RepositorioEpisodios) guardarDiagnosticos(ctx context.Context, tx pgx.Tx, id string, s dominio.SnapshotEpisodio) error {
	if _, err := tx.Exec(ctx, `DELETE FROM clinico.diagnosticos_cid WHERE episodio_id=$1`, id); err != nil {
		return fmt.Errorf("limpar diagnósticos: %w", err)
	}
	for _, d := range s.DiagnosticosCID {
		if _, err := tx.Exec(ctx,
			`INSERT INTO clinico.diagnosticos_cid (episodio_id, cid, principal) VALUES ($1,$2,$3)`,
			id, d.CID, d.Principal); err != nil {
			return fmt.Errorf("inserir diagnóstico: %w", err)
		}
	}
	return nil
}

// ObterPorID devolve o episódio com os diagnósticos. NaoEncontrado se não existir.
func (r *RepositorioEpisodios) ObterPorID(ctx context.Context, id string) (*dominio.EpisodioClinico, error) {
	const q = `
SELECT id::text, doente_id::text, tipo, especialidade_id::text, medico_id::text, inicio, fim,
       COALESCE(queixa_principal,''), COALESCE(historia_doenca,''), COALESCE(exame_objectivo,''),
       COALESCE(diagnostico,''), COALESCE(plano,''), estado, criado_em, actualizado_em,
       fechado_em, fechado_por::text
FROM clinico.episodios_clinicos WHERE id=$1`
	var s dominio.SnapshotEpisodio
	var tipo, estado string
	var queixa, historia, exame, diag, plano string
	var fechadoPor *string
	if err := r.pool.QueryRow(ctx, q, id).Scan(
		&s.ID, &s.DoenteID, &tipo, &s.EspecialidadeID, &s.MedicoID, &s.Inicio, &s.Fim,
		&queixa, &historia, &exame, &diag, &plano, &estado, &s.CriadoEm, &s.ActualizadoEm,
		&s.FechadoEm, &fechadoPor,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "episódio não encontrado")
		}
		return nil, fmt.Errorf("obter episódio: %w", err)
	}
	s.Tipo = dominio.TipoEpisodio(tipo)
	s.Estado = dominio.EstadoEpisodio(estado)
	s.Nota = dominio.NotaClinica{
		QueixaPrincipal: queixa, HistoriaDoenca: historia, ExameObjectivo: exame,
		Diagnostico: diag, Plano: plano,
	}
	s.FechadoPor = deref(fechadoPor)

	diags, err := r.carregarDiagnosticos(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	s.DiagnosticosCID = diags
	return dominio.ReconstruirEpisodio(s), nil
}

func (r *RepositorioEpisodios) carregarDiagnosticos(ctx context.Context, id string) ([]dominio.DiagnosticoCID, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT cid, principal FROM clinico.diagnosticos_cid WHERE episodio_id=$1 ORDER BY cid`, id)
	if err != nil {
		return nil, fmt.Errorf("carregar diagnósticos: %w", err)
	}
	defer linhas.Close()
	var out []dominio.DiagnosticoCID
	for linhas.Next() {
		var d dominio.DiagnosticoCID
		if err := linhas.Scan(&d.CID, &d.Principal); err != nil {
			return nil, fmt.Errorf("ler diagnóstico: %w", err)
		}
		out = append(out, d)
	}
	return out, linhas.Err()
}

// ListarPorDoente devolve uma página de episódios do doente, mais recentes primeiro.
func (r *RepositorioEpisodios) ListarPorDoente(ctx context.Context, f dominio.FiltroEpisodios) (dominio.PaginaEpisodios, error) {
	base := `FROM clinico.episodios_clinicos WHERE doente_id=$1 AND ($2='' OR estado=$2)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+base, f.DoenteID, f.Estado).Scan(&total); err != nil {
		return dominio.PaginaEpisodios{}, fmt.Errorf("contar episódios: %w", err)
	}
	q := `SELECT id::text, tipo, especialidade_id::text, medico_id::text, inicio, fim, estado ` +
		base + ` ORDER BY inicio DESC LIMIT $3 OFFSET $4`
	linhas, err := r.pool.Query(ctx, q, f.DoenteID, f.Estado, f.Limite, f.Deslocamento)
	if err != nil {
		return dominio.PaginaEpisodios{}, fmt.Errorf("listar episódios: %w", err)
	}
	defer linhas.Close()
	pagina := dominio.PaginaEpisodios{Total: total, Limite: f.Limite, Deslocamento: f.Deslocamento, Itens: []dominio.ResumoEpisodio{}}
	for linhas.Next() {
		var it dominio.ResumoEpisodio
		if err := linhas.Scan(&it.ID, &it.Tipo, &it.EspecialidadeID, &it.MedicoID, &it.Inicio, &it.Fim, &it.Estado); err != nil {
			return dominio.PaginaEpisodios{}, fmt.Errorf("ler resumo de episódio: %w", err)
		}
		pagina.Itens = append(pagina.Itens, it)
	}
	return pagina, linhas.Err()
}
