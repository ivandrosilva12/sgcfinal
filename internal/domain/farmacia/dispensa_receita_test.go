package farmacia_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func receitaComItem(t *testing.T, prescrita int) *farmacia.Receita {
	t.Helper()
	it, err := farmacia.NovoItemReceita("med-1", "1 comp 8/8h", nil, prescrita, "")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	emitida := time.Now()
	r, err := farmacia.NovaReceita("ep-1", "doente-1", "medico-1", []farmacia.ItemReceita{it}, "", emitida, emitida.AddDate(0, 0, 30))
	if err != nil {
		t.Fatalf("receita: %v", err)
	}
	return r
}

func TestRegistarDispensa_Parcial(t *testing.T) {
	r := receitaComItem(t, 20)
	if err := r.RegistarDispensa("med-1", 8); err != nil {
		t.Fatalf("dispensar: %v", err)
	}
	if r.Estado() != farmacia.ReceitaParcial {
		t.Fatalf("estado=%q, esperava PARCIAL", r.Estado())
	}
	if r.Snapshot().Itens[0].QuantidadeDispensada != 8 {
		t.Fatalf("quantidade dispensada=%d", r.Snapshot().Itens[0].QuantidadeDispensada)
	}
}

func TestRegistarDispensa_Total(t *testing.T) {
	r := receitaComItem(t, 20)
	if err := r.RegistarDispensa("med-1", 20); err != nil {
		t.Fatalf("dispensar: %v", err)
	}
	if r.Estado() != farmacia.ReceitaDispensada {
		t.Fatalf("estado=%q, esperava DISPENSADA", r.Estado())
	}
}

func TestRegistarDispensa_Excede(t *testing.T) {
	r := receitaComItem(t, 20)
	_ = r.RegistarDispensa("med-1", 15)
	if erros.CategoriaDe(r.RegistarDispensa("med-1", 10)) != erros.CategoriaRegraNegocio {
		t.Fatal("esperava RegraNegocio ao exceder o prescrito cumulativamente")
	}
}

func TestRegistarDispensa_MedicamentoAusente(t *testing.T) {
	r := receitaComItem(t, 20)
	if erros.CategoriaDe(r.RegistarDispensa("med-outro", 1)) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para medicamento fora da receita")
	}
}

func TestRegistarDispensa_ReceitaAnulada(t *testing.T) {
	r := receitaComItem(t, 20)
	_ = r.Anular()
	if erros.CategoriaDe(r.RegistarDispensa("med-1", 1)) != erros.CategoriaConflito {
		t.Fatal("esperava conflito ao dispensar uma receita não emitida/parcial")
	}
}

func TestRegistarDispensa_QuantidadeInvalida(t *testing.T) {
	r := receitaComItem(t, 20)
	if erros.CategoriaDe(r.RegistarDispensa("med-1", 0)) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para quantidade zero")
	}
}

func TestRegistarDispensa_DuasDispensasCompletamTotal(t *testing.T) {
	r := receitaComItem(t, 20)
	if err := r.RegistarDispensa("med-1", 12); err != nil {
		t.Fatalf("primeira dispensa: %v", err)
	}
	if r.Estado() != farmacia.ReceitaParcial {
		t.Fatalf("estado após primeira dispensa=%q, esperava PARCIAL", r.Estado())
	}
	if err := r.RegistarDispensa("med-1", 8); err != nil {
		t.Fatalf("segunda dispensa: %v", err)
	}
	if r.Estado() != farmacia.ReceitaDispensada {
		t.Fatalf("estado após segunda dispensa=%q, esperava DISPENSADA", r.Estado())
	}
}
