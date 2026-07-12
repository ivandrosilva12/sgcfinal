package farmacia_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func itemValido(t *testing.T) farmacia.ItemReceita {
	t.Helper()
	it, err := farmacia.NovoItemReceita("med-1", "1 comprimido 8/8h", nil, 20, "")
	if err != nil {
		t.Fatalf("NovoItemReceita: %v", err)
	}
	return it
}

func receitaValida(t *testing.T) *farmacia.Receita {
	t.Helper()
	emitida := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	expira := emitida.AddDate(0, 0, 30)
	r, err := farmacia.NovaReceita("ep-1", "doente-1", "medico-1", []farmacia.ItemReceita{itemValido(t)}, "", emitida, expira)
	if err != nil {
		t.Fatalf("NovaReceita: %v", err)
	}
	return r
}

func TestNovaReceita_EstadoInicialEmitida(t *testing.T) {
	r := receitaValida(t)
	if r.Estado() != farmacia.ReceitaEmitida {
		t.Fatalf("estado inicial=%q, esperava EMITIDA", r.Estado())
	}
	if r.DoenteID() != "doente-1" {
		t.Fatalf("doente=%q", r.DoenteID())
	}
}

func TestNovaReceita_ExigePeloMenosUmItem(t *testing.T) {
	emitida := time.Now()
	if _, err := farmacia.NovaReceita("ep-1", "d-1", "m-1", nil, "", emitida, emitida.AddDate(0, 0, 30)); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para receita sem itens")
	}
}

func TestNovoItemReceita_QuantidadeInvalida(t *testing.T) {
	if _, err := farmacia.NovoItemReceita("med-1", "posologia", nil, 0, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para quantidade zero")
	}
	if _, err := farmacia.NovoItemReceita("med-1", "  ", nil, 5, ""); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para posologia vazia")
	}
}

func TestReceita_Anular(t *testing.T) {
	r := receitaValida(t)
	if err := r.Anular(); err != nil {
		t.Fatalf("anular: %v", err)
	}
	if r.Estado() != farmacia.ReceitaAnulada {
		t.Fatalf("estado=%q, esperava ANULADA", r.Estado())
	}
	if erros.CategoriaDe(r.Anular()) != erros.CategoriaConflito {
		t.Fatal("esperava conflito ao anular uma receita já anulada")
	}
}

func TestReceita_EstadoEfectivoExpira(t *testing.T) {
	r := receitaValida(t) // expira em 2026-07-31
	depois := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if r.EstadoEfectivo(depois) != farmacia.ReceitaExpirada {
		t.Fatalf("esperava EXPIRADA após a data de expiração, obtive %q", r.EstadoEfectivo(depois))
	}
	antes := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	if r.EstadoEfectivo(antes) != farmacia.ReceitaEmitida {
		t.Fatalf("esperava EMITIDA antes da expiração, obtive %q", r.EstadoEfectivo(antes))
	}
	// Uma receita anulada não passa a EXPIRADA.
	_ = r.Anular()
	if r.EstadoEfectivo(depois) != farmacia.ReceitaAnulada {
		t.Fatalf("anulada não devia expirar: %q", r.EstadoEfectivo(depois))
	}
}

func TestReconstruirReceita_PreservaEstado(t *testing.T) {
	orig := receitaValida(t)
	_ = orig.Anular()
	snap := orig.Snapshot()
	snap.ID = "rec-1"
	rec := farmacia.ReconstruirReceita(snap)
	if rec.ID() != "rec-1" || rec.Estado() != farmacia.ReceitaAnulada || len(rec.Snapshot().Itens) != 1 {
		t.Fatalf("rehidratação perdeu estado: %+v", rec.Snapshot())
	}
}
