// internal/domain/recepcao/doc.go

// Package recepcao é o BC Recepção (Camada 1 — Domínio): o percurso ambulatório do
// doente antes da consulta. Este sub-projecto cobre a Marcação — a agenda declarada
// de cada médico (JanelaDisponibilidade) e o agendamento de consultas (Marcacao) com
// o seu ciclo de vida. Não importa infra (pgx/gin/net/http). O check-in (Recepção) e
// a Triagem são sub-projectos futuros.
package recepcao
