# ADR-039 — BC Financeiro: agregado Factura em RASCUNHO

- **Estado:** Aceite
- **Data:** 2026-07-18
- **Marco/Sprint:** M4 — Financeiro (Sprint 14)
- **Fontes:** design em
  `docs/superpowers/specs/2026-07-18-adr-039-bc-financeiro-factura-design.md`; plano
  em `docs/superpowers/plans/2026-07-18-adr-039-bc-financeiro-factura.md`; ADR-026
  (abertura do BC Clínico); ADR-031 (abertura do BC Laboratório); ADR-038 (padrão de
  agregado rico + repositório dono da tx).

## Contexto

O BC Financeiro é o último dos 5 bounded contexts por implementar e o de maior
risco regulatório do projecto: cadeia hash SHA-256, numeração sequencial por série,
imutabilidade, SAF-T-AO (submissão mensal à AGT) e integração EMIS Multicaixa. Segue
a convenção já estabelecida para abertura de BC — uma fatia vertical fina com ADR
próprio, como a ADR-026 abriu o Clínico e a ADR-031 abriu o Laboratório.

O blueprint separa Sprint 14 (domínio Factura) de Sprint 15 (cadeia hash +
numeração + imutabilidade). Esta fatia entrega **apenas o domínio em estado
RASCUNHO** (Opção A do brainstorming): construção do agregado, linhas com IVA e
totais, persistência transaccional, HTTP e RBAC. A emissão e a jóia da coroa
regulatória — hash, numeração por série, imutabilidade — ficam deliberadamente para
o ADR-040, para não misturar o risco regulatório com o arranque do domínio.

