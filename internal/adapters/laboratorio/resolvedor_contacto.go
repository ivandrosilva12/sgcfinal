package laboratorio

import (
	"context"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	identidade "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// ResolvedorContacto implementa applaboratorio.ResolvedorContacto lendo o telefone de
// um utilizador no BC Identidade. É trabalho de ACL: o adaptador pode conhecer
// Identidade — o domínio/aplicação do Laboratório não.
type ResolvedorContacto struct {
	utilizadores identidade.RepositorioUtilizadores
}

// NovoResolvedorContacto constrói o adaptador sobre o repositório de utilizadores.
func NovoResolvedorContacto(u identidade.RepositorioUtilizadores) *ResolvedorContacto {
	return &ResolvedorContacto{utilizadores: u}
}

// ContactoClinico devolve o telefone do utilizador. Um utilizador inexistente ou sem
// telefone devolve ok=false sem erro — para o alerta, "não sei o número" e "não há
// número" são a mesma resposta, e nunca fazem falhar a validação.
func (r *ResolvedorContacto) ContactoClinico(ctx context.Context, userID string) (string, bool, error) {
	u, err := r.utilizadores.ObterPorID(ctx, userID)
	if err != nil {
		if erros.CategoriaDe(err) == erros.CategoriaNaoEncontrado {
			return "", false, nil
		}
		return "", false, err
	}
	if u.Telefone == "" {
		return "", false, nil
	}
	return u.Telefone, true, nil
}

var _ applaboratorio.ResolvedorContacto = (*ResolvedorContacto)(nil)
