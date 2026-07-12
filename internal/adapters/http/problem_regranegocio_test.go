package http

import (
	nethttp "net/http"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestEstadoDe_RegraNegocio_422(t *testing.T) {
	if got := estadoDe(erros.CategoriaRegraNegocio); got != nethttp.StatusUnprocessableEntity {
		t.Fatalf("estadoDe(RegraNegocio)=%d, esperava 422", got)
	}
}
