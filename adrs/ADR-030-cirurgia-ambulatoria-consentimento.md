# ADR-030 — Cirurgia Ambulatória e Consentimento (LPDP)

- **Estado:** Aceite
- **Data:** 2026-07-13
- **Marco/Sprint:** M2 / Sprint 11
- **Fontes:** ADR-018 pt2, DDM-001 v2.0 (consentimentos), DDM-001 v2.1 adenda §4.

## Contexto

O critério de saída M2 exige cirurgia ambulatória: tipo de episódio dedicado,
agregado `ProcedimentoCirurgico` com state machine e consentimento cirúrgico com
anexo obrigatório. A tabela `clinico.consentimentos` (DDM v2.0) nunca fora criada
(a migração dos doentes adiou-a explicitamente) — é dependência crítica da FK
`procedimentos_cirurgicos.consentimento_id` e da invariante-estrela.

## Decisão

1. **Consentimento (LPDP) com ciclo completo:** tabela + agregado `Consentimento`
   (finalidades TRATAMENTO/COMUNICACAO/PARTILHA_SEGURADORA/INVESTIGACAO/CIRURGIA),
   registar/revogar/listar/obter, todos auditados nas escritas.
2. **Invariante-estrela (com qualificação temporal):** um consentimento de
   finalidade CIRURGIA exige estar concedido e ter `documento_url` (anexo).
   `NovoProcedimento` só aceita um consentimento CIRURGIA, com anexo e vigente
   (senão RegraNegocio/422). A invariante é verificada **no agendamento** e
   **revalidada no início** (`CasoIniciarProcedimento`), porque entre um e outro
   pode passar tempo e o mundo muda: o doente pode revogar o consentimento e o
   episódio pode ser fechado. A revalidação do início exige, cumulativamente:
   consentimento ainda CIRURGIA + com anexo + **vigente** (senão RegraNegocio/422)
   e episódio ainda **ABERTO** (senão Conflito/409). A revalidação vive na
   **aplicação** (precisa de repositórios), como já acontecia no agendamento.
3. **O início é o ponto de não-retorno; a conclusão e o cancelamento não são
   bloqueados.** `Concluir` e `Cancelar` de um procedimento já iniciado **não**
   revalidam o consentimento nem o estado do episódio: nesse ponto o acto
   cirúrgico já ocorreu, e recusar a conclusão só perderia o registo clínico do
   que efectivamente se fez ao doente (complicações, motivo de cancelamento
   intra-operatório) — deixando a linha eternamente EM_CURSO. Bloqueia-se a
   cirurgia antes de ela começar, não o registo do que já se fez.
4. **A revogação do consentimento nunca é bloqueada.** `CasoRevogarConsentimento`
   não olha para procedimentos: revogar é um direito LPDP do doente e não pode
   ser condicionado à agenda cirúrgica. O que se impede é a **cirurgia** (o
   início é recusado enquanto o consentimento não estiver vigente), não a
   revogação.
5. **Guardas de concorrência (compare-and-set) nos UPDATE de transição.** O
   `SnapshotProcedimento` expõe o `EstadoAnterior` (o estado com que o agregado
   foi rehidratado; vazio num agregado novo) e o UPDATE do repositório é
   condicionado a `WHERE id=$1 AND estado=$N`. Sem isto, duas transições
   concorrentes legais a partir do mesmo estado (concluir e cancelar em
   simultâneo, ou duplo-clique em iniciar) passavam ambas as guardas do domínio e
   escreviam ambas — deixando no audit log **imutável** dois eventos
   contraditórios para o mesmo procedimento. Quem perde a corrida recebe
   **Conflito/409** (a linha existe, mudou de estado), distinguido de
   NaoEncontrado/404. O mesmo raciocínio aplica-se à revogação do consentimento
   (`AND revogado_em IS NULL`). É a mesma defesa em profundidade do `MotorDispensa`
   (Sprint 10).
6. **State machine DDM-estrita:** AGENDADO → EM_CURSO (Iniciar) → CONCLUIDO
   (Concluir) ou → CANCELADO (Cancelar). `Cancelar` só de EM_CURSO — a CHECK do
   DDM obriga `inicio` não-nulo em CANCELADO. Um AGENDADO que não se realiza
   resolve-se cancelando o episódio. O motivo do cancelamento é **obrigatório**
   — `ProcedimentoCirurgico.Cancelar` rejeita motivo vazio/só-espaços
   (`CategoriaValidacao`), validado antes das guardas de estado — e fica
   registado nas observações do procedimento e no detalhe da auditoria
   (`clinico.procedimento.cancelado`).
