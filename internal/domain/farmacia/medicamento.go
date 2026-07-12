// Package farmacia é o domínio do Bounded Context Farmácia do SGC Angola: o
// catálogo de medicamentos e as receitas/prescrições. Camada 1 (Domínio):
// importa apenas a biblioteca-padrão e o Shared Kernel — sem infra e sem o
// domínio de outros bounded contexts.
package farmacia

import (
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// Medicamento é o agregado raiz do catálogo farmacêutico.
type Medicamento struct {
	id                string
	codigoInterno     string
	nomeComercial     string
	nomeGenerico      string
	formaFarmaceutica string
	dosagem           string
	viaAdministracao  string
	fabricante        string
	requerReceita     bool
	psicotropico      bool
	classeATC         *string
	stockMinimo       int
	activo            bool
	criadoEm          time.Time
	actualizadoEm     time.Time
}

// NovoMedicamento valida e constrói um medicamento activo. codigoInterno, nome
// comercial/genérico, forma, dosagem e via são obrigatórios; stockMinimo ≥ 0.
func NovoMedicamento(codigoInterno, nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante string, requerReceita, psicotropico bool, classeATC *string, stockMinimo int) (*Medicamento, error) {
	m := &Medicamento{
		id:            "",
		codigoInterno: strings.TrimSpace(codigoInterno),
		activo:        true,
	}
	if m.codigoInterno == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "código interno do medicamento em falta")
	}
	if err := m.aplicarCampos(nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante, requerReceita, psicotropico, classeATC, stockMinimo); err != nil {
		return nil, err
	}
	return m, nil
}

// Actualizar revalida e substitui os campos mutáveis do medicamento.
func (m *Medicamento) Actualizar(nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante string, requerReceita, psicotropico bool, classeATC *string, stockMinimo int) error {
	return m.aplicarCampos(nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante, requerReceita, psicotropico, classeATC, stockMinimo)
}

func (m *Medicamento) aplicarCampos(nomeComercial, nomeGenerico, formaFarmaceutica, dosagem, viaAdministracao, fabricante string, requerReceita, psicotropico bool, classeATC *string, stockMinimo int) error {
	nomeComercial = strings.TrimSpace(nomeComercial)
	nomeGenerico = strings.TrimSpace(nomeGenerico)
	formaFarmaceutica = strings.TrimSpace(formaFarmaceutica)
	dosagem = strings.TrimSpace(dosagem)
	viaAdministracao = strings.TrimSpace(viaAdministracao)
	if nomeComercial == "" || nomeGenerico == "" {
		return erros.Novo(erros.CategoriaValidacao, "nome comercial e genérico do medicamento são obrigatórios")
	}
	if formaFarmaceutica == "" || dosagem == "" || viaAdministracao == "" {
		return erros.Novo(erros.CategoriaValidacao, "forma farmacêutica, dosagem e via de administração são obrigatórias")
	}
	if stockMinimo < 0 {
		return erros.Novo(erros.CategoriaValidacao, "stock mínimo não pode ser negativo")
	}
	m.nomeComercial = nomeComercial
	m.nomeGenerico = nomeGenerico
	m.formaFarmaceutica = formaFarmaceutica
	m.dosagem = dosagem
	m.viaAdministracao = viaAdministracao
	m.fabricante = strings.TrimSpace(fabricante)
	m.requerReceita = requerReceita
	m.psicotropico = psicotropico
	m.classeATC = normalizarOpcional(classeATC)
	m.stockMinimo = stockMinimo
	return nil
}

// Activar/Desactivar alternam a disponibilidade do medicamento no catálogo.
func (m *Medicamento) Activar()    { m.activo = true }
func (m *Medicamento) Desactivar() { m.activo = false }

// CorrespondeSubstancia indica se a substância (case-insensitive, aparada) está
// contida no nome genérico ou comercial — heurística de validação de alergias.
func (m *Medicamento) CorrespondeSubstancia(substancia string) bool {
	s := strings.ToLower(strings.TrimSpace(substancia))
	if s == "" {
		return false
	}
	return strings.Contains(strings.ToLower(m.nomeGenerico), s) ||
		strings.Contains(strings.ToLower(m.nomeComercial), s)
}

// ID devolve o identificador atribuído pela base de dados (vazio se não persistido).
func (m *Medicamento) ID() string { return m.id }

// CodigoInterno devolve o código interno (MED-NNNNN).
func (m *Medicamento) CodigoInterno() string { return m.codigoInterno }

// Activo indica se o medicamento está activo no catálogo.
func (m *Medicamento) Activo() bool { return m.activo }

// normalizarOpcional apara espaços e devolve nil se o resultado for vazio.
func normalizarOpcional(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}

// SnapshotMedicamento carrega o estado completo para persistência/rehidratação.
type SnapshotMedicamento struct {
	ID                string
	CodigoInterno     string
	NomeComercial     string
	NomeGenerico      string
	FormaFarmaceutica string
	Dosagem           string
	ViaAdministracao  string
	Fabricante        string
	RequerReceita     bool
	Psicotropico      bool
	ClasseATC         *string
	StockMinimo       int
	Activo            bool
	CriadoEm          time.Time
	ActualizadoEm     time.Time
}

// Snapshot devolve o estado completo do agregado.
func (m *Medicamento) Snapshot() SnapshotMedicamento {
	return SnapshotMedicamento{
		ID: m.id, CodigoInterno: m.codigoInterno, NomeComercial: m.nomeComercial,
		NomeGenerico: m.nomeGenerico, FormaFarmaceutica: m.formaFarmaceutica, Dosagem: m.dosagem,
		ViaAdministracao: m.viaAdministracao, Fabricante: m.fabricante, RequerReceita: m.requerReceita,
		Psicotropico: m.psicotropico, ClasseATC: m.classeATC, StockMinimo: m.stockMinimo,
		Activo: m.activo, CriadoEm: m.criadoEm, ActualizadoEm: m.actualizadoEm,
	}
}

// ReconstruirMedicamento reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirMedicamento(s SnapshotMedicamento) *Medicamento {
	return &Medicamento{
		id: s.ID, codigoInterno: s.CodigoInterno, nomeComercial: s.NomeComercial,
		nomeGenerico: s.NomeGenerico, formaFarmaceutica: s.FormaFarmaceutica, dosagem: s.Dosagem,
		viaAdministracao: s.ViaAdministracao, fabricante: s.Fabricante, requerReceita: s.RequerReceita,
		psicotropico: s.Psicotropico, classeATC: s.ClasseATC, stockMinimo: s.StockMinimo,
		activo: s.Activo, criadoEm: s.CriadoEm, actualizadoEm: s.ActualizadoEm,
	}
}
