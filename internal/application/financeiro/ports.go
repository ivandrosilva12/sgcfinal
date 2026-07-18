// Package financeiro contém os casos de uso do BC Financeiro (Camada 2 — Aplicação).
package financeiro

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// Auditor persiste registos de auditoria de forma append-only.
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}

// Reexport do read model do domínio.
type ResumoFactura = dominio.ResumoFactura

// DadosNovaFactura é a entrada da criação de uma factura em rascunho.
type DadosNovaFactura struct {
	EpisodioID    string `json:"episodio_id"`
	ClienteNome   string `json:"cliente_nome"`
	ClienteNIF    string `json:"cliente_nif"`
	ClienteMorada string `json:"cliente_morada"`
}

// DadosNovoItem é a entrada da adição de uma linha. FacturaID vem do caminho.
type DadosNovoItem struct {
	FacturaID             string
	Descricao             string `json:"descricao"`
	Tipo                  string `json:"tipo"`
	OperacaoID            string `json:"operacao_id"`
	Quantidade            int    `json:"quantidade"`
	PrecoUnitarioCentimos int64  `json:"preco_unitario_centimos"`
	RegimeIVA             string `json:"regime_iva"`
}

// LinhaDetalhe é uma linha de factura numa resposta.
type LinhaDetalhe struct {
	ID                    string `json:"id"`
	Descricao             string `json:"descricao"`
	Tipo                  string `json:"tipo"`
	OperacaoID            string `json:"operacao_id,omitempty"`
	Quantidade            int    `json:"quantidade"`
	PrecoUnitarioCentimos int64  `json:"preco_unitario_centimos"`
	RegimeIVA             string `json:"regime_iva"`
	SubtotalCentimos      int64  `json:"subtotal_centimos"`
	ValorIVACentimos      int64  `json:"valor_iva_centimos"`
	TotalCentimos         int64  `json:"total_centimos"`
}

// DetalheFactura é o detalhe de uma factura numa resposta.
type DetalheFactura struct {
	ID               string         `json:"id"`
	Estado           string         `json:"estado"`
	ClienteNome      string         `json:"cliente_nome"`
	ClienteNIF       string         `json:"cliente_nif,omitempty"`
	ClienteMorada    string         `json:"cliente_morada,omitempty"`
	EpisodioID       string         `json:"episodio_id,omitempty"`
	Itens            []LinhaDetalhe `json:"itens"`
	SubtotalCentimos int64          `json:"subtotal_centimos"`
	TotalIVACentimos int64          `json:"total_iva_centimos"`
	TotalCentimos    int64          `json:"total_centimos"`
	Total            string         `json:"total"`
	CriadoEm         time.Time      `json:"criado_em"`
	Numero           string         `json:"numero,omitempty"`
	Serie            string         `json:"serie,omitempty"`
	Sequencial       int            `json:"sequencial,omitempty"`
	DataEmissao      time.Time      `json:"data_emissao,omitempty"`
	Hash             string         `json:"hash,omitempty"`
}

// ResultadoVerificacao é o diagnóstico da cadeia hash de uma série.
type ResultadoVerificacao struct {
	Serie         string `json:"serie"`
	TotalFacturas int    `json:"total_facturas"`
	Integra       bool   `json:"integra"`
	Detalhe       string `json:"detalhe,omitempty"`
}
