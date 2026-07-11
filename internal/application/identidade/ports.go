package identidade

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// VerificadorToken valida um token OIDC (JWT RS256: assinatura, issuer, audience,
// expiração) e devolve a Sessao derivada dos claims. É implementado pela camada
// de adaptadores (keycloak). Em caso de token inválido/ausente deve devolver um
// erros.ErroDominio de categoria NaoAutorizado (→ 401).
type VerificadorToken interface {
	Verificar(ctx context.Context, tokenBruto string) (dominio.Sessao, error)
}

// Auditor persiste registos de auditoria de forma append-only. Implementado por
// pgrepo (INSERT em auditoria.auditoria_eventos).
type Auditor interface {
	Registar(ctx context.Context, r auditoria.Registo) error
}
