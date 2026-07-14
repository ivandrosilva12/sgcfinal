//go:build integration

// Teste de integração da cirurgia ambulatória contra a BD real. Segue o padrão
// de episodios_test.go: SKIP (nunca FAIL) quando DATABASE_URL não está definido.
package integration_test

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	appclinico "github.com/ivandrosilva12/sgcfinal/internal/application/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

func TestCirurgia_CicloEProibicoes(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	repoEp := pgrepo.NovoRepositorioEpisodios(pool)
	repoCons := pgrepo.NovoRepositorioConsentimentos(pool)
	repoProc := pgrepo.NovoRepositorioProcedimentos(pool)
	repoCat := pgrepo.NovoRepositorioCatalogoProcedimentos(pool)

	// O catálogo tem PRC001, vindo do seed da migração 0005.
	cat, err := repoCat.ObterPorCodigo(ctx, "PRC001")
	if err != nil || cat.Codigo != "PRC001" {
		t.Fatalf("catálogo PRC001 devia existir do seed: %v", err)
	}

	// Doente + episódio de cirurgia ambulatória via agregados de domínio.
	nasc := time.Date(1985, 5, 5, 0, 0, 0, 0, time.UTC)
	bi := "00345678LA044"
	num, err := repoDoentes.ProximoNumeroProcesso(ctx, 2026)
	if err != nil {
		t.Fatalf("próximo número: %v", err)
	}
	ident, _ := dominio.NovaIdentificacao("Teste Cirurgia", nasc, dominio.SexoFeminino, &bi, nil, nil)
	ct, _ := dominio.NovosContactos("+244923111111", nil, nil)
	doente, _ := dominio.NovoDoente(num, ident, ct, "AO")
	doenteID, err := repoDoentes.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar doente: %v", err)
	}

	const espID = "00000000-0000-4000-8000-0000000000e1"
	const medID = "00000000-0000-4000-8000-0000000000e2"
	ep, _ := dominio.NovoEpisodio(doenteID, dominio.EpisodioCirurgiaAmbulatoria, espID, medID, time.Now())
	episodioID, err := repoEp.Guardar(ctx, ep)
	if err != nil {
		t.Fatalf("guardar episódio: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.procedimentos_cirurgicos WHERE episodio_id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.consentimentos WHERE doente_id=$1`, doenteID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.episodios_clinicos WHERE id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})

	cons, err := dominio.NovoConsentimento(doenteID, dominio.FinalidadeCirurgia, true, "s3://c.pdf",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("consentimento inválido: %v", err)
	}
	consID, err := repoCons.Guardar(ctx, cons)
	if err != nil {
		t.Fatalf("guardar consentimento: %v", err)
	}
	consPersistido, err := repoCons.ObterPorID(ctx, consID)
	if err != nil {
		t.Fatalf("obter consentimento: %v", err)
	}

	proc, err := dominio.NovoProcedimento(dominio.DadosNovoProcedimento{
		EpisodioID: episodioID, Codigo: "PRC001", Descricao: "Sutura de ferida superficial",
		CirurgiaoID: "00000000-0000-4000-8000-0000000000c1", Anestesia: dominio.AnestesiaLocal,
		AnestesistaID: "00000000-0000-4000-8000-0000000000a1",
	}, consPersistido)
	if err != nil {
		t.Fatalf("novo procedimento: %v", err)
	}
	procID, err := repoProc.Guardar(ctx, proc)
	if err != nil {
		t.Fatalf("guardar procedimento: %v", err)
	}

	// Ciclo: AGENDADO → EM_CURSO → CONCLUIDO, persistido e coerente com as
	// CHECKs estado↔timestamps de 0006 (é o UPDATE de transição do
	// procedimentos_repo que aqui se exercita a sério).
	p, err := repoProc.ObterPorID(ctx, procID)
	if err != nil {
		t.Fatalf("obter procedimento: %v", err)
	}
	if err := p.Iniciar(time.Now()); err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if _, err := repoProc.Guardar(ctx, p); err != nil {
		t.Fatalf("persistir início: %v", err)
	}
	p2, err := repoProc.ObterPorID(ctx, procID)
	if err != nil {
		t.Fatalf("obter procedimento após início: %v", err)
	}
	if err := p2.Concluir(time.Now().Add(time.Hour), "sem complicações", ""); err != nil {
		t.Fatalf("concluir: %v", err)
	}
	if _, err := repoProc.Guardar(ctx, p2); err != nil {
		t.Fatalf("persistir conclusão: %v", err)
	}
	final, err := repoProc.ObterPorID(ctx, procID)
	if err != nil {
		t.Fatalf("obter procedimento final: %v", err)
	}
	if final.Estado() != dominio.ProcConcluido {
		t.Fatalf("esperado CONCLUIDO, veio %s", final.Estado())
	}

	// Revalidação da invariante-estrela no início, contra a BD real: agenda-se um
	// segundo procedimento com um consentimento CIRURGIA vigente e persistido; o
	// doente revoga-o (a revogação NÃO é bloqueada — é um direito LPDP) e a
	// revogação é persistida. Iniciar tem então de falhar com RegraNegocio, com os
	// repositórios reais a servirem o caso de uso. Antes da correcção, o
	// CasoIniciarProcedimento nem sequer via o consentimento: a cirurgia iniciava-se
	// e concluía-se sobre um consentimento revogado.
	cons2, err := dominio.NovoConsentimento(doenteID, dominio.FinalidadeCirurgia, true, "s3://c2.pdf",
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("segundo consentimento inválido: %v", err)
	}
	cons2ID, err := repoCons.Guardar(ctx, cons2)
	if err != nil {
		t.Fatalf("guardar segundo consentimento: %v", err)
	}
	cons2Persistido, err := repoCons.ObterPorID(ctx, cons2ID)
	if err != nil {
		t.Fatalf("obter segundo consentimento: %v", err)
	}
	proc2, err := dominio.NovoProcedimento(dominio.DadosNovoProcedimento{
		EpisodioID: episodioID, Codigo: "PRC001", Descricao: "Excisão de lesão cutânea",
		CirurgiaoID: "00000000-0000-4000-8000-0000000000c1", Anestesia: dominio.AnestesiaNenhuma,
	}, cons2Persistido)
	if err != nil {
		t.Fatalf("novo procedimento (2): %v", err)
	}
	proc2ID, err := repoProc.Guardar(ctx, proc2)
	if err != nil {
		t.Fatalf("guardar procedimento (2): %v", err)
	}

	// Revogação persistida na BD.
	if err := cons2Persistido.Revogar(time.Now()); err != nil {
		t.Fatalf("revogar consentimento: %v", err)
	}
	if _, err := repoCons.Guardar(ctx, cons2Persistido); err != nil {
		t.Fatalf("persistir revogação: %v", err)
	}

	aud := &auditorEspiao{}
	iniciar := appclinico.NovoCasoIniciarProcedimento(repoProc, repoEp, repoCons, aud)
	_, err = iniciar.Executar(ctx, "00000000-0000-4000-8000-0000000000c1", proc2ID)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaRegraNegocio {
		t.Fatalf("iniciar com consentimento revogado (lido da BD) devia falhar com RegraNegocio, veio %v", err)
	}
	if aud.chamadas != 0 {
		t.Fatalf("um início recusado não devia auditar, veio %d registos", aud.chamadas)
	}
	// A linha continua AGENDADO na BD — nada foi escrito.
	depois, err := repoProc.ObterPorID(ctx, proc2ID)
	if err != nil {
		t.Fatalf("obter procedimento (2): %v", err)
	}
	if depois.Estado() != dominio.ProcAgendado {
		t.Fatalf("o procedimento devia continuar AGENDADO na BD, veio %s", depois.Estado())
	}
}

// auditorEspiao conta os registos de auditoria sem escrever na BD: o trilho é
// append-only (trigger bloqueia DELETE), e nestes testes o que interessa provar é
// que um caminho recusado não audita.
type auditorEspiao struct{ chamadas int }

func (a *auditorEspiao) Registar(_ context.Context, _ auditoria.Registo) error {
	a.chamadas++
	return nil
}

// TestProcedimento_TransicaoConcorrente_SegundoGuardarPerdeAcorrida prova, contra
// a BD real, a guarda compare-and-set do UPDATE de transição. Dois agregados são
// lidos do MESMO estado EM_CURSO (é o que acontece com um duplo-clique ou com dois
// postos do bloco operatório): um conclui, o outro cancela. Ambos passam as guardas
// do domínio (as duas transições são legais a partir de EM_CURSO). Antes da
// correcção, o UPDATE era um overwrite absoluto sem condição de estado e ambos
// escreviam — deixando o audit log imutável com `concluido` E `cancelado` para o
// mesmo procedimento, e o motivo do cancelamento nas observações de uma linha
// CONCLUIDO. Agora o segundo Guardar tem de falhar com Conflito e não tocar na linha.
func TestProcedimento_TransicaoConcorrente_SegundoGuardarPerdeAcorrida(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repoCons := pgrepo.NovoRepositorioConsentimentos(pool)
	repoProc := pgrepo.NovoRepositorioProcedimentos(pool)

	doenteID, episodioID := fixturaCirurgia(t, pool, ctx, "00456789LA045", "Teste Concorrência")

	cons, err := dominio.NovoConsentimento(doenteID, dominio.FinalidadeCirurgia, true, "s3://c.pdf", time.Now())
	if err != nil {
		t.Fatalf("consentimento inválido: %v", err)
	}
	consID, err := repoCons.Guardar(ctx, cons)
	if err != nil {
		t.Fatalf("guardar consentimento: %v", err)
	}
	consPersistido, err := repoCons.ObterPorID(ctx, consID)
	if err != nil {
		t.Fatalf("obter consentimento: %v", err)
	}
	proc, err := dominio.NovoProcedimento(dominio.DadosNovoProcedimento{
		EpisodioID: episodioID, Codigo: "PRC001", Descricao: "Sutura",
		CirurgiaoID: "00000000-0000-4000-8000-0000000000c1", Anestesia: dominio.AnestesiaNenhuma,
		Observacoes: "doente anticoagulado — varfarina suspensa a 5/7",
	}, consPersistido)
	if err != nil {
		t.Fatalf("novo procedimento: %v", err)
	}
	procID, err := repoProc.Guardar(ctx, proc)
	if err != nil {
		t.Fatalf("guardar procedimento: %v", err)
	}
	emCurso, err := repoProc.ObterPorID(ctx, procID)
	if err != nil {
		t.Fatalf("obter procedimento: %v", err)
	}
	inicio := time.Now()
	if err := emCurso.Iniciar(inicio); err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if _, err := repoProc.Guardar(ctx, emCurso); err != nil {
		t.Fatalf("persistir início: %v", err)
	}

	// Duas leituras do mesmo estado EM_CURSO — as duas transições concorrentes.
	a, err := repoProc.ObterPorID(ctx, procID)
	if err != nil {
		t.Fatalf("leitura A: %v", err)
	}
	b, err := repoProc.ObterPorID(ctx, procID)
	if err != nil {
		t.Fatalf("leitura B: %v", err)
	}
	if err := a.Concluir(inicio.Add(time.Hour), "sem complicações", ""); err != nil {
		t.Fatalf("concluir (A): %v", err)
	}
	if err := b.Cancelar(inicio.Add(time.Hour), "instabilidade hemodinâmica"); err != nil {
		t.Fatalf("cancelar (B): %v", err)
	}
	// O domínio deixa passar as duas — é a guarda do repositório que decide.
	if _, err := repoProc.Guardar(ctx, a); err != nil {
		t.Fatalf("o primeiro Guardar devia vencer a corrida: %v", err)
	}
	_, err = repoProc.Guardar(ctx, b)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("o segundo Guardar devia falhar com Conflito (a linha já não está EM_CURSO), veio %v", err)
	}

	// A linha ficou CONCLUIDO e sem o motivo do cancelamento nas observações.
	final, err := repoProc.ObterPorID(ctx, procID)
	if err != nil {
		t.Fatalf("obter procedimento final: %v", err)
	}
	s := final.Snapshot()
	if s.Estado != dominio.ProcConcluido {
		t.Fatalf("esperado CONCLUIDO (só a transição vencedora escreveu), veio %s", s.Estado)
	}
	if strings.Contains(s.Observacoes, "instabilidade hemodinâmica") {
		t.Fatalf("a transição perdedora não devia ter escrito o motivo nas observações, veio %q", s.Observacoes)
	}
	if !strings.Contains(s.Observacoes, "varfarina suspensa a 5/7") {
		t.Fatalf("a nota pré-operatória devia manter-se na linha, veio %q", s.Observacoes)
	}
}

