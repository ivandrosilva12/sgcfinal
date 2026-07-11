package identidade

import (
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

// UtilizadorAutenticado é emitido quando um utilizador autentica com sucesso.
type UtilizadorAutenticado struct {
	Sujeito string
	Em      time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e UtilizadorAutenticado) NomeEvento() string { return "identidade.utilizador.autenticado" }

// OcorridoEm implementa evento.EventoDominio.
func (e UtilizadorAutenticado) OcorridoEm() time.Time { return e.Em }

// PerfilConsultado é emitido quando um utilizador consulta o seu perfil.
type PerfilConsultado struct {
	Sujeito string
	Em      time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e PerfilConsultado) NomeEvento() string { return "identidade.perfil.consultado" }

// OcorridoEm implementa evento.EventoDominio.
func (e PerfilConsultado) OcorridoEm() time.Time { return e.Em }

// AcessoNegado é emitido quando um pedido autenticado é recusado por RBAC.
type AcessoNegado struct {
	Sujeito string
	Recurso string
	Em      time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e AcessoNegado) NomeEvento() string { return "identidade.acesso.negado" }

// OcorridoEm implementa evento.EventoDominio.
func (e AcessoNegado) OcorridoEm() time.Time { return e.Em }

// Garantias de conformidade com a interface de evento de domínio.
var (
	_ evento.EventoDominio = UtilizadorAutenticado{}
	_ evento.EventoDominio = PerfilConsultado{}
	_ evento.EventoDominio = AcessoNegado{}
)
