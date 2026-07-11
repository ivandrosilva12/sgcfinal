package identidade

import (
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// Autorizar aplica a regra RBAC: a sessão tem de possuir pelo menos um dos
// papéis permitidos. Sem papéis permitidos (endpoint sem restrição de papel),
// autoriza. Caso contrário, devolve um ErroDominio de categoria Proibido (→ 403)
// com mensagem pt-AO.
func Autorizar(s Sessao, permitidos ...Papel) error {
	if len(permitidos) == 0 {
		return nil
	}
	if s.TemAlgumPapel(permitidos...) {
		return nil
	}
	return erros.Novo(erros.CategoriaProibido, i18n.T(i18n.MsgSemPermissao))
}
