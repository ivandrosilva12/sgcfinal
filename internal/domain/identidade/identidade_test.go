package identidade_test

import (
	"testing"
	"time"

	ident "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestPapelValido(t *testing.T) {
	casos := map[string]bool{
		"Medico":             true,
		"Director":           true,
		"Auditor":            true,
		"FarmaceuticoSenior": true,
		"offline_access":     false,
		"":                   false,
		"medico":             false, // sensível a maiúsculas
	}
	for codigo, esperado := range casos {
		if got := ident.PapelValido(codigo); got != esperado {
			t.Errorf("PapelValido(%q) = %v; esperava %v", codigo, got, esperado)
		}
	}
}

func TestEhSensivel(t *testing.T) {
	sensiveis := []ident.Papel{ident.PapelDirector, ident.PapelAdmin, ident.PapelDPO, ident.PapelAuditor}
	for _, p := range sensiveis {
		if !ident.EhSensivel(p) {
			t.Errorf("EhSensivel(%q) devia ser true", p)
		}
	}
	naoSensiveis := []ident.Papel{ident.PapelMedico, ident.PapelEnfermeiro, ident.PapelTecnicoLab}
	for _, p := range naoSensiveis {
		if ident.EhSensivel(p) {
			t.Errorf("EhSensivel(%q) devia ser false", p)
		}
	}
}

func TestPapeisDe_FiltraDesconhecidos(t *testing.T) {
	entrada := []string{"Medico", "offline_access", "Director", "default-roles-sgc", "Auditor"}
	papeis := ident.PapeisDe(entrada)
	if len(papeis) != 3 {
		t.Fatalf("esperava 3 papéis válidos, obtive %d: %v", len(papeis), papeis)
	}
	esperados := map[ident.Papel]bool{ident.PapelMedico: true, ident.PapelDirector: true, ident.PapelAuditor: true}
	for _, p := range papeis {
		if !esperados[p] {
			t.Errorf("papel inesperado: %q", p)
		}
	}
}

func TestSessao_Papeis(t *testing.T) {
	s := ident.Sessao{Sujeito: "abc", Papeis: []ident.Papel{ident.PapelMedico}}
	if !s.TemPapel(ident.PapelMedico) {
		t.Error("esperava TemPapel(Medico) = true")
	}
	if s.TemPapel(ident.PapelAdmin) {
		t.Error("esperava TemPapel(Admin) = false")
	}
	if !s.TemAlgumPapel(ident.PapelAdmin, ident.PapelMedico) {
		t.Error("esperava TemAlgumPapel(Admin, Medico) = true")
	}
	if s.TemAlgumPapel(ident.PapelAdmin, ident.PapelDPO) {
		t.Error("esperava TemAlgumPapel(Admin, DPO) = false")
	}
	if s.TemAlgumPapel() {
		t.Error("sem papéis permitidos deve devolver false")
	}
}

func TestNovoUtilizador_Valido(t *testing.T) {
	u, err := ident.NovoUtilizador("uuid-1", "Ana Silva", "ana@sgc.ao",
		"+244 923 456 789", "00123456LA042", []ident.Papel{ident.PapelMedico})
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if !u.Activo {
		t.Error("utilizador novo deve estar activo")
	}
	if u.Telefone != "+244 923 456 789" {
		t.Errorf("telefone normalizado inesperado: %q", u.Telefone)
	}
	if u.BI != "00123456LA042" {
		t.Errorf("BI normalizado inesperado: %q", u.BI)
	}
	if !u.TemPapel(ident.PapelMedico) || u.TemAlgumPapel(ident.PapelAdmin) {
		t.Error("papéis do utilizador incorrectos")
	}
}

func TestNovoUtilizador_SemTelefoneNemBI(t *testing.T) {
	u, err := ident.NovoUtilizador("uuid-2", "Sem Contacto", "sc@sgc.ao", "", "", nil)
	if err != nil {
		t.Fatalf("telefone/BI vazios devem ser aceites: %v", err)
	}
	if u.Telefone != "" || u.BI != "" {
		t.Error("telefone/BI deviam ficar vazios")
	}
}

func TestNovoUtilizador_Invalidos(t *testing.T) {
	casos := []struct {
		nome                            string
		kid, nomeU, email, telefone, bi string
	}{
		{"sem keycloak_id", "", "N", "n@sgc.ao", "", ""},
		{"sem nome", "id", "", "n@sgc.ao", "", ""},
		{"email inválido", "id", "N", "não-é-email", "", ""},
		{"telefone inválido", "id", "N", "n@sgc.ao", "123", ""},
		{"BI inválido", "id", "N", "n@sgc.ao", "", "XX"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := ident.NovoUtilizador(c.kid, c.nomeU, c.email, c.telefone, c.bi, nil)
			if err == nil {
				t.Fatalf("esperava erro para %q", c.nome)
			}
			if erros.CategoriaDe(err) != erros.CategoriaValidacao {
				t.Errorf("esperava CategoriaValidacao, obtive %v", erros.CategoriaDe(err))
			}
		})
	}
}

func TestAutorizar(t *testing.T) {
	s := ident.Sessao{Papeis: []ident.Papel{ident.PapelMedico}}

	if err := ident.Autorizar(s); err != nil {
		t.Errorf("sem papéis permitidos deve autorizar: %v", err)
	}
	if err := ident.Autorizar(s, ident.PapelMedico, ident.PapelDirector); err != nil {
		t.Errorf("com papel correspondente deve autorizar: %v", err)
	}
	err := ident.Autorizar(s, ident.PapelAdmin, ident.PapelDPO)
	if err == nil {
		t.Fatal("sem papel correspondente deve recusar")
	}
	if erros.CategoriaDe(err) != erros.CategoriaProibido {
		t.Errorf("esperava CategoriaProibido, obtive %v", erros.CategoriaDe(err))
	}
}

func TestEventos(t *testing.T) {
	agora := time.Now()
	casos := []struct {
		evento interface {
			NomeEvento() string
			OcorridoEm() time.Time
		}
		nome string
	}{
		{ident.UtilizadorAutenticado{Sujeito: "a", Em: agora}, "identidade.utilizador.autenticado"},
		{ident.PerfilConsultado{Sujeito: "a", Em: agora}, "identidade.perfil.consultado"},
		{ident.AcessoNegado{Sujeito: "a", Recurso: "/x", Em: agora}, "identidade.acesso.negado"},
	}
	for _, c := range casos {
		if c.evento.NomeEvento() != c.nome {
			t.Errorf("NomeEvento = %q; esperava %q", c.evento.NomeEvento(), c.nome)
		}
		if !c.evento.OcorridoEm().Equal(agora) {
			t.Errorf("OcorridoEm incorrecto para %q", c.nome)
		}
	}
}
