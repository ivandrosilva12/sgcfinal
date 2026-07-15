# ADR-032 — BC Recepção: Marcação e agenda por disponibilidade

- **Estado:** Aceite
- **Data:** 2026-07-15
- **Marco/Sprint:** Marco "Percurso Ambulatório" / sub-projecto Marcação
- **Fontes:** design em `docs/superpowers/specs/2026-07-15-recepcao-marcacao-design.md`; DDM-001 (papéis); ADR-031 (precedente da ACL do Laboratório).

## Contexto

O `EpisodioClinico` nasce diretamente em ABERTO, assumindo que o doente já está à
frente do médico. Faltava modelar o percurso ambulatório antes da consulta: marcação,
recepção/check-in e triagem. Este marco abre esse percurso; este primeiro sub-projecto
entrega a **Marcação**.

## Decisão

1. **Novo BC `recepcao`**, com schema PostgreSQL próprio e as 4 camadas Clean, em vez
   de engrossar o BC Clínico. O agendamento administrativo é uma responsabilidade
   distinta do ato clínico. Recepção/check-in e Triagem são sub-projectos futuros do
   mesmo marco.
2. **Dois agregados:** `JanelaDisponibilidade` (agenda declarada por médico, com data
   concreta — sem motor de recorrência, YAGNI) e `Marcacao` (raiz, com a máquina de
   estados MARCADA → CANCELADA/REMARCADA/FALTOU). A chegada (COMPARECEU) pertence ao
   futuro módulo Recepção.
3. **Remarcação por *supersede*** (mesmo padrão da correcção de resultados do
   Laboratório): a original passa a REMARCADA e nasce uma nova MARCADA que a
   referencia (`remarca_de`), preservando o histórico, numa única transacção.
4. **Invariante de disponibilidade como função de domínio pura**
   (`VerificarDisponibilidade`): não no passado, cabe numa janela da especialidade, não
   sobrepõe outra marcação MARCADA. O caso de uso alimenta-a com dados dos repositórios.
5. **Defesa em profundidade na base de dados:** restrição `EXCLUDE USING gist` (com
   `btree_gist`) que nega marcações MARCADA sobrepostas do mesmo médico — o único
   guarda à prova de corridas concorrentes; o adaptador traduz o SQLSTATE 23P01 em
   Conflito (409).
6. **Sem FK cross-context.** Um adaptador ACL `LeitorDoente`
   (`internal/adapters/recepcao/leitor_doente.go`) valida o doente contra o BC Clínico;
   o domínio e a aplicação da Recepção nunca importam `clinico`.
7. **RBAC:** definir/remover janelas e marcar/remarcar/cancelar/registar falta são do
   `Administrativo` (supervisão `Director`/`Admin`); a leitura da agenda é aberta também
   ao `Medico`. O actor é sempre o sujeito autenticado.

## Consequências

- O BC Recepção fica pronto a receber os sub-projectos Recepção/Check-in e Triagem.
- A agenda por data concreta exige criar várias janelas para horários repetidos; a
  recorrência semanal fica para evolução futura, se pedida.
- O episódio clínico continua a nascer em ABERTO; a ligação marcação → episódio (abrir
  a consulta a partir da marcação) será desenhada quando o módulo Recepção existir.
