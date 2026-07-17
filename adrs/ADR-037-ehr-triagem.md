# ADR-037 — EHR: triagem por leitura ACL com filtragem por papel

- **Estado:** Aceite
- **Data:** 2026-07-17
- **Marco/Sprint:** Integração Percurso Ambulatório → Clínico, 2.ª fatia (fecha o
  último diferimento da ADR-034)
- **Fontes:** design em `docs/superpowers/specs/2026-07-17-ehr-triagem-design.md`;
  ADR-034 (Triagem), ADR-036 (início da consulta — a ponte `episodio_id`).

## Contexto

A triagem regista a prioridade de Manchester e os sinais vitais (ADR-034), e o início
da consulta liga a chegada ao episódio (`recepcao.chegadas.episodio_id`, ADR-036) —
mas o médico que abria o EHR não via nada disso. Faltava expor a triagem na projecção
clínica sem violar a minimização LPDP da ADR-034 (leitura da triagem restrita a
Médico/Enfermeiro/Director, enquanto o EHR é legível por mais papéis).

## Decisão

1. **Leitura via ACL, não snapshot.** Porta `LeitorTriagem` na aplicação do Clínico
   (DTOs próprios — `TriagemDoEpisodio`, `SinaisVitaisDTO`); o adaptador é a peça de
   integração existente (`pgrepo.IntegracaoInicioConsulta`), com
   `JOIN recepcao.chegadas ⋈ recepcao.triagens` por `episodio_id`. Fonte única de
   verdade na Recepção; zero migrações; zero alterações ao BC Recepção. Rejeitado:
   snapshot na criação do episódio (duplicaria dado clínico, exigiria migração e
   alargaria a transacção da ADR-036).
2. **Filtragem por papel na projecção.** A triagem só entra na resposta quando o actor
   tem papel Médico/Enfermeiro/Director (literais do BC Identidade, guardados como
   `[]string` — a Camada 2 do Clínico não importa Identidade). Sem papel, a resposta é
   a de sempre e a porta nem é invocada.
3. **Superfície:** `GET /episodios/:eid` ganha o bloco `triagem` completo (prioridade,
   9 sinais vitais, observações, enfermeiro, instante); os resumos de episódio (EHR e
   listagem) ganham só `prioridade_triagem` (leitura em lote). Episódios que não
   nasceram da fila ficam como hoje (sem bloco, sem erro).
4. **Falha franca:** erro do leitor propaga (500) — nunca degradar silenciosamente
   para "sem triagem".

## Consequências

- O percurso ambulatório fica clinicamente visível de ponta a ponta: a triagem que
  originou a consulta acompanha o episódio no EHR.
- O campo `ResumoEpisodio.PrioridadeTriagem` é preenchido pela aplicação (ACL), nunca
  pelo repositório do Clínico — a fronteira BC mantém-se.
- Correcção futura da triagem (hoje imutável) propagaria automaticamente ao EHR.
- Diferimentos: tendências de sinais vitais; sinais medidos durante a consulta;
  eventos via Outbox (ADR-038).
