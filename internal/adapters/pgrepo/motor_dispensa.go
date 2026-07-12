package pgrepo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
)

// MotorDispensa implementa appfarmacia.MotorDispensa: aloca stock por FEFO,
// regista os movimentos SAIDA_DISPENSA e persiste a receita, numa transacção.
type MotorDispensa struct {
	pool *pgxpool.Pool
}

// NovoMotorDispensa constrói o motor sobre o pool pgx.
func NovoMotorDispensa(pool *pgxpool.Pool) *MotorDispensa {
	return &MotorDispensa{pool: pool}
}

// Dispensar aloca `itens` por FEFO (bloqueando os lotes do medicamento com
// SELECT ... FOR UPDATE, ordenados por validade ASC), decrementa as
// quantidades dos lotes, regista os movimentos SAIDA_DISPENSA (quantidade
// negativa) e persiste a receita (já com RegistarDispensa aplicado pela
// aplicação) — tudo numa única transacção. Rollback total em qualquer erro
// (incluindo stock insuficiente).
func (m *MotorDispensa) Dispensar(ctx context.Context, receita dominio.SnapshotReceita, itens []appfarmacia.ItemDispensa, realizadoPor string) ([]dominio.AlocacaoFEFO, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var todas []dominio.AlocacaoFEFO
	for _, it := range itens {
		// Lotes válidos, ordenados por FEFO, bloqueados.
		linhas, err := tx.Query(ctx,
			`SELECT id::text, quantidade_actual FROM farmacia.lotes
			 WHERE medicamento_id=$1 AND quantidade_actual > 0 AND validade > CURRENT_DATE
			 ORDER BY validade ASC FOR UPDATE`, it.MedicamentoID)
		if err != nil {
			return nil, fmt.Errorf("bloquear lotes: %w", err)
		}
		var lotesFEFO []dominio.LoteFEFO
		for linhas.Next() {
			var lf dominio.LoteFEFO
			if err := linhas.Scan(&lf.LoteID, &lf.Disponivel); err != nil {
				linhas.Close()
				return nil, fmt.Errorf("ler lote: %w", err)
			}
			lotesFEFO = append(lotesFEFO, lf)
		}
		linhas.Close()
		if err := linhas.Err(); err != nil {
			return nil, err
		}

		alocs, err := dominio.AlocarFEFO(lotesFEFO, it.Quantidade)
		if err != nil {
			return nil, err // RegraNegocio (stock insuficiente) — rollback pelo defer
		}
		for _, a := range alocs {
			if _, err := tx.Exec(ctx,
				`UPDATE farmacia.lotes SET quantidade_actual = quantidade_actual - $2 WHERE id=$1`,
				a.LoteID, a.Quantidade); err != nil {
				return nil, fmt.Errorf("decrementar lote: %w", err)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO farmacia.movimentos_stock (tipo, medicamento_id, lote_id, quantidade, receita_id, realizado_por)
				 VALUES ($1,$2,$3,$4,$5,$6)`,
				string(dominio.MovimentoSaidaDispensa), it.MedicamentoID, a.LoteID, -a.Quantidade, receita.ID, realizadoPor); err != nil {
				return nil, fmt.Errorf("registar movimento de saída: %w", err)
			}
		}
		todas = append(todas, alocs...)
	}

	// Persistir a receita (estado + quantidades dispensadas) a partir do snapshot.
	if _, err := tx.Exec(ctx, `UPDATE farmacia.receitas SET estado=$2 WHERE id=$1`, receita.ID, string(receita.Estado)); err != nil {
		return nil, fmt.Errorf("actualizar estado da receita: %w", err)
	}
	for _, it := range receita.Itens {
		if _, err := tx.Exec(ctx,
			`UPDATE farmacia.itens_receita SET quantidade_dispensada=$3 WHERE receita_id=$1 AND medicamento_id=$2`,
			receita.ID, it.MedicamentoID, it.QuantidadeDispensada); err != nil {
			return nil, fmt.Errorf("actualizar item da receita: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("confirmar transacção: %w", err)
	}
	return todas, nil
}

// Garantia de conformidade com a porta.
var _ appfarmacia.MotorDispensa = (*MotorDispensa)(nil)
