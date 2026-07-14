//go:build integration

// Teste de integração do BC Laboratório contra a BD real. Segue o padrão de
// cirurgia_test.go: SKIP (nunca FAIL) quando DATABASE_URL não está definido.
//
// Os repositórios pgx do Laboratório ficam deliberadamente fora do gate de
// cobertura unitário (ADR-031) — é este ficheiro que os cobre, provando seis
// factos que só a base de dados real pode confirmar: a fila (ListarFila com
// nil), a regra de visibilidade fail-closed (ListarPorEpisodio), a atomicidade
// da emissão, a guarda compare-and-set com 409/404 distinguíveis, as CHECK de
// coerência da migração 0002 em cada transição, e o round-trip jsonb do
// catálogo.
package integration_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivandrosilva12/sgcfinal/internal/adapters/pgrepo"
	aplaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	clinico "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/platform/db"
	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// Ids fixos de pessoal (o BC Laboratório não valida o pessoal — é do Identidade).
const (
	medicoLabID  = "00000000-0000-4000-8000-0000000000f1"
	tecnicoLabID = "00000000-0000-4000-8000-0000000000f2"
	outroTecnico = "00000000-0000-4000-8000-0000000000f3"
)

// migrarLaboratorio aplica as migrações forward-only (idempotente) antes de cada
// teste — ligar(t) só liga o pool, não aplica esquema (ver cirurgia_test.go).
func migrarLaboratorio(t *testing.T, pool *pgxpool.Pool, ctx context.Context) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.AplicarMigracoes(ctx, pool, migrations.FS, logger); err != nil {
		t.Fatalf("migrations: %v", err)
	}
}

// fixturaLaboratorio cria um doente e um episódio de CONSULTA ABERTO na BD real,
// com limpeza registada (filhos antes de pais: resultados → itens → requisições
// → episódio → doente). Modelada em fixturaCirurgia (cirurgia_test.go).
func fixturaLaboratorio(t *testing.T, pool *pgxpool.Pool, ctx context.Context, bi, nome string) (string, string) {
	t.Helper()
	repoDoentes := pgrepo.NovoRepositorioDoentes(pool)
	repoEp := pgrepo.NovoRepositorioEpisodios(pool)

	num, err := repoDoentes.ProximoNumeroProcesso(ctx, 2026)
	if err != nil {
		t.Fatalf("próximo número: %v", err)
	}
	ident, _ := clinico.NovaIdentificacao(nome, time.Date(1985, 5, 5, 0, 0, 0, 0, time.UTC),
		clinico.SexoFeminino, &bi, nil, nil)
	ct, _ := clinico.NovosContactos("+244923111113", nil, nil)
	doente, _ := clinico.NovoDoente(num, ident, ct, "AO")
	doenteID, err := repoDoentes.Guardar(ctx, doente)
	if err != nil {
		t.Fatalf("guardar doente: %v", err)
	}
	const espID = "00000000-0000-4000-8000-0000000000e1"
	ep, _ := clinico.NovoEpisodio(doenteID, clinico.EpisodioConsulta, espID, medicoLabID, time.Now())
	episodioID, err := repoEp.Guardar(ctx, ep)
	if err != nil {
		t.Fatalf("guardar episódio: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `
DELETE FROM laboratorio.resultados WHERE requisicao_id IN
    (SELECT id FROM laboratorio.requisicoes WHERE episodio_id=$1)`, episodioID)
		_, _ = pool.Exec(ctx, `
DELETE FROM laboratorio.itens_requisicao WHERE requisicao_id IN
    (SELECT id FROM laboratorio.requisicoes WHERE episodio_id=$1)`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM laboratorio.requisicoes WHERE episodio_id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.episodios_clinicos WHERE id=$1`, episodioID)
		_, _ = pool.Exec(ctx, `DELETE FROM clinico.doentes WHERE id=$1`, doenteID)
	})
	return doenteID, episodioID
}

