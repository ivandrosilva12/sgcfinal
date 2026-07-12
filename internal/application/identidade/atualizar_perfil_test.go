package identidade_test

import (
	"context"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func ptr(s string) *string { return &s }

func TestAtualizarPerfil_DefineContacto(t *testing.T) {
	repo := &fakeRepo{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoAtualizarPerfil(repo, aud)

	perfil, err := caso.Executar(context.Background(), novaSessao(), ptr("+244 923 456 789"), ptr("00123456LA042"))
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if perfil.Telefone != "+244 923 456 789" || perfil.BI != "00123456LA042" {
		t.Fatalf("perfil não reflecte o contacto: %+v", perfil)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.perfil.actualizado" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
}

func TestAtualizarPerfil_TelefoneInvalido(t *testing.T) {
	caso := appident.NovoCasoAtualizarPerfil(&fakeRepo{}, &fakeAuditor{})
	_, err := caso.Executar(context.Background(), novaSessao(), ptr("123"), nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestAtualizarPerfil_OmitidoPreserva(t *testing.T) {
	repo := &fakeRepo{}
	caso := appident.NovoCasoAtualizarPerfil(repo, &fakeAuditor{})
	// Primeiro define um telefone.
	if _, err := caso.Executar(context.Background(), novaSessao(), ptr("+244 923 456 789"), nil); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Agora actualiza só o BI; telefone omitido (nil) deve preservar-se.
	perfil, err := caso.Executar(context.Background(), novaSessao(), nil, ptr("00123456LA042"))
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if perfil.Telefone != "+244 923 456 789" {
		t.Fatalf("telefone omitido devia preservar-se, obtive %q", perfil.Telefone)
	}
}
