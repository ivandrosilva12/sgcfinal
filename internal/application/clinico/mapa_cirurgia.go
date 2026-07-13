package clinico

import dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"

// paraDetalheConsentimento projecta o agregado num DTO de resposta.
func paraDetalheConsentimento(c *dominio.Consentimento) DetalheConsentimento {
	s := c.Snapshot()
	return DetalheConsentimento{
		ID: s.ID, DoenteID: s.DoenteID, Finalidade: string(s.Finalidade),
		Concedido: s.Concedido, DocumentoURL: s.DocumentoURL,
		ConcedidoEm: s.ConcedidoEm, RevogadoEm: s.RevogadoEm,
		Vigente: c.EstaVigente(),
	}
}
