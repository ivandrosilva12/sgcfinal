package recepcao_test

import (
	"testing"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func iptr(v int) *int         { return &v }
func fptr(v float64) *float64 { return &v }

func TestNovosSinaisVitais_VazioEValido(t *testing.T) {
	sv, err := recepcao.NovosSinaisVitais(recepcao.SinaisVitais{})
	if err != nil {
		t.Fatalf("um conjunto vazio devia ser válido: %v", err)
	}
	if sv.TensaoSistolica != nil || sv.Peso != nil {
		t.Fatal("campos não medidos deviam ficar nil")
	}
}

func TestNovosSinaisVitais_ValoresPlausiveis(t *testing.T) {
	_, err := recepcao.NovosSinaisVitais(recepcao.SinaisVitais{
		TensaoSistolica: iptr(120), TensaoDiastolica: iptr(80), FrequenciaCardiaca: iptr(72),
		Temperatura: fptr(36.6), FrequenciaRespiratoria: iptr(16), SaturacaoO2: iptr(98),
		Dor: iptr(3), Glicemia: iptr(95), Peso: fptr(70.5),
	})
	if err != nil {
		t.Fatalf("valores plausíveis não deviam falhar: %v", err)
	}
}

func TestNovosSinaisVitais_ForaDeIntervalo(t *testing.T) {
	casos := []recepcao.SinaisVitais{
		{TensaoSistolica: iptr(400)},      // > 300
		{TensaoDiastolica: iptr(10)},      // < 30
		{FrequenciaCardiaca: iptr(500)},   // > 300
		{Temperatura: fptr(50)},           // > 45
		{FrequenciaRespiratoria: iptr(2)}, // < 5
		{SaturacaoO2: iptr(40)},           // < 50
		{Dor: iptr(11)},                   // > 10
		{Glicemia: iptr(5)},               // < 20
		{Peso: fptr(0.1)},                 // < 0.5
	}
	for i, c := range casos {
		if _, err := recepcao.NovosSinaisVitais(c); erros.CategoriaDe(err) != erros.CategoriaValidacao {
			t.Fatalf("caso %d: esperava CategoriaValidacao, veio %v", i, erros.CategoriaDe(err))
		}
	}
}
