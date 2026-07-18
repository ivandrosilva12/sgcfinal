package financeiro_test

import (
	"testing"
	"time"

	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
)

func TestTipoLinhaExigeOperacao(t *testing.T) {
	if fin.LinhaConsulta.ExigeOperacao() {
		t.Error("CONSULTA liga-se ao episódio, não exige operacaoID")
	}
	for _, tp := range []fin.TipoLinha{fin.LinhaDispensa, fin.LinhaExameAnalise, fin.LinhaEstudoImagem, fin.LinhaProcedimentoCirurgico} {
		if !tp.ExigeOperacao() {
			t.Errorf("%s devia exigir operacaoID", tp)
		}
	}
	if fin.TipoLinha("XPTO").Valido() {
		t.Error("tipo desconhecido não é válido")
	}
}

func TestRegimeIVATaxa(t *testing.T) {
	if fin.RegimeIsento.TaxaPercent() != 0 {
		t.Error("ISENTO tem taxa 0")
	}
	if fin.RegimeStandard.TaxaPercent() != 14 {
		t.Error("STANDARD tem taxa 14%")
	}
	if fin.RegimeIVA("OUTRO").Valido() {
		t.Error("regime desconhecido não é válido")
	}
}

func TestNovoClienteSnapshot(t *testing.T) {
	if _, err := fin.NovoClienteSnapshot("", "", ""); err == nil {
		t.Error("nome do cliente é obrigatório")
	}
	c, err := fin.NovoClienteSnapshot("  Clínica Sol  ", "", " Rua 1 ")
	if err != nil {
		t.Fatalf("cliente válido devia passar: %v", err)
	}
	if c.Nome != "Clínica Sol" || c.Morada != "Rua 1" {
		t.Errorf("campos não normalizados: %+v", c)
	}
	_, err = fin.NovoClienteSnapshot("X", "NIF-INVALIDO!!", "")
	if err == nil {
		t.Fatal("NIF presente e inválido devia falhar")
	}
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Errorf("categoria = %v; esperava Validacao", erros.CategoriaDe(err))
	}
}

func TestItemFacturaCalculo(t *testing.T) {
	// 2 × 1.000,00 Kz = 2.000,00 Kz; IVA 14% = 280,00 Kz.
	it := fin.ItemFactura{
		Descricao: "Consulta", Tipo: fin.LinhaConsulta,
		Quantidade: 2, PrecoUnitario: moeda.DeKwanzas(1000), RegimeIVA: fin.RegimeStandard,
	}
	if got := it.Subtotal().Centimos(); got != 200000 {
		t.Errorf("subtotal = %d; esperava 200000", got)
	}
	if got := it.ValorIVA().Centimos(); got != 28000 {
		t.Errorf("IVA = %d; esperava 28000", got)
	}
	if got := it.Total().Centimos(); got != 228000 {
		t.Errorf("total = %d; esperava 228000", got)
	}

	// Isento: IVA = 0.
	isento := fin.ItemFactura{Quantidade: 1, PrecoUnitario: moeda.DeKwanzas(500), RegimeIVA: fin.RegimeIsento}
	if isento.ValorIVA().Centimos() != 0 {
		t.Error("linha isenta tem IVA 0")
	}

	// Arredondamento meia-acima: 1 × 3,21 Kz (321 cent) × 14% = 44,94 → 44,94? 321*14=4494; (4494+50)/100=45.
	arred := fin.ItemFactura{Quantidade: 1, PrecoUnitario: moeda.DeCentimos(321), RegimeIVA: fin.RegimeStandard}
	if got := arred.ValorIVA().Centimos(); got != 45 {
		t.Errorf("IVA arredondado = %d; esperava 45", got)
	}
}

func novaFacturaValida(t *testing.T) *fin.Factura {
	t.Helper()
	c, err := fin.NovoClienteSnapshot("Clínica Sol", "", "")
	if err != nil {
		t.Fatal(err)
	}
	f, err := fin.NovaFactura(c, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestNovaFacturaArrancaEmRascunho(t *testing.T) {
	f := novaFacturaValida(t)
	if f.Estado() != fin.FactRascunho {
		t.Errorf("estado = %s; esperava RASCUNHO", f.Estado())
	}
	if len(f.Itens()) != 0 {
		t.Error("factura nova não tem itens")
	}
}

func TestAdicionarItemInvariantes(t *testing.T) {
	f := novaFacturaValida(t)
	// Quantidade tem de ser > 0.
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 0, moeda.DeKwanzas(10), fin.RegimeIsento); err == nil {
		t.Error("quantidade 0 devia falhar")
	}
	// DISPENSA exige operacaoID.
	if err := f.AdicionarItem("Paracetamol", fin.LinhaDispensa, "", 1, moeda.DeKwanzas(10), fin.RegimeStandard); err == nil {
		t.Error("DISPENSA sem operacaoID devia falhar")
	}
	// Tipo inválido.
	if err := f.AdicionarItem("X", fin.TipoLinha("X"), "", 1, moeda.DeKwanzas(10), fin.RegimeIsento); err == nil {
		t.Error("tipo inválido devia falhar")
	}
	// Linha válida.
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(5000), fin.RegimeIsento); err != nil {
		t.Fatalf("linha válida devia passar: %v", err)
	}
	if len(f.Itens()) != 1 {
		t.Errorf("esperava 1 item; tem %d", len(f.Itens()))
	}
}

