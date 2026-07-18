package financeiro_test

import (
	"testing"

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
