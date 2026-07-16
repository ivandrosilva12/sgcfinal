//go:build integration

// Teste de integração do adaptador de integração Recepção→Clínico (início da
// consulta, ADR-036) contra a BD real. Prova a atomicidade da transacção única
// (INSERT do episódio + CAS da chegada), a recusa do médico errado e do duplo
// início, e as restrições de defesa em profundidade da migração recepcao/0004.
// SKIPa (nunca FAIL) quando DATABASE_URL não está definido.
package integration_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	domclinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	domrecepcao "github.com/ivandrosilva12/sgcfinal/internal/domain/recepcao"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

const (
	espInicioConsulta = "00000000-0000-4000-8000-000000000031"
	medInicioConsulta = "00000000-0000-4000-8000-000000000032"
	enfInicioConsulta = "00000000-0000-4000-8000-000000000033"
)

// criaChegadaTriadaComDoente cria um doente ACTIVO (FK do episódio) e uma chegada
// walk-in TRIADO atribuída a medInicioConsulta (via triagem transaccional, o único
// caminho real para TRIADO). Regista a limpeza.
func criaChegadaTriadaComDoente(t *testing.T, pool *pgxpool.Pool, ctx context.Context, bi, telefone string) (doenteID, chegadaID string) {
	t.Helper()
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	nasc := time.Date(1988, 2, 10, 0, 0, 0, 0, time.UTC)
	num, _ := repoDoentes.ProximoNumeroProcesso(ctx, 2026)
	ident, _ := domclinico.NovaIdentificacao("Rui Consulta", nasc, domclinico.SexoMasculino, &bi, nil, nil)
	ct, _ := domclinico.NovosContactos(telefone, nil, nil)
	doente, _ := domclinico.NovoDoente(num, ident, ct, "AO")
	var err error
	doenteID, err = repoDoentes.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar doente: %v", err)
	}

	chegRepo := pgrepo.NovoRepositorioChegadas(pool)
	triRepo := pgrepo.NovoRepositorioTriagens(pool)
	c, err := domrecepcao.NovaChegadaWalkIn(doenteID, espInicioConsulta, time.Now())
	if err != nil {
		t.Fatalf("construir chegada: %v", err)
	}
	if err := c.Chamar(time.Now()); err != nil {
		t.Fatalf("chamar (domínio): %v", err)
	}
	chegadaID, err = chegRepo.Guardar(ctx, c)
	if err != nil {
		t.Fatalf("guardar chegada: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.episodios_clinicos WHERE doente_id=$1`, doenteID)
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.triagens WHERE chegada_id=$1`, chegadaID)
		_, _ = pool.Exec(ctx, `DELETE FROM recepcao.chegadas WHERE id=$1`, chegadaID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})

	obt, err := chegRepo.ObterPorID(ctx, chegadaID)
	if err != nil {
		t.Fatalf("obter chegada: %v", err)
	}
	if err := obt.RegistarTriada(medInicioConsulta, time.Now()); err != nil {
		t.Fatalf("registar triada (domínio): %v", err)
	}
	tr, err := domrecepcao.NovaTriagem(chegadaID, enfInicioConsulta, domrecepcao.ManVerde, domrecepcao.SinaisVitais{}, "", time.Now())
	if err != nil {
		t.Fatalf("construir triagem: %v", err)
	}
	if _, err := triRepo.RegistarTriagem(ctx, tr, obt); err != nil {
		t.Fatalf("registar triagem: %v", err)
	}
	return doenteID, chegadaID
}

func contaEpisodios(t *testing.T, pool *pgxpool.Pool, ctx context.Context, doenteID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM clinico.episodios_clinicos WHERE doente_id=$1`, doenteID).Scan(&n); err != nil {
		t.Fatalf("contar episódios: %v", err)
	}
	return n
}

