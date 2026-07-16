package laboratorio

import (
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

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

// ResultadoValidado é emitido quando o patologista valida o preliminar. A partir
// daqui o resultado é visível ao médico.
type ResultadoValidado struct {
	ResultadoID   string
	RequisicaoID  string
	CodigoAnalise string
	ValorCritico  bool
	Em            time.Time
}

func (e ResultadoValidado) NomeEvento() string    { return "laboratorio.resultado.validado" }
func (e ResultadoValidado) OcorridoEm() time.Time { return e.Em }

// ValorCriticoDetectado é emitido quando a validação detecta um valor crítico.
type ValorCriticoDetectado struct {
	ResultadoID   string
	RequisicaoID  string
	CodigoAnalise string
	Valor         string
	Em            time.Time
}

func (e ValorCriticoDetectado) NomeEvento() string    { return "laboratorio.valor_critico.detectado" }
func (e ValorCriticoDetectado) OcorridoEm() time.Time { return e.Em }

// ResultadoCorrigido é emitido quando um resultado validado é corrigido: o original
// é arquivado e nasce um novo resultado.
type ResultadoCorrigido struct {
	ResultadoIDOriginal string
	ResultadoIDNovo     string
	RequisicaoID        string
	CodigoAnalise       string
	Em                  time.Time
}

func (e ResultadoCorrigido) NomeEvento() string    { return "laboratorio.resultado.corrigido" }
func (e ResultadoCorrigido) OcorridoEm() time.Time { return e.Em }

var (
	_ evento.EventoDominio = AmostraColhida{}
	_ evento.EventoDominio = AmostraRecusada{}
	_ evento.EventoDominio = ResultadoPreliminarSubmetido{}
	_ evento.EventoDominio = ResultadoValidado{}
	_ evento.EventoDominio = ValorCriticoDetectado{}
	_ evento.EventoDominio = ResultadoCorrigido{}
)
