// Package erros define os erros de domínio partilhados e as respectivas
// categorias, usados para mapear falhas de negócio para respostas HTTP
// (RFC 7807) na camada de adaptadores. Camada 1 (Domínio) — sem infra.
package erros

import "errors"

// Categoria classifica um erro de domínio para efeitos de mapeamento HTTP.
type Categoria int

const (
	// CategoriaValidacao — entrada inválida (→ 400/422).
	CategoriaValidacao Categoria = iota
	// CategoriaNaoAutorizado — sem autenticação válida (→ 401).
	CategoriaNaoAutorizado
	// CategoriaProibido — autenticado mas sem permissão (→ 403).
	CategoriaProibido
	// CategoriaNaoEncontrado — recurso inexistente (→ 404).
	CategoriaNaoEncontrado
	// CategoriaConflito — conflito de estado (→ 409).
	CategoriaConflito
	// CategoriaInterno — falha inesperada (→ 500).
	CategoriaInterno
)

// ErroDominio é um erro de negócio com categoria e mensagem PT-PT.
type ErroDominio struct {
	Categoria Categoria
	Mensagem  string
	Causa     error
}

// Error implementa a interface error.
func (e *ErroDominio) Error() string {
	return e.Mensagem
}

// Unwrap permite errors.Is/As sobre a causa subjacente.
func (e *ErroDominio) Unwrap() error {
	return e.Causa
}

// Novo constrói um ErroDominio.
func Novo(cat Categoria, mensagem string) *ErroDominio {
	return &ErroDominio{Categoria: cat, Mensagem: mensagem}
}

// CategoriaDe extrai a categoria de um erro; devolve CategoriaInterno se o erro
// não for um ErroDominio.
func CategoriaDe(err error) Categoria {
	var ed *ErroDominio
	if errors.As(err, &ed) {
		return ed.Categoria
	}
	return CategoriaInterno
}
