package clinico

import (
	"strings"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// GrupoSanguineo é o grupo sanguíneo ABO/Rh do doente (DDM-001: 8 valores).
type GrupoSanguineo string

const (
	GrupoAPositivo  GrupoSanguineo = "A+"
	GrupoANegativo  GrupoSanguineo = "A-"
	GrupoBPositivo  GrupoSanguineo = "B+"
	GrupoBNegativo  GrupoSanguineo = "B-"
	GrupoABPositivo GrupoSanguineo = "AB+"
	GrupoABNegativo GrupoSanguineo = "AB-"
	GrupoOPositivo  GrupoSanguineo = "O+"
	GrupoONegativo  GrupoSanguineo = "O-"
)

var gruposValidos = map[GrupoSanguineo]bool{
	GrupoAPositivo: true, GrupoANegativo: true,
	GrupoBPositivo: true, GrupoBNegativo: true,
	GrupoABPositivo: true, GrupoABNegativo: true,
	GrupoOPositivo: true, GrupoONegativo: true,
}

// ParseGrupoSanguineo valida e normaliza um grupo sanguíneo (aceita minúsculas).
func ParseGrupoSanguineo(codigo string) (GrupoSanguineo, error) {
	g := GrupoSanguineo(strings.ToUpper(strings.TrimSpace(codigo)))
	if !gruposValidos[g] {
		return "", erros.Novo(erros.CategoriaValidacao, "grupo sanguíneo inválido")
	}
	return g, nil
}

// String devolve a representação canónica do grupo sanguíneo.
func (g GrupoSanguineo) String() string { return string(g) }
