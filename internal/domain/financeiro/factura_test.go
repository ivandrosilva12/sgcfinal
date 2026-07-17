package financeiro_test

import (
	"testing"

	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
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
