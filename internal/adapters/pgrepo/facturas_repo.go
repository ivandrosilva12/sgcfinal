package pgrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// consulta é o mínimo de que a leitura precisa. Tanto o pool como uma transacção
// o satisfazem, o que permite ler a factura dentro ou fora de uma tx sem duplicar
// o SQL — importante porque a emissão tem de ler o agregado já sob o bloqueio.
type consulta interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// colunasFactura é a projecção completa do agregado, colunas de emissão
// incluídas. As colunas de emissão são NULL enquanto a factura está em rascunho;
// o COALESCE traduz isso para os zeros do snapshot.
const colunasFactura = `
SELECT id::text, estado, cliente_nome, COALESCE(cliente_nif,''), COALESCE(cliente_morada,''),
       episodio_id::text, criado_em, actualizado_em, COALESCE(numero,''), COALESCE(serie,''),
       COALESCE(sequencial,0), COALESCE(data_emissao, to_timestamp(0)),
       COALESCE(hash,''), COALESCE(hash_anterior,''), versao
FROM financeiro.facturas`

// lerFactura preenche o snapshot a partir de uma linha da projecção acima.
func lerFactura(sc interface{ Scan(...any) error }) (fin.SnapshotFactura, error) {
	var s fin.SnapshotFactura
	var estado, numero string
	err := sc.Scan(&s.ID, &estado, &s.Cliente.Nome, &s.Cliente.NIF, &s.Cliente.Morada,
		&s.EpisodioID, &s.CriadoEm, &s.ActualizadoEm, &numero, &s.Serie,
		&s.Sequencial, &s.DataEmissao, &s.Hash, &s.HashAnterior, &s.Versao)
	if err != nil {
		return fin.SnapshotFactura{}, err
	}
	s.Estado = fin.EstadoFactura(estado)
	s.Numero = fin.NumeroFactura(numero)
	return s, nil
}

