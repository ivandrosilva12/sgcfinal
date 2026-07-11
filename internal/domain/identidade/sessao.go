package identidade

// Sessao é o principal autenticado, derivado de um token OIDC já validado pela
// camada de adaptadores. Value Object imutável (Camada 1 — sem infra).
type Sessao struct {
	Sujeito string // keycloak_id (claim "sub")
	Nome    string
	Email   string
	Papeis  []Papel
}

// TemPapel indica se a sessão possui o papel indicado.
func (s Sessao) TemPapel(p Papel) bool {
	for _, atribuido := range s.Papeis {
		if atribuido == p {
			return true
		}
	}
	return false
}

// TemAlgumPapel indica se a sessão possui pelo menos um dos papéis indicados.
// Sem papéis permitidos, devolve false (nega por omissão).
func (s Sessao) TemAlgumPapel(permitidos ...Papel) bool {
	for _, p := range permitidos {
		if s.TemPapel(p) {
			return true
		}
	}
	return false
}
