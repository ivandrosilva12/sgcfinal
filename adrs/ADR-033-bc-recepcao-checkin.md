# ADR-033 — BC Recepção: Check-in e fila de espera

- **Estado:** Aceite
- **Data:** 2026-07-15
- **Marco/Sprint:** Percurso Ambulatório / sub-projecto Check-in
- **Fontes:** design em `docs/superpowers/specs/2026-07-15-recepcao-checkin-design.md`; ADR-032 (Marcação, precedente do mesmo BC).

## Contexto

A Marcação (ADR-032) entrega a agenda e o ciclo da marcação até FALTOU. Faltava a
chegada do doente à clínica e a fila de espera. Este sub-projecto entrega o Check-in.

## Decisão

1. **Novo agregado `Chegada`** no BC `recepcao`, que unifica o check-in de uma marcação
   e o walk-in (sem marcação) numa só entidade e numa só fila. O walk-in não é forçado
   pela invariante de disponibilidade das marcações.
2. **Novo estado `COMPARECEU` na `Marcacao`** (método `RegistarComparencia`, MARCADA→
   COMPARECEU) — desfecho simétrico ao FALTOU; depois de comparecer, a marcação já não
   pode ser cancelada, remarcada nem dar falta.
3. **Check-in de marcação é transaccional e cruza dois agregados:**
   `RepositorioChegadas.RegistarChegadaAgendada` transita a marcação para COMPARECEU
   (guarda compare-and-set sobre MARCADA) e insere a chegada na mesma transacção. O
   check-in duplo falha na guarda CAS (a marcação já não está MARCADA) → Conflito.
   Defesa em profundidade: índice `UNIQUE` parcial sobre `chegadas.marcacao_id`.
4. **Walk-in** valida o doente pela ACL `LeitorDoente` (doente registado, como na
   marcação) e regista a chegada sem marcação nem médico (o médico é atribuído depois).
5. **Fila** = chegadas em `AGUARDA`, ordenadas por hora de chegada (FIFO), filtráveis por
   especialidade. A prioridade clínica fica para a Triagem.
6. **RBAC:** check-in, walk-in e desistência são do `Administrativo` (balcão); chamar o
   próximo (`AGUARDA→CHAMADO`) é de quem atende (`Enfermeiro`/`Médico`) e do
   `Administrativo`; a fila é visível ao pessoal de balcão e clínico. Handler HTTP
   separado do de marcações para manter os construtores enxutos.

## Consequências

- O percurso ambulatório fica pronto para a Triagem (que consumirá os `CHAMADO`).
- A atribuição de médico ao walk-in e a prioridade clínica ficam para a Triagem.
- O check-in aceita qualquer marcação `MARCADA` (a data não é imposta — decisão
  operacional da recepção).
