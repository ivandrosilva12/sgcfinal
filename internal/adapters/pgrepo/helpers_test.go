package pgrepo

// Testes unitários dos helpers puros do pacote pgrepo — sem ligação à base de
// dados. Cobrem a tradução de erros de unicidade do Postgres e o mapeamento
// de campos opcionais do agregado Doente para colunas SQL.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// --- traduzUnicidadeMedicamento -------------------------------------------------

func TestTraduzUnicidadeMedicamento(t *testing.T) {
	casos := []struct {
		nome              string
		entrada           error
		categoriaEsperada erros.Categoria
	}{
		{
			nome:              "violação de unicidade 23505 devolve conflito",
			entrada:           &pgconn.PgError{Code: "23505", Message: "duplicate key"},
			categoriaEsperada: erros.CategoriaConflito,
		},
		{
			nome:              "outro código pgconn não é tratado como conflito",
			entrada:           &pgconn.PgError{Code: "23503", Message: "foreign key violation"},
			categoriaEsperada: erros.CategoriaInterno,
		},
		{
			nome:              "erro genérico não é tratado como conflito",
			entrada:           errors.New("falha de ligação"),
			categoriaEsperada: erros.CategoriaInterno,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			resultado := traduzUnicidadeMedicamento(c.entrada)
			if resultado == nil {
				t.Fatal("esperava um erro, obteve nil")
			}
			if categoria := erros.CategoriaDe(resultado); categoria != c.categoriaEsperada {
				t.Errorf("categoria = %v, esperava %v", categoria, c.categoriaEsperada)
			}
		})
	}
}

func TestTraduzUnicidadeMedicamento_EmbrulhaErroOriginal(t *testing.T) {
	original := errors.New("timeout de rede")
	resultado := traduzUnicidadeMedicamento(original)

	if erros.CategoriaDe(resultado) == erros.CategoriaConflito {
		t.Fatal("erro genérico não deveria ser categorizado como conflito")
	}
	if !errors.Is(resultado, original) {
		t.Error("esperava que o erro original permanecesse acessível via errors.Is (fmt.Errorf com %w)")
	}
}

// --- traduzErroUnicidade (doentes) ----------------------------------------------

func TestTraduzErroUnicidade(t *testing.T) {
	casos := []struct {
		nome              string
		entrada           error
		categoriaEsperada erros.Categoria
	}{
		{
			nome:              "violação de unicidade 23505 devolve conflito",
			entrada:           &pgconn.PgError{Code: "23505", Message: "duplicate key"},
			categoriaEsperada: erros.CategoriaConflito,
		},
		{
			nome:              "erro pgconn de código diferente não é conflito",
			entrada:           &pgconn.PgError{Code: "23514", Message: "check violation"},
			categoriaEsperada: erros.CategoriaInterno,
		},
		{
			nome:              "erro genérico não é conflito",
			entrada:           errors.New("ligação perdida"),
			categoriaEsperada: erros.CategoriaInterno,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			resultado := traduzErroUnicidade(c.entrada)
			if resultado == nil {
				t.Fatal("esperava um erro, obteve nil")
			}
			if categoria := erros.CategoriaDe(resultado); categoria != c.categoriaEsperada {
				t.Errorf("categoria = %v, esperava %v", categoria, c.categoriaEsperada)
			}
		})
	}
}

func TestTraduzErroUnicidade_EmbrulhaErroOriginal(t *testing.T) {
	original := errors.New("contexto cancelado")
	resultado := traduzErroUnicidade(original)

	if !errors.Is(resultado, original) {
		t.Error("esperava que o erro original permanecesse acessível via errors.Is (fmt.Errorf com %w)")
	}
}

// --- deref -----------------------------------------------------------------------

func TestDeref(t *testing.T) {
	valor := "Luanda"

	casos := []struct {
		nome     string
		entrada  *string
		esperado string
	}{
		{nome: "ponteiro nil devolve string vazia", entrada: nil, esperado: ""},
		{nome: "ponteiro válido devolve o valor apontado", entrada: &valor, esperado: "Luanda"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if resultado := deref(c.entrada); resultado != c.esperado {
				t.Errorf("deref(%v) = %q, esperava %q", c.entrada, resultado, c.esperado)
			}
		})
	}
}

// --- grupoTexto --------------------------------------------------------------------

func TestGrupoTexto(t *testing.T) {
	grupoOPositivo := dominio.GrupoOPositivo

	casos := []struct {
		nome     string
		snapshot dominio.SnapshotDoente
		esperado string
	}{
		{
			nome:     "sem grupo sanguíneo devolve string vazia",
			snapshot: dominio.SnapshotDoente{GrupoSanguineo: nil},
			esperado: "",
		},
		{
			nome:     "com grupo sanguíneo devolve o código canónico",
			snapshot: dominio.SnapshotDoente{GrupoSanguineo: &grupoOPositivo},
			esperado: "O+",
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if resultado := grupoTexto(c.snapshot); resultado != c.esperado {
				t.Errorf("grupoTexto(...) = %q, esperava %q", resultado, c.esperado)
			}
		})
	}
}

// --- desmontarMorada -----------------------------------------------------------

func TestDesmontarMorada_SemMorada(t *testing.T) {
	s := dominio.SnapshotDoente{Contactos: dominio.Contactos{Morada: nil}}

	mp, mm, mc, mb, mr, mca, mref := desmontarMorada(s)

	for nome, ptr := range map[string]*string{
		"provincia": mp, "municipio": mm, "comuna": mc, "bairro": mb, "rua": mr,
	} {
		if ptr != nil {
			t.Errorf("%s devia ser nil quando não há morada, obteve %q", nome, *ptr)
		}
	}
	if mca != nil {
		t.Errorf("casa devia ser nil quando não há morada, obteve %q", *mca)
	}
	if mref != nil {
		t.Errorf("referência devia ser nil quando não há morada, obteve %q", *mref)
	}
}

func TestDesmontarMorada_ComMorada(t *testing.T) {
	casa := "12"
	referencia := "perto do mercado"
	morada := &dominio.Morada{
		Provincia:  "Luanda",
		Municipio:  "Belas",
		Comuna:     "Kilamba",
		Bairro:     "Sector 7",
		Rua:        "Rua Principal",
		Casa:       &casa,
		Referencia: &referencia,
	}
	s := dominio.SnapshotDoente{Contactos: dominio.Contactos{Morada: morada}}

	mp, mm, mc, mb, mr, mca, mref := desmontarMorada(s)

	if mp == nil || *mp != "Luanda" {
		t.Errorf("provincia = %v, esperava \"Luanda\"", mp)
	}
	if mm == nil || *mm != "Belas" {
		t.Errorf("municipio = %v, esperava \"Belas\"", mm)
	}
	if mc == nil || *mc != "Kilamba" {
		t.Errorf("comuna = %v, esperava \"Kilamba\"", mc)
	}
	if mb == nil || *mb != "Sector 7" {
		t.Errorf("bairro = %v, esperava \"Sector 7\"", mb)
	}
	if mr == nil || *mr != "Rua Principal" {
		t.Errorf("rua = %v, esperava \"Rua Principal\"", mr)
	}
	if mca == nil || *mca != "12" {
		t.Errorf("casa = %v, esperava \"12\"", mca)
	}
	if mref == nil || *mref != "perto do mercado" {
		t.Errorf("referência = %v, esperava \"perto do mercado\"", mref)
	}
}
