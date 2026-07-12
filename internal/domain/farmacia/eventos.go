package farmacia

import (
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

// MedicamentoRegistado é emitido quando um medicamento é adicionado ao catálogo.
type MedicamentoRegistado struct {
	MedicamentoID string
	Em            time.Time
}

func (e MedicamentoRegistado) NomeEvento() string    { return "farmacia.medicamento.registado" }
func (e MedicamentoRegistado) OcorridoEm() time.Time { return e.Em }

// EventoReceitaEmitida é emitido quando uma receita é emitida.
// Nota: nomeado com o prefixo "Evento" (e não "ReceitaEmitida") para evitar
// colidir com a constante farmacia.ReceitaEmitida de EstadoReceita (enums.go);
// o pacote não compilaria com dois identificadores "ReceitaEmitida".
type EventoReceitaEmitida struct {
	ReceitaID string
	DoenteID  string
	Em        time.Time
}

func (e EventoReceitaEmitida) NomeEvento() string    { return "farmacia.receita.emitida" }
func (e EventoReceitaEmitida) OcorridoEm() time.Time { return e.Em }

// EventoReceitaAnulada é emitido quando uma receita é anulada.
// Nota: mesmo motivo do prefixo "Evento" que em EventoReceitaEmitida, para não
// colidir com a constante farmacia.ReceitaAnulada de EstadoReceita.
type EventoReceitaAnulada struct {
	ReceitaID string
	Em        time.Time
}

func (e EventoReceitaAnulada) NomeEvento() string    { return "farmacia.receita.anulada" }
func (e EventoReceitaAnulada) OcorridoEm() time.Time { return e.Em }

// EventoReceitaDispensada é emitido quando uma receita é (parcial ou totalmente) dispensada.
type EventoReceitaDispensada struct {
	ReceitaID string
	Em        time.Time
}

func (e EventoReceitaDispensada) NomeEvento() string    { return "farmacia.receita.dispensada" }
func (e EventoReceitaDispensada) OcorridoEm() time.Time { return e.Em }

// StockEntrado é emitido quando entra um lote de stock.
type StockEntrado struct {
	LoteID        string
	MedicamentoID string
	Em            time.Time
}

func (e StockEntrado) NomeEvento() string    { return "farmacia.stock.entrada" }
func (e StockEntrado) OcorridoEm() time.Time { return e.Em }

// Garantias de conformidade com a interface de evento de domínio.
var (
	_ evento.EventoDominio = MedicamentoRegistado{}
	_ evento.EventoDominio = EventoReceitaEmitida{}
	_ evento.EventoDominio = EventoReceitaAnulada{}
	_ evento.EventoDominio = EventoReceitaDispensada{}
	_ evento.EventoDominio = StockEntrado{}
)
