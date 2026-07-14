// Package laboratorio é o Bounded Context de Laboratório (Camada 1 — Domínio).
//
// Agregados: Analise (catálogo, com intervalos de referência e valores críticos em
// jsonb), RequisicaoLab (episódio + doente por id, sem FK cross-context — a ACL vive
// em internal/adapters/laboratorio) e Resultado (um por análise pedida, state machine
// PENDENTE → COLHIDA → PROCESSADA, com RECUSADA como saída; VALIDADA/CONCLUIDA e a
// segregação de funções submissor≠validador ficam para o Sprint 13). Ver ADR-031.
package laboratorio
