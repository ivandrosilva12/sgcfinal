// internal/application/recepcao/janelas_test.go
package recepcao_test

import (
	"context"
	"testing"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestDefinirJanela_CriaEAudita(t *testing.T) {
	janelas := novoFakeJanelas()
	aud := &fakeAuditor{}
	uc := app.NovoCasoDefinirJanela(janelas, aud)

	out, err := uc.Executar(context.Background(), "adm-1", app.DadosDefinirJanela{
		MedicoID: "med-1", EspecialidadeID: "esp-1", Inicio: inst("08:00"), Fim: inst("13:00"),
	})
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if out.ID == "" || out.MedicoID != "med-1" {
		t.Fatalf("detalhe mal preenchido: %+v", out)
	}
	if !aud.tem("recepcao.janela.definida") {
		t.Fatal("esperava auditoria recepcao.janela.definida")
	}
}

func TestDefinirJanela_IntervaloInvalido_Erro(t *testing.T) {
	uc := app.NovoCasoDefinirJanela(novoFakeJanelas(), &fakeAuditor{})
	_, err := uc.Executar(context.Background(), "adm-1", app.DadosDefinirJanela{
		MedicoID: "med-1", EspecialidadeID: "esp-1", Inicio: inst("13:00"), Fim: inst("08:00"),
	})
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestRemoverJanela_SemMarcacoes_RemoveEAudita(t *testing.T) {
	janelas := novoFakeJanelas()
	marc := novoFakeMarcacoes()
	aud := &fakeAuditor{}
	id, _ := janelas.Guardar(context.Background(), janelaAgregada(t, "med-1", "esp-1", "08:00", "13:00"))

	uc := app.NovoCasoRemoverJanela(janelas, marc, aud)
	if err := uc.Executar(context.Background(), "adm-1", id); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if _, err := janelas.ObterPorID(context.Background(), id); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatal("a janela devia ter sido removida")
	}
	if !aud.tem("recepcao.janela.removida") {
		t.Fatal("esperava auditoria recepcao.janela.removida")
	}
}

func TestRemoverJanela_ComMarcacaoActiva_Conflito(t *testing.T) {
	janelas := novoFakeJanelas()
	marc := novoFakeMarcacoes()
	id, _ := janelas.Guardar(context.Background(), janelaAgregada(t, "med-1", "esp-1", "08:00", "13:00"))
	// marcação activa dentro da janela
	_, _ = marc.Guardar(context.Background(), marcacaoAgregada(t, "doe-1", "med-1", "esp-1", "09:00", "09:30"))

	uc := app.NovoCasoRemoverJanela(janelas, marc, &fakeAuditor{})
	if err := uc.Executar(context.Background(), "adm-1", id); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}
