// Package pgrepo contém as implementações de repositório sobre PostgreSQL via
// pgx v5 (SQL puro, sem ORM). Camada 3 — Adaptadores.
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioDoentes implementa dominio.RepositorioDoentes com pgx.
type RepositorioDoentes struct {
	pool *pgxpool.Pool
}

// NovoRepositorioDoentes constrói o repositório sobre o pool pgx.
func NovoRepositorioDoentes(pool *pgxpool.Pool) *RepositorioDoentes {
	return &RepositorioDoentes{pool: pool}
}

// ProximoNumeroProcesso reserva atomicamente o próximo sequencial do ano e
// formata "P-{ano}-{sequencial:06d}".
func (r *RepositorioDoentes) ProximoNumeroProcesso(ctx context.Context, ano int) (string, error) {
	const q = `
INSERT INTO clinico.processo_sequencia (ano, ultimo) VALUES ($1, 1)
ON CONFLICT (ano) DO UPDATE SET ultimo = clinico.processo_sequencia.ultimo + 1
RETURNING ultimo`
	var ultimo int
	if err := r.pool.QueryRow(ctx, q, ano).Scan(&ultimo); err != nil {
		return "", fmt.Errorf("reservar número de processo: %w", err)
	}
	return fmt.Sprintf("P-%d-%06d", ano, ultimo), nil
}

// Guardar persiste o doente (INSERT se id vazio, senão UPDATE) e os seus filhos,
// numa única transacção. Conflitos de unicidade → CategoriaConflito.
func (r *RepositorioDoentes) Guardar(ctx context.Context, d *dominio.Doente) (string, error) {
	s := d.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := s.ID
	if id == "" {
		id, err = r.inserir(ctx, tx, s)
	} else {
		err = r.actualizar(ctx, tx, s)
	}
	if err != nil {
		return "", traduzErroUnicidade(err)
	}

	if err := r.guardarFilhos(ctx, tx, id, s); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar transacção: %w", err)
	}
	return id, nil
}

