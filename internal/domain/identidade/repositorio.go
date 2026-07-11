package identidade

import "context"

// RepositorioUtilizadores é a porta de saída (interface de domínio) para
// persistência do agregado Utilizador. A implementação vive na camada de
// adaptadores (pgrepo), respeitando a regra de dependência.
type RepositorioUtilizadores interface {
	// ObterPorID devolve o utilizador com o keycloak_id indicado, com os seus
	// papéis. Devolve um ErroDominio de categoria NaoEncontrado se não existir.
	ObterPorID(ctx context.Context, keycloakID string) (*Utilizador, error)

	// GuardarComPapeis persiste o utilizador (upsert por keycloak_id) e
	// sincroniza a sua lista de papéis, de forma atómica. Suporta o JIT
	// provisioning no primeiro login.
	GuardarComPapeis(ctx context.Context, u *Utilizador) error

	// AtualizarContacto persiste os campos de perfil local (telefone/BI) do
	// utilizador com o keycloak_id indicado. Devolve NaoEncontrado se a linha
	// não existir. Strings vazias limpam o campo.
	AtualizarContacto(ctx context.Context, keycloakID, telefone, bi string) error
}
