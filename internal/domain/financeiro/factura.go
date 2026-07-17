// Package financeiro é o Bounded Context Financeiro (Camada 1 — Domínio).
// Esta fatia (ADR-039) entrega o agregado Factura em estado RASCUNHO: linhas com
// tipo e snapshot, cálculo de IVA e totais. A emissão (cadeia hash, numeração,
// imutabilidade) é do ADR-040.
package financeiro

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/identity"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
)

// EstadoFactura é o estado do ciclo de vida de uma factura. Nesta fatia só
// RASCUNHO é alcançável; EMITIDA e ANULADA já figuram no enum (e na CHECK da BD)
// para o ADR-040/041, à imagem do padrão do BC Laboratório.
type EstadoFactura string

const (
	FactRascunho EstadoFactura = "RASCUNHO"
	FactEmitida  EstadoFactura = "EMITIDA"
	FactAnulada  EstadoFactura = "ANULADA"
)

// TipoLinha classifica a operação clínica de origem de uma linha de factura.
type TipoLinha string

const (
	LinhaConsulta              TipoLinha = "CONSULTA"
	LinhaDispensa              TipoLinha = "DISPENSA"
	LinhaExameAnalise          TipoLinha = "EXAME_ANALISE"
	LinhaEstudoImagem          TipoLinha = "ESTUDO_IMAGEM"
	LinhaProcedimentoCirurgico TipoLinha = "PROCEDIMENTO_CIRURGICO"
)

var tiposValidos = map[TipoLinha]bool{
	LinhaConsulta: true, LinhaDispensa: true, LinhaExameAnalise: true,
	LinhaEstudoImagem: true, LinhaProcedimentoCirurgico: true,
}

// Valido indica se o tipo é um dos valores canónicos.
func (t TipoLinha) Valido() bool { return tiposValidos[t] }

// ExigeOperacao indica se a linha tem de referenciar o id lógico da operação de
// origem. A CONSULTA liga-se ao episódio da factura; as restantes referenciam a
// operação concreta (dispensa, requisição, estudo de imagem, procedimento).
func (t TipoLinha) ExigeOperacao() bool { return t != LinhaConsulta }

// RegimeIVA é o regime de IVA de uma linha, configurável por item (CLAUDE.md §8):
// saúde geralmente isenta; produtos/serviços tributados à taxa standard.
type RegimeIVA string

const (
	RegimeIsento   RegimeIVA = "ISENTO"
	RegimeStandard RegimeIVA = "STANDARD"
)

// Valido indica se o regime é conhecido.
func (r RegimeIVA) Valido() bool { return r == RegimeIsento || r == RegimeStandard }

// TaxaPercent devolve a taxa de IVA em pontos percentuais inteiros.
func (r RegimeIVA) TaxaPercent() int64 {
	if r == RegimeStandard {
		return 14
	}
	return 0
}

// ClienteSnapshot é a fotografia dos dados fiscais do cliente no momento da
// factura. É um snapshot imutável — sem FK ao Doente (linguagem ubíqua: Cliente).
type ClienteSnapshot struct {
	Nome   string
	NIF    string
	Morada string
}

// NovoClienteSnapshot valida e normaliza o snapshot do cliente. O nome é
// obrigatório; o NIF, se presente, é validado pelo VO do Shared Kernel.
func NovoClienteSnapshot(nome, nif, morada string) (ClienteSnapshot, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return ClienteSnapshot{}, erros.Novo(erros.CategoriaValidacao, "nome do cliente em falta")
	}
	nif = strings.TrimSpace(nif)
	if nif != "" {
		n, err := identity.NovoNIF(nif)
		if err != nil {
			return ClienteSnapshot{}, erros.Novo(erros.CategoriaValidacao, "NIF do cliente inválido")
		}
		nif = n.String()
	}
	return ClienteSnapshot{Nome: nome, NIF: nif, Morada: strings.TrimSpace(morada)}, nil
}

var (
	_ = time.Time{} // usado pelo agregado nas tasks seguintes
	_ = moeda.AOA{} // usado pelo agregado nas tasks seguintes
)
