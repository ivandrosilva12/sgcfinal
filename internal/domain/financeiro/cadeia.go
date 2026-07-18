package financeiro

import (
	"fmt"
	"sort"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// VerificarCadeia confirma a integridade de uma série de facturas emitidas.
// Função pura: recebe os snapshots, ordena-os por sequencial e verifica três
// propriedades, devolvendo o PRIMEIRO problema encontrado —
//
//  1. a numeração é contígua desde 1 (sem buracos, REG-001 §3.2);
//  2. o hash de cada factura corresponde ao recálculo do seu conteúdo;
//  3. o hashAnterior de cada uma é o hash da que a precede.
//
// É esta função que torna a "quebra detectável" do REG-001 §3.2 verificável, e
// é sobre ela que assentará o cron diário do §3.4.
func VerificarCadeia(facturas []SnapshotFactura) error {
	if len(facturas) == 0 {
		return nil
	}
	ordenadas := make([]SnapshotFactura, len(facturas))
	copy(ordenadas, facturas)
	sort.Slice(ordenadas, func(i, j int) bool {
		return ordenadas[i].Sequencial < ordenadas[j].Sequencial
	})

	anterior := ""
	for i, f := range ordenadas {
		esperado := i + 1
		if f.Sequencial != esperado {
			return erros.Novo(erros.CategoriaRegraNegocio, fmt.Sprintf(
				"buraco na série %s: esperava o sequencial %08d e encontrou %08d",
				f.Serie, esperado, f.Sequencial))
		}
		if f.HashAnterior != anterior {
			return erros.Novo(erros.CategoriaRegraNegocio,
				"cadeia quebrada na factura "+f.Numero.String()+
					": o elo anterior não corresponde à factura que a precede")
		}
		if recalculado := HashDe(f); recalculado != f.Hash {
			return erros.Novo(erros.CategoriaRegraNegocio,
				"cadeia quebrada na factura "+f.Numero.String()+
					": o conteúdo não corresponde ao hash registado")
		}
		anterior = f.Hash
	}
	return nil
}
