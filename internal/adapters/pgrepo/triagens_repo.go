// internal/adapters/pgrepo/triagens_repo.go
package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// RepositorioTriagens implementa dominio.RepositorioTriagens com pgx.
type RepositorioTriagens struct {
	pool *pgxpool.Pool
}

// NovoRepositorioTriagens constrói o repositório sobre o pool pgx.
func NovoRepositorioTriagens(pool *pgxpool.Pool) *RepositorioTriagens {
	return &RepositorioTriagens{pool: pool}
}

const colunasTriagem = `id::text, chegada_id::text, enfermeiro_id::text, prioridade,
       tensao_sistolica, tensao_diastolica, frequencia_cardiaca, temperatura,
       frequencia_respiratoria, saturacao_o2, dor, glicemia, peso,
       COALESCE(observacoes,''), triada_em, criado_em`

// RegistarTriagem grava, numa única transacção, a chegada a passar a TRIADO (guarda
// compare-and-set sobre CHAMADO, com o médico atribuído) e a nova triagem.
func (r *RepositorioTriagens) RegistarTriagem(ctx context.Context, triagem *dominio.Triagem, chegada *dominio.Chegada) (string, error) {
	sc := chegada.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção de triagem: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	const upd = `UPDATE recepcao.chegadas SET estado=$2, medico_id=NULLIF($3,'')::uuid, actualizado_em=$4
WHERE id=$1 AND estado=$5`
	ct, err := tx.Exec(ctx, upd, sc.ID, string(sc.Estado), sc.MedicoID, sc.ActualizadoEm, string(sc.EstadoAnterior))
	if err != nil {
		return "", fmt.Errorf("transitar chegada para triada: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return "", r.erroChegada(ctx, sc.ID)
	}

	st := triagem.Snapshot()
	const ins = `
INSERT INTO recepcao.triagens
    (chegada_id, enfermeiro_id, prioridade, tensao_sistolica, tensao_diastolica,
     frequencia_cardiaca, temperatura, frequencia_respiratoria, saturacao_o2, dor,
     glicemia, peso, observacoes, triada_em)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NULLIF($13,''), $14)
RETURNING id::text`
	sv := st.SinaisVitais
	var id string
	err = tx.QueryRow(ctx, ins, st.ChegadaID, st.EnfermeiroID, string(st.Prioridade),
		sv.TensaoSistolica, sv.TensaoDiastolica, sv.FrequenciaCardiaca, sv.Temperatura,
		sv.FrequenciaRespiratoria, sv.SaturacaoO2, sv.Dor, sv.Glicemia, sv.Peso,
		st.Observacoes, st.TriadaEm).Scan(&id)
	if err != nil {
		if ehUnica(err) {
			return "", erros.Novo(erros.CategoriaConflito, "já existe uma triagem para esta chegada")
		}
		return "", fmt.Errorf("guardar triagem: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar triagem: %w", err)
	}
	return id, nil
}

// ObterPorChegada reconstrói a triagem de uma chegada. NaoEncontrado se não existir.
func (r *RepositorioTriagens) ObterPorChegada(ctx context.Context, chegadaID string) (*dominio.Triagem, error) {
	q := `SELECT ` + colunasTriagem + ` FROM recepcao.triagens WHERE chegada_id=$1`
	var s dominio.SnapshotTriagem
	var prioridade string
	var sv dominio.SinaisVitais
	err := r.pool.QueryRow(ctx, q, chegadaID).Scan(&s.ID, &s.ChegadaID, &s.EnfermeiroID, &prioridade,
		&sv.TensaoSistolica, &sv.TensaoDiastolica, &sv.FrequenciaCardiaca, &sv.Temperatura,
		&sv.FrequenciaRespiratoria, &sv.SaturacaoO2, &sv.Dor, &sv.Glicemia, &sv.Peso,
		&s.Observacoes, &s.TriadaEm, &s.CriadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "triagem não encontrada")
		}
		return nil, fmt.Errorf("obter triagem: %w", err)
	}
	s.Prioridade = dominio.PrioridadeManchester(prioridade)
	s.SinaisVitais = sv
	return dominio.ReconstruirTriagem(s), nil
}

// ListarFilaClinica devolve as chegadas TRIADO com a sua triagem, ordenadas por
// severidade de Manchester (mais urgente primeiro) e depois por hora de chegada. Médico
// vazio = todos.
func (r *RepositorioTriagens) ListarFilaClinica(ctx context.Context, medicoID string) ([]dominio.ResumoFilaClinica, error) {
	const q = `
SELECT c.id::text, t.id::text, c.doente_id::text, COALESCE(c.medico_id::text,''),
       c.especialidade_id::text, t.prioridade, c.hora_chegada, t.triada_em
FROM recepcao.triagens t
JOIN recepcao.chegadas c ON c.id = t.chegada_id
WHERE c.estado='TRIADO' AND ($1='' OR c.medico_id=NULLIF($1,'')::uuid)
ORDER BY CASE t.prioridade
    WHEN 'VERMELHO' THEN 1 WHEN 'LARANJA' THEN 2 WHEN 'AMARELO' THEN 3
    WHEN 'VERDE' THEN 4 WHEN 'AZUL' THEN 5 ELSE 9 END,
    c.hora_chegada, c.id`
	linhas, err := r.pool.Query(ctx, q, medicoID)
	if err != nil {
		return nil, fmt.Errorf("listar fila clínica: %w", err)
	}
	defer linhas.Close()
	out := []dominio.ResumoFilaClinica{}
	for linhas.Next() {
		var rc dominio.ResumoFilaClinica
		if err := linhas.Scan(&rc.ChegadaID, &rc.TriagemID, &rc.DoenteID, &rc.MedicoID,
			&rc.EspecialidadeID, &rc.Prioridade, &rc.HoraChegada, &rc.TriadaEm); err != nil {
			return nil, fmt.Errorf("ler fila clínica: %w", err)
		}
		out = append(out, rc)
	}
	return out, linhas.Err()
}

// erroChegada distingue 404 (chegada inexistente) de 409 (estado mudou) na transição.
func (r *RepositorioTriagens) erroChegada(ctx context.Context, id string) error {
	var existe bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM recepcao.chegadas WHERE id=$1)`, id).Scan(&existe); err != nil {
		return fmt.Errorf("verificar chegada: %w", err)
	}
	if !existe {
		return erros.Novo(erros.CategoriaNaoEncontrado, "chegada não encontrada")
	}
	return erros.Novo(erros.CategoriaConflito,
		"o estado da chegada mudou entretanto; recarregue e repita a operação")
}

// Garantia de conformidade com a porta.
var _ dominio.RepositorioTriagens = (*RepositorioTriagens)(nil)
