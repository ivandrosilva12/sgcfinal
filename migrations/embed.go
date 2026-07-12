// Package migrations expõe as migrations SQL forward-only embebidas no binário,
// organizadas por bounded context. É consumido pelo runner em
// internal/platform/db. Não contém lógica — apenas o embed.FS.
package migrations

import "embed"

// FS contém todas as migrations, agrupadas por subdirectório (bounded context).
//
//go:embed auditoria clinico identidade shared
var FS embed.FS
