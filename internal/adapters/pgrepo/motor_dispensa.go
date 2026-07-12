package pgrepo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
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

// itemReceitaFresco é o estado persistido (sob lock, dentro da transacção) de
// um item da receita — nunca o snapshot lido pela aplicação antes do lock.
type itemReceitaFresco struct {
	dispensada int
	prescrita  int
}

// Dispensar aloca `itens` por FEFO (bloqueando os lotes do medicamento com
// SELECT ... FOR UPDATE, ordenados por validade ASC), decrementa as
// quantidades dos lotes, regista os movimentos SAIDA_DISPENSA (quantidade
// negativa) e persiste a receita — tudo numa única transacção. Rollback total
// em qualquer erro (incluindo stock insuficiente).
//
// Concorrência: a receita (as suas linhas de itens_receita) é bloqueada com
// FOR UPDATE logo no início da transacção, ANTES de tocar em stock. Isto
// serializa dispensas concorrentes da MESMA receita — a segunda só obtém o
// lock depois da primeira confirmar (ou reverter), e nesse momento relê o
// estado fresco persistido, pelo que a re-validação do não-exceder (passo 2)
// é sempre feita contra dados actuais, nunca contra um snapshot obsoleto. A
// persistência é incremental (quantidade_dispensada = quantidade_dispensada +
// delta), nunca um overwrite absoluto a partir do snapshot — o que eliminava
// a corrida que permitia sobre-dispensar a mesma receita.
func (m *MotorDispensa) Dispensar(ctx context.Context, receita dominio.SnapshotReceita, itens []appfarmacia.ItemDispensa, realizadoPor string) ([]dominio.AlocacaoFEFO, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("iniciar transacção: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1) Bloqueia TODAS as linhas de itens da receita (serializa qualquer
	// dispensa concorrente da mesma receita, independentemente do item) e lê
	// o estado fresco persistido.
	linhasItens, err := tx.Query(ctx,
		`SELECT medicamento_id::text, quantidade_dispensada, quantidade_prescrita
		 FROM farmacia.itens_receita WHERE receita_id=$1 FOR UPDATE`, receita.ID)
	if err != nil {
		return nil, fmt.Errorf("bloquear itens da receita: %w", err)
	}
	estadoFresco := make(map[string]*itemReceitaFresco)
	for linhasItens.Next() {
		var medID string
		var f itemReceitaFresco
		if err := linhasItens.Scan(&medID, &f.dispensada, &f.prescrita); err != nil {
			linhasItens.Close()
			return nil, fmt.Errorf("ler item da receita: %w", err)
		}
		copia := f
		estadoFresco[medID] = &copia
	}
	linhasItens.Close()
	if err := linhasItens.Err(); err != nil {
		return nil, err
	}

	// 2) Re-valida o não-exceder DENTRO da tx contra o estado fresco —
	// autoritário; a pré-validação em memória da aplicação (antes do lock)
	// é só uma verificação rápida de UX, não pode ser a guarda final.
	for _, it := range itens {
		f, ok := estadoFresco[it.MedicamentoID]
		if !ok {
			return nil, erros.Novo(erros.CategoriaValidacao, "o medicamento não consta da receita")
		}
		if f.dispensada+it.Quantidade > f.prescrita {
			return nil, erros.Novo(erros.CategoriaRegraNegocio, "a quantidade a dispensar excede a prescrita")
		}
		// Contabiliza no estado em memória para o caso de o mesmo medicamento
		// aparecer mais do que uma vez neste pedido de dispensa.
		f.dispensada += it.Quantidade
	}

	// 3) Aloca por FEFO, decrementa lotes e regista os movimentos de saída
	// (inalterado — já serializava correctamente o stock por medicamento).
	var todas []dominio.AlocacaoFEFO
	for _, it := range itens {
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

	// 4) Persiste o incremento de cada item de forma INCREMENTAL e guardada
	// (nunca overwrite absoluto a partir do snapshot): o próprio UPDATE só
	// aplica se, com o delta, ainda não exceder o prescrito — defesa em
	// profundidade sobre o passo 2, atómica ao nível da linha.
	for _, it := range itens {
		ct, err := tx.Exec(ctx,
			`UPDATE farmacia.itens_receita
			 SET quantidade_dispensada = quantidade_dispensada + $3
			 WHERE receita_id=$1 AND medicamento_id=$2
			   AND quantidade_dispensada + $3 <= quantidade_prescrita`,
			receita.ID, it.MedicamentoID, it.Quantidade)
		if err != nil {
			return nil, fmt.Errorf("actualizar item da receita: %w", err)
		}
		if ct.RowsAffected() != 1 {
			return nil, erros.Novo(erros.CategoriaRegraNegocio, "a quantidade a dispensar excede a prescrita")
		}
	}

	// 5) Recalcula o estado da receita DENTRO da tx a partir do estado fresco
	// pós-UPDATE (nunca do snapshot): DISPENSADA se todos os itens estão
	// totalmente dispensados, senão PARCIAL — mesma regra de
	// dominio.Receita.recalcularEstadoDispensa, aplicada aos dados persistidos.
	linhasFinal, err := tx.Query(ctx,
		`SELECT quantidade_dispensada, quantidade_prescrita FROM farmacia.itens_receita WHERE receita_id=$1`,
		receita.ID)
	if err != nil {
		return nil, fmt.Errorf("reler itens da receita: %w", err)
	}
	novoEstado := dominio.ReceitaDispensada
	for linhasFinal.Next() {
		var dispensada, prescrita int
		if err := linhasFinal.Scan(&dispensada, &prescrita); err != nil {
			linhasFinal.Close()
			return nil, fmt.Errorf("ler item final da receita: %w", err)
		}
		if dispensada < prescrita {
			novoEstado = dominio.ReceitaParcial
		}
	}
	linhasFinal.Close()
	if err := linhasFinal.Err(); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `UPDATE farmacia.receitas SET estado=$2 WHERE id=$1`, receita.ID, string(novoEstado)); err != nil {
		return nil, fmt.Errorf("actualizar estado da receita: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("confirmar transacção: %w", err)
	}
	return todas, nil
}

// Garantia de conformidade com a porta.
var _ appfarmacia.MotorDispensa = (*MotorDispensa)(nil)
