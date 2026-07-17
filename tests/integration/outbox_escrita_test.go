//go:build integration

// Teste de integração: o fecho do episódio escreve o evento EpisodioFechado no
// shared.outbox NA MESMA transacção do UPDATE de estado (garantia atómica do
// padrão Outbox, ADR-038).
package integration_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// Fecha um episódio via repositório e verifica que a linha de outbox foi escrita
// na MESMA transacção (existe após o commit do Guardar).
func TestGuardar_FechoEscreveOutboxNaMesmaTx(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repo := pgrepo.NovoRepositorioEpisodios(pool)

	// monta e persiste um episódio ABERTO, depois fecha-o.
	ep := episodioAbertoParaTeste(t, pool, ctx) // helper: cria doente + episódio ABERTO, devolve o agregado relido
	const medID = "33333333-3333-4333-8333-333333333333"
	if err := ep.Fechar(medID, agora()); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	id, err := repo.Guardar(ctx, ep)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}

	var n int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM shared.outbox
		WHERE tipo_evento='clinico.episodio.fechado'
		AND payload->>'EpisodioID' = $1 AND publicado_em IS NULL`, id).Scan(&n)
	if err != nil {
		t.Fatalf("consultar outbox: %v", err)
	}
	if n != 1 {
		t.Fatalf("esperava 1 linha de outbox pendente, obtive %d", n)
	}
}

// agora devolve o instante actual; existe só para dar nome semântico ao "Em" dos
// eventos de domínio nos testes de integração.
func agora() time.Time {
	return time.Now()
}

// episodioAbertoParaTeste cria um doente e um episódio ABERTO, regista a limpeza
// (episódio + doente + linhas de outbox geradas no teste) e devolve o agregado
// relido do repositório — uma instância "limpa" (sem eventos pendentes), pronta
// para que o chamador invoque Fechar() sobre ELA e depois Guardar() sobre a MESMA
// instância (Snapshot/ReconstruirEpisodio não transportam eventos pendentes).
func episodioAbertoParaTeste(t *testing.T, pool *pgxpool.Pool, ctx context.Context) *dominio.EpisodioClinico {
	t.Helper()
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	repoEp := pgrepo.NovoRepositorioEpisodios(pool)

	nasc := time.Date(1988, 3, 12, 0, 0, 0, 0, time.UTC)
	bi := "00123456LA042"
	num, err := repoDoentes.ProximoNumeroProcesso(ctx, 2026)
	if err != nil {
		t.Fatalf("próximo número de processo: %v", err)
	}
	ident, err := dominio.NovaIdentificacao("Beatriz Outbox", nasc, dominio.SexoFeminino, &bi, nil, nil)
	if err != nil {
		t.Fatalf("identificação: %v", err)
	}
	ct, err := dominio.NovosContactos("+244923456790", nil, nil)
	if err != nil {
		t.Fatalf("contactos: %v", err)
	}
	doente, err := dominio.NovoDoente(num, ident, ct, "AO")
	if err != nil {
		t.Fatalf("novo doente: %v", err)
	}
	doenteID, err := repoDoentes.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar doente: %v", err)
	}

	const espID = "00000000-0000-4000-8000-000000000001"
	const medID = "00000000-0000-4000-8000-000000000002"
	ep, err := dominio.NovoEpisodio(doenteID, dominio.EpisodioConsulta, espID, medID, time.Now())
	if err != nil {
		t.Fatalf("novo episódio: %v", err)
	}
	epID, err := repoEp.Guardar(ctx, ep)
	if err != nil {
		t.Fatalf("guardar episódio: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM shared.outbox WHERE payload->>'EpisodioID' = $1`, epID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.episodios_clinicos WHERE doente_id=$1`, doenteID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})

	lido, err := repoEp.ObterPorID(ctx, epID)
	if err != nil {
		t.Fatalf("obter episódio: %v", err)
	}
	// Fechar() exige nota clínica completa e pelo menos um diagnóstico CID.
	if err := lido.ActualizarNota(dominio.NovaNotaClinica("Cefaleia", "", "Sem sinais focais", "Enxaqueca", "Analgesia")); err != nil {
		t.Fatalf("actualizar nota: %v", err)
	}
	cid, err := dominio.NovoDiagnosticoCID("G43", true)
	if err != nil {
		t.Fatalf("novo diagnóstico CID: %v", err)
	}
	if err := lido.DefinirDiagnosticosCID([]dominio.DiagnosticoCID{cid}); err != nil {
		t.Fatalf("definir diagnósticos: %v", err)
	}
	return lido
}
