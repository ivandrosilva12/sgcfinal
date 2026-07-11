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

// PapelAtribuido é emitido quando um administrador atribui um papel.
type PapelAtribuido struct {
	Actor string
	Alvo  string
	Papel Papel
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e PapelAtribuido) NomeEvento() string { return "identidade.papel.atribuido" }

// OcorridoEm implementa evento.EventoDominio.
func (e PapelAtribuido) OcorridoEm() time.Time { return e.Em }

// PapelRevogado é emitido quando um administrador revoga um papel.
type PapelRevogado struct {
	Actor string
	Alvo  string
	Papel Papel
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e PapelRevogado) NomeEvento() string { return "identidade.papel.revogado" }

// OcorridoEm implementa evento.EventoDominio.
func (e PapelRevogado) OcorridoEm() time.Time { return e.Em }

// UtilizadorActivado é emitido quando um administrador activa uma conta.
type UtilizadorActivado struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e UtilizadorActivado) NomeEvento() string { return "identidade.utilizador.activado" }

// OcorridoEm implementa evento.EventoDominio.
func (e UtilizadorActivado) OcorridoEm() time.Time { return e.Em }

// UtilizadorDesactivado é emitido quando um administrador desactiva uma conta.
type UtilizadorDesactivado struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e UtilizadorDesactivado) NomeEvento() string { return "identidade.utilizador.desactivado" }

// OcorridoEm implementa evento.EventoDominio.
func (e UtilizadorDesactivado) OcorridoEm() time.Time { return e.Em }

// UtilizadorCriado é emitido quando um administrador cria um utilizador.
type UtilizadorCriado struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e UtilizadorCriado) NomeEvento() string { return "identidade.utilizador.criado" }

// OcorridoEm implementa evento.EventoDominio.
func (e UtilizadorCriado) OcorridoEm() time.Time { return e.Em }

// PasswordReposta é emitido quando um administrador repõe a password.
type PasswordReposta struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e PasswordReposta) NomeEvento() string { return "identidade.password.reposta" }

// OcorridoEm implementa evento.EventoDominio.
func (e PasswordReposta) OcorridoEm() time.Time { return e.Em }

// OtpReposto é emitido quando um administrador repõe o OTP.
type OtpReposto struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e OtpReposto) NomeEvento() string { return "identidade.otp.reposto" }

// OcorridoEm implementa evento.EventoDominio.
func (e OtpReposto) OcorridoEm() time.Time { return e.Em }

// SessoesRevogadas é emitido quando as sessões de um utilizador são revogadas.
type SessoesRevogadas struct {
	Actor string
	Alvo  string
	Em    time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e SessoesRevogadas) NomeEvento() string { return "identidade.sessoes.revogadas" }

// OcorridoEm implementa evento.EventoDominio.
func (e SessoesRevogadas) OcorridoEm() time.Time { return e.Em }

// PerfilActualizado é emitido quando um utilizador actualiza o seu perfil.
type PerfilActualizado struct {
	Sujeito string
	Em      time.Time
}

// NomeEvento implementa evento.EventoDominio.
func (e PerfilActualizado) NomeEvento() string { return "identidade.perfil.actualizado" }

// OcorridoEm implementa evento.EventoDominio.
func (e PerfilActualizado) OcorridoEm() time.Time { return e.Em }

// Garantias de conformidade com a interface de evento de domínio.
var (
	_ evento.EventoDominio = UtilizadorAutenticado{}
	_ evento.EventoDominio = PerfilConsultado{}
	_ evento.EventoDominio = AcessoNegado{}
	_ evento.EventoDominio = PapelAtribuido{}
	_ evento.EventoDominio = PapelRevogado{}
	_ evento.EventoDominio = UtilizadorActivado{}
	_ evento.EventoDominio = UtilizadorDesactivado{}
	_ evento.EventoDominio = UtilizadorCriado{}
	_ evento.EventoDominio = PasswordReposta{}
	_ evento.EventoDominio = OtpReposto{}
	_ evento.EventoDominio = SessoesRevogadas{}
	_ evento.EventoDominio = PerfilActualizado{}
)
