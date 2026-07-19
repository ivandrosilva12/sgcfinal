// Package financeiro é o Bounded Context Financeiro (Camada 1 — Domínio).
// O agregado Factura nasce em RASCUNHO (ADR-039): linhas com tipo e snapshot,
// cálculo de IVA e totais. A emissão (ADR-040) fixa número, data e o hash
// SHA-256 canónico — invariante do agregado, calculado aqui, nunca num serviço.
// A composição canónica é injectiva por enquadramento (ADR-041): todo o campo de
// texto leva prefixo de comprimento, e o selo cobre a identidade completa do
// cliente e a proveniência de cada linha.
package financeiro

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
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
	numero        NumeroFactura
	serie         string
	sequencial    int
	dataEmissao   time.Time
	hash          string
	hashAnterior  string
	versao        int
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

// TotaisDe soma, por linha, subtotais e IVA. Partilhada pelo agregado e pela
// verificação da cadeia, que trabalha sobre snapshots.
func TotaisDe(itens []ItemFactura) Totais {
	sub := moeda.DeCentimos(0)
	iva := moeda.DeCentimos(0)
	for _, it := range itens {
		sub = sub.Somar(it.Subtotal())
		iva = iva.Somar(it.ValorIVA())
	}
	return Totais{Subtotal: sub, TotalIVA: iva, Total: sub.Somar(iva)}
}

// Totais calcula os totais da factura.
func (f *Factura) Totais() Totais { return TotaisDe(f.itens) }

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

// CriadoEm devolve o instante de criação da factura.
func (f *Factura) CriadoEm() time.Time { return f.criadoEm }

// Emitir transita a factura de RASCUNHO para EMITIDA, fixando número, data e o
// seu elo na cadeia de integridade. O hash é calculado aqui — é invariante do
// agregado, nunca de um serviço (antipadrão M4).
func (f *Factura) Emitir(serie string, sequencial int, hashAnterior string, momento time.Time) error {
	if f.estado != FactRascunho {
		return erros.Novo(erros.CategoriaConflito, "só é possível emitir uma factura em rascunho")
	}
	if len(f.itens) == 0 {
		return erros.Novo(erros.CategoriaRegraNegocio, "não é possível emitir uma factura sem linhas")
	}
	numero, err := NovoNumeroFactura(serie, sequencial)
	if err != nil {
		return err
	}
	f.numero = numero
	f.serie = strings.TrimSpace(serie)
	f.sequencial = sequencial
	f.dataEmissao = momento.UTC().Truncate(time.Second)
	f.hashAnterior = hashAnterior
	f.estado = FactEmitida
	f.hash = HashDe(f.Snapshot())
	return nil
}

// enquadrar prefixa um campo de texto com o seu comprimento em bytes, tornando a
// composição canónica injectiva: nenhum conteúdo consegue imitar um separador,
// porque quem lê consome exactamente os bytes anunciados.
//
// A regra é cega de propósito — aplica-se a todo o campo de texto, sem excepções.
// O defeito que a ADR-041 corrige nasceu precisamente de se ter julgado quais os
// campos eram seguros ("as descrições vêm de catálogo") e de esse juízo estar
// errado. Uma regra sem excepções não pode ser mal julgada por quem vier a seguir.
func enquadrar(s string) string { return strconv.Itoa(len(s)) + ":" + s }

