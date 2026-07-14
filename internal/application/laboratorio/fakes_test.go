package laboratorio_test

import (
	"context"
	"strconv"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// fakeAnalises é um RepositorioAnalises em memória.
type fakeAnalises struct {
	porCodigo map[string]*laboratorio.Analise
}

func novoFakeAnalises() *fakeAnalises {
	return &fakeAnalises{porCodigo: map[string]*laboratorio.Analise{}}
}

func (f *fakeAnalises) Guardar(_ context.Context, a *laboratorio.Analise) error {
	if _, existe := f.porCodigo[a.Codigo()]; existe {
		return erros.Novo(erros.CategoriaConflito, "já existe uma análise com este código")
	}
	f.porCodigo[a.Codigo()] = a
	return nil
}

func (f *fakeAnalises) ObterPorCodigo(_ context.Context, codigo string) (*laboratorio.Analise, error) {
	a, ok := f.porCodigo[codigo]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "análise não encontrada")
	}
	return a, nil
}

func (f *fakeAnalises) Listar(_ context.Context) ([]laboratorio.ResumoAnalise, error) {
	out := []laboratorio.ResumoAnalise{}
	for _, a := range f.porCodigo {
		s := a.Snapshot()
		out = append(out, laboratorio.ResumoAnalise{
			Codigo: s.Codigo, Nome: s.Nome, Unidade: s.Unidade, Activo: s.Activo,
		})
	}
	return out, nil
}

// fakeRequisicoes é um RepositorioRequisicoes em memória. Emitir guarda a requisição
// e os resultados (o fake não simula transacções — a atomicidade é do pgrepo).
type fakeRequisicoes struct {
	porID      map[string]*laboratorio.RequisicaoLab
	resultados *fakeResultados
	seq        int
}

func novoFakeRequisicoes(res *fakeResultados) *fakeRequisicoes {
	return &fakeRequisicoes{porID: map[string]*laboratorio.RequisicaoLab{}, resultados: res}
}

func (f *fakeRequisicoes) Emitir(_ context.Context, r *laboratorio.RequisicaoLab, resultados []*laboratorio.Resultado) (string, error) {
	f.seq++
	id := "req-" + strconv.Itoa(f.seq)
	s := r.Snapshot()
	s.ID = id
	f.porID[id] = laboratorio.ReconstruirRequisicao(s)
	// O fake regista o episódio da requisição para que ListarPorEpisodio dos
	// resultados possa fazer a junção que, no repositório real, é um JOIN SQL.
	f.resultados.episodioDe[id] = s.EpisodioID
	for _, res := range resultados {
		sr := res.Snapshot()
		sr.RequisicaoID = id
		f.resultados.inserir(sr)
	}
	return id, nil
}

func (f *fakeRequisicoes) ObterPorID(_ context.Context, id string) (*laboratorio.RequisicaoLab, error) {
	r, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "requisição não encontrada")
	}
	return r, nil
}

func (f *fakeRequisicoes) ListarPorEpisodio(_ context.Context, episodioID string) ([]laboratorio.ResumoRequisicao, error) {
	out := []laboratorio.ResumoRequisicao{}
	for _, r := range f.porID {
		s := r.Snapshot()
		if s.EpisodioID != episodioID {
			continue
		}
		out = append(out, laboratorio.ResumoRequisicao{
			ID: s.ID, EpisodioID: s.EpisodioID, DoenteID: s.DoenteID,
			Prioridade: string(s.Prioridade), Estado: string(s.Estado),
			NumAnalises: len(s.Itens), CriadoEm: s.CriadoEm,
		})
	}
	return out, nil
}

// fakeResultados é um RepositorioResultados em memória.
type fakeResultados struct {
	porID      map[string]laboratorio.SnapshotResultado
	episodioDe map[string]string // requisicaoID → episodioID
	seq        int
}

func novoFakeResultados() *fakeResultados {
	return &fakeResultados{
		porID:      map[string]laboratorio.SnapshotResultado{},
		episodioDe: map[string]string{},
	}
}

func (f *fakeResultados) inserir(s laboratorio.SnapshotResultado) string {
	f.seq++
	id := "res-" + strconv.Itoa(f.seq)
	s.ID = id
	f.porID[id] = s
	return id
}

func (f *fakeResultados) ObterPorID(_ context.Context, id string) (*laboratorio.Resultado, error) {
	s, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
	}
	return laboratorio.ReconstruirResultado(s), nil
}

func (f *fakeResultados) Transitar(_ context.Context, r *laboratorio.Resultado) error {
	s := r.Snapshot()
	actual, ok := f.porID[s.ID]
	if !ok {
		return erros.Novo(erros.CategoriaNaoEncontrado, "resultado não encontrado")
	}
	// Compare-and-set, como o repositório real.
	if actual.Estado != s.EstadoAnterior {
		return erros.Novo(erros.CategoriaConflito, "o estado do resultado mudou entretanto")
	}
	f.porID[s.ID] = s
	return nil
}

func (f *fakeResultados) contem(estados []laboratorio.EstadoResultado, e laboratorio.EstadoResultado) bool {
	if len(estados) == 0 {
		return true
	}
	for _, x := range estados {
		if x == e {
			return true
		}
	}
	return false
}

func (f *fakeResultados) ListarFila(_ context.Context, estados []laboratorio.EstadoResultado) ([]laboratorio.ResumoResultado, error) {
	out := []laboratorio.ResumoResultado{}
	for _, s := range f.porID {
		if !f.contem(estados, s.Estado) {
			continue
		}
		out = append(out, laboratorio.ResumoResultado{
			ID: s.ID, RequisicaoID: s.RequisicaoID, CodigoAnalise: s.CodigoAnalise,
			Valor: s.Valor, Unidade: s.Unidade, Estado: string(s.Estado),
			ValorCritico: s.ValorCritico, ColhidaEm: s.ColhidaEm, SubmetidaEm: s.SubmetidaEm,
		})
	}
	return out, nil
}

func (f *fakeResultados) ListarPorEpisodio(_ context.Context, episodioID string, estados []laboratorio.EstadoResultado) ([]laboratorio.ResumoResultado, error) {
	out := []laboratorio.ResumoResultado{}
	for _, s := range f.porID {
		if f.episodioDe[s.RequisicaoID] != episodioID {
			continue
		}
		if !f.contem(estados, s.Estado) {
			continue
		}
		out = append(out, laboratorio.ResumoResultado{
			ID: s.ID, RequisicaoID: s.RequisicaoID, EpisodioID: episodioID,
			CodigoAnalise: s.CodigoAnalise, Valor: s.Valor, Unidade: s.Unidade,
			Estado: string(s.Estado), ValorCritico: s.ValorCritico,
		})
	}
	return out, nil
}

// fakeLeitorClinico é a ACL em memória.
type fakeLeitorClinico struct {
	doenteActivo   bool
	episodioAberto bool
}

func (f *fakeLeitorClinico) DoenteActivo(_ context.Context, _ string) (bool, error) {
	return f.doenteActivo, nil
}

func (f *fakeLeitorClinico) EpisodioAbertoDoDoente(_ context.Context, _, _ string) (bool, error) {
	return f.episodioAberto, nil
}

// fakeAuditor recolhe os registos de auditoria.
type fakeAuditor struct {
	registos []auditoria.Registo
}

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
