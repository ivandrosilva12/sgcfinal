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

// RepositorioResultados implementa dominio.RepositorioResultados com pgx.
type RepositorioResultados struct {
	pool *pgxpool.Pool
}

// NovoRepositorioResultados constrói o repositório sobre o pool pgx.
func NovoRepositorioResultados(pool *pgxpool.Pool) *RepositorioResultados {
	return &RepositorioResultados{pool: pool}
}

// ObterPorID reconstrói o resultado. NaoEncontrado se não existir.
func (r *RepositorioResultados) ObterPorID(ctx context.Context, id string) (*dominio.Resultado, error) {
	const q = `
SELECT id::text, requisicao_id::text, codigo_analise, COALESCE(valor,''), unidade,
       COALESCE(observacoes,''), COALESCE(motivo_recusa,''), estado,
       COALESCE(tecnico_colheita_id::text,''), COALESCE(tecnico_submissor_id::text,''),
       COALESCE(patologista_validador_id::text,''),
       colhida_em, submetida_em, validada_em, valor_critico, criado_em
FROM laboratorio.resultados WHERE id=$1`
	var s dominio.SnapshotResultado
	var estado string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &s.RequisicaoID, &s.CodigoAnalise,
		&s.Valor, &s.Unidade, &s.Observacoes, &s.MotivoRecusa, &estado,
		&s.TecnicoColheitaID, &s.TecnicoSubmissorID, &s.PatologistaValidadorID,
		&s.ColhidaEm, &s.SubmetidaEm, &s.ValidadaEm, &s.ValorCritico, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
		}
		return nil, fmt.Errorf("obter resultado: %w", err)
	}
	s.Estado = dominio.EstadoResultado(estado)
	return dominio.ReconstruirResultado(s), nil
}

