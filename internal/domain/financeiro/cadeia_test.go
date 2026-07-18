package financeiro_test

import (
	"strings"
	"testing"
	"time"

	fin "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
)

// cadeiaValida devolve n snapshots correctamente encadeados na série 2026.
func cadeiaValida(t *testing.T, n int) []fin.SnapshotFactura {
	t.Helper()
	m := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	out := make([]fin.SnapshotFactura, 0, n)
	anterior := ""
	for i := 1; i <= n; i++ {
		cliente, err := fin.NovoClienteSnapshot("Cliente", "", "")
		if err != nil {
			t.Fatalf("NovoClienteSnapshot: %v", err)
		}
		f, err := fin.NovaFactura(cliente, "6f1e7a8c-0b2d-4c3e-9f10-1a2b3c4d5e6f")
		if err != nil {
			t.Fatalf("NovaFactura: %v", err)
		}
		if err := f.AdicionarItem("Consulta", fin.LinhaConsulta, "", 1,
			moeda.DeCentimos(int64(1000*i)), fin.RegimeIsento); err != nil {
			t.Fatalf("AdicionarItem: %v", err)
		}
		if err := f.Emitir("2026", i, anterior, m); err != nil {
			t.Fatalf("Emitir: %v", err)
		}
		anterior = f.Hash()
		out = append(out, f.Snapshot())
	}
	return out
}

func TestVerificarCadeia_Intacta(t *testing.T) {
	if err := fin.VerificarCadeia(cadeiaValida(t, 5)); err != nil {
		t.Errorf("cadeia válida devia verificar, deu: %v", err)
	}
}

func TestVerificarCadeia_VaziaEIntacta(t *testing.T) {
	if err := fin.VerificarCadeia(nil); err != nil {
		t.Errorf("cadeia vazia é trivialmente íntegra, deu: %v", err)
	}
}

func TestVerificarCadeia_DetectaHashAdulterado(t *testing.T) {
	c := cadeiaValida(t, 5)
	c[2].Itens[0].Descricao = "Adulterada"
	err := fin.VerificarCadeia(c)
	if err == nil {
		t.Fatal("adulteração de linha devia quebrar a cadeia")
	}
	if !strings.Contains(err.Error(), "00000003") {
		t.Errorf("erro devia apontar a 3.ª factura, deu: %v", err)
	}
}

func TestVerificarCadeia_DetectaEncadeamentoErrado(t *testing.T) {
	c := cadeiaValida(t, 5)
	c[3].HashAnterior = c[0].Hash
	c[3].Hash = fin.HashDe(c[3]) // recalculado: o hash bate, o elo é que não
	if err := fin.VerificarCadeia(c); err == nil {
		t.Fatal("elo apontado à factura errada devia quebrar a cadeia")
	}
}

func TestVerificarCadeia_DetectaBuraco(t *testing.T) {
	c := cadeiaValida(t, 5)
	semTerceira := append(append([]fin.SnapshotFactura{}, c[:2]...), c[3:]...)
	err := fin.VerificarCadeia(semTerceira)
	if err == nil || !strings.Contains(err.Error(), "buraco") {
		t.Errorf("buraco na série devia ser detectado, deu: %v", err)
	}
}
