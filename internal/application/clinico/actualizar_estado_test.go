package clinico_test

import (
	"context"
	"testing"
	"time"

	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestActualizarDoente_Contactos(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoActualizarDoente(repo, aud)

	novoTel := "+244912000000"
	out, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{
		Contactos: &appclinico.DadosContactos{Telefone: novoTel},
	})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.Telefone != "+244 912 000 000" {
		t.Fatalf("telefone não actualizado: %q", out.Telefone)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.actualizado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestActualizarDoente_GrupoSanguineo(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoActualizarDoente(repo, &fakeAuditor{})
	g := "O+"
	out, err := caso.Executar(context.Background(), "actor-1", id, appclinico.DadosActualizarDoente{GrupoSanguineo: &g})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.GrupoSanguineo == nil || *out.GrupoSanguineo != "O+" {
		t.Fatalf("grupo sanguíneo não definido: %v", out.GrupoSanguineo)
	}
}

func TestGerirEstado_Desactivar(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, aud)

	out, err := caso.Desactivar(context.Background(), "actor-1", id, "dados duplicados")
	if err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	if out.Estado != "INACTIVO" {
		t.Fatalf("estado=%q, esperava INACTIVO", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.desactivado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestGerirEstado_DesactivarSemMotivo(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, &fakeAuditor{})
	_, err := caso.Desactivar(context.Background(), "actor-1", id, "  ")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestGerirEstado_DeclararFalecido(t *testing.T) {
	repo := novoFakeRepo()
	id := registarNoRepo(t, repo)
	aud := &fakeAuditor{}
	caso := appclinico.NovoCasoGerirEstadoDoente(repo, aud)

	data := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	out, err := caso.DeclararFalecido(context.Background(), "actor-1", id, data, "I21")
	if err != nil {
		t.Fatalf("declarar falecido: %v", err)
	}
	if out.Estado != "FALECIDO" {
		t.Fatalf("estado=%q, esperava FALECIDO", out.Estado)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "clinico.doente.falecido" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}
