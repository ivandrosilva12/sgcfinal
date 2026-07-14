package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioRequisicoes implementa dominio.RepositorioRequisicoes com pgx.
type RepositorioRequisicoes struct {
	pool *pgxpool.Pool
}

// NovoRepositorioRequisicoes constrói o repositório sobre o pool pgx.
func NovoRepositorioRequisicoes(pool *pgxpool.Pool) *RepositorioRequisicoes {
	return &RepositorioRequisicoes{pool: pool}
}

// Emitir grava a requisição, os seus itens e os resultados PENDENTE numa única
// transacção. Se qualquer INSERT falhar, nada fica escrito: uma requisição sem
// resultados nunca apareceria na fila do laboratório e ficaria invisível para todos.
// O RequisicaoID dos resultados vem do id acabado de gerar — o valor que o caso de
// uso lá pôs é ignorado (na altura ainda não havia id).
func (r *RepositorioRequisicoes) Emitir(ctx context.Context, req *dominio.RequisicaoLab, resultados []*dominio.Resultado) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção da requisição: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	s := req.Snapshot()
	const qReq = `
INSERT INTO laboratorio.requisicoes (episodio_id, doente_id, medico_requisitante_id, prioridade, estado)
VALUES ($1,$2,$3,$4,$5) RETURNING id::text`
	var id string
	if err := tx.QueryRow(ctx, qReq, s.EpisodioID, s.DoenteID, s.MedicoRequisitanteID,
		string(s.Prioridade), string(s.Estado)).Scan(&id); err != nil {
		return "", fmt.Errorf("inserir requisição: %w", err)
	}

	const qItem = `
INSERT INTO laboratorio.itens_requisicao (requisicao_id, codigo_analise, observacoes)
VALUES ($1,$2,NULLIF($3,''))`
	for _, item := range s.Itens {
		if _, err := tx.Exec(ctx, qItem, id, item.CodigoAnalise, item.Observacoes); err != nil {
			return "", fmt.Errorf("inserir item da requisição: %w", err)
		}
	}

	const qRes = `
INSERT INTO laboratorio.resultados (requisicao_id, codigo_analise, unidade, estado)
VALUES ($1,$2,$3,$4)`
	for _, res := range resultados {
		sr := res.Snapshot()
		if _, err := tx.Exec(ctx, qRes, id, sr.CodigoAnalise, sr.Unidade, string(sr.Estado)); err != nil {
			return "", fmt.Errorf("inserir resultado pendente: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar a emissão da requisição: %w", err)
	}
	return id, nil
}

// ObterPorID reconstrói a requisição com os seus itens.
func (r *RepositorioRequisicoes) ObterPorID(ctx context.Context, id string) (*dominio.RequisicaoLab, error) {
	const q = `
SELECT id::text, episodio_id::text, doente_id::text, medico_requisitante_id::text,
       prioridade, estado, criado_em
FROM laboratorio.requisicoes WHERE id=$1`
	var s dominio.SnapshotRequisicao
	var prioridade, estado string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.EpisodioID, &s.DoenteID,
		&s.MedicoRequisitanteID, &prioridade, &estado, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "requisição não encontrada")
		}
		return nil, fmt.Errorf("obter requisição: %w", err)
	}
	s.Prioridade = dominio.Prioridade(prioridade)
	s.Estado = dominio.EstadoRequisicao(estado)

	const qItens = `
SELECT codigo_analise, COALESCE(observacoes,'')
FROM laboratorio.itens_requisicao WHERE requisicao_id=$1 ORDER BY codigo_analise`
	linhas, err := r.pool.Query(ctx, qItens, id)
	if err != nil {
		return nil, fmt.Errorf("listar itens da requisição: %w", err)
	}
	defer linhas.Close()
	for linhas.Next() {
		var it dominio.ItemRequisicao
		if err := linhas.Scan(&it.CodigoAnalise, &it.Observacoes); err != nil {
			return nil, fmt.Errorf("ler item da requisição: %w", err)
		}
		s.Itens = append(s.Itens, it)
	}
	if err := linhas.Err(); err != nil {
		return nil, fmt.Errorf("ler itens da requisição: %w", err)
	}
	return dominio.ReconstruirRequisicao(s), nil
}

// ListarPorEpisodio devolve as requisições do episódio (mais recentes primeiro).
func (r *RepositorioRequisicoes) ListarPorEpisodio(ctx context.Context, episodioID string) ([]dominio.ResumoRequisicao, error) {
	const q = `
SELECT r.id::text, r.episodio_id::text, r.doente_id::text, r.prioridade, r.estado,
       (SELECT count(*) FROM laboratorio.itens_requisicao i WHERE i.requisicao_id = r.id),
       r.criado_em
FROM laboratorio.requisicoes r
WHERE r.episodio_id=$1
ORDER BY r.criado_em DESC`
	linhas, err := r.pool.Query(ctx, q, episodioID)
	if err != nil {
		return nil, fmt.Errorf("listar requisições: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoRequisicao{}
	for linhas.Next() {
		var rr dominio.ResumoRequisicao
		if err := linhas.Scan(&rr.ID, &rr.EpisodioID, &rr.DoenteID, &rr.Prioridade,
			&rr.Estado, &rr.NumAnalises, &rr.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler requisição: %w", err)
		}
		out = append(out, rr)
	}
	return out, linhas.Err()
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioRequisicoes = (*RepositorioRequisicoes)(nil)
