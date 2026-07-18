package financeiro_test

import (
	"context"
	"strconv"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeFacturas é um RepositorioFacturas em memória.
type fakeFacturas struct {
	porID map[string]dominio.SnapshotFactura
	seq   int
}

func novoFakeFacturas() *fakeFacturas {
	return &fakeFacturas{porID: map[string]dominio.SnapshotFactura{}}
}

func (f *fakeFacturas) Guardar(_ context.Context, fa *dominio.Factura) (string, error) {
	s := fa.Snapshot()
	if s.ID == "" {
		f.seq++
		s.ID = "fac-" + strconv.Itoa(f.seq)
	}
	// Atribui ids às linhas sem id (como o pgrepo).
	for i := range s.Itens {
		if s.Itens[i].ID == "" {
			f.seq++
			s.Itens[i].ID = "item-" + strconv.Itoa(f.seq)
		}
	}
	f.porID[s.ID] = s
	return s.ID, nil
}

func (f *fakeFacturas) ObterPorID(_ context.Context, id string) (*dominio.Factura, error) {
	s, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "factura não encontrada")
	}
	return dominio.ReconstruirFactura(s), nil
}

func (f *fakeFacturas) ListarPorEpisodio(_ context.Context, episodioID string) ([]dominio.ResumoFactura, error) {
	out := []dominio.ResumoFactura{}
	for _, s := range f.porID {
		if s.EpisodioID != episodioID {
			continue
		}
		fa := dominio.ReconstruirFactura(s)
		out = append(out, dominio.ResumoFactura{
			ID: s.ID, Estado: string(s.Estado), ClienteNome: s.Cliente.Nome,
			EpisodioID: s.EpisodioID, NumItens: len(s.Itens),
			TotalCentimos: fa.Totais().Total.Centimos(), CriadoEm: s.CriadoEm,
		})
	}
	return out, nil
}

// fakeAuditor recolhe os registos de auditoria.
type fakeAuditor struct{ registos []auditoria.Registo }

func (f *fakeAuditor) Registar(_ context.Context, r auditoria.Registo) error {
	f.registos = append(f.registos, r)
	return nil
}

func (f *fakeAuditor) tem(accao string) bool {
	for _, r := range f.registos {
		if r.Accao == accao {
			return true
		}
	}
	return false
}
