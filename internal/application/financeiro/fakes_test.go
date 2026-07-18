package financeiro_test

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
)

// cabecaSerie é a cabeça da numeração e da cadeia de uma série (o equivalente
// em memória da tabela financeiro.series).
type cabecaSerie struct {
	ultimoSequencial int
	ultimoHash       string
}

// fakeFacturas é um RepositorioFacturas em memória.
type fakeFacturas struct {
	mu     sync.Mutex
	porID  map[string]dominio.SnapshotFactura
	series map[string]cabecaSerie
	seq    int
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

// Emitir replica em memória a alocação serializada do pgrepo: sequencial
// seguinte da série e elo da cadeia, com o hash calculado pelo agregado.
func (f *fakeFacturas) Emitir(_ context.Context, facturaID string, momento time.Time) (*dominio.Factura, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.porID[facturaID]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "factura não encontrada")
	}
	serie := dominio.SerieDe(momento)
	if f.series == nil {
		f.series = map[string]cabecaSerie{}
	}
	cabeca := f.series[serie]
	fa := dominio.ReconstruirFactura(s)
	if err := fa.Emitir(serie, cabeca.ultimoSequencial+1, cabeca.ultimoHash, momento); err != nil {
		return nil, err
	}
	nova := fa.Snapshot()
	nova.Versao = s.Versao + 1
	f.porID[facturaID] = nova
	f.series[serie] = cabecaSerie{ultimoSequencial: nova.Sequencial, ultimoHash: nova.Hash}
	return dominio.ReconstruirFactura(nova), nil
}

// ListarSnapshotsPorSerie devolve as facturas emitidas da série, por sequencial.
func (f *fakeFacturas) ListarSnapshotsPorSerie(_ context.Context, serie string) ([]dominio.SnapshotFactura, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []dominio.SnapshotFactura
	for _, s := range f.porID {
		if s.Serie == serie && s.Estado != dominio.FactRascunho {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequencial < out[j].Sequencial })
	return out, nil
}

// adulterarPrimeiraLinha simula uma adulteração directa na BD: reescreve a
// descrição da primeira linha da factura no par série/sequencial dado, sem
// recalcular o hash. Como o agregado não expõe mutação de linhas depois de
// emitido, mexe directamente no snapshot guardado.
func (f *fakeFacturas) adulterarPrimeiraLinha(serie string, sequencial int, descricao string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, s := range f.porID {
		if s.Serie == serie && s.Sequencial == sequencial && len(s.Itens) > 0 {
			s.Itens[0].Descricao = descricao
			f.porID[id] = s
			return
		}
	}
}

// rascunhoComLinha semeia um rascunho com uma linha e devolve o seu id.
func rascunhoComLinha(t *testing.T, f *fakeFacturas) string {
	t.Helper()
	cliente, err := dominio.NovoClienteSnapshot("Cliente", "", "")
	if err != nil {
		t.Fatalf("NovoClienteSnapshot: %v", err)
	}
	fa, err := dominio.NovaFactura(cliente, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("NovaFactura: %v", err)
	}
	if err := fa.AdicionarItem("Consulta", dominio.LinhaConsulta, "", 1,
		moeda.DeCentimos(50000), dominio.RegimeIsento); err != nil {
		t.Fatalf("AdicionarItem: %v", err)
	}
	id, err := f.Guardar(context.Background(), fa)
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	return id
}

// rascunhoSemLinhas semeia um rascunho vazio e devolve o seu id.
func rascunhoSemLinhas(t *testing.T, f *fakeFacturas) string {
	t.Helper()
	cliente, _ := dominio.NovoClienteSnapshot("Cliente", "", "")
	fa, _ := dominio.NovaFactura(cliente, "11111111-1111-1111-1111-111111111111")
	id, err := f.Guardar(context.Background(), fa)
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	return id
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