O M4 pressupõe um papel que os 11 papéis canónicos do DDM-001 v2.0 (reconciliados
na ERRATA-001) não incluem: Tesoureiro ("Tesoureiro + Director + Auditor assinam
UAT"). É preciso fundação RBAC antes de expor qualquer rota financeira.

## Decisão

1. **Agregado `Factura` (raiz) + `ItemFactura` (entidade-filho) + `ClienteSnapshot`
   (VO).** `Factura` nasce em `RASCUNHO` via `NovaFactura(cliente, episodioID)`;
   `AdicionarItem`/`RemoverItem` só são permitidos em RASCUNHO. `EstadoFactura`
   **antecipa** `EMITIDA` e `ANULADA` no enum e na CHECK da BD — à imagem do padrão
   VALIDADA/CONCLUIDA do BC Laboratório antes de as suas transições existirem —
   mas nesta fatia só `RASCUNHO` é alcançável; as transições ficam para o ADR-040/041.
2. **Tipos de linha com snapshot + id lógico, sem FK cross-context.** `TipoLinha`
   (`CONSULTA`, `DISPENSA`, `EXAME_ANALISE`, `ESTUDO_IMAGEM`,
   `PROCEDIMENTO_CIRURGICO`) classifica a operação de origem; `CONSULTA` liga-se ao
   `episodioID` da própria factura, as restantes exigem `operacaoID` (uuid lógico
   nu, sem FK). A descrição e o preço de cada linha são um **snapshot** capturado no
   momento — nunca uma referência viva a outro BC. `ClienteSnapshot` (nome
   obrigatório, NIF opcional validado pelo VO do Shared Kernel, morada opcional) é a
   mesma disciplina aplicada ao cliente da factura: fotografia imutável, sem FK ao
   `Doente`. Rejeitado: FK cross-context para `episodio_id`/`operacao_id` — quebraria
   a regra "sem FK cross-context" (CLAUDE.md §3) que já rege todos os outros BCs.
3. **IVA por item (`ISENTO`/`STANDARD` 14%), arredondamento meia-acima por linha,
   total autoritário no domínio.** `RegimeIVA` é configurável por linha (CLAUDE.md
   §8: saúde geralmente isenta, mas nem sempre). Aritmética inteira em cêntimos
   (`moeda.AOA`) — nunca vírgula flutuante. Por linha:
   `ivaCentimos = (subtotalCentimos × 14 + 50) / 100` quando STANDARD, `0` quando
   ISENTO. Os totais da factura somam os subtotais e os IVA **já arredondados por
   linha** (prática fiscal — arredondar por linha e somar, não somar e arredondar).
   O cálculo vive inteiramente no agregado (`Totais()`); nunca em SQL nem em serviço
   de aplicação — o total que aparece à Tesouraria é sempre o do domínio.
4. **Novo papel RBAC: Tesoureiro (12.º papel, ERRATA-002).** Os 11 papéis do
   DDM-001 v2.0 não previam facturação. Acrescenta-se `PapelTesoureiro` ao enum
   `identidade.Papel`, **não-sensível** nesta fatia (sem MFA) — decisão confirmada
   explicitamente no brainstorming e registada como algo a **rever no ADR-040**,
   quando a emissão (acção irreversível com efeito fiscal) tornar o argumento a
   favor de MFA mais forte. Escrita das rotas financeiras exige Tesoureiro; leitura
   aceita Tesoureiro, Director e Auditor.
5. **Persistência por upsert transaccional, seguindo o padrão de repositório dono
   da tx.** `RepositorioFacturas.Guardar` faz INSERT quando a factura é nova ou
   UPDATE guardado por `estado='RASCUNHO'` quando já existe, e **reescreve as linhas
   por substituição** (delete + insert) na mesma transacção — sem diff linha a
   linha, porque a fatia não tem edição concorrente de linhas a proteger. Migração
   `migrations/financeiro/0001_facturas.sql`: schema `financeiro`, tabelas
   `facturas`/`itens_factura` com CHECKs de estado/tipo/regime/quantidade/preço; FK
   `itens_factura → facturas` é **intra-BC** e portanto permitida; `episodio_id` e
   `operacao_id` continuam uuid nus, sem FK cross-context.
6. **Snapshot de linha fornecido no pedido; auto-população via ACL deferida.**
   Quem factura indica descrição e preço no próprio pedido de
   `AdicionarItemFactura` — não há, nesta fatia, uma leitura ACL que vá buscar o
   preço/descrição às operações clínicas (Clínico/Farmácia/Laboratório). Rejeitado
   para esta fatia: construir já essa ACL — cada BC de origem tem o seu próprio
   catálogo de preços e o desenho da leitura correcta (que preço? de qual momento?)
   merece o seu próprio ADR, não deve bloquear o arranque do domínio Financeiro.

## Consequências

**Positivas**

- O BC Financeiro existe e está provado ponta a ponta (domínio → aplicação → HTTP
  → persistência), com o mesmo padrão arquitectural dos outros 4 BCs — nada de
  atalho ou excepção para o último bounded context.
- O cálculo de IVA/totais fica isolado no domínio desde o primeiro dia, pronto a
  ser reutilizado (e nunca reescrito em SQL) quando o ADR-040 fixar a factura na
  emissão.
- A fundação RBAC (Tesoureiro) está pronta para as fatias seguintes do M4 sem
  reabrir o enum de papéis a cada ADR.
- O corte Opção A mantém o risco regulatório (hash, numeração, imutabilidade,
  SAF-T-AO, EMIS) isolado em ADRs próprias, cada uma revisável e testável
  isoladamente.

**Negativas**

- Sem emissão nesta fatia, uma `Factura` em RASCUNHO não tem qualquer valor legal —
  é puramente interno até ao ADR-040. Não é utilizável em produção isoladamente.
- O papel Tesoureiro sem MFA é uma decisão explicitamente provisória: até ao
  ADR-040, uma conta Tesoureiro comprometida pode criar/alterar rascunhos de
  factura (sem efeito fiscal, porque nada é emitido) sem o factor adicional exigido
  aos outros papéis sensíveis.
- O snapshot de linha fornecido no pedido confia em quem factura para introduzir o
  preço correcto — sem auto-população via ACL, não há validação cruzada contra o
  preço praticado no Clínico/Farmácia/Laboratório nesta fatia.

## Diferido

- **ADR-040**: emissão — cadeia hash SHA-256, numeração sequencial por série,
  imutabilidade; e reavaliação da exigência de MFA para o papel Tesoureiro.
- **ADR-041**: anulação por nova factura (nunca `UPDATE`) e pagamentos (parcial,
  múltiplos métodos).
- **ADR-042**: SAF-T-AO — geração XML, validação XSD, submissão em sandbox.
- **ADR-043**: integração EMIS Multicaixa (sandbox, webhook, estado pendente).
- Auto-população de linhas via ACL sobre o Clínico/Farmácia/Laboratório; validação
  de `episodio_id` via ACL (nesta fatia é um uuid lógico sem verificação
  cross-BC).
- **Bloqueio optimista (coluna `versao`)** para editar linhas de rascunho em
  concorrência: a edição actual (adicionar/remover item) é read-modify-write sem
  guarda de versão — duas escritas concorrentes sobre o mesmo rascunho perdem uma
  actualização (lost-update). Aceite nesta fatia porque o rascunho não tem valor
  legal; a resolver no **ADR-040**, antes de a factura ganhar valor legal na
  emissão.
