package laboratorio

import "time"

// AmostraColhida é emitido quando o técnico regista a colheita.
type AmostraColhida struct {
	ResultadoID   string
	RequisicaoID  string
	CodigoAnalise string
	Em            time.Time
}

func (e AmostraColhida) NomeEvento() string    { return "laboratorio.amostra.colhida" }
func (e AmostraColhida) OcorridoEm() time.Time { return e.Em }

// AmostraRecusada é emitido quando a amostra é recusada por inviabilidade.
type AmostraRecusada struct {
	ResultadoID  string
	RequisicaoID string
	Motivo       string
	Em           time.Time
}

func (e AmostraRecusada) NomeEvento() string    { return "laboratorio.amostra.recusada" }
func (e AmostraRecusada) OcorridoEm() time.Time { return e.Em }

// ResultadoPreliminarSubmetido é emitido quando o técnico submete o preliminar. O
// resultado ainda NÃO é visível ao médico — só a validação (Sprint 13) o torna.
type ResultadoPreliminarSubmetido struct {
	ResultadoID   string
	RequisicaoID  string
	CodigoAnalise string
	Em            time.Time
}

func (e ResultadoPreliminarSubmetido) NomeEvento() string {
	return "laboratorio.resultado.preliminar_submetido"
}
func (e ResultadoPreliminarSubmetido) OcorridoEm() time.Time { return e.Em }
