package farmacia

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// ItemReceita é uma linha de prescrição (medicamento + posologia + quantidade).
// Campos exportados; construído por NovoItemReceita, que valida.
type ItemReceita struct {
	MedicamentoID        string
	Posologia            string
	DuracaoDias          *int
	QuantidadePrescrita  int
	QuantidadeDispensada int
	Notas                string
}

// NovoItemReceita valida e constrói um item. MedicamentoID e posologia não-vazios;
// quantidade prescrita > 0; quantidade dispensada inicial 0.
func NovoItemReceita(medicamentoID, posologia string, duracaoDias *int, quantidadePrescrita int, notas string) (ItemReceita, error) {
	medicamentoID = strings.TrimSpace(medicamentoID)
	if medicamentoID == "" {
		return ItemReceita{}, erros.Novo(erros.CategoriaValidacao, "medicamento do item da receita em falta")
	}
	posologia = strings.TrimSpace(posologia)
	if posologia == "" {
		return ItemReceita{}, erros.Novo(erros.CategoriaValidacao, "posologia do item da receita em falta")
	}
	if quantidadePrescrita <= 0 {
		return ItemReceita{}, erros.Novo(erros.CategoriaValidacao, "quantidade prescrita deve ser positiva")
	}
	return ItemReceita{
		MedicamentoID:        medicamentoID,
		Posologia:            posologia,
		DuracaoDias:          duracaoDias,
		QuantidadePrescrita:  quantidadePrescrita,
		QuantidadeDispensada: 0,
		Notas:                strings.TrimSpace(notas),
	}, nil
}

// Receita é o agregado raiz da prescrição, emitida num episódio clínico.
type Receita struct {
	id         string
	episodioID string
	doenteID   string
	medicoID   string
	emitidaEm  time.Time
	estado     EstadoReceita
	notas      string
	expiraEm   time.Time
	itens      []ItemReceita
}

// NovaReceita valida e constrói uma receita no estado EMITIDA. Os três ids são
// obrigatórios; exige pelo menos um item; expiraEm posterior a emitidaEm.
func NovaReceita(episodioID, doenteID, medicoID string, itens []ItemReceita, notas string, emitidaEm, expiraEm time.Time) (*Receita, error) {
	episodioID = strings.TrimSpace(episodioID)
	doenteID = strings.TrimSpace(doenteID)
	medicoID = strings.TrimSpace(medicoID)
	if episodioID == "" || doenteID == "" || medicoID == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "episódio, doente e médico da receita são obrigatórios")
	}
	if len(itens) == 0 {
		return nil, erros.Novo(erros.CategoriaValidacao, "a receita tem de ter pelo menos um item")
	}
	for _, it := range itens {
		if _, err := NovoItemReceita(it.MedicamentoID, it.Posologia, it.DuracaoDias, it.QuantidadePrescrita, it.Notas); err != nil {
			return nil, err
		}
	}
	if !expiraEm.After(emitidaEm) {
		return nil, erros.Novo(erros.CategoriaValidacao, "a data de expiração tem de ser posterior à emissão")
	}
	return &Receita{
		episodioID: episodioID,
		doenteID:   doenteID,
		medicoID:   medicoID,
		emitidaEm:  emitidaEm,
		estado:     ReceitaEmitida,
		notas:      strings.TrimSpace(notas),
		expiraEm:   expiraEm,
		itens:      itens,
	}, nil
}

// ID/DoenteID/Estado — getters.
func (r *Receita) ID() string            { return r.id }
func (r *Receita) DoenteID() string      { return r.doenteID }
func (r *Receita) Estado() EstadoReceita { return r.estado }

// Anular passa a receita a ANULADA. Só de EMITIDA/PARCIAL.
func (r *Receita) Anular() error {
	if r.estado != ReceitaEmitida && r.estado != ReceitaParcial {
		return erros.Novo(erros.CategoriaConflito, "só é possível anular uma receita emitida ou parcial")
	}
	r.estado = ReceitaAnulada
	return nil
}

