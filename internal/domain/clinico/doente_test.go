package clinico_test

import (
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func doenteValido(t *testing.T) *clinico.Doente {
	t.Helper()
	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	ident, err := clinico.NovaIdentificacao("Ana Domingos", nasc, clinico.SexoFeminino, ptr("00123456LA042"), nil, nil)
	if err != nil {
		t.Fatalf("identificação: %v", err)
	}
	ct, err := clinico.NovosContactos("+244923456789", nil, nil)
	if err != nil {
		t.Fatalf("contactos: %v", err)
	}
	d, err := clinico.NovoDoente("P-2026-000001", ident, ct, "AO")
	if err != nil {
		t.Fatalf("NovoDoente: %v", err)
	}
	return d
}

func TestNovoDoente_EstadoInicialActivo(t *testing.T) {
	d := doenteValido(t)
	if d.Estado() != clinico.EstadoActivo {
		t.Fatalf("estado inicial=%q, esperava ACTIVO", d.Estado())
	}
	if d.NumProcesso() != "P-2026-000001" {
		t.Fatalf("num processo=%q", d.NumProcesso())
	}
}

func TestNovoDoente_NacionalidadeDefault(t *testing.T) {
	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	ident, _ := clinico.NovaIdentificacao("Ana", nasc, clinico.SexoFeminino, ptr("00123456LA042"), nil, nil)
	ct, _ := clinico.NovosContactos("+244923456789", nil, nil)
	d, err := clinico.NovoDoente("P-2026-000002", ident, ct, "")
	if err != nil {
		t.Fatalf("inesperado: %v", err)
	}
	if d.Snapshot().Nacionalidade != "AO" {
		t.Fatalf("nacionalidade default=%q, esperava AO", d.Snapshot().Nacionalidade)
	}
}

func TestDoente_Desactivar(t *testing.T) {
	d := doenteValido(t)
	em := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	if err := d.Desactivar("dados duplicados", em); err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	if d.Estado() != clinico.EstadoInactivo {
		t.Fatalf("estado=%q, esperava INACTIVO", d.Estado())
	}
	if d.Snapshot().DesactivadoMotivo != "dados duplicados" || d.Snapshot().DesactivadoEm == nil {
		t.Fatalf("campos de desactivação não preenchidos: %+v", d.Snapshot())
	}
}

func TestDoente_DesactivarSemMotivo(t *testing.T) {
	d := doenteValido(t)
	if erros.CategoriaDe(d.Desactivar("  ", time.Now())) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para motivo vazio")
	}
}

func TestDoente_DeclararFalecido(t *testing.T) {
	d := doenteValido(t)
	data := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := d.DeclararFalecido(data, "I21"); err != nil {
		t.Fatalf("declarar falecido: %v", err)
	}
	if d.Estado() != clinico.EstadoFalecido {
		t.Fatalf("estado=%q, esperava FALECIDO", d.Estado())
	}
	// Um doente falecido não pode ser desactivado.
	if d.Desactivar("x", time.Now()) == nil {
		t.Fatal("esperava erro ao desactivar um falecido")
	}
}

func TestDoente_DeclararFalecidoDataFutura(t *testing.T) {
	d := doenteValido(t)
	if erros.CategoriaDe(d.DeclararFalecido(time.Now().AddDate(0, 1, 0), "")) != erros.CategoriaValidacao {
		t.Fatal("esperava validação para data de óbito futura")
	}
}

func TestDoente_Reactivar(t *testing.T) {
	d := doenteValido(t)
	_ = d.Desactivar("engano", time.Now())
	if err := d.Reactivar(); err != nil {
		t.Fatalf("reactivar: %v", err)
	}
	if d.Estado() != clinico.EstadoActivo || d.Snapshot().DesactivadoEm != nil {
		t.Fatalf("reactivação incompleta: %+v", d.Snapshot())
	}
}

func TestDoente_AdicionarAlergia(t *testing.T) {
	d := doenteValido(t)
	a, _ := clinico.NovaAlergia("Penicilina", clinico.SeveridadeGrave, "", nil, "")
	if err := d.AdicionarAlergia(a); err != nil {
		t.Fatalf("adicionar alergia: %v", err)
	}
	if len(d.Snapshot().Alergias) != 1 {
		t.Fatalf("esperava 1 alergia, obtive %d", len(d.Snapshot().Alergias))
	}
}