// obter reconstrói a factura com as suas linhas, sobre o pool ou sobre uma tx.
func (r *RepositorioFacturas) obter(ctx context.Context, q consulta, id string) (*fin.Factura, error) {
	s, err := lerFactura(q.QueryRow(ctx, colunasFactura+` WHERE id=$1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaNaoEncontrado, "factura não encontrada")
		}
		return nil, fmt.Errorf("obter factura: %w", err)
	}
	itens, err := r.itensDe(ctx, q, s.ID)
	if err != nil {
		return nil, err
	}
	s.Itens = itens
	return fin.ReconstruirFactura(s), nil
}

// itensDe lê as linhas de uma factura pela sua ordem de apresentação.
func (r *RepositorioFacturas) itensDe(ctx context.Context, q consulta, facturaID string) ([]fin.ItemFactura, error) {
	const qItens = `
SELECT id::text, descricao, tipo, COALESCE(operacao_id::text,''), quantidade, preco_unitario_centimos, regime_iva
FROM financeiro.itens_factura WHERE factura_id=$1 ORDER BY ordem`
	linhas, err := q.Query(ctx, qItens, facturaID)
	if err != nil {
		return nil, fmt.Errorf("listar linhas da factura: %w", err)
	}
	defer linhas.Close()
	var itens []fin.ItemFactura
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
		itens = append(itens, it)
	}
	if err := linhas.Err(); err != nil {
		return nil, fmt.Errorf("ler linhas da factura: %w", err)
	}
	return itens, nil
}

// ObterPorID reconstrói a factura com as suas linhas.
func (r *RepositorioFacturas) ObterPorID(ctx context.Context, id string) (*fin.Factura, error) {
	return r.obter(ctx, r.pool, id)
}

// Emitir aloca o sequencial e o elo da cadeia sob serialização e transita a
// factura para EMITIDA, tudo numa transacção.
//
// O ponto de serialização é o SELECT ... FOR UPDATE sobre a linha da série: duas
// emissões simultâneas na mesma série põem-se em fila. Se a transacção reverter,
// o contador não avança — a ausência de buracos exigida pela AGT é propriedade da
// estrutura, não uma verificação a posteriori. Uma sequência do PostgreSQL não
// serviria: não é transaccional e deixaria buracos em cada rollback.
//
// O hash nunca é calculado aqui — é invariante do agregado (Factura.Emitir). O
// adaptador só lhe entrega o sequencial e o elo anterior.
func (r *RepositorioFacturas) Emitir(ctx context.Context, facturaID string, momento time.Time) (*fin.Factura, error) {
	serie := fin.SerieDe(momento)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("iniciar transacção de emissão: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Criar a série antes de a bloquear resolve a viragem de ano: na primeira
	// emissão de uma série nova a linha ainda não existe, e criá-la fora do lock
	// seria ela própria uma corrida.
	if _, err := tx.Exec(ctx,
		`INSERT INTO financeiro.series (serie) VALUES ($1) ON CONFLICT DO NOTHING`, serie); err != nil {
		return nil, fmt.Errorf("garantir a série: %w", err)
	}

	var ultimoSeq int
	var ultimoHash string
	if err := tx.QueryRow(ctx, `
SELECT ultimo_sequencial, ultimo_hash FROM financeiro.series
 WHERE serie=$1 FOR UPDATE`, serie).Scan(&ultimoSeq, &ultimoHash); err != nil {
		return nil, fmt.Errorf("bloquear a série: %w", err)
	}

	f, err := r.obter(ctx, tx, facturaID)
	if err != nil {
		return nil, err
	}
	if err := f.Emitir(serie, ultimoSeq+1, ultimoHash, momento); err != nil {
		return nil, err
	}
	s := f.Snapshot()

	// A guarda por estado e versão fecha a corrida com uma edição do rascunho a
	// decorrer noutra transacção (o mesmo bloqueio optimista do Guardar).
	var versaoNova int
	var actualizadoEm time.Time
	err = tx.QueryRow(ctx, `
UPDATE financeiro.facturas
   SET estado='EMITIDA', numero=$2, serie=$3, sequencial=$4, data_emissao=$5,
       hash=$6, hash_anterior=$7, versao=versao+1, actualizado_em=now()
 WHERE id=$1 AND estado='RASCUNHO' AND versao=$8
 RETURNING versao, actualizado_em`,
		facturaID, s.Numero.String(), s.Serie, s.Sequencial, s.DataEmissao,
		s.Hash, s.HashAnterior, s.Versao).Scan(&versaoNova, &actualizadoEm)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, erros.Novo(erros.CategoriaConflito,
				"a factura já não está em rascunho ou foi alterada entretanto")
		}
		return nil, fmt.Errorf("emitir factura: %w", err)
	}

	if _, err := tx.Exec(ctx, `
UPDATE financeiro.series SET ultimo_sequencial=$2, ultimo_hash=$3, actualizado_em=now()
 WHERE serie=$1`, serie, s.Sequencial, s.Hash); err != nil {
		return nil, fmt.Errorf("avançar a série: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("confirmar a emissão: %w", err)
	}

	// Devolver o agregado com a versão que ficou gravada: reutilizar a instância
	// com a versão antiga faria a escrita seguinte falhar com um conflito falso.
	s.Versao = versaoNova
	s.ActualizadoEm = actualizadoEm
	return fin.ReconstruirFactura(s), nil
}

// ListarSnapshotsPorSerie devolve os snapshots das facturas emitidas de uma
// série, ordenados por sequencial — a entrada de VerificarCadeia.
func (r *RepositorioFacturas) ListarSnapshotsPorSerie(ctx context.Context, serie string) ([]fin.SnapshotFactura, error) {
	linhas, err := r.pool.Query(ctx, colunasFactura+`
 WHERE serie=$1 AND estado <> 'RASCUNHO'
 ORDER BY sequencial`, serie)
	if err != nil {
		return nil, fmt.Errorf("listar facturas da série: %w", err)
	}
	defer linhas.Close()

	var snaps []fin.SnapshotFactura
	for linhas.Next() {
		s, err := lerFactura(linhas)
		if err != nil {
			return nil, fmt.Errorf("ler factura da série: %w", err)
		}
		snaps = append(snaps, s)
	}
	if err := linhas.Err(); err != nil {
		return nil, fmt.Errorf("ler facturas da série: %w", err)
	}
	// As linhas só se leem depois de fechar o cursor: o pool tem uma ligação por
	// consulta e o cursor acima ainda a ocupa.
	for i := range snaps {
		itens, err := r.itensDe(ctx, r.pool, snaps[i].ID)
		if err != nil {
			return nil, err
		}
		snaps[i].Itens = itens
	}
	return snaps, nil
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
