// internal/application/recepcao/fakes_test.go
package recepcao_test

import (
	"context"
	"testing"
	"time"

	app "github.com/ivandrosilva12/sgcfinal/internal/application/recepcao"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func inst(hhmm string) time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-07-20T"+hhmm+":00Z")
	return t
}

// agoraFixo devolve um relógio de teste fixo.
func agoraFixo(hhmm string) func() time.Time {
	return func() time.Time { return inst(hhmm) }
}

// fakeJanelas guarda janelas em memória, indexadas por id.
type fakeJanelas struct {
	dados    map[string]*dominio.JanelaDisponibilidade
	seq      int
	removido []string
}

func novoFakeJanelas() *fakeJanelas {
	return &fakeJanelas{dados: map[string]*dominio.JanelaDisponibilidade{}}
}

func (f *fakeJanelas) Guardar(_ context.Context, j *dominio.JanelaDisponibilidade) (string, error) {
	f.seq++
	id := "jan-" + itoa(f.seq)
	s := j.Snapshot()
	s.ID = id
	f.dados[id] = dominio.ReconstruirJanela(s)
	return id, nil
}

func (f *fakeJanelas) ObterPorID(_ context.Context, id string) (*dominio.JanelaDisponibilidade, error) {
	j, ok := f.dados[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "janela não encontrada")
	}
	return j, nil
}

func (f *fakeJanelas) ListarPorMedicoIntervalo(_ context.Context, medicoID string, de, ate time.Time) ([]dominio.JanelaDisponibilidade, error) {
	var out []dominio.JanelaDisponibilidade
	for _, j := range f.dados {
		if j.MedicoID() == medicoID && j.Inicio().Before(ate) && de.Before(j.Fim()) {
			out = append(out, *j)
		}
	}
	return out, nil
}

func (f *fakeJanelas) Remover(_ context.Context, id string) error {
	if _, ok := f.dados[id]; !ok {
		return erros.Novo(erros.CategoriaNaoEncontrado, "janela não encontrada")
	}
	delete(f.dados, id)
	f.removido = append(f.removido, id)
	return nil
}

// fakeMarcacoes guarda marcações em memória.
type fakeMarcacoes struct {
	dados map[string]*dominio.Marcacao
	seq   int
}

func novoFakeMarcacoes() *fakeMarcacoes {
	return &fakeMarcacoes{dados: map[string]*dominio.Marcacao{}}
}

func (f *fakeMarcacoes) Guardar(_ context.Context, m *dominio.Marcacao) (string, error) {
	f.seq++
	id := "marc-" + itoa(f.seq)
	s := m.Snapshot()
	s.ID = id
	f.dados[id] = dominio.ReconstruirMarcacao(s)
	return id, nil
}

func (f *fakeMarcacoes) ObterPorID(_ context.Context, id string) (*dominio.Marcacao, error) {
	m, ok := f.dados[id]
	if !ok {
		return nil, erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
	}
	// devolve uma cópia rehidratada (EstadoAnterior fixado no estado persistido)
	return dominio.ReconstruirMarcacao(m.Snapshot()), nil
}

func (f *fakeMarcacoes) Transitar(_ context.Context, m *dominio.Marcacao) error {
	s := m.Snapshot()
	cur, ok := f.dados[s.ID]
	if !ok {
		return erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
	}
	if cur.Estado() != s.EstadoAnterior {
		return erros.Novo(erros.CategoriaConflito, "o estado da marcação mudou entretanto")
	}
	f.dados[s.ID] = dominio.ReconstruirMarcacao(s)
	return nil
}

