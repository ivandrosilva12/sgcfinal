package farmacia

// EstadoReceita é o estado do ciclo de vida de uma receita (DDM-001).
type EstadoReceita string

const (
	ReceitaEmitida    EstadoReceita = "EMITIDA"
	ReceitaParcial    EstadoReceita = "PARCIAL"
	ReceitaDispensada EstadoReceita = "DISPENSADA"
	ReceitaExpirada   EstadoReceita = "EXPIRADA"
	ReceitaAnulada    EstadoReceita = "ANULADA"
)
