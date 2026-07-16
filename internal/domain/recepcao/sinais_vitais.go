package recepcao

import (
	"fmt"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// SinaisVitais é um value object com os sinais vitais medidos na triagem. Todos os
// campos são opcionais (ponteiro nil = não medido). Os intervalos validados são limites
// de sanidade (rejeitam erros de digitação), não intervalos de normalidade clínica.
type SinaisVitais struct {
	TensaoSistolica        *int     `json:"tensao_sistolica,omitempty"`
	TensaoDiastolica       *int     `json:"tensao_diastolica,omitempty"`
	FrequenciaCardiaca     *int     `json:"frequencia_cardiaca,omitempty"`
	Temperatura            *float64 `json:"temperatura,omitempty"`
	FrequenciaRespiratoria *int     `json:"frequencia_respiratoria,omitempty"`
	SaturacaoO2            *int     `json:"saturacao_o2,omitempty"`
	Dor                    *int     `json:"dor,omitempty"`
	Glicemia               *int     `json:"glicemia,omitempty"`
	Peso                   *float64 `json:"peso,omitempty"`
}

// NovosSinaisVitais valida os campos presentes e devolve o VO. Um valor fora do
// intervalo plausível devolve CategoriaValidacao; um conjunto vazio é válido.
func NovosSinaisVitais(c SinaisVitais) (SinaisVitais, error) {
	if err := intNoIntervalo("tensão sistólica", c.TensaoSistolica, 50, 300); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("tensão diastólica", c.TensaoDiastolica, 30, 200); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("frequência cardíaca", c.FrequenciaCardiaca, 20, 300); err != nil {
		return SinaisVitais{}, err
	}
	if err := floatNoIntervalo("temperatura", c.Temperatura, 30, 45); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("frequência respiratória", c.FrequenciaRespiratoria, 5, 80); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("saturação de O2", c.SaturacaoO2, 50, 100); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("escala de dor", c.Dor, 0, 10); err != nil {
		return SinaisVitais{}, err
	}
	if err := intNoIntervalo("glicemia", c.Glicemia, 20, 600); err != nil {
		return SinaisVitais{}, err
	}
	if err := floatNoIntervalo("peso", c.Peso, 0.5, 400); err != nil {
		return SinaisVitais{}, err
	}
	return c, nil
}

func intNoIntervalo(nome string, v *int, min, max int) error {
	if v == nil {
		return nil
	}
	if *v < min || *v > max {
		return erros.Novo(erros.CategoriaValidacao,
			fmt.Sprintf("%s fora do intervalo plausível (%d–%d)", nome, min, max))
	}
	return nil
}

func floatNoIntervalo(nome string, v *float64, min, max float64) error {
	if v == nil {
		return nil
	}
	if *v < min || *v > max {
		return erros.Novo(erros.CategoriaValidacao,
			fmt.Sprintf("%s fora do intervalo plausível (%g–%g)", nome, min, max))
	}
	return nil
}