func (f *fakeMarcacoes) Remarcar(_ context.Context, original, nova *dominio.Marcacao) (string, error) {
	so := original.Snapshot()
	cur, ok := f.dados[so.ID]
	if !ok {
		return "", erros.Novo(erros.CategoriaNaoEncontrado, "marcação não encontrada")
	}
	if cur.Estado() != so.EstadoAnterior {
		return "", erros.Novo(erros.CategoriaConflito, "o estado da marcação mudou entretanto")
	}
	f.dados[so.ID] = dominio.ReconstruirMarcacao(so)
	return f.Guardar(context.Background(), nova)
}

func (f *fakeMarcacoes) ListarActivasPorMedicoIntervalo(_ context.Context, medicoID string, de, ate time.Time) ([]dominio.Marcacao, error) {
	var out []dominio.Marcacao
	for _, m := range f.dados {
		if m.MedicoID() == medicoID && m.Estado() == dominio.MarcMarcada &&
			m.Inicio().Before(ate) && de.Before(m.Fim()) {
			out = append(out, *dominio.ReconstruirMarcacao(m.Snapshot()))
		}
	}
	return out, nil
}

func (f *fakeMarcacoes) ListarPorMedicoIntervalo(_ context.Context, medicoID string, de, ate time.Time) ([]dominio.ResumoMarcacao, error) {
	var out []dominio.ResumoMarcacao
	for _, m := range f.dados {
		if m.MedicoID() == medicoID && m.Inicio().Before(ate) && de.Before(m.Fim()) {
			out = append(out, resumo(m))
		}
	}
	return out, nil
}

func (f *fakeMarcacoes) ListarPorDoente(_ context.Context, doenteID string) ([]dominio.ResumoMarcacao, error) {
	var out []dominio.ResumoMarcacao
	for _, m := range f.dados {
		if m.DoenteID() == doenteID {
			out = append(out, resumo(m))
		}
	}
	return out, nil
}

func resumo(m *dominio.Marcacao) dominio.ResumoMarcacao {
	s := m.Snapshot()
	return dominio.ResumoMarcacao{
		ID: s.ID, DoenteID: s.DoenteID, MedicoID: s.MedicoID, EspecialidadeID: s.EspecialidadeID,
		Estado: string(s.Estado), Motivo: s.Motivo, Inicio: s.Inicio, Fim: s.Fim, CriadoEm: s.CriadoEm,
	}
}

// fakeLeitorDoente responde à ACL sobre o Clínico.
type fakeLeitorDoente struct {
	activos map[string]bool
	erro    error
}

func (f fakeLeitorDoente) DoenteActivo(_ context.Context, doenteID string) (bool, error) {
	if f.erro != nil {
		return false, f.erro
	}
	return f.activos[doenteID], nil
}

// fakeAuditor acumula os registos e permite consultá-los por acção.
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

// itoa é um Itoa mínimo para evitar importar strconv nos fakes.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// janelaAgregada constrói uma janela válida directamente através do construtor do
// domínio, para preparar cenários de teste.
func janelaAgregada(t *testing.T, medico, esp, de, ate string) *dominio.JanelaDisponibilidade {
	t.Helper()
	j, err := dominio.NovaJanela(medico, esp, inst(de), inst(ate))
	if err != nil {
		t.Fatalf("janela inválida no teste: %v", err)
	}
	return j
}

// marcacaoAgregada constrói uma marcação válida directamente através do construtor do
// domínio, para preparar cenários de teste.
func marcacaoAgregada(t *testing.T, doe, medico, esp, de, ate string) *dominio.Marcacao {
	t.Helper()
	m, err := dominio.NovaMarcacao(doe, medico, esp, inst(de), inst(ate))
	if err != nil {
		t.Fatalf("marcação inválida no teste: %v", err)
	}
	return m
}

// Garantias de conformidade com as portas.
var (
	_ dominio.RepositorioJanelas   = (*fakeJanelas)(nil)
	_ dominio.RepositorioMarcacoes = (*fakeMarcacoes)(nil)
	_ app.LeitorDoente             = fakeLeitorDoente{}
	_ app.Auditor                  = (*fakeAuditor)(nil)
)