// TestLaboratorio_CicloRequisicaoAtePreliminar cobre o ciclo completo contra
// Postgres: emissão transaccional (requisição + itens + resultados PENDENTE),
// a Prova 1 (ListarFila(ctx, nil) devolve a fila inteira, não '{}' em silêncio),
// colheita, submissão do preliminar, e a Prova 2 (a regra de visibilidade —
// PROCESSADA nunca aparece na leitura clínica, e uma lista de estados vazia é
// fail-closed: zero linhas, não todas).
func TestLaboratorio_CicloRequisicaoAtePreliminar(t *testing.T) {
	pool, ctx := ligar(t) // salta se DATABASE_URL vazio
	migrarLaboratorio(t, pool, ctx)

	repoAnalises := pgrepo.NovoRepositorioAnalises(pool)
	repoRequisicoes := pgrepo.NovoRepositorioRequisicoes(pool)
	repoResultados := pgrepo.NovoRepositorioResultados(pool)

	// O seed da migration 0001 tem de estar lá.
	hb, err := repoAnalises.ObterPorCodigo(ctx, "HB")
	if err != nil {
		t.Fatalf("o seed do catálogo devia conter HB: %v", err)
	}
	if hb.Unidade() != "g/dL" {
		t.Fatalf("esperava unidade g/dL, veio %q", hb.Unidade())
	}

	doenteID, episodioID := fixturaLaboratorio(t, pool, ctx, "12345678LA123", "Ana Laboratório")

	req, err := dominio.NovaRequisicao(dominio.DadosNovaRequisicao{
		EpisodioID: episodioID, DoenteID: doenteID, MedicoRequisitanteID: medicoLabID,
		Prioridade: dominio.PrioridadeRotina,
		Itens:      []dominio.ItemRequisicao{{CodigoAnalise: "HB"}, {CodigoAnalise: "GLIC"}},
	})
	if err != nil {
		t.Fatalf("construir requisição: %v", err)
	}
	glic, err := repoAnalises.ObterPorCodigo(ctx, "GLIC")
	if err != nil {
		t.Fatalf("obter GLIC: %v", err)
	}
	// "por-atribuir": NovoResultado exige uma requisição não vazia, mas o id só
	// existe depois do INSERT. O repositório ignora este valor e usa o id real
	// da requisição que acabou de inserir, dentro da mesma transacção.
	resHB, _ := dominio.NovoResultado("por-atribuir", "HB", hb.Unidade())
	resGLIC, _ := dominio.NovoResultado("por-atribuir", "GLIC", glic.Unidade())

	reqID, err := repoRequisicoes.Emitir(ctx, req, []*dominio.Resultado{resHB, resGLIC})
	if err != nil {
		t.Fatalf("emitir requisição: %v", err)
	}

	// PROVA 1: ListarFila(ctx, nil) devolve a fila completa (N > 0), não vazia
	// em silêncio. É o ponto mais frágil do adaptador: se estadosTexto(nil)
	// codificasse '{}' em vez de NULL, `estado = ANY('{}')` seria sempre falso.
	filaCompleta, err := repoResultados.ListarFila(ctx, nil)
	if err != nil {
		t.Fatalf("listar fila (nil): %v", err)
	}
	if len(filaCompleta) == 0 {
		t.Fatal("ListarFila(ctx, nil) devolveu 0 linhas — a fila do laboratório está a vir vazia em silêncio")
	}

	// A emissão criou dois resultados PENDENTE.
	fila, err := repoResultados.ListarFila(ctx, []dominio.EstadoResultado{dominio.ResPendente})
	if err != nil {
		t.Fatalf("listar fila: %v", err)
	}
	var meus []string
	for _, r := range fila {
		if r.RequisicaoID == reqID {
			meus = append(meus, r.ID)
		}
	}
	if len(meus) != 2 {
		t.Fatalf("esperava 2 resultados PENDENTE da requisição, veio %d", len(meus))
	}

	// Colher e submeter o preliminar do primeiro.
	res, err := repoResultados.ObterPorID(ctx, meus[0])
	if err != nil {
		t.Fatalf("obter resultado: %v", err)
	}
	if err := res.ColherAmostra(tecnicoLabID, time.Now()); err != nil {
		t.Fatalf("colher: %v", err)
	}
	if err := repoResultados.Transitar(ctx, res); err != nil {
		t.Fatalf("gravar colheita: %v", err)
	}
	res, _ = repoResultados.ObterPorID(ctx, meus[0])
	if err := res.SubmeterPreliminar(tecnicoLabID, "12.5", "", time.Now()); err != nil {
		t.Fatalf("submeter: %v", err)
	}
	if err := repoResultados.Transitar(ctx, res); err != nil {
		t.Fatalf("gravar submissão: %v", err)
	}

	// PROVA 2 (parte 1): visibilidade — o preliminar NÃO aparece na leitura
	// clínica. Usa a mesma var pública que o caso de uso real consulta
	// (aplaboratorio.EstadosVisiveisAoMedico), não uma cópia manual.
	visiveis, err := repoResultados.ListarPorEpisodio(ctx, episodioID, aplaboratorio.EstadosVisiveisAoMedico)
	if err != nil {
		t.Fatalf("listar resultados visíveis: %v", err)
	}
	if len(visiveis) != 0 {
		t.Fatalf("o preliminar não pode ser visível ao médico, veio %+v", visiveis)
	}
	// Mas a fila do laboratório vê-o.
	processados, err := repoResultados.ListarPorEpisodio(ctx, episodioID,
		[]dominio.EstadoResultado{dominio.ResProcessada})
	if err != nil {
		t.Fatalf("listar processados: %v", err)
	}
	if len(processados) != 1 || processados[0].Valor != "12.5" {
		t.Fatalf("esperava 1 resultado PROCESSADA com valor 12.5, veio %+v", processados)
	}

	// PROVA 2 (parte 2): fail-closed — uma lista de estados vazia/nil devolve
	// ZERO linhas nesta leitura (ao contrário de ListarFila), mesmo havendo
	// resultados no episódio. Se algum dia chegasse vazia por engano (ex.: um
	// bug a montante que esvazie o filtro), o preliminar não pode vazar.
	vazio, err := repoResultados.ListarPorEpisodio(ctx, episodioID, nil)
	if err != nil {
		t.Fatalf("listar por episódio (estados nil): %v", err)
	}
	if len(vazio) != 0 {
		t.Fatalf("ListarPorEpisodio com estados vazios devia ser fail-closed (0 linhas), veio %d", len(vazio))
	}
}

