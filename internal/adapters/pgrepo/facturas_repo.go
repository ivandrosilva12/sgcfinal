package pgrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
)

// RepositorioFacturas implementa fin.RepositorioFacturas com pgx.
type RepositorioFacturas struct {
	pool *pgxpool.Pool
}

// NovoRepositorioFacturas constrói o repositório sobre o pool pgx.
func NovoRepositorioFacturas(pool *pgxpool.Pool) *RepositorioFacturas {
	return &RepositorioFacturas{pool: pool}
}

// Guardar faz o upsert transaccional da factura e reescreve as suas linhas.
func (r *RepositorioFacturas) Guardar(ctx context.Context, f *fin.Factura) (string, error) {
	s := f.Snapshot()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("iniciar transacção da factura: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := s.ID
	if id == "" {
		const qIns = `
INSERT INTO financeiro.facturas (estado, cliente_nome, cliente_nif, cliente_morada, episodio_id)
VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5::uuid) RETURNING id::text`
		if err := tx.QueryRow(ctx, qIns, string(s.Estado), s.Cliente.Nome, s.Cliente.NIF,
			s.Cliente.Morada, s.EpisodioID).Scan(&id); err != nil {
			return "", fmt.Errorf("inserir factura: %w", err)
		}
	} else {
		const qUpd = `
UPDATE financeiro.facturas
SET cliente_nome=$2, cliente_nif=NULLIF($3,''), cliente_morada=NULLIF($4,''),
    versao=versao+1, actualizado_em=now()
WHERE id=$1 AND estado='RASCUNHO' AND versao=$5`
		ct, err := tx.Exec(ctx, qUpd, id, s.Cliente.Nome, s.Cliente.NIF, s.Cliente.Morada, s.Versao)
		if err != nil {
			return "", fmt.Errorf("actualizar factura: %w", err)
		}
		if ct.RowsAffected() != 1 {
			// Distingue-se pela leitura do estado actual: se continua em rascunho, o
			// que falhou foi a versão (outra escrita passou à frente).
			var estado string
			if e := tx.QueryRow(ctx, `SELECT estado FROM financeiro.facturas WHERE id=$1`, id).Scan(&estado); e == nil && estado == "RASCUNHO" {
				return "", erros.Novo(erros.CategoriaConflito,
					"a factura foi alterada entretanto — recarregue e tente de novo")
			}
			return "", erros.Novo(erros.CategoriaConflito, "a factura já não está em rascunho ou não existe")
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM financeiro.itens_factura WHERE factura_id=$1`, id); err != nil {
		return "", fmt.Errorf("limpar linhas da factura: %w", err)
	}
	const qItem = `
INSERT INTO financeiro.itens_factura
    (id, factura_id, descricao, tipo, operacao_id, quantidade, preco_unitario_centimos, regime_iva, ordem)
VALUES (COALESCE(NULLIF($1,'')::uuid, gen_random_uuid()), $2, $3, $4, NULLIF($5,'')::uuid, $6, $7, $8, $9)`
	for ordem, it := range s.Itens {
		if _, err := tx.Exec(ctx, qItem, it.ID, id, it.Descricao, string(it.Tipo),
			it.OperacaoID, it.Quantidade, it.PrecoUnitario.Centimos(), string(it.RegimeIVA), ordem); err != nil {
			return "", fmt.Errorf("inserir linha da factura: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("confirmar a gravação da factura: %w", err)
	}
	return id, nil
}

// ObterPorID reconstrói a factura com as suas linhas.
func (r *RepositorioFacturas) ObterPorID(ctx context.Context, id string) (*fin.Factura, error) {
	const q = `
SELECT id::text, estado, cliente_nome, COALESCE(cliente_nif,''), COALESCE(cliente_morada,''),
       episodio_id::text, criado_em, actualizado_em, versao
FROM financeiro.facturas WHERE id=$1`
	var s fin.SnapshotFactura
	var estado string
	err := r.pool.QueryRow(ctx, q, id).Scan(&s.ID, &estado, &s.Cliente.Nome, &s.Cliente.NIF,
		&s.Cliente.Morada, &s.EpisodioID, &s.CriadoEm, &s.ActualizadoEm, &s.Versao)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "factura não encontrada")
		}
		return nil, fmt.Errorf("obter factura: %w", err)
	}
	s.Estado = fin.EstadoFactura(estado)

	const qItens = `
SELECT id::text, descricao, tipo, COALESCE(operacao_id::text,''), quantidade, preco_unitario_centimos, regime_iva
FROM financeiro.itens_factura WHERE factura_id=$1 ORDER BY ordem`
	linhas, err := r.pool.Query(ctx, qItens, id)
	if err != nil {
		return nil, fmt.Errorf("listar linhas da factura: %w", err)
	}
	defer linhas.Close()
	for linhas.Next() {
		var it fin.ItemFactura
		var tipo, regime string
		var centimos int64
		if err := linhas.Scan(&it.ID, &it.Descricao, &tipo, &it.OperacaoID, &it.Quantidade, &centimos, &regime); err != nil {
			return nil, fmt.Errorf("ler linha da factura: %w", err)
		}
		it.Tipo = fin.TipoLinha(tipo)
		it.RegimeIVA = fin.RegimeIVA(regime)
		it.PrecoUnitario = moeda.DeCentimos(centimos)
		s.Itens = append(s.Itens, it)
	}
	if err := linhas.Err(); err != nil {
		return nil, fmt.Errorf("ler linhas da factura: %w", err)
	}
	return fin.ReconstruirFactura(s), nil
}

// ListarPorEpisodio devolve os resumos das facturas do episódio (recentes primeiro).
// O total replica, em aritmética inteira, a fórmula de IVA do domínio
// (ItemFactura.ValorIVA): o read model é uma projecção; o cálculo autoritário é do
// domínio.
func (r *RepositorioFacturas) ListarPorEpisodio(ctx context.Context, episodioID string) ([]fin.ResumoFactura, error) {
	const q = `
SELECT f.id::text, f.estado, f.cliente_nome, f.episodio_id::text,
       (SELECT count(*) FROM financeiro.itens_factura i WHERE i.factura_id=f.id),
       COALESCE((SELECT sum(i.preco_unitario_centimos*i.quantidade
                 + CASE i.regime_iva WHEN 'STANDARD'
                       THEN (i.preco_unitario_centimos*i.quantidade*14 + 50)/100 ELSE 0 END)
                 FROM financeiro.itens_factura i WHERE i.factura_id=f.id), 0),
       f.criado_em
FROM financeiro.facturas f
WHERE f.episodio_id=$1
ORDER BY f.criado_em DESC`
	linhas, err := r.pool.Query(ctx, q, episodioID)
	if err != nil {
		return nil, fmt.Errorf("listar facturas: %w", err)
	}
	defer linhas.Close()
	out := []fin.ResumoFactura{}
	for linhas.Next() {
		var rf fin.ResumoFactura
		if err := linhas.Scan(&rf.ID, &rf.Estado, &rf.ClienteNome, &rf.EpisodioID,
			&rf.NumItens, &rf.TotalCentimos, &rf.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler factura: %w", err)
		}
		rf.Total = moeda.DeCentimos(rf.TotalCentimos).String()
		out = append(out, rf)
	}
	return out, linhas.Err()
}

// Garantia de conformidade com a porta.
var _ fin.RepositorioFacturas = (*RepositorioFacturas)(nil)
