package farmacia

import (
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
)

const (
	limiteDefault = 20
	limiteMaximo  = 100
)

// paraDetalheMedicamento mapeia o agregado Medicamento para o DTO de detalhe.
func paraDetalheMedicamento(m *dominio.Medicamento) DetalheMedicamento {
	s := m.Snapshot()
	return DetalheMedicamento{
		ID: s.ID, CodigoInterno: s.CodigoInterno, NomeComercial: s.NomeComercial,
		NomeGenerico: s.NomeGenerico, FormaFarmaceutica: s.FormaFarmaceutica, Dosagem: s.Dosagem,
		ViaAdministracao: s.ViaAdministracao, Fabricante: s.Fabricante, RequerReceita: s.RequerReceita,
		Psicotropico: s.Psicotropico, ClasseATC: s.ClasseATC, StockMinimo: s.StockMinimo,
		Activo: s.Activo, CriadoEm: s.CriadoEm, ActualizadoEm: s.ActualizadoEm,
	}
}

// paraDetalheReceita mapeia o agregado Receita para o DTO, com o estado efectivo
// (considera a expiração calculada em `agora`).
func paraDetalheReceita(r *dominio.Receita, agora time.Time) DetalheReceita {
	s := r.Snapshot()
	det := DetalheReceita{
		ID: s.ID, EpisodioID: s.EpisodioID, DoenteID: s.DoenteID, MedicoID: s.MedicoID,
		EmitidaEm: s.EmitidaEm, Estado: string(r.EstadoEfectivo(agora)), Notas: s.Notas,
		ExpiraEm: s.ExpiraEm, Itens: []ItemReceitaDTO{},
	}
	for _, it := range s.Itens {
		det.Itens = append(det.Itens, ItemReceitaDTO{
			MedicamentoID: it.MedicamentoID, Posologia: it.Posologia, DuracaoDias: it.DuracaoDias,
			QuantidadePrescrita: it.QuantidadePrescrita, QuantidadeDispensada: it.QuantidadeDispensada, Notas: it.Notas,
		})
	}
	return det
}

// paraDetalheFornecedor mapeia o agregado Fornecedor para o DTO.
func paraDetalheFornecedor(f *dominio.Fornecedor) DetalheFornecedor {
	s := f.Snapshot()
	return DetalheFornecedor{ID: s.ID, Nome: s.Nome, NIF: s.NIF, Contacto: s.Contacto, Activo: s.Activo, CriadoEm: s.CriadoEm}
}

// paraDetalheLote mapeia o agregado Lote para o DTO.
func paraDetalheLote(l *dominio.Lote) DetalheLote {
	s := l.Snapshot()
	return DetalheLote{
		ID: s.ID, MedicamentoID: s.MedicamentoID, NumeroLote: s.NumeroLote, Validade: s.Validade,
		QuantidadeInicial: s.QuantidadeInicial, QuantidadeActual: s.QuantidadeActual,
		PrecoUnitarioCusto: s.PrecoUnitarioCusto, FornecedorID: s.FornecedorID, EntradaEm: s.EntradaEm, Notas: s.Notas,
	}
}
