package clinico

import "context"

// papeisLeituraTriagem são os papéis que veem a triagem na projecção clínica
// (minimização LPDP, ADR-034/ADR-037): Médico, Enfermeiro e Director. Literais
// iguais aos códigos do BC Identidade — a Camada 2 do Clínico não o importa.
var papeisLeituraTriagem = map[string]bool{
	"Medico": true, "Enfermeiro": true, "Director": true,
}

// temPapelLeituraTriagem indica se algum papel do actor autoriza ver a triagem.
func temPapelLeituraTriagem(papeis []string) bool {
	for _, p := range papeis {
		if papeisLeituraTriagem[p] {
			return true
		}
	}
	return false
}

// preencherPrioridadesTriagem anota os resumos com a cor de Manchester da
// triagem de origem, quando o actor a pode ver. Ids sem triagem ficam vazios.
func preencherPrioridadesTriagem(ctx context.Context, leitor LeitorTriagem, papeis []string, itens []ResumoEpisodio) error {
	if len(itens) == 0 || !temPapelLeituraTriagem(papeis) {
		return nil
	}
	ids := make([]string, 0, len(itens))
	for _, it := range itens {
		ids = append(ids, it.ID)
	}
	prioridades, err := leitor.PrioridadesDosEpisodios(ctx, ids)
	if err != nil {
		return err
	}
	for i := range itens {
		itens[i].PrioridadeTriagem = prioridades[itens[i].ID]
	}
	return nil
}
