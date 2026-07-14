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
2. **Invariante-estrela:** um consentimento de finalidade CIRURGIA exige estar
   concedido e ter `documento_url` (anexo). `NovoProcedimento` só aceita um
   consentimento CIRURGIA, com anexo e vigente (senão RegraNegocio/422).
3. **State machine DDM-estrita:** AGENDADO → EM_CURSO (Iniciar) → CONCLUIDO
   (Concluir) ou → CANCELADO (Cancelar). `Cancelar` só de EM_CURSO — a CHECK do
   DDM obriga `inicio` não-nulo em CANCELADO. Um AGENDADO que não se realiza
   resolve-se cancelando o episódio. O motivo do cancelamento é **obrigatório**
   — `ProcedimentoCirurgico.Cancelar` rejeita motivo vazio/só-espaços
   (`CategoriaValidacao`), validado antes das guardas de estado — e fica
   registado nas observações do procedimento e no detalhe da auditoria
   (`clinico.procedimento.cancelado`).
4. **Anestesia:** VO com NENHUMA/LOCAL/SEDACAO_LIGEIRA/LOCO_REGIONAL; anestesista
   obrigatório se anestesia ≠ NENHUMA; a flag `requer_anestesista` do catálogo
   reforça a exigência na aplicação.
5. **RBAC:** escrita de cirurgia = Médico; escrita de consentimento = Médico +
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
  `Conflito`. `concluirProcedimento` manteve-se inalterado — a conclusão
  continua a aceitar corpo vazio/opcional.

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