func TestIntegracaoInicioConsulta_FluxoFeliz(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	doenteID, chegadaID := criaChegadaTriadaComDoente(t, pool, ctx, "00987654LA021", "+244923111222")
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)

	// o leitor devolve o retrato da chegada TRIADO
	ct, err := integ.ChegadaTriada(ctx, chegadaID)
	if err != nil || ct.DoenteID != doenteID || ct.MedicoID != medInicioConsulta || ct.EspecialidadeID != espInicioConsulta {
		t.Fatalf("chegada triada: %v (%+v)", err, ct)
	}

	// consumir e iniciar: episódio criado + chegada EM_CONSULTA, atomicamente
	ep, err := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	if err != nil {
		t.Fatalf("construir episódio: %v", err)
	}
	epID, err := integ.ConsumirEIniciar(ctx, chegadaID, medInicioConsulta, ep)
	if err != nil {
		t.Fatalf("consumir e iniciar: %v", err)
	}
	lido, err := pgrepo.NovoRepositorioEpisodios(pool).ObterPorID(ctx, epID)
	if err != nil || lido.Estado() != domclinico.EstadoEpisodioAberto {
		t.Fatalf("episódio não ficou ABERTO: %v", err)
	}
	rec, err := pgrepo.NovoRepositorioChegadas(pool).ObterPorID(ctx, chegadaID)
	if err != nil || rec.Estado() != domrecepcao.ChegEmConsulta || rec.EpisodioID() != epID {
		t.Fatalf("chegada mal consumida: %v estado=%s episodio=%q", err, rec.Estado(), rec.EpisodioID())
	}

	// o leitor deixa de devolver a chegada (saiu da fila clínica)
	if _, err := integ.ChegadaTriada(ctx, chegadaID); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("chegada consumida devia dar NaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

func TestIntegracaoInicioConsulta_ChegadaInexistente_NaoEncontrado(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)
	if _, err := integ.ChegadaTriada(ctx, "00000000-0000-4000-8000-00000000dead"); erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("esperava NaoEncontrado, veio %v", erros.CategoriaDe(err))
	}
}

func TestIntegracaoInicioConsulta_MedicoErrado_ProibidoENadaMuda(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	doenteID, chegadaID := criaChegadaTriadaComDoente(t, pool, ctx, "00987655LA022", "+244923111333")
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)

	outro := "00000000-0000-4000-8000-000000000099"
	ep, _ := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, outro, time.Now())
	if _, err := integ.ConsumirEIniciar(ctx, chegadaID, outro, ep); erros.CategoriaDe(err) != erros.CategoriaProibido {
		t.Fatalf("médico errado devia dar Proibido, veio %v", erros.CategoriaDe(err))
	}
	// atomicidade: nem episódio criado, nem chegada consumida
	if n := contaEpisodios(t, pool, ctx, doenteID); n != 0 {
		t.Fatalf("não devia existir episódio, existem %d", n)
	}
	rec, _ := pgrepo.NovoRepositorioChegadas(pool).ObterPorID(ctx, chegadaID)
	if rec.Estado() != domrecepcao.ChegTriado {
		t.Fatalf("a chegada devia continuar TRIADO, veio %s", rec.Estado())
	}
}

func TestIntegracaoInicioConsulta_DuploInicio_Conflito(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	doenteID, chegadaID := criaChegadaTriadaComDoente(t, pool, ctx, "00987656LA023", "+244923111444")
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)

	ep1, _ := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	if _, err := integ.ConsumirEIniciar(ctx, chegadaID, medInicioConsulta, ep1); err != nil {
		t.Fatalf("primeiro início: %v", err)
	}
	ep2, _ := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	if _, err := integ.ConsumirEIniciar(ctx, chegadaID, medInicioConsulta, ep2); erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("duplo início devia dar Conflito, veio %v", erros.CategoriaDe(err))
	}
	// atomicidade: só o primeiro episódio existe
	if n := contaEpisodios(t, pool, ctx, doenteID); n != 1 {
		t.Fatalf("devia existir exactamente 1 episódio, existem %d", n)
	}
}

func TestIntegracaoInicioConsulta_RestricoesMigracao0004(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	doenteID, chegadaA := criaChegadaTriadaComDoente(t, pool, ctx, "00987657LA024", "+244923111555")
	_, chegadaB := criaChegadaTriadaComDoente(t, pool, ctx, "00987658LA025", "+244923111666")
	integ := pgrepo.NovaIntegracaoInicioConsulta(pool)

	// CHECK: EM_CONSULTA sem episodio_id → 23514
	var pgErr *pgconn.PgError
	_, err := pool.Exec(ctx, `UPDATE recepcao.chegadas SET estado='EM_CONSULTA' WHERE id=$1`, chegadaB)
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("EM_CONSULTA sem episódio devia violar o CHECK (23514), veio %v", err)
	}

	// UNIQUE parcial: duas chegadas com o mesmo episodio_id → 23505
	ep, _ := domclinico.NovoEpisodio(doenteID, domclinico.EpisodioConsulta, espInicioConsulta, medInicioConsulta, time.Now())
	epID, err := integ.ConsumirEIniciar(ctx, chegadaA, medInicioConsulta, ep)
	if err != nil {
		t.Fatalf("iniciar consulta da chegada A: %v", err)
	}
	_, err = pool.Exec(ctx,
		`UPDATE recepcao.chegadas SET estado='EM_CONSULTA', episodio_id=$2 WHERE id=$1`, chegadaB, epID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("episodio_id repetido devia violar o UNIQUE (23505), veio %v", err)
	}
}
