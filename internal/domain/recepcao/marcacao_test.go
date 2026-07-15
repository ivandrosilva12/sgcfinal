// internal/domain/recepcao/marcacao_test.go
package recepcao_test

import (
	"testing"
	"time"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func novaMarcacaoValida(t *testing.T) *recepcao.Marcacao {
	t.Helper()
	m, err := recepcao.NovaMarcacao("doe-1", "med-1", "esp-1", inst("09:00"), inst("09:30"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	return m
}

func TestNovaMarcacao_NasceMarcada(t *testing.T) {
	m := novaMarcacaoValida(t)
	if m.Estado() != recepcao.MarcMarcada {
		t.Fatalf("esperava MARCADA, veio %s", m.Estado())
	}
	if m.DoenteID() != "doe-1" || m.MedicoID() != "med-1" || m.EspecialidadeID() != "esp-1" {
		t.Fatal("campos mal preenchidos")
	}
}

func TestNovaMarcacao_FimNaoPosterior_Erro(t *testing.T) {
	_, err := recepcao.NovaMarcacao("doe-1", "med-1", "esp-1", inst("09:30"), inst("09:00"))
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestNovaMarcacao_DoenteEmFalta_Erro(t *testing.T) {
	_, err := recepcao.NovaMarcacao("  ", "med-1", "esp-1", inst("09:00"), inst("09:30"))
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao com doente em falta, veio %v", erros.CategoriaDe(err))
	}
}

func TestNovaMarcacao_MedicoEmFalta_Erro(t *testing.T) {
	_, err := recepcao.NovaMarcacao("doe-1", "  ", "esp-1", inst("09:00"), inst("09:30"))
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao com médico em falta, veio %v", erros.CategoriaDe(err))
	}
}

func TestNovaMarcacao_EspecialidadeEmFalta_Erro(t *testing.T) {
	_, err := recepcao.NovaMarcacao("doe-1", "med-1", "  ", inst("09:00"), inst("09:30"))
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao com especialidade em falta, veio %v", erros.CategoriaDe(err))
	}
}

func TestNovaMarcacao_InicioZero_Erro(t *testing.T) {
	_, err := recepcao.NovaMarcacao("doe-1", "med-1", "esp-1", time.Time{}, inst("09:30"))
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao com início zero, veio %v", erros.CategoriaDe(err))
	}
}

