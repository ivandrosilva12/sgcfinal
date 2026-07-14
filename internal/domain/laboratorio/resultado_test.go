package laboratorio_test

import (
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

func TestNovoResultado_NasceEmPendente(t *testing.T) {
	r := novoRes(t)
	if r.Estado() != dominio.ResPendente {
		t.Fatalf("esperava PENDENTE, veio %s", r.Estado())
	}
	if r.TecnicoSubmissorID() != "" {
		t.Fatalf("um resultado novo não tem submissor")
	}
}

func TestResultado_CicloAteAoPreliminar(t *testing.T) {
	r := novoRes(t)
	quando := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	// Submeter sem colher → Conflito.
	if err := r.SubmeterPreliminar("tec-1", "12.5", "", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("submeter sem colher devia falhar com Conflito, veio %v", err)
	}
	if err := r.ColherAmostra("tec-1", quando); err != nil {
		t.Fatalf("colher devia funcionar: %v", err)
	}
	if r.Estado() != dominio.ResColhida {
		t.Fatalf("esperava COLHIDA, veio %s", r.Estado())
	}
	// Colher de novo → Conflito.
	if err := r.ColherAmostra("tec-1", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("colher duas vezes devia falhar com Conflito, veio %v", err)
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
	s := r.Snapshot()
	if s.EstadoAnterior != dominio.ResColhida {
		t.Fatalf("o snapshot devia expor o estado anterior COLHIDA (compare-and-set), veio %s", s.EstadoAnterior)
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
	// Recusar de novo → Conflito.
	if err := r.RecusarAmostra("outra razão", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("recusar duas vezes devia falhar com Conflito, veio %v", err)
	}

	// Depois de processada já não se recusa.
	p := novoRes(t)
	_ = p.ColherAmostra("tec-1", quando)
	_ = p.SubmeterPreliminar("tec-1", "12.5", "", quando)
	if err := p.RecusarAmostra("tarde demais", quando); err == nil ||
		erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("recusar uma amostra já processada devia falhar com Conflito, veio %v", err)
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

func TestResultado_RequisicaoID(t *testing.T) {
	r := novoRes(t)
	if r.RequisicaoID() != "req-1" {
		t.Fatalf("esperava requisicao req-1, veio %q", r.RequisicaoID())
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
