package clinico

import "strings"

// NotaClinica é o Value Object da nota clínica estruturada de um episódio
// (queixa, história, exame, diagnóstico, plano). Pode estar incompleta enquanto
// o episódio está ABERTO; o fecho exige-a completa.
type NotaClinica struct {
	QueixaPrincipal string
	HistoriaDoenca  string
	ExameObjectivo  string
	Diagnostico     string
	Plano           string
}

// NovaNotaClinica constrói a nota, aparando espaços. Não impõe obrigatoriedade.
func NovaNotaClinica(queixa, historia, exame, diagnostico, plano string) NotaClinica {
	return NotaClinica{
		QueixaPrincipal: strings.TrimSpace(queixa),
		HistoriaDoenca:  strings.TrimSpace(historia),
		ExameObjectivo:  strings.TrimSpace(exame),
		Diagnostico:     strings.TrimSpace(diagnostico),
		Plano:           strings.TrimSpace(plano),
	}
}

// Completa indica se a nota tem os campos obrigatórios para fechar um episódio:
// queixa principal, exame objectivo, diagnóstico e plano (história é opcional).
func (n NotaClinica) Completa() bool {
	return n.QueixaPrincipal != "" && n.ExameObjectivo != "" && n.Diagnostico != "" && n.Plano != ""
}
