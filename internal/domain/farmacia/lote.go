package farmacia

import (
	"regexp"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// formatoPreco valida um decimal não-negativo com até 4 casas (NUMERIC(14,4)).
var formatoPreco = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,4})?$`)

// Lote é o agregado de um lote de stock de um medicamento.
type Lote struct {
	id                 string
	medicamentoID      string
	numeroLote         string
	validade           time.Time
	quantidadeInicial  int
	quantidadeActual   int
	precoUnitarioCusto string
	fornecedorID       *string
	entradaEm          time.Time
	notas              string
}

// NovoLote valida e constrói um lote. Medicamento e número obrigatórios;
// quantidade > 0 (RN-FAR-02); validade futura (RN-FAR-01); preço decimal ≥ 0.
func NovoLote(medicamentoID, numeroLote string, validade time.Time, quantidade int, precoUnitarioCusto string, fornecedorID *string, notas string) (*Lote, error) {
	medicamentoID = strings.TrimSpace(medicamentoID)
	numeroLote = strings.TrimSpace(numeroLote)
	if medicamentoID == "" || numeroLote == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "medicamento e número de lote são obrigatórios")
	}
	if quantidade <= 0 {
		return nil, erros.Novo(erros.CategoriaValidacao, "a quantidade do lote deve ser positiva")
	}
	if !validade.After(time.Now()) {
		return nil, erros.Novo(erros.CategoriaValidacao, "a validade do lote tem de ser futura")
	}
	preco := strings.TrimSpace(precoUnitarioCusto)
	if !formatoPreco.MatchString(preco) {
		return nil, erros.Novo(erros.CategoriaValidacao, "preço unitário inválido (decimal não-negativo, até 4 casas)")
	}
	return &Lote{
		medicamentoID: medicamentoID, numeroLote: numeroLote, validade: validade,
		quantidadeInicial: quantidade, quantidadeActual: quantidade, precoUnitarioCusto: preco,
		fornecedorID: normalizarOpcional(fornecedorID), notas: strings.TrimSpace(notas),
	}, nil
}

func (l *Lote) ID() string            { return l.id }
func (l *Lote) MedicamentoID() string { return l.medicamentoID }
func (l *Lote) QuantidadeActual() int { return l.quantidadeActual }

// Disponivel indica se o lote tem stock e ainda está válido.
func (l *Lote) Disponivel(agora time.Time) bool {
	return l.quantidadeActual > 0 && agora.Before(l.validade)
}

// SnapshotLote carrega o estado completo para persistência/rehidratação.
type SnapshotLote struct {
	ID                 string
	MedicamentoID      string
	NumeroLote         string
	Validade           time.Time
	QuantidadeInicial  int
	QuantidadeActual   int
	PrecoUnitarioCusto string
	FornecedorID       *string
	EntradaEm          time.Time
	Notas              string
}

func (l *Lote) Snapshot() SnapshotLote {
	return SnapshotLote{
		ID: l.id, MedicamentoID: l.medicamentoID, NumeroLote: l.numeroLote, Validade: l.validade,
		QuantidadeInicial: l.quantidadeInicial, QuantidadeActual: l.quantidadeActual,
		PrecoUnitarioCusto: l.precoUnitarioCusto, FornecedorID: l.fornecedorID, EntradaEm: l.entradaEm, Notas: l.notas,
	}
}

func ReconstruirLote(s SnapshotLote) *Lote {
	return &Lote{
		id: s.ID, medicamentoID: s.MedicamentoID, numeroLote: s.NumeroLote, validade: s.Validade,
		quantidadeInicial: s.QuantidadeInicial, quantidadeActual: s.QuantidadeActual,
		precoUnitarioCusto: s.PrecoUnitarioCusto, fornecedorID: s.FornecedorID, entradaEm: s.EntradaEm, notas: s.Notas,
	}
}
