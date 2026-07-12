package identidade_test

import (
	"context"
	"errors"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
)

func TestListarSessoes_Delega(t *testing.T) {
	admin := &fakeAdmin{sessoesPorUtilizador: map[string][]appident.SessaoActiva{
		"u1": {{ID: "sess-1", IP: "10.0.0.5"}},
	}}
	caso := appident.NovoCasoListarSessoes(admin)

	out, err := caso.Executar(context.Background(), "u1")
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(out) != 1 || out[0].ID != "sess-1" {
		t.Fatalf("sessões inesperadas: %v", out)
	}
}

func TestRevogarSessao_Audita(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoRevogarSessao(admin, aud)

	if err := caso.Executar(context.Background(), "actor-1", "sess-1"); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if len(admin.sessoesRevogadas1) != 1 || admin.sessoesRevogadas1[0] != "sess-1" {
		t.Fatalf("revogação não delegada: %v", admin.sessoesRevogadas1)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.sessao.revogada" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
	if aud.registos[0].Actor != "actor-1" || aud.registos[0].EntidadeID != "sess-1" {
		t.Fatalf("auditoria com dados errados: %+v", aud.registos[0])
	}
}

func TestRevogarSessao_PropagaErro(t *testing.T) {
	admin := &fakeAdmin{err: errors.New("kc down")}
	caso := appident.NovoCasoRevogarSessao(admin, &fakeAuditor{})
	if err := caso.Executar(context.Background(), "actor-1", "sess-1"); err == nil {
		t.Fatal("esperava erro propagado")
	}
}
