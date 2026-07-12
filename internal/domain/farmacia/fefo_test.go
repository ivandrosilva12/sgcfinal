package farmacia_test

import (
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestAlocarFEFO_UmLote(t *testing.T) {
	alocs, err := farmacia.AlocarFEFO([]farmacia.LoteFEFO{{LoteID: "a", Disponivel: 50}}, 20)
	if err != nil || len(alocs) != 1 || alocs[0].Quantidade != 20 {
		t.Fatalf("alocação inesperada: %+v, %v", alocs, err)
	}
}

func TestAlocarFEFO_MultiplosLotes(t *testing.T) {
	// Ordem FEFO: consome 'a' (validade mais próxima) primeiro, depois 'b'.
	alocs, err := farmacia.AlocarFEFO([]farmacia.LoteFEFO{{LoteID: "a", Disponivel: 15}, {LoteID: "b", Disponivel: 30}}, 20)
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if len(alocs) != 2 || alocs[0].LoteID != "a" || alocs[0].Quantidade != 15 || alocs[1].LoteID != "b" || alocs[1].Quantidade != 5 {
		t.Fatalf("alocação FEFO errada: %+v", alocs)
	}
}

func TestAlocarFEFO_Insuficiente(t *testing.T) {
	_, err := farmacia.AlocarFEFO([]farmacia.LoteFEFO{{LoteID: "a", Disponivel: 5}}, 20)
	if erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("esperava RegraNegocio (stock insuficiente), obtive %v", err)
	}
}

func TestAlocarFEFO_QuantidadeInvalida(t *testing.T) {
	if _, err := farmacia.AlocarFEFO(nil, 0); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para quantidade zero")
	}
}

func TestAlocarFEFO_LoteDisponivelZeroSaltado(t *testing.T) {
	alocs, err := farmacia.AlocarFEFO([]farmacia.LoteFEFO{{LoteID: "a", Disponivel: 0}, {LoteID: "b", Disponivel: 20}}, 20)
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if len(alocs) != 1 || alocs[0].LoteID != "b" || alocs[0].Quantidade != 20 {
		t.Fatalf("esperava saltar lote com disponível 0: %+v", alocs)
	}
}
