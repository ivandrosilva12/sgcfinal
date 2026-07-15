//go:build integration

// Teste de integração do repositório pgx RepositorioJanelas (BC Recepção,
// Task 9). Segue o padrão de migracoes_test.go: ligar(t) só liga o pool,
// as migrações aplicam-se explicitamente a seguir, e SKIPa (nunca FAIL)
// quando DATABASE_URL não está definido.
package integration_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// Ids fixos e distintos — o BC Recepção não valida médico/especialidade
// (referências textuais a outros bounded contexts, sem FK, ver migração 0001).
const (
	medicoJanelaID        = "11111111-1111-1111-1111-111111111111"
	especialidadeJanelaID = "22222222-2222-2222-2222-222222222222"
)

func instJanela(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("data inválida: %v", err)
	}
	return v
}

func TestRecepcaoJanelasRepo_GuardarObterListarRemover(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	repo := pgrepo.NovoRepositorioJanelas(pool)

	inicio := instJanela(t, "2026-08-01T08:00:00Z")
	fim := instJanela(t, "2026-08-01T13:00:00Z")
	j, err := dominio.NovaJanela(medicoJanelaID, especialidadeJanelaID, inicio, fim)
	if err != nil {
		t.Fatalf("construir janela: %v", err)
	}

	id, err := repo.Guardar(ctx, j)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.janelas WHERE id=$1`, id)
	})
	if id == "" {
		t.Fatal("esperava id não vazio devolvido pela base de dados")
	}

	got, err := repo.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if got.ID() != id {
		t.Fatalf("id devolvido = %q, esperava %q", got.ID(), id)
	}
	if got.MedicoID() != medicoJanelaID {
		t.Fatalf("medicoID = %q, esperava %q", got.MedicoID(), medicoJanelaID)
	}
	if got.EspecialidadeID() != especialidadeJanelaID {
		t.Fatalf("especialidadeID = %q, esperava %q", got.EspecialidadeID(), especialidadeJanelaID)
	}
	if !got.Inicio().Equal(inicio) {
		t.Fatalf("início = %v, esperava %v", got.Inicio(), inicio)
	}
	if !got.Fim().Equal(fim) {
		t.Fatalf("fim = %v, esperava %v", got.Fim(), fim)
	}

	// Intervalo [09:00,10:00] está contido em [08:00,13:00) — sobrepõe-se.
	lista, err := repo.ListarPorMedicoIntervalo(ctx, medicoJanelaID,
		instJanela(t, "2026-08-01T09:00:00Z"), instJanela(t, "2026-08-01T10:00:00Z"))
	if err != nil {
		t.Fatalf("listar sobreposição: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("listar sobreposição: esperava 1 janela, obtive %d", len(lista))
	}
	if lista[0].ID() != id {
		t.Fatalf("janela listada tem id %q, esperava %q", lista[0].ID(), id)
	}

	// Intervalo totalmente fora [14:00,15:00] não se sobrepõe — lista vazia.
	fora, err := repo.ListarPorMedicoIntervalo(ctx, medicoJanelaID,
		instJanela(t, "2026-08-01T14:00:00Z"), instJanela(t, "2026-08-01T15:00:00Z"))
	if err != nil {
		t.Fatalf("listar sem sobreposição: %v", err)
	}
	if len(fora) != 0 {
		t.Fatalf("listar sem sobreposição: esperava 0 janelas, obtive %d", len(fora))
	}

	if err := repo.Remover(ctx, id); err != nil {
		t.Fatalf("remover: %v", err)
	}

	if _, err := repo.ObterPorID(ctx, id); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("obter após remover: esperava CategoriaNaoEncontrado, obtive %v (%v)", erros.CategoriaDe(err), err)
	}

	if err := repo.Remover(ctx, id); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("remover já removida: esperava CategoriaNaoEncontrado, obtive %v (%v)", erros.CategoriaDe(err), err)
	}
}
