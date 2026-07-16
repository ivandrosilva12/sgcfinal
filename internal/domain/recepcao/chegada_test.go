// internal/domain/recepcao/chegada_test.go
package recepcao_test

import (
	"testing"
	"time"

	recepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func TestNovaChegadaAgendada_NasceAguarda(t *testing.T) {
	c, err := recepcao.NovaChegadaAgendada("doe-1", "marc-1", "med-1", "esp-1", inst("09:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.Estado() != recepcao.ChegAguarda {
		t.Fatalf("esperava AGUARDA, veio %s", c.Estado())
	}
	if c.DoenteID() != "doe-1" || c.MarcacaoID() != "marc-1" || c.MedicoID() != "med-1" || c.EspecialidadeID() != "esp-1" {
		t.Fatal("campos mal preenchidos")
	}
}

func TestNovaChegadaAgendada_CamposObrigatorios(t *testing.T) {
	casos := []struct {
		nome                      string
		doente, marc, medico, esp string
	}{
		{"sem doente", "", "marc-1", "med-1", "esp-1"},
		{"sem marcacao", "doe-1", "", "med-1", "esp-1"},
		{"sem medico", "doe-1", "marc-1", "", "esp-1"},
		{"sem especialidade", "doe-1", "marc-1", "med-1", ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := recepcao.NovaChegadaAgendada(c.doente, c.marc, c.medico, c.esp, inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
				t.Fatalf("%s: esperava CategoriaValidacao, veio %v", c.nome, erros.CategoriaDe(err))
			}
		})
	}
}

func TestNovaChegadaWalkIn_SemMarcacaoNemMedico(t *testing.T) {
	c, err := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.Estado() != recepcao.ChegAguarda || c.MarcacaoID() != "" || c.MedicoID() != "" {
		t.Fatalf("walk-in mal construído: %+v", c.Snapshot())
	}
	if c.DoenteID() != "doe-1" || c.EspecialidadeID() != "esp-1" {
		t.Fatal("doente/especialidade mal preenchidos")
	}
}

func TestNovaChegadaWalkIn_CamposObrigatorios(t *testing.T) {
	if _, err := recepcao.NovaChegadaWalkIn("", "esp-1", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("sem doente: esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
	if _, err := recepcao.NovaChegadaWalkIn("doe-1", "", inst("09:00")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("sem especialidade: esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestChamar_DeAguarda(t *testing.T) {
	c, _ := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	if err := c.Chamar(inst("09:10")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.Estado() != recepcao.ChegChamado {
		t.Fatalf("esperava CHAMADO, veio %s", c.Estado())
	}
}

func TestRegistarDesistencia_DeAguarda(t *testing.T) {
	c, _ := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	if err := c.RegistarDesistencia(inst("09:10")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.Estado() != recepcao.ChegDesistiu {
		t.Fatalf("esperava DESISTIU, veio %s", c.Estado())
	}
}

func TestChegada_TransicoesInvalidas_Conflito(t *testing.T) {
	c, _ := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	_ = c.Chamar(inst("09:10"))
	if err := c.Chamar(inst("09:20")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("chamar duas vezes devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
	if err := c.RegistarDesistencia(inst("09:20")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("desistir depois de chamado devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestChegada_SnapshotEstadoAnterior_NaoMutaAposTransicao(t *testing.T) {
	rehidratada := recepcao.ReconstruirChegada(recepcao.SnapshotChegada{
		ID: "cheg-1", DoenteID: "doe-1", EspecialidadeID: "esp-1",
		Estado: recepcao.ChegAguarda, HoraChegada: inst("09:00"),
	})
	if err := rehidratada.Chamar(inst("09:10")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if rehidratada.Snapshot().EstadoAnterior != recepcao.ChegAguarda {
		t.Fatalf("EstadoAnterior devia continuar AGUARDA, veio %s", rehidratada.Snapshot().EstadoAnterior)
	}
}

func TestNovaChegadaAgendada_HoraEmFalta(t *testing.T) {
	if _, err := recepcao.NovaChegadaAgendada("doe-1", "marc-1", "med-1", "esp-1", time.Time{}); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestNovaChegadaWalkIn_HoraEmFalta(t *testing.T) {
	if _, err := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", time.Time{}); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestChegada_RegistarDesistencia_DepoisDeChamado_Conflito(t *testing.T) {
	c, _ := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	_ = c.Chamar(inst("09:10"))
	if err := c.RegistarDesistencia(inst("09:20")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava Conflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestChegada_RegistarDesistencia_Duplicada_Conflito(t *testing.T) {
	c, _ := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	_ = c.RegistarDesistencia(inst("09:10"))
	if err := c.RegistarDesistencia(inst("09:20")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava Conflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestNovaChegadaAgendada_ID_VazioAntesDePersistir(t *testing.T) {
	c, _ := recepcao.NovaChegadaAgendada("doe-1", "marc-1", "med-1", "esp-1", inst("09:00"))
	if c.ID() != "" {
		t.Fatalf("esperava id vazio antes de persistir, veio %q", c.ID())
	}
}

func chegadaChamada(t *testing.T, walkin bool) *recepcao.Chegada {
	t.Helper()
	var c *recepcao.Chegada
	var err error
	if walkin {
		c, err = recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00"))
	} else {
		c, err = recepcao.NovaChegadaAgendada("doe-1", "marc-1", "med-1", "esp-1", inst("09:00"))
	}
	if err != nil {
		t.Fatalf("chegada inválida: %v", err)
	}
	if err := c.Chamar(inst("09:05")); err != nil {
		t.Fatalf("chamar: %v", err)
	}
	return c
}

func TestRegistarTriada_WalkIn_AtribuiMedico(t *testing.T) {
	c := chegadaChamada(t, true) // walk-in, sem médico
	if err := c.RegistarTriada("med-9", inst("09:10")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.Estado() != recepcao.ChegTriado {
		t.Fatalf("esperava TRIADO, veio %s", c.Estado())
	}
	if c.MedicoID() != "med-9" {
		t.Fatalf("o médico do walk-in devia ser atribuído, veio %q", c.MedicoID())
	}
}

func TestRegistarTriada_WalkIn_SemMedico_Validacao(t *testing.T) {
	c := chegadaChamada(t, true)
	if err := c.RegistarTriada("", inst("09:10")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("walk-in sem médico devia dar CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestRegistarTriada_Agendada_HerdaMedico(t *testing.T) {
	c := chegadaChamada(t, false) // agendada, já com med-1
	if err := c.RegistarTriada("", inst("09:10")); err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
	if c.MedicoID() != "med-1" {
		t.Fatalf("devia herdar o médico da marcação, veio %q", c.MedicoID())
	}
}

func TestRegistarTriada_Agendada_MedicoIndevido_Validacao(t *testing.T) {
	c := chegadaChamada(t, false)
	if err := c.RegistarTriada("med-9", inst("09:10")); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("re-atribuir médico a chegada agendada devia dar CategoriaValidacao, veio %v", erros.CategoriaDe(err))
	}
}

func TestRegistarTriada_ForaDeChamado_Conflito(t *testing.T) {
	c, _ := recepcao.NovaChegadaWalkIn("doe-1", "esp-1", inst("09:00")) // AGUARDA, não CHAMADO
	if err := c.RegistarTriada("med-9", inst("09:10")); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("triar uma chegada não chamada devia dar CategoriaConflito, veio %v", erros.CategoriaDe(err))
	}
}

func TestReconstruirChegada_PreserveCampos(t *testing.T) {
	horaCriacao := inst("08:30")
	horaActualizacao := inst("09:05")
	s := recepcao.SnapshotChegada{
		ID: "cheg-2", DoenteID: "doe-2", MarcacaoID: "marc-2", MedicoID: "med-2",
		EspecialidadeID: "esp-2", HoraChegada: inst("09:00"), Estado: recepcao.ChegChamado,
		EstadoAnterior: recepcao.ChegAguarda, CriadoEm: horaCriacao, ActualizadoEm: horaActualizacao,
	}
	c := recepcao.ReconstruirChegada(s)
	if c.ID() != "cheg-2" || c.DoenteID() != "doe-2" || c.MarcacaoID() != "marc-2" ||
		c.MedicoID() != "med-2" || c.EspecialidadeID() != "esp-2" || c.Estado() != recepcao.ChegChamado {
		t.Fatalf("reconstrução perdeu campos: %+v", c.Snapshot())
	}
	if !c.HoraChegada().Equal(inst("09:00")) {
		t.Fatal("hora de chegada mal reconstruída")
	}
	got := c.Snapshot()
	if got.CriadoEm != horaCriacao || got.ActualizadoEm != horaActualizacao {
		t.Fatal("timestamps mal reconstruídos")
	}
}