// digestLinhas resume as linhas por ordem, selando descrição, tipo, quantidade,
// preço, regime e proveniência — não só o total. Sem isto, trocar "Consulta" por
// "Cirurgia" mantendo o valor passaria despercebido.
func digestLinhas(itens []ItemFactura) string {
	h := sha256.New()
	for ordem, it := range itens {
		// hash.Hash.Write nunca devolve erro (contrato do pacote hash), pelo que o
		// retorno se descarta explicitamente — errcheck exige-o e um panic aqui
		// seria pior: partiria a emissão por uma condição que não pode ocorrer.
		_, _ = fmt.Fprintf(h, "%d|%s|%s|%d|%d|%s|%s\n", ordem,
			enquadrar(it.Descricao), enquadrar(string(it.Tipo)),
			it.Quantidade, it.PrecoUnitario.Centimos(),
			enquadrar(string(it.RegimeIVA)), enquadrar(it.OperacaoID))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// HashDe calcula o SHA-256 canónico de uma factura a partir do seu snapshot.
// O formato está documentado na ADR-041 e é fixo: não deriva do esquema da BD,
// para continuar reproduzível ao longo dos 10 anos de retenção legal. Todo o
// campo de texto vai enquadrado (comprimento em bytes + ':'); os inteiros vão
// nus. Campos ausentes canonicalizam-se como "0:" — nunca null.
//
//	serie|sequencial|dataEmissaoRFC3339UTC|clienteNome|clienteNIF|clienteMorada|subtotal|iva|total|digestLinhas|hashAnterior
func HashDe(s SnapshotFactura) string {
	t := TotaisDe(s.Itens)
	canonico := strings.Join([]string{
		enquadrar(s.Serie),
		strconv.Itoa(s.Sequencial),
		enquadrar(s.DataEmissao.UTC().Truncate(time.Second).Format(time.RFC3339)),
		enquadrar(s.Cliente.Nome),
		enquadrar(s.Cliente.NIF),
		enquadrar(s.Cliente.Morada),
		strconv.FormatInt(t.Subtotal.Centimos(), 10),
		strconv.FormatInt(t.TotalIVA.Centimos(), 10),
		strconv.FormatInt(t.Total.Centimos(), 10),
		enquadrar(digestLinhas(s.Itens)),
		enquadrar(s.HashAnterior),
	}, "|")
	soma := sha256.Sum256([]byte(canonico))
	return hex.EncodeToString(soma[:])
}

// Numero devolve o número legal (vazio enquanto RASCUNHO).
func (f *Factura) Numero() NumeroFactura { return f.numero }

// Serie devolve a série da factura.
func (f *Factura) Serie() string { return f.serie }

// Sequencial devolve o sequencial dentro da série.
func (f *Factura) Sequencial() int { return f.sequencial }

// DataEmissao devolve o instante da emissão (zero enquanto RASCUNHO).
func (f *Factura) DataEmissao() time.Time { return f.dataEmissao }

// Hash devolve o elo desta factura na cadeia de integridade.
func (f *Factura) Hash() string { return f.hash }

// HashAnterior devolve o elo da factura imediatamente anterior na série.
func (f *Factura) HashAnterior() string { return f.hashAnterior }

// Versao devolve a versão para bloqueio optimista.
func (f *Factura) Versao() int { return f.versao }

// SnapshotFactura carrega o estado completo para persistência ou rehidratação.
type SnapshotFactura struct {
	ID            string
	Estado        EstadoFactura
	Cliente       ClienteSnapshot
	EpisodioID    string
	Itens         []ItemFactura
	CriadoEm      time.Time
	ActualizadoEm time.Time
	Numero        NumeroFactura
	Serie         string
	Sequencial    int
	DataEmissao   time.Time
	Hash          string
	HashAnterior  string
	Versao        int
}

// Snapshot devolve o estado completo do agregado.
func (f *Factura) Snapshot() SnapshotFactura {
	itens := make([]ItemFactura, len(f.itens))
	copy(itens, f.itens)
	return SnapshotFactura{
		ID: f.id, Estado: f.estado, Cliente: f.cliente, EpisodioID: f.episodioID,
		Itens: itens, CriadoEm: f.criadoEm, ActualizadoEm: f.actualizadoEm,
		Numero: f.numero, Serie: f.serie, Sequencial: f.sequencial,
		DataEmissao: f.dataEmissao, Hash: f.hash, HashAnterior: f.hashAnterior,
		Versao: f.versao,
	}
}

// ReconstruirFactura reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirFactura(s SnapshotFactura) *Factura {
	itens := make([]ItemFactura, len(s.Itens))
	copy(itens, s.Itens)
	return &Factura{
		id: s.ID, estado: s.Estado, cliente: s.Cliente, episodioID: s.EpisodioID,
		itens: itens, criadoEm: s.CriadoEm, actualizadoEm: s.ActualizadoEm,
		numero: s.Numero, serie: s.Serie, sequencial: s.Sequencial,
		dataEmissao: s.DataEmissao, hash: s.Hash, hashAnterior: s.HashAnterior,
		versao: s.Versao,
	}
}

// ResumoFactura é a projecção de leitura de uma factura (listagem).
type ResumoFactura struct {
	ID            string    `json:"id"`
	Estado        string    `json:"estado"`
	ClienteNome   string    `json:"cliente_nome"`
	EpisodioID    string    `json:"episodio_id,omitempty"`
	NumItens      int       `json:"num_itens"`
	TotalCentimos int64     `json:"total_centimos"`
	Total         string    `json:"total"`
	CriadoEm      time.Time `json:"criado_em"`
}

// RepositorioFacturas é a porta de saída de persistência de facturas.
//
// Guardar é um upsert transaccional guardado por estado e versão (bloqueio
// optimista): INSERT da factura (id gerado) quando nova, ou UPDATE guardado por
// estado=RASCUNHO quando existente, reescrevendo as linhas numa única transacção.
// Devolve o id da factura.
//
// Emitir aloca o sequencial e o elo da cadeia sob serialização e transita a
// factura para EMITIDA numa única transacção. O domínio não sabe como — só que
// a alocação é atómica e sem buracos (AGT/SAF-T-AO).
//
// ListarSnapshotsPorSerie devolve, ordenados por sequencial, os snapshots das
// facturas emitidas de uma série: a entrada de VerificarCadeia.
type RepositorioFacturas interface {
	Guardar(ctx context.Context, f *Factura) (string, error)
	ObterPorID(ctx context.Context, id string) (*Factura, error)
	ListarPorEpisodio(ctx context.Context, episodioID string) ([]ResumoFactura, error)
	Emitir(ctx context.Context, facturaID string, momento time.Time) (*Factura, error)
	ListarSnapshotsPorSerie(ctx context.Context, serie string) ([]SnapshotFactura, error)
}