func TestTotaisSomaPorLinha(t *testing.T) {
	f := novaFacturaValida(t)
	_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(5000), fin.RegimeIsento)
	_ = f.AdicionarItem("Medicamento", fin.LinhaDispensa, "22222222-2222-2222-2222-222222222222", 2, moeda.DeKwanzas(1000), fin.RegimeStandard)
	tot := f.Totais()
	// Subtotal: 5000 + 2×1000 = 7000 Kz = 700000 cent.
	if tot.Subtotal.Centimos() != 700000 {
		t.Errorf("subtotal = %d; esperava 700000", tot.Subtotal.Centimos())
	}
	// IVA: consulta isenta (0) + medicamento 14% de 200000 = 28000.
	if tot.TotalIVA.Centimos() != 28000 {
		t.Errorf("IVA = %d; esperava 28000", tot.TotalIVA.Centimos())
	}
	if tot.Total.Centimos() != 728000 {
		t.Errorf("total = %d; esperava 728000", tot.Total.Centimos())
	}
}

func TestRemoverItem(t *testing.T) {
	f := novaFacturaValida(t)
	_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(5000), fin.RegimeIsento)
	// Dar id ao item via reconstrução (simula item já persistido).
	s := f.Snapshot()
	s.Itens[0].ID = "item-1"
	f = fin.ReconstruirFactura(s)
	if err := f.RemoverItem("nao-existe"); err == nil {
		t.Error("remover item inexistente devia falhar")
	}
	if err := f.RemoverItem("item-1"); err != nil {
		t.Fatalf("remover item existente: %v", err)
	}
	if len(f.Itens()) != 0 {
		t.Error("item devia ter sido removido")
	}
}

func TestNovaFacturaValidaCliente(t *testing.T) {
	if _, err := fin.NovaFactura(fin.ClienteSnapshot{}, "11111111-1111-1111-1111-111111111111"); err == nil {
		t.Error("cliente sem nome devia falhar")
	}
	c, _ := fin.NovoClienteSnapshot("Clínica Sol", "", "")
	if _, err := fin.NovaFactura(c, "   "); err == nil {
		t.Error("episodioID em branco devia falhar")
	}
}

func TestFacturaGetters(t *testing.T) {
	c, _ := fin.NovoClienteSnapshot("Clínica Sol", "", "")
	f, err := fin.NovaFactura(c, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if f.ID() != "" {
		t.Error("id vazio antes de persistir")
	}
	if f.Cliente() != c {
		t.Errorf("cliente = %+v; esperava %+v", f.Cliente(), c)
	}
	if f.EpisodioID() != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("episodioID = %s", f.EpisodioID())
	}
	// CriadoEm: zero antes de persistir; reconstrução a partir de snapshot
	// devolve o instante gravado.
	if !f.CriadoEm().IsZero() {
		t.Errorf("criadoEm devia ser zero antes de persistir; tem %v", f.CriadoEm())
	}
	esperado := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	s := f.Snapshot()
	s.CriadoEm = esperado
	f = fin.ReconstruirFactura(s)
	if !f.CriadoEm().Equal(esperado) {
		t.Errorf("criadoEm = %v; esperava %v", f.CriadoEm(), esperado)
	}
}

func TestAdicionarItemPrecoNegativoERegimeInvalido(t *testing.T) {
	f := novaFacturaValida(t)
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeCentimos(-100), fin.RegimeIsento); err == nil {
		t.Error("preço negativo devia falhar")
	}
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(10), fin.RegimeIVA("OUTRO")); err == nil {
		t.Error("regime inválido devia falhar")
	}
}

func TestAlterarLinhasForaDeRascunhoFalha(t *testing.T) {
	f := novaFacturaValida(t)
	s := f.Snapshot()
	s.Estado = fin.FactEmitida
	f = fin.ReconstruirFactura(s)
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeKwanzas(10), fin.RegimeIsento); err == nil {
		t.Error("adicionar item fora de RASCUNHO devia falhar")
	}
	if err := f.RemoverItem("qualquer"); err == nil {
		t.Error("remover item fora de RASCUNHO devia falhar")
	}
}

