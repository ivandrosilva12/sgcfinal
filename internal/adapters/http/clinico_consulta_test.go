package http_test

import (
	"context"
	nethttp "net/http"
	"testing"

	"github.com/gin-gonic/gin"

	adhttp "github.com/ivandrosilva12/sgcfinal/internal/adapters/http"
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

type fakeIniciarConsulta struct {
	out       appclinico.DetalheEpisodio
	err       error
	actor     string
	chegadaID string
}

func (f *fakeIniciarConsulta) Executar(_ context.Context, actor, chegadaID string) (appclinico.DetalheEpisodio, error) {
	f.actor, f.chegadaID = actor, chegadaID
	return f.out, f.err
}

func routerConsulta(sessao dominio.Sessao, f *fakeIniciarConsulta) *gin.Engine {
	r := novoRouter()
	r.Use(adhttp.RequestID())
	h := adhttp.NovoClinicoConsultaHandler(f)
	adhttp.RegistarClinicoConsulta(r, h, adhttp.Auth(fakeAuth{sessao: sessao}))
	return r
}

func TestIniciarConsultaHTTP_Medico_201(t *testing.T) {
	f := &fakeIniciarConsulta{out: appclinico.DetalheEpisodio{ID: "ep-1", Estado: "ABERTO", Tipo: "CONSULTA"}}
	r := routerConsulta(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, f)
	w := pedidoCorpo(r, "POST", "/api/v1/chegadas/c1/iniciar-consulta", ``)
	if w.Code != nethttp.StatusCreated {
		t.Fatalf("esperava 201, veio %d (%s)", w.Code, w.Body.String())
	}
	if f.actor != "m1" || f.chegadaID != "c1" {
		t.Fatalf("actor/chegada mal passados ao caso de uso: %q %q", f.actor, f.chegadaID)
	}
}

func TestIniciarConsultaHTTP_Enfermeiro_403(t *testing.T) {
	f := &fakeIniciarConsulta{}
	r := routerConsulta(dominio.Sessao{Sujeito: "e1", Papeis: []dominio.Papel{dominio.PapelEnfermeiro}}, f)
	w := pedidoCorpo(r, "POST", "/api/v1/chegadas/c1/iniciar-consulta", ``)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("enfermeiro devia receber 403, veio %d", w.Code)
	}
}

func TestIniciarConsultaHTTP_Administrativo_403(t *testing.T) {
	f := &fakeIniciarConsulta{}
	r := routerConsulta(dominio.Sessao{Sujeito: "a1", Papeis: []dominio.Papel{dominio.PapelAdministrativo}}, f)
	w := pedidoCorpo(r, "POST", "/api/v1/chegadas/c1/iniciar-consulta", ``)
	if w.Code != nethttp.StatusForbidden {
		t.Fatalf("administrativo devia receber 403, veio %d", w.Code)
	}
}

func TestIniciarConsultaHTTP_ErrosPropagados(t *testing.T) {
	casos := []struct {
		nome   string
		err    error
		codigo int
	}{
		{"chegada não encontrada", erros.Novo(erros.CategoriaNaoEncontrado, "chegada triada não encontrada"), nethttp.StatusNotFound},
		{"médico não atribuído", erros.Novo(erros.CategoriaProibido, "só o médico atribuído pode iniciar a consulta"), nethttp.StatusForbidden},
		{"chegada já consumida", erros.Novo(erros.CategoriaConflito, "o estado da chegada mudou entretanto; recarregue e repita a operação"), nethttp.StatusConflict},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			f := &fakeIniciarConsulta{err: caso.err}
			r := routerConsulta(dominio.Sessao{Sujeito: "m1", Papeis: []dominio.Papel{dominio.PapelMedico}}, f)
			w := pedidoCorpo(r, "POST", "/api/v1/chegadas/c1/iniciar-consulta", ``)
			if w.Code != caso.codigo {
				t.Fatalf("esperava %d, veio %d (%s)", caso.codigo, w.Code, w.Body.String())
			}
		})
	}
}
