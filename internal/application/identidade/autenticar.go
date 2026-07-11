package identidade

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
)

// CasoAutenticar valida um token OIDC e devolve a sessão autenticada. Corre por
// pedido (no middleware de auth), pelo que é deliberadamente leve: não escreve
// na base de dados nem audita — o provisionamento JIT e a auditoria de acesso
// ocorrem nos casos de uso de negócio (ex.: CasoObterPerfil).
type CasoAutenticar struct {
	verificador VerificadorToken
}

// NovoCasoAutenticar constrói o caso de uso com a porta de verificação de token.
func NovoCasoAutenticar(v VerificadorToken) *CasoAutenticar {
	return &CasoAutenticar{verificador: v}
}

// Executar valida o token e devolve a Sessao. Propaga o erro categorizado do
// verificador (NaoAutorizado → 401) sem o mascarar.
func (c *CasoAutenticar) Executar(ctx context.Context, tokenBruto string) (dominio.Sessao, error) {
	return c.verificador.Verificar(ctx, tokenBruto)
}
