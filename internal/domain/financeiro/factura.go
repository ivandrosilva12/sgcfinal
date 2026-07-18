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

// ItemFactura é uma linha de factura: entidade-filho do agregado Factura. Guarda
// o snapshot (descrição e preço) da operação de origem — sem FK cross-context.
type ItemFactura struct {
	ID            string
	Descricao     string
	Tipo          TipoLinha
	OperacaoID    string
	Quantidade    int
	PrecoUnitario moeda.AOA
	RegimeIVA     RegimeIVA
}

// Subtotal é preço unitário × quantidade (antes de IVA).
func (i ItemFactura) Subtotal() moeda.AOA {
	return moeda.DeCentimos(i.PrecoUnitario.Centimos() * int64(i.Quantidade))
}

// ValorIVA é o IVA da linha, em aritmética inteira de cêntimos, arredondado
// meia-acima. Linha isenta → 0.
func (i ItemFactura) ValorIVA() moeda.AOA {
	taxa := i.RegimeIVA.TaxaPercent()
	if taxa == 0 {
		return moeda.DeCentimos(0)
	}
	sub := i.Subtotal().Centimos()
	return moeda.DeCentimos((sub*taxa + 50) / 100)
}

// Total é subtotal + IVA da linha.
func (i ItemFactura) Total() moeda.AOA { return i.Subtotal().Somar(i.ValorIVA()) }

// Factura é o agregado raiz do BC Financeiro. Nasce em RASCUNHO; as linhas
// podem ser adicionadas e removidas enquanto está em rascunho. A emissão
// (ADR-040) fixa a factura.
type Factura struct {
	id            string
	estado        EstadoFactura
	cliente       ClienteSnapshot
	episodioID    string
	itens         []ItemFactura
	criadoEm      time.Time
	actualizadoEm time.Time
}

// NovaFactura cria uma factura em RASCUNHO, sem itens. O episodioID é um id
// lógico (uuid) sem FK cross-context.
func NovaFactura(cliente ClienteSnapshot, episodioID string) (*Factura, error) {
	if cliente.Nome == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "cliente da factura em falta")
	}
	episodioID = strings.TrimSpace(episodioID)
	if episodioID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "episódio da factura em falta")
	}
	return &Factura{estado: FactRascunho, cliente: cliente, episodioID: episodioID}, nil
}

// AdicionarItem acrescenta uma linha. Só é permitido em RASCUNHO. O item nasce
// sem id — o repositório atribui-o na persistência.
func (f *Factura) AdicionarItem(descricao string, tipo TipoLinha, operacaoID string, quantidade int, preco moeda.AOA, regime RegimeIVA) error {
	if f.estado != FactRascunho {
		return erros.Novo(erros.CategoriaConflito, "só é possível alterar linhas de uma factura em rascunho")
	}
	descricao = strings.TrimSpace(descricao)
	if descricao == "" {
		return erros.Novo(erros.CategoriaValidacao, "descrição da linha em falta")
	}
	if !tipo.Valido() {
		return erros.Novo(erros.CategoriaValidacao, "tipo de linha inválido")
	}
	operacaoID = strings.TrimSpace(operacaoID)
	if tipo.ExigeOperacao() && operacaoID == "" {
		return erros.Novo(erros.CategoriaValidacao, "operação de origem da linha em falta")
	}
	if quantidade <= 0 {
		return erros.Novo(erros.CategoriaValidacao, "quantidade tem de ser positiva")
	}
	if preco.Negativo() {
		return erros.Novo(erros.CategoriaValidacao, "preço unitário não pode ser negativo")
	}
	if !regime.Valido() {
		return erros.Novo(erros.CategoriaValidacao, "regime de IVA inválido")
	}
	f.itens = append(f.itens, ItemFactura{
		Descricao: descricao, Tipo: tipo, OperacaoID: operacaoID,
		Quantidade: quantidade, PrecoUnitario: preco, RegimeIVA: regime,
	})
	return nil
}

// RemoverItem retira a linha com o id dado. Só é permitido em RASCUNHO.
func (f *Factura) RemoverItem(itemID string) error {
	if f.estado != FactRascunho {
		return erros.Novo(erros.CategoriaConflito, "só é possível alterar linhas de uma factura em rascunho")
	}
	for idx, it := range f.itens {
		if it.ID == itemID && itemID != "" {
			f.itens = append(f.itens[:idx], f.itens[idx+1:]...)
			return nil
		}
	}
	return erros.Novo(erros.CategoriaNaoEncontrado, "linha da factura não encontrada")
}

// Totais soma, por linha, os subtotais e o IVA (arredondar por linha e somar,
// prática fiscal). O total autoritário vive aqui — nunca em SQL.
type Totais struct {
	Subtotal moeda.AOA
	TotalIVA moeda.AOA
	Total    moeda.AOA
}

// Totais calcula os totais da factura.
func (f *Factura) Totais() Totais {
	sub := moeda.DeCentimos(0)
	iva := moeda.DeCentimos(0)
	for _, it := range f.itens {
		sub = sub.Somar(it.Subtotal())
		iva = iva.Somar(it.ValorIVA())
	}
	return Totais{Subtotal: sub, TotalIVA: iva, Total: sub.Somar(iva)}
}

// ID devolve o identificador (vazio antes de persistir).
func (f *Factura) ID() string { return f.id }

// Estado devolve o estado actual.
func (f *Factura) Estado() EstadoFactura { return f.estado }

// Cliente devolve o snapshot do cliente.
func (f *Factura) Cliente() ClienteSnapshot { return f.cliente }

// EpisodioID devolve o id lógico do episódio.
func (f *Factura) EpisodioID() string { return f.episodioID }

// Itens devolve uma cópia das linhas.
func (f *Factura) Itens() []ItemFactura {
	out := make([]ItemFactura, len(f.itens))
	copy(out, f.itens)
	return out
}

// SnapshotFactura carrega o estado completo para persistência ou rehidratação.
type SnapshotFactura struct {
	ID            string
	Estado        EstadoFactura
	Cliente       ClienteSnapshot
	EpisodioID    string
	Itens         []ItemFactura
	CriadoEm      time.Time
	ActualizadoEm time.Time
}

// Snapshot devolve o estado completo do agregado.
func (f *Factura) Snapshot() SnapshotFactura {
	itens := make([]ItemFactura, len(f.itens))
	copy(itens, f.itens)
	return SnapshotFactura{
		ID: f.id, Estado: f.estado, Cliente: f.cliente, EpisodioID: f.episodioID,
		Itens: itens, CriadoEm: f.criadoEm, ActualizadoEm: f.actualizadoEm,
	}
}

// ReconstruirFactura reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirFactura(s SnapshotFactura) *Factura {
	itens := make([]ItemFactura, len(s.Itens))
	copy(itens, s.Itens)
	return &Factura{
		id: s.ID, estado: s.Estado, cliente: s.Cliente, episodioID: s.EpisodioID,
		itens: itens, criadoEm: s.CriadoEm, actualizadoEm: s.ActualizadoEm,
	}
}
