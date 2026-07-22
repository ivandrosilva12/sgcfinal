package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// chaveBloqueioMigracoes é a chave do advisory lock que serializa a aplicação
// das migrations. O valor é arbitrário mas FIXO — só tem de ser o mesmo em
// todos os processos que migram esta base (ADR-043).
const chaveBloqueioMigracoes int64 = 5_043_2026

// AplicarMigracoes executa, de forma forward-only e idempotente, todas as
// migrations SQL presentes em fsys, organizadas por bounded context
// (subdirectório). Cada ficheiro é aplicado uma única vez, dentro de uma
// transacção própria, e registado em public.schema_migrations. A ordem é
// determinística: bounded contexts por ordem alfabética, ficheiros por ordem
// numérica do nome.
//
// A execução é serializada por um advisory lock ao nível da sessão: entre o
// `jaAplicada` e o `aplicarUma` há uma janela em que dois migradores
// concorrentes vêem ambos a migration por aplicar e tentam ambos aplicá-la —
// um comete, o outro rebenta (chave duplicada em schema_migrations, ou o
// próprio DDL a colidir). Não é hipotético: passou a acontecer quando o passo
// de integração da CI passou a correr, no mesmo `go test`, dois pacotes que
// migram. Quem chega em segundo espera e, ao entrar, encontra tudo já aplicado
// e não faz nada.
//
// Os dois cenários de concorrência REAIS são esses dois: os pacotes de teste, e
// um pipeline que invoque `api migrate` mais do que uma vez em simultâneo. NÃO
// inclui "duas réplicas da API a arrancar ao mesmo tempo": `ExecutarServidor`
// não migra — `AplicarMigracoes` é chamada apenas de `ExecutarMigracoes` e dos
// testes, e o runbook §4 faz da migração um passo separado. A afirmação
// anterior reivindicava um risco que não existe (ADR-043, N2 da re-revisão).
func AplicarMigracoes(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, logger *slog.Logger) error {
	// O lock é de SESSÃO, pelo que tem de ser tomado e largado na MESMA
	// ligação, retirada do pool e segurada até ao fim. O corpo continua a usar
	// o pool, pelo que uma ligação NÃO chega: com pool_max_conns=1 a primeira
	// consulta do corpo ficaria à espera da ligação que esta função segura, e
	// o resultado é um bloqueio permanente — medido, com prazo de 4 s no
	// contexto: "criar schema_migrations: context deadline exceeded" aos
	// 4,001 s. Em produção (`api migrate`, contexto sem prazo) seria uma
	// paragem silenciosa e para sempre. pgxpool.ParseConfig honra
	// `pool_max_conns` na própria connection string, pelo que o caso é
	// alcançável só por configuração; recusar à cabeça, com mensagem que diz
	// porquê, é preferível a pendurar (ADR-043, N3 da re-revisão).
	if max := pool.Config().MaxConns; max < 2 {
		return fmt.Errorf("o pool de migração tem MaxConns=%d: são precisas pelo menos 2 "+
			"ligações, porque uma fica segura com o bloqueio de migrações e as restantes "+
			"consultas usam o pool — com uma só, a migração bloqueia para sempre. Corrija "+
			"`pool_max_conns` na DATABASE_MIGRATION_URL (ADR-043)", max)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("obter ligação para o bloqueio de migrações: %w", err)
	}
	defer conn.Release()

	// Tentar primeiro sem bloquear, para poder DIZER que se vai esperar: o
	// contexto de ExecutarMigracoes não tem prazo e pg_advisory_lock espera
	// indefinidamente, pelo que o operador via um `api migrate` parado sem uma
	// linha de log, a meio de uma janela de deploy (ADR-043, N4 da re-revisão).
	// O comportamento é o mesmo; muda só a legibilidade.
	var obtido bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, chaveBloqueioMigracoes).Scan(&obtido); err != nil {
		return fmt.Errorf("tentar o bloqueio de migrações: %w", err)
	}
	if !obtido {
		if logger != nil {
			logger.Info("à espera do bloqueio de migrações",
				"motivo", "outro migrador está a aplicar migrations nesta base de dados")
		}
		if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, chaveBloqueioMigracoes); err != nil {
			return fmt.Errorf("obter o bloqueio de migrações: %w", err)
		}
	}
	defer func() {
		if _, err := conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, chaveBloqueioMigracoes); err != nil && logger != nil {
			// Largar o lock a falhar não é fatal: ele cai sozinho quando a
			// ligação for fechada. Fica registado para não desaparecer.
			logger.Warn("largar o bloqueio de migrações falhou", "erro", err)
		}
	}()

	if err := garantirTabelaControlo(ctx, pool); err != nil {
		return err
	}

	bcs, err := boundedContexts(fsys)
	if err != nil {
		return err
	}

	total := 0
	for _, bc := range bcs {
		ficheiros, err := ficheirosSQL(fsys, bc)
		if err != nil {
			return err
		}
		for _, fich := range ficheiros {
			versao := strings.TrimSuffix(fich, ".sql")
			aplicada, err := jaAplicada(ctx, pool, bc, versao)
			if err != nil {
				return err
			}
			if aplicada {
				continue
			}
			conteudo, err := fs.ReadFile(fsys, path.Join(bc, fich))
			if err != nil {
				return fmt.Errorf("ler migration %s/%s: %w", bc, fich, err)
			}
			if err := aplicarUma(ctx, pool, bc, versao, string(conteudo)); err != nil {
				return err
			}
			total++
			if logger != nil {
				logger.Info("migration aplicada", "bounded_context", bc, "versao", versao)
			}
		}
	}

	if logger != nil {
		logger.Info("migrations concluídas", "aplicadas_agora", total)
	}
	return nil
}

