package laboratorio_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestRegistarAnalise(t *testing.T) {
	repo := novoFakeAnalises()
	aud := &fakeAuditor{}
	uc := app.NovoCasoRegistarAnalise(repo, aud)

	out, err := uc.Executar(context.Background(), "admin-1", app.DadosNovaAnalise{
		Codigo: "glic", Nome: "Glicemia", Unidade: "mg/dL",
		Intervalos: []dominio.IntervaloReferencia{
			{Perfil: dominio.PerfilAdulto, Sexo: dominio.SexoAmbos, Minimo: 70, Maximo: 110},
		},
	})
	if err != nil {
		t.Fatalf("registar análise: %v", err)
	}
	if out.Codigo != "GLIC" {
		t.Fatalf("esperava código normalizado GLIC, veio %q", out.Codigo)
	}
	if !aud.tem("laboratorio.analise.registada") {
		t.Fatalf("esperava auditoria do registo: %+v", aud.registos)
	}

	// Duplicado → Conflito.
	_, err = uc.Executar(context.Background(), "admin-1", app.DadosNovaAnalise{
		Codigo: "GLIC", Nome: "Glicemia", Unidade: "mg/dL",
	})
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("código duplicado devia falhar com Conflito, veio %v", err)
	}
}

func TestListarAnalises(t *testing.T) {
	repo := novoFakeAnalises()
	a, _ := dominio.NovaAnalise("HB", "Hemoglobina", "g/dL", nil, nil)
	_ = repo.Guardar(context.Background(), a)

	out, err := app.NovoCasoListarAnalises(repo).Executar(context.Background())
	if err != nil {
		t.Fatalf("listar análises: %v", err)
	}
	if len(out) != 1 || out[0].Codigo != "HB" {
		t.Fatalf("esperava a análise HB, veio %+v", out)
	}
}
