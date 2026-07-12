package clinico

import (
	"strings"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// DiagnosticoCID é um código de diagnóstico CID-10 associado a um episódio. Um
// episódio pode ter vários; no máximo um é o principal.
type DiagnosticoCID struct {
	CID       string
	Principal bool
}

// NovoDiagnosticoCID valida e constrói um DiagnosticoCID (CID não-vazio).
func NovoDiagnosticoCID(cid string, principal bool) (DiagnosticoCID, error) {
	cid = strings.TrimSpace(cid)
	if cid == "" {
		return DiagnosticoCID{}, erros.Novo(erros.CategoriaValidacao, "código CID em falta")
	}
	return DiagnosticoCID{CID: cid, Principal: principal}, nil
}
