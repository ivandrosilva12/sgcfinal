package laboratorio

import dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"

// paraDetalheAnalise projecta o agregado do catálogo num DTO de resposta.
func paraDetalheAnalise(a *dominio.Analise) DetalheAnalise {
	s := a.Snapshot()
	return DetalheAnalise{
		Codigo: s.Codigo, Nome: s.Nome, Unidade: s.Unidade,
		Intervalos: s.Intervalos, ValoresCriticos: s.ValoresCriticos, Activo: s.Activo,
	}
}

// paraDetalheRequisicao projecta a requisição num DTO de resposta.
func paraDetalheRequisicao(r *dominio.RequisicaoLab) DetalheRequisicao {
	s := r.Snapshot()
	return DetalheRequisicao{
		ID: s.ID, EpisodioID: s.EpisodioID, DoenteID: s.DoenteID,
		MedicoRequisitanteID: s.MedicoRequisitanteID, Prioridade: string(s.Prioridade),
		Estado: string(s.Estado), Itens: s.Itens, CriadoEm: s.CriadoEm,
	}
}

// paraDetalheResultado projecta o resultado num DTO de resposta.
func paraDetalheResultado(r *dominio.Resultado) DetalheResultado {
	s := r.Snapshot()
	return DetalheResultado{
		ID: s.ID, RequisicaoID: s.RequisicaoID, CodigoAnalise: s.CodigoAnalise,
		Valor: s.Valor, Unidade: s.Unidade, Observacoes: s.Observacoes,
		MotivoRecusa: s.MotivoRecusa, Estado: string(s.Estado),
		TecnicoSubmissorID: s.TecnicoSubmissorID,
		ColhidaEm:          s.ColhidaEm, SubmetidaEm: s.SubmetidaEm, ValorCritico: s.ValorCritico,
	}
}