// TestLaboratorio_TransicaoConcorrentePerdeACorrida verifica a guarda
// compare-and-set (PROVA 4): dois agregados lidos no mesmo estado, duas
// transições — só a primeira escreve, a segunda recebe Conflito (409). E um
// Transitar sobre um uuid inexistente devolve NaoEncontrado (404) — 409 e 404
// têm de ser distinguíveis.
func TestLaboratorio_TransicaoConcorrentePerdeACorrida(t *testing.T) {
	pool, ctx := ligar(t)
	migrarLaboratorio(t, pool, ctx)

	repoAnalises := pgrepo.NovoRepositorioAnalises(pool)
	repoRequisicoes := pgrepo.NovoRepositorioRequisicoes(pool)
	repoResultados := pgrepo.NovoRepositorioResultados(pool)

	hb, err := repoAnalises.ObterPorCodigo(ctx, "HB")
	if err != nil {
		t.Fatalf("obter HB: %v", err)
	}
	doenteID, episodioID := fixturaLaboratorio(t, pool, ctx, "12345678LA124", "Bia Laboratório")
	req, _ := dominio.NovaRequisicao(dominio.DadosNovaRequisicao{
		EpisodioID: episodioID, DoenteID: doenteID, MedicoRequisitanteID: medicoLabID,
		Prioridade: dominio.PrioridadeRotina,
		Itens:      []dominio.ItemRequisicao{{CodigoAnalise: "HB"}},
	})
	res, _ := dominio.NovoResultado("por-atribuir", "HB", hb.Unidade())
	reqID, err := repoRequisicoes.Emitir(ctx, req, []*dominio.Resultado{res})
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	fila, _ := repoResultados.ListarFila(ctx, []dominio.EstadoResultado{dominio.ResPendente})
	var id string
	for _, r := range fila {
		if r.RequisicaoID == reqID {
			id = r.ID
		}
	}
	if id == "" {
		t.Fatal("não encontrei o resultado PENDENTE recém-emitido na fila")
	}

	// Dois técnicos lêem o mesmo resultado PENDENTE.
	a, _ := repoResultados.ObterPorID(ctx, id)
	b, _ := repoResultados.ObterPorID(ctx, id)
	_ = a.ColherAmostra(tecnicoLabID, time.Now())
	_ = b.ColherAmostra(outroTecnico, time.Now())

	if err := repoResultados.Transitar(ctx, a); err != nil {
		t.Fatalf("a primeira colheita devia escrever: %v", err)
	}
	err = repoResultados.Transitar(ctx, b)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaConflito {
		t.Fatalf("a segunda colheita devia perder a corrida com Conflito, veio %v", err)
	}

	// Confirmação relendo a BD (não o objecto em memória): a linha ficou COLHIDA
	// pelo técnico que venceu, não sobreposta pelo perdedor.
	final, err := repoResultados.ObterPorID(ctx, id)
	if err != nil {
		t.Fatalf("obter resultado final: %v", err)
	}
	if final.Estado() != dominio.ResColhida {
		t.Fatalf("esperado COLHIDA (só a transição vencedora escreveu), veio %s", final.Estado())
	}

	// 404 vs 409: Transitar sobre um uuid que não existe na BD tem de devolver
	// NaoEncontrado, distinguível do Conflito acima.
	fantasma := dominio.ReconstruirResultado(dominio.SnapshotResultado{
		ID: "00000000-0000-4000-8000-00000000dead", RequisicaoID: reqID,
		CodigoAnalise: "HB", Unidade: "g/dL", Estado: dominio.ResPendente,
	})
	err = repoResultados.Transitar(ctx, fantasma)
	if err == nil || erros.CategoriaDe(err) != erros.CategoriaNaoEncontrado {
		t.Fatalf("transitar um resultado inexistente devia dar NaoEncontrado, veio %v", err)
	}
}

