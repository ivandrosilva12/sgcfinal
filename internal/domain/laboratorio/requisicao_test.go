package laboratorio_test

import (
	"testing"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func dadosReq() dominio.DadosNovaRequisicao {
	return dominio.DadosNovaRequisicao{
		EpisodioID: "ep-1", DoenteID: "doente-1", MedicoRequisitanteID: "med-1",
		Prioridade: dominio.PrioridadeRotina,
		Itens:      []dominio.ItemRequisicao{{CodigoAnalise: "HB"}, {CodigoAnalise: "GLIC"}},
	}
}

func TestNovaRequisicao_Valida(t *testing.T) {
	r, err := dominio.NovaRequisicao(dadosReq())
	if err != nil {
		t.Fatalf("requisição válida falhou: %v", err)
	}
	if r.Estado() != dominio.RequisicaoEmitida {
		t.Fatalf("esperava EMITIDA, veio %s", r.Estado())
	}
	if len(r.Itens()) != 2 {
		t.Fatalf("esperava 2 itens, veio %d", len(r.Itens()))
	}
}

func TestNovaRequisicao_SemItens(t *testing.T) {
	d := dadosReq()
	d.Itens = nil
	if _, err := dominio.NovaRequisicao(d); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("requisição sem itens devia falhar com Validacao, veio %v", err)
	}
}

func TestNovaRequisicao_ItensDuplicados(t *testing.T) {
	d := dadosReq()
	d.Itens = []dominio.ItemRequisicao{{CodigoAnalise: "HB"}, {CodigoAnalise: "hb"}}
	if _, err := dominio.NovaRequisicao(d); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("análise repetida (mesmo em minúsculas) devia falhar com Validacao, veio %v", err)
	}
}

func TestNovaRequisicao_CamposObrigatorios(t *testing.T) {
	casos := map[string]func(*dominio.DadosNovaRequisicao){
		"episódio em falta": func(d *dominio.DadosNovaRequisicao) { d.EpisodioID = "" },
		"doente em falta":   func(d *dominio.DadosNovaRequisicao) { d.DoenteID = "" },
		"médico em falta":   func(d *dominio.DadosNovaRequisicao) { d.MedicoRequisitanteID = "" },
	}
	for nome, mutar := range casos {
		t.Run(nome, func(t *testing.T) {
			d := dadosReq()
			mutar(&d)
			if _, err := dominio.NovaRequisicao(d); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
				t.Fatalf("esperava Validacao, veio %v", err)
			}
		})
	}
}

func TestParsePrioridade(t *testing.T) {
	if p, err := dominio.ParsePrioridade("urgente"); err != nil || p != dominio.PrioridadeUrgente {
		t.Fatalf("urgente devia ser válida, veio (%s, %v)", p, err)
	}
	if _, err := dominio.ParsePrioridade("IMEDIATA"); err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("IMEDIATA devia falhar com Validacao, veio %v", err)
	}
}
