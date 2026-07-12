package farmacia

import "github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"

// LoteFEFO é um lote candidato à alocação (já ordenado por validade ASC).
type LoteFEFO struct {
	LoteID     string
	Disponivel int
}

// AlocacaoFEFO é a quantidade a retirar de um lote.
type AlocacaoFEFO struct {
	LoteID     string
	Quantidade int
}

// AlocarFEFO aloca `quantidade` a partir dos lotes (já ordenados por validade
// ASC — o mais próximo a expirar primeiro), gulosamente. Devolve RegraNegocio se
// o total disponível não chegar.
func AlocarFEFO(lotes []LoteFEFO, quantidade int) ([]AlocacaoFEFO, error) {
	if quantidade <= 0 {
		return nil, erros.Novo(erros.CategoriaValidacao, "a quantidade a alocar deve ser positiva")
	}
	restante := quantidade
	alocacoes := make([]AlocacaoFEFO, 0, len(lotes))
	for _, l := range lotes {
		if restante == 0 {
			break
		}
		if l.Disponivel <= 0 {
			continue
		}
		usar := l.Disponivel
		if usar > restante {
			usar = restante
		}
		alocacoes = append(alocacoes, AlocacaoFEFO{LoteID: l.LoteID, Quantidade: usar})
		restante -= usar
	}
	if restante > 0 {
		return nil, erros.Novo(erros.CategoriaRegraNegocio, "stock insuficiente para a quantidade pedida")
	}
	return alocacoes, nil
}