func garantirTabelaControlo(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    bounded_context text        NOT NULL,
    versao          text        NOT NULL,
    aplicada_em     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (bounded_context, versao)
);`
	if _, err := pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("criar schema_migrations: %w", err)
	}
	return nil
}

func boundedContexts(fsys fs.FS) ([]string, error) {
	entradas, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("listar bounded contexts: %w", err)
	}
	var bcs []string
	for _, e := range entradas {
		if e.IsDir() {
			bcs = append(bcs, e.Name())
		}
	}
	sort.Strings(bcs)
	return bcs, nil
}

func ficheirosSQL(fsys fs.FS, bc string) ([]string, error) {
	entradas, err := fs.ReadDir(fsys, bc)
	if err != nil {
		return nil, fmt.Errorf("listar migrations de %s: %w", bc, err)
	}
	var ficheiros []string
	for _, e := range entradas {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			ficheiros = append(ficheiros, e.Name())
		}
	}
	sort.Strings(ficheiros)
	return ficheiros, nil
}

func jaAplicada(ctx context.Context, pool *pgxpool.Pool, bc, versao string) (bool, error) {
	var existe bool
	const q = `SELECT EXISTS(
        SELECT 1 FROM public.schema_migrations WHERE bounded_context = $1 AND versao = $2
    )`
	if err := pool.QueryRow(ctx, q, bc, versao).Scan(&existe); err != nil {
		return false, fmt.Errorf("verificar migration %s/%s: %w", bc, versao, err)
	}
	return existe, nil
}

func aplicarUma(ctx context.Context, pool *pgxpool.Pool, bc, versao, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciar transacção para %s/%s: %w", bc, versao, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback após commit é no-op

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("aplicar migration %s/%s: %w", bc, versao, err)
	}
	const ins = `INSERT INTO public.schema_migrations (bounded_context, versao) VALUES ($1, $2)`
	if _, err := tx.Exec(ctx, ins, bc, versao); err != nil {
		return fmt.Errorf("registar migration %s/%s: %w", bc, versao, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar migration %s/%s: %w", bc, versao, err)
	}
	return nil
}