func TestResumoFacturaCampos(t *testing.T) {
	var _ fin.RepositorioFacturas // a porta tem de existir
	r := fin.ResumoFactura{ID: "f1", Estado: "RASCUNHO", ClienteNome: "Sol", NumItens: 2, TotalCentimos: 728000}
	if r.TotalCentimos != 728000 {
		t.Error("ResumoFactura deve expor o total em cêntimos")
	}
}

func TestEmitir_FixaNumeroDataEHash(t *testing.T) {
	f := novaFacturaValida(t)
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1,
		moeda.DeCentimos(50000), fin.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem: %v", err)
	}
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	if err := f.Emitir("2026", 1, "", m); err != nil {
		t.Fatalf("Emitir: %v", err)
	}
	if f.Estado() != fin.FactEmitida {
		t.Errorf("estado = %q, queria EMITIDA", f.Estado())
	}
	if got := f.Numero().String(); got != "FAC 2026/00000001" {
		t.Errorf("número = %q", got)
	}
	if f.Hash() == "" {
		t.Error("hash não podia ficar vazio")
	}
	if len(f.Hash()) != 64 {
		t.Errorf("hash tem %d caracteres, queria 64 (SHA-256 hex)", len(f.Hash()))
	}
}

func TestEmitir_SoDeRascunho(t *testing.T) {
	f := novaFacturaValida(t)
	_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeCentimos(1000), fin.RegimeIsento)
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	if err := f.Emitir("2026", 1, "", m); err != nil {
		t.Fatalf("primeira emissão: %v", err)
	}
	err := f.Emitir("2026", 2, f.Hash(), m)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Errorf("segunda emissão devia dar Conflito, deu %v", err)
	}
}

func TestEmitir_RecusaFacturaSemLinhas(t *testing.T) {
	f := novaFacturaValida(t)
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	err := f.Emitir("2026", 1, "", m)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Errorf("factura sem linhas devia dar RegraNegocio, deu %v", err)
	}
}

func TestHash_DeterministicoEEstavel(t *testing.T) {
	cria := func() *fin.Factura {
		f := novaFacturaValida(t)
		_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeCentimos(50000), fin.RegimeIsento)
		return f
	}
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	a, b := cria(), cria()
	_ = a.Emitir("2026", 1, "", m)
	_ = b.Emitir("2026", 1, "", m)
	if a.Hash() != b.Hash() {
		t.Error("mesma entrada devia dar o mesmo hash")
	}
}

func TestHash_IgnoraSubSegundo(t *testing.T) {
	cria := func() *fin.Factura {
		f := novaFacturaValida(t)
		_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeCentimos(50000), fin.RegimeIsento)
		return f
	}
	base := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	a, b := cria(), cria()
	_ = a.Emitir("2026", 1, "", base)
	_ = b.Emitir("2026", 1, "", base.Add(999*time.Millisecond))
	if a.Hash() != b.Hash() {
		t.Error("sub-segundo não podia entrar no hash: o valor relido da BD tem outra precisão")
	}
}

func TestHash_SelaConteudoDaLinha(t *testing.T) {
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	comDescricao := func(d string) string {
		f := novaFacturaValida(t)
		_ = f.AdicionarItem(d, fin.LinhaConsulta, "", 1, moeda.DeCentimos(50000), fin.RegimeIsento)
		_ = f.Emitir("2026", 1, "", m)
		return f.Hash()
	}
	if comDescricao("Consulta") == comDescricao("Cirurgia") {
		t.Error("alterar a descrição da linha tinha de mudar o hash (total igual não chega)")
	}
}

func TestHash_EncadeiaComAnterior(t *testing.T) {
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	comAnterior := func(ha string) string {
		f := novaFacturaValida(t)
		_ = f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1, moeda.DeCentimos(50000), fin.RegimeIsento)
		_ = f.Emitir("2026", 2, ha, m)
		return f.Hash()
	}
	if comAnterior("") == comAnterior("aaaa") {
		t.Error("o hash anterior tinha de entrar no cálculo")
	}
}

