package clinico

import dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"

// construirNota converte o DTO de nota clínica no VO de domínio.
func construirNota(d DadosNotaClinica) dominio.NotaClinica {
	return dominio.NovaNotaClinica(d.QueixaPrincipal, d.HistoriaDoenca, d.ExameObjectivo, d.Diagnostico, d.Plano)
}

// construirDiagnosticos converte os DTOs de diagnóstico nos VOs de domínio (sem
// validar aqui — a validação vive em EpisodioClinico.DefinirDiagnosticosCID).
func construirDiagnosticos(ds []DadosDiagnosticoCID) []dominio.DiagnosticoCID {
	out := make([]dominio.DiagnosticoCID, 0, len(ds))
	for _, d := range ds {
		out = append(out, dominio.DiagnosticoCID{CID: d.CID, Principal: d.Principal})
	}
	return out
}

// paraDetalheEpisodio mapeia um agregado EpisodioClinico para o DTO de detalhe.
func paraDetalheEpisodio(e *dominio.EpisodioClinico) DetalheEpisodio {
	s := e.Snapshot()
	det := DetalheEpisodio{
		ID:              s.ID,
		DoenteID:        s.DoenteID,
		Tipo:            string(s.Tipo),
		EspecialidadeID: s.EspecialidadeID,
		MedicoID:        s.MedicoID,
		Inicio:          s.Inicio,
		Fim:             s.Fim,
		Nota: NotaClinicaDTO{
			QueixaPrincipal: s.Nota.QueixaPrincipal,
			HistoriaDoenca:  s.Nota.HistoriaDoenca,
			ExameObjectivo:  s.Nota.ExameObjectivo,
			Diagnostico:     s.Nota.Diagnostico,
			Plano:           s.Nota.Plano,
		},
		DiagnosticosCID: []DiagnosticoCIDDTO{},
		Estado:          string(s.Estado),
		CriadoEm:        s.CriadoEm,
		ActualizadoEm:   s.ActualizadoEm,
		FechadoEm:       s.FechadoEm,
		FechadoPor:      s.FechadoPor,
	}
	for _, c := range s.DiagnosticosCID {
		det.DiagnosticosCID = append(det.DiagnosticosCID, DiagnosticoCIDDTO{CID: c.CID, Principal: c.Principal})
	}
	return det
}