func TestCancelar_DeMarcada(t *testing.T) {
	m := novaMarcacaoValida(t)
	if err := m.Cancelar("doente desistiu", inst("08:00")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if m.Estado() != recepcao.MarcCancelada {
		t.Fatalf("esperava CANCELADA, veio %s", m.Estado())
	}
}

func TestCancelar_SemMotivo_Erro(t *testing.T) {
	m := novaMarcacaoValida(t)
	if err := m.Cancelar("  ", inst("08:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao sem motivo, veio %v", erros.CategoriaDe(err))
	}
}

func TestCancelar_JaCancelada_Conflito(t *testing.T) {
	m := novaMarcacaoValida(t)
	_ = m.Cancelar("motivo", inst("08:00"))
	if err := m.Cancelar("outra vez", inst("08:00")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestRemarcar_SupersedePreservandoOriginal(t *testing.T) {
	original := novaMarcacaoValida(t)
	// simula que a original já foi persistida com um id
	original = recepcao.ReconstruirMarcacao(comID(original.Snapshot(), "marc-1"))

	nova, err := original.Remarcar(inst("10:00"), inst("10:30"), inst("08:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if original.Estado() != recepcao.MarcRemarcada {
		t.Fatalf("original devia ficar REMARCADA, veio %s", original.Estado())
	}
	if nova.Estado() != recepcao.MarcMarcada {
		t.Fatalf("nova devia ser MARCADA, veio %s", nova.Estado())
	}
	if nova.RemarcaDe() != "marc-1" {
		t.Fatalf("nova devia apontar para a original (marc-1), veio %q", nova.RemarcaDe())
	}
	if nova.DoenteID() != "doe-1" || nova.MedicoID() != "med-1" || nova.EspecialidadeID() != "esp-1" {
		t.Fatal("a nova marcação devia preservar doente/médico/especialidade")
	}
	if !nova.Inicio().Equal(inst("10:00")) {
		t.Fatal("a nova marcação devia ter o novo início")
	}
}

func TestRemarcar_OriginalNaoPersistida_Erro(t *testing.T) {
	original := novaMarcacaoValida(t) // sem id: nunca foi persistida
	_, err := original.Remarcar(inst("10:00"), inst("10:30"), inst("08:00"))
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao para original não persistida, veio %v", erros.CategoriaDe(err))
	}
}

func TestRemarcar_NovoIntervaloInvalido_MantemOriginalMarcada(t *testing.T) {
	original := novaMarcacaoValida(t)
	original = recepcao.ReconstruirMarcacao(comID(original.Snapshot(), "marc-1"))

	// novoFim antes de novoInicio: NovaMarcacao interna rejeita.
	_, err := original.Remarcar(inst("10:30"), inst("10:00"), inst("08:00"))
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao com novo intervalo inválido, veio %v", erros.CategoriaDe(err))
	}
	if original.Estado() != recepcao.MarcMarcada {
		t.Fatalf("a original não devia mudar de estado em caso de erro, veio %s", original.Estado())
	}
}

func TestRegistarFalta_DeEstadoNaoMarcada_Conflito(t *testing.T) {
	m := novaMarcacaoValida(t)
	if err := m.Cancelar("doente desistiu", inst("08:00")); err != nil {
		t.Fatalf("não esperava erro ao cancelar: %v", err)
	}
	if err := m.RegistarFalta(inst("10:00")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito ao registar falta de uma marcação CANCELADA, veio %v", erros.CategoriaDe(err))
	}
}

func TestMarcacao_GettersIDEFim(t *testing.T) {
	s := recepcao.SnapshotMarcacao{
		ID: "marc-9", DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"), Estado: recepcao.MarcMarcada,
		CriadoEm: inst("08:00"), ActualizadoEm: inst("08:00"),
	}
	m := recepcao.ReconstruirMarcacao(s)
	if m.ID() != "marc-9" {
		t.Fatalf("esperava ID marc-9, veio %q", m.ID())
	}
	if !m.Fim().Equal(inst("09:30")) {
		t.Fatalf("esperava Fim 09:30, veio %v", m.Fim())
	}
}

// TestReconstruirMarcacao_EstadoAnterior_NaoMutaAposTransicao blinda a semântica de
// EstadoAnterior: tem de reflectir sempre o estado lido da base de dados no momento da
// rehidratação, mesmo depois de uma transição alterar Estado(). Se alguém alterar uma
// transição (ex.: Cancelar) para fazer estadoAnterior = estado, este teste falha.
func TestReconstruirMarcacao_EstadoAnterior_NaoMutaAposTransicao(t *testing.T) {
	s := recepcao.SnapshotMarcacao{
		ID: "marc-1", DoenteID: "doe-1", MedicoID: "med-1", EspecialidadeID: "esp-1",
		Inicio: inst("09:00"), Fim: inst("09:30"), Estado: recepcao.MarcMarcada,
		CriadoEm: inst("08:00"), ActualizadoEm: inst("08:00"),
	}
	m := recepcao.ReconstruirMarcacao(s)
	if m.Snapshot().EstadoAnterior != recepcao.MarcMarcada {
		t.Fatalf("esperava EstadoAnterior MARCADA logo após rehidratação, veio %s", m.Snapshot().EstadoAnterior)
	}

	if err := m.Cancelar("doente desistiu", inst("08:30")); err != nil {
		t.Fatalf("não esperava erro ao cancelar: %v", err)
	}
	if m.Estado() != recepcao.MarcCancelada {
		t.Fatalf("esperava Estado CANCELADA após transição, veio %s", m.Estado())
	}
	if m.Snapshot().EstadoAnterior != recepcao.MarcMarcada {
		t.Fatalf("EstadoAnterior devia continuar MARCADA (o estado rehidratado), veio %s", m.Snapshot().EstadoAnterior)
	}
}

func TestRegistarFalta_SoAposAHora(t *testing.T) {
	m := novaMarcacaoValida(t) // fim = 09:30
	// antes da hora: recusa
	if err := m.RegistarFalta(inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("falta antes da hora devia dar CategoriaRegraNegocio, veio %v", erros.CategoriaDe(err))
	}
	// depois da hora: aceita
	if err := m.RegistarFalta(inst("10:00")); err != nil {
		t.Fatalf("não esperava erro após a hora: %v", err)
	}
	if m.Estado() != recepcao.MarcFaltou {
		t.Fatalf("esperava FALTOU, veio %s", m.Estado())
	}
}

func TestRegistarComparencia_DeMarcada(t *testing.T) {
	m := novaMarcacaoValida(t)
	if err := m.RegistarComparencia(inst("09:00")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if m.Estado() != recepcao.MarcCompareceu {
		t.Fatalf("esperava COMPARECEU, veio %s", m.Estado())
	}
}

func TestRegistarComparencia_NaoMarcada_Conflito(t *testing.T) {
	m := novaMarcacaoValida(t)
	_ = m.Cancelar("motivo", inst("08:00"))
	if err := m.RegistarComparencia(inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestTransicoes_RecusamAPartirDeCompareceu(t *testing.T) {
	// Depois de comparecer, a marcação já não pode ser cancelada, remarcada nem dar falta.
	base := novaMarcacaoValida(t)
	base = recepcao.ReconstruirMarcacao(comID(base.Snapshot(), "marc-1"))
	_ = base.RegistarComparencia(inst("09:00"))

	if err := base.Cancelar("x", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("Cancelar a partir de COMPARECEU devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
	if _, err := base.Remarcar(inst("10:00"), inst("10:30"), inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("Remarcar a partir de COMPARECEU devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
	if err := base.RegistarFalta(inst("12:00")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("RegistarFalta a partir de COMPARECEU devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
}

// comID devolve uma cópia do snapshot com o id preenchido (utilitário de teste).
func comID(s recepcao.SnapshotMarcacao, id string) recepcao.SnapshotMarcacao {
	s.ID = id
	return s
}
