package financeiro

import dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"

// paraDetalheFactura projecta o agregado para o DTO de resposta. Os totais vêm do
// domínio (fonte autoritária do cálculo de IVA).
func paraDetalheFactura(f *dominio.Factura) DetalheFactura {
	c := f.Cliente()
	tot := f.Totais()
	itens := make([]LinhaDetalhe, 0, len(f.Itens()))
	for _, it := range f.Itens() {
		itens = append(itens, LinhaDetalhe{
			ID: it.ID, Descricao: it.Descricao, Tipo: string(it.Tipo), OperacaoID: it.OperacaoID,
			Quantidade: it.Quantidade, PrecoUnitarioCentimos: it.PrecoUnitario.Centimos(),
			RegimeIVA:        string(it.RegimeIVA),
			SubtotalCentimos: it.Subtotal().Centimos(),
			ValorIVACentimos: it.ValorIVA().Centimos(),
			TotalCentimos:    it.Total().Centimos(),
		})
	}
	return DetalheFactura{
		ID: f.ID(), Estado: string(f.Estado()), ClienteNome: c.Nome, ClienteNIF: c.NIF,
		ClienteMorada: c.Morada, EpisodioID: f.EpisodioID(), Itens: itens,
		SubtotalCentimos: tot.Subtotal.Centimos(), TotalIVACentimos: tot.TotalIVA.Centimos(),
		TotalCentimos: tot.Total.Centimos(), Total: tot.Total.String(),
		CriadoEm: f.CriadoEm(),
	}
}
