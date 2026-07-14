package laboratorio

import (
	"context"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
)

// CasoListarFila lista a fila de trabalho do laboratório (técnico e patologista).
// Vê todos os estados: é a fila de quem executa o trabalho.
type CasoListarFila struct {
	resultados dominio.RepositorioResultados
}

// NovoCasoListarFila constrói o caso de uso.
func NovoCasoListarFila(r dominio.RepositorioResultados) *CasoListarFila {
	return &CasoListarFila{resultados: r}
}

// Executar devolve a fila; uma lista de estados vazia devolve todos.
func (uc *CasoListarFila) Executar(ctx context.Context, estados []dominio.EstadoResultado) ([]ResumoResultado, error) {
	return uc.resultados.ListarFila(ctx, estados)
}

// CasoListarResultadosDoEpisodio é a leitura clínica dos resultados de um episódio.
//
// Impõe a regra de visibilidade do marco: o resultado preliminar (PROCESSADA) NÃO é
// visível ao médico — só o que o patologista validou. A regra vive aqui, e não no
// RBAC de rota, porque o RBAC não chegaria: o médico tem de poder ver os validados,
// pelo que a distinção é pelo estado, não pelo papel. Enquanto a validação não
// existir (Sprint 13), esta listagem devolve vazio — e é isso que se espera.
type CasoListarResultadosDoEpisodio struct {
	resultados dominio.RepositorioResultados
}

// NovoCasoListarResultadosDoEpisodio constrói o caso de uso.
func NovoCasoListarResultadosDoEpisodio(r dominio.RepositorioResultados) *CasoListarResultadosDoEpisodio {
	return &CasoListarResultadosDoEpisodio{resultados: r}
}

// Executar devolve apenas os resultados visíveis ao médico.
func (uc *CasoListarResultadosDoEpisodio) Executar(ctx context.Context, episodioID string) ([]ResumoResultado, error) {
	return uc.resultados.ListarPorEpisodio(ctx, episodioID, EstadosVisiveisAoMedico)
}