// TestLaboratorio_EmitirAtomico prova a PROVA 3: um item inválido a meio da
// transacção (aqui, um estado de resultado fora do CHECK da migração 0002) faz
// tudo falhar — nem a requisição, nem os itens, nem os resultados já inseridos
// antes do falhanço ficam na BD. Sem a transacção, a requisição e o primeiro
// item ficariam órfãos: visíveis nas listagens mas sem todos os resultados que
// a fila do laboratório espera.
func TestLaboratorio_EmitirAtomico(t *testing.T) {
	pool, ctx := ligar(t)
	migrarLaboratorio(t, pool, ctx)

	repoAnalises := pgrepo.NovoRepositorioAnalises(pool)
	repoRequisicoes := pgrepo.NovoRepositorioRequisicoes(pool)

	hb, err := repoAnalises.ObterPorCodigo(ctx, "HB")
	if err != nil {
		t.Fatalf("obter HB: %v", err)
	}
	glic, err := repoAnalises.ObterPorCodigo(ctx, "GLIC")
	if err != nil {
		t.Fatalf("obter GLIC: %v", err)
	}
	doenteID, episodioID := fixturaLaboratorio(t, pool, ctx, "12345678LA125", "Cátia Laboratório")

	req, err := dominio.NovaRequisicao(dominio.DadosNovaRequisicao{
		EpisodioID: episodioID, DoenteID: doenteID, MedicoRequisitanteID: medicoLabID,
		Prioridade: dominio.PrioridadeRotina,
		Itens:      []dominio.ItemRequisicao{{CodigoAnalise: "HB"}, {CodigoAnalise: "GLIC"}},
	})
	if err != nil {
		t.Fatalf("construir requisição: %v", err)
	}
	resValido, _ := dominio.NovoResultado("por-atribuir", "HB", hb.Unidade())
	// O segundo resultado é construído por ReconstruirResultado (não por
	// NovoResultado, que validaria e recusaria) com um estado fora do enum
	// aceite pela CHECK de laboratorio.resultados — simula "um item inválido"
	// a chegar ao repositório depois de já ter passado a requisição e o
	// primeiro resultado, forçando o INSERT a violar a CHECK a meio da
	// transacção.
	resInvalido := dominio.ReconstruirResultado(dominio.SnapshotResultado{
		RequisicaoID: "por-atribuir", CodigoAnalise: "GLIC", Unidade: glic.Unidade(),
		Estado: "ESTADO_INEXISTENTE",
	})

	_, err = repoRequisicoes.Emitir(ctx, req, []*dominio.Resultado{resValido, resInvalido})
	if err == nil {
		t.Fatal("a emissão com um resultado inválido devia falhar")
	}

	// Relendo a BD: nada do que a transacção tentou escrever ficou lá — nem a
	// requisição, nem o item HB, nem o resultado HB que fora inserido antes do
	// falhanço do segundo.
	var nReq, nItens, nRes int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM laboratorio.requisicoes WHERE episodio_id=$1`, episodioID).Scan(&nReq); err != nil {
		t.Fatalf("contar requisições: %v", err)
	}
	if err := pool.QueryRow(ctx, `
SELECT count(*) FROM laboratorio.itens_requisicao i
JOIN laboratorio.requisicoes r ON r.id = i.requisicao_id
WHERE r.episodio_id=$1`, episodioID).Scan(&nItens); err != nil {
		t.Fatalf("contar itens: %v", err)
	}
	if err := pool.QueryRow(ctx, `
SELECT count(*) FROM laboratorio.resultados res
JOIN laboratorio.requisicoes r ON r.id = res.requisicao_id
WHERE r.episodio_id=$1`, episodioID).Scan(&nRes); err != nil {
		t.Fatalf("contar resultados: %v", err)
	}
	if nReq != 0 || nItens != 0 || nRes != 0 {
		t.Fatalf("a emissão falhada deixou vestígios: %d requisições, %d itens, %d resultados", nReq, nItens, nRes)
	}
}

// TestLaboratorio_ChecksCoerenciaTransicoes prova a PROVA 5: as CHECK de
// coerência estado↔timestamps↔autores da migração 0002 sobrevivem a cada
// transição real contra Postgres — COLHIDA, PROCESSADA, RECUSADA a partir de
// PENDENTE, e RECUSADA a partir de COLHIDA. Cada transição é confirmada
// relendo a BD (não o agregado em memória).
func TestLaboratorio_ChecksCoerenciaTransicoes(t *testing.T) {
	pool, ctx := ligar(t)
	migrarLaboratorio(t, pool, ctx)

	repoAnalises := pgrepo.NovoRepositorioAnalises(pool)
	repoRequisicoes := pgrepo.NovoRepositorioRequisicoes(pool)
	repoResultados := pgrepo.NovoRepositorioResultados(pool)

	codigos := []string{"HB", "GLIC", "CREAT", "UREIA"}
	unidades := map[string]string{}
	itens := make([]dominio.ItemRequisicao, 0, len(codigos))
	for _, c := range codigos {
		a, err := repoAnalises.ObterPorCodigo(ctx, c)
		if err != nil {
			t.Fatalf("obter %s: %v", c, err)
		}
		unidades[c] = a.Unidade()
		itens = append(itens, dominio.ItemRequisicao{CodigoAnalise: c})
	}

	doenteID, episodioID := fixturaLaboratorio(t, pool, ctx, "12345678LA126", "Dora Laboratório")
	req, err := dominio.NovaRequisicao(dominio.DadosNovaRequisicao{
		EpisodioID: episodioID, DoenteID: doenteID, MedicoRequisitanteID: medicoLabID,
		Prioridade: dominio.PrioridadeUrgente, Itens: itens,
	})
	if err != nil {
		t.Fatalf("construir requisição: %v", err)
	}
	resultados := make([]*dominio.Resultado, 0, len(codigos))
	for _, c := range codigos {
		r, err := dominio.NovoResultado("por-atribuir", c, unidades[c])
		if err != nil {
			t.Fatalf("construir resultado %s: %v", c, err)
		}
		resultados = append(resultados, r)
	}
	reqID, err := repoRequisicoes.Emitir(ctx, req, resultados)
	if err != nil {
		t.Fatalf("emitir requisição: %v", err)
	}

	fila, err := repoResultados.ListarFila(ctx, []dominio.EstadoResultado{dominio.ResPendente})
	if err != nil {
		t.Fatalf("listar fila: %v", err)
	}
	porCodigo := map[string]string{}
	for _, r := range fila {
		if r.RequisicaoID == reqID {
			porCodigo[r.CodigoAnalise] = r.ID
		}
	}
	for _, c := range codigos {
		if porCodigo[c] == "" {
			t.Fatalf("não encontrei o resultado PENDENTE de %s na fila", c)
		}
	}

	// HB: PENDENTE → COLHIDA. A CHECK exige colhida_em e tecnico_colheita_id.
	hb, err := repoResultados.ObterPorID(ctx, porCodigo["HB"])
	if err != nil {
		t.Fatalf("obter HB: %v", err)
	}
	if err := hb.ColherAmostra(tecnicoLabID, time.Now()); err != nil {
		t.Fatalf("colher HB: %v", err)
	}
	if err := repoResultados.Transitar(ctx, hb); err != nil {
		t.Fatalf("a CHECK de COLHIDA devia aceitar a transição de HB: %v", err)
	}
	hbFinal, err := repoResultados.ObterPorID(ctx, porCodigo["HB"])
	if err != nil {
		t.Fatalf("reler HB: %v", err)
	}
	if hbFinal.Estado() != dominio.ResColhida {
		t.Fatalf("HB devia estar COLHIDA na BD, veio %s", hbFinal.Estado())
	}

	// GLIC: PENDENTE → COLHIDA → PROCESSADA. A CHECK exige submetida_em,
	// tecnico_submissor_id e valor.
	glic, err := repoResultados.ObterPorID(ctx, porCodigo["GLIC"])
	if err != nil {
		t.Fatalf("obter GLIC: %v", err)
	}
	if err := glic.ColherAmostra(tecnicoLabID, time.Now()); err != nil {
		t.Fatalf("colher GLIC: %v", err)
	}
	if err := repoResultados.Transitar(ctx, glic); err != nil {
		t.Fatalf("gravar colheita de GLIC: %v", err)
	}
	glic, _ = repoResultados.ObterPorID(ctx, porCodigo["GLIC"])
	if err := glic.SubmeterPreliminar(tecnicoLabID, "95", "", time.Now()); err != nil {
		t.Fatalf("submeter GLIC: %v", err)
	}
	if err := repoResultados.Transitar(ctx, glic); err != nil {
		t.Fatalf("a CHECK de PROCESSADA devia aceitar a transição de GLIC: %v", err)
	}
	glicFinal, err := repoResultados.ObterPorID(ctx, porCodigo["GLIC"])
	if err != nil {
		t.Fatalf("reler GLIC: %v", err)
	}
	if glicFinal.Estado() != dominio.ResProcessada {
		t.Fatalf("GLIC devia estar PROCESSADA na BD, veio %s", glicFinal.Estado())
	}

	// CREAT: PENDENTE → RECUSADA (directamente). A CHECK exige motivo_recusa.
	creat, err := repoResultados.ObterPorID(ctx, porCodigo["CREAT"])
	if err != nil {
		t.Fatalf("obter CREAT: %v", err)
	}
	if err := creat.RecusarAmostra("amostra hemolisada", time.Now()); err != nil {
		t.Fatalf("recusar CREAT: %v", err)
	}
	if err := repoResultados.Transitar(ctx, creat); err != nil {
		t.Fatalf("a CHECK de RECUSADA-de-PENDENTE devia aceitar a transição de CREAT: %v", err)
	}
	creatFinal, err := repoResultados.ObterPorID(ctx, porCodigo["CREAT"])
	if err != nil {
		t.Fatalf("reler CREAT: %v", err)
	}
	if creatFinal.Estado() != dominio.ResRecusada {
		t.Fatalf("CREAT devia estar RECUSADA na BD, veio %s", creatFinal.Estado())
	}

	// UREIA: PENDENTE → COLHIDA → RECUSADA. A mesma CHECK de motivo_recusa,
	// agora a partir de COLHIDA.
	ureia, err := repoResultados.ObterPorID(ctx, porCodigo["UREIA"])
	if err != nil {
		t.Fatalf("obter UREIA: %v", err)
	}
	if err := ureia.ColherAmostra(tecnicoLabID, time.Now()); err != nil {
		t.Fatalf("colher UREIA: %v", err)
	}
	if err := repoResultados.Transitar(ctx, ureia); err != nil {
		t.Fatalf("gravar colheita de UREIA: %v", err)
	}
	ureia, _ = repoResultados.ObterPorID(ctx, porCodigo["UREIA"])
	if err := ureia.RecusarAmostra("volume insuficiente", time.Now()); err != nil {
		t.Fatalf("recusar UREIA: %v", err)
	}
	if err := repoResultados.Transitar(ctx, ureia); err != nil {
		t.Fatalf("a CHECK de RECUSADA-de-COLHIDA devia aceitar a transição de UREIA: %v", err)
	}
	ureiaFinal, err := repoResultados.ObterPorID(ctx, porCodigo["UREIA"])
	if err != nil {
		t.Fatalf("reler UREIA: %v", err)
	}
	if ureiaFinal.Estado() != dominio.ResRecusada {
		t.Fatalf("UREIA devia estar RECUSADA na BD, veio %s", ureiaFinal.Estado())
	}
}

// TestLaboratorio_CatalogoRoundTripJSONB prova a PROVA 6: Guardar → ObterPorCodigo
// devolve os mesmos intervalos de referência e valores críticos (jsonb), e uma
// análise sem intervalos guarda literalmente '[]' na coluna — não 'null'.
func TestLaboratorio_CatalogoRoundTripJSONB(t *testing.T) {
	pool, ctx := ligar(t)
	migrarLaboratorio(t, pool, ctx)
	repoAnalises := pgrepo.NovoRepositorioAnalises(pool)

	// Análise com intervalos e críticos: o round-trip preserva os valores.
	intervalos := []dominio.IntervaloReferencia{
		{Perfil: dominio.PerfilAdulto, Sexo: dominio.SexoMasculino, Minimo: 4.0, Maximo: 11.0},
		{Perfil: dominio.PerfilPediatrico, Sexo: dominio.SexoAmbos, Minimo: 5.0, Maximo: 15.0},
	}
	criticos := []dominio.ValorCritico{
		{Operador: dominio.CriticoMenor, Limite: 2.0, Descricao: "leucopenia grave"},
		{Operador: dominio.CriticoMaior, Limite: 30.0, Descricao: "leucocitose grave"},
	}
	const codigo = "TESTLAB1"
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM laboratorio.analises WHERE codigo=$1`, codigo) })

	analise, err := dominio.NovaAnalise(codigo, "Leucócitos (teste de integração)", "10^3/uL", intervalos, criticos)
	if err != nil {
		t.Fatalf("construir análise: %v", err)
	}
	if err := repoAnalises.Guardar(ctx, analise); err != nil {
		t.Fatalf("guardar análise: %v", err)
	}

	lida, err := repoAnalises.ObterPorCodigo(ctx, codigo)
	if err != nil {
		t.Fatalf("obter análise: %v", err)
	}
	s := lida.Snapshot()
	if len(s.Intervalos) != len(intervalos) {
		t.Fatalf("esperava %d intervalos, veio %d: %+v", len(intervalos), len(s.Intervalos), s.Intervalos)
	}
	for i, esperado := range intervalos {
		if s.Intervalos[i] != esperado {
			t.Fatalf("intervalo %d não sobreviveu ao round-trip: esperava %+v, veio %+v", i, esperado, s.Intervalos[i])
		}
	}
	if len(s.ValoresCriticos) != len(criticos) {
		t.Fatalf("esperava %d valores críticos, veio %d: %+v", len(criticos), len(s.ValoresCriticos), s.ValoresCriticos)
	}
	for i, esperado := range criticos {
		if s.ValoresCriticos[i] != esperado {
			t.Fatalf("valor crítico %d não sobreviveu ao round-trip: esperava %+v, veio %+v", i, esperado, s.ValoresCriticos[i])
		}
	}

	// Análise SEM intervalos nem críticos: a coluna jsonb tem de guardar '[]'
	// literal, não 'null' — é a garantia de naoNil/naoNilCriticos no adaptador.
	const codigoVazio = "TESTLAB2"
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM laboratorio.analises WHERE codigo=$1`, codigoVazio) })
	analiseVazia, err := dominio.NovaAnalise(codigoVazio, "Análise sem intervalos (teste)", "un", nil, nil)
	if err != nil {
		t.Fatalf("construir análise vazia: %v", err)
	}
	if err := repoAnalises.Guardar(ctx, analiseVazia); err != nil {
		t.Fatalf("guardar análise vazia: %v", err)
	}
	var intervalosTexto, criticosTexto string
	if err := pool.QueryRow(ctx,
		`SELECT intervalos_referencia::text, valores_criticos::text FROM laboratorio.analises WHERE codigo=$1`,
		codigoVazio).Scan(&intervalosTexto, &criticosTexto); err != nil {
		t.Fatalf("ler jsonb em bruto: %v", err)
	}
	if intervalosTexto != "[]" {
		t.Fatalf("intervalos_referencia devia ser '[]', veio %q", intervalosTexto)
	}
	if criticosTexto != "[]" {
		t.Fatalf("valores_criticos devia ser '[]', veio %q", criticosTexto)
	}
}