// RegistarDispensa regista a dispensa de `quantidade` de um medicamento da
// receita: valida que não excede o prescrito (cumulativamente) e recalcula o
// estado (DISPENSADA se tudo dispensado, senão PARCIAL). Só de EMITIDA/PARCIAL.
func (r *Receita) RegistarDispensa(medicamentoID string, quantidade int) error {
	if r.estado != ReceitaEmitida && r.estado != ReceitaParcial {
		return erros.Novo(erros.CategoriaConflito, "só é possível dispensar uma receita emitida ou parcial")
	}
	if quantidade <= 0 {
		return erros.Novo(erros.CategoriaValidacao, "a quantidade a dispensar deve ser positiva")
	}
	for i := range r.itens {
		if r.itens[i].MedicamentoID == medicamentoID {
			if r.itens[i].QuantidadeDispensada+quantidade > r.itens[i].QuantidadePrescrita {
				return erros.Novo(erros.CategoriaRegraNegocio, "a quantidade a dispensar excede a prescrita")
			}
			r.itens[i].QuantidadeDispensada += quantidade
			r.recalcularEstadoDispensa()
			return nil
		}
	}
	return erros.Novo(erros.CategoriaValidacao, "o medicamento não consta da receita")
}

// recalcularEstadoDispensa põe a receita em DISPENSADA se todos os itens estão
// totalmente dispensados, senão em PARCIAL.
func (r *Receita) recalcularEstadoDispensa() {
	for _, it := range r.itens {
		if it.QuantidadeDispensada < it.QuantidadePrescrita {
			r.estado = ReceitaParcial
			return
		}
	}
	r.estado = ReceitaDispensada
}

// EstadoEfectivoReceita devolve o estado tendo em conta a expiração: se
// EMITIDA/PARCIAL e a data de expiração já passou (comparação por dia), devolve
// EXPIRADA; caso contrário o estado indicado.
func EstadoEfectivoReceita(estado EstadoReceita, expiraEm, agora time.Time) EstadoReceita {
	if (estado == ReceitaEmitida || estado == ReceitaParcial) && agora.Truncate(24*time.Hour).After(expiraEm.Truncate(24*time.Hour)) {
		return ReceitaExpirada
	}
	return estado
}

// EstadoEfectivo devolve o estado tendo em conta a expiração: se EMITIDA/PARCIAL e
// a data de expiração já passou, devolve EXPIRADA (não persistido — calculado na
// leitura). Compara por data.
func (r *Receita) EstadoEfectivo(agora time.Time) EstadoReceita {
	return EstadoEfectivoReceita(r.estado, r.expiraEm, agora)
}

// SnapshotReceita carrega o estado completo para persistência/rehidratação.
type SnapshotReceita struct {
	ID         string
	EpisodioID string
	DoenteID   string
	MedicoID   string
	EmitidaEm  time.Time
	Estado     EstadoReceita
	Notas      string
	ExpiraEm   time.Time
	Itens      []ItemReceita
}

// Snapshot devolve o estado completo do agregado.
func (r *Receita) Snapshot() SnapshotReceita {
	return SnapshotReceita{
		ID: r.id, EpisodioID: r.episodioID, DoenteID: r.doenteID, MedicoID: r.medicoID,
		EmitidaEm: r.emitidaEm, Estado: r.estado, Notas: r.notas, ExpiraEm: r.expiraEm, Itens: r.itens,
	}
}

// ReconstruirReceita reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirReceita(s SnapshotReceita) *Receita {
	return &Receita{
		id: s.ID, episodioID: s.EpisodioID, doenteID: s.DoenteID, medicoID: s.MedicoID,
		emitidaEm: s.EmitidaEm, estado: s.Estado, notas: s.Notas, expiraEm: s.ExpiraEm, itens: s.Itens,
	}
}
