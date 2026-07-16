package laboratorio_test

import (
	"reflect"
	"testing"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

func novoRes(t *testing.T) *dominio.Resultado {
	t.Helper()
	r, err := dominio.NovoResultado("req-1", "HB", "g/dL")
	if err != nil {
		t.Fatalf("resultado base inválido: %v", err)
	}
	return r
}

// resultadoEmEstado devolve um agregado rehidratado directamente num dado estado,
// como se tivesse sido lido da base de dados — é assim que se constroem os casos de
// partida da matriz de transições e do teste de fidelidade do round-trip.
func resultadoEmEstado(t *testing.T, estado dominio.EstadoResultado) *dominio.Resultado {
	t.Helper()
	return dominio.ReconstruirResultado(dominio.SnapshotResultado{
		ID: "res-1", RequisicaoID: "req-1", CodigoAnalise: "HB", Unidade: "g/dL",
		Estado: estado,
	})
}

// processadaPor devolve um resultado rehidratado em PROCESSADA submetido por `submissor`.
func processadaPor(t *testing.T, submissor string) *dominio.Resultado {
	t.Helper()
	colhida := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	submetida := colhida.Add(time.Hour)
	return dominio.ReconstruirResultado(dominio.SnapshotResultado{
		ID: "res-1", RequisicaoID: "req-1", CodigoAnalise: "HB", Valor: "2.5", Unidade: "g/dL",
		Estado: dominio.ResProcessada, TecnicoColheitaID: submissor, TecnicoSubmissorID: submissor,
		ColhidaEm: &colhida, SubmetidaEm: &submetida,
	})
}

func TestNovoResultado_NasceEmPendente(t *testing.T) {
	r := novoRes(t)
	if r.Estado() != dominio.ResPendente {
		t.Fatalf("esperava PENDENTE, veio %s", r.Estado())
	}
	if r.TecnicoSubmissorID() != "" {
		t.Fatalf("um resultado novo não tem submissor")
	}
}

func TestResultado_FluxoFeliz_AteAoPreliminar(t *testing.T) {
	r := novoRes(t)
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	if err := r.ColherAmostra("tec-1", quando); err != nil {
		t.Fatalf("colher devia funcionar: %v", err)
	}
	if r.Estado() != dominio.ResColhida {
		t.Fatalf("esperava COLHIDA, veio %s", r.Estado())
	}
	// Submeter sem valor → Validacao.
	if err := r.SubmeterPreliminar("tec-1", "  ", "", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("submeter sem valor devia falhar com Validacao, veio %v", err)
	}
	// Submeter sem técnico → Validacao. O submissor é o sujeito autenticado: se
	// chegasse vazio, a segregação do Sprint 13 não teria contra quem comparar.
	if err := r.SubmeterPreliminar("", "12.5", "", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("submeter sem técnico devia falhar com Validacao, veio %v", err)
	}
	if err := r.SubmeterPreliminar("tec-1", "12.5", "amostra hemolisada", quando.Add(time.Hour)); err != nil {
		t.Fatalf("submeter preliminar devia funcionar: %v", err)
	}
	if r.Estado() != dominio.ResProcessada {
		t.Fatalf("esperava PROCESSADA, veio %s", r.Estado())
	}
	if r.TecnicoSubmissorID() != "tec-1" {
		t.Fatalf("esperava submissor tec-1, veio %q", r.TecnicoSubmissorID())
	}
}

func TestResultado_RecusarAmostra(t *testing.T) {
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	// Sem motivo → Validacao.
	r := novoRes(t)
	if err := r.RecusarAmostra("  ", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("recusar sem motivo devia falhar com Validacao, veio %v", err)
	}
	if err := r.RecusarAmostra("amostra coagulada", quando); err != nil {
		t.Fatalf("recusar em PENDENTE devia funcionar: %v", err)
	}
	if r.Estado() != dominio.ResRecusada {
		t.Fatalf("esperava RECUSADA, veio %s", r.Estado())
	}
}

func TestNovoResultado_CamposObrigatorios(t *testing.T) {
	casos := []struct {
		nome                          string
		requisicaoID, codigo, unidade string
	}{
		{"requisição em falta", "  ", "HB", "g/dL"},
		{"código em falta", "req-1", "  ", "g/dL"},
		{"unidade em falta", "req-1", "HB", "  "},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := dominio.NovoResultado(c.requisicaoID, c.codigo, c.unidade); err == nil ||
				erros.CategoriaDe(err) != erros.CategoriaValidacao {
				t.Fatalf("%s devia falhar com Validacao, veio %v", c.nome, err)
			}
		})
	}
}

