package clinico

import (
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/evento"
)

// DoenteRegistado é emitido quando um doente é registado.
type DoenteRegistado struct {
	DoenteID string
	Em       time.Time
}

func (e DoenteRegistado) NomeEvento() string    { return "clinico.doente.registado" }
func (e DoenteRegistado) OcorridoEm() time.Time { return e.Em }

// DoenteDesactivado é emitido quando um doente é desactivado.
type DoenteDesactivado struct {
	DoenteID string
	Em       time.Time
}

func (e DoenteDesactivado) NomeEvento() string    { return "clinico.doente.desactivado" }
func (e DoenteDesactivado) OcorridoEm() time.Time { return e.Em }

// DoenteFalecido é emitido quando um doente é declarado falecido.
type DoenteFalecido struct {
	DoenteID string
	Em       time.Time
}

func (e DoenteFalecido) NomeEvento() string    { return "clinico.doente.falecido" }
func (e DoenteFalecido) OcorridoEm() time.Time { return e.Em }

// AlergiaRegistada é emitido quando uma alergia é registada num doente.
type AlergiaRegistada struct {
	DoenteID string
	Em       time.Time
}

func (e AlergiaRegistada) NomeEvento() string    { return "clinico.alergia.registada" }
func (e AlergiaRegistada) OcorridoEm() time.Time { return e.Em }

// Garantias de conformidade com a interface de evento de domínio.
var (
	_ evento.EventoDominio = DoenteRegistado{}
	_ evento.EventoDominio = DoenteDesactivado{}
	_ evento.EventoDominio = DoenteFalecido{}
	_ evento.EventoDominio = AlergiaRegistada{}
)
