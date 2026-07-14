package laboratorio_test

import (
	"testing"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func intervaloValido() dominio.IntervaloReferencia {
	return dominio.IntervaloReferencia{
		Perfil: dominio.PerfilAdulto, Sexo: dominio.SexoAmbos, Minimo: 70, Maximo: 110,
	}
}

func TestNovaAnalise_CamposObrigatorios(t *testing.T) {
	casos := []struct{ nome, codigo, nomeAnalise, unidade string }{
		{"código em falta", "", "Glicemia", "mg/dL"},
		{"nome em falta", "GLIC", "", "mg/dL"},
		{"unidade em falta", "GLIC", "Glicemia", ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := dominio.NovaAnalise(c.codigo, c.nomeAnalise, c.unidade, nil, nil)
			if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
				t.Fatalf("esperava Validacao, veio %v", err)
			}
		})
	}
}

func TestNovaAnalise_NormalizaCodigo(t *testing.T) {
	a, err := dominio.NovaAnalise(" glic ", "Glicemia", "mg/dL", []dominio.IntervaloReferencia{intervaloValido()}, nil)
	if err != nil {
		t.Fatalf("análise válida falhou: %v", err)
	}
	if a.Codigo() != "GLIC" {
		t.Fatalf("esperava código normalizado GLIC, veio %q", a.Codigo())
	}
	if !a.Activo() {
		t.Fatalf("uma análise nova devia nascer activa")
	}
}

func TestNovaAnalise_IntervaloInvertido(t *testing.T) {
	mau := dominio.IntervaloReferencia{Perfil: dominio.PerfilAdulto, Sexo: dominio.SexoAmbos, Minimo: 110, Maximo: 70}
	_, err := dominio.NovaAnalise("GLIC", "Glicemia", "mg/dL", []dominio.IntervaloReferencia{mau}, nil)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("mínimo > máximo devia falhar com Validacao, veio %v", err)
	}
}

func TestNovaAnalise_ValorCriticoOperadorInvalido(t *testing.T) {
	mau := dominio.ValorCritico{Operador: "==", Limite: 7, Descricao: "x"}
	_, err := dominio.NovaAnalise("HB", "Hemoglobina", "g/dL", nil, []dominio.ValorCritico{mau})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("operador inválido devia falhar com Validacao, veio %v", err)
	}
}

func TestNovaAnalise_PerfilInvalido(t *testing.T) {
	mau := dominio.IntervaloReferencia{Perfil: "INFANTIL", Sexo: dominio.SexoAmbos, Minimo: 1, Maximo: 2}
	_, err := dominio.NovaAnalise("GLIC", "Glicemia", "mg/dL", []dominio.IntervaloReferencia{mau}, nil)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("perfil inválido devia falhar com Validacao, veio %v", err)
	}
}

func TestNovaAnalise_SexoInvalido(t *testing.T) {
	mau := dominio.IntervaloReferencia{Perfil: dominio.PerfilAdulto, Sexo: "X", Minimo: 1, Maximo: 2}
	_, err := dominio.NovaAnalise("GLIC", "Glicemia", "mg/dL", []dominio.IntervaloReferencia{mau}, nil)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("sexo inválido devia falhar com Validacao, veio %v", err)
	}
}

func TestNovaAnalise_ValorCriticoDescricaoEmFalta(t *testing.T) {
	mau := dominio.ValorCritico{Operador: dominio.CriticoMenor, Limite: 7, Descricao: "  "}
	_, err := dominio.NovaAnalise("HB", "Hemoglobina", "g/dL", nil, []dominio.ValorCritico{mau})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("descrição em falta devia falhar com Validacao, veio %v", err)
	}
}

func TestAnalise_Getters(t *testing.T) {
	a, err := dominio.NovaAnalise("GLIC", "Glicemia", "mg/dL", nil, nil)
	if err != nil {
		t.Fatalf("análise válida falhou: %v", err)
	}
	if a.Nome() != "Glicemia" {
		t.Fatalf("esperava nome Glicemia, veio %q", a.Nome())
	}
	if a.Unidade() != "mg/dL" {
		t.Fatalf("esperava unidade mg/dL, veio %q", a.Unidade())
	}
}

func TestAnalise_SnapshotEReconstruir(t *testing.T) {
	a, _ := dominio.NovaAnalise("HB", "Hemoglobina", "g/dL",
		[]dominio.IntervaloReferencia{intervaloValido()},
		[]dominio.ValorCritico{{Operador: dominio.CriticoMenor, Limite: 7, Descricao: "anemia grave"}})
	s := a.Snapshot()
	s.Activo = false
	b := dominio.ReconstruirAnalise(s)
	if b.Codigo() != "HB" || b.Activo() {
		t.Fatalf("reconstrução não preservou o snapshot: %+v", b.Snapshot())
	}
	if len(b.Snapshot().ValoresCriticos) != 1 {
		t.Fatalf("esperava 1 valor crítico preservado")
	}
}
