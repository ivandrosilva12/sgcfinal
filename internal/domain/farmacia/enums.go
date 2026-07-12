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

// TipoMovimento classifica um movimento de stock (DDM-001).
type TipoMovimento string

const (
	MovimentoEntrada       TipoMovimento = "ENTRADA"
	MovimentoSaidaDispensa TipoMovimento = "SAIDA_DISPENSA"
	MovimentoSaidaVenda    TipoMovimento = "SAIDA_VENDA"
	MovimentoAjuste        TipoMovimento = "AJUSTE"
	MovimentoExpirado      TipoMovimento = "EXPIRADO"
	MovimentoTransferencia TipoMovimento = "TRANSFERENCIA"
)
