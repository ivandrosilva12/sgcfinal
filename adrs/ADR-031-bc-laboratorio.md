# ADR-031 — BC Laboratório: requisição, amostra e resultado preliminar

- **Estado:** Aceite
- **Data:** 2026-07-14
- **Marco/Sprint:** M3 / Sprint 12
- **Fontes:** CCD-M3 (blueprint), DDM-001, ADR-028/029 (precedente da ACL da Farmácia).

## Contexto

O BC Laboratório era o último bounded context clínico por abrir. O blueprint (CCD-M3)
descreve-o em duas fatias: até ao resultado preliminar, e depois a validação com
segregação de funções e valores críticos. Esta ADR cobre a primeira e regista três
decisões tomadas pelo dono do produto durante a execução que divergem do plano
original (secção "Desvios ao plano" abaixo).

## Decisão

1. **A requisição vive no BC Laboratório**, não no Clínico. `RequisicaoLab` refere
   `episodio_id` e `doente_id` por id, sem FK cross-context (ver
   `migrations/laboratorio/0002_requisicoes_resultados.sql`); a existência e o estado
   do episódio são validados por uma ACL (`applaboratorio.LeitorClinico`) na camada de
   aplicação, com o adaptador `internal/adapters/laboratorio.LeitorClinico`
   (`internal/adapters/laboratorio/leitor_clinico.go`) a ler os repositórios
   `clinico.RepositorioDoentes`/`clinico.RepositorioEpisodios` — este adaptador importa,
   sim, tipos do domínio Clínico, porque traduzi-los é precisamente o seu trabalho de
   ACL. A garantia que interessa é outra e mais estrita: **o domínio e a aplicação do
   Laboratório nunca importam o Clínico** — só o adaptador conhece `clinico`, e do
   outro lado da porta `applaboratorio.LeitorClinico` só chegam duas perguntas
   booleanas (`DoenteActivo`, `EpisodioAbertoDoDoente`). É exactamente o desenho da
   Receita (ADR-028): um segundo padrão para o mesmo problema só criaria confusão.
2. **Emitir uma requisição cria um `Resultado` PENDENTE por análise pedida**, na mesma
   transacção (`RepositorioRequisicoes.Emitir`, `internal/adapters/pgrepo/requisicoes_repo.go`):
   requisição, itens e resultados são inseridos sob a mesma `tx`, com `Commit` só no
   fim; qualquer falha faz `Rollback` de tudo. Uma requisição sem resultados nunca
   apareceria na fila do laboratório e ficaria invisível para toda a gente.
3. **O submissor é o sujeito autenticado, nunca um campo do corpo do pedido.** Em
   `CasoEmitirRequisicao`, `CasoColherAmostra`, `CasoRecusarAmostra` e
   `CasoSubmeterPreliminar` o `actor` vem sempre de `SessaoDe(c).Sujeito` no handler
   HTTP, nunca do JSON. Esta é a decisão que torna real a segregação de funções do
   Sprint 13: se o cliente pudesse declarar quem submeteu, poderia validar o seu
   próprio resultado declarando outro nome. A mesma regra valerá para o validador.
4. **A máquina de estados do `Resultado`** (`internal/domain/laboratorio/resultado.go`)
   é PENDENTE → COLHIDA → PROCESSADA, com RECUSADA como saída a partir de PENDENTE ou
   COLHIDA:

   ```
   PENDENTE → COLHIDA → PROCESSADA → VALIDADA → CONCLUIDA
       └──────────┴─────► RECUSADA
   ```

   VALIDADA e CONCLUIDA já existem no enum `EstadoResultado` e nas CHECK da base de
   dados, mas a transição `Validar` (com a invariante de segregação submissor ≠
   validador) fica para o Sprint 13.
5. **Guarda compare-and-set nos UPDATE de transição** (lição do Sprint 11): o `UPDATE`
   de `RepositorioResultados.Transitar` inclui `WHERE id=$1 AND estado=$10`, comparando
   com o `EstadoAnterior` do snapshot lido. Quando `RowsAffected()==0`,
   `erroTransicaoFalhada` distingue "a linha não existe" (404,
   `CategoriaNaoEncontrado`) de "a linha existe mas já mudou de estado entretanto" (409,
   `CategoriaConflito`) — duas colheitas/recusas/submissões concorrentes da mesma
   amostra não se atropelam: a segunda perde a corrida e recebe 409, não uma escrita
   silenciosa por cima da primeira.
6. **CHECK de coerência estado↔timestamps↔autores** na tabela `laboratorio.resultados`:
   a base de dados recusa uma COLHIDA sem `tecnico_colheita_id`/`colhida_em`, uma
   PROCESSADA sem `tecnico_submissor_id`/`submetida_em`/`valor`, uma RECUSADA sem
   `motivo_recusa`, e uma VALIDADA/CONCLUIDA sem `patologista_validador_id`/
   `validada_em`.
7. **CHECK de segregação de funções já existe na tabela**:
   `CHECK (patologista_validador_id IS NULL OR patologista_validador_id <> tecnico_submissor_id)`.
   A transição de validação só chega no Sprint 13, mas a invariante negativa
   (submissor ≠ validador) é defesa em profundidade barata de escrever agora — mesmo
   sem caso de uso que a exercite ainda.