// TestConsentimento_RevogacaoConcorrente_SegundaFalhaComConflito prova a guarda
// `AND revogado_em IS NULL` do UPDATE de revogação: dois agregados lidos do mesmo
// consentimento vigente passam ambos a guarda do domínio, mas só um escreve. Sem a
// guarda, ambos escreviam e o trilho imutável ficava com dois
// `clinico.consentimento.revogado` para o mesmo consentimento.
func TestConsentimento_RevogacaoConcorrente_SegundaFalhaComConflito(t *testing.T) {
	pool, ctx := ligar(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	repoCons := pgrepo.NovoRepositorioConsentimentos(pool)
	doenteID, _ := fixturaCirurgia(t, pool, ctx, "00567890LA046", "Teste Revogação Concorrente")

	cons, err := dominio.NovoConsentimento(doenteID, dominio.FinalidadeTratamento, true, "", time.Now())
	if err != nil {
		t.Fatalf("consentimento inválido: %v", err)
	}
	consID, err := repoCons.Guardar(ctx, cons)
	if err != nil {
		t.Fatalf("guardar consentimento: %v", err)
	}

	a, err := repoCons.ObterPorID(ctx, consID)
	if err != nil {
		t.Fatalf("leitura A: %v", err)
	}
	b, err := repoCons.ObterPorID(ctx, consID)
	if err != nil {
		t.Fatalf("leitura B: %v", err)
	}
	if err := a.Revogar(time.Now()); err != nil {
		t.Fatalf("revogar (A): %v", err)
	}
	if err := b.Revogar(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("revogar (B): %v", err)
	}
	if _, err := repoCons.Guardar(ctx, a); err != nil {
		t.Fatalf("a primeira revogação devia vencer: %v", err)
	}
	_, err = repoCons.Guardar(ctx, b)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("a segunda revogação devia falhar com Conflito, veio %v", err)
	}
}