func TestResultado_ColherAmostra_DataEmFalta(t *testing.T) {
	r := novoRes(t)
	if err := r.ColherAmostra("tec-1", time.Time{}); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("colher sem data devia falhar com Validacao, veio %v", err)
	}
}

func TestResultado_RecusarAmostra_DataEmFalta(t *testing.T) {
	r := novoRes(t)
	if err := r.RecusarAmostra("amostra coagulada", time.Time{}); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("recusar sem data devia falhar com Validacao, veio %v", err)
	}
}

func TestResultado_ReconstruirPreservaEstado(t *testing.T) {
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	r := novoRes(t)
	_ = r.ColherAmostra("tec-1", quando)
	s := r.Snapshot()
	s.ID = "res-1"
	b := dominio.ReconstruirResultado(s)
	if b.Estado() != dominio.ResColhida || b.ID() != "res-1" {
		t.Fatalf("reconstrução não preservou o snapshot: %+v", b.Snapshot())
	}
	// Um agregado reconstruído tem EstadoAnterior = Estado: ainda não transitou.
	if b.Snapshot().EstadoAnterior != dominio.ResColhida {
		t.Fatalf("o estado anterior de um agregado recém-lido é o próprio estado")
	}
}

// TestResultado_Snapshot_EstadoAnterior prova a base da guarda compare-and-set do
// repositório (Task 8): um agregado novo (via NovoResultado, nunca persistido) não
// tem estado anterior — vai por INSERT, não por compare-and-set; um agregado
// rehidratado expõe o estado com que foi lido, mesmo depois de transitar.
func TestResultado_Snapshot_EstadoAnterior(t *testing.T) {
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	novo := novoRes(t)
	_ = novo.ColherAmostra("tec-1", quando)
	if ea := novo.Snapshot().EstadoAnterior; ea != "" {
		t.Fatalf("um resultado novo não devia ter estado anterior, veio %q", ea)
	}

	lido := dominio.ReconstruirResultado(dominio.SnapshotResultado{
		ID: "res-1", RequisicaoID: "req-1", CodigoAnalise: "HB", Unidade: "g/dL",
		Estado: dominio.ResColhida, TecnicoColheitaID: "tec-1", ColhidaEm: &quando,
	})
	if err := lido.SubmeterPreliminar("tec-2", "12.5", "", quando); err != nil {
		t.Fatalf("submeter preliminar: %v", err)
	}
	// EstadoAnterior tem de continuar ResColhida — é o estado que está na BD.
	if s := lido.Snapshot(); s.EstadoAnterior != dominio.ResColhida {
		t.Fatalf("esperado estado anterior COLHIDA (compare-and-set), veio %s", s.EstadoAnterior)
	}
}

// TestResultado_Snapshot_EstadoAnterior_DuasTransicoesEmMemoria garante que
// EstadoAnterior fica fixado no estado lido da base de dados mesmo depois de DUAS
// transições em memória — não no estado intermédio (COLHIDA) por onde o agregado
// passou a caminho de PROCESSADA. Se EstadoAnterior fosse actualizado a cada
// transição (por exemplo, reintroduzindo "r.estadoAnterior = r.estado" em
// SubmeterPreliminar), este teste apanhava-o: o compare-and-set do pgrepo faria
// UPDATE ... WHERE estado='COLHIDA', um estado que nunca existiu na base de dados.
func TestResultado_Snapshot_EstadoAnterior_DuasTransicoesEmMemoria(t *testing.T) {
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	lido := resultadoEmEstado(t, dominio.ResPendente)
	if err := lido.ColherAmostra("tec-1", quando); err != nil {
		t.Fatalf("colher devia funcionar: %v", err)
	}
	if err := lido.SubmeterPreliminar("tec-1", "12.5", "", quando.Add(time.Hour)); err != nil {
		t.Fatalf("submeter preliminar devia funcionar: %v", err)
	}
	if s := lido.Snapshot(); s.EstadoAnterior != dominio.ResPendente {
		t.Fatalf("esperado estado anterior PENDENTE (o que está na base de dados), veio %s", s.EstadoAnterior)
	}
}