// TestEmitir_DataEmissaoTruncadaSemNanosegundosEUTC fixa que Emitir trunca ao
// segundo e normaliza para UTC o campo persistido — não só o hash. Se algum dia
// a truncatura dentro de Emitir for removida e ficar só a de HashDe, o valor
// gravado em DataEmissao passa a divergir do que o hash sela, e nenhum outro
// teste desta suite apanharia essa deriva (todos comparam hashes entre si).
func TestEmitir_DataEmissaoTruncadaSemNanosegundosEUTC(t *testing.T) {
	f := novaFacturaValida(t)
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1,
		moeda.DeCentimos(50000), fin.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem: %v", err)
	}
	// Fuso não-UTC (WAT, Angola) e nanosegundos não-nulos: o momento tal como
	// chegaria de time.Now() num servidor real, antes de ser persistido.
	wat := time.FixedZone("WAT", 3600)
	m := time.Date(2026, 7, 18, 11, 0, 0, 123456789, wat)
	if err := f.Emitir("2026", 1, "", m); err != nil {
		t.Fatalf("Emitir: %v", err)
	}
	if got := f.DataEmissao().Nanosecond(); got != 0 {
		t.Errorf("DataEmissao().Nanosecond() = %d; esperava 0 (truncado ao segundo)", got)
	}
	if loc := f.DataEmissao().Location(); loc != time.UTC {
		t.Errorf("DataEmissao().Location() = %v; esperava UTC", loc)
	}
}

// TestReconstruirFactura_PreservaFacturaEmitida cobre ReconstruirFactura sobre
// uma factura EMITIDA — os testes existentes só a exercitam sobre RASCUNHO. Os
// 7 campos de emissão (Numero, Serie, Sequencial, DataEmissao, Hash,
// HashAnterior, Estado) são o tipo de literal que se perde numa edição futura
// de Snapshot()/ReconstruirFactura sem que nenhum teste dê pelo erro — e a
// consequência é a cadeia deixar de fechar depois de um restart do sistema.
func TestReconstruirFactura_PreservaFacturaEmitida(t *testing.T) {
	f := novaFacturaValida(t)
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1,
		moeda.DeCentimos(50000), fin.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem: %v", err)
	}
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	if err := f.Emitir("2026", 9, "elo-anterior-xyz", m); err != nil {
		t.Fatalf("Emitir: %v", err)
	}

	recon := fin.ReconstruirFactura(f.Snapshot())

	if recon.Estado() != fin.FactEmitida {
		t.Errorf("Estado() = %q; esperava EMITIDA", recon.Estado())
	}
	if recon.Numero() != f.Numero() {
		t.Errorf("Numero() = %q; esperava %q", recon.Numero(), f.Numero())
	}
	if recon.Serie() != f.Serie() {
		t.Errorf("Serie() = %q; esperava %q", recon.Serie(), f.Serie())
	}
	if recon.Sequencial() != f.Sequencial() {
		t.Errorf("Sequencial() = %d; esperava %d", recon.Sequencial(), f.Sequencial())
	}
	if !recon.DataEmissao().Equal(f.DataEmissao()) {
		t.Errorf("DataEmissao() = %v; esperava %v", recon.DataEmissao(), f.DataEmissao())
	}
	if recon.HashAnterior() != f.HashAnterior() {
		t.Errorf("HashAnterior() = %q; esperava %q", recon.HashAnterior(), f.HashAnterior())
	}
	if recon.Hash() != f.Hash() {
		t.Errorf("Hash() = %q; esperava %q", recon.Hash(), f.Hash())
	}
	// O elo tem de fechar: recalcular o hash sobre o snapshot reconstruído
	// devolve o mesmo valor gravado — não só o campo Hash é copiado às cegas.
	if got := fin.HashDe(recon.Snapshot()); got != f.Hash() {
		t.Errorf("HashDe(recon.Snapshot()) = %q; esperava o mesmo elo %q", got, f.Hash())
	}
}

// Vector dourado: fixa o hash canónico de uma factura conhecida. Se a
// canonicalização mudar (ordem dos campos, separadores, formato da data), este
// teste falha — é a única salvaguarda contra tornar irreproduzível a cadeia das
// facturas já emitidas (retenção AGT/SAF-T-AO, 10 anos).
const hashDourado = "8caeeee0017219380ffbca9560b2d24894b07a45ba1fdb63a6cc4710293cc169"

func TestHash_VectorDourado(t *testing.T) {
	c, err := fin.NovoClienteSnapshot("Sol", "", "")
	if err != nil {
		t.Fatalf("cliente: %v", err)
	}
	f, err := fin.NovaFactura(c, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("factura: %v", err)
	}
	if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1,
		moeda.DeCentimos(50000), fin.RegimeIsento); err != nil {
		t.Fatalf("item1: %v", err)
	}
	if err := f.AdicionarItem("Paracetamol", fin.LinhaDispensa,
		"22222222-2222-2222-2222-222222222222", 2,
		moeda.DeCentimos(1000), fin.RegimeStandard); err != nil {
		t.Fatalf("item2: %v", err)
	}
	m := time.Date(2026, 7, 18, 10, 0, 0, 123456789, time.UTC)
	if err := f.Emitir("2026", 7, "abc", m); err != nil {
		t.Fatalf("Emitir: %v", err)
	}
	if f.Hash() != hashDourado {
		t.Errorf("hash = %q, queria %q", f.Hash(), hashDourado)
	}
}
