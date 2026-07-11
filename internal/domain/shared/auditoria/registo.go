// Package auditoria define o registo de auditoria do Shared Kernel — a
// representação de domínio de um evento append-only. A persistência imutável
// (trigger PG, retenção 10 anos) vive na camada de adaptadores/migrations.
// Camada 1 (Domínio) — sem infra.
package auditoria

import "time"

// Registo representa um evento de auditoria a persistir de forma append-only.
// É imutável por construção: uma vez criado, nunca é alterado.
type Registo struct {
	Actor      string    // keycloak_id ou identificador do actor
	Accao      string    // ex.: "identidade.perfil.consultado"
	Entidade   string    // tipo de entidade afectada
	EntidadeID string    // identificador da entidade afectada
	OcorridoEm time.Time // instante do evento
	Detalhe    string    // metadados adicionais (JSON serializado)
}
