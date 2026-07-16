package clinico_test

import (
	"context"
	"strconv"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// errSimulado é o erro genérico usado nos testes para injectar falhas de
// infra-estrutura (BD, auditoria) nos fakes, sem depender de uma causa real.
// Partilhado por todos os ficheiros de teste do pacote clinico_test.
var errSimulado = erros.Novo(erros.CategoriaInterno, "falha simulada (teste)")

// fakeRepo é um repositório de doentes em memória para os testes de aplicação.
type fakeRepo struct {
	porID      map[string]*clinico.Doente
	seq        int
	proxErr    error
	guardarErr error
	// obterErr, se definido, faz ObterPorID falhar. obterErrNaChamada, se >0,
	// restringe a falha a essa chamada (1-based); 0 falha em todas as chamadas —
	// permite simular quer a leitura inicial quer a releitura final a falhar
	// isoladamente.
	obterErr          error
	obterErrNaChamada int
	obterChamadas     int
	pagina            clinico.PaginaDoentes
	ultimoFilt        clinico.FiltroDoentes
}

func novoFakeRepo() *fakeRepo { return &fakeRepo{porID: map[string]*clinico.Doente{}} }

func (f *fakeRepo) Guardar(_ context.Context, d *clinico.Doente) (string, error) {
	if f.guardarErr != nil {
		return "", f.guardarErr
	}
	snap := d.Snapshot()
	id := snap.ID
	if id == "" {
		f.seq++
		id = "id-" + strconv.Itoa(f.seq)
		snap.ID = id
	}
	f.porID[id] = clinico.ReconstruirDoente(snap)
	return id, nil
}

func (f *fakeRepo) ObterPorID(_ context.Context, id string) (*clinico.Doente, error) {
	f.obterChamadas++
	if f.obterErr != nil && (f.obterErrNaChamada == 0 || f.obterChamadas == f.obterErrNaChamada) {
		return nil, f.obterErr
	}
	d, ok := f.porID[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
	}
	return d, nil
}

func (f *fakeRepo) ObterPorNumProcesso(_ context.Context, num string) (*clinico.Doente, error) {
	for _, d := range f.porID {
		if d.NumProcesso() == num {
			return d, nil
		}
	}
	return nil, erros.Novo(erros.CategoriaNaoEncontrado, "doente não encontrado")
}

func (f *fakeRepo) Pesquisar(_ context.Context, filt clinico.FiltroDoentes) (clinico.PaginaDoentes, error) {
	f.ultimoFilt = filt
	return f.pagina, nil
}

func (f *fakeRepo) ProximoNumeroProcesso(_ context.Context, ano int) (string, error) {
	if f.proxErr != nil {
		return "", f.proxErr
	}
	f.seq++
	return "P-" + strconv.Itoa(ano) + "-" + leftPad(f.seq), nil
}

func leftPad(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 6 {
		s = "0" + s
	}
	return s
}

// fakeAuditor recolhe os registos de auditoria. Se err estiver definido,
// Registar falha sem gravar nada — simula a auditoria indisponível.
type fakeAuditor struct {
	registos []auditoria.Registo
	err      error
}

func (a *fakeAuditor) Registar(_ context.Context, r auditoria.Registo) error {
	if a.err != nil {
		return a.err
	}
	a.registos = append(a.registos, r)
	return nil
}
