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
