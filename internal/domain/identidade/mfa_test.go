package identidade_test

import (
	"errors"
	"testing"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestExigeAutenticacaoForte(t *testing.T) {
	casos := []struct {
		nome   string
		papeis []dominio.Papel
		quer   bool
	}{
		{"admin exige", []dominio.Papel{dominio.PapelAdmin}, true},
		{"director exige", []dominio.Papel{dominio.PapelMedico, dominio.PapelDirector}, true},
		{"medico nao exige", []dominio.Papel{dominio.PapelMedico}, false},
		{"sem papeis nao exige", nil, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := dominio.ExigeAutenticacaoForte(c.papeis); got != c.quer {
				t.Fatalf("ExigeAutenticacaoForte(%v) = %v; quer %v", c.papeis, got, c.quer)
			}
		})
	}
}

func TestVerificarAutenticacaoForte(t *testing.T) {
	t.Run("papel sensivel sem MFA nega", func(t *testing.T) {
		s := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}, AutenticacaoForte: false}
		err := dominio.VerificarAutenticacaoForte(s)
		if err == nil {
			t.Fatal("esperava erro MFA")
		}
		if erros.CategoriaDe(err) != erros.CategoriaMFAObrigatorio {
			t.Fatalf("categoria = %v; quer MFAObrigatorio", erros.CategoriaDe(err))
		}
	})
	t.Run("papel sensivel com MFA permite", func(t *testing.T) {
		s := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelAdmin}, AutenticacaoForte: true}
		if err := dominio.VerificarAutenticacaoForte(s); err != nil {
			t.Fatalf("esperava nil, obtive %v", err)
		}
	})
	t.Run("papel comum sem MFA permite", func(t *testing.T) {
		s := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelMedico}, AutenticacaoForte: false}
		if err := dominio.VerificarAutenticacaoForte(s); err != nil {
			t.Fatalf("esperava nil, obtive %v", err)
		}
	})
	t.Run("erro nao mascara categoria", func(t *testing.T) {
		s := dominio.Sessao{Papeis: []dominio.Papel{dominio.PapelDPO}}
		var ed *erros.ErroDominio
		if !errors.As(dominio.VerificarAutenticacaoForte(s), &ed) {
			t.Fatal("esperava ErroDominio")
		}
	})
}
