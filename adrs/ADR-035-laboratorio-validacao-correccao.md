# ADR-035 — BC Laboratório: validação, valores críticos e correcção

- **Estado:** Aceite
- **Data:** 2026-07-15
- **Marco/Sprint:** M3 / Sprint 13 (fecha o marco)
- **Fontes:** ADR-031 (requisição, amostra e resultado preliminar, precedente do mesmo
  BC); `docs/superpowers/specs/2026-07-15-sprint13-laboratorio-validacao-correccao-design.md`
  (desenho) e `docs/superpowers/plans/2026-07-15-sprint13-laboratorio-validacao-correccao.md`
  (plano das 13 tasks, TDD).

## Contexto

O Sprint 12 (ADR-031) abriu o BC Laboratório até ao resultado preliminar: PENDENTE →
COLHIDA → PROCESSADA, sem visibilidade clínica. Faltava a validação com segregação de
funções (o critério de saída central do blueprint), a detecção e notificação de
valores críticos, e a correcção de um resultado já validado. Este sprint entrega as
três fatias e fecha o marco M3.

## Decisão

1. **`CONCLUIDA` passa a significar "arquivado/substituído"**, não apenas um estado
   terminal genérico: a correcção (`Resultado.Corrigir`,
   `internal/domain/laboratorio/resultado.go`) arquiva o original em CONCLUIDA e cria
   um novo `Resultado` em VALIDADA, com `corrige_resultado_id → original`
   (`migrations/laboratorio/0003_correccao_resultados.sql`). O original nunca é
   apagado nem reescrito — a proveniência (técnico submissor, valores, timestamps
   originais) fica preservada na linha arquivada.
2. **O valor crítico é avaliado na validação**, no domínio, contra o catálogo:
   `Analise.AvaliarCritico` (`internal/domain/laboratorio/analise.go`) compara o valor
   submetido com os limiares críticos da análise; um valor não-numérico **nunca** é
   crítico (falha segura — não bloqueia a validação, só não dispara o alerta). A
   correcção reavalia o crítico do valor corrigido pela mesma função, porque um valor
   corrigido pode cruzar o limiar num sentido ou noutro.
3. **Segregação de funções em `Validar` e `Corrigir`**: ambas exigem
   `actor != tecnicoSubmissorID`, devolvendo `CategoriaRegraNegocio` (422) no domínio
   quando violado — testado em `internal/domain/laboratorio/resultado_test.go`
   (`TestResultado_Validar`). É **defesa em profundidade** com a CHECK da base de
   dados: a CHECK `patologista_validador_id IS NULL OR patologista_validador_id <>
   tecnico_submissor_id` já existia desde a migração 0002 (registada na ADR-031,
   ponto 7, sem caso de uso que a exercitasse ainda) e ganha o nome real
   `resultados_check4` ao materializar-se. O teste de integração
   (`tests/integration/laboratorio_test.go`) prova-a directamente contra a BD,
   contornando deliberadamente `RepositorioResultados.Transitar` (cujo CAS
   `WHERE estado=...` nunca deixaria uma linha inconsistente chegar à CHECK) e
   confirmando o código `SQLSTATE 23514` com `ConstraintName == "resultados_check4"`.
4. **SMS ao médico requisitante por extensão da ACL sobre a Identidade**:
   `ResolvedorContacto.ContactoClinico` resolve o telefone do médico a partir do
   `MedicoRequisitanteID` da requisição; `NotificadorCritico` é o gateway de envio
   (adaptador HTTP real ou `NotificadorNulo` por configuração,
   `internal/adapters/laboratorio`). O envio é **best-effort e sempre auditado**
   (`alertarValorCritico`, `internal/application/laboratorio/notificacao.go`): falha a
   resolver o contacto, falta de telefone ou falha no envio nunca revertem a validação
   ou a correcção já persistidas — só mudam o `Detalhe` do registo de auditoria
   `laboratorio.valor_critico.notificado`, que é a prova de "SMS auditado" exigida
   pelo critério de saída, não a entrega efectiva do SMS.
5. **`EstadosVisiveisAoMedico` reduzido a `{VALIDADA}`** — o arquivado (CONCLUIDA)
   deixa de fazer parte da leitura clínica normal do episódio; só o resultado vigente
   (o mais recente, validado) é visível ao médico. A fila de trabalho do laboratório
   continua fail-open (ADR-031, ponto 9) e mostra ambos, para o Patologista/Director
   perceberem o histórico de correcção.
6. **Transacção única na correcção**: `RepositorioResultados.Corrigir`
   (`internal/adapters/pgrepo/resultados_repo.go`) faz o CAS que arquiva o original
   (`WHERE id=$1 AND estado=$2`, de VALIDADA para CONCLUIDA) e o `INSERT` do novo
   resultado VALIDADA sob a mesma `tx`, com `Commit` só no fim — a mesma disciplina
   transaccional das ADR-031/ADR-034 (nenhuma correcção fica "meio feita").

## Consequências

- Fecha o marco M3 — Laboratório: catálogo → requisição → amostra → preliminar →
  validação (segregação, crítico) → correcção, com visibilidade clínica correcta em
  cada fatia.
- Dívida mantida (não-bloqueante, herdada e ampliada da ADR-031):
  - Integração real de SMS (o gateway HTTP existe; a operadora concreta fica por
    escolher/contratar).
  - Eventos definidos mas não emitidos — `laboratorio.resultado.validado`,
    `laboratorio.valor_critico.detectado`, `laboratorio.resultado.corrigido`
    (`internal/domain/laboratorio/eventos.go`) são scaffolding à espera do Outbox.
  - Auditoria fora da transacção das escritas (como nos sprints anteriores).
  - Falta uma migração 0004 futura com índices únicos parciais como defesa em
    profundidade — `UNIQUE (requisicao_id, codigo_analise) WHERE estado='VALIDADA'` e
    `UNIQUE (corrige_resultado_id) WHERE corrige_resultado_id IS NOT NULL`. Hoje a
    invariante "um só resultado vigente por análise" é garantida apenas pelo CAS da
    aplicação (`RepositorioResultados.Corrigir`), sem rede de segurança ao nível da BD.
  - A auditoria do alerta de valor crítico é "sempre tentada" mas não garantida: se o
    próprio `auditor.Registar` falhar em `alertarValorCritico`
    (`internal/application/laboratorio/notificacao.go`), o erro é engolido sem log —
    fica sem rasto tanto o envio como a tentativa de auditar.
  - `DetalheResultado` não expõe ainda a proveniência (validador, `validadaEm`,
    `corrigeResultadoID`) na leitura clínica — falta para a UI, fica para um sprint
    futuro.
