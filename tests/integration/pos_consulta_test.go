//go:build integration

// Teste de integração do consumidor da Recepção para o evento
// clinico.episodio.fechado (ADR-038): prova que a chegada que originou o
// episódio transita EM_CONSULTA→ATENDIDO pela ponte episodio_id, e que a
// operação é idempotente (entrega at-least-once do relay do outbox).
package integration_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	domclinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// chegadaEmConsultaComEpisodio compõe o cenário EM_CONSULTA reutilizando
// criaChegadaTriadaComDoente (TRIADO) e o adaptador real do início da consulta
// (ADR-036), que grava o episodio_id na chegada. Devolve o episodioID
// devolvido por ConsumirEIniciar e o chegadaID.
func chegadaEmConsultaComEpisodio(t *testing.T, pool *pgxpool.Pool, ctx context.Context) (episodioID, chegadaID string) {
	t.Helper()
	doenteID, chegadaID := criaChegadaTriadaComDoente(t, pool, ctx, "00987659LA026", "+244923111777")
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)
	ep, err := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	if err != nil {
		t.Fatalf("construir episódio: %v", err)
	}
	episodioID, err = integ.ConsumirEIniciar(ctx, chegadaID, medInicioConsulta, ep)
	if err != nil {
		t.Fatalf("consumir e iniciar: %v", err)
	}
	return episodioID, chegadaID
}

func TestMarcarChegadaAtendida_TransitaEIdempotente(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	// põe uma chegada EM_CONSULTA com episódio (via o adaptador ADR-036)
	episodioID, chegadaID := chegadaEmConsultaComEpisodio(t, pool, ctx)

	pos := pgrepo.NovaIntegracaoPosConsulta(pool)
	if err := pos.MarcarChegadaAtendida(ctx, episodioID); err != nil {
		t.Fatalf("marcar atendida: %v", err)
	}
	var estado string
	if err := pool.QueryRow(ctx, `SELECT estado FROM recepcao.chegadas WHERE id=$1`, chegadaID).Scan(&estado); err != nil {
		t.Fatalf("ler chegada: %v", err)
	}
	if estado != "ATENDIDO" {
		t.Fatalf("esperava ATENDIDO, obtive %q", estado)
	}
	// idempotência: segunda chamada é no-op sem erro
	if err := pos.MarcarChegadaAtendida(ctx, episodioID); err != nil {
		t.Fatalf("segunda marcação devia ser no-op, obtive %v", err)
	}
}

func TestMarcarChegadaAtendida_SemChegada_NoOp(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	pos := pgrepo.NovaIntegracaoPosConsulta(pool)
	// episódio que não nasceu da fila → nenhuma chegada com este episodio_id
	if err := pos.MarcarChegadaAtendida(ctx, "00000000-0000-4000-8000-0000000000ff"); err != nil {
		t.Fatalf("episódio sem chegada devia ser no-op, obtive %v", err)
	}
}
