package clinico

// EstadoDoente é o estado do ciclo de vida de um doente (DDM-001).
type EstadoDoente string

const (
	EstadoActivo   EstadoDoente = "ACTIVO"
	EstadoInactivo EstadoDoente = "INACTIVO"
	EstadoFalecido EstadoDoente = "FALECIDO"
	EstadoApagado  EstadoDoente = "APAGADO"
)