// fixturaCirurgia cria um doente e um episódio de cirurgia ambulatória ABERTO na
// BD real, com limpeza registada. Devolve os ids.
func fixturaCirurgia(t *testing.T, pool *pgxpool.Pool, ctx context.Context, bi, nome string) (string, string) {
	t.Helper()
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	repoEp := pgrepo.NovoRepositorioEpisodios(pool)

	num, err := repoDoentes.ProximoNumeroProcesso(ctx, 2026)
	if err != nil {
		t.Fatalf("próximo número: %v", err)
	}
	ident, _ := dominio.NovaIdentificacao(nome, time.Date(1985, 5, 5, 0, 0, 0, 0, time.UTC),
		dominio.SexoFeminino, &bi, nil, nil)
	ct, _ := dominio.NovosContactos("+244923111112", nil, nil)
	doente, _ := dominio.NovoDoente(num, ident, ct, "AO")
	doenteID, err := repoDoentes.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar doente: %v", err)
	}
	const espID = "00000000-0000-4000-8000-0000000000e1"
	const medID = "00000000-0000-4000-8000-0000000000e2"
	ep, _ := dominio.NovoEpisodio(doenteID, dominio.EpisodioCirurgiaAmbulatoria, espID, medID, time.Now())
	episodioID, err := repoEp.Guardar(ctx, ep)
	if err != nil {
		t.Fatalf("guardar episódio: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.procedimentos_cirurgicos WHERE episodio_id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.consentimentos WHERE doente_id=$1`, doenteID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.episodios_clinicos WHERE id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})
	return doenteID, episodioID
}