// Transitar aplica a transição de estado com guarda compare-and-set: o UPDATE só se
// aplica se a linha ainda estiver no estado com que o agregado foi lido. Duas
// transições concorrentes a partir do mesmo estado (dois técnicos a colher a mesma
// amostra, duplo-clique em submeter) passam ambas as guardas do domínio — mas só uma
// escreve; a outra perde a corrida e recebe Conflito.
//
// Escreve apenas as colunas que uma transição altera — nunca as de identidade do
// resultado (requisicao_id, codigo_analise, unidade).
func (r *RepositorioResultados) Transitar(ctx context.Context, res *dominio.Resultado) error {
	s := res.Snapshot()
	if s.ID == "" {
		return erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
	}
	const q = `
UPDATE laboratorio.resultados SET
    estado=$2, valor=NULLIF($3,''), observacoes=NULLIF($4,''), motivo_recusa=NULLIF($5,''),
    tecnico_colheita_id=NULLIF($6,'')::uuid, tecnico_submissor_id=NULLIF($7,'')::uuid,
    colhida_em=$8, submetida_em=$9,
    patologista_validador_id=NULLIF($11,'')::uuid, validada_em=$12, valor_critico=$13
WHERE id=$1 AND estado=$10`
	ct, err := r.pool.Exec(ctx, q, s.ID, string(s.Estado), s.Valor, s.Observacoes, s.MotivoRecusa,
		s.TecnicoColheitaID, s.TecnicoSubmissorID, s.ColhidaEm, s.SubmetidaEm,
		string(s.EstadoAnterior), s.PatologistaValidadorID, s.ValidadaEm, s.ValorCritico)
	if err != nil {
		return fmt.Errorf("actualizar resultado: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return r.erroTransicaoFalhada(ctx, s.ID)
	}
	return nil
}

// Corrigir persiste a correcção de um resultado validado numa única transacção: o
// original transita VALIDADA → CONCLUIDA (compare-and-set) e o novo resultado é
// inserido em VALIDADA a apontar-lhe via corrige_resultado_id. Qualquer falha faz
// rollback de ambos; devolve o id do novo resultado.
func (r *RepositorioResultados) Corrigir(ctx context.Context, novo, original *dominio.Resultado) (string, error) {
	so := original.Snapshot()
	sn := novo.Snapshot()
	if so.ID == "" {
		return "", erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção de correcção: %w", err)
	}
	defer tx.Rollback(ctx)

	const arquivar = `UPDATE laboratorio.resultados SET estado=$2 WHERE id=$1 AND estado=$3`
	ct, err := tx.Exec(ctx, arquivar, so.ID, string(dominio.ResConcluida), string(so.EstadoAnterior))
	if err != nil {
		return "", fmt.Errorf("arquivar resultado original: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", r.erroTransicaoFalhada(ctx, so.ID)
	}

	const inserir = `
INSERT INTO laboratorio.resultados
    (requisicao_id, codigo_analise, valor, unidade, observacoes, estado,
     tecnico_colheita_id, tecnico_submissor_id, patologista_validador_id,
     colhida_em, submetida_em, validada_em, valor_critico, corrige_resultado_id)
VALUES ($1,$2,NULLIF($3,''),$4,NULLIF($5,''),$6,
        NULLIF($7,'')::uuid, NULLIF($8,'')::uuid, NULLIF($9,'')::uuid,
        $10,$11,$12,$13,$14::uuid)
RETURNING id::text`
	var novoID string
	if err := tx.QueryRow(ctx, inserir,
		sn.RequisicaoID, sn.CodigoAnalise, sn.Valor, sn.Unidade, sn.Observacoes, string(sn.Estado),
		sn.TecnicoColheitaID, sn.TecnicoSubmissorID, sn.PatologistaValidadorID,
		sn.ColhidaEm, sn.SubmetidaEm, sn.ValidadaEm, sn.ValorCritico, sn.CorrigeResultadoID,
	).Scan(&novoID); err != nil {
		return "", fmt.Errorf("inserir resultado corrigido: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar correcção: %w", err)
	}
	return novoID, nil
}

// erroTransicaoFalhada distingue "a linha não existe" (NaoEncontrado/404) de "a linha
// existe mas já não está no estado esperado" (Conflito/409, corrida perdida).
func (r *RepositorioResultados) erroTransicaoFalhada(ctx context.Context, id string) error {
	const q = `SELECT EXISTS (SELECT 1 FROM laboratorio.resultados WHERE id=$1)`
	var existe bool
	if err := r.pool.QueryRow(ctx, q, id).Scan(&existe); err != nil {
		return fmt.Errorf("verificar resultado: %w", err)
	}
	if !existe {
		return erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
	}
	return erros.Novo(erros.CategoriaConflito,
		"o estado do resultado mudou entretanto; recarregue o resultado e repita a operação")
}

// estadosTexto converte a lista de estados para texto (nil = todos).
func estadosTexto(estados []dominio.EstadoResultado) []string {
	if len(estados) == 0 {
		return nil
	}
	out := make([]string, 0, len(estados))
	for _, e := range estados {
		out = append(out, string(e))
	}
	return out
}

// ListarFila devolve a fila de trabalho do laboratório. Lista de estados vazia = todos.
func (r *RepositorioResultados) ListarFila(ctx context.Context, estados []dominio.EstadoResultado) ([]dominio.ResumoResultado, error) {
	const q = `
SELECT res.id::text, res.requisicao_id::text, req.episodio_id::text, res.codigo_analise,
       COALESCE(res.valor,''), res.unidade, res.estado, res.valor_critico,
       res.colhida_em, res.submetida_em, res.criado_em
FROM laboratorio.resultados res
JOIN laboratorio.requisicoes req ON req.id = res.requisicao_id
WHERE ($1::text[] IS NULL OR res.estado = ANY($1))
ORDER BY res.criado_em, res.id`
	return r.consultar(ctx, q, estadosTexto(estados))
}

// ListarPorEpisodio devolve os resultados de um episódio nos estados dados. Ao
// contrário de ListarFila, uma lista de estados vazia/nil devolve zero linhas — é a
// rede de segurança que impede um preliminar (PROCESSADA) de vazar para a vista
// clínica do médico caso o filtro de estados alguma vez chegue vazio.
func (r *RepositorioResultados) ListarPorEpisodio(ctx context.Context, episodioID string, estados []dominio.EstadoResultado) ([]dominio.ResumoResultado, error) {
	if len(estados) == 0 {
		return nil, nil
	}
	const q = `
SELECT res.id::text, res.requisicao_id::text, req.episodio_id::text, res.codigo_analise,
       COALESCE(res.valor,''), res.unidade, res.estado, res.valor_critico,
       res.colhida_em, res.submetida_em, res.criado_em
FROM laboratorio.resultados res
JOIN laboratorio.requisicoes req ON req.id = res.requisicao_id
WHERE req.episodio_id = $2 AND res.estado = ANY($1)
ORDER BY res.criado_em, res.id`
	return r.consultar(ctx, q, estadosTexto(estados), episodioID)
}

// consultar corre uma das duas queries de listagem e mapeia as linhas.
func (r *RepositorioResultados) consultar(ctx context.Context, q string, args ...any) ([]dominio.ResumoResultado, error) {
	linhas, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listar resultados: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoResultado{}
	for linhas.Next() {
		var rr dominio.ResumoResultado
		if err := linhas.Scan(&rr.ID, &rr.RequisicaoID, &rr.EpisodioID, &rr.CodigoAnalise,
			&rr.Valor, &rr.Unidade, &rr.Estado, &rr.ValorCritico,
			&rr.ColhidaEm, &rr.SubmetidaEm, &rr.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler resultado: %w", err)
		}
		out = append(out, rr)
	}
	return out, linhas.Err()
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioResultados = (*RepositorioResultados)(nil)
