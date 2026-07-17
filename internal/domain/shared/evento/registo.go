package evento

// RegistoEventos é um mixin embutível por agregados que emitem eventos de
// domínio. Acumula os eventos ocorridos durante um comportamento; o adaptador
// de persistência drena-os (EventosPendentes) e escreve-os no Outbox na mesma
// transacção da mudança de estado. Camada 1 — sem infra.
type RegistoEventos struct {
	pendentes []EventoDominio
}

// RegistarEvento acrescenta um evento à lista pendente.
func (r *RegistoEventos) RegistarEvento(e EventoDominio) {
	r.pendentes = append(r.pendentes, e)
}

// EventosPendentes devolve os eventos ainda não persistidos, pela ordem de
// ocorrência.
func (r *RegistoEventos) EventosPendentes() []EventoDominio {
	return r.pendentes
}

// LimparEventos esvazia a lista pendente (após persistência).
func (r *RegistoEventos) LimparEventos() {
	r.pendentes = nil
}
