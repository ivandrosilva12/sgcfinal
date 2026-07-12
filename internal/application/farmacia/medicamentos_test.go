package farmacia_test

import (
	"context"
	"errors"
	"testing"

	appfarmacia "github.com/ivandrosilva12/sgcfinal/internal/application/farmacia"
)

func dadosMedBase() appfarmacia.DadosNovoMedicamento {
	return appfarmacia.DadosNovoMedicamento{
		NomeComercial: "Amoxil 500mg", NomeGenerico: "Amoxicilina",
		FormaFarmaceutica: "COMPRIMIDO", Dosagem: "500 mg", ViaAdministracao: "ORAL",
		RequerReceita: true, StockMinimo: 10,
	}
}

func TestRegistarMedicamento_GeraCodigoEAudita(t *testing.T) {
	repo := novoFakeRepoMed()
	aud := &fakeAuditor{}
	caso := appfarmacia.NovoCasoRegistarMedicamento(repo, aud)
	out, err := caso.Executar(context.Background(), "farm-1", dadosMedBase())
	if err != nil {
		t.Fatalf("registar: %v", err)
	}
	if out.ID == "" || out.CodigoInterno == "" {
		t.Fatalf("saída incompleta: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.medicamento.registado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestRegistarMedicamento_Invalido(t *testing.T) {
	repo := novoFakeRepoMed()
	caso := appfarmacia.NovoCasoRegistarMedicamento(repo, &fakeAuditor{})
	dados := dadosMedBase()
	dados.NomeComercial = ""
	if _, err := caso.Executar(context.Background(), "farm-1", dados); err == nil {
		t.Fatal("esperava erro de validação")
	}
}

func TestDesactivarMedicamento(t *testing.T) {
	repo := novoFakeRepoMed()
	id, _ := repo.Guardar(context.Background(), medicamentoParaRepo(t))
	caso := appfarmacia.NovoCasoDefinirEstadoMedicamento(repo, &fakeAuditor{})
	out, err := caso.Desactivar(context.Background(), "farm-1", id)
	if err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	if out.Activo {
		t.Fatal("esperava inactivo")
	}
}

func TestPesquisarMedicamentos_LimiteDefault(t *testing.T) {
	repo := novoFakeRepoMed()
	caso := appfarmacia.NovoCasoPesquisarMedicamentos(repo)
	if _, err := caso.Executar(context.Background(), appfarmacia.FiltroMedicamentos{Termo: "amox"}); err != nil {
		t.Fatalf("pesquisar: %v", err)
	}
	if repo.ultimoFilt.Limite != 20 {
		t.Fatalf("limite default=%d, esperava 20", repo.ultimoFilt.Limite)
	}
}

func TestPesquisarMedicamentos_LimiteMaximoEDeslocamentoNegativo(t *testing.T) {
	repo := novoFakeRepoMed()
	caso := appfarmacia.NovoCasoPesquisarMedicamentos(repo)
	if _, err := caso.Executar(context.Background(), appfarmacia.FiltroMedicamentos{Limite: 500, Deslocamento: -5}); err != nil {
		t.Fatalf("pesquisar: %v", err)
	}
	if repo.ultimoFilt.Limite != 100 {
		t.Fatalf("limite máximo=%d, esperava 100", repo.ultimoFilt.Limite)
	}
	if repo.ultimoFilt.Deslocamento != 0 {
		t.Fatalf("deslocamento=%d, esperava 0", repo.ultimoFilt.Deslocamento)
	}
}

func TestActualizarMedicamento_Sucesso(t *testing.T) {
	repo := novoFakeRepoMed()
	id, _ := repo.Guardar(context.Background(), medicamentoParaRepo(t))
	aud := &fakeAuditor{}
	caso := appfarmacia.NovoCasoActualizarMedicamento(repo, aud)
	dados := dadosMedBase()
	dados.NomeComercial = "Amoxil Forte"
	out, err := caso.Executar(context.Background(), "farm-1", id, dados)
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	if out.NomeComercial != "Amoxil Forte" {
		t.Fatalf("nome não actualizado: %+v", out)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "farmacia.medicamento.actualizado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestActualizarMedicamento_Invalido(t *testing.T) {
	repo := novoFakeRepoMed()
	id, _ := repo.Guardar(context.Background(), medicamentoParaRepo(t))
	caso := appfarmacia.NovoCasoActualizarMedicamento(repo, &fakeAuditor{})
	dados := dadosMedBase()
	dados.NomeComercial = ""
	if _, err := caso.Executar(context.Background(), "farm-1", id, dados); err == nil {
		t.Fatal("esperava erro de validação")
	}
}

func TestActualizarMedicamento_NaoEncontrado(t *testing.T) {
	repo := novoFakeRepoMed()
	caso := appfarmacia.NovoCasoActualizarMedicamento(repo, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "farm-1", "inexistente", dadosMedBase()); err == nil {
		t.Fatal("esperava erro de não encontrado")
	}
}

func TestActivarMedicamento(t *testing.T) {
	repo := novoFakeRepoMed()
	id, _ := repo.Guardar(context.Background(), medicamentoParaRepo(t))
	aud := &fakeAuditor{}
	caso := appfarmacia.NovoCasoDefinirEstadoMedicamento(repo, aud)
	if _, err := caso.Desactivar(context.Background(), "farm-1", id); err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	out, err := caso.Activar(context.Background(), "farm-1", id)
	if err != nil {
		t.Fatalf("activar: %v", err)
	}
	if !out.Activo {
		t.Fatal("esperava activo")
	}
	if len(aud.registos) != 2 || aud.registos[1].Accao != "farmacia.medicamento.activado" {
		t.Fatalf("auditoria em falta: %+v", aud.registos)
	}
}

func TestDefinirEstadoMedicamento_NaoEncontrado(t *testing.T) {
	repo := novoFakeRepoMed()
	caso := appfarmacia.NovoCasoDefinirEstadoMedicamento(repo, &fakeAuditor{})
	if _, err := caso.Desactivar(context.Background(), "farm-1", "inexistente"); err == nil {
		t.Fatal("esperava erro de não encontrado")
	}
}

func TestObterMedicamento_Sucesso(t *testing.T) {
	repo := novoFakeRepoMed()
	id, _ := repo.Guardar(context.Background(), medicamentoParaRepo(t))
	caso := appfarmacia.NovoCasoObterMedicamento(repo)
	out, err := caso.Executar(context.Background(), id)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if out.ID != id {
		t.Fatalf("id=%q, esperava %q", out.ID, id)
	}
}

func TestObterMedicamento_NaoEncontrado(t *testing.T) {
	repo := novoFakeRepoMed()
	caso := appfarmacia.NovoCasoObterMedicamento(repo)
	if _, err := caso.Executar(context.Background(), "inexistente"); err == nil {
		t.Fatal("esperava erro de não encontrado")
	}
}

func TestRegistarMedicamento_ErroAoGuardar(t *testing.T) {
	repo := novoFakeRepoMed()
	repo.guardarErr = errors.New("falha de persistência")
	caso := appfarmacia.NovoCasoRegistarMedicamento(repo, &fakeAuditor{})
	if _, err := caso.Executar(context.Background(), "farm-1", dadosMedBase()); err == nil {
		t.Fatal("esperava erro ao guardar")
	}
}
