package clinico

// EstadoProcedimento é o estado do ciclo de vida de um procedimento cirúrgico.
type EstadoProcedimento string

const (
	ProcAgendado  EstadoProcedimento = "AGENDADO"
	ProcEmCurso   EstadoProcedimento = "EM_CURSO"
	ProcConcluido EstadoProcedimento = "CONCLUIDO"
	ProcCancelado EstadoProcedimento = "CANCELADO"
)
