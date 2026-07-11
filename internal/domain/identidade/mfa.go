package identidade

import (
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// ExigeAutenticacaoForte indica se algum dos papéis exige segundo factor (MFA).
// Alinhado com EhSensivel (Director, Admin, DPO, Auditor).
func ExigeAutenticacaoForte(papeis []Papel) bool {
	for _, p := range papeis {
		if EhSensivel(p) {
			return true
		}
	}
	return false
}

// VerificarAutenticacaoForte devolve um ErroDominio de categoria
// MFAObrigatorio se a sessão tiver um papel sensível mas o token não comprovar
// segundo factor. Caso contrário devolve nil. Função pura (Camada 1).
func VerificarAutenticacaoForte(s Sessao) error {
	if ExigeAutenticacaoForte(s.Papeis) && !s.AutenticacaoForte {
		return erros.Novo(erros.CategoriaMFAObrigatorio, i18n.T(i18n.MsgMFAObrigatoria))
	}
	return nil
}
