package clinico_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func ptr(s string) *string { return &s }

func TestNovaIdentificacao_ValidaComBI(t *testing.T) {
	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	id, err := clinico.NovaIdentificacao("Ana Domingos", nasc, clinico.SexoFeminino, ptr("00123456la042"), nil, nil)
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if id.BI == nil || *id.BI != "00123456LA042" {
		t.Fatalf("BI não normalizado: %v", id.BI)
	}
}

func TestNovaIdentificacao_PassaporteSemBI(t *testing.T) {
	nasc := time.Date(1985, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := clinico.NovaIdentificacao("João Paulo", nasc, clinico.SexoMasculino, nil, nil, ptr("N1234567")); err != nil {
		t.Fatalf("passaporte devia bastar: %v", err)
	}
}

func TestNovaIdentificacao_SemBINemPassaporte(t *testing.T) {
	nasc := time.Date(1985, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := clinico.NovaIdentificacao("João", nasc, clinico.SexoMasculino, nil, nil, nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestNovaIdentificacao_DataFutura(t *testing.T) {
	futuro := time.Now().AddDate(1, 0, 0)
	_, err := clinico.NovaIdentificacao("Ana", futuro, clinico.SexoFeminino, ptr("00123456LA042"), nil, nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação, obtive %v", err)
	}
}

func TestNovaIdentificacao_NIFInvalido(t *testing.T) {
	nasc := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := clinico.NovaIdentificacao("Ana", nasc, clinico.SexoFeminino, ptr("00123456LA042"), ptr("XYZ"), nil)
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação para NIF inválido, obtive %v", err)
	}
}

func TestParseSexo(t *testing.T) {
	if s, err := clinico.ParseSexo("m"); err != nil || s != clinico.SexoMasculino {
		t.Fatalf("ParseSexo(m)=%v,%v", s, err)
	}
	if _, err := clinico.ParseSexo("X"); err == nil {
		t.Fatal("esperava erro para sexo inválido")
	}
}
