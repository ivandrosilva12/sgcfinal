//go:build integration

// Teste e2e do relay de outbox (ADR-038): prova que o fecho real de um
// episódio — nascido do início da consulta (ADR-036) — escreve o evento
// clinico.episodio.fechado na mesma transacção do Guardar, e que o relay,
// ao processar o lote, entrega o evento ao consumidor da Recepção e transita
// a chegada correspondente para ATENDIDO.
package integration_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/outbox"
	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
)

func TestOutbox_E2E_FechoTransitaChegada(t *testing.T) {
	pool, ctx := ligar(t)
	episodioID, chegadaID := chegadaEmConsultaComEpisodio(t, pool, ctx)

	// fecha o episódio pelo repositório (escreve o outbox na mesma tx). O
	// episódio nascido do início da consulta está ABERTO sem nota nem CID —
	// completa-se aqui antes do fecho, sobre a MESMA instância relida (Fechar
	// e Guardar têm de correr sobre a mesma instância para não perder o
	// evento de domínio pendente).
	repo := pgrepo.NovoRepositorioEpisodios(pool)
	ep, err := repo.ObterPorID(ctx, episodioID)
	if err != nil {
		t.Fatalf("obter episódio: %v", err)
	}
	if err := ep.ActualizarNota(dominio.NovaNotaClinica("Tosse", "", "Auscultação limpa", "Bronquite aguda", "Broncodilatador")); err != nil {
		t.Fatalf("actualizar nota: %v", err)
	}
	cid, err := dominio.NovoDiagnosticoCID("J11", true)
	if err != nil {
		t.Fatalf("novo diagnóstico CID: %v", err)
	}
	if err := ep.DefinirDiagnosticosCID([]dominio.DiagnosticoCID{cid}); err != nil {
		t.Fatalf("definir diagnósticos: %v", err)
	}
	if err := ep.Fechar(medInicioConsulta, agora()); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if _, err := repo.Guardar(ctx, ep); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM shared.outbox WHERE payload->>'EpisodioID' = $1`, episodioID)
	})

	// relay entrega ao consumidor (Recepção): fila do outbox → handler →
	// transição da chegada.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	relay := outbox.NovoRelay(pool, 100, outbox.ObservadorNulo{}, log)
	pos := pgrepo.NovaIntegracaoPosConsulta(pool)
	relay.Registar("clinico.episodio.fechado", pos.HandlerEpisodioFechado)
	if _, err := relay.ProcessarLote(ctx); err != nil {
		t.Fatalf("processar lote: %v", err)
	}

	var estado string
	if err := pool.QueryRow(ctx, `SELECT estado FROM recepcao.chegadas WHERE id=$1`, chegadaID).Scan(&estado); err != nil {
		t.Fatalf("ler chegada: %v", err)
	}
	if estado != "ATENDIDO" {
		t.Fatalf("esperava ATENDIDO após o relay, obtive %q", estado)
	}
}
