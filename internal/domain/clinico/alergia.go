package clinico

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Severidade classifica a gravidade de uma alergia (DDM-001).
type Severidade string

const (
	SeveridadeLeve         Severidade = "LEVE"
	SeveridadeModerada     Severidade = "MODERADA"
	SeveridadeGrave        Severidade = "GRAVE"
	SeveridadeAnafilactica Severidade = "ANAFILACTICA"
)

var severidadesValidas = map[Severidade]bool{
	SeveridadeLeve: true, SeveridadeModerada: true,
	SeveridadeGrave: true, SeveridadeAnafilactica: true,
}

// ParseSeveridade valida e normaliza uma severidade (aceita minúsculas).
func ParseSeveridade(codigo string) (Severidade, error) {
	s := Severidade(strings.ToUpper(strings.TrimSpace(codigo)))
	if !severidadesValidas[s] {
		return "", erros.Novo(erros.CategoriaValidacao, "severidade inválida (esperado LEVE, MODERADA, GRAVE ou ANAFILACTICA)")
	}
	return s, nil
}

// Alergia é uma entidade-filho do agregado Doente: uma alergia conhecida.
type Alergia struct {
	Substancia    string
	Severidade    Severidade
	ReaccaoTipica string
	ConfirmadaEm  *time.Time
	Notas         string
}

// NovaAlergia valida e constrói uma Alergia. Substância obrigatória; severidade
// válida.
func NovaAlergia(substancia string, sev Severidade, reaccao string, confirmadaEm *time.Time, notas string) (Alergia, error) {
	substancia = strings.TrimSpace(substancia)
	if substancia == "" {
		return Alergia{}, erros.Novo(erros.CategoriaValidacao, "substância da alergia em falta")
	}
	if _, err := ParseSeveridade(string(sev)); err != nil {
		return Alergia{}, err
	}
	return Alergia{
		Substancia:    substancia,
		Severidade:    sev,
		ReaccaoTipica: strings.TrimSpace(reaccao),
		ConfirmadaEm:  confirmadaEm,
		Notas:         strings.TrimSpace(notas),
	}, nil
}
