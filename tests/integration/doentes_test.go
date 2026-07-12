//go:build integration

// Testes de integração do BC Clínico (agregado Doente) contra a BD real. Seguem o
// padrão de ciclo_vida_test.go: SKIP (nunca FAIL) quando DATABASE_URL não está
// definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestRepositorioDoentes_CicloCompleto(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repo := pgrepo.NovoRepositorioDoentes(pool)

	// Número automático.
	num, err := repo.ProximoNumeroProcesso(ctx, 2026)
	if err != nil {
		t.Fatalf("próximo número: %v", err)
	}
	if num[:2] != "P-" {
		t.Fatalf("formato inesperado: %q", num)
	}

	nasc := time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC)
	bi := "00123456LA042"
	ident, _ := dominio.NovaIdentificacao("Ana Integração", nasc, dominio.SexoFeminino, &bi, nil, nil)
	ct, _ := dominio.NovosContactos("+244923456789", nil, nil)
	doente, _ := dominio.NovoDoente(num, ident, ct, "AO")

	id, err := repo.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, id) })

	// Pesquisa por parte do nome (trigram).
	pag, err := repo.Pesquisar(ctx, dominio.FiltroDoentes{Termo: "Integr", Limite: 10})
	if err != nil {
		t.Fatalf("pesquisar: %v", err)
	}
	if pag.Total < 1 {
		t.Fatalf("esperava >=1 resultado, obtive %d", pag.Total)
	}

	// Adicionar alergia e persistir.
	alergia, _ := dominio.NovaAlergia("Penicilina", dominio.SeveridadeGrave, "", nil, "")
	_ = doente.AdicionarAlergia(alergia)
	if _, err := repo.Guardar(ctx, dominio.ReconstruirDoente(comID(doente, id))); err != nil {
		t.Fatalf("guardar com alergia: %v", err)
	}
	lido, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("obter doente: %v", err)
	}
	if len(lido.Snapshot().Alergias) != 1 {
		t.Fatalf("alergia não persistida: alergias=%d", len(lido.Snapshot().Alergias))
	}

	// Unicidade do número de processo.
	dup, _ := dominio.NovoDoente(num, ident, ct, "AO")
	if _, err := repo.Guardar(ctx, dup); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("esperava conflito de num de processo, obtive %v", err)
	}
}

// comID devolve um snapshot do doente com o id atribuído (o agregado em memória
// não conhece o id gerado pela BD).
func comID(d *dominio.Doente, id string) dominio.SnapshotDoente {
	s := d.Snapshot()
	s.ID = id
	return s
}
