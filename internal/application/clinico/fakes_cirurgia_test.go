package clinico_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// nowUTC devolve um instante fixo (UTC) para usar nos testes de cirurgia/consentimento.
func nowUTC() time.Time { return time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) }

// novoDoenteValido constrói um *clinico.Doente mínimo e válido para os testes.
func novoDoenteValido(t *testing.T) *clinico.Doente {
	t.Helper()
	ident, err := clinico.NovaIdentificacao("Ana Domingos", time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC),
		clinico.SexoFeminino, ptrS("00123456LA042"), nil, nil)
	if err != nil {
		t.Fatalf("identificação inválida: %v", err)
	}
	ct, err := clinico.NovosContactos("+244923456789", nil, nil)
	if err != nil {
		t.Fatalf("contactos inválidos: %v", err)
	}
	d, err := clinico.NovoDoente("P-2026-000001", ident, ct, "AO")
	if err != nil {
		t.Fatalf("doente inválido: %v", err)
	}
	return d
}

// fakeConsentimentos é um RepositorioConsentimentos em memória.
type fakeConsentimentos struct {
	porID map[string]*clinico.Consentimento
	seq   int
	lista []clinico.ResumoConsentimento
}

func novoFakeConsentimentos() *fakeConsentimentos {
	return &fakeConsentimentos{porID: map[string]*clinico.Consentimento{}}
}

func (f *fakeConsentimentos) Guardar(_ context.Context, c *clinico.Consentimento) (string, error) {
	s := c.Snapshot()
	id := s.ID
	if id == "" {
		f.seq++
		id = "cons-" + strconv.Itoa(f.seq)
		s.ID = id
	}
	f.porID[id] = clinico.ReconstruirConsentimento(s)
	return id, nil
}

func (f *fakeConsentimentos) ObterPorID(_ context.Context, id string) (*clinico.Consentimento, error) {
	c, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "consentimento não encontrado")
	}
	return c, nil
}

func (f *fakeConsentimentos) ListarPorDoente(_ context.Context, _ string, _ clinico.FiltroConsentimentos) ([]clinico.ResumoConsentimento, error) {
	return f.lista, nil
}

// fakeProcedimentos é um RepositorioProcedimentos em memória.
type fakeProcedimentos struct {
	porID map[string]*clinico.ProcedimentoCirurgico
	seq   int
	lista []clinico.ResumoProcedimento
}

func novoFakeProcedimentos() *fakeProcedimentos {
	return &fakeProcedimentos{porID: map[string]*clinico.ProcedimentoCirurgico{}}
}

func (f *fakeProcedimentos) Guardar(_ context.Context, p *clinico.ProcedimentoCirurgico) (string, error) {
	s := p.Snapshot()
	id := s.ID
	if id == "" {
		f.seq++
		id = "proc-" + strconv.Itoa(f.seq)
		s.ID = id
	}
	f.porID[id] = clinico.ReconstruirProcedimento(s)
	return id, nil
}

func (f *fakeProcedimentos) ObterPorID(_ context.Context, id string) (*clinico.ProcedimentoCirurgico, error) {
	p, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "procedimento não encontrado")
	}
	return p, nil
}

func (f *fakeProcedimentos) ListarPorEpisodio(_ context.Context, _ string) ([]clinico.ResumoProcedimento, error) {
	return f.lista, nil
}

// fakeCatalogo é um RepositorioCatalogoProcedimentos em memória.
type fakeCatalogo struct {
	porCodigo map[string]*clinico.CatalogoProcedimento
}

func novoFakeCatalogo() *fakeCatalogo {
	return &fakeCatalogo{porCodigo: map[string]*clinico.CatalogoProcedimento{
		"PRC001": {Codigo: "PRC001", Descricao: "Sutura", Activo: true},
	}}
}

// ObterPorCodigo normaliza a chave de pesquisa (maiúsculas, sem espaços) tal como
// o repositório pgx real — o caso de uso tem de continuar a funcionar quando o
// cliente envia o código em minúsculas.
func (f *fakeCatalogo) ObterPorCodigo(_ context.Context, codigo string) (*clinico.CatalogoProcedimento, error) {
	c, ok := f.porCodigo[strings.ToUpper(strings.TrimSpace(codigo))]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "procedimento do catálogo não encontrado")
	}
	return c, nil
}