8. **Regra de visibilidade do marco: o preliminar (PROCESSADA) não é visível ao
   médico.** É imposta em duas camadas, deliberadamente:
   - **Aplicação** — `CasoListarResultadosDoEpisodio.Executar` chama
     `resultados.ListarPorEpisodio(ctx, episodioID, EstadosVisiveisAoMedico)`, onde
     `EstadosVisiveisAoMedico = {ResValidada, ResConcluida}`
     (`internal/application/laboratorio/ports.go`). O RBAC de rota não bastaria aqui:
     o médico *tem* de ver os resultados validados do mesmo endpoint, pelo que a
     distinção certa é pelo estado, não pelo papel.
   - **RBAC de rota** — `GET /api/v1/laboratorio/fila` (a fila de trabalho de quem
     executa e dirige o laboratório) devolve **todos** os estados por desenho, o que
     inclui PROCESSADA. Por isso está fechada ao papel `Medico`: `filaLab := RBAC(
     PapelTecnicoLab, PapelPatologista, PapelDirector)`
     (`internal/adapters/http/laboratorio_handler.go`). Sem esta segunda porta, o
     preliminar vazaria para o médico pela fila, mesmo com o filtro da aplicação
     correcto na leitura clínica.
9. **`ListarPorEpisodio` é fail-closed, `ListarFila` é fail-open — assimetria
   deliberada e documentada na porta.** `EstadosVisiveisAoMedico` é uma `var` pública e
   mutável; se `RepositorioResultados.ListarPorEpisodio` tratasse uma lista de estados
   vazia como "todos os estados" (o que fazia até ao commit `a78acf0`), qualquer
   alteração acidental que esvaziasse essa variável abriria a leitura clínica ao
   preliminar. Por isso `ListarPorEpisodio` devolve **zero linhas** quando `estados` é
   vazio/nil (`internal/adapters/pgrepo/resultados_repo.go`). `ListarFila` mantém
   "vazio = todos", que é o que a fila de trabalho precisa (um técnico sem filtro quer
   ver tudo). A assimetria está documentada no comentário da interface
   `RepositorioResultados` em `internal/domain/laboratorio/resultado.go`.
10. **O papel Admin não lê dados clínicos do Laboratório** (minimização LPDP, commits
    `e5dd0f5` e `34ddc2a`). O plano original dava ao Admin leitura clínica; foi
    removido de `GET /api/v1/episodios/:eid/requisicoes`,
    `GET /api/v1/episodios/:eid/resultados` e `GET /api/v1/requisicoes/:rid` — o grupo
    `leituraClinicaLaboratorio` em `RegistarLaboratorio` só inclui
    `Medico, Enfermeiro, Director, TecnicoLab, Patologista`. O Admin mantém a **gestão
    do catálogo de análises** — escrita (`catalogoEscrita = RBAC(Admin, Director)`) e
    leitura (`catalogoLeitura`, que inclui o Admin) de `/api/v1/analises` — porque o
    catálogo é configuração/dado de referência, não dado de doente.

## Desvios ao blueprint (conscientes)

- **Numeração de marcos.** O CCD-M3 do blueprint é "Farmácia + Laboratório". Neste
  repositório a Farmácia foi entregue no M2 (Sprints 9–10), pelo que o M3 é apenas o
  Laboratório (Sprints 12–13). Os marcos do blueprint continuam a valer como
  referência de âmbito, não de numeração.
- **O "biólogo validador" é o papel `Patologista`.** Os 11 papéis do DDM-001 não
  incluem `Biologo` (ver `docs/ERRATA-001-papeis.md`).
- **Intervalos de referência e valores críticos em `jsonb`**, não em tabelas-filho:
  são lidos em bloco com o agregado (`internal/domain/laboratorio/analise.go`) e nunca
  consultados isoladamente por SQL.

## Desvios ao plano de execução (decisões do dono do produto)

Três decisões tomadas durante a execução das Tasks 5–9 divergem do plano original
desta fatia e ficam registadas aqui:

1. **A regra de visibilidade central do marco exige a dupla guarda descrita no ponto 8
   acima** (aplicação + RBAC de rota) — o plano previa só o filtro na aplicação; o
   dono do produto identificou que a fila do laboratório, por não filtrar estados por
   desenho, era uma segunda via de fuga do preliminar e teria de ser fechada ao papel
   Medico.
2. **`ListarPorEpisodio` passou a ser fail-closed** (commit `a78acf0`): o plano original
   tratava lista de estados vazia como "todos os estados" nos dois repositórios; o
   dono do produto decidiu que, sendo `EstadosVisiveisAoMedico` uma `var` mutável, essa
   semântica era um default fail-open perigoso especificamente na leitura clínica, e
   pediu a assimetria descrita no ponto 9.
3. **O Admin deixou de ler dados clínicos do Laboratório** (commits `e5dd0f5` e
   `34ddc2a`): o plano dava-lhe leitura de requisições e resultados; o dono do produto
   removeu-a por minimização LPDP, mantendo apenas a gestão do catálogo (ponto 10).

## Consequências

- Abre o marco M3 e prepara o Sprint 13 (validação, segregação, valores críticos).
- O endpoint de leitura clínica de resultados existe e devolve vazio até haver
  validação — é o critério de saída do blueprint ("o médico tenta ver o preliminar e
  não vê").

## Dívida registada (não-bloqueante)

- **Override de alergia da Farmácia sem dupla aprovação.** O CCD-M3 exige aprovação do
  médico prescritor **e** do Director Clínico; o que existe é um override de actor
  único com justificação auditada (`internal/application/farmacia/dispensa.go`). Não se
  resolve dentro de um sprint de Laboratório.
- Auditoria fora da transacção das escritas (como nos sprints anteriores).
- Cancelamento de requisição (estado CANCELADA existe no schema, sem caso de uso).
- Eventos definidos mas não emitidos (scaffolding, à espera do Outbox).
