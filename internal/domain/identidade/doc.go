// Package identidade é o Bounded Context de Identidade (Camada 1 — Domínio).
//
// Contém o agregado Utilizador, o Value Object Sessao (principal autenticado),
// o enum Papel (12 papéis do DDM-001 v2.0 (+ Tesoureiro, ERRATA-002) — ver
// docs/ERRATA-001-papeis.md), as regras RBAC puras, os eventos de domínio e a
// interface de repositório.
//
// Regra de dependência: só depende de si próprio e do Shared Kernel do domínio.
// Não importa infra (pgx, gin, net/http) nem vendors (incluindo uuid) — o
// keycloak_id é representado como string.
package identidade