func (r *RepositorioDoentes) inserir(ctx context.Context, tx pgx.Tx, s dominio.SnapshotDoente) (string, error) {
	const q = `
INSERT INTO clinico.doentes (
    num_processo, nome_completo, data_nascimento, sexo, bi, nif, passaporte,
    nacionalidade, telefone, email,
    morada_provincia, morada_municipio, morada_comuna, morada_bairro, morada_rua, morada_casa, morada_referencia,
    grupo_sanguineo, estado, falecido_em, causa_morte_cid, desactivado_em, desactivado_motivo
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NULLIF($18,''),$19,$20,NULLIF($21,''),$22,NULLIF($23,'')
) RETURNING id::text`
	mp, mm, mc, mb, mr, mca, mref := desmontarMorada(s)
	var id string
	err := tx.QueryRow(ctx, q,
		s.NumProcesso, s.Identificacao.NomeCompleto, s.Identificacao.DataNascimento, string(s.Identificacao.Sexo),
		s.Identificacao.BI, s.Identificacao.NIF, s.Identificacao.Passaporte,
		s.Nacionalidade, s.Contactos.Telefone, s.Contactos.Email,
		mp, mm, mc, mb, mr, mca, mref,
		grupoTexto(s), string(s.Estado), s.FalecidoEm, s.CausaMorteCID, s.DesactivadoEm, s.DesactivadoMotivo,
	).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (r *RepositorioDoentes) actualizar(ctx context.Context, tx pgx.Tx, s dominio.SnapshotDoente) error {
	const q = `
UPDATE clinico.doentes SET
    num_processo=$2, nome_completo=$3, data_nascimento=$4, sexo=$5, bi=$6, nif=$7, passaporte=$8,
    nacionalidade=$9, telefone=$10, email=$11,
    morada_provincia=$12, morada_municipio=$13, morada_comuna=$14, morada_bairro=$15, morada_rua=$16, morada_casa=$17, morada_referencia=$18,
    grupo_sanguineo=NULLIF($19,''), estado=$20, falecido_em=$21, causa_morte_cid=NULLIF($22,''),
    desactivado_em=$23, desactivado_motivo=NULLIF($24,''), actualizado_em=now()
WHERE id=$1`
	mp, mm, mc, mb, mr, mca, mref := desmontarMorada(s)
	ct, err := tx.Exec(ctx, q, s.ID,
		s.NumProcesso, s.Identificacao.NomeCompleto, s.Identificacao.DataNascimento, string(s.Identificacao.Sexo),
		s.Identificacao.BI, s.Identificacao.NIF, s.Identificacao.Passaporte,
		s.Nacionalidade, s.Contactos.Telefone, s.Contactos.Email,
		mp, mm, mc, mb, mr, mca, mref,
		grupoTexto(s), string(s.Estado), s.FalecidoEm, s.CausaMorteCID, s.DesactivadoEm, s.DesactivadoMotivo,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
	}
	return nil
}

// guardarFilhos substitui alergias e antecedentes por delete-and-reinsert.
func (r *RepositorioDoentes) guardarFilhos(ctx context.Context, tx pgx.Tx, id string, s dominio.SnapshotDoente) error {
	if _, err := tx.Exec(ctx, `DELETE FROM clinico.alergias WHERE doente_id=$1`, id); err != nil {
		return fmt.Errorf("limpar alergias: %w", err)
	}
	for _, a := range s.Alergias {
		if _, err := tx.Exec(ctx,
			`INSERT INTO clinico.alergias (doente_id, substancia, severidade, reaccao_tipica, confirmada_em, notas)
			 VALUES ($1,$2,$3,NULLIF($4,''),$5,NULLIF($6,''))`,
			id, a.Substancia, string(a.Severidade), a.ReaccaoTipica, a.ConfirmadaEm, a.Notas); err != nil {
			return fmt.Errorf("inserir alergia: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM clinico.antecedentes_clinicos WHERE doente_id=$1`, id); err != nil {
		return fmt.Errorf("limpar antecedentes: %w", err)
	}
	for _, a := range s.Antecedentes {
		if _, err := tx.Exec(ctx,
			`INSERT INTO clinico.antecedentes_clinicos (doente_id, tipo, descricao, cid, data_inicio, activo, notas)
			 VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,NULLIF($7,''))`,
			id, string(a.Tipo), a.Descricao, a.CID, a.DataInicio, a.Activo, a.Notas); err != nil {
			return fmt.Errorf("inserir antecedente: %w", err)
		}
	}
	return nil
}

// ObterPorID devolve o doente com os filhos. NaoEncontrado se não existir.
func (r *RepositorioDoentes) ObterPorID(ctx context.Context, id string) (*dominio.Doente, error) {
	return r.obter(ctx, `id=$1`, id)
}

// ObterPorNumProcesso devolve o doente pelo número de processo.
func (r *RepositorioDoentes) ObterPorNumProcesso(ctx context.Context, num string) (*dominio.Doente, error) {
	return r.obter(ctx, `num_processo=$1`, num)
}

func (r *RepositorioDoentes) obter(ctx context.Context, cond string, arg any) (*dominio.Doente, error) {
	q := `
SELECT id::text, num_processo, nome_completo, data_nascimento, sexo, bi, nif, passaporte,
       nacionalidade, telefone, email,
       morada_provincia, morada_municipio, morada_comuna, morada_bairro, morada_rua, morada_casa, morada_referencia,
       grupo_sanguineo, estado, falecido_em, COALESCE(causa_morte_cid, ''), criado_em, actualizado_em,
       desactivado_em, desactivado_motivo
FROM clinico.doentes WHERE ` + cond
	var s dominio.SnapshotDoente
	var sexo, estado string
	var grupo, motivo *string
	// Sete destinos para os campos opcionais da morada: se `mp` (província) vier
	// nil, consideramos que o doente não tem morada registada.
	var mp, mm, mc, mb, mr, mca, mref *string
	if err := r.pool.QueryRow(ctx, q, arg).Scan(
		&s.ID, &s.NumProcesso, &s.Identificacao.NomeCompleto, &s.Identificacao.DataNascimento, &sexo,
		&s.Identificacao.BI, &s.Identificacao.NIF, &s.Identificacao.Passaporte,
		&s.Nacionalidade, &s.Contactos.Telefone, &s.Contactos.Email,
		&mp, &mm, &mc, &mb, &mr, &mca, &mref,
		&grupo, &estado, &s.FalecidoEm, &s.CausaMorteCID, &s.CriadoEm, &s.ActualizadoEm,
		&s.DesactivadoEm, &motivo,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
		}
		return nil, fmt.Errorf("obter doente: %w", err)
	}
	s.Identificacao.Sexo = dominio.Sexo(sexo)
	s.Estado = dominio.EstadoDoente(estado)
	if grupo != nil {
		g := dominio.GrupoSanguineo(*grupo)
		s.GrupoSanguineo = &g
	}
	if motivo != nil {
		s.DesactivadoMotivo = *motivo
	}
	if mp != nil {
		s.Contactos.Morada = &dominio.Morada{
			Provincia:  deref(mp),
			Municipio:  deref(mm),
			Comuna:     deref(mc),
			Bairro:     deref(mb),
			Rua:        deref(mr),
			Casa:       mca,
			Referencia: mref,
		}
	}
	filhos, err := r.carregarFilhos(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	s.Alergias, s.Antecedentes = filhos.alergias, filhos.antecedentes
	return dominio.ReconstruirDoente(s), nil
}

type destinoFilhos struct {
	alergias     []dominio.Alergia
	antecedentes []dominio.AntecedenteClinico
}

func (r *RepositorioDoentes) carregarFilhos(ctx context.Context, id string) (destinoFilhos, error) {
	var out destinoFilhos
	linhasA, err := r.pool.Query(ctx,
		`SELECT substancia, severidade, COALESCE(reaccao_tipica,''), confirmada_em, COALESCE(notas,'')
		 FROM clinico.alergias WHERE doente_id=$1 ORDER BY criada_em`, id)
	if err != nil {
		return out, fmt.Errorf("carregar alergias: %w", err)
	}
	defer linhasA.Close()
	for linhasA.Next() {
		var a dominio.Alergia
		var sev string
		if err := linhasA.Scan(&a.Substancia, &sev, &a.ReaccaoTipica, &a.ConfirmadaEm, &a.Notas); err != nil {
			return out, fmt.Errorf("ler alergia: %w", err)
		}
		a.Severidade = dominio.Severidade(sev)
		out.alergias = append(out.alergias, a)
	}
	if err := linhasA.Err(); err != nil {
		return out, err
	}
	linhasAnt, err := r.pool.Query(ctx,
		`SELECT tipo, descricao, COALESCE(cid,''), data_inicio, activo, COALESCE(notas,'')
		 FROM clinico.antecedentes_clinicos WHERE doente_id=$1 ORDER BY criado_em`, id)
	if err != nil {
		return out, fmt.Errorf("carregar antecedentes: %w", err)
	}
	defer linhasAnt.Close()
	for linhasAnt.Next() {
		var a dominio.AntecedenteClinico
		var tipo string
		if err := linhasAnt.Scan(&tipo, &a.Descricao, &a.CID, &a.DataInicio, &a.Activo, &a.Notas); err != nil {
			return out, fmt.Errorf("ler antecedente: %w", err)
		}
		a.Tipo = dominio.TipoAntecedente(tipo)
		out.antecedentes = append(out.antecedentes, a)
	}
	return out, linhasAnt.Err()
}

// Pesquisar devolve uma página de doentes. Nome via trigram (ILIKE); BI, número
// de processo e telefone por igualdade. Filtro de estado opcional.
func (r *RepositorioDoentes) Pesquisar(ctx context.Context, f dominio.FiltroDoentes) (dominio.PaginaDoentes, error) {
	base := `FROM clinico.doentes WHERE ($1='' OR nome_completo ILIKE '%'||$1||'%' OR bi=$1 OR num_processo=$1 OR telefone=$1) AND ($2='' OR estado=$2)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+base, f.Termo, f.Estado).Scan(&total); err != nil {
		return dominio.PaginaDoentes{}, fmt.Errorf("contar doentes: %w", err)
	}
	q := `SELECT id::text, num_processo, nome_completo, data_nascimento, sexo, telefone, estado ` +
		base + ` ORDER BY nome_completo LIMIT $3 OFFSET $4`
	linhas, err := r.pool.Query(ctx, q, f.Termo, f.Estado, f.Limite, f.Deslocamento)
	if err != nil {
		return dominio.PaginaDoentes{}, fmt.Errorf("pesquisar doentes: %w", err)
	}
	defer linhas.Close()
	pagina := dominio.PaginaDoentes{Total: total, Limite: f.Limite, Deslocamento: f.Deslocamento, Itens: []dominio.ResumoDoente{}}
	for linhas.Next() {
		var it dominio.ResumoDoente
		if err := linhas.Scan(&it.ID, &it.NumProcesso, &it.NomeCompleto, &it.DataNascimento, &it.Sexo, &it.Telefone, &it.Estado); err != nil {
			return dominio.PaginaDoentes{}, fmt.Errorf("ler resumo: %w", err)
		}
		pagina.Itens = append(pagina.Itens, it)
	}
	return pagina, linhas.Err()
}

// desmontarMorada devolve os sete campos da morada (nil se ausente).
func desmontarMorada(s dominio.SnapshotDoente) (mp, mm, mc, mb, mr, mca, mref *string) {
	if s.Contactos.Morada == nil {
		return nil, nil, nil, nil, nil, nil, nil
	}
	m := s.Contactos.Morada
	return &m.Provincia, &m.Municipio, &m.Comuna, &m.Bairro, &m.Rua, m.Casa, m.Referencia
}

// deref devolve o valor apontado, ou "" se o ponteiro for nil.
func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// grupoTexto devolve o grupo sanguíneo como texto ("" se ausente → NULLIF na SQL).
func grupoTexto(s dominio.SnapshotDoente) string {
	if s.GrupoSanguineo == nil {
		return ""
	}
	return s.GrupoSanguineo.String()
}

// traduzErroUnicidade mapeia a violação de unicidade do Postgres (23505) para um
// erro de domínio de conflito; os restantes erros passam inalterados.
func traduzErroUnicidade(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return erros.Novo(erros.CategoriaConflito, "já existe um doente com este número de processo ou Bilhete de Identidade")
	}
	return fmt.Errorf("guardar doente: %w", err)
}