func TestReconstruirDoente_PreservaEstado(t *testing.T) {
	orig := doenteValido(t)
	_ = orig.Desactivar("motivo", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	snap := orig.Snapshot()
	snap.ID = "id-1"
	rec := clinico.ReconstruirDoente(snap)
	if rec.ID() != "id-1" || rec.Estado() != clinico.EstadoInactivo {
		t.Fatalf("rehidratação perdeu estado: id=%q estado=%q", rec.ID(), rec.Estado())
	}
}

func TestNovoDoente_NumProcessoVazio(t *testing.T) {
	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	ident, _ := clinico.NovaIdentificacao("Ana", nasc, clinico.SexoFeminino, ptr("00123456LA042"), nil, nil)
	ct, _ := clinico.NovosContactos("+244923456789", nil, nil)
	_, err := clinico.NovoDoente("  ", ident, ct, "AO")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação para número de processo vazio, obtive %v", err)
	}
}

func TestNovoDoente_IdentificacaoInvalida(t *testing.T) {
	ct, _ := clinico.NovosContactos("+244923456789", nil, nil)
	_, err := clinico.NovoDoente("P-2026-000003", clinico.Identificacao{}, ct, "AO")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação para identificação inválida, obtive %v", err)
	}
}

func TestNovoDoente_ContactosInvalidos(t *testing.T) {
	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	ident, _ := clinico.NovaIdentificacao("Ana", nasc, clinico.SexoFeminino, ptr("00123456LA042"), nil, nil)
	_, err := clinico.NovoDoente("P-2026-000004", ident, clinico.Contactos{}, "AO")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação para contactos inválidos, obtive %v", err)
	}
}

func TestNovoDoente_ContactosInvalidos_TelefoneFormatoInvalido(t *testing.T) {
	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	ident, _ := clinico.NovaIdentificacao("Ana", nasc, clinico.SexoFeminino, ptr("00123456LA042"), nil, nil)
	mau := clinico.Contactos{Telefone: "999"} // telefone inválido, por literal
	_, err := clinico.NovoDoente("P-2026-000099", ident, mau, "AO")
	if erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação para contactos inválidos, obtive %v", err)
	}
}

func TestDoente_AdicionarAlergia_Invalida(t *testing.T) {
	d := doenteValido(t)
	if err := d.AdicionarAlergia(clinico.Alergia{}); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação para alergia inválida, obtive %v", err)
	}
}

func TestDoente_AdicionarAntecedente(t *testing.T) {
	d := doenteValido(t)
	a, _ := clinico.NovoAntecedente(clinico.AntecedenteFamiliar, "Diabetes", "E11", nil, true, "")
	if err := d.AdicionarAntecedente(a); err != nil {
		t.Fatalf("adicionar antecedente: %v", err)
	}
	if len(d.Snapshot().Antecedentes) != 1 {
		t.Fatalf("esperava 1 antecedente, obtive %d", len(d.Snapshot().Antecedentes))
	}
}

func TestDoente_AdicionarAntecedente_Invalido(t *testing.T) {
	d := doenteValido(t)
	if err := d.AdicionarAntecedente(clinico.AntecedenteClinico{}); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação para antecedente inválido, obtive %v", err)
	}
}

func TestDoente_Reactivar_EstadoInvalido(t *testing.T) {
	d := doenteValido(t)
	if err := d.Reactivar(); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito ao reactivar um doente activo, obtive %v", err)
	}
}

func TestDoente_AtualizarIdentificacao(t *testing.T) {
	d := doenteValido(t)
	nasc := time.Date(1991, 3, 10, 0, 0, 0, 0, time.UTC)
	novaIdent, _ := clinico.NovaIdentificacao("Ana Maria Domingos", nasc, clinico.SexoFeminino, ptr("00123456LA042"), nil, nil)
	if err := d.AtualizarIdentificacao(novaIdent); err != nil {
		t.Fatalf("atualizar identificação: %v", err)
	}
	if d.Snapshot().Identificacao.NomeCompleto != "Ana Maria Domingos" {
		t.Fatalf("identificação não actualizada: %+v", d.Snapshot().Identificacao)
	}
}

func TestDoente_AtualizarIdentificacao_Invalida(t *testing.T) {
	d := doenteValido(t)
	if err := d.AtualizarIdentificacao(clinico.Identificacao{}); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação para identificação inválida, obtive %v", err)
	}
}

