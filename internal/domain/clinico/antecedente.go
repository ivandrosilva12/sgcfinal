package clinico

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// TipoAntecedente classifica um antecedente clínico (DDM-001).
type TipoAntecedente string

const (
	AntecedentePessoal    TipoAntecedente = "PESSOAL"
	AntecedenteFamiliar   TipoAntecedente = "FAMILIAR"
	AntecedenteCirurgico  TipoAntecedente = "CIRURGICO"
	AntecedenteObstetrico TipoAntecedente = "OBSTETRICO"
)

var tiposAntecedenteValidos = map[TipoAntecedente]bool{
	AntecedentePessoal: true, AntecedenteFamiliar: true,
	AntecedenteCirurgico: true, AntecedenteObstetrico: true,
}

// ParseTipoAntecedente valida e normaliza um tipo de antecedente.
func ParseTipoAntecedente(codigo string) (TipoAntecedente, error) {
	t := TipoAntecedente(strings.ToUpper(strings.TrimSpace(codigo)))
	if !tiposAntecedenteValidos[t] {
		return "", erros.Novo(erros.CategoriaValidacao, "tipo de antecedente inválido (esperado PESSOAL, FAMILIAR, CIRURGICO ou OBSTETRICO)")
	}
	return t, nil
}

// AntecedenteClinico é uma entidade-filho do agregado Doente: um antecedente
// clínico (pessoal, familiar, cirúrgico ou obstétrico).
type AntecedenteClinico struct {
	Tipo       TipoAntecedente
	Descricao  string
	CID        string
	DataInicio *time.Time
	Activo     bool
	Notas      string
}

// NovoAntecedente valida e constrói um AntecedenteClinico. Descrição obrigatória;
// tipo válido.
func NovoAntecedente(tipo TipoAntecedente, descricao, cid string, dataInicio *time.Time, activo bool, notas string) (AntecedenteClinico, error) {
	if _, err := ParseTipoAntecedente(string(tipo)); err != nil {
		return AntecedenteClinico{}, err
	}
	descricao = strings.TrimSpace(descricao)
	if descricao == "" {
		return AntecedenteClinico{}, erros.Novo(erros.CategoriaValidacao, "descrição do antecedente em falta")
	}
	return AntecedenteClinico{
		Tipo:       tipo,
		Descricao:  descricao,
		CID:        strings.TrimSpace(cid),
		DataInicio: dataInicio,
		Activo:     activo,
		Notas:      strings.TrimSpace(notas),
	}, nil
}