// TestResultado_RecusarAmostra_DesdeColhida afirma o segundo caso válido de
// RecusarAmostra: um agregado rehidratado em COLHIDA (não só PENDENTE) também
// transita para RECUSADA e guarda o motivo. A matriz table-driven salta este caso
// por ser válido, e TestResultado_RecusarAmostra só cobre PENDENTE — sem este
// teste, uma guarda restrita só a PENDENTE (por exemplo, "if r.estado !=
// dominio.ResPendente") passava a suite inteira sem ser apanhada.
func TestResultado_RecusarAmostra_DesdeColhida(t *testing.T) {
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	r := resultadoEmEstado(t, dominio.ResColhida)
	if err := r.RecusarAmostra("amostra hemolisada", quando); err != nil {
		t.Fatalf("recusar em COLHIDA devia funcionar: %v", err)
	}
	if r.Estado() != dominio.ResRecusada {
		t.Fatalf("esperava RECUSADA, veio %s", r.Estado())
	}
	if s := r.Snapshot(); s.MotivoRecusa != "amostra hemolisada" {
		t.Fatalf("esperava motivo %q, veio %q", "amostra hemolisada", s.MotivoRecusa)
	}
}

// TestResultado_Transicoes_ConflitoForaDoEstadoValido é a matriz table-driven
// (estado de partida × método → CategoriaConflito): para cada um dos três métodos
// de transição, cobre TODOS os estados de partida inválidos — incluindo o
// duplo-submit (PROCESSADA → SubmeterPreliminar outra vez), o caso clássico do
// duplo-clique numa fila de laboratório. Os agregados de partida são construídos
// com ReconstruirResultado, como se tivessem sido lidos da base de dados.
func TestResultado_Transicoes_ConflitoForaDoEstadoValido(t *testing.T) {
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	todos := []dominio.EstadoResultado{
		dominio.ResPendente, dominio.ResColhida, dominio.ResProcessada,
		dominio.ResValidada, dominio.ResConcluida, dominio.ResRecusada,
	}

	casos := []struct {
		metodo   string
		validos  []dominio.EstadoResultado
		executar func(r *dominio.Resultado) error
	}{
		{
			metodo:  "ColherAmostra",
			validos: []dominio.EstadoResultado{dominio.ResPendente},
			executar: func(r *dominio.Resultado) error {
				return r.ColherAmostra("tec-1", quando)
			},
		},
		{
			metodo:  "RecusarAmostra",
			validos: []dominio.EstadoResultado{dominio.ResPendente, dominio.ResColhida},
			executar: func(r *dominio.Resultado) error {
				return r.RecusarAmostra("amostra coagulada", quando)
			},
		},
		{
			metodo:  "SubmeterPreliminar",
			validos: []dominio.EstadoResultado{dominio.ResColhida},
			executar: func(r *dominio.Resultado) error {
				return r.SubmeterPreliminar("tec-2", "12.5", "", quando)
			},
		},
	}

	ehValido := func(estado dominio.EstadoResultado, validos []dominio.EstadoResultado) bool {
		for _, v := range validos {
			if estado == v {
				return true
			}
		}
		return false
	}

	for _, c := range casos {
		for _, estado := range todos {
			if ehValido(estado, c.validos) {
				continue
			}
			t.Run(c.metodo+"_desde_"+string(estado), func(t *testing.T) {
				r := resultadoEmEstado(t, estado)
				err := c.executar(r)
				if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
					t.Fatalf("%s desde %s devia falhar com Conflito, veio %v", c.metodo, estado, err)
				}
			})
		}
	}
}