func TestDoente_AtualizarContactos(t *testing.T) {
	d := doenteValido(t)
	novosCt, _ := clinico.NovosContactos("+244912345678", nil, nil)
	if err := d.AtualizarContactos(novosCt); err != nil {
		t.Fatalf("atualizar contactos: %v", err)
	}
	if d.Snapshot().Contactos.Telefone != novosCt.Telefone {
		t.Fatalf("contactos não actualizados: %+v", d.Snapshot().Contactos)
	}
}

func TestDoente_AtualizarContactos_Invalidos(t *testing.T) {
	d := doenteValido(t)
	if err := d.AtualizarContactos(clinico.Contactos{}); erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("esperava validação para contactos inválidos, obtive %v", err)
	}
}

func TestDoente_DefinirGrupoSanguineo(t *testing.T) {
	d := doenteValido(t)
	g := clinico.GrupoOPositivo
	d.DefinirGrupoSanguineo(&g)
	if d.Snapshot().GrupoSanguineo == nil || *d.Snapshot().GrupoSanguineo != clinico.GrupoOPositivo {
		t.Fatalf("grupo sanguíneo não definido: %+v", d.Snapshot().GrupoSanguineo)
	}
	d.DefinirGrupoSanguineo(nil)
	if d.Snapshot().GrupoSanguineo != nil {
		t.Fatal("esperava grupo sanguíneo limpo")
	}
}

func doenteApagado(t *testing.T) *clinico.Doente {
	t.Helper()
	orig := doenteValido(t)
	snap := orig.Snapshot()
	snap.Estado = clinico.EstadoApagado
	return clinico.ReconstruirDoente(snap)
}

func TestDoente_TransicoesProibidasParaApagado(t *testing.T) {
	alergia, _ := clinico.NovaAlergia("Penicilina", clinico.SeveridadeGrave, "", nil, "")
	antecedente, _ := clinico.NovoAntecedente(clinico.AntecedenteFamiliar, "Diabetes", "E11", nil, true, "")
	novaIdent, _ := clinico.NovaIdentificacao("Ana", time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC), clinico.SexoFeminino, ptr("00123456LA042"), nil, nil)
	novosCt, _ := clinico.NovosContactos("+244923456789", nil, nil)

	casos := map[string]func(d *clinico.Doente) error{
		"AdicionarAlergia":       func(d *clinico.Doente) error { return d.AdicionarAlergia(alergia) },
		"AdicionarAntecedente":   func(d *clinico.Doente) error { return d.AdicionarAntecedente(antecedente) },
		"Desactivar":             func(d *clinico.Doente) error { return d.Desactivar("motivo", time.Now()) },
		"DeclararFalecido":       func(d *clinico.Doente) error { return d.DeclararFalecido(time.Now(), "") },
		"AtualizarIdentificacao": func(d *clinico.Doente) error { return d.AtualizarIdentificacao(novaIdent) },
		"AtualizarContactos":     func(d *clinico.Doente) error { return d.AtualizarContactos(novosCt) },
	}

	for nome, accao := range casos {
		d := doenteApagado(t)
		if err := accao(d); erros.CategoriaDe(err) != erros.CategoriaConflito {
			t.Fatalf("%s: esperava conflito para doente apagado, obtive %v", nome, err)
		}
	}
}

func TestEventosClinico_NomeEOcorridoEm(t *testing.T) {
	em := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)

	registado := clinico.DoenteRegistado{DoenteID: "d-1", Em: em}
	if registado.NomeEvento() != "clinico.doente.registado" || !registado.OcorridoEm().Equal(em) {
		t.Fatalf("DoenteRegistado inesperado: %+v", registado)
	}

	desactivado := clinico.DoenteDesactivado{DoenteID: "d-1", Em: em}
	if desactivado.NomeEvento() != "clinico.doente.desactivado" || !desactivado.OcorridoEm().Equal(em) {
		t.Fatalf("DoenteDesactivado inesperado: %+v", desactivado)
	}

	falecido := clinico.DoenteFalecido{DoenteID: "d-1", Em: em}
	if falecido.NomeEvento() != "clinico.doente.falecido" || !falecido.OcorridoEm().Equal(em) {
		t.Fatalf("DoenteFalecido inesperado: %+v", falecido)
	}

	alergiaRegistada := clinico.AlergiaRegistada{DoenteID: "d-1", Em: em}
	if alergiaRegistada.NomeEvento() != "clinico.alergia.registada" || !alergiaRegistada.OcorridoEm().Equal(em) {
		t.Fatalf("AlergiaRegistada inesperado: %+v", alergiaRegistada)
	}
}
