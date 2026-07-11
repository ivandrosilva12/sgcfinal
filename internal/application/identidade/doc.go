// Package identidade contém os casos de uso do Bounded Context Identidade
// (Camada 2 — Aplicação). Importa apenas o Domínio.
//
// Casos de uso: CasoAutenticar (valida o token OIDC, sem I/O) e CasoObterPerfil
// (JIT provisioning + auditoria de acesso + leitura do perfil). As portas de
// saída (VerificadorToken, Auditor) e a interface de repositório do domínio são
// implementadas pela camada de adaptadores.
package identidade