7. **As observações são anexadas, nunca sobrepostas.** `Concluir` e `Cancelar`
   acrescentam o novo texto (observações da conclusão, motivo do cancelamento) ao
   que já lá estava, em nova linha. Sobrepor apagava definitivamente a nota
   pré-operatória — que é registo clínico e não tem versionamento na linha (p.ex.
   "doente anticoagulado — varfarina suspensa a 5/7" desaparecia ao cancelar por
   intercorrência). Um texto novo vazio deixa o anterior intacto.
8. **Código do procedimento canónico:** o código gravado em
   `procedimentos_cirurgicos.codigo_procedimento` é o do **catálogo** (fonte de
   verdade), não o que o cliente enviou. A pesquisa do catálogo normaliza para
   maiúsculas, logo `"prc001"` é aceite — mas, como não há FK para o catálogo (ver
   desvios), a linha ficaria gravada em minúsculas e partia os consumidores por
   código (Facturação, relatório MINSA).
9. **Anestesia:** VO com NENHUMA/LOCAL/SEDACAO_LIGEIRA/LOCO_REGIONAL; anestesista
   obrigatório se anestesia ≠ NENHUMA; a flag `requer_anestesista` do catálogo
   reforça a exigência na aplicação.
10. **RBAC:** escrita de cirurgia = Médico; escrita de consentimento = Médico +
    Administrativo; leitura = leque clínico.

## Desvios ao blueprint (conscientes)

- `ProcedimentoCirurgico`/`Consentimento` no pacote `clinico` (plano), não em
  subpacote `clinico/cirurgia` — consistência com `episodio`/`doente`.
- Erros por `erros.Novo(categoria, msg)` PT-PT em vez de sentinelas `ErrXxx`.
- FK de consentimento sem `ON DELETE CASCADE` (doente é soft-delete).
- Sem FK `codigo_procedimento` → catálogo; validação na aplicação (dá também o
  `requer_anestesista`).
- **Motivo do cancelamento obrigatório:** o DDM/plano original aceitava motivo
  vazio no cancelamento do procedimento. A revisão da Task 8 encontrou um
  defeito decorrente disso — corpo JSON malformado em
  `POST /procedimentos/:pid/cancelar` devolvia 200 e cancelava com o motivo em
  branco, porque o handler ignorava o erro de bind e o domínio aceitava motivo
  vazio. Por decisão explícita do utilizador humano, o motivo passou a ser
  obrigatório: é registo clínico-legal de um cancelamento intra-operatório e
  não pode perder-se em silêncio. Efeito lateral aceite: cancelar um
  procedimento AGENDADO com motivo vazio devolve agora `Validacao` em vez de
  `Conflito`. A conclusão continua a aceitar corpo **ausente** (200), mas a
  revisão final do branch encontrou o gémeo do mesmo defeito em
  `concluirProcedimento` — um corpo **presente e malformado** era ignorado em
  silêncio e devolvia 200, confirmando ao cliente o registo de complicações que
  na verdade se tinham perdido. Passou a distinguir-se corpo ausente (`io.EOF` →
  200) de corpo malformado (→ 400, `Validacao`).

## Consequências

- Fecha o critério de saída M2 de cirurgia ambulatória.
- `documento_url` é referência textual ao anexo; o upload binário (MinIO) fica
  para fatia futura.
- Evento `ProcedimentoCirurgicoConcluido` definido mas não emitido (scaffolding),
  para consumo por Financeiro/reporting em marcos posteriores.

## Dívida registada (não-bloqueante)

- Auditoria fora da transacção das escritas (janela sem trilho se a auditoria
  falhar), como nos sprints anteriores.
- Upload binário do anexo de consentimento.
- Integração com Facturação (linha por procedimento) e relatório MINSA.
- A CHECK estado↔timestamps da migração `0006` **não** obriga `fim IS NOT NULL`
  em CONCLUIDO/CANCELADO (só exige `inicio IS NOT NULL`). A coerência do `fim`
  depende inteiramente do domínio, que o garante em todos os caminhos
  (`Concluir` e `Cancelar` fixam sempre `fim`, e são as únicas transições para
  esses estados). Apertar a CHECK exigiria migração nova — fica registado.
