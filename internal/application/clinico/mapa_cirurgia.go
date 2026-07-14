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

// paraDetalheProcedimento projecta o agregado num DTO de resposta.
func paraDetalheProcedimento(p *dominio.ProcedimentoCirurgico) DetalheProcedimento {
	s := p.Snapshot()
	return DetalheProcedimento{
		ID: s.ID, EpisodioID: s.EpisodioID, Codigo: s.Codigo, Descricao: s.Descricao,
		Sala: s.Sala, CirurgiaoID: s.CirurgiaoID, AuxiliarID: s.AuxiliarID,
		Anestesia: string(s.Anestesia), AnestesistaID: s.AnestesistaID,
		ConsentimentoID: s.ConsentimentoID, Inicio: s.Inicio, Fim: s.Fim,
		Complicacoes: s.Complicacoes, Observacoes: s.Observacoes,
		Estado: string(s.Estado), CriadoEm: s.CriadoEm,
	}
}
