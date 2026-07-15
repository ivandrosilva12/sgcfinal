// internal/domain/recepcao/marcacao_test.go
package recepcao_test

import (
	"testing"

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

// comID devolve uma cópia do snapshot com o id preenchido (utilitário de teste).
func comID(s recepcao.SnapshotMarcacao, id string) recepcao.SnapshotMarcacao {
	s.ID = id
	return s
}