// TestResultado_ReconstruirRoundTrip garante que ReconstruirResultado(s).Snapshot()
// devolve s para TODOS os campos: a Task 8 depende de fidelidade total, e um
// esquecimento no mapeamento (ex.: motivoRecusa ou valorCritico a cair) não seria
// apanhado por nenhum outro teste. EstadoAnterior é o caso especial — é derivado do
// Estado por ReconstruirResultado, nunca transportado do snapshot de origem.
func TestResultado_ReconstruirRoundTrip(t *testing.T) {
	colhidaEm := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	submetidaEm := colhidaEm.Add(time.Hour)
	validadaEm := submetidaEm.Add(time.Hour)

	original := dominio.SnapshotResultado{
		ID:                     "res-1",
		RequisicaoID:           "req-1",
		CodigoAnalise:          "HB",
		Valor:                  "12.5",
		Unidade:                "g/dL",
		Observacoes:            "amostra hemolisada",
		MotivoRecusa:           "amostra coagulada",
		Estado:                 dominio.ResValidada,
		TecnicoColheitaID:      "tec-1",
		TecnicoSubmissorID:     "tec-2",
		PatologistaValidadorID: "pat-1",
		ColhidaEm:              &colhidaEm,
		SubmetidaEm:            &submetidaEm,
		ValidadaEm:             &validadaEm,
		ValorCritico:           true,
		CriadoEm:               colhidaEm,
	}
	// EstadoAnterior não é transportado — é derivado do Estado na reconstrução.
	esperado := original
	esperado.EstadoAnterior = original.Estado

	obtido := dominio.ReconstruirResultado(original).Snapshot()
	if !reflect.DeepEqual(esperado, obtido) {
		t.Fatalf("round-trip não preservou o snapshot:\nesperado %+v\nveio     %+v", esperado, obtido)
	}
}

func TestResultado_Validar_FluxoFeliz(t *testing.T) {
	quando := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	r := processadaPor(t, "tec-1")
	if err := r.Validar("pat-9", true, quando); err != nil {
		t.Fatalf("validar devia funcionar: %v", err)
	}
	if r.Estado() != dominio.ResValidada {
		t.Fatalf("esperava VALIDADA, veio %s", r.Estado())
	}
	s := r.Snapshot()
	if s.PatologistaValidadorID != "pat-9" || s.ValidadaEm == nil || !s.ValorCritico {
		t.Fatalf("validação não gravou validador/data/crítico: %+v", s)
	}
}

func TestResultado_Validar_Segregacao(t *testing.T) {
	quando := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	r := processadaPor(t, "tec-1")
	// O próprio submissor não pode validar.
	err := r.Validar("tec-1", false, quando)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("auto-validação devia falhar com RegraNegocio, veio %v", err)
	}
}

func TestResultado_Validar_ForaDeProcessada_Conflito(t *testing.T) {
	quando := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	for _, estado := range []dominio.EstadoResultado{
		dominio.ResPendente, dominio.ResColhida, dominio.ResValidada,
		dominio.ResConcluida, dominio.ResRecusada,
	} {
		r := resultadoEmEstado(t, estado)
		err := r.Validar("pat-1", false, quando)
		if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
			t.Fatalf("validar desde %s devia dar Conflito, veio %v", estado, err)
		}
	}
}

func TestResultado_Validar_CamposEmFalta(t *testing.T) {
	r := processadaPor(t, "tec-1")
	if err := r.Validar("  ", false, time.Now()); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("validar sem patologista devia dar Validacao, veio %v", err)
	}
	r2 := processadaPor(t, "tec-1")
	if err := r2.Validar("pat-1", false, time.Time{}); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaValidacao {
		t.Fatalf("validar sem data devia dar Validacao, veio %v", err)
	}
}
